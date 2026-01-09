package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

func DiagnoseMCP(config *Config) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🔍 MCP 诊断工具")
	fmt.Println(strings.Repeat("=", 80) + "\n")

	fmt.Println("📡 步骤 1: 测试远程MCP服务器列表...")
	servers, err := GetRemoteMCPServers(config)
	if err != nil {
		log.Printf("❌ 获取远程MCP服务器失败: %v", err)
	} else {
		log.Printf("✅ 成功获取 %d 个远程MCP服务器", len(servers))
		for i, server := range servers {
			fmt.Printf("   服务器 %d:\n", i+1)
			fmt.Printf("     - 名称: %s\n", server.Name)
			fmt.Printf("     - UUID: %s\n", server.UUID)
			fmt.Printf("     - URL: %s\n", server.URL)
			fmt.Printf("     - 已认证: %v\n", server.IsAuthenticated)
		}
	}

	fmt.Println()

	fmt.Println("🔄 步骤 2: 测试Bootstrap SSE流获取工具...")
	tools, err := GetRemoteMCPToolsViaBootstrap(config)
	if err != nil {
		log.Printf("❌ 获取远程MCP工具失败: %v", err)
	} else {
		log.Printf("✅ 成功获取 %d 个远程MCP工具", len(tools))
		for i, tool := range tools {
			fmt.Printf("   工具 %d:\n", i+1)
			fmt.Printf("     - 名称: %s\n", tool.Name)
			fmt.Printf("     - 集成: %s\n", tool.IntegrationName)
			fmt.Printf("     - 服务器UUID: %s\n", tool.MCPServerUUID)
			fmt.Printf("     - 需要审批: %v\n", tool.NeedsApproval)
			fmt.Printf("     - 描述: %s\n", truncateString(tool.Description, 60))
		}
	}

	fmt.Println()

	if mcpManager != nil {
		fmt.Println("🔧 步骤 3: 测试MCP Manager...")
		allTools := mcpManager.GetAllMCPTools()
		log.Printf("✅ MCP Manager返回 %d 个工具", len(allTools))

		remoteCount := 0
		localCount := 0
		builtinCount := 0

		for _, tool := range allTools {
			toolType := tool["type"]
			if toolType != nil {
				builtinCount++
			} else if tool["mcp_server_uuid"] != nil {
				remoteCount++
			} else {
				localCount++
			}
		}

		fmt.Printf("   - 远程MCP工具: %d\n", remoteCount)
		fmt.Printf("   - 本地MCP工具: %d\n", localCount)
		fmt.Printf("   - 内置工具: %d\n", builtinCount)
	} else {
		fmt.Println("⚠️  步骤 3: MCP Manager未初始化")
	}

	fmt.Println()

	fmt.Println("📝 步骤 4: 测试构建completion请求...")
	if mcpManager != nil {
		tools := mcpManager.GetAllMCPTools()
		reqBody := map[string]interface{}{
			"prompt":              "测试消息",
			"parent_message_uuid": "00000000-0000-4000-8000-000000000000",
			"timezone":            "Asia/Shanghai",
			"tools":               tools,
			"attachments":         []interface{}{},
			"files":               []interface{}{},
			"sync_sources":        []interface{}{},
			"rendering_mode":      "messages",
		}

		jsonData, err := json.MarshalIndent(reqBody, "", "  ")
		if err != nil {
			log.Printf("❌ 序列化请求失败: %v", err)
		} else {
			fmt.Println("✅ 成功构建completion请求")
			fmt.Printf("   请求体大小: %d 字节\n", len(jsonData))
			fmt.Printf("   工具数量: %d\n", len(tools))

			if len(tools) > 0 {
				fmt.Println("\n   前3个工具:")
				for i := 0; i < min(3, len(tools)); i++ {
					tool := tools[i]
					fmt.Printf("   %d. %s", i+1, tool["name"])
					if tool["integration_name"] != nil {
						fmt.Printf(" (%s)", tool["integration_name"])
					}
					fmt.Println()
				}
			}
		}
	} else {
		fmt.Println("⚠️  MCP Manager未初始化，无法测试")
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("✅ 诊断完成！")
	fmt.Println(strings.Repeat("=", 80) + "\n")
}

func TestRemoteMCPConnection(config *Config, serverUUID string) error {
	fmt.Printf("\n🔗 测试连接到远程MCP服务器: %s\n", serverUUID)

	servers, err := GetRemoteMCPServers(config)
	if err != nil {
		return fmt.Errorf("获取服务器列表失败: %v", err)
	}

	var targetServer *RemoteMCPServer
	for i, server := range servers {
		if server.UUID == serverUUID {
			targetServer = &servers[i]
			break
		}
	}

	if targetServer == nil {
		return fmt.Errorf("未找到服务器 UUID: %s", serverUUID)
	}

	fmt.Printf("✅ 找到服务器: %s\n", targetServer.Name)
	fmt.Printf("   URL: %s\n", targetServer.URL)
	fmt.Printf("   已认证: %v\n", targetServer.IsAuthenticated)

	fmt.Println("\n📡 测试Bootstrap SSE流...")
	startTime := time.Now()
	tools, err := GetRemoteMCPToolsViaBootstrap(config)
	duration := time.Since(startTime)

	if err != nil {
		return fmt.Errorf("Bootstrap流失败: %v", err)
	}

	serverTools := []MCPToolDefinition{}
	for _, tool := range tools {
		if tool.MCPServerUUID == serverUUID {
			serverTools = append(serverTools, tool)
		}
	}

	fmt.Printf("✅ Bootstrap成功 (耗时: %v)\n", duration)
	fmt.Printf("   该服务器的工具数: %d\n", len(serverTools))

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
