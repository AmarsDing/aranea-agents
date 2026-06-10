package event

import (
	"context"
	"sync"
	"testing"
	"time"
)

type captureBus struct {
	mu   sync.Mutex
	envs []Envelope
}

func (b *captureBus) Publish(_ context.Context, env Envelope) {
	b.mu.Lock()
	b.envs = append(b.envs, env)
	b.mu.Unlock()
}

func (b *captureBus) Subscribe(_ SubscribeOptions) (<-chan Envelope, func()) {
	return nil, func() {}
}

func (b *captureBus) DropCount() uint64 { return 0 }

func TestTraceEmitterPublishesFlowLog(t *testing.T) {
	bus := &captureBus{}
	tc := TraceContext{
		TraceID:   "tr_test",
		SessionID: "sess_1",
		RunID:     "run_1",
		Domain:    TraceDomainChat,
		AgentKey:  "a1",
	}
	em := NewTraceEmitter(bus, nil, tc, nil)
	em.LogStart("chat.llm.invoke", "正在调用语言模型")
	em.LogDone("chat.llm.invoke", "模型已返回")
	time.Sleep(50 * time.Millisecond)

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.envs) < 2 {
		t.Fatalf("expected >=2 flow_log envelopes, got %d", len(bus.envs))
	}
	for _, env := range bus.envs {
		if env.Type != EnvelopeTypeFlowLog {
			t.Fatalf("expected flow_log, got %s", env.Type)
		}
		sev, _ := env.Metadata["severity"].(string)
		if sev == "" {
			t.Fatal("missing severity in metadata")
		}
		title, _ := env.Metadata["title"].(string)
		if title == "" {
			t.Fatal("missing title in metadata")
		}
	}
}

func TestTraceEmitterSkipsChatErrorForMonitorOnlySteps(t *testing.T) {
	bus := &captureBus{}
	em := NewTraceEmitter(bus, nil, TraceContext{SessionID: "sess_1"}, nil)
	em.LogError("chat.usage_record", "用量落库失败")
	time.Sleep(50 * time.Millisecond)

	bus.mu.Lock()
	defer bus.mu.Unlock()
	for _, env := range bus.envs {
		if env.Type == EnvelopeTypeError {
			t.Fatalf("expected no chat error envelope for chat.usage_record, got %+v", env)
		}
	}
}

func TestTraceEmitterMetadataJSON(t *testing.T) {
	em := NewTraceEmitter(nil, nil, TraceContext{TraceID: "tr_x", RunID: "r1"}, nil)
	em.FinishRoot("ok")
	raw := em.MetadataJSON()
	if raw == "" || raw == "{}" {
		t.Fatal("expected metadata with spans")
	}
}

// TestEmitProgress_StartDoneCapturesDuration verifies that calling EmitProgress
// with phase=start records a flow timing, and the subsequent phase=done envelope
// carries the measured duration_ms in its metadata.
//
// See docs/reports/2026-06-10-proposal-execution-progress-inline.md
func TestEmitProgress_StartDoneCapturesDuration(t *testing.T) {
	bus := &captureBus{}
	tc := TraceContext{SessionID: "sess_1", RunID: "run_1"}
	em := NewTraceEmitter(bus, nil, tc, nil)
	ctx := context.Background()

	em.EmitProgress(ctx, "chat.llm.invoke", "start", "正在调用语言模型", "orchestration",
		P("provider", "openai"), P("model", "gpt-4"))
	time.Sleep(20 * time.Millisecond)
	em.EmitProgress(ctx, "chat.llm.invoke", "done", "语言模型已返回", "orchestration")

	bus.mu.Lock()
	defer bus.mu.Unlock()

	if got := len(bus.envs); got != 2 {
		t.Fatalf("expected 2 execution_progress envelopes, got %d", got)
	}
	start, done := bus.envs[0], bus.envs[1]
	if start.Type != EnvelopeTypeExecutionProgress || done.Type != EnvelopeTypeExecutionProgress {
		t.Fatalf("expected type=execution_progress, got start=%s done=%s", start.Type, done.Type)
	}
	if start.Metadata["phase"] != "start" {
		t.Fatalf("expected start phase, got %v", start.Metadata["phase"])
	}
	if start.Metadata["step_id"] != "chat.llm.invoke" {
		t.Fatalf("expected step_id=chat.llm.invoke, got %v", start.Metadata["step_id"])
	}
	if start.Metadata["category"] != "orchestration" {
		t.Fatalf("expected category=orchestration, got %v", start.Metadata["category"])
	}
	if start.Metadata["message"] != "正在调用语言模型" {
		t.Fatalf("expected message=正在调用语言模型, got %v", start.Metadata["message"])
	}
	// extra Pair values should be merged into metadata
	if start.Metadata["provider"] != "openai" {
		t.Fatalf("expected provider=openai in metadata, got %v", start.Metadata["provider"])
	}

	if done.Metadata["phase"] != "done" {
		t.Fatalf("expected done phase, got %v", done.Metadata["phase"])
	}
	dur, ok := done.Metadata["duration_ms"].(int64)
	if !ok || dur < 10 {
		t.Fatalf("expected duration_ms >= 10 from recorded timing, got %v", done.Metadata["duration_ms"])
	}
	// Channel must be chat so the WS routes it to the chat subscriber
	if start.Channel != "chat" {
		t.Fatalf("expected channel=chat, got %q", start.Channel)
	}
}

