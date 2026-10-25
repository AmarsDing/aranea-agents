package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// executor_waiter_decision_repo.go —— P0-6 阶段 1b（D6，
// 97-multi-replica-executor-waiters）：biz.ExecutorWaiterDecisionPort 的 PG
// 原始 SQL 实现，结构镜像 board_run_lease_repo.go（RenumberPlaceholders +
// apierror 稳定模块码 + 定宽时间布局复用 formatBoardRunLeaseTime）。
//
// 写语义=INSERT ... ON CONFLICT(kind,session_id,scope_id) DO NOTHING：
// 0 行命中=先到者赢、绝不 UPDATE 翻案（评审确认项）；LoadDecision 单读。
// 测试须用 ensureExecutorWaiterDecisionsTable 镜像 DDL（raw-SQL 表不经
// ent Schema.Create；迁移文件 20261302 改动必须同步镜像）。

const executorWaiterDecisionModule = "EXECUTOR_WAITER_DECISION"

type executorWaiterDecisionRepo struct {
	data *Data
}

var _ biz.ExecutorWaiterDecisionPort = (*executorWaiterDecisionRepo)(nil)

// NewExecutorWaiterDecisionRepo nil-data 安全壳：读写一律 apierror Internal。
func NewExecutorWaiterDecisionRepo(d *Data) biz.ExecutorWaiterDecisionPort {
	return &executorWaiterDecisionRepo{data: d}
}

func (r *executorWaiterDecisionRepo) TryRecordDecision(ctx context.Context, dec biz.ExecutorWaiterDecision) (bool, error) {
	if r == nil || r.data == nil {
		return false, apierror.Internal(executorWaiterDecisionModule, "repository unavailable")
	}
	if err := biz.ValidateExecutorWaiterDecision(dec); err != nil {
		return false, apierror.BadRequest(executorWaiterDecisionModule, err.Error())
	}
	if dec.DecidedAt.IsZero() {
		dec.DecidedAt = time.Now().UTC()
	}
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return false, apierror.Internal(executorWaiterDecisionModule, "repository unavailable")
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
INSERT INTO executor_waiter_decisions (kind, session_id, scope_id, decision, reason, actor, decided_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (kind, session_id, scope_id) DO NOTHING`),
		strings.TrimSpace(dec.Kind), strings.TrimSpace(dec.SessionID), strings.TrimSpace(dec.ScopeID),
		dec.Approved, strings.TrimSpace(dec.Reason), strings.TrimSpace(dec.Actor),
		formatBoardRunLeaseTime(dec.DecidedAt),
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *executorWaiterDecisionRepo) LoadDecision(ctx context.Context, key biz.ExecutorWaiterDecisionKey) (biz.ExecutorWaiterDecision, bool, error) {
	if r == nil || r.data == nil {
		return biz.ExecutorWaiterDecision{}, false, apierror.Internal(executorWaiterDecisionModule, "repository unavailable")
	}
	key.Kind = strings.TrimSpace(key.Kind)
	key.SessionID = strings.TrimSpace(key.SessionID)
	key.ScopeID = strings.TrimSpace(key.ScopeID)
	if key.Kind == "" || key.ScopeID == "" {
		return biz.ExecutorWaiterDecision{}, false, apierror.BadRequest(executorWaiterDecisionModule, "kind and scope_id are required")
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return biz.ExecutorWaiterDecision{}, false, apierror.Internal(executorWaiterDecisionModule, "repository unavailable")
	}
	// execer 面只有 QueryContext（无 QueryRowContext）——行存否由 rows.Next()
	// 判定，镜像 board_run_lease_repo.go 的单行读先例。
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
SELECT decision, reason, actor, decided_at
FROM executor_waiter_decisions
WHERE kind = ? AND session_id = ? AND scope_id = ?`),
		key.Kind, key.SessionID, key.ScopeID,
	)
	if err != nil {
		return biz.ExecutorWaiterDecision{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.ExecutorWaiterDecision{}, false, rows.Err()
	}
	var (
		approved      bool
		reason, actor string
		decidedAtS    string
	)
	if err := rows.Scan(&approved, &reason, &actor, &decidedAtS); err != nil {
		return biz.ExecutorWaiterDecision{}, false, err
	}
	decidedAt, _ := time.ParseInLocation(boardRunLeaseTimeLayout, decidedAtS, time.UTC)
	return biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: key,
		Approved:                  approved,
		Reason:                    reason,
		Actor:                     actor,
		DecidedAt:                 decidedAt,
	}, true, nil
}
