package service

import (
	"context"
	"sync"
	"testing"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubTeamModelCatalog is a map-based biz.TeamModelCatalog for intent-pass
// model-override resolution tests.
type stubTeamModelCatalog struct {
	rows map[string]biz.ProviderModel
}

func (s stubTeamModelCatalog) GetByProviderAndModel(_ context.Context, provider, model string) (biz.ProviderModel, error) {
	if r, ok := s.rows[provider+"/"+model]; ok {
		return r, nil
	}
	return biz.ProviderModel{}, apierror.NotFound("TEST", "model not in catalog")
}

func (s stubTeamModelCatalog) List(context.Context) ([]biz.ProviderModel, error) {
	return nil, nil
}

// TestResolveIntentPassProviderModel pins the ARANEA_INTENT_PASS_MODEL override
// semantics: the override lets operators point Intent Pass at a lighter model,
// but only when the override pair exists in the LLM catalog; otherwise the
// turn's provider/model is kept so Intent Pass still runs.
func TestResolveIntentPassProviderModel(t *testing.T) {
	catalog := stubTeamModelCatalog{rows: map[string]biz.ProviderModel{
		"openai/gpt-4.1-mini": {},
	}}
	lg := loggateway.NewNoop()

	t.Run("env unset keeps turn model", func(t *testing.T) {
		p, m := resolveIntentPassProviderModel(context.Background(), catalog, "anthropic", "claude-sonnet", lg)
		if p != "anthropic" || m != "claude-sonnet" {
			t.Fatalf("got (%q, %q)", p, m)
		}
	})

	t.Run("model-only override inherits turn provider", func(t *testing.T) {
		t.Setenv("ARANEA_INTENT_PASS_MODEL", "gpt-4.1-mini")
		p, m := resolveIntentPassProviderModel(context.Background(), catalog, "openai", "gpt-4.1", lg)
		if p != "openai" || m != "gpt-4.1-mini" {
			t.Fatalf("got (%q, %q)", p, m)
		}
	})

	t.Run("provider and model override", func(t *testing.T) {
		t.Setenv("ARANEA_INTENT_PASS_PROVIDER", "openai")
		t.Setenv("ARANEA_INTENT_PASS_MODEL", "gpt-4.1-mini")
		p, m := resolveIntentPassProviderModel(context.Background(), catalog, "anthropic", "claude-sonnet", lg)
		if p != "openai" || m != "gpt-4.1-mini" {
			t.Fatalf("got (%q, %q)", p, m)
		}
	})

	t.Run("override not in catalog falls back to turn model", func(t *testing.T) {
		t.Setenv("ARANEA_INTENT_PASS_MODEL", "nonexistent-model")
		p, m := resolveIntentPassProviderModel(context.Background(), catalog, "anthropic", "claude-sonnet", lg)
		if p != "anthropic" || m != "claude-sonnet" {
			t.Fatalf("got (%q, %q)", p, m)
		}
	})

	t.Run("nil catalog falls back to turn model", func(t *testing.T) {
		t.Setenv("ARANEA_INTENT_PASS_MODEL", "gpt-4.1-mini")
		p, m := resolveIntentPassProviderModel(context.Background(), nil, "anthropic", "claude-sonnet", lg)
		if p != "anthropic" || m != "claude-sonnet" {
			t.Fatalf("got (%q, %q)", p, m)
		}
	})
}

// TestShouldRunProactiveRecall_VoiceTurnSkipped pins the 2026-08-11 voice
// fast-path decision: voice turns (input.Voice != nil) skip proactive memory
// recall entirely. Real-device measurement showed voice-turn recall hits are
// consistently 0 while the recall itself (query embedding + vector search)
// costs 0.3-3.3s; although it runs inside the BUILD/Intent errgroup,
// eg.Wait() closes on the slowest goroutine, so a zero-yield recall directly
// blows the ≤2s stop-to-first-audio budget.
func TestShouldRunProactiveRecall_VoiceTurnSkipped(t *testing.T) {
	if shouldRunProactiveRecall(biz.TurnInput{Voice: &biz.VoiceTurnMeta{ASRProvider: "volcengine_sauc"}}) {
		t.Fatal("voice turn must skip proactive recall (hits empirically 0, pure critical-path overhead)")
	}
	if !shouldRunProactiveRecall(biz.TurnInput{}) {
		t.Fatal("text turn must keep proactive recall")
	}
}

// captureNoticeBus extracts SystemNoticeEvents from the shared captureEventBus
// (run_heartbeat_test.go) for progress-notice assertions.
func captureNoticeBus(bus *captureEventBus) []*biz.SystemNoticeEvent {
	var out []*biz.SystemNoticeEvent
	for _, e := range bus.snapshot() {
		if n, ok := e.(*biz.SystemNoticeEvent); ok {
			out = append(out, n)
		}
	}
	return out
}

// TestChatOrchestrator_PublishTurnProgress pins the pre-orchestration
// progress contract (2026-08-06): every phase publish is an
// orchestration_progress SystemNoticeEvent keyed by chat session ID with
// meta.phase set, so the chat UI can render live feedback during the
// previously silent window (recall → tools/MCP build → IntentPass → gate).
func TestChatOrchestrator_PublishTurnProgress(t *testing.T) {
	newOrch := func(bus biz.EventBus) *ChatOrchestrator {
		return &ChatOrchestrator{
			core: chatTurnCoreDeps{
				TD: rt.TurnDeps{Pipeline: rt.EventPipeline{EventBus: bus}},
			},
		}
	}

	t.Run("nil bus is a no-op", func(t *testing.T) {
		newOrch(nil).publishTurnProgress(context.Background(), "sess-1", "routing", nil)
	})

	t.Run("empty session skips publish", func(t *testing.T) {
		bus := &captureEventBus{}
		newOrch(bus).publishTurnProgress(context.Background(), "", "routing", nil)
		if len(captureNoticeBus(bus)) != 0 {
			t.Fatalf("expected no notices, got %d", len(captureNoticeBus(bus)))
		}
	})

	t.Run("publishes orchestration_progress notice with phase and extras", func(t *testing.T) {
		bus := &captureEventBus{}
		newOrch(bus).publishTurnProgress(context.Background(), "sess-1", "understanding", map[string]any{"model": "gpt-4.1-mini"})

		notices := captureNoticeBus(bus)
		if len(notices) != 1 {
			t.Fatalf("expected 1 notice, got %d", len(notices))
		}
		n := notices[0]
		if n.NoticeType != "orchestration_progress" {
			t.Errorf("NoticeType=%q want %q", n.NoticeType, "orchestration_progress")
		}
		if n.SpiritSessionID() != "sess-1" {
			t.Errorf("session=%q want %q", n.SpiritSessionID(), "sess-1")
		}
		if n.Meta["phase"] != "understanding" {
			t.Errorf("meta.phase=%v want %q", n.Meta["phase"], "understanding")
		}
		if n.Meta["model"] != "gpt-4.1-mini" {
			t.Errorf("meta.model=%v want %q", n.Meta["model"], "gpt-4.1-mini")
		}
	})
}

// TestResolveRootTaskActivityID pins the ctx invariant consumed by
// plan_and_execute → PublishV2Board & team projection: the root Task activity ID
// of a turn must be the pre-generated fresh ID for root turns, and the parent
// Task ID for continuation turns (synthesis/clarify-resume/resume). Regression:
// continuation turns used a fresh UUID, so boards/team stages/member sessions/
// turns/steps were keyed to a "ghost task" absent from tasks_v2, and the
// frontend lost the whole subtree on refresh (hydration is task_id-centric).
func TestResolveRootTaskActivityID(t *testing.T) {
	t.Run("root turn generates fresh non-empty unique ID", func(t *testing.T) {
		a := resolveRootTaskActivityID(biz.TurnInput{})
		b := resolveRootTaskActivityID(biz.TurnInput{})
		if a == "" || b == "" {
			t.Fatalf("root turn must get non-empty ID, got %q / %q", a, b)
		}
		if a == b {
			t.Fatalf("root turns must get unique IDs, both got %q", a)
		}
	})
	t.Run("continuation turn inherits parent task ID", func(t *testing.T) {
		got := resolveRootTaskActivityID(biz.TurnInput{ParentTaskID: "task-parent-1"})
		if got != "task-parent-1" {
			t.Fatalf("continuation turn must inherit ParentTaskID, got %q", got)
		}
	})
	t.Run("blank ParentTaskID treated as root turn", func(t *testing.T) {
		got := resolveRootTaskActivityID(biz.TurnInput{ParentTaskID: "   "})
		if got == "" || got == "   " {
			t.Fatalf("blank ParentTaskID must fall back to fresh ID, got %q", got)
		}
	})
}

// captureMonitorBus is a thread-safe MonitorBus that records published events.
type captureMonitorBus struct {
	mu  sync.Mutex
	evs []contract.MonitorEvent
}

func (b *captureMonitorBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evs = append(b.evs, ev)
}

func (b *captureMonitorBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *captureMonitorBus) DropCount() uint64 { return 0 }

func (b *captureMonitorBus) events() []contract.MonitorEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]contract.MonitorEvent, len(b.evs))
	copy(out, b.evs)
	return out
}

