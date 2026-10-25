package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/shared"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/apierror"
)

// approvalPolicyRepo 是 session_approval_policies 表（迁移 20261291，P0-5 审批
// 工程化）的 PG 仓储。实现 biztool.ApprovalPolicyStore（读口 ListEnabled +
// 管理面 List/Get/Upsert/Delete）。写路径复用 biztool.ValidateApprovalPolicy
// 单一校验函数——与 param_rules Upsert 同一编译函数，防两套 pattern 语义
// （评审 §12.2 P0-5①）。列序与 20261291 建表语句逐列对应（13 字段）。
type approvalPolicyRepo struct {
	data *Data
}

var _ biztool.ApprovalPolicyStore = (*approvalPolicyRepo)(nil)

// NewApprovalPolicyRepo 构造审批策略 PG 仓储；Data 为 nil 时返回空实现
// （所有读返回 nil、写报 apierror.Internal——CLI/单测形态不炸装配）。
func NewApprovalPolicyRepo(d *Data) biztool.ApprovalPolicyStore {
	if d == nil {
		return &approvalPolicyRepo{}
	}
	return &approvalPolicyRepo{data: d}
}

// approvalPolicyColumns 是 SELECT 列序的单源（scan 与之对齐）。
const approvalPolicyColumns = `id, scope, scope_key, tool_pattern, command_prefix, args_pattern, mode, priority, enabled, shadow_only, expires_at, created_at, created_by`

func scanApprovalPolicy(row interface{ Scan(dest ...any) error }) (biztool.ApprovalPolicy, error) {
	var p biztool.ApprovalPolicy
	var enabledInt, shadowInt int
	err := row.Scan(&p.ID, &p.Scope, &p.ScopeKey, &p.ToolPattern, &p.CommandPrefix, &p.ArgsPattern,
		&p.Mode, &p.Priority, &enabledInt, &shadowInt, &p.ExpiresAt, &p.CreatedAt, &p.CreatedBy)
	if err != nil {
		return biztool.ApprovalPolicy{}, err
	}
	p.Enabled = enabledInt != 0
	p.ShadowOnly = shadowInt != 0
	return p, nil
}

// approvalPolicyOrderBy 与 InMem 同序：Priority 升序、同档 id 字典序（确定性）。
func (r *approvalPolicyRepo) ListApprovalPolicies(ctx context.Context, scope, scopeKey string) ([]biztool.ApprovalPolicy, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	scope = strings.TrimSpace(scope)
	scopeKey = strings.TrimSpace(scopeKey)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+approvalPolicyColumns+` FROM session_approval_policies WHERE scope = ? AND scope_key = ? ORDER BY priority ASC, id ASC`),
		scope, scopeKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]biztool.ApprovalPolicy, 0, 16)
	for rows.Next() {
		p, err := scanApprovalPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *approvalPolicyRepo) ListEnabledApprovalPolicies(ctx context.Context, scope, scopeKey string, nowUnix int64) ([]biztool.ApprovalPolicy, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	scope = strings.TrimSpace(scope)
	scopeKey = strings.TrimSpace(scopeKey)
	// enabled=1 且未过期（expires_at=0 永不过期）；排序语义与 InMem/管理面一致。
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+approvalPolicyColumns+` FROM session_approval_policies
		 WHERE scope = ? AND scope_key = ? AND enabled <> 0 AND (expires_at = 0 OR expires_at > ?)
		 ORDER BY priority ASC, id ASC`),
		scope, scopeKey, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]biztool.ApprovalPolicy, 0, 16)
	for rows.Next() {
		p, err := scanApprovalPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *approvalPolicyRepo) GetApprovalPolicyByID(ctx context.Context, id string) (biztool.ApprovalPolicy, error) {
	if r == nil || r.data == nil {
		return biztool.ApprovalPolicy{}, shared.ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return biztool.ApprovalPolicy{}, shared.ErrNotFound
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+approvalPolicyColumns+` FROM session_approval_policies WHERE id = ?`), id)
	if err != nil {
		// 对抗评审 #5：真实 DB 错误不可塌缩成 NotFound（404 会掩盖 500 故障，
		// 管理面看不到 DB 挂了）。仅「行不存在」才属 NotFound。
		return biztool.ApprovalPolicy{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biztool.ApprovalPolicy{}, shared.ErrNotFound
	}
	p, err := scanApprovalPolicy(rows)
	if err != nil {
		return biztool.ApprovalPolicy{}, err
	}
	return p, nil
}

func (r *approvalPolicyRepo) UpsertApprovalPolicy(ctx context.Context, p biztool.ApprovalPolicy) error {
	if r == nil || r.data == nil {
		return apierror.Internal("APPROVAL_POLICY", "repository unavailable")
	}
	p.Scope = strings.TrimSpace(p.Scope)
	p.ScopeKey = strings.TrimSpace(p.ScopeKey)
	p.Mode = strings.TrimSpace(p.Mode)
	p.ID = strings.TrimSpace(p.ID)
	p.ToolPattern = strings.TrimSpace(p.ToolPattern)
	if err := biztool.ValidateApprovalPolicy(p); err != nil {
		return err
	}
	// D5-f F6：ON CONFLICT 分支无条件保留既有 created_at/created_by（不取
	// excluded 值）——并发双新建同一 id 时审计元数据归属恒为最先落库者，
	// 不随服务层读-改-写时序翻转。service 层 Get→copy 保留逻辑仍有效
	//（InMem 仓无 ON CONFLICT 语义，更新路径需其承接）。
	enabledInt, shadowInt := 0, 0
	if p.Enabled {
		enabledInt = 1
	}
	if p.ShadowOnly {
		shadowInt = 1
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO session_approval_policies
		 (id, scope, scope_key, tool_pattern, command_prefix, args_pattern, mode, priority, enabled, shadow_only, expires_at, created_at, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   scope = excluded.scope, scope_key = excluded.scope_key,
		   tool_pattern = excluded.tool_pattern, command_prefix = excluded.command_prefix,
		   args_pattern = excluded.args_pattern, mode = excluded.mode, priority = excluded.priority,
		   enabled = excluded.enabled, shadow_only = excluded.shadow_only,
		   expires_at = excluded.expires_at,
		   created_at = session_approval_policies.created_at,
		   created_by = session_approval_policies.created_by`),
		p.ID, p.Scope, p.ScopeKey, p.ToolPattern, p.CommandPrefix, p.ArgsPattern, p.Mode,
		p.Priority, enabledInt, shadowInt, p.ExpiresAt, p.CreatedAt, p.CreatedBy,
	)
	return err
}

func (r *approvalPolicyRepo) DeleteApprovalPolicy(ctx context.Context, id string) error {
	if r == nil || r.data == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM session_approval_policies WHERE id = ?`), id)
	return err
}
