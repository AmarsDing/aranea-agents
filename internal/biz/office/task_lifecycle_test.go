package office

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---------- 假仓储：任务改写、删除与附件 ----------

func (f *mailFakeRepos) UpdateOfficeTask(_ context.Context, t *OfficeTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.tasks[t.ID]
	if !ok || cur.UserID != t.UserID {
		return ErrOfficeTaskNotFound
	}
	c := *t
	c.FileCount = 0
	f.tasks[t.ID] = &c
	return nil
}

func (f *mailFakeRepos) DeleteOfficeTask(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.tasks[id]
	if !ok || cur.UserID != userID {
		return ErrOfficeTaskNotFound
	}
	delete(f.tasks, id)
	for fid, file := range f.taskFiles {
		if file.TaskID == id {
			delete(f.taskFiles, fid)
		}
	}
	for _, m := range f.messages {
		if m.TaskRef == id && m.UserID == userID {
			m.TaskRef = ""
		}
	}
	return nil
}

func (f *mailFakeRepos) ListOfficeTaskFiles(_ context.Context, taskID, userID string) ([]*OfficeTaskFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*OfficeTaskFile, 0)
	for _, file := range f.taskFiles {
		if file.TaskID == taskID && file.UserID == userID {
			c := *file
			c.Content = nil
			out = append(out, &c)
		}
	}
	return out, nil
}

func (f *mailFakeRepos) CreateOfficeTaskFile(_ context.Context, file *OfficeTaskFile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := *file
	f.taskFiles[file.ID] = &c
	return nil
}

func (f *mailFakeRepos) GetOfficeTaskFile(_ context.Context, id, userID string) (*OfficeTaskFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.taskFiles[id]
	if !ok || cur.UserID != userID {
		return nil, ErrOfficeTaskFileNotFound
	}
	c := *cur
	return &c, nil
}

func (f *mailFakeRepos) DeleteOfficeTaskFile(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.taskFiles[id]
	if !ok || cur.UserID != userID {
		return ErrOfficeTaskFileNotFound
	}
	delete(f.taskFiles, id)
	return nil
}

