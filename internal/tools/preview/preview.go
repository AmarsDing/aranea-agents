package preview

import (
	"regexp"
	"strings"
)

const defaultMaxPreview = 2000

var (
	emailRE    = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRE    = regexp.MustCompile(`(?:(?:\+|00)\d{1,3}[\s-]?)?(?:\d[\s-]?){8,14}\d`)
	secretKVRE = regexp.MustCompile(`(?i)"(?:api[_-]?key|secret|token|password|authorization)"\s*:\s*"[^"]*"`)
	secretRE   = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|authorization|bearer)\s*[:=]\s*\S+`)
)

// RedactAndTruncate masks common sensitive patterns then truncates to maxLen.
func RedactAndTruncate(raw string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = defaultMaxPreview
	}
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = emailRE.ReplaceAllString(s, "[email redacted]")
	s = phoneRE.ReplaceAllString(s, "[phone redacted]")
	s = secretKVRE.ReplaceAllString(s, `"[secret redacted]"`)
	s = secretRE.ReplaceAllString(s, "[secret redacted]")
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
