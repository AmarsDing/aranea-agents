package graph

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- Fakes ---

// recordingReplanBus is a biz.EventBus that records published events.
// It extracts the wrapped v1 ActivityEvent from ActivityBridgeEvent payloads
// so tests can assert on the original ActivityEvent shape.
type recordingReplanBus struct {
	mu        sync.Mutex
	published []biz.ActivityEvent
}

func (b *recordingReplanBus) Publish(_ context.Context, e biz.Event) {
	bridge, ok := e.(*biz.ActivityBridgeEvent)
	if !ok {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, bridge.Event)
}

func (b *recordingReplanBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	ch := make(chan biz.Event)
	return ch, func() { close(ch) }
}

func (b *recordingReplanBus) events() []biz.ActivityEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.ActivityEvent, len(b.published))
	copy(out, b.published)
	return out
}

// --- Helpers ---

func newTestReplanner(bus *recordingReplanBus) *RuntimeReplannerImpl {
	if bus == nil {
		bus = &recordingReplanBus{}
	}
	return &RuntimeReplannerImpl{
		eventBus:     bus,
		lg:           loggateway.NewNoop().With(loggateway.Domain("runtime_replanner")),
		attemptCount: lifecycle.NewManagedMap[string, int](0),
	}
}

func testReplanExecution() *biz.GraphExecution {
	return biz.NewGraphExecution(
		context.Background(),
		"exec-1",
		"graph-1",
		"session-1",
		"running",
	)
}

// --- Tests ---

