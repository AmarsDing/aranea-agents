package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/biz/policyrule"
	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const errToolConfirmationRequired = "TOOL_CONFIRMATION_REQUIRED"

// defaultToolConfirmationTimeout is the maximum duration to wait for a user
// to approve or deny a tool confirmation request. After this deadline the
// tool invocation is rejected with ErrorCodeConfirmationTimeout.
const defaultToolConfirmationTimeout = 5 * time.Minute

type toolCallStartKey struct{}

type toolConfirmationBeforeHook struct {
	gate *toolConfirmGate
	ag   biz.Agent
	deps TRPCBuilderDeps
	// confirmTimeout bounds the wait for a user decision. Zero means
	// defaultToolConfirmationTimeout; overridable in tests.
	confirmTimeout time.Duration
}

var _ callbacks.BeforeToolHook = (*toolConfirmationBeforeHook)(nil)

func newToolConfirmationBeforeHook(gate *toolConfirmGate, ag biz.Agent, deps TRPCBuilderDeps) *toolConfirmationBeforeHook {
	return &toolConfirmationBeforeHook{gate: gate, ag: ag, deps: deps}
}

func (h *toolConfirmationBeforeHook) Point() callbacks.CallbackPoint {
	return callbacks.PointBeforeTool
}
func (h *toolConfirmationBeforeHook) Priority() int { return 10 }

