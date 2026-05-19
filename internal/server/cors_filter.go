package server

import (
	"net/http"
	"os"
	"strings"

	httpm "github.com/go-kratos/kratos/v2/transport/http"
)

// corsAllowedDevPrefixes lists Origin prefixes mirrored when no extra env is set.
var corsAllowedDevPrefixes = []string{
	"http://localhost:",
	"http://127.0.0.1:",
	"http://[::1]:",
	"https://localhost:",
	"https://127.0.0.1:",
	"https://[::1]:",
}

func OriginAllowed(origin string) bool {
	o := strings.TrimSpace(origin)
	if o == "" {
		return false
	}
	for _, p := range corsAllowedDevPrefixes {
		if strings.HasPrefix(o, p) {
			return true
		}
	}
	for _, raw := range strings.Split(os.Getenv("KRATOS_HTTP_EXTRA_CORS_ORIGINS"), ",") {
		if suf := strings.TrimSpace(raw); suf != "" && o == suf {
			return true
		}
	}
	return false
}

// CorsDevFilter echoes allowed browser Origins so local SPAs (e.g. Quasar on :9001 hitting API on :8000)
// receive Access-Control-* on errors as well—otherwise 401 without CORS surfaces as “blocked by CORS”.
func CorsDevFilter() httpm.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			o := strings.TrimSpace(r.Header.Get("Origin"))
			if OriginAllowed(o) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", o)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				h.Set("Access-Control-Max-Age", "86400")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
