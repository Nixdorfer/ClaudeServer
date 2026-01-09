package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MCPTool represents a tool provided by an MCP server
type MCPToolDefinition struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	InputSchema     map[string]interface{} `json:"input_schema"`
	IntegrationName string                 `json:"integration_name"`
	MCPServerUUID   string                 `json:"mcp_server_uuid"`
	NeedsApproval   bool                   `json:"needs_approval"`
}

// MCPToolsCache 缓存MCP工具列表
type MCPToolsCache struct {
	tools      []MCPToolDefinition
	mu         sync.RWMutex
	lastUpdate time.Time
}

var mcpToolsCache = &MCPToolsCache{
	tools: make([]MCPToolDefinition, 0),
}

// GetAllMCPTools 获取所有MCP工具（用于添加到completion请求）
func (m *MCPManager) GetAllMCPTools() []map[string]interface{} {
	allTools := make([]map[string]interface{}, 0)

	// 1. 获取本地WebSocket MCP工具
	localTools := m.getLocalMCPTools()
	allTools = append(allTools, localTools...)

	// 2. 获取远程MCP工具
	remoteTools, err := GetRemoteMCPToolsViaBootstrap(m.config)
	if err != nil {
		if m.config.Debug {
			DebugLog("Failed to get remote MCP tools: %v", err)
		}
	} else {
		for _, tool := range remoteTools {
			allTools = append(allTools, map[string]interface{}{
				"name":             tool.Name,
				"description":      tool.Description,
				"input_schema":     tool.InputSchema,
				"integration_name": tool.IntegrationName,
				"mcp_server_uuid":  tool.MCPServerUUID,
				"needs_approval":   tool.NeedsApproval,
			})
		}
		if m.config.Debug {
			DebugLog("Added %d remote MCP tools", len(remoteTools))
		}
	}

	// 3. 添加内置工具
	allTools = append(allTools, map[string]interface{}{
		"type": "web_search_v0",
		"name": "web_search",
	})
	allTools = append(allTools, map[string]interface{}{
		"type": "artifacts_v0",
		"name": "artifacts",
	})

	if m.config.Debug {
		DebugLog("Total MCP tools: %d (local: %d, remote: %d, built-in: 2)",
			len(allTools), len(localTools), len(remoteTools))
	}

	return allTools
}

// getLocalMCPTools 获取本地WebSocket MCP工具
func (m *MCPManager) getLocalMCPTools() []map[string]interface{} {
	// 检查缓存
	mcpToolsCache.mu.RLock()
	if time.Since(mcpToolsCache.lastUpdate) < 5*time.Minute && len(mcpToolsCache.tools) > 0 {
		// 使用缓存
		tools := make([]map[string]interface{}, 0, len(mcpToolsCache.tools))
		for _, tool := range mcpToolsCache.tools {
			tools = append(tools, map[string]interface{}{
				"name":             tool.Name,
				"description":      tool.Description,
				"input_schema":     tool.InputSchema,
				"integration_name": tool.IntegrationName,
				"mcp_server_uuid":  tool.MCPServerUUID,
				"needs_approval":   tool.NeedsApproval,
			})
		}
		mcpToolsCache.mu.RUnlock()
		if m.config.Debug {
			DebugLog("Using cached local MCP tools: %d tools", len(tools))
		}
		return tools
	}
	mcpToolsCache.mu.RUnlock()

	// 刷新缓存
	m.refreshMCPTools()

	// 返回工具列表
	mcpToolsCache.mu.RLock()
	defer mcpToolsCache.mu.RUnlock()

	tools := make([]map[string]interface{}, 0, len(mcpToolsCache.tools))
	for _, tool := range mcpToolsCache.tools {
		tools = append(tools, map[string]interface{}{
			"name":             tool.Name,
			"description":      tool.Description,
			"input_schema":     tool.InputSchema,
			"integration_name": tool.IntegrationName,
			"mcp_server_uuid":  tool.MCPServerUUID,
			"needs_approval":   tool.NeedsApproval,
		})
	}

	return tools
}

// refreshMCPTools 刷新MCP工具列表
func (m *MCPManager) refreshMCPTools() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	allTools := make([]MCPToolDefinition, 0)

	for id, connector := range m.connectors {
		if !connector.Enabled || !connector.IsConnected {
			continue
		}

		// 通过WebSocket获取工具列表
		tools, err := m.getToolsFromMCPServer(id, connector)
		if err != nil {
			if m.config.Debug {
				DebugLog("Failed to get tools from MCP server %s: %v", connector.Name, err)
			}
			continue
		}

		allTools = append(allTools, tools...)
	}

	// 更新缓存
	mcpToolsCache.mu.Lock()
	mcpToolsCache.tools = allTools
	mcpToolsCache.lastUpdate = time.Now()
	mcpToolsCache.mu.Unlock()

	if m.config.Debug {
		DebugLog("Refreshed MCP tools cache: %d tools from %d servers", len(allTools), len(m.connectors))
	}
}

// getToolsFromMCPServer 通过WebSocket从MCP服务器获取工具列表
func (m *MCPManager) getToolsFromMCPServer(serverID string, connector *MCPConnector) ([]MCPToolDefinition, error) {
	// 构建WebSocket URL
	wsURL := fmt.Sprintf("wss://claude.ai/api/ws/organizations/%s/mcp/servers/%s/",
		m.config.GetOrganizationID(), serverID)

	// 创建WebSocket连接
	dialer := websocket.DefaultDialer
	if m.config.Proxy.Enable {
		// TODO: 配置代理
	}

	// 设置请求头
	headers := make(map[string][]string)
	headers["Cookie"] = []string{m.config.GetCookie()}
	headers["Origin"] = []string{"https://claude.ai"}
	headers["User-Agent"] = []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"}

	// 连接WebSocket
	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server via WebSocket: %v", err)
	}
	defer conn.Close()

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// 发送MCP协议的list_tools请求
	listToolsRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	if err := conn.WriteJSON(listToolsRequest); err != nil {
		return nil, fmt.Errorf("failed to send tools/list request: %v", err)
	}

	// 读取响应
	var response map[string]interface{}
	if err := conn.ReadJSON(&response); err != nil {
		return nil, fmt.Errorf("failed to read tools/list response: %v", err)
	}

	// 解析工具列表
	if result, ok := response["result"].(map[string]interface{}); ok {
		if toolsList, ok := result["tools"].([]interface{}); ok {
			tools := make([]MCPToolDefinition, 0, len(toolsList))
			for _, toolData := range toolsList {
				if toolMap, ok := toolData.(map[string]interface{}); ok {
					tool := MCPToolDefinition{
						IntegrationName: connector.Name,
						MCPServerUUID:   serverID,
						NeedsApproval:   true,
					}

					if name, ok := toolMap["name"].(string); ok {
						tool.Name = name
					}
					if desc, ok := toolMap["description"].(string); ok {
						tool.Description = desc
					}
					if schema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
						tool.InputSchema = schema
					}

					tools = append(tools, tool)
				}
			}

			if m.config.Debug {
				log.Printf("✓ Got %d tools from MCP server: %s", len(tools), connector.Name)
			}

			return tools, nil
		}
	}

	return nil, fmt.Errorf("unexpected response format from MCP server")
}
