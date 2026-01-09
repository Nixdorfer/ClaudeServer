package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type UsageResponse struct {
	FiveHourUtilization     float64 `json:"five_hour_utilization"`
	FiveHourResetsAt        *string `json:"five_hour_resets_at"`
	SevenDayUtilization     float64 `json:"seven_day_utilization"`
	SevenDayResetsAt        *string `json:"seven_day_resets_at"`
	SevenDayOpusUtilization float64 `json:"seven_day_opus_utilization"`
	SevenDayOpusResetsAt    *string `json:"seven_day_opus_resets_at"`
}

type UsageStatus struct {
	FiveHour            float64 `json:"five_hour"`
	FiveHourReset       string  `json:"five_hour_reset"`
	SevenDay            float64 `json:"seven_day"`
	SevenDayReset       string  `json:"seven_day_reset"`
	SevenDaySonnet      float64 `json:"seven_day_sonnet"`
	SevenDaySonnetReset string  `json:"seven_day_sonnet_reset"`
	IsBlocked           bool    `json:"is_blocked"`
	BlockReason         string  `json:"block_reason"`
	BlockResetTime      string  `json:"block_reset_time"`
	LimitFiveHour       float64 `json:"limit_five_hour"`
	LimitSevenDay       float64 `json:"limit_seven_day"`
}

type App struct {
	ctx            context.Context
	conn           *websocket.Conn
	connMutex      sync.RWMutex
	serverURL      string
	requestID      int64
	requestMux     sync.Mutex
	storage        *LocalStorage
	config         *ClientConfig
	cachedUsage    *UsageStatus
	usageCacheTime time.Time
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type DialogueRequest struct {
	Request        string `json:"request"`
	ConversationID string `json:"conversation_id,omitempty"`
	Model          string `json:"model,omitempty"`
	Style          string `json:"style,omitempty"`
}

type APIRequest struct {
	RequestID int64  `json:"request_id"`
	Endpoint  string `json:"endpoint"`
	Method    string `json:"method"`
}

type Conversation struct {
	ID             string    `json:"conversation_id"`
	MessageCount   int       `json:"message_count"`
	LastUsedTime   time.Time `json:"last_used_time"`
	IsGenerating   bool      `json:"is_generating"`
	FirstMessage   string    `json:"first_message,omitempty"`
}

type Message struct {
	ID             int64      `json:"id"`
	ConversationID string     `json:"conversation_id"`
	ExchangeNumber int        `json:"exchange_number"`
	Request        string     `json:"request"`
	Response       string     `json:"response"`
	Status         string     `json:"status"`
	ReceiveTime    time.Time  `json:"receive_time"`
	SendTime       *time.Time `json:"send_time,omitempty"`
	ResponseTime   *time.Time `json:"response_time,omitempty"`
	Duration       *float64   `json:"duration,omitempty"`
}

func NewApp() *App {
	config := LoadConfig()
	app := &App{
		serverURL: config.ServerURL,
		config:    config,
	}
	return app
}

func (a *App) SetServerURL(url string) {
	a.serverURL = url
}

func (a *App) GetServerURL() string {
	return a.serverURL
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	storage, err := NewLocalStorage()
	if err != nil {
		log.Printf("本地存储初始化失败: %v", err)
	} else {
		a.storage = storage
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := a.Connect(); err != nil {
			log.Printf("Auto-connect failed: %v", err)
		}
	}()
}

func (a *App) shutdown(ctx context.Context) {
	if a.storage != nil {
		a.storage.Close()
	}
	a.Disconnect()
}

func (a *App) Connect() error {
	a.connMutex.Lock()
	defer a.connMutex.Unlock()

	if a.conn != nil {
		a.conn.Close()
	}

	log.Printf("正在连接到 %s", a.serverURL)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig:  tlsConfig,
	}

	headers := http.Header{}
	headers.Set("Origin", "https://claude.nixdorfer.com")
	headers.Set("Host", "claude.nixdorfer.com")
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	headers.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	conn, resp, err := dialer.Dial(a.serverURL, headers)
	if err != nil {
		errMsg := fmt.Sprintf("连接失败: %v", err)
		if resp != nil {
			errMsg = fmt.Sprintf("连接失败: %v (HTTP状态: %d)", err, resp.StatusCode)
		}
		log.Printf(errMsg)
		runtime.EventsEmit(a.ctx, "connection_error", errMsg)
		return fmt.Errorf(errMsg)
	}

	// 设置 pong 处理器，收到 pong 时更新读取截止时间
	conn.SetPongHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	log.Printf("WebSocket 连接成功")
	a.conn = conn
	runtime.EventsEmit(a.ctx, "connected", true)

	go a.listenMessages()
	go a.keepAlive()

	return nil
}

