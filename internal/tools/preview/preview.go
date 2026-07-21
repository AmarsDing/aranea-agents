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
	secretRE   = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|authorization|bearer)\s*[:=]\s*\S{8,}`)

	// API key patterns with word-boundary anchoring to prevent false positives
	// (e.g. task-, disk-, risk- prefixed words that end in -sk).
	openAIKeyRE    = regexp.MustCompile(`\bsk-[a-zA-Z0-9]{10,}\b`)
	anthropicKeyRE = regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9\-]{10,}\b`)
	xaiKeyRE       = regexp.MustCompile(`\bxai-[a-zA-Z0-9]{8,}\b`)
	awsAKIARE      = regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`)
	githubPATRE    = regexp.MustCompile(`\bghp_[a-zA-Z0-9]{20,}\b`)
	googleKeyRE    = regexp.MustCompile(`\bAIza[0-9A-Za-z\-_]{30,}\b`)

	// Slack tokens: xoxb- (bot), xoxp- (user), xoxa- (app), xoxr- (refresh).
	slackTokenRE = regexp.MustCompile(`\bxox[bpar]-[a-zA-Z0-9\-]{10,}\b`)

	// Stripe keys: sk_live_, rk_live_, sk_test_, rk_test_.
	stripeKeyRE = regexp.MustCompile(`\b[sr]k_(live|test)_[a-zA-Z0-9]{10,}\b`)

	// Authorization: Bearer <token> pattern (very common in HTTP headers).
	authBearerRE = regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+\S{8,}`)

	// JWT pattern: three base64url segments separated by dots.
	jwtRE = regexp.MustCompile(`\beyJ[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+\b`)

	// PEM private key blocks.
	privateKeyRE = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[^-]*-----END [A-Z ]*PRIVATE KEY-----`)

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
	s = awsAKIARE.ReplaceAllString(s, "[secret redacted]")
	s = githubPATRE.ReplaceAllString(s, "[secret redacted]")
	s = googleKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = slackTokenRE.ReplaceAllString(s, "[secret redacted]")
	s = stripeKeyRE.ReplaceAllString(s, "[secret redacted]")
	s = authBearerRE.ReplaceAllString(s, "[secret redacted]")
	s = jwtRE.ReplaceAllString(s, "[secret redacted]")
	s = privateKeyRE.ReplaceAllString(s, "[secret redacted]")
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
