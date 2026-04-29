package middleware

import (
	"net/http"
	"os"
)

func BasicAuth(next http.Handler) http.Handler {
	user := os.Getenv("API_BASIC_USER")
	pass := os.Getenv("API_BASIC_PASS")
	if user == "" || pass == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="arenea"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
