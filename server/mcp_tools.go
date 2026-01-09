package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type MCPToolDefinition struct {
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	InputSchema     map[string]any `json:"input_schema"`
	IntegrationName string         `json:"integration_name"`
	MCPServerUUID   string         `json:"mcp_server_uuid"`
	NeedsApproval   bool           `json:"needs_approval"`
}

type MCPToolsCache struct {
	tools      []MCPToolDefinition
	mu         sync.RWMutex
	lastUpdate time.Time
}

var mcpToolsCache = &MCPToolsCache{
	tools: make([]MCPToolDefinition, 0),
}

func (m *MCPManager) GetAllMCPTools() []map[string]any {
	allTools := make([]map[string]any, 0)
	localTools := m.getLocalMCPTools()
	allTools = append(allTools, localTools...)
	remoteTools, err := GetRemoteMCPToolsViaBootstrap(m.config)
	if err != nil {
		if m.config.Debug {
			DebugLog("Failed to get remote MCP tools: %v", err)
		}
	} else {
		for _, tool := range remoteTools {
			allTools = append(allTools, map[string]any{
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
	allTools = append(allTools, map[string]any{
		"type": "web_search_v0",
		"name": "web_search",
	})
	allTools = append(allTools, map[string]any{
		"type": "artifacts_v0",
		"name": "artifacts",
	})
	if m.config.Debug {
		DebugLog("Total MCP tools: %d (local: %d, remote: %d, built-in: 2)",
			len(allTools), len(localTools), len(remoteTools))
	}
	return allTools
}

func (m *MCPManager) getLocalMCPTools() []map[string]any {
	mcpToolsCache.mu.RLock()
	if time.Since(mcpToolsCache.lastUpdate) < 5*time.Minute && len(mcpToolsCache.tools) > 0 {
		tools := make([]map[string]any, 0, len(mcpToolsCache.tools))
		for _, tool := range mcpToolsCache.tools {
			tools = append(tools, map[string]any{
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
	m.refreshMCPTools()
	mcpToolsCache.mu.RLock()
	defer mcpToolsCache.mu.RUnlock()
	tools := make([]map[string]any, 0, len(mcpToolsCache.tools))
	for _, tool := range mcpToolsCache.tools {
		tools = append(tools, map[string]any{
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

func (m *MCPManager) refreshMCPTools() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	allTools := make([]MCPToolDefinition, 0)
	for id, connector := range m.connectors {
		if !connector.Enabled || !connector.IsConnected {
			continue
		}
		tools, err := m.getToolsFromMCPServer(id, connector)
		if err != nil {
			if m.config.Debug {
				DebugLog("Failed to get tools from MCP server %s: %v", connector.Name, err)
			}
			continue
		}
		allTools = append(allTools, tools...)
	}
	mcpToolsCache.mu.Lock()
	mcpToolsCache.tools = allTools
	mcpToolsCache.lastUpdate = time.Now()
	mcpToolsCache.mu.Unlock()
	if m.config.Debug {
		DebugLog("Refreshed MCP tools cache: %d tools from %d servers", len(allTools), len(m.connectors))
	}
}

func (m *MCPManager) getToolsFromMCPServer(serverID string, connector *MCPConnector) ([]MCPToolDefinition, error) {
	wsURL := fmt.Sprintf("wss://claude.ai/api/ws/organizations/%s/mcp/servers/%s/",
		m.config.GetOrganizationID(), serverID)
	dialer := websocket.DefaultDialer
	if m.config.Proxy.Enable {
	}
	headers := make(map[string][]string)
	headers["Cookie"] = []string{m.config.GetCookie()}
	headers["Origin"] = []string{"https://claude.ai"}
	headers["User-Agent"] = []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"}
	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server via WebSocket: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	listToolsRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	if err := conn.WriteJSON(listToolsRequest); err != nil {
		return nil, fmt.Errorf("failed to send tools/list request: %v", err)
	}
	var response map[string]any
	if err := conn.ReadJSON(&response); err != nil {
		return nil, fmt.Errorf("failed to read tools/list response: %v", err)
	}
	if result, ok := response["result"].(map[string]any); ok {
		if toolsList, ok := result["tools"].([]any); ok {
			tools := make([]MCPToolDefinition, 0, len(toolsList))
			for _, toolData := range toolsList {
				if toolMap, ok := toolData.(map[string]any); ok {
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
					if schema, ok := toolMap["inputSchema"].(map[string]any); ok {
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
