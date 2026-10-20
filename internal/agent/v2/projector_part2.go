package v2

import (
	"aranea-agents/internal/biz"
	"context"
	"encoding/json"
	"strings"
	"time"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// processToolResponse handles tool result events.
func (p *ActivityProjector) processToolResponse(ctx context.Context, ev *trpcevent.Event) {
	if len(ev.Response.Choices) == 0 {
		return
	}
	msg := ev.Response.Choices[0].Message
	toolID := strings.TrimSpace(msg.ToolID)
	if toolID == "" {
		return
	}

	p.mu.Lock()
	stepID, ok := p.toolCallSteps[toolID]
	delete(p.toolCallSteps, toolID)
	p.mu.Unlock()
	if !ok {
		// Tool result without a prior OnToolCall — nothing to complete.
		return
	}

	resultJSON := json.RawMessage(msg.Content)
	if ev.Response.Error != nil {
		p.OnToolResult(ctx, stepID, resultJSON, &toolResponseError{msg: ev.Response.Error.Message, code: ev.Response.Error.Type})
		return
	}
	p.OnToolResult(ctx, stepID, resultJSON, nil)
}

// toolResponseError wraps a trpc ResponseError as a Go error so it can be
// passed to OnToolResult's err parameter. failStep records err.Error() as
// ToolErrorCode.
type toolResponseError struct {
	msg  string
	code string
}

func (e *toolResponseError) Error() string {
	if e.code != "" {
		return e.code + ": " + e.msg
	}
	return e.msg
}

// handleToolCall records the tool_call_id → stepID mapping and delegates to OnToolCall.
func (p *ActivityProjector) handleToolCall(ctx context.Context, toolCallID, toolName string, args json.RawMessage) {
	stepID := p.OnToolCall(ctx, p.eventMeta(), toolName, args)
	if toolCallID != "" && stepID != "" {
		p.mu.Lock()
		p.toolCallSteps[toolCallID] = stepID
		p.mu.Unlock()
	}
}

// handleReasoningDelta lazily creates a thinking step on the first delta,
// then emits the streaming delta.
//
// 2026-07-04 问题 1 根因 2 修复：使用 per-agentKey map 隔离不同 member
// agent 的 thinking step，避免 graph 模式下后续 member 复用首个 member
// 的 thinking step。
func (p *ActivityProjector) handleReasoningDelta(ctx context.Context, delta string) {
	agentKey := p.currentAgentKey()
	stepID, ok := p.thinkingStepIDs[agentKey]
	if !ok || stepID == "" {
		stepID = p.BeginStep(ctx, p.eventMeta(), biz.StepKindThinking)
		p.thinkingStepIDs[agentKey] = stepID
	}
	p.OnReasoningDelta(ctx, stepID, delta, "")
}

// handleTextDelta lazily creates a reply step on the first non-blank delta,
// then emits the streaming delta.
//
// Pure-whitespace leading deltas (e.g. "\n", " ") do NOT create a step —
// LLM frameworks often emit these as separators. Creating a step for them
// would leave an empty ReplyStep when no real content follows, producing
// empty ReplyBlocks in the frontend. Once a step exists, subsequent
// whitespace deltas are streamed normally (whitespace is meaningful content
// mid-stream).
//
// Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.1.1
//
// 2026-07-04 问题 1 根因 2 修复：使用 per-agentKey map 隔离不同 member
// agent 的 reply step。
func (p *ActivityProjector) handleTextDelta(ctx context.Context, delta string) {
	agentKey := p.currentAgentKey()
	stepID, ok := p.replyStepIDs[agentKey]
	if !ok || stepID == "" {
		// 防止 LLM 输出引导空白（"\n", " "）导致创建空 ReplyStep。
		// 仅在 delta 含非空白字符时才创建 step；纯空白 delta 丢弃。
		if strings.TrimSpace(delta) == "" {
			return
		}
		stepID = p.BeginStep(ctx, p.eventMeta(), biz.StepKindReply)
		p.replyStepIDs[agentKey] = stepID
	}
	p.OnTextDelta(ctx, stepID, delta, "")
}

// handleReasoningDone finalizes the thinking step. If no thinking step was
// created (no prior delta) and the content is empty, this is a no-op.
func (p *ActivityProjector) handleReasoningDone(ctx context.Context, finalContent string) {
	agentKey := p.currentAgentKey()
	stepID, ok := p.thinkingStepIDs[agentKey]
	if !ok || stepID == "" {
		// No thinking step was started. If there's reasoning content, create
		// and immediately complete a step for it; otherwise skip.
		if strings.TrimSpace(finalContent) == "" {
			return
		}
		stepID = p.BeginStep(ctx, p.eventMeta(), biz.StepKindThinking)
		p.thinkingStepIDs[agentKey] = stepID
	}
	p.OnReasoningDone(ctx, stepID, finalContent)
	delete(p.thinkingStepIDs, agentKey)
}

// handleTextDone finalizes the reply step. If no reply step was created and
// the content is empty, this is a no-op. If a reply step was created (e.g.
// by an earlier delta) but both accumulated Content and finalContent are
// blank, the step is finalized with Status=cancelled (not completed) so the
// frontend can filter out empty ReplyBlocks.
//
// Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.1.2
func (p *ActivityProjector) handleTextDone(ctx context.Context, finalContent string) {
	agentKey := p.currentAgentKey()
	stepID, ok := p.replyStepIDs[agentKey]
	if !ok || stepID == "" {
		if strings.TrimSpace(finalContent) == "" {
			return
		}
		stepID = p.BeginStep(ctx, p.eventMeta(), biz.StepKindReply)
		p.replyStepIDs[agentKey] = stepID
	}
	// 检查 step 是否为空（已创建但 finalContent 为空白）。
	// P3-resume 注：流式 checkpoint 让 step.Content 在流式期间累积 delta，
	// 不再恒空；空回复判定须只看 finalContent（2026-07-04 既定语义：
	// 最终无可见内容 → cancelled，前端过滤空 ReplyBlock）。
	p.mu.Lock()
	step, stepOk := p.activeStep[stepID]
	isBlank := stepOk && strings.TrimSpace(finalContent) == ""
	if isBlank {
		// 空 reply 取消而非完成，前端按 status=cancelled 过滤
		now := time.Now()
		step.Status = biz.StepStatusCancelled
		step.IsFinal = false
		step.CompletedAt = &now
		step.Version++
		delete(p.activeStep, stepID)
	}
	p.mu.Unlock()
	if isBlank {
		p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
		delete(p.replyStepIDs, agentKey)
		return
	}
	// isFinal=true for the root turn's reply so the frontend can mark it.
	p.OnTextDone(ctx, stepID, finalContent, true)
	delete(p.replyStepIDs, agentKey)
}

// choiceStreamContent extracts text and reasoning from a streaming choice.
// Mirrors internal/agent.ChoiceStreamContent but is duplicated here to avoid
// importing internal/agent (which would re-introduce the circular dependency).
func choiceStreamContent(choice trpcmodel.Choice, partial bool) (text, reasoning string) {
	msg := choice.Message
	delta := choice.Delta
	if partial {
		text = delta.Content
		reasoning = delta.ReasoningContent
		if text == "" {
			text = strings.TrimSpace(msg.Content)
		}
		if reasoning == "" {
			reasoning = strings.TrimSpace(msg.ReasoningContent)
		}
		return text, reasoning
	}
	text = firstNonEmpty(msg.Content, delta.Content)
	reasoning = firstNonEmpty(msg.ReasoningContent, delta.ReasoningContent)
	return text, reasoning
}

// firstNonEmpty returns the first non-empty string after trimming.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
