package controller

import (
	"encoding/json"
	"fmt"
	"genspark2api/common"
	"genspark2api/common/config"
	logger "genspark2api/common/loggger"
	"genspark2api/model"
	"github.com/deanxv/CycleTLS/cycletls"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"net/http"
	"strings"
	"time"
)

type responsesRequest struct {
	Model              string           `json:"model"`
	Stream             bool             `json:"stream"`
	Input              json.RawMessage  `json:"input"`
	Instructions       string           `json:"instructions"`
	Tools              []map[string]any `json:"tools"`
	ToolChoice         any              `json:"tool_choice"`
	PreviousResponseID string           `json:"previous_response_id"`
}

type responsesInputMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func cookieAvailabilityError() (int, string) {
	if len(config.GetGSCookies()) == 0 {
		return http.StatusInternalServerError, "No valid cookies available"
	}

	if len(config.NewCookieManager().Cookies) == 0 {
		entries := config.ListRateLimitCookies()
		until := ""
		now := time.Now()
		for _, entry := range entries {
			if entry.ExpirationTime.After(now) && (until == "" || entry.ExpirationTime.Format(time.RFC3339) < until) {
				until = entry.ExpirationTime.Format(time.RFC3339)
			}
		}
		if until != "" {
			return http.StatusTooManyRequests, fmt.Sprintf("All cookies are rate limited until %s", until)
		}
		return http.StatusTooManyRequests, "All cookies are temporarily unavailable"
	}

	return http.StatusInternalServerError, "No valid cookies available"
}

func normalizeResponsesModelName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(name, "-search") {
		return strings.TrimSpace(strings.TrimSuffix(name, "-search")), true
	}
	return name, false
}

func responsesRequestAsksForSearch(req responsesRequest) bool {
	if _, suffixSearch := normalizeResponsesModelName(req.Model); suffixSearch {
		return true
	}
	for _, tool := range req.Tools {
		if toolType, _ := tool["type"].(string); strings.Contains(strings.ToLower(toolType), "web_search") {
			return true
		}
		if toolName, _ := tool["name"].(string); strings.Contains(strings.ToLower(toolName), "web_search") {
			return true
		}
	}
	return false
}

func extractProjectIDFromEvent(current string, event map[string]any) string {
	if strings.TrimSpace(current) != "" {
		return current
	}

	if v, ok := event["project_id"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}

	eventType, _ := event["type"].(string)
	switch eventType {
	case "project_start", "project_field":
		if v, ok := event["id"].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	case "message_field":
		// In some streams, an _updatetime field is emitted with message_id equal to project_id.
		if fieldName, _ := event["field_name"].(string); fieldName == "_updatetime" {
			if v, ok := event["message_id"].(string); ok && strings.TrimSpace(v) != "" {
				return v
			}
		}
	}

	return current
}

