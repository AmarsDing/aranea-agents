package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Meta Team pipeline orchestration (73-self-iteration-v3, design D5) ──────
//
// SelfImprovementPipelineUsecase drives one detected run through the Meta
// Team stages: Analyst(LLM) → Patcher(LLM) → Verifier(沙盒 Gate) → Critic(LLM)
// → Governor(RiskClassifier 纯代码)。Observer 由 SelfImprovementObserveUsecase
// 前置完成（信号→建议→run）；Applier 属 Phase 4。
//
// 重试回路：Verifier 失败 → 带 Gate 输出回 Patcher（attempts 上限
// MaxAttempts，默认 3，design D5/D10）。
//
// Fail-fast 策略门禁（Verify 前，design D9/D10/SEL-08）：保护文件、敏感内容、
// 超规模 diff 一律 reject，不消耗沙盒 Gate。

// SIPatchRequest is one Patcher invocation (attempt-scoped).
type SIPatchRequest struct {
	Run          *SelfImprovementRun
	Diagnosis    *Diagnosis
	WorktreePath string
	RetryHint    string // 上一次失败的 Gate 输出摘要（首次为空）
	Attempt      int    // 1-based
}

// SIAnalystStage is the Analyst LLM stage (D5).
// Stability:evolving
type SIAnalystStage interface {
	Analyze(ctx context.Context, run *SelfImprovementRun, sug *UnifiedEvolutionSuggestion) (*Diagnosis, error)
}

// SIPatcherStage is the Patcher LLM stage (D5).
// Stability:evolving
type SIPatcherStage interface {
	Patch(ctx context.Context, req SIPatchRequest) (*PatcherOutput, error)
}

// SICriticStage is the Critic LLM stage (G4, D5).
// Stability:evolving
type SICriticStage interface {
	Review(ctx context.Context, run *SelfImprovementRun, patch *PatcherOutput) (*CriticReport, error)
}

// SelfImprovementPipelineDeps carries the pipeline's injected dependencies.
type SelfImprovementPipelineDeps struct {
	Analyst      SIAnalystStage
	Patcher      SIPatcherStage
	Critic       SICriticStage // nil → G4 降级跳过（如配额耗尽）
	Sandbox      RepoSandbox
	Suggestions  UnifiedEvolutionQueryReader
	RunReader    SelfImprovementRunReader
	RunWriter    SelfImprovementRunWriter
	Classifier   *SIRiskClassifier // nil → 默认规则
	ActivitySink SIActivitySink    // nil → 不挂载过程活动（T3.6）
	Control      *SIControlPlane   // nil → 无用户介入（T3.6）
	MaxAttempts  int               // ≤0 → 3
	MaxDiffLines int               // ≤0 → DefaultMaxDiffLines
	Lg           loggateway.Logger
}

// SelfImprovementPipelineUsecase orchestrates the Meta Team for one run.
type SelfImprovementPipelineUsecase struct {
	analyst      SIAnalystStage
	patcher      SIPatcherStage
	critic       SICriticStage
	sandbox      RepoSandbox
	suggestions  UnifiedEvolutionQueryReader
	runReader    SelfImprovementRunReader
	runWriter    SelfImprovementRunWriter
	classifier   *SIRiskClassifier
	activitySink SIActivitySink
	control      *SIControlPlane
	maxAttempts  int
	maxDiffLines int
	lg           loggateway.Logger
}

// siStageCursor tracks the currently-open stage node so fail/reject paths can
// close it with a failed emission instead of leaving a dangling running node.
type siStageCursor struct {
	stage   string
	attempt int
}

