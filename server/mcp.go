package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type MCPManager struct {
	connectors map[string]*MCPConnector
	mu         sync.RWMutex
	config     *Config
}

type MCPConnector struct {
	Name          string
	UUID          string
	Enabled       bool
	ID            string
	URL           string
	AuthType      string
	AutoConnect   bool
	AutoEnableAll bool
	IsConnected   bool
	LastError     string
	ConnectedAt   time.Time
	mu            sync.RWMutex
}

var mcpManager *MCPManager

func InitMCPManager(config *Config) {
	mcpManager = &MCPManager{
		connectors: make(map[string]*MCPConnector),
		config:     config,
	}
	for _, cfg := range config.MCPConnectors {
		connector := &MCPConnector{
			Name:        cfg.Name,
			UUID:        cfg.UUID,
			Enabled:     cfg.Enabled,
			IsConnected: false,
		}
		mcpManager.connectors[cfg.UUID] = connector
		if config.Debug {
			DebugLog("Loaded MCP connector: %s (UUID: %s) - Enabled: %v",
				connector.Name, connector.UUID, connector.Enabled)
		}
	}
	log.Printf("MCP Manager initialized with %d connectors", len(mcpManager.connectors))
}

func (m *MCPManager) AutoConnectAll() {
	if m.config.Debug {
		DebugLog("AutoConnectAll called in client mode - no action needed")
	}
}

func (m *MCPManager) Connect(serverID string) error {
	m.mu.RLock()
	connector, exists := m.connectors[serverID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("MCP connector not found: %s", serverID)
	}
	if !connector.Enabled {
		return fmt.Errorf("MCP connector is disabled: %s", connector.Name)
	}
	connector.mu.Lock()
	connector.IsConnected = true
	connector.ConnectedAt = time.Now()
	connector.LastError = ""
	connector.mu.Unlock()
	return nil
}

func (m *MCPManager) autoEnableAllTools(serverID string) {
	if m.config.Debug {
		DebugLog("Auto-enabling all tools for MCP connector: %s", serverID)
	}
	tools := m.getAllToolsForServer(serverID)
	if len(tools) == 0 {
		if m.config.Debug {
			DebugLog("No specific tools found for server %s, using wildcard enablement", serverID)
		}
		tools = m.getDefaultToolsForServer(serverID)
	}
	if err := m.enableToolsInClaudeAPI(tools); err != nil {
		log.Printf("Failed to auto-enable tools for MCP connector %s: %v", serverID, err)
	} else {
		if m.config.Debug {
			DebugLog("Auto-enabled %d tools for MCP connector: %s", len(tools), serverID)
		}
		log.Printf("Auto-enabled all tools for MCP connector: %s", serverID)
	}
}

func (m *MCPManager) getAllToolsForServer(serverID string) []MCPTool {
	servers, err := getMCPServers(m.config.GetOrganizationID(), m.config.GetCookie())
	if err != nil {
		if m.config.Debug {
			DebugLog("Failed to get MCP servers: %v", err)
		}
		return []MCPTool{}
	}
	for _, server := range servers {
		if server.ID == serverID {
			if m.config.Debug {
				DebugLog("Found MCP server: %s (%s)", server.Name, server.ID)
			}
			return []MCPTool{}
		}
	}
	return []MCPTool{}
}

func (m *MCPManager) getDefaultToolsForServer(serverID string) []MCPTool {
	commonTools := []string{
		"execute_sql",
		"list_tables",
		"describe_table",
		"query_database",
		"create_merchant",
		"toggle_merchant",
		"list_all_entities",
		"update_merchant_amount",
		"create_invoice",
		"list_transactions",
	}
	tools := make([]MCPTool, 0, len(commonTools))
	for _, toolName := range commonTools {
		tools = append(tools, MCPTool{
			ServerID: serverID,
			Name:     toolName,
			ToolID:   fmt.Sprintf("%s:%s", serverID, toolName),
		})
	}
	return tools
}

