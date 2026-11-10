package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// secops 密级标签 + 模型策略 + 会话/运行水位线台账（E 阶段 W2b D6，
// DDL 迁移 20261327）。查询走 RWDB().ReadDB/WriteDB 返回的 execer
//（事务感知；单行读 QueryContext + LIMIT 1，与 secops_offboard_repo
// 同例）；upsert 用 ON CONFLICT ... DO UPDATE（PG/SQLite 双方言可用）；
// 水位线只升不降写路径 GREATEST（Dialect.Greatest，PG 先例 dialect.go）。
// ============================================================================

type secopsClassificationRepo struct {
	data *Data
}

var (
	_ biz.SecopsClassificationRepo   = (*secopsClassificationRepo)(nil)
	_ biz.SecopsModelPolicyRepo      = (*secopsClassificationRepo)(nil)
	_ biz.SecopsSessionWaterlineRepo = (*secopsClassificationRepo)(nil)
)

// NewSecopsClassificationRepo 装配密级标签台账。
func NewSecopsClassificationRepo(d *Data) biz.SecopsClassificationRepo {
	if d == nil {
		return nil
	}
	return &secopsClassificationRepo{data: d}
}

// NewSecopsModelPolicyRepo 装配模型策略台账。
func NewSecopsModelPolicyRepo(d *Data) biz.SecopsModelPolicyRepo {
	if d == nil {
		return nil
	}
	return &secopsClassificationRepo{data: d}
}

// NewSecopsSessionWaterlineRepo 装配会话/运行水位线台账。
func NewSecopsSessionWaterlineRepo(d *Data) biz.SecopsSessionWaterlineRepo {
	if d == nil {
		return nil
	}
	return &secopsClassificationRepo{data: d}
}

// ── 密级标签 ─────────────────────────────────────────────────────────

func (r *secopsClassificationRepo) ListLabels(ctx context.Context) ([]biz.ClassificationLabel, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT level, name, description, updated_at FROM classification_labels ORDER BY level ASC`)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	out := make([]biz.ClassificationLabel, 0)
	for rows.Next() {
		var label biz.ClassificationLabel
		var updatedAt string
		if err := rows.Scan(&label.Level, &label.Name, &label.Description, &updatedAt); err != nil {
			return nil, entErrToBizErr(err, "SECOPS")
		}
		label.UpdatedAt = parseRFC3339Time(updatedAt)
		out = append(out, label)
	}
	return out, entErrToBizErr(rows.Err(), "SECOPS")
}

// PutLabel upsert：ON CONFLICT (level) DO UPDATE（种子行 L1-L5 已存在，
// 改名/改描述走 UPDATE 分支；updated_at 写即刷新）。
func (r *secopsClassificationRepo) PutLabel(ctx context.Context, label biz.ClassificationLabel) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	now := nowRFC3339()
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO classification_labels (level, name, description, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (level) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, updated_at = EXCLUDED.updated_at`),
		label.Level, label.Name, label.Description, now)
	return entErrToBizErr(err, "SECOPS")
}

// ── 模型策略 ─────────────────────────────────────────────────────────

// GetPolicy 读单行 id=1；表空（迁移未跑/被清）→ (nil, nil)，biz 层补默认。
func (r *secopsClassificationRepo) GetPolicy(ctx context.Context) (*biz.SecopsModelPolicy, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT local_threshold, updated_by, updated_at FROM model_policy WHERE id = 1 LIMIT 1`)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, entErrToBizErr(err, "SECOPS")
		}
		return nil, nil
	}
	var policy biz.SecopsModelPolicy
	var updatedAt string
	if err := rows.Scan(&policy.LocalThreshold, &policy.UpdatedBy, &updatedAt); err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	policy.UpdatedAt = parseRFC3339Time(updatedAt)
	return &policy, nil
}

// SetPolicy upsert 单行 id=1（当前唯一写口 = SetModelPolicy RPC）。
func (r *secopsClassificationRepo) SetPolicy(ctx context.Context, threshold int, updatedBy string) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO model_policy (id, local_threshold, updated_by, updated_at)
			VALUES (1, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET local_threshold = EXCLUDED.local_threshold, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`),
		threshold, updatedBy, nowRFC3339())
	return entErrToBizErr(err, "SECOPS")
}

// ── 会话/运行水位线（GREATEST 只升不降）──────────────────────────────

// GetSessionCeiling 读会话水位线；无行（会话不存在/未迁移）→ 0, nil
// （未定级语义；显式 Raise 对不存在会话仍报 NotFound，二者口径有别）。
func (r *secopsClassificationRepo) GetSessionCeiling(ctx context.Context, sessionID string) (int, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT ceiling FROM sessions WHERE id = ? LIMIT 1`),
		sessionID)
	if err != nil {
		return 0, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, entErrToBizErr(err, "SECOPS")
		}
		return 0, nil
	}
	var ceiling int
	if err := rows.Scan(&ceiling); err != nil {
		return 0, entErrToBizErr(err, "SECOPS")
	}
	return ceiling, nil
}

// RaiseSessionCeiling GREATEST(ceiling, level) 只升不降；UPDATE 命中 0 行
// → 会话不存在 → NotFound（不隐式建行——会话生命周期归 Ent 管）。
func (r *secopsClassificationRepo) RaiseSessionCeiling(ctx context.Context, sessionID string, level int) (int, error) {
	return r.raiseCeiling(ctx, "sessions", sessionID, level)
}

func (r *secopsClassificationRepo) RaiseRunCeiling(ctx context.Context, runID string, level int) (int, error) {
	return r.raiseCeiling(ctx, "session_runs", runID, level)
}

func (r *secopsClassificationRepo) raiseCeiling(ctx context.Context, table, id string, level int) (int, error) {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return 0, apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	// GREATEST 片段内占位符随整句 RenumberPlaceholders 顺序编号，
	// 参数序 = (level, id) —— 与 SQL 文本中 ? 出现序一致。
	stmt := fmt.Sprintf(`UPDATE %s SET ceiling = %s WHERE id = ?`,
		table, r.data.Dialect().Greatest("ceiling", "?"))
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(stmt), level, id)
	if err != nil {
		return 0, entErrToBizErr(err, "SECOPS")
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, entErrToBizErr(err, "SECOPS")
	}
	if affected == 0 {
		return 0, apierror.NotFound(apierror.DomainAdmin, "%s %q not found", table, id)
	}
	// 写后读回最终值（并发合并下 GREATEST 结果以库内为准）。
	switch table {
	case "sessions":
		return r.GetSessionCeiling(ctx, id)
	default:
		rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
			r.data.Dialect().RenumberPlaceholders(`SELECT ceiling FROM session_runs WHERE id = ? LIMIT 1`), id)
		if err != nil {
			return 0, entErrToBizErr(err, "SECOPS")
		}
		defer rows.Close()
		var ceiling int
		if rows.Next() {
			if err := rows.Scan(&ceiling); err != nil {
				return 0, entErrToBizErr(err, "SECOPS")
			}
		}
		return ceiling, entErrToBizErr(rows.Err(), "SECOPS")
	}
}
