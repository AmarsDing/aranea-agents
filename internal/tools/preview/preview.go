package preview

import (
	"regexp"
	"strings"
)

const defaultMaxPreview = 2000

// 本包是工具输出/参数预览脱敏的唯一权威实现（A4 合并）。
// biz 侧 RedactToolPreview 直接委托至此，禁止再维护平行 regex 链。
var (
	emailRE = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	// Phone: 必须带国家码前缀或分隔符，否则不匹配。裸数字串（如 10 位秒级或
	// 13 位毫秒级 Unix 时间戳）一律保留——工具输出中时间戳远比号码常见，
	// 误伤时间戳对排查日志的伤害大于漏脱敏一个无格式号码。
	// "00" 前缀分支带 \b，避免在纯数字时间戳中段（如 1700000000000 内的 00…）误匹配。
	phoneRE   = regexp.MustCompile(`(?:\+\d{1,3}[\s-]?|\b00\d{1,3}[\s-]?)(?:\d[\s-]?){6,11}\d|\b\d{2,4}(?:[\s-]\d{3,4}){2,3}`)
	secretKVRE = regexp.MustCompile(`(?i)"(?:api[_-]?key|secret|token|password|authorization)"\s*:\s*"[^"]*"`)
	secretRE   = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|authorization|bearer)\s*[:=]\s*\S{8,}`)

	// API key patterns with word-boundary anchoring to prevent false positives
	// (e.g. task-, disk-, risk- prefixed words that end in -sk).
	openAIKeyRE    = regexp.MustCompile(`\bsk-[a-zA-Z0-9]{10,}\b`)
	anthropicKeyRE = regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9\-]{10,}\b`)
	xaiKeyRE       = regexp.MustCompile(`\bxai-[a-zA-Z0-9]{8,}\b`)

	// AWS access key ID 前缀（AKIA 长期 / ASIA 临时，外加常见历史前缀）。
	awsKeyRE = regexp.MustCompile(`\b(?:AKIA|ASIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA)[0-9A-Z]{16}\b`)

	// GitHub tokens: ghp_/gho_/ghu_/ghs_/ghr_ + fine-grained github_pat_。
	githubTokenRE = regexp.MustCompile(`\b(?:gh[pousr]_[a-zA-Z0-9]{20,}|github_pat_[a-zA-Z0-9_]{20,})\b`)
	// GitLab personal access token。
	gitlabTokenRE = regexp.MustCompile(`\bglpat-[a-zA-Z0-9_\-]{16,}\b`)
	// Hugging Face access token。
	huggingFaceRE = regexp.MustCompile(`\bhf_[a-zA-Z0-9]{16,}\b`)
	googleKeyRE   = regexp.MustCompile(`\bAIza[0-9A-Za-z\-_]{30,}\b`)

	// Slack tokens: xoxb- (bot), xoxp- (user), xoxa- (app), xoxr- (refresh).
	slackTokenRE = regexp.MustCompile(`\bxox[bpar]-[a-zA-Z0-9\-]{10,}\b`)

	// Stripe keys: sk_/rk_/pk_ × live|test。
	stripeKeyRE = regexp.MustCompile(`\b[srp]k_(live|test)_[a-zA-Z0-9]{10,}\b`)

	// Authorization: Bearer <token> pattern (very common in HTTP headers).
	authBearerRE = regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+\S{8,}`)

	// JWT pattern: header 固定以 "eyJ"（{"）开头，三段 base64url 各 ≥5。
	jwtRE = regexp.MustCompile(`\beyJ[a-zA-Z0-9\-_]{5,}\.[a-zA-Z0-9\-_]{5,}\.[a-zA-Z0-9\-_]{5,}\b`)

	// PEM private key blocks（截断预览可能不含 END：整块 dotall 非贪婪优先，
	// 无 END 时兜底单独脱敏 BEGIN 行）。
	privateKeyRE     = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	privateKeyHeadRE = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)

	// DSN with embedded password: scheme://user:password@host
	dsnPasswordRE = regexp.MustCompile(`(?i)(postgres|mysql|redis|mongodb|amqp|nats)://[^:]+:[^@]+@`)
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
	// Apply specific API key patterns first (more precise than generic secretRE).
	s = openAIKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = anthropicKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = xaiKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = awsKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = githubTokenRE.ReplaceAllString(s, "[secret redacted]")
	s = gitlabTokenRE.ReplaceAllString(s, "[secret redacted]")
	s = huggingFaceRE.ReplaceAllString(s, "[secret redacted]")
	s = googleKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = slackTokenRE.ReplaceAllString(s, "[secret redacted]")
	s = stripeKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = authBearerRE.ReplaceAllString(s, "[secret redacted]")
	s = jwtRE.ReplaceAllString(s, "[secret redacted]")
	s = privateKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = privateKeyHeadRE.ReplaceAllString(s, "[secret redacted]")
	s = dsnPasswordRE.ReplaceAllString(s, "[secret redacted]")

	// Then apply generic patterns (email, phone, key-value secrets).
	s = emailRE.ReplaceAllString(s, "[email redacted]")
	s = phoneRE.ReplaceAllString(s, "[phone redacted]")
	s = secretKVRE.ReplaceAllString(s, `"[secret redacted]"`)
	// Always apply generic secretRE — it catches assignments (password=xxx)
	// that were not matched by any specific pattern above. The 8-character
	// minimum on the value prevents short tokens like "abc" from being
	// redacted, avoiding false positives.
	s = secretRE.ReplaceAllString(s, "[secret redacted]")
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}
