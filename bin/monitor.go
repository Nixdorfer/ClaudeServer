package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	lastPromptHash string
	promptMutex    sync.RWMutex
)

type ClaudeUsageResponse struct {
	FiveHour struct {
		Utilization float64    `json:"utilization"`
		ResetsAt    *time.Time `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization float64    `json:"utilization"`
		ResetsAt    *time.Time `json:"resets_at"`
	} `json:"seven_day"`
	SevenDayOAuthApps interface{} `json:"seven_day_oauth_apps"`
	SevenDayOpus      struct {
		Utilization float64    `json:"utilization"`
		ResetsAt    *time.Time `json:"resets_at"`
	} `json:"seven_day_opus"`
}

func MonitorUsage(cfg *Config, database *Database) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	fetchAndSaveUsage(cfg, database)

	for range ticker.C {
		fetchAndSaveUsage(cfg, database)
	}
}

func fetchAndSaveUsage(cfg *Config, database *Database) {
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/usage", cfg.GetOrganizationID())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("创建使用量请求失败: %v", err)
		return
	}

	req.Header.Set("Cookie", cfg.GetCookie())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-client-platform", "web_claude_ai")
	req.Header.Set("anthropic-client-version", "1.0.0")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://claude.ai/")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="141", "Not?A_Brand";v="8", "Chromium";v="141"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	client := globalConfig.CreateHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("获取使用量失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("获取使用量失败,状态码: %d, 响应: %s", resp.StatusCode, string(body))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取使用量响应失败: %v", err)
		return
	}

	var usageData ClaudeUsageResponse
	if err := json.Unmarshal(body, &usageData); err != nil {
		log.Printf("解析使用量数据失败: %v", err)
		return
	}
	log.Printf("✓ 使用量已更新 - 5小时: %d%%, 7天: %d%%, Opus: %d%%",
		int(usageData.FiveHour.Utilization),
		int(usageData.SevenDay.Utilization),
		int(usageData.SevenDayOpus.Utilization))

	broadcastUsage()
}

func MonitorPromptChanges() {
	promptMutex.Lock()
	lastPromptHash = getPromptHash()
	promptMutex.Unlock()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		currentHash := getPromptHash()

		promptMutex.RLock()
		lastHash := lastPromptHash
		promptMutex.RUnlock()

		if currentHash != lastHash {
			promptMutex.Lock()
			lastPromptHash = currentHash
			promptMutex.Unlock()

			log.Println("\n⚠️ 提示词已更新")
		}
	}
}

func getPromptHash() string {
	prompt := LoadSystemPrompt()
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])
}

func initPromptsFile() {
	if _, err := os.Stat("../src/prompts.txt"); os.IsNotExist(err) {
		file, err := os.Create("../src/prompts.txt")
		if err != nil {
			log.Printf("创建 prompts.txt 失败: %v", err)
			return
		}
		file.Close()
	}
}

func init() {
	initPromptsFile()
}
