package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func HandleMCPRemoteServers(c *gin.Context) {
	if mcpManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MCP manager not initialized",
		})
		return
	}
	servers := mcpManager.GetRemoteServers()
	c.JSON(http.StatusOK, MCPRemoteServersResponse{
		Servers: servers,
	})
}

func HandleMCPStartAuth(c *gin.Context) {
	serverID := c.Param("server_id")
	redirectURL := c.Query("redirect_url")
	openInBrowser := c.Query("open_in_browser") == "1"
	if mcpManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MCP manager not initialized",
		})
		return
	}
	if redirectURL == "" {
		redirectURL = "/settings/connectors"
	}
	resp, err := mcpManager.StartAuth(serverID, redirectURL, openInBrowser)
	if err != nil {
		errorURL := redirectURL + "?&server=" + serverID + "&step=start_error"
		c.Redirect(http.StatusTemporaryRedirect, errorURL)
		return
	}
	if openInBrowser {
		c.Redirect(http.StatusTemporaryRedirect, resp.AuthURL)
	} else {
		c.JSON(http.StatusOK, resp)
	}
}

func HandleMCPLogout(c *gin.Context) {
	serverID := c.Param("server_id")
	if mcpManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MCP manager not initialized",
		})
		return
	}
	err := mcpManager.Disconnect(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, MCPLogoutResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, MCPLogoutResponse{
		Success: true,
		Message: "Successfully disconnected",
	})
}

func HandleMCPConnect(c *gin.Context) {
	serverID := c.Param("server_id")
	if mcpManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MCP manager not initialized",
		})
		return
	}
	err := mcpManager.Connect(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Successfully connected",
	})
}

func HandleMCPStatus(c *gin.Context) {
	serverID := c.Param("server_id")
	if mcpManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MCP manager not initialized",
		})
		return
	}
	connector, err := mcpManager.GetConnector(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}
	connector.mu.RLock()
	defer connector.mu.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"id":           connector.ID,
		"name":         connector.Name,
		"url":          connector.URL,
		"auth_type":    connector.AuthType,
		"enabled":      connector.Enabled,
		"is_connected": connector.IsConnected,
		"connected_at": connector.ConnectedAt,
		"last_error":   connector.LastError,
	})
}
