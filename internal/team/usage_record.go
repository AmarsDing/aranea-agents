package team

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// recordMemberUsage writes model_token_usage_events for a team member step when tokens were consumed.
// usageSource records how tin/tout were obtained ("streaming"/"estimated"…);
// persisted into metadata_json["usage_source"] so estimated rows stay identifiable.
//
// attribution records metadata_json["usage_attribution"]. Non-empty marks
// completion-path rows (run_level_anchor_fallback / member_level_stream /
// stream_anchor_remainder): such rows duplicate the team_turn row's totals, so
// they must NOT additionally accumulate session metrics — the team_turn row is
// the single session-metrics accumulator on completion paths (P2-1/P2-1b,
// 2026-08-19). Empty attribution = genuine row on a path WITHOUT a team_turn
// row (e.g. failed-run partial steps) → keeps accumulating.
func (r *Runner) recordMemberUsage(
	ctx context.Context,
	run biz.TeamRunRecord,
	teamID string,
	ag biz.Agent,
	asst biz.ChatMessage,
	prov, mod, dialogMode string,
	stepID string,
	cachedTok int,
	usageSource string,
	attribution string,
) {
	if r == nil || r.usage == nil {
		return
	}
	tin, tout := asst.TokenIn, asst.TokenOut
	if tin <= 0 && tout <= 0 {
		return
	}
	now := time.Now().UTC()
	status := biz.NormalizeTokenUsageStatus(asst.Status)
	latency := asst.LatencyMS
	tps := 0.0
	if latency > 0 && tout > 0 {
		tps = float64(tout) / (float64(latency) / 1000.0)
	}
	meta := "{}"
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		meta = em.MetadataJSON()
	} else {
		b, _ := json.Marshal(map[string]any{
			"source":  "team_member_step",
			"run_id":  run.ID,
			"step_id": stepID,
		})
		meta = string(b)
	}
	meta = biz.MergeUsageSourceMetadata(meta, usageSource)
	meta = biz.MergeUsageAttributionMetadata(meta, attribution)
	ev := biz.TokenUsageEvent{
		ID:                uuid.NewString(),
		TeamID:            teamID,
		SessionID:         run.SessionID,
		AgentID:           ag.ID,
		AgentKey:          ag.AgentKey,
		MessageID:         stepID,
		ProviderCode:      prov,
		ModelAPIID:        mod,
		ModelDisplayName:  mod,
		InputTokens:       tin,
		OutputTokens:      tout,
		CachedInputTokens: cachedTok,
		TotalTokens:       tin + tout,
		LatencyMS:         latency,
		TokensPerSecond:   tps,
		Status:            status,
		ErrorMessage:      asst.ErrorMessage,
		UsageKind:         biz.UsageKindTeamMember,
		PromptMode:        dialogMode,
		MetadataJSON:      meta,
		OccurredAt:        now.Format(time.RFC3339),
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	// recEv carries the normalized row (CallCount=1); accumulate/publish from it
	// (P1-1 fix: ev.CallCount was 0, so session model_call_count never grew).
	recEv, err := r.usage.RecordTokenUsageEvent(recCtx, ev)
	if err != nil {
		r.lg.Warn("团队成员用量落库失败",
			loggateway.StepID("team.usage_record_fail"),
			loggateway.Err(err),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Str("team_id", teamID),
			loggateway.Str("step_id", stepID),
			loggateway.Str("usage_kind", ev.UsageKind),
		)
		return
	}
	if r.td.Sessions != nil && strings.TrimSpace(run.SessionID) != "" && strings.TrimSpace(attribution) == "" {
		r.td.Sessions.AccumulateMetricsDelta(session.SessionMetricsDelta{
			SessionID:         run.SessionID,
			ModelCallCount:    recEv.CallCount,
			InputTokens:       int64(recEv.InputTokens),
			OutputTokens:      int64(recEv.OutputTokens),
			TotalTokens:       int64(recEv.TotalTokens),
			TotalCostMicroUsd: recEv.TotalCostMicroUSD,
		})
	}
	// Run-level token budget gate (2026-08-24): accumulate only genuine member
	// rows (attribution=="" — mirror rows duplicate the team_turn totals and
	// would double-count), then cancel the run once on exceed. The cancel
	// propagates through the run ctx, so no further member steps execute.
	if strings.TrimSpace(attribution) == "" {
		if tripped, used, limit := r.accumulateRunTokenBudget(run.ID, recEv.InputTokens); tripped {
			r.tripRunTokenBudget(ctx, run, teamID, used, limit)
		}
	}
	// Envelope publishing is handled inside RecordTokenUsageEvent (P1-4) —
	// publishing here as well would double-emit.
}

