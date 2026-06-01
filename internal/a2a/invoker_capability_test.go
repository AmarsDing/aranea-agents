package a2a

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type mockTurnRunner struct {
	lastAgentID string
	lastInput   string
}

func (m *mockTurnRunner) RunAgentTurn(_ context.Context, agentID, input string, _ int) (string, error) {
	m.lastAgentID = agentID
	m.lastInput = input
	return "ok", nil
}

type memA2ARepo struct {
	cards map[string]biz.A2AAgentCard
}

func (m *memA2ARepo) UpsertAgentCard(_ context.Context, card biz.A2AAgentCard) (biz.A2AAgentCard, error) {
	m.cards[card.AgentID] = card
	return card, nil
}
func (m *memA2ARepo) GetAgentCard(_ context.Context, agentID string) (biz.A2AAgentCard, error) {
	c, ok := m.cards[agentID]
	if !ok {
		return biz.A2AAgentCard{}, biz.ErrNotFound
	}
	return c, nil
}
func (m *memA2ARepo) ListEnabledCards(context.Context, string, string) ([]biz.A2AAgentCard, error) {
	return nil, nil
}
func (m *memA2ARepo) MapEndpointEnabled(_ context.Context, agentIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(agentIDs))
	for _, id := range agentIDs {
		if c, ok := m.cards[id]; ok {
			out[id] = c.Enabled
		}
	}
	return out, nil
}
func (m *memA2ARepo) CreateRemoteAgent(_ context.Context, agent biz.A2ARemoteAgent) (biz.A2ARemoteAgent, error) {
	return agent, nil
}
func (m *memA2ARepo) ListRemoteAgents(context.Context, string) ([]biz.A2ARemoteAgent, error) {
	return nil, nil
}
func (m *memA2ARepo) DeleteRemoteAgent(context.Context, string) error { return nil }
func (m *memA2ARepo) GetRemoteAgent(context.Context, string) (biz.A2ARemoteAgent, error) {
	return biz.A2ARemoteAgent{}, biz.ErrNotFound
}
func (m *memA2ARepo) DiscoverRemoteCard(context.Context, biz.RemoteCardDiscoverInput) (biz.A2AAgentCard, error) {
	return biz.A2AAgentCard{}, nil
}
func (m *memA2ARepo) CreateInvocation(context.Context, biz.A2AInvocation) (biz.A2AInvocation, error) {
	return biz.A2AInvocation{}, nil
}
func (m *memA2ARepo) UpdateInvocation(context.Context, biz.A2AInvocation) error { return nil }
func (m *memA2ARepo) InsertAudit(context.Context, biz.A2AAuditEntry) error      { return nil }
func (m *memA2ARepo) ListAudit(context.Context, string, string, int, int) ([]biz.A2AAuditEntry, int, error) {
	return nil, 0, nil
}
func (m *memA2ARepo) UpdateRemoteAgentHealth(context.Context, string, bool, string) error {
	return nil
}

func TestNewInvoker_RequiresEnabledCapability(t *testing.T) {
	t.Parallel()
	repo := &memA2ARepo{cards: map[string]biz.A2AAgentCard{
		"callee": {
			AgentID: "callee",
			Enabled: true,
			Capabilities: []biz.A2ACapability{
				{Name: "chat"},
			},
		},
	}}
	uc := biz.NewA2AUsecase(repo)
	runner := &mockTurnRunner{}
	inv := NewInvoker(runner, uc, nil, loggateway.NewNoop())
	ctx := WithCallerAgentID(context.Background(), "caller")

	_, err := inv(ctx, "callee", "missing", `{}`, 30)
	if err == nil || !strings.Contains(err.Error(), "capability is not advertised") {
		t.Fatalf("expected capability error, got %v", err)
	}

	out, err := inv(ctx, "callee", "chat", `{"message":"hi"}`, 30)
	if err != nil {
		t.Fatal(err)
	}
	if runner.lastAgentID != "callee" || runner.lastInput != "hi" {
		t.Fatalf("runner args: id=%q input=%q", runner.lastAgentID, runner.lastInput)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("unexpected output %q", out)
	}
}
