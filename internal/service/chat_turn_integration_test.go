//go:build integration

package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Stubs for Chat Turn integration tests
// ---------------------------------------------------------------------------

// stubTurnStore implements persistentTurnStore for integration tests.
type stubTurnStore struct {
	mu    sync.Mutex
	turns map[string]biz.SessionTurn // id → turn
}

func newStubTurnStore() *stubTurnStore {
	return &stubTurnStore{turns: make(map[string]biz.SessionTurn)}
}

func (s *stubTurnStore) CreateTurn(_ context.Context, turn biz.SessionTurn) (biz.SessionTurn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns[turn.ID] = turn
	return turn, nil
}

func (s *stubTurnStore) UpdateTurn(_ context.Context, id string, fields biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.turns[id]
	if !ok {
		return biz.SessionTurn{}, fmt.Errorf("turn not found: %s", id)
	}
	if fields.Status != nil {
		t.Status = *fields.Status
	}
	if fields.EndedAt != nil {
		t.EndedAt = *fields.EndedAt
	}
	if fields.ErrorMessage != nil {
		t.ErrorMessage = *fields.ErrorMessage
	}
	s.turns[id] = t
	return t, nil
}

// stubTurnProjector implements TurnProjector for integration tests.
type stubTurnProjector struct {
	mu     sync.Mutex
	events []biz.TurnEvent
}

func newStubTurnProjector() *stubTurnProjector {
	return &stubTurnProjector{}
}

func (p *stubTurnProjector) ProjectTurnEvent(_ context.Context, event biz.TurnEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

func (p *stubTurnProjector) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func (p *stubTurnProjector) lastEvent() *biz.TurnEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		return nil
	}
	return &p.events[len(p.events)-1]
}

// stubTurnExecutor implements TurnExecutor for integration tests.
type stubTurnExecutor struct {
	mu       sync.Mutex
	result   biz.TurnResult
	execErr  error
	executed bool
}

func newStubTurnExecutor(result biz.TurnResult) *stubTurnExecutor {
	return &stubTurnExecutor{result: result}
}

func (e *stubTurnExecutor) ExecuteTurn(_ context.Context, _ biz.CanonicalTurn, _ biz.TurnInput) (biz.TurnResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executed = true
	return e.result, e.execErr
}

func (e *stubTurnExecutor) wasExecuted() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.executed
}

// ---------------------------------------------------------------------------
// Integration Tests: Chat Turn Pipeline
// ---------------------------------------------------------------------------

