package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- Benchmarks (T6.2 performance baseline) ---
// goleak detection is configured in testinit_test.go TestMain.

// BenchmarkProcessEvent_StreamingText measures the throughput of processing
// streaming chat completion chunks through the ActivityProjector. This is the
// hot path for AF (Activity-First) real-time event projection.
//
// Baseline metrics (run on dev machine, 2026-06-18):
//   - TTFT proxy: time from first event to first activity_start envelope
//   - Throughput: events/sec for streaming text chunks
func BenchmarkProcessEvent_StreamingText(b *testing.B) {
	p, _, _ := newTestProjector(b)
	p.Configure(ProjectMeta{
		SessionID: "sess-bench",
		RequestID: "turn-bench",
		AgentID:   "agent-bench",
	}, nil)

	ctx := context.Background()
	ev := &trpcevent.Event{
		Author: "agent-bench",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{Content: "Hello world, this is a benchmark chunk"},
			}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ProcessEvent(ctx, ev)
	}
}

// BenchmarkOnTextDelta_StreamingReply measures the throughput of
// OnTextDelta calls, which is the per-chunk hot path during streaming replies.
func BenchmarkOnTextDelta_StreamingReply(b *testing.B) {
	p, _, _ := newTestProjector(b)
	p.Configure(ProjectMeta{
		SessionID: "sess-bench",
		RequestID: "turn-bench",
		AgentID:   "agent-bench",
	}, nil)

	ctx := context.Background()
	chunk := "This is a streaming text chunk for benchmarking. "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.OnTextDelta(ctx, "agent-bench", chunk)
	}
}

// BenchmarkOnMemberMessageDelta_TeamReply measures the throughput of
// OnMemberMessageDelta calls (AF-GAP-04 team member message path).
func BenchmarkOnMemberMessageDelta_TeamReply(b *testing.B) {
	p, _, _ := newTestProjector(b)
	p.Configure(ProjectMeta{
		SessionID:       "sess-bench",
		RequestID:       "turn-bench",
		TeamID:          "team-bench",
		AgentID:         "coordinator",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}},
	}, nil)

	ctx := context.Background()
	chunk := "Member reply chunk for benchmarking. "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.OnMemberMessageDelta(ctx, "worker-a", chunk)
	}
}

// BenchmarkBuildActivityEnvelope measures the envelope construction overhead,
// which is called on every activity event (start/delta/done).
func BenchmarkBuildActivityEnvelope(b *testing.B) {
	p, _, _ := newTestProjector(b)
	a := &biz.Activity{
		ID:        "act-bench",
		Kind:      biz.ActivityKindReply,
		Status:    biz.ActivityStatusRunning,
		SessionID: "sess-bench",
		TurnID:    "turn-bench",
		Content:   "Hello world",
		AgentKey:  "agent-bench",
		AgentName: "Agent Bench",
		TeamID:    "team-bench",
		Meta:      map[string]any{"member_id": "worker-a"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.buildActivityEnvelope(a, contract.EnvelopeTypeActivityStart)
	}
}
