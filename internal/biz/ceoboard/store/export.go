// 审计 CSV 导出(自 ceo-dashboard/internal/store/export.go 移植)。
// 保真要点:
//   - 单只读事务内导出,REPEATABLE READ 快照:导出开始后提交的数据不进入本次快照;
//   - 行数封顶 10 万;流式写 CSV,每 500 行 flush;
//   - safeCSVCell 防表格软件把审计文本当公式执行。
//
// 移植差异:WAL 只读连接 → Ent 只读事务;单条流式查询 → 事务内 (at,id) 键集分页
// (审计是追加写且 at 取本机时钟,键集游标天然排除新提交行;REPEATABLE READ 进一步保证)。
package store

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/ceoboardaudit"
)

const maxAuditExportRows = 100000

// auditExportPageSize 是导出分页大小(快照由只读事务保证,与分页无关)。
const auditExportPageSize = 2000

// AuditExportFilter 限定审计导出的时间窗、动作与最大行数。
type AuditExportFilter struct {
	From   time.Time
	To     time.Time
	Action string
	Limit  int
}

// auditCursor 是导出键集分页位置(at DESC, id DESC)。
type auditCursor struct {
	at time.Time
	id int64
}

// auditPage 在导出事务内按游标取一页审计。
func auditPage(ctx context.Context, tx *ent.Tx, action string, from, to time.Time, cur *auditCursor, limit int) ([]*ent.CeoboardAudit, error) {
	q := tx.CeoboardAudit.Query().
		Where(ceoboardaudit.AtGTE(from), ceoboardaudit.AtLT(to))
	if action != "" {
		q = q.Where(ceoboardaudit.ActionEQ(action))
	}
	if cur != nil {
		q = q.Where(ceoboardaudit.Or(
			ceoboardaudit.AtLT(cur.at),
			ceoboardaudit.And(ceoboardaudit.AtEQ(cur.at), ceoboardaudit.IDLT(cur.id)),
		))
	}
	return q.
		Order(ent.Desc(ceoboardaudit.FieldAt), ent.Desc(ceoboardaudit.FieldID)).
		Limit(limit).
		All(ctx)
}

// ExportAudit 在单个只读事务中分页导出 CSV;导出开始后提交的数据不进入本次快照。
func (s *Store) ExportAudit(ctx context.Context, w io.Writer, filter AuditExportFilter) (int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = maxAuditExportRows
	}
	if limit > maxAuditExportRows {
		return 0, fmt.Errorf("审计导出最多 %d 行", maxAuditExportRows)
	}
	from, to := filter.From.UTC(), filter.To.UTC()
	tx, err := s.client.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("开启只读快照: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 先取首页再输出:快照建立在任何写入 w 之前——调用方可能在首个 Write 处阻塞,
	// 阻塞期间的新提交不得进入本次导出(PG REPEATABLE READ 快照取自首条语句)。
	pageSize := limit
	if pageSize > auditExportPageSize {
		pageSize = auditExportPageSize
	}
	rows, err := auditPage(ctx, tx, filter.Action, from, to, nil, pageSize)
	if err != nil {
		return 0, fmt.Errorf("查询审计快照: %w", err)
	}

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"id", "at", "actor", "action", "target", "ok", "detail"}); err != nil {
		return 0, fmt.Errorf("写 CSV 表头: %w", err)
	}
	count := 0
	for {
		for _, e := range rows {
			record := []string{
				strconv.FormatInt(e.ID, 10), e.At.UTC().Format(time.RFC3339Nano),
				safeCSVCell(e.Actor), safeCSVCell(e.Action), safeCSVCell(e.Target),
				strconv.FormatBool(e.Ok), safeCSVCell(e.Detail),
			}
			if err := cw.Write(record); err != nil {
				return count, fmt.Errorf("写 CSV 行: %w", err)
			}
			count++
			if count%500 == 0 {
				cw.Flush()
				if err := cw.Error(); err != nil {
					return count, fmt.Errorf("刷新 CSV: %w", err)
				}
			}
		}
		if len(rows) < pageSize || count >= limit {
			break
		}
		last := rows[len(rows)-1]
		pageSize = limit - count
		if pageSize > auditExportPageSize {
			pageSize = auditExportPageSize
		}
		rows, err = auditPage(ctx, tx, filter.Action, from, to, &auditCursor{at: last.At, id: last.ID}, pageSize)
		if err != nil {
			return count, fmt.Errorf("查询审计快照: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return count, fmt.Errorf("完成 CSV: %w", err)
	}
	return count, nil
}

// safeCSVCell 防止表格软件把审计文本当成公式执行。
func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
