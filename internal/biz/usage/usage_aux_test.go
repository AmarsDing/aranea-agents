package usage

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockSessAccum struct {
	deltas []SessionMetricsDelta
}

func (m *mockSessAccum) AccumulateMetricsDelta(d SessionMetricsDelta) {
	m.deltas = append(m.deltas, d)
}

type mockEnvelopePub struct {
	events []TokenUsageEvent
}

func (m *mockEnvelopePub) PublishTokenUsageEnvelope(_ context.Context, e TokenUsageEvent) {
	m.events = append(m.events, e)
}

func TestRecordAuxLLMUsage(t *testing.T) {
	t.Run("nil usecase is no-op", func(t *testing.T) {
		var u *Usecase
		if err := u.RecordAuxLLMUsage(context.Background(), AuxLLMUsageInput{Kind: KindAuxIntent}); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("empty kind returns error", func(t *testing.T) {
		u := NewUsecase(&mockUsageRepo{}, nil)
		err := u.RecordAuxLLMUsage(context.Background(), AuxLLMUsageInput{Kind: "  "})
		if err == nil {
			t.Fatal("expected error for empty kind")
		}
	})

	t.Run("persists event with kind and tokens", func(t *testing.T) {
		var got TokenUsageEvent
		repo := &mockUsageRepo{
			recordTokenUsageEventFn: func(_ context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
				got = e
				return e, nil
			},
		}
		u := NewUsecase(repo, nil)
		in := AuxLLMUsageInput{
			Kind:          KindAuxIntent,
			SessionID:     "sess-1",
			RunID:         "run-1",
			AgentID:       "agent-1",
			AgentKey:      "agent-key-1",
			TeamID:        "team-1",
			UserID:        "user-1",
			Provider:      "deepseek",
			Model:         "deepseek-chat",
			Status:        "success",
			PromptTok:     120,
			CompletionTok: 30,
			CachedTok:     64,
			Latency:       850 * time.Millisecond,
			ErrMsg:        "",
		}
		if err := u.RecordAuxLLMUsage(context.Background(), in); err != nil {
			t.Fatalf("record failed: %v", err)
		}
		if got.ID == "" {
			t.Error("expected ID to be generated")
		}
		if got.UsageKind != KindAuxIntent {
			t.Errorf("UsageKind = %q, want %q", got.UsageKind, KindAuxIntent)
		}
		if got.InputTokens != 120 || got.OutputTokens != 30 || got.TotalTokens != 150 {
			t.Errorf("tokens = %d/%d/%d, want 120/30/150", got.InputTokens, got.OutputTokens, got.TotalTokens)
		}
		if got.CachedInputTokens != 64 {
			t.Errorf("CachedInputTokens = %d, want 64", got.CachedInputTokens)
		}
		if got.MessageID != "run-1" {
			t.Errorf("MessageID = %q, want run-1", got.MessageID)
		}
		if got.SessionID != "sess-1" || got.AgentID != "agent-1" || got.TeamID != "team-1" || got.UserID != "user-1" {
			t.Errorf("scope ids mismatch: %+v", got)
		}
		if got.ProviderCode != "deepseek" || got.ModelAPIID != "deepseek-chat" {
			t.Errorf("provider/model = %q/%q", got.ProviderCode, got.ModelAPIID)
		}
		if got.LatencyMS != 850 {
			t.Errorf("LatencyMS = %d, want 850", got.LatencyMS)
		}
		if got.OccurredAt == "" || got.DateKey == "" || got.HourKey == "" {
			t.Error("expected time keys to be filled")
		}
	})

	t.Run("accumulates session metrics only when session id set", func(t *testing.T) {
		repo := &mockUsageRepo{
			recordTokenUsageEventFn: func(_ context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
				return e, nil
			},
		}
		u := NewUsecase(repo, nil)
		sa := &mockSessAccum{}
		u.SetSessionMetricsAccumulator(sa)

		// With session id → one delta.
		if err := u.RecordAuxLLMUsage(context.Background(), AuxLLMUsageInput{
			Kind: KindAuxTitle, SessionID: "sess-9", PromptTok: 10, CompletionTok: 5,
		}); err != nil {
			t.Fatalf("record failed: %v", err)
		}
		if len(sa.deltas) != 1 {
			t.Fatalf("expected 1 delta, got %d", len(sa.deltas))
		}
		d := sa.deltas[0]
		if d.SessionID != "sess-9" || d.InputTokens != 10 || d.OutputTokens != 5 || d.TotalTokens != 15 {
			t.Errorf("unexpected delta: %+v", d)
		}
		if d.ModelCallCount != 1 {
			t.Errorf("ModelCallCount = %d, want 1", d.ModelCallCount)
		}

		// Without session id → no new delta.
		if err := u.RecordAuxLLMUsage(context.Background(), AuxLLMUsageInput{
			Kind: KindAuxEvolution, AgentID: "agent-1", PromptTok: 7, CompletionTok: 3,
		}); err != nil {
			t.Fatalf("record failed: %v", err)
		}
		if len(sa.deltas) != 1 {
			t.Fatalf("expected still 1 delta, got %d", len(sa.deltas))
		}
	})

	t.Run("publishes envelope", func(t *testing.T) {
		repo := &mockUsageRepo{
			recordTokenUsageEventFn: func(_ context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
				return e, nil
			},
		}
		u := NewUsecase(repo, nil)
		pub := &mockEnvelopePub{}
		u.SetUsageEnvelopePublisher(pub)
		if err := u.RecordAuxLLMUsage(context.Background(), AuxLLMUsageInput{
			Kind: KindAuxSubagent, RunID: "sub-run-1", PromptTok: 100, CompletionTok: 40,
		}); err != nil {
			t.Fatalf("record failed: %v", err)
		}
		if len(pub.events) != 1 {
			t.Fatalf("expected 1 envelope, got %d", len(pub.events))
		}
		if pub.events[0].UsageKind != KindAuxSubagent {
			t.Errorf("envelope kind = %q, want %q", pub.events[0].UsageKind, KindAuxSubagent)
		}
	})

	t.Run("repo error propagates and skips side effects", func(t *testing.T) {
		repo := &mockUsageRepo{
			recordTokenUsageEventFn: func(_ context.Context, _ TokenUsageEvent) (TokenUsageEvent, error) {
				return TokenUsageEvent{}, errors.New("db down")
			},
		}
		u := NewUsecase(repo, nil)
		sa := &mockSessAccum{}
		pub := &mockEnvelopePub{}
		u.SetSessionMetricsAccumulator(sa)
		u.SetUsageEnvelopePublisher(pub)
		err := u.RecordAuxLLMUsage(context.Background(), AuxLLMUsageInput{
			Kind: KindAuxIntent, SessionID: "sess-1", PromptTok: 1, CompletionTok: 1,
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if len(sa.deltas) != 0 {
			t.Errorf("expected no deltas on failure, got %d", len(sa.deltas))
		}
		if len(pub.events) != 0 {
			t.Errorf("expected no envelopes on failure, got %d", len(pub.events))
		}
	})

	t.Run("pricing enrichment applies to aux rows", func(t *testing.T) {
		repo := &mockUsageRepo{
			getActiveModelPricingFn: func(_ context.Context, prov, mod string) (ModelPricingSnapshot, bool, error) {
				if prov != "deepseek" || mod != "deepseek-chat" {
					t.Errorf("pricing lookup = %q/%q", prov, mod)
				}
				return ModelPricingSnapshot{
					InputPriceUSDPer1M:  0.27,
					OutputPriceUSDPer1M: 1.10,
				}, true, nil
			},
			recordTokenUsageEventFn: func(_ context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
				if e.TotalCostMicroUSD <= 0 {
					t.Errorf("expected positive cost, got %d", e.TotalCostMicroUSD)
				}
				if e.InputCostMicroUSD <= 0 || e.OutputCostMicroUSD <= 0 {
					t.Errorf("expected per-kind costs, got in=%d out=%d", e.InputCostMicroUSD, e.OutputCostMicroUSD)
				}
				return e, nil
			},
		}
		u := NewUsecase(repo, nil)
		if err := u.RecordAuxLLMUsage(context.Background(), AuxLLMUsageInput{
			Kind: KindAuxIntent, Provider: "deepseek", Model: "deepseek-chat",
			PromptTok: 1000, CompletionTok: 500,
		}); err != nil {
			t.Fatalf("record failed: %v", err)
		}
	})
}
