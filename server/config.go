package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Tokens            TokensConfig         `yaml:"tokens"`
	Debug             bool                 `yaml:"debug"`
	PrivateMode       bool                 `yaml:"private_mode"`
	ThreadNum         int                  `yaml:"thread_num"`
	ServerPort        int                  `yaml:"server_port"`
	MinClientVersion  string               `yaml:"min_client_version"`
	APIEndpoint       string               `yaml:"api_endpoint"`
	Proxy             ProxyConfig          `yaml:"proxy"`
	MaxTPM            int                  `yaml:"max_tpm"`
	MaxRPM            int                  `yaml:"max_rpm"`
	MaxRPD            int                  `yaml:"max_rpd"`
	RequestIntervalMS int                  `yaml:"request_interval_ms"`
	UsageLimitFiveHour int                 `yaml:"usage_limit_five_hour"`
	UsageLimitSevenDay int                 `yaml:"usage_limit_seven_day"`
	DBHost            string               `yaml:"db_host"`
	DBPort            int                  `yaml:"db_port"`
	DBUser            string               `yaml:"db_user"`
	DBPassword        string               `yaml:"db_password"`
	DBName            string               `yaml:"db_name"`
	Models            []ModelConfig        `yaml:"models"`
	DefaultModel      string               `yaml:"default_model"`
	DefaultStyle      string               `yaml:"default_style"`
	Styles            []StyleConfig        `yaml:"styles"`
	MCPConnectors     []MCPConnectorConfig `yaml:"mcp_connectors"`
	OrganizationID    string               `yaml:"organization_id,omitempty"`
	SessionKey        string               `yaml:"sessionKey,omitempty"`
	Cookie            string               `yaml:"cookie,omitempty"`
}

type TokensConfig struct {
	OrganizationID    string `yaml:"organization_id"`
	SessionKey        string `yaml:"session_key"`
	CFClearance       string `yaml:"cf_clearance"`
	CFBm              string `yaml:"cf_bm"`
	DeviceID          string `yaml:"device_id"`
	AnonymousID       string `yaml:"anonymous_id"`
	UserID            string `yaml:"user_id,omitempty"`
	ActivitySessionID string `yaml:"activity_session_id,omitempty"`
	IntercomDeviceID  string `yaml:"intercom_device_id,omitempty"`
	IntercomSessionID string `yaml:"intercom_session_id,omitempty"`
}

type ProxyConfig struct {
	Enable bool   `yaml:"enable"`
	HTTP   string `yaml:"http"`
	HTTPS  string `yaml:"https"`
}

