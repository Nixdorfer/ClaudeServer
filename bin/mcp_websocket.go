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
		return true // 允许所有来源
	},
}

// HandleMCPWebSocket 处理MCP WebSocket连接
// Claude前端通过WebSocket连接，我们将JSON-RPC请求转发到HTTP POST的MCP服务器
func HandleMCPWebSocket(c *gin.Context) {
	orgID := c.Param("org_id")
	serverID := c.Param("server_id")

	if globalConfig.Debug {
		DebugLog("📡 MCP WebSocket connection request - OrgID: %s, ServerID: %s", orgID, serverID)
	}

	// 查找对应的MCP连接器配置
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

	// 升级HTTP连接为WebSocket
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ Failed to upgrade connection: %v", err)
		return
	}
	defer clientConn.Close()

	log.Printf("✅ WebSocket connection established for MCP server: %s", mcpServerName)

	// 创建MCP代理会话
	session := &MCPWebSocketSession{
		clientConn:    clientConn,
		mcpServerURL:  mcpServerURL,
		mcpServerName: mcpServerName,
		serverID:      serverID,
		config:        globalConfig,
	}

	// 处理WebSocket消息
	session.Handle()
}

// MCPWebSocketSession 表示一个MCP WebSocket会话
type MCPWebSocketSession struct {
	clientConn    *websocket.Conn
	mcpServerURL  string
	mcpServerName string
	serverID      string
	config        *Config
	mu            sync.Mutex
}

// Handle 处理WebSocket消息
// 接收Claude的JSON-RPC请求，通过HTTP POST转发到MCP服务器，然后通过WebSocket返回响应
func (s *MCPWebSocketSession) Handle() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ MCP WebSocket session panic: %v", r)
		}
		log.Printf("🔌 MCP WebSocket session closed: %s", s.mcpServerName)
	}()

	for {
		// 读取来自Claude的消息
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

		// 解析JSON-RPC请求
		var jsonRPCRequest map[string]interface{}
		if err := json.Unmarshal(message, &jsonRPCRequest); err != nil {
			log.Printf("❌ Failed to parse JSON-RPC request: %v", err)
			s.sendError(0, -32700, "Parse error", nil)
			continue
		}

		if s.config.Debug {
			s.logRequest(jsonRPCRequest, message)
		}

		// 转发请求到MCP服务器（HTTP POST）
		response, err := s.forwardToMCPServer(message)
		if err != nil {
			log.Printf("❌ Failed to forward request to MCP server: %v", err)

			// 获取请求ID
			requestID := s.getRequestID(jsonRPCRequest)
			s.sendError(requestID, -32603, fmt.Sprintf("Internal error: %v", err), nil)
			continue
		}

		// 发送响应给Claude
		if err := s.clientConn.WriteMessage(websocket.TextMessage, response); err != nil {
			log.Printf("❌ Failed to send response to client: %v", err)
			break
		}

		if s.config.Debug {
			s.logResponse(response)
		}
	}
}

// forwardToMCPServer 转发JSON-RPC请求到真实的MCP服务器（HTTP POST）
func (s *MCPWebSocketSession) forwardToMCPServer(message []byte) ([]byte, error) {
	// 解析请求
	var jsonRPCRequest map[string]interface{}
	if err := json.Unmarshal(message, &jsonRPCRequest); err != nil {
		return nil, fmt.Errorf("parse request failed: %v", err)
	}

	method, _ := jsonRPCRequest["method"].(string)
	requestID := s.getRequestID(jsonRPCRequest)

	// 创建HTTP客户端
	client := s.config.CreateHTTPClient(60 * time.Second)

	// 发送POST请求到MCP服务器
	req, err := http.NewRequest("POST", s.mcpServerURL, bytes.NewReader(message))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Claude-Adapter/1.0")

	// 如果MCP服务器需要认证，添加认证头
	if s.config.GetCookie() != "" {
		req.Header.Set("Cookie", s.config.GetCookie())
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ MCP server returned error status: %d, body: %s", resp.StatusCode, truncateString(string(responseBody), 200))

		// 构造JSON-RPC错误响应
		errorResp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      requestID,
			"error": map[string]interface{}{
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

// logRequest 记录请求日志
func (s *MCPWebSocketSession) logRequest(jsonRPC map[string]interface{}, message []byte) {
	method, _ := jsonRPC["method"].(string)
	id := jsonRPC["id"]
	params, _ := jsonRPC["params"].(map[string]interface{})

	// 特殊处理tools/call
	if method == "tools/call" && params != nil {
		toolName, _ := params["name"].(string)
		log.Printf("→ Client to Server [%v] tools/call: %s", id, toolName)
		if args, ok := params["arguments"].(map[string]interface{}); ok {
			argsJSON, _ := json.Marshal(args)
			DebugLog("   Arguments: %s", truncateString(string(argsJSON), 200))
		}
	} else if method != "" {
		log.Printf("→ Client to Server [%v] %s", id, method)
	} else {
		DebugLog("→ Client to Server: %s", truncateString(string(message), 200))
	}
}

// logResponse 记录响应日志
func (s *MCPWebSocketSession) logResponse(response []byte) {
	var jsonRPC map[string]interface{}
	if err := json.Unmarshal(response, &jsonRPC); err != nil {
		DebugLog("← Server to Client [RAW]: %s", truncateString(string(response), 200))
		return
	}

	id := jsonRPC["id"]

	if result, ok := jsonRPC["result"]; ok {
		// 这是一个成功的响应
		if resultMap, ok := result.(map[string]interface{}); ok {
			// 检查是否是tools/list的响应
			if tools, ok := resultMap["tools"].([]interface{}); ok {
				log.Printf("← Server to Client [%v] result: tools/list (%d tools)", id, len(tools))
			} else if content, ok := resultMap["content"].([]interface{}); ok {
				log.Printf("← Server to Client [%v] result: tools/call (%d content items)", id, len(content))
			} else {
				log.Printf("← Server to Client [%v] result", id)
			}
		} else {
			log.Printf("← Server to Client [%v] result", id)
		}
	} else if err, ok := jsonRPC["error"]; ok {
		// 这是一个错误响应
		log.Printf("← Server to Client [%v] error: %v", id, err)
	} else if method, ok := jsonRPC["method"].(string); ok {
		// 这是一个通知或请求
		log.Printf("← Server to Client [%v] %s", id, method)
	}
}

// getRequestID 获取请求ID
func (s *MCPWebSocketSession) getRequestID(jsonRPC map[string]interface{}) interface{} {
	if id, ok := jsonRPC["id"]; ok {
		return id
	}
	return 0
}

// sendError 发送JSON-RPC错误响应
func (s *MCPWebSocketSession) sendError(id interface{}, code int, message string, data interface{}) {
	errorResp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}

	if data != nil {
		errorResp["error"].(map[string]interface{})["data"] = data
	}

	responseJSON, _ := json.Marshal(errorResp)
	s.clientConn.WriteMessage(websocket.TextMessage, responseJSON)
}
