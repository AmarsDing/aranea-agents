package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/transport"
	httpm "github.com/go-kratos/kratos/v2/transport/http"
)

// defaultWebConsolePort 是宿主机上前端静态站（nginx）的发布端口，与
// docker-compose.yaml 的 web 服务保持一致。
const defaultWebConsolePort = "9402"

var (
	// noAuthPaths defines the paths that do not require authentication.
	noAuthPaths = map[string]struct{}{
		"/v1/admins/login": {},
		"/healthz":         {},
		// Prometheus 抓取端点（TwinMonitor 监控采集，2026-08-13）：指标不含敏感数据，
		// 与 /healthz 同级公开；若需收紧可改用独立监听端口或 Bearer 抓取。
		"/metrics": {},
	}
	noAuthPathPrefixes = []string{
		"/v1/a2a/public/",
		// E 阶段 K-3：ics 只读订阅为 token-in-path 公开端点（日历客户端无法带
		// Bearer，令牌即凭据；吊销即 401）。与 permission_registry_gen.go 的
		// public 豁免留痕一致（GetICalFeed 不登记权限项；该前缀下无其他路由）。
		"/v1/office/calendar/feeds/",
	}
	// authSecretKey is the secret key used for signing JWT tokens.
	authSecretKey = authSecretFromEnv("KRATOS_AUTH_SECRET")
	// cookieName is the name of the cookie that stores the authorization token.
	cookieName = cookieNameFromEnv("KRATOS_AUTH_COOKIE")
	// ErrUnauthorized indicates that the token is invalid.
	ErrUnauthorized = errors.Unauthorized("UNAUTHORIZED", "Token is invalid")
	// ErrForbidden indicates that access is denied.
	ErrForbidden = errors.Forbidden("FORBIDDEN", "Access denied")
)

// RegisterNoAuthPath marks an exact path as not requiring authentication.
func RegisterNoAuthPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	noAuthPaths[path] = struct{}{}
}

// RegisterNoAuthPathPrefix marks paths with the given prefix as not requiring authentication.
func RegisterNoAuthPathPrefix(prefix string) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return
	}
	noAuthPathPrefixes = append(noAuthPathPrefixes, prefix)
}

