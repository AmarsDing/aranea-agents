package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type stubHookRepo struct {
	items []Hook
}

func (s *stubHookRepo) ListHooks(context.Context) ([]Hook, error) { return s.items, nil }
func (s *stubHookRepo) GetHook(context.Context, string) (Hook, error) {
	return Hook{}, nil
}
func (s *stubHookRepo) CreateHook(context.Context, Hook) (Hook, error) { return Hook{}, nil }
func (s *stubHookRepo) UpdateHook(context.Context, Hook) (Hook, error) { return Hook{}, nil }
func (s *stubHookRepo) DeleteHook(context.Context, string) error       { return nil }

func TestHookResolver_Resolve_filtersAgent(t *testing.T) {
	repo := &stubHookRepo{items: []Hook{
		{
			Key:     "h1",
			Enabled: true,
			Status:  "active",
			ConfigJSON: `{"callback_point":"before_agent","condition":{"agent_id":"a1"},"action":{"type":"log"}}`,
		},
		{
			Key:     "h2",
			Enabled: true,
			Status:  "active",
			ConfigJSON: `{"callback_point":"before_model","action":{"type":"log"}}`,
		},
		{
			Key:        "h3",
			Enabled:    false,
			Status:     "active",
			ConfigJSON: `{"callback_point":"before_agent","action":{"type":"log"}}`,
		},
	}}
	r := NewHookResolver(NewHookUsecase(repo), loggateway.NewNoop())
	if err := r.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.Resolve("a1", "ak1")
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	gotOther := r.Resolve("other", "ak1")
	if len(gotOther) != 1 {
		t.Fatalf("len=%d want 1 (global hook only)", len(gotOther))
	}
}
