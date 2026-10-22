package agent

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// stream_consumer_invalid_test.go — P0-4（A9）：消费端 finalize 打标
// InvalidKind / ToolCallDispatched / PartialContentHadFirstByte 的回归 pin。

func invalidDeltaEvent(text string) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{
				{Delta: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: text}},
			},
		},
	}
}

func invalidFinalEvent(text, finishReason string) *trpcevent.Event {
	fr := finishReason
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeChatCompletion,
			Choices: []trpcmodel.Choice{
				{
					Message:      trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: text},
					FinishReason: &fr,
				},
			},
		},
	}
}

func invalidToolCallDeltaEvent(id, name string) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{
				{Delta: trpcmodel.Message{
					Role: trpcmodel.RoleAssistant,
					ToolCalls: []trpcmodel.ToolCall{
						{ID: id, Function: trpcmodel.FunctionDefinitionParam{Name: name}},
					},
				}},
			},
		},
	}
}

func invalidErrorEvent(errType, msg string) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeError,
			Error:  &trpcmodel.ResponseError{Type: errType, Message: msg},
		},
	}
}

func consumeInvalidMeta() ProjectMeta {
	return ProjectMeta{SessionID: "sess-invalid", RequestID: "task-invalid", InvocationID: "turn-invalid"}
}

func TestStreamConsumer_InvalidKind_NoFinishReason(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	events <- invalidDeltaEvent("partial answer chunk")
	close(events) // 无 finish_reason 即收官

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if result.InvalidKind != StreamInvalidNoFinishReason {
		t.Fatalf("InvalidKind=%v, want no_finish_reason", result.InvalidKind)
	}
	if !result.PartialContentHadFirstByte {
		t.Fatal("PartialContentHadFirstByte=false, want true (流起过活)")
	}
	if result.ToolCallDispatched {
		t.Fatal("ToolCallDispatched=true, want false")
	}
	if !result.HasContent {
		t.Fatal("HasContent=false, want true")
	}
}

func TestStreamConsumer_InvalidKind_StopIsNone(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	events <- invalidDeltaEvent("hello ")
	events <- invalidFinalEvent("hello world", "stop")
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if result.InvalidKind != StreamInvalidNone {
		t.Fatalf("InvalidKind=%v, want none for normal stop", result.InvalidKind)
	}
}

func TestStreamConsumer_InvalidKind_LengthTruncation(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	events <- invalidDeltaEvent("truncated output ")
	events <- invalidFinalEvent("truncated output tail", "length")
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if result.InvalidKind != StreamInvalidMaxTokens {
		t.Fatalf("InvalidKind=%v, want max_tokens", result.InvalidKind)
	}
	if !result.HasContent {
		t.Fatal("HasContent=false, want true (续写分支)")
	}
}

func TestStreamConsumer_InvalidKind_ContentFilterSafety(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	events <- invalidFinalEvent("", "content_filter")
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if result.InvalidKind != StreamInvalidSafety {
		t.Fatalf("InvalidKind=%v, want safety", result.InvalidKind)
	}
}

func TestStreamConsumer_InvalidKind_MalformedToolCallError(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	events <- invalidErrorEvent("invalid_request_error", `invalid model response: tool call function name is empty: response_id="r1"`)
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if result.InvalidKind != StreamInvalidMalformedToolCall {
		t.Fatalf("InvalidKind=%v, want malformed_tool_call", result.InvalidKind)
	}
	if result.ToolCallDispatched {
		t.Fatal("ToolCallDispatched=true, want false (未观察到带 ID 的 tool_call)")
	}
}

func TestStreamConsumer_InvalidKind_HTTPEOF(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	events <- invalidDeltaEvent("half answer ")
	events <- invalidErrorEvent("stream_error", "stream error: unexpected EOF")
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if result.InvalidKind != StreamInvalidHTTPEOF {
		t.Fatalf("InvalidKind=%v, want http_eof", result.InvalidKind)
	}
	if !result.PartialContentHadFirstByte {
		t.Fatal("PartialContentHadFirstByte=false, want true")
	}
}

func TestStreamConsumer_InvalidKind_StallExhaustedIsNone(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	events <- invalidErrorEvent("stream_error", "llm stream liveness exhausted: phase=stall reconnects=5/5 stall_timeout=1m30s")
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	// stall 预算已在模型层耗尽：不归 invalid-retry（否则绕过预算共账）。
	if result.InvalidKind != StreamInvalidNone {
		t.Fatalf("InvalidKind=%v, want none (stall 预算已耗尽)", result.InvalidKind)
	}
}

func TestStreamConsumer_ToolCallDispatchedFlag(t *testing.T) {
	events := make(chan *trpcevent.Event, 8)
	events <- invalidToolCallDeltaEvent("call_1", "read_file")
	events <- invalidFinalEvent("done", "stop")
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if !result.ToolCallDispatched {
		t.Fatal("ToolCallDispatched=false, want true (观察到带 ID 的 tool_call)")
	}
	if result.InvalidKind != StreamInvalidNone {
		t.Fatalf("InvalidKind=%v, want none (stop 收官)", result.InvalidKind)
	}
}

func TestStreamConsumer_InvalidKind_DoomLoopStaysNone(t *testing.T) {
	events := make(chan *trpcevent.Event, 16)
	repetitive := "I need to check the file and verify the output."
	for i := 0; i < 6; i++ {
		events <- invalidDeltaEvent(repetitive)
	}
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if !result.DoomLoopDetected {
		t.Fatal("DoomLoopDetected=false, want true")
	}
	// doom abort 恒终态：InvalidKind 必须 None（不可重试）。
	if result.InvalidKind != StreamInvalidNone {
		t.Fatalf("InvalidKind=%v, want none for doom abort", result.InvalidKind)
	}
}

func TestStreamConsumer_InvalidKind_EmptyStreamIsNoFinishReason(t *testing.T) {
	events := make(chan *trpcevent.Event, 1)
	close(events)

	result := ConsumeEventStream(context.Background(), events, consumeInvalidMeta(), nil, loggateway.NewNoop())
	if result.InvalidKind != StreamInvalidNoFinishReason {
		t.Fatalf("InvalidKind=%v, want no_finish_reason for empty stream", result.InvalidKind)
	}
	if result.PartialContentHadFirstByte {
		t.Fatal("PartialContentHadFirstByte=true, want false (零产出)")
	}
}
