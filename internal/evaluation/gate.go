package evaluation

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// P2-1: publish regression gate.
//
// Y2 (2026-08-14): the gate is an ASYNC ADVISORY. The old synchronous design
// ran a full dataset evaluation inline with the publish request: the HTTP
// call hung for minutes, and a client/gateway timeout cancelled the eval —
// the aborted run then blocked the publish with a spurious "执行失败". Now
// Check only performs the fast synchronous decisions (config enabled,
// baseline existence) and launches the regression run in the background;
// threshold breaches surface as eval_gate_blocked notifications instead of
// blocking errors. The one remaining hard block is "no baseline" (Y12): when
// max_drop is configured but no completed baseline run exists, the publish is
// rejected and a gate run is launched to establish the baseline.

// triggerGate is the trigger_source recorded on gate runs so trend panels can
// split them out of manual/after_turn series.
const triggerGate = "gate"

// gateBaselineScan bounds how many recent runs are scanned for the latest
// completed baseline.
const gateBaselineScan = 20

// PublishGate guards skill publish / pack install with a regression
// evaluation. The gate is a singleton config; when disabled Check is a no-op.
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
// beval.GateTriggerSkillPublish / beval.GateTriggerPackInstall.
//
// Returns an apierror.Conflict only when max_drop is configured but no
// completed baseline run exists (Y12) — the caller must establish a baseline
// first; a gate run is launched in the background so a retry after its
// completion finds one. All threshold outcomes of the regression run itself
// are advisory (notification only) and never fail the publish (Y2).
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

	runs, _, hasBaseline, err := g.scanRuns(ctx, cfg, "")
	if err != nil {
		// Baseline query failed: fail open (cannot distinguish "no runs" from
		// a storage error; blocking here would be a false positive).
		g.lg.Warn("eval gate: load baseline failed, allowing",
			loggateway.StepID("evaluation.gate.baseline_fail"),
			loggateway.Err(err))
		return nil
	}

	// In-flight dedup: one gate run already covers the current code state;
	// a publish burst must not fan out N full-dataset evaluations (LLM cost).
	for _, r := range runs {
		if r.TriggerSource == triggerGate && (r.Status == "pending" || r.Status == "running") {
			g.lg.Info("eval gate: run already in flight, skipping duplicate",
				loggateway.StepID("evaluation.gate.dedup"),
				loggateway.Str("trigger", trigger),
				loggateway.Str("run_id", r.ID))
			return nil
		}
	}

	run, err := g.launch(ctx, cfg)
	if err != nil {
		return err
	}
	g.lg.Info("eval gate: regression run launched",
		loggateway.StepID("evaluation.gate.launch"),
		loggateway.Str("trigger", trigger),
		loggateway.Str("run_id", run.ID))

	// Y12: max_drop without any completed baseline is a hard block — the drop
	// check would otherwise be silently skipped. The gate run launched above
	// measures the pre-publish state and becomes the baseline for the retry.
	if cfg.MaxDrop > 0 && !hasBaseline {
		return g.block(ctx, trigger, cfg,
			"无可用基线：已启动基线评估，待其完成后重试发布（或先手动运行一次评估）", nil)
	}
	return nil
}

// launch creates the gate run and starts the background evaluation. The
// evaluation runs on a detached context: the publish request's cancellation
// (client timeout/disconnect) must not abort it.
func (g *PublishGate) launch(ctx context.Context, cfg beval.GateConfig) (beval.Run, error) {
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
		return beval.Run{}, err
	}
	execCtx := context.WithoutCancel(ctx)
	safego.Go(appctx.Ctx(), "eval-gate-run", func() {
		g.evaluate(execCtx, cfg, run.ID)
	})
	return run, nil
}

