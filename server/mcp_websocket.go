package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleMCPWebSocket(c *gin.Context) {
	orgID := c.Param("org_id")
	serverID := c.Param("server_id")
	if globalConfig.Debug {
		DebugLog("📡 MCP WebSocket connection request - OrgID: %s, ServerID: %s", orgID, serverID)
	}
	var mcpServerURL string
	var mcpServerName string
	if mcpManager != nil {
		connector, err := mcpManager.GetConnector(serverID)
		if err != nil {
			log.Printf("❌ MCP connector not found: %s", serverID)
			c.JSON(http.StatusNotFound, gin.H{"error": "MCP connector not found"})
			return
		}
		mcpServerURL = connector.URL
		mcpServerName = connector.Name
	} else {
		log.Printf("❌ MCP manager not initialized")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP manager not initialized"})
		return
	}
	log.Printf("🔌 Establishing MCP WebSocket proxy: %s (%s)", mcpServerName, serverID)
	log.Printf("   Client: wss://claude.ai/api/ws/organizations/%s/mcp/servers/%s/", orgID, serverID)
	log.Printf("   Server: %s (HTTP POST)", mcpServerURL)
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ Failed to upgrade connection: %v", err)
		return
	}
	defer clientConn.Close()
	log.Printf("✅ WebSocket connection established for MCP server: %s", mcpServerName)
	session := &MCPWebSocketSession{
		clientConn:    clientConn,
		mcpServerURL:  mcpServerURL,
		mcpServerName: mcpServerName,
		serverID:      serverID,
		config:        globalConfig,
	}
	session.Handle()
}

type MCPWebSocketSession struct {
	clientConn    *websocket.Conn
	mcpServerURL  string
	mcpServerName string
	serverID      string
	config        *Config
	mu            sync.Mutex
}

func (s *MCPWebSocketSession) Handle() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ MCP WebSocket session panic: %v", r)
		}
		log.Printf("🔌 MCP WebSocket session closed: %s", s.mcpServerName)
	}()
	for {
		messageType, message, err := s.clientConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("⚠️  MCP WebSocket unexpected close: %v", err)
			} else if s.config.Debug {
				DebugLog("MCP WebSocket connection closed")
			}
			break
		}
		if messageType != websocket.TextMessage {
			log.Printf("⚠️  Received non-text message type: %d", messageType)
			continue
		}
		var jsonRPCRequest map[string]any
		if err := json.Unmarshal(message, &jsonRPCRequest); err != nil {
			log.Printf("❌ Failed to parse JSON-RPC request: %v", err)
			s.sendError(0, -32700, "Parse error", nil)
			continue
		}
		if s.config.Debug {
			s.logRequest(jsonRPCRequest, message)
		}
		response, err := s.forwardToMCPServer(message)
		if err != nil {
			log.Printf("❌ Failed to forward request to MCP server: %v", err)
			requestID := s.getRequestID(jsonRPCRequest)
			s.sendError(requestID, -32603, fmt.Sprintf("Internal error: %v", err), nil)
			continue
		}
		if err := s.clientConn.WriteMessage(websocket.TextMessage, response); err != nil {
			log.Printf("❌ Failed to send response to client: %v", err)
			break
		}
		if s.config.Debug {
			s.logResponse(response)
		}
	}
}

func (s *MCPWebSocketSession) forwardToMCPServer(message []byte) ([]byte, error) {
	var jsonRPCRequest map[string]any
	if err := json.Unmarshal(message, &jsonRPCRequest); err != nil {
		return nil, fmt.Errorf("parse request failed: %v", err)
	}
	method, _ := jsonRPCRequest["method"].(string)
	requestID := s.getRequestID(jsonRPCRequest)
	client := s.config.CreateHTTPClient(60 * time.Second)
	req, err := http.NewRequest("POST", s.mcpServerURL, bytes.NewReader(message))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Claude-Adapter/1.0")
	if s.config.GetCookie() != "" {
		req.Header.Set("Cookie", s.config.GetCookie())
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ MCP server returned error status: %d, body: %s", resp.StatusCode, truncateString(string(responseBody), 200))
		errorResp := map[string]any{
			"jsonrpc": "2.0",
			"id":      requestID,
			"error": map[string]any{
				"code":    -32000,
				"message": fmt.Sprintf("MCP server error: status %d", resp.StatusCode),
				"data":    truncateString(string(responseBody), 500),
			},
		}
		return json.Marshal(errorResp)
	}
	log.Printf("✅ MCP server response for method=%s, id=%v, status=%d", method, requestID, resp.StatusCode)
	return responseBody, nil
}

func (s *MCPWebSocketSession) logRequest(jsonRPC map[string]any, message []byte) {
	method, _ := jsonRPC["method"].(string)
	id := jsonRPC["id"]
	params, _ := jsonRPC["params"].(map[string]any)
	if method == "tools/call" && params != nil {
		toolName, _ := params["name"].(string)
		log.Printf("→ Client to Server [%v] tools/call: %s", id, toolName)
		if args, ok := params["arguments"].(map[string]any); ok {
			argsJSON, _ := json.Marshal(args)
			DebugLog("   Arguments: %s", truncateString(string(argsJSON), 200))
		}
	} else if method != "" {
		log.Printf("→ Client to Server [%v] %s", id, method)
	} else {
		DebugLog("→ Client to Server: %s", truncateString(string(message), 200))
	}
}

func (s *MCPWebSocketSession) logResponse(response []byte) {
	var jsonRPC map[string]any
	if err := json.Unmarshal(response, &jsonRPC); err != nil {
		DebugLog("← Server to Client [RAW]: %s", truncateString(string(response), 200))
		return
	}
	id := jsonRPC["id"]
	if result, ok := jsonRPC["result"]; ok {
		if resultMap, ok := result.(map[string]any); ok {
			if tools, ok := resultMap["tools"].([]any); ok {
				log.Printf("← Server to Client [%v] result: tools/list (%d tools)", id, len(tools))
			} else if content, ok := resultMap["content"].([]any); ok {
				log.Printf("← Server to Client [%v] result: tools/call (%d content items)", id, len(content))
			} else {
				log.Printf("← Server to Client [%v] result", id)
			}
		} else {
			log.Printf("← Server to Client [%v] result", id)
		}
	} else if err, ok := jsonRPC["error"]; ok {
		log.Printf("← Server to Client [%v] error: %v", id, err)
	} else if method, ok := jsonRPC["method"].(string); ok {
		log.Printf("← Server to Client [%v] %s", id, method)
	}
}

func (s *MCPWebSocketSession) getRequestID(jsonRPC map[string]any) any {
	if id, ok := jsonRPC["id"]; ok {
		return id
	}
	return 0
}

func (s *MCPWebSocketSession) sendError(id any, code int, message string, data any) {
	errorResp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	if data != nil {
		errorResp["error"].(map[string]any)["data"] = data
	}
	responseJSON, _ := json.Marshal(errorResp)
	s.clientConn.WriteMessage(websocket.TextMessage, responseJSON)
}