// TestChatTurnIntegration_TurnPipeline_AdmitAndComplete verifies the full
// TurnPipeline flow: AdmitTurn → ExecuteTurn → CompleteTurn → ProjectEvents.
// Run with: go test -tags=integration ./internal/service/... -run TestChatTurnIntegration_TurnPipeline -count=1
func TestChatTurnIntegration_TurnPipeline_AdmitAndComplete(t *testing.T) {
	ctx := context.Background()
	_ = loggateway.NewNoop()

	t.Run("SuccessfulTurn_PipelineFlow", func(t *testing.T) {
		store := newStubTurnStore()
		projector := newStubTurnProjector()
		executor := newStubTurnExecutor(biz.TurnResult{
			Outcome:      biz.TurnOutcomeCompleted,
			UserMsg:      biz.ChatMessage{ID: "msg-user-1", Role: "user", ContentMarkdown: "Hello"},
			AssistantMsg: biz.ChatMessage{ID: "msg-assistant-1", Role: "assistant", ContentMarkdown: "Hi there!"},
		})

		pipeline := TurnPipeline{
			Service:   &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }},
			Executor:  executor,
			Projector: projector,
			Lg:        loggateway.NewNoop(),
		}

		intent := biz.TurnIntent{
			Source:     biz.TurnSourceWeb,
			SessionID:  "session-turn-001",
			TargetType: biz.ConversationTargetAgent,
			AgentID:    "agent-001",
			Content:    "Hello",
		}

		turn, result, err := pipeline.Run(ctx, intent)
		if err != nil {
			t.Fatalf("TurnPipeline.Run failed: %v", err)
		}

		// Verify turn was admitted with queued status
		if turn.SessionID != "session-turn-001" {
			t.Errorf("expected session_id=session-turn-001, got %q", turn.SessionID)
		}
		if turn.Source != biz.TurnSourceWeb {
			t.Errorf("expected source=web, got %q", turn.Source)
		}

		// Verify executor was called
		if !executor.wasExecuted() {
			t.Error("expected executor to be called")
		}

		// Verify result
		if result.Outcome != biz.TurnOutcomeCompleted {
			t.Errorf("expected outcome completed, got %q", result.Outcome)
		}

		// Verify events were projected
		if projector.count() < 2 {
			t.Errorf("expected at least 2 projected events (queued + completed), got %d", projector.count())
		}

		// Verify turn was persisted in store
		store.mu.Lock()
		stored, ok := store.turns[turn.ID]
		store.mu.Unlock()
		if !ok {
			t.Fatal("expected turn to be persisted in store")
		}
		if stored.SessionID != "session-turn-001" {
			t.Errorf("expected stored session_id=session-turn-001, got %q", stored.SessionID)
		}
	})

	t.Run("FailedTurn_PipelineFlow", func(t *testing.T) {
		store := newStubTurnStore()
		projector := newStubTurnProjector()
		executor := &stubTurnExecutor{
			result:  biz.TurnResult{},
			execErr: fmt.Errorf("LLM provider timeout"),
		}

		pipeline := TurnPipeline{
			Service:   &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }},
			Executor:  executor,
			Projector: projector,
			Lg:        loggateway.NewNoop(),
		}

		intent := biz.TurnIntent{
			Source:     biz.TurnSourceWeb,
			SessionID:  "session-turn-002",
			TargetType: biz.ConversationTargetAgent,
			AgentID:    "agent-002",
			Content:    "Help me",
		}

		_, _, err := pipeline.Run(ctx, intent)
		if err == nil {
			t.Fatal("expected error from failed turn, got nil")
		}

		// Verify failure events were projected
		lastEvent := projector.lastEvent()
		if lastEvent == nil {
			t.Fatal("expected at least one projected event")
		}
		if lastEvent.Type != biz.TurnEventFailed {
			t.Errorf("expected last event type failed, got %q", lastEvent.Type)
		}
	})

	t.Run("QueuedTurn_PipelineFlow", func(t *testing.T) {
		store := newStubTurnStore()
		projector := newStubTurnProjector()
		executor := newStubTurnExecutor(biz.TurnResult{
			Outcome:   biz.TurnOutcomeQueued,
			PendingID: "pending-001",
		})

		pipeline := TurnPipeline{
			Service:   &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }},
			Executor:  executor,
			Projector: projector,
			Lg:        loggateway.NewNoop(),
		}

		intent := biz.TurnIntent{
			Source:     biz.TurnSourceWS,
			SessionID:  "session-turn-003",
			TargetType: biz.ConversationTargetAgent,
			AgentID:    "agent-003",
			Content:    "Queue this",
		}

		turn, result, err := pipeline.Run(ctx, intent)
		if err != nil {
			t.Fatalf("TurnPipeline.Run for queued turn failed: %v", err)
		}

		if result.Outcome != biz.TurnOutcomeQueued {
			t.Errorf("expected outcome queued, got %q", result.Outcome)
		}

		// Verify queued event was projected
		lastEvent := projector.lastEvent()
		if lastEvent == nil {
			t.Fatal("expected at least one projected event")
		}
		if lastEvent.Type != biz.TurnEventQueued {
			t.Errorf("expected last event type queued, got %q", lastEvent.Type)
		}

		// Verify turn was persisted
		store.mu.Lock()
		_, ok := store.turns[turn.ID]
		store.mu.Unlock()
		if !ok {
			t.Fatal("expected turn to be persisted in store")
		}
	})
}

// TestChatTurnIntegration_TurnIntentCanonicalize verifies that TurnIntent
// canonicalization fills defaults correctly for different entry points.
func TestChatTurnIntegration_TurnIntentCanonicalize(t *testing.T) {
	t.Run("WebSource_DefaultsToAgentTarget", func(t *testing.T) {
		intent := biz.TurnIntent{
			Source:    biz.TurnSourceWeb,
			SessionID: "session-canon-001",
			Content:   "Hello",
		}
		canon := intent.Canonicalize()
		if canon.TargetType != biz.ConversationTargetAgent {
			t.Errorf("expected target_type=agent, got %q", canon.TargetType)
		}
		if canon.Source != biz.TurnSourceWeb {
			t.Errorf("expected source=web, got %q", canon.Source)
		}
	})

	t.Run("TeamID_SetsTeamTarget", func(t *testing.T) {
		intent := biz.TurnIntent{
			Source:    biz.TurnSourceChannel,
			SessionID: "session-canon-002",
			TeamID:    "team-001",
			Content:   "Run team task",
		}
		canon := intent.Canonicalize()
		if canon.TargetType != biz.ConversationTargetTeam {
			t.Errorf("expected target_type=team, got %q", canon.TargetType)
		}
	})

	t.Run("EmptySource_DefaultsToWeb", func(t *testing.T) {
		intent := biz.TurnIntent{
			SessionID: "session-canon-003",
			Content:   "Hello",
		}
		canon := intent.Canonicalize()
		if canon.Source != biz.TurnSourceWeb {
			t.Errorf("expected source=web for empty source, got %q", canon.Source)
		}
	})

	t.Run("WhitespaceTrimmed", func(t *testing.T) {
		intent := biz.TurnIntent{
			Source:    biz.TurnSourceWeb,
			SessionID: "  session-canon-004  ",
			Content:   "  Hello  ",
			AgentKey:  "  agent-key  ",
		}
		canon := intent.Canonicalize()
		if canon.SessionID != "session-canon-004" {
			t.Errorf("expected trimmed session_id, got %q", canon.SessionID)
		}
		if canon.Content != "Hello" {
			t.Errorf("expected trimmed content, got %q", canon.Content)
		}
		if canon.AgentKey != "agent-key" {
			t.Errorf("expected trimmed agent_key, got %q", canon.AgentKey)
		}
	})
}

