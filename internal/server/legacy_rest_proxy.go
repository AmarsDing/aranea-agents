package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	httpm "github.com/go-kratos/kratos/v2/transport/http"
)

// LegacyRESTProxyFilter forwards selected **`/v1/...`** requests to another HTTP server (**`/api/v1/...`** on **LEGACY_REST_ORIGIN**), for routes not yet implemented in **cmd/admin** (e.g. chat, skills import multipart).
// **memory/v1** is served inside **cmd/admin** — do not proxy memory paths here.
//
// Non-matching paths fall through to the normal Kratos mux (**`/v1/sessions/{id}/messages`** 等不受影响).
//
// Set **LEGACY_REST_ORIGIN** while that upstream still exposes **`/api/v1/chat/*`** or **`/api/v1/skills/import*`**. **pkg/backend** is deprecated as the long-term home for those handlers; migrate into **cmd/admin** when ready.
//
// Rewritten prefixes（不含 **memory/v1** 路径）:
// - **`/v1/chat/*`** · **`/v1/skills/import*`**
func LegacyRESTProxyFilter() httpm.FilterFunc {
	raw := strings.TrimSpace(os.Getenv("LEGACY_REST_ORIGIN"))
	if raw == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		p := req.URL.Path
		if shouldRewriteLegacyREST(p) {
			req.URL.Path = "/api" + p
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldRewriteLegacyREST(r.URL.Path) {
				proxy.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func shouldRewriteLegacyREST(path string) bool {
	if strings.HasPrefix(path, "/v1/chat/") {
		return true
	}
	if path == "/v1/skills/import" || strings.HasPrefix(path, "/v1/skills/import/") {
		return true
	}
	return false
}
