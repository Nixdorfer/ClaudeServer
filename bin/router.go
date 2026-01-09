package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

func getStaticPath() string {
	staticPath := "../static"

	absPath, err := filepath.Abs(staticPath)
	if err != nil {
		log.Printf("警告: 无法获取静态文件绝对路径: %v", err)
		return staticPath
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		log.Printf("警告: 静态文件目录不存在: %s", absPath)
		return staticPath
	}

	log.Printf("静态文件目录: %s", absPath)
	return absPath
}

func SetupRouter(cfg *Config, db *Database) *gin.Engine {
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(CORSMiddleware())

	handler := NewHandler(cfg, db)

	r.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/static/dashboard/" {
			c.Redirect(http.StatusMovedPermanently, "/static/dashboard/dashboard.html")
			c.Abort()
			return
		}
		if path == "/static/requests/" {
			c.Redirect(http.StatusMovedPermanently, "/static/processing/processing.html")
			c.Abort()
			return
		}
		if path == "/static/processing/" {
			c.Redirect(http.StatusMovedPermanently, "/static/processing/processing.html")
			c.Abort()
			return
		}
		if path == "/static/dialogues/" {
			c.Redirect(http.StatusMovedPermanently, "/static/dialogues/dialogues.html")
			c.Abort()
			return
		}
		if path == "/static/history/" {
			c.Redirect(http.StatusMovedPermanently, "/static/history/history.html")
			c.Abort()
			return
		}
		if path == "/static/apis/" {
			c.Redirect(http.StatusMovedPermanently, "/static/apis/apis.html")
			c.Abort()
			return
		}
		if path == "/static/changes/" {
			c.Redirect(http.StatusMovedPermanently, "/static/changes/changes.html")
			c.Abort()
			return
		}
		if path == "/static/about/" {
			c.Redirect(http.StatusMovedPermanently, "/static/about/about.html")
			c.Abort()
			return
		}
		c.Next()
	})

	r.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if filepath.Ext(path) == ".html" {
			c.Header("Content-Type", "text/html; charset=utf-8")
		} else if filepath.Ext(path) == ".js" {
			c.Header("Content-Type", "application/javascript; charset=utf-8")
		} else if filepath.Ext(path) == ".css" {
			c.Header("Content-Type", "text/css; charset=utf-8")
		}
		c.Next()
	})

	staticPath := getStaticPath()
	r.Static("/static", staticPath)

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/static/dashboard/dashboard.html")
	})

	faviconPath := filepath.Join(staticPath, "favicon.ico")
	r.StaticFile("/favicon.ico", faviconPath)
	r.GET("/health", handler.HealthCheck)

	r.GET("/.well-known/appspecific/com.chrome.devtools.json", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	chat := r.Group("/chat")
	{
		chat.POST("/dialogue/http", RateLimitMiddleware(cfg, db), handler.DialogueChatEnhanced)
		chat.GET("/dialogue/event", RateLimitMiddleware(cfg, db), handler.DialogueEvent)
		chat.GET("/dialogue/websocket", RateLimitMiddleware(cfg, db), handler.DialogueStream)
		chat.POST("/dialogue/keepalive/:id", handler.KeepAlive)
		chat.DELETE("/dialogue/:id", handler.DeleteDialogue)
	}

	data := r.Group("/data")
	{
		data.GET("/websocket/create", handler.PersistentWebSocket)
	}

	api := r.Group("/api")
	{
		orgGroup := api.Group("/organizations/:org_id")
		{
			mcpGroup := orgGroup.Group("/mcp")
			{
				mcpGroup.GET("/remote_servers", HandleMCPRemoteServersAdapter)
				mcpGroup.GET("/v2/bootstrap", HandleMCPBootstrap)
				mcpGroup.GET("/start-auth/:server_id", HandleMCPStartAuth)
				mcpGroup.POST("/logout/:server_id", HandleMCPLogout)
				mcpGroup.POST("/connect/:server_id", HandleMCPConnect)
				mcpGroup.GET("/status/:server_id", HandleMCPStatus)
			}
		}

		wsGroup := api.Group("/ws/organizations/:org_id/mcp/servers/:server_id")
		{
			wsGroup.GET("/", HandleMCPWebSocket)
		}
	}

	api.GET("/usage", handler.GetUsage)
	api.GET("/stats", handler.GetStats)
	api.GET("/device/status", handler.CheckDeviceStatus)

	return r
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

func RateLimitMiddleware(cfg *Config, db *Database) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db.IsShutdown() {
			stats := db.GetStats()
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":  "Service temporarily unavailable",
				"reason": stats.ShutdownReason,
			})
			c.Abort()
			return
		}

		tpm, rpm, rpd, err := db.CalculateRates()
		if err == nil {
			if cfg.MaxTPM > 0 && int(tpm) >= cfg.MaxTPM {
				reason := "Max TPM limit reached"
				db.SetShutdown(reason)
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": reason})
				c.Abort()
				return
			}

			if cfg.MaxRPM > 0 && int(rpm) >= cfg.MaxRPM {
				reason := "Max RPM limit reached"
				db.SetShutdown(reason)
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": reason})
				c.Abort()
				return
			}

			if cfg.MaxRPD > 0 && int(rpd) >= cfg.MaxRPD {
				reason := "Max RPD limit reached"
				db.SetShutdown(reason)
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": reason})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

type Handler struct {
	config          *Config
	db              *Database
	semaphore       chan struct{}
	dialogueManager *DialogueManager
}

func NewHandler(cfg *Config, db *Database) *Handler {
	return &Handler{
		config:          cfg,
		db:              db,
		semaphore:       make(chan struct{}, cfg.ThreadNum),
		dialogueManager: NewDialogueManager(),
	}
}

func (h *Handler) ServeIndex(c *gin.Context) {
	indexPath := filepath.Join("static", "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		content, err := os.ReadFile("index.html")
		if err != nil {
			c.String(http.StatusNotFound, "index.html not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", content)
		return
	}

	content, err := os.ReadFile(indexPath)
	if err != nil {
		c.String(http.StatusNotFound, "index.html not found")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
}

func (h *Handler) HealthCheck(c *gin.Context) {
	stats := h.db.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"status":     "healthy",
		"processing": stats.Processing,
		"completed":  stats.Completed,
		"failed":     stats.Failed,
	})
}

func (h *Handler) ListModels(c *gin.Context) {
	log.Printf("=== /v1/models 请求 ===")
	log.Printf("User-Agent: %s", c.GetHeader("User-Agent"))
	log.Printf("Accept: %s", c.GetHeader("Accept"))
	log.Printf("请求方法: %s", c.Request.Method)
	log.Printf("请求路径: %s", c.Request.URL.Path)
	log.Printf("查询参数: %s", c.Request.URL.RawQuery)

	currentTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	models := make([]OllamaModel, 0, len(h.config.Models))
	for _, model := range h.config.Models {
		models = append(models, OllamaModel{
			Name:       model.ID,
			ModifiedAt: currentTime,
			Size:       0,
			Digest:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		})
	}
	response := OllamaTagsResponse{
		Models: models,
	}

	jsonData, _ := json.Marshal(response)
	log.Printf("响应 JSON: %s", string(jsonData))
	log.Printf("响应模型数量: %d", len(models))
	log.Printf("响应类型: Ollama 格式 (OllamaTagsResponse)")
	log.Printf("===================")

	c.JSON(http.StatusOK, response)
}

func (h *Handler) OllamaListModels(c *gin.Context) {
	log.Printf("=== /api/tags 请求 ===")
	log.Printf("User-Agent: %s", c.GetHeader("User-Agent"))
	log.Printf("Accept: %s", c.GetHeader("Accept"))
	log.Printf("请求方法: %s", c.Request.Method)
	log.Printf("请求路径: %s", c.Request.URL.Path)

	currentTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	models := make([]OllamaModel, 0, len(h.config.Models))
	for _, model := range h.config.Models {
		models = append(models, OllamaModel{
			Name:       model.ID,
			ModifiedAt: currentTime,
			Size:       0,
			Digest:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		})
	}
	response := OllamaTagsResponse{
		Models: models,
	}

	jsonData, _ := json.Marshal(response)
	log.Printf("响应 JSON: %s", string(jsonData))
	log.Printf("响应模型数量: %d", len(models))
	log.Printf("===================")

	c.JSON(http.StatusOK, response)
}

func (h *Handler) OllamaListModelsDebug(c *gin.Context) {
	currentTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	models := make([]OllamaModel, 0, len(h.config.Models))
	for _, model := range h.config.Models {
		models = append(models, OllamaModel{
			Name:       model.ID,
			ModifiedAt: currentTime,
			Size:       0,
			Digest:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		})
	}
	response := OllamaTagsResponse{
		Models: models,
	}

	jsonData, _ := json.MarshalIndent(response, "", "  ")

	c.JSON(http.StatusOK, gin.H{
		"endpoint":         "/api/tags/debug",
		"response_object":  response,
		"response_json":    string(jsonData),
		"models_count":     len(models),
		"has_models_field": true,
		"user_agent":       c.GetHeader("User-Agent"),
	})
}

func (h *Handler) OllamaListModelsRaw(c *gin.Context) {
	rawJSON := `{
  "models": [
    {
      "name": "sonnet-4.5",
      "modified_at": "2025-11-01T12:00:00Z",
      "size": 0,
      "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    },
    {
      "name": "opus-4.1",
      "modified_at": "2025-11-01T12:00:00Z",
      "size": 0,
      "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    },
    {
      "name": "haiku-4.5",
      "modified_at": "2025-11-01T12:00:00Z",
      "size": 0,
      "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    }
  ]
}`

	c.Header("Content-Type", "application/json; charset=utf-8")
	c.String(http.StatusOK, rawJSON)
}

func (h *Handler) GetConfig(c *gin.Context) {
	endpoints := []gin.H{
		{"path": "/chat/dialogue/http", "description": "Classic HTTP dialogue with long timeout", "method": "POST"},
		{"path": "/chat/dialogue/event", "description": "SSE dialogue streaming", "method": "GET"},
		{"path": "/chat/dialogue/websocket", "description": "WebSocket dialogue streaming", "method": "GET"},
		{"path": "/health", "description": "Health check", "method": "GET"},
		{"path": "/api/stats", "description": "Get statistics", "method": "GET"},
		{"path": "/api/records", "description": "Get incremental records", "method": "POST"},
		{"path": "/api/record/:id", "description": "Get single record details", "method": "GET"},
		{"path": "/api/processing", "description": "Get processing requests", "method": "GET"},
		{"path": "/api/processing-stream", "description": "SSE stream for processing requests", "method": "GET"},
		{"path": "/api/processing/:id", "description": "Get processing request details", "method": "GET"},
		{"path": "/api/stream/:id", "description": "SSE stream for request progress", "method": "GET"},
		{"path": "/api/history-stream", "description": "SSE stream for history updates", "method": "GET"},
		{"path": "/api/config", "description": "Get service config", "method": "GET"},
		{"path": "/api/version-changes", "description": "Get version changes", "method": "GET"},
		{"path": "/data/websocket/create", "description": "Create persistent WebSocket connection", "method": "GET"},
	}

	changes, err := LoadVersionChanges("../src/changes.yaml")
	version := "1.0.0"
	if err == nil {
		version = GetLatestVersion(changes)
	}

	c.JSON(http.StatusOK, gin.H{
		"version":             version,
		"thread_num":          h.config.ThreadNum,
		"debug":               h.config.Debug,
		"incognito":           true,
		"api_endpoint":        h.config.APIEndpoint,
		"max_tpm":             h.config.MaxTPM,
		"max_rpm":             h.config.MaxRPM,
		"max_rpd":             h.config.MaxRPD,
		"request_interval_ms": h.config.RequestIntervalMS,
		"models":              h.config.Models,
		"endpoints":           endpoints,
	})
}

func handleSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	messageChan := make(chan SSEMessage, 10)
	broker.addClient(messageChan)
	defer broker.removeClient(messageChan)

	sendInitialData()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case msg := <-messageChan:
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", msg.Event, msg.Data)
			flusher.Flush()
		}
	}
}
