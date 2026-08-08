package evaluation

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// P2-1: publish regression gate.

// triggerGate is the trigger_source recorded on gate runs so trend panels can
// split them out of manual/after_turn series.
const triggerGate = "gate"

// gateBaselineScan bounds how many recent runs are scanned for the latest
// completed baseline.
const gateBaselineScan = 20

// PublishGate blocks skill publish / pack install when the configured
// regression evaluation falls below the floor or drops too far vs the latest
// completed baseline. The gate is a singleton config; when disabled Check is
// a no-op.
type PublishGate struct {
	uc     *beval.Usecase
	runner *Runner
	bus    biz.EventBus
	lg     loggateway.Logger
}

// NewPublishGate constructs a PublishGate. Nil runner disables the gate
// (Check always allows).
func NewPublishGate(uc *beval.Usecase, runner *Runner, bus biz.EventBus, lg loggateway.Logger) *PublishGate {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &PublishGate{uc: uc, runner: runner, bus: bus, lg: lg}
}

// Check runs the gate for one publish operation. trigger is
// beval.GateTriggerSkillPublish / beval.GateTriggerPackInstall. Returns an
// apierror.Conflict when the regression run fails or breaches the configured
// thresholds; nil when the gate is disabled or the run passes.
func (g *PublishGate) Check(ctx context.Context, trigger string) error {
	if g == nil || g.uc == nil || g.runner == nil {
		return nil
	}
	cfg, err := g.uc.GetGateConfig(ctx)
	if err != nil {
		// Config unreadable: fail open with a warning — a storage hiccup must
		// not block every publish in the system.
		g.lg.Warn("eval gate: load config failed, allowing",
			loggateway.StepID("evaluation.gate.config_fail"),
			loggateway.Err(err))
		return nil
	}
	if !cfg.Enabled {
		return nil
	}

	// Baseline is fetched before the gate run is created so the gate run
	// itself never becomes its own baseline.
	baseline, hasBaseline := g.latestBaseline(ctx, cfg)

	in := biz.EvalRun{
		DatasetID:     cfg.DatasetID,
		AgentID:       cfg.AgentID,
		TriggerSource: triggerGate,
	}
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
	}
	run, err := g.uc.CreateRun(ctx, in)
	if err != nil {
		return err
	}
	final, runErr := g.runner.RunSync(ctx, run, cfg.Metric, 1)
	if runErr != nil {
		return g.block(ctx, trigger, cfg, fmt.Sprintf("评估回归执行失败：%s", final.ErrorMessage), runErr)
	}
	score, ok := beval.RunMetricScore(final, cfg.Metric)
	if !ok {
		return g.block(ctx, trigger, cfg, fmt.Sprintf("评估回归缺少指标 %s 得分", cfg.Metric), nil)
	}
	if cfg.MinScore > 0 && score < cfg.MinScore {
		return g.block(ctx, trigger, cfg,
			fmt.Sprintf("评估回归得分 %.2f 低于下限 %.2f", score, cfg.MinScore), nil)
	}
	if cfg.MaxDrop > 0 && hasBaseline && score < baseline-cfg.MaxDrop {
		return g.block(ctx, trigger, cfg,
			fmt.Sprintf("评估回归得分 %.2f 较基线 %.2f 下跌超过 %.2f", score, baseline, cfg.MaxDrop), nil)
	}
	g.lg.Info("eval gate passed",
		loggateway.StepID("evaluation.gate.pass"),
		loggateway.Str("trigger", trigger),
		loggateway.Str("run_id", final.ID),
		loggateway.Str("metric", cfg.Metric),
		loggateway.Float64("score", float64(score)))
	return nil
}

// latestBaseline returns the newest completed run's metric score for the
// gated agent+dataset. Manual/after_turn/gate runs all qualify — the baseline
// is simply "the last known good quality point".
func (g *PublishGate) latestBaseline(ctx context.Context, cfg beval.GateConfig) (float32, bool) {
	runs, _, err := g.uc.ListRuns(ctx, cfg.DatasetID, cfg.AgentID, gateBaselineScan, 0)
	if err != nil {
		g.lg.Warn("eval gate: load baseline failed",
			loggateway.StepID("evaluation.gate.baseline_fail"),
			loggateway.Err(err))
		return 0, false
	}
	for _, r := range runs {
		if r.Status != "completed" {
			continue
		}
		if score, ok := beval.RunMetricScore(r, cfg.Metric); ok {
			return score, true
		}
	}
	return 0, false
}

// block publishes the blocked notification and returns the Conflict error.
func (g *PublishGate) block(ctx context.Context, trigger string, cfg beval.GateConfig, reason string, cause error) error {
	msg := fmt.Sprintf("发布门禁拦截（%s）：%s", trigger, reason)
	fields := []loggateway.Field{
		loggateway.StepID("evaluation.gate.block"),
		loggateway.Str("trigger", trigger),
		loggateway.Str("agent_id", cfg.AgentID),
		loggateway.Str("dataset_id", cfg.DatasetID),
		loggateway.Str("metric", cfg.Metric),
	}
	if cause != nil {
		fields = append(fields, loggateway.Err(cause))
	}
	g.lg.Warn(msg, fields...)
	if g.bus != nil {
		meta := map[string]any{
			"event_type": "eval_gate_blocked",
			"trigger":    trigger,
			"agent_id":   cfg.AgentID,
			"dataset_id": cfg.DatasetID,
			"metric":     cfg.Metric,
			"reason":     reason,
		}
		// Fire-and-forget on a detached context, same as other notice publishers.
		g.bus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "eval_gate_blocked", msg, meta))
	}
	return apierror.Conflict("EVAL", msg)
}
