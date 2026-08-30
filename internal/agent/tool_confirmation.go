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
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const errToolConfirmationRequired = "TOOL_CONFIRMATION_REQUIRED"

// defaultToolConfirmationTimeout is the maximum duration to wait for a user
// to approve or deny a tool confirmation request. After this deadline the
// tool invocation is rejected with ErrorCodeConfirmationTimeout.
const defaultToolConfirmationTimeout = 5 * time.Minute

// hitlWaitVisibleAfter is the threshold after which a waiting confirmation
// becomes session-visible (notice + log). Timeout retry/reject actions stay
// opt-in; this round only closes the silent-wait gap (S7 ~8.5min).
const hitlWaitVisibleAfter = 60 * time.Second

const hitlWaitNoticeType = "hitl_wait"

type toolCallStartKey struct{}

type toolConfirmationBeforeHook struct {
	gate *toolConfirmGate
	ag   biz.Agent
	deps TRPCBuilderDeps
	// loopGuard 由装配链注入（P2-③ HITL 恢复路径）：非批准出口（拒绝/超时/
	// 无回复通道）下工具不再执行、框架短路、AfterTool 不运行，必须显式归还
	// 守卫 inflight 槽位；拒绝/无通道还登记否决签名，同参重发由守卫首拦。
	// nil-safe（单测可不注入）。
	loopGuard *toolLoopGuard
	// confirmTimeout bounds the wait for a user decision. Zero means
	// defaultToolConfirmationTimeout; overridable in tests.
	confirmTimeout time.Duration
	// confirmRetries is extra confirmation waits after the first timeout.
	// 0 = default 1 retry (re-issue the card once). Negative = no retry (tests).
	confirmRetries int
	// hitlVisibleAfter overrides hitlWaitVisibleAfter in tests. Zero = default.
	hitlVisibleAfter time.Duration
}

var _ callbacks.BeforeToolHook = (*toolConfirmationBeforeHook)(nil)

func newToolConfirmationBeforeHook(gate *toolConfirmGate, ag biz.Agent, deps TRPCBuilderDeps) *toolConfirmationBeforeHook {
	return &toolConfirmationBeforeHook{gate: gate, ag: ag, deps: deps}
}

