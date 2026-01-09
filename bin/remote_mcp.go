package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// RemoteMCPServer represents a remote MCP server
type RemoteMCPServer struct {
	UUID                      string `json:"uuid"`
	Name                      string `json:"name"`
	URL                       string `json:"url"`
	CreatedAt                 string `json:"created_at"`
	UpdatedAt                 string `json:"updated_at"`
	CustomOAuthClientID       string `json:"custom_oauth_client_id"`
	HasCustomOAuthCredentials bool   `json:"has_custom_oauth_credentials"`
	IsAuthenticated           bool   `json:"is_authenticated"`
}

// RemoteMCPToolsCache 缓存远程MCP工具列表
type RemoteMCPToolsCache struct {
	tools      []MCPToolDefinition
	lastUpdate time.Time
}

var remoteMCPToolsCache = &RemoteMCPToolsCache{
	tools: make([]MCPToolDefinition, 0),
}

// GetRemoteMCPServers 获取远程MCP服务器列表
func GetRemoteMCPServers(config *Config) ([]RemoteMCPServer, error) {
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers",
		config.GetOrganizationID())

	client := config.CreateHTTPClient(30 * time.Second)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Cookie", config.GetCookie())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Referer", "https://claude.ai/")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Failed to get remote MCP servers: %v", err)
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if config.Debug {
		DebugLog("📥 Remote servers response status: %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Get remote MCP servers failed with status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var servers []RemoteMCPServer
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return servers, nil
}

// GetRemoteMCPToolsViaBootstrap 通过bootstrap SSE流获取远程MCP工具
func GetRemoteMCPToolsViaBootstrap(config *Config) ([]MCPToolDefinition, error) {
	// 检查缓存（5分钟有效期）
	if time.Since(remoteMCPToolsCache.lastUpdate) < 5*time.Minute && len(remoteMCPToolsCache.tools) > 0 {
		if config.Debug {
			DebugLog("✅ Using cached remote MCP tools: %d tools", len(remoteMCPToolsCache.tools))
		}
		return remoteMCPToolsCache.tools, nil
	}

	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/v2/bootstrap",
		config.GetOrganizationID())

	if config.Debug {
		DebugLog("🔄 Fetching remote MCP tools from: %s", url)
	}

	client := config.CreateHTTPClient(60 * time.Second)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Cookie", config.GetCookie())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Referer", "https://claude.ai/")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Bootstrap request failed: %v", err)
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if config.Debug {
		DebugLog("📥 Bootstrap response status: %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Bootstrap failed with status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// 解析SSE流
	allTools := make([]MCPToolDefinition, 0)
	serverNames := make(map[string]string) // server_uuid -> server_name

	reader := bufio.NewReader(resp.Body)
	timeout := time.After(30 * time.Second)
	completed := false

	for !completed {
		select {
		case <-timeout:
			if config.Debug {
				DebugLog("Bootstrap SSE stream timeout, using partial results")
			}
			completed = true
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					completed = true
					break
				}
				return nil, fmt.Errorf("failed to read SSE stream: %v", err)
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// 解析SSE事件
			if strings.HasPrefix(line, "event:") {
				eventType := strings.TrimSpace(strings.TrimPrefix(line, "event:"))

				// 读取下一行（data行）
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					continue
				}

				if !strings.HasPrefix(dataLine, "data:") {
					continue
				}

				dataContent := strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))

				switch eventType {
				case "server_base":
					// 解析服务器基础信息
					var serverBase struct {
						UUID string `json:"uuid"`
						Name string `json:"name"`
						URL  string `json:"url"`
					}
					if err := json.Unmarshal([]byte(dataContent), &serverBase); err == nil {
						serverNames[serverBase.UUID] = serverBase.Name
						log.Printf("📡 Found remote MCP server: %s (UUID: %s)", serverBase.Name, serverBase.UUID)
						if config.Debug {
							DebugLog("   Server URL: %s", serverBase.URL)
						}
					}

				case "tools":
					// 解析工具列表
					var toolsData struct {
						ServerUUID string                   `json:"server_uuid"`
						Tools      []map[string]interface{} `json:"tools"`
					}
					if err := json.Unmarshal([]byte(dataContent), &toolsData); err == nil {
						serverName := serverNames[toolsData.ServerUUID]
						if serverName == "" {
							serverName = "Remote MCP Server"
						}

						for _, toolMap := range toolsData.Tools {
							tool := MCPToolDefinition{
								IntegrationName: serverName,
								MCPServerUUID:   toolsData.ServerUUID,
								NeedsApproval:   true,
							}

							if name, ok := toolMap["name"].(string); ok {
								tool.Name = name
							}
							if desc, ok := toolMap["description"].(string); ok {
								tool.Description = desc
							}
							if schema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
								tool.InputSchema = schema
							}

							allTools = append(allTools, tool)
						}

						log.Printf("✅ Got %d tools from remote MCP server: %s", len(toolsData.Tools), serverName)
						if config.Debug {
							for _, tool := range toolsData.Tools {
								DebugLog("   - %s: %s", tool["name"], truncateString(fmt.Sprintf("%v", tool["description"]), 50))
							}
						}
					}

				case "completed":
					if config.Debug {
						DebugLog("Bootstrap SSE stream completed")
					}
					completed = true
				}
			}
		}
	}

	// 更新缓存
	remoteMCPToolsCache.tools = allTools
	remoteMCPToolsCache.lastUpdate = time.Now()

	log.Printf("✅ Total remote MCP tools: %d", len(allTools))
	if config.Debug {
		// 按服务器分组统计
		serverToolCount := make(map[string]int)
		for _, tool := range allTools {
			serverToolCount[tool.MCPServerUUID]++
		}
		for uuid, count := range serverToolCount {
			DebugLog("   Server %s: %d tools", uuid[:8], count)
		}
	}

	return allTools, nil
}
