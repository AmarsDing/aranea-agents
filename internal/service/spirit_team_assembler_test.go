package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

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
// Problem 2 (TeamCard display name).
//
// publishSpiritTeamAssembled publishes a v2 TeamStageCreatedEvent with Members.
func TestPublishSpiritTeamAssembled_UsesDisplayName(t *testing.T) {
	eventBus := &captureEventBus{}
	reader := &stubAgentReaderByKey{byKey: map[string]biz.Agent{
		"deep-researcher": {AgentKey: "deep-researcher", DisplayName: "Deep Researcher"},
		"code-writer":     {AgentKey: "code-writer", DisplayName: "Code Writer"},
	}}
	a := &SpiritTeamAssembler{eventBus: eventBus, agentReader: reader, lg: loggateway.NewNoop()}

	a.publishSpiritTeamAssembled(
		context.Background(),
		"spirit-sess-1",
		biz.Team{ID: "team-1", DisplayName: "Research Team"},
		biz.Session{ID: "sess-1"},
		"sequential",
		"research topic",
		"manual",
		[]string{"deep-researcher", "code-writer", "unknown-agent"},
		map[string]string{
			"deep-researcher": "agent-sess-deep-researcher",
			"code-writer":     "agent-sess-code-writer",
		},
	)

	v2Events := eventBus.snapshot()
	if len(v2Events) < 1 {
		t.Fatalf("expected >=1 v2 event, got %d", len(v2Events))
	}
	var tsEv *biz.TeamStageCreatedEvent
	for _, e := range v2Events {
		if ev, ok := e.(*biz.TeamStageCreatedEvent); ok {
			tsEv = ev
			break
		}
	}
	if tsEv == nil {
		t.Fatalf("expected *biz.TeamStageCreatedEvent among %d events", len(v2Events))
	}
	if tsEv.TeamStage.Stage != biz.TeamStageStageAssembled {
		t.Fatalf("expected stage %s, got %s", biz.TeamStageStageAssembled, tsEv.TeamStage.Stage)
	}
	members := tsEv.TeamStage.Members
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	cases := []struct{ key, wantName string }{
		{"deep-researcher", "Deep Researcher"},
		{"code-writer", "Code Writer"},
		{"unknown-agent", "unknown-agent"},
	}
	for _, c := range cases {
		found := false
		for _, m := range members {
			if m.AgentKey == c.key {
				if m.AgentName != c.wantName {
					t.Fatalf("member %s: agent_name=%v, want %s", c.key, m.AgentName, c.wantName)
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