// setLoopGuard 注入工具循环守卫（P2-③）。装配链在确认门禁注册前调用；
// 重复设置同值无害。
func (h *toolConfirmationBeforeHook) setLoopGuard(g *toolLoopGuard) {
	h.loopGuard = g
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
			// F12：grant 秒过必须落 hitl_approval 决策记录。S07 复查 Evidence：
			// 同参 spawn 前 2 次超时 blocked、首次 approve 后第 3 次 5ms 放行——
			// 放行本身符合 spawn 批量授权设计，但决策层无迹可寻，被误判为
			// 确认闸策略不一致。shell_safe 是 E1 免确认设计（非人工授权事件），
			// 不进 hitl_approval。
			if decision.reason == confirmReasonGrantSession || decision.reason == confirmReasonGrantPersisted {
				h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "grant_skip", "", args.Arguments, "")
			}
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
		attempts := 1 + extraConfirmAttemptsForTool(toolKey, h.confirmRetries)
		var confirmActivityID string
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
				stopVisible := h.startHITLWaitVisibility(ctx, toolKey, emitter)
				confirmCtx, confirmCancel := context.WithTimeout(confirmReqCtx, effectiveConfirmTimeout)
				waitReply, waitErr = fn(confirmCtx)
				stopVisible()
				waitMS := int(time.Since(waitStart).Milliseconds())
				AddConfirmWaitMS(ctx, waitMS)
				h.deps.Logger().Info("HITL confirmation wait finished",
					loggateway.StepID("agent.hitl_wait"),
					loggateway.Str("tool", toolKey),
					loggateway.Int("hitl_wait_ms", waitMS),
					loggateway.Int("attempt", attempt+1),
				)
				waitTimedOut = waitErr != nil && confirmCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil
				confirmCancel()
				if emitter != nil && confirmActivityID != "" {
					if waitTimedOut {
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
						approved := waitErr == nil && toolConfirmApproved(waitReply)
						if emitErr := emitter.EmitConfirmResult(ctx, confirmActivityID, approved); emitErr != nil {
							h.deps.Logger().Warn("EmitConfirmResult failed",
								loggateway.StepID("agent.tool_confirm"),
								loggateway.Err(emitErr))
						}
					}
				}
				if waitTimedOut && attempt+1 < attempts {
					h.deps.Logger().Info("tool confirmation timed out, re-issuing",
						loggateway.StepID("agent.tool_confirm"),
						loggateway.Str("tool", toolKey),
						loggateway.Int("attempt", attempt+1))
					continue
				}
				break
			}
			return waitReply, waitErr, waitTimedOut
		}
		reply, err, confirmationTimedOut, follower := waitConfirmCoalesced(sessionID, h.ag.ID, toolKey, ctx, waitConfirm)
		if err != nil {
			if confirmationTimedOut {
				// P2-③：超时不构成用户否决（确认卡仍有效，用户授意的重发须能
				// 再走确认流程）——只归还守卫 inflight 槽位，不登记 denied。
				// leader 与 follower 各自的调用都已在守卫侧 beginInflight，
				// 必须逐调用归还，故不随 follower 早退跳过。
				h.loopGuard.noteConfirmationOutcome(ctx, toolKey, args.Arguments, false)
				if !follower {
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
					h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "timeout", "", args.Arguments, timeoutBlockCause(toolKey))
				}
				timeoutMsg := toolConfirmTimeoutMessage(toolKey, attempts, effectiveConfirmTimeout)
				// P2-③ 方案 C 回执：存在未核销副作用时引导模型先补偿清除而非重发。
				if cue := planCCompensationCue(ctx, toolKey); cue != "" {
					timeoutMsg += " " + cue
				}
				return callbacks.Reject(timeoutMsg).BeforeToolResult(ctx), nil
			}
			// P2-③：等待确认异常（如 ctx 取消）同样短路、工具未执行，归还槽位。
			h.loopGuard.noteConfirmationOutcome(ctx, toolKey, args.Arguments, false)
			return nil, fmt.Errorf("%s: awaiting user confirmation failed: %w", errToolConfirmationRequired, err)
		}
		if toolConfirmApproved(reply) {
			h.applyGrantOutcome(ctx, sessionID, toolKey, reply)
			h.maybeSessionGrantBatchTool(sessionID, toolKey)
			if !follower {
				h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "approved", reply, args.Arguments, "")
				metrics.PluginInvokeTotal.WithLabelValues("confirm_gate", "before_tool", "success").Inc()
				h.deps.Logger().Info("tool confirmation approved",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Str("tool", toolKey),
					loggateway.Str("agent_id", h.ag.ID),
					loggateway.Str("decision_reason", decision.reason))
			}
			return &trpctool.BeforeToolResult{Context: plugintrpc.WithToolConfirmHandled(ctx)}, nil
		}
		// P2-③：用户明确否决——归还守卫 inflight 槽位并登记 denied 签名，
		// 本节点生命周期内同参重发由守卫首次即拦（守卫侧拼方案 C 引导）。
		// 不随 follower 早退跳过：各调用的 inflight 需逐一归还。
		h.loopGuard.noteConfirmationOutcome(ctx, toolKey, args.Arguments, true)
		if !follower {
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
			h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "rejected", reply, args.Arguments, "")
		}
		denyMsg := fmt.Sprintf("%s: 用户拒绝了工具 \"%s\" 的执行。这是用户的明确决定，不是系统故障。禁止重试相同或等价的工具调用；请直接向用户说明该操作已被取消，并询问接下来如何处理。", errToolConfirmationRequired, toolKey)
		// P2-③ 方案 C 回执：存在未核销副作用时引导模型先补偿清除而非重发。
		if cue := planCCompensationCue(ctx, toolKey); cue != "" {
			denyMsg += " " + cue
		}
		return callbacks.Reject(denyMsg).BeforeToolResult(ctx), nil
	}

	// P2-③：无回复通道——本 invocation 内重发必然同路失败，归还守卫
	// inflight 槽位并登记 denied 签名，同参重发由守卫首次即拦止损。
	h.loopGuard.noteConfirmationOutcome(ctx, toolKey, args.Arguments, true)
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
	h.emitToolConfirmDecisionRecord(ctx, toolKey, args.ToolCallID, decision.reason, "rejected", "", args.Arguments, "no_reply_channel")
	noChannelMsg := fmt.Sprintf("%s: 工具 \"%s\" 需要用户确认后才能执行，但当前运行环境无法向用户发起确认请求（无回复通道）。该工具本次不可执行，不要重试；请向用户说明情况，并请用户在支持确认的会话中重新发起该操作。", errToolConfirmationRequired, toolKey)
	if cue := planCCompensationCue(ctx, toolKey); cue != "" {
		noChannelMsg += " " + cue
	}
	return callbacks.Reject(noChannelMsg).BeforeToolResult(ctx), nil
}