func ResponsesForOpenAI(c *gin.Context) {
	client := cycletls.Init()
	defer safeClose(client)

	var req responsesRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.OpenAIErrorResponse{
			OpenAIError: model.OpenAIError{
				Message: "Invalid request parameters",
				Type:    "request_error",
				Code:    "400",
			},
		})
		return
	}

	if strings.TrimSpace(req.Model) == "" {
		c.JSON(http.StatusBadRequest, model.OpenAIErrorResponse{
			OpenAIError: model.OpenAIError{
				Message: "model is required",
				Type:    "request_error",
				Code:    "400",
			},
		})
		return
	}

	if raw, err := json.Marshal(req.Tools); err == nil {
		logger.Infof(c.Request.Context(), "responses request: model=%s stream=%v previous_response_id=%s tools=%s", req.Model, req.Stream, req.PreviousResponseID, string(raw))
	} else {
		logger.Infof(c.Request.Context(), "responses request: model=%s stream=%v previous_response_id=%s tools=<marshal_error>", req.Model, req.Stream, req.PreviousResponseID)
	}

	requestedModel := strings.TrimSpace(req.Model)
	upstreamModel, _ := normalizeResponsesModelName(requestedModel)
	openAIReq := model.OpenAIChatCompletionRequest{
		Model:  upstreamModel,
		Stream: req.Stream,
	}

	messages, err := messagesFromResponsesInput(req.Input)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.OpenAIErrorResponse{
			OpenAIError: model.OpenAIError{
				Message: err.Error(),
				Type:    "request_error",
				Code:    "400",
			},
		})
		return
	}

	if toolPrompt := buildToolPrompt(req); toolPrompt != "" {
		messages = append([]model.OpenAIChatMessage{{
			Role:    "system",
			Content: toolPrompt,
		}}, messages...)
	}

	if strings.TrimSpace(req.Instructions) != "" {
		messages = append([]model.OpenAIChatMessage{{
			Role:    "system",
			Content: req.Instructions,
		}}, messages...)
	}

	openAIReq.Messages = messages

	// 模型映射
	if strings.HasPrefix(openAIReq.Model, "deepseek") {
		openAIReq.Model = strings.Replace(openAIReq.Model, "deepseek", "deep-seek", 1)
	}

	// only support text via /responses for now
	if !lo.Contains(common.TextModelList, openAIReq.Model) {
		// allow Mixture-of-Agents / search modes via existing chat logic, but avoid image/video here
		if lo.Contains(common.ImageModelList, openAIReq.Model) || lo.Contains(common.VideoModelList, openAIReq.Model) {
			c.JSON(http.StatusBadRequest, model.OpenAIErrorResponse{
				OpenAIError: model.OpenAIError{
					Message: "responses endpoint only supports text models",
					Type:    "request_error",
					Code:    "400",
				},
			})
			return
		}
	}

	cookieManager := config.NewCookieManager()
	cookie, err := cookieManager.GetRandomCookie()
	if err != nil {
		statusCode, message := cookieAvailabilityError()
		c.JSON(statusCode, model.OpenAIErrorResponse{
			OpenAIError: model.OpenAIError{
				Message: message,
				Type:    "server_error",
				Code:    fmt.Sprintf("%d", statusCode),
			},
		})
		return
	}

	isSearchModel := responsesRequestAsksForSearch(req)
	requestBody, err := createRequestBodyWithOptions(c, client, cookie, &openAIReq, gensparkRequestOptions{
		RequestWebKnowledge:      isSearchModel,
		AllowSharedProjectReuse:  false,
		PersistLearnedProjectMap: false,
	})
	if err != nil {
		statusCode := http.StatusInternalServerError
		code := "500"
		if strings.Contains(err.Error(), "rate limited until") || strings.Contains(err.Error(), "temporarily unavailable") {
			statusCode = http.StatusTooManyRequests
			code = "429"
		}
		c.JSON(statusCode, model.OpenAIErrorResponse{
			OpenAIError: model.OpenAIError{
				Message: err.Error(),
				Type:    "server_error",
				Code:    code,
			},
		})
		return
	}
	prepareResponsesRequestBody(c, req, requestBody)

	recordUpstreamRequestSummary(c, cookie, openAIReq.Model, requestBody)

	if req.Stream {
		streamResponses(c, req, client, cookie, cookieManager, requestBody, requestedModel, isSearchModel, false)
		return
	}

	text, projectID, err := fetchResponsesText(c, client, cookie, cookieManager, requestBody, requestedModel, isSearchModel, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.OpenAIErrorResponse{
			OpenAIError: model.OpenAIError{
				Message: err.Error(),
				Type:    "server_error",
				Code:    "500",
			},
		})
		return
	}

	if response, ok := buildToolCallResponse(req, requestedModel, text); ok {
		storeResponseProjectID(response, projectID)
		logger.Infof(c.Request.Context(), "responses non-stream: returning tool call type=%v", response["output"])
		c.JSON(http.StatusOK, response)
		return
	}

	logger.Infof(c.Request.Context(), "responses non-stream: returning text response")
	response := buildResponsesResponse(requestedModel, text, "", "", 0)
	storeResponseProjectID(response, projectID)
	c.JSON(http.StatusOK, response)
}

