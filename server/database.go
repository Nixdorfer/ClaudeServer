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
	ID            int       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform      string    `gorm:"type:varchar;not null" json:"platform"`
	CreateTime    time.Time `gorm:"type:timestamptz;default:CURRENT_TIMESTAMP;not null" json:"create_time"`
	UpdateTime    time.Time `gorm:"type:timestamptz;default:CURRENT_TIMESTAMP;not null" json:"update_time"`
	Notice        *string   `gorm:"type:varchar" json:"notice"`
	Banned        bool      `gorm:"default:false;not null" json:"banned"`
	BanReason     *string   `gorm:"type:varchar" json:"ban_reason"`
	Admin         bool      `gorm:"default:false;not null" json:"admin"`
	AdminPassword string    `gorm:"type:varchar;not null;uniqueIndex" json:"-"`
	Fingerprint   string    `gorm:"type:varchar;not null;uniqueIndex" json:"fingerprint"`
}

func (CldDevice) TableName() string {
	return "cld_device"
}

type CldConversation struct {
	ID       int    `gorm:"primaryKey;autoIncrement" json:"id"`
	UID      string `gorm:"type:varchar;not null;uniqueIndex" json:"uid"`
	DeviceID int    `gorm:"not null;index" json:"device_id"`
}

func (CldConversation) TableName() string {
	return "cld_conversation"
}

type CldDialogue struct {
	ID               int        `gorm:"primaryKey;autoIncrement" json:"id"`
	UID              string     `gorm:"type:varchar;not null;uniqueIndex" json:"uid"`
	ConversationID   int        `gorm:"not null;index" json:"conversation_id"`
	Order            int        `gorm:"column:order;default:1;not null" json:"order"`
	UserMessage      string     `gorm:"type:text;not null" json:"user_message"`
	AssistantMessage *string    `gorm:"type:text" json:"assistant_message"`
	CreateTime       time.Time  `gorm:"type:timestamptz;default:CURRENT_TIMESTAMP;not null" json:"create_time"`
	FinishTime       *time.Time `gorm:"type:timestamptz" json:"finish_time"`
	RequestTime      *time.Time `gorm:"type:timetz" json:"request_time"`
	Status           string     `gorm:"type:varchar;default:'processing';not null" json:"status"`
	Duration         *int       `json:"duration"`
	PromptID         *int       `gorm:"index" json:"prompt_id"`
}

type CldPrompt struct {
	ID         int       `gorm:"primaryKey;autoIncrement" json:"id"`
	Prompt     string    `gorm:"type:text" json:"prompt"`
	UpdateTime time.Time `gorm:"type:timestamptz;default:CURRENT_TIMESTAMP;not null" json:"update_time"`
}

func (CldDialogue) TableName() string {
	return "cld_dialogue"
}

func (CldPrompt) TableName() string {
	return "cld_prompt"
}

type CldError struct {
	ID             int       `gorm:"primaryKey;autoIncrement" json:"id"`
	ConversationID string    `gorm:"type:varchar" json:"conversation_id"`
	Error          string    `gorm:"type:text;not null" json:"error"`
	DeviceID       string    `gorm:"type:varchar" json:"device_id"`
	Platform       string    `gorm:"type:varchar" json:"platform"`
	Version        string    `gorm:"type:varchar" json:"version"`
	CreateTime     time.Time `gorm:"type:timestamptz;default:CURRENT_TIMESTAMP;not null" json:"create_time"`
}

