package controller

import (
	"genspark2api/common/config"
	"github.com/gin-gonic/gin"
	"net/http"
	"sort"
	"strings"
	"time"
)

type debugSessionBinding struct {
	Model     string `json:"model"`
	ProjectID string `json:"project_id"`
	Cookie    string `json:"cookie"`
	Source    string `json:"source"`
}

type debugRateLimit struct {
	Cookie string `json:"cookie"`
	Until  string `json:"until"`
}

// DebugModelBindings returns current model->project bindings.
//
// It intentionally avoids returning raw cookies and only works when DEBUG=true.
func DebugModelBindings(c *gin.Context) {
	if !config.DebugEnabled {
		c.Status(http.StatusNotFound)
		return
	}

	now := time.Now()

	// Static bindings from env: MODEL_CHAT_MAP (model -> project/chat id)
	static := make(map[string]string, len(config.ModelChatMap))
	for k, v := range config.ModelChatMap {
		static[k] = v
	}

	// Dynamic bindings learned at runtime (cookie+model -> project/chat id)
	dynamic := make([]debugSessionBinding, 0)
	if config.GlobalSessionManager != nil {
		for _, entry := range config.GlobalSessionManager.ListSessions() {
			dynamic = append(dynamic, debugSessionBinding{
				Model:     entry.Model,
				ProjectID: entry.ChatID,
				Cookie:    redactCookie(entry.Cookie),
				Source:    "auto",
			})
		}
	}

	sort.Slice(dynamic, func(i, j int) bool {
		if dynamic[i].Model != dynamic[j].Model {
			return dynamic[i].Model < dynamic[j].Model
		}
		if dynamic[i].ProjectID != dynamic[j].ProjectID {
			return dynamic[i].ProjectID < dynamic[j].ProjectID
		}
		return dynamic[i].Cookie < dynamic[j].Cookie
	})

	// Cookie pool summary
	allCookies := config.GetGSCookies()
	availableCookies := config.NewCookieManager().Cookies

	rateLimited := make([]debugRateLimit, 0)
	for _, entry := range config.ListRateLimitCookies() {
		if entry.ExpirationTime.After(now) {
			rateLimited = append(rateLimited, debugRateLimit{
				Cookie: redactCookie(entry.Cookie),
				Until:  entry.ExpirationTime.Format(time.RFC3339),
			})
		}
	}
	sort.Slice(rateLimited, func(i, j int) bool { return rateLimited[i].Until < rateLimited[j].Until })

	// Helpful note if user pasted a full Cookie header with commas (will break splitting)
	cookieHint := ""
	if raw := strings.TrimSpace(config.GSCookie); raw != "" && strings.Contains(raw, "g_state=") && strings.Contains(raw, ",") {
		cookieHint = "GS_COOKIE appears to include cookies with commas; prefer only session_id=... values to avoid splitting issues."
	}

	c.JSON(http.StatusOK, gin.H{
		"debug_enabled":            true,
		"auto_model_chat_map_type": config.AutoModelChatMapType,
		"static_model_chat_map":    static,
		"dynamic_session_bindings": dynamic,
		"cookie_pool": gin.H{
			"configured_total": len(allCookies),
			"available_now":    len(availableCookies),
			"rate_limited":     rateLimited,
		},
		"hint": cookieHint,
	})
}

// DebugLastUpstreamTrace returns the last recorded upstream request/event summary.
// It intentionally avoids storing prompts/contents and only works when DEBUG=true.
func DebugLastUpstreamTrace(c *gin.Context) {
	if !config.DebugEnabled {
		c.Status(http.StatusNotFound)
		return
	}

	if trace, ok := getLastUpstreamTrace(); ok {
		c.JSON(http.StatusOK, trace)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"debug_enabled": true,
		"message":       "no upstream trace recorded yet",
	})
}

func DebugUpstreamTraceHistory(c *gin.Context) {
	if !config.DebugEnabled {
		c.Status(http.StatusNotFound)
		return
	}

	items, ok := getUpstreamTraceHistory()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"debug_enabled": true, "items": []any{}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"debug_enabled": true,
		"max_items":     50,
		"items":         items,
	})
}

func DebugLastUpstreamEvents(c *gin.Context) {
	if !config.DebugEnabled {
		c.Status(http.StatusNotFound)
		return
	}

	meta, events, ok := getLastUpstreamEvents()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"debug_enabled": true, "events": []any{}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"debug_enabled": true,
		"max_events":    200,
		"meta":          meta,
		"events":        events,
	})
}
