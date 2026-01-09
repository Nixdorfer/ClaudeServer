package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// checkUsageLimits checks if usage exceeds configured limits
// Returns (isBlocked, blockReason, blockResetTime)
func checkUsageLimits() (bool, string, string) {
	usage := getUsage()

	isBlocked, _ := usage["is_blocked"].(bool)
	blockReason, _ := usage["block_reason"].(string)
	blockResetTime, _ := usage["block_reset_time"].(string)

	return isBlocked, blockReason, blockResetTime
}

func (h *Handler) DialogueChatEnhanced(c *gin.Context) {
	var req DialogueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Check usage limits before processing
	if isBlocked, blockReason, blockResetTime := checkUsageLimits(); isBlocked {
		log.Printf("[Usage Limit] Request blocked - Reason: %s, Reset: %s", blockReason, blockResetTime)
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":            "Usage limit exceeded",
			"block_reason":     blockReason,
			"block_reset_time": blockResetTime,
			"is_blocked":       true,
		})
		return
	}

	if req.KeepAlive {
		if req.ConversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Conversation ID required for keepalive"})
			return
		}
		h.dialogueManager.TouchSession(req.ConversationID)
		c.JSON(http.StatusOK, gin.H{
			"conversation_id": req.ConversationID,
			"status":          "keepalive",
			"message":         "Session refreshed",
		})
		return
	}

	if req.Request == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request cannot be empty"})
		return
	}

	if globalMCPSessionManager != nil {
		if err := globalMCPSessionManager.EnsureInitialized(); err != nil {
			log.Printf("⚠️ MCP initialization failed (continuing without MCP): %v", err)
		}
	}

	cookie := h.config.GetCookie()
	var conversationID string
	var parentMessageUUID string

	if req.ConversationID != "" {
		conversationID = req.ConversationID
		session := h.dialogueManager.GetOrCreateSession(conversationID)

		session.GeneratingMutex.RLock()
		parentMessageUUID = session.LastMessageUUID
		session.GeneratingMutex.RUnlock()

		if parentMessageUUID == "00000000-0000-4000-8000-000000000000" {
			newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conversation history"})
				return
			}
			parentMessageUUID = newParentUUID
			h.dialogueManager.UpdateSession(conversationID, parentMessageUUID)
		}
	} else {
		var err error
		conversationID, err = createConversation(h.config.GetOrganizationID(), cookie, true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
			return
		}
		parentMessageUUID = "00000000-0000-4000-8000-000000000000"
		h.dialogueManager.GetOrCreateSession(conversationID)
	}

	receiveTime := time.Now()
	exchangeNum, _ := h.db.GetNextExchangeNumber(conversationID)

	msg := &Message{
		ConversationID: conversationID,
		ExchangeNumber: exchangeNum,
		Request:        req.Request,
		ReceiveTime:    receiveTime,
		Status:         "processing",
	}
	h.db.CreateMessage(msg)

	session := h.dialogueManager.GetOrCreateSession(conversationID)
	session.GeneratingMutex.Lock()
	session.IsGenerating = true
	session.GeneratingMutex.Unlock()

	broadcastDialogues()

	select {
	case h.semaphore <- struct{}{}:
		defer func() {
			<-h.semaphore
			session.GeneratingMutex.Lock()
			session.IsGenerating = false
			session.GeneratingMutex.Unlock()
			broadcastDialogues()
		}()

		sendTime := time.Now()
		msg.SendTime = &sendTime

		dialogueStreamMutex.Lock()
		dialogueStreams[conversationID] = ""
		dialogueStreamMutex.Unlock()

		var attachments []FileAttachment
		if len(req.Files) > 0 {
			for _, file := range req.Files {
				if err := file.DecodeContent(); err != nil {
					dialogueStreamMutex.Lock()
					delete(dialogueStreams, conversationID)
					dialogueStreamMutex.Unlock()

					msg.Status = "failed"
					msg.Notice = fmt.Sprintf("File decode error: %v", err)
					h.db.UpdateMessage(msg)
					c.JSON(http.StatusBadRequest, gin.H{"error": msg.Notice})
					return
				}

				uploadResp, err := uploadFile(h.config.GetOrganizationID(), conversationID, cookie, &file)
				if err != nil {
					dialogueStreamMutex.Lock()
					delete(dialogueStreams, conversationID)
					dialogueStreamMutex.Unlock()

					msg.Status = "failed"
					msg.Notice = fmt.Sprintf("File upload error: %v", err)
					h.db.UpdateMessage(msg)
					c.JSON(http.StatusInternalServerError, gin.H{"error": msg.Notice})
					return
				}

				attachments = append(attachments, FileAttachment{
					FileUUID: uploadResp.FileUUID,
					FileName: uploadResp.FileName,
					FileType: file.Type,
					FileSize: uploadResp.SizeBytes,
				})
			}
		}

		response, err := sendDialogueMessageWithFiles(
			h.config.GetOrganizationID(),
			conversationID,
			cookie,
			req.Request,
			parentMessageUUID,
			req.Model,
			req.Style,
			attachments,
			func(chunk string) {
				dialogueStreamMutex.Lock()
				dialogueStreams[conversationID] = chunk
				dialogueStreamMutex.Unlock()
			},
		)

		dialogueStreamMutex.Lock()
		delete(dialogueStreams, conversationID)
		dialogueStreamMutex.Unlock()

		responseTime := time.Now()
		msg.ResponseTime = &responseTime
		duration := responseTime.Sub(sendTime).Seconds()
		msg.Duration = &duration

		if err != nil {
			msg.Status = "failed"
			msg.Notice = err.Error()
			h.db.UpdateMessage(msg)
			LogExchange(req.Request, err.Error(), true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message: " + err.Error()})
			return
		}

		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}

		msg.Response = response
		msg.Status = "done"
		h.db.UpdateMessage(msg)
		LogExchange(req.Request, response, false)

		c.JSON(http.StatusOK, DialogueResponse{
			ConversationID: conversationID,
			Response:       response,
		})

	default:
		msg.Status = "overloaded"
		msg.Notice = "Server busy"
		h.db.UpdateMessage(msg)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Server busy, try again later"})
	}
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func (h *Handler) DialogueStream(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var req DialogueStreamRequest
	if err := conn.ReadJSON(&req); err != nil {
		sendWSError(conn, "Invalid request format")
		return
	}

	if req.Request == "" {
		sendWSError(conn, "Request cannot be empty")
		return
	}

	// Check usage limits before processing
	if isBlocked, blockReason, blockResetTime := checkUsageLimits(); isBlocked {
		log.Printf("[Usage Limit] WebSocket request blocked - Reason: %s, Reset: %s", blockReason, blockResetTime)
		sendWSMessage(conn, "usage_blocked", map[string]interface{}{
			"error":            "Usage limit exceeded",
			"block_reason":     blockReason,
			"block_reset_time": blockResetTime,
			"is_blocked":       true,
		})
		return
	}

	if globalMCPSessionManager != nil {
		if err := globalMCPSessionManager.EnsureInitialized(); err != nil {
			log.Printf("⚠️ MCP initialization failed (continuing without MCP): %v", err)
		}
	}

	cookie := h.config.GetCookie()
	var conversationID string
	var parentMessageUUID string

	if req.ConversationID != "" {
		conversationID = req.ConversationID
		session := h.dialogueManager.GetOrCreateSession(conversationID)
		h.dialogueManager.SetStreamMode(conversationID, true)

		session.GeneratingMutex.RLock()
		parentMessageUUID = session.LastMessageUUID
		session.GeneratingMutex.RUnlock()

		if parentMessageUUID == "00000000-0000-4000-8000-000000000000" {
			newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
			if err != nil {
				sendWSError(conn, "Failed to get conversation history")
				return
			}
			parentMessageUUID = newParentUUID
			h.dialogueManager.UpdateSession(conversationID, parentMessageUUID)
		}
	} else {
		var err error
		conversationID, err = createConversation(h.config.GetOrganizationID(), cookie, true)
		if err != nil {
			sendWSError(conn, "Failed to create conversation")
			return
		}
		parentMessageUUID = "00000000-0000-4000-8000-000000000000"
		h.dialogueManager.GetOrCreateSession(conversationID)
		h.dialogueManager.SetStreamMode(conversationID, true)
	}

	sendWSMessage(conn, "conversation_id", map[string]string{"conversation_id": conversationID})

	receiveTime := time.Now()
	exchangeNum, _ := h.db.GetNextExchangeNumber(conversationID)

	msg := &Message{
		ConversationID: conversationID,
		ExchangeNumber: exchangeNum,
		Request:        req.Request,
		ReceiveTime:    receiveTime,
		Status:         "processing",
	}
	h.db.CreateMessage(msg)

	session := h.dialogueManager.GetOrCreateSession(conversationID)
	session.GeneratingMutex.Lock()
	session.IsGenerating = true
	session.GeneratingMutex.Unlock()

	broadcastDialogues()

	defer func() {
		h.dialogueManager.MarkSSEClosed(conversationID)
		session.GeneratingMutex.Lock()
		session.IsGenerating = false
		session.GeneratingMutex.Unlock()
		broadcastDialogues()
	}()

	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()

		sendTime := time.Now()
		msg.SendTime = &sendTime

		var attachments []FileAttachment
		if len(req.Files) > 0 {
			for _, file := range req.Files {
				if err := file.DecodeContent(); err != nil {
					msg.Status = "failed"
					msg.Notice = fmt.Sprintf("File decode error: %v", err)
					h.db.UpdateMessage(msg)
					sendWSError(conn, msg.Notice)
					return
				}

				uploadResp, err := uploadFile(h.config.GetOrganizationID(), conversationID, cookie, &file)
				if err != nil {
					msg.Status = "failed"
					msg.Notice = fmt.Sprintf("File upload error: %v", err)
					h.db.UpdateMessage(msg)
					sendWSError(conn, msg.Notice)
					return
				}

				attachments = append(attachments, FileAttachment{
					FileUUID: uploadResp.FileUUID,
					FileName: uploadResp.FileName,
					FileType: file.Type,
					FileSize: uploadResp.SizeBytes,
				})
			}
		}

		response, err := sendDialogueMessageWithFiles(
			h.config.GetOrganizationID(),
			conversationID,
			cookie,
			req.Request,
			parentMessageUUID,
			req.Model,
			req.Style,
			attachments,
			func(chunk string) {
				if err := sendWSMessage(conn, "content", map[string]string{
					"delta": chunk,
					"text":  chunk,
				}); err != nil {
					log.Printf("发送流式内容失败: %v", err)
				}
			},
		)

		responseTime := time.Now()
		msg.ResponseTime = &responseTime
		duration := responseTime.Sub(sendTime).Seconds()
		msg.Duration = &duration

		if err != nil {
			msg.Status = "failed"
			msg.Notice = err.Error()
			h.db.UpdateMessage(msg)
			LogExchange(req.Request, err.Error(), true)
			sendWSError(conn, "Failed to send message: "+err.Error())
			return
		}

		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}

		msg.Response = response
		msg.Status = "done"
		h.db.UpdateMessage(msg)
		LogExchange(req.Request, response, false)

		sendWSMessage(conn, "done", map[string]interface{}{
			"conversation_id": conversationID,
			"response":        response,
			"done":            true,
		})

	default:
		msg.Status = "overloaded"
		msg.Notice = "Server busy"
		h.db.UpdateMessage(msg)
		sendWSError(conn, "Server busy, try again later")
	}
}

