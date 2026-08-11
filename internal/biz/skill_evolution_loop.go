package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Evolution Loop Stage Constants ────────────────────────────────────────────

const (
	EvoStageSolve   = "solve"
	EvoStageObserve = "observe"
	EvoStageEvolve  = "evolve"
	EvoStageGate    = "gate"
	EvoStageReload  = "reload"

	// EvoExpirationDays is the number of days after which a pending evolution
	// suggestion is automatically marked as expired.
	EvoExpirationDays = 7

	// GatePerformanceDegradationThreshold is the maximum allowed percentage
	// degradation in duration or token usage before the Gate rejects.
	GatePerformanceDegradationThreshold = 0.20

	// GateMaxDraftLength is the maximum allowed draft body length in characters.
	GateMaxDraftLength = 10000

	// GateHarmfulRuleRejectThreshold 是 effectiveness 维度的拒绝阈值：当前
	// 正文中 harmful 计数达到该值的规则，若 draft 原样保留其内容则拒绝
	// （必须重写或移除）。
	GateHarmfulRuleRejectThreshold = 3

	// GateHelpfulRuleKeepThreshold 是 drift 维度（P2 F2 破坏性更新）的保留
	// 阈值：当前正文中 helpful 计数达到该值的规则被 draft 删除时拒绝。
	// 与 harmful 拒绝阈值同值（计数对称语义），独立命名避免语义错位。
	GateHelpfulRuleKeepThreshold = GateHarmfulRuleRejectThreshold

	// GateMaxRemoveRatio 是 drift 维度（P2 F2 破坏性更新）的最大删除比例：
	// 当前规则数 ≥ GateRemoveRatioMinRules 且 draft 删除比例超过该值时拒绝。
	GateMaxRemoveRatio = 0.5
	// GateRemoveRatioMinRules 是删除比例判定生效的最小当前规则数（防小基数
	// 误杀）。
	GateRemoveRatioMinRules = 4
	// GateMaxRuleGrowthRatio 与 GateMaxRuleGrowthAbs 是 drift 维度（P2 F2
	// 臃肿检测）的双条件阈值：draft 规则数同时超过「当前 × 比例」与
	// 「当前 + 绝对值」才拒绝（双条件防小基数误杀）。
	GateMaxRuleGrowthRatio = 1.5
	GateMaxRuleGrowthAbs   = 5

	// ReplayPassThreshold 是数据集回放（Solve 接线）的通过率阈值，低于
	// 该值 Gate 功能维拒绝。
	ReplayPassThreshold = 0.6

	// ReplayMaxCases 是单次回放最多执行的评测用例数。
	ReplayMaxCases = 5
)

// ── Evolution Loop Types ──────────────────────────────────────────────────────

// SkillTaskResult is the result of executing a target task with the current
// Skill configuration during the Solve stage.
type SkillTaskResult struct {
	Success      bool
	DurationMS   int
	TokenUsage   int
	Output       string
	ErrorMessage string
}

// EvolutionObservationReport is the structured observation data collected during
// the Observe stage. It contains performance metrics, structured logs, and
// invocation success rates.
type EvolutionObservationReport struct {
	SuccessRate        float64
	AvgDurationMS      int
	AvgTokenUsage      int
	InvocationCount    int
	FailureTagCounts   map[string]int
	StructuredLogs     []string
	PerformanceMetrics map[string]float64

	// Baseline metrics for performance comparison in Gate stage.
	BaselineDurationMS int
	BaselineTokenUsage int
}

// GateCheckResult is the result of a single Gate verification dimension.
type GateCheckResult struct {
	Name   string
	Passed bool
	Reason string
}

// GateVerificationResult is the combined result of all Gate verification dimensions.
type GateVerificationResult struct {
	Passed bool
	Checks []GateCheckResult
}

// EvolutionLoopResult is the final result of running the five-stage evolution loop.
type EvolutionLoopResult struct {
	Passed     bool
	Stage      string
	DraftBody  string
	GateResult *GateVerificationResult
	// Err is non-nil when a stage failed due to a system error (not business rejection).
	// Business rejection: Passed=false, Err=nil. System failure: Passed=false, Err!=nil.
	Err error
}

// ── Evolution Loop Port Interfaces ────────────────────────────────────────────

// SkillTaskRunner executes a target task with the current Skill configuration
// during the Solve stage.
type SkillTaskRunner interface {
	RunTask(ctx context.Context, skillID string, task string) (*SkillTaskResult, error)
}

// SkillObserver collects structured observation data during the Observe stage.
type SkillObserver interface {
	Observe(ctx context.Context, skillID string, result *SkillTaskResult) (*EvolutionObservationReport, error)
}

// SkillEvolver calls the Curator Agent to analyze observation data and generate
// a Skill draft (SKILL.md) during the Evolve stage.
type SkillEvolver interface {
	Evolve(ctx context.Context, skillID string, report *EvolutionObservationReport) (string, error)
}