func (f *mailFakeRepos) ListOfficeTaskExecutions(_ context.Context, userID, taskID string, limit int) ([]*OfficeTaskExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*OfficeTaskExecution, 0)
	for _, e := range f.taskExecs {
		if e.UserID == userID && (taskID == "" || e.TaskID == taskID) && len(out) < limit {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *mailFakeRepos) AddOfficeTaskExecution(_ context.Context, e *OfficeTaskExecution) (*OfficeTaskExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cur := range f.taskExecs {
		if cur.TaskID == e.TaskID && cur.Kind == e.Kind && cur.RefID == e.RefID {
			cp := *cur
			return &cp, nil
		}
	}
	cp := *e
	f.taskExecs = append(f.taskExecs, &cp)
	return e, nil
}

func (f *mailFakeRepos) DeleteOfficeTaskExecution(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, cur := range f.taskExecs {
		if cur.ID == id && cur.UserID == userID {
			f.taskExecs = append(f.taskExecs[:i], f.taskExecs[i+1:]...)
			return nil
		}
	}
	return ErrOfficeTaskExecutionNotFound
}

func newTaskTestUsecase() (*MailMessageUsecase, *mailFakeRepos) {
	repo := newMailFakeRepos()
	uc := NewMailMessageUsecase(repo, repo, repo, nil, nil)
	tick := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	uc.now = func() time.Time { tick = tick.Add(time.Minute); return tick }
	return uc, repo
}

// 生命周期：待办 → 我在做 → 派给 AI（离开我的任务、进进行中）→ 收回 → 删除；以及改。
func TestOfficeTaskAssignUpdateDelete(t *testing.T) {
	uc, repo := newTaskTestUsecase()
	ctx := context.Background()
	due := time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC)

	task, err := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "整理周报", DueAt: &due})
	if err != nil || task.DueAt == nil || !task.DueAt.Equal(due) {
		t.Fatalf("create with due: %+v err=%v", task, err)
	}
	note, _ := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "随手记", Source: "note", DueAt: &due})
	if note.DueAt != nil {
		t.Fatalf("notes carry no due date: %+v", note)
	}

	if _, err := uc.SetTaskStatus(ctx, task.ID, "u-1", "doing"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := uc.ListTasks(ctx, "u-1", "", "", 0); len(rows) != 0 {
		t.Fatalf("open list must not include doing tasks: %d", len(rows))
	}
	if rows, _ := uc.ListTasks(ctx, "u-1", "active", "", 0); len(rows) != 1 || rows[0].Status != OfficeTaskDoing {
		t.Fatalf("active list = %+v", rows)
	}

	// 改：空标题拒绝；只改带了的字段；清到期优先。
	blank := "  "
	if _, err := uc.UpdateTask(ctx, task.ID, "u-1", UpdateOfficeTaskInput{Title: &blank}); !errors.Is(err, ErrOfficeTaskTitleRequired) {
		t.Fatalf("blank title: %v", err)
	}
	body := " 附上上周数据 "
	upd, err := uc.UpdateTask(ctx, task.ID, "u-1", UpdateOfficeTaskInput{Body: &body, ClearDue: true, DueAt: &due})
	if err != nil || upd.Title != "整理周报" || upd.Body != "附上上周数据" || upd.DueAt != nil {
		t.Fatalf("update = %+v err=%v", upd, err)
	}
	if _, err := uc.UpdateTask(ctx, task.ID, "u-2", UpdateOfficeTaskInput{Body: &body}); !errors.Is(err, ErrOfficeTaskNotFound) {
		t.Fatalf("update by another user: %v", err)
	}

	// 派给 AI：缺会话拒绝；随手记录不能派；成功后回到 open、离开我的任务、进 delegate 列表。
	if _, err := uc.AssignTask(ctx, task.ID, "u-1", AssignOfficeTaskInput{Kind: "agent", ID: "ag-1"}); !errors.Is(err, ErrOfficeTaskAssigneeNeeded) {
		t.Fatalf("assign without session: %v", err)
	}
	if _, err := uc.AssignTask(ctx, note.ID, "u-1", AssignOfficeTaskInput{Kind: "agent", ID: "ag-1", SessionID: "s"}); !errors.Is(err, ErrOfficeTaskNoteAssign) {
		t.Fatalf("assign note: %v", err)
	}
	assigned, err := uc.AssignTask(ctx, task.ID, "u-1", AssignOfficeTaskInput{Kind: " Team ", ID: " tm-1 ", Name: "内容组", SessionID: " sess-9 "})
	if err != nil || assigned.Status != OfficeTaskOpen || assigned.AssigneeKind != OfficeAssigneeTeam ||
		assigned.AssigneeID != "tm-1" || assigned.SessionID != "sess-9" || assigned.Source != OfficeTaskSourceManual {
		t.Fatalf("assigned = %+v err=%v", assigned, err)
	}
	if rows, _ := uc.ListTasks(ctx, "u-1", "active", "", 0); len(rows) != 0 {
		t.Fatalf("assigned task must leave my tasks: %d", len(rows))
	}
	if rows, _ := uc.ListTasks(ctx, "u-1", "", "delegate", 0); len(rows) != 1 || rows[0].ID != task.ID {
		t.Fatalf("delegate list = %+v", rows)
	}

	// 收回：清执行对象、进 doing；指令栏直接交办的任务收回后变成我建的。
	back, err := uc.AssignTask(ctx, task.ID, "u-1", AssignOfficeTaskInput{})
	if err != nil || back.Status != OfficeTaskDoing || back.AssigneeKind != "" || back.SessionID != "" {
		t.Fatalf("take back = %+v err=%v", back, err)
	}
	direct, _ := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{
		Title: "一句话交办", Source: "delegate", SourceRef: "sess-1", AssigneeKind: "agent", AssigneeID: "ag-1",
	})
	if direct.SessionID != "sess-1" || direct.SourceRef != "sess-1" {
		t.Fatalf("delegate create must record the session: %+v", direct)
	}
	back, _ = uc.AssignTask(ctx, direct.ID, "u-1", AssignOfficeTaskInput{})
	if back.Source != OfficeTaskSourceManual || back.SourceRef != "" {
		t.Fatalf("taken-back direct delegation = %+v", back)
	}

	// 删：邮件回链清掉；别人删不了；删后不存在。
	repo.messages["m-1"] = &MailMessage{ID: "m-1", UserID: "u-1", TaskRef: task.ID}
	if err := uc.DeleteTask(ctx, task.ID, "u-2"); !errors.Is(err, ErrOfficeTaskNotFound) {
		t.Fatalf("delete by another user: %v", err)
	}
	if err := uc.DeleteTask(ctx, task.ID, "u-1"); err != nil {
		t.Fatal(err)
	}
	if repo.messages["m-1"].TaskRef != "" {
		t.Fatal("mail task_ref must be cleared when its task is deleted")
	}
	if _, err := uc.GetTask(ctx, task.ID, "u-1"); !errors.Is(err, ErrOfficeTaskNotFound) {
		t.Fatalf("deleted task: %v", err)
	}
}

