package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	processingRequests  = make(map[string]*ProcessingRequest)
	processingMutex     sync.RWMutex
	dialogueStreams     = make(map[string]string)
	dialogueStreamMutex sync.RWMutex
)

type ProcessingRequest struct {
	ID          string
	SubmitTime  time.Time
	InputTokens int
	UserMessage string
}

func (h *Handler) ChatCompletion(c *gin.Context) {
	var req OpenAIChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Messages cannot be empty"})
		return
	}
	conversationID := uuid.New().String()
	receiveTime := time.Now()
	exchangeNum, _ := h.db.GetNextExchangeNumber(conversationID)
	userMessage := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			userMessage = m.Content
			break
		}
	}
	msg := &Message{
		ConversationID: conversationID,
		ExchangeNumber: exchangeNum,
		Request:        userMessage,
		ReceiveTime:    receiveTime,
		Status:         "processing",
	}
	h.db.CreateMessage(msg)
	h.db.IncrementProcessing()
	requestID := uuid.New().String()
	processingMutex.Lock()
	processingRequests[requestID] = &ProcessingRequest{
		ID:          requestID,
		SubmitTime:  receiveTime,
		InputTokens: 0,
		UserMessage: userMessage,
	}
	processingMutex.Unlock()
	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()
		h.db.DecrementProcessing()
		processingMutex.Lock()
		delete(processingRequests, requestID)
		processingMutex.Unlock()
		sendTime := time.Now()
		msg.SendTime = &sendTime
		cookie := h.config.GetCookie()
		response, err := sendChatCompletion(h.config.GetOrganizationID(), cookie, req.Messages, req.Model, req.Stream)
		responseTime := time.Now()
		msg.ResponseTime = &responseTime
		duration := responseTime.Sub(sendTime).Seconds()
		msg.Duration = &duration
		if err != nil {
			msg.Status = "failed"
			msg.Notice = err.Error()
			h.db.UpdateMessage(msg)
			h.db.IncrementFailed()
			LogExchange(userMessage, err.Error(), true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		msg.Response = response.Choices[0].Message.Content
		msg.Status = "done"
		inputTokens := response.Usage.PromptTokens
		outputTokens := response.Usage.CompletionTokens
		totalTokens := response.Usage.TotalTokens
		msg.RequestTokens = &inputTokens
		msg.ResponseTokens = &outputTokens
		msg.Tokens = &totalTokens
		h.db.UpdateMessage(msg)
		h.db.IncrementCompleted()
		LogExchange(userMessage, msg.Response, false)
		c.JSON(http.StatusOK, response)
	default:
		msg.Status = "overloaded"
		msg.Notice = "Server busy"
		h.db.UpdateMessage(msg)
		h.db.IncrementFailed()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Server busy, try again later"})
	}
}

func (h *Handler) OllamaChat(c *gin.Context) {
	var req OllamaChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Messages cannot be empty"})
		return
	}
	conversationID := uuid.New().String()
	receiveTime := time.Now()
	exchangeNum, _ := h.db.GetNextExchangeNumber(conversationID)
	userMessage := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			userMessage = m.Content
			break
		}
	}
	msg := &Message{
		ConversationID: conversationID,
		ExchangeNumber: exchangeNum,
		Request:        userMessage,
		ReceiveTime:    receiveTime,
		Status:         "processing",
	}
	h.db.CreateMessage(msg)
	h.db.IncrementProcessing()
	requestID := uuid.New().String()
	processingMutex.Lock()
	processingRequests[requestID] = &ProcessingRequest{
		ID:          requestID,
		SubmitTime:  receiveTime,
		InputTokens: 0,
		UserMessage: userMessage,
	}
	processingMutex.Unlock()
	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()
		h.db.DecrementProcessing()
		processingMutex.Lock()
		delete(processingRequests, requestID)
		processingMutex.Unlock()
		sendTime := time.Now()
		msg.SendTime = &sendTime
		cookie := h.config.GetCookie()
		openaiReq := OpenAIChatRequest{
			Model:    req.Model,
			Messages: req.Messages,
			Stream:   req.Stream,
		}
		response, err := sendChatCompletion(h.config.GetOrganizationID(), cookie, openaiReq.Messages, openaiReq.Model, openaiReq.Stream)
		responseTime := time.Now()
		msg.ResponseTime = &responseTime
		duration := responseTime.Sub(sendTime).Seconds()
		msg.Duration = &duration
		if err != nil {
			msg.Status = "failed"
			msg.Notice = err.Error()
			h.db.UpdateMessage(msg)
			h.db.IncrementFailed()
			LogExchange(userMessage, err.Error(), true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		msg.Response = response.Choices[0].Message.Content
		msg.Status = "done"
		inputTokens := response.Usage.PromptTokens
		outputTokens := response.Usage.CompletionTokens
		totalTokens := response.Usage.TotalTokens
		msg.RequestTokens = &inputTokens
		msg.ResponseTokens = &outputTokens
		msg.Tokens = &totalTokens
		h.db.UpdateMessage(msg)
		h.db.IncrementCompleted()
		LogExchange(userMessage, msg.Response, false)
		ollamaResponse := OllamaChatResponse{
			Model:     req.Model,
			CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			Message:   response.Choices[0].Message,
			Done:      true,
		}
		c.JSON(http.StatusOK, ollamaResponse)
	default:
		msg.Status = "overloaded"
		msg.Notice = "Server busy"
		h.db.UpdateMessage(msg)
		h.db.IncrementFailed()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Server busy, try again later"})
	}
}

