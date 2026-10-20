package data

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"
)

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

	forkOnce("a0000000-warm")
	const runs = 10
	durations := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		durations = append(durations, forkOnce(fmt.Sprintf("b%07d-scale", i)))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(95*runs+99)/100-1]
	t.Logf("fork scale durations: min=%v p50=%v p95=%v max=%v",
		durations[0], durations[runs/2], p95, durations[runs-1])
	if p95 >= 3*time.Second {
		t.Fatalf("fork P95 = %v, want < 3s", p95)
	}
}

// 多代 fork（2026-08-28 T5 裁定放开）：继承 id 的 fk<dst8>- 前缀必须可剥离——
// FindTurnEventBoundary 以剥离后的原 turn id 匹配复制事件的 invocationId。
func TestStripForkIDPrefixes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"inv-A", "inv-A"},
		{"fk11111111-inv-A", "inv-A"},
		{"fk22222222-fk11111111-inv-A", "inv-A"},
		{"fkzzzzzzzz-inv-A", "fkzzzzzzzz-inv-A"},
		{"fk1234-inv-A", "fk1234-inv-A"},
		{"fk11111111", "fk11111111"},
		{"12345678-abcd-ef00-0000-000000000000", "12345678-abcd-ef00-0000-000000000000"},
	}
	for _, tc := range cases {
		if got := stripForkIDPrefixes(tc.in); got != tc.want {
			t.Fatalf("stripForkIDPrefixes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// fork 会话内分叉后的新增活动：新 task(seq3) + turn inv-D + step + 事件。
func seedForkPostActivity(t *testing.T, d *Data, session string) {
	t.Helper()
	ctx := context.Background()
	w := d.RWDB().WriteDB(ctx)
	stmts := []string{
		`INSERT INTO tasks_v2 (id, session_id, user_message, status, seq, version, workspace_id, created_at, updated_at)
		 VALUES ('task-d', '` + session + `', 'md', 'completed', 3, 1, '', now(), now())`,
		`INSERT INTO turns_v2 (id, task_id, session_id, spirit_session_id, parent_turn_id, agent_key, team_id, team_stage_id, seq, version, status, started_at)
		 VALUES ('inv-D', 'task-d', '` + session + `', '` + session + `', '', 'a', '', '', 1, 1, 'completed', now())`,
		`INSERT INTO steps_v2 (id, turn_id, task_id, session_id, spirit_session_id, kind, author_agent_key, seq, content, reasoning, tool_name, tool_call_id, tool_args, tool_result, tool_duration_ms, tool_error_code, notice_type, status, is_final, started_at, version)
		 VALUES ('inv-D-s1', 'inv-D', 'task-d', '` + session + `', '` + session + `', 'reply', 'a', 1, 'post fork', '', '', '', '', '', 0, '', '', 'completed', true, now(), 1)`,
		`INSERT INTO trpc_session_events (app_name, user_id, session_id, event)
		 VALUES ('app', 'u1', '` + session + `', '{"invocationId":"inv-D","author":"agent"}'::jsonb)`,
	}
	for _, stmt := range stmts {
		if _, err := w.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed post activity: %v\nSQL: %s", err, stmt)
		}
	}
}

// 多代 fork（T5）：fork 会话可再 fork。
// ① 分叉点为继承 turn（fk 前缀 id）——边界经前缀剥离命中复制事件；
// ② 分叉点为 fork 后新增 turn（原 id）——直接命中；
// ③ 重映射 id 归一为单层新前缀（世代叠加不超限）；④ 各级源会话零影响。
func TestSessionForkRepo_ChainedFork(t *testing.T) {
	d := openTestDataWithRWDB(t)
	setupForkTestTables(t, d)
	const src = "src-chain-0"
	// dst 用 uuid 形态——生产前缀 fk<8hex>- 由 uuid 派生，剥离正则按 hex 锚定。
	const dst1 = "11111111-1111-4111-8111-111111111111"
	const dst2a = "22222222-2222-4222-8222-222222222222"
	const dst2b = "33333333-3333-4333-8333-333333333333"
	forkTurn := seedForkSource(t, d, src)
	repo := NewSessionForkRepo(d)
	ctx := context.Background()

	err := repo.ForkSessionInTx(ctx, func(txCtx context.Context) error {
		boundary, found, err := repo.FindTurnEventBoundary(txCtx, src, forkTurn)
		if err != nil || !found {
			return fmt.Errorf("gen1 boundary: found=%v err=%w", found, err)
		}
		if err := repo.CreateFrameworkState(txCtx, src, dst1); err != nil {
			return err
		}
		if _, err := repo.CopyFrameworkEvents(txCtx, src, dst1, boundary); err != nil {
			return err
		}
		_, _, _, err = repo.CopyV2Records(txCtx, src, dst1, forkTurn)
		return err
	})
	if err != nil {
		t.Fatalf("gen1 fork: %v", err)
	}

	seedForkPostActivity(t, d, dst1)

	p1 := forkIDPrefix(dst1)
	p2a := forkIDPrefix(dst2a)
	p2b := forkIDPrefix(dst2b)

	err = repo.ForkSessionInTx(ctx, func(txCtx context.Context) error {
		boundary, found, err := repo.FindTurnEventBoundary(txCtx, dst1, p1+"inv-B")
		if err != nil || !found {
			return fmt.Errorf("gen2a boundary (inherited turn): found=%v err=%w", found, err)
		}
		if err := repo.CreateFrameworkState(txCtx, dst1, dst2a); err != nil {
			return err
		}
		events, err := repo.CopyFrameworkEvents(txCtx, dst1, dst2a, boundary)
		if err != nil {
			return err
		}
		if events != 5 {
			return fmt.Errorf("gen2a events = %d, want 5", events)
		}
		tasks, turns, steps, err := repo.CopyV2Records(txCtx, dst1, dst2a, p1+"inv-B")
		if err != nil {
			return err
		}
		if tasks != 2 || turns != 2 || steps != 2 {
			return fmt.Errorf("gen2a v2 = %d/%d/%d, want 2/2/2", tasks, turns, steps)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("gen2a fork: %v", err)
	}

	r := d.RWDB().ReadDB(ctx)
	// id 归一单层前缀：fk22222222-inv-B 存在；世代叠加 fk22222222-fk11111111-* 不存在。
	var n int
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1 AND id = $2 AND task_id = $3`,
		[]any{dst2a, p2a + "inv-B", p2a + "task-2"}, &n); err != nil || n != 1 {
		t.Fatalf("gen2a normalized turn missing: n=%d err=%v", n, err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1 AND id LIKE $2`,
		[]any{dst2a, p2a + "fk%"}, &n); err != nil || n != 0 {
		t.Fatalf("gen2a accumulated prefix leaked: n=%d err=%v", n, err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM steps_v2 WHERE session_id = $1 AND id LIKE $2`,
		[]any{dst2a, p2a + "fk%"}, &n); err != nil || n != 0 {
		t.Fatalf("gen2a accumulated step prefix leaked: n=%d err=%v", n, err)
	}

	err = repo.ForkSessionInTx(ctx, func(txCtx context.Context) error {
		boundary, found, err := repo.FindTurnEventBoundary(txCtx, dst1, "inv-D")
		if err != nil || !found {
			return fmt.Errorf("gen2b boundary (new turn): found=%v err=%w", found, err)
		}
		if err := repo.CreateFrameworkState(txCtx, dst1, dst2b); err != nil {
			return err
		}
		events, err := repo.CopyFrameworkEvents(txCtx, dst1, dst2b, boundary)
		if err != nil {
			return err
		}
		if events != 6 {
			return fmt.Errorf("gen2b events = %d, want 6", events)
		}
		tasks, turns, steps, err := repo.CopyV2Records(txCtx, dst1, dst2b, "inv-D")
		if err != nil {
			return err
		}
		if tasks != 3 || turns != 3 || steps != 3 {
			return fmt.Errorf("gen2b v2 = %d/%d/%d, want 3/3/3", tasks, turns, steps)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("gen2b fork: %v", err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1 AND id = $2`,
		[]any{dst2b, p2b + "inv-D"}, &n); err != nil || n != 1 {
		t.Fatalf("gen2b new turn missing: n=%d err=%v", n, err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1 AND id = $2`,
		[]any{dst2b, p2b + "inv-B"}, &n); err != nil || n != 1 {
		t.Fatalf("gen2b inherited turn not normalized: n=%d err=%v", n, err)
	}

	// ④ 中间代源会话零影响：dst1 计数不变（events 6 / turns 3 / tasks 3 / steps 3）。
	var ev, tn, tk, st int
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM trpc_session_events WHERE session_id = $1`, []any{dst1}, &ev); err != nil {
		t.Fatalf("count dst1 events: %v", err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM turns_v2 WHERE session_id = $1`, []any{dst1}, &tn); err != nil {
		t.Fatalf("count dst1 turns: %v", err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM tasks_v2 WHERE session_id = $1`, []any{dst1}, &tk); err != nil {
		t.Fatalf("count dst1 tasks: %v", err)
	}
	if err := queryRowScan(ctx, r, `SELECT COUNT(1) FROM steps_v2 WHERE session_id = $1`, []any{dst1}, &st); err != nil {
		t.Fatalf("count dst1 steps: %v", err)
	}
	if ev != 6 || tn != 3 || tk != 3 || st != 3 {
		t.Fatalf("dst1 mutated: events=%d turns=%d tasks=%d steps=%d, want 6/3/3/3", ev, tn, tk, st)
	}
}
