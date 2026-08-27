package data

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
)

// 79-runtime-governance R6：SessionForkStore 的 PG 集成测试。
// 覆盖：事件边界定位（event->>'invocationId'）、框架事件前缀复制、
// state 行创建、v2 (task,turn) 二维边界复制与 id 确定性重映射、源会话零影响。

func setupForkTestTables(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS trpc_session_states (
			id BIGSERIAL PRIMARY KEY,
			app_name VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			session_id VARCHAR(255) NOT NULL,
			state JSONB DEFAULT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP DEFAULT NULL,
			deleted_at TIMESTAMP DEFAULT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS trpc_session_events (
			id BIGSERIAL PRIMARY KEY,
			app_name VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			session_id VARCHAR(255) NOT NULL,
			event JSONB NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP DEFAULT NULL,
			deleted_at TIMESTAMP DEFAULT NULL
		)`,
	}
	for _, stmt := range ddl {
		if _, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, stmt); err != nil {
			t.Fatalf("create framework table: %v", err)
		}
	}
}

func seedForkSource(t *testing.T, d *Data, src string) (boundaryTurn string) {
	t.Helper()
	ctx := context.Background()
	w := d.RWDB().WriteDB(ctx)

	stateJSON := `{"id":"` + src + `","state":{},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}`
	if _, err := w.ExecContext(ctx, `
		INSERT INTO trpc_session_states (app_name, user_id, session_id, state)
		VALUES ('app', 'u1', $1, $2::jsonb)`, src, stateJSON); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	// 事件序：inv-A×3 → inv-B×2 → inv-C×1（inv-C 在分叉点之后，不应复制）。
	events := []string{"inv-A", "inv-A", "inv-A", "inv-B", "inv-B", "inv-C"}
	for _, inv := range events {
		eventJSON := `{"invocationId":"` + inv + `","author":"agent"}`
		if _, err := w.ExecContext(ctx, `
			INSERT INTO trpc_session_events (app_name, user_id, session_id, event)
			VALUES ('app', 'u1', $1, $2::jsonb)`, src, eventJSON); err != nil {
			t.Fatalf("seed event %s: %v", inv, err)
		}
	}

	// v2：task1(seq1: turn inv-A) → task2(seq2: turns inv-B seq1 / inv-B2 seq2)。
	v2 := []string{
		`INSERT INTO tasks_v2 (id, session_id, user_message, status, seq, version, workspace_id, created_at, updated_at)
		 VALUES ('task-1', '` + src + `', 'm1', 'completed', 1, 1, '', now(), now())`,
		`INSERT INTO tasks_v2 (id, session_id, user_message, status, seq, version, workspace_id, created_at, updated_at)
		 VALUES ('task-2', '` + src + `', 'm2', 'completed', 2, 1, '', now(), now())`,
		`INSERT INTO turns_v2 (id, task_id, session_id, spirit_session_id, parent_turn_id, agent_key, team_id, team_stage_id, seq, version, status, started_at)
		 VALUES ('inv-A', 'task-1', '` + src + `', '` + src + `', '', 'a', '', '', 1, 1, 'completed', now())`,
		`INSERT INTO turns_v2 (id, task_id, session_id, spirit_session_id, parent_turn_id, agent_key, team_id, team_stage_id, seq, version, status, started_at)
		 VALUES ('inv-B', 'task-2', '` + src + `', '` + src + `', '', 'a', '', '', 1, 1, 'completed', now())`,
		`INSERT INTO turns_v2 (id, task_id, session_id, spirit_session_id, parent_turn_id, agent_key, team_id, team_stage_id, seq, version, status, started_at)
		 VALUES ('inv-B2', 'task-2', '` + src + `', '` + src + `', '', 'a', '', '', 2, 1, 'completed', now())`,
		`INSERT INTO steps_v2 (id, turn_id, task_id, session_id, spirit_session_id, kind, author_agent_key, seq, content, reasoning, tool_name, tool_call_id, tool_args, tool_result, tool_duration_ms, tool_error_code, notice_type, status, is_final, started_at, version)
		 VALUES ('inv-A-s1', 'inv-A', 'task-1', '` + src + `', '` + src + `', 'reply', 'a', 1, 'hello', '', '', '', '', '', 0, '', '', 'completed', true, now(), 1)`,
		`INSERT INTO steps_v2 (id, turn_id, task_id, session_id, spirit_session_id, kind, author_agent_key, seq, content, reasoning, tool_name, tool_call_id, tool_args, tool_result, tool_duration_ms, tool_error_code, notice_type, status, is_final, started_at, version)
		 VALUES ('inv-B-s1', 'inv-B', 'task-2', '` + src + `', '` + src + `', 'reply', 'a', 1, 'world', '', '', '', '', '', 0, '', '', 'completed', true, now(), 1)`,
		`INSERT INTO steps_v2 (id, turn_id, task_id, session_id, spirit_session_id, kind, author_agent_key, seq, content, reasoning, tool_name, tool_call_id, tool_args, tool_result, tool_duration_ms, tool_error_code, notice_type, status, is_final, started_at, version)
		 VALUES ('inv-B2-s1', 'inv-B2', 'task-2', '` + src + `', '` + src + `', 'reply', 'a', 1, 'after fork', '', '', '', '', '', 0, '', '', 'completed', true, now(), 1)`,
	}
	for _, stmt := range v2 {
		if _, err := w.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v2: %v\nSQL: %s", err, stmt)
		}
	}
	return "inv-B"
}

func TestSessionForkRepo_ForkCopy(t *testing.T) {
	d := openTestDataWithRWDB(t)
	setupForkTestTables(t, d)
	const src = "src-fork-1"
	const dst = "dst-fork-1"
	forkTurn := seedForkSource(t, d, src)

	repo := NewSessionForkRepo(d)
	ctx := context.Background()

	err := repo.ForkSessionInTx(ctx, func(txCtx context.Context) error {
		boundary, found, err := repo.FindTurnEventBoundary(txCtx, src, forkTurn)
		if err != nil || !found {
			t.Fatalf("FindTurnEventBoundary: found=%v err=%v", found, err)
		}

		if err := repo.CreateFrameworkState(txCtx, src, dst); err != nil {
			t.Fatalf("CreateFrameworkState: %v", err)
		}
		events, err := repo.CopyFrameworkEvents(txCtx, src, dst, boundary)
		if err != nil {
			t.Fatalf("CopyFrameworkEvents: %v", err)
		}
		if events != 5 { // inv-A×3 + inv-B×2；inv-C 在边界之后
			t.Fatalf("events copied = %d, want 5", events)
		}

		tasks, turns, steps, err := repo.CopyV2Records(txCtx, src, dst, forkTurn)
		if err != nil {
			t.Fatalf("CopyV2Records: %v", err)
		}
		if tasks != 2 || turns != 2 || steps != 2 {
			t.Fatalf("v2 copied tasks/turns/steps = %d/%d/%d, want 2/2/2", tasks, turns, steps)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ForkSessionInTx: %v", err)
	}

	// 事务外验证复制结果（读已提交数据）。
	r := d.RWDB().ReadDB(ctx)
	prefix := forkIDPrefix(dst)

	var stateCount int
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM trpc_session_states WHERE session_id = $1 AND state IS NOT NULL`, []any{dst}, &stateCount); err != nil {
		t.Fatalf("count dst state: %v", err)
	}
	if stateCount != 1 {
		t.Fatalf("dst state rows = %d, want 1", stateCount)
	}
	// state JSON 必须可被框架 getSession 解析（含 id/state 键）。
	var stateJSON string
	if err := queryRowScan(ctx, r, `SELECT state::text FROM trpc_session_states WHERE session_id = $1`, []any{dst}, &stateJSON); err != nil {
		t.Fatalf("read dst state: %v", err)
	}
	if stateJSON == "" || stateJSON == "null" {
		t.Fatalf("dst state JSON invalid: %q", stateJSON)
	}

	// 事件保序且不含 inv-C。
	rows, err := r.QueryContext(ctx, `SELECT event->>'invocationId' FROM trpc_session_events WHERE session_id = $1 ORDER BY created_at ASC, id ASC`, dst)
	if err != nil {
		t.Fatalf("list dst events: %v", err)
	}
	defer rows.Close()
	var invs []string
	for rows.Next() {
		var inv string
		if err := rows.Scan(&inv); err != nil {
			t.Fatalf("scan event: %v", err)
		}
		invs = append(invs, inv)
	}
	if len(invs) != 5 || invs[0] != "inv-A" || invs[4] != "inv-B" {
		t.Fatalf("dst event invocations = %v, want [inv-A x3, inv-B x2]", invs)
	}

	// v2：复制行 id 前缀 + 挂接新会话；分叉后的 inv-B2 不出现。
	var copiedTurn int
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1 AND id = $2 AND task_id = $3`,
		[]any{dst, prefix + "inv-B", prefix + "task-2"}, &copiedTurn); err != nil {
		t.Fatalf("check remapped turn: %v", err)
	}
	if copiedTurn != 1 {
		t.Fatalf("remapped turn %q not found in dst", prefix+"inv-B")
	}
	var afterFork int
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1 AND id = $2`,
		[]any{dst, prefix + "inv-B2"}, &afterFork); err != nil {
		t.Fatalf("check excluded turn: %v", err)
	}
	if afterFork != 0 {
		t.Fatalf("turn after fork point leaked into dst")
	}
	var stepSession int
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM steps_v2 WHERE session_id = $1 AND spirit_session_id = $1`, []any{dst}, &stepSession); err != nil {
		t.Fatalf("check dst steps: %v", err)
	}
	if stepSession != 2 {
		t.Fatalf("dst steps = %d, want 2", stepSession)
	}

	// 源会话零影响：源行计数不变。
	var srcEvents, srcTurns int
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM trpc_session_events WHERE session_id = $1`, []any{src}, &srcEvents); err != nil {
		t.Fatalf("count src events: %v", err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1`, []any{src}, &srcTurns); err != nil {
		t.Fatalf("count src turns: %v", err)
	}
	if srcEvents != 6 || srcTurns != 3 {
		t.Fatalf("source mutated: events=%d turns=%d, want 6/3", srcEvents, srcTurns)
	}
}

func TestSessionForkRepo_BoundaryNotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	setupForkTestTables(t, d)
	const src = "src-fork-2"
	seedForkSource(t, d, src)

	repo := NewSessionForkRepo(d)
	ctx := context.Background()

	_, found, err := repo.FindTurnEventBoundary(ctx, src, "inv-nonexistent")
	if err != nil {
		t.Fatalf("FindTurnEventBoundary: %v", err)
	}
	if found {
		t.Fatalf("found = true for unknown turn")
	}

	_, _, _, err = repo.CopyV2Records(ctx, src, "dst-fork-2", "inv-nonexistent")
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("CopyV2Records unknown turn: err = %v, want NOT_FOUND", err)
	}
}

// 运行中 turn 拒绝分叉（与前端 forkable 条件一致）：复制 status='running'
// 的 turn 会把它带进新会话，而 fork 会话没有任何 runner 再写这些复制行——
// 该 turn 永不收敛（永远转圈）。
func TestSessionForkRepo_RunningTurnRejected(t *testing.T) {
	d := openTestDataWithRWDB(t)
	setupForkTestTables(t, d)
	const src = "src-fork-3"
	forkTurn := seedForkSource(t, d, src)

	ctx := context.Background()
	if _, err := d.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE turns_v2 SET status = 'running' WHERE id = $1 AND session_id = $2`, forkTurn, src); err != nil {
		t.Fatalf("mark turn running: %v", err)
	}

	repo := NewSessionForkRepo(d)
	if _, _, _, err := repo.CopyV2Records(ctx, src, "dst-fork-3", forkTurn); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("CopyV2Records running turn: err = %v, want BAD_REQUEST", err)
	}
}

