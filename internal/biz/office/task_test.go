package office

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOfficeTaskLifecycle(t *testing.T) {
	repo := newMailFakeRepos()
	uc := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	tick := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	uc.now = func() time.Time { tick = tick.Add(time.Minute); return tick }
	ctx := context.Background()

	if _, err := uc.CreateTask(ctx, "u-1", "ws", "   ", ""); !errors.Is(err, ErrOfficeTaskTitleRequired) {
		t.Fatalf("blank title: %v", err)
	}
	first, err := uc.CreateTask(ctx, "u-1", "ws", "  写复盘提纲 ", " 备注 ")
	if err != nil {
		t.Fatal(err)
	}
	if first.Title != "写复盘提纲" || first.Body != "备注" || first.Source != "manual" || first.Status != OfficeTaskOpen {
		t.Fatalf("created = %+v", first)
	}
	second, _ := uc.CreateTask(ctx, "u-1", "ws", strings.Repeat("长", 300), "")
	if n := len([]rune(second.Title)); n != officeTaskTitleMax {
		t.Fatalf("title should be clipped to %d runes, got %d", officeTaskTitleMax, n)
	}
	if _, err := uc.CreateTask(ctx, "u-2", "ws", "别人的任务", ""); err != nil {
		t.Fatal(err)
	}

	open, err := uc.ListTasks(ctx, "u-1", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != second.ID || open[1].ID != first.ID {
		t.Fatalf("open list should be own tasks newest first: %+v", open)
	}
	if _, err := uc.ListTasks(ctx, "u-1", "archived", 0); !errors.Is(err, ErrOfficeTaskStatusInvalid) {
		t.Fatalf("bad status filter: %v", err)
	}

	if _, err := uc.SetTaskStatus(ctx, first.ID, "u-2", OfficeTaskDone); !errors.Is(err, ErrOfficeTaskNotFound) {
		t.Fatalf("other user's task must look missing: %v", err)
	}
	if _, err := uc.SetTaskStatus(ctx, first.ID, "u-1", "closed"); !errors.Is(err, ErrOfficeTaskStatusInvalid) {
		t.Fatalf("bad status: %v", err)
	}
	done, err := uc.SetTaskStatus(ctx, first.ID, "u-1", " Done ")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != OfficeTaskDone || !done.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("done = %+v", done)
	}
	open, _ = uc.ListTasks(ctx, "u-1", OfficeTaskOpen, 0)
	closed, _ := uc.ListTasks(ctx, "u-1", OfficeTaskDone, 0)
	if len(open) != 1 || len(closed) != 1 || closed[0].ID != first.ID {
		t.Fatalf("after done: open=%d done=%d", len(open), len(closed))
	}
}
