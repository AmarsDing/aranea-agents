package data

import (
	"context"

	bridge "aranea-agents/internal/biz/agentbridge"
	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// coding_task_event_repo.go —— 03 方案 P0 ④（统一事件模型）：
// bridge.TaskEventRepo 的原始 SQL 实现。coding_task_events 表不经 ent
// Schema.Create（迁移 20261308_coding_workbench.sql 仅建表），结构镜像
// board_run_lease_repo.go 先例（TEXT 时间列、RenumberPlaceholders、nil-data 安全壳）。
//
// 事件由 service 层进度处理器在 ACP session/update 到达时追加（seq 为任务内
// 单调序号），前端以 after_seq 游标增量拉取实现回放/刷新后回看（借鉴
// vibe-kanban MsgStore 服务端回放语义，传输用轮询游标而非其 JSON Patch WS）。

type codingTaskEventRepo struct {
	data *Data
}

var _ bridge.TaskEventRepo = (*codingTaskEventRepo)(nil)

// NewCodingTaskEventRepo nil-data 安全壳：读写一律 apierror Internal。
func NewCodingTaskEventRepo(d *Data) bridge.TaskEventRepo {
	return &codingTaskEventRepo{data: d}
}

// eventTextLimit 单条事件详情上限（防止超长输出撑爆表；摘要语义非全文转录，
// 与 acpSummaryLimit 同一思路）。
const eventTextLimit = 4096

func (r *codingTaskEventRepo) Append(ctx context.Context, ev *bridge.CodingTaskEvent) error {
	if r == nil || r.data == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	if ev == nil || ev.TaskID == "" {
		return apierror.BadRequest(apierror.DomainAgentBridge, "task_id is required")
	}
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	id := ev.ID
	if id == "" {
		id = "codingevent_" + uuid.NewString()
	}
	text := ev.Text
	if len(text) > eventTextLimit {
		text = text[:eventTextLimit]
	}
	createdAt := ev.CreatedAt
	if createdAt == "" {
		createdAt = nowRFC3339()
	}
	_, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO coding_task_events (id, task_id, seq, kind, title, text, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		id, ev.TaskID, ev.Seq, ev.Kind, ev.Title, text, createdAt)
	return err
}

func (r *codingTaskEventRepo) List(ctx context.Context, taskID string, afterSeq int64, limit int) ([]*bridge.CodingTaskEvent, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "task_id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, task_id, seq, kind, title, text, created_at FROM coding_task_events
		 WHERE task_id=? AND seq>? ORDER BY seq ASC LIMIT ?`), taskID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*bridge.CodingTaskEvent, 0)
	for rows.Next() {
		var ev bridge.CodingTaskEvent
		if err := rows.Scan(&ev.ID, &ev.TaskID, &ev.Seq, &ev.Kind, &ev.Title, &ev.Text, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &ev)
	}
	return out, rows.Err()
}