func (a *App) Disconnect() {
	a.connMutex.Lock()
	defer a.connMutex.Unlock()

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	runtime.EventsEmit(a.ctx, "disconnected", true)
}

func (a *App) IsConnected() bool {
	a.connMutex.RLock()
	defer a.connMutex.RUnlock()
	return a.conn != nil
}

func (a *App) listenMessages() {
	for {
		a.connMutex.RLock()
		conn := a.conn
		a.connMutex.RUnlock()

		if conn == nil {
			return
		}

		// 设置读取截止时间，防止连接静默断开
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))

		_, message, err := conn.ReadMessage()
		if err != nil {
			// 检查是否是正常关闭
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket 正常关闭")
			} else {
				log.Printf("Read error: %v", err)
				runtime.EventsEmit(a.ctx, "connection_error", err.Error())
			}
			a.Disconnect()
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("JSON parse error: %v", err)
			continue
		}

		switch msg.Type {
		case "connected":
			runtime.EventsEmit(a.ctx, "ws_connected", msg.Data)
		case "conversation_id":
			runtime.EventsEmit(a.ctx, "conversation_id", msg.Data)
		case "content":
			runtime.EventsEmit(a.ctx, "content", msg.Data)
		case "done":
			runtime.EventsEmit(a.ctx, "done", msg.Data)
		case "error":
			runtime.EventsEmit(a.ctx, "ws_error", msg.Data)
		case "response":
			runtime.EventsEmit(a.ctx, "api_response", msg.Data)
		case "pong":
			// pong 由 PongHandler 处理，这里忽略
		default:
			runtime.EventsEmit(a.ctx, "ws_message", msg)
		}
	}
}

// keepAlive 定期发送 ping 保持连接活跃
func (a *App) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		a.connMutex.RLock()
		conn := a.conn
		a.connMutex.RUnlock()

		if conn == nil {
			return
		}

		// 发送 WebSocket ping 帧
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			log.Printf("Ping 发送失败: %v", err)
			a.Disconnect()
			return
		}
	}
}

