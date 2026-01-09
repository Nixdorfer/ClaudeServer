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
	fmt.Println("MCP Diagnostic Tool")
	fmt.Println(strings.Repeat("=", 80) + "\n")
	fmt.Println("Step 1: Testing remote MCP server list...")
	servers, err := GetRemoteMCPServers(config)
	if err != nil {
		log.Printf("Failed to get remote MCP servers: %v", err)
	} else {
		log.Printf("Successfully got %d remote MCP servers", len(servers))
		for i, server := range servers {
			fmt.Printf("   Server %d:\n", i+1)
			fmt.Printf("     - Name: %s\n", server.Name)
			fmt.Printf("     - UUID: %s\n", server.UUID)
			fmt.Printf("     - URL: %s\n", server.URL)
			fmt.Printf("     - Authenticated: %v\n", server.IsAuthenticated)
		}
	}
	fmt.Println()
	fmt.Println("Step 2: Testing Bootstrap SSE stream to get tools...")
	tools, err := GetRemoteMCPToolsViaBootstrap(config)
	if err != nil {
		log.Printf("Failed to get remote MCP tools: %v", err)
	} else {
		log.Printf("Successfully got %d remote MCP tools", len(tools))
		for i, tool := range tools {
			fmt.Printf("   Tool %d:\n", i+1)
			fmt.Printf("     - Name: %s\n", tool.Name)
			fmt.Printf("     - Integration: %s\n", tool.IntegrationName)
			fmt.Printf("     - Server UUID: %s\n", tool.MCPServerUUID)
			fmt.Printf("     - Needs approval: %v\n", tool.NeedsApproval)
			fmt.Printf("     - Description: %s\n", truncateString(tool.Description, 60))
		}
	}
	fmt.Println()
	if mcpManager != nil {
		fmt.Println("Step 3: Testing MCP Manager...")
		allTools := mcpManager.GetAllMCPTools()
		log.Printf("MCP Manager returned %d tools", len(allTools))
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
		fmt.Printf("   - Remote MCP tools: %d\n", remoteCount)
		fmt.Printf("   - Local MCP tools: %d\n", localCount)
		fmt.Printf("   - Built-in tools: %d\n", builtinCount)
	} else {
		fmt.Println("Step 3: MCP Manager not initialized")
	}
	fmt.Println()
	fmt.Println("Step 4: Testing build completion request...")
	if mcpManager != nil {
		tools := mcpManager.GetAllMCPTools()
		reqBody := map[string]any{
			"prompt":              "Test message",
			"parent_message_uuid": "00000000-0000-4000-8000-000000000000",
			"timezone":            "Asia/Shanghai",
			"tools":               tools,
			"attachments":         []any{},
			"files":               []any{},
			"sync_sources":        []any{},
			"rendering_mode":      "messages",
		}
		jsonData, err := json.MarshalIndent(reqBody, "", "  ")
		if err != nil {
			log.Printf("Failed to serialize request: %v", err)
		} else {
			fmt.Println("Successfully built completion request")
			fmt.Printf("   Request body size: %d bytes\n", len(jsonData))
			fmt.Printf("   Tool count: %d\n", len(tools))
			if len(tools) > 0 {
				fmt.Println("\n   First 3 tools:")
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
		fmt.Println("MCP Manager not initialized, cannot test")
	}
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Diagnostic completed!")
	fmt.Println(strings.Repeat("=", 80) + "\n")
}

func TestRemoteMCPConnection(config *Config, serverUUID string) error {
	fmt.Printf("\nTesting connection to remote MCP server: %s\n", serverUUID)
	servers, err := GetRemoteMCPServers(config)
	if err != nil {
		return fmt.Errorf("failed to get server list: %v", err)
	}
	var targetServer *RemoteMCPServer
	for i, server := range servers {
		if server.UUID == serverUUID {
			targetServer = &servers[i]
			break
		}
	}
	if targetServer == nil {
		return fmt.Errorf("server UUID not found: %s", serverUUID)
	}
	fmt.Printf("Found server: %s\n", targetServer.Name)
	fmt.Printf("   URL: %s\n", targetServer.URL)
	fmt.Printf("   Authenticated: %v\n", targetServer.IsAuthenticated)
	fmt.Println("\nTesting Bootstrap SSE stream...")
	startTime := time.Now()
	tools, err := GetRemoteMCPToolsViaBootstrap(config)
	duration := time.Since(startTime)
	if err != nil {
		return fmt.Errorf("bootstrap stream failed: %v", err)
	}
	serverTools := []MCPToolDefinition{}
	for _, tool := range tools {
		if tool.MCPServerUUID == serverUUID {
			serverTools = append(serverTools, tool)
		}
	}
	fmt.Printf("Bootstrap successful (duration: %v)\n", duration)
	fmt.Printf("   Tools for this server: %d\n", len(serverTools))
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
