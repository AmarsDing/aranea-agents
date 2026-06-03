package a2a

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// invokeIntegrationRepo satisfies biz.A2ARepo for Invoke + workspace checks.
type invokeIntegrationRepo struct {
	memA2ARepo
	remotes map[string]biz.A2ARemoteAgent
}

func (r *invokeIntegrationRepo) GetRemoteAgent(_ context.Context, id string) (biz.A2ARemoteAgent, error) {
	rmt, ok := r.remotes[id]
	if !ok {
		return biz.A2ARemoteAgent{}, biz.ErrNotFound
	}
	return rmt, nil
}

func TestInvokeIntegration_LocalChatCapability(t *testing.T) {
	t.Parallel()
	repo := &invokeIntegrationRepo{
		memA2ARepo: memA2ARepo{cards: map[string]biz.A2AAgentCard{
			"agent-a": {
				AgentID:    "agent-a",
				Enabled:    true,
				Workspace:  "ws-1",
				Capabilities: []biz.A2ACapability{{Name: "chat"}},
			},
		}},
	}
	uc := biz.NewA2AUsecase(repo, repo, repo, repo)
	runner := &mockTurnRunner{}
	inv := NewInvoker(runner, uc, nil, loggateway.NewNoop())
	ctx := workspace.WithContext(
		WithCallerAgentID(context.Background(), "caller-1"),
		"ws-1",
	)

	out, err := inv(ctx, "agent-a", "chat", `{"message":"ping"}`, 30)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ok") || runner.lastInput != "ping" {
		t.Fatalf("invoke out=%q input=%q", out, runner.lastInput)
	}
}

func TestInvokeIntegration_CrossWorkspaceDenied(t *testing.T) {
	t.Parallel()
	repo := &invokeIntegrationRepo{
		memA2ARepo: memA2ARepo{cards: map[string]biz.A2AAgentCard{
			"caller-1": {
				AgentID:   "caller-1",
				Enabled:   true,
				Workspace: "ws-b",
			},
			"agent-a": {
				AgentID:      "agent-a",
				Enabled:      true,
				Workspace:    "ws-a",
				Capabilities: []biz.A2ACapability{{Name: "chat"}},
			},
		}},
	}
	uc := biz.NewA2AUsecase(repo, repo, repo, repo)
	inv := NewInvoker(&mockTurnRunner{}, uc, nil, loggateway.NewNoop())
	ctx := WithCallerAgentID(context.Background(), "caller-1")
	_, err := inv(ctx, "agent-a", "chat", `{"message":"x"}`, 30)
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("expected workspace error, got %v", err)
	}
}
