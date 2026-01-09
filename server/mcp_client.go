package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type MCPClient struct {
	config      *Config
	orgID       string
	sessionKey  string
	cookie      string
	deviceID    string
	anonymousID string
	httpClient  *http.Client
	mu          sync.Mutex
}

type MCPServerInfo struct {
	UUID                        string `json:"uuid"`
	Name                        string `json:"name"`
	URL                         string `json:"url"`
	CreatedAt                   string `json:"created_at"`
	UpdatedAt                   string `json:"updated_at"`
	CustomOAuthClientID         any    `json:"custom_oauth_client_id"`
	HasCustomOAuthCredentials   bool   `json:"has_custom_oauth_credentials"`
	CustomOAuthClientSecretMask any    `json:"custom_oauth_client_secret_mask"`
}

type MCPConnection struct {
	client     *MCPClient
	serverInfo *MCPServerInfo
	conn       *websocket.Conn
	requestID  int
	mu         sync.Mutex
}

func NewMCPClient(config *Config, orgID, sessionKey, cookie string) *MCPClient {
	return &MCPClient{
		config:      config,
		orgID:       orgID,
		sessionKey:  sessionKey,
		cookie:      cookie,
		deviceID:    "bf4867fd-1cbf-4778-933f-c5d8eb8aa670",
		anonymousID: "claudeai.v1.d5a52e83-fad9-435c-8c21-6d4e1fe76116",
		httpClient:  config.CreateHTTPClient(30 * time.Second),
	}
}

func (c *MCPClient) GetRemoteServers() ([]MCPServerInfo, error) {
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers", c.orgID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %v", err)
	}
	c.setCommonHeaders(req)
	log.Printf("Fetching MCP servers from claude.ai...")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	var servers []MCPServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, fmt.Errorf("decode response failed: %v", err)
	}
	log.Printf("Found %d MCP servers", len(servers))
	for _, srv := range servers {
		log.Printf("   - %s (%s) at %s", srv.Name, srv.UUID, srv.URL)
	}
	return servers, nil
}