func TestRuntimeReplanner_TransientFailure_Retry(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()

	action, err := r.OnNodeFailure(
		context.Background(),
		exec,
		"step1",
		errors.New("node step1 execution failed: connection timeout"),
	)
	if err != nil {
		t.Fatalf("OnNodeFailure failed: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	if action.Type != ReplanRetry {
		t.Errorf("Type=%q want %q", action.Type, ReplanRetry)
	}
}

func TestRuntimeReplanner_AgentIncapable_InsertFallback(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()

	action, err := r.OnNodeFailure(
		context.Background(),
		exec,
		"step2",
		errors.New("agent incapable: cannot handle this task type"),
	)
	if err != nil {
		t.Fatalf("OnNodeFailure failed: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	if action.Type != ReplanInsertFallback {
		t.Errorf("Type=%q want %q", action.Type, ReplanInsertFallback)
	}
	// insert_fallback should produce at least one new node
	if len(action.NewNodes) == 0 {
		t.Error("expected at least one new fallback node")
	}
	// insert_fallback should produce edges connecting the fallback node
	if len(action.NewEdges) == 0 {
		t.Error("expected at least one new edge for fallback")
	}
}

func TestRuntimeReplanner_SubtaskInvalid_RebuildSubgraph(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()

	action, err := r.OnNodeFailure(
		context.Background(),
		exec,
		"step3",
		errors.New("subtask invalid: malformed input schema"),
	)
	if err != nil {
		t.Fatalf("OnNodeFailure failed: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	if action.Type != ReplanRebuildSubgraph {
		t.Errorf("Type=%q want %q", action.Type, ReplanRebuildSubgraph)
	}
	// rebuild_subgraph should produce new nodes
	if len(action.NewNodes) == 0 {
		t.Error("expected at least one new node for rebuilt subgraph")
	}
	// rebuild_subgraph should skip the failed node
	if len(action.SkipNodes) == 0 {
		t.Error("expected SkipNodes to contain the failed node")
	}
}

func TestRuntimeReplanner_RouteBlocked_Reroute(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()

	action, err := r.OnNodeFailure(
		context.Background(),
		exec,
		"step4",
		errors.New("route blocked: downstream node unreachable"),
	)
	if err != nil {
		t.Fatalf("OnNodeFailure failed: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	if action.Type != ReplanReroute {
		t.Errorf("Type=%q want %q", action.Type, ReplanReroute)
	}
}

func TestRuntimeReplanner_UnknownFailure_ReturnsError(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()

	action, err := r.OnNodeFailure(
		context.Background(),
		exec,
		"step5",
		errors.New("something completely unexpected happened"),
	)
	if err == nil {
		t.Fatal("expected error for unknown failure type, got nil")
	}
	if action != nil {
		t.Errorf("expected nil action on error, got %+v", action)
	}
	// Verify it's an apierror.Internal
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeInternal {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeInternal)
	}
	if apiErr.Domain != apierror.DomainGraph {
		t.Errorf("Domain=%q want %q", apiErr.Domain, apierror.DomainGraph)
	}
}

func TestRuntimeReplanner_NilExecution_ReturnsError(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)

	action, err := r.OnNodeFailure(
		context.Background(),
		nil,
		"step1",
		errors.New("timeout"),
	)
	if err == nil {
		t.Fatal("expected error for nil execution, got nil")
	}
	if action != nil {
		t.Errorf("expected nil action on nil execution, got %+v", action)
	}
}

func TestRuntimeReplanner_NilError_TreatedAsUnknown(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()

	// nil error should be treated as unknown failure (no keywords to match)
	action, err := r.OnNodeFailure(
		context.Background(),
		exec,
		"step1",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for nil error input, got nil")
	}
	if action != nil {
		t.Errorf("expected nil action on nil error, got %+v", action)
	}
}

func TestRuntimeReplanner_MaxAttemptsExceeded_ReturnsError(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()
	ctx := context.Background()

	// Exhaust the max replan attempts (default 3)
	transientErr := errors.New("connection timeout")
	for i := 0; i < maxReplanAttempts; i++ {
		action, err := r.OnNodeFailure(ctx, exec, "step1", transientErr)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", i, err)
		}
		if action == nil || action.Type != ReplanRetry {
			t.Fatalf("attempt %d: expected retry action, got %+v", i, action)
		}
	}

	// Next call should exceed the limit
	action, err := r.OnNodeFailure(ctx, exec, "step1", transientErr)
	if err == nil {
		t.Fatal("expected error when max replan attempts exceeded, got nil")
	}
	if action != nil {
		t.Errorf("expected nil action when max exceeded, got %+v", action)
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeInternal {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeInternal)
	}
}

func TestRuntimeReplanner_PublishesReplanEvent(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()

	_, err := r.OnNodeFailure(
		context.Background(),
		exec,
		"step1",
		errors.New("connection timeout"),
	)
	if err != nil {
		t.Fatalf("OnNodeFailure failed: %v", err)
	}

	events := bus.events()
	if len(events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(events))
	}
	ev := events[0]
	if ev.Event != biz.ActivityEventUpdated {
		t.Errorf("Event=%q want %q", ev.Event, biz.ActivityEventUpdated)
	}
	if ev.Activity.Kind != biz.ActivityKindGraphStage {
		t.Errorf("Kind=%q want %q", ev.Activity.Kind, biz.ActivityKindGraphStage)
	}
	if ev.Activity.Stage != "replanned" {
		t.Errorf("Stage=%q want %q", ev.Activity.Stage, "replanned")
	}
	if ev.Domain != biz.ActivityDomainChat {
		t.Errorf("Domain=%q want %q", ev.Domain, biz.ActivityDomainChat)
	}
	if ev.Activity.SessionID != exec.SessionID {
		t.Errorf("SessionID=%q want %q", ev.Activity.SessionID, exec.SessionID)
	}
	// Verify metadata carries replan details
	if ev.Activity.Meta == nil {
		t.Fatal("expected non-nil Meta")
	}
	if v, ok := ev.Activity.Meta["replan_type"].(string); !ok || v != string(ReplanRetry) {
		t.Errorf("Meta[replan_type]=%v want %q", ev.Activity.Meta["replan_type"], ReplanRetry)
	}
	if v, ok := ev.Activity.Meta["failed_node"].(string); !ok || v != "step1" {
		t.Errorf("Meta[failed_node]=%v want %q", ev.Activity.Meta["failed_node"], "step1")
	}
	if v, ok := ev.Activity.Meta["execution_id"].(string); !ok || v != exec.ID {
		t.Errorf("Meta[execution_id]=%v want %q", ev.Activity.Meta["execution_id"], exec.ID)
	}
}