func (CldError) TableName() string {
	return "cld_error"
}

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
	if err := db.AutoMigrate(&CldDevice{}, &CldConversation{}, &CldDialogue{}, &CldError{}, &CldPrompt{}); err != nil {
		return nil, fmt.Errorf("failed to auto migrate: %v", err)
	}
	db.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'cld_device_check') THEN
				ALTER TABLE cld_device ADD CONSTRAINT cld_device_check
				CHECK (platform IN ('windows', 'android', 'linux', 'macos', 'ios'));
			END IF;
		END $$;
	`)
	db.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'cld_dialogue_check') THEN
				ALTER TABLE cld_dialogue ADD CONSTRAINT cld_dialogue_check
				CHECK (status IN ('waiting', 'processing', 'replying', 'done', 'send_failed', 'reply_failed'));
			END IF;
		END $$;
	`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_dialogue_conversation_id ON cld_dialogue(conversation_id)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_dialogue_create_time ON cld_dialogue(create_time DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_dialogue_status ON cld_dialogue(status)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_conversation_device_id ON cld_conversation(device_id)`)
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
	if err := d.Model(&CldDialogue{}).Where("status = ?", "done").Count(&completed).Error; err != nil {
		return err
	}
	if err := d.Model(&CldDialogue{}).Where("status NOT IN ?", []string{"done", "processing", "waiting", "replying"}).Count(&failed).Error; err != nil {
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

func (d *Database) CreateDialogue(dialogue *CldDialogue) error {
	err := d.Create(dialogue).Error
	if err == nil {
		broadcastHistory()
		broadcastStats()
	}
	return err
}

func (d *Database) UpdateDialogue(dialogue *CldDialogue) error {
	err := d.Save(dialogue).Error
	if err == nil {
		broadcastHistory()
		broadcastStats()
	}
	return err
}

func (d *Database) GetDialogueByID(id int) (*CldDialogue, error) {
	var dialogue CldDialogue
	err := d.First(&dialogue, id).Error
	return &dialogue, err
}

func (d *Database) GetConversationDialogues(conversationID int) ([]CldDialogue, error) {
	var dialogues []CldDialogue
	err := d.Where("conversation_id = ?", conversationID).
		Order(`"order" ASC`).
		Find(&dialogues).Error
	return dialogues, err
}

func (d *Database) GetRecentDialogues(limit int) ([]CldDialogue, error) {
	var dialogues []CldDialogue
	err := d.Order("create_time DESC").Limit(limit).Find(&dialogues).Error
	return dialogues, err
}

func (d *Database) GetDialoguesByStatus(status string) ([]CldDialogue, error) {
	var dialogues []CldDialogue
	err := d.Where("status = ?", status).Find(&dialogues).Error
	return dialogues, err
}

func (d *Database) DeleteConversation(conversationID int) error {
	return d.Where("id = ?", conversationID).Delete(&CldConversation{}).Error
}

func (d *Database) CalculateRates() (tpm, rpm, rpd float64, err error) {
	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)
	oneDayAgo := now.Add(-24 * time.Hour)
	var requestCount int64
	err = d.Model(&CldDialogue{}).
		Where("create_time >= ?", oneMinuteAgo).
		Count(&requestCount).Error
	if err != nil {
		return 0, 0, 0, err
	}
	rpm = float64(requestCount)
	var dailyCount int64
	err = d.Model(&CldDialogue{}).
		Where("create_time >= ?", oneDayAgo).
		Count(&dailyCount).Error
	if err != nil {
		return 0, 0, 0, err
	}
	rpd = float64(dailyCount)
	return 0, rpm, rpd, nil
}

func (d *Database) GetNextDialogueOrder(conversationID int) (int, error) {
	var maxOrder int
	err := d.Model(&CldDialogue{}).
		Where("conversation_id = ?", conversationID).
		Select(`COALESCE(MAX("order"), 0)`).
		Scan(&maxOrder).Error
	return maxOrder + 1, err
}

func (d *Database) GetHistory(limit int) ([]CldDialogue, error) {
	var dialogues []CldDialogue
	err := d.Order("create_time DESC").Limit(limit).Find(&dialogues).Error
	return dialogues, err
}

type ConversationInfo struct {
	ID           int       `json:"id"`
	DeviceID     int       `json:"device_id"`
	LastMessage  string    `json:"last_message"`
	UpdatedAt    time.Time `json:"updated_at"`
	DialogueCount int      `json:"dialogue_count"`
}

func (d *Database) GetAllConversations() ([]ConversationInfo, error) {
	var results []ConversationInfo
	err := d.Raw(`
		SELECT
			c.id,
			c.device_id,
			(SELECT user_message FROM cld_dialogue WHERE conversation_id = c.id ORDER BY "order" DESC LIMIT 1) as last_message,
			(SELECT create_time FROM cld_dialogue WHERE conversation_id = c.id ORDER BY "order" DESC LIMIT 1) as updated_at,
			(SELECT COUNT(*) FROM cld_dialogue WHERE conversation_id = c.id) as dialogue_count
		FROM cld_conversation c
		ORDER BY updated_at DESC NULLS LAST
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

func (d *Database) GetOrCreateDevice(fingerprint string, platform string) (*CldDevice, error) {
	var device CldDevice
	err := d.Where("fingerprint = ?", fingerprint).First(&device).Error
	if err == nil {
		d.Model(&device).Update("update_time", time.Now())
		if platform != "" && device.Platform != platform {
			d.Model(&device).Update("platform", platform)
			device.Platform = platform
		}
		return &device, nil
	}
	device = CldDevice{
		Platform:      platform,
		CreateTime:    time.Now(),
		UpdateTime:    time.Now(),
		Banned:        false,
		Admin:         false,
		AdminPassword: "",
		Fingerprint:   fingerprint,
	}
	if err := d.Create(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (d *Database) GetDeviceByID(id int) (*CldDevice, error) {
	var device CldDevice
	err := d.First(&device, id).Error
	return &device, err
}

func (d *Database) GetDeviceByFingerprint(fingerprint string) (*CldDevice, error) {
	var device CldDevice
	err := d.Where("fingerprint = ?", fingerprint).First(&device).Error
	return &device, err
}

func (d *Database) IsDeviceBanned(deviceID int) (bool, string, error) {
	var device CldDevice
	err := d.First(&device, deviceID).Error
	if err != nil {
		return false, "", nil
	}
	banReason := ""
	if device.BanReason != nil {
		banReason = *device.BanReason
	}
	return device.Banned, banReason, nil
}

func (d *Database) BanDevice(deviceID int, reason string) error {
	return d.Model(&CldDevice{}).Where("id = ?", deviceID).Updates(map[string]any{
		"banned":     true,
		"ban_reason": reason,
	}).Error
}

func (d *Database) UnbanDevice(deviceID int) error {
	return d.Model(&CldDevice{}).Where("id = ?", deviceID).Updates(map[string]any{
		"banned":     false,
		"ban_reason": nil,
	}).Error
}

func (d *Database) GetAllDevices() ([]CldDevice, error) {
	var devices []CldDevice
	err := d.Order("create_time DESC").Find(&devices).Error
	return devices, err
}

func (d *Database) GetBannedDevices() ([]CldDevice, error) {
	var devices []CldDevice
	err := d.Where("banned = ?", true).Order("update_time DESC").Find(&devices).Error
	return devices, err
}

func (d *Database) CreateConversation(deviceID int, uid string) (*CldConversation, error) {
	conv := CldConversation{
		UID:      uid,
		DeviceID: deviceID,
	}
	if err := d.Create(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}
func (d *Database) GetConversationByUID(uid string) (*CldConversation, error) {
	var conv CldConversation
	err := d.Where("uid = ?", uid).First(&conv).Error
	return &conv, err
}

func (d *Database) GetConversation(id int) (*CldConversation, error) {
	var conv CldConversation
	err := d.First(&conv, id).Error
	return &conv, err
}

func (d *Database) GetDeviceConversations(deviceID int) ([]CldConversation, error) {
	var conversations []CldConversation
	err := d.Where("device_id = ?", deviceID).Find(&conversations).Error
	return conversations, err
}

func (d *Database) IsDeviceAdmin(fingerprint string, password string) bool {
	var device CldDevice
	err := d.Where("fingerprint = ?", fingerprint).First(&device).Error
	if err != nil {
		return false
	}
	return device.Admin && device.AdminPassword == password
}

func (d *Database) UpdateDeviceNotice(fingerprint string, notice string) error {
	return d.Model(&CldDevice{}).Where("fingerprint = ?", fingerprint).Update("notice", notice).Error
}

func (d *Database) GetDialogueWithConversation(dialogueID int) (*CldDialogue, *CldConversation, *CldDevice, error) {
	var dialogue CldDialogue
	if err := d.First(&dialogue, dialogueID).Error; err != nil {
		return nil, nil, nil, err
	}
	var conv CldConversation
	if err := d.First(&conv, dialogue.ConversationID).Error; err != nil {
		return nil, nil, nil, err
	}
	var device CldDevice
	if err := d.First(&device, conv.DeviceID).Error; err != nil {
		return nil, nil, nil, err
	}
	return &dialogue, &conv, &device, nil
}

func (d *Database) SaveError(conversationID, errorMsg, deviceID, platform, version string) error {
	errRecord := CldError{
		ConversationID: conversationID,
		Error:          errorMsg,
		DeviceID:       deviceID,
		Platform:       platform,
		Version:        version,
		CreateTime:     time.Now(),
	}
	return d.Create(&errRecord).Error
}

func (d *Database) GetCurrentPromptID() *int {
	var prompt CldPrompt
	err := d.Order("id DESC").First(&prompt).Error
	if err != nil {
		return nil
	}
	return &prompt.ID
}

func (d *Database) CreatePrompt(promptText string) (*CldPrompt, error) {
	prompt := CldPrompt{
		Prompt:     promptText,
		UpdateTime: time.Now(),
	}
	if err := d.Create(&prompt).Error; err != nil {
		return nil, err
	}
	return &prompt, nil
}

func (d *Database) GetLatestPrompt() (*CldPrompt, error) {
	var prompt CldPrompt
	err := d.Order("id DESC").First(&prompt).Error
	if err != nil {
		return nil, err
	}
	return &prompt, nil
}