func (c *MCPClient) ConnectToServer(serverUUID string) (*MCPConnection, error) {
	servers, err := c.GetRemoteServers()
	if err != nil {
		return nil, fmt.Errorf("get servers failed: %v", err)
	}
	var serverInfo *MCPServerInfo
	for _, srv := range servers {
		if srv.UUID == serverUUID {
			serverInfo = &srv
			break
		}
	}
	if serverInfo == nil {
		return nil, fmt.Errorf("server %s not found", serverUUID)
	}
	wsURL := fmt.Sprintf("wss://claude.ai/api/ws/organizations/%s/mcp/servers/%s/",
		c.orgID, serverUUID)
	log.Printf("Connecting to MCP server via WebSocket...")
	log.Printf("   Server: %s (%s)", serverInfo.Name, serverUUID)
	log.Printf("   URL: %s", wsURL)
	fullCookie := c.cookie
	if c.config.Debug {
		DebugLog("Analyzing Cookie content:")
		cookieParts := strings.Split(fullCookie, ";")
		DebugLog("  Total cookie parts: %d", len(cookieParts))
		for i, part := range cookieParts {
			part = strings.TrimSpace(part)
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				key := kv[0]
				value := kv[1]
				if len(value) > 15 {
					DebugLog("  [%d] %s: %s...[%d chars]", i+1, key, value[:15], len(value))
				} else {
					DebugLog("  [%d] %s: [%d chars]", i+1, key, len(value))
				}
			}
		}
		hasCfClearance := strings.Contains(fullCookie, "cf_clearance=")
		hasCfBm := strings.Contains(fullCookie, "__cf_bm=")
		hasSessionKey := strings.Contains(fullCookie, "sessionKey=")
		DebugLog("Critical Cookie fields check:")
		DebugLog("  sessionKey: %v", hasSessionKey)
		DebugLog("  cf_clearance: %v", hasCfClearance)
		DebugLog("  __cf_bm: %v", hasCfBm)
		if !hasCfClearance || !hasCfBm {
			log.Printf("Warning: Cookie is missing Cloudflare verification fields")
			if !hasCfClearance {
				log.Printf("   - Missing cf_clearance")
			}
			if !hasCfBm {
				log.Printf("   - Missing __cf_bm")
			}
			log.Printf("   MCP WebSocket connection may fail")
		}
	}
	header := http.Header{}
	header.Set("Origin", "https://claude.ai")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	header.Set("Cookie", fullCookie)
	header.Set("Sec-WebSocket-Protocol", "mcp")
	header.Set("Cache-Control", "no-cache")
	header.Set("Pragma", "no-cache")
	header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if c.config.Debug {
		DebugLog("WebSocket Request Headers:")
		for key, values := range header {
			for _, value := range values {
				if key == "Cookie" && len(value) > 20 {
					DebugLog("  %s: %s...[%d chars]", key, value[:20], len(value))
				} else {
					DebugLog("  %s: %s", key, value)
				}
			}
		}
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: 45 * time.Second,
	}
	if c.config.Proxy.Enable {
		if c.config.Proxy.HTTPS != "" || c.config.Proxy.HTTP != "" {
			dialer.Proxy = func(req *http.Request) (*url.URL, error) {
				if c.config.Proxy.HTTPS != "" {
					return url.Parse(c.config.Proxy.HTTPS)
				}
				return url.Parse(c.config.Proxy.HTTP)
			}
			if c.config.Debug {
				DebugLog("WebSocket will use proxy: %s",
					func() string {
						if c.config.Proxy.HTTPS != "" {
							return c.config.Proxy.HTTPS
						}
						return c.config.Proxy.HTTP
					}())
			}
		}
	} else {
		dialer.Proxy = http.ProxyFromEnvironment
	}
	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			log.Printf("WebSocket handshake failed with status %d", resp.StatusCode)
			log.Printf("Error details: %v", err)
			if c.config.Debug {
				DebugLog("Response Status: %s", resp.Status)
				DebugLog("Response Headers:")
				for key, values := range resp.Header {
					for _, value := range values {
						DebugLog("  %s: %s", key, value)
					}
				}
				if resp.Body != nil {
					bodyBytes, readErr := io.ReadAll(resp.Body)
					if readErr == nil && len(bodyBytes) > 0 {
						DebugLog("Response Body: %s", truncateString(string(bodyBytes), 500))
					}
				}
			}
			switch resp.StatusCode {
			case 302:
				log.Printf("Hint: Received 302 redirect, this is usually due to missing Cloudflare verification token")
				log.Printf("   Please ensure you have added to the tokens section in config:")
				log.Printf("   - cf_clearance")
				log.Printf("   - cf_bm")
			case 401, 403:
				log.Printf("Hint: Authentication failed, please check these settings:")
				log.Printf("   - Is organization_id correct")
				log.Printf("   - Is session_key valid")
				log.Printf("   - Are other Cookie fields complete")
			}
			return nil, fmt.Errorf("websocket dial failed (status %d): %v", resp.StatusCode, err)
		}
		log.Printf("WebSocket connection failed: %v", err)
		log.Printf("Hint: If this is a network error, please check:")
		log.Printf("   - Is network connection working")
		log.Printf("   - Is proxy configured correctly (if using proxy)")
		return nil, fmt.Errorf("websocket dial failed: %v", err)
	}
	log.Printf("WebSocket connected to claude.ai")
	if c.config.Debug {
		DebugLog("WebSocket connection established successfully")
		DebugLog("Response Status: %s", resp.Status)
	}
	conn.SetCloseHandler(func(code int, text string) error {
		log.Printf("WebSocket close handler triggered:")
		log.Printf("   Close code: %d", code)
		if text != "" {
			log.Printf("   Close reason: %s", text)
		}
		return nil
	})
	conn.SetPongHandler(func(appData string) error {
		if c.config.Debug {
			DebugLog("Received pong: %s", appData)
		}
		return nil
	})
	if c.config.Debug {
		DebugLog("WebSocket handlers configured")
	}
	mcpConn := &MCPConnection{
		client:     c,
		serverInfo: serverInfo,
		conn:       conn,
		requestID:  0,
	}
	return mcpConn, nil
}

func (c *MCPClient) setCommonHeaders(req *http.Request) {
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("anthropic-client-platform", "web_claude_ai")
	req.Header.Set("anthropic-client-version", "1.0.0")
	req.Header.Set("anthropic-device-id", c.deviceID)
	req.Header.Set("anthropic-anonymous-id", c.anonymousID)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("Referer", "https://claude.ai/")
	req.Header.Set("Origin", "https://claude.ai")
}

