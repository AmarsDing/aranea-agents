package knowledge

import (
	"context"
	"strings"
)

// ── G-08 部门级可见性：调用方组织可见域（org scope）ctx 传播 ─────────────────
//
// scope = 调用方（专项 Agent）可见的 org 节点 ID 集合：{本岗位 ID, 本部门 ID}；
// 团队 Run 为全体成员岗位/部门的并集（借调成员带入其部门读取权，对齐 M78）。
// 由 runCtx 装配期一次性解析注入（ADR-KN-20261020-F），读侧只读此键：
//   - 数据层 docVisibilityPredicate 生成 `shared_with && $n` 腿；
//   - 本包 DocumentVisibleTo 做 shared 档 Go 侧判定。
// 人类用户无 org 绑定——ctx 无此键时 shared 档仅属主可见（fail-closed）。

type orgScopeKey struct{}

// WithOrgScope 注入调用方组织可见域。空/全空白列表不写入（保持原 ctx）。
// 列表在注入时去重去空白，读侧拿到的即规范化形态。
func WithOrgScope(ctx context.Context, ids []string) context.Context {
	filtered := normalizeOrgScope(ids)
	if len(filtered) == 0 {
		return ctx
	}
	return context.WithValue(ctx, orgScopeKey{}, filtered)
}

// OrgScopeFromContext 读取调用方组织可见域；未注入时返回 nil。
func OrgScopeFromContext(ctx context.Context) []string {
	ids, _ := ctx.Value(orgScopeKey{}).([]string)
	return ids
}

// OrgScopeIntersects 报告 scope 与共享名单存在交集（visibility=shared 命中条件）。
func OrgScopeIntersects(scope, sharedWith []string) bool {
	if len(scope) == 0 || len(sharedWith) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(sharedWith))
	for _, id := range sharedWith {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = struct{}{}
		}
	}
	for _, id := range scope {
		if _, ok := set[strings.TrimSpace(id)]; ok {
			return true
		}
	}
	return false
}

func normalizeOrgScope(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