func (a *App) SendMessage(conversationID, message string) error {
	a.connMutex.RLock()
	conn := a.conn
	a.connMutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	req := WSMessage{
		Type: "dialogue",
		Data: DialogueRequest{
			Request:        message,
			ConversationID: conversationID,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

func (a *App) GetDialogues() error {
	return a.sendAPIRequest("/api/dialogues", "GET")
}

func (a *App) GetHistory(conversationID string) error {
	return a.sendAPIRequest(fmt.Sprintf("/api/dialogues/%s/history", conversationID), "GET")
}

func (a *App) DeleteDialogue(conversationID string) error {
	return a.sendAPIRequest(fmt.Sprintf("/api/dialogues/%s", conversationID), "DELETE")
}

func (a *App) GetStats() error {
	return a.sendAPIRequest("/api/stats", "GET")
}

func (a *App) GetUsage() error {
	return a.sendAPIRequest("/api/usage", "GET")
}

func (a *App) sendAPIRequest(endpoint, method string) error {
	a.connMutex.RLock()
	conn := a.conn
	a.connMutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	a.requestMux.Lock()
	a.requestID++
	reqID := a.requestID
	a.requestMux.Unlock()

	req := WSMessage{
		Type: "api_request",
		Data: APIRequest{
			RequestID: reqID,
			Endpoint:  endpoint,
			Method:    method,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

func (a *App) Ping() error {
	a.connMutex.RLock()
	conn := a.conn
	a.connMutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	req := WSMessage{
		Type: "ping",
		Data: nil,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

func (a *App) GetLocalConversations() []LocalConversation {
	if a.storage == nil {
		return []LocalConversation{}
	}
	convs, err := a.storage.GetConversations()
	if err != nil {
		log.Printf("获取本地对话失败: %v", err)
		return []LocalConversation{}
	}
	return convs
}

func (a *App) CreateLocalConversation(id, firstMessage string) error {
	if a.storage == nil {
		return fmt.Errorf("storage not initialized")
	}
	return a.storage.CreateConversation(id, firstMessage)
}

func (a *App) RenameConversation(id, name string) error {
	if a.storage == nil {
		return fmt.Errorf("storage not initialized")
	}
	return a.storage.UpdateConversationName(id, name)
}

func (a *App) DeleteLocalConversation(id string) error {
	if a.storage == nil {
		return fmt.Errorf("storage not initialized")
	}
	return a.storage.DeleteConversation(id)
}

func (a *App) GetLocalMessages(conversationID string) []LocalMessage {
	if a.storage == nil {
		return []LocalMessage{}
	}
	msgs, err := a.storage.GetMessages(conversationID)
	if err != nil {
		log.Printf("获取本地消息失败: %v", err)
		return []LocalMessage{}
	}
	return msgs
}

func (a *App) SaveLocalMessage(conversationID, role, content string) error {
	if a.storage == nil {
		return fmt.Errorf("storage not initialized")
	}
	if !a.storage.ConversationExists(conversationID) {
		if err := a.storage.CreateConversation(conversationID, ""); err != nil {
			return err
		}
	}
	return a.storage.AddMessage(conversationID, role, content)
}

func (a *App) UpdateLocalMessage(conversationID, content string) error {
	if a.storage == nil {
		return fmt.Errorf("storage not initialized")
	}
	return a.storage.UpdateLastMessage(conversationID, content)
}

func (a *App) GenerateConversationID() string {
	return fmt.Sprintf("local_%d", time.Now().UnixNano())
}

func (a *App) FetchUsage() (*UsageStatus, error) {
	if a.cachedUsage != nil && time.Since(a.usageCacheTime) < 30*time.Second {
		return a.cachedUsage, nil
	}

	url := "https://claude.nixdorfer.com/api/usage"

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Origin", "https://claude.nixdorfer.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取用量失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("用量API返回非200状态: %d, 响应: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("用量API返回错误状态: %d", resp.StatusCode)
	}

	var usageResp UsageResponse
	if err := json.Unmarshal(body, &usageResp); err != nil {
		log.Printf("用量API响应解析失败, 原始响应: %s", string(body))
		return nil, fmt.Errorf("解析用量数据失败: %v", err)
	}

	status := &UsageStatus{
		LimitFiveHour:  a.config.UsageLimitFiveHour,
		LimitSevenDay:  a.config.UsageLimitSevenDay,
		FiveHour:       usageResp.FiveHourUtilization,
		SevenDay:       usageResp.SevenDayUtilization,
		SevenDaySonnet: usageResp.SevenDayOpusUtilization,
	}

	if usageResp.FiveHourResetsAt != nil {
		status.FiveHourReset = *usageResp.FiveHourResetsAt
	}
	if usageResp.SevenDayResetsAt != nil {
		status.SevenDayReset = *usageResp.SevenDayResetsAt
	}
	if usageResp.SevenDayOpusResetsAt != nil {
		status.SevenDaySonnetReset = *usageResp.SevenDayOpusResetsAt
	}

	if status.FiveHour > a.config.UsageLimitFiveHour {
		status.IsBlocked = true
		status.BlockReason = "5小时用量过高"
		status.BlockResetTime = status.FiveHourReset
	} else if status.SevenDay > a.config.UsageLimitSevenDay {
		status.IsBlocked = true
		status.BlockReason = "周总用量过高"
		status.BlockResetTime = status.SevenDayReset
	}

	a.cachedUsage = status
	a.usageCacheTime = time.Now()

	return status, nil
}

func (a *App) GetUsageStatus() *UsageStatus {
	status, err := a.FetchUsage()
	if err != nil {
		log.Printf("获取用量失败: %v", err)
		return &UsageStatus{
			LimitFiveHour: a.config.UsageLimitFiveHour,
			LimitSevenDay: a.config.UsageLimitSevenDay,
		}
	}
	return status
}

func (a *App) CheckUsageLimit() (bool, string, string) {
	status, err := a.FetchUsage()
	if err != nil {
		log.Printf("检查用量失败: %v", err)
		return false, "", ""
	}

	if status.IsBlocked {
		return true, status.BlockReason, status.BlockResetTime
	}
	return false, "", ""
}

func (a *App) SetUsageLimits(fiveHour, sevenDay float64) {
	a.config.UsageLimitFiveHour = fiveHour
	a.config.UsageLimitSevenDay = sevenDay
	a.cachedUsage = nil
	SaveConfig(a.config)
}

func (a *App) FormatResetTime(isoTime string) string {
	t, err := time.Parse(time.RFC3339, isoTime)
	if err != nil {
		return isoTime
	}
	return t.Local().Format("01-02 15:04")
}

func (a *App) GetConfig() *ClientConfig {
	return a.config
}

func (a *App) UpdateConfig(serverURL string, fiveHourLimit, sevenDayLimit float64) {
	if serverURL != "" {
		a.config.ServerURL = serverURL
		a.serverURL = serverURL
	}
	if fiveHourLimit > 0 {
		a.config.UsageLimitFiveHour = fiveHourLimit
	}
	if sevenDayLimit > 0 {
		a.config.UsageLimitSevenDay = sevenDayLimit
	}
	a.cachedUsage = nil
	SaveConfig(a.config)
}
