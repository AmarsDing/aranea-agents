package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// TestStreamConsumer_V2DualPath is a placeholder integration test for the v2
// dual-path dispatch in turnStreamConsumer.
//
// It verifies that when a v2 projector is wired via StreamConsumeOptions,
// trpc events are dispatched to both v1 and v2 projectors, and the v2
// projector emits the expected v2 events (task.created, turn.started,
// step.created, step.streaming, step.completed, turn.completed, task.completed).
//
// SKIPPED: The full integration requires a realistic trpc event stream
// (chat.completion.chunk events with choices/deltas). The v2 projector's
// event translation is unit-tested in internal/agent/v2/projector_test.go;
// this test will be enabled once the v2 event stream harness is available.
func TestStreamConsumer_V2DualPath(t *testing.T) {
	t.Skip("v2 dual-path integration test: requires trpc event stream harness (see v2/projector_test.go for unit coverage)")

	ctx := context.Background()
	events := make(chan *trpcevent.Event)
	go func() {
		defer close(events)
		// TODO: emit a realistic chat.completion.chunk sequence here.
	}()

	v2Proj := v2.NewActivityProjector(nil, v2.NewDefaultSeqAssigner(), loggateway.NewNoop())
	opts := &StreamConsumeOptions{
		V2Projector: v2Proj,
	}
	meta := ProjectMeta{
		SessionID:       "sess-1",
		SpiritSessionID: "sess-1",
		RequestID:       "task-1",
		InvocationID:    "turn-1",
		AgentID:         "agent-1",
		TaskContent:     "hello",
	}

	_ = ConsumeEventStream(ctx, events, meta, opts, loggateway.NewNoop())
	// TODO: assert v2 events were emitted via a capturing sequencer.
}

// 00:52 会话补充取证：stream_consumer 对每个流式 event 写一条 Info
// （实测 8287 条/4min），属高频洪泛，违反「高频路径计数器限流」红线。
// chunk 类（text_delta/response）首条 + 每 200 条采样；重要事件逐条。
func TestShouldLogStreamEvent(t *testing.T) {
	// 高频 chunk 类：第 1、200、400 条记录，其余跳过。
	for _, evType := range []string{"text_delta", "response"} {
		if !shouldLogStreamEvent(evType, 1) {
			t.Fatalf("%s: first event must be logged", evType)
		}
		if shouldLogStreamEvent(evType, 2) || shouldLogStreamEvent(evType, 199) {
			t.Fatalf("%s: intermediate events must be skipped", evType)
		}
		if !shouldLogStreamEvent(evType, 200) {
			t.Fatalf("%s: 200th event must be logged", evType)
		}
	}
	// 低频重要事件：每条都记录。
	for _, evType := range []string{"tool_call", "runner_completion", "response_error", "unknown"} {
		for _, n := range []int64{1, 2, 57} {
			if !shouldLogStreamEvent(evType, n) {
				t.Fatalf("%s: event %d must always be logged", evType, n)
			}
		}
	}
}
