package preview

import "strings"

const (
	ToolStatusCalling = "calling"
	ToolStatusOK      = "ok"
	ToolStatusError   = "error"
)

// NormalizeToolStatus maps projector / trpc statuses to preview semantics.
func NormalizeToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ToolStatusCalling, "running", "pending", "in_progress":
		return ToolStatusCalling
	case ToolStatusError, "failed":
		return ToolStatusError
	case ToolStatusOK, "success", "done":
		return ToolStatusOK
	default:
		return strings.TrimSpace(status)
	}
}

// ToolStatusInFlight reports whether a tool is still running.
func ToolStatusInFlight(status string) bool {
	switch NormalizeToolStatus(status) {
	case ToolStatusCalling:
		return true
	default:
		return false
	}
}

// IsTerminalToolStatus reports whether a tool status is terminal (card may show final state).
func IsTerminalToolStatus(status string) bool {
	return !ToolStatusInFlight(status)
}

// IsToolStatusError reports whether a tool finished with an error.
func IsToolStatusError(status string) bool {
	return NormalizeToolStatus(status) == ToolStatusError
}
