package server

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"aranea-agents/internal/legacychat"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterLegacyChatForwardHTTPServer mounts **/v1/chat/*** on **cmd/admin**.
//
// - When **LEGACY_REST_ORIGIN** is set: reverse-proxy to **{origin}/api/v1/chat/***（与旧 transport 路径一致）。
// - When unset: **503** JSON，提示配置上游（直至 **arenea/backend** + ADK 可编入本二进制）。
//
// 不再经由 **LegacyRESTProxyFilter**，便于后续替换为本进程内 **chat/v1** 实现。
func RegisterLegacyChatForwardHTTPServer(srv *kratoshttp.Server) {
	r := srv.Route("/")
	raw := strings.TrimSpace(os.Getenv("LEGACY_REST_ORIGIN"))
	if raw == "" {
		r.POST("/v1/chat/messages", chatUpstreamUnavailable)
		r.POST("/v1/chat/messages/stream", chatUpstreamUnavailable)
		r.GET("/v1/chat/options", chatUpstreamUnavailable)
		return
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Host == "" {
		r.POST("/v1/chat/messages", chatUpstreamUnavailable)
		r.POST("/v1/chat/messages/stream", chatUpstreamUnavailable)
		r.GET("/v1/chat/options", chatUpstreamUnavailable)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		p := req.URL.Path
		if strings.HasPrefix(p, "/v1/chat/") {
			req.URL.Path = "/api" + p
		}
	}
	h := func(ctx kratoshttp.Context) error {
		proxy.ServeHTTP(ctx.Response(), ctx.Request())
		return nil
	}
	r.POST("/v1/chat/messages", h)
	r.POST("/v1/chat/messages/stream", h)
	r.GET("/v1/chat/options", h)
}

func chatUpstreamUnavailable(ctx kratoshttp.Context) error {
	w := ctx.Response()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"reason":  "CHAT_UPSTREAM_NOT_CONFIGURED",
		"message": "Set LEGACY_REST_ORIGIN to an HTTP root that serves " + legacychat.LegacyRoutePrefix + "/* until native chat is embedded (requires ADK / arenea/backend build deps).",
	})
	return nil
}