// TestEmitProgress_ErrorPhase verifies that phase=error publishes an envelope
// with the error message attached, but does NOT record a duration (error may
// occur before/during the step).
func TestEmitProgress_ErrorPhase(t *testing.T) {
	bus := &captureBus{}
	em := NewTraceEmitter(bus, nil, TraceContext{SessionID: "sess_1"}, nil)
	em.EmitProgress(context.Background(), "chat.llm.invoke", "error", "语言模型调用失败", "orchestration",
		P("error", "timeout"))

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if got := len(bus.envs); got != 1 {
		t.Fatalf("expected 1 envelope, got %d", got)
	}
	env := bus.envs[0]
	if env.Metadata["phase"] != "error" {
		t.Fatalf("expected phase=error, got %v", env.Metadata["phase"])
	}
	if env.Metadata["error"] != "timeout" {
		t.Fatalf("expected error=timeout, got %v", env.Metadata["error"])
	}
	if _, has := env.Metadata["duration_ms"]; has {
		t.Fatalf("error envelope should not carry duration_ms, got %v", env.Metadata["duration_ms"])
	}
}

// TestEmitProgress_NilEmitterSafe verifies that calling EmitProgress on a
// nil emitter is a no-op (defensive: callers pass the emitter through
// dependency injection and must not nil-deref).
func TestEmitProgress_NilEmitterSafe(t *testing.T) {
	var em *TraceEmitter
	em.EmitProgress(context.Background(), "chat.llm.invoke", "start", "msg", "orchestration") // must not panic
}

// TestEmitProgress_NilInfraSafe verifies that EmitProgress is a no-op when
// the emitter was constructed without infra (e.g. legacy test fixtures).
func TestEmitProgress_NilInfraSafe(t *testing.T) {
	bus := &captureBus{}
	em := NewTraceEmitter(bus, nil, TraceContext{SessionID: "sess_1"}, nil)
	em.infra = nil // simulate legacy construction
	em.EmitProgress(context.Background(), "chat.llm.invoke", "start", "msg", "orchestration")

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if got := len(bus.envs); got != 0 {
		t.Fatalf("expected 0 envelopes when infra is nil, got %d", got)
	}
}

