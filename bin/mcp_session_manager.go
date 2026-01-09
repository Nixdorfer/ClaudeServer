package main

import (
	"fmt"
	"log"
	"sync"
)

// MCPSessionManager 管理所有MCP连接会话
type MCPSessionManager struct {
	config      *Config
	sessions    map[string]*MCPConnection // UUID -> Connection
	tools       []map[string]interface{}  // 所有MCP工具的缓存
	mu          sync.RWMutex
	initialized bool
	initMutex   sync.Mutex
}

var globalMCPSessionManager *MCPSessionManager

// InitMCPSessionManager 初始化全局MCP会话管理器
func InitMCPSessionManager(config *Config) {
	globalMCPSessionManager = &MCPSessionManager{
		config:      config,
		sessions:    make(map[string]*MCPConnection),
		tools:       make([]map[string]interface{}, 0),
		initialized: false,
	}
}

// EnsureInitialized 确保所有启用的MCP已经初始化
func (m *MCPSessionManager) EnsureInitialized() error {
	m.initMutex.Lock()
	defer m.initMutex.Unlock()

	// 如果已经初始化，直接返回
	if m.initialized {
		return nil
	}

	log.Printf("🔧 Initializing MCP sessions...")

	// 检查是否有启用的MCP连接器
	enabledConnectors := make([]MCPConnectorConfig, 0)
	for _, connector := range m.config.MCPConnectors {
		if connector.Enabled {
			enabledConnectors = append(enabledConnectors, connector)
		}
	}

	if len(enabledConnectors) == 0 {
		log.Printf("ℹ️  No enabled MCP connectors found")
		m.initialized = true
		return nil
	}

	log.Printf("📋 Found %d enabled MCP connector(s)", len(enabledConnectors))

	// 创建MCP客户端
	client := NewMCPClient(
		m.config,
		m.config.GetOrganizationID(),
		m.config.GetSessionKey(),
		m.config.GetCookie(),
	)

	// 为每个启用的连接器建立连接
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(enabledConnectors))
	allTools := make([][]map[string]interface{}, len(enabledConnectors))

	for i, connector := range enabledConnectors {
		wg.Add(1)
		go func(idx int, conn MCPConnectorConfig) {
			defer wg.Done()

			log.Printf("🔌 Connecting to MCP: %s (%s)", conn.Name, conn.UUID)

			// 连接到MCP服务器
			mcpConn, err := client.ConnectToServer(conn.UUID)
			if err != nil {
				errorsChan <- fmt.Errorf("failed to connect to %s: %v", conn.Name, err)
				return
			}

			// 初始化MCP（阶段3）
			if err := mcpConn.Initialize(); err != nil {
				mcpConn.Close()
				errorsChan <- fmt.Errorf("failed to initialize %s: %v", conn.Name, err)
				return
			}

			// 获取工具列表（阶段4）
			tools, err := mcpConn.GetToolsForCompletion()
			if err != nil {
				mcpConn.Close()
				errorsChan <- fmt.Errorf("failed to get tools from %s: %v", conn.Name, err)
				return
			}

			log.Printf("✅ MCP '%s' ready with %d tools", conn.Name, len(tools))

			// 保存连接和工具
			m.mu.Lock()
			m.sessions[conn.UUID] = mcpConn
			allTools[idx] = tools
			m.mu.Unlock()

		}(i, connector)
	}

	// 等待所有连接完成
	wg.Wait()
	close(errorsChan)

	// 检查是否有错误
	var errors []error
	for err := range errorsChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		// 关闭所有已建立的连接
		m.mu.Lock()
		for _, conn := range m.sessions {
			conn.Close()
		}
		m.sessions = make(map[string]*MCPConnection)
		m.mu.Unlock()

		return fmt.Errorf("MCP initialization failed: %v", errors)
	}

	// 合并所有工具
	m.mu.Lock()
	for _, tools := range allTools {
		m.tools = append(m.tools, tools...)
	}
	m.initialized = true
	m.mu.Unlock()

	log.Printf("🎉 MCP initialization complete! Total tools: %d", len(m.tools))

	return nil
}

// GetAllTools 获取所有MCP工具列表
func (m *MCPSessionManager) GetAllTools() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 返回工具副本
	tools := make([]map[string]interface{}, len(m.tools))
	copy(tools, m.tools)
	return tools
}

// CallTool 调用指定的MCP工具
func (m *MCPSessionManager) CallTool(serverUUID, toolName string, arguments map[string]interface{}) (map[string]interface{}, error) {
	m.mu.RLock()
	conn, exists := m.sessions[serverUUID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("MCP connection not found: %s", serverUUID)
	}

	return conn.CallTool(toolName, arguments)
}

// Shutdown 关闭所有MCP连接
func (m *MCPSessionManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("🔌 Shutting down MCP sessions...")

	for uuid, conn := range m.sessions {
		if err := conn.Close(); err != nil {
			log.Printf("⚠️  Error closing MCP connection %s: %v", uuid, err)
		}
	}

	m.sessions = make(map[string]*MCPConnection)
	m.tools = make([]map[string]interface{}, 0)
	m.initialized = false
}

// GetToolsForRequest 获取用于completion请求的工具列表（包含内置工具）
func (m *MCPSessionManager) GetToolsForRequest() []map[string]interface{} {
	tools := m.GetAllTools()

	// 添加内置工具
	builtinTools := []map[string]interface{}{
		{"type": "web_search_v0", "name": "web_search"},
		{"type": "artifacts_v0", "name": "artifacts"},
	}

	return append(tools, builtinTools...)
}

// GetStatus 获取MCP会话管理器状态
func (m *MCPSessionManager) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]map[string]interface{}, 0)
	for uuid, conn := range m.sessions {
		sessions = append(sessions, map[string]interface{}{
			"uuid":        uuid,
			"server_name": conn.serverInfo.Name,
			"connected":   true,
		})
	}

	return map[string]interface{}{
		"initialized":    m.initialized,
		"total_tools":    len(m.tools),
		"total_sessions": len(m.sessions),
		"sessions":       sessions,
	}
}
