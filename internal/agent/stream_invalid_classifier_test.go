package agent

import "testing"

// TestClassifyInvalidStream 表驱动 pin：全枚举 × 边界。任何一行改动都会
// 打破本测试——这是设计 §5.5/§12.2 的行为契约。
func TestClassifyInvalidStream(t *testing.T) {
	cases := []struct {
		name string
		obs  StreamInvalidObs
		want StreamInvalidKind
	}{
		// ── None 族 ──
		{"empty_stream_no_finish_no_error", StreamInvalidObs{}, StreamInvalidNoFinishReason},
		{"valid_stop", StreamInvalidObs{SawFinishReason: true, FinishReason: "stop", HasContent: true}, StreamInvalidNone},
		{"valid_tool_calls", StreamInvalidObs{SawFinishReason: true, FinishReason: "tool_calls"}, StreamInvalidNone},
		{"valid_function_call", StreamInvalidObs{SawFinishReason: true, FinishReason: "function_call"}, StreamInvalidNone},
		{"valid_end_turn", StreamInvalidObs{SawFinishReason: true, FinishReason: "end_turn"}, StreamInvalidNone},
		{"canceled_turn", StreamInvalidObs{Canceled: true, HasError: true, LastError: "stream error: unexpected EOF"}, StreamInvalidNone},
		{"doom_loop_abort", StreamInvalidObs{DoomLoopDetected: true, HasError: true, LastError: "doom loop detected: repetitive LLM output, turn aborted"}, StreamInvalidNone},
		{"first_byte_timeout", StreamInvalidObs{FirstByteTimedOut: true, HasError: true, LastError: "first byte timeout"}, StreamInvalidNone},
		// doom 与 canceled 共存（doom 路径会同时置 canceled）——恒 None。
		{"doom_with_canceled", StreamInvalidObs{DoomLoopDetected: true, Canceled: true, HasError: true, LastError: "stream error: unexpected EOF"}, StreamInvalidNone},

		// ── finish_reason 权威终态 ──
		{"finish_content_filter", StreamInvalidObs{SawFinishReason: true, FinishReason: "content_filter", HasContent: true}, StreamInvalidSafety},
		{"finish_content_filter_with_eof_error", StreamInvalidObs{SawFinishReason: true, FinishReason: "content_filter", HasError: true, LastError: "stream error: unexpected EOF"}, StreamInvalidSafety},
		{"finish_length_with_content", StreamInvalidObs{SawFinishReason: true, FinishReason: "length", HasContent: true}, StreamInvalidMaxTokens},
		{"finish_length_no_content", StreamInvalidObs{SawFinishReason: true, FinishReason: "length"}, StreamInvalidMaxTokens},
		{"finish_max_tokens_alias", StreamInvalidObs{SawFinishReason: true, FinishReason: "max_tokens"}, StreamInvalidMaxTokens},
		{"finish_unknown_value", StreamInvalidObs{SawFinishReason: true, FinishReason: "some_vendor_reason", HasContent: true}, StreamInvalidNone},

		// ── 错误标记分析 ──
		{"err_safety_marker", StreamInvalidObs{HasError: true, LastError: `{"error":{"type":"content_filter","message":"blocked by safety system"}}`}, StreamInvalidSafety},
		{"err_safety_moderation", StreamInvalidObs{HasError: true, LastError: "request failed moderation checks"}, StreamInvalidSafety},
		{"err_malformed_tool_call", StreamInvalidObs{HasError: true, LastError: `invalid model response: tool call function name is empty: response_id="r1" is_done=true`}, StreamInvalidMalformedToolCall},
		{"err_eof_unexpected", StreamInvalidObs{HasError: true, LastError: "stream error: unexpected EOF"}, StreamInvalidHTTPEOF},
		{"err_eof_generic", StreamInvalidObs{HasError: true, LastError: "read tcp 10.0.0.1:443: EOF"}, StreamInvalidHTTPEOF},
		{"err_max_tokens_text", StreamInvalidObs{HasError: true, LastError: "max_tokens exceeded for this request"}, StreamInvalidMaxTokens},
		{"err_generic_stream_error", StreamInvalidObs{HasError: true, LastError: "stream_error: provider exploded", LastErrorType: "stream_error"}, StreamInvalidStreamError},
		{"err_type_stream_error_generic", StreamInvalidObs{HasError: true, LastError: "connection reset by peer", LastErrorType: "stream_error"}, StreamInvalidStreamError},

		// ── 明确不属于本分类器的边界（防双重重试 pin）──
		{"err_context_overflow", StreamInvalidObs{HasError: true, LastError: "maximum context length exceeded"}, StreamInvalidNone},
		{"err_context_overflow_code", StreamInvalidObs{HasError: true, LastError: "context_length_exceeded: reduce your prompt"}, StreamInvalidNone},
		{"err_stall_liveness_exhausted", StreamInvalidObs{HasError: true, LastError: "llm stream liveness exhausted: phase=stall reconnects=5/5 stall_timeout=1m30s", LastErrorType: "stream_error"}, StreamInvalidNone},

		// ── 混合：finish_reason 优先于错误猜测 ──
		{"finish_length_wins_over_generic_error", StreamInvalidObs{SawFinishReason: true, FinishReason: "length", HasError: true, LastError: "some transient blurb", HasContent: true}, StreamInvalidMaxTokens},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyInvalidStream(tc.obs); got != tc.want {
				t.Fatalf("ClassifyInvalidStream(%+v) = %v, want %v", tc.obs, got, tc.want)
			}
		})
	}
}

