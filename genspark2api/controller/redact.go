package controller

import "strings"

func redactCookie(cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return ""
	}

	if strings.HasPrefix(cookie, "session_id=") {
		value := strings.TrimPrefix(cookie, "session_id=")
		if len(value) <= 16 {
			return "session_id=***"
		}
		return "session_id=" + value[:8] + "..." + value[len(value)-6:]
	}

	if len(cookie) <= 16 {
		return "***"
	}
	return cookie[:8] + "..." + cookie[len(cookie)-6:]
}

