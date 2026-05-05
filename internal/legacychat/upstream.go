package legacychat

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// LegacyChatReverseProxy builds a single-host reverse proxy to the legacy HTTP root (LEGACY_REST_ORIGIN).
// Incoming paths under /v1/chat/ are rewritten to /api/v1/chat/ on the upstream.
func LegacyChatReverseProxy(upstreamRoot *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(upstreamRoot)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		p := req.URL.Path
		if strings.HasPrefix(p, AdminRoutePrefix+"/") {
			req.URL.Path = RewriteAdminRequestPath(p)
		}
	}
	return proxy
}
