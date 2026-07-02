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

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreateTask(ctx, biz.Task{
			ID: "lt-" + string(rune('a'+i)), SessionID: "ls", UserMessage: "msg",
			Status: biz.TaskStatusPending, Seq: seq, Version: 1,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("CreateTask[%d]: %v", i, err)
		}
	}
	tasks, err := repo.ListTasksBySession(ctx, "ls")
	if err != nil {
		t.Fatalf("ListTasksBySession: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Seq != 1 || tasks[1].Seq != 2 || tasks[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %d,%d,%d",
			tasks[0].Seq, tasks[1].Seq, tasks[2].Seq)
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
