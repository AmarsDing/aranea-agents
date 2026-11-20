package auth

import (
	"sort"
	"strings"
	"sync"
)

// 本文件实现三项目整合 P1 权限底座的角色→权限解析（P0 详设 01 §2/§3）。
//
// 权限码命名：`<域>.<动作>`；内置角色映射与 P0 详设 01 §3 表保持一致，
// 作为 DB seed 的同步来源。自定义角色（F 期放开增删）经 SetPermissionResolver
// 注入 DB 缓存解析器；未注入时使用 builtinRolePermissions 静态解析。

// PermissionResolver resolves role codes into a flat permission-code set.
// Implementations must be safe for concurrent use and non-blocking (serve from
// cache); the DB-backed resolver is wired during P1-5/P1-6.
type PermissionResolver interface {
	// PermissionsForRoles returns the union of permission codes granted by the
	// given role codes. Unknown roles contribute nothing.
	PermissionsForRoles(roles []string) map[string]struct{}
}

// 内置角色 → 权限码（P0 详设 01 §3；域清单见 §2）。
// 取值含两类通配：`*`（全量，superadmin）与 `<域>.*`（域前缀）。
var builtinRolePermissions = map[string][]string{
	"superadmin": {"*"},
	"sysadmin": {
		"user.read", "user.write", "user.offboard",
		"space.*",
		"system.setting", "system.cron",
		// system.self_improve：自我改进控制台（评审 2026-11-09 A3 补录）——
		// 运维向进化管线（运行审批/风险规则/回滚），与 system.setting/cron 同档；
		// 授予后菜单 /self-improvement 接码可见性与 rpc 判定对齐。
		"system.self_improve",
		"office.*",
		// coding.*：agentbridge 编码桥管理/派发（评审 2026-11-09 H4 补录；
		// 主机命令执行面，默认只给运维总管，工作台用户经 DB 角色按需授予）。
		"coding.*",
		// E 阶段（2026-11-09 需求文档）：凭据管理高敏收窄仅 sysadmin；
		// 审批矩阵写、GitHub watch 管理、发文审批决议同档。
		"credential.manage", "approval.policy", "github.manage",
		"review.view", "review.decide",
		// 2026-11-19：Harness › 连接——通道、出站推送、跨实例协作（A2A + 联邦）的管理面，
		// 此前默认登录即可增删改；与凭据管理同档只给运维总管。
		"channel.manage", "webhook.manage", "a2a.manage",
	},
	"secadmin": {
		"knowledge.classify",
		"maskgate.*",
		"secops.policy",
		"user.clearance",
		"model.route",
	},
	"auditor": {
		"audit.read", "audit.export",
		"usage.read",
	},
	"dataowner": {
		"knowledge.read", "knowledge.write",
		"space.read", "space.write", "space.member",
	},
	"ceo": {
		"ceoboard.*",
		"audit.read",
		"usage.read",
		"approval.review",
		// E 阶段：CEO 总览面延伸——GitHub 情报面板读、发文审批读+决议。
		"github.view", "review.view", "review.decide",
	},
	"oncall": {
		"ceoboard.view", "ceoboard.ops",
		"system.monitor",
	},
	"viewer": {
		"ceoboard.view",
		"system.monitor",
		"github.view",
	},
	"user": {
		"knowledge.read", "knowledge.write",
		"memory.read", "memory.write", "memory.personal",
		"approval.submit",
		"office.schedule", "office.mail", "office.apps",
		"session.*",
		// E 阶段：统一审批中心按身份过滤（reviewer 可裁决自己名下的）；
		// 发文审批与 GitHub 面板是个人工作台日常面。
		"approval.review", "review.view", "review.decide", "github.view",
	},
}

// BuiltinRolePermissions returns a deep copy of the builtin role→permission
// mapping（P0 详设 01 §3）。它是 DB seed 的唯一来源——P1-9 迁移工具用它生成
// roles/permissions/role_permissions 种子 SQL，禁止手工誊抄（防漂移）。
func BuiltinRolePermissions() map[string][]string {
	out := make(map[string][]string, len(builtinRolePermissions))
	for role, codes := range builtinRolePermissions {
		out[role] = append([]string(nil), codes...)
	}
	return out
}

// builtinResolver 是默认解析器：仅含内置角色静态映射。
type builtinResolver struct{}

func (builtinResolver) PermissionsForRoles(roles []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, r := range roles {
		for _, code := range builtinRolePermissions[r] {
			out[code] = struct{}{}
		}
	}
	return out
}

var (
	permResolverMu sync.RWMutex
	permResolver   PermissionResolver = builtinResolver{}
)

// SetPermissionResolver replaces the process-wide role→permission resolver.
// Called once at startup after the DB-backed cache is ready (P1-5/P1-6);
// passing nil restores the builtin static resolver.
func SetPermissionResolver(r PermissionResolver) {
	permResolverMu.Lock()
	defer permResolverMu.Unlock()
	if r == nil {
		permResolver = builtinResolver{}
		return
	}
	permResolver = r
}

// resolvePermissions unions the permission codes for roles via the active resolver.
func resolvePermissions(roles []string) map[string]struct{} {
	permResolverMu.RLock()
	r := permResolver
	permResolverMu.RUnlock()
	return r.PermissionsForRoles(roles)
}

// permissionSetContains reports whether the granted set satisfies code,
// honouring `*` (superadmin) and `<域>.*` prefix wildcards.
func permissionSetContains(granted map[string]struct{}, code string) bool {
	if code == "" {
		return false
	}
	if _, ok := granted["*"]; ok {
		return true
	}
	if _, ok := granted[code]; ok {
		return true
	}
	if i := strings.IndexByte(code, '.'); i > 0 {
		if _, ok := granted[code[:i+1]+"*"]; ok {
			return true
		}
	}
	return false
}

// LegacyRolesForAccess maps the pre-merge `access` column to role codes during
// the transition window (P0 详设 01 §1.1：access=="admin"→superadmin，其余→user）。
// Used when a principal has no roles yet (pre-migration rows); migration P1-9
// backfills roles so this fallback eventually disappears with the access column.
func LegacyRolesForAccess(access string) []string {
	if access == "admin" {
		return []string{"superadmin"}
	}
	return []string{"user"}
}

// PermissionsFor returns the principal's effective permission codes, sorted for
// stable output（P1-7 菜单下发：下发给前端做展示层过滤）。通配授权按原样返回
// （superadmin 为单个 `*`），不做展开——展开会让客户端拿到本不该暴露的域清单，
// 且判定语义（`*` / `<域>.*`）由 permissionSetContains 单点定义。
func PermissionsFor(a *Auth) []string {
	if a == nil {
		return nil
	}
	granted := resolvePermissions(a.effectiveRoles())
	out := make([]string, 0, len(granted))
	for code := range granted {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