// TestChatTurnIntegration_CanonicalTurnStatusMapping verifies the mapping from
// TurnOutcome to canonical CanonicalTurnStatus.
func TestChatTurnIntegration_CanonicalTurnStatusMapping(t *testing.T) {
	cases := []struct {
		outcome    biz.TurnOutcome
		wantStatus biz.CanonicalTurnStatus
	}{
		{biz.TurnOutcomeCompleted, biz.CanonicalTurnStatusCompleted},
		{biz.TurnOutcomeQueued, biz.CanonicalTurnStatusQueued},
		{biz.TurnOutcomeRejected, biz.CanonicalTurnStatusRejected},
		{biz.TurnOutcomeFailed, biz.CanonicalTurnStatusFailed},
	}
	for _, tc := range cases {
		t.Run(string(tc.outcome), func(t *testing.T) {
			got := biz.CanonicalTurnStatusFromOutcome(tc.outcome)
			if got != tc.wantStatus {
				t.Errorf("CanonicalTurnStatusFromOutcome(%q) = %q, want %q", tc.outcome, got, tc.wantStatus)
			}
		})
	}
}

// TestChatTurnIntegration_PersistentTurnService verifies the
// PersistentTurnService lifecycle: AdmitTurn → CompleteTurn / FailTurn.
func TestChatTurnIntegration_PersistentTurnService(t *testing.T) {
	ctx := context.Background()

	t.Run("AdmitTurn_PersistsToStore", func(t *testing.T) {
		store := newStubTurnStore()
		svc := &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }}

		intent := biz.TurnIntent{
			Source:     biz.TurnSourceWeb,
			SessionID:  "session-pts-001",
			TargetType: biz.ConversationTargetAgent,
			AgentID:    "agent-pts-001",
			Content:    "Test message",
		}

		turn, err := svc.AdmitTurn(ctx, intent)
		if err != nil {
			t.Fatalf("AdmitTurn failed: %v", err)
		}

		if turn.ID == "" {
			t.Error("expected non-empty turn ID")
		}
		if turn.SessionID != "session-pts-001" {
			t.Errorf("expected session_id=session-pts-001, got %q", turn.SessionID)
		}
		if turn.Status != biz.CanonicalTurnStatusQueued {
			t.Errorf("expected status=queued, got %q", turn.Status)
		}
		if turn.Source != biz.TurnSourceWeb {
			t.Errorf("expected source=web, got %q", turn.Source)
		}

		// Verify persisted in store
		store.mu.Lock()
		stored, ok := store.turns[turn.ID]
		store.mu.Unlock()
		if !ok {
			t.Fatal("expected turn to be persisted in store")
		}
		if stored.SessionID != "session-pts-001" {
			t.Errorf("expected stored session_id=session-pts-001, got %q", stored.SessionID)
		}
	})

	t.Run("CompleteTurn_UpdatesStatus", func(t *testing.T) {
		store := newStubTurnStore()
		svc := &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }}

		intent := biz.TurnIntent{
			Source:     biz.TurnSourceWeb,
			SessionID:  "session-pts-002",
			TargetType: biz.ConversationTargetAgent,
			Content:    "Complete me",
		}

		turn, _ := svc.AdmitTurn(ctx, intent)
		result := biz.TurnResult{Outcome: biz.TurnOutcomeCompleted}

		completed, err := svc.CompleteTurn(ctx, turn, result)
		if err != nil {
			t.Fatalf("CompleteTurn failed: %v", err)
		}

		if completed.Status != biz.CanonicalTurnStatusCompleted {
			t.Errorf("expected status=completed, got %q", completed.Status)
		}

		// Verify updated in store
		store.mu.Lock()
		stored := store.turns[turn.ID]
		store.mu.Unlock()
		if stored.Status != string(biz.CanonicalTurnStatusCompleted) {
			t.Errorf("expected stored status=completed, got %q", stored.Status)
		}
	})

	t.Run("FailTurn_UpdatesStatusAndError", func(t *testing.T) {
		store := newStubTurnStore()
		svc := &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }}

		intent := biz.TurnIntent{
			Source:     biz.TurnSourceWeb,
			SessionID:  "session-pts-003",
			TargetType: biz.ConversationTargetAgent,
			Content:    "Fail me",
		}

		turn, _ := svc.AdmitTurn(ctx, intent)
		failErr := fmt.Errorf("provider timeout")

		failed, err := svc.FailTurn(ctx, turn, failErr)
		if err != nil {
			t.Fatalf("FailTurn failed: %v", err)
		}

		if failed.Status != biz.CanonicalTurnStatusFailed {
			t.Errorf("expected status=failed, got %q", failed.Status)
		}

		// Verify updated in store
		store.mu.Lock()
		stored := store.turns[turn.ID]
		store.mu.Unlock()
		if stored.Status != string(biz.CanonicalTurnStatusFailed) {
			t.Errorf("expected stored status=failed, got %q", stored.Status)
		}
		if stored.ErrorMessage != "provider timeout" {
			t.Errorf("expected stored error_message='provider timeout', got %q", stored.ErrorMessage)
		}
	})
}

