package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

func TestTaskV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreateTask(ctx, biz.Task{
		ID: "t-1", SessionID: "s-1", UserMessage: "hi",
		Status: biz.TaskStatusPending, Seq: 1, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.ID != "t-1" || created.UserMessage != "hi" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetTask(ctx, "t-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.UserMessage != "hi" || got.Seq != 1 || got.Version != 1 {
		t.Fatalf("task mismatch: %+v", got)
	}
}

func TestTaskV2Repo_Upsert_VersionGuard(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// v1: insert (no existing row → falls through to Create path)
	_, err := repo.UpsertTask(ctx, biz.Task{
		ID: "t-2", SessionID: "s-1", UserMessage: "v1",
		Status: biz.TaskStatusRunning, Seq: 2, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertTask v1: %v", err)
	}
	// v0 (older): should NOT overwrite — falls through to Create, which
	// fails with ConstraintError, then returns the existing (newer) row.
	stale, err := repo.UpsertTask(ctx, biz.Task{
		ID: "t-2", SessionID: "s-1", UserMessage: "stale",
		Status: biz.TaskStatusPending, Seq: 2, Version: 0,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertTask stale: %v", err)
	}
	if stale.UserMessage != "v1" {
		t.Fatalf("stale version overwrote: got %s", stale.UserMessage)
	}
	if stale.Status != biz.TaskStatusRunning {
		t.Fatalf("status changed: got %s", stale.Status)
	}
	// v2 (newer): should overwrite via update path
	_, err = repo.UpsertTask(ctx, biz.Task{
		ID: "t-2", SessionID: "s-1", UserMessage: "v2",
		Status: biz.TaskStatusCompleted, Seq: 2, Version: 2,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertTask v2: %v", err)
	}
	got, _ := repo.GetTask(ctx, "t-2")
	if got.UserMessage != "v2" || got.Status != biz.TaskStatusCompleted {
		t.Fatalf("newer version did not overwrite: %+v", got)
	}
	if got.Version != 2 {
		t.Fatalf("version: expected 2, got %d", got.Version)
	}
}

func TestTaskV2Repo_ListBySession(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// 2026-07-26 修复：排序键从 seq 改为 created_at（id 兜底并列）。seq 赋值
	// 源头不可靠（澄清门硬编码 Seq=1、正常 turn 路径默认 0），多任务会话下
	// resolveLatestUserTaskID 按 seq 取"最后一个"会选错父任务；created_at
	// 才是"最近用户任务"的语义真相。构造 seq 与 created_at 顺序相反的数据，
	// 断言按 created_at 升序返回。
	seed := []struct {
		id        string
		seq       int64
		createdAt time.Time
	}{
		{"lt-b", 1, now.Add(2 * time.Minute)}, // seq 并列 1 但创建最晚
		{"lt-a", 1, now.Add(time.Minute)},     // seq 并列 1 创建居中
		{"lt-c", 0, now},                      // seq=0（正常路径默认值）创建最早
	}
	for _, s := range seed {
		_, err := repo.CreateTask(ctx, biz.Task{
			ID: s.id, SessionID: "ls", UserMessage: "msg",
			Status: biz.TaskStatusPending, Seq: s.seq, Version: 1,
			CreatedAt: s.createdAt, UpdatedAt: s.createdAt,
		})
		if err != nil {
			t.Fatalf("CreateTask[%s]: %v", s.id, err)
		}
	}
	tasks, err := repo.ListTasksBySession(ctx, "ls")
	if err != nil {
		t.Fatalf("ListTasksBySession: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	want := []string{"lt-c", "lt-a", "lt-b"} // created_at 升序
	got := []string{tasks[0].ID, tasks[1].ID, tasks[2].ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: expected %v got %v", want, got)
		}
	}
}

func TestTaskV2Repo_GetTask_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetTask(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent task, got nil")
	}
}

// L3 (2026-07-22)：interrupted → running CAS。只有 interrupted 状态的 task
// 能被恢复；其他状态 ok=false 且数据不被触碰；重复恢复第二次必须失败
// （防双击/并发复活两次）。
func TestTaskV2Repo_ResumeInterruptedTask(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	completedAt := now.Add(-time.Hour)

	if _, err := repo.CreateTask(ctx, biz.Task{
		ID: "t-resume", SessionID: "s-1", UserMessage: "do it",
		Status: biz.TaskStatusInterrupted, Seq: 1, Version: 3,
		CreatedAt: now, UpdatedAt: now, CompletedAt: &completedAt,
	}); err != nil {
		t.Fatalf("seed interrupted task: %v", err)
	}
	if _, err := repo.CreateTask(ctx, biz.Task{
		ID: "t-failed", SessionID: "s-1", UserMessage: "nope",
		Status: biz.TaskStatusFailed, Seq: 2, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed failed task: %v", err)
	}

	resumeAt := now.Add(time.Minute)
	got, ok, err := repo.ResumeInterruptedTask(ctx, "t-resume", resumeAt)
	if err != nil || !ok {
		t.Fatalf("resume interrupted: ok=%v err=%v", ok, err)
	}
	if got.Status != biz.TaskStatusRunning {
		t.Errorf("status=%s, want running", got.Status)
	}
	if got.Version != 4 {
		t.Errorf("version=%d, want 4 (3+1)", got.Version)
	}
	if got.CompletedAt != nil {
		t.Errorf("completed_at=%v, want cleared", got.CompletedAt)
	}
	if !got.UpdatedAt.Equal(resumeAt) {
		t.Errorf("updated_at=%v, want %v", got.UpdatedAt, resumeAt)
	}

	// Second resume must fail (already running).
	if _, ok, err = repo.ResumeInterruptedTask(ctx, "t-resume", resumeAt); err != nil || ok {
		t.Errorf("second resume: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	// Non-interrupted task must not be touched.
	if _, ok, err = repo.ResumeInterruptedTask(ctx, "t-failed", resumeAt); err != nil || ok {
		t.Errorf("resume failed-status task: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	failed, _ := repo.GetTask(ctx, "t-failed")
	if failed.Status != biz.TaskStatusFailed || failed.Version != 1 {
		t.Errorf("failed task touched: status=%s version=%d", failed.Status, failed.Version)
	}
	// Unknown id: ok=false, err=nil.
	if _, ok, err = repo.ResumeInterruptedTask(ctx, "t-missing", resumeAt); err != nil || ok {
		t.Errorf("resume missing task: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

// L3 (2026-07-22)：CompleteTaskTerminal 无条件终态化（version DB+1，不看事件
// version）。修复 resume 场景的 version 冲突：synthesis turn 的 OnTurnEnd 硬编码
// Version=2，而 resume CAS 已把 version 推高，走 UpsertTask 的 VersionLT guard
// 会被拒绝导致 task 永远 running。终态事件天然单调，不依赖调用方提供正确 version。
func TestTaskV2Repo_CompleteTaskTerminal(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := repo.CreateTask(ctx, biz.Task{
		ID: "t-run", SessionID: "s-1", UserMessage: "work",
		Status: biz.TaskStatusRunning, Seq: 1, Version: 5,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed running task: %v", err)
	}

	// 1) running → completed：事件 version=2 远低于 DB version=5 也必须生效。
	completedAt := now.Add(time.Minute)
	got, err := repo.CompleteTaskTerminal(ctx, biz.Task{
		ID: "t-run", Status: biz.TaskStatusCompleted, CompletedAt: &completedAt, Version: 2,
	})
	if err != nil {
		t.Fatalf("CompleteTaskTerminal: %v", err)
	}
	if got.Status != biz.TaskStatusCompleted {
		t.Errorf("status=%s, want completed", got.Status)
	}
	if got.Version != 6 {
		t.Errorf("version=%d, want 6 (DB 5+1, not event version)", got.Version)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Errorf("completed_at=%v, want %v", got.CompletedAt, completedAt)
	}
	if got.UserMessage != "work" {
		t.Errorf("user_message=%q, want preserved", got.UserMessage)
	}

	// 2) 已终态 → 第二个终态事件不覆盖（幂等）：completed 不被 failed 翻转。
	got, err = repo.CompleteTaskTerminal(ctx, biz.Task{
		ID: "t-run", Status: biz.TaskStatusFailed, Version: 7,
	})
	if err != nil {
		t.Fatalf("CompleteTaskTerminal second terminal: %v", err)
	}
	if got.Status != biz.TaskStatusCompleted || got.Version != 6 {
		t.Errorf("terminal overwritten: status=%s version=%d, want completed/6", got.Status, got.Version)
	}

	// 3) interrupted → completed：interrupted 是恢复占位不是真终态，允许覆盖。
	if _, err := repo.CreateTask(ctx, biz.Task{
		ID: "t-int", SessionID: "s-1", UserMessage: "resumable",
		Status: biz.TaskStatusInterrupted, Seq: 2, Version: 3,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed interrupted task: %v", err)
	}
	got, err = repo.CompleteTaskTerminal(ctx, biz.Task{
		ID: "t-int", Status: biz.TaskStatusCompleted, CompletedAt: &completedAt, Version: 2,
	})
	if err != nil {
		t.Fatalf("CompleteTaskTerminal interrupted: %v", err)
	}
	if got.Status != biz.TaskStatusCompleted || got.Version != 4 {
		t.Errorf("interrupted→completed: status=%s version=%d, want completed/4", got.Status, got.Version)
	}

	// 4) 不存在的 ID → 按事件内容创建（PublishTurnFailure 的 shadow task 语义）。
	got, err = repo.CompleteTaskTerminal(ctx, biz.Task{
		ID: "t-shadow", SessionID: "s-1", Status: biz.TaskStatusFailed,
		CompletedAt: &completedAt, Version: 1, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CompleteTaskTerminal shadow create: %v", err)
	}
	if got.Status != biz.TaskStatusFailed || got.ID != "t-shadow" {
		t.Errorf("shadow task: id=%s status=%s, want t-shadow/failed", got.ID, got.Status)
	}
}

func TestTaskV2Repo_ListTaskSituations_ACLBeforeCountAndStablePages(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	ctx := workspace.WithContext(context.Background(), "ws-own")
	now := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < 35; i++ {
		id := fmt.Sprintf("task-%02d", i)
		if _, err := repo.CreateTask(ctx, biz.Task{
			ID: id, SessionID: "session-own", UserMessage: "visible " + id,
			Status: biz.TaskStatusRunning, Version: 1, WorkspaceID: "ws-own",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed visible %s: %v", id, err)
		}
	}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("foreign-%02d", i)
		if _, err := repo.CreateTask(ctx, biz.Task{
			ID: id, SessionID: "session-foreign", UserMessage: "hidden " + id,
			Status: biz.TaskStatusRunning, Version: 1, WorkspaceID: "ws-other",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed foreign %s: %v", id, err)
		}
	}

	first, err := repo.ListTaskSituations(ctx, biz.TaskSituationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	second, err := repo.ListTaskSituations(ctx, biz.TaskSituationListOptions{Limit: 20, Offset: 20})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if first.Total != 35 || second.Total != 35 {
		t.Fatalf("ACL must apply before count: totals=%d/%d, want 35", first.Total, second.Total)
	}
	if len(first.Items) != 20 || len(second.Items) != 15 {
		t.Fatalf("page sizes=%d/%d, want 20/15", len(first.Items), len(second.Items))
	}
	seen := make(map[string]int, 35)
	for page, items := range [][]biz.TaskSituation{first.Items, second.Items} {
		for _, item := range items {
			seen[item.Task.ID]++
			if item.Task.WorkspaceID != "ws-own" {
				t.Fatalf("foreign task leaked on page %d: %+v", page+1, item.Task)
			}
		}
	}
	if len(seen) != 35 {
		t.Fatalf("paged union=%d, want 35; counts=%v", len(seen), seen)
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("task %s appeared %d times", id, count)
		}
	}
	// Identical updated_at values must still have a deterministic id tie-breaker.
	for i := 1; i < len(first.Items); i++ {
		if first.Items[i-1].Task.ID <= first.Items[i].Task.ID {
			t.Fatalf("unstable desc id order at %d: %s <= %s", i, first.Items[i-1].Task.ID, first.Items[i].Task.ID)
		}
	}
}

func TestTaskV2Repo_ListTaskSituations_WaitingMatchesProjection(t *testing.T) {
	d := openTestDataWithRWDB(t)
	taskRepo := NewTaskV2Repo(d, loggateway.NewNoop())
	turnRepo := NewTurnV2Repo(d, loggateway.NewNoop())
	stepRepo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := workspace.WithContext(context.Background(), "ws-own")
	now := time.Now().UTC().Truncate(time.Second)

	tasks := []biz.Task{
		{ID: "wait-interrupted", Status: biz.TaskStatusInterrupted},
		{ID: "wait-input", Status: biz.TaskStatusRunning},
		{ID: "wait-tool", Status: biz.TaskStatusRunning},
		{ID: "active-latest-step", Status: biz.TaskStatusRunning},
		{ID: "active-latest-turn", Status: biz.TaskStatusRunning},
	}
	for _, task := range tasks {
		task.SessionID = "session-own"
		task.UserMessage = task.ID
		task.Version = 1
		task.WorkspaceID = "ws-own"
		task.CreatedAt = now
		task.UpdatedAt = now
		if _, err := taskRepo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}

	turns := []biz.Turn{
		{ID: "turn-input", TaskID: "wait-input", Seq: 1},
		{ID: "turn-tool", TaskID: "wait-tool", Seq: 1},
		{ID: "turn-latest-step", TaskID: "active-latest-step", Seq: 1},
		{ID: "turn-old", TaskID: "active-latest-turn", Seq: 1, StartedAt: now},
		{ID: "turn-new", TaskID: "active-latest-turn", Seq: 2, StartedAt: now.Add(time.Second)},
	}
	for _, turn := range turns {
		turn.SessionID = "session-own"
		turn.SpiritSessionID = "session-own"
		turn.Status = biz.TurnStatusRunning
		turn.Version = 1
		if turn.StartedAt.IsZero() {
			turn.StartedAt = now
		}
		if _, err := turnRepo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("seed turn %s: %v", turn.ID, err)
		}
	}

	steps := []biz.Step{
		{ID: "step-input", TurnID: "turn-input", TaskID: "wait-input", Seq: 1, Status: biz.StepStatusAwaitingInput},
		{ID: "step-tool", TurnID: "turn-tool", TaskID: "wait-tool", Seq: 1, Status: biz.StepStatusToolBlocked},
		{ID: "step-old-wait", TurnID: "turn-latest-step", TaskID: "active-latest-step", Seq: 1, Status: biz.StepStatusAwaitingInput, StartedAt: now},
		{ID: "step-new-run", TurnID: "turn-latest-step", TaskID: "active-latest-step", Seq: 2, Status: biz.StepStatusRunning, StartedAt: now.Add(time.Second)},
		{ID: "step-old-turn", TurnID: "turn-old", TaskID: "active-latest-turn", Seq: 1, Status: biz.StepStatusAwaitingInput},
		{ID: "step-new-turn", TurnID: "turn-new", TaskID: "active-latest-turn", Seq: 1, Status: biz.StepStatusRunning},
	}
	for _, step := range steps {
		step.SessionID = "session-own"
		step.SpiritSessionID = "session-own"
		step.Kind = biz.StepKindThinking
		step.Version = 1
		if step.StartedAt.IsZero() {
			step.StartedAt = now
		}
		if _, err := stepRepo.CreateStep(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	page, err := taskRepo.ListTaskSituations(ctx, biz.TaskSituationListOptions{Status: "waiting", Limit: 20})
	if err != nil {
		t.Fatalf("list waiting: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("waiting total/items=%d/%d, want 3/3", page.Total, len(page.Items))
	}
	want := map[string]bool{"wait-interrupted": true, "wait-input": true, "wait-tool": true}
	for _, item := range page.Items {
		if !want[item.Task.ID] {
			t.Errorf("non-waiting task leaked: %s", item.Task.ID)
		}
		delete(want, item.Task.ID)
	}
	if len(want) != 0 {
		t.Fatalf("waiting tasks missing: %v", want)
	}
}