func (conn *MCPConnection) Initialize() error {
	if conn.client.config.Debug {
		DebugLog("Checking if server sends any initial messages...")
	}
	conn.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, initialMsg, err := conn.conn.ReadMessage()
	if err == nil {
		log.Printf("Server sent initial message (%d bytes)", len(initialMsg))
		if conn.client.config.Debug {
			DebugLog("Initial message: %s", truncateString(string(initialMsg), 500))
		}
		var response map[string]any
		if err := json.Unmarshal(initialMsg, &response); err == nil {
			if method, ok := response["method"].(string); ok {
				log.Printf("Initial message method: %s", method)
			}
		}
	} else {
		if conn.client.config.Debug {
			DebugLog("No initial message from server (timeout/error: %v)", err)
		}
	}
	conn.conn.SetReadDeadline(time.Time{})
	conn.mu.Lock()
	requestID := conn.requestID
	conn.requestID++
	conn.mu.Unlock()
	initReq := map[string]any{
		"method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "claude-ai",
				"version": "0.1.0",
			},
		},
		"jsonrpc": "2.0",
		"id":      requestID,
	}
	log.Printf("Sending initialize request (id=%d)", requestID)
	if conn.client.config.Debug {
		reqJSON, _ := json.MarshalIndent(initReq, "", "  ")
		DebugLog("Initialize request payload:\n%s", string(reqJSON))
	}
	if conn.client.config.Debug {
		DebugLog("Sending initialize via WriteJSON...")
	}
	if err := conn.conn.WriteJSON(initReq); err != nil {
		log.Printf("Failed to send initialize request: %v", err)
		return fmt.Errorf("send initialize failed: %v", err)
	}
	if conn.client.config.Debug {
		DebugLog("Initialize request sent successfully, waiting for response...")
	}
	for i := 0; i < 2; i++ {
		if conn.client.config.Debug {
			DebugLog("Waiting for message %d/2...", i+1)
		}
		conn.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, message, err := conn.conn.ReadMessage()
		if err != nil {
			log.Printf("Failed to read message %d/2: %v", i+1, err)
			if conn.client.config.Debug {
				DebugLog("Read deadline was: %v", time.Now().Add(30*time.Second))
			}
			if strings.Contains(err.Error(), "1006") {
				log.Printf("Hint: WebSocket abnormally closed (1006), possible reasons:")
				log.Printf("   1. Cloudflare verification token missing or expired (cf_clearance, cf_bm)")
				log.Printf("   2. Session token (session_key) has expired")
				log.Printf("   3. Cookie is missing other required fields")
				log.Printf("   Please try:")
				log.Printf("   - Get the latest Cookie from browser again")
				log.Printf("   - Make sure to fill in all Cookie fields shown in browser")
			}
			return fmt.Errorf("read message failed: %v", err)
		}
		conn.conn.SetReadDeadline(time.Time{})
		if conn.client.config.Debug {
			DebugLog("Received message %d/2 (%d bytes)", i+1, len(message))
			DebugLog("Raw message: %s", truncateString(string(message), 500))
		}
		var response map[string]any
		if err := json.Unmarshal(message, &response); err != nil {
			log.Printf("Failed to parse response: %v", err)
			log.Printf("Raw message: %s", truncateString(string(message), 200))
			return fmt.Errorf("parse response failed: %v", err)
		}
		if method, ok := response["method"].(string); ok && method == "connected" {
			log.Printf("Received 'connected' notification")
		} else if result, ok := response["result"].(map[string]any); ok {
			log.Printf("Received initialize result")
			if serverInfo, ok := result["serverInfo"].(map[string]any); ok {
				log.Printf("   Server: %s v%s", serverInfo["name"], serverInfo["version"])
			}
			if conn.client.config.Debug && len(result) > 0 {
				resultJSON, _ := json.MarshalIndent(result, "  ", "  ")
				DebugLog("  Full result:\n  %s", string(resultJSON))
			}
		} else {
			if conn.client.config.Debug {
				DebugLog("Received unexpected response format: %v", response)
			}
		}
	}
	notifReq := map[string]any{
		"method":  "notifications/initialized",
		"jsonrpc": "2.0",
	}
	log.Printf("Sending notifications/initialized")
	if err := conn.conn.WriteJSON(notifReq); err != nil {
		return fmt.Errorf("send initialized notification failed: %v", err)
	}
	log.Printf("MCP initialization completed")
	return nil
}

func (conn *MCPConnection) ListTools() ([]any, error) {
	conn.mu.Lock()
	requestID := conn.requestID
	conn.requestID++
	conn.mu.Unlock()
	toolsReq := map[string]any{
		"method":  "tools/list",
		"params":  map[string]any{},
		"jsonrpc": "2.0",
		"id":      requestID,
	}
	log.Printf("Sending tools/list request (id=%d)", requestID)
	if err := conn.conn.WriteJSON(toolsReq); err != nil {
		return nil, fmt.Errorf("send tools/list failed: %v", err)
	}
	_, message, err := conn.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message failed: %v", err)
	}
	var response map[string]any
	if err := json.Unmarshal(message, &response); err != nil {
		return nil, fmt.Errorf("parse response failed: %v", err)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		return nil, fmt.Errorf("invalid tools format")
	}
	log.Printf("Received %d tools from MCP server", len(tools))
	for i, tool := range tools {
		if toolMap, ok := tool.(map[string]any); ok {
			name := toolMap["name"]
			desc := toolMap["description"]
			log.Printf("   %d. %s - %s", i+1, name, truncateString(fmt.Sprintf("%v", desc), 60))
		}
	}
	return tools, nil
}

