package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// G3（ADR-G，2026-08-14）：runner 侧交付物质量门集成
//
// 位置：finalizeTeamRun 中二元门（HasRealDeliverable）之后、success 转换之前。
// fail-open 哲学：预算耗尽 / judge infra error / 未装配修订通道 → 放行 + warn，
// 不得把今天二元门会放行的交付物卡死（防回归）。修订经 P2-3 followup 路基
// 入队：当前 turn 结束后反馈作为新 turn 输入，团队成员携历史上下文修订。
// ---------------------------------------------------------------------------

// maxQualityRevisions 是 team+session 维度的质量门修订预算（防循环，ADR-G）。
const maxQualityRevisions = 2

// SetQualityGate wires the deliverable quality gate consulted after the binary
// gate passes for DAG teams (biz.SpiritTeamUsecase.EvaluateDeliverableQuality).
func (r *Runner) SetQualityGate(fn biz.TeamQualityGateFunc) {
	if r == nil {
		return
	}
	r.qualityGate = fn
}

// SetRevisionEnqueuer wires the revision followup channel (P2-3 roadbed:
// ChatEnqueueKindFollowup → pending 队列，当前 turn 结束后作为新 turn 输入）。
func (r *Runner) SetRevisionEnqueuer(fn biz.TeamRevisionEnqueuerFunc) {
	if r == nil {
		return
	}
	r.revisionEnqueuer = fn
}

// qualityRevisionCount 返回 team+session 维度已用修订次数（测试/观测用）。
func (r *Runner) qualityRevisionCount(teamID, sessionID string) int {
	r.qualityReviseMu.Lock()
	defer r.qualityReviseMu.Unlock()
	return r.qualityReviseCount[qualityReviseKey(teamID, sessionID)]
}

// seedQualityRevisionCount 预置修订计数（测试用）。
func (r *Runner) seedQualityRevisionCount(teamID, sessionID string, n int) {
	r.qualityReviseMu.Lock()
	defer r.qualityReviseMu.Unlock()
	if r.qualityReviseCount == nil {
		r.qualityReviseCount = map[string]int{}
	}
	r.qualityReviseCount[qualityReviseKey(teamID, sessionID)] = n
}

func qualityReviseKey(teamID, sessionID string) string { return teamID + "|" + sessionID }

// qualityGateBlocks 在 success 转换前评估质量门。返回 true 表示 run 已被
// 拦截（内部完成 finishRunErr + 修订 followup 入队），调用方直接返回。
func (r *Runner) qualityGateBlocks(
	ctx context.Context,
	sess biz.Session,
	teamRow biz.Team,
	run *biz.TeamRunRecord,
	t0 time.Time,
	teamEmitter *event.TraceEmitter,
) bool {
	if r.qualityGate == nil {
		return false
	}
	res, err := r.qualityGate(ctx, teamRow)
	if err != nil {
		// fail-open：判分 infra 错误不得卡死交付（K3 降级双轨日志）。
		r.lg.Warn("交付物质量门判分失败（infra），fail-open 放行",
			loggateway.StepID("team.quality_gate.judge_error"),
			loggateway.Str("team_id", teamRow.ID),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
		if teamEmitter != nil {
			teamEmitter.LogWarn("team.quality_gate.bypass", "质量门判分异常，放行",
				err.Error(), event.P("team_id", teamRow.ID), event.P("run_id", run.ID), event.P("reason", "judge_error"))
		}
		return false
	}
	if res.Verdict == biz.TeamQualityPass || res.Verdict == "" {
		r.resetQualityRevisionCount(teamRow.ID, sess.ID)
		return false
	}

	// revise/fail：无修订通道 → fail-open（legacy/测试路径不得卡死）。
	if r.revisionEnqueuer == nil {
		r.lg.Warn("交付物质量门打回但未装配修订通道，fail-open 放行",
			loggateway.StepID("team.quality_gate.no_enqueuer"),
			loggateway.Str("team_id", teamRow.ID),
			loggateway.Str("run_id", run.ID))
		if teamEmitter != nil {
			teamEmitter.LogWarn("team.quality_gate.bypass", "质量门打回但无修订通道，放行",
				res.Feedback, event.P("team_id", teamRow.ID), event.P("run_id", run.ID), event.P("reason", "no_enqueuer"))
		}
		return false
	}

	key := qualityReviseKey(teamRow.ID, sess.ID)
	r.qualityReviseMu.Lock()
	used := r.qualityReviseCount[key]
	if used >= maxQualityRevisions {
		delete(r.qualityReviseCount, key)
	}
	r.qualityReviseMu.Unlock()
	if used >= maxQualityRevisions {
		// 预算耗尽 → fail-open（K3：今天二元门会放行的交付物不被质量门卡死）。
		r.lg.Warn("交付物质量门修订预算耗尽，fail-open 放行",
			loggateway.StepID("team.quality_gate.budget_exhausted"),
			loggateway.Str("team_id", teamRow.ID),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("feedback", res.Feedback))
		if teamEmitter != nil {
			teamEmitter.LogWarn("team.quality_gate.bypass", "质量门修订预算耗尽，放行",
				res.Feedback, event.P("team_id", teamRow.ID), event.P("run_id", run.ID), event.P("reason", "budget_exhausted"))
		}
		return false
	}

	feedback := fmt.Sprintf("[交付物质量门] 第 %d 次修订打回：%s\n请分析上述问题，修正后重新提交交付物（set_deliverable）。",
		used+1, res.Feedback)
	if eerr := r.revisionEnqueuer(ctx, sess.ID, feedback); eerr != nil {
		// 入队失败 = 修订不会发生 → fail-open（不得出现「打回了却没人修」的假失败）。
		r.lg.Warn("交付物质量门修订 followup 入队失败，fail-open 放行",
			loggateway.StepID("team.quality_gate.enqueue_fail"),
			loggateway.Str("team_id", teamRow.ID),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(eerr))
		if teamEmitter != nil {
			teamEmitter.LogWarn("team.quality_gate.bypass", "质量门修订入队失败，放行",
				eerr.Error(), event.P("team_id", teamRow.ID), event.P("run_id", run.ID), event.P("reason", "enqueue_fail"))
		}
		return false
	}

	r.qualityReviseMu.Lock()
	if r.qualityReviseCount == nil {
		r.qualityReviseCount = map[string]int{}
	}
	r.qualityReviseCount[key] = used + 1
	r.qualityReviseMu.Unlock()

	if teamEmitter != nil {
		teamEmitter.LogWarn("team.quality_gate.revise", "交付物质量门打回修订",
			res.Feedback,
			event.P("team_id", teamRow.ID),
			event.P("run_id", run.ID),
			event.P("verdict", string(res.Verdict)),
			event.P("revision", used+1),
			event.P("rule_hits", strings.Join(res.RuleHits, " | ")))
	}
	r.finishRunErr(ctx, run, t0, fmt.Sprintf("交付物质量门打回（第 %d 次修订）：%s", used+1, res.Feedback))
	return true
}

func (r *Runner) resetQualityRevisionCount(teamID, sessionID string) {
	r.qualityReviseMu.Lock()
	defer r.qualityReviseMu.Unlock()
	delete(r.qualityReviseCount, qualityReviseKey(teamID, sessionID))
}