// evaluate executes the gate run and applies the advisory thresholds. Called
// asynchronously from launch; extracted for deterministic tests.
func (g *PublishGate) evaluate(ctx context.Context, cfg beval.GateConfig, runID string) {
	run, err := g.uc.GetRun(ctx, runID)
	if err != nil {
		g.lg.Warn("eval gate: reload run failed",
			loggateway.StepID("evaluation.gate.run_load_fail"),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
		return
	}
	// Baseline is re-fetched after the run completes: runs launched between
	// Check and now (including this gate run itself, which is terminal by
	// now) must not become their own baseline — scanRuns filters them out by
	// creation time (notAfter = this run's created_at) plus an explicit ID
	// exclusion for same-second ties.
	_, baseline, hasBaseline, err := g.scanRuns(ctx, cfg, run.CreatedAt, run.ID)
	if err != nil {
		g.lg.Warn("eval gate: reload baseline failed",
			loggateway.StepID("evaluation.gate.baseline_fail"),
			loggateway.Err(err))
	}
	final, runErr := g.runner.RunSync(ctx, run, cfg.Metric, 1)
	if runErr != nil {
		g.notifyBreach(ctx, cfg, final.ID, fmt.Sprintf("评估回归执行失败：%s", final.ErrorMessage), runErr)
		return
	}
	score, ok := beval.RunMetricScore(final, cfg.Metric)
	if !ok {
		g.notifyBreach(ctx, cfg, final.ID, fmt.Sprintf("评估回归缺少指标 %s 得分", cfg.Metric), nil)
		return
	}
	if cfg.MinScore > 0 && score < cfg.MinScore {
		g.notifyBreach(ctx, cfg, final.ID,
			fmt.Sprintf("评估回归得分 %.2f 低于下限 %.2f", score, cfg.MinScore), nil)
		return
	}
	if cfg.MaxDrop > 0 && hasBaseline && score < baseline-cfg.MaxDrop {
		g.notifyBreach(ctx, cfg, final.ID,
			fmt.Sprintf("评估回归得分 %.2f 较基线 %.2f 下跌超过 %.2f", score, baseline, cfg.MaxDrop), nil)
		return
	}
	g.lg.Info("eval gate passed",
		loggateway.StepID("evaluation.gate.pass"),
		loggateway.Str("run_id", final.ID),
		loggateway.Str("metric", cfg.Metric),
		loggateway.Float64("score", float64(score)))
}

// scanRuns lists recent runs for the gated agent+dataset and extracts the
// newest completed run's metric score as the baseline. Manual/after_turn/gate
// runs all qualify — the baseline is simply "the last known good quality
// point". notAfter excludes runs created after the given RFC3339 timestamp
// (a run younger than the gate run measures newer code and can never be its
// baseline); excludeID additionally skips specific runs (same-second ties,
// the just-finished gate run itself).
func (g *PublishGate) scanRuns(ctx context.Context, cfg beval.GateConfig, notAfter string, excludeID ...string) (runs []beval.Run, baseline float32, hasBaseline bool, err error) {
	runs, _, err = g.uc.ListRuns(ctx, cfg.DatasetID, cfg.AgentID, gateBaselineScan, 0)
	if err != nil {
		return nil, 0, false, err
	}
	excluded := map[string]bool{}
	for _, id := range excludeID {
		excluded[id] = true
	}
	for _, r := range runs {
		if r.Status != "completed" || excluded[r.ID] {
			continue
		}
		// RFC3339 UTC timestamps compare lexicographically.
		if notAfter != "" && r.CreatedAt > notAfter {
			continue
		}
		if score, ok := beval.RunMetricScore(r, cfg.Metric); ok {
			return runs, score, true, nil
		}
	}
	return runs, 0, false, nil
}

// block publishes the blocked notification and returns the Conflict error.
// Used by the one remaining synchronous gate decision (no baseline, Y12).
func (g *PublishGate) block(ctx context.Context, trigger string, cfg beval.GateConfig, reason string, cause error) error {
	msg := fmt.Sprintf("发布门禁拦截（%s）：%s", trigger, reason)
	g.emitGateEvent(ctx, trigger, cfg, "", reason, cause, msg)
	return apierror.Conflict("EVAL", msg)
}

// notifyBreach publishes the advisory breach notification (Y2): the publish
// has already proceeded; admins are alerted to the regression after the fact.
func (g *PublishGate) notifyBreach(ctx context.Context, cfg beval.GateConfig, runID, reason string, cause error) {
	msg := fmt.Sprintf("发布门禁告警（已放行）：%s", reason)
	g.emitGateEvent(ctx, "", cfg, runID, reason, cause, msg)
}

// emitGateEvent logs and publishes the eval_gate_blocked system notice. The
// event type is kept for both blocking and advisory outcomes so existing
// consumers see one stream; the message text distinguishes them.
func (g *PublishGate) emitGateEvent(_ context.Context, trigger string, cfg beval.GateConfig, runID, reason string, cause error, msg string) {
	fields := []loggateway.Field{
		loggateway.StepID("evaluation.gate.block"),
		loggateway.Str("trigger", trigger),
		loggateway.Str("agent_id", cfg.AgentID),
		loggateway.Str("dataset_id", cfg.DatasetID),
		loggateway.Str("metric", cfg.Metric),
		loggateway.Str("run_id", runID),
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
			"run_id":     runID,
			"reason":     reason,
		}
		// Fire-and-forget on a detached context, same as other notice publishers.
		g.bus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "eval_gate_blocked", msg, meta))
	}
}
