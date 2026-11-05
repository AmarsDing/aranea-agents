package a2a

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	a2aserver "trpc.group/trpc-go/trpc-a2a-go/server"
)

// identity_hmac_middleware.go — 签名身份通道（aranea V-2）的应用层实现。
//
// 该能力原由 vendored fork（pkg/trpc-agent-go）提供；出库后上游
// trpc-agent-go 无此扩展，改在应用层用上游暴露的中间件钩子复刻：
//   - server 端：WithPreAuthA2AMiddleware 在 defaultAuthProvider 之前运行，
//     未签名的非匿名 UserID 头在此被剥离（E2E-P0 G3：未认证调用方不得冒领
//     任意身份读取/污染对应 userID 域的运行时状态）；签名匹配则保留头，
//     由上游按原值采信。
//   - client 端：trpc-a2a-go client.Middleware 在出站请求上补
//     "<userIDHeader>-Signature" 头（hex(HMAC-SHA256(secret, userID))）。
//
// 匿名命名空间判定逻辑复刻自上游 server/a2a/server_option.go（未导出），
// 语义保持一致：A2A_ANONYMOUS_<hex(sha256(scopeURL))>_<hex(16B rand)>。

const (
	anonymousUserIDPrefix         = "A2A_ANONYMOUS_"
	anonymousUserIDScopeSeparator = "_"
)

// SignUserIDForHeader computes hex(HMAC-SHA256(secret, userID)).
func SignUserIDForHeader(secret, userID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyUserIDSignature compares a presented signature in constant time.
func VerifyUserIDSignature(secret, userID, presented string) bool {
	if secret == "" || userID == "" || presented == "" {
		return false
	}
	expected := SignUserIDForHeader(secret, userID)
	if len(expected) != len(presented) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}

// UserIDSignatureHeader derives the companion signature header name.
func UserIDSignatureHeader(userIDHeader string) string {
	return strings.TrimSpace(userIDHeader) + "-Signature"
}

// --- server 端：pre-auth 剥离未签名身份 ---

type signedIdentityPreAuthMiddleware struct {
	secret       string
	userIDHeader string
	cookieScope  string
}

// NewSignedIdentityPreAuthMiddleware 返回 trpc-agent-go server/a2a
// WithPreAuthA2AMiddleware 可用的中间件。agentURL 用于推导匿名命名空间的
// cookieScope（与上游 buildA2AServer 同源：anonymousCookieScopeFromAgentURL）。
func NewSignedIdentityPreAuthMiddleware(secret, userIDHeader, agentURL string) a2aserver.Middleware {
	return &signedIdentityPreAuthMiddleware{
		secret:       strings.TrimSpace(secret),
		userIDHeader: strings.TrimSpace(userIDHeader),
		cookieScope:  anonymousCookieScopeFromAgentURL(agentURL),
	}
}

func (m *signedIdentityPreAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.Header.Get(m.userIDHeader))
		if userID != "" && !isAnonymousUserIDForScope(userID, m.cookieScope) {
			if !VerifyUserIDSignature(m.secret, userID,
				strings.TrimSpace(r.Header.Get(UserIDSignatureHeader(m.userIDHeader)))) {
				// 未签名的非匿名身份头：剥离，让上游回落到匿名主体。
				r.Header.Del(m.userIDHeader)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// --- client 端：出站加签 ---

type signedIdentityClientMiddleware struct {
	secret       string
	userIDHeader string
}

// NewSignedIdentityClientMiddleware 返回 trpc-a2a-go client.WithMiddleware
// 可用的中间件：出站请求携带 userIDHeader 时补签 "<header>-Signature"。
// secret 为空时不加签（与 P0 行为一致）。
func NewSignedIdentityClientMiddleware(secret, userIDHeader string) a2aclient.Middleware {
	return &signedIdentityClientMiddleware{
		secret:       strings.TrimSpace(secret),
		userIDHeader: strings.TrimSpace(userIDHeader),
	}
}

func (m *signedIdentityClientMiddleware) Wrap(next a2aclient.HTTPReqHandler) a2aclient.HTTPReqHandler {
	return httpReqHandlerFunc(
		func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
			if m.secret != "" {
				if userID := strings.TrimSpace(req.Header.Get(m.userIDHeader)); userID != "" {
					req.Header.Set(UserIDSignatureHeader(m.userIDHeader),
						SignUserIDForHeader(m.secret, userID))
				}
			}
			return next.Handle(ctx, client, req)
		})
}

// httpReqHandlerFunc 是 a2aclient.HTTPReqHandler 的函数适配器
// （上游 client 包未导出等价类型）。
type httpReqHandlerFunc func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error)

func (f httpReqHandlerFunc) Handle(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	return f(ctx, client, req)
}

// --- 匿名命名空间判定（复刻上游未导出逻辑） ---

func anonymousCookieScopeFromAgentURL(agentURL string) string {
	normalized := strings.TrimSpace(agentURL)
	parsed, err := url.Parse(normalized)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Path = anonymousCookiePathForBasePath(parsed.Path)
		parsed.RawPath = ""
		parsed.RawQuery = ""
		parsed.Fragment = ""
		normalized = parsed.String()
	} else {
		normalized = strings.TrimRight(normalized, "/")
	}
	scope := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(scope[:])
}

func anonymousCookiePathForBasePath(basePath string) string {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" || basePath == "/" {
		return "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return strings.TrimRight(basePath, "/")
}

func isAnonymousUserID(userID string) bool {
	if !strings.HasPrefix(userID, anonymousUserIDPrefix) {
		return false
	}
	encoded := strings.TrimPrefix(userID, anonymousUserIDPrefix)
	if decoded, err := hex.DecodeString(encoded); err == nil && len(decoded) == 16 {
		return true
	}
	parts := strings.Split(encoded, anonymousUserIDScopeSeparator)
	if len(parts) != 2 {
		return false
	}
	scope, scopeErr := hex.DecodeString(parts[0])
	principal, principalErr := hex.DecodeString(parts[1])
	return scopeErr == nil && len(scope) == sha256.Size &&
		principalErr == nil && len(principal) == 16
}

func isAnonymousUserIDForScope(userID, cookieScope string) bool {
	if strings.TrimSpace(cookieScope) == "" {
		return isAnonymousUserID(userID)
	}
	if !isAnonymousUserID(userID) {
		return false
	}
	encoded := strings.TrimPrefix(userID, anonymousUserIDPrefix)
	parts := strings.Split(encoded, anonymousUserIDScopeSeparator)
	return len(parts) == 2 && strings.EqualFold(parts[0], cookieScope)
}