func (h *Handler) KeepAlive(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing conversation ID"})
		return
	}

	h.dialogueManager.TouchSession(conversationID)
	c.JSON(http.StatusOK, gin.H{
		"conversation_id": conversationID,
		"status":          "keepalive",
		"message":         "Session refreshed",
	})
}

func sendWSMessage(conn *websocket.Conn, msgType string, data interface{}) error {
	msg := WSMessage{
		Type: msgType,
		Data: data,
	}
	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return conn.WriteJSON(msg)
}

func sendWSError(conn *websocket.Conn, errorMsg string) error {
	return sendWSMessage(conn, "error", map[string]string{"error": errorMsg})
}

func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
	flusher.Flush()
}

func sendSSEError(w http.ResponseWriter, flusher http.Flusher, errorMsg string) {
	sendSSEEvent(w, flusher, "error", map[string]string{"error": errorMsg})
}

func (h *Handler) DialogueEvent(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	request := c.Query("request")
	conversationID := c.Query("conversation_id")
	model := c.Query("model")
	style := c.Query("style")

	if request == "" {
		sendSSEError(c.Writer, flusher, "Request cannot be empty")
		return
	}

	// Check usage limits before processing
	if isBlocked, blockReason, blockResetTime := checkUsageLimits(); isBlocked {
		log.Printf("[Usage Limit] SSE request blocked - Reason: %s, Reset: %s", blockReason, blockResetTime)
		sendSSEEvent(c.Writer, flusher, "usage_blocked", map[string]interface{}{
			"error":            "Usage limit exceeded",
			"block_reason":     blockReason,
			"block_reset_time": blockResetTime,
			"is_blocked":       true,
		})
		return
	}

	if globalMCPSessionManager != nil {
		if err := globalMCPSessionManager.EnsureInitialized(); err != nil {
			log.Printf("⚠️ MCP initialization failed (continuing without MCP): %v", err)
		}
	}

	cookie := h.config.GetCookie()
	var parentMessageUUID string

	if conversationID != "" {
		session := h.dialogueManager.GetOrCreateSession(conversationID)
		h.dialogueManager.SetStreamMode(conversationID, true)

		session.GeneratingMutex.RLock()
		parentMessageUUID = session.LastMessageUUID
		session.GeneratingMutex.RUnlock()

		if parentMessageUUID == "00000000-0000-4000-8000-000000000000" {
			newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
			if err != nil {
				sendSSEError(c.Writer, flusher, "Failed to get conversation history")
				return
			}
			parentMessageUUID = newParentUUID
			h.dialogueManager.UpdateSession(conversationID, parentMessageUUID)
		}
	} else {
		var err error
		conversationID, err = createConversation(h.config.GetOrganizationID(), cookie, true)
		if err != nil {
			sendSSEError(c.Writer, flusher, "Failed to create conversation")
			return
		}
		parentMessageUUID = "00000000-0000-4000-8000-000000000000"
		h.dialogueManager.GetOrCreateSession(conversationID)
		h.dialogueManager.SetStreamMode(conversationID, true)
	}

	sendSSEEvent(c.Writer, flusher, "conversation_id", map[string]string{"conversation_id": conversationID})

	receiveTime := time.Now()
	exchangeNum, _ := h.db.GetNextExchangeNumber(conversationID)

	msg := &Message{
		ConversationID: conversationID,
		ExchangeNumber: exchangeNum,
		Request:        request,
		ReceiveTime:    receiveTime,
		Status:         "processing",
	}
	h.db.CreateMessage(msg)

	session := h.dialogueManager.GetOrCreateSession(conversationID)
	session.GeneratingMutex.Lock()
	session.IsGenerating = true
	session.GeneratingMutex.Unlock()

	broadcastDialogues()

	defer func() {
		h.dialogueManager.MarkSSEClosed(conversationID)
		session.GeneratingMutex.Lock()
		session.IsGenerating = false
		session.GeneratingMutex.Unlock()
		broadcastDialogues()
	}()

	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()

		sendTime := time.Now()
		msg.SendTime = &sendTime

		response, err := sendDialogueMessageWithFiles(
			h.config.GetOrganizationID(),
			conversationID,
			cookie,
			request,
			parentMessageUUID,
			model,
			style,
			nil,
			func(chunk string) {
				sendSSEEvent(c.Writer, flusher, "content", map[string]string{
					"delta": chunk,
					"text":  chunk,
				})
			},
		)

		responseTime := time.Now()
		msg.ResponseTime = &responseTime
		duration := responseTime.Sub(sendTime).Seconds()
		msg.Duration = &duration

		if err != nil {
			msg.Status = "failed"
			msg.Notice = err.Error()
			h.db.UpdateMessage(msg)
			LogExchange(request, err.Error(), true)
			sendSSEError(c.Writer, flusher, "Failed to send message: "+err.Error())
			return
		}

		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}

		msg.Response = response
		msg.Status = "done"
		h.db.UpdateMessage(msg)
		LogExchange(request, response, false)

		sendSSEEvent(c.Writer, flusher, "done", map[string]interface{}{
			"conversation_id": conversationID,
			"response":        response,
			"done":            true,
		})

	default:
		msg.Status = "overloaded"
		msg.Notice = "Server busy"
		h.db.UpdateMessage(msg)
		sendSSEError(c.Writer, flusher, "Server busy, try again later")
	}
}

