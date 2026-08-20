package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// 业务层优雅收尾（2026-08-14 用户批准的处置方案，不动 vendored 框架）：
// 框架 functioncall 处理器在达到 MaxToolIterations 时直接 EndInvocation 并
// 发出 flow_error 事件，agent 没有机会再让 LLM 输出最终总结，图节点的
// lastResponse 只剩中间叙述（“让我检查知识库…”）。本包装器挂在图 agent
// 节点解析层：转发内部事件流并记录转录，一旦检测到工具上限终止事件，在
// 流结束后用同一模型追加一次【无工具】总结调用，把最终报告作为最后一条
// assistant 响应事件发出，使节点 output_summary 完整。
type summaryFallbackAgent struct {
	inner trpcagent.Agent
	model trpcmodel.Model
	lg    loggateway.Logger
}

// newSummaryFallbackAgent wraps inner with the tool-limit summary fallback.
// Returns inner unchanged when model is nil (no fallback possible).
func newSummaryFallbackAgent(inner trpcagent.Agent, m trpcmodel.Model, lg loggateway.Logger) trpcagent.Agent {
	if inner == nil || m == nil {
		return inner
	}
	return &summaryFallbackAgent{inner: inner, model: m, lg: lg}
}

func (a *summaryFallbackAgent) Tools() []trpctool.Tool { return a.inner.Tools() }

func (a *summaryFallbackAgent) Info() trpcagent.Info { return a.inner.Info() }

func (a *summaryFallbackAgent) SubAgents() []trpcagent.Agent { return a.inner.SubAgents() }

func (a *summaryFallbackAgent) FindSubAgent(name string) trpcagent.Agent {
	return a.inner.FindSubAgent(name)
}

func (a *summaryFallbackAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	innerCh, err := a.inner.Run(ctx, inv)
	if err != nil {
		return nil, err
	}
	out := make(chan *trpcevent.Event, 256)
	safego.Go(ctx, "graph.adapter.summary_fallback.pump", func() { a.pump(ctx, inv, innerCh, out) })
	return out, nil
}

// isToolLimitTerminalEvent reports whether ev is the framework's hard-stop
// event: "max tool iterations (%d) exceeded" flow_error from functioncall.go.
func isToolLimitTerminalEvent(ev *trpcevent.Event) bool {
	if ev == nil || ev.Response == nil || ev.Response.Error == nil {
		return false
	}
	return ev.Response.Object == trpcmodel.ObjectTypeError &&
		ev.Response.Error.Type == trpcmodel.ErrorTypeFlowError &&
		strings.Contains(ev.Response.Error.Message, "max tool iterations")
}

const (
	summaryFallbackTranscriptCap = 16000 // total transcript chars kept (tail wins)
	summaryFallbackToolResultCap = 1500  // per tool result
)

// accumulateTranscript appends human-readable lines for assistant messages
// (text + called tool names) and tool results observed in the event stream.
func accumulateTranscript(sb *strings.Builder, ev *trpcevent.Event) {
	if ev == nil || ev.Response == nil || ev.Response.IsPartial {
		return
	}
	rsp := ev.Response
	if rsp.Object == trpcmodel.ObjectTypeToolResponse {
		for _, ch := range rsp.Choices {
			text := ch.Delta.Content
			if text == "" {
				text = ch.Message.Content
			}
			if text == "" {
				continue
			}
			if len(text) > summaryFallbackToolResultCap {
				text = text[:summaryFallbackToolResultCap] + "…(截断)"
			}
			name := ch.Message.ToolName
			if name == "" {
				name = ch.Message.ToolID
			}
			fmt.Fprintf(sb, "\n【工具结果 %s】\n%s\n", name, text)
		}
		return
	}
	if len(rsp.Choices) == 0 {
		return
	}
	msg := rsp.Choices[0].Message
	if msg.Role != trpcmodel.RoleAssistant {
		return
	}
	if msg.Content != "" {
		fmt.Fprintf(sb, "\n【助手】\n%s\n", msg.Content)
	}
	for _, tc := range msg.ToolCalls {
		if tc.Function.Name != "" {
			fmt.Fprintf(sb, "\n【调用工具】%s\n", tc.Function.Name)
		}
	}
}

func (a *summaryFallbackAgent) pump(ctx context.Context, inv *trpcagent.Invocation, innerCh <-chan *trpcevent.Event, out chan<- *trpcevent.Event) {
	defer close(out)
	var transcript strings.Builder
	hitToolLimit := false
	for ev := range innerCh {
		if isToolLimitTerminalEvent(ev) {
			hitToolLimit = true
		} else {
			accumulateTranscript(&transcript, ev)
		}
		if err := trpcagent.EmitEvent(ctx, inv, out, ev); err != nil {
			return
		}
	}
	if !hitToolLimit {
		return
	}
	summary := a.runSummaryCall(ctx, inv, transcript.String())
	if strings.TrimSpace(summary) == "" {
		return
	}
	if a.lg != nil {
		a.lg.Info("工具上限硬停兜底：已生成最终总结",
			loggateway.StepID("graph.summary_fallback"),
			loggateway.Str("agent", inv.AgentName))
	}
	resp := &trpcmodel.Response{
		Object: trpcmodel.ObjectTypeChatCompletion,
		Done:   true,
		Choices: []trpcmodel.Choice{{
			Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: summary},
		}},
	}
	// 交付物兜底（2026-08-15）：硬停后 set_deliverable 已不可能执行，DAG 团队
	// 会被真实交付物闸门判失败。把总结内容合成为 deliverable StateDelta 挂在
	// 总结事件上——与真实 set_deliverable 走完全相同的传播链路（agent 节点
	// fallback delta → deliverableAwareOutputMapper → 图状态 MergeReducer）。
	var opts []trpcevent.Option
	if delta := a.buildFallbackDeliverableDelta(summary); delta != nil {
		opts = append(opts, trpcevent.WithStateDelta(delta))
		if a.lg != nil {
			a.lg.Info("工具上限硬停兜底：已合成 deliverable 状态增量",
				loggateway.StepID("graph.summary_fallback.deliverable"),
				loggateway.Str("agent", inv.AgentName))
		}
	}
	_ = trpcagent.EmitEvent(ctx, inv, out, trpcevent.NewResponseEvent(inv.InvocationID, inv.AgentName, resp, opts...))
}

