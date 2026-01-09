package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func HandleMCPBootstrap(c *gin.Context) {
	orgID := c.Param("org_id")

	if globalConfig.Debug {
		DebugLog("📡 MCP Bootstrap request - OrgID: %s", orgID)
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	log.Printf("🔄 Starting MCP bootstrap stream...")

	tools, err := GetRemoteMCPToolsViaBootstrap(globalConfig)
	if err != nil {
		log.Printf("❌ Failed to get remote MCP tools: %v", err)
		fmt.Fprintf(c.Writer, "event: error\n")
		fmt.Fprintf(c.Writer, "data: {\"error\": \"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	serverTools := make(map[string][]MCPToolDefinition)
	serverInfo := make(map[string]struct {
		Name string
		URL  string
	})

	for _, tool := range tools {
		serverUUID := tool.MCPServerUUID
		serverTools[serverUUID] = append(serverTools[serverUUID], tool)

		if _, exists := serverInfo[serverUUID]; !exists {
			if mcpManager != nil {
				connector, err := mcpManager.GetConnector(serverUUID)
				if err == nil {
					serverInfo[serverUUID] = struct {
						Name string
						URL  string
					}{
						Name: connector.Name,
						URL:  connector.URL,
					}
				} else {
					serverInfo[serverUUID] = struct {
						Name string
						URL  string
					}{
						Name: tool.IntegrationName,
						URL:  "",
					}
				}
			}
		}
	}

	for uuid, info := range serverInfo {
		serverBase := map[string]interface{}{
			"uuid": uuid,
			"name": info.Name,
			"url":  info.URL,
		}

		serverBaseJSON, _ := json.Marshal(serverBase)

		fmt.Fprintf(c.Writer, "event: server_base\n")
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(serverBaseJSON))
		flusher.Flush()

		if globalConfig.Debug {
			DebugLog("📤 Sent server_base: %s (%s)", info.Name, uuid)
		}

		time.Sleep(10 * time.Millisecond)
	}

	for uuid, toolsList := range serverTools {
		toolsData := make([]map[string]interface{}, 0, len(toolsList))
		for _, tool := range toolsList {
			toolData := map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			}
			toolsData = append(toolsData, toolData)
		}

		toolsEvent := map[string]interface{}{
			"server_uuid": uuid,
			"tools":       toolsData,
		}

		toolsJSON, _ := json.Marshal(toolsEvent)

		fmt.Fprintf(c.Writer, "event: tools\n")
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(toolsJSON))
		flusher.Flush()

		log.Printf("📤 Sent %d tools for server: %s", len(toolsData), serverInfo[uuid].Name)

		time.Sleep(10 * time.Millisecond)
	}

	fmt.Fprintf(c.Writer, "event: completed\n")
	fmt.Fprintf(c.Writer, "data: {}\n\n")
	flusher.Flush()

	log.Printf("✅ MCP bootstrap stream completed: %d servers, %d tools", len(serverInfo), len(tools))
}

func HandleMCPRemoteServersAdapter(c *gin.Context) {
	if globalConfig.Debug {
		DebugLog("📡 MCP Remote Servers request")
	}

	servers, err := GetRemoteMCPServers(globalConfig)
	if err != nil {
		log.Printf("❌ Failed to get remote MCP servers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]map[string]interface{}, 0, len(servers))
	for _, server := range servers {
		serverData := map[string]interface{}{
			"uuid":                            server.UUID,
			"name":                            server.Name,
			"url":                             server.URL,
			"created_at":                      server.CreatedAt,
			"updated_at":                      server.UpdatedAt,
			"custom_oauth_client_id":          server.CustomOAuthClientID,
			"has_custom_oauth_credentials":    server.HasCustomOAuthCredentials,
			"custom_oauth_client_secret_mask": nil,
		}
		response = append(response, serverData)
	}

	log.Printf("✅ Returned %d remote MCP servers", len(response))

	c.JSON(http.StatusOK, response)
}
