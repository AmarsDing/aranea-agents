package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// sessionForkRepo 是 biz.SessionForkStore 的 raw-SQL 实现
// （79-runtime-governance R6 Fork-from-Turn）。
//
// 复制边界（与 biz/session/fork.go 头注释一致）：
//   - 框架事件：trpc_session_events 无 invocation 列，分叉点经
//     event->>'invocationId'（框架 Event 的 JSON key）定位末条事件自增 id，
//     复制 id ≤ boundary 的未删事件前缀——任何 id ≤ boundary 的事件都在分叉
//     turn 完成前落盘（含该 turn 期间 member/sub-agent 调用产生的事件），
//     构成干净的历史前缀。created_at 原文保留 + 按源 id 序插入（新 BIGSERIAL
//     保持同序），框架 GetSession 的 ORDER BY created_at, id 语义不变。
//   - v2 记录：turns_v2.seq 在 task 内单调、tasks_v2.seq 在 session 内单调，
//     边界 = (forkTask.seq, forkTurn.seq)；复制 task.seq < forkTask.seq 的全部，
//     及边界 task 内 turn.seq ≤ forkTurn.seq 的部分。仅复制 session_id = 源会话
//     的行（biz 层已限定根会话）；team 成员 turn（session_id = 成员会话）不在
//     复制面，fork 出的 team 根会话呈现协调者视角历史。
//   - id 重映射：确定性前缀 fk<dst8>-（dst8 = 新会话 uuid 去横线前 8 位）。
//     源 id 最长约 43（step id = turnUUID+"-s"+n），加前缀 ≤ 54 < 64 列上限。
//
// 全部写操作经 ForkSessionInTx 的 ent.Tx（RWDB.WriteDB 从事务 ctx 取
// execer），与 sessions 行的 ent 写入同事务。
type sessionForkRepo struct {
	data *Data
}

// NewSessionForkRepo 返回 biz.SessionForkStore 端口；d 为 nil 时返回 nil
// （service 层据此 503，与 NewSessionForkUsecase 的 nil 约定一致）。
func NewSessionForkRepo(d *Data) biz.SessionForkStore {
	if d == nil {
		return nil
	}
	return &sessionForkRepo{data: d}
}

func (r *sessionForkRepo) ForkSessionInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.data.ExecInTx(ctx, fn)
}

// forkIDPrefix 生成确定性 id 重映射前缀（见类型注释）。
func forkIDPrefix(dstSessionID string) string {
	compact := strings.ReplaceAll(dstSessionID, "-", "")
	if len(compact) > 8 {
		compact = compact[:8]
	}
	return "fk" + compact + "-"
}

func (r *sessionForkRepo) FindTurnEventBoundary(ctx context.Context, sessionID, turnID string) (int64, bool, error) {
	// JSONExtract 双方言：PG COALESCE(...::jsonb)->>'invocationId'；SQLite json_extract。
	invExpr := r.data.Dialect().JSONExtract("event", "invocationId")
	q := r.data.Dialect().RenumberPlaceholders(`
		SELECT COUNT(1), COALESCE(MAX(id), 0) FROM trpc_session_events
		WHERE session_id = ? AND deleted_at IS NULL AND ` + invExpr + ` = ?`)
	var count int
	var boundary int64
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, []any{sessionID, turnID}, &count, &boundary); err != nil {
		return 0, false, err
	}
	return boundary, count > 0, nil
}

func (r *sessionForkRepo) CopyFrameworkEvents(ctx context.Context, srcSessionID, dstSessionID string, boundary int64) (int, error) {
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		INSERT INTO trpc_session_events
			(app_name, user_id, session_id, event, created_at, updated_at, expires_at, deleted_at)
		SELECT app_name, user_id, ?, event, created_at, updated_at, expires_at, deleted_at
		FROM trpc_session_events
		WHERE session_id = ? AND id <= ? AND deleted_at IS NULL
		ORDER BY id`), dstSessionID, srcSessionID, boundary)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(n), nil
}

// forkSessionStateRow 与框架 postgres.SessionState 的 JSON 形态一致
// （{"id","state","createdAt","updatedAt"}）。state 必须为非 null 合法 JSON——
// 框架 getSession 对 state 列直接 json.Unmarshal，NULL 会报
// "unexpected end of JSON input"（vendored service_helper.go L42-50）。
type forkSessionStateRow struct {
	ID        string                     `json:"id"`
	State     map[string]json.RawMessage `json:"state"`
	CreatedAt time.Time                  `json:"createdAt"`
	UpdatedAt time.Time                  `json:"updatedAt"`
}

func (r *sessionForkRepo) CreateFrameworkState(ctx context.Context, srcSessionID, dstSessionID string) error {
	read := r.data.RWDB().ReadDB(ctx)
	var appName, userID string
	// app/user 键与框架写入一致：优先源 state 行，缺失（异常残留）回退事件首行。
	err := queryRowScan(ctx, read, r.data.Dialect().RenumberPlaceholders(`
		SELECT app_name, user_id FROM trpc_session_states
		WHERE session_id = ? AND deleted_at IS NULL LIMIT 1`), []any{srcSessionID}, &appName, &userID)
	if err != nil {
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			return err
		}
		if err = queryRowScan(ctx, read, r.data.Dialect().RenumberPlaceholders(`
			SELECT app_name, user_id FROM trpc_session_events
			WHERE session_id = ? AND deleted_at IS NULL ORDER BY id LIMIT 1`), []any{srcSessionID}, &appName, &userID); err != nil {
			return err
		}
	}
	now := time.Now()
	stateJSON, err := json.Marshal(forkSessionStateRow{
		ID: dstSessionID, State: map[string]json.RawMessage{}, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		INSERT INTO trpc_session_states (app_name, user_id, session_id, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`), appName, userID, dstSessionID, string(stateJSON), now, now)
	return err
}