// RunCancelReasonTokenBudget 是 token 预算闸跳闸的 run 取消原因（P2.5 口径，
// 与 RunCancelReasonNoProgress 同族）。写入 RunRegistry 状态条目 ErrMsg 与
// run 终态记录，审计/前端可区分守卫终止与用户停止。
const RunCancelReasonTokenBudget = "team_token_budget_exceeded"

// tripRunTokenBudget 是预算跳闸的完整处置序列（2026-08-26 方案A 抽公共）：
// 告警日志 → M80 系统闸决策双写（设计 §3.2 row 3，与 G-R5 同坐标）→ trace
// critical → system notice 事件 → RunRegistry.Cancel。finish-path
// （recordMemberUsage genuine 行累计）与 mid-stream（流式中途累计钩子）
// 两个入口共用，保证两处跳闸的日志/决策/事件/取消四件套完全一致。
//
// SetStatus 兜底：RunTeamTest 等不经 chat orchestrator 的路径从未
// StoreCancelable，Cancel 找不到活动条目空转、reason 丢失（终态降级为
// client_disconnect_or_abort）。Cancel 未生效时回退 SetStatus 直接写入
// cancelled+reason，保住 finishRunErr 的 P2.5 口径。
func (r *Runner) tripRunTokenBudget(ctx context.Context, run biz.TeamRunRecord, teamID string, used, limit int64) {
	r.lg.Warn("团队 run 累计 input token 超预算，取消 run",
		loggateway.StepID("team.token_budget.exceeded"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("team_id", teamID),
		loggateway.Str("session_id", run.SessionID),
		loggateway.Int64("budget_used_input_tokens", used),
		loggateway.Int64("budget_limit_input_tokens", limit),
	)
	event.EmitGate(ctx, r.cfg.DecisionCollector, decision.GateDecision{
		TriggerRule:   decision.TriggerTokenBudgetTripped,
		Outcome:       "tripped",
		Scenario:      "run 累计 input token 超预算",
		Reasoning:     fmt.Sprintf("run 累计 input %d 超预算上限 %d，取消 run", used, limit),
		GuardName:     "token_budget",
		RunID:         run.ID,
		SessionID:     run.SessionID,
		Entities:      []decision.EntityRef{{Type: "team", Key: teamID}},
		ObservedValue: used,
		Threshold:     limit,
		Action:        "cancel_run",
	})
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogCritical("chat.team.token_budget_exceeded", "团队 run 累计 input token 超预算，已取消",
			event.P("run_id", run.ID), event.P("used", used), event.P("limit", limit))
	}
	// P2.5：守卫终止在事件流分列——与 team_run_no_progress 同型注记，
	// reason 与 RunRegistry 状态条目/run 终态记录三处口径一致。
	spiritSID := run.SpiritSessionID
	if spiritSID == "" {
		spiritSID = run.SessionID
	}
	r.publishEvent(ctx, biz.NewSystemNoticeEvent(spiritSID, "team_run_token_budget_exceeded",
		"团队 run 累计 input token 超预算，run 已终止", map[string]any{
			"run_id":  run.ID,
			"team_id": teamID,
			"used":    used,
			"limit":   limit,
			"reason":  RunCancelReasonTokenBudget,
		}))
	if r.cfg.Runs != nil {
		if cancelled, _ := r.cfg.Runs.Cancel(run.SessionID, RunCancelReasonTokenBudget); !cancelled {
			// 无活动条目（RunTeamTest 路径）或条目不可取消：Cancel 未记录
			// reason，SetStatus 兜底写入，cancelReason 读回口径不失真。
			r.cfg.Runs.SetStatus(run.SessionID, run.ID, biz.SessionRunPhaseCancelled, RunCancelReasonTokenBudget)
		}
	}
	r.backfillTrippedRunTokenIn(ctx, run, used)
}

