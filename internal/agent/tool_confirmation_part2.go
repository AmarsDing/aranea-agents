package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/event"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// emitToolConfirmDecisionRecord 把 HITL 工具确认结果双写到 M80 决策记录层
// (hitl_approval，设计 §3.2 row 2)。flowlog 先写；collector 为 nil 时记
// collector_nil 错误，不再静默跳过（D1）。四出口分支各一次。
// blockCause 仅无回复通道分支使用（"no_reply_channel"）。
//
// P0-5 对偶审计（批 A12）：correlationID 为确认卡 activity id（首张卡，重试
// 补发沿用）——asked/decided 两个半部以同一 correlation_id 配对联结（ability
// 可回收）。无确认卡的出口（grant_skip / no_reply_channel）传空串（此时也无
// approval.asked 半部，语义诚实：无人被真正询问过）。事件定级 AS-EVT-01=
// Important（decision_records 经 outbox BlockUpTo；不进 Critical lane）。
func (h *toolConfirmationBeforeHook) emitToolConfirmDecisionRecord(ctx context.Context, toolKey, toolCallID, gateReason, outcome, reply string, arguments []byte, blockCause string, correlationID string, policyAskDriven bool) {
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
	if correlationID != "" {
		metadata["correlation_id"] = correlationID
	}
	projectApprovalSourceMode(ctx, metadata, policyAskDriven)

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

// emitToolConfirmAskedRecord 落 approval.asked 对偶半部（P0-5 批 A12）：确认
// 卡发出即记——此前只有 decided 侧五出口，asked 事件全缺，被 never/超时拒绝
// 的调用无独立审计轨迹。outcome=asked、Actor=system:hitl_approval；correlation
// 锚定首张确认卡 activity id，与后续 decided 半部配对（重试补发第二卡沿用
// 首个 id，保证整场确认流程 1:1 成对）。事件定级 AS-EVT-01=Important（同
// decided 半部：decision_records 经 outbox BlockUpTo，不进 Critical lane）。
// collector=nil 时 flowlog 仍落（EmitDecision D1 不变量）。
func (h *toolConfirmationBeforeHook) emitToolConfirmAskedRecord(ctx context.Context, toolKey, toolCallID, gateReason, correlationID string, arguments []byte, policyAskDriven bool) {
	if strings.TrimSpace(correlationID) == "" {
		return
	}
	scenario := fmt.Sprintf("工具确认请求: %s (%s)", toolKey, gateReason)
	if target := strings.TrimSpace(previewFromToolArgs(arguments)); target != "" {
		scenario += " 目标: " + target
	}
	metadata := map[string]any{
		"decision_reason": gateReason,
		"correlation_id":  correlationID,
	}
	projectApprovalSourceMode(ctx, metadata, policyAskDriven)
	entities := []decision.EntityRef{{Type: "tool", Key: toolKey}}
	if h.ag.AgentKey != "" {
		entities = append(entities, decision.EntityRef{Type: "agent", Key: h.ag.AgentKey})
	}
	sessionID := toolConfirmSessionID(ctx)
	if sessionID != "" {
		metadata["session_id"] = sessionID
	}
	flowPairs := []event.Pair{event.P("trigger", "hitl_approval"), event.P("outcome", "asked")}
	if toolKey != "" {
		flowPairs = append(flowPairs, event.P("tool_key", toolKey))
	}
	event.EmitDecision(ctx, h.deps.DecisionCollector, decision.Record{
		DecisionKey:     uuid.NewString(),
		Category:        decision.CategoryHITLApproval,
		Scenario:        scenario,
		Outcome:         "asked",
		ActorType:       decision.ActorSystem,
		ActorKey:        "system:hitl_approval",
		RelatedEntities: entities,
		SourceRef:       decision.SourceRef{ToolInvocationID: toolCallID, SessionID: sessionID},
		Metadata:        metadata,
	}, "system.gate.hitl_approval", scenario, flowPairs...)
}

// projectApprovalSourceMode 投影审批裁决来源档（D5/phase 2）：auto verdict 产生
// 的确认记录携带 source_mode=auto——auto 与 ask 同消费路径（reason=policy_ask），
// auto_review_verdict 审计经此字段区分两种来源；ask/trusted/无 verdict 路径不
// 投影（缺省即普通 HITL，零存量记录污染）。
//
// D5-f 裁决（policyAskDriven 显式驱动位）：投影仅在「审批策略 ask/auto 分支
// 驱动」的记录上生效。param rule ask 驱动的记录（reason=param_rule_ask）即使
// ctx 同时携带 auto verdict 也不投影——裁决链 param 分支先于 policy 分支，
// auto 未驱动该流程。auto+grant 秒过（policy 分支内 grant 满足，grant_skip
// 出口）policyAskDriven=true，投影照常。
func projectApprovalSourceMode(ctx context.Context, metadata map[string]any, policyAskDriven bool) {
	if !policyAskDriven {
		return
	}
	if pv := approvalPolicyVerdictFromCtx(ctx); pv != nil && pv.mode == biztool.ApprovalModeAuto {
		metadata["source_mode"] = string(biztool.ApprovalModeAuto)
	}
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
