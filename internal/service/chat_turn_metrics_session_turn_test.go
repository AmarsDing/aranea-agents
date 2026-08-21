package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubSessionTurnRecorder captures CreateTurn/UpdateTurn payloads so tests can
// assert exactly which token fields RecordSessionTurn persists.
type stubSessionTurnRecorder struct {
	biz.SessionTurnManager
	created biz.SessionTurn
	createN int
	updated biz.SessionTurnUpdateFields
	updateN int
}

func (s *stubSessionTurnRecorder) CreateTurn(_ context.Context, turn biz.SessionTurn) (biz.SessionTurn, error) {
	s.createN++
	s.created = turn
	return turn, nil
}

func (s *stubSessionTurnRecorder) UpdateTurn(_ context.Context, _ string, fields biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	s.updateN++
	s.updated = fields
	return biz.SessionTurn{}, nil
}

// CachedTok must land in session_turns.cached_input_tokens on the Create path
// (no admitted turn id in ctx) so turn-level cache hit ratio is computable
// without joining model_token_usage_events.
func TestRecordSessionTurn_CreatePathPersistsCachedTok(t *testing.T) {
	stub := &stubSessionTurnRecorder{}
	m := newChatTurnMetrics(stub, nil, nil, loggateway.NewNoop())
	m.RecordSessionTurn(context.Background(), SessionTurnRecordParams{
		SessionID:     "s1",
		OwnerType:     "agent",
		OwnerID:       "a1",
		PromptTok:     2000,
		CompletionTok: 50,
		CachedTok:     1500,
	})
	if stub.createN != 1 {
		t.Fatalf("expected 1 CreateTurn call, got %d", stub.createN)
	}
	if stub.created.CachedInputTokens != 1500 {
		t.Errorf("CachedInputTokens = %d, want 1500", stub.created.CachedInputTokens)
	}
	if stub.created.InputTokens != 2000 || stub.created.OutputTokens != 50 {
		t.Errorf("tokens = (%d,%d), want (2000,50)", stub.created.InputTokens, stub.created.OutputTokens)
	}
}

// Same assertion for the Update path (admittedTurnID present): cached tokens
// ride alongside InputTokens/OutputTokens (written unconditionally — zero is a
// meaningful "no cache hit" observation, unlike duration-style gauges).
func TestRecordSessionTurn_UpdatePathPersistsCachedTok(t *testing.T) {
	stub := &stubSessionTurnRecorder{}
	m := newChatTurnMetrics(stub, nil, nil, loggateway.NewNoop())
	ctx := contextWithAdmittedTurnID(context.Background(), "turn-1")
	m.RecordSessionTurn(ctx, SessionTurnRecordParams{
		SessionID:     "s1",
		OwnerType:     "agent",
		OwnerID:       "a1",
		PromptTok:     2000,
		CompletionTok: 50,
		CachedTok:     1500,
	})
	if stub.updateN != 1 {
		t.Fatalf("expected 1 UpdateTurn call, got %d", stub.updateN)
	}
	if stub.updated.CachedInputTokens == nil || *stub.updated.CachedInputTokens != 1500 {
		t.Fatalf("CachedInputTokens = %v, want 1500", stub.updated.CachedInputTokens)
	}
}
