package services

import (
	"claudechat/models"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const updateUrl = "https://raw.githubusercontent.com/Nixdorfer/ClaudeServer/main/info.json"

type UpdateService struct {
	version string
	exeDir  string
}

func NewUpdateService(version, exeDir string) *UpdateService {
	return &UpdateService{version: version, exeDir: exeDir}
}

func (u *UpdateService) CheckForUpdate() (*models.UpdateCheckResult, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(updateUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var versions []models.VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return &models.UpdateCheckResult{
			HasUpdate:      false,
			CurrentVersion: u.version,
		}, nil
	}
	latest := versions[len(versions)-1]
	hasUpdate := compareVersions(u.version, latest.Version)
	return &models.UpdateCheckResult{
		HasUpdate:      hasUpdate,
		CurrentVersion: u.version,
		LatestVersion:  latest.Version,
		Notes:          latest.Note,
		DownloadUrl:    latest.Url,
	}, nil
}

func (u *UpdateService) GetCurrentVersion() string {
	return u.version
}

func (u *UpdateService) GetNotice() (string, error) {
	noticePath := filepath.Join(u.exeDir, "notice.md")
	content, err := os.ReadFile(noticePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

func compareVersions(current, latest string) bool {
	parse := func(v string) []int {
		parts := strings.Split(v, ".")
		result := make([]int, 3)
		for i := 0; i < 3 && i < len(parts); i++ {
			result[i], _ = strconv.Atoi(parts[i])
		}
		return result
	}
	curr := parse(current)
	lat := parse(latest)
	for i := 0; i < 3; i++ {
		if lat[i] > curr[i] {
			return true
		}
		if curr[i] > lat[i] {
			return false
		}
	}
	return false
}
