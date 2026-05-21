package memory

import "aranea-agents/pkg/strutil"

func TruncateString(s string, n int) string {
	return strutil.TruncateBytes(s, n)
}
