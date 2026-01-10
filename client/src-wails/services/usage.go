package services

import (
	"claudechat/models"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

func getUsageUrl() string {
	if os.Getenv("DEV_MODE") == "true" {
		return "http://localhost:5000/api/usage"
	}
	return "https://claude.nixdorfer.com/api/usage"
}

type UsageService struct{}

func NewUsageService() *UsageService {
	return &UsageService{}
}

func (u *UsageService) GetUsageStatus() (*models.UsageStatus, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, err := http.NewRequest("GET", getUsageUrl(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Origin", "https://claude.nixdorfer.com")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var usage models.UsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, err
	}
	return &models.UsageStatus{
		FiveHour:            usage.FiveHourUtilization,
		FiveHourReset:       usage.FiveHourResetsAt,
		SevenDay:            usage.SevenDayUtilization,
		SevenDayReset:       usage.SevenDayResetsAt,
		SevenDaySonnet:      usage.SevenDayOpusUtilization,
		SevenDaySonnetReset: usage.SevenDayOpusResetsAt,
		IsBlocked:           usage.IsBlocked,
		BlockReason:         usage.BlockReason,
		BlockResetTime:      usage.BlockResetTime,
	}, nil
}

func (u *UsageService) FormatResetTime(isoTime string) string {
	t, err := time.Parse(time.RFC3339, isoTime)
	if err != nil {
		return isoTime
	}
	return t.Local().Format("01-02 15:04")
}