// TestEmitProgress_OrchestratorCallsitePattern is a S10 integration-style test
// that simulates the exact 3-call pattern from chat_orchestrator_turn.go
// (start → done | error) and verifies the envelope metadata shape that the
// frontend's useAgentBlocks will consume.
//
// This test guards against accidental contract drift between the backend
// EmitProgress callsite and the frontend readExecutionProgressMetadata
// parser. If you change the field names here, you must also update
// EnvelopeExecutionProgressMetadata in web/src/realtime/envelope.ts.
//
// See docs/reports/2026-06-10-proposal-execution-progress-inline.md
func TestEmitProgress_OrchestratorCallsitePattern(t *testing.T) {
	t.Run("start: typical chat.llm.invoke entry", func(t *testing.T) {
		bus := &captureBus{}
		em := NewTraceEmitter(bus, nil, TraceContext{SessionID: "sess_1", RunID: "r1"}, nil)
		em.EmitProgress(context.Background(), "chat.llm.invoke", "start", "正在调用语言模型", "orchestration",
			P("run_id", "r1"), P("provider", "openai"), P("model", "gpt-4"))

		bus.mu.Lock()
		defer bus.mu.Unlock()
		if got := len(bus.envs); got != 1 {
			t.Fatalf("expected 1 envelope, got %d", got)
		}
		env := bus.envs[0]
		required := []string{"step_id", "phase", "message", "category", "run_id", "provider", "model"}
		for _, key := range required {
			if _, ok := env.Metadata[key]; !ok {
				t.Errorf("required metadata key %q missing; got %+v", key, env.Metadata)
			}
		}
		if env.Type != EnvelopeTypeExecutionProgress {
			t.Fatalf("expected type=execution_progress, got %s", env.Type)
		}
		if env.Author != "system" {
			t.Fatalf("expected author=system, got %q", env.Author)
		}
		if env.SessionID != "sess_1" {
			t.Fatalf("expected session_id=sess_1, got %q", env.SessionID)
		}
		if env.Channel != "chat" {
			t.Fatalf("expected channel=chat, got %q", env.Channel)
		}
	})

	t.Run("done: completion must carry duration_ms and override message", func(t *testing.T) {
		bus := &captureBus{}
		em := NewTraceEmitter(bus, nil, TraceContext{SessionID: "sess_1", RunID: "r1"}, nil)
		// Real orchestrator pattern: emit start first to record timing, then done.
		em.EmitProgress(context.Background(), "chat.llm.invoke", "start", "正在调用语言模型", "orchestration",
			P("run_id", "r1"), P("provider", "openai"), P("model", "gpt-4"))
		time.Sleep(15 * time.Millisecond)
		em.EmitProgress(context.Background(), "chat.llm.invoke", "done", "语言模型已返回", "orchestration",
			P("run_id", "r1"))

		bus.mu.Lock()
		defer bus.mu.Unlock()
		if got := len(bus.envs); got != 2 {
			t.Fatalf("expected 2 envelopes (start + done), got %d", got)
		}
		env := bus.envs[1]
		required := []string{"step_id", "phase", "message", "category", "run_id", "duration_ms"}
		for _, key := range required {
			if _, ok := env.Metadata[key]; !ok {
				t.Errorf("required metadata key %q missing; got %+v", key, env.Metadata)
			}
		}
		// duration_ms should be a real number from the recorded timing
		dur, ok := env.Metadata["duration_ms"].(int64)
		if !ok || dur < 10 {
			t.Fatalf("expected duration_ms >= 10 (real measurement), got %v", env.Metadata["duration_ms"])
		}
		// message should be the done-phase message, not the start message
		if env.Metadata["message"] != "语言模型已返回" {
			t.Fatalf("expected message=语言模型已返回, got %v", env.Metadata["message"])
		}
		// provider/model from start should NOT auto-flow to done (frontend would
		// only see them in the start envelope; if it needs them on done, the
		// orchestrator must re-emit them as Pair extras).
		for _, forbidden := range []string{"provider", "model"} {
			if _, ok := env.Metadata[forbidden]; ok {
				t.Errorf("done envelope should not carry start-only metadata %q; got %+v", forbidden, env.Metadata)
			}
		}
	})

	t.Run("error: failure path with error message", func(t *testing.T) {
		bus := &captureBus{}
		em := NewTraceEmitter(bus, nil, TraceContext{SessionID: "sess_1", RunID: "r1"}, nil)
		em.EmitProgress(context.Background(), "chat.llm.invoke", "error", "语言模型调用失败", "orchestration",
			P("run_id", "r1"), P("error", "timeout"))

		bus.mu.Lock()
		defer bus.mu.Unlock()
		if got := len(bus.envs); got != 1 {
			t.Fatalf("expected 1 envelope, got %d", got)
		}
		env := bus.envs[0]
		required := []string{"step_id", "phase", "message", "category", "run_id", "error"}
		for _, key := range required {
			if _, ok := env.Metadata[key]; !ok {
				t.Errorf("required metadata key %q missing; got %+v", key, env.Metadata)
			}
		}
		// error envelope should not carry duration_ms (failure may not have a measured duration)
		if _, has := env.Metadata["duration_ms"]; has {
			t.Errorf("error envelope should not carry duration_ms, got %v", env.Metadata["duration_ms"])
		}
	})
}

// TestStepIDChatLLMInvoke_Constant is a S4 guard test that prevents
// accidental renaming of the canonical step_id string used by both the
// orchestrator callsite and the frontend mergeProgressEvents step key.
func TestStepIDChatLLMInvoke_Constant(t *testing.T) {
	if StepIDChatLLMInvoke != "chat.llm.invoke" {
		t.Fatalf("StepIDChatLLMInvoke constant changed: %q (frontend relies on this literal)", StepIDChatLLMInvoke)
	}

	// Round-trip: emit and re-read using the constant
	bus := &captureBus{}
	em := NewTraceEmitter(bus, nil, TraceContext{SessionID: "sess_1"}, nil)
	em.EmitProgress(context.Background(), StepIDChatLLMInvoke, "start", "m", "orchestration")

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if got := bus.envs[0].Metadata["step_id"]; got != "chat.llm.invoke" {
		t.Fatalf("step_id metadata = %v, want chat.llm.invoke", got)
	}
}
