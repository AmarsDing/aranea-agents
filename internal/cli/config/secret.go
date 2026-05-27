package config

import "strings"

// MaskToken returns a masked version of a token.
// Tokens up to 10 chars are fully masked. For longer tokens, shows a tiny
// trailing suffix proportional to length (at most 2 chars).
func MaskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 10 {
		return "***"
	}
	n := 40 / len(token)
	if n > 2 {
		n = 2
	}
	if n == 0 {
		return "***"
	}
	return "***" + token[len(token)-n:]
}

// TokenDisplay returns the token display string:
// - masked by default
// - full text only if showFull is true (caller must also print a warning)
func TokenDisplay(token string, showFull bool) string {
	if showFull {
		return token
	}
	return MaskToken(token)
}

// StripBearerPrefix strips "Bearer " prefix if present, returning the raw token.
func StripBearerPrefix(s string) string {
	if strings.HasPrefix(s, "Bearer ") {
		return strings.TrimPrefix(s, "Bearer ")
	}
	return s
}
