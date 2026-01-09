package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	*gorm.DB
	stats      Stats
	statsMutex sync.RWMutex
}

type Stats struct {
	Processing      int
	Completed       int
	Failed          int
	ServiceShutdown bool
	ShutdownReason  string
}

type CldDevice struct {
	DeviceID  string    `gorm:"primaryKey;type:varchar(64)" json:"device_id"`
	Platform  string    `gorm:"type:varchar(16)" json:"platform"`
	FirstSeen time.Time `gorm:"not null;autoCreateTime" json:"first_seen"`
	IsBanned  bool      `gorm:"not null;default:false" json:"is_banned"`
	BanReason string    `gorm:"type:text" json:"ban_reason"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CldDevice) TableName() string {
	return "cld_device"
}

type CldMessage struct {
	ID             int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ConversationID string     `gorm:"type:varchar(36);not null;index:idx_cld_conversation_id" json:"conversation_id"`
	DeviceID       string     `gorm:"type:varchar(64);index:idx_cld_device_id" json:"device_id"`
	ExchangeNumber int        `gorm:"not null" json:"exchange_number"`
	Request        string     `gorm:"type:text;not null" json:"request"`
	Response       string     `gorm:"type:text" json:"response"`
	ReceiveTime    time.Time  `gorm:"not null" json:"receive_time"`
	SendTime       *time.Time `json:"send_time"`
	ResponseTime   *time.Time `json:"response_time"`
	Duration       *float64   `json:"duration"`
	RequestTokens  *int       `json:"request_tokens"`
	ResponseTokens *int       `json:"response_tokens"`
	Tokens         *int       `json:"tokens"`
	Status         string     `gorm:"type:varchar(20);not null;default:'processing';index:idx_cld_status" json:"status"`
	Notice         string     `gorm:"type:text" json:"notice"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (CldMessage) TableName() string {
	return "cld_message"
}

type Message = CldMessage

func InitDB(cfg *Config) (*Database, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %v", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := db.AutoMigrate(&CldDevice{}); err != nil {
		return nil, fmt.Errorf("migrate cld_device table failed: %v", err)
	}
	if !db.Migrator().HasTable(&CldMessage{}) {
		if err := db.Migrator().CreateTable(&CldMessage{}); err != nil {
			return nil, fmt.Errorf("create cld_message table failed: %v", err)
		}
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_conversation_id ON cld_message(conversation_id)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_receive_time ON cld_message(receive_time DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_status ON cld_message(status)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_conversation_exchange ON cld_message(conversation_id, exchange_number)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_device_id ON cld_message(device_id)`)
	log.Println("Database initialized successfully")
	return &Database{DB: db}, nil
}

func (d *Database) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (d *Database) LoadStats() error {
	var completed, failed int64
	if err := d.Model(&Message{}).Where("status = ?", "done").Count(&completed).Error; err != nil {
		return err
	}
	if err := d.Model(&Message{}).Where("status != ? AND status != ?", "done", "processing").Count(&failed).Error; err != nil {
		return err
	}
	d.statsMutex.Lock()
	d.stats.Completed = int(completed)
	d.stats.Failed = int(failed)
	d.statsMutex.Unlock()
	return nil
}

func (d *Database) GetStats() Stats {
	d.statsMutex.RLock()
	defer d.statsMutex.RUnlock()
	return d.stats
}

func (d *Database) IncrementProcessing() {
	d.statsMutex.Lock()
	d.stats.Processing++
	d.statsMutex.Unlock()
}

func (d *Database) DecrementProcessing() {
	d.statsMutex.Lock()
	d.stats.Processing--
	d.statsMutex.Unlock()
}

func (d *Database) IncrementCompleted() {
	d.statsMutex.Lock()
	d.stats.Completed++
	d.statsMutex.Unlock()
}

func (d *Database) IncrementFailed() {
	d.statsMutex.Lock()
	d.stats.Failed++
	d.statsMutex.Unlock()
}

func (d *Database) SetShutdown(reason string) {
	d.statsMutex.Lock()
	d.stats.ServiceShutdown = true
	d.stats.ShutdownReason = reason
	d.statsMutex.Unlock()
}

func (d *Database) IsShutdown() bool {
	d.statsMutex.RLock()
	defer d.statsMutex.RUnlock()
	return d.stats.ServiceShutdown
}

func (d *Database) CreateMessage(msg *Message) error {
	err := d.Create(msg).Error
	if err == nil {
		broadcastHistory()
		broadcastStats()
	}
	return err
}

func (d *Database) UpdateMessage(msg *Message) error {
	err := d.Save(msg).Error
	if err == nil {
		broadcastHistory()
		broadcastStats()
	}
	return err
}

func (d *Database) GetMessageByID(id int64) (*Message, error) {
	var msg Message
	err := d.First(&msg, id).Error
	return &msg, err
}

func (d *Database) GetConversationMessages(conversationID string) ([]Message, error) {
	var messages []Message
	err := d.Where("conversation_id = ?", conversationID).
		Order("exchange_number ASC").
		Find(&messages).Error
	return messages, err
}

func (d *Database) GetRecentMessages(limit int) ([]Message, error) {
	var messages []Message
	err := d.Order("receive_time DESC").Limit(limit).Find(&messages).Error
	return messages, err
}

func (d *Database) GetMessagesAfterTime(afterTime time.Time, limit int) ([]Message, error) {
	var messages []Message
	err := d.Where("receive_time > ?", afterTime).
		Order("receive_time DESC").
		Limit(limit).
		Find(&messages).Error
	return messages, err
}

func (d *Database) GetMessagesByStatus(status string) ([]Message, error) {
	var messages []Message
	err := d.Where("status = ?", status).Find(&messages).Error
	return messages, err
}

func (d *Database) DeleteConversation(conversationID string) error {
	return d.Where("conversation_id = ?", conversationID).Delete(&Message{}).Error
}

func (d *Database) CalculateRates() (tpm, rpm, rpd float64, err error) {
	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)
	oneDayAgo := now.Add(-24 * time.Hour)
	var tokenCount int64
	err = d.Model(&Message{}).
		Where("receive_time >= ? AND status = ?", oneMinuteAgo, "done").
		Select("COALESCE(SUM(tokens), 0)").
		Scan(&tokenCount).Error
	if err != nil {
		return 0, 0, 0, err
	}
	tpm = float64(tokenCount)
	var requestCount int64
	err = d.Model(&Message{}).
		Where("receive_time >= ?", oneMinuteAgo).
		Count(&requestCount).Error
	if err != nil {
		return 0, 0, 0, err
	}
	rpm = float64(requestCount)
	var dailyCount int64
	err = d.Model(&Message{}).
		Where("receive_time >= ?", oneDayAgo).
		Count(&dailyCount).Error
	if err != nil {
		return 0, 0, 0, err
	}
	rpd = float64(dailyCount)
	return tpm, rpm, rpd, nil
}

func (d *Database) GetNextExchangeNumber(conversationID string) (int, error) {
	var maxNum int
	err := d.Model(&Message{}).
		Where("conversation_id = ?", conversationID).
		Select("COALESCE(MAX(exchange_number), 0)").
		Scan(&maxNum).Error
	return maxNum + 1, err
}

func (d *Database) GetHistory(limit int) ([]Message, error) {
	var messages []Message
	err := d.Order("receive_time DESC").Limit(limit).Find(&messages).Error
	return messages, err
}

type ConversationInfo struct {
	ConversationID string    `json:"conversation_id"`
	LastMessage    string    `json:"last_message"`
	UpdatedAt      time.Time `json:"updated_at"`
	MessageCount   int       `json:"message_count"`
}

func (d *Database) GetAllConversations() ([]ConversationInfo, error) {
	var results []ConversationInfo
	err := d.Raw(`
		SELECT
			conversation_id,
			MAX(request) as last_message,
			MAX(receive_time) as updated_at,
			COUNT(*) as message_count
		FROM cld_message
		GROUP BY conversation_id
		ORDER BY MAX(receive_time) DESC
	`).Scan(&results).Error
	return results, err
}

type APIInfo struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Method      string `json:"method"`
}

func (d *Database) GetAllAPIs() ([]APIInfo, error) {
	apis := []APIInfo{
		{"/v1/chat/completions", "OpenAI兼容的对话API", "POST"},
		{"/v1/models", "获取可用模型列表", "GET"},
		{"/api/tags", "Ollama兼容的模型列表", "GET"},
		{"/api/chat", "Ollama兼容的对话API", "POST"},
		{"/health", "健康检查", "GET"},
		{"/api/stats", "获取统计数据", "GET"},
		{"/api/records", "获取增量记录", "POST"},
		{"/api/record/:id", "获取单条记录详情", "GET"},
		{"/api/processing", "获取处理中请求", "GET"},
		{"/api/usage", "获取用量信息", "GET"},
		{"/api/dialogues", "获取对话列表", "GET"},
		{"/api/dialogues/:id/history", "获取对话历史", "GET"},
		{"/api/dialogues/:id", "删除对话", "DELETE"},
		{"/chat/dialogue/http", "对话聊天接口", "POST"},
	}
	return apis, nil
}

func (d *Database) GetOrCreateDevice(deviceID string, platform string) (*CldDevice, error) {
	var device CldDevice
	err := d.Where("device_id = ?", deviceID).First(&device).Error
	if err == nil {
		if platform != "" && device.Platform != platform {
			d.Model(&device).Update("platform", platform)
			device.Platform = platform
		}
		return &device, nil
	}
	device = CldDevice{
		DeviceID:  deviceID,
		Platform:  platform,
		FirstSeen: time.Now(),
		IsBanned:  false,
	}
	if err := d.Create(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (d *Database) IsDeviceBanned(deviceID string) (bool, string, error) {
	var device CldDevice
	err := d.Where("device_id = ?", deviceID).First(&device).Error
	if err != nil {
		return false, "", nil
	}
	return device.IsBanned, device.BanReason, nil
}

func (d *Database) BanDevice(deviceID string, reason string) error {
	return d.Model(&CldDevice{}).Where("device_id = ?", deviceID).Updates(map[string]any{
		"is_banned":  true,
		"ban_reason": reason,
	}).Error
}

func (d *Database) UnbanDevice(deviceID string) error {
	return d.Model(&CldDevice{}).Where("device_id = ?", deviceID).Updates(map[string]any{
		"is_banned":  false,
		"ban_reason": "",
	}).Error
}

func (d *Database) GetAllDevices() ([]CldDevice, error) {
	var devices []CldDevice
	err := d.Order("first_seen DESC").Find(&devices).Error
	return devices, err
}

func (d *Database) GetBannedDevices() ([]CldDevice, error) {
	var devices []CldDevice
	err := d.Where("is_banned = ?", true).Order("updated_at DESC").Find(&devices).Error
	return devices, err
}
