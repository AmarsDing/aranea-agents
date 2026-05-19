package auth

import (
	"net/http"
	"strings"
)

// TokenFromHTTPRequest extracts a session JWT from Cookie, Authorization Bearer, or token query.
func TokenFromHTTPRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if c, err := r.Cookie(cookieName); err == nil {
		if v := strings.TrimSpace(c.Value); v != "" {
			return v
		}
	}
	if v := bearerToken(r.Header.Get("Authorization")); v != "" {
		return v
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func bearerToken(header string) string {
	v := strings.TrimSpace(header)
	after, found := strings.CutPrefix(v, "Bearer ")
	if !found {
		after, found = strings.CutPrefix(v, "bearer ")
	}
	if !found {
		return ""
	}
	return strings.TrimSpace(after)
}
