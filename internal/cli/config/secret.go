package config

import "strings"

// MaskToken returns a masked version of a token.
// Tokens up to 4 chars are fully masked. For longer tokens, shows the last 4
// chars prefixed by "***" (per PRD §7.1: get 显示为 ***<last4>).
func MaskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return "***"
	}
	return "***" + token[len(token)-4:]
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