// TestChatTurnIntegration_DeliveryStatusMapping verifies the
// DeliveryStatusFromChannelRecord mapping for various status strings.
func TestChatTurnIntegration_DeliveryStatusMapping(t *testing.T) {
	cases := []struct {
		input string
		want  biz.DeliveryStatus
	}{
		{"queued", biz.DeliveryStatusPending},
		{"pending", biz.DeliveryStatusPending},
		{"sending", biz.DeliveryStatusSending},
		{"streaming", biz.DeliveryStatusSending},
		{"sent", biz.DeliveryStatusDelivered},
		{"delivered", biz.DeliveryStatusDelivered},
		{"ok", biz.DeliveryStatusDelivered},
		{"failed", biz.DeliveryStatusFailed},
		{"error", biz.DeliveryStatusFailed},
		{"timeout", biz.DeliveryStatusFailed},
		{"skipped", biz.DeliveryStatusSkipped},
		{"skipped_duplicate", biz.DeliveryStatusSkipped},
		{"unknown_status", biz.DeliveryStatus("")},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := biz.DeliveryStatusFromChannelRecord(tc.input)
			if got != tc.want {
				t.Errorf("DeliveryStatusFromChannelRecord(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestChatTurnIntegration_SelfHealObserverIntegration verifies the
// integration between Chat Turn error handling and the SelfHealObserver.
// When a turn fails, the error should be observable by the SelfHealObserver.
func TestChatTurnIntegration_SelfHealObserverIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("TurnError_ObservedBySelfHeal", func(t *testing.T) {
		repo := newStubHealRecordRepo()
		engine := heal.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		observer, err := heal.NewSelfHealObserver(nil, repo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Simulate a turn error event from the pipeline
		observer.ObserveFlowLogEvent(ctx, map[string]any{
			"flow_phase":    "error",
			"step_id":       "llm-call-turn-1",
			"trace_id":      "trace-turn-001",
			"session_id":    "session-turn-heal-001",
			"auto_healed":   false,
			"error_message": "request timed out after 30s",
		})

		// Verify heal record was created
		result, err := repo.ListHealRecords(ctx, heal.HealRecordQuery{Limit: 10})
		if err != nil {
			t.Fatalf("ListHealRecords failed: %v", err)
		}
		if result.Total < 1 {
			t.Fatal("expected at least 1 heal record from turn error observation")
		}

		// Find the observed_failed record
		var observedRec *heal.HealRecord
		for i := range result.Items {
			if result.Items[i].Status == string(heal.HealStatusObservedFailed) {
				observedRec = &result.Items[i]
				break
			}
		}
		if observedRec == nil {
			t.Fatal("expected an observed_failed record from turn error")
		}
		if observedRec.SessionID != "session-turn-heal-001" {
			t.Errorf("expected session_id=session-turn-heal-001, got %q", observedRec.SessionID)
		}
	})
}
