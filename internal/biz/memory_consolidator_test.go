package biz

import (
	"context"
	"errors"
	"testing"
)

type mockMemoryRepo struct {
	upsertCalls int
	lastAgent   string
	lastFactID  string
	lastStmt    string
}

func (m *mockMemoryRepo) Insert(context.Context, *AgentMemory) error { return nil }
func (m *mockMemoryRepo) FindSimilar(context.Context, string, []float32, int) ([]*AgentMemory, error) {
	return nil, nil
}
func (m *mockMemoryRepo) FindSimilarWithUser(context.Context, string, string, []float32, int) ([]*AgentMemory, error) {
	return nil, nil
}
func (m *mockMemoryRepo) UpsertFactVector(_ context.Context, agentID, _ string, factID, statement string, _ []float32) error {
	m.upsertCalls++
	m.lastAgent = agentID
	m.lastFactID = factID
	m.lastStmt = statement
	return nil
}

type mockEmbedder struct{}

func (mockEmbedder) Embed(context.Context, string) ([]float32, error) {
	return make([]float32, 4), nil
}

func TestMemoryUsecase_SyncFactIndexFromRow(t *testing.T) {
	repo := &mockMemoryRepo{}
	uc := NewMemoryUsecase(repo, mockEmbedder{})
	raw := []byte(`{"id":"fact-1","scope_id":"agent-1","agent_id":"agent-1","user_id":"u1","statement":"hello"}`)
	if err := uc.SyncFactIndexFromRow(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if repo.upsertCalls != 1 || repo.lastFactID != "fact-1" || repo.lastStmt != "hello" {
		t.Fatalf("unexpected upsert: %+v", repo)
	}
}

func TestMemoryUsecase_SyncFactIndexUnavailable(t *testing.T) {
	uc := NewMemoryUsecase(unavailableMemoryRepo{}, nil)
	err := uc.SyncFactIndex(context.Background(), "a", "u", "f", "s")
	if !errors.Is(err, ErrMemoryUnavailable) {
		t.Fatalf("expected ErrMemoryUnavailable, got %v", err)
	}
}

type unavailableMemoryRepo struct{}

func (unavailableMemoryRepo) Insert(context.Context, *AgentMemory) error { return ErrMemoryUnavailable }
func (unavailableMemoryRepo) FindSimilar(context.Context, string, []float32, int) ([]*AgentMemory, error) {
	return nil, ErrMemoryUnavailable
}
func (unavailableMemoryRepo) FindSimilarWithUser(context.Context, string, string, []float32, int) ([]*AgentMemory, error) {
	return nil, ErrMemoryUnavailable
}
func (unavailableMemoryRepo) UpsertFactVector(context.Context, string, string, string, string, []float32) error {
	return ErrMemoryUnavailable
}

func TestHeuristicConsolidator_Extract(t *testing.T) {
	c := NewHeuristicConsolidator()
	props, err := c.Extract(context.Background(), ConsolidateInput{
		Messages: []ConsolidateMessage{{Role: "user", Content: "My name is Alice", MessageID: "msg-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(props) != 1 || props[0].Statement == "" {
		t.Fatalf("unexpected proposals: %+v", props)
	}
	if props[0].SourceMessageID != "msg-1" {
		t.Fatalf("expected source message id msg-1, got %q", props[0].SourceMessageID)
	}
}

func TestResolveProposalMessageID(t *testing.T) {
	msgs := []ConsolidateMessage{
		{Role: "user", Content: "hello", MessageID: "m1"},
		{Role: "user", Content: "I prefer dark mode", MessageID: "m2"},
	}
	if got := ResolveProposalMessageID("I prefer dark mode", msgs); got != "m2" {
		t.Fatalf("expected m2, got %q", got)
	}
}

func TestChainConsolidator_FallsBackToHeuristic(t *testing.T) {
	c := NewChainConsolidator(NewLLMConsolidator(nil), NewHeuristicConsolidator())
	props, err := c.Extract(context.Background(), ConsolidateInput{
		Messages: []ConsolidateMessage{{Role: "user", Content: "I prefer tea"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(props) == 0 {
		t.Fatal("expected fallback proposals")
	}
}

func TestBuildFeedbackStatement(t *testing.T) {
	if got := BuildFeedbackStatement("positive", "great tone", ""); got == "" {
		t.Fatal("expected positive feedback statement")
	}
}
