package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

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
			log.Printf("MCP initialization failed (continuing without MCP): %v", err)
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
	devicePassword := c.GetHeader("X-Device-ID")
	platform := c.GetHeader("X-Platform")
	if platform == "" {
		platform = "windows"
	}
	device, _ := h.db.GetOrCreateDevice(devicePassword, platform)
	conv, _ := h.db.CreateConversation(device.ID, conversationID)
	dialogueOrder, _ := h.db.GetNextDialogueOrder(conv.ID)
	dialogueUID := uuid.New().String()
	dialogue := &CldDialogue{
		UID:            dialogueUID,
		ConversationID: conv.ID,
		Order:          dialogueOrder,
		UserMessage:    req.Request,
		CreateTime:     time.Now(),
		Status:         "processing",
		PromptID:       h.db.GetCurrentPromptID(),
	}
	h.db.CreateDialogue(dialogue)
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
		requestTime := time.Now()
		dialogue.RequestTime = &requestTime
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
					dialogue.Status = "send_failed"
					h.db.UpdateDialogue(dialogue)
					c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("File decode error: %v", err)})
					return
				}
				uploadResp, err := uploadFile(h.config.GetOrganizationID(), conversationID, cookie, &file)
				if err != nil {
					dialogueStreamMutex.Lock()
					delete(dialogueStreams, conversationID)
					dialogueStreamMutex.Unlock()
					dialogue.Status = "send_failed"
					h.db.UpdateDialogue(dialogue)
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("File upload error: %v", err)})
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
		finishTime := time.Now()
		dialogue.FinishTime = &finishTime
		duration := int(finishTime.Sub(dialogue.CreateTime).Milliseconds())
		dialogue.Duration = &duration
		if err != nil {
			dialogue.Status = "send_failed"
			h.db.UpdateDialogue(dialogue)
			LogExchange(req.Request, err.Error(), true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message: " + err.Error()})
			return
		}
		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}
		dialogue.AssistantMessage = &response
		dialogue.Status = "done"
		h.db.UpdateDialogue(dialogue)
		LogExchange(req.Request, response, false)
		c.JSON(http.StatusOK, DialogueResponse{
			ConversationID: conversationID,
			Response:       response,
		})
	default:
		dialogue.Status = "send_failed"
		h.db.UpdateDialogue(dialogue)
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
	Type string `json:"type"`
	Data any    `json:"data"`
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
	if isBlocked, blockReason, blockResetTime := checkUsageLimits(); isBlocked {
		log.Printf("[Usage Limit] WebSocket request blocked - Reason: %s, Reset: %s", blockReason, blockResetTime)
		sendWSMessage(conn, "usage_blocked", map[string]any{
			"error":            "Usage limit exceeded",
			"block_reason":     blockReason,
			"block_reset_time": blockResetTime,
			"is_blocked":       true,
		})
		return
	}
	if globalMCPSessionManager != nil {
		if err := globalMCPSessionManager.EnsureInitialized(); err != nil {
			log.Printf("MCP initialization failed (continuing without MCP): %v", err)
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
	devicePassword := c.GetHeader("X-Device-ID")
	platform := c.GetHeader("X-Platform")
	if platform == "" {
		platform = "windows"
	}
	device, _ := h.db.GetOrCreateDevice(devicePassword, platform)
	conv, _ := h.db.CreateConversation(device.ID, conversationID)
	dialogueOrder, _ := h.db.GetNextDialogueOrder(conv.ID)
	dialogueUID := uuid.New().String()
	dialogue := &CldDialogue{
		UID:            dialogueUID,
		ConversationID: conv.ID,
		Order:          dialogueOrder,
		UserMessage:    req.Request,
		CreateTime:     time.Now(),
		Status:         "processing",
		PromptID:       h.db.GetCurrentPromptID(),
	}
	h.db.CreateDialogue(dialogue)
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
		requestTime := time.Now()
		dialogue.RequestTime = &requestTime
		var attachments []FileAttachment
		if len(req.Files) > 0 {
			for _, file := range req.Files {
				if err := file.DecodeContent(); err != nil {
					dialogue.Status = "send_failed"
					h.db.UpdateDialogue(dialogue)
					sendWSError(conn, fmt.Sprintf("File decode error: %v", err))
					return
				}
				uploadResp, err := uploadFile(h.config.GetOrganizationID(), conversationID, cookie, &file)
				if err != nil {
					dialogue.Status = "send_failed"
					h.db.UpdateDialogue(dialogue)
					sendWSError(conn, fmt.Sprintf("File upload error: %v", err))
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
		finishTime := time.Now()
		dialogue.FinishTime = &finishTime
		duration := int(finishTime.Sub(dialogue.CreateTime).Milliseconds())
		dialogue.Duration = &duration
		if err != nil {
			dialogue.Status = "send_failed"
			h.db.UpdateDialogue(dialogue)
			LogExchange(req.Request, err.Error(), true)
			sendWSError(conn, "Failed to send message: "+err.Error())
			return
		}
		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}
		dialogue.AssistantMessage = &response
		dialogue.Status = "done"
		h.db.UpdateDialogue(dialogue)
		LogExchange(req.Request, response, false)
		sendWSMessage(conn, "done", map[string]any{
			"conversation_id": conversationID,
			"response":        response,
			"done":            true,
		})
	default:
		dialogue.Status = "send_failed"
		h.db.UpdateDialogue(dialogue)
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

func sendWSMessage(conn *websocket.Conn, msgType string, data any) error {
	msg := WSMessage{
		Type: msgType,
		Data: data,
	}
	if msgType != "content" {
		DebugLogResponse(msgType, data)
	}
	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return conn.WriteJSON(msg)
}

func sendWSError(conn *websocket.Conn, errorMsg string) error {
	return sendWSMessage(conn, "error", map[string]string{"error": errorMsg})
}

func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
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
	if isBlocked, blockReason, blockResetTime := checkUsageLimits(); isBlocked {
		log.Printf("[Usage Limit] SSE request blocked - Reason: %s, Reset: %s", blockReason, blockResetTime)
		sendSSEEvent(c.Writer, flusher, "usage_blocked", map[string]any{
			"error":            "Usage limit exceeded",
			"block_reason":     blockReason,
			"block_reset_time": blockResetTime,
			"is_blocked":       true,
		})
		return
	}
	if globalMCPSessionManager != nil {
		if err := globalMCPSessionManager.EnsureInitialized(); err != nil {
			log.Printf("MCP initialization failed (continuing without MCP): %v", err)
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
	devicePassword := c.GetHeader("X-Device-ID")
	platform := c.GetHeader("X-Platform")
	if platform == "" {
		platform = "windows"
	}
	device, _ := h.db.GetOrCreateDevice(devicePassword, platform)
	conv, _ := h.db.CreateConversation(device.ID, conversationID)
	dialogueOrder, _ := h.db.GetNextDialogueOrder(conv.ID)
	dialogueUID := uuid.New().String()
	dialogue := &CldDialogue{
		UID:            dialogueUID,
		ConversationID: conv.ID,
		Order:          dialogueOrder,
		UserMessage:    request,
		CreateTime:     time.Now(),
		Status:         "processing",
		PromptID:       h.db.GetCurrentPromptID(),
	}
	h.db.CreateDialogue(dialogue)
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
		requestTime := time.Now()
		dialogue.RequestTime = &requestTime
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
		finishTime := time.Now()
		dialogue.FinishTime = &finishTime
		duration := int(finishTime.Sub(dialogue.CreateTime).Milliseconds())
		dialogue.Duration = &duration
		if err != nil {
			dialogue.Status = "send_failed"
			h.db.UpdateDialogue(dialogue)
			LogExchange(request, err.Error(), true)
			sendSSEError(c.Writer, flusher, "Failed to send message: "+err.Error())
			return
		}
		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}
		dialogue.AssistantMessage = &response
		dialogue.Status = "done"
		h.db.UpdateDialogue(dialogue)
		LogExchange(request, response, false)
		sendSSEEvent(c.Writer, flusher, "done", map[string]any{
			"conversation_id": conversationID,
			"response":        response,
			"done":            true,
		})
	default:
		dialogue.Status = "send_failed"
		h.db.UpdateDialogue(dialogue)
		sendSSEError(c.Writer, flusher, "Server busy, try again later")
	}
}

func (h *Handler) PersistentWebSocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	devicePassword := c.Request.Header.Get("X-Device-ID")
	if devicePassword == "" {
		devicePassword = c.Query("device_id")
	}
	platform := c.Request.Header.Get("X-Platform")
	if platform == "" {
		platform = c.Query("platform")
	}
	clientVersion := c.Request.Header.Get("X-Client-Version")
	if h.config.MinClientVersion != "" && clientVersion != "" {
		if !CompareVersions(clientVersion, h.config.MinClientVersion) {
			sendWSMessage(conn, "version_outdated", map[string]any{
				"current_version":  clientVersion,
				"required_version": h.config.MinClientVersion,
				"message":          "当前版本已过时，无法继续使用，请更新到最新版本",
			})
			log.Printf("Outdated client version: %s (required: %s)", clientVersion, h.config.MinClientVersion)
			return
		}
	}
	var deviceID int
	if devicePassword != "" {
		device, err := h.db.GetOrCreateDevice(devicePassword, platform)
		if err != nil {
			log.Printf("Failed to register device %s: %v", devicePassword, err)
		}
		if device != nil {
			deviceID = device.ID
		}
		isBanned, banReason, err := h.db.IsDeviceBanned(deviceID)
		if err != nil {
			log.Printf("Failed to check device ban status: %v", err)
		}
		if isBanned {
			sendWSMessage(conn, "banned", map[string]any{
				"banned": true,
				"reason": banReason,
			})
			log.Printf("Banned device attempted connection: %s", devicePassword)
			return
		}
	}
	c.Set("device_id", devicePassword)
	conn.SetPongHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})
	sendWSMessage(conn, "connected", map[string]string{
		"status":  "connected",
		"message": "WebSocket connection established",
	})
	log.Printf("WebSocket连接已建立: %s (device: %d)", c.Request.RemoteAddr, deviceID)
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
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		var msg map[string]any
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
		DebugLogRequest(msgType, msg)
		switch msgType {
		case "dialogue":
			h.handleWSDialogueRequest(conn, msg)
		case "keepalive":
			h.handleWSKeepalive(conn, msg)
		case "api_request":
			h.handleWSAPIRequest(conn, msg)
		case "ping":
			sendWSMessage(conn, "pong", map[string]string{"timestamp": time.Now().Format(time.RFC3339)})
		case "ack":
			h.handleWSAck(conn, msg)
		default:
			sendWSError(conn, fmt.Sprintf("Unknown message type: %s", msgType))
		}
	}
}