func (h *toolConfirmationBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil || h.gate == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	toolKey := strings.TrimSpace(args.ToolName)
	if toolKey == "" || toolConfirmationBypass() {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	// 79-runtime-governance R9（2026-08-27 二轮审查根修）：param 规则 ask 裁定
	// 优先于插件 allow 早退——pluginAllowWithoutChannel 不消费 ctx verdict，
	// 若不排除 ask（含规则读取失败的降级 ask），命中 ask 的调用会被静默放行。
	// ask 落入 decide() 走确认（session/persisted grant 仍可满足，语义与
	// catalog 确认路径一致）。
	paramAskPending := false
	if v := paramRuleVerdictFromCtx(ctx); v != nil && v.effect == policyrule.EffectAsk {
		paramAskPending = true
	}
	if !paramAskPending && h.gate.pluginAllowWithoutChannel(toolKey, args.Arguments) {
		// P1-10: pluginAllowWithoutChannel means the product gate defers to the
		// plugin's allow policy — mark handled so ConfirmationGuardPlugin does
		// not hard-block after this allow path.
		return &trpctool.BeforeToolResult{Context: plugintrpc.WithToolConfirmHandled(ctx)}, nil
	}

	sessionID := toolConfirmSessionID(ctx)
	decision := h.gate.decide(ctx, sessionID, h.ag.ID, toolKey, args.Arguments)
	if !decision.needsConfirm {
		// param_rule_allow 是参数规则放行（非 grant），不进 grant 日志。
		if decision.reason != confirmReasonDefaultAllow && decision.reason != confirmReasonParamRuleAllow {
			h.deps.Logger().Info("tool confirmation skipped by grant",
				loggateway.StepID("agent.tool_confirm"),
				loggateway.Str("tool", toolKey),
				loggateway.Str("agent_id", h.ag.ID),
				loggateway.Str("decision_reason", decision.reason))
		}
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}

	// effectiveConfirmTimeout resolves the confirmation wait budget (test-overridable).
	effectiveConfirmTimeout := h.confirmTimeout
	if effectiveConfirmTimeout <= 0 {
		effectiveConfirmTimeout = defaultToolConfirmationTimeout
	}

	if fn := serviceawaitreply.ReplyFuncFromContext(ctx); fn != nil {
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
			if markErr := trpcagent.MarkAwaitingUserReply(inv); markErr != nil {
				// Non-fatal: the confirmation flow can still proceed, but
				// the UI may not show the "awaiting reply" indicator.
				h.deps.Logger().Warn("MarkAwaitingUserReply failed",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Err(markErr))
			}
		}
		// N-21: Emit a confirm Activity so the frontend can render a
		// confirmation card in the activity timeline.
		emitter := biz.ActivityEmitterFromContext(ctx)
		var confirmActivityID string
		if emitter != nil {
			confirmContent := fmt.Sprintf("工具 %s 需要确认后执行", toolKey)
			if decision.reason == confirmReasonShellOnFailure {
				confirmContent = fmt.Sprintf("上一条命令失败后，工具 %s 需要再次确认", toolKey)
			}
			id, emitErr := emitter.EmitConfirmRequest(ctx, biz.ActivityConfirmParams{
				ToolName:      toolKey,
				ToolArguments: string(args.Arguments),
				Content:       confirmContent,
				// Route the step to this exact tool call's await channel so
				// parallel confirmations no longer share one session-level
				// slot (BUG-02, chat-e2e-20260823).
				ToolCallID: args.ToolCallID,
				// 75 A5: computer-use danger-word hits surface a 高危 badge on
				// the confirm card.
				Danger: decision.reason == confirmReasonPolicyDanger || decision.reason == confirmReasonShellDanger,
				// Attribute the confirm step to the agent whose tool is
				// gated (team member in graph mode); the projector's base
				// meta carries the anchor agent key.
				AuthorAgentKey: h.ag.AgentKey,
			})
			if emitErr != nil {
				h.deps.Logger().Warn("EmitConfirmRequest failed",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Err(emitErr))
			} else {
				confirmActivityID = id
			}
		}
		confirmCtx := serviceawaitreply.WithToolConfirmRequest(ctx, serviceawaitreply.ToolConfirmRequest{
			ToolKey:    toolKey,
			ToolCallID: args.ToolCallID,
		})
		// Apply confirmation timeout to prevent indefinite blocking.
		confirmCtx, confirmCancel := context.WithTimeout(confirmCtx, effectiveConfirmTimeout)
		defer confirmCancel()
		reply, err := fn(confirmCtx)
		// N-21: Update the confirm Activity with the user's response. A deadline
		// expiry is NOT a user rejection — emit the timeout variant so the UI
		// renders "已超时" instead of "已拒绝".
		confirmationTimedOut := err != nil && confirmCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil
		if emitter != nil && confirmActivityID != "" {
			if confirmationTimedOut {
				if emitErr := emitter.EmitConfirmTimeout(ctx, confirmActivityID); emitErr != nil {
					h.deps.Logger().Warn("EmitConfirmTimeout failed",
						loggateway.StepID("agent.tool_confirm"),
						loggateway.Err(emitErr))
				}
			} else {
				approved := err == nil && toolConfirmApproved(reply)
				if emitErr := emitter.EmitConfirmResult(ctx, confirmActivityID, approved); emitErr != nil {
					h.deps.Logger().Warn("EmitConfirmResult failed",
						loggateway.StepID("agent.tool_confirm"),
						loggateway.Err(emitErr))
				}
			}
		}
		if err != nil {
			// Distinguish between confirmation timeout and other errors.
			// Only report ErrorCodeConfirmationTimeout when the confirmation
			// deadline itself expired but the parent context is still alive.
			// If the parent context (e.g., tool execution timeout) also expired,
			// the error is not a confirmation timeout — it will be handled by
			// the execution timeout logic upstream.
			if confirmationTimedOut {
				recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
					ToolKey:      toolKey,
					AgentID:      h.ag.ID,
					Status:       "blocked",
					ErrorCode:    event.ErrorCodeConfirmationTimeout,
					ErrorMessage: fmt.Sprintf("tool confirmation timed out after %s (decision_reason=%s)", effectiveConfirmTimeout, decision.reason),
					InputPreview: previewFromToolArgs(args.Arguments),
					StartedAt:    time.Now().UTC().Format(time.RFC3339),
					EndedAt:      time.Now().UTC().Format(time.RFC3339),
					Source:       biz.ToolInvocationSourceRuntime,
					ToolCallID:   args.ToolCallID,
					ParamsJSON:   paramsJSONFromToolArgs(args.Arguments),
				}, nil, h.ag, h.deps)
				// M80 1.5: 超时是 HITL 决策的一种结果（非用户拒绝），独立 outcome。
				h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "timeout", "", args.Arguments, "")
				// P1-3: 超时是显式 Reject（CustomResult 短路），不是回调错误——
				// 用户未响应≠拦截器故障，error 路径会触发框架 Errorf 误报。
				return callbacks.Reject(fmt.Sprintf("%s: 工具 \"%s\" 的确认请求在 %s 内未收到用户响应（超时）。这不代表用户拒绝，只是暂时没有回应。不要立即重试该工具；请先询问用户是否仍要执行该操作。", errToolConfirmationRequired, toolKey, effectiveConfirmTimeout)).BeforeToolResult(ctx), nil
			}
			// 回复通道自身故障属基础设施错误，保留 error 语义（框架按拦截器
			// 故障处理）。
			return nil, fmt.Errorf("%s: awaiting user confirmation failed: %w", errToolConfirmationRequired, err)
		}
		if toolConfirmApproved(reply) {
			// Grant side effects for grant-scoped approvals. The current
			// invocation is always allowed; a failed grant write only means
			// the next invocation prompts again (fail-closed).
			h.applyGrantOutcome(ctx, sessionID, toolKey, reply)
			// M80 1.5: approve 分支工具尚未执行、无 tool_invocations 行，
			// decision_record 先行（tool_invocation_id=ToolCallID 允许悬空）。
			h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "approved", reply, args.Arguments, "")
			metrics.PluginInvokeTotal.WithLabelValues("confirm_gate", "before_tool", "success").Inc()
			h.deps.Logger().Info("tool confirmation approved",
				loggateway.StepID("agent.tool_confirm"),
				loggateway.Str("tool", toolKey),
				loggateway.Str("agent_id", h.ag.ID),
				loggateway.Str("decision_reason", decision.reason))
			// P1-10: mark context so ConfirmationGuardPlugin skips its own
			// check. Without this, the plugin (which runs after Chain
			// callbacks via mergeToolCallbacks) would re-block the tool that
			// the user just approved. See E2E-P1-10.
			return &trpctool.BeforeToolResult{Context: plugintrpc.WithToolConfirmHandled(ctx)}, nil
		}
		recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
			ToolKey:      toolKey,
			AgentID:      h.ag.ID,
			Status:       "blocked",
			ErrorCode:    event.ErrorCodeConfirmationDenied,
			ErrorMessage: fmt.Sprintf("user denied tool confirmation (decision_reason=%s)", decision.reason),
			InputPreview: previewFromToolArgs(args.Arguments),
			StartedAt:    time.Now().UTC().Format(time.RFC3339),
			EndedAt:      time.Now().UTC().Format(time.RFC3339),
			Source:       biz.ToolInvocationSourceRuntime,
			ToolCallID:   args.ToolCallID,
			ParamsJSON:   paramsJSONFromToolArgs(args.Arguments),
		}, nil, h.ag, h.deps)
		// M80 1.5: 用户拒绝。
		h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "rejected", reply, args.Arguments, "")
		// P1-3: 用户拒绝是显式 Reject 决策（CustomResult 短路），不走 error——error 语义保留给拦截器自身故障，且 error
		// 路径会触发框架 "Before tool callback failed" Errorf 误报。
		return callbacks.Reject(fmt.Sprintf("%s: 用户拒绝了工具 \"%s\" 的执行。这是用户的明确决定，不是系统故障。禁止重试相同或等价的工具调用；请直接向用户说明该操作已被取消，并询问接下来如何处理。", errToolConfirmationRequired, toolKey)).BeforeToolResult(ctx), nil
	}

	recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
		ToolKey:      toolKey,
		AgentID:      h.ag.ID,
		Status:       "blocked",
		ErrorCode:    event.ErrorCodeConfirmationRequired,
		ErrorMessage: fmt.Sprintf("tool requires user confirmation before execution (decision_reason=%s)", decision.reason),
		InputPreview: previewFromToolArgs(args.Arguments),
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		EndedAt:      time.Now().UTC().Format(time.RFC3339),
		Source:       biz.ToolInvocationSourceRuntime,
		ToolCallID:   args.ToolCallID,
		ParamsJSON:   paramsJSONFromToolArgs(args.Arguments),
	}, nil, h.ag, h.deps)
	// M80 1.5: 无回复通道按拒绝处理，block_cause 标记环境原因（非用户意愿）。
	h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "rejected", "", args.Arguments, "no_reply_channel")
	// P1-3: 无回复通道 = 显式 Reject（环境能力不满足，非拦截器故障）。
	return callbacks.Reject(fmt.Sprintf("%s: 工具 \"%s\" 需要用户确认后才能执行，但当前运行环境无法向用户发起确认请求（无回复通道）。该工具本次不可执行，不要重试；请向用户说明情况，并请用户在支持确认的会话中重新发起该操作。", errToolConfirmationRequired, toolKey)).BeforeToolResult(ctx), nil
}

