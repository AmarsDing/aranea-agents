// Package middleware contains Kratos middleware adapters used by the HTTP server.
package middleware

import (
	"net/http"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"

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

// WorkspaceFilter is an HTTP filter (pre-handler) that extracts the
// workspace ID from the request and injects it into the context.
//
// Resolution order:
//  1. X-Workspace-ID header
//  2. workspace_id query parameter
//  3. "default" workspace (single-tenant fallback)
//
// If an empty string is provided via header/query we treat it as absent.
// Inject this filter AFTER the auth filter so that auth has already
// validated the bearer token.
func WorkspaceFilter() kratoshttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wsID := r.Header.Get(HeaderWorkspaceID)
			if wsID == "" {
				wsID = r.URL.Query().Get(QueryWorkspaceID)
			}
			if wsID == "" {
				wsID = workspace.DefaultWorkspaceID
			}
			ctx := workspace.WithContext(r.Context(), wsID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AssertWorkspace is a helper for service-layer validation of workspace access.
// Returns apierror.Forbidden when the caller's workspace does not match the
// resource workspace. Pass as a guard at the top of service methods.
// System workspace (WithSystemWorkspace) bypasses the check.
func AssertWorkspace(ctxWorkspaceID, resourceWorkspaceID string) error {
	if ctxWorkspaceID == workspace.SystemWorkspaceID {
		return nil
	}
	if resourceWorkspaceID == "" || ctxWorkspaceID == resourceWorkspaceID {
		return nil
	}
	return apierror.Forbidden("workspace", "access to resource in another workspace is not allowed")
}