func (h *Handler) handleWSDialogueRequest(conn *websocket.Conn, msg map[string]any) {
	data, ok := msg["data"].(map[string]any)
	if !ok {
		sendWSError(conn, "Invalid dialogue request: missing data field")
		return
	}
	request, _ := data["request"].(string)
	conversationID, _ := data["conversation_id"].(string)
	model, _ := data["model"].(string)
	style, _ := data["style"].(string)
	devicePassword, _ := data["device_id"].(string)
	if devicePassword == "" {
		sendWSError(conn, "Device ID is required")
		return
	}
	if request == "" {
		sendWSError(conn, "Request cannot be empty")
		return
	}
	if isBlocked, blockReason, blockResetTime := checkUsageLimits(); isBlocked {
		log.Printf("[Usage Limit] Persistent WebSocket request blocked - Reason: %s, Reset: %s", blockReason, blockResetTime)
		sendWSMessage(conn, "usage_blocked", map[string]any{
			"error":            "Usage limit exceeded",
			"block_reason":     blockReason,
			"block_reset_time": blockResetTime,
			"is_blocked":       true,
		})
		return
	}
	platform := "windows"
	device, err := h.db.GetOrCreateDevice(devicePassword, platform)
	if err != nil || device == nil {
		sendWSError(conn, "Failed to get or create device")
		return
	}
	isBanned, banReason, _ := h.db.IsDeviceBanned(device.ID)
	if isBanned {
		sendWSMessage(conn, "banned", map[string]any{
			"banned": true,
			"reason": banReason,
		})
		return
	}
	if globalMCPSessionManager != nil {
		if err := globalMCPSessionManager.EnsureInitialized(); err != nil {
			log.Printf("MCP initialization failed (continuing without MCP): %v", err)
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
	conv, err := h.db.CreateConversation(device.ID, conversationID)
	if err != nil || conv == nil {
		conv, err = h.db.GetConversationByUID(conversationID)
		if err != nil || conv == nil {
			log.Printf("无法创建或获取对话: %v", err)
			sendWSMessage(conn, "error", map[string]string{"error": "无法创建对话"})
			return
		}
	}
	dialogueOrder, _ := h.db.GetNextDialogueOrder(conv.ID)
	dialogueUID := uuid.New().String()
	dialogue := &CldDialogue{
		UID:            dialogueUID,
		ConversationID: conv.ID,
		Order:          dialogueOrder,
		UserMessage:    request,
		CreateTime:     time.Now(),
		Status:         "processing",
		PromptID:       h.db.GetCurrentPromptID(),
	}
	h.db.CreateDialogue(dialogue)
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
		requestTime := time.Now()
		dialogue.RequestTime = &requestTime
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
		finishTime := time.Now()
		dialogue.FinishTime = &finishTime
		duration := int(finishTime.Sub(dialogue.CreateTime).Milliseconds())
		dialogue.Duration = &duration
		if err != nil {
			dialogue.Status = "send_failed"
			h.db.UpdateDialogue(dialogue)
			LogExchange(request, err.Error(), true)
			sendWSError(conn, "Failed to send message: "+err.Error())
			return
		}
		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), conversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(conversationID, newParentUUID)
		}
		dialogue.AssistantMessage = &response
		dialogue.Status = "replying"
		h.db.UpdateDialogue(dialogue)
		LogExchange(request, response, false)
		ackChan := make(chan struct{}, 1)
		h.pendingAcks.Store(dialogue.ID, ackChan)
		sendWSMessage(conn, "done", map[string]any{
			"conversation_id": conversationID,
			"dialogue_id":     dialogue.ID,
			"response":        response,
			"done":            true,
		})
		go func(dialogueID int) {
			select {
			case <-ackChan:
				h.pendingAcks.Delete(dialogueID)
			case <-time.After(30 * time.Second):
				h.pendingAcks.Delete(dialogueID)
				d, err := h.db.GetDialogueByID(dialogueID)
				if err == nil && d.Status == "replying" {
					d.Status = "reply_failed"
					h.db.UpdateDialogue(d)
				}
			}
		}(dialogue.ID)
	default:
		dialogue.Status = "send_failed"
		h.db.UpdateDialogue(dialogue)
		sendWSError(conn, "Server busy, try again later")
	}
}

