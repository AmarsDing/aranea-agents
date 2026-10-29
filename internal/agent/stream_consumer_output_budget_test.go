package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func outputBudgetDeltaEvent(content, reasoning string) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{
				{Delta: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: content, ReasoningContent: reasoning}},
			},
		},
	}
}

// 输出预算护栏（2026-10-29）：reasoning 累计超限 → 消费端自中止并置终态
// 信号。背景：推理型模型 reasoning 空转烧穿 provider 上下文（44 实测流中段
// "Context size has been exceeded"），护栏必须远早于溢出触发。
// 注意：VisibleStreamingDelta 有快照去重语义（chunk 以累计文本为前缀时视为
// 快照替换），且 doom 检测默认开启——测试 chunk 必须互不相同。
func distinctChunk(i int, n int) string {
	return strings.Repeat(string(rune('a'+i)), n)
}

func TestStreamConsumer_OutputBudget_ReasoningExceeded(t *testing.T) {
	events := make(chan *trpcevent.Event, 16)
	for i := 0; i < 5; i++ {
		events <- outputBudgetDeltaEvent("", distinctChunk(i, 100))
	}
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID: "sess-budget", RequestID: "task-budget", InvocationID: "turn-budget",
	}, &StreamConsumeOptions{MaxReasoningChars: 150}, loggateway.NewNoop())

	if !result.OutputBudgetExceeded {
		t.Fatal("expected OutputBudgetExceeded=true when reasoning exceeds limit")
	}
	if !result.HasError {
		t.Fatal("expected HasError=true on output budget abort")
	}
	if !strings.Contains(result.LastError, "output budget exceeded") || !strings.Contains(result.LastError, "reasoning") {
		t.Fatalf("LastError must name the guard and side, got %q", result.LastError)
	}
	if !IsTerminalStreamFailure(result) {
		t.Fatal("budget abort must be a terminal stream failure (persist/team guard contract)")
	}
	// drain-only 语义：超限即提前退出，累计文本不得继续增长完整 5 段。
	if result.Reasoning.Len() >= 500 {
		t.Fatalf("consumer should stop accumulating after abort, got %d chars", result.Reasoning.Len())
	}
}

func TestStreamConsumer_OutputBudget_ReplyExceeded(t *testing.T) {
	events := make(chan *trpcevent.Event, 16)
	for i := 0; i < 5; i++ {
		events <- outputBudgetDeltaEvent(distinctChunk(i, 100), "")
	}
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID: "sess-budget2", RequestID: "task-budget2", InvocationID: "turn-budget2",
	}, &StreamConsumeOptions{MaxOutputChars: 150}, loggateway.NewNoop())

	if !result.OutputBudgetExceeded {
		t.Fatal("expected OutputBudgetExceeded=true when reply exceeds limit")
	}
	if !strings.Contains(result.LastError, "reply") {
		t.Fatalf("LastError must name reply side, got %q", result.LastError)
	}
}

func TestStreamConsumer_OutputBudget_UnderLimitNoTrigger(t *testing.T) {
	events := make(chan *trpcevent.Event, 16)
	for i := 0; i < 3; i++ {
		events <- outputBudgetDeltaEvent("short reply chunk.", "short reasoning chunk.")
	}
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID: "sess-ok3", RequestID: "task-ok3", InvocationID: "turn-ok3",
	}, &StreamConsumeOptions{MaxReasoningChars: 65536, MaxOutputChars: 65536}, loggateway.NewNoop())

	if result.OutputBudgetExceeded || result.HasError {
		t.Fatalf("budget guard must not fire under limit: %+v last=%q", result, result.LastError)
	}
	if !result.HasContent {
		t.Fatal("expected HasContent=true for normal deltas")
	}
}

// 默认（0）= 不启用：保持接线前逐字节一致。
func TestStreamConsumer_OutputBudget_DisabledByDefault(t *testing.T) {
	events := make(chan *trpcevent.Event, 16)
	for i := 0; i < 3; i++ {
		events <- outputBudgetDeltaEvent("", distinctChunk(i, 1000))
	}
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID: "sess-off", RequestID: "task-off", InvocationID: "turn-off",
	}, &StreamConsumeOptions{}, loggateway.NewNoop())

	if result.OutputBudgetExceeded || result.HasError {
		t.Fatalf("guard disabled (0) must not fire: %+v", result)
	}
}

// IsTerminalStreamFailure 契约 pin：chat persistTurn 与 team runner 共用。
// doom loop 除外（既有截断持久化语义）；无 LastError 的 HasError 不算终态。
func TestIsTerminalStreamFailure(t *testing.T) {
	cases := []struct {
		name   string
		result EventStreamResult
		want   bool
	}{
		{"clean", EventStreamResult{}, false},
		{"content_no_error", EventStreamResult{HasContent: true}, false},
		// 2026-10-29 44 实测形态：reasoning 有内容 + 流中段 response_error。
		{"mid_stream_provider_error", EventStreamResult{
			HasError: true, HasContent: true,
			LastError: `received error while streaming: {"code":500,"message":"Context size has been exceeded.","type":"server_error"}`,
		}, true},
		{"output_budget", EventStreamResult{HasError: true, OutputBudgetExceeded: true, LastError: "output budget exceeded: reasoning length 17000 chars > limit 16384 chars"}, true},
		{"doom_excluded", EventStreamResult{HasError: true, DoomLoopDetected: true, LastError: "doom loop detected: repetitive LLM output, turn aborted"}, false},
		{"error_without_detail", EventStreamResult{HasError: true}, false},
		{"error_blank_detail", EventStreamResult{HasError: true, LastError: "   "}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTerminalStreamFailure(tc.result); got != tc.want {
				t.Fatalf("IsTerminalStreamFailure() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStreamOutputGuardLimitsFromEnv(t *testing.T) {
	const prefix = "ARANEA_TEST_OUTPUT_GUARD"
	t.Setenv(prefix+"_MAX_REASONING_CHARS", "1234")
	// output 未设置 → 默认值。
	reasoning, output := StreamOutputGuardLimitsFromEnv(prefix, 100, 200)
	if reasoning != 1234 || output != 200 {
		t.Fatalf("env override failed: got (%d, %d)", reasoning, output)
	}
	// 非法值回落默认；负数/0 显式禁用。
	t.Setenv(prefix+"_MAX_REASONING_CHARS", "not-a-number")
	t.Setenv(prefix+"_MAX_OUTPUT_CHARS", "-5")
	reasoning, output = StreamOutputGuardLimitsFromEnv(prefix, 100, 200)
	if reasoning != 100 || output != 0 {
		t.Fatalf("fallback/disable semantics broken: got (%d, %d)", reasoning, output)
	}
}