func (conn *MCPConnection) CallTool(toolName string, arguments map[string]any) (map[string]any, error) {
	conn.mu.Lock()
	requestID := conn.requestID
	conn.requestID++
	conn.mu.Unlock()
	callReq := map[string]any{
		"method": "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": arguments,
		},
		"jsonrpc": "2.0",
		"id":      requestID,
	}
	log.Printf("Calling tool: %s (id=%d)", toolName, requestID)
	if argsJSON, err := json.Marshal(arguments); err == nil {
		log.Printf("   Arguments: %s", truncateString(string(argsJSON), 200))
	}
	if err := conn.conn.WriteJSON(callReq); err != nil {
		return nil, fmt.Errorf("send tools/call failed: %v", err)
	}
	_, message, err := conn.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message failed: %v", err)
	}
	var response map[string]any
	if err := json.Unmarshal(message, &response); err != nil {
		return nil, fmt.Errorf("parse response failed: %v", err)
	}
	if errObj, ok := response["error"]; ok {
		return nil, fmt.Errorf("tool call error: %v", errObj)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}
	log.Printf("Tool call completed: %s", toolName)
	if content, ok := result["content"].([]any); ok {
		log.Printf("   Result: %d content items", len(content))
		for i, item := range content {
			if itemMap, ok := item.(map[string]any); ok {
				if text, ok := itemMap["text"].(string); ok {
					log.Printf("   Content %d: %s", i+1, truncateString(text, 100))
				}
			}
		}
	}
	return result, nil
}

func (conn *MCPConnection) GetToolsForCompletion() ([]map[string]any, error) {
	tools, err := conn.ListTools()
	if err != nil {
		return nil, err
	}
	formattedTools := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if toolMap, ok := tool.(map[string]any); ok {
			formattedTool := map[string]any{
				"name":              toolMap["name"],
				"description":       toolMap["description"],
				"input_schema":      toolMap["inputSchema"],
				"integration_name":  conn.serverInfo.Name,
				"mcp_server_uuid":   conn.serverInfo.UUID,
				"mcp_server_url":    conn.serverInfo.URL,
				"needs_approval":    false,
				"backend_execution": false,
			}
			formattedTools = append(formattedTools, formattedTool)
		}
	}
	return formattedTools, nil
}

func (conn *MCPConnection) Close() error {
	if conn.conn != nil {
		return conn.conn.Close()
	}
	return nil
}

func TestMCPConnection(config *Config, orgID, sessionKey, cookie, serverUUID string) error {
	log.Printf("Starting MCP connection test...")
	log.Printf("   Organization: %s", orgID)
	log.Printf("   Server UUID: %s", serverUUID)
	client := NewMCPClient(config, orgID, sessionKey, cookie)
	conn, err := client.ConnectToServer(serverUUID)
	if err != nil {
		return fmt.Errorf("connect failed: %v", err)
	}
	defer conn.Close()
	log.Printf("\nPhase 3: MCP Initialization")
	if err := conn.Initialize(); err != nil {
		return fmt.Errorf("initialize failed: %v", err)
	}
	log.Printf("\nPhase 4: Tool Discovery")
	tools, err := conn.ListTools()
	if err != nil {
		return fmt.Errorf("list tools failed: %v", err)
	}
	log.Printf("\nMCP is now ready! Successfully discovered %d tools.", len(tools))
	log.Printf("MCP server '%s' is fully operational", conn.serverInfo.Name)
	if config.Debug {
		log.Printf("\nPhase 5 (Optional): Testing Tool Call")
		log.Printf("   Testing query_database tool with sample SQL...")
		result, err := conn.CallTool("query_database", map[string]any{
			"sql": "SELECT * FROM zm_merchant WHERE id=118;",
		})
		if err != nil {
			log.Printf("Tool call test failed (this is optional): %v", err)
		} else {
			log.Printf("Tool call test successful!")
			if resultJSON, err := json.MarshalIndent(result, "", "  "); err == nil {
				log.Printf("   Result: %s", truncateString(string(resultJSON), 500))
			}
		}
	}
	return nil
}
