package main

import (
	"fmt"
	"strings"
)

const (
	ColorReset = "\033[0m"
	ColorGreen = "\033[32m"
	ColorBlue  = "\033[34m"
	ColorRed   = "\033[31m"
)

func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen*2 {
		return text
	}
	return string(runes[:maxLen]) + "..." + string(runes[len(runes)-maxLen:])
}

func LogExchange(userText, claudeText string, isError bool) {
	userTruncated := truncateText(strings.TrimSpace(userText), 20)
	claudeTruncated := truncateText(strings.TrimSpace(claudeText), 20)
	userColor := ColorGreen
	claudeColor := ColorBlue
	if isError {
		claudeColor = ColorRed
	}
	fmt.Printf("%sUser: %s%s\n", userColor, userTruncated, ColorReset)
	fmt.Printf("%sClaude: %s%s\n", claudeColor, claudeTruncated, ColorReset)
}
