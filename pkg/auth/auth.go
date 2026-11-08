package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// DefaultWorkspaceID is stamped into JWTs when no membership workspace is set.
// Kept in pkg/auth (not internal/workspace) so token issuance has no reverse dep.
const DefaultWorkspaceID = "default"

// Auth user auth.
//
// 三项目整合 P1（P0 详设 01 §5）：claims 扩展为
// `{id, access(过渡), roles[], clearance, workspace_id, tv}`。
// Access 为过渡字段（语义由 Roles 接管，迁移完成后删除）；tv 即
// admins.token_version，用于会话吊销（踢会话/改密/离职/角色密级变更即递增）。
type Auth struct {
	UserID       int64    `json:"id"`
	Access       string   `json:"access"`
	WorkspaceID  string   `json:"workspace_id,omitempty"`
	Roles        []string `json:"roles,omitempty"`
	Clearance    int      `json:"clearance,omitempty"`
	TokenVersion int64    `json:"tv,omitempty"`
	jwt.RegisteredClaims
}

// HasAdminAccess checks if the user has admin access.
//
// Deprecated(transition): 存量调用点在 P1-6 普查收口前保留；新代码用
// HasRole("superadmin") 或 HasPermission(<code>)。
func (a *Auth) HasAdminAccess() bool {
	return a.Access == "admin"
}

// HasRole reports whether the principal carries the given role code.
func (a *Auth) HasRole(code string) bool {
	if a == nil {
		return false
	}
	for _, r := range a.Roles {
		if r == code {
			return true
		}
	}
	return false
}

// HasAnyRole reports whether the principal carries at least one of the given role codes.
func (a *Auth) HasAnyRole(codes ...string) bool {
	for _, c := range codes {
		if a.HasRole(c) {
			return true
		}
	}
	return false
}

// ClearanceLevel returns the principal's 密级资质（L1-L5）。零值（旧 token /
// 未迁移用户）按 L1 处理——最低资质，与 admins.clearance 默认值一致。
func (a *Auth) ClearanceLevel() int {
	if a == nil || a.Clearance < 1 {
		return 1
	}
	if a.Clearance > 5 {
		return 5
	}
	return a.Clearance
}

// HasClearance reports whether the principal's clearance reaches the required level.
func (a *Auth) HasClearance(level int) bool {
	return a.ClearanceLevel() >= level
}

// HasPermission reports whether the principal's roles grant the permission code,
// honouring `*` and `<域>.*` wildcards (P0 详设 01 §3)。角色→权限解析走
// 当前 PermissionResolver（默认内置静态表，P1-5/6 注入 DB 缓存版）。
func (a *Auth) HasPermission(code string) bool {
	if a == nil {
		return false
	}
	return permissionSetContains(resolvePermissions(a.Roles), code)
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
