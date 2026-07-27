package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
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