func (h *Handler) DialogueChat(c *gin.Context) {
	var req DialogueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if req.Request == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request cannot be empty"})
		return
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
		response, err := sendDialogueMessageWithOptions(
			h.config.GetOrganizationID(),
			conversationID,
			cookie,
			req.Request,
			parentMessageUUID,
			req.Model,
			req.Style,
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

func (h *Handler) GetProcessingRequests(c *gin.Context) {
	processingMutex.RLock()
	requests := make([]map[string]any, 0, len(processingRequests))
	for _, pr := range processingRequests {
		requests = append(requests, map[string]any{
			"id":           pr.ID,
			"submit_time":  pr.SubmitTime,
			"input_tokens": pr.InputTokens,
			"user_message": truncateString(pr.UserMessage, 50),
			"type":         "api",
		})
	}
	processingMutex.RUnlock()
	h.dialogueManager.mutex.RLock()
	dialogues := make([]map[string]any, 0, len(h.dialogueManager.sessions))
	for id, session := range h.dialogueManager.sessions {
		session.GeneratingMutex.RLock()
		if session.IsGenerating {
			messages, _ := h.db.GetConversationMessages(id)
			userMsg := ""
			if len(messages) > 0 {
				userMsg = truncateString(messages[len(messages)-1].Request, 50)
			}
			dialogues = append(dialogues, map[string]any{
				"id":           id,
				"submit_time":  session.LastUsedTime,
				"user_message": userMsg,
				"type":         "dialogue",
			})
		}
		session.GeneratingMutex.RUnlock()
	}
	h.dialogueManager.mutex.RUnlock()
	allRequests := append(requests, dialogues...)
	c.JSON(http.StatusOK, gin.H{"requests": allRequests})
}

func (h *Handler) StreamProcessingRequests(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			processingMutex.RLock()
			requests := make([]map[string]any, 0, len(processingRequests))
			for _, pr := range processingRequests {
				requests = append(requests, map[string]any{
					"id":           pr.ID,
					"submit_time":  pr.SubmitTime,
					"input_tokens": pr.InputTokens,
					"user_message": truncateString(pr.UserMessage, 50),
					"type":         "api",
				})
			}
			processingMutex.RUnlock()
			h.dialogueManager.mutex.RLock()
			for id, session := range h.dialogueManager.sessions {
				session.GeneratingMutex.RLock()
				if session.IsGenerating {
					messages, _ := h.db.GetConversationMessages(id)
					userMsg := ""
					if len(messages) > 0 {
						userMsg = truncateString(messages[len(messages)-1].Request, 50)
					}
					requests = append(requests, map[string]any{
						"id":           id,
						"submit_time":  session.LastUsedTime,
						"user_message": userMsg,
						"type":         "dialogue",
					})
				}
				session.GeneratingMutex.RUnlock()
			}
			h.dialogueManager.mutex.RUnlock()
			data := map[string]any{"requests": requests}
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
			flusher.Flush()
		}
	}
}

func (h *Handler) GetStats(c *gin.Context) {
	stats := h.db.GetStats()
	tpm, rpm, rpd, _ := h.db.CalculateRates()
	c.JSON(http.StatusOK, StatsResponse{
		Processing:      stats.Processing,
		Completed:       stats.Completed,
		Failed:          stats.Failed,
		TPM:             tpm,
		RPM:             rpm,
		RPD:             rpd,
		ServiceShutdown: stats.ServiceShutdown,
		ShutdownReason:  stats.ShutdownReason,
	})
}

