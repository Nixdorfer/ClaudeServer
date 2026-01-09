package main

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
)

type SSEBroker struct {
	clients map[chan SSEMessage]bool
	mu      sync.RWMutex
}

type SSEMessage struct {
	Event string
	Data  string
}

func (b *SSEBroker) addClient(client chan SSEMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[client] = true
}

func (b *SSEBroker) removeClient(client chan SSEMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, client)
	close(client)
}

func (b *SSEBroker) broadcast(msg SSEMessage) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for client := range b.clients {
		select {
		case client <- msg:
		default:
		}
	}
}

var (
	globalConfig *Config
	db           *Database
	broker       = &SSEBroker{
		clients: make(map[chan SSEMessage]bool),
	}
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--diagnose-mcp", "-d":
			config, err := LoadConfig("../src/config.yaml")
			if err != nil {
				log.Fatal("配置加载失败:", err)
			}
			globalConfig = config
			DiagnoseMCP(config)
			return
		case "--test-mcp-client", "-t":
			config, err := LoadConfig("../src/config.yaml")
			if err != nil {
				log.Fatal("配置加载失败:", err)
			}
			globalConfig = config
			log.Println("====================================================================")
			log.Println("Testing MCP Client (Simulating Claude Frontend)")
			log.Println("====================================================================")
			if len(config.MCPConnectors) == 0 {
				log.Println("⚠️  No MCP connectors configured in config.yaml")
				log.Println("Please add MCP connectors to mcp_connectors section")
				log.Println("\nExample:")
				log.Println("mcp_connectors:")
				log.Println("  - name: \"云壳\"")
				log.Println("    uuid: \"fc8fdf60-9a35-43ff-97e0-a5ca4b0047ea\"")
				log.Println("    enabled: true")
				return
			}
			successCount := 0
			failCount := 0
			for _, connector := range config.MCPConnectors {
				if !connector.Enabled {
					log.Printf("\n⏭️  Skipping disabled connector: %s", connector.Name)
					continue
				}
				log.Printf("\n======================================================================")
				log.Printf("Testing MCP Connector: %s", connector.Name)
				log.Printf("======================================================================")
				err := TestMCPConnection(
					config,
					config.GetOrganizationID(),
					config.GetSessionKey(),
					config.GetCookie(),
					connector.UUID,
				)
				if err != nil {
					log.Printf("❌ MCP connector '%s' test failed: %v\n", connector.Name, err)
					failCount++
				} else {
					log.Printf("✅ MCP connector '%s' test succeeded!\n", connector.Name)
					successCount++
				}
			}
			log.Println("\n======================================================================")
			log.Printf("MCP Client Test Summary: %d succeeded, %d failed", successCount, failCount)
			log.Println("======================================================================")
			if failCount > 0 {
				os.Exit(1)
			}
			return
		case "--help", "-h":
			fmt.Println("Claude Adapter - MCP Integration Tool")
			fmt.Println("\nUsage:")
			fmt.Println("  claude-adapter                启动HTTP服务器")
			fmt.Println("  claude-adapter -d             运行MCP诊断")
			fmt.Println("  claude-adapter -t             测试MCP客户端（模拟Claude前端）")
			fmt.Println("  claude-adapter --help         显示帮助信息")
			return
		}
	}
	config, err := LoadConfig("../src/config.yaml")
	if err != nil {
		log.Fatal("配置加载失败:", err)
	}
	globalConfig = config
	db, err = InitDB(config)
	if err != nil {
		log.Fatal("数据库初始化失败:", err)
	}
	defer db.Close()
	InitRequestLimiter(config.RequestIntervalMS)
	if err := db.LoadStats(); err != nil {
		log.Printf("加载统计信息失败: %v", err)
	}
	InitMCPManager(config)
	InitMCPSessionManager(config)
	if !config.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	PrintStartupInfo(config)
	if mcpManager != nil {
		mcpManager.AutoConnectAll()
	}
	go MonitorPromptChanges()
	go MonitorUsage(config, db)
	r := SetupRouter(config, db)
	if err := r.Run(config.GetServerAddr()); err != nil {
		log.Fatal("服务器启动失败:", err)
	}
}

func PrintStartupInfo(cfg *Config) {
	log.Println("✓ 配置加载成功")
	log.Printf("组织ID: %s\n", cfg.GetOrganizationID())
	log.Printf("线程数: %d\n", cfg.ThreadNum)
	log.Printf("服务端口: %d\n", cfg.ServerPort)
	log.Printf("数据库: %s:%d/%s\n", cfg.DBHost, cfg.DBPort, cfg.DBName)
	log.Printf("最大TPM: %d\n", cfg.MaxTPM)
	log.Printf("最大RPM: %d\n", cfg.MaxRPM)
	log.Printf("最大RPD: %d\n", cfg.MaxRPD)
	log.Printf("请求间隔: %d毫秒\n", cfg.RequestIntervalMS)
	log.Printf("MCP 连接器: %d\n", len(cfg.MCPConnectors))
	if cfg.Debug {
		log.Println("调试模式: 已开启")
	}
	log.Println("隐身模式: 已开启")
	systemPrompt := LoadSystemPrompt()
	if systemPrompt != "" {
		log.Println("\n当前系统提示词:")
		log.Println("---")
		log.Println(systemPrompt)
		log.Println("---")
	} else {
		log.Println("\n系统提示词: 未设置")
	}
	log.Printf("\n✓ 服务器启动在 :%d\n", cfg.ServerPort)
	log.Printf("访问 http://localhost:%d 查看监控面板\n\n", cfg.ServerPort)
}

func LoadSystemPrompt() string {
	content, err := os.ReadFile("../src/prompts.txt")
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取 prompts.txt 失败: %v", err)
		}
		return ""
	}
	return string(content)
}

func DebugLog(format string, args ...any) {
	if globalConfig != nil && globalConfig.Debug {
		fmt.Printf("\033[34m[DEBUG] "+format+"\033[0m\n", args...)
	}
}
