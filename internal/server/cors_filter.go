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

// localhostReflectionAllowed gates the reflexive localhost/loopback allowance
// (F1): reflecting any localhost Origin WITH credentials means any malicious
// local webpage can ride the user's API session in the desktop-single-host
// form factor (SameSite=Lax treats ports as same-site). Keep the reflexive
// allowance for dev/test/unset deployments only; explicit non-dev deployments
// must opt in per origin via KRATOS_HTTP_EXTRA_CORS_ORIGINS.
func localhostReflectionAllowed() bool {
	env := strings.TrimSpace(strings.ToLower(os.Getenv("DEPLOY_ENV")))
	switch env {
	case "", "dev", "development", "test":
		return true
	}
	return false
}

func OriginAllowed(origin string) bool {
	o := strings.TrimSpace(origin)
	if o == "" {
		return false
	}
	if localhostReflectionAllowed() {
		for _, p := range corsAllowedDevPrefixes {
			if strings.HasPrefix(o, p) {
				return true
			}
		}
	}
	for _, raw := range strings.Split(os.Getenv("KRATOS_HTTP_EXTRA_CORS_ORIGINS"), ",") {
		if suf := strings.TrimSpace(raw); suf != "" && o == suf {
			return true
		}
	}
	return false
}

// WSOriginAllowed 是 WebSocket 升级请求的 Origin 判定（评审 2026-11-09 V2）：
// /v1/ws 与 /v1/voice 共用它，与 HTTP 面 OriginAllowed 同一策略——空
// Origin（非浏览器客户端：CLI/测试/服务间）放行；浏览器 Origin 必须命中
// CORS 白名单（dev localhost 前缀 + KRATOS_HTTP_EXTRA_CORS_ORIGINS 精确
// 匹配）。任何「同同源或白名单」之外的放宽都必须同时改 HTTP CORS 面，
// 保持两侧语义一致。
func WSOriginAllowed(r *http.Request) bool {
	o := strings.TrimSpace(r.Header.Get("Origin"))
	if o == "" {
		return true
	}
	return OriginAllowed(o)
}

// CorsDevFilter echoes allowed browser Origins so local SPAs (e.g. Quasar on :9301 hitting API on :8800)
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
