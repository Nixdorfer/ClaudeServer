package main

import "time"

type OpenAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type DialogueRequest struct {
	ConversationID string        `json:"conversation_id,omitempty"`
	Request        string        `json:"request"`
	Model          string        `json:"model,omitempty"`
	Style          string        `json:"style,omitempty"`
	Files          []RequestFile `json:"files,omitempty"`
	KeepAlive      bool          `json:"keep_alive,omitempty"`
}

type DialogueResponse struct {
	ConversationID string `json:"conversation_id"`
	Response       string `json:"response"`
}

type DialogueStreamRequest struct {
	ConversationID string        `json:"conversation_id,omitempty"`
	Request        string        `json:"request"`
	Model          string        `json:"model,omitempty"`
	Style          string        `json:"style,omitempty"`
	Files          []RequestFile `json:"files,omitempty"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RequestFile struct {
	Name       string `json:"name"`
	Content    string `json:"content"`
	Type       string `json:"type"`
	ContentRaw []byte `json:"-"`
}

type OpenAIResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []OpenAIResponseChoice `json:"choices"`
	Usage   OpenAIUsage            `json:"usage"`
}

type OpenAIResponseChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OllamaTagsResponse struct {
	Models []OllamaModel `json:"models"`
}

type OllamaModel struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
	Digest     string `json:"digest"`
}

type OllamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type OllamaChatResponse struct {
	Model     string        `json:"model"`
	CreatedAt string        `json:"created_at"`
	Message   OpenAIMessage `json:"message"`
	Done      bool          `json:"done"`
}

type RecordsRequest struct {
	After time.Time `json:"after"`
	Limit int       `json:"limit"`
}

type StatsResponse struct {
	Processing      int     `json:"processing"`
	Completed       int     `json:"completed"`
	Failed          int     `json:"failed"`
	TPM             float64 `json:"tpm"`
	RPM             float64 `json:"rpm"`
	RPD             float64 `json:"rpd"`
	ServiceShutdown bool    `json:"service_shutdown"`
	ShutdownReason  string  `json:"shutdown_reason,omitempty"`
}

type MCPRemoteServer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	AuthType    string `json:"auth_type"`
	IsConnected bool   `json:"is_connected"`
	Enabled     bool   `json:"enabled"`
}

type MCPRemoteServersResponse struct {
	Servers []MCPRemoteServer `json:"servers"`
}

type MCPAuthRequest struct {
	RedirectURL   string `json:"redirect_url"`
	OpenInBrowser bool   `json:"open_in_browser"`
}

type MCPAuthResponse struct {
	AuthURL string `json:"auth_url"`
	Status  string `json:"status"`
}

type MCPLogoutResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type MCPTool struct {
	ServerID string `json:"server_id"`
	Name     string `json:"name"`
	ToolID   string `json:"tool_id"`
}

type MCPToolEnableRequest struct {
	EnabledMCPTools map[string]bool `json:"enabled_mcp_tools"`
}