// confirmTimeoutForTool returns the HITL wait budget. Tests may override via
// hook.confirmTimeout. Spawn cards are shorter (2m) so a burst does not pin
// the UI for 5m x N; shell/default stay at 5m.
func confirmTimeoutForTool(toolKey string, override time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	switch toolKey {
	case "subagents_spawn":
		return 2 * time.Minute
	case "send_email", "message":
		return 3 * time.Minute
	default:
		return defaultToolConfirmationTimeout
	}
}

// sessionBatchGrantTool is confirmed once per session after the first approve
// (spawn 7→1: remaining parallel/follow-up spawns reuse the grant).
func sessionBatchGrantTool(toolKey string) bool {
	return toolKey == "subagents_spawn"
}

func (h *toolConfirmationBeforeHook) maybeSessionGrantBatchTool(sessionID, toolKey string) {
	if h == nil || h.gate == nil || h.gate.sessionGrants == nil {
		return
	}
	if !sessionBatchGrantTool(toolKey) {
		return
	}
	h.gate.sessionGrants.GrantSession(sessionID, h.ag.ID, toolKey)
}

type confirmCoalesceSlot struct {
	done     chan struct{}
	reply    string
	err      error
	timedOut bool
}

var confirmCoalesce sync.Map

// waitConfirmCoalesced lets parallel subagents_spawn share one confirm card.
// The first waiter is the leader and runs wait(); followers block on the
// same result. Non-spawn tools always run wait() themselves.
func waitConfirmCoalesced(sessionID, agentID, toolKey string, ctx context.Context, wait func() (string, error, bool)) (reply string, err error, timedOut, follower bool) {
	if !sessionBatchGrantTool(toolKey) || sessionID == "" || agentID == "" {
		reply, err, timedOut = wait()
		return
	}
	key := sessionID + "\x00" + agentID + "\x00" + toolKey
	slot := &confirmCoalesceSlot{done: make(chan struct{})}
	actual, loaded := confirmCoalesce.LoadOrStore(key, slot)
	s := actual.(*confirmCoalesceSlot)
	if loaded {
		select {
		case <-s.done:
			return s.reply, s.err, s.timedOut, true
		case <-ctx.Done():
			return "", ctx.Err(), false, true
		}
	}
	reply, err, timedOut = wait()
	s.reply, s.err, s.timedOut = reply, err, timedOut
	close(s.done)
	confirmCoalesce.Delete(key)
	return reply, err, timedOut, false
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
// (hitl_approval，设计 §3.2 row 2)。flowlog 先写；collector 为 nil 时记
// collector_nil 错误，不再静默跳过（D1）。四出口分支各一次。
// blockCause 仅无回复通道分支使用（"no_reply_channel"）。
func (h *toolConfirmationBeforeHook) emitToolConfirmDecisionRecord(ctx context.Context, toolKey, toolCallID, gateReason, outcome, reply string, arguments []byte, blockCause string) {
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
	sessionID := toolConfirmSessionID(ctx)
	if sessionID != "" {
		metadata["session_id"] = sessionID
	}
	// SP-1b：统一入口一次完成 decision_records + flow_log 双写；collector 为
	// nil 时 EmitDecision 内部记 collector_nil 且 flowlog 仍落（D1）。
	flowPairs := []event.Pair{event.P("trigger", "hitl_approval"), event.P("outcome", outcome)}
	if toolKey != "" {
		flowPairs = append(flowPairs, event.P("tool_key", toolKey))
	}
	event.EmitDecision(ctx, h.deps.DecisionCollector, decision.Record{
		DecisionKey:     uuid.NewString(),
		Category:        decision.CategoryHITLApproval,
		Scenario:        scenario,
		Reasoning:       reasoning,
		Outcome:         outcome,
		ActorType:       decision.ActorHuman,
		ActorKey:        userID,
		RelatedEntities: entities,
		SourceRef:       decision.SourceRef{ToolInvocationID: toolCallID, SessionID: sessionID},
		Metadata:        metadata,
	}, "system.gate.hitl_approval", scenario, flowPairs...)
}

func toolConfirmSessionID(ctx context.Context) string {
	if sid := decision.GateSessionIDFromContext(ctx); sid != "" {
		return sid
	}
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
		return strings.TrimSpace(inv.Session.ID)
	}
	return ""
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

func extraConfirmAttemptsForTool(toolKey string, configured int) int {
	if sessionBatchGrantTool(toolKey) {
		return 0
	}
	if configured < 0 {
		return 0
	}
	if configured == 0 {
		return 1
	}
	return configured
}

func timeoutBlockCause(toolKey string) string {
	if sessionBatchGrantTool(toolKey) {
		return "timeout_degrade"
	}
	return "retryable_timeout"
}

func toolConfirmTimeoutMessage(toolKey string, attempts int, timeout time.Duration) string {
	if sessionBatchGrantTool(toolKey) {
		return fmt.Sprintf("%s: 工具 \"%s\" 的确认已超时（%s）。不要再次调用 %s，也不要执行该高危操作。请告知用户无人审批，并给出改走部门主管会话或精灵组队的路由建议；禁止声称已经派发。",
			errToolConfirmationRequired, toolKey, timeout, toolKey)
	}
	return fmt.Sprintf("%s: 工具 \"%s\" 的确认请求已连续 %d 次超时（每次 %s）。这不代表用户拒绝，也不是整步被静默取消——确认卡仍然有效，用户可随时批准。请告知用户界面上的确认已超时并可以重试，询问是否仍要执行；不要当作本步已取消。",
		errToolConfirmationRequired, toolKey, attempts, timeout)
}

func (h *toolConfirmationBeforeHook) visibleWaitAfter() time.Duration {
	if h != nil && h.hitlVisibleAfter > 0 {
		return h.hitlVisibleAfter
	}
	return hitlWaitVisibleAfter
}

// startHITLWaitVisibility emits a session-visible notice after the wait
// threshold so unattended confirmations are not a silent stall. Returns a
// stop func that must be called when the wait returns (approve/timeout).
func (h *toolConfirmationBeforeHook) startHITLWaitVisibility(ctx context.Context, toolKey string, emitter biz.ActivityEmitter) func() {
	stop := make(chan struct{})
	after := h.visibleWaitAfter()
	lg := h.deps.Logger()
	safego.Go(ctx, "hitl-wait-visible", func() {
		timer := time.NewTimer(after)
		defer timer.Stop()
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-timer.C:
			if lg != nil {
				lg.Info("HITL confirmation still waiting",
					loggateway.StepID("agent.hitl_wait"),
					loggateway.Str("tool", toolKey),
					loggateway.Int64("waited_ms", after.Milliseconds()),
				)
			}
			if emitter == nil {
				return
			}
			content := fmt.Sprintf("仍在等待你确认工具 %s（已等待超过 %s）", toolKey, after.Truncate(time.Second))
			if err := emitter.EmitNotice(ctx, content, hitlWaitNoticeType); err != nil && lg != nil {
				lg.Warn("EmitNotice hitl_wait failed",
					loggateway.StepID("agent.hitl_wait"),
					loggateway.Err(err))
			}
		}
	})
	return func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
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