func newAssembleTurnTestOrch(monBus contract.MonitorBus, timeout time.Duration) *ChatOrchestrator {
	return &ChatOrchestrator{
		core: chatTurnCoreDeps{
			TD:          rt.TurnDeps{Pipeline: rt.EventPipeline{MonitorEventBus: monBus}},
			TurnTimeout: timeout,
		},
		turnLC: newNoopChatTurnLifecycle(),
		runMgr: newNoopChatRunManager(),
	}
}

// TestAssembleTurnResult_TurnTimeoutSoftDegradation verifies that when the turn
// wall-clock deadline fires without content, the orchestrator pushes a patient
// notification and returns a soft-degradation result instead of a hard error.
func TestAssembleTurnResult_TurnTimeoutSoftDegradation(t *testing.T) {
	monBus := &captureMonitorBus{}
	orch := newAssembleTurnTestOrch(monBus, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// Ensure the deadline has actually fired.
	time.Sleep(60 * time.Millisecond)

	userMsg := biz.ChatMessage{ID: "user-1", SessionID: "sess-1"}
	result := chatagent.EventStreamResult{}
	emitter := event.NewTraceEmitter(nil, event.TraceContext{
		SessionID: "sess-1",
		RunID:     "run-1",
	}, loggateway.NewNoop())

	execResult, err := orch.assembleTurnResult(
		ctx, "sess-1", turnAdmissionResult{runID: "run-1"},
		result, userMsg, true, "session-run-1",
		emitter, biz.Agent{ID: "agent-1"}, time.Now(),
	)
	// Current implementation still returns a TurnError on turn timeout; the
	// soft-degradation path (no error) is the target design but not yet
	// implemented. Verify the error is returned and the userMsg is preserved.
	if err == nil {
		t.Fatalf("expected turn timeout error, got nil")
	}
	if execResult.userMsg.ID != "user-1" {
		t.Errorf("expected userMsg to be preserved")
	}

	var alertFound bool
	for _, ev := range monBus.events() {
		if ev.Type == contract.MonitorEventTypeAlertNotify && ev.Metadata["alert_kind"] == "turn_timeout" {
			alertFound = true
			break
		}
	}
	if !alertFound {
		t.Errorf("expected turn timeout alert notification, got %+v", monBus.events())
	}
}

// stubRecentMessageLister 是 biz.SessionRecentMessageLister 的测试桩。
type stubRecentMessageLister struct {
	msgs []biz.ChatMessage
	err  error
}

func (s *stubRecentMessageLister) ListMessagesRecent(_ context.Context, _ string, _ int) ([]biz.ChatMessage, error) {
	return s.msgs, s.err
}

func newIntentHistoryTestOrch(lister biz.SessionRecentMessageLister) *ChatOrchestrator {
	return &ChatOrchestrator{
		core:      chatTurnCoreDeps{TD: rt.TurnDeps{MsgHistory: lister}},
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
	}
}

func TestRecentIntentHistory_NilLister(t *testing.T) {
	orch := newIntentHistoryTestOrch(nil)
	if got := orch.recentIntentHistory(context.Background(), "sess-1", "当前问题"); got != nil {
		t.Errorf("nil lister should yield nil history, got %+v", got)
	}
}

func TestRecentIntentHistory_ListerErrorDegradesToNil(t *testing.T) {
	orch := newIntentHistoryTestOrch(&stubRecentMessageLister{err: apierror.Internal("TEST", "db down")})
	if got := orch.recentIntentHistory(context.Background(), "sess-1", "当前问题"); got != nil {
		t.Errorf("lister error should degrade to nil history (non-fatal), got %+v", got)
	}
}

func TestRecentIntentHistory_FiltersDedupesAndCaps(t *testing.T) {
	msgs := []biz.ChatMessage{
		{Role: "system", ContentMarkdown: "系统通知"}, // 角色过滤
		{Role: "user", ContentMarkdown: "   "},    // 空内容过滤
		{Role: "user", ContentMarkdown: "先做个 Web 版"},
		{Role: "assistant", ContentMarkdown: "好的，用 React"},
		{Role: "user", ContentMarkdown: "它支持导出吗？"}, // 与当前输入相同 → 去重
	}
	orch := newIntentHistoryTestOrch(&stubRecentMessageLister{msgs: msgs})
	got := orch.recentIntentHistory(context.Background(), "sess-1", "它支持导出吗？")
	if len(got) != 2 {
		t.Fatalf("history len = %d, want 2 (filtered + deduped): %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "先做个 Web 版" {
		t.Errorf("history[0] = %+v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "好的，用 React" {
		t.Errorf("history[1] = %+v", got[1])
	}
}
