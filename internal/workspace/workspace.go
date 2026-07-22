// Package workspace provides the workspace-scoping context key and helpers
// used by the middleware, Ent hooks, and background jobs.
package workspace

import (
	"context"

	"aranea-agents/pkg/apierror"
)

const (
	// SystemWorkspaceID is the sentinel value used by cron / admin tasks
	// that need to bypass per-tenant filtering.
	SystemWorkspaceID = "__system__"

	// DefaultWorkspaceID is the fallback when no workspace header is supplied
	// and the installation has not yet configured multi-tenancy.
	DefaultWorkspaceID = "default"
)

// DomainWorkspace 是 workspace 校验失败的 apierror domain。
const DomainWorkspace = "workspace"

type ctxKey struct{}

// WithContext returns a new context carrying the given workspace ID.
func WithContext(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, workspaceID)
}

// FromContext extracts the workspace ID from ctx.
// Returns ("", false) if the context carries no workspace ID.
func FromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	return v, ok && v != ""
}

// IDFromContext returns the workspace ID or the default if not set.
func IDFromContext(ctx context.Context) string {
	if id, ok := FromContext(ctx); ok {
		return id
	}
	return DefaultWorkspaceID
}

// WithSystemWorkspace returns a context that bypasses workspace filtering.
// Use for cron jobs, admin background tasks, or system-level operations.
func WithSystemWorkspace(ctx context.Context) context.Context {
	return WithContext(ctx, SystemWorkspaceID)
}

// IsSystem reports whether ctx is running under the system workspace bypass.
func IsSystem(ctx context.Context) bool {
	id, ok := FromContext(ctx)
	return ok && id == SystemWorkspaceID
}

// AssertWorkspace 校验 caller workspace 是否拥有 resource workspace。
// P1-1/P1-2: 用于 service 层 IDOR 防护（避免 service → server/middleware 反向依赖）。
//
// 规则：
//   - callerWS == SystemWorkspaceID → 绕过（cron/admin 后台任务）
//   - resourceWS == "" → 视为 DefaultWorkspaceID（历史数据兼容）
//   - callerWS == resourceWS → 允许
//   - 其他 → Forbidden
//
// 2026-07-15 P1-2: 从 middleware.AssertWorkspace 提升到 workspace 包，
// 让 service 层可调用。middleware.AssertWorkspace 保留为 thin wrapper。
//
// 适用场景：tenant-owned 实体（如 admin、cron_task 等私有资源）。
// 对于可共享实体（agent/team/graph/plugin），使用 AssertWorkspaceOrShared。
func AssertWorkspace(callerWS, resourceWS string) error {
	if callerWS == SystemWorkspaceID {
		return nil
	}
	effective := resourceWS
	if effective == "" {
		effective = DefaultWorkspaceID
	}
	if callerWS == effective {
		return nil
	}
	return apierror.Forbidden(DomainWorkspace, "access to resource in another workspace is not allowed")
}

// AssertWorkspaceOrShared 校验 caller workspace 是否可访问 resource workspace，
// 将空 resourceWS 视为"全局共享"而非 default 私有。
//
// 规则：
//   - callerWS == SystemWorkspaceID → 绕过（cron/admin 后台任务）
//   - resourceWS == "" → 视为全局共享，允许任何 caller 访问（legacy/系统内置资源）
//   - callerWS == resourceWS → 允许
//   - 其他 → Forbidden
//
// 适用场景：可共享实体（agent/team/graph_definition/plugin），这些实体可能由
// 系统内置（workspace_id=""）供所有租户使用，也可能是租户私有（workspace_id="ws-xxx"）。
//
// 与 AssertWorkspace 的区别：AssertWorkspace 把空 resourceWS 视为 DefaultWorkspaceID
// （私有，仅 default 租户可访问）；本函数把空 resourceWS 视为全局共享（所有租户可访问）。
func AssertWorkspaceOrShared(callerWS, resourceWS string) error {
	if callerWS == SystemWorkspaceID {
		return nil
	}
	// 空 resourceWS = 全局共享（legacy/系统内置），允许任何 caller 访问。
	if resourceWS == "" {
		return nil
	}
	if callerWS == resourceWS {
		return nil
	}
	return apierror.Forbidden(DomainWorkspace, "access to resource in another workspace is not allowed")
}

// AssertWorkspaceMutate 校验 caller workspace 是否可修改 resource workspace。
// 共享资源（resourceWS=""）对任何非系统 caller 都是只读的（fail-closed）。
//
// 规则：
//   - callerWS == SystemWorkspaceID → 绕过（cron/admin 后台任务可写共享资源）
//   - resourceWS == "" → Forbidden（共享资源禁止 mutate，防止跨租户写入系统内置资源）
//   - callerWS == resourceWS → 允许
//   - 其他 → Forbidden
//
// 适用场景：可共享实体的**写**操作（agent/team/graph_definition/plugin/mcp_server/
// knowledge collection）。与 AssertWorkspaceOrShared（读）成对使用：
// 读允许跨租户访问共享资源，写禁止——防止任何租户篡改系统内置/legacy 共享资源。
//
// 业务依据（internal/service/knowledge.go:173-177）：
// "Shared collections (workspace=”) are read-only for tenants (fail-closed)."
//
// 与 AssertWorkspace / AssertWorkspaceOrShared 的区别：
//   - AssertWorkspace 把空 resourceWS 视为 DefaultWorkspaceID（default 租户可写）
//   - AssertWorkspaceOrShared 把空 resourceWS 视为全局共享（所有租户可读）
//   - 本函数把空 resourceWS 视为禁止写入（所有非系统租户不可写）
func AssertWorkspaceMutate(callerWS, resourceWS string) error {
	if callerWS == SystemWorkspaceID {
		return nil
	}
	// 共享资源对非系统 caller 是只读的——fail-closed 防止跨租户写入。
	if resourceWS == "" {
		return apierror.Forbidden(DomainWorkspace, "cannot mutate shared resource")
	}
	if callerWS == resourceWS {
		return nil
	}
	return apierror.Forbidden(DomainWorkspace, "access to resource in another workspace is not allowed")
}
