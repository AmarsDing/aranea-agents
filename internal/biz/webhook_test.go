package biz

import (
	"context"
	"testing"
)

func TestMergeWebhookPatch_PreservesEnabledWhenUnset(t *testing.T) {
	cur := WebhookConfig{
		ID:      "wh-1",
		Name:    "Alerts",
		URL:     "https://example.com/hook",
		Enabled: true,
	}
	merged := mergeWebhookPatch(cur, WebhookUpdatePatch{
		ID:   "wh-1",
		Name: "Renamed",
	})
	if merged.Name != "Renamed" {
		t.Fatalf("name=%q", merged.Name)
	}
	if !merged.Enabled {
		t.Fatal("expected enabled to remain true when patch.Enabled is nil")
	}
}

func TestMergeWebhookPatch_UpdatesEnabledWhenSet(t *testing.T) {
	cur := WebhookConfig{ID: "wh-1", Enabled: true}
	disabled := false
	merged := mergeWebhookPatch(cur, WebhookUpdatePatch{ID: "wh-1", Enabled: &disabled})
	if merged.Enabled {
		t.Fatal("expected enabled=false")
	}
}

func TestWebhookSubscribes_GraphTaskStatus(t *testing.T) {
	types := `["graph.task.status"]`
	if !WebhookSubscribes(types, WebhookEventGraphTaskStatus) {
		t.Fatal("expected subscription to graph.task.status")
	}
}

func TestWebhookUsecase_Create_PreservesDisabled(t *testing.T) {
	repo := &stubWebhookRepo{}
	uc := NewWebhookUsecase(repo, repo)
	_, err := uc.Create(context.Background(), WebhookConfig{
		Name:    "Alerts",
		URL:     "https://example.com/hook",
		Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.created.Enabled {
		t.Fatal("expected enabled=false when caller sets it")
	}
}

type stubWebhookRepo struct {
	created WebhookConfig
	list    []WebhookConfig
	get     WebhookConfig
}

func (s *stubWebhookRepo) Create(_ context.Context, w WebhookConfig) (WebhookConfig, error) {
	s.created = w
	return w, nil
}
func (s *stubWebhookRepo) Get(context.Context, string) (WebhookConfig, error) {
	return s.get, nil
}
func (s *stubWebhookRepo) List(context.Context) ([]WebhookConfig, error) { return s.list, nil }
func (s *stubWebhookRepo) ListPaged(_ context.Context, q WebhookListQuery) (WebhookListResult, error) {
	return WebhookListResult{Limit: q.Limit, Offset: q.Offset}, nil
}
func (s *stubWebhookRepo) ListEnabled(context.Context) ([]WebhookConfig, error) {
	return nil, nil
}
func (s *stubWebhookRepo) Update(_ context.Context, w WebhookConfig) (WebhookConfig, error) {
	return w, nil
}
func (s *stubWebhookRepo) Delete(context.Context, string) error { return nil }

func TestWebhookUsecase_Create_RejectsDuplicateName(t *testing.T) {
	repo := &stubWebhookRepo{list: []WebhookConfig{
		{ID: "wh-1", Name: "Alerts", URL: "https://example.com/a"},
	}}
	uc := NewWebhookUsecase(repo, repo)
	_, err := uc.Create(context.Background(), WebhookConfig{
		Name: "alerts", // case-insensitive duplicate
		URL:  "https://example.com/b",
	})
	if err == nil {
		t.Fatal("expected duplicate name conflict, got nil")
	}
}

func TestWebhookUsecase_Update_RejectsDuplicateName(t *testing.T) {
	repo := &stubWebhookRepo{
		get: WebhookConfig{ID: "wh-1", Name: "Alerts", URL: "https://example.com/a"},
		list: []WebhookConfig{
			{ID: "wh-1", Name: "Alerts", URL: "https://example.com/a"},
			{ID: "wh-2", Name: "Pager", URL: "https://example.com/b"},
		},
	}
	uc := NewWebhookUsecase(repo, repo)
	_, err := uc.Update(context.Background(), WebhookUpdatePatch{ID: "wh-1", Name: "pager"})
	if err == nil {
		t.Fatal("expected duplicate name conflict, got nil")
	}
}

func TestWebhookUsecase_Update_AllowsUnchangedName(t *testing.T) {
	repo := &stubWebhookRepo{
		get: WebhookConfig{ID: "wh-1", Name: "Alerts", URL: "https://example.com/a"},
		list: []WebhookConfig{
			{ID: "wh-1", Name: "Alerts", URL: "https://example.com/a"},
		},
	}
	uc := NewWebhookUsecase(repo, repo)
	if _, err := uc.Update(context.Background(), WebhookUpdatePatch{ID: "wh-1", Name: "Alerts"}); err != nil {
		t.Fatalf("keeping own name must succeed: %v", err)
	}
}
