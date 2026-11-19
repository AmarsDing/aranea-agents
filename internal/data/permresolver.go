package data

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// 本文件实现三项目整合 P1 的 DB 版角色→权限解析器（P0 详设 01 §3;P1 任务
// 分解 §4 待确认项「未接线的 DB 版 PermissionResolver」收口）。
//
// 语义:
//   - DB 权威:角色在 roles 表中存在即以 DB 为准——包括被剥成零权限边的角色
//     (不会被内置映射"复活");自定义角色与种子角色的运行时增删改经周期刷新生效;
//   - 内置兜底:角色不在库中(种子未跑/代码新增内置角色未播种)时回退内置静态
//     映射——保证 bootstrap 期(种子 SQL 未执行、roles 表为空)superadmin 等内置
//     角色可用,不会出现全员锁死;
//   - 失败保旧:刷新失败保留上一份缓存(last-good),启动期首刷失败则从空
//     缓存起步(效果=纯内置兜底,fail-usable 而非 fail-open:DB 收窄的授权
//     不会因缓存缺失而放大)。
//
// 漂移防护:P1-9 verify 的 RBAC 比对(种子 SQL vs BuiltinRolePermissions)
// 在迁移期防漂移;运行期以 DB 为准,内置表只兜未入库角色。
//
// 表很小(种子:9 角色/34 码/40 边),全量重载 + 原子换缓存即可,不做增量。

// RolePermissionLoader 加载一次全量 角色码→权限码 映射(测试可注入假实现)。
// 契约:必须返回 roles 表中的**全部**角色码——无权限边的角色映射到空切片,
// 以维持"在库即 DB 权威"语义(见 PermissionsForRoles)。
type RolePermissionLoader interface {
	LoadRolePermissions(ctx context.Context) (map[string][]string, error)
}

// RolePermissionResolver 实现 auth.PermissionResolver(进程级注入
// auth.SetPermissionResolver)。并发安全,PermissionsForRoles 只读缓存不阻塞。
type RolePermissionResolver struct {
	loader  RolePermissionLoader
	lg      loggateway.Logger
	builtin map[string][]string // 内置角色静态映射快照(未入库角色的兜底)

	mu    sync.RWMutex
	cache map[string]map[string]struct{} // 角色码 → 权限码集合
}

// NewRolePermissionResolver 构造解析器(不触发加载;首刷由 Refresh 显式调用,
// 成败由调用方决定启动语义)。
func NewRolePermissionResolver(loader RolePermissionLoader, lg loggateway.Logger) *RolePermissionResolver {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &RolePermissionResolver{
		loader:  loader,
		lg:      lg,
		builtin: auth.BuiltinRolePermissions(),
		cache:   map[string]map[string]struct{}{},
	}
}

var _ auth.PermissionResolver = (*RolePermissionResolver)(nil)

// Refresh 全量重载并原子换缓存;失败保留上一份缓存并返回错误。
func (r *RolePermissionResolver) Refresh(ctx context.Context) error {
	m, err := r.loader.LoadRolePermissions(ctx)
	if err != nil {
		r.lg.Warn("permission resolver refresh failed (keeping last-good cache)",
			loggateway.StepID("auth.perm_resolver_refresh"), loggateway.Err(err))
		return err
	}
	next := make(map[string]map[string]struct{}, len(m))
	for role, codes := range m {
		set := make(map[string]struct{}, len(codes))
		for _, c := range codes {
			set[c] = struct{}{}
		}
		next[role] = set
	}
	r.mu.Lock()
	r.cache = next
	r.mu.Unlock()
	r.lg.Info("permission resolver cache refreshed",
		loggateway.StepID("auth.perm_resolver_refresh"), loggateway.Int("roles", len(next)))
	return nil
}

// PermissionsForRoles 实现 auth.PermissionResolver:DB 命中角色取 DB 授权,
// 未入库角色回退内置静态映射(bootstrap 兜底)。
func (r *RolePermissionResolver) PermissionsForRoles(roles []string) map[string]struct{} {
	r.mu.RLock()
	cache := r.cache
	r.mu.RUnlock()
	out := make(map[string]struct{})
	for _, role := range roles {
		if set, ok := cache[role]; ok {
			for c := range set {
				out[c] = struct{}{}
			}
			continue
		}
		for _, c := range r.builtin[role] {
			out[c] = struct{}{}
		}
	}
	return out
}

// RunRefreshLoop 周期刷新(默认间隔由调用方定);ctx 取消即退出。
// 刷新失败不致命:Refresh 内已保旧缓存。
func (r *RolePermissionResolver) RunRefreshLoop(ctx context.Context, interval time.Duration) {
	tk := time.NewTicker(interval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			_ = r.Refresh(ctx)
		}
	}
}

// ---------- ent 加载实现 ----------

// entRolePermissionLoader 经 ent 客户端读 roles/permissions/role_permissions
// 三小表全量装配映射(扁平 schema 无边,按 id 人工 join)。
type entRolePermissionLoader struct{ data *Data }

// NewRolePermissionLoader 返回生产加载器(组合根装配用)。
func NewRolePermissionLoader(d *Data) RolePermissionLoader {
	return &entRolePermissionLoader{d}
}

func (l *entRolePermissionLoader) LoadRolePermissions(ctx context.Context) (map[string][]string, error) {
	client := l.data.RW().Read(ctx)
	roles, err := client.Role.Query().All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AUTH")
	}
	perms, err := client.Permission.Query().All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AUTH")
	}
	edges, err := client.RolePermission.Query().All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AUTH")
	}
	roleCode := make(map[int64]string, len(roles))
	out := make(map[string][]string, len(roles))
	for _, role := range roles {
		roleCode[role.ID] = role.Code
		out[role.Code] = nil // 在库即 DB 权威:零权限边角色也必须占位,防内置映射复活
	}
	permCode := make(map[int64]string, len(perms))
	for _, p := range perms {
		permCode[p.ID] = p.Code
	}
	for _, e := range edges {
		rc, ok := roleCode[e.RoleID]
		if !ok {
			continue // 悬挂边(角色已删):跳过不阻断
		}
		pc, ok := permCode[e.PermissionID]
		if !ok {
			continue // 悬挂边(权限已删):跳过不阻断
		}
		out[rc] = append(out[rc], pc)
	}
	return out, nil
}
