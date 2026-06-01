package chatactivity

import (
	"context"
	"testing"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

func TestCancelRunningActivityMessages_NilSessions(t *testing.T) {
	n, err := CancelRunningActivityMessages(context.Background(), nil, "sess1")
	if err != nil || n != 0 {
		t.Fatalf("expected 0,nil got %d,%v", n, err)
	}
}

func TestCancelRunningActivityMessages_EmptySessionID(t *testing.T) {
	n, err := CancelRunningActivityMessages(context.Background(), nil, "  ")
	if err != nil || n != 0 {
		t.Fatalf("expected 0,nil got %d,%v", n, err)
	}
}

func TestNewStreamConsumeOptions_Nil(t *testing.T) {
	opts := NewStreamConsumeOptions(nil, nil, nil)
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestNewStreamConsumeOptions_WithDeps(t *testing.T) {
	opts := NewStreamConsumeOptions(&biz.ToolUsecase{}, &stubAgentRepo{}, nil)
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
	if opts.MetaResolver == nil {
		t.Fatal("expected non-nil resolver")
	}
}

func TestStreamOptsFactoryAdapter(t *testing.T) {
	a := &StreamOptsFactoryAdapter{
		Tools:  &biz.ToolUsecase{},
		Agents: &stubAgentRepo{},
	}
	opts := a.NewStreamConsumeOptions()
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestStreamOptsFactoryAdapter_Nil(t *testing.T) {
	a := &StreamOptsFactoryAdapter{}
	opts := a.NewStreamConsumeOptions()
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestCatalogActivityMetaResolver_Nil(t *testing.T) {
	var r *catalogActivityMetaResolver
	if r.ResolveDisplayLabel(context.Background(), "tool1") != "" {
		t.Fatal("expected empty for nil resolver")
	}
	if r.ResolveAgentDisplayName(context.Background(), "agent1") != "" {
		t.Fatal("expected empty for nil resolver")
	}
	if r.ResolveAgentID(context.Background(), "agent1") != "" {
		t.Fatal("expected empty for nil resolver")
	}
}

func TestCatalogActivityMetaResolver_EmptyInput(t *testing.T) {
	r := newCatalogActivityMetaResolver(nil, nil)
	if r.ResolveDisplayLabel(context.Background(), "") != "" {
		t.Fatal("expected empty for empty tool name")
	}
	if r.ResolveAgentDisplayName(context.Background(), "") != "" {
		t.Fatal("expected empty for empty agent key")
	}
	if r.ResolveAgentID(context.Background(), "") != "" {
		t.Fatal("expected empty for empty agent key")
	}
}

func TestSessionActivityPersister_Nil(t *testing.T) {
	var p *sessionActivityPersister
	if p.UpsertActivity(context.Background(), chatagent.ProjectMeta{}, event.EnvelopeToolCall{}) != nil {
		t.Fatal("expected nil for nil persister")
	}
}

func TestCatalogActivityMetaResolver_WithAgentRepo(t *testing.T) {
	r := newCatalogActivityMetaResolver(nil, &stubAgentRepo{
		names: map[string]string{"test-agent": "Agent test-agent"},
		ids:   map[string]string{"test-agent": "id-test-agent"},
	})
	name := r.ResolveAgentDisplayName(context.Background(), "test-agent")
	if name != "Agent test-agent" {
		t.Fatalf("expected 'Agent test-agent', got %q", name)
	}
	id := r.ResolveAgentID(context.Background(), "test-agent")
	if id != "id-test-agent" {
		t.Fatalf("expected 'id-test-agent', got %q", id)
	}
}

func TestCatalogActivityMetaResolver_CachesResult(t *testing.T) {
	r := newCatalogActivityMetaResolver(nil, &stubAgentRepo{
		names: map[string]string{"cache-agent": "Cache Agent"},
		ids:   map[string]string{"cache-agent": "id-cache-agent"},
	})
	name1 := r.ResolveAgentDisplayName(context.Background(), "cache-agent")
	name2 := r.ResolveAgentDisplayName(context.Background(), "cache-agent")
	if name1 != name2 {
		t.Fatalf("cache should return same result: %q vs %q", name1, name2)
	}
}
