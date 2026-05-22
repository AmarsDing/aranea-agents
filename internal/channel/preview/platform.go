package preview

import "strings"

// PlatformTextLimit returns the safe rune limit for IM preview PATCH on a platform.
func PlatformTextLimit(platform string) int {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "feishu", "lark", "slack":
		return 11800
	case "telegram":
		return 4000
	default:
		return 4000
	}
}
