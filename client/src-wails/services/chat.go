package services

import (
	"bytes"
	"claudechat/models"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorBlue   = "\033[34m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

func isDevMode() bool {
	return os.Getenv("DEV_MODE") == "true"
}

func logSend(data string) {
	if !isDevMode() {
		return
	}
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal([]byte(data), &prettyJSON); err == nil {
		formatted, _ := json.MarshalIndent(prettyJSON, "", "  ")
		fmt.Printf("%s[SEND] →%s\n%s\n", colorCyan, colorReset, string(formatted))
	} else {
		fmt.Printf("%s[SEND] →%s %s\n", colorCyan, colorReset, data)
	}
}

func logRecv(data []byte) {
	if !isDevMode() {
		return
	}
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(data, &prettyJSON); err == nil {
		formatted, _ := json.MarshalIndent(prettyJSON, "", "  ")
		fmt.Printf("%s[RECV] ←%s\n%s\n", colorYellow, colorReset, string(formatted))
	} else {
		fmt.Printf("%s[RECV] ←%s %s\n", colorYellow, colorReset, string(data))
	}
}

func getServerUrl() string {
	if os.Getenv("DEV_MODE") == "true" {
		return "ws://localhost:5000/data/websocket/create"
	}
	return "wss://claude.nixdorfer.com/data/websocket/create"
}

type ChatService struct {
	app       *application.App
	conn      *websocket.Conn
	connected bool
	mu        sync.RWMutex
	sendChan  chan string
	db        *DatabaseService
	version   string
}

func NewChatService(db *DatabaseService, version string) *ChatService {
	return &ChatService{
		db:       db,
		version:  version,
		sendChan: make(chan string, 100),
	}
}

func (c *ChatService) SetApp(app *application.App) {
	c.app = app
}

func (c *ChatService) Connect() error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	header := http.Header{}
	header.Set("Origin", "https://claude.nixdorfer.com")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	header.Set("X-Device-ID", GetHwid())
	header.Set("X-Client-Version", c.version)
	header.Set("X-Platform", runtime.GOOS)
	conn, _, err := dialer.Dial(getServerUrl(), header)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()
	c.app.Event.Emit("connected", true)
	go c.writeLoop()
	go c.readLoop()
	return nil
}

func (c *ChatService) writeLoop() {
	for msg := range c.sendChan {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			break
		}
		logSend(msg)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			break
		}
	}
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

func (c *ChatService) readLoop() {
	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			break
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			c.app.Event.Emit("connection_error", err.Error())
			break
		}
		logRecv(msg)
		var wsMsg models.WSMessage
		if err := json.Unmarshal(msg, &wsMsg); err != nil {
			continue
		}
		switch wsMsg.Type {
		case "connected":
			c.app.Event.Emit("ws_connected", wsMsg.Data)
		case "version_outdated":
			c.app.Event.Emit("version_outdated", wsMsg.Data)
		case "banned":
			c.app.Event.Emit("device_banned", wsMsg.Data)
		case "conversation_id":
			c.app.Event.Emit("conversation_id", wsMsg.Data)
		case "content":
			c.app.Event.Emit("content", wsMsg.Data)
		case "done":
			c.app.Event.Emit("done", wsMsg.Data)
			if dataMap, ok := wsMsg.Data.(map[string]interface{}); ok {
				if dialogueID, ok := dataMap["dialogue_id"].(float64); ok {
					c.sendAck(int(dialogueID))
				}
			}
		case "error":
			c.app.Event.Emit("ws_error", wsMsg.Data)
		case "usage_blocked":
			c.app.Event.Emit("usage_blocked", wsMsg.Data)
		default:
			c.app.Event.Emit("ws_message", wsMsg)
		}
	}
	c.mu.Lock()
	c.connected = false
	c.conn = nil
	c.mu.Unlock()
	c.app.Event.Emit("disconnected", true)
}

func (c *ChatService) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	return nil
}

func (c *ChatService) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *ChatService) SendMessage(conversationId, message string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("Not connected")
	}
	c.mu.RUnlock()
	req := models.WSMessage{
		Type: "dialogue",
		Data: models.DialogueRequest{
			Request:        message,
			ConversationId: conversationId,
			DeviceId:       GetHwid(),
		},
	}
	jsonData, err := json.Marshal(req)
	if err != nil {
		return err
	}
	select {
	case c.sendChan <- string(jsonData):
		return nil
	default:
		return fmt.Errorf("Send buffer full")
	}
}

func (c *ChatService) GetLocalConversations() ([]models.LocalConversation, error) {
	return c.db.GetConversations()
}

func (c *ChatService) GetLocalMessages(conversationId string) ([]models.LocalMessage, error) {
	return c.db.GetMessages(conversationId)
}

func (c *ChatService) SaveLocalMessage(conversationId, role, content string) error {
	if !c.db.ConversationExists(conversationId) {
		if err := c.db.CreateConversation(conversationId, ""); err != nil {
			return err
		}
	}
	return c.db.SaveMessage(conversationId, role, content)
}

func (c *ChatService) CreateLocalConversation(id, firstMessage string) error {
	return c.db.CreateConversation(id, firstMessage)
}

func (c *ChatService) RenameConversation(id, name string) error {
	return c.db.RenameConversation(id, name)
}

func (c *ChatService) DeleteLocalConversation(id string) error {
	return c.db.DeleteConversation(id)
}

func (c *ChatService) GenerateConversationId() string {
	return fmt.Sprintf("local_%d", time.Now().UnixNano())
}

func (c *ChatService) sendAck(dialogueID int) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()
	req := models.WSMessage{
		Type: "ack",
		Data: map[string]interface{}{
			"dialogue_id": dialogueID,
		},
	}
	jsonData, err := json.Marshal(req)
	if err != nil {
		return
	}
	select {
	case c.sendChan <- string(jsonData):
	default:
	}
}
func (c *ChatService) ReportError(errorMessage, conversationId string) error {
	apiUrl := "https://claude.nixdorfer.com/api/error"
	if os.Getenv("DEV_MODE") == "true" {
		apiUrl = "http://localhost:5000/api/error"
	}
	reqBody := map[string]string{
		"error":           errorMessage,
		"conversation_id": conversationId,
		"device_id":       GetHwid(),
		"platform":        runtime.GOOS,
		"version":         c.version,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Post(apiUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ChatService) UpdateDeviceNotice(notice string) error {
	apiUrl := "https://claude.nixdorfer.com/api/device/notice"
	if os.Getenv("DEV_MODE") == "true" {
		apiUrl = "http://localhost:5000/api/device/notice"
	}
	reqBody := map[string]string{
		"device_id": GetHwid(),
		"notice":    notice,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Post(apiUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	return nil
}
