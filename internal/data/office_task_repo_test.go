package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/office"
)

// TestOfficeMailRepo_TaskSourcesAndAssignee 任务按来源 / 是否派给 AI 分列表、交办对象随行
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
			AssigneeKind: "agent", AssigneeID: "ag-1", AssigneeName: "内容组", SessionID: "sess-1",
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

	ids := func(f office.OfficeTaskFilter) []string {
		f.Statuses = []string{"open"}
		got, err := repo.ListOfficeTasks(ctx, "u-1", f, 10)
		if err != nil {
			t.Fatalf("list %+v: %v", f, err)
		}
		out := make([]string, 0, len(got))
		for _, g := range got {
			out = append(out, g.ID)
		}
		return out
	}
	if got := ids(office.OfficeTaskFilter{Sources: []string{"manual", "mail"}, Assigned: -1}); len(got) != 2 || got[0] != "otk_mail" || got[1] != "otk_m" {
		t.Fatalf("my tasks newest first: %v", got)
	}
	if got := ids(office.OfficeTaskFilter{Sources: []string{"note"}}); len(got) != 1 || got[0] != "otk_n" {
		t.Fatalf("notes: %v", got)
	}
	delegated, err := repo.ListOfficeTasks(ctx, "u-1", office.OfficeTaskFilter{Statuses: []string{"open"}, Assigned: 1}, 10)
	if err != nil || len(delegated) != 1 {
		t.Fatalf("delegated: %+v err=%v", delegated, err)
	}
	if d := delegated[0]; d.SourceRef != "sess-1" || d.SessionID != "sess-1" || d.AssigneeKind != "agent" || d.AssigneeID != "ag-1" || d.AssigneeName != "内容组" {
		t.Fatalf("assignee roundtrip: %+v", d)
	}

	if err := repo.SetOfficeTaskStatus(ctx, "otk_d", "u-2", "done", time.Now()); !errors.Is(err, office.ErrOfficeTaskNotFound) {
		t.Fatalf("other user's task must look missing: %v", err)
	}
	if err := repo.SetOfficeTaskStatus(ctx, "otk_d", "u-1", "done", time.Now()); err != nil {
		t.Fatalf("accept delegated task: %v", err)
	}
	if got := ids(office.OfficeTaskFilter{Assigned: 1}); len(got) != 0 {
		t.Fatalf("accepted task must leave the open list: %v", got)
	}
	got, err := repo.GetOfficeTask(ctx, "otk_d", "u-1")
	if err != nil || got.Status != "done" || got.AssigneeName != "内容组" {
		t.Fatalf("get after accept: %+v err=%v", got, err)
	}
}

