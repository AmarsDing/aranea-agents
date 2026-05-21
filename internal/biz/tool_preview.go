package biz

import (
	"regexp"
	"strings"
)

const toolPreviewMaxLen = 2000

var (
	toolPreviewEmailRE    = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	toolPreviewPhoneRE    = regexp.MustCompile(`(?:(?:\+|00)\d{1,3}[\s-]?)?(?:\d[\s-]?){8,14}\d`)
	toolPreviewSecretKVRE = regexp.MustCompile(`(?i)"(?:api[_-]?key|secret|token|password|authorization)"\s*:\s*"[^"]*"`)
	toolPreviewSecretRE   = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|authorization|bearer)\s*[:=]\s*\S+`)
)

// RedactToolPreview masks common sensitive patterns then truncates.
func RedactToolPreview(raw string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = toolPreviewMaxLen
	}
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = toolPreviewEmailRE.ReplaceAllString(s, "[email redacted]")
	s = toolPreviewPhoneRE.ReplaceAllString(s, "[phone redacted]")
	s = toolPreviewSecretKVRE.ReplaceAllString(s, `"[secret redacted]"`)
	s = toolPreviewSecretRE.ReplaceAllString(s, "[secret redacted]")
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
