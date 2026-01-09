package main

import (
	"strings"
	"time"
)

type ClaudeAPIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func sendChatCompletion(orgID, cookie string, messages []OpenAIMessage, model string, stream bool) (*OpenAIResponse, error) {
	conversationID := ""
	parentMessageUUID := "00000000-0000-4000-8000-000000000000"

	var fullResponse strings.Builder

	for _, msg := range messages {
		if msg.Role == "user" {
			response, err := sendDialogueMessageWithOptions(orgID, conversationID, cookie, msg.Content, parentMessageUUID, model, "", func(chunk string) {
				fullResponse.WriteString(chunk)
			})

			if err != nil {
				return nil, err
			}

			fullResponse.Reset()
			fullResponse.WriteString(response)
		}
	}

	return &OpenAIResponse{
		ID:      conversationID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIResponseChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: fullResponse.String(),
				},
				FinishReason: "stop",
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}, nil
}