// TestOfficeTaskRepo_LifecycleAndFiles 任务表的过滤（状态集合 / 来源 / 是否派给 AI）、
// 整行回写（含可空到期）、附件读写与附件数物化、删除级联 + 邮件回链清理、属主隔离。
func TestOfficeTaskRepo_LifecycleAndFiles(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(24 * time.Hour)

	mk := func(id, source, status, assignee string, at time.Time) *office.OfficeTask {
		task := &office.OfficeTask{ID: id, WorkspaceID: "ws", UserID: "u-1", Title: id, Source: source, Status: status,
			AssigneeKind: assignee, CreatedAt: at, UpdatedAt: at}
		if assignee != "" {
			task.AssigneeID, task.SessionID = "ag-1", "sess-"+id
		}
		if err := repo.CreateOfficeTask(ctx, task); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		return task
	}
	todo := mk("otk_todo", office.OfficeTaskSourceManual, office.OfficeTaskOpen, "", now)
	mk("otk_doing", office.OfficeTaskSourceMail, office.OfficeTaskDoing, "", now.Add(time.Second))
	mk("otk_ai", office.OfficeTaskSourceManual, office.OfficeTaskOpen, "agent", now.Add(2*time.Second))
	mk("otk_note", office.OfficeTaskSourceNote, office.OfficeTaskOpen, "", now.Add(3*time.Second))

	ids := func(f office.OfficeTaskFilter) []string {
		rows, err := repo.ListOfficeTasks(ctx, "u-1", f, 50)
		if err != nil {
			t.Fatalf("list %+v: %v", f, err)
		}
		out := []string{}
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	mine := office.OfficeTaskFilter{Statuses: []string{"open", "doing"}, Sources: []string{"manual", "mail"}, Assigned: -1}
	if got := ids(mine); len(got) != 2 || got[0] != "otk_doing" || got[1] != "otk_todo" {
		t.Fatalf("my active tasks = %v", got)
	}
	if got := ids(office.OfficeTaskFilter{Statuses: []string{"open"}, Assigned: 1}); len(got) != 1 || got[0] != "otk_ai" {
		t.Fatalf("assigned tasks = %v", got)
	}

	// 整行回写：到期时间写入后能读回，置空后读回 nil；他人的回写看作不存在。
	todo.Title, todo.DueAt, todo.Status = "改过的标题", &due, office.OfficeTaskDoing
	if err := repo.UpdateOfficeTask(ctx, todo); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetOfficeTask(ctx, todo.ID, "u-1")
	if err != nil || got.Title != "改过的标题" || got.DueAt == nil || !got.DueAt.Equal(due) || got.Status != "doing" {
		t.Fatalf("after update = %+v err=%v", got, err)
	}
	todo.DueAt = nil
	_ = repo.UpdateOfficeTask(ctx, todo)
	if got, _ = repo.GetOfficeTask(ctx, todo.ID, "u-1"); got.DueAt != nil {
		t.Fatalf("due should be cleared: %v", got.DueAt)
	}
	other := *todo
	other.UserID = "u-2"
	if err := repo.UpdateOfficeTask(ctx, &other); !errors.Is(err, office.ErrOfficeTaskNotFound) {
		t.Fatalf("cross-user update: %v", err)
	}

	// 附件：写入、元数据列表、含内容读取、附件数、属主隔离、删除。
	file := &office.OfficeTaskFile{ID: "otf_1", TaskID: todo.ID, UserID: "u-1", Filename: "a.png",
		ContentType: "image/png", Size: 3, Content: []byte{1, 2, 3}, CreatedAt: now}
	if err := repo.CreateOfficeTaskFile(ctx, file); err != nil {
		t.Fatal(err)
	}
	_ = repo.CreateOfficeTaskFile(ctx, &office.OfficeTaskFile{ID: "otf_2", TaskID: todo.ID, UserID: "u-1",
		Filename: "b.txt", Size: 1, Content: []byte("b"), CreatedAt: now.Add(time.Second)})
	files, err := repo.ListOfficeTaskFiles(ctx, todo.ID, "u-1")
	if err != nil || len(files) != 2 || files[0].ID != "otf_1" || files[0].Content != nil {
		t.Fatalf("files = %+v err=%v", files, err)
	}
	if got, _ = repo.GetOfficeTask(ctx, todo.ID, "u-1"); got.FileCount != 2 {
		t.Fatalf("file count = %d", got.FileCount)
	}
	full, err := repo.GetOfficeTaskFile(ctx, "otf_1", "u-1")
	if err != nil || string(full.Content) != "\x01\x02\x03" {
		t.Fatalf("get file = %+v err=%v", full, err)
	}
	if _, err := repo.GetOfficeTaskFile(ctx, "otf_1", "u-2"); !errors.Is(err, office.ErrOfficeTaskFileNotFound) {
		t.Fatalf("cross-user get file: %v", err)
	}
	if err := repo.DeleteOfficeTaskFile(ctx, "otf_2", "u-2"); !errors.Is(err, office.ErrOfficeTaskFileNotFound) {
		t.Fatalf("cross-user delete file: %v", err)
	}
	if err := repo.DeleteOfficeTaskFile(ctx, "otf_2", "u-1"); err != nil {
		t.Fatal(err)
	}

	// 删任务：附件级联、邮件回链清掉。
	m := mailTestMessage("omm_task", "u-1", "", 0, "转任务的邮件", now)
	m.TaskRef = todo.ID
	if _, err := repo.InsertMessageWithAttachmentsTx(ctx, m); err != nil {
		t.Fatalf("insert mail: %v", err)
	}
	if err := repo.DeleteOfficeTask(ctx, todo.ID, "u-2"); !errors.Is(err, office.ErrOfficeTaskNotFound) {
		t.Fatalf("cross-user delete: %v", err)
	}
	if err := repo.DeleteOfficeTask(ctx, todo.ID, "u-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetOfficeTaskFile(ctx, "otf_1", "u-1"); !errors.Is(err, office.ErrOfficeTaskFileNotFound) {
		t.Fatalf("files must cascade: %v", err)
	}
	if mail, err := repo.GetMailMessage(ctx, "omm_task", "u-1"); err != nil || mail.TaskRef != "" {
		t.Fatalf("mail task_ref must be cleared: %+v err=%v", mail, err)
	}
}
