package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type FileAttachment struct {
	FileUUID      string `json:"file_uuid"`
	FileName      string `json:"file_name"`
	FileType      string `json:"file_type"`
	FileSize      int64  `json:"file_size"`
	ExtractedText string `json:"extracted_content"`
}

type UploadResponse struct {
	Success       bool   `json:"success"`
	Path          string `json:"path"`
	SanitizedName string `json:"sanitized_name"`
	SizeBytes     int64  `json:"size_bytes"`
	FileKind      string `json:"file_kind"`
	FileUUID      string `json:"file_uuid"`
	FileName      string `json:"file_name"`
	CreatedAt     string `json:"created_at"`
	UserUUID      string `json:"user_uuid"`
}

type StreamCallback func(text string)

func (rf *RequestFile) DecodeContent() error {
	if rf.Content == "" {
		return fmt.Errorf("file content is empty")
	}
	decoded, err := base64.StdEncoding.DecodeString(rf.Content)
	if err != nil {
		return fmt.Errorf("base64 decode failed: %v", err)
	}
	rf.ContentRaw = decoded
	DebugLog("Decoded file size: %d bytes", len(decoded))
	return nil
}

func createConversation(orgID, cookie string, incognito bool) (string, error) {
	maxRetries := 3
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(time.Duration(i) * time.Second)
			DebugLog("Retrying create conversation, attempt %d/%d", i+1, maxRetries)
		}
		WaitForNextRequest()
		url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations", orgID)
		reqBody := map[string]any{
			"uuid":                             generateUUID(),
			"name":                             "",
			"is_temporary":                     incognito,
			"include_conversation_preferences": true,
		}
		jsonData, _ := json.Marshal(reqBody)
		DebugLog("Create conversation request: %s", string(jsonData))
		req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		req.Header.Set("Origin", "https://claude.ai")
		req.Header.Set("Referer", "https://claude.ai/")
		client := globalConfig.CreateHTTPClient(30 * time.Second)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		DebugLog("Create conversation response status: %d", resp.StatusCode)
		DebugLog("Create conversation response body: %s", string(body))
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
			continue
		}
		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = fmt.Errorf("parse response failed: %v", err)
			DebugLog("Failed to parse JSON response: %v, body: %s", err, string(body))
			continue
		}
		var fields []string
		for key := range result {
			fields = append(fields, key)
		}
		DebugLog("Conversation response contains fields: %v", fields)
		if result["uuid"] == nil {
			lastErr = fmt.Errorf("no uuid field in response (available fields: %v)", fields)
			DebugLog("Response missing uuid field. Full response: %+v", result)
			continue
		}
		return result["uuid"].(string), nil
	}
	return "", fmt.Errorf("failed after %d retries: %v", maxRetries, lastErr)
}

func sendMessage(orgID, conversationID, cookie, prompt string, attachments []FileAttachment) (string, error) {
	return sendMessageWithCallback(orgID, conversationID, cookie, prompt, attachments, nil)
}

