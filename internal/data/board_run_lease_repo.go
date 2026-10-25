package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// board_run_lease_repo.go —— P0-6 阶段 1a（90-multi-replica-board-lease）：
// biz.BoardRunLeaseRepo 的 PG 原始 SQL 实现，结构镜像 channel_runtime_lease.go
// 先例（TEXT 时间列 RFC3339Nano、RenumberPlaceholders、apierror 稳定模块码）。
//
// 与 channel_runtime_lease 收养语义的唯一差异：TryClaim 仅允许**过期行收养**
//（WHERE expires_at<=now，不含同 owner 分支）——同进程二次 startBoardDAG
// 必须 claimed=false（C-20 单进程幂等底线的跨进程对齐）。
// 测试须用 ensureBoardRunLeaseTable 镜像 DDL（raw-SQL 表不经 ent Schema.Create；
// 迁移文件 20261295_board_run_leases.sql 改动必须同步镜像，列序钉死）。

const boardRunLeaseModule = "BOARD_RUN_LEASE"

// boardRunLeaseTimeLayout 定宽布局：纳秒恒定 9 位小数，保证 TEXT 时间列
// 字典序 == 时序。RFC3339Nano 是变宽小数，整秒无小数的行会字典序倒置
//（"…:05Z" > "…:05.9Z" 但时序更早）→ 活跃租约被误收养 / HasActive 漏判
//（对抗评审 #0/#11）。阶段 1a 刚落地且 flag 缺省 off，无存量行需迁移。
const boardRunLeaseTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func formatBoardRunLeaseTime(t time.Time) string {
	return t.UTC().Format(boardRunLeaseTimeLayout)
}

type boardRunLeaseRepo struct {
	data *Data
}

var _ biz.BoardRunLeaseRepo = (*boardRunLeaseRepo)(nil)

// NewBoardRunLeaseRepo nil-data 安全壳：读写一律 apierror Internal。
func NewBoardRunLeaseRepo(d *Data) biz.BoardRunLeaseRepo {
	return &boardRunLeaseRepo{data: d}
}

func (r *boardRunLeaseRepo) TryClaimBoardRunLease(ctx context.Context, lease biz.BoardRunLease, ownerID string, ttl time.Duration) (bool, error) {
	if r == nil || r.data == nil {
		return false, apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	ownerID = strings.TrimSpace(ownerID)
	if lease.BoardID == "" || ownerID == "" || ttl <= 0 {
		return false, apierror.BadRequest(boardRunLeaseModule, "board_id, owner_id and ttl are required")
	}
	now := time.Now().UTC()
	expires := formatBoardRunLeaseTime(now.Add(ttl))
	nowS := formatBoardRunLeaseTime(now)
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return false, apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
INSERT INTO board_run_leases (board_id, session_id, owner_id, expires_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(board_id) DO UPDATE SET
  session_id = excluded.session_id,
  owner_id   = excluded.owner_id,
  expires_at = excluded.expires_at,
  updated_at = excluded.updated_at
WHERE board_run_leases.expires_at <= ?`),
		lease.BoardID, lease.SessionID, ownerID, expires, nowS, nowS, nowS,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *boardRunLeaseRepo) RenewBoardRunLease(ctx context.Context, boardID, ownerID string, ttl time.Duration) (bool, error) {
	if r == nil || r.data == nil {
		return false, apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	boardID = strings.TrimSpace(boardID)
	ownerID = strings.TrimSpace(ownerID)
	if boardID == "" || ownerID == "" || ttl <= 0 {
		return false, apierror.BadRequest(boardRunLeaseModule, "board_id, owner_id and ttl are required")
	}
	now := time.Now().UTC()
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return false, apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
UPDATE board_run_leases
SET expires_at = ?, updated_at = ?
WHERE board_id = ? AND owner_id = ? AND expires_at > ?`),
		formatBoardRunLeaseTime(now.Add(ttl)), formatBoardRunLeaseTime(now),
		boardID, ownerID, formatBoardRunLeaseTime(now),
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *boardRunLeaseRepo) ReleaseBoardRunLease(ctx context.Context, boardID, ownerID string) error {
	if r == nil || r.data == nil {
		return apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	boardID = strings.TrimSpace(boardID)
	ownerID = strings.TrimSpace(ownerID)
	if boardID == "" || ownerID == "" {
		return apierror.BadRequest(boardRunLeaseModule, "board_id and owner_id are required")
	}
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	_, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM board_run_leases WHERE board_id = ? AND owner_id = ?`), boardID, ownerID)
	return err
}

func (r *boardRunLeaseRepo) HasActiveBoardRunForSession(ctx context.Context, sessionID string, now time.Time) (bool, error) {
	if r == nil || r.data == nil {
		return false, apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, nil // 空 session 恒无归属（与 InMem 端口语义对齐）
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return false, apierror.Internal(boardRunLeaseModule, "repository unavailable")
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT 1 FROM board_run_leases WHERE session_id = ? AND expires_at > ? LIMIT 1`),
		sessionID, formatBoardRunLeaseTime(now),
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if rows.Next() {
		return true, nil
	}
	return false, rows.Err()
}