func (h *Handler) handleWSKeepalive(conn *websocket.Conn, msg map[string]any) {
	data, ok := msg["data"].(map[string]any)
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
	sendWSMessage(conn, "keepalive", map[string]any{
		"conversation_id": conversationID,
		"status":          "keepalive",
		"message":         "Session refreshed",
	})
}

func (h *Handler) handleWSAck(conn *websocket.Conn, msg map[string]any) {
	data, ok := msg["data"].(map[string]any)
	if !ok {
		sendWSError(conn, "Invalid ack request: missing data field")
		return
	}
	dialogueID, ok := data["dialogue_id"].(float64)
	if !ok {
		sendWSError(conn, "Invalid ack request: missing dialogue_id")
		return
	}
	id := int(dialogueID)
	if ackChanVal, ok := h.pendingAcks.Load(id); ok {
		if ackChan, ok := ackChanVal.(chan struct{}); ok {
			select {
			case ackChan <- struct{}{}:
			default:
			}
		}
	}
	dialogue, err := h.db.GetDialogueByID(id)
	if err == nil && dialogue.Status == "replying" {
		dialogue.Status = "done"
		h.db.UpdateDialogue(dialogue)
	}
	sendWSMessage(conn, "ack_received", map[string]any{
		"dialogue_id": id,
		"status":      "ok",
	})
}

