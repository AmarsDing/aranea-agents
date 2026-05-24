package biz

import "strings"

var channelCancelCommands = map[string]struct{}{
	"取消": {}, "停止": {}, "cancel": {}, "stop": {}, "/cancel": {}, "/stop": {},
}

var channelBackgroundCommands = map[string]struct{}{
	"/background": {}, "background": {}, "后台": {}, "后台继续": {},
}

// IsChannelCancelCommand reports whether inbound IM text requests run cancellation (E6-4).
func IsChannelCancelCommand(text string) bool {
	key := strings.ToLower(strings.TrimSpace(text))
	if key == "" {
		return false
	}
	_, ok := channelCancelCommands[key]
	return ok
}

// IsChannelBackgroundCommand reports whether inbound IM text requests durable escalation (CC-R-02).
func IsChannelBackgroundCommand(text string) bool {
	key := strings.ToLower(strings.TrimSpace(text))
	if key == "" {
		return false
	}
	_, ok := channelBackgroundCommands[key]
	return ok
}

// IsChannelStatusQuery reports busy-line status intent (CH-BOR-03).
func IsChannelStatusQuery(text string) bool {
	switch strings.TrimSpace(text) {
	case "?", "？":
		return true
	default:
		return false
	}
}
