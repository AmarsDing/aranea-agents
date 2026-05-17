package plugintrpc

import "aranea-agents/internal/biz"

// Check validates that the caller has the required permission on a plugin.
// action values: "view" | "toggle" | "edit_config" | "view_logs"
func Check(p biz.Plugin, action string) bool {
	switch action {
	case "view":
		return p.Permissions.CanView
	case "toggle":
		return p.Permissions.CanToggle
	case "edit_config":
		return p.Permissions.CanEditConfig
	case "view_logs":
		return p.Permissions.CanViewLogs
	default:
		return false
	}
}

// AdminPermissions returns full-access permissions for admin-role callers.
func AdminPermissions() biz.PluginPermissions {
	return biz.PluginPermissions{
		CanView:       true,
		CanToggle:     true,
		CanEditConfig: true,
		CanViewLogs:   true,
	}
}
