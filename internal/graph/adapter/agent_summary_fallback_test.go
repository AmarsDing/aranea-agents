package adapter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	deliverabletools "aranea-agents/internal/tools/deliverable"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- test doubles ---

type stubFallbackInnerAgent struct {
	tools  []trpctool.Tool
	events []*trpcevent.Event
}

func (s *stubFallbackInnerAgent) Run(_ context.Context, _ *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	ch := make(chan *trpcevent.Event, len(s.events))
	for _, ev := range s.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (s *stubFallbackInnerAgent) Tools() []trpctool.Tool             { return s.tools }
func (s *stubFallbackInnerAgent) Info() trpcagent.Info               { return trpcagent.Info{Name: "stub"} }
func (s *stubFallbackInnerAgent) SubAgents() []trpcagent.Agent       { return nil }
func (s *stubFallbackInnerAgent) FindSubAgent(string) trpcagent.Agent { return nil }

type stubFallbackModel struct{ text string }

func (m stubFallbackModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	ch := make(chan *trpcmodel.Response, 1)
	ch <- &trpcmodel.Response{
		Done: true,
		Choices: []trpcmodel.Choice{{
			Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: m.text},
		}},
	}
	close(ch)
	return ch, nil
}

func (m stubFallbackModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "stub-model"} }

func toolLimitTerminalEvent() *trpcevent.Event {
	return trpcevent.NewErrorEvent("inv-1", "stub",
		trpcmodel.ErrorTypeFlowError, "max tool iterations (5) exceeded")
}

func fallbackTestInvocation() *trpcagent.Invocation {
	return &trpcagent.Invocation{
		InvocationID: "inv-1",
		AgentName:    "ops_fault_diagnosis",
		Message:      trpcmodel.Message{Role: trpcmodel.RoleUser, Content: "诊断当前活跃告警"},
	}
}

func runFallbackAndCollect(t *testing.T, fallback trpcagent.Agent) []*trpcevent.Event {
	t.Helper()
	ch, err := fallback.Run(context.Background(), fallbackTestInvocation())
	if err != nil {
		t.Fatal(err)
	}
	var out []*trpcevent.Event
	for ev := range ch {
		out = append(out, ev)
	}
	if len(out) == 0 {
		t.Fatal("no events emitted")
	}
	return out
}

func decodeDeliverableDelta(t *testing.T, ev *trpcevent.Event) map[string]any {
	t.Helper()
	raw := ev.StateDelta[biz.DeliverableStateKey]
	if len(raw) == 0 {
		t.Fatal("final event carries no deliverable StateDelta")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("deliverable delta not a JSON object: %v", err)
	}
	return m
}

// --- tests ---

// 工具上限硬停 + 单 topic 契约：兜底总结必须同时合成 deliverable StateDelta，
// 且归档到契约 topic 下（闸门 HasRealDeliverable 判定来源）。
func TestSummaryFallback_ToolLimitSynthesizesContractedDeliverable(t *testing.T) {
	t.Parallel()
	contract := &biz.MemberDeliverableContract{Entries: []biz.MemberDeliverableEntry{
		{Topic: "诊断报告", Required: true},
	}}
	inner := &stubFallbackInnerAgent{
		tools:  deliverabletools.ToolsWithContract(contract),
		events: []*trpcevent.Event{toolLimitTerminalEvent()},
	}
	fallback := newSummaryFallbackAgent(inner, stubFallbackModel{text: "最终诊断报告内容"}, loggateway.NewNoop())

	events := runFallbackAndCollect(t, fallback)
	last := events[len(events)-1]
	if last.Response == nil || len(last.Response.Choices) == 0 ||
		last.Response.Choices[0].Message.Content != "最终诊断报告内容" {
		t.Fatalf("last event must be the fallback summary, got %+v", last.Response)
	}
	m := decodeDeliverableDelta(t, last)
	if m["summary"] != "最终诊断报告内容" {
		t.Fatalf("reserved summary key wrong: %v", m["summary"])
	}
	topicData, ok := m["诊断报告"].(map[string]any)
	if !ok {
		t.Fatalf("contract topic missing in synthesized deliverable: %v", m)
	}
	if topicData["summary"] != "最终诊断报告内容" {
		t.Fatalf("topic summary wrong: %v", topicData)
	}
	// 文档范式兜底：topic 必须携带 content（全文）与 format，下游才能按
	// artifacts 载荷清单内联或指针化消费——兜底产出与正常产出同构。
	if topicData["content"] != "最终诊断报告内容" {
		t.Fatalf("topic content should carry the full fallback text: %v", topicData)
	}
	if topicData["format"] != "markdown" {
		t.Fatalf("topic format should be markdown: %v", topicData)
	}
}