func messagesFromResponsesInput(input json.RawMessage) ([]model.OpenAIChatMessage, error) {
	if len(input) == 0 || string(input) == "null" {
		return nil, fmt.Errorf("input is required")
	}

	var str string
	if err := json.Unmarshal(input, &str); err == nil {
		if strings.TrimSpace(str) == "" {
			return nil, fmt.Errorf("input is empty")
		}
		return []model.OpenAIChatMessage{{Role: "user", Content: str}}, nil
	}

	var items []map[string]any
	if err := json.Unmarshal(input, &items); err == nil {
		var msgs []model.OpenAIChatMessage
		for _, item := range items {
			msg, ok, err := messageFromResponsesItem(item)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			msgs = append(msgs, msg)
		}
		if len(msgs) == 0 {
			return nil, fmt.Errorf("input is empty")
		}
		return msgs, nil
	}

	var item map[string]any
	if err := json.Unmarshal(input, &item); err == nil {
		msg, ok, err := messageFromResponsesItem(item)
		if err != nil {
			return nil, err
		}
		if ok {
			return []model.OpenAIChatMessage{msg}, nil
		}
	}

	return nil, fmt.Errorf("unsupported input format")
}

func messageFromResponsesItem(item map[string]any) (model.OpenAIChatMessage, bool, error) {
	itemType, _ := item["type"].(string)
	switch itemType {
	case "", "message":
		b, err := json.Marshal(item)
		if err != nil {
			return model.OpenAIChatMessage{}, false, fmt.Errorf("invalid input item")
		}
		var m responsesInputMessage
		if err := json.Unmarshal(b, &m); err != nil {
			return model.OpenAIChatMessage{}, false, fmt.Errorf("invalid input item")
		}
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "user"
		}
		content, err := normalizeResponsesContent(m.Content)
		if err != nil {
			return model.OpenAIChatMessage{}, false, err
		}
		if strings.TrimSpace(content) == "" {
			return model.OpenAIChatMessage{}, false, nil
		}
		return model.OpenAIChatMessage{Role: role, Content: content}, true, nil
	case "function_call_output":
		callID, _ := item["call_id"].(string)
		output, _ := item["output"].(string)
		content := fmt.Sprintf("Tool result for %s:\n%s", callID, output)
		if strings.TrimSpace(callID) == "" {
			content = fmt.Sprintf("Tool result:\n%s", output)
		}
		return model.OpenAIChatMessage{Role: "user", Content: content}, true, nil
	default:
		return model.OpenAIChatMessage{}, false, nil
	}
}

func normalizeResponsesContent(content any) (string, error) {
	switch v := content.(type) {
	case string:
		return v, nil
	case []any:
		var sb strings.Builder
		for _, part := range v {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := partMap["type"].(string)
			switch partType {
			case "input_text", "text":
				if t, ok := partMap["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String(), nil
	default:
		return "", nil
	}
}

func buildResponsesResponse(modelName string, text string, respID string, msgID string, createdAt int64) map[string]any {
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	if respID == "" {
		respID = fmt.Sprintf("resp_%s", time.Now().Format("20060102150405"))
	}
	if msgID == "" {
		msgID = fmt.Sprintf("msg_%s", time.Now().Format("20060102150405"))
	}

	item := map[string]any{
		"id":     msgID,
		"type":   "message",
		"role":   "assistant",
		"status": "completed",
		"content": []any{
			map[string]any{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			},
		},
	}

	return map[string]any{
		"id":          respID,
		"object":      "response",
		"created_at":  createdAt,
		"model":       modelName,
		"status":      "completed",
		"output":      []any{item},
		"output_text": text,
	}
}

func buildToolPrompt(req responsesRequest) string {
	toolType, toolName, argName, ok := findShellTool(req)
	if !ok {
		return ""
	}

	return fmt.Sprintf(`You are assisting through Codex CLI with access to a local shell tool.
If you need to inspect files, create files, edit files, run tests, or verify results, do not describe the command in prose.
Reply with exactly one JSON object and nothing else:
{"tool":"%s","arguments":{"%s":"<shell command>"}}
You may also use {"tool":"%s","cmd":"<shell command>"}.
Only emit this JSON when a shell command should be executed locally. Tool type: %s.`, toolName, argName, toolName, toolType)
}

func detectShellCommand(text string) string {
	if cmd := detectShellCommandJSON(text); cmd != "" {
		return cmd
	}

	lines := strings.Split(text, "\n")
	inCodeBlock := false
	var codeLines []string

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeLines = nil
			} else {
				inCodeBlock = false
				cmd := strings.TrimSpace(strings.Join(codeLines, "\n"))
				if looksLikeShellCommand(cmd) {
					return cmd
				}
			}
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
		}
	}

	for i := len(lines) - 1; i >= 0; i-- {
		trim := strings.TrimSpace(lines[i])
		if looksLikeShellCommand(trim) {
			return trim
		}
	}

	return ""
}

func detectShellCommandJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}

	var payload struct {
		Tool      string         `json:"tool"`
		Command   string         `json:"command"`
		Cmd       string         `json:"cmd"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.Command) != "" {
		return strings.TrimSpace(payload.Command)
	}
	if strings.TrimSpace(payload.Cmd) != "" {
		return strings.TrimSpace(payload.Cmd)
	}
	for _, key := range []string{"cmd", "command", "input", "script"} {
		if value, ok := payload.Arguments[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func looksLikeShellCommand(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	prefixes := []string{
		"ls", "cat", "echo ", "pwd", "mkdir ", "touch ", "rm ", "cp ", "mv ",
		"grep ", "find ", "sed ", "awk ", "python ", "python3 ", "node ",
		"npm ", "pnpm ", "yarn ", "go ", "git ", "cargo ", "pytest ", "bash ", "sh ", "test ",
	}
	for _, prefix := range prefixes {
		if s == prefix || strings.HasPrefix(s, prefix) {
			return true
		}
	}

	return strings.Contains(s, " && ") || strings.Contains(s, " > ") || strings.Contains(s, " | ")
}

func findShellTool(req responsesRequest) (toolType string, toolName string, argName string, ok bool) {
	for _, tool := range req.Tools {
		name, _ := tool["name"].(string)
		typ, _ := tool["type"].(string)
		lowerName := strings.ToLower(name)
		if lowerName != "shell" &&
			lowerName != "bash" &&
			lowerName != "exec" &&
			!strings.Contains(lowerName, "shell") &&
			!strings.Contains(lowerName, "exec") {
			continue
		}

		argName = "cmd"
		if params, ok := tool["parameters"].(map[string]any); ok {
			if props, ok := params["properties"].(map[string]any); ok {
				for _, candidate := range []string{"cmd", "command", "input", "script"} {
					if _, exists := props[candidate]; exists {
						argName = candidate
						break
					}
				}
			}
		}

		return typ, name, argName, true
	}

	return "", "", "", false
}

func buildToolCallResponse(req responsesRequest, modelName string, text string) (map[string]any, bool) {
	cmd := detectShellCommand(text)
	toolType, toolName, argName, ok := findShellTool(req)
	if !ok || cmd == "" {
		return nil, false
	}

	responseID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	callID := fmt.Sprintf("call_%d", time.Now().UnixNano())
	itemID := fmt.Sprintf("fc_%d", time.Now().UnixNano())

	if toolType == "custom" {
		return map[string]any{
			"id":         responseID,
			"object":     "response",
			"created_at": time.Now().Unix(),
			"status":     "completed",
			"model":      modelName,
			"output": []any{
				map[string]any{
					"id":      itemID,
					"type":    "custom_tool_call",
					"status":  "completed",
					"call_id": callID,
					"name":    toolName,
					"input":   cmd,
				},
			},
		}, true
	}

	argsBytes, _ := json.Marshal(map[string]any{argName: cmd})
	return map[string]any{
		"id":         responseID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      modelName,
		"output": []any{
			map[string]any{
				"id":        itemID,
				"type":      "function_call",
				"status":    "completed",
				"call_id":   callID,
				"name":      toolName,
				"arguments": string(argsBytes),
			},
		},
	}, true
}

func writeResponsesEvent(c *gin.Context, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.Writer.Write([]byte("data: " + string(b) + "\n\n"))
	if err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func writeResponsesDone(c *gin.Context) error {
	_, err := c.Writer.Write([]byte("data: [DONE]\n\n"))
	if err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func prepareResponsesRequestBody(c *gin.Context, req responsesRequest, requestBody map[string]interface{}) {
	requestType, _ := requestBody["type"].(string)
	if strings.TrimSpace(requestType) == "" {
		requestType = chatType
	}

	if config.GlobalResponseSessionManager != nil && strings.TrimSpace(req.PreviousResponseID) != "" {
		if projectID, ok := config.GlobalResponseSessionManager.GetProjectID(req.PreviousResponseID); ok && strings.TrimSpace(projectID) != "" {
			requestBody["project_id"] = projectID
			delete(requestBody, "current_query_string")
			logger.Infof(c.Request.Context(), "responses request: reusing project_id=%s for previous_response_id=%s", projectID, req.PreviousResponseID)
			return
		}
	}

	delete(requestBody, "current_query_string")
}

func storeResponseProjectID(response map[string]any, projectID string) {
	if config.GlobalResponseSessionManager == nil || strings.TrimSpace(projectID) == "" {
		return
	}
	responseID, _ := response["id"].(string)
	config.GlobalResponseSessionManager.AddSession(responseID, projectID)
}

func isFinalAssistantMessage(event map[string]any) bool {
	messageObj, _ := event["message"].(map[string]any)
	if role, _ := messageObj["role"].(string); role != "" && role != "assistant" {
		return false
	}
	if role, _ := event["role"].(string); role != "" && role != "assistant" {
		return false
	}
	if toolCalls, ok := messageObj["tool_calls"].([]any); ok && len(toolCalls) > 0 {
		return false
	}
	finalText, ok := getEventContent(event)
	return ok && strings.TrimSpace(finalText) != ""
}

func streamResponses(c *gin.Context, req responsesRequest, client cycletls.CycleTLS, cookie string, cookieManager *config.CookieManager, requestBody map[string]interface{}, modelName string, searchModel bool, persistLearnedProjectMap bool) {
	const (
		errNoValidCookies         = "No valid cookies available"
		errCloudflareChallengeMsg = "Detected Cloudflare Challenge Page"
		errCloudflareBlock        = "CloudFlare: Sorry, you have been blocked"
		errServerErrMsg           = "An error occurred with the current request, please try again."
		errServiceUnavailable     = "Genspark Service Unavailable"
	)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	respID := fmt.Sprintf("resp_%s", time.Now().Format("20060102150405"))
	msgID := fmt.Sprintf("msg_%s", time.Now().Format("20060102150405"))
	now := time.Now().Unix()
	_ = searchModel

	_ = writeResponsesEvent(c, map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         respID,
			"object":     "response",
			"created_at": now,
			"model":      modelName,
			"status":     "in_progress",
			"output":     []any{},
		},
	})
	_ = writeResponsesEvent(c, map[string]any{
		"type": "response.in_progress",
		"response": map[string]any{
			"id":         respID,
			"object":     "response",
			"created_at": now,
			"model":      modelName,
			"status":     "in_progress",
			"output":     []any{},
		},
	})

	ctx := c.Request.Context()
	maxRetries := len(cookieManager.Cookies)
	var fullText strings.Builder
	var projectID string

	for attempt := 0; attempt < maxRetries; attempt++ {
		rb, err := cheat(requestBody, c, cookie)
		if err != nil {
			_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": err.Error()}})
			return
		}
		jsonData, err := json.Marshal(rb)
		if err != nil {
			_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": "Failed to marshal request body"}})
			return
		}

		sseChan, err := makeStreamRequest(c, client, agentAskProxyEndpoint, jsonData, cookie)
		if err != nil {
			_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": err.Error()}})
			return
		}

		isRateLimit := false
		for response := range sseChan {
			if response.Done {
				break
			}
			line := strings.TrimSpace(response.Data)
			if line == "" {
				continue
			}

			switch {
			case common.IsCloudflareChallenge(line):
				logger.Errorf(ctx, errCloudflareChallengeMsg)
				_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": errCloudflareChallengeMsg}})
				return
			case common.IsCloudflareBlock(line):
				logger.Errorf(ctx, errCloudflareBlock)
				_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": errCloudflareBlock}})
				return
			case common.IsServiceUnavailablePage(line):
				logger.Errorf(ctx, errServiceUnavailable)
				_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": errServiceUnavailable}})
				return
			case common.IsServerError(line):
				logger.Errorf(ctx, errServerErrMsg)
				_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": errServerErrMsg}})
				return
			case common.IsRateLimit(line):
				isRateLimit = true
				logger.Warnf(ctx, "Cookie rate limited, switching to next cookie, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, redactCookie(cookie))
				config.AddRateLimitCookie(cookie, time.Now().Add(time.Duration(config.RateLimitCookieLockDuration)*time.Second))
				break
			case common.IsFreeLimit(line):
				isRateLimit = true
				logger.Warnf(ctx, "Cookie free rate limited, switching to next cookie, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, redactCookie(cookie))
				config.AddRateLimitCookie(cookie, time.Now().Add(24*60*60*time.Second))
				break
			case common.IsNotLogin(line):
				isRateLimit = true
				logger.Warnf(ctx, "Cookie Not Login, switching to next cookie, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, redactCookie(cookie))
				config.RemoveCookie(cookie)
				break
			}

			data := line
			if strings.HasPrefix(data, "data: ") {
				data = strings.TrimPrefix(data, "data: ")
			}
			data = strings.TrimSpace(data)
			if !strings.HasPrefix(data, "{") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			projectID = extractProjectIDFromEvent(projectID, event)
			recordUpstreamEvent(cookie, modelName, projectID, event)
			recordUpstreamRawEvent(event)

			eventType, _ := event["type"].(string)
			switch eventType {
			case "message_field_delta":
				fieldName, _ := event["field_name"].(string)
				if fieldName != "content" {
					continue
				}
				delta, _ := event["delta"].(string)
				if delta == "" {
					continue
				}
				fullText.WriteString(delta)
			case "message_result":
				if !isFinalAssistantMessage(event) {
					continue
				}
				finalText, _ := getEventContent(event)
				fullText.Reset()
				fullText.WriteString(finalText)

				text := fullText.String()
				if persistLearnedProjectMap && config.AutoModelChatMapType == 1 && strings.TrimSpace(projectID) != "" {
					config.GlobalSessionManager.AddSession(cookie, modelName, projectID)
				}
				if toolResp, ok := buildToolCallResponse(req, modelName, text); ok {
					storeResponseProjectID(toolResp, projectID)
					logger.Infof(c.Request.Context(), "responses stream: emitting tool call response")
					emitToolCallStreamEvents(c, respID, toolResp)
				} else {
					logger.Infof(c.Request.Context(), "responses stream: emitting text response text=%q", text)
					response := buildResponsesResponse(modelName, text, respID, msgID, now)
					storeResponseProjectID(response, projectID)
					emitTextStreamEvents(c, response)
				}
				return
			}
		}

		if !isRateLimit {
			_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": "All cookies are temporarily unavailable."}})
			return
		}

		cookie, err = cookieManager.GetNextCookie()
		if err != nil {
			_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": errNoValidCookies}})
			return
		}
	}

	_ = writeResponsesEvent(c, map[string]any{"type": "response.error", "error": map[string]any{"message": "All cookies are temporarily unavailable."}})
}

func emitTextStreamEvents(c *gin.Context, response map[string]any) {
	respID, _ := response["id"].(string)
	outputText, _ := response["output_text"].(string)
	output, _ := response["output"].([]any)
	msgID := ""
	if len(output) > 0 {
		if item, ok := output[0].(map[string]any); ok {
			msgID, _ = item["id"].(string)
		}
	}
	_ = writeResponsesEvent(c, map[string]any{
		"type":         "response.output_item.added",
		"response_id":  respID,
		"output_index": 0,
		"item": map[string]any{
			"id":     msgID,
			"type":   "message",
			"role":   "assistant",
			"status": "in_progress",
			"content": []any{
				map[string]any{
					"type":        "output_text",
					"text":        "",
					"annotations": []any{},
				},
			},
		},
	})
	_ = writeResponsesEvent(c, map[string]any{
		"type":          "response.content_part.added",
		"response_id":   respID,
		"item_id":       msgID,
		"output_index":  0,
		"content_index": 0,
		"part": map[string]any{
			"type":        "output_text",
			"text":        "",
			"annotations": []any{},
		},
	})
	_ = writeResponsesEvent(c, map[string]any{
		"type":          "response.output_text.delta",
		"response_id":   respID,
		"item_id":       msgID,
		"output_index":  0,
		"content_index": 0,
		"delta":         outputText,
	})
	_ = writeResponsesEvent(c, map[string]any{
		"type":          "response.output_text.done",
		"response_id":   respID,
		"item_id":       msgID,
		"output_index":  0,
		"content_index": 0,
		"text":          outputText,
	})
	_ = writeResponsesEvent(c, map[string]any{
		"type":          "response.content_part.done",
		"response_id":   respID,
		"item_id":       msgID,
		"output_index":  0,
		"content_index": 0,
		"part": map[string]any{
			"type":        "output_text",
			"text":        outputText,
			"annotations": []any{},
		},
	})
	_ = writeResponsesEvent(c, map[string]any{
		"type":         "response.output_item.done",
		"response_id":  respID,
		"output_index": 0,
		"item": map[string]any{
			"id":     msgID,
			"type":   "message",
			"role":   "assistant",
			"status": "completed",
			"content": []any{
				map[string]any{
					"type":        "output_text",
					"text":        outputText,
					"annotations": []any{},
				},
			},
		},
	})
	_ = writeResponsesEvent(c, map[string]any{
		"type":     "response.completed",
		"response": response,
	})
	_ = writeResponsesDone(c)
}

func emitToolCallStreamEvents(c *gin.Context, respID string, response map[string]any) {
	response["id"] = respID

	output, _ := response["output"].([]any)
	if len(output) == 0 {
		return
	}

	item, _ := output[0].(map[string]any)
	itemID, _ := item["id"].(string)
	itemType, _ := item["type"].(string)

	switch itemType {
	case "custom_tool_call":
		input, _ := item["input"].(string)
		_ = writeResponsesEvent(c, map[string]any{
			"type":         "response.output_item.added",
			"response_id":  respID,
			"output_index": 0,
			"item": map[string]any{
				"id":      itemID,
				"type":    itemType,
				"status":  "in_progress",
				"call_id": item["call_id"],
				"name":    item["name"],
				"input":   "",
			},
		})
		_ = writeResponsesEvent(c, map[string]any{
			"type":         "response.output_item.done",
			"response_id":  respID,
			"output_index": 0,
			"item": map[string]any{
				"id":      itemID,
				"type":    itemType,
				"status":  "completed",
				"call_id": item["call_id"],
				"name":    item["name"],
				"input":   input,
			},
		})
	default:
		arguments, _ := item["arguments"].(string)
		callID, _ := item["call_id"].(string)
		name, _ := item["name"].(string)
		_ = writeResponsesEvent(c, map[string]any{
			"type":         "response.output_item.added",
			"response_id":  respID,
			"output_index": 0,
			"item": map[string]any{
				"id":        itemID,
				"type":      itemType,
				"status":    "in_progress",
				"call_id":   item["call_id"],
				"name":      item["name"],
				"arguments": "",
			},
		})
		_ = writeResponsesEvent(c, map[string]any{
			"type":         "response.function_call_arguments.delta",
			"response_id":  respID,
			"item_id":      itemID,
			"output_index": 0,
			"call_id":      callID,
			"name":         name,
			"delta":        arguments,
		})
		_ = writeResponsesEvent(c, map[string]any{
			"type":         "response.function_call_arguments.done",
			"response_id":  respID,
			"item_id":      itemID,
			"output_index": 0,
			"call_id":      callID,
			"name":         name,
			"arguments":    arguments,
		})
		_ = writeResponsesEvent(c, map[string]any{
			"type":         "response.output_item.done",
			"response_id":  respID,
			"output_index": 0,
			"item": map[string]any{
				"id":        itemID,
				"type":      itemType,
				"status":    "completed",
				"call_id":   item["call_id"],
				"name":      item["name"],
				"arguments": arguments,
			},
		})
	}

	_ = writeResponsesEvent(c, map[string]any{
		"type":     "response.completed",
		"response": response,
	})
	_ = writeResponsesDone(c)
}

func fetchResponsesText(c *gin.Context, client cycletls.CycleTLS, cookie string, cookieManager *config.CookieManager, requestBody map[string]interface{}, modelName string, searchModel bool, persistLearnedProjectMap bool) (string, string, error) {
	maxRetries := len(cookieManager.Cookies)
	ctx := c.Request.Context()
	_ = modelName
	_ = searchModel
	var projectID string

	for attempt := 0; attempt < maxRetries; attempt++ {
		rb, err := cheat(requestBody, c, cookie)
		if err != nil {
			return "", "", err
		}
		jsonData, err := json.Marshal(rb)
		if err != nil {
			return "", "", fmt.Errorf("Failed to marshal request body")
		}

		resp, err := makeRequest(client, agentAskProxyEndpoint, jsonData, cookie, false)
		if err != nil {
			return "", "", err
		}

		lines := strings.Split(resp.Body, "\n")
		var full strings.Builder
		isRateLimit := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			switch {
			case common.IsRateLimit(line):
				isRateLimit = true
				logger.Warnf(ctx, "Cookie rate limited, switching to next cookie, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, redactCookie(cookie))
				config.AddRateLimitCookie(cookie, time.Now().Add(time.Duration(config.RateLimitCookieLockDuration)*time.Second))
				break
			case common.IsFreeLimit(line):
				isRateLimit = true
				logger.Warnf(ctx, "Cookie free rate limited, switching to next cookie, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, redactCookie(cookie))
				config.AddRateLimitCookie(cookie, time.Now().Add(24*60*60*time.Second))
				break
			case common.IsNotLogin(line):
				isRateLimit = true
				logger.Warnf(ctx, "Cookie Not Login, switching to next cookie, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, redactCookie(cookie))
				config.RemoveCookie(cookie)
				break
			}

			data := line
			if strings.HasPrefix(data, "data: ") {
				data = strings.TrimPrefix(data, "data: ")
			}
			data = strings.TrimSpace(data)
			if !strings.HasPrefix(data, "{") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			projectID = extractProjectIDFromEvent(projectID, event)
			recordUpstreamEvent(cookie, modelName, projectID, event)
			recordUpstreamRawEvent(event)

			eventType, _ := event["type"].(string)
			switch eventType {
			case "message_field_delta":
				fieldName, _ := event["field_name"].(string)
				if fieldName != "content" {
					continue
				}
				delta, _ := event["delta"].(string)
				full.WriteString(delta)
			case "message_result":
				if !isFinalAssistantMessage(event) {
					continue
				}
				finalText, _ := getEventContent(event)
				if persistLearnedProjectMap && config.AutoModelChatMapType == 1 && strings.TrimSpace(projectID) != "" {
					config.GlobalSessionManager.AddSession(cookie, modelName, projectID)
				}
				return finalText, projectID, nil
			}
		}

		if !isRateLimit {
			if full.String() != "" {
				if persistLearnedProjectMap && config.AutoModelChatMapType == 1 && strings.TrimSpace(projectID) != "" {
					config.GlobalSessionManager.AddSession(cookie, modelName, projectID)
				}
				return full.String(), projectID, nil
			}
			return "", "", fmt.Errorf("All cookies are temporarily unavailable.")
		}

		cookie, err = cookieManager.GetNextCookie()
		if err != nil {
			return "", "", fmt.Errorf("No valid cookies available")
		}
	}

	return "", "", fmt.Errorf("All cookies are temporarily unavailable.")
}