func isNoAuthPath(path string) bool {
	if _, ok := noAuthPaths[path]; ok {
		return true
	}
	for _, prefix := range noAuthPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// webConsoleURL 判定「浏览器文档导航打到了 API 端口根路径」并给出前端站点地址。
//
// 背景（2026-11-12）：宿主机同时发布 8810（本服务）与 9402（前端 nginx）。用户把
// 地址栏指到 8810 时，`GET /` 会落到本中间件，返回裸文本 "Unauthorized"，看起来
// 像整站不可用。这里只对**文档导航**（Accept 含 text/html）的根路径 302 到前端
// 站点，不放宽任何接口鉴权：`/v1/**` 一律仍需令牌，API 客户端（无 text/html
// Accept）拿到的仍是原来的 401。
func webConsoleURL(r *http.Request) string {
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return ""
	}
	switch r.URL.Path {
	case "/", "/index.html", "/login":
	default:
		return ""
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_WEB_URL")); v != "" {
		return strings.TrimRight(v, "/") + "/"
	}
	host := strings.TrimSpace(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(host, defaultWebConsolePort) + "/"
}

// Middleware is an authentication middleware for HTTP servers.
func Middleware(lg loggateway.Logger) httpm.FilterFunc {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if HTTPAuthBypassEnabled() {
				lg.Warn("auth bypass active: injecting dev principal",
					loggateway.StepID("auth.bypass"), loggateway.Str("path", r.URL.Path))
				next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), DevBypassPrincipal())))
				return
			}
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			// 浏览器误把 API 端口当站点入口：送到前端 nginx，而不是裸 401 文本。
			if target := webConsoleURL(r); target != "" {
				lg.Info("auth redirect: browser hit api root",
					loggateway.StepID("auth.web_redirect"), loggateway.Str("path", r.URL.Path))
				http.Redirect(w, r, target, http.StatusFound)
				return
			}
			if isNoAuthPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			// EP-SEC-03: only allow registered webhook paths; unregistered paths return 401.
			// Registered paths must still carry a signing header (Lark, GitHub, etc.) in
			// non-bypass mode; the actual signature content is verified by the handler.
			if strings.HasPrefix(r.URL.Path, "/webhooks/") {
				if !isRegisteredWebhookPath(r.URL.Path) {
					lg.Warn("webhook rejected: unregistered path",
						loggateway.StepID("auth.webhook"), loggateway.Str("path", r.URL.Path))
					http.Error(w, "Forbidden: unregistered webhook path", http.StatusForbidden)
					return
				}
				if !HTTPAuthBypassEnabled() && !hasWebhookSigningHeader(r) {
					http.Error(w, "Forbidden: webhook requires a signing header", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			tokenStr := TokenFromHTTPRequest(r)
			if tokenStr == "" {
				lg.Info("auth rejected: no token",
					loggateway.StepID("auth.no_token"), loggateway.Str("path", r.URL.Path))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			auth, err := ParseToken(tokenStr, authSecretKey)
			if err != nil {
				// K2: 登录失败 Warn 需节流防爆破刷屏（10s 窗口，简单计数）。
				if ok, suppressed := tokenInvalidThrottle.allow(); ok {
					fields := []loggateway.Field{
						loggateway.StepID("auth.token_invalid"),
						loggateway.Str("path", r.URL.Path),
						loggateway.Err(err),
					}
					if suppressed > 0 {
						fields = append(fields, loggateway.Int("suppressed", suppressed))
					}
					lg.Warn("auth rejected: token parse failed", fields...)
				}
				ec := errors.FromError(err)
				http.Error(w, ec.Message, int(ec.Code))
				return
			}
			// P1-3 会话吊销：claims.tv 与服务端 token_version 逐请求比对。
			if err := enforceTokenVersion(r.Context(), auth, lg, "auth.session_revoked"); err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := NewContext(r.Context(), auth)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SetCookie sets the login cookie in the HTTP response.
func SetCookie(ctx context.Context, userID int64, access string, expiresAt time.Time) error {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return fmt.Errorf("failed to get transport from context")
	}
	token, err := GenerateToken(userID, access, authSecretKey, expiresAt)
	if err != nil {
		return err
	}
	tr.ReplyHeader().Add("Set-Cookie", newSessionCookie(token, expiresAt).String())
	return nil
}

// IssueTokenForWorkspace generates a JWT bound to the given workspaceID using
// the server secret. P2 (mobile): the login handler issues the token once,
// sets it as the session cookie AND returns it in the response body so
// cross-origin clients (WebView / frp) can authenticate via Bearer header or
// WS `?token=` query when the HttpOnly cookie is unavailable.
//
// Deprecated(transition): P1 起登录链路改用 IssueTokenForPrincipal 以携带
// roles/clearance/tv claims；本函数仅兼容无角色的旧调用点。
func IssueTokenForWorkspace(userID int64, access, workspaceID string, expiresAt time.Time) (string, error) {
	return GenerateTokenForWorkspace(userID, access, workspaceID, authSecretKey, expiresAt)
}

// IssueTokenForPrincipal generates a JWT carrying the full P1 claim set
// (roles/clearance/tv) using the server secret. 登录链路唯一入口（P1-2）。
func IssueTokenForPrincipal(p Principal, expiresAt time.Time) (string, error) {
	return GenerateTokenForPrincipal(p, authSecretKey, expiresAt)
}

// SetCookieWithToken sets the login cookie from an already-issued token so
// the cookie and the response-body token stay identical (same jti/expiry).
func SetCookieWithToken(ctx context.Context, token string, expiresAt time.Time) error {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return fmt.Errorf("failed to get transport from context")
	}
	tr.ReplyHeader().Add("Set-Cookie", newSessionCookie(token, expiresAt).String())
	return nil
}

// SetCookieForWorkspace sets the login cookie with a JWT bound to the given
// workspaceID. This is the B-01 P2-A entry point: workspace membership is
// stamped into the JWT claim at login so subsequent requests carry it via
// cookie, not via forgeable client headers (see middleware/workspace.go).
//
// Use this instead of SetCookie when the principal has a bound workspace
// (e.g. admin login flows admin.WorkspaceID into the JWT). An empty
// workspaceID is normalized to DefaultWorkspaceID by GenerateTokenForWorkspace.
func SetCookieForWorkspace(ctx context.Context, userID int64, access, workspaceID string, expiresAt time.Time) error {
	token, err := IssueTokenForWorkspace(userID, access, workspaceID, expiresAt)
	if err != nil {
		return err
	}
	return SetCookieWithToken(ctx, token, expiresAt)
}

// DeleteCookie clears the login cookie in the HTTP response.
func DeleteCookie(ctx context.Context) error {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return fmt.Errorf("failed to get transport from context")
	}
	tr.ReplyHeader().Add("Set-Cookie", newClearedSessionCookie().String())
	return nil
}
