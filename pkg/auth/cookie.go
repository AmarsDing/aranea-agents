package auth

import (
	"net/http"
	"os"
	"strings"
	"time"
)

func newSessionCookie(value string, expiresAt time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if cookieSecureEnabled() {
		c.Secure = true
	}
	return c
}

func newClearedSessionCookie() *http.Cookie {
	expires := time.Now().AddDate(0, 0, -1)
	c := newSessionCookie("", expires)
	c.MaxAge = -1
	return c
}

func cookieSecureEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("KRATOS_AUTH_COOKIE_SECURE")))
	if v == "1" || v == "true" || v == "yes" {
		return true
	}
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	// Default: Secure only when explicitly requested; local HTTP dev stays non-Secure.
	return false
}

// CookieName returns the session cookie name (default access_token).
func CookieName() string {
	return cookieName
}
