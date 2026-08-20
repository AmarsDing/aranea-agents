package biz

import (
	"encoding/json"
	"time"
)

// EvolutionSuggestionType defines the type of skill evolution suggestion.
type EvolutionSuggestionType string

const (
	EvoSuggestionFixFailure      EvolutionSuggestionType = "fix_failure"
	EvoSuggestionBoostEfficiency EvolutionSuggestionType = "boost_efficiency"
	EvoSuggestionMergeDuplicate  EvolutionSuggestionType = "merge_duplicate"
	EvoSuggestionCreateSkill     EvolutionSuggestionType = "create_skill"
	// EvoSuggestionSuccessPattern 成功沉淀（P2 F3）：高成功率 skill 固化正向
	// 模式——强化有效规则、补充成功示例，防止好模式在后续全量重写中丢失。
	EvoSuggestionSuccessPattern EvolutionSuggestionType = "success_pattern"
)

// EvolutionSuggestionStatus defines the status of a skill evolution suggestion.
type EvolutionSuggestionStatus string

const (
	EvoSuggestionPending  EvolutionSuggestionStatus = "pending"
	EvoSuggestionApproved EvolutionSuggestionStatus = "approved"
	EvoSuggestionRejected EvolutionSuggestionStatus = "rejected"
	EvoSuggestionApplied  EvolutionSuggestionStatus = "applied"
)

// SkillEvolutionSuggestion represents a suggestion to evolve an existing skill.
// This is distinct from SkillProposal which proposes creating a NEW skill.
//
// Deprecated: Use UnifiedEvolutionSuggestion instead. For skill-level improvements,
// use UnifiedEvolutionSuggestion{TargetType: "skill", ActionType: "improve_skill"}.
// Status mapping: pending↔pending, approved↔approved, rejected↔rejected,
// applied↔registered (semantic equivalence: both mean "action executed").
// See also: skill_evolution_unified.go.
type SkillEvolutionSuggestion struct {
	ID              string
	SkillID         string
	Type            EvolutionSuggestionType
	Status          EvolutionSuggestionStatus
	SourceReportIDs []string
	TriggerReason   string
	DraftSkillBody  string          // LLM-generated draft of the new skill body
	DraftVersionID  string          // ID of the draft skill version (if created)
	SandboxPassed   bool            // Whether sandbox validation passed
	SandboxResult   json.RawMessage // Detailed sandbox validation results
	PreVerifyResult json.RawMessage // Pre-verification results (rule-based)
	ApprovedBy      string          // User who approved
	RejectedBy      string          // User who rejected
	RejectionReason string          // Reason for rejection
	CreatedAt       time.Time
	ResolvedAt      *time.Time // When approved/rejected/applied
	// Curator Agent evolution tracking fields
	ParentVersionID string                   // ID of the parent skill version this evolution is based on
	EvolutionReason string                   // Detailed reason for the evolution (populated by Curator Agent)
	LifecycleStatus EvolutionLifecycleStatus // Current lifecycle status: draft, validating, ready, applied
	// DraftOrigin records how DraftSkillBody was produced: DraftOriginLLM or
	// DraftOriginRuleTemplate (F8 — template degradation must be observable).
	DraftOrigin string
	// Target 维度（ADR-3）：skill 行 TargetType="skill" 且 TargetID==SkillID；
	// agent create_skill 行 TargetType="agent"、TargetID=agentID、DraftName 为
	// 拟注册技能名。空 TargetType 视为 "skill"（历史行兼容）。
	TargetType string
	TargetID   string
	DraftName  string
}

// Draft origin values for SkillEvolutionSuggestion.DraftOrigin (F8).
const (
	DraftOriginLLM          = "llm"
	DraftOriginRuleTemplate = "rule_template"
)

// EvolutionLifecycleStatus defines the lifecycle status of a skill evolution suggestion.
type EvolutionLifecycleStatus string

// EvolutionLifecycleStatus constants for SkillEvolutionSuggestion.LifecycleStatus.
const (
	EvoLifecycleDraft      EvolutionLifecycleStatus = "draft"      // Initial state after suggestion creation
	EvoLifecycleValidating EvolutionLifecycleStatus = "validating" // Sandbox validation in progress
	EvoLifecycleReady      EvolutionLifecycleStatus = "ready"      // Validation passed, ready for approval
	EvoLifecycleApplied    EvolutionLifecycleStatus = "applied"    // Suggestion has been applied to the skill
)

// EvoTriggerThresholds defines when evolution suggestions should be triggered.
const (
	EvoTriggerScoreThreshold = 60  // Score < 60 triggers suggestion
	EvoTriggerFailureRate    = 0.3 // 30d failure rate > 30% triggers suggestion
	EvoTriggerMinInvocations = 10  // Minimum invocations for statistical significance
	EvoTriggerCooldownHours  = 168 // Same skill: 7 days between suggestions

	// Curator Agent semi-automatic evolution triggers
	EvoTrigger7dSuccessRate    = 0.6 // 7d success rate < 60% triggers suggestion
	EvoTrigger7dMinInvocations = 5   // Minimum invocations in 7d window for significance
	EvoTriggerSameTagThreshold = 5   // Same failure tag >= 5 times triggers suggestion

	// SuccessTriggerSource 是 SuccessTrigger（P2 F3 成功沉淀）的触发来源标识，
	// 与 health 共用 (skill, improve_skill) 冷却槽（有意的保守，见设计 §5.2）。
	SuccessTriggerSource = "success"
	// SuccessTriggerSuccessRate 是成功沉淀的 30d 成功率阈值（≥ 触发）。
	SuccessTriggerSuccessRate = 0.85
)

// Pre-computed duration constants derived from EvoExpirationDays and EvoTriggerCooldownHours.
// These replace inline `EvoExpirationDays * 24 * time.Hour` and
// `EvoTriggerCooldownHours * time.Hour` expressions to eliminate magic-number arithmetic.
var (
	evoExpirationDuration = time.Duration(EvoExpirationDays) * 24 * time.Hour
	evoCooldownDuration   = time.Duration(EvoTriggerCooldownHours) * time.Hour
)