// SkillReplayABRunner replays the skill's bound evaluation dataset against
// BOTH the current live body (baseline) and the evolved draft over the same
// case set (P2 F1 AB 对照回放). Baseline may be nil when the current body is
// unavailable — callers then apply only the absolute threshold (no ratchet).
//
// Stability:evolving
type SkillReplayABRunner interface {
	ReplayAB(ctx context.Context, skillID string, draftBody string, maxCases int) (*SkillReplayABResult, error)
}

// SkillReplayABResult is one A/B comparison replay outcome.
type SkillReplayABResult struct {
	Baseline *SkillReplayResult // nil = 当前正文不可得，仅绝对阈值生效
	Draft    *SkillReplayResult
}

// SkillGateVerifier performs multi-dimensional Gate verification.
type SkillGateVerifier interface {
	Verify(ctx context.Context, skillID string, draftBody string, observation *EvolutionObservationReport) (*GateVerificationResult, error)
}

// SkillReloader registers a new Skill version during the Reload stage.
type SkillReloader interface {
	Reload(ctx context.Context, skillID string, draftBody string, parentVersionID string, evolutionReason string) error
}

// SandboxRunner runs a draft skill in a sandbox environment for functional
// verification during the Gate stage.
type SandboxRunner interface {
	RunSandbox(ctx context.Context, skillID string, draftBody string) (bool, json.RawMessage, error)
}

// SkillLintChecker performs style/lint checks on a draft skill body.
type SkillLintChecker interface {
	LintCheck(ctx context.Context, draftBody string) (bool, string, error)
}

// ── Solve 接线（P1：数据集回放进 Gate）──────────────────────────────────────

// ErrNoReplayDataset 表示该 skill 没有对应的 evaluation 数据集（按名称
// 寻址约定未命中）；Gate 功能维视为跳过，不阻断。
var ErrNoReplayDataset = apierror.NotFound("SKILL_REPLAY", "no evaluation dataset bound to this skill")

// SkillReplayResult 是一次数据集回放的汇总结果。
type SkillReplayResult struct {
	DatasetID   string
	DatasetName string
	Total       int
	Passed      int
	PassRate    float64
	// CaseResults 是逐 case 判定（P3 M1 配对归因的数据基础），与数据集 case
	// 顺序对齐。旧实现/假 runner 可能为空——消费方按跳过语义降级。
	CaseResults []CaseVerdict
}

// CaseVerdict 是一个 case 的回放判定（P3 M1）。OutputHash 为 trim 后输出的
// sha256（十六进制）；LLM 调用失败时 Passed=false 且 OutputHash 为空，
// 空 hash 不参与等价比较。
type CaseVerdict struct {
	CaseID     string
	Passed     bool
	OutputHash string
}

// SkillReplayRunner 用 evaluation 数据集对 draft 做真实任务回放（Solve 阶段
// 的功能验证）。
//
// 语义约定（best-effort，与项目降级风格一致）：
//   - 无绑定数据集 → 返回 ErrNoReplayDataset（Gate 跳过回放检查）
//   - LLM 未配置等回放不可用 → 返回错误（Gate 跳过回放检查，不阻断）
//   - 回放成功但通过率 < ReplayPassThreshold → 返回结果，Gate 拒绝
//
// Stability:evolving
type SkillReplayRunner interface {
	Replay(ctx context.Context, skillID string, draftBody string, maxCases int) (*SkillReplayResult, error)
}

// ── SkillEvolutionLoop ────────────────────────────────────────────────────────

// EvolutionLoopOptions holds optional parameters for the evolution loop Run method.
type EvolutionLoopOptions struct {
	CurrentVersionID string
}

// SkillEvolutionLoop implements the Solve→Observe→Evolve→Gate→Reload five-stage
// Skill evolution loop with multi-dimensional Gate verification and expiration.
type SkillEvolutionLoop struct {
	runner   SkillTaskRunner
	observer SkillObserver
	evolver  SkillEvolver
	gate     SkillGateVerifier
	reloader SkillReloader
	lg       loggateway.Logger
}

// NewSkillEvolutionLoop constructs a SkillEvolutionLoop.
func NewSkillEvolutionLoop(
	runner SkillTaskRunner,
	observer SkillObserver,
	evolver SkillEvolver,
	gate SkillGateVerifier,
	reloader SkillReloader,
	lg loggateway.Logger,
) *SkillEvolutionLoop {
	return &SkillEvolutionLoop{
		runner:   runner,
		observer: observer,
		evolver:  evolver,
		gate:     gate,
		reloader: reloader,
		lg:       lg,
	}
}

