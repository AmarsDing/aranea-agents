package a2a

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

type resolveRepo struct {
	cards   map[string]biz.A2AAgentCard
	remotes map[string]biz.A2ARemoteAgent
}

func (r *resolveRepo) UpsertAgentCard(context.Context, biz.A2AAgentCard) (biz.A2AAgentCard, error) {
	return biz.A2AAgentCard{}, nil
}
func (r *resolveRepo) GetAgentCard(_ context.Context, id string) (biz.A2AAgentCard, error) {
	c, ok := r.cards[id]
	if !ok {
		return biz.A2AAgentCard{}, biz.ErrNotFound
	}
	return c, nil
}
func (r *resolveRepo) ListEnabledCards(context.Context, string, string) ([]biz.A2AAgentCard, error) {
	return nil, nil
}
func (r *resolveRepo) MapEndpointEnabled(context.Context, []string) (map[string]bool, error) {
	return nil, nil
}
func (r *resolveRepo) CreateRemoteAgent(context.Context, biz.A2ARemoteAgent) (biz.A2ARemoteAgent, error) {
	return biz.A2ARemoteAgent{}, nil
}
func (r *resolveRepo) ListRemoteAgents(context.Context, string) ([]biz.A2ARemoteAgent, error) {
	return nil, nil
}
func (r *resolveRepo) DeleteRemoteAgent(context.Context, string) error { return nil }
func (r *resolveRepo) GetRemoteAgent(_ context.Context, id string) (biz.A2ARemoteAgent, error) {
	rmt, ok := r.remotes[id]
	if !ok {
		return biz.A2ARemoteAgent{}, biz.ErrNotFound
	}
	return rmt, nil
}
func (r *resolveRepo) DiscoverRemoteCard(context.Context, biz.RemoteCardDiscoverInput) (biz.A2AAgentCard, error) {
	return biz.A2AAgentCard{}, nil
}
func (r *resolveRepo) CreateInvocation(context.Context, biz.A2AInvocation) (biz.A2AInvocation, error) {
	return biz.A2AInvocation{}, nil
}
func (r *resolveRepo) UpdateInvocation(context.Context, biz.A2AInvocation) error { return nil }
func (r *resolveRepo) InsertAudit(context.Context, biz.A2AAuditEntry) error      { return nil }
func (r *resolveRepo) ListAudit(context.Context, string, string, int, int) ([]biz.A2AAuditEntry, int, error) {
	return nil, 0, nil
}

func TestResolveInvokeTarget_LocalDisabledDoesNotFallbackRemote(t *testing.T) {
	t.Parallel()
	repo := &resolveRepo{
		cards: map[string]biz.A2AAgentCard{
			"agent-1": {AgentID: "agent-1", Enabled: false},
		},
		remotes: map[string]biz.A2ARemoteAgent{
			"agent-1": {ID: "agent-1", Enabled: true},
		},
	}
	uc := biz.NewA2AUsecase(repo)
	_, err := ResolveInvokeTarget(context.Background(), uc, "agent-1")
	if err == nil {
		t.Fatal("expected forbidden for disabled local card")
	}
}

func TestResolveInvokeTarget_RemoteWhenNoLocalCard(t *testing.T) {
	t.Parallel()
	repo := &resolveRepo{
		remotes: map[string]biz.A2ARemoteAgent{
			"remote-1": {ID: "remote-1", Enabled: true, DiscoveredCard: biz.A2AAgentCard{Enabled: true}},
		},
	}
	uc := biz.NewA2AUsecase(repo)
	target, err := ResolveInvokeTarget(context.Background(), uc, "remote-1")
	if err != nil || target.Kind != InvokeTargetRemote {
		t.Fatalf("got %#v err=%v", target, err)
	}
}
