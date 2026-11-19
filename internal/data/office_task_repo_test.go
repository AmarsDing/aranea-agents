package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/office"
)

// TestOfficeMailRepo_TaskSourcesAndAssignee 任务按来源分列表、交办对象随行
// 读写、状态写路径按属主隔离。
func TestOfficeMailRepo_TaskSourcesAndAssignee(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)

	rows := []*office.OfficeTask{
		{ID: "otk_m", UserID: "u-1", Title: "自己做", Source: "manual"},
		{ID: "otk_mail", UserID: "u-1", Title: "邮件转的", Source: "mail", SourceRef: "omm_x"},
		{ID: "otk_n", UserID: "u-1", Title: "记一笔", Source: "note"},
		{
			ID: "otk_d", UserID: "u-1", Title: "竞品周报", Source: "delegate", SourceRef: "sess-1",
			AssigneeKind: "agent", AssigneeID: "ag-1", AssigneeName: "内容组",
		},
		{ID: "otk_other", UserID: "u-2", Title: "别人的交办", Source: "delegate", SourceRef: "sess-2", AssigneeKind: "team", AssigneeID: "tm-1"},
	}
	for i, row := range rows {
		row.WorkspaceID, row.Status = "ws-1", "open"
		row.CreatedAt = base.Add(time.Duration(i) * time.Minute)
		row.UpdatedAt = row.CreatedAt
		if err := repo.CreateOfficeTask(ctx, row); err != nil {
			t.Fatalf("create %s: %v", row.ID, err)
		}
	}

	ids := func(sources ...string) []string {
		got, err := repo.ListOfficeTasks(ctx, "u-1", "open", sources, 10)
		if err != nil {
			t.Fatalf("list %v: %v", sources, err)
		}
		out := make([]string, 0, len(got))
		for _, g := range got {
			out = append(out, g.ID)
		}
		return out
	}
	if got := ids("manual", "mail"); len(got) != 2 || got[0] != "otk_mail" || got[1] != "otk_m" {
		t.Fatalf("my tasks newest first: %v", got)
	}
	if got := ids("note"); len(got) != 1 || got[0] != "otk_n" {
		t.Fatalf("notes: %v", got)
	}
	if got := ids(); len(got) != 0 {
		t.Fatalf("empty source set must return nothing: %v", got)
	}
	delegated, err := repo.ListOfficeTasks(ctx, "u-1", "open", []string{"delegate"}, 10)
	if err != nil || len(delegated) != 1 {
		t.Fatalf("delegated: %+v err=%v", delegated, err)
	}
	if d := delegated[0]; d.SourceRef != "sess-1" || d.AssigneeKind != "agent" || d.AssigneeID != "ag-1" || d.AssigneeName != "内容组" {
		t.Fatalf("assignee roundtrip: %+v", d)
	}

	if err := repo.SetOfficeTaskStatus(ctx, "otk_d", "u-2", "done", time.Now()); !errors.Is(err, office.ErrOfficeTaskNotFound) {
		t.Fatalf("other user's task must look missing: %v", err)
	}
	if err := repo.SetOfficeTaskStatus(ctx, "otk_d", "u-1", "done", time.Now()); err != nil {
		t.Fatalf("accept delegated task: %v", err)
	}
	if got := ids("delegate"); len(got) != 0 {
		t.Fatalf("accepted task must leave the open list: %v", got)
	}
	got, err := repo.GetOfficeTask(ctx, "otk_d", "u-1")
	if err != nil || got.Status != "done" || got.AssigneeName != "内容组" {
		t.Fatalf("get after accept: %+v err=%v", got, err)
	}
}
