package biz

import (
	"context"
	"testing"

	"aranea-agents/internal/workspace"
)

// 出站推送按工作区隔离：别的工作区的推送看不到、改不了、删不掉；存量空串行归 default。
func TestWebhookUsecase_ScopedToCallerWorkspace(t *testing.T) {
	repo := &stubWebhookRepo{
		list: []WebhookConfig{
			{ID: "a", Name: "mine", WorkspaceID: "ws-a"},
			{ID: "b", Name: "theirs", WorkspaceID: "ws-b"},
			{ID: "legacy", Name: "legacy", WorkspaceID: ""},
		},
		get: WebhookConfig{ID: "b", Name: "theirs", WorkspaceID: "ws-b"},
	}
	uc := NewWebhookUsecase(repo, repo)
	ctxA := workspace.WithContext(context.Background(), "ws-a")

	got, err := uc.List(ctxA)
	if err != nil || len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("ws-a should only list its own webhook, got %+v err=%v", got, err)
	}
	if def, _ := uc.List(workspace.WithContext(context.Background(), workspace.DefaultWorkspaceID)); len(def) != 1 || def[0].ID != "legacy" {
		t.Fatalf("legacy rows belong to default, got %+v", def)
	}
	if all, _ := uc.List(workspace.WithSystemWorkspace(context.Background())); len(all) != 3 {
		t.Fatalf("system caller sees all, got %d", len(all))
	}
	if _, err := uc.Get(ctxA, "b"); err == nil {
		t.Fatal("reading another workspace's webhook must fail")
	}
	if err := uc.Delete(ctxA, "b"); err == nil {
		t.Fatal("deleting another workspace's webhook must fail")
	}
	enabled := false
	if _, err := uc.Update(ctxA, WebhookUpdatePatch{ID: "b", Enabled: &enabled}); err == nil {
		t.Fatal("updating another workspace's webhook must fail")
	}

	created, err := uc.Create(ctxA, WebhookConfig{Name: "theirs", URL: "https://example.com/h", EventTypesJSON: `["run.failed"]`})
	if err != nil {
		t.Fatalf("same name in another workspace is allowed: %v", err)
	}
	if created.WorkspaceID != "ws-a" {
		t.Fatalf("create must stamp caller workspace, got %q", created.WorkspaceID)
	}
}

type sessionWSRepo struct {
	stubWebhookRepo
	sessions map[string]string
}

func (r *sessionWSRepo) SessionWorkspaceID(_ context.Context, id string) (string, bool) {
	ws, ok := r.sessions[id]
	return ws, ok
}

func TestWebhookDispatcher_EventWorkspace(t *testing.T) {
	d := &WebhookDispatcher{repo: &sessionWSRepo{sessions: map[string]string{"s-b": "ws-b", "s-legacy": ""}}}
	ctx := context.Background()
	cases := []struct{ ctxWS, session, want string }{
		{"", "s-b", "ws-b"},                                             // 会话优先：后台 ctx 不带租户
		{"ws-a", "s-b", "ws-b"},                                         // 会话比 ctx 可信
		{"", "s-legacy", workspace.DefaultWorkspaceID},                  // 存量会话无工作区 → default
		{"ws-a", "unknown", "ws-a"},                                     // 查不到会话时用 ctx 的租户
		{workspace.SystemWorkspaceID, "", workspace.DefaultWorkspaceID}, // system 不是租户
		{"", "", workspace.DefaultWorkspaceID},
	}
	for _, c := range cases {
		if got := d.eventWorkspace(ctx, c.ctxWS, c.session); got != c.want {
			t.Fatalf("eventWorkspace(%q,%q)=%q want %q", c.ctxWS, c.session, got, c.want)
		}
	}
}
