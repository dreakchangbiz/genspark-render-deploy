package controller

import (
	"fmt"
	"genspark2api/common/config"
	"github.com/gin-gonic/gin"
	"strings"
	"sync"
	"time"
)

type upstreamTrace struct {
	AtRFC3339 string `json:"at_rfc3339"`

	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Cookie   string `json:"cookie"`

	RequestType        string   `json:"request_type"`
	RequestedModels    []string `json:"requested_models"`
	CurrentQueryString string   `json:"current_query_string"`
	SearchEnabled      bool     `json:"search_enabled"`
	MessageCount       int      `json:"message_count"`
	MessageRoles       []string `json:"message_roles,omitempty"`
	UserInputPreview   string   `json:"user_input_preview,omitempty"`

	ProjectID string `json:"project_id,omitempty"`

	UpstreamModelHint       string `json:"upstream_model_hint,omitempty"`
	UpstreamModelHintSource string `json:"upstream_model_hint_source,omitempty"`
}

var (
	upstreamTraceMu      sync.Mutex
	upstreamTraceHistory []upstreamTrace

	lastUpstreamEventsMeta upstreamTrace
	lastUpstreamEvents     []map[string]any
)

const maxUpstreamTraceHistory = 50
const maxUpstreamEvents = 200

func recordUpstreamRequestSummary(c *gin.Context, cookie string, modelName string, requestBody map[string]any) {
	if !config.DebugEnabled {
		return
	}

	var requestType string
	if v, ok := requestBody["type"].(string); ok {
		requestType = v
	}
	var currentQueryString string
	if v, ok := requestBody["current_query_string"].(string); ok {
		currentQueryString = v
	}

	var requestedModels []string
	if extra, ok := requestBody["extra_data"].(map[string]any); ok {
		if ms, ok := extra["models"].([]string); ok {
			requestedModels = append(requestedModels, ms...)
		} else if msAny, ok := extra["models"].([]any); ok {
			for _, item := range msAny {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					requestedModels = append(requestedModels, s)
				}
			}
		}
	}

	trace := upstreamTrace{
		AtRFC3339:          time.Now().Format(time.RFC3339),
		Endpoint:           c.FullPath(),
		Model:              modelName,
		Cookie:             redactCookie(cookie),
		RequestType:        requestType,
		RequestedModels:    requestedModels,
		CurrentQueryString: currentQueryString,
	}
	if enabled, ok := requestBody["ai_chat_enable_search"].(bool); ok {
		trace.SearchEnabled = enabled
	}
	if userInput, ok := requestBody["user_s_input"].(string); ok {
		trace.UserInputPreview = truncateOneLine(userInput, 120)
	}
	if projectID, ok := requestBody["project_id"].(string); ok && strings.TrimSpace(projectID) != "" {
		trace.ProjectID = projectID
	}
	if messages, ok := requestBody["messages"].([]map[string]any); ok {
		trace.MessageCount = len(messages)
		for _, message := range messages {
			if role, ok := message["role"].(string); ok && strings.TrimSpace(role) != "" {
				trace.MessageRoles = append(trace.MessageRoles, role)
			}
		}
	} else if messagesAny, ok := requestBody["messages"].([]any); ok {
		trace.MessageCount = len(messagesAny)
		for _, raw := range messagesAny {
			if message, ok := raw.(map[string]any); ok {
				if role, ok := message["role"].(string); ok && strings.TrimSpace(role) != "" {
					trace.MessageRoles = append(trace.MessageRoles, role)
				}
			}
		}
	}

	upstreamTraceMu.Lock()
	defer upstreamTraceMu.Unlock()
	upstreamTraceHistory = append(upstreamTraceHistory, trace)
	if len(upstreamTraceHistory) > maxUpstreamTraceHistory {
		// keep the newest N entries
		upstreamTraceHistory = upstreamTraceHistory[len(upstreamTraceHistory)-maxUpstreamTraceHistory:]
	}

	// Reset last-upstream-events buffer for easier manual inspection after each request.
	lastUpstreamEventsMeta = trace
	lastUpstreamEvents = lastUpstreamEvents[:0]
}