// 边界 task 状态钳制：fork turn 已终态但 task 仍 running（澄清暂停 / team 派发
// 延迟 completed），verbatim 复制会在 fork 会话留下永久 running 的僵尸 task
// （重启后还会被全局 sweeper 标 interrupted 变成可 resume 的幽灵）——边界 task
// 的非终态必须按 fork turn 终态改写，非边界 task 与源会话不受影响。
func TestSessionForkRepo_BoundaryTaskStatusClamped(t *testing.T) {
	d := openTestDataWithRWDB(t)
	setupForkTestTables(t, d)
	const src = "src-fork-4"
	const dst = "dst-fork-4"
	forkTurn := seedForkSource(t, d, src) // inv-B → task-2(seq2) 的 turn seq1

	ctx := context.Background()
	// 模拟澄清暂停：fork turn(inv-B) completed，task-2 仍 running、completed_at 为 NULL。
	if _, err := d.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE tasks_v2 SET status = 'running', completed_at = NULL WHERE id = 'task-2' AND session_id = $1`, src); err != nil {
		t.Fatalf("mark boundary task running: %v", err)
	}

	repo := NewSessionForkRepo(d)
	if _, _, _, err := repo.CopyV2Records(ctx, src, dst, forkTurn); err != nil {
		t.Fatalf("CopyV2Records: %v", err)
	}

	r := d.RWDB().ReadDB(ctx)
	prefix := forkIDPrefix(dst)

	// 边界 task：running → completed，completed_at 补上。
	var status string
	var completedAt sql.NullTime
	if err := queryRowScan(ctx, r,
		`SELECT status, completed_at FROM tasks_v2 WHERE session_id = $1 AND id = $2`,
		[]any{dst, prefix + "task-2"}, &status, &completedAt); err != nil {
		t.Fatalf("read clamped task: %v", err)
	}
	if status != "completed" {
		t.Fatalf("boundary task status = %q, want completed", status)
	}
	if !completedAt.Valid {
		t.Fatalf("boundary task completed_at is NULL, want stamped")
	}

	// 非边界 task：原本终态，保持原值。
	if err := queryRowScan(ctx, r,
		`SELECT status FROM tasks_v2 WHERE session_id = $1 AND id = $2`,
		[]any{dst, prefix + "task-1"}, &status); err != nil {
		t.Fatalf("read earlier task: %v", err)
	}
	if status != "completed" {
		t.Fatalf("earlier task status = %q, want completed (unchanged)", status)
	}

	// 源会话零影响：源 task-2 仍 running。
	if err := queryRowScan(ctx, r,
		`SELECT status FROM tasks_v2 WHERE session_id = $1 AND id = 'task-2'`,
		[]any{src}, &status); err != nil {
		t.Fatalf("read src task: %v", err)
	}
	if status != "running" {
		t.Fatalf("src task mutated: status = %q, want running", status)
	}
}

// 钳制映射全分支：前端 forkable 仅排 running，failed/cancelled turn 均可分叉——
// 边界 task 非终态必须分别映射 failed/cancelled（default 分支只服务 completed），
// 且 completed_at 取 fork turn 的完成时间而非 fork 时刻。
func TestSessionForkRepo_BoundaryTaskClampMapping(t *testing.T) {
	cases := []struct {
		name       string
		turnStatus string
		taskStatus string
		want       string
	}{
		{"failed turn clamps running task to failed", "failed", "running", "failed"},
		{"cancelled turn clamps pending task to cancelled", "cancelled", "pending", "cancelled"},
		{"failed turn clamps interrupted task to failed", "failed", "interrupted", "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := openTestDataWithRWDB(t)
			setupForkTestTables(t, d)
			src := "src-clamp-" + tc.turnStatus + "-" + tc.taskStatus
			dst := "dst-clamp-" + tc.turnStatus + "-" + tc.taskStatus
			forkTurn := seedForkSource(t, d, src) // inv-B → task-2(seq2) 的 turn seq1

			ctx := context.Background()
			w := d.RWDB().WriteDB(ctx)
			// fork turn 置为目标终态，completed_at 固定在 1 小时前（区别于 fork 时刻）。
			if _, err := w.ExecContext(ctx,
				`UPDATE turns_v2 SET status = $1, completed_at = now() - interval '1 hour' WHERE id = $2 AND session_id = $3`,
				tc.turnStatus, forkTurn, src); err != nil {
				t.Fatalf("mark turn %s: %v", tc.turnStatus, err)
			}
			// 边界 task 置为非终态、completed_at 为 NULL（澄清暂停/sweeper 残留形态）。
			if _, err := w.ExecContext(ctx,
				`UPDATE tasks_v2 SET status = $1, completed_at = NULL WHERE id = 'task-2' AND session_id = $2`,
				tc.taskStatus, src); err != nil {
				t.Fatalf("mark task %s: %v", tc.taskStatus, err)
			}

			repo := NewSessionForkRepo(d)
			if _, _, _, err := repo.CopyV2Records(ctx, src, dst, forkTurn); err != nil {
				t.Fatalf("CopyV2Records: %v", err)
			}

			var status string
			var completedAt sql.NullTime
			if err := queryRowScan(ctx, d.RWDB().ReadDB(ctx),
				`SELECT status, completed_at FROM tasks_v2 WHERE session_id = $1 AND id = $2`,
				[]any{dst, forkIDPrefix(dst) + "task-2"}, &status, &completedAt); err != nil {
				t.Fatalf("read clamped task: %v", err)
			}
			if status != tc.want {
				t.Fatalf("boundary task status = %q, want %q", status, tc.want)
			}
			// completed_at ≈ 1 小时前 → 取自 turn.completed_at，而非 fork 时刻 time.Now()。
			if !completedAt.Valid {
				t.Fatalf("boundary task completed_at is NULL, want stamped from turn")
			}
			if age := time.Since(completedAt.Time); age < 59*time.Minute || age > 61*time.Minute {
				t.Fatalf("boundary task completed_at age = %v, want ≈1h (turn completed_at)", age)
			}

			// 源会话零影响：源 task-2 仍为非终态。
			var srcStatus string
			if err := queryRowScan(ctx, d.RWDB().ReadDB(ctx),
				`SELECT status FROM tasks_v2 WHERE session_id = $1 AND id = 'task-2'`,
				[]any{src}, &srcStatus); err != nil {
				t.Fatalf("read src task: %v", err)
			}
			if srcStatus != tc.taskStatus {
				t.Fatalf("src task mutated: status = %q, want %q", srcStatus, tc.taskStatus)
			}
		})
	}
}

// 幂等重跑防护：同一 (dst, turn) 重复 fork 第二次必须失败（确定性前缀 id 冲突），
// 由调用方（biz 每次生成新 dst uuid）保证不触发——此处锁定该前提。
func TestForkIDPrefix_Deterministic(t *testing.T) {
	a := forkIDPrefix("12345678-abcd-ef00-0000-000000000000")
	b := forkIDPrefix("12345678-abcd-ef00-0000-000000000000")
	if a != b || a != "fk12345678-" {
		t.Fatalf("prefix = %q/%q, want fk12345678-", a, b)
	}
	if got := forkIDPrefix("short"); got != "fkshort-" {
		t.Fatalf("short prefix = %q", got)
	}
}

// 79 Phase 4 验收：万级消息会话 fork P95 < 3s。
// 规模：10,000 框架事件（500 task × 2 turn × 10 event）+ v2 500/1000/5000 行，
// 分叉点取末 turn（turn-500-2，全量前缀复制——最坏情形）。
// 1 轮预热（PG 缓存）+ 10 轮测量，最近秩 P95 = 第 10 小值。
func TestSessionForkRepo_ForkScaleP95(t *testing.T) {
	d := openTestDataWithRWDB(t)
	setupForkTestTables(t, d)
	const src = "src-fork-scale"
	ctx := context.Background()
	w := d.RWDB().WriteDB(ctx)

	stateJSON := `{"id":"` + src + `","state":{},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}`
	if _, err := w.ExecContext(ctx, `
		INSERT INTO trpc_session_states (app_name, user_id, session_id, state)
		VALUES ('app', 'u1', $1, $2::jsonb)`, src, stateJSON); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	seeds := []string{
		// 每 task 20 条事件：turn-k-1（g 段前 10）+ turn-k-2（后 10）。
		`INSERT INTO trpc_session_events (app_name, user_id, session_id, event)
		 SELECT 'app','u1','` + src + `',
		   jsonb_build_object('invocationId','turn-'||(((g-1)/20)+1)||'-'||((((g-1)/10)%2)+1),'author','agent','seq',g)
		 FROM generate_series(1,10000) g`,
		`INSERT INTO tasks_v2 (id, session_id, user_message, status, seq, version, workspace_id, created_at, updated_at)
		 SELECT 'task-'||g, '` + src + `', 'm'||g, 'completed', g, 1, '', now(), now()
		 FROM generate_series(1,500) g`,
		`INSERT INTO turns_v2 (id, task_id, session_id, spirit_session_id, parent_turn_id, agent_key, team_id, team_stage_id, seq, version, status, started_at)
		 SELECT 'turn-'||k||'-'||s, 'task-'||k, '` + src + `', '` + src + `', '', 'a', '', '', s, 1, 'completed', now()
		 FROM generate_series(1,500) k CROSS JOIN generate_series(1,2) s`,
		`INSERT INTO steps_v2 (id, turn_id, task_id, session_id, spirit_session_id, kind, author_agent_key, seq, content, reasoning, tool_name, tool_call_id, tool_args, tool_result, tool_duration_ms, tool_error_code, notice_type, status, is_final, started_at, version)
		 SELECT 'turn-'||k||'-'||s||'-st'||st, 'turn-'||k||'-'||s, 'task-'||k, '` + src + `', '` + src + `',
		   'reply','a', st, 'content', '', '', '', '', '', 0, '', '', 'completed', true, now(), 1
		 FROM generate_series(1,500) k CROSS JOIN generate_series(1,2) s CROSS JOIN generate_series(1,5) st`,
	}
	for _, stmt := range seeds {
		if _, err := w.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed scale data: %v", err)
		}
	}

	repo := NewSessionForkRepo(d)
	const forkTurn = "turn-500-2"
	forkOnce := func(dst string) time.Duration {
		start := time.Now()
		err := repo.ForkSessionInTx(ctx, func(txCtx context.Context) error {
			boundary, found, err := repo.FindTurnEventBoundary(txCtx, src, forkTurn)
			if err != nil || !found {
				return fmt.Errorf("boundary: found=%v err=%w", found, err)
			}
			if err := repo.CreateFrameworkState(txCtx, src, dst); err != nil {
				return err
			}
			events, err := repo.CopyFrameworkEvents(txCtx, src, dst, boundary)
			if err != nil {
				return err
			}
			if events != 10000 {
				return fmt.Errorf("events = %d, want 10000", events)
			}
			tasks, turns, steps, err := repo.CopyV2Records(txCtx, src, dst, forkTurn)
			if err != nil {
				return err
			}
			if tasks != 500 || turns != 1000 || steps != 5000 {
				return fmt.Errorf("v2 = %d/%d/%d, want 500/1000/5000", tasks, turns, steps)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("fork %s: %v", dst, err)
		}
		return time.Since(start)
	}

	forkOnce("a0000000-warm") // 预热：PG 缓存/计划（dst 前 8 位唯一，模拟生产 uuid）
	const runs = 10
	durations := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		durations = append(durations, forkOnce(fmt.Sprintf("b%07d-scale", i)))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(95*runs+99)/100-1] // 最近秩
	t.Logf("fork scale durations: min=%v p50=%v p95=%v max=%v",
		durations[0], durations[runs/2], p95, durations[runs-1])
	if p95 >= 3*time.Second {
		t.Fatalf("fork P95 = %v, want < 3s", p95)
	}
}
