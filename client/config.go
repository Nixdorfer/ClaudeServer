package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

type ClientConfig struct {
	ServerURL         string  `json:"server_url"`
	UsageLimitFiveHour float64 `json:"usage_limit_five_hour"`
	UsageLimitSevenDay float64 `json:"usage_limit_seven_day"`
}

func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		ServerURL:         "wss://claude.nixdorfer.com/data/websocket/create",
		UsageLimitFiveHour: 50.0,
		UsageLimitSevenDay: 50.0,
	}
}

func getAppDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}

func getConfigPath() string {
	return filepath.Join(getAppDir(), "config.json")
}

func LoadConfig() *ClientConfig {
	configPath := getConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("配置文件不存在，使用默认配置: %s", configPath)
		config := DefaultConfig()
		SaveConfig(config)
		return config
	}

	var config ClientConfig
	if err := json.Unmarshal(data, &config); err != nil {
		log.Printf("配置文件解析失败，使用默认配置: %v", err)
		return DefaultConfig()
	}

	log.Printf("配置加载成功: %s", configPath)
	return &config
}

func SaveConfig(config *ClientConfig) error {
	configPath := getConfigPath()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