func (h *Handler) PersistentWebSocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	deviceID := c.Request.Header.Get("X-Device-ID")
	if deviceID == "" {
		deviceID = c.Query("device_id")
	}
	platform := c.Request.Header.Get("X-Platform")
	if platform == "" {
		platform = c.Query("platform")
	}
	clientVersion := c.Request.Header.Get("X-Client-Version")
	if h.config.MinClientVersion != "" && clientVersion != "" {
		if !CompareVersions(clientVersion, h.config.MinClientVersion) {
			sendWSMessage(conn, "version_outdated", map[string]interface{}{
				"current_version":  clientVersion,
				"required_version": h.config.MinClientVersion,
				"message":          "当前版本已过时，无法继续使用，请更新到最新版本",
			})
			log.Printf("Outdated client version: %s (required: %s)", clientVersion, h.config.MinClientVersion)
			return
		}
	}
	if deviceID != "" {
		_, err := h.db.GetOrCreateDevice(deviceID, platform)
		if err != nil {
			log.Printf("Failed to register device %s: %v", deviceID, err)
		}
		isBanned, banReason, err := h.db.IsDeviceBanned(deviceID)
		if err != nil {
			log.Printf("Failed to check device ban status: %v", err)
		}
		if isBanned {
			sendWSMessage(conn, "banned", map[string]interface{}{
				"banned": true,
				"reason": banReason,
			})
			log.Printf("Banned device attempted connection: %s", deviceID)
			return
		}
	}
	c.Set("device_id", deviceID)

	// 设置 pong 处理器，收到 pong 时更新读取截止时间
	conn.SetPongHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	// 设置 ping 处理器，自动响应 ping
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})

	sendWSMessage(conn, "connected", map[string]string{
		"status":  "connected",
		"message": "WebSocket connection established",
	})

	log.Printf("WebSocket连接已建立: %s (device: %s)", c.Request.RemoteAddr, deviceID)

	// 启动心跳 goroutine
	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					log.Printf("WebSocket ping 发送失败: %v", err)
					return
				}
			}
		}
	}()

	for {
		// 设置读取截止时间
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))

		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket意外关闭: %v", err)
			} else {
				log.Printf("WebSocket正常关闭: %s", c.Request.RemoteAddr)
			}
			break
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			sendWSError(conn, "Invalid message format: missing type field")
			continue
		}

		switch msgType {
		case "dialogue":
			h.handleWSDialogueRequest(conn, msg)
		case "keepalive":
			h.handleWSKeepalive(conn, msg)
		case "api_request":
			h.handleWSAPIRequest(conn, msg)
		case "ping":
			sendWSMessage(conn, "pong", map[string]string{"timestamp": time.Now().Format(time.RFC3339)})
		default:
			sendWSError(conn, fmt.Sprintf("Unknown message type: %s", msgType))
		}
	}
}