func TestOfficeTaskFiles(t *testing.T) {
	uc, _ := newTaskTestUsecase()
	ctx := context.Background()
	task, _ := uc.CreateTask(ctx, "u-1", "ws", NewOfficeTaskInput{Title: "带附件"})

	if _, err := uc.UploadTaskFile(ctx, task.ID, "u-1", "a.txt", "", nil); !errors.Is(err, ErrOfficeTaskFileEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := uc.UploadTaskFile(ctx, task.ID, "u-1", "big.mp4", "video/mp4", make([]byte, OfficeTaskFileMaxBytes+1)); !errors.Is(err, ErrOfficeTaskFileTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	if _, err := uc.UploadTaskFile(ctx, task.ID, "u-2", "a.txt", "", []byte("x")); !errors.Is(err, ErrOfficeTaskNotFound) {
		t.Fatalf("upload to another user's task: %v", err)
	}
	png := []byte("\x89PNG\r\n\x1a\n0000")
	f, err := uc.UploadTaskFile(ctx, task.ID, "u-1", `C:\Users\me\截图.png`, "application/octet-stream", png)
	if err != nil || f.Filename != "截图.png" || f.ContentType != "image/png" || f.Size != int64(len(png)) || f.Content != nil {
		t.Fatalf("upload = %+v err=%v", f, err)
	}
	got, err := uc.GetTaskFile(ctx, f.ID, "u-1")
	if err != nil || string(got.Content) != string(png) {
		t.Fatalf("get = %+v err=%v", got, err)
	}
	if _, err := uc.GetTaskFile(ctx, f.ID, "u-2"); !errors.Is(err, ErrOfficeTaskFileNotFound) {
		t.Fatalf("get by another user: %v", err)
	}
	if rows, _ := uc.ListTasks(ctx, "u-1", "", "", 0); len(rows) != 1 || rows[0].FileCount != 1 {
		t.Fatalf("file count = %+v", rows)
	}
	for i := 1; i < officeTaskFilesMax; i++ {
		if _, err := uc.UploadTaskFile(ctx, task.ID, "u-1", "n.txt", "text/plain", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := uc.UploadTaskFile(ctx, task.ID, "u-1", "n.txt", "text/plain", []byte("x")); !errors.Is(err, ErrOfficeTaskFilesFull) {
		t.Fatalf("21st file: %v", err)
	}
	if err := uc.DeleteTaskFile(ctx, f.ID, "u-2"); !errors.Is(err, ErrOfficeTaskFileNotFound) {
		t.Fatalf("delete by another user: %v", err)
	}
	if err := uc.DeleteTaskFile(ctx, f.ID, "u-1"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := uc.ListTaskFiles(ctx, task.ID, "u-1"); len(rows) != officeTaskFilesMax-1 {
		t.Fatalf("after delete: %d", len(rows))
	}
}

func TestTaskExecutions_AddListRemove(t *testing.T) {
	u, _ := newTaskTestUsecase()
	ctx := context.Background()
	task, err := u.CreateTask(ctx, "u1", "w1", NewOfficeTaskInput{Title: "发版"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.AddTaskExecution(ctx, task.ID, "u1", NewOfficeTaskExecutionInput{Kind: "nope", RefID: "x"}); err != ErrOfficeTaskExecutionKind {
		t.Fatalf("unknown kind: got %v", err)
	}
	if _, err := u.AddTaskExecution(ctx, task.ID, "u1", NewOfficeTaskExecutionInput{Kind: OfficeExecGraph}); err != ErrOfficeTaskExecutionRef {
		t.Fatalf("empty ref: got %v", err)
	}
	if _, err := u.AddTaskExecution(ctx, task.ID, "u2", NewOfficeTaskExecutionInput{Kind: OfficeExecGraph, RefID: "x1"}); err != ErrOfficeTaskNotFound {
		t.Fatalf("someone else's task: got %v", err)
	}
	in := NewOfficeTaskExecutionInput{Kind: " Graph_Execution ", RefID: "x1", Executor: "flow:g1", ExecutorName: "发版流程", Hand: "【任务】发版"}
	first, err := u.AddTaskExecution(ctx, task.ID, "u1", in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != OfficeExecGraph || first.RefID != "x1" || first.Hand != "【任务】发版" {
		t.Fatalf("stored row: %+v", first)
	}
	again, err := u.AddTaskExecution(ctx, task.ID, "u1", in)
	if err != nil || again.ID != first.ID {
		t.Fatalf("adding the same record twice must return the first row: %v %+v", err, again)
	}
	rows, err := u.ListTaskExecutions(ctx, "u1", "", 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list all: %v %d", err, len(rows))
	}
	if rows, _ := u.ListTaskExecutions(ctx, "u2", "", 0); len(rows) != 0 {
		t.Fatalf("other users must not see it: %d", len(rows))
	}
	if err := u.RemoveTaskExecution(ctx, first.ID, "u2"); err != ErrOfficeTaskExecutionNotFound {
		t.Fatalf("remove by other user: got %v", err)
	}
	if err := u.RemoveTaskExecution(ctx, first.ID, "u1"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTask_SessionlessExecutors(t *testing.T) {
	u, _ := newTaskTestUsecase()
	ctx := context.Background()
	for _, kind := range []string{OfficeAssigneeTool, OfficeAssigneeRun, OfficeAssigneeFlow} {
		task, err := u.CreateTask(ctx, "u1", "w1", NewOfficeTaskInput{
			Title: "发版", Source: OfficeTaskSourceDelegate, AssigneeKind: kind, AssigneeID: "x", AssigneeName: "发版流程",
		})
		if err != nil {
			t.Fatalf("%s without a session must be accepted: %v", kind, err)
		}
		if task.AssigneeKind != kind || task.SessionID != "" || task.SourceRef != "" {
			t.Fatalf("%s: stored %+v", kind, task)
		}
	}
	// 交给同事 / 团队仍然必须有承载会话
	if _, err := u.CreateTask(ctx, "u1", "w1", NewOfficeTaskInput{
		Title: "发版", Source: OfficeTaskSourceDelegate, AssigneeKind: OfficeAssigneeAgent, AssigneeID: "a1",
	}); err != ErrOfficeTaskAssigneeNeeded {
		t.Fatalf("agent without a session: got %v", err)
	}
	if _, err := u.CreateTask(ctx, "u1", "w1", NewOfficeTaskInput{
		Title: "发版", Source: OfficeTaskSourceDelegate, AssigneeKind: OfficeAssigneeFlow,
	}); err != ErrOfficeTaskAssigneeNeeded {
		t.Fatalf("flow without an id: got %v", err)
	}
}
