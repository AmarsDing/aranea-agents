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
}

// EvoTriggerThresholds defines when evolution suggestions should be triggered.
const (
	EvoTriggerScoreThreshold = 60  // Score < 60 triggers suggestion
	EvoTriggerFailureRate    = 0.3 // 30d failure rate > 30% triggers suggestion
	EvoTriggerMinInvocations = 10  // Minimum invocations for statistical significance
	EvoTriggerCooldownHours  = 168 // Same skill: 7 days between suggestions
)
