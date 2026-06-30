package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// capturingBus is a minimal ActivityEventBus that records every published
// event so tests can assert on the payload. It is not safe for concurrent
// subscribers — only Publish is exercised by publishSpiritTeamAssembled.
type capturingBus struct {
	mu       sync.Mutex
	published []biz.ActivityEvent
}

func (b *capturingBus) Publish(_ context.Context, event biz.ActivityEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, event)
}

func (b *capturingBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	return nil, func() {}
}

func (b *capturingBus) DropCount() uint64 { return 0 }

func (b *capturingBus) snapshot() []biz.ActivityEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.ActivityEvent, len(b.published))
	copy(out, b.published)
	return out
}

// stubAgentReaderByKey is an in-memory AgentReader keyed by agent_key.
// Only GetAgentByAgentKey is exercised by publishSpiritTeamAssembled; the
// other methods return empty/err to satisfy the interface.
type stubAgentReaderByKey struct {
	byKey map[string]biz.Agent
}

func (s *stubAgentReaderByKey) SearchAgents(_ context.Context, _ biz.AgentListQuery) (biz.AgentListResult, error) {
	return biz.AgentListResult{}, nil
}
func (s *stubAgentReaderByKey) GetAgentByID(_ context.Context, _ string) (biz.Agent, error) {
	return biz.Agent{}, errors.New("not implemented")
}
func (s *stubAgentReaderByKey) GetAgentByAgentKey(_ context.Context, agentKey string) (biz.Agent, error) {
	if a, ok := s.byKey[agentKey]; ok {
		return a, nil
	}
	return biz.Agent{}, shared.ErrNotFound
}
func (s *stubAgentReaderByKey) ListExtrasForAgents(_ context.Context, _ []string) (map[string]biz.AgentListExtras, error) {
	return nil, nil
}
func (s *stubAgentReaderByKey) ListAgentsByIDs(_ context.Context, _ []string) ([]biz.Agent, error) {
	return nil, nil
}

// TestPublishSpiritTeamAssembled_UsesDisplayName verifies that the team_stage
// "assembled" event populates members[].agent_name with the catalog
// DisplayName rather than the raw agent_key. This is the regression guard for
// Problem 2 (TeamCard 成员名丑).
func TestPublishSpiritTeamAssembled_UsesDisplayName(t *testing.T) {
	bus := &capturingBus{}
	reader := &stubAgentReaderByKey{byKey: map[string]biz.Agent{
		"deep-researcher": {AgentKey: "deep-researcher", DisplayName: "深度研究员"},
		"code-writer":     {AgentKey: "code-writer", DisplayName: "代码编写者"},
	}}
	// spiritUC is intentionally nil: publishSpiritGraphStageSnapshot early-returns
	// when spiritUC is nil, so the only event captured is the team_stage event.
	a := &SpiritTeamAssembler{bus: bus, agentReader: reader, lg: loggateway.NewNoop()}

	a.publishSpiritTeamAssembled(
		context.Background(),
		"spirit-sess-1",
		biz.Team{ID: "team-1", DisplayName: "研究团队"},
		biz.Session{ID: "sess-1"},
		"sequential",
		"研究话题",
		"manual",
		[]string{"deep-researcher", "code-writer", "unknown-agent"},
		map[string]string{
			"deep-researcher": "agent-sess-deep-researcher",
			"code-writer":     "agent-sess-code-writer",
		},
	)

	events := bus.snapshot()
	// publishSpiritGraphStageSnapshot no-ops (spiritUC nil), so exactly one event.
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Activity.Kind != biz.ActivityKindTeamStage || ev.Activity.Stage != "assembled" {
		t.Fatalf("unexpected event: kind=%s stage=%s", ev.Activity.Kind, ev.Activity.Stage)
	}
	members, _ := ev.Activity.Meta["members"].([]map[string]any)
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	cases := []struct{ key, wantName string }{
		{"deep-researcher", "深度研究员"},
		{"code-writer", "代码编写者"},
		{"unknown-agent", "unknown-agent"}, // not in catalog → fallback to agent_key
	}
	for _, c := range cases {
		found := false
		for _, m := range members {
			if m["agent_key"] == c.key {
				if m["agent_name"] != c.wantName {
					t.Fatalf("member %s: agent_name=%v, want %s", c.key, m["agent_name"], c.wantName)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("member %s not found in members array", c.key)
		}
	}
}
