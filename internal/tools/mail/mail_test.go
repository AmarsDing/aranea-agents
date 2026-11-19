package mail

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/decision"
)

type fakeService struct {
	listAcc  string
	page     int
	pageSize int
	readID   string
	query    string
	limit    int
	taskID   string
	titleOv  string
}

func (f *fakeService) ListMyMessages(_ context.Context, sessionID, accountID string, page, pageSize int) ([]MessageView, int64, error) {
	f.listAcc, f.page, f.pageSize = accountID, page, pageSize
	return []MessageView{{ID: "omm_1", Subject: "s", ReceivedAt: time.Now().UTC().Format(time.RFC3339)}}, 1, nil
}

func (f *fakeService) ReadMyMessage(_ context.Context, sessionID, messageID string) (*MessageDetailView, error) {
	f.readID = messageID
	return &MessageDetailView{MessageView: MessageView{ID: messageID, Subject: "s"}, Body: "b"}, nil
}

func (f *fakeService) SearchMyMessages(_ context.Context, sessionID, query string, limit int) ([]MessageView, error) {
	f.query, f.limit = query, limit
	return []MessageView{{ID: "omm_9", Subject: "hit"}}, nil
}

func (f *fakeService) ToTask(_ context.Context, sessionID, messageID, titleOverride string) (string, error) {
	f.taskID, f.titleOv = messageID, titleOverride
	return "otk_1", nil
}

func TestNewToolset_NilServicePruned(t *testing.T) {
	if NewToolset(nil) != nil {
		t.Fatal("nil service should prune read toolset")
	}
	if NewTaskToolset(nil) != nil {
		t.Fatal("nil service should prune task toolset")
	}
}

func TestMailToolset_Names(t *testing.T) {
	svc := &fakeService{}
	read := NewToolset(svc)
	if len(read) != 3 {
		t.Fatalf("read toolset size=%d, want 3", len(read))
	}
	names := map[string]bool{}
	for _, tl := range read {
		names[tl.Declaration().Name] = true
	}
	for _, want := range []string{"mail_list", "mail_read", "mail_search"} {
		if !names[want] {
			t.Fatalf("missing verb %s in %v", want, names)
		}
	}
	task := NewTaskToolset(svc)
	if len(task) != 1 || task[0].Declaration().Name != "mail_to_task" {
		t.Fatalf("task toolset: %+v", task)
	}
}

func TestMailToolset_SessionGuard(t *testing.T) {
	svc := &fakeService{}
	all := append(NewToolset(svc), NewTaskToolset(svc)...)
	for _, tl := range all {
		if _, err := tl.Call(context.Background(), []byte(`{"message_id":"m1","query":"q"}`)); err == nil {
			t.Fatalf("%s without session should fail", tl.Declaration().Name)
		}
	}
}

func TestMailToolset_CallThrough(t *testing.T) {
	svc := &fakeService{}
	ctx := decision.WithGateSessionID(context.Background(), "sess-1")

	list := NewToolset(svc)[0]
	res, err := list.Call(ctx, []byte(`{"account_id":"omac_1"}`))
	if err != nil {
		t.Fatalf("mail_list: %v", err)
	}
	if svc.page != 1 || svc.pageSize != 20 || svc.listAcc != "omac_1" {
		t.Fatalf("list args: acc=%q page=%d size=%d", svc.listAcc, svc.page, svc.pageSize)
	}
	m := res.(map[string]any)
	if m["total"].(int64) != 1 {
		t.Fatalf("list total: %v", m)
	}

	read := NewToolset(svc)[1]
	if _, err := read.Call(ctx, []byte(`{"message_id":"omm_1"}`)); err != nil {
		t.Fatalf("mail_read: %v", err)
	}
	if svc.readID != "omm_1" {
		t.Fatalf("read id %q", svc.readID)
	}

	search := NewToolset(svc)[2]
	if _, err := search.Call(ctx, []byte(`{"query":"资金"}`)); err != nil {
		t.Fatalf("mail_search: %v", err)
	}
	if svc.query != "资金" || svc.limit != 10 {
		t.Fatalf("search args: q=%q limit=%d", svc.query, svc.limit)
	}

	task := NewTaskToolset(svc)[0]
	res2, err := task.Call(ctx, []byte(`{"message_id":"omm_1","title_override":"跟进"}`))
	if err != nil {
		t.Fatalf("mail_to_task: %v", err)
	}
	if svc.taskID != "omm_1" || svc.titleOv != "跟进" {
		t.Fatalf("task args: id=%q title=%q", svc.taskID, svc.titleOv)
	}
	if res2.(map[string]any)["task_id"] != "otk_1" {
		t.Fatalf("task result: %v", res2)
	}
}
