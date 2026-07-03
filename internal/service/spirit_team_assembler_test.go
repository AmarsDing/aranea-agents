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
	mu        sync.Mutex
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

// capturingActivityRepo captures UpsertActivity calls so tests can assert on
// the synchronously-persisted v1 Activity (used by publishSpiritTeamAssembled
// to store the rich Meta payload that v2 TeamStage drops).
type capturingActivityRepo struct {
	mu         sync.Mutex
	activities []biz.Activity
}

func (r *capturingActivityRepo) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activities = append(r.activities, a)
	return a, nil
}

func (r *capturingActivityRepo) snapshot() []biz.Activity {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]biz.Activity, len(r.activities))
	copy(out, r.activities)
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
//
// Phase 3b-D Task 10: publishSpiritTeamAssembled now publishes a v2
// TeamStageCreatedEvent (no Meta) AND synchronously persists a v1 Activity
// (carrying the rich Meta with members). This test asserts on the
// sync-persisted Activity since that is where the members data lives.
func TestPublishSpiritTeamAssembled_UsesDisplayName(t *testing.T) {
	bus := &capturingBus{}
	eventBus := &captureEventBus{}
	activityRepo := &capturingActivityRepo{}
	reader := &stubAgentReaderByKey{byKey: map[string]biz.Agent{
		"deep-researcher": {AgentKey: "deep-researcher", DisplayName: "深度研究员"},
		"code-writer":     {AgentKey: "code-writer", DisplayName: "代码编写者"},
	}}
	// spiritUC is intentionally nil: publishSpiritGraphStageSnapshot early-returns
	// when spiritUC is nil, so no graph_stage event is captured on either bus.
	a := &SpiritTeamAssembler{bus: bus, eventBus: eventBus, activityRepo: activityRepo, agentReader: reader, lg: loggateway.NewNoop()}

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

	// v2 EventBus should have exactly one TeamStageCreatedEvent.
	v2Events := eventBus.snapshot()
	if len(v2Events) != 1 {
		t.Fatalf("expected 1 v2 event, got %d", len(v2Events))
	}
	tsEv, ok := v2Events[0].(*biz.TeamStageCreatedEvent)
	if !ok {
		t.Fatalf("expected *biz.TeamStageCreatedEvent, got %T", v2Events[0])
	}
	if tsEv.TeamStage.Stage != biz.TeamStageStageAssembled {
		t.Fatalf("expected stage %s, got %s", biz.TeamStageStageAssembled, tsEv.TeamStage.Stage)
	}

	// v1 capturingBus should be empty (graph_stage early-returned because spiritUC is nil).
	if len(bus.snapshot()) != 0 {
		t.Fatalf("expected 0 v1 bus events, got %d", len(bus.snapshot()))
	}

	// The synchronously-persisted v1 Activity carries the members Meta.
	activities := activityRepo.snapshot()
	if len(activities) != 1 {
		t.Fatalf("expected 1 sync-persisted Activity, got %d", len(activities))
	}
	activity := activities[0]
	if activity.Kind != biz.ActivityKindTeamStage || activity.Stage != "assembled" {
		t.Fatalf("unexpected activity: kind=%s stage=%s", activity.Kind, activity.Stage)
	}
	members, _ := activity.Meta["members"].([]map[string]any)
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
