package main

import (
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

//go:embed wails.json
var wailsJSON []byte

//go:embed notice.md
var noticeContent string

// WailsConfig represents the wails.json configuration
type WailsConfig struct {
	Version string `json:"version"`
}

// Version returns the version from wails.json
var Version string

// Notice content from notice.md
var Notice string

func init() {
	// Parse version from embedded wails.json
	var config WailsConfig
	if err := json.Unmarshal(wailsJSON, &config); err == nil {
		Version = config.Version
	} else {
		Version = "dev"
	}

	// Set notice from embedded notice.md
	Notice = noticeContent
}

// VersionInfo represents version information from remote
type VersionInfo struct {
	Version string   `json:"version"`
	Note    []string `json:"note"`
	URL     string   `json:"url"`
}

// UpdateCheckResult contains the result of an update check
type UpdateCheckResult struct {
	HasUpdate      bool     `json:"has_update"`
	CurrentVersion string   `json:"current_version"`
	LatestVersion  string   `json:"latest_version"`
	Notes          []string `json:"notes"`
	DownloadURL    string   `json:"download_url"`
}

// GetCurrentVersion returns the compiled version
func (a *App) GetCurrentVersion() string {
	return Version
}

// GetNotice returns the compiled notice content
func (a *App) GetNotice() string {
	return Notice
}

// CheckForUpdate checks if a newer version is available
func (a *App) CheckForUpdate() (*UpdateCheckResult, error) {
	url := "https://raw.githubusercontent.com/Nixdorfer/ClaudeServer/refs/heads/main/info.json"

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version info: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Parse as array (the info.json is an array)
	var versionInfos []VersionInfo
	if err := json.Unmarshal(body, &versionInfos); err != nil {
		return nil, fmt.Errorf("failed to parse version info: %v", err)
	}

	if len(versionInfos) == 0 {
		return nil, fmt.Errorf("empty version info")
	}

	// Get the latest version (first element)
	latestInfo := versionInfos[0]

	result := &UpdateCheckResult{
		CurrentVersion: Version,
		LatestVersion:  latestInfo.Version,
		Notes:          latestInfo.Note,
		DownloadURL:    latestInfo.URL,
		HasUpdate:      compareVersions(latestInfo.Version, Version) > 0,
	}

	return result, nil
}

// compareVersions compares two version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Handle "dev" version - always consider it older
	if v2 == "dev" {
		return 1
	}
	if v1 == "dev" {
		return -1
	}

	// Parse version components
	var major1, minor1, patch1 int
	var major2, minor2, patch2 int

	fmt.Sscanf(v1, "%d.%d.%d", &major1, &minor1, &patch1)
	fmt.Sscanf(v2, "%d.%d.%d", &major2, &minor2, &patch2)

	if major1 != major2 {
		if major1 > major2 {
			return 1
		}
		return -1
	}

	if minor1 != minor2 {
		if minor1 > minor2 {
			return 1
		}
		return -1
	}

	if patch1 != patch2 {
		if patch1 > patch2 {
			return 1
		}
		return -1
	}

	return 0
}
