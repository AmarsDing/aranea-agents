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
	// ForceReset（P1 补漏·强制改密门）：签发时刻 admins.force_reset 的快照。
	// 改密成功即递增 tv 吊销全部会话，旧 token（含本 claim）随之失效，
	// 不存在「claim 已清但会话未吊销」的漂移窗口——claim 快照因此可信。
	ForceReset bool `json:"fr,omitempty"`
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

// effectiveRoles 返回权限判定用角色集（评审 2026-11-09 H5）。
//
// 升级前签发的 token 无 roles claim（json omitempty），灰度窗口内按遗留
// access 列回落映射（与 Login 的 LegacyRolesForAccess 行为一致），避免老
// 会话对全部已登记 rpc 意外 403。P1-9 迁移回填 roles、老 token 到期后，
// 本回落自然失效。
//
// 无复活风险：登录后签发的 token 恒带 roles（Login 已回落），故运行时
// len(Roles)==0 ⟺ 老 token；角色被清零用户的旧 token 在 tv 递增后期满
// 失效，与本回落无关（角色变更不递增 tv 是已登记 LOW-4，迁移脚本负责 bump）。
func (a *Auth) effectiveRoles() []string {
	if len(a.Roles) > 0 {
		return a.Roles
	}
	return LegacyRolesForAccess(a.Access)
}

// HasPermission reports whether the principal's roles grant the permission code,
// honouring `*` and `<域>.*` wildcards (P0 详设 01 §3)。角色→权限解析走
// 当前 PermissionResolver（默认内置静态表，P1-5/6 注入 DB 缓存版）；
// 老 token（无 roles claim）经 effectiveRoles 回落遗留 access 映射。
func (a *Auth) HasPermission(code string) bool {
	if a == nil {
		return false
	}
	return permissionSetContains(resolvePermissions(a.effectiveRoles()), code)
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