// 多 topic 契约：不猜测归属，只写保留 summary key。
func TestSummaryFallback_MultiTopicContractWritesSummaryOnly(t *testing.T) {
	t.Parallel()
	contract := &biz.MemberDeliverableContract{Entries: []biz.MemberDeliverableEntry{
		{Topic: "诊断报告"}, {Topic: "恢复执行报告"},
	}}
	inner := &stubFallbackInnerAgent{
		tools:  deliverabletools.ToolsWithContract(contract),
		events: []*trpcevent.Event{toolLimitTerminalEvent()},
	}
	fallback := newSummaryFallbackAgent(inner, stubFallbackModel{text: "报告"}, loggateway.NewNoop())

	events := runFallbackAndCollect(t, fallback)
	m := decodeDeliverableDelta(t, events[len(events)-1])
	if m["summary"] != "报告" {
		t.Fatalf("reserved summary key wrong: %v", m)
	}
	if _, exists := m["诊断报告"]; exists {
		t.Fatal("multi-topic contract must not guess a topic")
	}
	if len(m) != 1 {
		t.Fatalf("expected summary-only deliverable, got %v", m)
	}
}

// 无 set_deliverable 工具（deliverable 通道未开启）：不合成 StateDelta，
// 总结事件行为同前。
func TestSummaryFallback_NoDeliverableToolSkipsDelta(t *testing.T) {
	t.Parallel()
	inner := &stubFallbackInnerAgent{
		tools:  nil,
		events: []*trpcevent.Event{toolLimitTerminalEvent()},
	}
	fallback := newSummaryFallbackAgent(inner, stubFallbackModel{text: "报告"}, loggateway.NewNoop())

	events := runFallbackAndCollect(t, fallback)
	last := events[len(events)-1]
	if last.Response == nil || last.Response.Choices[0].Message.Content != "报告" {
		t.Fatal("fallback summary must still be emitted")
	}
	if len(last.StateDelta) != 0 {
		t.Fatalf("no StateDelta expected without set_deliverable tool, got %v", last.StateDelta)
	}
}

// 正常结束（无工具上限事件）：透传 inner 事件流，不追加任何兜底事件。
func TestSummaryFallback_NormalCompletionPassThrough(t *testing.T) {
	t.Parallel()
	normal := trpcevent.NewResponseEvent("inv-1", "stub", &trpcmodel.Response{
		Object: trpcmodel.ObjectTypeChatCompletion,
		Done:   true,
		Choices: []trpcmodel.Choice{{
			Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "正常回答"},
		}},
	})
	inner := &stubFallbackInnerAgent{
		tools:  deliverabletools.Tools(),
		events: []*trpcevent.Event{normal},
	}
	fallback := newSummaryFallbackAgent(inner, stubFallbackModel{text: "不应出现"}, loggateway.NewNoop())

	events := runFallbackAndCollect(t, fallback)
	if len(events) != 1 {
		t.Fatalf("normal completion must pass through unchanged, got %d events", len(events))
	}
	if events[0].Response.Choices[0].Message.Content != "正常回答" {
		t.Fatalf("unexpected content: %v", events[0].Response.Choices[0].Message.Content)
	}
}

// 空总结（模型调用失败/空响应）：不发兜底事件，更不写 deliverable。
func TestSummaryFallback_EmptySummaryEmitsNothing(t *testing.T) {
	t.Parallel()
	inner := &stubFallbackInnerAgent{
		tools:  deliverabletools.Tools(),
		events: []*trpcevent.Event{toolLimitTerminalEvent()},
	}
	fallback := newSummaryFallbackAgent(inner, stubFallbackModel{text: "   "}, loggateway.NewNoop())

	events := runFallbackAndCollect(t, fallback)
	if len(events) != 1 {
		t.Fatalf("empty summary must not append events, got %d", len(events))
	}
}

// 跨包行为锚定（2026-08-15 评审修复 6）：adapter 的保留 summary key 字面量
// 必须等于 tools/deliverable 包 set_deliverable 实际拒绝的保留 topic——该
// 包常量为私有，无法直接等值断言，故经公开行为（保留 topic 写入报错）钉
// 住。漂移会让兜底交付物写到错误 key，信封 summary 静默为空。
func TestDeliverableReservedSummaryKey_AnchoredToToolsPackage(t *testing.T) {
	t.Parallel()
	tool := deliverabletools.NewSetDeliverableTool()
	_, err := tool.Call(context.Background(),
		[]byte(`{"topic":"`+deliverableReservedSummaryKey+`","data":{"x":1}}`))
	if err == nil || !strings.Contains(err.Error(), "reserved key") {
		t.Fatalf("set_deliverable must reject topic=%q as a reserved key: %v",
			deliverableReservedSummaryKey, err)
	}
}