// TestInvalidActionFor 表驱动 pin：kind × 内容 × 派发闸门 × doom override。
func TestInvalidActionFor(t *testing.T) {
	retry2 := StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2}
	continue1 := StreamInvalidAction{Action: InvalidActionContinue, MaxAttempts: 1}
	continue0 := StreamInvalidAction{Action: InvalidActionContinue, MaxAttempts: 0}
	terminate := StreamInvalidAction{Action: InvalidActionTerminate, MaxAttempts: 0}

	cases := []struct {
		name string
		kind StreamInvalidKind
		obs  StreamInvalidObs
		want StreamInvalidAction
	}{
		// None：恒接受（无额外尝试）。
		{"none_content", StreamInvalidNone, StreamInvalidObs{HasContent: true}, continue0},
		{"none_empty", StreamInvalidNone, StreamInvalidObs{}, continue0},

		// NoFinishReason：有内容→续写 1 次；空→重发 2 次。
		{"no_finish_content", StreamInvalidNoFinishReason, StreamInvalidObs{HasContent: true}, continue1},
		{"no_finish_empty", StreamInvalidNoFinishReason, StreamInvalidObs{}, retry2},

		// MalformedToolCall：未派发→重发 2 次；已派发→TERMINATE（防重复副作用）。
		{"malformed_not_dispatched_content", StreamInvalidMalformedToolCall, StreamInvalidObs{HasContent: true}, retry2},
		{"malformed_not_dispatched_empty", StreamInvalidMalformedToolCall, StreamInvalidObs{}, retry2},
		{"malformed_dispatched", StreamInvalidMalformedToolCall, StreamInvalidObs{ToolCallDispatched: true}, terminate},

		// MaxTokens：有内容→续写 1 次；空→TERMINATE。
		{"max_tokens_content", StreamInvalidMaxTokens, StreamInvalidObs{HasContent: true}, continue1},
		{"max_tokens_empty", StreamInvalidMaxTokens, StreamInvalidObs{}, terminate},

		// Safety：恒 TERMINATE。
		{"safety_content", StreamInvalidSafety, StreamInvalidObs{HasContent: true}, terminate},
		{"safety_empty", StreamInvalidSafety, StreamInvalidObs{}, terminate},
		{"safety_not_dispatched", StreamInvalidSafety, StreamInvalidObs{ToolCallDispatched: false}, terminate},

		// HTTPEOF：未派发→重发 2 次；已派发→TERMINATE。
		{"eof_not_dispatched", StreamInvalidHTTPEOF, StreamInvalidObs{}, retry2},
		{"eof_not_dispatched_content", StreamInvalidHTTPEOF, StreamInvalidObs{HasContent: true}, retry2},
		{"eof_dispatched", StreamInvalidHTTPEOF, StreamInvalidObs{ToolCallDispatched: true}, terminate},

		// StreamError：未派发→重发 2 次；已派发→TERMINATE。
		{"stream_error_not_dispatched", StreamInvalidStreamError, StreamInvalidObs{}, retry2},
		{"stream_error_dispatched", StreamInvalidStreamError, StreamInvalidObs{ToolCallDispatched: true}, terminate},

		// doom flag override：对全枚举恒 TERMINATE（钉死语义，防御纵深）。
		{"doom_overrides_none", StreamInvalidNone, StreamInvalidObs{DoomLoopDetected: true}, terminate},
		{"doom_overrides_no_finish", StreamInvalidNoFinishReason, StreamInvalidObs{DoomLoopDetected: true}, terminate},
		{"doom_overrides_malformed", StreamInvalidMalformedToolCall, StreamInvalidObs{DoomLoopDetected: true}, terminate},
		{"doom_overrides_max_tokens", StreamInvalidMaxTokens, StreamInvalidObs{DoomLoopDetected: true, HasContent: true}, terminate},
		{"doom_overrides_safety", StreamInvalidSafety, StreamInvalidObs{DoomLoopDetected: true}, terminate},
		{"doom_overrides_eof", StreamInvalidHTTPEOF, StreamInvalidObs{DoomLoopDetected: true}, terminate},
		{"doom_overrides_stream_error", StreamInvalidStreamError, StreamInvalidObs{DoomLoopDetected: true}, terminate},

		// 未知 kind（未来枚举扩展未入表）：安全终态。
		{"unknown_kind", StreamInvalidKind(99), StreamInvalidObs{}, terminate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InvalidActionFor(tc.kind, tc.obs); got != tc.want {
				t.Fatalf("InvalidActionFor(%v, %+v) = %+v, want %+v", tc.kind, tc.obs, got, tc.want)
			}
		})
	}
}

// TestInvalidActionFor_DispatchedGateCoversContinue pin：已派发 tool_call 时
// 连「续写」也不允许（续写同样要重发整轮 Run，会重复执行工具）。
func TestInvalidActionFor_DispatchedGateCoversContinue(t *testing.T) {
	got := InvalidActionFor(StreamInvalidMaxTokens, StreamInvalidObs{HasContent: true, ToolCallDispatched: true})
	if got.Action != InvalidActionTerminate || got.MaxAttempts != 0 {
		t.Fatalf("max_tokens+dispatched = %+v, want terminate/0", got)
	}
	got = InvalidActionFor(StreamInvalidNoFinishReason, StreamInvalidObs{HasContent: true, ToolCallDispatched: true})
	if got.Action != InvalidActionTerminate || got.MaxAttempts != 0 {
		t.Fatalf("no_finish+content+dispatched = %+v, want terminate/0", got)
	}
}