func (r *sessionForkRepo) CopyV2Records(ctx context.Context, srcSessionID, dstSessionID, forkTurnID string) (int, int, int, error) {
	read := r.data.RWDB().ReadDB(ctx)
	write := r.data.RWDB().WriteDB(ctx)
	d := r.data.Dialect()

	// 边界定位：fork turn → (task seq, turn seq)。v2 中查无此 turn 说明 UI 记录
	// 边界无法建立（v2 写入缺失），拒绝复制而非静默产出空历史。
	// 运行中 turn 拒绝分叉（与前端 forkable 条件一致）：复制会把 status='running'
	// 的 turn 带进新会话，而 fork 会话没有任何 runner 会再写这些复制行——该
	// turn 将永远转圈；且 boundary 之后继续落盘的同 turn 事件会使 fork 历史
	// 成为截断的进行中片段。
	var forkTaskID, forkTurnStatus string
	var forkTurnSeq int64
	if err := queryRowScan(ctx, read, d.RenumberPlaceholders(`
		SELECT task_id, seq, status FROM turns_v2 WHERE id = ? AND session_id = ?`),
		[]any{forkTurnID, srcSessionID}, &forkTaskID, &forkTurnSeq, &forkTurnStatus); err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return 0, 0, 0, apierror.NotFound("SESSION", "turn not found in v2 records of source session")
		}
		return 0, 0, 0, err
	}
	if forkTurnStatus == string(biz.TurnStatusRunning) {
		return 0, 0, 0, apierror.BadRequest("SESSION", "cannot fork from a running turn; wait for it to finish")
	}
	var forkTaskSeq int64
	if err := queryRowScan(ctx, read, d.RenumberPlaceholders(`
		SELECT seq FROM tasks_v2 WHERE id = ? AND session_id = ?`),
		[]any{forkTaskID, srcSessionID}, &forkTaskSeq); err != nil {
		return 0, 0, 0, err
	}

	prefix := forkIDPrefix(dstSessionID)

	// tasks：seq ≤ forkTask.seq 全量复制（边界 task 的后续 turn 在 turns 复制中截断）。
	taskRes, err := write.ExecContext(ctx, d.RenumberPlaceholders(`
		INSERT INTO tasks_v2 (id, session_id, user_message, status, seq, version, workspace_id, created_at, updated_at, completed_at)
		SELECT ? || id, ?, user_message, status, seq, version, workspace_id, created_at, updated_at, completed_at
		FROM tasks_v2
		WHERE session_id = ? AND seq <= ?
		ORDER BY seq`), prefix, dstSessionID, srcSessionID, forkTaskSeq)
	if err != nil {
		return 0, 0, 0, err
	}
	tasks, _ := taskRes.RowsAffected()

	// turns：边界 task 之前全量 + 边界 task 内 seq ≤ forkTurn.seq。
	// JOIN 命中的是源行（复制行 id 带前缀，不会被 t.task_id = k.id 匹配）。
	turnRes, err := write.ExecContext(ctx, d.RenumberPlaceholders(`
		INSERT INTO turns_v2 (id, task_id, session_id, spirit_session_id, parent_turn_id, agent_key, team_id, team_stage_id, seq, version, status, started_at, completed_at)
		SELECT ? || t.id, ? || t.task_id, ?, ?,
			CASE WHEN t.parent_turn_id = '' THEN '' ELSE ? || t.parent_turn_id END,
			t.agent_key, t.team_id, t.team_stage_id, t.seq, t.version, t.status, t.started_at, t.completed_at
		FROM turns_v2 t JOIN tasks_v2 k ON k.id = t.task_id
		WHERE t.session_id = ? AND (k.seq < ? OR (k.seq = ? AND t.seq <= ?))
		ORDER BY k.seq, t.seq`),
		prefix, prefix, dstSessionID, dstSessionID, prefix, srcSessionID, forkTaskSeq, forkTaskSeq, forkTurnSeq)
	if err != nil {
		return 0, 0, 0, err
	}
	turns, _ := turnRes.RowsAffected()

	// steps：属于已复制 turn 的全部 step（同一边界谓词，经 turn→task 关联）。
	stepRes, err := write.ExecContext(ctx, d.RenumberPlaceholders(`
		INSERT INTO steps_v2 (id, turn_id, task_id, session_id, spirit_session_id, kind, author_agent_key, seq,
			content, reasoning, tool_name, tool_call_id, tool_args, tool_result, tool_duration_ms, tool_error_code,
			notice_type, status, is_final, started_at, completed_at, version)
		SELECT ? || s.id, ? || s.turn_id, ? || s.task_id, ?, ?,
			s.kind, s.author_agent_key, s.seq, s.content, s.reasoning, s.tool_name, s.tool_call_id,
			s.tool_args, s.tool_result, s.tool_duration_ms, s.tool_error_code,
			s.notice_type, s.status, s.is_final, s.started_at, s.completed_at, s.version
		FROM steps_v2 s
		JOIN turns_v2 t ON t.id = s.turn_id
		JOIN tasks_v2 k ON k.id = s.task_id
		WHERE s.session_id = ? AND (k.seq < ? OR (k.seq = ? AND t.seq <= ?))
		ORDER BY t.seq, s.seq`),
		prefix, prefix, prefix, dstSessionID, dstSessionID, srcSessionID, forkTaskSeq, forkTaskSeq, forkTurnSeq)
	if err != nil {
		return 0, 0, 0, err
	}
	steps, _ := stepRes.RowsAffected()

	return int(tasks), int(turns), int(steps), nil
}
