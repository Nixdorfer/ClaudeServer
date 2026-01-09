package main

import (
	"encoding/json"
	"fmt"
	"io"
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
	requests := make([]map[string]interface{}, 0, len(processingRequests))
	for _, pr := range processingRequests {
		requests = append(requests, map[string]interface{}{
			"id":           pr.ID,
			"submit_time":  pr.SubmitTime,
			"input_tokens": pr.InputTokens,
			"user_message": truncateString(pr.UserMessage, 50),
			"type":         "api",
		})
	}
	processingMutex.RUnlock()

	h.dialogueManager.mutex.RLock()
	dialogues := make([]map[string]interface{}, 0, len(h.dialogueManager.sessions))
	for id, session := range h.dialogueManager.sessions {
		session.GeneratingMutex.RLock()
		if session.IsGenerating {
			messages, _ := h.db.GetConversationMessages(id)
			userMsg := ""
			if len(messages) > 0 {
				userMsg = truncateString(messages[len(messages)-1].Request, 50)
			}
			dialogues = append(dialogues, map[string]interface{}{
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
			requests := make([]map[string]interface{}, 0, len(processingRequests))
			for _, pr := range processingRequests {
				requests = append(requests, map[string]interface{}{
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
					requests = append(requests, map[string]interface{}{
						"id":           id,
						"submit_time":  session.LastUsedTime,
						"user_message": userMsg,
						"type":         "dialogue",
					})
				}
				session.GeneratingMutex.RUnlock()
			}
			h.dialogueManager.mutex.RUnlock()

			data := map[string]interface{}{"requests": requests}
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
				data := map[string]interface{}{
					"status":   message.Status,
					"response": message.Response,
					"done":     true,
				}
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
				flusher.Flush()
				return
			}

			data := map[string]interface{}{
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
			data := map[string]interface{}{"messages": messages}
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
				data := map[string]interface{}{"done": true}
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
				flusher.Flush()
				return
			}

			if len(response) > lastLength {
				data := map[string]interface{}{
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

func getUsage() map[string]interface{} {
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/usage", globalConfig.GetOrganizationID())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return map[string]interface{}{
			"five_hour_utilization":      0,
			"five_hour_resets_at":        nil,
			"seven_day_utilization":      0,
			"seven_day_resets_at":        nil,
			"seven_day_opus_utilization": 0,
			"seven_day_opus_resets_at":   nil,
		}
	}

	req.Header.Set("Cookie", globalConfig.GetCookie())
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "*/*")

	client := globalConfig.CreateHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return map[string]interface{}{
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
		return map[string]interface{}{
			"five_hour_utilization":      0,
			"five_hour_resets_at":        nil,
			"seven_day_utilization":      0,
			"seven_day_resets_at":        nil,
			"seven_day_opus_utilization": 0,
			"seven_day_opus_resets_at":   nil,
		}
	}

	body, _ := io.ReadAll(resp.Body)

	var usageData struct {
		FiveHour struct {
			Utilization float64    `json:"utilization"`
			ResetsAt    *time.Time `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			Utilization float64    `json:"utilization"`
			ResetsAt    *time.Time `json:"resets_at"`
		} `json:"seven_day"`
		SevenDayOpus struct {
			Utilization float64    `json:"utilization"`
			ResetsAt    *time.Time `json:"resets_at"`
		} `json:"seven_day_opus"`
	}

	if err := json.Unmarshal(body, &usageData); err != nil {
		return map[string]interface{}{
			"five_hour_utilization":      0,
			"five_hour_resets_at":        nil,
			"seven_day_utilization":      0,
			"seven_day_resets_at":        nil,
			"seven_day_opus_utilization": 0,
			"seven_day_opus_resets_at":   nil,
		}
	}

	return map[string]interface{}{
		"five_hour_utilization":      int(usageData.FiveHour.Utilization),
		"five_hour_resets_at":        usageData.FiveHour.ResetsAt,
		"seven_day_utilization":      int(usageData.SevenDay.Utilization),
		"seven_day_resets_at":        usageData.SevenDay.ResetsAt,
		"seven_day_opus_utilization": int(usageData.SevenDayOpus.Utilization),
		"seven_day_opus_resets_at":   usageData.SevenDayOpus.ResetsAt,
	}
}

func getStats() map[string]interface{} {
	stats := db.GetStats()
	tpm, rpm, rpd, _ := db.CalculateRates()

	return map[string]interface{}{
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
