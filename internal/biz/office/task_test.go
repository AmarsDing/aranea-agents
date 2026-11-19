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

	if _, err := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "   "}); !errors.Is(err, ErrOfficeTaskTitleRequired) {
		t.Fatalf("blank title: %v", err)
	}
	first, err := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "  写复盘提纲 ", Body: " 备注 ", AssigneeKind: "agent", AssigneeID: "ag-x"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Title != "写复盘提纲" || first.Body != "备注" || first.Source != "manual" || first.Status != OfficeTaskOpen || first.AssigneeKind != "" || first.AssigneeID != "" {
		t.Fatalf("created = %+v", first)
	}
	second, _ := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: strings.Repeat("长", 300)})
	if n := len([]rune(second.Title)); n != officeTaskTitleMax {
		t.Fatalf("title should be clipped to %d runes, got %d", officeTaskTitleMax, n)
	}
	if _, err := uc.CreateTask(ctx, "u-2", "ws", NewOfficeTaskInput{Title: "别人的任务"}); err != nil {
		t.Fatal(err)
	}

	open, err := uc.ListTasks(ctx, "u-1", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != second.ID || open[1].ID != first.ID {
		t.Fatalf("open list should be own tasks newest first: %+v", open)
	}
	if _, err := uc.ListTasks(ctx, "u-1", "archived", "", 0); !errors.Is(err, ErrOfficeTaskStatusInvalid) {
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
	open, _ = uc.ListTasks(ctx, "u-1", OfficeTaskOpen, "", 0)
	closed, _ := uc.ListTasks(ctx, "u-1", OfficeTaskDone, "", 0)
	if len(open) != 1 || len(closed) != 1 || closed[0].ID != first.ID {
		t.Fatalf("after done: open=%d done=%d", len(open), len(closed))
	}
}

// 交办与随手记录和「我的任务」同表，但各自只出现在自己的列表里。
func TestOfficeTaskSources(t *testing.T) {
	repo := newMailFakeRepos()
	uc := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	tick := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	uc.now = func() time.Time { tick = tick.Add(time.Minute); return tick }
	ctx := context.Background()

	mine, err := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "自己做"})
	if err != nil {
		t.Fatal(err)
	}
	note, err := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "王总说合同月底前签", Source: " Note ", SourceRef: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if note.Source != OfficeTaskSourceNote || note.SourceRef != "" {
		t.Fatalf("note = %+v", note)
	}
	delegated, err := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{
		Title: "出一份竞品周报", Body: "周三前", Source: "delegate", SourceRef: " sess-1 ",
		AssigneeKind: "Agent", AssigneeID: " ag-1 ", AssigneeName: strings.Repeat("名", 200),
	})
	if err != nil {
		t.Fatal(err)
	}
	if delegated.SourceRef != "sess-1" || delegated.AssigneeKind != OfficeAssigneeAgent || delegated.AssigneeID != "ag-1" {
		t.Fatalf("delegated = %+v", delegated)
	}
	if n := len([]rune(delegated.AssigneeName)); n != officeTaskNameMax {
		t.Fatalf("assignee name should be clipped to %d runes, got %d", officeTaskNameMax, n)
	}

	for name, in := range map[string]NewOfficeTaskInput{
		"no session":  {Title: "x", Source: "delegate", AssigneeKind: "agent", AssigneeID: "ag-1"},
		"no assignee": {Title: "x", Source: "delegate", SourceRef: "sess-1", AssigneeKind: "agent"},
		"bad kind":    {Title: "x", Source: "delegate", SourceRef: "sess-1", AssigneeKind: "user", AssigneeID: "u-9"},
	} {
		if _, err := uc.CreateTask(ctx, "u-1", "ws", in); !errors.Is(err, ErrOfficeTaskAssigneeNeeded) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// mail 只能由邮件转任务产生；未知来源一律拒绝。
	for _, source := range []string{"mail", "cron"} {
		if _, err := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "x", Source: source}); !errors.Is(err, ErrOfficeTaskSourceInvalid) {
			t.Fatalf("source %q: %v", source, err)
		}
	}

	ids := func(source string) []string {
		rows, err := uc.ListTasks(ctx, "u-1", "", source, 0)
		if err != nil {
			t.Fatalf("list %q: %v", source, err)
		}
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	if got := ids(""); len(got) != 1 || got[0] != mine.ID {
		t.Fatalf("my tasks must exclude notes and delegations: %v", got)
	}
	if got := ids("delegate"); len(got) != 1 || got[0] != delegated.ID {
		t.Fatalf("delegated list: %v", got)
	}
	if got := ids("note"); len(got) != 1 || got[0] != note.ID {
		t.Fatalf("note list: %v", got)
	}
	if _, err := uc.ListTasks(ctx, "u-1", "", "cron", 0); !errors.Is(err, ErrOfficeTaskSourceInvalid) {
		t.Fatalf("bad source filter: %v", err)
	}
}