func TestRuntimeReplanner_NoEventOnMaxExceeded(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	exec := testReplanExecution()
	ctx := context.Background()
	transientErr := errors.New("connection timeout")

	// Exhaust the max replan attempts
	for i := 0; i < maxReplanAttempts; i++ {
		_, _ = r.OnNodeFailure(ctx, exec, "step1", transientErr)
	}
	// Clear published events so we can verify no new event on the exceeding call
	bus.mu.Lock()
	bus.published = bus.published[:0]
	bus.mu.Unlock()

	// Next call should exceed the limit and NOT publish an event
	_, err := r.OnNodeFailure(ctx, exec, "step1", transientErr)
	if err == nil {
		t.Fatal("expected error when max replan attempts exceeded")
	}
	if len(bus.events()) != 0 {
		t.Errorf("expected 0 events on max-exceeded error, got %d", len(bus.events()))
	}
}

func TestRuntimeReplanner_AttemptCountTrackedPerExecution(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	ctx := context.Background()
	transientErr := errors.New("connection timeout")

	// Use up 2 attempts on exec-1
	exec1 := testReplanExecution() // exec-1
	for i := 0; i < 2; i++ {
		_, err := r.OnNodeFailure(ctx, exec1, "step1", transientErr)
		if err != nil {
			t.Fatalf("exec1 attempt %d: unexpected error: %v", i, err)
		}
	}

	// exec-2 should have its own counter (still has all 3 attempts)
	exec2 := biz.NewGraphExecution(ctx, "exec-2", "graph-1", "session-2", "running")
	for i := 0; i < maxReplanAttempts; i++ {
		_, err := r.OnNodeFailure(ctx, exec2, "step1", transientErr)
		if err != nil {
			t.Fatalf("exec2 attempt %d: unexpected error: %v", i, err)
		}
	}
	// exec-2 should now be at the limit
	_, err := r.OnNodeFailure(ctx, exec2, "step1", transientErr)
	if err == nil {
		t.Fatal("expected exec2 to hit max attempts")
	}

	// exec-1 should still have 1 attempt left (used 2 of 3)
	action, err := r.OnNodeFailure(ctx, exec1, "step1", transientErr)
	if err != nil {
		t.Fatalf("exec1 should still have 1 attempt left: %v", err)
	}
	if action == nil || action.Type != ReplanRetry {
		t.Errorf("exec1 expected retry action, got %+v", action)
	}
}

func TestAnalyzeFailure_Transient(t *testing.T) {
	r := newTestReplanner(nil)
	tests := []struct {
		name string
		err  error
	}{
		{"timeout", errors.New("operation timeout")},
		{"connection", errors.New("connection refused")},
		{"temporary", errors.New("temporary failure")},
		{"transient", errors.New("transient error")},
		{"deadline", errors.New("deadline exceeded")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := r.analyzeFailure(tt.err)
			if analysis.Severity != failureSeverityTransient {
				t.Errorf("Severity=%q want %q", analysis.Severity, failureSeverityTransient)
			}
			if analysis.SuggestedAction != ReplanRetry {
				t.Errorf("SuggestedAction=%q want %q", analysis.SuggestedAction, ReplanRetry)
			}
		})
	}
}

func TestAnalyzeFailure_AgentIncapable(t *testing.T) {
	r := newTestReplanner(nil)
	tests := []struct {
		name string
		err  error
	}{
		{"incapable", errors.New("agent incapable of handling request")},
		{"unable to handle", errors.New("unable to handle this task")},
		{"not supported", errors.New("operation not supported")},
		{"unsupported", errors.New("unsupported capability")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := r.analyzeFailure(tt.err)
			if analysis.Severity != failureSeverityAgentIncapable {
				t.Errorf("Severity=%q want %q", analysis.Severity, failureSeverityAgentIncapable)
			}
			if analysis.SuggestedAction != ReplanInsertFallback {
				t.Errorf("SuggestedAction=%q want %q", analysis.SuggestedAction, ReplanInsertFallback)
			}
		})
	}
}

