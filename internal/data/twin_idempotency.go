package data

import (
	"context"
	"errors"
	"time"

	"aranea-agents/internal/biz"
)

// twinIdempotencyGCWindow 幂等键保留窗口。语义对齐消费方重试/对账窗口：
// webhook 投递与 run 终态到达以分钟计，对账拍以小时计，7 天足够覆盖任何
// 合理的重试周期；超窗键允许重提（消费方此时应已具备终态，重复提交属
// 新意图而非重试）。
const twinIdempotencyGCWindow = 7 * 24 * time.Hour

// twinIdempotencyRepo 是 biz.TwinIdempotencyStore 的 raw-SQL 实现
// （表 twin_openapi_idempotency，迁移 20261306；双方言：PG + SQLite，
// ON CONFLICT DO NOTHING 两方言均原生支持）。
type twinIdempotencyRepo struct {
	data *Data
}

// NewTwinIdempotencyRepo 幂等键仓储 Provider（对齐 NewMCPServerRepo 直接
// 返回接口的先例）。
func NewTwinIdempotencyRepo(d *Data) biz.TwinIdempotencyStore {
	return &twinIdempotencyRepo{data: d}
}

func (r *twinIdempotencyRepo) LookupRunID(ctx context.Context, caller, key string) (string, error) {
	if r == nil || r.data == nil {
		return "", errors.New("twin idempotency db nil")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(
			`SELECT run_id FROM twin_openapi_idempotency WHERE caller = ? AND idem_key = ?`),
		caller, key)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return "", err
		}
		return runID, nil
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func (r *twinIdempotencyRepo) SaveRunID(ctx context.Context, caller, key, runID, graphID string) error {
	if r == nil || r.data == nil {
		return errors.New("twin idempotency db nil")
	}
	now := time.Now().UTC()
	db := r.data.RWDB().WriteDB(ctx)
	// lazy GC 先行。GC 失败如实返回错误（不静默吞错让表无界增长无人察觉）；
	// 门面把 Save 错误按 warn 降级处理（幂等保护暂失效、主提交语义不受影响）。
	if _, err := db.ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(
			`DELETE FROM twin_openapi_idempotency WHERE created_at < ?`),
		now.Add(-twinIdempotencyGCWindow).Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(
			`INSERT INTO twin_openapi_idempotency (caller, idem_key, run_id, graph_id, created_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (caller, idem_key) DO NOTHING`),
		caller, key, runID, graphID, now.Format(time.RFC3339))
	return err
}