// deliverableReservedSummaryKey 镜像 deliverable 包的保留 key "summary"
// （该包常量为私有；此处用字面量避免 tools 层反向依赖）。
const deliverableReservedSummaryKey = "summary"

// buildFallbackDeliverableDelta 合成硬停 agent 未能写出的 deliverable 状态
// 增量。形状：{"summary": <报告>}；当成员交付契约恰好声明一个 topic 时附加
// {<topic>: {"summary": <报告>, "note": …}}。多 topic 契约不猜测归属（避免
// 张冠李戴），只写 summary——契约 topic 缺失由完成时 advisory
// （RequiredTopicsMissing）与质量门兜底，二者均为 fail-open。inner agent 无
// set_deliverable 工具（deliverable 通道未开启）或总结为空时返回 nil。
func (a *summaryFallbackAgent) buildFallbackDeliverableDelta(summary string) map[string][]byte {
	topic, ok := fallbackDeliverableTopic(a.inner.Tools())
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return nil
	}
	m := map[string]any{deliverableReservedSummaryKey: trimmed}
	if topic != "" {
		// 文档载荷范式：content 承载全文（下游信封 artifacts 依此内联或
		// 指针化），summary 供信封摘要聚合回退——兜底产出与正常产出同构。
		m[topic] = map[string]any{
			"summary": trimmed,
			"format":  "markdown",
			"content": trimmed,
			"note":    "工具调用次数上限触发的兜底提交：内容基于已完成工具调用的最终总结，未经契约写入校验",
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return map[string][]byte{biz.DeliverableStateKey: b}
}

// fallbackDeliverableTopic 在工具列表中查找 set_deliverable 并解析兜底交付物
// 应归档的契约 topic。ok=false 表示工具不存在（deliverable 通道未开启，无需
// 合成）；ok=true 且 topic="" 表示工具在但无契约或契约多 topic（不写 topic，
// 只写保留 summary key）。
func fallbackDeliverableTopic(tools []trpctool.Tool) (topic string, ok bool) {
	for _, t := range tools {
		if t == nil || t.Declaration() == nil || t.Declaration().Name != "set_deliverable" {
			continue
		}
		ok = true
		provider, has := t.(interface{ DeliverableContractTopics() []string })
		if !has {
			return "", true
		}
		if topics := provider.DeliverableContractTopics(); len(topics) == 1 {
			return topics[0], true
		}
		return "", true
	}
	return "", false
}

// runSummaryCall issues ONE LLM call without tools so the agent run still ends
// with a complete final report instead of the last intermediate narration.
func (a *summaryFallbackAgent) runSummaryCall(ctx context.Context, inv *trpcagent.Invocation, transcript string) string {
	if len(transcript) > summaryFallbackTranscriptCap {
		transcript = "…(前段截断)…" + transcript[len(transcript)-summaryFallbackTranscriptCap:]
	}
	sysPrompt := "你是运维专家 Agent。你在执行任务时达到了工具调用次数上限，被系统终止。" +
		"请基于已经完成的工具调用过程，直接输出面向用户的最终完整报告。" +
		"不要再说要去调用工具；若信息不足，明确说明哪些结论可靠、哪些未能验证。"
	userMsg := "【原始任务】\n" + inv.Message.Content + "\n\n【已完成的工具调用过程】" + transcript +
		"\n\n请输出最终报告。"
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			{Role: trpcmodel.RoleSystem, Content: sysPrompt},
			{Role: trpcmodel.RoleUser, Content: userMsg},
		},
		// 非流式：一次性拿最终结果；无 Tools，杜绝再次工具循环。
		GenerationConfig: trpcmodel.GenerationConfig{Stream: false},
	}
	ch, err := a.model.GenerateContent(ctx, req)
	if err != nil {
		if a.lg != nil {
			a.lg.Warn("工具上限硬停兜底：总结调用失败",
				loggateway.StepID("graph.summary_fallback"),
				loggateway.Str("agent", inv.AgentName),
				loggateway.Err(err))
		}
		return ""
	}
	var sb strings.Builder
	for rsp := range ch {
		if rsp == nil {
			continue
		}
		if len(rsp.Choices) == 0 {
			continue
		}
		if rsp.IsPartial {
			sb.WriteString(rsp.Choices[0].Delta.Content)
			continue
		}
		if rsp.Choices[0].Message.Content != "" {
			// 非流式最终消息为全量，覆盖已累积的分片。
			sb.Reset()
			sb.WriteString(rsp.Choices[0].Message.Content)
		}
	}
	return sb.String()
}
