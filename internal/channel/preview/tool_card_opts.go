package preview

import (
	"net/url"
	"strings"
)

// ToolCardBuildOpts carries session context for Feishu tool cards.
type ToolCardBuildOpts struct {
	SessionID string
	WebOrigin string
}

// BuildSessionWebURL returns a link to the web session detail page.
func BuildSessionWebURL(webOrigin, sessionID, toolID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	path := "/sessions/" + sessionID
	if toolID = strings.TrimSpace(toolID); toolID != "" {
		path += "?focus=tool&tool_id=" + url.QueryEscape(toolID)
	}
	origin := strings.TrimRight(strings.TrimSpace(webOrigin), "/")
	if origin == "" {
		return path
	}
	return origin + path
}