func sendMessageWithCallback(orgID, conversationID, cookie, prompt string, attachments []FileAttachment, callback StreamCallback) (string, error) {
	if mcpManager != nil {
		DebugLog("Ensuring MCP tools are enabled before sending message...")
		if err := mcpManager.EnsureAllMCPToolsEnabled(); err != nil {
			DebugLog("Failed to ensure MCP tools enabled: %v (continuing anyway)", err)
		} else {
			DebugLog("MCP tools ensured enabled")
		}
	}
	WaitForNextRequest()
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s/completion",
		orgID, conversationID)
	attachmentsPayload := make([]map[string]any, 0)
	for _, att := range attachments {
		attachmentsPayload = append(attachmentsPayload, map[string]any{
			"file_uuid":         att.FileUUID,
			"file_name":         att.FileName,
			"file_type":         att.FileType,
			"file_size":         att.FileSize,
			"extracted_content": att.ExtractedText,
		})
	}
	var tools []map[string]any
	if mcpManager != nil {
		tools = mcpManager.GetAllMCPTools()
		if len(tools) > 0 {
			log.Printf("✅ Adding %d tools to completion request", len(tools))
			remoteCount := 0
			localCount := 0
			builtinCount := 0
			for _, tool := range tools {
				if tool["type"] != nil {
					builtinCount++
				} else if tool["mcp_server_uuid"] != nil {
					remoteCount++
				} else {
					localCount++
				}
			}
			log.Printf("   Remote MCP: %d, Local MCP: %d, Built-in: %d", remoteCount, localCount, builtinCount)
			if globalConfig.Debug {
				mcpToolsShown := 0
				for _, tool := range tools {
					if tool["mcp_server_uuid"] != nil && mcpToolsShown < 3 {
						DebugLog("   MCP Tool: %s (%s)", tool["name"], tool["integration_name"])
						mcpToolsShown++
					}
				}
			}
		}
	}
	if len(tools) == 0 {
		log.Printf("⚠️  No MCP tools found, using only built-in tools")
		tools = []map[string]any{
			{
				"type": "web_search_v0",
				"name": "web_search",
			},
			{
				"type": "artifacts_v0",
				"name": "artifacts",
			},
		}
	}
	reqBody := map[string]any{
		"prompt":              prompt,
		"parent_message_uuid": "00000000-0000-4000-8000-000000000000",
		"timezone":            "Asia/Shanghai",
		"attachments":         attachmentsPayload,
		"files":               []any{},
		"sync_sources":        []any{},
		"rendering_mode":      "messages",
		"tools":               tools,
	}
	jsonData, _ := json.Marshal(reqBody)
	DebugLog("Request body: %s", string(jsonData))
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	client := globalConfig.CreateHTTPClient(300 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	var eventType string
	var fullResponse strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			DebugLog("Event type: %s", eventType)
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var eventData map[string]any
			if json.Unmarshal([]byte(data), &eventData) == nil {
				if eventType == "content_block_delta" {
					if delta, ok := eventData["delta"].(map[string]any); ok {
						if text, ok := delta["text"].(string); ok {
							fullResponse.WriteString(text)
							if callback != nil {
								callback(fullResponse.String())
							}
						}
					}
				}
			}
			if eventType == "message_stop" {
				return fullResponse.String(), nil
			}
			if eventType == "error" {
				return "", fmt.Errorf("received error event: %s", data)
			}
			eventType = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read response failed: %v", err)
	}
	return fullResponse.String(), nil
}

func uploadFile(orgID, conversationID, cookie string, file *RequestFile) (*UploadResponse, error) {
	WaitForNextRequest()
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/conversations/%s/wiggle/upload-file",
		orgID, conversationID)
	DebugLog("Preparing to upload file: %s, size: %d bytes", file.Name, len(file.ContentRaw))
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", file.Name)
	if err != nil {
		return nil, fmt.Errorf("create form file failed: %v", err)
	}
	if _, err := part.Write(file.ContentRaw); err != nil {
		return nil, fmt.Errorf("write file content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer failed: %v", err)
	}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Referer", "https://claude.ai/")
	client := globalConfig.CreateHTTPClient(60 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %v", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %v", err)
	}
	DebugLog("Upload response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed, status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var uploadResp UploadResponse
	if err := json.Unmarshal(bodyBytes, &uploadResp); err != nil {
		return nil, fmt.Errorf("parse upload response failed: %v", err)
	}
	DebugLog("File uploaded successfully: %+v", uploadResp)
	return &uploadResp, nil
}

type StylePrompt struct {
	Key     string
	Name    string
	Summary string
	Prompt  string
}