func (h *Handler) handleWSDialogueRequest(conn *websocket.Conn, msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		sendWSError(conn, "Invalid dialogue request: missing data field")
		return
	}

	request, _ := data["request"].(string)
	conversationID, _ := data["conversation_id"].(string)
	model, _ := data["model"].(string)
	style, _ := data["style"].(string)
	deviceID, _ := data["device_id"].(string)

	if request == "" {
		sendWSError(conn, "Request cannot be empty")
		return
	}

	// Check usage limits before processing
	if isBlocked, blockReason, blockResetTime := checkUsageLimits(); isBlocked {
		log.Printf("[Usage Limit] Persistent WebSocket request blocked - Reason: %s, Reset: %s", blockReason, blockResetTime)
		sendWSMessage(conn, "usage_blocked", map[string]interface{}{
			"error":            "Usage limit exceeded",
			"block_reason":     blockReason,
			"block_reset_time": blockResetTime,
			"is_blocked":       true,
		})
		return
	}

	// Check device ban status before processing request
	if deviceID != "" {
		isBanned, banReason, _ := h.db.IsDeviceBanned(deviceID)
		if isBanned {
			sendWSMessage(conn, "banned", map[string]interface{}{
				"banned": true,
				"reason": banReason,
			})
			return
		}
	}

	if globalMCPSessionManager != nil {
		if err := globalMCPSessionManager.EnsureInitialized(); err != nil {
			log.Printf("⚠️ MCP initialization failed (continuing without MCP): %v", err)
		}
	}

	cookie := h.config.GetCookie()
	var parentMessageUUID string

	if conversationID != "" {
		session := h.dialogueManager.GetOrCreateSession(conversationID)
		h.dialogueManager.SetStreamMode(conversationID, true)

		session.GeneratingMutex.RLock()
		parentMessageUUID = session.LastMessageUUID
		session.GeneratingMutex.RUnlock()

		if parentMessageUUID == "00000000-0000-4000-8000-000000000000" {
			newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
			if err != nil {
				sendWSError(conn, "Failed to get conversation history")
				return
			}
			parentMessageUUID = newParentUUID
			h.dialogueManager.UpdateSession(conversationID, parentMessageUUID)
		}
	} else {
		var err error
		conversationID, err = createConversation(h.config.GetOrganizationID(), cookie, true)
		if err != nil {
			sendWSError(conn, "Failed to create conversation")
			return
		}
		parentMessageUUID = "00000000-0000-4000-8000-000000000000"
		h.dialogueManager.GetOrCreateSession(conversationID)
		h.dialogueManager.SetStreamMode(conversationID, true)
	}

	sendWSMessage(conn, "conversation_id", map[string]string{"conversation_id": conversationID})

	receiveTime := time.Now()
	exchangeNum, _ := h.db.GetNextExchangeNumber(conversationID)

	dbMsg := &Message{
		ConversationID: conversationID,
		DeviceID:       deviceID,
		ExchangeNumber: exchangeNum,
		Request:        request,
		ReceiveTime:    receiveTime,
		Status:         "processing",
	}
	h.db.CreateMessage(dbMsg)

	session := h.dialogueManager.GetOrCreateSession(conversationID)
	session.GeneratingMutex.Lock()
	session.IsGenerating = true
	session.GeneratingMutex.Unlock()

	broadcastDialogues()

	defer func() {
		session.GeneratingMutex.Lock()
		session.IsGenerating = false
		session.GeneratingMutex.Unlock()
		broadcastDialogues()
	}()

	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()

		sendTime := time.Now()
		dbMsg.SendTime = &sendTime

		response, err := sendDialogueMessageWithFiles(
			h.config.GetOrganizationID(),
			conversationID,
			cookie,
			request,
			parentMessageUUID,
			model,
			style,
			nil,
			func(chunk string) {
				if err := sendWSMessage(conn, "content", map[string]string{
					"delta": chunk,
					"text":  chunk,
				}); err != nil {
					log.Printf("发送流式内容失败: %v", err)
				}
			},
		)

		responseTime := time.Now()
		dbMsg.ResponseTime = &responseTime
		duration := responseTime.Sub(sendTime).Seconds()
		dbMsg.Duration = &duration

		if err != nil {
			dbMsg.Status = "failed"
			dbMsg.Notice = err.Error()
			h.db.UpdateMessage(dbMsg)
			LogExchange(request, err.Error(), true)
			sendWSError(conn, "Failed to send message: "+err.Error())
			return
		}

		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}

		dbMsg.Response = response
		dbMsg.Status = "done"
		h.db.UpdateMessage(dbMsg)
		LogExchange(request, response, false)

		sendWSMessage(conn, "done", map[string]interface{}{
			"conversation_id": conversationID,
			"response":        response,
			"done":            true,
		})

	default:
		dbMsg.Status = "overloaded"
		dbMsg.Notice = "Server busy"
		h.db.UpdateMessage(dbMsg)
		sendWSError(conn, "Server busy, try again later")
	}
}

