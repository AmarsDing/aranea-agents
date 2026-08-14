package strutil

import (
	"strings"
	"unicode/utf8"
)

func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// TruncateRunes shortens s to at most maxRunes Unicode code points (safe for UTF-8 / protobuf).
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

// ValidUTF8 returns s if valid UTF-8; otherwise strips/replaces invalid sequences for proto string fields.
func ValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "")
}

// ProtoPreview prepares user-generated text for protobuf string fields.
func ProtoPreview(s string, maxRunes int) string {
	return ValidUTF8(TruncateRunes(s, maxRunes))
}

func SliceToSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			m[s] = true
		}
	}
	return m
}

func TruncateBytes(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TruncateRunesEllipsis shortens s to at most maxRunes code points and
// appends an ellipsis marker. Short strings pass through unchanged;
// maxRunes<=0 returns "" (consistent with TruncateRunes). This is the
// canonical prompt-facing truncation helper（链路优化批次 P1-3：rune 口径
// 统一）— prompt 注入链上的字段/块级截断一律走它，禁止再写本地副本。
func TruncateRunesEllipsis(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// TruncateBytesRuneSafe caps s at maxBytes, backing off to a UTF-8 rune
// boundary so the result is always valid UTF-8. No marker is appended.
// 用于字节口径的存储/协议上限（如 DB preview 列宽）。
func TruncateBytesRuneSafe(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// SliceBytesRuneSafe slices data to target bytes by mode, adjusting cut
// points to UTF-8 rune boundaries so the result never splits a multi-byte
// rune（修复工具结果截断把 CJK 切出 U+FFFD 的问题）。mode 语义：
//   - "tail"：保留头部 data[:target]（截掉尾部）
//   - "head"：保留尾部 data[len-target:]（截掉头部）
//   - "middle"：保留首尾各一半（截掉中间）
//
// target >= len(data) 时原样返回。
func SliceBytesRuneSafe(data []byte, target int, mode string) []byte {
	if target >= len(data) {
		return data
	}
	if target <= 0 {
		return nil
	}
	switch mode {
	case "head":
		start := len(data) - target
		for start < len(data) && !utf8.RuneStart(data[start]) {
			start++
		}
		return data[start:]
	case "middle":
		half := target / 2
		head := SliceBytesRuneSafe(data, half, "tail")
		tail := SliceBytesRuneSafe(data, target-half, "head")
		return append(append([]byte{}, head...), tail...)
	default: // "tail"
		for target > 0 && !utf8.RuneStart(data[target]) {
			target--
		}
		return data[:target]
	}
}
