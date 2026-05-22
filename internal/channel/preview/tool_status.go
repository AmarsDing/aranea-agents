package preview

import "strings"

const (
	ToolStatusCalling = "calling"
	ToolStatusOK      = "ok"
	ToolStatusError   = "error"
)

// ToolStatusInFlight reports whether a tool is still running.
func ToolStatusInFlight(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ToolStatusCalling, "running", "pending":
		return true
	default:
		return false
	}
}

// IsTerminalToolStatus reports whether a tool status is terminal (card may show final state).
func IsTerminalToolStatus(status string) bool {
	return !ToolStatusInFlight(status)
}
