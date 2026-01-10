package models

type LocalConversation struct {
	ConversationId string `json:"conversation_id"`
	Name           string `json:"name"`
	FirstMessage   string `json:"first_message"`
	MessageCount   int    `json:"message_count"`
	LastUsedTime   string `json:"last_used_time"`
	IsGenerating   bool   `json:"is_generating"`
}

type LocalMessage struct {
	Id             int64  `json:"id"`
	ConversationId string `json:"conversation_id"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	Timestamp      string `json:"timestamp"`
}

type LocalDialogue struct {
	ID               int     `json:"id"`
	UID              string  `json:"uid"`
	ConversationID   int     `json:"conversation_id"`
	Order            int     `json:"order"`
	UserMessage      string  `json:"user_message"`
	AssistantMessage *string `json:"assistant_message"`
	CreateTime       string  `json:"create_time"`
	FinishTime       *string `json:"finish_time"`
	RequestTime      *string `json:"request_time"`
	Status           string  `json:"status"`
	Duration         *int    `json:"duration"`
}

type LocalConv struct {
	ID       int    `json:"id"`
	UID      string `json:"uid"`
	DeviceID int    `json:"device_id"`
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
}

type UpdateCheckResult struct {
	HasUpdate      bool     `json:"has_update"`
	CurrentVersion string   `json:"current_version"`
	LatestVersion  string   `json:"latest_version"`
	Notes          []string `json:"notes"`
	DownloadUrl    string   `json:"download_url"`
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type DialogueRequest struct {
	Request        string `json:"request"`
	ConversationId string `json:"conversation_id,omitempty"`
	DeviceId       string `json:"device_id"`
}

type UsageResponse struct {
	FiveHourUtilization     float64 `json:"five_hour_utilization"`
	FiveHourResetsAt        string  `json:"five_hour_resets_at"`
	SevenDayUtilization     float64 `json:"seven_day_utilization"`
	SevenDayResetsAt        string  `json:"seven_day_resets_at"`
	SevenDayOpusUtilization float64 `json:"seven_day_opus_utilization"`
	SevenDayOpusResetsAt    string  `json:"seven_day_opus_resets_at"`
	IsBlocked               bool    `json:"is_blocked"`
	BlockReason             string  `json:"block_reason"`
	BlockResetTime          string  `json:"block_reset_time"`
}

type VersionInfo struct {
	Version string   `json:"version"`
	Note    []string `json:"note"`
	Url     string   `json:"url"`
}
