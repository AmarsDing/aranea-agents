package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
	// confirmRetries is extra confirmation waits after the first timeout.
	// 0 = default 1 retry (re-issue the card once). Negative = no retry (tests).
	confirmRetries int
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
		// P1-10 不变量对放行态同样成立（2026-08-27 三轮审查根修）：产品
		// 门禁已作出「无需确认」裁定，与另两出口（:75 pluginAllow、:218
		// approve）对齐标记 handled——执行序若如 confirm_policy.go 注释
		// 所述「插件在链后」，ConfirmationGuardPlugin 不得再硬拦已放行
		// 的调用（param_rule_allow 走的正是本出口）。
		return &trpctool.BeforeToolResult{Context: plugintrpc.WithToolConfirmHandled(ctx)}, nil
	}

	// effectiveConfirmTimeout resolves the confirmation wait budget
	// (test-overridable, otherwise per-tool TTL).
	effectiveConfirmTimeout := confirmTimeoutForTool(toolKey, h.confirmTimeout)

	if fn := serviceawaitreply.ReplyFuncFromContext(ctx); fn != nil {
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
			if markErr := trpcagent.MarkAwaitingUserReply(inv); markErr != nil {
				h.deps.Logger().Warn("MarkAwaitingUserReply failed",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Err(markErr))
			}
		}
		emitter := biz.ActivityEmitterFromContext(ctx)
		confirmContent := fmt.Sprintf("工具 %s 需要确认后执行", toolKey)
		if decision.reason == confirmReasonShellOnFailure {
			confirmContent = fmt.Sprintf("上一条命令失败后，工具 %s 需要再次确认", toolKey)
		}
		attempts := 1 + h.extraConfirmAttempts()
		var (
			reply                string
			err                  error
			confirmationTimedOut bool
			confirmActivityID    string
			follower             bool
		)
		waitConfirm := func() (string, error, bool) {
			var (
				waitReply    string
				waitErr      error
				waitTimedOut bool
			)
			for attempt := 0; attempt < attempts; attempt++ {
			content := confirmContent
			if attempt > 0 {
				content = fmt.Sprintf("确认已超时，请再次确认是否执行工具 %s（可重试，不会静默取消）", toolKey)
			}
			if emitter != nil {
				id, emitErr := emitter.EmitConfirmRequest(ctx, biz.ActivityConfirmParams{
					ToolName:       toolKey,
					ToolArguments:  string(args.Arguments),
					Content:        content,
					ToolCallID:     args.ToolCallID,
					Danger:         decision.reason == confirmReasonPolicyDanger || decision.reason == confirmReasonShellDanger,
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
			confirmReqCtx := serviceawaitreply.WithToolConfirmRequest(ctx, serviceawaitreply.ToolConfirmRequest{
				ToolKey:    toolKey,
				ToolCallID: args.ToolCallID,
			})
			waitStart := time.Now()
			confirmCtx, confirmCancel := context.WithTimeout(confirmReqCtx, effectiveConfirmTimeout)
			reply, err = fn(confirmCtx)
			waitMS := int(time.Since(waitStart).Milliseconds())
			AddConfirmWaitMS(ctx, waitMS)
			confirmationTimedOut = err != nil && confirmCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil
			confirmCancel()
			if emitter != nil && confirmActivityID != "" {
				if confirmationTimedOut {
					timeoutCtx := ctx
					if attempt+1 < attempts {
						timeoutCtx = biz.WithConfirmTimeoutRetrying(ctx)
					}
					if emitErr := emitter.EmitConfirmTimeout(timeoutCtx, confirmActivityID); emitErr != nil {
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
			if confirmationTimedOut && attempt+1 < attempts {
				h.deps.Logger().Info("tool confirmation timed out, re-issuing",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Str("tool", toolKey),
					loggateway.Int("attempt", attempt+1))
				continue
			}
			break
		}
		if err != nil {
			if confirmationTimedOut {
				recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
					ToolKey:      toolKey,
					AgentID:      h.ag.ID,
					Status:       "blocked",
					ErrorCode:    event.ErrorCodeConfirmationTimeout,
					ErrorMessage: fmt.Sprintf("tool confirmation timed out after %s x%d (decision_reason=%s)", effectiveConfirmTimeout, attempts, decision.reason),
					InputPreview: previewFromToolArgs(args.Arguments),
					StartedAt:    time.Now().UTC().Format(time.RFC3339),
					EndedAt:      time.Now().UTC().Format(time.RFC3339),
					Source:       biz.ToolInvocationSourceRuntime,
					ToolCallID:   args.ToolCallID,
					ParamsJSON:   paramsJSONFromToolArgs(args.Arguments),
				}, nil, h.ag, h.deps)
				h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "timeout", "", args.Arguments, "retryable_timeout")
				return callbacks.Reject(fmt.Sprintf("%s: 工具 \"%s\" 的确认请求已连续 %d 次超时（每次 %s）。这不代表用户拒绝，也不是整步被静默取消——确认卡仍然有效，用户可随时批准。请告知用户界面上的确认已超时并可以重试，询问是否仍要执行；不要当作本步已取消。", errToolConfirmationRequired, toolKey, attempts, effectiveConfirmTimeout)).BeforeToolResult(ctx), nil
			}
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

func (h *toolConfirmationBeforeHook) extraConfirmAttempts() int {
	if h.confirmRetries < 0 {
		return 0
	}
	if h.confirmRetries == 0 {
		return 1
	}
	return h.confirmRetries
}

type confirmWaitAccKey struct{}

// WithConfirmWaitAcc attaches a HITL wait accumulator to ctx so turn usage
// can split wait_ms out of wall latency.
func WithConfirmWaitAcc(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(confirmWaitAccKey{}).(*int64); ok {
		return ctx
	}
	var acc int64
	return context.WithValue(ctx, confirmWaitAccKey{}, &acc)
}

// AddConfirmWaitMS records milliseconds spent waiting on HITL confirmation.
func AddConfirmWaitMS(ctx context.Context, ms int) {
	if ms <= 0 {
		return
	}
	v, _ := ctx.Value(confirmWaitAccKey{}).(*int64)
	if v == nil {
		return
	}
	atomic.AddInt64(v, int64(ms))
}

// ConfirmWaitMS returns accumulated HITL wait milliseconds on ctx.
func ConfirmWaitMS(ctx context.Context) int {
	v, _ := ctx.Value(confirmWaitAccKey{}).(*int64)
	if v == nil {
		return 0
	}
	return int(atomic.LoadInt64(v))
}
