package toolapi

import (
	"fmt"
	"strings"
)

// ArgString 从 Arguments 中取字符串参数（容错 nil、数字等转文本）。
func ArgString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
