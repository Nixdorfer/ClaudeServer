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
	devicePassword := c.GetHeader("X-Device-ID")
	if devicePassword == "" {
		devicePassword = uuid.New().String()
	}
	platform := c.GetHeader("X-Platform")
	if platform == "" {
		platform = "windows"
	}
	device, err := h.db.GetOrCreateDevice(devicePassword, platform)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create device"})
		return
	}
	convUID := uuid.New().String()
	conv, err := h.db.CreateConversation(device.ID, convUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}
	userMessage := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			userMessage = m.Content
			break
		}
	}
	dialogueUID := uuid.New().String()
	dialogue := &CldDialogue{
		UID:            dialogueUID,
		ConversationID: conv.ID,
		Order:          1,
		UserMessage:    userMessage,
		CreateTime:     time.Now(),
		Status:         "processing",
		PromptID:       h.db.GetCurrentPromptID(),
	}
	h.db.CreateDialogue(dialogue)
	h.db.IncrementProcessing()
	requestID := uuid.New().String()
	processingMutex.Lock()
	processingRequests[requestID] = &ProcessingRequest{
		ID:          requestID,
		SubmitTime:  time.Now(),
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
		requestTime := time.Now()
		dialogue.RequestTime = &requestTime
		cookie := h.config.GetCookie()
		response, err := sendChatCompletion(h.config.GetOrganizationID(), cookie, req.Messages, req.Model, req.Stream)
		finishTime := time.Now()
		dialogue.FinishTime = &finishTime
		duration := int(finishTime.Sub(dialogue.CreateTime).Milliseconds())
		dialogue.Duration = &duration
		if err != nil {
			dialogue.Status = "send_failed"
			h.db.UpdateDialogue(dialogue)
			h.db.IncrementFailed()
			LogExchange(userMessage, err.Error(), true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		assistantMsg := response.Choices[0].Message.Content
		dialogue.AssistantMessage = &assistantMsg
		dialogue.Status = "done"
		h.db.UpdateDialogue(dialogue)
		h.db.IncrementCompleted()
		LogExchange(userMessage, assistantMsg, false)
		c.JSON(http.StatusOK, response)
	default:
		dialogue.Status = "send_failed"
		h.db.UpdateDialogue(dialogue)
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
	devicePassword := c.GetHeader("X-Device-ID")
	if devicePassword == "" {
		devicePassword = uuid.New().String()
	}
	platform := c.GetHeader("X-Platform")
	if platform == "" {
		platform = "windows"
	}
	device, err := h.db.GetOrCreateDevice(devicePassword, platform)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create device"})
		return
	}
	convUID := uuid.New().String()
	conv, err := h.db.CreateConversation(device.ID, convUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}
	userMessage := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			userMessage = m.Content
			break
		}
	}
	dialogueUID := uuid.New().String()
	dialogue := &CldDialogue{
		UID:            dialogueUID,
		ConversationID: conv.ID,
		Order:          1,
		UserMessage:    userMessage,
		CreateTime:     time.Now(),
		Status:         "processing",
		PromptID:       h.db.GetCurrentPromptID(),
	}
	h.db.CreateDialogue(dialogue)
	h.db.IncrementProcessing()
	requestID := uuid.New().String()
	processingMutex.Lock()
	processingRequests[requestID] = &ProcessingRequest{
		ID:          requestID,
		SubmitTime:  time.Now(),
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
		requestTime := time.Now()
		dialogue.RequestTime = &requestTime
		cookie := h.config.GetCookie()
		openaiReq := OpenAIChatRequest{
			Model:    req.Model,
			Messages: req.Messages,
			Stream:   req.Stream,
		}
		response, err := sendChatCompletion(h.config.GetOrganizationID(), cookie, openaiReq.Messages, openaiReq.Model, openaiReq.Stream)
		finishTime := time.Now()
		dialogue.FinishTime = &finishTime
		duration := int(finishTime.Sub(dialogue.CreateTime).Milliseconds())
		dialogue.Duration = &duration
		if err != nil {
			dialogue.Status = "send_failed"
			h.db.UpdateDialogue(dialogue)
			h.db.IncrementFailed()
			LogExchange(userMessage, err.Error(), true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		assistantMsg := response.Choices[0].Message.Content
		dialogue.AssistantMessage = &assistantMsg
		dialogue.Status = "done"
		h.db.UpdateDialogue(dialogue)
		h.db.IncrementCompleted()
		LogExchange(userMessage, assistantMsg, false)
		ollamaResponse := OllamaChatResponse{
			Model:     req.Model,
			CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			Message:   response.Choices[0].Message,
			Done:      true,
		}
		c.JSON(http.StatusOK, ollamaResponse)
	default:
		dialogue.Status = "send_failed"
		h.db.UpdateDialogue(dialogue)
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
	var claudeConversationID string
	var parentMessageUUID string
	if req.ConversationID != "" {
		claudeConversationID = req.ConversationID
		session := h.dialogueManager.GetOrCreateSession(claudeConversationID)
		session.GeneratingMutex.RLock()
		parentMessageUUID = session.LastMessageUUID
		session.GeneratingMutex.RUnlock()
		if parentMessageUUID == "00000000-0000-4000-8000-000000000000" {
			newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), claudeConversationID, cookie)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conversation history"})
				return
			}
			parentMessageUUID = newParentUUID
			h.dialogueManager.UpdateSession(claudeConversationID, parentMessageUUID)
		}
	} else {
		var err error
		claudeConversationID, err = createConversation(h.config.GetOrganizationID(), cookie, true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
			return
		}
		parentMessageUUID = "00000000-0000-4000-8000-000000000000"
		h.dialogueManager.GetOrCreateSession(claudeConversationID)
	}
	devicePassword := c.GetHeader("X-Device-ID")
	if devicePassword == "" {
		devicePassword = uuid.New().String()
	}
	platform := c.GetHeader("X-Platform")
	if platform == "" {
		platform = "windows"
	}
	device, _ := h.db.GetOrCreateDevice(devicePassword, platform)
	conv, _ := h.db.CreateConversation(device.ID, claudeConversationID)
	dialogueOrder, _ := h.db.GetNextDialogueOrder(conv.ID)
	dialogueUID := uuid.New().String()
	dialogue := &CldDialogue{
		UID:            dialogueUID,
		ConversationID: conv.ID,
		Order:          dialogueOrder,
		UserMessage:    req.Request,
		CreateTime:     time.Now(),
		Status:         "waiting",
		PromptID:       h.db.GetCurrentPromptID(),
	}
	h.db.CreateDialogue(dialogue)
	session := h.dialogueManager.GetOrCreateSession(claudeConversationID)
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
		dialogue.Status = "processing"
		h.db.UpdateDialogue(dialogue)
		dialogueStreamMutex.Lock()
		dialogueStreams[claudeConversationID] = ""
		dialogueStreamMutex.Unlock()
		response, err := sendDialogueMessageWithOptions(
			h.config.GetOrganizationID(),
			claudeConversationID,
			cookie,
			req.Request,
			parentMessageUUID,
			req.Model,
			req.Style,
			func(chunk string) {
				dialogueStreamMutex.Lock()
				dialogueStreams[claudeConversationID] = chunk
				dialogueStreamMutex.Unlock()
			},
		)
		dialogueStreamMutex.Lock()
		delete(dialogueStreams, claudeConversationID)
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
		newParentUUID, err := getConversationHistory(h.config.GetOrganizationID(), claudeConversationID, cookie)
		if err == nil {
			h.dialogueManager.UpdateSession(claudeConversationID, newParentUUID)
		}
		dialogue.AssistantMessage = &response
		dialogue.Status = "done"
		h.db.UpdateDialogue(dialogue)
		LogExchange(req.Request, response, false)
		c.JSON(http.StatusOK, DialogueResponse{
			ConversationID: claudeConversationID,
			Response:       response,
		})
	default:
		dialogue.Status = "send_failed"
		h.db.UpdateDialogue(dialogue)
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
			dialogues = append(dialogues, map[string]any{
				"id":          id,
				"submit_time": session.LastUsedTime,
				"type":        "dialogue",
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
					requests = append(requests, map[string]any{
						"id":          id,
						"submit_time": session.LastUsedTime,
						"type":        "dialogue",
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
	dialogues, err := h.db.GetHistory(req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get records"})
		return
	}
	if h.config.PrivateMode {
		result := make([]map[string]any, len(dialogues))
		for i, d := range dialogues {
			conv, _ := h.db.GetConversation(d.ConversationID)
			deviceID := 0
			if conv != nil {
				deviceID = conv.DeviceID
			}
			result[i] = map[string]any{
				"id":          d.ID,
				"device_id":   deviceID,
				"create_time": d.CreateTime,
				"duration":    d.Duration,
				"status":      d.Status,
				"request":     truncateRunes(d.UserMessage, 5),
			}
		}
		c.JSON(http.StatusOK, gin.H{"messages": result, "private_mode": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": dialogues})
}

func (h *Handler) GetRecordDetail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing record ID"})
		return
	}
	var dialogueID int
	fmt.Sscanf(id, "%d", &dialogueID)
	dialogue, err := h.db.GetDialogueByID(dialogueID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		return
	}
	c.JSON(http.StatusOK, dialogue)
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
	var dialogueID int
	fmt.Sscanf(id, "%d", &dialogueID)
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			dialogue, err := h.db.GetDialogueByID(dialogueID)
			if err != nil {
				continue
			}
			if dialogue.Status == "done" || dialogue.Status == "send_failed" || dialogue.Status == "reply_failed" {
				response := ""
				if dialogue.AssistantMessage != nil {
					response = *dialogue.AssistantMessage
				}
				data := map[string]any{
					"status":   dialogue.Status,
					"response": response,
					"done":     true,
				}
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
				flusher.Flush()
				return
			}
			data := map[string]any{
				"status": dialogue.Status,
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
	changes, err := LoadVersionChanges("src/changes.yaml")
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
	var convID int
	fmt.Sscanf(id, "%d", &convID)
	dialogues, err := h.db.GetConversationDialogues(convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dialogue history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": dialogues})
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
	devicePassword := c.Query("device_id")
	if devicePassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id required"})
		return
	}
	platform := c.Query("platform")
	device, err := h.db.GetOrCreateDevice(devicePassword, platform)
	if err != nil {
		log.Printf("Failed to register device %s: %v", devicePassword, err)
	}
	deviceID := 0
	if device != nil {
		deviceID = device.ID
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

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func (h *Handler) GetUIConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"private_mode": h.config.PrivateMode,
	})
}

type ErrorReportRequest struct {
	Error          string `json:"error"`
	ConversationID string `json:"conversation_id"`
	DeviceID       string `json:"device_id"`
	Platform       string `json:"platform"`
	Version        string `json:"version"`
}

func (h *Handler) ReportError(c *gin.Context) {
	var req ErrorReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if req.Error == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error message is required"})
		return
	}
	if err := h.db.SaveError(req.ConversationID, req.Error, req.DeviceID, req.Platform, req.Version); err != nil {
		log.Printf("Failed to save error report: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Error reported successfully"})
}

type UpdateDeviceNoticeRequest struct {
	DeviceID string `json:"device_id"`
	Notice   string `json:"notice"`
}

func (h *Handler) UpdateDeviceNotice(c *gin.Context) {
	var req UpdateDeviceNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if req.DeviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required"})
		return
	}
	if err := h.db.UpdateDeviceNotice(req.DeviceID, req.Notice); err != nil {
		log.Printf("Failed to update device notice: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update notice"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Notice updated successfully"})
}
