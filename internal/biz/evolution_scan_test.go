package biz

import (
	"context"
	"strings"
	"testing"
)

type scanAgentRepo struct {
	AgentRepository
	items []Agent
}

func (r *scanAgentRepo) SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error) {
	return AgentListResult{Items: r.items}, nil
}

func (r *scanAgentRepo) ListAgentCreators(ctx context.Context) ([]AgentCreator, error) {
	return nil, nil
}

func (r *scanAgentRepo) GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	return AgentRuntimeSettings{}, context.Canceled
}

func TestScanAll_joinsPerAgentErrors(t *testing.T) {
	uc := &EvolutionUsecase{
		agents: &scanAgentRepo{items: []Agent{{ID: "a1"}}},
	}
	err := uc.ScanAll(context.Background())
	if err == nil {
		t.Fatal("expected joined errors")
	}
	if !strings.Contains(err.Error(), "agent a1") {
		t.Fatalf("unexpected error: %v", err)
	}
}
