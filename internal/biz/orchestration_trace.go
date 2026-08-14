// Package biz — Orchestration trace MAST annotation trigger (P3-1, 编排自进化).
//
// Reads terminal (failed/cancelled) orchestrations plus their flow-log error
// aggregates, annotates each trace with a MAST failure mode (Multi-Agent
// System Failure taxonomy, 14 modes in 3 categories), clusters by mode, and
// emits one pending platform suggestion per cluster. User/parent-cascaded
// cancellations and completed runs are excluded — they are not failures.
//
// P3-2 反哺映射：FM-1.x/2.x（规范/对齐类）→ patch_prompt；FM-3.x（验证/终止
// 类）→ tune_config。既有管线按 ActionType 路由到对应 Patcher，风险分级走
// 既有 approval 通道（先人工评审后应用）。
package biz

import (
	"context"
	"fmt"
	"sort"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── MAST vocabulary (P3-1) ───────────────────────────────────────────────────

// MASTFailureMode is one of the 14 Multi-Agent System failure modes.
type MASTFailureMode string

const (
	// Category 1: Specification & system design failures.
	MASTDisobeyTaskSpec     MASTFailureMode = "FM-1.1" // disobey_task_specification
	MASTDisobeyRoleSpec     MASTFailureMode = "FM-1.2" // disobey_role_specification
	MASTStepRepetition      MASTFailureMode = "FM-1.3" // step_repetition
	MASTLossOfHistory       MASTFailureMode = "FM-1.4" // loss_of_conversation_history
	MASTUnawareTermination  MASTFailureMode = "FM-1.5" // unaware_of_termination_conditions
	// Category 2: Inter-agent misalignment.
	MASTConversationReset   MASTFailureMode = "FM-2.1"
	MASTNoClarification     MASTFailureMode = "FM-2.2"
	MASTTaskDerailment      MASTFailureMode = "FM-2.3"
	MASTInfoWithholding     MASTFailureMode = "FM-2.4"
	MASTIgnoredAgentInput   MASTFailureMode = "FM-2.5"
	MASTReasoningActionGap  MASTFailureMode = "FM-2.6"
	// Category 3: Verification & termination failures.
	MASTPrematureTermination  MASTFailureMode = "FM-3.1"
	MASTNoVerification        MASTFailureMode = "FM-3.2"
	MASTIncorrectVerification MASTFailureMode = "FM-3.3"
)

// MASTCategory groups failure modes into the three MAST categories.
type MASTCategory string

const (
	MASTCategorySpecification MASTCategory = "specification" // FM-1.x
	MASTCategoryMisalignment  MASTCategory = "misalignment"  // FM-2.x
	MASTCategoryVerification  MASTCategory = "verification"  // FM-3.x
)

// mastCategoryOf maps a failure mode to its category.
func mastCategoryOf(m MASTFailureMode) MASTCategory {
	switch m {
	case MASTDisobeyTaskSpec, MASTDisobeyRoleSpec, MASTStepRepetition, MASTLossOfHistory, MASTUnawareTermination:
		return MASTCategorySpecification
	case MASTConversationReset, MASTNoClarification, MASTTaskDerailment, MASTInfoWithholding, MASTIgnoredAgentInput, MASTReasoningActionGap:
		return MASTCategoryMisalignment
	default:
		return MASTCategoryVerification
	}
}

// mastModeTitle 中文标题（流程日志/建议 reason 展示用）。
func mastModeTitle(m MASTFailureMode) string {
	switch m {
	case MASTDisobeyTaskSpec:
		return "违背任务规范"
	case MASTDisobeyRoleSpec:
		return "违背角色规范"
	case MASTStepRepetition:
		return "步骤重复"
	case MASTLossOfHistory:
		return "对话历史丢失"
	case MASTUnawareTermination:
		return "不知终止条件"
	case MASTConversationReset:
		return "对话重置"
	case MASTNoClarification:
		return "未请求澄清"
	case MASTTaskDerailment:
		return "任务脱轨"
	case MASTInfoWithholding:
		return "信息隐瞒"
	case MASTIgnoredAgentInput:
		return "忽略他方输入"
	case MASTReasoningActionGap:
		return "推理-行动不匹配"
	case MASTPrematureTermination:
		return "过早终止"
	case MASTNoVerification:
		return "无/不完整验证"
	case MASTIncorrectVerification:
		return "错误验证"
	}
	return string(m)
}

// ── Signal types ─────────────────────────────────────────────────────────────

// OrchestrationTrace is one terminal orchestration plus its failure signals
// aggregated from flow_log_events by trace_id.
type OrchestrationTrace struct {
	OrchestrationID string
	SpiritSessionID string
	TraceID         string
	Strategy        string
	Status          string // failed / cancelled / interrupted
	CancelReason    string // P2-6 typed reason
	TeamCount       int
	DurationMS      int64
	ErrorSteps      map[string]int // step_id → error count（severity=error 聚合）
	WarnCount       int
	LastError       string
	UpdatedAt       time.Time
}

// OrchestrationTraceReader reads terminal orchestrations with flow-log
// aggregates for the observation window.
// Stability:evolving
type OrchestrationTraceReader interface {
	ListTerminalOrchestrationTraces(ctx context.Context, since time.Time, limit int) ([]OrchestrationTrace, error)
}

// ── Rule-based annotation ────────────────────────────────────────────────────

// MASTAnnotation is the deterministic rule-chain labeling result.
type MASTAnnotation struct {
	Mode       MASTFailureMode
	Category   MASTCategory
	Confidence float64
	Evidence   string
}

// orchestrationTraceRepeatThreshold 是同一 step 错误重复判定阈值（FM-1.3）。
const orchestrationTraceRepeatThreshold = 3

// orchestrationTracePrematureMS 是 team 类策略快速失败判定阈值（FM-3.1）。
// direct/single_agent 策略快速完成属正常，不适用此规则。
const orchestrationTracePrematureMS = 30_000

// AnnotateOrchestrationTrace applies the deterministic rule chain (first match
// wins, highest-confidence rules first). Returns nil for non-failures
// (completed) and user-initiated cancellations.
func AnnotateOrchestrationTrace(t OrchestrationTrace) *MASTAnnotation {
	// 非失败语义直接排除：completed 成功；user/parent 取消是用户意图不是系统失败。
	if t.Status == string(OrchestrationStatusCompleted) {
		return nil
	}
	if t.Status == string(OrchestrationStatusCancelled) {
		switch NormalizeCancelReason(t.CancelReason) {
		case CancelReasonUser, CancelReasonParent, CancelReasonUnknown:
			return nil
		}
	}

	// R1: doom_loop 取消 → FM-1.3 步骤重复（最高置信：检测器显式判定）。
	if NormalizeCancelReason(t.CancelReason) == CancelReasonDoomLoop {
		return &MASTAnnotation{
			Mode:       MASTStepRepetition,
			Category:   MASTCategorySpecification,
			Confidence: 0.95,
			Evidence:   "cancel_reason=doom_loop",
		}
	}

	// R2: 同一 step 错误重复 ≥ 阈值 → FM-1.3。
	maxStep, maxCount := "", 0
	for step, n := range t.ErrorSteps {
		if n > maxCount {
			maxStep, maxCount = step, n
		}
	}
	if maxCount >= orchestrationTraceRepeatThreshold {
		return &MASTAnnotation{
			Mode:       MASTStepRepetition,
			Category:   MASTCategorySpecification,
			Confidence: 0.8,
			Evidence:   fmt.Sprintf("step %q errored %d times", maxStep, maxCount),
		}
	}

	// R3: 超时取消 → FM-1.5 不知终止条件。
	if NormalizeCancelReason(t.CancelReason) == CancelReasonTimeout {
		return &MASTAnnotation{
			Mode:       MASTUnawareTermination,
			Category:   MASTCategorySpecification,
			Confidence: 0.9,
			Evidence:   "cancel_reason=timeout",
		}
	}

	// R4: 0 team 的 setup 失败 → FM-1.1 违背任务规范（规划/分配未产出）。
	if t.Status == string(OrchestrationStatusFailed) && t.TeamCount == 0 {
		return &MASTAnnotation{
			Mode:       MASTDisobeyTaskSpec,
			Category:   MASTCategorySpecification,
			Confidence: 0.7,
			Evidence:   "setup failed: 0 teams created",
		}
	}

	// R5: team 类策略快速失败 → FM-3.1 过早终止。
	if t.Status == string(OrchestrationStatusFailed) && t.TeamCount > 0 &&
		t.DurationMS > 0 && t.DurationMS < orchestrationTracePrematureMS {
		return &MASTAnnotation{
			Mode:       MASTPrematureTermination,
			Category:   MASTCategoryVerification,
			Confidence: 0.6,
			Evidence:   fmt.Sprintf("failed after %dms with %d teams", t.DurationMS, t.TeamCount),
		}
	}

	// R6: 兜底——failed 但无明确信号 → FM-3.2 低置信观测。
	if t.Status == string(OrchestrationStatusFailed) {
		return &MASTAnnotation{
			Mode:       MASTNoVerification,
			Category:   MASTCategoryVerification,
			Confidence: 0.3,
			Evidence:   "failed without distinctive failure signals",
		}
	}
	return nil
}

// ── Trigger ──────────────────────────────────────────────────────────────────

// TriggerSourceOrchestrationTrace is the platform trigger source for
// orchestration-trace MAST annotations (P3-1).
const TriggerSourceOrchestrationTrace = "orchestration_trace"

// Defaults.
const (
	SIDefaultTraceWindowHours = 24
	SIDefaultTraceScanLimit   = 200
	siTraceMaxSamplesPerMode  = 5
)

// OrchestrationTraceTrigger annotates terminal orchestration traces with MAST
// failure modes and emits one clustered suggestion per mode (P3-1).
type OrchestrationTraceTrigger struct {
	reader OrchestrationTraceReader
	window time.Duration
	limit  int
	lg     loggateway.Logger
}

// NewOrchestrationTraceTrigger creates the trigger; windowHours/limit <= 0
// fall back to defaults (24h / 200).
func NewOrchestrationTraceTrigger(reader OrchestrationTraceReader, windowHours, limit int, lg loggateway.Logger) *OrchestrationTraceTrigger {
	window := time.Duration(windowHours) * time.Hour
	if window <= 0 {
		window = SIDefaultTraceWindowHours * time.Hour
	}
	if limit <= 0 {
		limit = SIDefaultTraceScanLimit
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &OrchestrationTraceTrigger{reader: reader, window: window, limit: limit, lg: lg}
}

// TargetType implements EvolutionTrigger.
func (t *OrchestrationTraceTrigger) TargetType() EvolutionTargetType { return EvolutionTargetPlatform }

// ActionType implements EvolutionTrigger (primary; per-suggestion overrides by
// MAST category in Check).
func (t *OrchestrationTraceTrigger) ActionType() EvolutionActionType { return EvolutionActionPatchPrompt }

// TriggerSource implements EvolutionTrigger.
func (t *OrchestrationTraceTrigger) TriggerSource() string { return TriggerSourceOrchestrationTrace }

// mastActionType maps the MAST category to the remediation action (P3-2):
// specification/misalignment → prompt 修复；verification → 配置调优。
func mastActionType(c MASTCategory) EvolutionActionType {
	if c == MASTCategoryVerification {
		return EvolutionActionTuneConfig
	}
	return EvolutionActionPatchPrompt
}

// Check implements EvolutionTrigger. Aggregates traces by dominant MAST mode
// and emits one suggestion per mode cluster (ErrorClusterTrigger 同语义：
// 建议代表失败模式而非单次失败，重复同类失败经签名去重收敛为一条)。
func (t *OrchestrationTraceTrigger) Check(ctx context.Context, _ string) ([]UnifiedEvolutionSuggestion, error) {
	if t == nil || t.reader == nil {
		return nil, nil
	}
	since := time.Now().UTC().Add(-t.window)
	traces, err := t.reader.ListTerminalOrchestrationTraces(ctx, since, t.limit)
	if err != nil {
		return nil, err
	}

	type cluster struct {
		mode       MASTFailureMode
		category   MASTCategory
		confidence float64
		count      int
		samples    []string
		evidence   []string
	}
	clusters := map[MASTFailureMode]*cluster{}
	for _, tr := range traces {
		a := AnnotateOrchestrationTrace(tr)
		if a == nil {
			continue
		}
		c := clusters[a.Mode]
		if c == nil {
			c = &cluster{mode: a.Mode, category: a.Category, confidence: a.Confidence}
			clusters[a.Mode] = c
		}
		c.count++
		if a.Confidence > c.confidence {
			c.confidence = a.Confidence
		}
		if len(c.samples) < siTraceMaxSamplesPerMode {
			c.samples = append(c.samples, tr.OrchestrationID)
			c.evidence = append(c.evidence, a.Evidence)
		}
	}
	if len(clusters) == 0 {
		return nil, nil
	}

	// 稳定排序：count 降序 → mode 升序，保证多次扫描输出一致。
	modes := make([]MASTFailureMode, 0, len(clusters))
	for m := range clusters {
		modes = append(modes, m)
	}
	sort.Slice(modes, func(i, j int) bool {
		ci, cj := clusters[modes[i]], clusters[modes[j]]
		if ci.count != cj.count {
			return ci.count > cj.count
		}
		return modes[i] < modes[j]
	})

	suggestions := make([]UnifiedEvolutionSuggestion, 0, len(modes))
	for _, m := range modes {
		c := clusters[m]
		sig := siSignature(TriggerSourceOrchestrationTrace, string(m))
		priority := 1
		if c.confidence >= 0.8 || c.count >= 3 {
			priority = 2
		}
		reason := fmt.Sprintf("编排轨迹 MAST 标注：%s（%s）窗口内 %d 次，最高置信 %.2f",
			m, mastModeTitle(m), c.count, c.confidence)
		suggestions = append(suggestions, buildPlatformSuggestion(
			TriggerSourceOrchestrationTrace, mastActionType(c.category), sig, reason, priority,
			map[string]any{
				"mast_mode":                string(m),
				"mast_mode_title":          mastModeTitle(m),
				"mast_category":            string(c.category),
				"cluster_count":            c.count,
				"max_confidence":           c.confidence,
				"sample_orchestration_ids": c.samples,
				"sample_evidence":          c.evidence,
				"window_hours":             int(t.window.Hours()),
			},
		))
	}
	return suggestions, nil
}