// NewSelfImprovementPipelineUsecase wires the pipeline usecase.
func NewSelfImprovementPipelineUsecase(deps SelfImprovementPipelineDeps) *SelfImprovementPipelineUsecase {
	maxAttempts := deps.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	maxDiffLines := deps.MaxDiffLines
	if maxDiffLines <= 0 {
		maxDiffLines = DefaultMaxDiffLines
	}
	classifier := deps.Classifier
	if classifier == nil {
		classifier = NewSIRiskClassifier()
	}
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SelfImprovementPipelineUsecase{
		analyst:      deps.Analyst,
		patcher:      deps.Patcher,
		critic:       deps.Critic,
		sandbox:      deps.Sandbox,
		suggestions:  deps.Suggestions,
		runReader:    deps.RunReader,
		runWriter:    deps.RunWriter,
		classifier:   classifier,
		activitySink: deps.ActivitySink,
		control:      deps.Control,
		maxAttempts:  maxAttempts,
		maxDiffLines: maxDiffLines,
		lg:           lg.With(loggateway.Domain("self_improve_pipeline")),
	}
}

// siConfidenceFloor is the Analyst confidence threshold (types.go: <0.5 降级为仅记录).
const siConfidenceFloor = 0.5

// Execute runs the pipeline for one detected run until a terminal or
// governance-waiting state. Stage errors fail the run (status=failed) and are
// returned; policy rejections and verify exhaustion are normal terminals
// (return nil).
func (uc *SelfImprovementPipelineUsecase) Execute(ctx context.Context, runID string) error {
	if uc == nil || uc.runReader == nil || uc.runWriter == nil {
		return apierror.Internal("SELF_IMPROVEMENT", "pipeline not initialized")
	}
	if uc.analyst == nil || uc.patcher == nil || uc.sandbox == nil {
		return apierror.Internal("SELF_IMPROVEMENT", "pipeline stages not wired (analyst/patcher/sandbox required)")
	}
	run, err := uc.runReader.GetByID(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return apierror.NotFound("SELF_IMPROVEMENT", "run %s not found", runID)
	}
	if run.Status != RunStatusDetected {
		return apierror.Conflict("SELF_IMPROVEMENT", "run %s not detected (current %s)", runID, run.Status)
	}

	// ── 过程活动挂载（T3.6，nil-safe） ─────────────────────────────────────
	cursor := &siStageCursor{}
	emit := func(a SIActivityRecord) {
		if uc.activitySink == nil {
			return
		}
		a.RunID = run.ID
		if serr := uc.activitySink.EmitSIActivity(ctx, a); serr != nil {
			uc.lg.Warn("self-improve activity emit degraded",
				loggateway.StepID("si_pipeline.activity"),
				loggateway.Str("run_id", run.ID), loggateway.Err(serr))
		}
	}
	emitStage := func(stage string, attempt int, status ActivityStatus, summary string) {
		emit(SIActivityRecord{
			ID:               SIStageActivityID(run.ID, stage, attempt),
			ParentActivityID: SIRunActivityID(run.ID),
			Stage:            stage,
			Attempt:          attempt,
			Status:           status,
			Summary:          summary,
		})
	}
	emit(SIActivityRecord{ID: SIRunActivityID(run.ID), Stage: SIStageRun, Status: ActivityStatusRunning})
	defer func() {
		var final ActivityStatus
		switch run.Status {
		case RunStatusAwaitingGovernance, RunStatusClosed:
			final = ActivityStatusCompleted
		case RunStatusRejected, RunStatusVerifyFailed, RunStatusFailed:
			final = ActivityStatusFailed
		default:
			return // 暂停/中途退出：根节点保持 running（恢复入口属 Phase 4）
		}
		emit(SIActivityRecord{ID: SIRunActivityID(run.ID), Stage: SIStageRun, Status: final, Summary: run.ClosedReason})
	}()

	// ── 用户介入（T3.6，nil-safe） ─────────────────────────────────────────
	pollControl := func() (SIControlCommand, bool) {
		if uc.control == nil {
			return "", false
		}
		return uc.control.Poll(run.ID)
	}
	skipRetry := false

	sm := NewSelfImprovementRunStateMachine()
	transition := func(event SelfImprovementRunEvent) error {
		from := run.Status
		to, terr := sm.Transition(from, event)
		if terr != nil {
			return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --%s--> ?: %s", from, event, terr)
		}
		run.Status = to
		run.UpdatedAt = time.Now().UTC()
		return uc.runWriter.Update(ctx, run, from)
	}
	failWith := func(cause error) error {
		if cursor.stage != "" {
			emitStage(cursor.stage, cursor.attempt, ActivityStatusFailed, cause.Error())
			cursor.stage = ""
		}
		run.ClosedReason = cause.Error()
		if terr := transition(RunEventError); terr != nil {
			uc.lg.Error("self-improve pipeline: fail transition broken",
				loggateway.StepID("si_pipeline.error"), loggateway.Err(terr))
		}
		return cause
	}
	rejectWith := func(reason string) error {
		if cursor.stage != "" {
			emitStage(cursor.stage, cursor.attempt, ActivityStatusFailed, reason)
			cursor.stage = ""
		}
		run.ClosedReason = reason
		uc.lg.Info("self-improve run rejected",
			loggateway.StepID("si_pipeline.reject"),
			loggateway.Str("run_id", run.ID), loggateway.Str("reason", reason))
		return transition(RunEventReject)
	}
	// handleControl applies one consumed user command; exit=true 时调用方应
	// 立即返回（pause/rollback）；skip_retry 仅置标志位继续。
	handleControl := func(cmd SIControlCommand) (exitErr error, exit bool) {
		switch cmd {
		case SIControlPause:
			uc.lg.Info("self-improve run paused by user",
				loggateway.StepID("si_pipeline.pause"), loggateway.Str("run_id", run.ID))
			return ErrSIRunPaused, true
		case SIControlRollback:
			return rejectWith("user_rollback: aborted before apply"), true
		case SIControlSkipRetry:
			skipRetry = true
			uc.lg.Info("self-improve run skip-retry by user",
				loggateway.StepID("si_pipeline.skip_retry"), loggateway.Str("run_id", run.ID))
		}
		return nil, false
	}

	// ── Diagnosing ──────────────────────────────────────────────────────────
	if err := transition(RunEventDiagnose); err != nil {
		return err
	}
	cursor.stage, cursor.attempt = SIStageDiagnosing, 0
	emitStage(SIStageDiagnosing, 0, ActivityStatusRunning, "")
	var sug *UnifiedEvolutionSuggestion
	if uc.suggestions != nil {
		if s, serr := uc.suggestions.GetByID(ctx, run.SuggestionID); serr == nil {
			sug = s
		}
	}
	diagnosis, err := uc.analyst.Analyze(ctx, run, sug)
	if err != nil {
		return failWith(fmt.Errorf("analyst: %w", err))
	}
	run.Diagnosis = diagnosis
	emitStage(SIStageDiagnosing, 0, ActivityStatusCompleted, "")
	cursor.stage = ""
	if diagnosis.Confidence < siConfidenceFloor {
		run.ClosedReason = fmt.Sprintf("record_only: confidence %.2f < %.2f", diagnosis.Confidence, siConfidenceFloor)
		uc.lg.Info("self-improve run record-only (low confidence)",
			loggateway.StepID("si_pipeline.record_only"), loggateway.Str("run_id", run.ID))
		return transition(RunEventRecordOnly)
	}

	// ── Patching ────────────────────────────────────────────────────────────
	if err := transition(RunEventPatch); err != nil {
		return err
	}
	worktree, cleanup, err := uc.sandbox.PrepareWorktree(ctx, run.ID, run.BaseRef)
	if err != nil {
		return failWith(fmt.Errorf("prepare worktree: %w", err))
	}
	if cleanup != nil {
		defer cleanup()
	}
	run.WorktreePath = worktree
	run.Branch = "self-improve/" + run.ID

	retryHint := ""
	for attempt := 1; ; attempt++ {
		// 用户介入（T3.6）：每次 Patcher 调用前消费控制指令。
		if cmd, ok := pollControl(); ok {
			if exitErr, exit := handleControl(cmd); exit {
				return exitErr
			}
		}
		cursor.stage, cursor.attempt = SIStagePatching, attempt
		emitStage(SIStagePatching, attempt, ActivityStatusRunning, "")
		patch, perr := uc.patcher.Patch(ctx, SIPatchRequest{
			Run: run, Diagnosis: diagnosis, WorktreePath: worktree, RetryHint: retryHint, Attempt: attempt,
		})
		if perr != nil {
			return failWith(fmt.Errorf("patcher: %w", perr))
		}

		// Fail-fast 策略门禁（Verify 前，不消耗沙盒 Gate）。
		if verr := ValidateDiffSize(patch.Diff, uc.maxDiffLines); verr != nil {
			return rejectWith("oversize_diff: " + verr.Error())
		}
		if kinds := DetectSensitiveContent(patch.Diff); len(kinds) > 0 {
			return rejectWith("sensitive_content: " + strings.Join(kinds, ","))
		}
		changes := ParseUnifiedDiffFiles(patch.Diff)
		if hits := CheckProtectedFiles(changes, DefaultProtectedFileRules()); len(hits) > 0 {
			return rejectWith(fmt.Sprintf("protected_file: %s (%s)", hits[0].Path, hits[0].Reason))
		}

		if aerr := uc.sandbox.ApplyDiff(ctx, worktree, patch.Diff); aerr != nil {
			return failWith(fmt.Errorf("apply diff: %w", aerr))
		}
		run.Diff = patch.Diff
		run.DiffStats = ComputeDiffStats(patch.Diff)
		run.PatchKind = patch.Kind
		emitStage(SIStagePatching, attempt, ActivityStatusCompleted, "")
		cursor.stage = ""

		// ── Verifying ───────────────────────────────────────────────────────
		if err := transition(RunEventVerify); err != nil {
			return err
		}
		cursor.stage, cursor.attempt = SIStageVerifying, attempt
		emitStage(SIStageVerifying, attempt, ActivityStatusRunning, "")
		goPkgs, _ := DeriveAffectedScopes(changes)
		var report []SandboxGateResult
		allPass := true
		for _, gate := range []SandboxGateKind{SandboxGateBuild, SandboxGateTest, SandboxGateLint} {
			res, gerr := uc.sandbox.RunGate(ctx, worktree, gate, goPkgs)
			if gerr != nil {
				res = SandboxGateResult{Gate: gate, Passed: false, Output: "gate exec error: " + gerr.Error()}
			}
			report = append(report, res)
			if !res.Passed {
				allPass = false
				break
			}
		}
		run.VerificationReport = report

		if allPass {
			emitStage(SIStageVerifying, attempt, ActivityStatusCompleted, "")
			cursor.stage = ""
			return uc.govern(ctx, run, patch, transition, rejectWith, emitStage, cursor)
		}
		emitStage(SIStageVerifying, attempt, ActivityStatusFailed, siFailedGateDigest(report))
		cursor.stage = ""

		// 用户介入（T3.6）：verify 失败决策点前再消费一次。
		if cmd, ok := pollControl(); ok {
			if exitErr, exit := handleControl(cmd); exit {
				return exitErr
			}
		}

		// 重试回路：记录 attempt → 带上 Gate 输出回 Patcher。
		if aerr := uc.runWriter.RecordAttempt(ctx, run.ID); aerr != nil {
			uc.lg.Warn("self-improve pipeline: record attempt failed",
				loggateway.StepID("si_pipeline.attempt"), loggateway.Err(aerr))
		}
		run.Attempts++
		if skipRetry || attempt >= uc.maxAttempts {
			if skipRetry {
				run.ClosedReason = fmt.Sprintf("user_skip_retry: verify gates failed at attempt %d", attempt)
			} else {
				run.ClosedReason = fmt.Sprintf("verify gates failed after %d attempts", attempt)
			}
			return transition(RunEventVerifyFailFinal)
		}
		retryHint = siFailedGateDigest(report)
		if err := transition(RunEventVerifyFail); err != nil {
			return err
		}
	}
}

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
			// Critic 失败降级为无报告（不阻断流水线，R4 不生效）。
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
	// 状态不变，仅持久化治理产物（CAS from = 当前状态）。
	run.UpdatedAt = time.Now().UTC()
	return uc.runWriter.Update(ctx, run, RunStatusAwaitingGovernance)
}

// siFailedGateDigest condenses failed-gate output into a Patcher retry hint.
func siFailedGateDigest(report []SandboxGateResult) string {
	const maxOut = 500
	var b strings.Builder
	for _, r := range report {
		if r.Passed {
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