// toolConfirmSessionID extracts the session ID from the invocation context.
// Empty when no invocation/session is attached; grant lookups with an empty
// session ID never match (fail-closed).
func toolConfirmSessionID(ctx context.Context) string {
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
		return inv.Session.ID
	}
	return ""
}

// applyGrantOutcome records session-scoped / persisted grants when the user
// approved with a grant scope. Grant write failures are logged but never
// block the already-approved invocation (fail-closed: the next invocation
// simply prompts again).
func (h *toolConfirmationBeforeHook) applyGrantOutcome(ctx context.Context, sessionID, toolKey, reply string) {
	outcome, structured := serviceawaitreply.ParseToolConfirmOutcome(reply)
	if !structured {
		return
	}
	switch outcome {
	case serviceawaitreply.ToolConfirmOutcomeApproveSession:
		if h.gate.sessionGrants != nil {
			h.gate.sessionGrants.GrantSession(sessionID, h.ag.ID, toolKey)
			h.deps.Logger().Info("session tool grant recorded",
				loggateway.StepID("agent.tool_confirm"),
				loggateway.Str("tool", toolKey),
				loggateway.Str("agent_id", h.ag.ID),
				loggateway.Str("decision_reason", confirmReasonGrantSession))
		}
	case serviceawaitreply.ToolConfirmOutcomeApproveAlways:
		if h.deps.ToolUC == nil {
			return
		}
		if err := h.deps.ToolUC.GrantTool(ctx, h.ag.ID, toolKey, toolConfirmUserID(ctx)); err != nil {
			h.deps.Logger().Warn("persist tool grant failed",
				loggateway.StepID("agent.tool_confirm"),
				loggateway.Str("tool", toolKey),
				loggateway.Str("agent_id", h.ag.ID),
				loggateway.Err(err))
			return
		}
		h.deps.Logger().Info("persisted tool grant recorded",
			loggateway.StepID("agent.tool_confirm"),
			loggateway.Str("tool", toolKey),
			loggateway.Str("agent_id", h.ag.ID),
			loggateway.Str("decision_reason", confirmReasonGrantPersisted))
	}
}

