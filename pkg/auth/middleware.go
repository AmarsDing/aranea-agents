package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/transport"
	httpm "github.com/go-kratos/kratos/v2/transport/http"
)

var (
	// noAuthPaths defines the paths that do not require authentication.
	noAuthPaths = map[string]struct{}{
		"/v1/admins/login": {},
		"/healthz":         {},
	}
	noAuthPathPrefixes = []string{
		"/v1/a2a/public/",
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
				lg.Warn("auth rejected: token parse failed",
					loggateway.StepID("auth.token_invalid"), loggateway.Str("path", r.URL.Path), loggateway.Err(err))
				ec := errors.FromError(err)
				http.Error(w, ec.Message, int(ec.Code))
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

// SetCookieForWorkspace sets the login cookie with a JWT bound to the given
// workspaceID. This is the B-01 P2-A entry point: workspace membership is
// stamped into the JWT claim at login so subsequent requests carry it via
// cookie, not via forgeable client headers (see middleware/workspace.go).
//
// Use this instead of SetCookie when the principal has a bound workspace
// (e.g. admin login flows admin.WorkspaceID into the JWT). An empty
// workspaceID is normalized to DefaultWorkspaceID by GenerateTokenForWorkspace.
func SetCookieForWorkspace(ctx context.Context, userID int64, access, workspaceID string, expiresAt time.Time) error {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return fmt.Errorf("failed to get transport from context")
	}
	token, err := GenerateTokenForWorkspace(userID, access, workspaceID, authSecretKey, expiresAt)
	if err != nil {
		return err
	}
	tr.ReplyHeader().Add("Set-Cookie", newSessionCookie(token, expiresAt).String())
	return nil
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