func (h *Handler) GetRecords(c *gin.Context) {
	var req RecordsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	messages, err := h.db.GetHistory(req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get records"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (h *Handler) GetRecordDetail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing record ID"})
		return
	}
	var msgID int64
	fmt.Sscanf(id, "%d", &msgID)
	message, err := h.db.GetMessageByID(msgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		return
	}
	c.JSON(http.StatusOK, message)
}

func (h *Handler) GetProcessingRequestDetail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing request ID"})
		return
	}
	processingMutex.RLock()
	req, exists := processingRequests[id]
	processingMutex.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
		return
	}
	c.JSON(http.StatusOK, req)
}

func (h *Handler) StreamProcessingRequest(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing request ID"})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var msgID int64
	fmt.Sscanf(id, "%d", &msgID)
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			message, err := h.db.GetMessageByID(msgID)
			if err != nil {
				continue
			}
			if message.Status == "done" || message.Status == "failed" {
				data := map[string]any{
					"status":   message.Status,
					"response": message.Response,
					"done":     true,
				}
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
				flusher.Flush()
				return
			}
			data := map[string]any{
				"status": message.Status,
				"done":   false,
			}
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
			flusher.Flush()
		}
	}
}

func (h *Handler) StreamHistoryUpdates(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			messages, _ := h.db.GetHistory(100)
			data := map[string]any{"messages": messages}
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
			flusher.Flush()
		}
	}
}

func (h *Handler) GetVersionChanges(c *gin.Context) {
	changes, err := LoadVersionChanges("../src/changes.yaml")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load version changes"})
		return
	}
	c.JSON(http.StatusOK, changes)
}

func (h *Handler) GetDialogues(c *gin.Context) {
	conversations, err := h.db.GetAllConversations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dialogues"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
}

func (h *Handler) GetDialogueHistory(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing dialogue ID"})
		return
	}
	messages, err := h.db.GetConversationMessages(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dialogue history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (h *Handler) StreamDialogueResponse(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing dialogue ID"})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	lastLength := 0
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			dialogueStreamMutex.RLock()
			response, exists := dialogueStreams[conversationID]
			dialogueStreamMutex.RUnlock()
			if !exists {
				data := map[string]any{"done": true}
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
				flusher.Flush()
				return
			}
			if len(response) > lastLength {
				data := map[string]any{
					"response": response,
					"done":     false,
				}
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
				flusher.Flush()
				lastLength = len(response)
			}
		}
	}
}

func (h *Handler) DeleteDialogue(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing dialogue ID"})
		return
	}
	h.dialogueManager.DeleteSession(id)
	broadcastDialogues()
	c.JSON(http.StatusOK, gin.H{"message": "Dialogue deleted successfully"})
}

func (h *Handler) GetUsage(c *gin.Context) {
	usage := getUsage()
	c.JSON(http.StatusOK, usage)
}

func (h *Handler) CheckDeviceStatus(c *gin.Context) {
	deviceID := c.Query("device_id")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id required"})
		return
	}
	platform := c.Query("platform")
	_, err := h.db.GetOrCreateDevice(deviceID, platform)
	if err != nil {
		log.Printf("Failed to register device %s: %v", deviceID, err)
	}
	isBanned, banReason, _ := h.db.IsDeviceBanned(deviceID)
	usage := getUsage()
	c.JSON(http.StatusOK, gin.H{
		"is_banned":        isBanned,
		"ban_reason":       banReason,
		"is_blocked":       usage["is_blocked"],
		"block_reason":     usage["block_reason"],
		"block_reset_time": usage["block_reset_time"],
	})
}