func recordUpstreamEvent(cookie string, modelName string, projectID string, event map[string]any) {
	if !config.DebugEnabled {
		return
	}

	hint, hintSource := extractUpstreamModelHint(event)
	if strings.TrimSpace(hint) == "" && strings.TrimSpace(projectID) == "" {
		return
	}

	upstreamTraceMu.Lock()
	defer upstreamTraceMu.Unlock()

	if len(upstreamTraceHistory) == 0 {
		return
	}

	redacted := redactCookie(cookie)

	// Update the most recent matching trace (same model + cookie).
	for i := len(upstreamTraceHistory) - 1; i >= 0; i-- {
		if upstreamTraceHistory[i].Model != modelName || upstreamTraceHistory[i].Cookie != redacted {
			continue
		}
		if strings.TrimSpace(projectID) != "" && strings.TrimSpace(upstreamTraceHistory[i].ProjectID) == "" {
			upstreamTraceHistory[i].ProjectID = projectID
		}
		if strings.TrimSpace(hint) != "" {
			upstreamTraceHistory[i].UpstreamModelHint = truncateOneLine(hint, 120)
			upstreamTraceHistory[i].UpstreamModelHintSource = hintSource
		}
		return
	}
}

func getLastUpstreamTrace() (*upstreamTrace, bool) {
	if !config.DebugEnabled {
		return nil, false
	}
	upstreamTraceMu.Lock()
	defer upstreamTraceMu.Unlock()
	if len(upstreamTraceHistory) == 0 {
		return nil, false
	}
	cp := upstreamTraceHistory[len(upstreamTraceHistory)-1]
	return &cp, true
}

func getUpstreamTraceHistory() ([]upstreamTrace, bool) {
	if !config.DebugEnabled {
		return nil, false
	}
	upstreamTraceMu.Lock()
	defer upstreamTraceMu.Unlock()
	if len(upstreamTraceHistory) == 0 {
		return []upstreamTrace{}, true
	}
	cp := make([]upstreamTrace, len(upstreamTraceHistory))
	copy(cp, upstreamTraceHistory)
	return cp, true
}

func recordUpstreamRawEvent(event map[string]any) {
	if !config.DebugEnabled {
		return
	}

	sanitized := sanitizeUpstreamEvent(event)

	upstreamTraceMu.Lock()
	defer upstreamTraceMu.Unlock()

	lastUpstreamEvents = append(lastUpstreamEvents, sanitized)
	if len(lastUpstreamEvents) > maxUpstreamEvents {
		lastUpstreamEvents = lastUpstreamEvents[len(lastUpstreamEvents)-maxUpstreamEvents:]
	}
}

func getLastUpstreamEvents() (upstreamTrace, []map[string]any, bool) {
	if !config.DebugEnabled {
		return upstreamTrace{}, nil, false
	}
	upstreamTraceMu.Lock()
	defer upstreamTraceMu.Unlock()

	meta := lastUpstreamEventsMeta
	events := make([]map[string]any, len(lastUpstreamEvents))
	copy(events, lastUpstreamEvents)
	return meta, events, true
}

func truncateOneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func extractUpstreamModelHint(event map[string]any) (string, string) {
	// Common places we've seen metadata show up in event-streams:
	// - top-level "model" / "llm_model"
	// - message_field events where field_name contains "model"
	candidates := []string{"llm_model", "model", "assistant_model", "engine", "provider"}
	for _, k := range candidates {
		if v, ok := event[k].(string); ok && strings.TrimSpace(v) != "" {
			return v, k
		}
	}

	if fieldName, ok := event["field_name"].(string); ok {
		lower := strings.ToLower(fieldName)
		if strings.Contains(lower, "model") || strings.Contains(lower, "models") {
			if v, ok := event["field_value"].(string); ok && strings.TrimSpace(v) != "" {
				return v, "field_value:" + fieldName
			}
			if v, ok := event["delta"].(string); ok && strings.TrimSpace(v) != "" {
				return v, "delta:" + fieldName
			}
			if fvAny, ok := event["field_value"].([]any); ok && len(fvAny) > 0 {
				var parts []string
				for _, item := range fvAny {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						parts = append(parts, s)
					}
				}
				if len(parts) > 0 {
					return strings.Join(parts, ","), "field_value:" + fieldName
				}
			}
		}
	}

	if fv, ok := event["field_value"].(map[string]any); ok {
		for _, k := range candidates {
			if v, ok := fv[k].(string); ok && strings.TrimSpace(v) != "" {
				return v, "field_value." + k
			}
		}
	}

	return "", ""
}