func TestAnalyzeFailure_SubtaskInvalid(t *testing.T) {
	r := newTestReplanner(nil)
	tests := []struct {
		name string
		err  error
	}{
		{"invalid", errors.New("invalid subtask definition")},
		{"malformed", errors.New("malformed input schema")},
		{"incorrect", errors.New("incorrect parameters")},
		{"bad request", errors.New("bad request format")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := r.analyzeFailure(tt.err)
			if analysis.Severity != failureSeveritySubtaskInvalid {
				t.Errorf("Severity=%q want %q", analysis.Severity, failureSeveritySubtaskInvalid)
			}
			if analysis.SuggestedAction != ReplanRebuildSubgraph {
				t.Errorf("SuggestedAction=%q want %q", analysis.SuggestedAction, ReplanRebuildSubgraph)
			}
		})
	}
}

func TestAnalyzeFailure_RouteBlocked(t *testing.T) {
	r := newTestReplanner(nil)
	tests := []struct {
		name string
		err  error
	}{
		{"blocked", errors.New("route blocked by policy")},
		{"unreachable", errors.New("downstream node unreachable")},
		{"denied", errors.New("access denied")},
		{"forbidden", errors.New("operation forbidden")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := r.analyzeFailure(tt.err)
			if analysis.Severity != failureSeverityRouteBlocked {
				t.Errorf("Severity=%q want %q", analysis.Severity, failureSeverityRouteBlocked)
			}
			if analysis.SuggestedAction != ReplanReroute {
				t.Errorf("SuggestedAction=%q want %q", analysis.SuggestedAction, ReplanReroute)
			}
		})
	}
}

func TestAnalyzeFailure_Unknown(t *testing.T) {
	r := newTestReplanner(nil)
	analysis := r.analyzeFailure(errors.New("something completely unexpected"))
	if analysis.Severity != failureSeverityUnknown {
		t.Errorf("Severity=%q want %q", analysis.Severity, failureSeverityUnknown)
	}
	if analysis.SuggestedAction != "" {
		t.Errorf("SuggestedAction=%q want empty", analysis.SuggestedAction)
	}
}

func TestAnalyzeFailure_NilError(t *testing.T) {
	r := newTestReplanner(nil)
	analysis := r.analyzeFailure(nil)
	if analysis.Severity != failureSeverityUnknown {
		t.Errorf("Severity=%q want %q", analysis.Severity, failureSeverityUnknown)
	}
}

// TestRuntimeReplanner_ConcurrentAccess verifies that OnNodeFailure is safe
// for concurrent use across multiple executions (BD5 concurrency test).
// The attemptCount is protected by ManagedMap's internal mutex; this test
// exercises concurrent reads/writes to catch data races when run with -race.
func TestRuntimeReplanner_ConcurrentAccess(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	ctx := context.Background()
	transientErr := errors.New("connection timeout")

	const numExecs = 8
	var wg sync.WaitGroup
	wg.Add(numExecs)
	for i := 0; i < numExecs; i++ {
		go func(idx int) {
			defer wg.Done()
			exec := biz.NewGraphExecution(ctx,
				"exec-concurrent-"+strconv.Itoa(idx),
				"graph-1", "session-"+strconv.Itoa(idx), "running")
			// Each execution can have up to maxReplanAttempts (3) successful calls.
			for j := 0; j < maxReplanAttempts; j++ {
				_, err := r.OnNodeFailure(ctx, exec, "step1", transientErr)
				if err != nil {
					t.Errorf("exec %d attempt %d: %v", idx, j, err)
				}
			}
			// The next call should exceed the limit.
			_, err := r.OnNodeFailure(ctx, exec, "step1", transientErr)
			if err == nil {
				t.Errorf("exec %d: expected max-exceeded error", idx)
			}
		}(i)
	}
	wg.Wait()

	// Verify total published events = numExecs * maxReplanAttempts
	// (only successful replans publish events; max-exceeded errors don't).
	got := len(bus.events())
	want := numExecs * maxReplanAttempts
	if got != want {
		t.Errorf("published events = %d, want %d", got, want)
	}
}