func getUsage() map[string]any {
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/usage", globalConfig.GetOrganizationID())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return map[string]any{
			"five_hour_utilization":      0,
			"five_hour_resets_at":        nil,
			"seven_day_utilization":      0,
			"seven_day_resets_at":        nil,
			"seven_day_opus_utilization": 0,
			"seven_day_opus_resets_at":   nil,
		}
	}
	req.Header.Set("Cookie", globalConfig.GetCookie())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-client-platform", "web_claude_ai")
	req.Header.Set("anthropic-client-version", "1.0.0")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://claude.ai/")
	client := globalConfig.CreateHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{
			"five_hour_utilization":      0,
			"five_hour_resets_at":        nil,
			"seven_day_utilization":      0,
			"seven_day_resets_at":        nil,
			"seven_day_opus_utilization": 0,
			"seven_day_opus_resets_at":   nil,
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return map[string]any{
			"five_hour_utilization":      0,
			"five_hour_resets_at":        nil,
			"seven_day_utilization":      0,
			"seven_day_resets_at":        nil,
			"seven_day_opus_utilization": 0,
			"seven_day_opus_resets_at":   nil,
		}
	}
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[Usage API] Raw response: %s", string(body))
	var rawData map[string]any
	if err := json.Unmarshal(body, &rawData); err != nil {
		log.Printf("[Usage API] Failed to parse as map: %v", err)
		return map[string]any{
			"five_hour_utilization":      0,
			"five_hour_resets_at":        nil,
			"seven_day_utilization":      0,
			"seven_day_resets_at":        nil,
			"seven_day_opus_utilization": 0,
			"seven_day_opus_resets_at":   nil,
		}
	}
	log.Printf("[Usage API] Parsed data keys: %v", getMapKeys(rawData))
	result := map[string]any{
		"five_hour_utilization":      0,
		"five_hour_resets_at":        nil,
		"seven_day_utilization":      0,
		"seven_day_resets_at":        nil,
		"seven_day_opus_utilization": 0,
		"seven_day_opus_resets_at":   nil,
	}
	if daily, ok := rawData["daily"].(map[string]any); ok {
		if val, ok := daily["usage"].(float64); ok {
			result["five_hour_utilization"] = int(val * 100)
		} else if val, ok := daily["utilization"].(float64); ok {
			result["five_hour_utilization"] = int(val)
		}
		if resetAt, ok := daily["resets_at"].(string); ok {
			result["five_hour_resets_at"] = resetAt
		} else if resetAt, ok := daily["reset_at"].(string); ok {
			result["five_hour_resets_at"] = resetAt
		}
	} else if fiveHour, ok := rawData["five_hour"].(map[string]any); ok {
		if val, ok := fiveHour["usage"].(float64); ok {
			result["five_hour_utilization"] = int(val * 100)
		} else if val, ok := fiveHour["utilization"].(float64); ok {
			result["five_hour_utilization"] = int(val)
		}
		if resetAt, ok := fiveHour["resets_at"].(string); ok {
			result["five_hour_resets_at"] = resetAt
		} else if resetAt, ok := fiveHour["reset_at"].(string); ok {
			result["five_hour_resets_at"] = resetAt
		}
	}
	if monthly, ok := rawData["monthly"].(map[string]any); ok {
		if val, ok := monthly["usage"].(float64); ok {
			result["seven_day_utilization"] = int(val * 100)
		} else if val, ok := monthly["utilization"].(float64); ok {
			result["seven_day_utilization"] = int(val)
		}
		if resetAt, ok := monthly["resets_at"].(string); ok {
			result["seven_day_resets_at"] = resetAt
		} else if resetAt, ok := monthly["reset_at"].(string); ok {
			result["seven_day_resets_at"] = resetAt
		}
	} else if sevenDay, ok := rawData["seven_day"].(map[string]any); ok {
		if val, ok := sevenDay["usage"].(float64); ok {
			result["seven_day_utilization"] = int(val * 100)
		} else if val, ok := sevenDay["utilization"].(float64); ok {
			result["seven_day_utilization"] = int(val)
		}
		if resetAt, ok := sevenDay["resets_at"].(string); ok {
			result["seven_day_resets_at"] = resetAt
		} else if resetAt, ok := sevenDay["reset_at"].(string); ok {
			result["seven_day_resets_at"] = resetAt
		}
	}
	if sevenDayOpus, ok := rawData["seven_day_opus"].(map[string]any); ok {
		if val, ok := sevenDayOpus["usage"].(float64); ok {
			result["seven_day_opus_utilization"] = int(val * 100)
		} else if val, ok := sevenDayOpus["utilization"].(float64); ok {
			result["seven_day_opus_utilization"] = int(val)
		}
		if resetAt, ok := sevenDayOpus["resets_at"].(string); ok {
			result["seven_day_opus_resets_at"] = resetAt
		} else if resetAt, ok := sevenDayOpus["reset_at"].(string); ok {
			result["seven_day_opus_resets_at"] = resetAt
		}
	}
	log.Printf("[Usage API] Final result: %v", result)
	fiveHourUtil := 0
	sevenDayUtil := 0
	if val, ok := result["five_hour_utilization"].(int); ok {
		fiveHourUtil = val
	}
	if val, ok := result["seven_day_utilization"].(int); ok {
		sevenDayUtil = val
	}
	isBlocked := false
	blockReason := ""
	blockResetTime := ""
	if globalConfig.UsageLimitFiveHour > 0 && fiveHourUtil >= globalConfig.UsageLimitFiveHour {
		isBlocked = true
		blockReason = fmt.Sprintf("5小时用量已达 %d%%/%d%%", fiveHourUtil, globalConfig.UsageLimitFiveHour)
		if resetAt, ok := result["five_hour_resets_at"].(string); ok && resetAt != "" {
			blockResetTime = resetAt
		}
	}
	if globalConfig.UsageLimitSevenDay > 0 && sevenDayUtil >= globalConfig.UsageLimitSevenDay {
		isBlocked = true
		if blockReason != "" {
			blockReason += "\n"
		}
		blockReason += fmt.Sprintf("7天用量已达 %d%%/%d%%", sevenDayUtil, globalConfig.UsageLimitSevenDay)
		if resetAt, ok := result["seven_day_resets_at"].(string); ok && resetAt != "" {
			if blockResetTime == "" {
				blockResetTime = resetAt
			} else {
				if resetAt > blockResetTime {
					blockResetTime = resetAt
				}
			}
		}
	}
	result["is_blocked"] = isBlocked
	result["block_reason"] = blockReason
	result["block_reset_time"] = blockResetTime
	if isBlocked {
		log.Printf("[Usage API] User is BLOCKED - Reason: %s, Reset at: %s", blockReason, blockResetTime)
	}
	return result
}