func (h *Handler) handleWSAPIRequest(conn *websocket.Conn, msg map[string]any) {
	data, ok := msg["data"].(map[string]any)
	if !ok {
		sendWSError(conn, "Invalid API request: missing data field")
		return
	}
	requestID, _ := data["request_id"].(float64)
	endpoint, _ := data["endpoint"].(string)
	_, _ = data["method"].(string)
	if endpoint == "" {
		sendWSMessage(conn, "error", map[string]any{
			"request_id": requestID,
			"error":      "Missing endpoint",
		})
		return
	}
	var responseData any
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
		responseData = map[string]any{
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
			responseData = map[string]any{"conversations": []any{}}
		} else {
			responseData = map[string]any{"conversations": conversations}
		}
	case "/api/dialogues/:id/history":
		var convID int
		fmt.Sscanf(dialogueID, "%d", &convID)
		dialogues, err := h.db.GetConversationDialogues(convID)
		if err != nil {
			responseData = map[string]any{"messages": []any{}}
		} else {
			responseData = map[string]any{"messages": dialogues}
		}
	case "/api/dialogues/:id":
		h.dialogueManager.DeleteSession(dialogueID)
		broadcastDialogues()
		responseData = map[string]any{"message": "Dialogue deleted successfully"}
	case "/api/record/:id":
		var msgID int
		fmt.Sscanf(recordID, "%d", &msgID)
		dialogue, err := h.db.GetDialogueByID(msgID)
		if err == nil {
			responseData = dialogue
		} else {
			err = fmt.Errorf("Record not found")
		}
	case "/api/records":
		body, _ := data["body"].(map[string]any)
		limit := 100
		if body != nil {
			if l, ok := body["limit"].(float64); ok {
				limit = int(l)
			}
		}
		messages, err := h.db.GetHistory(limit)
		if err != nil {
			responseData = map[string]any{"messages": []any{}}
		} else {
			responseData = map[string]any{"messages": messages}
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
		if changes, err := LoadVersionChanges("src/changes.yaml"); err == nil {
			version = GetLatestVersion(changes)
		}
		responseData = map[string]any{
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
		sendWSMessage(conn, "error", map[string]any{
			"request_id": requestID,
			"error":      fmt.Sprintf("Unknown endpoint: %s", endpoint),
		})
		return
	}
	response := map[string]any{
		"request_id": requestID,
	}
	if dataMap, ok := responseData.(map[string]any); ok {
		for k, v := range dataMap {
			response[k] = v
		}
	}
	sendWSMessage(conn, "response", response)
}