type ModelConfig struct {
	ID      string `yaml:"id" json:"id"`
	Object  string `yaml:"object" json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type StyleConfig struct {
	Key     string `yaml:"key"`
	Name    string `yaml:"name"`
	Summary string `yaml:"summary"`
	Prompt  string `yaml:"prompt"`
}

type MCPConnectorConfig struct {
	Name    string `yaml:"name"`
	UUID    string `yaml:"uuid"`
	Enabled bool   `yaml:"enabled"`
}

func LoadConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		templatePath := "src/config-template.yaml"
		if _, err := os.Stat(templatePath); err == nil {
			log.Printf("\n⚠️  配置文件不存在: %s\n", path)
			log.Println("请按照以下步骤创建配置文件：")
			log.Println("1. 复制模板文件：cp server/src/config-template.yaml server/src/config.yaml")
			log.Println("2. 编辑 server/src/config.yaml，填入您的配置信息")
			log.Println("   - organization_id: 从 claude.ai URL 中获取")
			log.Println("   - sessionKey: 从浏览器 Cookie 中获取")
			log.Println()
			os.Exit(0)
		}
		return createDefaultConfig(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	config.SetDefaults()
	return &config, nil
}

func createDefaultConfig(path string) (*Config, error) {
	template := Config{
		OrganizationID: "你的组织ID",
		SessionKey:     "你的sessionKey值",
		Debug:          false,
		ThreadNum:      5,
		ServerPort:     5000,
		APIEndpoint:    "http://localhost:5000",
		MaxTPM:         100000,
		MaxRPM:         50,
		MaxRPD:         3000,
		DBHost:         "localhost",
		DBPort:         5432,
		DBUser:         "postgres",
		DBPassword:     "xdfrt123",
		DBName:         "claude_db",
		Models: []ModelConfig{
			{ID: "claude-sonnet-4.5", Object: "model"},
			{ID: "claude-opus-4.1", Object: "model"},
		},
	}
	data, _ := yaml.Marshal(template)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("创建配置文件失败: %v", err)
	}
	log.Printf("✓ 已创建配置文件: %s\n\n", path)
	log.Println("请按以下步骤配置:")
	log.Println("1. 打开浏览器访问 https://claude.ai")
	log.Println("2. 按 F12 打开开发者工具")
	log.Println("3. 切换到 Application/存储 标签")
	log.Println("4. 找到 Cookies → https://claude.ai")
	log.Println("5. 复制 sessionKey 的值")
	log.Println("6. 从任意请求的 URL 中复制组织ID(organizations/后面的UUID)")
	log.Println("7. 修改 config.json 文件,填入这些信息")
	log.Println()
	os.Exit(0)
	return nil, nil
}

func (c *Config) Validate() error {
	orgID := c.GetOrganizationID()
	if orgID == "你的组织ID" || orgID == "" {
		return fmt.Errorf("请在配置文件中填入正确的 organization_id\n" +
			"获取方法：访问 claude.ai，从 URL 中复制组织ID (organizations/后面的UUID)")
	}
	sessionKey := c.GetSessionKey()
	if sessionKey == "你的sessionKey值" || sessionKey == "" {
		return fmt.Errorf("请在配置文件中填入正确的 session_key\n" +
			"获取方法：打开浏览器开发者工具，在 Cookie 中找到 sessionKey 的值")
	}
	if c.Tokens.SessionKey != "" && len(c.MCPConnectors) > 0 {
		hasEnabledMCP := false
		for _, connector := range c.MCPConnectors {
			if connector.Enabled {
				hasEnabledMCP = true
				break
			}
		}
		if hasEnabledMCP && c.Debug {
			if c.Tokens.CFClearance == "" || c.Tokens.CFBm == "" {
				log.Printf("[DEBUG] 提示：MCP WebSocket 连接可能需要 Cloudflare token")
				log.Printf("[DEBUG] 如果连接失败，请尝试在配置文件的 tokens 部分添加：")
				log.Printf("[DEBUG]   - cf_clearance 和 cf_bm")
				log.Printf("[DEBUG] 获取方法见配置文件注释\n")
			}
		}
	}
	return nil
}

func (c *Config) SetDefaults() {
	if c.ThreadNum <= 0 {
		c.ThreadNum = 5
	}
	if c.ServerPort <= 0 {
		c.ServerPort = 5000
	}
	if c.DBHost == "" {
		c.DBHost = "localhost"
	}
	if c.DBPort <= 0 {
		c.DBPort = 5432
	}
	if c.DBUser == "" {
		c.DBUser = "postgres"
	}
	if c.DBName == "" {
		c.DBName = "claude_db"
	}
}

func (c *Config) GetServerAddr() string {
	return fmt.Sprintf(":%d", c.ServerPort)
}

func (c *Config) GetCookie() string {
	if c.Tokens.SessionKey != "" {
		return c.Tokens.BuildCookie()
	}
	if c.Cookie != "" {
		return c.Cookie
	}
	if c.SessionKey != "" {
		return fmt.Sprintf("sessionKey=%s", c.SessionKey)
	}
	return ""
}

func (c *Config) GetOrganizationID() string {
	if c.Tokens.OrganizationID != "" {
		return c.Tokens.OrganizationID
	}
	return c.OrganizationID
}

func (c *Config) GetSessionKey() string {
	if c.Tokens.SessionKey != "" {
		return c.Tokens.SessionKey
	}
	return c.SessionKey
}

func (t *TokensConfig) BuildCookie() string {
	var parts []string
	if t.SessionKey != "" {
		parts = append(parts, fmt.Sprintf("sessionKey=%s", t.SessionKey))
	}
	if t.DeviceID != "" {
		parts = append(parts, fmt.Sprintf("anthropic-device-id=%s", t.DeviceID))
	}
	if t.CFClearance != "" {
		parts = append(parts, fmt.Sprintf("cf_clearance=%s", t.CFClearance))
	}
	if t.CFBm != "" {
		parts = append(parts, fmt.Sprintf("__cf_bm=%s", t.CFBm))
	}
	if t.OrganizationID != "" {
		parts = append(parts, fmt.Sprintf("lastActiveOrg=%s", t.OrganizationID))
	}
	if t.AnonymousID != "" {
		parts = append(parts, fmt.Sprintf("ajs_anonymous_id=%s", t.AnonymousID))
	}
	if t.UserID != "" {
		parts = append(parts, fmt.Sprintf("ajs_user_id=%s", t.UserID))
	}
	if t.ActivitySessionID != "" {
		parts = append(parts, fmt.Sprintf("activitySessionId=%s", t.ActivitySessionID))
	}
	if t.IntercomDeviceID != "" {
		parts = append(parts, fmt.Sprintf("intercom-device-id-lupk8zyo=%s", t.IntercomDeviceID))
	}
	if t.IntercomSessionID != "" {
		parts = append(parts, fmt.Sprintf("intercom-session-lupk8zyo=%s", t.IntercomSessionID))
	}
	cookie := joinCookieParts(parts)
	if globalConfig != nil && globalConfig.Debug {
		log.Printf("[DEBUG] Built Cookie with %d fields:", len(parts))
		for _, part := range parts {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				key := kv[0]
				value := kv[1]
				if len(value) > 10 {
					log.Printf("[DEBUG]   %s: %s...[%d chars]", key, value[:10], len(value))
				} else {
					log.Printf("[DEBUG]   %s: [%d chars]", key, len(value))
				}
			}
		}
		log.Printf("[DEBUG] Total Cookie length: %d chars", len(cookie))
	}
	return cookie
}

func joinCookieParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "; " + parts[i]
	}
	return result
}

func CompareVersions(current, required string) bool {
	parseVersion := func(v string) []int {
		parts := strings.Split(v, ".")
		result := make([]int, 3)
		for i := 0; i < len(parts) && i < 3; i++ {
			fmt.Sscanf(parts[i], "%d", &result[i])
		}
		return result
	}
	curr := parseVersion(current)
	req := parseVersion(required)
	for i := 0; i < 3; i++ {
		if curr[i] < req[i] {
			return false
		}
		if curr[i] > req[i] {
			return true
		}
	}
	return true
}

func (c *Config) CreateHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{}
	if c.Proxy.Enable {
		if c.Proxy.HTTP != "" || c.Proxy.HTTPS != "" {
			transport.Proxy = func(req *http.Request) (*url.URL, error) {
				if req.URL.Scheme == "https" && c.Proxy.HTTPS != "" {
					return url.Parse(c.Proxy.HTTPS)
				} else if c.Proxy.HTTP != "" {
					return url.Parse(c.Proxy.HTTP)
				}
				return nil, nil
			}
			if c.Debug {
				log.Printf("[DEBUG] Proxy enabled - HTTP: %s, HTTPS: %s", c.Proxy.HTTP, c.Proxy.HTTPS)
			}
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
