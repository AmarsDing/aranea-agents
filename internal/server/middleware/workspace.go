// Package middleware contains Kratos middleware adapters used by the HTTP server.
package middleware

import (
	"net/http"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

const (
	// HeaderWorkspaceID is the HTTP request header that carries the workspace
	// identifier. Clients may also embed workspace_id in the JWT claims (future).
	HeaderWorkspaceID = "X-Workspace-ID"

	// QueryWorkspaceID is the query parameter fallback (used by SDK clients
	// that cannot set custom headers).
	QueryWorkspaceID = "workspace_id"
)

// WorkspaceFilter is an HTTP filter (pre-handler) that resolves the caller's
// workspace and injects it into the context.
//
// B-01 (2026-07-16): workspace is bound to the authenticated principal's JWT
// membership. Client Header/Query values are NOT trusted for any principal
// (including admin). Admins that need another workspace must hold a JWT
// stamped with that workspace_id (1:1 admin→workspace model / P2-A).
//
// Resolution rules:
//  1. No principal (public/no-auth paths): default workspace (ignore client forge).
//  2. Authenticated principal: always JWT EffectiveWorkspaceID. A mismatched
//     Header/Query is rejected with 403.
//
// Inject this filter AFTER the auth filter so that auth has already
// validated the bearer token.
func WorkspaceFilter() kratoshttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requested := r.Header.Get(HeaderWorkspaceID)
			if requested == "" {
				requested = r.URL.Query().Get(QueryWorkspaceID)
			}

			claims, ok := auth.FromContext(r.Context())
			var wsID string
			switch {
			case !ok || claims == nil:
				// Unauthenticated path: do not honor client-supplied IDs.
				wsID = workspace.DefaultWorkspaceID
			default:
				bound := claims.EffectiveWorkspaceID()
				if requested != "" && requested != bound {
					http.Error(w, "workspace not allowed for this principal", http.StatusForbidden)
					return
				}
				wsID = bound
			}

			ctx := workspace.WithContext(r.Context(), wsID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AssertWorkspace is a backward-compatibility thin wrapper around
// workspace.AssertWorkspace.
//
// 2026-07-15 P1-2: 实现已提升到 workspace 包，让 service 层可调用而无需
// 反向依赖 server/middleware。此函数保留以避免破坏现有调用者（如有），
// 行为与 workspace.AssertWorkspace 完全一致。
//
// 语义变更（P1-2）：空 resourceWorkspaceID 现在被视为 DefaultWorkspaceID
// 而非"任意 caller 都可访问"。这修复了旧实现中遗留数据（无 WorkspaceID）
// 被任意租户访问的 IDOR 风险——legacy 数据现在归属 default workspace。
func AssertWorkspace(ctxWorkspaceID, resourceWorkspaceID string) error {
	return workspace.AssertWorkspace(ctxWorkspaceID, resourceWorkspaceID)
}
