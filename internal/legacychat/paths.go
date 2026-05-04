package legacychat

import "strings"

// Paths shared by RegisterLegacyChatForwardHTTPServer and internal/cronrunner POST dispatch,
// so admin-mounted /v1/chat/* and upstream /api/v1/chat/* cannot drift.
const (
	AdminRoutePrefix  = "/v1/chat"
	LegacyRoutePrefix = "/api/v1/chat"
	MessagesPath      = LegacyRoutePrefix + "/messages"
)

// RewriteAdminRequestPath maps an incoming admin path (e.g. /v1/chat/messages) to the
// legacy upstream path (e.g. /api/v1/chat/messages). Unmatched paths are returned unchanged.
func RewriteAdminRequestPath(adminPath string) string {
	if strings.HasPrefix(adminPath, AdminRoutePrefix) {
		return LegacyRoutePrefix + strings.TrimPrefix(adminPath, AdminRoutePrefix)
	}
	return adminPath
}