func getStylePrompt(styleKey string) *StylePrompt {
	styles := map[string]*StylePrompt{
		"concise": {
			Key:     "Concise",
			Name:    "Concise",
			Summary: "Shorter responses & more messages",
			Prompt:  "Claude is operating in Concise Mode. In this mode, Claude aims to reduce its output tokens while maintaining its helpfulness, quality, completeness, and accuracy.\nClaude provides answers to questions without much unneeded preamble or postamble. It focuses on addressing the specific query or task at hand, avoiding tangential information unless helpful for understanding or completing the request. If it decides to create a list, Claude focuses on key information instead of comprehensive enumeration.\nClaude maintains a helpful tone while avoiding excessive pleasantries or redundant offers of assistance.\nClaude provides relevant evidence and supporting details when substantiation is helpful for factuality and understanding of its response. For numerical data, Claude includes specific figures when important to the answer's accuracy.\nFor code, artifacts, written content, or other generated outputs, Claude maintains the exact same level of quality, completeness, and functionality as when NOT in Concise Mode. There should be no impact to these output types.\nClaude does not compromise on completeness, correctness, appropriateness, or helpfulness for the sake of brevity.\nIf the human requests a long or detailed response, Claude will set aside Concise Mode constraints and provide a more comprehensive answer.\nIf the human appears frustrated with Claude's conciseness, repeatedly requests longer or more detailed responses, or directly asks about changes in Claude's response style, Claude informs them that it's currently in Concise Mode and explains that Concise Mode can be turned off via Claude's UI if desired. Besides these scenarios, Claude does not mention Concise Mode.",
		},
		"explanatory": {
			Key:     "Explanatory",
			Name:    "Explanatory",
			Summary: "Educational responses for learning",
			Prompt:  "Claude aims to give clear, thorough explanations that help the human deeply understand complex topics.\nClaude approaches questions like a teacher would, breaking down ideas into easier parts and building up to harder concepts. It uses comparisons, examples, and step-by-step explanations to improve understanding.\nClaude keeps a patient and encouraging tone, trying to spot and address possible points of confusion before they arise. Claude may ask thinking questions or suggest mental exercises to get the human more involved in learning.\nClaude gives background info when it helps create a fuller picture of the topic. It might sometimes branch into related topics if they help build a complete understanding of the subject.\nWhen writing code or other technical content, Claude adds helpful comments to explain the thinking behind important steps.\nClaude always writes prose and in full sentences, especially for reports, documents, explanations, and question answering. Claude can use bullets only if the user asks specifically for a list.",
		},
	}
	if style, exists := styles[styleKey]; exists {
		return style
	}
	return nil
}

func generateUUID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "-")
}

func getConversationHistory(orgID, conversationID, cookie string) (string, error) {
	WaitForNextRequest()
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s", orgID, conversationID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID))
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	client := globalConfig.CreateHTTPClient(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response failed: %v", err)
	}
	messages, ok := result["chat_messages"].([]any)
	if !ok || len(messages) == 0 {
		return "00000000-0000-4000-8000-000000000000", nil
	}
	lastMessage := messages[len(messages)-1].(map[string]any)
	if lastUUID, ok := lastMessage["uuid"].(string); ok {
		return lastUUID, nil
	}
	return "00000000-0000-4000-8000-000000000000", nil
}

func sendDialogueMessage(orgID, conversationID, cookie, prompt, parentMessageUUID string) (string, error) {
	return sendDialogueMessageWithCallback(orgID, conversationID, cookie, prompt, parentMessageUUID, nil)
}

func sendDialogueMessageWithCallback(orgID, conversationID, cookie, prompt, parentMessageUUID string, callback StreamCallback) (string, error) {
	return sendDialogueMessageWithOptions(orgID, conversationID, cookie, prompt, parentMessageUUID, "", "", callback)
}