// backfillTrippedRunTokenIn 回填被预算中止 run 的 token_in（2026-08-27
// t-dr-4）。预算中止走 finishRunErr（failed），finalizeTeamRun 的 success
// 路径 token 写回不执行——run.token_in 恒 0，审计/前端看到的终止 run 像
// "零消耗"。跳闸观测值 used 是 run 累计 input 的最佳可得口径（与决策记录
// observed_value 同源），读-改-写回填让终态记录反映中止时真实消耗：
// 读 fresh 行避免全行覆盖冲掉 graph_execution_id/trace_id 等途中写字段；
// finishRunErr 的 TransitionRunStatus 重读 DB 行且不触碰 TokenIn，回填
// 先于终态转换即存活。WithoutCancel：本序列执行后 run ctx 即被取消。
func (r *Runner) backfillTrippedRunTokenIn(ctx context.Context, run biz.TeamRunRecord, used int64) {
	if r.runReader == nil || r.runWriter == nil || used <= 0 || run.ID == "" {
		return
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	fresh, err := r.runReader.GetTeamRunByID(wctx, run.ID)
	if err != nil {
		r.lg.Warn("预算中止 run token_in 回填：读取失败",
			loggateway.StepID("team.token_budget.backfill"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	if fresh.TokenIn > 0 {
		return // 已被并发路径写入（如 success 边界竞态），不覆盖。
	}
	fresh.TokenIn = int(used)
	if err := r.runWriter.UpdateTeamRun(wctx, fresh); err != nil {
		r.lg.Warn("预算中止 run token_in 回填：写库失败",
			loggateway.StepID("team.token_budget.backfill"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
	}
}

// recordAuxUsage records auxiliary LLM usage for team-side旁路 calls (intent pass).
// Zero-token rows are skipped (skipped/failed calls consumed nothing observable).
func (r *Runner) recordAuxUsage(ctx context.Context, in biz.AuxLLMUsageInput) {
	if r == nil || r.usage == nil {
		return
	}
	if in.PromptTok <= 0 && in.CompletionTok <= 0 {
		return
	}
	if err := r.usage.RecordAuxLLMUsage(ctx, in); err != nil {
		r.lg.Warn("团队旁路用量落库失败",
			loggateway.StepID("team.usage_record_fail"),
			loggateway.Err(err),
			loggateway.Str("team_id", in.TeamID),
			loggateway.Str("usage_kind", in.Kind),
		)
	}
}

// recordTeamRunUsage writes one aggregated team turn row (workflow-level tokens).
func (r *Runner) recordTeamRunUsage(
	ctx context.Context,
	run biz.TeamRunRecord,
	teamID string,
	anchor biz.Agent,
	promptTok, completionTok, cachedTok int,
	prov, mod, dialogMode string,
	usageSource string,
) {
	if (r == nil || r.usage == nil) || (promptTok <= 0 && completionTok <= 0) {
		return
	}
	now := time.Now().UTC()
	meta := "{}"
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		meta = em.MetadataJSON()
	} else {
		b, _ := json.Marshal(map[string]any{
			"source": "team_run",
			"run_id": run.ID,
		})
		meta = string(b)
	}
	meta = biz.MergeUsageSourceMetadata(meta, usageSource)
	ev := biz.TokenUsageEvent{
		ID:                uuid.NewString(),
		TeamID:            teamID,
		SessionID:         run.SessionID,
		AgentID:           anchor.ID,
		AgentKey:          anchor.AgentKey,
		MessageID:         run.ID,
		ProviderCode:      prov,
		ModelAPIID:        mod,
		ModelDisplayName:  mod,
		InputTokens:       promptTok,
		OutputTokens:      completionTok,
		CachedInputTokens: cachedTok,
		TotalTokens:       promptTok + completionTok,
		UsageKind:         biz.UsageKindTeamTurn,
		PromptMode:        dialogMode,
		Status:            biz.TokenUsageStatusSuccess,
		MetadataJSON:      meta,
		OccurredAt:        now.Format(time.RFC3339),
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	recEv, err := r.usage.RecordTokenUsageEvent(recCtx, ev)
	if err != nil {
		r.lg.Warn("团队轮次用量落库失败",
			loggateway.StepID("team.usage_record_fail"),
			loggateway.Err(err),
			loggateway.Str("team_id", teamID),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("usage_kind", biz.UsageKindTeamTurn),
		)
		return
	}
	if r.td.Sessions != nil && strings.TrimSpace(run.SessionID) != "" {
		r.td.Sessions.AccumulateMetricsDelta(session.SessionMetricsDelta{
			SessionID:         run.SessionID,
			ModelCallCount:    recEv.CallCount,
			InputTokens:       int64(recEv.InputTokens),
			OutputTokens:      int64(recEv.OutputTokens),
			TotalTokens:       int64(recEv.TotalTokens),
			TotalCostMicroUsd: recEv.TotalCostMicroUSD,
		})
	}
	// Envelope publishing is handled inside RecordTokenUsageEvent (P1-4) —
	// publishing here as well would double-emit.
}
