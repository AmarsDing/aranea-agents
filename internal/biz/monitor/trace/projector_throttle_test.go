package trace_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor/trace"
	"aranea-agents/internal/event/contract"
)

// stubMonitorBus is a minimal contract.MonitorBus implementation for tests
// that drive the projector via HandleExposed without a real subscription.
type stubMonitorBus struct{}

func (b *stubMonitorBus) Publish(context.Context, contract.MonitorEvent) {}
func (b *stubMonitorBus) Subscribe(contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	ch := make(chan contract.MonitorEvent)
	return ch, func() {}
}
func (b *stubMonitorBus) DropCount() uint64 { return 0 }

// Fix D2: when the trace repo is down (e.g. PG connection lost), the
// per-span UpsertMonitorTraceSpan Warn must be time-window throttled instead
// of flooding one Warn per span event.
func TestTraceProjector_UpsertSpanWarnThrottled(t *testing.T) {
	repo := &mockTraceWriter{
		upsertMonitorTraceSpanFn: func(context.Context, trace.TraceSpanWrite) error {
			return errors.New("pg down")
		},
	}
	lg := &warnCountingLogger{}
	p := trace.NewTraceProjector(repo, lg, nil, &stubMonitorBus{})
	if p == nil {
		t.Fatal("NewTraceProjector returned nil")
	}
	p.SetUpsertWarnIntervalForTest(time.Minute)

	ev := func(id string) contract.MonitorEvent {
		return contract.MonitorEvent{
			ID:        id,
			Type:      contract.MonitorEventTypeFlowLog,
			Timestamp: time.Now().UTC(),
			Source:    "flow",
			Metadata: map[string]any{
				"trace_id": "trace-1",
				"step_id":  "llm.call",
				"domain":   "chat",
			},
		}
	}

	// First failure warns; subsequent failures within the window are silent.
	p.HandleExposed(context.Background(), ev("e1"))
	p.HandleExposed(context.Background(), ev("e2"))
	p.HandleExposed(context.Background(), ev("e3"))
	if lg.warnCount != 1 {
		t.Fatalf("warn count within throttle window = %d, want 1", lg.warnCount)
	}

	// After the window expires, the next failure warns again.
	p.SetUpsertWarnIntervalForTest(20 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	p.HandleExposed(context.Background(), ev("e4"))
	if lg.warnCount != 2 {
		t.Fatalf("warn count after window expiry = %d, want 2", lg.warnCount)
	}
}

// P6: when the repo is down, every new trace's InsertMonitorTrace fails.
// The Warn must be time-window throttled instead of one Warn per trace.
func TestTraceProjector_InsertTraceWarnThrottled(t *testing.T) {
	repo := &mockTraceWriter{
		insertMonitorTraceFn: func(context.Context, trace.TraceWrite) error {
			return errors.New("pg down")
		},
	}
	lg := &warnCountingLogger{}
	p := trace.NewTraceProjector(repo, lg, nil, &stubMonitorBus{})
	if p == nil {
		t.Fatal("NewTraceProjector returned nil")
	}
	p.SetRepoWarnIntervalForTest(time.Minute)

	ctx := context.Background()
	p.EnsureTraceExposed(ctx, "t1", "", "", "", "", "", "", "")
	p.EnsureTraceExposed(ctx, "t2", "", "", "", "", "", "", "")
	p.EnsureTraceExposed(ctx, "t3", "", "", "", "", "", "", "")
	if lg.warnCount != 1 {
		t.Fatalf("insert warn count within throttle window = %d, want 1", lg.warnCount)
	}

	p.SetRepoWarnIntervalForTest(20 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	p.EnsureTraceExposed(ctx, "t4", "", "", "", "", "", "", "")
	if lg.warnCount != 2 {
		t.Fatalf("insert warn count after window expiry = %d, want 2", lg.warnCount)
	}
}

// P6: when the repo is down, every runner completion's
// UpdateMonitorTraceCompletion fails. The Warn must be throttled.
func TestTraceProjector_CompletionWarnThrottled(t *testing.T) {
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(context.Context, string, trace.TraceCompletion) error {
			return errors.New("pg down")
		},
	}
	lg := &warnCountingLogger{}
	p := trace.NewTraceProjector(repo, lg, nil, &stubMonitorBus{})
	if p == nil {
		t.Fatal("NewTraceProjector returned nil")
	}
	p.SetRepoWarnIntervalForTest(time.Minute)

	ctx := context.Background()
	p.OnRunnerCompletion(ctx, "t1", "ok", 10)
	p.OnRunnerCompletion(ctx, "t2", "ok", 10)
	p.OnRunnerCompletion(ctx, "t3", "ok", 10)
	if lg.warnCount != 1 {
		t.Fatalf("completion warn count within throttle window = %d, want 1", lg.warnCount)
	}

	p.SetRepoWarnIntervalForTest(20 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	p.OnRunnerCompletion(ctx, "t4", "ok", 10)
	if lg.warnCount != 2 {
		t.Fatalf("completion warn count after window expiry = %d, want 2", lg.warnCount)
	}
}

// P6: when the usage repo is down, every runner completion's
// AggregateUsageByTrace fails. The Warn must be throttled.
func TestTraceProjector_UsageAggWarnThrottled(t *testing.T) {
	repo := &mockTraceWriter{}
	usage := &mockUsageRepo{
		aggregateFn: func(context.Context, string) (trace.UsageAggregate, error) {
			return trace.UsageAggregate{}, errors.New("pg down")
		},
	}
	lg := &warnCountingLogger{}
	p := trace.NewTraceProjector(repo, lg, usage, &stubMonitorBus{})
	if p == nil {
		t.Fatal("NewTraceProjector returned nil")
	}
	p.SetRepoWarnIntervalForTest(time.Minute)

	ctx := context.Background()
	p.OnRunnerCompletion(ctx, "t1", "ok", 10)
	p.OnRunnerCompletion(ctx, "t2", "ok", 10)
	p.OnRunnerCompletion(ctx, "t3", "ok", 10)
	if lg.warnCount != 1 {
		t.Fatalf("usage-agg warn count within throttle window = %d, want 1", lg.warnCount)
	}

	p.SetRepoWarnIntervalForTest(20 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	p.OnRunnerCompletion(ctx, "t4", "ok", 10)
	if lg.warnCount != 2 {
		t.Fatalf("usage-agg warn count after window expiry = %d, want 2", lg.warnCount)
	}
}

func TestSpanKindFromStep(t *testing.T) {
	tests := []struct {
		name   string
		stepID string
		domain string
		want   string
	}{
		{"empty_step", "", "", ""},
		{"chat_turn", "chat.turn", "", "root"},
		{"turn", "turn", "", "root"},
		{"llm_prefix", "llm.call", "", "llm"},
		{"model_prefix", "model.complete", "", "llm"},
		{"completion_prefix", "completion.generate", "", "llm"},
		{"tool_prefix", "tool.search", "", "tool"},
		{"function_prefix", "function.execute", "", "tool"},
		{"tool_suffix", "my.tool", "", "tool"},
		{"function_suffix", "my.function", "", "tool"},
		{"retrieve_prefix", "retrieve.docs", "", "memory"},
		{"memory_prefix", "memory.store", "", "memory"},
		{"recall_prefix", "recall.context", "", "memory"},
		{"graph_prefix", "graph.run", "", "graph"},
		{"node_prefix", "node.step", "", "graph"},
		{"node_suffix", "my.node", "", "graph"},
		{"hitl_prefix", "hitl.confirm", "", "hitl"},
		{"human_prefix", "human.review", "", "hitl"},
		{"confirm_prefix", "confirm.action", "", "hitl"},
		{"team_prefix", "team.delegate", "", "team"},
		{"subteam_prefix", "subteam.run", "", "team"},
		{"unknown_step", "unknown.step", "", "step"},
		{"case_insensitive", "LLM.Call", "", "llm"},
		{"case_insensitive_tool", "Tool.Search", "", "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trace.SpanKindFromStep(tt.stepID, tt.domain)
			if got != tt.want {
				t.Errorf("SpanKindFromStep(%q, %q) = %q, want %q", tt.stepID, tt.domain, got, tt.want)
			}
		})
	}
}

func TestMetaStr(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want string
	}{
		{"nil_map", nil, "key", ""},
		{"missing_key", map[string]any{"other": "val"}, "key", ""},
		{"nil_value", map[string]any{"key": nil}, "key", ""},
		{"string_value", map[string]any{"key": "hello"}, "key", "hello"},
		{"int_value", map[string]any{"key": 42}, "key", "42"},
		{"float_value", map[string]any{"key": 3.14}, "key", "3.14"},
		{"bool_value", map[string]any{"key": true}, "key", "true"},
		{"trimmed_value", map[string]any{"key": "  hello  "}, "key", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trace.MetaStr(tt.m, tt.key)
			if got != tt.want {
				t.Errorf("MetaStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCoalesceStr(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{"both_nonempty", "first", "second", "first"},
		{"a_empty", "", "second", "second"},
		{"a_whitespace", "  ", "second", "second"},
		{"both_empty", "", "", ""},
		{"both_whitespace", "  ", "  ", "  "},
		{"a_nonempty_b_empty", "first", "", "first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trace.CoalesceStr(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CoalesceStr(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
