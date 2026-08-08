package biz

import (
	"encoding/json"
	"time"
)

// ── Platform self-improvement enums (V3) ─────────────────────────────────────
//
// The unified evolution enums are extended for the platform target. The
// suggestions table stores these as plain TEXT (no CHECK constraint), so the
// extension is code-only.

const (
	// EvolutionTargetPlatform marks a suggestion produced by the platform
	// self-improvement (V3) pipeline.
	EvolutionTargetPlatform EvolutionTargetType = "platform"
)

const (
	EvolutionActionPatchCode   EvolutionActionType = "patch_code"
	EvolutionActionTuneConfig  EvolutionActionType = "tune_config"
	EvolutionActionPatchPrompt EvolutionActionType = "patch_prompt"
)

// Platform trigger sources (UnifiedEvolutionSuggestion.TriggerSource).
const (
	TriggerSourceErrorCluster   = "error_cluster"
	TriggerSourcePerfBottleneck = "perf_bottleneck"
	TriggerSourceEvalRegression = "eval_regression"
	TriggerSourceTestFailure    = "test_failure"
)

// EvoMetaTriggerSignature is the metadata key holding the dedup signature hash
// of a platform suggestion. The pending-dedup unique index reads
// metadata.pattern_hash, so platform suggestions mirror the signature into
// EvoMetaPatternHash as well (per-signature dedup for free).
const EvoMetaTriggerSignature = "trigger_signature"

// ── Patch classification ─────────────────────────────────────────────────────

// SelfImprovementPatchKind classifies a patch for the apply channel.
type SelfImprovementPatchKind string

const (
	PatchKindCode   SelfImprovementPatchKind = "code"
	PatchKindConfig SelfImprovementPatchKind = "config"
	PatchKindPrompt SelfImprovementPatchKind = "prompt"
	PatchKindDocs   SelfImprovementPatchKind = "docs"
	PatchKindTest   SelfImprovementPatchKind = "test"
)

// SelfImprovementRiskLevel is the governance risk tier (design D6).
type SelfImprovementRiskLevel string

const (
	RiskLevelLow    SelfImprovementRiskLevel = "low"
	RiskLevelMedium SelfImprovementRiskLevel = "medium"
	RiskLevelHigh   SelfImprovementRiskLevel = "high"
)

// SelfImprovementVerdict is the outcome attribution of a closed run (D8).
type SelfImprovementVerdict string

const (
	VerdictEffective SelfImprovementVerdict = "effective"
	VerdictNeutral   SelfImprovementVerdict = "neutral"
	VerdictRegressed SelfImprovementVerdict = "regressed"
)

// ── JSON sub-structures ──────────────────────────────────────────────────────

// DiffStats summarizes a unified diff.
type DiffStats struct {
	Files     int `json:"files"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// Diagnosis is the Analyst Agent structured output (D5).
type Diagnosis struct {
	RootCause     string   `json:"root_cause"`
	AffectedFiles []string `json:"affected_files"`
	ImpactScope   string   `json:"impact_scope"` // local / module / global
	FixStrategy   string   `json:"fix_strategy"`
	Confidence    float64  `json:"confidence"` // 0-1；<0.5 降级为仅记录
}

// SandboxGateKind identifies one sandbox verification gate.
// (Prefixed to avoid collision with verification_gate.go's GateResult.)
type SandboxGateKind string

const (
	SandboxGateBuild  SandboxGateKind = "g1_build"
	SandboxGateTest   SandboxGateKind = "g2_test"
	SandboxGateLint   SandboxGateKind = "g3_lint"
	SandboxGateCritic SandboxGateKind = "g4_critic"
	// SandboxGateEvalBase（G5 评估基线）当前未真实执行：pipeline 在 G1-G3
	// 全过后落一条 passed/skipped 记录保持控制台透明（design D4 注记）。
	SandboxGateEvalBase SandboxGateKind = "g5_eval"
)

// SandboxGateResult is the outcome of a single verification gate.
type SandboxGateResult struct {
	Gate       SandboxGateKind `json:"gate"`
	Passed     bool            `json:"passed"`
	Output     string          `json:"output"` // 截断 64KB
	DurationMS int64           `json:"duration_ms"`
}

// CriticReport is the Critic Agent structured review (V2 契约复用).
type CriticReport struct {
	IsSafe     bool     `json:"is_safe"`
	RiskLevel  string   `json:"risk_level"` // low / medium / high
	Concerns   []string `json:"concerns"`
	Suggestion string   `json:"suggestion"`
}

// GovernanceDecision is the RiskClassifier output (D6).
type GovernanceDecision struct {
	RiskLevel SelfImprovementRiskLevel `json:"risk_level"`
	Channel   string                   `json:"channel"` // auto / notify / approval / reject
	RuleHits  []string                 `json:"rule_hits"`
}

// MetricsSnapshot is a 1h sliding-window metrics view for the observe window.
type MetricsSnapshot struct {
	ErrorRate  float64 `json:"error_rate"`
	P95MS      float64 `json:"p95_ms"`
	AlertCount int     `json:"alert_count"`
}

// ── SelfImprovementRun ───────────────────────────────────────────────────────

// SelfImprovementRun is one execution instance of the seven-stage loop, bound
// 1:1 to a UnifiedEvolutionSuggestion (target_type=platform).
type SelfImprovementRun struct {
	ID                 string
	SuggestionID       string
	Status             SelfImprovementRunStatus
	TriggerSource      string
	PatchKind          SelfImprovementPatchKind
	RiskLevel          SelfImprovementRiskLevel
	BaseRef            string
	Branch             string
	WorktreePath       string
	Diff               string
	DiffStats          DiffStats
	Diagnosis          *Diagnosis
	VerificationReport []SandboxGateResult
	CriticReport       *CriticReport
	Governance         *GovernanceDecision
	Attempts           int
	ApprovedBy         string
	AppliedCommit      string
	RollbackPointer    string
	ObserveUntil       *time.Time
	ClosedReason       string
	Metadata           json.RawMessage
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// PatchOutcome is the terminal attribution record of a run (D8).
type PatchOutcome struct {
	ID             string
	RunID          string
	SuggestionID   string
	Verdict        SelfImprovementVerdict
	MetricsBefore  *MetricsSnapshot
	MetricsAfter   *MetricsSnapshot
	RollbackReason string
	PatternHash    string
	CreatedAt      time.Time
}

// RunFilter filters the run list query.
type RunFilter struct {
	Status SelfImprovementRunStatus
	// Statuses 多状态 IN 过滤（worker 侧按职责域圈选，如 drive 只驱动 6 个
	// 中途态，避免每 tick 全表扫描含重 JSON 字段的终态行）。与 Status 叠加（AND）。
	Statuses      []SelfImprovementRunStatus
	RiskLevel     SelfImprovementRiskLevel
	TriggerSource string
	Limit         int
	Offset        int
}
