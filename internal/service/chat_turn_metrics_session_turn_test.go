package service

import (
	"context"
	"encoding/json"
	"testing"

	chatagent "aranea-agents/internal/agent"
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

// context_budget 台账必须随 Update 路径落 session_turns.metadata_json——前端
// 会话曲线悬停面板按该字段渲染该轮 prompt 构成（此前只落 usage 事件表，
// 悬停恒为回退视图）。
func TestRecordSessionTurn_UpdatePathPersistsContextBudget(t *testing.T) {
	stub := &stubSessionTurnRecorder{}
	m := newChatTurnMetrics(stub, nil, nil, loggateway.NewNoop())
	ctx, _ := chatagent.WithContextBudget(context.Background())
	chatagent.RecordContextBudget(ctx, chatagent.ContextBudgetCategoryToolsSchema, 3500)
	ctx = contextWithAdmittedTurnID(ctx, "turn-1")
	m.RecordSessionTurn(ctx, SessionTurnRecordParams{
		SessionID: "s1",
		OwnerType: "agent",
		OwnerID:   "a1",
	})
	if stub.updateN != 1 {
		t.Fatalf("expected 1 UpdateTurn call, got %d", stub.updateN)
	}
	if stub.updated.MetadataJSON == nil {
		t.Fatal("MetadataJSON must carry the context_budget ledger")
	}
	assertContextBudgetMeta(t, *stub.updated.MetadataJSON)
}

// Create 路径同理；无台账时 MetadataJSON 必须留空（透传不污染）。
func TestRecordSessionTurn_CreatePathPersistsContextBudget(t *testing.T) {
	stub := &stubSessionTurnRecorder{}
	m := newChatTurnMetrics(stub, nil, nil, loggateway.NewNoop())
	ctx, _ := chatagent.WithContextBudget(context.Background())
	chatagent.RecordContextBudget(ctx, chatagent.ContextBudgetCategoryHistory, 700)
	m.RecordSessionTurn(ctx, SessionTurnRecordParams{
		SessionID: "s1",
		OwnerType: "agent",
		OwnerID:   "a1",
	})
	if stub.createN != 1 {
		t.Fatalf("expected 1 CreateTurn call, got %d", stub.createN)
	}
	assertContextBudgetMeta(t, stub.created.MetadataJSON)

	stub2 := &stubSessionTurnRecorder{}
	m2 := newChatTurnMetrics(stub2, nil, nil, loggateway.NewNoop())
	m2.RecordSessionTurn(context.Background(), SessionTurnRecordParams{SessionID: "s1", OwnerType: "agent", OwnerID: "a1"})
	if stub2.created.MetadataJSON != "" {
		t.Fatalf("no-budget turn must leave MetadataJSON empty, got %s", stub2.created.MetadataJSON)
	}
}

func assertContextBudgetMeta(t *testing.T, meta string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(meta), &payload); err != nil {
		t.Fatalf("turn metadata not valid JSON: %v", err)
	}
	cb, ok := payload["context_budget"].(map[string]any)
	if !ok {
		t.Fatalf("context_budget key missing: %s", meta)
	}
	if _, ok := cb["est_tokens"].(map[string]any); !ok {
		t.Fatalf("est_tokens missing: %s", meta)
	}
}