func (h *Handler) handleWSKeepalive(conn *websocket.Conn, msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		sendWSError(conn, "Invalid keepalive request: missing data field")
		return
	}

	conversationID, _ := data["conversation_id"].(string)
	if conversationID == "" {
		sendWSError(conn, "Conversation ID required for keepalive")
		return
	}

	h.dialogueManager.TouchSession(conversationID)
	sendWSMessage(conn, "keepalive", map[string]interface{}{
		"conversation_id": conversationID,
		"status":          "keepalive",
		"message":         "Session refreshed",
	})
}

func (h *Handler) handleWSAPIRequest(conn *websocket.Conn, msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		sendWSError(conn, "Invalid API request: missing data field")
		return
	}

	requestID, _ := data["request_id"].(float64)
	endpoint, _ := data["endpoint"].(string)
	_, _ = data["method"].(string)

	if endpoint == "" {
		sendWSMessage(conn, "error", map[string]interface{}{
			"request_id": requestID,
			"error":      "Missing endpoint",
		})
		return
	}

	var responseData interface{}
	var err error

	var recordID string
	var dialogueID string
	if strings.HasPrefix(endpoint, "/api/record/") {
		recordID = strings.TrimPrefix(endpoint, "/api/record/")
		endpoint = "/api/record/:id"
	} else if strings.HasPrefix(endpoint, "/api/dialogues/") && strings.HasSuffix(endpoint, "/history") {
		dialogueID = strings.TrimSuffix(strings.TrimPrefix(endpoint, "/api/dialogues/"), "/history")
		endpoint = "/api/dialogues/:id/history"
	} else if strings.HasPrefix(endpoint, "/api/dialogues/") && !strings.Contains(strings.TrimPrefix(endpoint, "/api/dialogues/"), "/") {
		dialogueID = strings.TrimPrefix(endpoint, "/api/dialogues/")
		endpoint = "/api/dialogues/:id"
	}

	switch endpoint {
	case "/api/stats":
		stats := h.db.GetStats()
		tpm, rpm, rpd, _ := h.db.CalculateRates()
		responseData = map[string]interface{}{
			"processing":       stats.Processing,
			"completed":        stats.Completed,
			"failed":           stats.Failed,
			"tpm":              tpm,
			"rpm":              rpm,
			"rpd":              rpd,
			"service_shutdown": stats.ServiceShutdown,
			"shutdown_reason":  stats.ShutdownReason,
		}
	case "/api/usage":
		responseData = getUsage()
	case "/api/dialogues":
		conversations, err := h.db.GetAllConversations()
		if err != nil {
			responseData = map[string]interface{}{"conversations": []interface{}{}}
		} else {
			responseData = map[string]interface{}{"conversations": conversations}
		}
	case "/api/dialogues/:id/history":
		messages, err := h.db.GetConversationMessages(dialogueID)
		if err != nil {
			responseData = map[string]interface{}{"messages": []interface{}{}}
		} else {
			responseData = map[string]interface{}{"messages": messages}
		}
	case "/api/dialogues/:id":
		h.dialogueManager.DeleteSession(dialogueID)
		broadcastDialogues()
		responseData = map[string]interface{}{"message": "Dialogue deleted successfully"}
	case "/api/record/:id":
		var msgID int64
		fmt.Sscanf(recordID, "%d", &msgID)
		message, err := h.db.GetMessageByID(msgID)
		if err == nil {
			responseData = message
		} else {
			err = fmt.Errorf("Record not found")
		}
	case "/api/records":
		body, _ := data["body"].(map[string]interface{})
		limit := 100
		if body != nil {
			if l, ok := body["limit"].(float64); ok {
				limit = int(l)
			}
		}
		messages, err := h.db.GetHistory(limit)
		if err != nil {
			responseData = map[string]interface{}{"messages": []interface{}{}}
		} else {
			responseData = map[string]interface{}{"messages": messages}
		}
	case "/api/config":
		endpoints := []map[string]string{
			{"path": "/chat/dialogue/http", "description": "Classic HTTP dialogue with long timeout", "method": "POST"},
			{"path": "/chat/dialogue/event", "description": "SSE dialogue streaming", "method": "GET"},
			{"path": "/chat/dialogue/websocket", "description": "WebSocket dialogue streaming", "method": "GET"},
			{"path": "/health", "description": "Health check", "method": "GET"},
			{"path": "/data/websocket/create", "description": "Create persistent WebSocket connection", "method": "GET"},
		}

		version := "1.0.0"
		if changes, err := LoadVersionChanges("../src/changes.yaml"); err == nil {
			version = GetLatestVersion(changes)
		}

		responseData = map[string]interface{}{
			"version":             version,
			"thread_num":          h.config.ThreadNum,
			"debug":               h.config.Debug,
			"incognito":           true,
			"api_endpoint":        h.config.APIEndpoint,
			"max_tpm":             h.config.MaxTPM,
			"max_rpm":             h.config.MaxRPM,
			"max_rpd":             h.config.MaxRPD,
			"request_interval_ms": h.config.RequestIntervalMS,
			"models":              h.config.Models,
			"endpoints":           endpoints,
		}
	default:
		sendWSMessage(conn, "error", map[string]interface{}{
			"request_id": requestID,
			"error":      fmt.Sprintf("Unknown endpoint: %s", endpoint),
		})
		return
	}

	if err != nil {
		sendWSMessage(conn, "error", map[string]interface{}{
			"request_id": requestID,
			"error":      err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"request_id": requestID,
	}

	if dataMap, ok := responseData.(map[string]interface{}); ok {
		for k, v := range dataMap {
			response[k] = v
		}
	}

	sendWSMessage(conn, "response", response)
}
