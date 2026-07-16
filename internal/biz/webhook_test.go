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
}

func (s *stubWebhookRepo) Create(_ context.Context, w WebhookConfig) (WebhookConfig, error) {
	s.created = w
	return w, nil
}
func (s *stubWebhookRepo) Get(context.Context, string) (WebhookConfig, error) {
	return WebhookConfig{}, nil
}
func (s *stubWebhookRepo) List(context.Context) ([]WebhookConfig, error) { return nil, nil }
func (s *stubWebhookRepo) ListPaged(_ context.Context, q WebhookListQuery) (WebhookListResult, error) {
	return WebhookListResult{Limit: q.Limit, Offset: q.Offset}, nil
}
func (s *stubWebhookRepo) ListEnabled(context.Context) ([]WebhookConfig, error) {
	return nil, nil
}
func (s *stubWebhookRepo) Update(context.Context, WebhookConfig) (WebhookConfig, error) {
	return WebhookConfig{}, nil
}
func (s *stubWebhookRepo) Delete(context.Context, string) error { return nil }
