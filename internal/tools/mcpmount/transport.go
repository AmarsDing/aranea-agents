package mcpmount

import (
	"strings"
	"time"
)

func parseDurationSec(sec int) time.Duration {
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func normalizeTransport(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "stdio":
		return "stdio"
	case "sse":
		return "sse"
	case "streamable_http", "streamable":
		return "streamable"
	default:
		return t
	}
}