// Run executes the five-stage evolution loop:
//  1. Solve: Execute target task with current Skill configuration, record results
//  2. Observe: Collect structured logs, performance metrics, Skill invocation
//     success rate, store in experience report
//  3. Evolve: Call Curator Agent to analyze observation data and generate Skill
//     draft (SKILL.md)
//  4. Gate: Multi-dimensional verification (functional, security, performance,
//     style)
//  5. Reload: Register new Skill version, mark parent_version_id and
//     evolution_reason
func (l *SkillEvolutionLoop) Run(ctx context.Context, skillID string, task string, opts ...EvolutionLoopOptions) (*EvolutionLoopResult, error) {
	skillID, err := requireNonEmpty(skillID, "EVO_LOOP", "skill_id")
	if err != nil {
		return nil, err
	}

	var currentVersionID string
	if len(opts) > 0 {
		currentVersionID = opts[0].CurrentVersionID
	}

	// Stage 1: Solve
	solveResult, err := l.solve(ctx, skillID, task)
	if err != nil || !solveResult.Success {
		reason := ""
		if err != nil {
			reason = err.Error()
		} else {
			reason = solveResult.ErrorMessage
		}
		l.lg.Warn("EvolutionLoop: Solve stage failed",
			loggateway.StepID("evo_loop.solve"),
			loggateway.Str("skill_id", skillID),
			loggateway.Str("reason", reason))
		return &EvolutionLoopResult{
			Passed: false,
			Stage:  EvoStageSolve,
			Err:    err,
		}, nil
	}

	// Stage 2: Observe
	observeReport, err := l.observe(ctx, skillID, solveResult)
	if err != nil {
		l.lg.Warn("EvolutionLoop: Observe stage failed",
			loggateway.StepID("evo_loop.observe"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return &EvolutionLoopResult{
			Passed: false,
			Stage:  EvoStageObserve,
			Err:    err,
		}, nil
	}

	// Stage 3: Evolve
	draftBody, err := l.evolve(ctx, skillID, observeReport)
	if err != nil {
		l.lg.Warn("EvolutionLoop: Evolve stage failed",
			loggateway.StepID("evo_loop.evolve"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return &EvolutionLoopResult{
			Passed: false,
			Stage:  EvoStageEvolve,
			Err:    err,
		}, nil
	}

	// Stage 4: Gate
	gateResult, err := l.gateVerify(ctx, skillID, draftBody, observeReport)
	if err != nil || !gateResult.Passed {
		l.lg.Warn("EvolutionLoop: Gate stage rejected",
			loggateway.StepID("evo_loop.gate"),
			loggateway.Str("skill_id", skillID))
		return &EvolutionLoopResult{
			Passed:     false,
			Stage:      EvoStageGate,
			DraftBody:  draftBody,
			GateResult: gateResult,
			Err:        err,
		}, nil
	}

	// Stage 5: Reload
	evolutionReason := fmt.Sprintf("evolution: auto-improved skill based on observation (success_rate=%.2f, invocations=%d)",
		observeReport.SuccessRate, observeReport.InvocationCount)
	if err := l.reload(ctx, skillID, draftBody, currentVersionID, evolutionReason); err != nil {
		l.lg.Warn("EvolutionLoop: Reload stage failed",
			loggateway.StepID("evo_loop.reload"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return &EvolutionLoopResult{
			Passed:    false,
			Stage:     EvoStageReload,
			DraftBody: draftBody,
			Err:       err,
		}, nil
	}

	l.lg.Info("EvolutionLoop: completed successfully",
		loggateway.StepID("evo_loop.complete"),
		loggateway.Str("skill_id", skillID))

	return &EvolutionLoopResult{
		Passed:     true,
		Stage:      EvoStageReload,
		DraftBody:  draftBody,
		GateResult: gateResult,
	}, nil
}

// ── Stage implementations ─────────────────────────────────────────────────────

func (l *SkillEvolutionLoop) solve(ctx context.Context, skillID string, task string) (*SkillTaskResult, error) {
	if l.runner == nil {
		return nil, apierror.BadRequest("EVO_LOOP", "skill task runner not configured")
	}
	return l.runner.RunTask(ctx, skillID, task)
}

func (l *SkillEvolutionLoop) observe(ctx context.Context, skillID string, result *SkillTaskResult) (*EvolutionObservationReport, error) {
	if l.observer == nil {
		return nil, apierror.BadRequest("EVO_LOOP", "skill observer not configured")
	}
	return l.observer.Observe(ctx, skillID, result)
}

func (l *SkillEvolutionLoop) evolve(ctx context.Context, skillID string, report *EvolutionObservationReport) (string, error) {
	if l.evolver == nil {
		return "", apierror.BadRequest("EVO_LOOP", "skill evolver not configured")
	}
	return l.evolver.Evolve(ctx, skillID, report)
}

func (l *SkillEvolutionLoop) gateVerify(ctx context.Context, skillID string, draftBody string, observation *EvolutionObservationReport) (*GateVerificationResult, error) {
	if l.gate == nil {
		return nil, apierror.BadRequest("EVO_LOOP", "skill gate verifier not configured")
	}
	return l.gate.Verify(ctx, skillID, draftBody, observation)
}

func (l *SkillEvolutionLoop) reload(ctx context.Context, skillID string, draftBody string, parentVersionID string, evolutionReason string) error {
	if l.reloader == nil {
		return apierror.BadRequest("EVO_LOOP", "skill reloader not configured")
	}
	return l.reloader.Reload(ctx, skillID, draftBody, parentVersionID, evolutionReason)
}

// ── GateVerifier ──────────────────────────────────────────────────────────────

// GateOption configures optional GateVerifier dimensions.
type GateOption func(*GateVerifier)

// WithReplayRunner enables the dataset-replay functional check (P1 Solve
// 接线): after the sandbox check passes, the draft is replayed against the
// skill's bound evaluation dataset.
func WithReplayRunner(r SkillReplayRunner) GateOption {
	return func(v *GateVerifier) { v.replayRunner = r }
}

// WithSkillLookup enables the effectiveness dimension (P1 计数归因): rules
// whose harmful counter reached GateHarmfulRuleRejectThreshold must not be
// kept unchanged in the draft.
func WithSkillLookup(r SkillLookupReader) GateOption {
	return func(v *GateVerifier) { v.skillLookup = r }
}

// WithABReplayRunner enables the A/B comparison replay (P2 F1 棘轮门控): the
// draft is replayed side-by-side with the current live body over the same
// case set; the draft must not regress below the baseline. When wired, the
// AB runner takes precedence over the single replay runner.
func WithABReplayRunner(r SkillReplayABRunner) GateOption {
	return func(v *GateVerifier) { v.abRunner = r }
}

// GateVerifier performs multi-dimensional Gate verification for skill evolution.
// Dimensions:
//   - Functional correctness (Sandbox Runner + optional dataset replay, or
//     rule-based fallback; with AB wired, includes the ratchet + absolute
//     threshold over replay summaries)
//   - Security (sensitive info detection: API key, password, token)
//   - Performance (Token/duration comparison, >20% degradation → reject)
//   - Style (lint check / length check)
//   - Effectiveness (harmful-count rules must not be kept unchanged; only
//     when WithSkillLookup is wired)
//   - Drift (destructive update / bloat detection; WithSkillLookup)
//   - Trigger accuracy (golden-set regression; WithTriggerGoldenRunner)
//   - Paired regression (P3 M1: per-case paired comparison over AB replay
//     verdicts — a case passing under baseline must not fail under draft)
//   - No-op change (P3 M1: draft outputs byte-identical to baseline on every
//     comparable case → no measurable effect → reject)
type GateVerifier struct {
	sandboxRunner       SandboxRunner
	lintChecker         SkillLintChecker
	replayRunner        SkillReplayRunner
	abRunner            SkillReplayABRunner
	skillLookup         SkillLookupReader
	triggerGoldenRunner SkillTriggerGoldenRunner
}

// NewGateVerifier constructs a GateVerifier. sandboxRunner and lintChecker are
// optional; if nil, rule-based fallback checks are used. Optional dimensions
// (replay / effectiveness) are enabled via GateOption.
func NewGateVerifier(sandboxRunner SandboxRunner, lintChecker SkillLintChecker, opts ...GateOption) *GateVerifier {
	v := &GateVerifier{
		sandboxRunner: sandboxRunner,
		lintChecker:   lintChecker,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Verify performs all nine Gate verification dimensions. Any failure rejects
// the evolution.
func (v *GateVerifier) Verify(ctx context.Context, skillID string, draftBody string, observation *EvolutionObservationReport) (*GateVerificationResult, error) {
	var checks []GateCheckResult

	// Dimension 1: Functional correctness (sandbox/base + optional dataset replay).
	// P3 M1：AB 回放提升为共享单次执行——functional（棘轮/绝对阈值）、
	// paired_regression、no_op_change 三个维度共用同一份回放结果，LLM 成本
	// 只付一次。base 检查失败时不触发回放（非法 draft 不烧 LLM 调用）。
	base := v.verifyFunctionalBase(ctx, skillID, draftBody)
	var ab *SkillReplayABResult
	var abErr error
	if base.Passed && v.abRunner != nil {
		ab, abErr = v.abRunner.ReplayAB(ctx, skillID, draftBody, ReplayMaxCases)
	}
	functional := base
	if functional.Passed {
		functional = v.verifyReplay(ctx, skillID, draftBody, ab, abErr)
	}
	checks = append(checks, functional)

	// Dimension 2: Security
	checks = append(checks, v.verifySecurity(draftBody))

	// Dimension 3: Performance
	checks = append(checks, v.verifyPerformance(observation))

	// Dimension 4: Style
	checks = append(checks, v.verifyStyle(ctx, draftBody))

	// Dimension 5: Effectiveness (P1 计数归因)
	checks = append(checks, v.verifyEffectiveness(ctx, skillID, draftBody))

	// Dimension 6: Drift (P2 F2 破坏性更新 + 臃肿检测)
	checks = append(checks, v.verifyDrift(ctx, skillID, draftBody))

	// Dimension 7: Trigger accuracy (P2 F4 触发率黄金集回归)
	checks = append(checks, v.verifyTriggerAccuracy(ctx, skillID, draftBody))

	// Dimension 8: Paired regression (P3 M1 per-case 配对判定)
	checks = append(checks, v.verifyPairedRegression(ab, abErr))

	// Dimension 9: No-op change (P3 M1 等价改动检测)
	checks = append(checks, v.verifyNoOpChange(ab, abErr))

	allPassed := true
	for _, c := range checks {
		if !c.Passed {
			allPassed = false
			break
		}
	}

	return &GateVerificationResult{
		Passed: allPassed,
		Checks: checks,
	}, nil
}

// verifyFunctionalBase is the pre-P1 functional check: Sandbox Runner when
// wired, otherwise rule-based structure/content checks.
func (v *GateVerifier) verifyFunctionalBase(ctx context.Context, skillID string, draftBody string) GateCheckResult {
	if v.sandboxRunner != nil {
		passed, _, err := v.sandboxRunner.RunSandbox(ctx, skillID, draftBody)
		if err != nil {
			return GateCheckResult{
				Name:   "functional",
				Passed: false,
				Reason: fmt.Sprintf("sandbox execution error: %s", err.Error()),
			}
		}
		if !passed {
			return GateCheckResult{
				Name:   "functional",
				Passed: false,
				Reason: "sandbox validation failed",
			}
		}
		return GateCheckResult{Name: "functional", Passed: true}
	}

	// Rule-based fallback: basic structure and content quality checks
	if draftBody == "" {
		return GateCheckResult{
			Name:   "functional",
			Passed: false,
			Reason: "draft body is empty",
		}
	}
	if skillID == "" {
		return GateCheckResult{
			Name:   "functional",
			Passed: false,
			Reason: "skill ID is empty",
		}
	}
	// Draft must contain at least one actionable section (## heading with content).
	lines := strings.Split(draftBody, "\n")
	hasHeading := false
	hasContentAfterHeading := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			hasHeading = true
			// Check if there is non-empty content within the next few lines.
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
					hasContentAfterHeading = true
					break
				}
			}
		}
	}
	if !hasHeading || !hasContentAfterHeading {
		return GateCheckResult{
			Name:   "functional",
			Passed: false,
			Reason: "draft body must contain at least one heading with actionable content",
		}
	}
	return GateCheckResult{Name: "functional", Passed: true}
}

// verifyReplay runs the dataset-replay functional check (P1 Solve 接线 + P2
// F1 AB 对照棘轮). When the AB runner is wired, the caller-prefetched AB
// result (ab/abErr) is used — the replay executes at most once per Verify.
// Skip semantics: no replay runner wired, no bound dataset, or replay
// infrastructure unavailable all degrade to pass (the functional verdict then
// rests on the sandbox/base check alone). Only a completed replay below the
// pass threshold (or, with AB wired, below the baseline) rejects.
func (v *GateVerifier) verifyReplay(ctx context.Context, skillID string, draftBody string, ab *SkillReplayABResult, abErr error) GateCheckResult {
	if v.abRunner != nil {
		return replayABGateCheck(ab, abErr)
	}
	if v.replayRunner == nil {
		return GateCheckResult{Name: "functional", Passed: true}
	}
	result, err := v.replayRunner.Replay(ctx, skillID, draftBody, ReplayMaxCases)
	if err != nil || result == nil {
		// ErrNoReplayDataset 与回放不可用（LLM 未配置等）均跳过，不阻断。
		return GateCheckResult{Name: "functional", Passed: true, Reason: "dataset replay skipped"}
	}
	if result.Total > 0 && result.PassRate < ReplayPassThreshold {
		return GateCheckResult{
			Name:   "functional",
			Passed: false,
			Reason: fmt.Sprintf("dataset replay pass rate %.0f%% < %.0f%% (dataset=%s, %d/%d)",
				result.PassRate*100, ReplayPassThreshold*100, result.DatasetName, result.Passed, result.Total),
		}
	}
	return GateCheckResult{
		Name:   "functional",
		Passed: true,
		Reason: fmt.Sprintf("dataset replay passed (dataset=%s, %d/%d)", result.DatasetName, result.Passed, result.Total),
	}
}

// replayABGateCheck derives the functional-dimension verdict from a
// prefetched A/B comparison replay result (P2 F1). Verdicts:
//   - replay unavailable (error / nil draft result) → skip, pass
//   - baseline available and draft < baseline → ratchet rejection
//   - draft < ReplayPassThreshold → absolute-threshold rejection
//   - otherwise → pass
func replayABGateCheck(ab *SkillReplayABResult, err error) GateCheckResult {
	if err != nil || ab == nil || ab.Draft == nil {
		// ErrNoReplayDataset 与回放不可用（LLM 未配置等）均跳过，不阻断。
		return GateCheckResult{Name: "functional", Passed: true, Reason: "AB replay skipped"}
	}
	draft := ab.Draft
	// 棘轮：draft 不得劣于 baseline（baseline 不可得时跳过）。
	if ab.Baseline != nil && ab.Baseline.Total > 0 && draft.Total > 0 && draft.PassRate < ab.Baseline.PassRate {
		return GateCheckResult{
			Name:   "functional",
			Passed: false,
			Reason: fmt.Sprintf("AB replay ratchet: draft pass rate %.0f%% < baseline %.0f%% (dataset=%s, %d/%d vs %d/%d)",
				draft.PassRate*100, ab.Baseline.PassRate*100, draft.DatasetName, draft.Passed, draft.Total, ab.Baseline.Passed, ab.Baseline.Total),
		}
	}
	if draft.Total > 0 && draft.PassRate < ReplayPassThreshold {
		return GateCheckResult{
			Name:   "functional",
			Passed: false,
			Reason: fmt.Sprintf("dataset replay pass rate %.0f%% < %.0f%% (dataset=%s, %d/%d)",
				draft.PassRate*100, ReplayPassThreshold*100, draft.DatasetName, draft.Passed, draft.Total),
		}
	}
	return GateCheckResult{
		Name:   "functional",
		Passed: true,
		Reason: fmt.Sprintf("AB replay passed (dataset=%s, draft %d/%d)", draft.DatasetName, draft.Passed, draft.Total),
	}
}

// pairedCaseResults aligns baseline/draft per-case verdicts for the P3 M1
// paired dimensions. ok=false when data is unavailable (replay error, no
// baseline, legacy runner without per-case collection) or the two sides
// cannot be paired (different case sets) — callers degrade to skip-to-pass.
func pairedCaseResults(ab *SkillReplayABResult, abErr error) (base, draft []CaseVerdict, ok bool) {
	if abErr != nil || ab == nil || ab.Baseline == nil || ab.Draft == nil {
		return nil, nil, false
	}
	base, draft = ab.Baseline.CaseResults, ab.Draft.CaseResults
	if len(base) == 0 || len(base) != len(draft) {
		return nil, nil, false
	}
	for i := range base {
		if base[i].CaseID != draft[i].CaseID {
			return nil, nil, false
		}
	}
	return base, draft, true
}

// verifyPairedRegression is the eighth Gate dimension (P3 M1 per-case 配对判
// 定): a case that passed under the baseline must not fail under the draft —
// wins on other cases do not compensate a regression. Win/loss/tie counts are
// recorded in the reason for approval audit. Skips (passes) when paired
// per-case data is unavailable.
func (v *GateVerifier) verifyPairedRegression(ab *SkillReplayABResult, abErr error) GateCheckResult {
	base, draft, ok := pairedCaseResults(ab, abErr)
	if !ok {
		return GateCheckResult{Name: "paired_regression", Passed: true, Reason: "AB per-case data unavailable, skipped"}
	}
	wins, losses, ties := 0, 0, 0
	var regressions []string
	for i := range base {
		switch {
		case base[i].Passed && !draft[i].Passed:
			losses++
			regressions = append(regressions, base[i].CaseID)
		case !base[i].Passed && draft[i].Passed:
			wins++
		default:
			ties++
		}
	}
	if len(regressions) > 0 {
		shown := regressions
		if len(shown) > 3 {
			shown = shown[:3]
		}
		return GateCheckResult{
			Name:   "paired_regression",
			Passed: false,
			Reason: fmt.Sprintf("per-case regression on %d case(s) [%s] (win/loss/tie=%d/%d/%d): cases passing under baseline must not fail under draft",
				len(regressions), strings.Join(shown, ","), wins, losses, ties),
		}
	}
	return GateCheckResult{
		Name:   "paired_regression",
		Passed: true,
		Reason: fmt.Sprintf("no per-case regression (win/loss/tie=%d/%d/%d)", wins, losses, ties),
	}
}

// verifyNoOpChange is the ninth Gate dimension (P3 M1 等价改动检测): when the
// draft's replay outputs are byte-identical to the baseline on every
// comparable case, the change has no measurable effect and must not consume a
// version/approval cycle. Cases with a failed LLM call (empty hash) on either
// side are not comparable and excluded. Skips (passes) when no comparable
// case exists.
func (v *GateVerifier) verifyNoOpChange(ab *SkillReplayABResult, abErr error) GateCheckResult {
	base, draft, ok := pairedCaseResults(ab, abErr)
	if !ok {
		return GateCheckResult{Name: "no_op_change", Passed: true, Reason: "AB per-case data unavailable, skipped"}
	}
	comparable, identical := 0, 0
	for i := range base {
		if base[i].OutputHash == "" || draft[i].OutputHash == "" {
			continue
		}
		comparable++
		if base[i].OutputHash == draft[i].OutputHash {
			identical++
		}
	}
	if comparable > 0 && identical == comparable {
		return GateCheckResult{
			Name:   "no_op_change",
			Passed: false,
			Reason: fmt.Sprintf("draft outputs byte-identical to baseline on all %d comparable case(s): no measurable effect", comparable),
		}
	}
	return GateCheckResult{Name: "no_op_change", Passed: true}
}

// verifyEffectiveness is the fifth Gate dimension (P1 计数归因): current-body
// rules whose harmful counter reached GateHarmfulRuleRejectThreshold must not
// be kept unchanged in the draft — the Curator must rewrite or remove them.
// Skips (passes) when no skill lookup is wired, the current body has no rule
// blocks, or the lookup fails (nil-safe degradation).
func (v *GateVerifier) verifyEffectiveness(ctx context.Context, skillID string, draftBody string) GateCheckResult {
	if v.skillLookup == nil {
		return GateCheckResult{Name: "effectiveness", Passed: true}
	}
	currentBody, err := v.skillLookup.GetLatestSkillMarkdown(ctx, skillID)
	if err != nil || !HasRuleBlocks(currentBody) {
		return GateCheckResult{Name: "effectiveness", Passed: true}
	}
	currentDoc := ParseRuleBlocks(currentBody)
	draftDoc := ParseRuleBlocks(draftBody)
	for _, r := range currentDoc.Rules() {
		if r.Harmful < GateHarmfulRuleRejectThreshold {
			continue
		}
		draftRule := draftDoc.RuleByID(r.ID)
		if draftRule != nil && strings.TrimSpace(draftRule.Content) == strings.TrimSpace(r.Content) {
			return GateCheckResult{
				Name:   "effectiveness",
				Passed: false,
				Reason: fmt.Sprintf("rule %q kept unchanged despite harmful count %d (>= %d): consecutive ineffective improvements, rewrite or remove it",
					r.ID, r.Harmful, GateHarmfulRuleRejectThreshold),
			}
		}
	}
	return GateCheckResult{Name: "effectiveness", Passed: true}
}

// verifyDrift is the sixth Gate dimension (P2 F2 漂移检测). It implements two
// of the three SKILL-KD drift classes that are programmatically decidable:
//   - destructive-update drift: the draft removes a current rule whose helpful
//     counter reached GateHelpfulRuleKeepThreshold, or removes more than
//     GateMaxRemoveRatio of the current rules (when there are at least
//     GateRemoveRatioMinRules of them)
//   - skill-bloat drift: the draft rule count exceeds BOTH
//     current × GateMaxRuleGrowthRatio AND current + GateMaxRuleGrowthAbs
//     (dual condition avoids small-base false positives)
//
// modify/merge keep the rule ID and therefore never count as removal.
// Skips (passes) when no skill lookup is wired, the current body has no rule
// blocks (full rewrites cannot attribute removals), or the lookup fails.
func (v *GateVerifier) verifyDrift(ctx context.Context, skillID string, draftBody string) GateCheckResult {
	if v.skillLookup == nil {
		return GateCheckResult{Name: "drift", Passed: true}
	}
	currentBody, err := v.skillLookup.GetLatestSkillMarkdown(ctx, skillID)
	if err != nil || !HasRuleBlocks(currentBody) {
		return GateCheckResult{Name: "drift", Passed: true}
	}
	current := ParseRuleBlocks(currentBody).Rules()
	if len(current) == 0 {
		return GateCheckResult{Name: "drift", Passed: true}
	}
	draftDoc := ParseRuleBlocks(draftBody)

	// 删除集 = 当前规则 id 集合 − draft 规则 id 集合（modify/merge 保留 id，
	// 不计删除）。
	var removed []*RuleBlock
	for _, r := range current {
		if draftDoc.RuleByID(r.ID) == nil {
			removed = append(removed, r)
		}
	}

	// 破坏性更新 1：删除高 helpful 规则。
	for _, r := range removed {
		if r.Helpful >= GateHelpfulRuleKeepThreshold {
			return GateCheckResult{
				Name:   "drift",
				Passed: false,
				Reason: fmt.Sprintf("destructive update: rule %q with helpful count %d (>= %d) was removed",
					r.ID, r.Helpful, GateHelpfulRuleKeepThreshold),
			}
		}
	}

	// 破坏性更新 2：删除比例超阈值。
	if len(current) >= GateRemoveRatioMinRules && len(removed) > 0 &&
		float64(len(removed))/float64(len(current)) > GateMaxRemoveRatio {
		return GateCheckResult{
			Name:   "drift",
			Passed: false,
			Reason: fmt.Sprintf("destructive update: %d/%d rules removed (> %.0f%%)",
				len(removed), len(current), GateMaxRemoveRatio*100),
		}
	}

	// 臃肿：双条件同时成立才拒绝。
	draftCount := len(draftDoc.Rules())
	if float64(draftCount) > float64(len(current))*GateMaxRuleGrowthRatio &&
		draftCount > len(current)+GateMaxRuleGrowthAbs {
		return GateCheckResult{
			Name:   "drift",
			Passed: false,
			Reason: fmt.Sprintf("skill bloat: rule count %d → %d (exceeds both ×%.1f and +%d)",
				len(current), draftCount, GateMaxRuleGrowthRatio, GateMaxRuleGrowthAbs),
		}
	}

	return GateCheckResult{Name: "drift", Passed: true}
}

// Sensitive info detection patterns.
var (
	// API key patterns: common prefixes like sk-, pk-, ak-, etc. followed by 20+ alphanumeric chars
	apiKeyPattern = regexp.MustCompile(`(?i)(sk|pk|ak|api[_-]?key)\s*[:=]\s*[\w\-]{20,}`)
	// Standalone API key pattern: sk- followed by 20+ alphanumeric/hyphen chars
	apiKeyStandalonePattern = regexp.MustCompile(`(?i)sk-[a-zA-Z0-9\-]{20,}`)
	// Password patterns
	passwordPattern = regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`)
	// Token patterns: Bearer tokens, JWT-like strings
	tokenPattern = regexp.MustCompile(`(?i)(token|bearer)\s*[:=]\s*(eyJ[\w\-]+\.[\w\-]+\.[\w\-]+|[\w\-]{32,})`)
)

// verifySecurity checks for sensitive information in the draft body.
func (v *GateVerifier) verifySecurity(draftBody string) GateCheckResult {
	if apiKeyPattern.MatchString(draftBody) || apiKeyStandalonePattern.MatchString(draftBody) {
		return GateCheckResult{
			Name:   "security",
			Passed: false,
			Reason: "sensitive info detected: API key pattern",
		}
	}
	if passwordPattern.MatchString(draftBody) {
		return GateCheckResult{
			Name:   "security",
			Passed: false,
			Reason: "sensitive info detected: password pattern",
		}
	}
	if tokenPattern.MatchString(draftBody) {
		return GateCheckResult{
			Name:   "security",
			Passed: false,
			Reason: "sensitive info detected: token pattern",
		}
	}
	return GateCheckResult{Name: "security", Passed: true}
}

// verifyPerformance checks for performance degradation compared to baseline.
// If duration or token usage degrades by more than 20%, the evolution is rejected.
func (v *GateVerifier) verifyPerformance(observation *EvolutionObservationReport) GateCheckResult {
	if observation == nil {
		return GateCheckResult{Name: "performance", Passed: true}
	}

	// Check duration degradation
	if observation.BaselineDurationMS > 0 && observation.AvgDurationMS > 0 {
		durationDegradation := float64(observation.AvgDurationMS-observation.BaselineDurationMS) / float64(observation.BaselineDurationMS)
		if durationDegradation > GatePerformanceDegradationThreshold {
			return GateCheckResult{
				Name:   "performance",
				Passed: false,
				Reason: fmt.Sprintf("duration degradation %.1f%% exceeds threshold %.0f%% (%dms → %dms)",
					durationDegradation*100, GatePerformanceDegradationThreshold*100,
					observation.BaselineDurationMS, observation.AvgDurationMS),
			}
		}
	} else if observation.BaselineDurationMS == 0 && observation.AvgDurationMS > 60000 {
		return GateCheckResult{
			Name:   "performance",
			Passed: false,
			Reason: fmt.Sprintf("insufficient baseline data: avg duration %dms exceeds absolute threshold 60000ms", observation.AvgDurationMS),
		}
	}

	// Check token usage degradation
	if observation.BaselineTokenUsage > 0 && observation.AvgTokenUsage > 0 {
		tokenDegradation := float64(observation.AvgTokenUsage-observation.BaselineTokenUsage) / float64(observation.BaselineTokenUsage)
		if tokenDegradation > GatePerformanceDegradationThreshold {
			return GateCheckResult{
				Name:   "performance",
				Passed: false,
				Reason: fmt.Sprintf("token usage degradation %.1f%% exceeds threshold %.0f%% (%d → %d)",
					tokenDegradation*100, GatePerformanceDegradationThreshold*100,
					observation.BaselineTokenUsage, observation.AvgTokenUsage),
			}
		}
	} else if observation.BaselineTokenUsage == 0 && observation.AvgTokenUsage > 10000 {
		return GateCheckResult{
			Name:   "performance",
			Passed: false,
			Reason: fmt.Sprintf("insufficient baseline data: avg token usage %d exceeds absolute threshold 10000", observation.AvgTokenUsage),
		}
	}

	return GateCheckResult{Name: "performance", Passed: true}
}

// verifyStyle checks the draft body for style compliance.
func (v *GateVerifier) verifyStyle(ctx context.Context, draftBody string) GateCheckResult {
	// Use lint checker if available
	if v.lintChecker != nil {
		passed, reason, err := v.lintChecker.LintCheck(ctx, draftBody)
		if err != nil {
			return GateCheckResult{
				Name:   "style",
				Passed: false,
				Reason: fmt.Sprintf("lint check error: %s", err.Error()),
			}
		}
		if !passed {
			return GateCheckResult{
				Name:   "style",
				Passed: false,
				Reason: reason,
			}
		}
		return GateCheckResult{Name: "style", Passed: true}
	}

	// Rule-based fallback: length check
	if len(draftBody) > GateMaxDraftLength {
		return GateCheckResult{
			Name:   "style",
			Passed: false,
			Reason: fmt.Sprintf("draft body length %d exceeds maximum %d characters", len(draftBody), GateMaxDraftLength),
		}
	}

	// Basic structure check: should contain at least a heading
	if !strings.Contains(draftBody, "#") {
		return GateCheckResult{
			Name:   "style",
			Passed: false,
			Reason: "draft body should contain at least one markdown heading",
		}
	}

	return GateCheckResult{Name: "style", Passed: true}
}