func sendDialogueMessageWithOptions(orgID, conversationID, cookie, prompt, parentMessageUUID, modelID, styleKey string, callback StreamCallback) (string, error) {
	WaitForNextRequest()
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s/completion",
		orgID, conversationID)
	systemPrompt := LoadSystemPrompt()
	if systemPrompt != "" {
		prompt = systemPrompt + "\n\n" + prompt
	}
	reqBody := map[string]any{
		"prompt":              prompt,
		"parent_message_uuid": parentMessageUUID,
		"timezone":            "Asia/Shanghai",
		"rendering_mode":      "messages",
		"tools": []map[string]string{
			{"type": "web_search_v0", "name": "web_search"},
			{"type": "artifacts_v0", "name": "artifacts"},
		},
		"attachments": []any{},
		"files":       []any{},
	}
	if modelID != "" && modelID != "default" {
		switch modelID {
		case "opus-4.1":
			reqBody["model"] = "claude-opus-4-1-20250805"
		case "sonnet-4.5":
		}
	}
	if styleKey != "" && styleKey != "normal" {
		stylePrompt := getStylePrompt(styleKey)
		if stylePrompt != nil {
			reqBody["personalized_styles"] = []map[string]any{
				{
					"type":   "preset",
					"key":    stylePrompt.Key,
					"name":   stylePrompt.Name,
					"prompt": stylePrompt.Prompt,
				},
			}
		}
	}
	jsonData, _ := json.Marshal(reqBody)
	DebugLog("Dialogue request body: %s", string(jsonData))
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	client := globalConfig.CreateHTTPClient(300 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	var eventType string
	var fullResponse strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var eventData map[string]any
			if json.Unmarshal([]byte(data), &eventData) == nil {
				if eventType == "content_block_delta" {
					if delta, ok := eventData["delta"].(map[string]any); ok {
						if text, ok := delta["text"].(string); ok {
							fullResponse.WriteString(text)
							if callback != nil {
								callback(fullResponse.String())
							}
						}
					}
				}
			}
			if eventType == "message_stop" {
				return fullResponse.String(), nil
			}
			if eventType == "error" {
				return "", fmt.Errorf("received error event: %s", data)
			}
			eventType = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read response failed: %v", err)
	}
	return fullResponse.String(), nil
}

func sendDialogueMessageWithFiles(orgID, conversationID, cookie, prompt, parentMessageUUID, modelID, styleKey string, attachments []FileAttachment, callback StreamCallback) (string, error) {
	WaitForNextRequest()
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s/completion",
		orgID, conversationID)
	systemPrompt := LoadSystemPrompt()
	if systemPrompt != "" {
		prompt = systemPrompt + "\n\n" + prompt
	}
	attachmentsPayload := make([]map[string]any, 0)
	for _, att := range attachments {
		attachmentsPayload = append(attachmentsPayload, map[string]any{
			"file_name":         att.FileName,
			"file_type":         att.FileType,
			"file_size":         att.FileSize,
			"extracted_content": att.ExtractedText,
		})
	}
	var tools []map[string]any
	if globalMCPSessionManager != nil {
		tools = globalMCPSessionManager.GetToolsForRequest()
	} else {
		tools = []map[string]any{
			{"type": "web_search_v0", "name": "web_search"},
			{"type": "artifacts_v0", "name": "artifacts"},
		}
	}
	reqBody := map[string]any{
		"prompt":              prompt,
		"parent_message_uuid": parentMessageUUID,
		"timezone":            "Asia/Shanghai",
		"rendering_mode":      "messages",
		"tools":               tools,
		"attachments":         attachmentsPayload,
		"files":               []any{},
	}
	if modelID != "" && modelID != "default" {
		switch modelID {
		case "opus-4.1":
			reqBody["model"] = "claude-opus-4-1-20250805"
		case "sonnet-4.5":
		}
	}
	if styleKey != "" && styleKey != "normal" {
		stylePrompt := getStylePrompt(styleKey)
		if stylePrompt != nil {
			reqBody["personalized_styles"] = []map[string]any{
				{
					"type":   "preset",
					"key":    stylePrompt.Key,
					"name":   stylePrompt.Name,
					"prompt": stylePrompt.Prompt,
				},
			}
		}
	}
	jsonData, _ := json.Marshal(reqBody)
	DebugLog("Dialogue request with files body: %s", string(jsonData))
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	client := globalConfig.CreateHTTPClient(300 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	var eventType string
	var fullResponse strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var eventData map[string]any
			if json.Unmarshal([]byte(data), &eventData) == nil {
				if eventType == "content_block_delta" {
					if delta, ok := eventData["delta"].(map[string]any); ok {
						if text, ok := delta["text"].(string); ok {
							fullResponse.WriteString(text)
							if callback != nil {
								callback(fullResponse.String())
							}
						}
					}
				}
			}
			if eventType == "message_stop" {
				return fullResponse.String(), nil
			}
			if eventType == "error" {
				return "", fmt.Errorf("received error event: %s", data)
			}
			eventType = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read response failed: %v", err)
	}
	return fullResponse.String(), nil
}