func sanitizeUpstreamEvent(event map[string]any) map[string]any {
	out := map[string]any{}

	keepKeys := []string{"type", "field_name", "id", "project_id", "message_id", "tool_call_id", "role"}
	for _, k := range keepKeys {
		if v, ok := event[k]; ok {
			out[k] = v
		}
	}

	// Keep small "field_value" values (strings/bools/numbers); truncate long strings.
	if v, ok := event["field_value"]; ok {
		switch t := v.(type) {
		case string:
			out["field_value"] = truncateOneLine(t, 200)
		case bool, float64, int, int64:
			out["field_value"] = t
		case []any:
			// Keep at most a few items (stringified) for debugging model metadata like session_state.models.
			var items []string
			for _, item := range t {
				if len(items) >= 10 {
					break
				}
				items = append(items, fmt.Sprintf("%v", item))
			}
			out["field_value"] = items
		}
	}

	if delta, ok := event["delta"].(string); ok && strings.TrimSpace(delta) != "" {
		out["delta"] = truncateOneLine(delta, 200)
	}

	if message, ok := event["message"].(map[string]any); ok {
		messageSummary := map[string]any{}
		if role, ok := message["role"].(string); ok && strings.TrimSpace(role) != "" {
			messageSummary["role"] = role
		}
		if content, ok := message["content"].(string); ok && strings.TrimSpace(content) != "" {
			messageSummary["content"] = truncateOneLine(content, 240)
		}
		if toolCalls, ok := summarizeToolCalls(message["tool_calls"]); ok {
			messageSummary["tool_calls"] = toolCalls
		}
		if len(messageSummary) > 0 {
			out["message"] = messageSummary
		}
	}

	if toolCalls, ok := summarizeToolCalls(event["tool_calls"]); ok {
		out["tool_calls"] = toolCalls
	}

	// Also keep explicit model hints if present.
	if hint, src := extractUpstreamModelHint(event); strings.TrimSpace(hint) != "" {
		out["upstream_model_hint"] = truncateOneLine(hint, 120)
		out["upstream_model_hint_source"] = src
	}

	return out
}

func summarizeToolCalls(raw any) ([]map[string]any, bool) {
	toolCalls, ok := raw.([]any)
	if !ok || len(toolCalls) == 0 {
		return nil, false
	}

	summaries := make([]map[string]any, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		callMap, ok := toolCall.(map[string]any)
		if !ok {
			continue
		}
		summary := map[string]any{}
		for _, key := range []string{"id", "type"} {
			if value, ok := callMap[key]; ok {
				summary[key] = value
			}
		}
		if functionMap, ok := callMap["function"].(map[string]any); ok {
			functionSummary := map[string]any{}
			if name, ok := functionMap["name"].(string); ok && strings.TrimSpace(name) != "" {
				functionSummary["name"] = name
			}
			if arguments, ok := functionMap["arguments"].(string); ok && strings.TrimSpace(arguments) != "" {
				functionSummary["arguments"] = truncateOneLine(arguments, 240)
			}
			if len(functionSummary) > 0 {
				summary["function"] = functionSummary
			}
		}
		if len(summary) > 0 {
			summaries = append(summaries, summary)
		}
	}

	if len(summaries) == 0 {
		return nil, false
	}
	return summaries, true
}
