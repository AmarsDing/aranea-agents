package biz

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"fmt"
	"strings"
	"time"
)

// govern runs the Critic (G4, degradable) + Governor (RiskClassifier) stages
// and persists the governance decision. The run stays awaiting_governance for
// the router (T3.5) unless the decision is reject.
func (uc *SelfImprovementPipelineUsecase) govern(
	ctx context.Context,
	run *SelfImprovementRun,
	patch *PatcherOutput,
	transition func(SelfImprovementRunEvent) error,
	rejectWith func(string) error,
	emitStage func(stage string, attempt int, status ActivityStatus, summary string),
	cursor *siStageCursor,
) error {
	if err := transition(RunEventVerifyPass); err != nil {
		return err
	}
	cursor.stage, cursor.attempt = SIStageGoverning, 0
	emitStage(SIStageGoverning, 0, ActivityStatusRunning, "")
	if uc.critic != nil {
		report, cerr := uc.critic.Review(ctx, run, patch)
		if cerr != nil {

			uc.lg.Warn("self-improve pipeline: critic degraded",
				loggateway.StepID("si_pipeline.critic"),
				loggateway.Str("run_id", run.ID), loggateway.Err(cerr))
		} else {
			run.CriticReport = report
		}
	}
	decision := uc.classifier.Classify(*patch, run.CriticReport)
	run.Governance = &decision
	run.RiskLevel = decision.RiskLevel
	if decision.Channel == "reject" {
		return rejectWith("governance reject: " + strings.Join(decision.RuleHits, ","))
	}
	emitStage(SIStageGoverning, 0, ActivityStatusCompleted, "")
	cursor.stage = ""

	run.UpdatedAt = time.Now().UTC()
	return uc.runWriter.Update(ctx, run, RunStatusAwaitingGovernance)
}

// siFailClosedWebLint treats a skipped frontend lint as a verify failure when
// the patch is code/test and only touches web/ (no Go gates ran).
func siFailClosedWebLint(plan SIVerifyPlan, res SandboxGateResult) (bool, SandboxGateResult) {
	if res.Gate != SandboxGateWebLint || !res.Skipped {
		return false, res
	}
	if plan.Kind != PatchKindCode && plan.Kind != PatchKindTest {
		return false, res
	}
	if len(plan.GoPkgs) > 0 {
		return false, res
	}
	res.Skipped = false
	res.Passed = false
	if strings.TrimSpace(res.Output) == "" {
		res.Output = "fail-closed: web-only code patch requires frontend lint"
	} else if !strings.Contains(res.Output, "fail-closed") {
		res.Output = res.Output + "\nfail-closed: web-only code patch requires frontend lint"
	}
	return true, res
}

// siFailedGateDigest condenses failed-gate output into a Patcher retry hint.
func siFailedGateDigest(report []SandboxGateResult) string {
	const maxOut = 500
	var b strings.Builder
	for _, r := range report {
		if r.Skipped || r.Passed {
			continue
		}
		out := r.Output
		if len(out) > maxOut {
			out = out[:maxOut] + "…"
		}
		fmt.Fprintf(&b, "gate %s failed: %s\n", r.Gate, out)
	}
	return strings.TrimSpace(b.String())
}
