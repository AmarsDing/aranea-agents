package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// DefaultWorkspaceID is stamped into JWTs when no membership workspace is set.
// Kept in pkg/auth (not internal/workspace) so token issuance has no reverse dep.
const DefaultWorkspaceID = "default"

// Auth user auth.
type Auth struct {
	UserID      int64  `json:"id"`
	Access      string `json:"access"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	jwt.RegisteredClaims
}

// HasAdminAccess checks if the user has admin access.
func (a *Auth) HasAdminAccess() bool {
	return a.Access == "admin"
}

// EffectiveWorkspaceID returns the principal's bound workspace, or DefaultWorkspaceID.
func (a *Auth) EffectiveWorkspaceID() string {
	if a == nil || a.WorkspaceID == "" {
		return DefaultWorkspaceID
	}
	return a.WorkspaceID
}

type authKey struct{}

// NewContext returns a new Context that carries value.
func NewContext(ctx context.Context, auth *Auth) context.Context {
	return context.WithValue(ctx, authKey{}, auth)
}

// FromContext returns the Auth value stored in ctx, if any.
func FromContext(ctx context.Context) (auth *Auth, ok bool) {
	auth, ok = ctx.Value(authKey{}).(*Auth)
	return
}
