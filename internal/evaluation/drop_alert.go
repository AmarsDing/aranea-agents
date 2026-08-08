package evaluation

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/loggateway"
)

// AgentEvalConfigReader reads the per-agent evaluation auto-config (narrow
// port). Implemented at the service layer over the agent config_json.
// Stability: evolving
type AgentEvalConfigReader interface {
	EvalAutoConfigForAgent(ctx context.Context, agentID string) (biz.AgentEvalAutoConfig, error)
}

// ScoreDropAlerter detects consecutive score drops across online (after_turn)
// runs and publishes one SystemNoticeEvent when the configured streak is
// reached (P2-2 online quality watch).
type ScoreDropAlerter struct {
	uc      *beval.Usecase
	configs AgentEvalConfigReader
	bus     biz.EventBus
	lg      loggateway.Logger
}

// NewScoreDropAlerter constructs a ScoreDropAlerter. A nil bus disables
// publishing (detection still runs and logs).
func NewScoreDropAlerter(uc *beval.Usecase, configs AgentEvalConfigReader, bus biz.EventBus, lg loggateway.Logger) *ScoreDropAlerter {
	return &ScoreDropAlerter{uc: uc, configs: configs, bus: bus, lg: lg}
}

// CheckAfterRun evaluates the drop condition after one run completed. It only
// reacts to completed after_turn runs whose agent config enables alerting.
// All failures are logged and swallowed: alerting must never fail the run.
func (a *ScoreDropAlerter) CheckAfterRun(ctx context.Context, run biz.EvalRun) {
	if a == nil || a.uc == nil || a.configs == nil {
		return
	}
	if run.TriggerSource != triggerAfterTurn || run.Status != "completed" {
		return
	}
	cfg, err := a.configs.EvalAutoConfigForAgent(ctx, run.AgentID)
	if err != nil {
		a.lg.Warn("eval drop alert: load agent config failed",
			loggateway.StepID("evaluation.drop_alert.config_fail"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("agent_id", run.AgentID),
			loggateway.Err(err))
		return
	}
	n := cfg.AlertConsecutiveDrops
	if n < 2 || cfg.DatasetID == "" || cfg.DatasetID != run.DatasetID {
		return
	}
	points, err := a.uc.GetAgentEvalTrend(ctx, run.AgentID, run.DatasetID, n*2)
	if err != nil {
		a.lg.Warn("eval drop alert: load trend failed",
			loggateway.StepID("evaluation.drop_alert.trend_fail"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("agent_id", run.AgentID),
			loggateway.Err(err))
		return
	}
	online := make([]beval.TrendPoint, 0, n)
	for _, p := range points {
		if p.TriggerSource == triggerAfterTurn {
			online = append(online, p)
		}
		if len(online) == n {
			break
		}
	}
	if len(online) < n {
		return
	}
	scores := make([]float32, n)
	for i, p := range online {
		scores[i] = trendMetricValue(p, cfg.AlertMetric)
	}
	// points are newest-first: a drop streak means every newer score is
	// strictly lower than the previous one.
	for i := 0; i+1 < n; i++ {
		if scores[i] >= scores[i+1] {
			return
		}
	}
	msg := fmt.Sprintf("在线评估 %s 连续 %d 次下跌：%s（最新 %.2f）",
		cfg.AlertMetric, n, formatScoreSeries(scores), scores[0])
	a.lg.Warn("eval online score consecutive drop",
		loggateway.StepID("evaluation.drop_alert.fire"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("agent_id", run.AgentID),
		loggateway.Str("dataset_id", run.DatasetID),
		loggateway.Str("metric", cfg.AlertMetric),
		loggateway.Str("message", msg))
	if a.bus == nil {
		return
	}
	meta := map[string]any{
		"event_type": "eval_score_drop",
		"agent_id":   run.AgentID,
		"dataset_id": run.DatasetID,
		"metric":     cfg.AlertMetric,
		"drops":      n,
		"scores":     scores,
		"run_id":     run.ID,
	}
	// Fire-and-forget on a detached context, same as other notice publishers.
	a.bus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "eval_score_drop", msg, meta))
}

// trendMetricValue extracts the watched metric from one trend point. Unknown
// metric names fall back to the LLM judge score.
func trendMetricValue(p beval.TrendPoint, metric string) float32 {
	switch metric {
	case "exact_match":
		return p.ExactMatchScore
	case "contains_match":
		return p.ContainsMatchScore
	case "tool_call_accuracy":
		return p.ToolCallAccuracy
	default:
		return p.LLMJudgeScore
	}
}

// formatScoreSeries renders newest-first scores as "0.40 ← 0.55 ← 0.80".
func formatScoreSeries(scores []float32) string {
	parts := make([]string, len(scores))
	for i, s := range scores {
		parts[i] = fmt.Sprintf("%.2f", s)
	}
	return strings.Join(parts, " ← ")
}
