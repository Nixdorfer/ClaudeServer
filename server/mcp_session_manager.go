package main

import (
	"fmt"
	"log"
	"sync"
)

type MCPSessionManager struct {
	config      *Config
	sessions    map[string]*MCPConnection
	tools       []map[string]any
	mu          sync.RWMutex
	initialized bool
	initMutex   sync.Mutex
}

var globalMCPSessionManager *MCPSessionManager

func InitMCPSessionManager(config *Config) {
	globalMCPSessionManager = &MCPSessionManager{
		config:      config,
		sessions:    make(map[string]*MCPConnection),
		tools:       make([]map[string]any, 0),
		initialized: false,
	}
}

func (m *MCPSessionManager) EnsureInitialized() error {
	m.initMutex.Lock()
	defer m.initMutex.Unlock()
	if m.initialized {
		return nil
	}
	log.Printf("🔧 Initializing MCP sessions...")
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
	client := NewMCPClient(
		m.config,
		m.config.GetOrganizationID(),
		m.config.GetSessionKey(),
		m.config.GetCookie(),
	)
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(enabledConnectors))
	allTools := make([][]map[string]any, len(enabledConnectors))
	for i, connector := range enabledConnectors {
		wg.Add(1)
		go func(idx int, conn MCPConnectorConfig) {
			defer wg.Done()
			log.Printf("🔌 Connecting to MCP: %s (%s)", conn.Name, conn.UUID)
			mcpConn, err := client.ConnectToServer(conn.UUID)
			if err != nil {
				errorsChan <- fmt.Errorf("failed to connect to %s: %v", conn.Name, err)
				return
			}
			if err := mcpConn.Initialize(); err != nil {
				mcpConn.Close()
				errorsChan <- fmt.Errorf("failed to initialize %s: %v", conn.Name, err)
				return
			}
			tools, err := mcpConn.GetToolsForCompletion()
			if err != nil {
				mcpConn.Close()
				errorsChan <- fmt.Errorf("failed to get tools from %s: %v", conn.Name, err)
				return
			}
			log.Printf("✅ MCP '%s' ready with %d tools", conn.Name, len(tools))
			m.mu.Lock()
			m.sessions[conn.UUID] = mcpConn
			allTools[idx] = tools
			m.mu.Unlock()
		}(i, connector)
	}
	wg.Wait()
	close(errorsChan)
	var errors []error
	for err := range errorsChan {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		m.mu.Lock()
		for _, conn := range m.sessions {
			conn.Close()
		}
		m.sessions = make(map[string]*MCPConnection)
		m.mu.Unlock()
		return fmt.Errorf("MCP initialization failed: %v", errors)
	}
	m.mu.Lock()
	for _, tools := range allTools {
		m.tools = append(m.tools, tools...)
	}
	m.initialized = true
	m.mu.Unlock()
	log.Printf("🎉 MCP initialization complete! Total tools: %d", len(m.tools))
	return nil
}

func (m *MCPSessionManager) GetAllTools() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tools := make([]map[string]any, len(m.tools))
	copy(tools, m.tools)
	return tools
}

func (m *MCPSessionManager) CallTool(serverUUID, toolName string, arguments map[string]any) (map[string]any, error) {
	m.mu.RLock()
	conn, exists := m.sessions[serverUUID]
	m.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("MCP connection not found: %s", serverUUID)
	}
	return conn.CallTool(toolName, arguments)
}

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
	m.tools = make([]map[string]any, 0)
	m.initialized = false
}

func (m *MCPSessionManager) GetToolsForRequest() []map[string]any {
	tools := m.GetAllTools()
	builtinTools := []map[string]any{
		{"type": "web_search_v0", "name": "web_search"},
		{"type": "artifacts_v0", "name": "artifacts"},
	}
	return append(tools, builtinTools...)
}

func (m *MCPSessionManager) GetStatus() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessions := make([]map[string]any, 0)
	for uuid, conn := range m.sessions {
		sessions = append(sessions, map[string]any{
			"uuid":        uuid,
			"server_name": conn.serverInfo.Name,
			"connected":   true,
		})
	}
	return map[string]any{
		"initialized":    m.initialized,
		"total_tools":    len(m.tools),
		"total_sessions": len(m.sessions),
		"sessions":       sessions,
	}
}
