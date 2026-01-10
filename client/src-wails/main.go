package main

import (
	"claudechat/services"
	"embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

type versionInfo struct {
	Version string `json:"version"`
}

func getVersion() string {
	data, err := os.ReadFile("D:\\ClaudeServer\\info.json")
	if err != nil {
		return "0.0.0"
	}
	var versions []versionInfo
	if err := json.Unmarshal(data, &versions); err != nil || len(versions) == 0 {
		return "0.0.0"
	}
	return versions[0].Version
}

func main() {
	version := getVersion()
	exeDir := getExeDir()
	devMode := os.Getenv("DEV_MODE") == "true"
	dbService := services.NewDatabaseService(exeDir)
	chatService := services.NewChatService(dbService, version)
	updateService := services.NewUpdateService(version, exeDir)
	usageService := services.NewUsageService()
	app := application.New(application.Options{
		Name:        "ClaudeChat",
		Description: "Claude Chat Client by Nix",
		Services: []application.Service{
			application.NewService(chatService),
			application.NewService(updateService),
			application.NewService(usageService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})
	chatService.SetApp(app)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "Claude Client by Nix",
		Width:           1300,
		Height:          1000,
		MinWidth:        800,
		MinHeight:       600,
		URL:             "/",
		DevToolsEnabled: devMode,
	})
	if devMode {
		window.OnWindowEvent(events.Common.WindowRuntimeReady, func(event *application.WindowEvent) {
			window.OpenDevTools()
		})
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func getExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
