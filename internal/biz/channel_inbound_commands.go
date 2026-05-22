package biz

import "strings"

var channelCancelCommands = map[string]struct{}{
	"取消": {}, "停止": {}, "cancel": {}, "stop": {}, "/cancel": {}, "/stop": {},
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