func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func getStats() map[string]any {
	stats := db.GetStats()
	tpm, rpm, rpd, _ := db.CalculateRates()
	return map[string]any{
		"processing":       stats.Processing,
		"completed":        stats.Completed,
		"failed":           stats.Failed,
		"tpm":              tpm,
		"rpm":              rpm,
		"rpd":              rpd,
		"service_shutdown": stats.ServiceShutdown,
		"shutdown_reason":  stats.ShutdownReason,
	}
}

func sendInitialData() {
	stats := getStats()
	statsJSON, _ := json.Marshal(stats)
	broker.broadcast(SSEMessage{Event: "stats", Data: string(statsJSON)})
	history, _ := db.GetHistory(100)
	historyJSON, _ := json.Marshal(history)
	broker.broadcast(SSEMessage{Event: "history", Data: string(historyJSON)})
	dialogues, _ := db.GetAllConversations()
	dialoguesJSON, _ := json.Marshal(dialogues)
	broker.broadcast(SSEMessage{Event: "dialogues", Data: string(dialoguesJSON)})
	usage := getUsage()
	usageJSON, _ := json.Marshal(usage)
	broker.broadcast(SSEMessage{Event: "usage", Data: string(usageJSON)})
	apis, _ := db.GetAllAPIs()
	apisJSON, _ := json.Marshal(apis)
	broker.broadcast(SSEMessage{Event: "apis", Data: string(apisJSON)})
}

func broadcastStats() {
	stats := getStats()
	statsJSON, _ := json.Marshal(stats)
	broker.broadcast(SSEMessage{Event: "stats", Data: string(statsJSON)})
}

func broadcastHistory() {
	history, _ := db.GetHistory(100)
	historyJSON, _ := json.Marshal(history)
	broker.broadcast(SSEMessage{Event: "history", Data: string(historyJSON)})
}

func broadcastDialogues() {
	dialogues, _ := db.GetAllConversations()
	dialoguesJSON, _ := json.Marshal(dialogues)
	broker.broadcast(SSEMessage{Event: "dialogues", Data: string(dialoguesJSON)})
}

func broadcastUsage() {
	usage := getUsage()
	usageJSON, _ := json.Marshal(usage)
	broker.broadcast(SSEMessage{Event: "usage", Data: string(usageJSON)})
}

func broadcastAPIs() {
	apis, _ := db.GetAllAPIs()
	apisJSON, _ := json.Marshal(apis)
	broker.broadcast(SSEMessage{Event: "apis", Data: string(apisJSON)})
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