// toolConfirmUserID returns the session user ID for grant audit attribution.
func toolConfirmUserID(ctx context.Context) string {
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
		return inv.Session.UserID
	}
	return ""
}

// emitToolConfirmDecisionRecord 把 HITL 工具确认结果双写到 M80 决策记录层
// (hitl_approval，设计 §3.2 row 2)。collector 为 nil（旧构造路径）时静默跳过；
// 与 recordToolInvocationWrite 同坐标调用，四出口分支各一次。
// blockCause 仅无回复通道分支使用（"no_reply_channel"）。
func (h *toolConfirmationBeforeHook) emitToolConfirmDecisionRecord(ctx context.Context, toolKey, toolCallID, gateReason, outcome, reply string, arguments []byte, blockCause string) {
	c := h.deps.DecisionCollector
	if c == nil {
		return
	}
	userID := toolConfirmUserID(ctx)
	if userID == "" {
		userID = "unknown"
	}
	scenario := fmt.Sprintf("工具确认: %s (%s)", toolKey, gateReason)
	if target := strings.TrimSpace(previewFromToolArgs(arguments)); target != "" {
		scenario += " 目标: " + target
	}
	metadata := map[string]any{"decision_reason": gateReason}
	if blockCause != "" {
		metadata["block_cause"] = blockCause
	}
	// reasoning 只承载自由文本备注；结构化回复（按钮 token）映射 grant_scope，
	// 不把 "__aranea:tool_confirm:*" 内部令牌写进审计文本。
	reasoning := ""
	if outcome == "approved" || outcome == "rejected" {
		if parsed, structured := serviceawaitreply.ParseToolConfirmOutcome(reply); structured {
			switch parsed {
			case serviceawaitreply.ToolConfirmOutcomeApprove:
				metadata["grant_scope"] = "once"
			case serviceawaitreply.ToolConfirmOutcomeApproveSession:
				metadata["grant_scope"] = "session"
			case serviceawaitreply.ToolConfirmOutcomeApproveAlways:
				metadata["grant_scope"] = "always"
			}
		} else {
			reasoning = strings.TrimSpace(reply)
		}
	}
	entities := []decision.EntityRef{{Type: "tool", Key: toolKey}}
	if h.ag.AgentKey != "" {
		entities = append(entities, decision.EntityRef{Type: "agent", Key: h.ag.AgentKey})
	}
	c.Emit(ctx, decision.Record{
		DecisionKey:     uuid.NewString(),
		Category:        decision.CategoryHITLApproval,
		Scenario:        scenario,
		Reasoning:       reasoning,
		Outcome:         outcome,
		ActorType:       decision.ActorHuman,
		ActorKey:        userID,
		RelatedEntities: entities,
		SourceRef:       decision.SourceRef{ToolInvocationID: toolCallID},
		Metadata:        metadata,
	})
}

// toolConfirmationBypass reports whether tool confirmation should be
// skipped entirely. This is a development-only escape hatch.
//
// Security model: the bypass requires ARANEA_DEV_MODE to be set (only
// in local/docker-compose dev environments). In production, where
// ARANEA_DEV_MODE is never set, the bypass is impossible regardless
// of any other environment variable.
//
// The legacy KRATOS_TOOL_AUTO_APPROVE env var is also gated by
// ARANEA_DEV_MODE for backward compatibility.
func toolConfirmationBypass() bool {
	if strings.TrimSpace(os.Getenv("ARANEA_DEV_MODE")) == "" {
		return false
	}
	// Both the legacy and new env vars are accepted when in dev mode.
	if strings.TrimSpace(os.Getenv("KRATOS_TOOL_AUTO_APPROVE")) == "1" {
		return true
	}
	return strings.TrimSpace(os.Getenv("ARANEA_TOOL_AUTO_APPROVE")) == "1"
}