func (m *MCPManager) enableToolsInClaudeAPI(tools []MCPTool) error {
	if len(tools) == 0 {
		return nil
	}
	enabledTools := make(map[string]bool)
	for _, tool := range tools {
		enabledTools[tool.ToolID] = true
	}
	request := MCPToolEnableRequest{
		EnabledMCPTools: enabledTools,
	}
	return patchAccountSettings(m.config.GetOrganizationID(), m.config.GetCookie(), request)
}

func (m *MCPManager) EnsureAllMCPToolsEnabled() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var wg sync.WaitGroup
	errors := make(chan error, len(m.connectors))
	for id, connector := range m.connectors {
		if !connector.Enabled {
			continue
		}
		wg.Add(1)
		go func(id string, c *MCPConnector) {
			defer wg.Done()
			c.mu.RLock()
			isConnected := c.IsConnected
			c.mu.RUnlock()
			if !isConnected {
				if m.config.Debug {
					DebugLog("MCP connector %s not connected, attempting to connect...", c.Name)
				}
				if err := m.Connect(id); err != nil {
					errors <- fmt.Errorf("failed to connect to %s: %v", c.Name, err)
					return
				}
				time.Sleep(500 * time.Millisecond)
			}
			if m.config.Debug {
				DebugLog("MCP connector %s ready (client mode)", c.Name)
			}
		}(id, connector)
	}
	wg.Wait()
	close(errors)
	var lastError error
	for err := range errors {
		log.Printf("MCP enablement error: %v", err)
		lastError = err
	}
	if lastError != nil && m.config.Debug {
		DebugLog("Some MCP connectors failed to enable, but continuing...")
	}
	return nil
}

func (m *MCPManager) Disconnect(serverID string) error {
	m.mu.RLock()
	connector, exists := m.connectors[serverID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("MCP connector not found: %s", serverID)
	}
	connector.mu.Lock()
	defer connector.mu.Unlock()
	if !connector.IsConnected {
		return fmt.Errorf("MCP connector not connected: %s", connector.Name)
	}
	connector.IsConnected = false
	connector.ConnectedAt = time.Time{}
	if m.config.Debug {
		DebugLog("Disconnected from MCP connector: %s (%s)", connector.Name, serverID)
	}
	return nil
}

func (m *MCPManager) GetRemoteServers() []MCPRemoteServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	servers := make([]MCPRemoteServer, 0, len(m.connectors))
	for _, connector := range m.connectors {
		connector.mu.RLock()
		server := MCPRemoteServer{
			ID:          connector.ID,
			Name:        connector.Name,
			URL:         connector.URL,
			AuthType:    connector.AuthType,
			IsConnected: connector.IsConnected,
			Enabled:     connector.Enabled,
		}
		connector.mu.RUnlock()
		servers = append(servers, server)
	}
	return servers
}

func (m *MCPManager) GetConnector(serverID string) (*MCPConnector, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	connector, exists := m.connectors[serverID]
	if !exists {
		return nil, fmt.Errorf("MCP connector not found: %s", serverID)
	}
	return connector, nil
}

func (m *MCPManager) StartAuth(serverID, redirectURL string, openInBrowser bool) (*MCPAuthResponse, error) {
	m.mu.RLock()
	connector, exists := m.connectors[serverID]
	m.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("MCP connector not found: %s", serverID)
	}
	if connector.AuthType == "none" {
		return nil, fmt.Errorf("MCP connector does not require authentication: %s", connector.Name)
	}
	authURL := fmt.Sprintf("%s/auth?redirect=%s", connector.URL, redirectURL)
	if m.config.Debug {
		DebugLog("Starting auth for MCP connector: %s, URL: %s", connector.Name, authURL)
	}
	return &MCPAuthResponse{
		AuthURL: authURL,
		Status:  "pending",
	}, nil
}
