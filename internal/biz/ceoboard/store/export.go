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

// maxAuditExportBytes 导出响应字节预算(评审 2026-11-09 D4):gRPC 默认单消息
// 上限 4MiB,100k 行 CSV(含 ≤1024 字符 detail 列)轻松超限——行数上限管查询
// 成本,字节预算管响应可送达,两者独立钳制。取 3MiB 为 proto 帧与其他字段留
// 余量。触预算即停止并写截断标记行(调用方经 truncated 返回值得知)。
const maxAuditExportBytes = 3 << 20

// auditExportTruncatedMark 是 CSV 尾部的截断标记(单格 # 前缀行;消费方据此
// 识别不完整快照,按提示收窄窗口/动作过滤重试)。
const auditExportTruncatedMark = "# truncated: export byte budget reached; narrow the time window or action filter and retry"

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
// 双钳制(D4):行数 ≤ maxAuditExportRows(查询成本),字节 ≤ maxAuditExportBytes
// (响应可送达,gRPC 4MiB 默认上限内);触字节预算提前停止,尾部写截断标记行,
// 并返回 truncated=true(行数不满 limit 也成立)。
func (s *Store) ExportAudit(ctx context.Context, w io.Writer, filter AuditExportFilter) (rows int, truncated bool, err error) {
	return s.exportAudit(ctx, w, filter, maxAuditExportBytes)
}

// exportAudit 是 ExportAudit 的可参数化实现(maxBytes 供测试用小预算)。
func (s *Store) exportAudit(ctx context.Context, w io.Writer, filter AuditExportFilter, maxBytes int) (int, bool, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = maxAuditExportRows
	}
	if limit > maxAuditExportRows {
		return 0, false, fmt.Errorf("审计导出最多 %d 行", maxAuditExportRows)
	}
	from, to := filter.From.UTC(), filter.To.UTC()
	tx, err := s.client.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, false, fmt.Errorf("开启只读快照: %w", err)
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
		return 0, false, fmt.Errorf("查询审计快照: %w", err)
	}

	cw := csv.NewWriter(w)
	header := []string{"id", "at", "actor", "action", "target", "ok", "detail"}
	if err := cw.Write(header); err != nil {
		return 0, false, fmt.Errorf("写 CSV 表头: %w", err)
	}
	written := estimateCSVRowBytes(header)
	count := 0
	truncated := false
loop:
	for {
		for _, e := range rows {
			record := []string{
				strconv.FormatInt(e.ID, 10), e.At.UTC().Format(time.RFC3339Nano),
				safeCSVCell(e.Actor), safeCSVCell(e.Action), safeCSVCell(e.Target),
				strconv.FormatBool(e.Ok), safeCSVCell(e.Detail),
			}
			est := estimateCSVRowBytes(record)
			if maxBytes > 0 && written+est > maxBytes {
				// D4:触字节预算——写截断标记(装得下才写)后收工;
				// 不硬撑超限行,否则 gRPC 层整体失败、一行都拿不到。
				marker := []string{auditExportTruncatedMark}
				if written+estimateCSVRowBytes(marker) <= maxBytes {
					if err := cw.Write(marker); err != nil {
						return count, true, fmt.Errorf("写截断标记: %w", err)
					}
				}
				truncated = true
				break loop
			}
			if err := cw.Write(record); err != nil {
				return count, false, fmt.Errorf("写 CSV 行: %w", err)
			}
			written += est
			count++
			if count%500 == 0 {
				cw.Flush()
				if err := cw.Error(); err != nil {
					return count, false, fmt.Errorf("刷新 CSV: %w", err)
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
			return count, false, fmt.Errorf("查询审计快照: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return count, truncated, fmt.Errorf("完成 CSV: %w", err)
	}
	return count, truncated, nil
}

// estimateCSVRowBytes 估算一行的序列化字节上界:逗号+换行 + 每字段强制
// 引号(2B)且内容全为需转义引号(2x)。上界估法保证预算判定永不失真超限。
func estimateCSVRowBytes(fields []string) int {
	n := len(fields) + 1
	for _, f := range fields {
		n += 2*len(f) + 2
	}
	return n
}

// safeCSVCell 防止表格软件把审计文本当成公式执行。
func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
