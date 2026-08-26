// Package decision implements the unified decision record layer (M80):
// five categories of high-value decisions (HITL approval, planner routing,
// system guard, knowledge arbitration, evolution application) are normalized
// into queryable, auditable, traceable first-class assets.
//
// Data flow is one-way: decision source -> Collector -> decision_records ->
// query surface. No module reads the decision table for business judgment.
package decision

import (
	"fmt"
	"strings"
)

// Category enumerates the five decision sources (whitelist-validated on Emit).
type Category string

const (
	CategoryHITLApproval         Category = "hitl_approval"
	CategoryPlannerOrchestration Category = "planner_orchestration"
	CategorySystemGuard          Category = "system_guard"
	CategoryKnowledgeArbitration Category = "knowledge_arbitration"
	CategoryEvolutionApplied     Category = "evolution_applied"
)

// validCategories is the Emit-time whitelist (application-layer constraint per
// project convention, mirroring agent_tool_policy canon maps).
var validCategories = map[Category]bool{
	CategoryHITLApproval:         true,
	CategoryPlannerOrchestration: true,
	CategorySystemGuard:          true,
	CategoryKnowledgeArbitration: true,
	CategoryEvolutionApplied:     true,
}

// ActorType identifies who made the decision.
type ActorType string

const (
	ActorAgent  ActorType = "agent"
	ActorHuman  ActorType = "human"
	ActorSystem ActorType = "system"
)

// Field length caps (NFR: scenario truncates to 2000 chars; reasoning same).
const (
	maxScenarioLen  = 2000
	maxReasoningLen = 2000
	maxOutcomeLen   = 64
	maxActorKeyLen  = 128
)

// EntityRef is one related entity pointer; Key aligns with L4 entity-key
// conventions (e.g. tool:gns3_fault_inject, agent:ops_compliance_check).
type EntityRef struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// SourceRef points back at the fact source; fields are an on-demand subset.
type SourceRef struct {
	RunID            string `json:"run_id,omitempty"`
	StepID           string `json:"step_id,omitempty"`
	ToolInvocationID string `json:"tool_invocation_id,omitempty"`
	TwinApprovalID   string `json:"twin_approval_id,omitempty"`
	FlowTraceID      string `json:"flow_trace_id,omitempty"`
	SuggestionID     string `json:"suggestion_id,omitempty"`
	FactID           string `json:"fact_id,omitempty"`
	TaskID           string `json:"task_id,omitempty"`
}

// Record is the biz-layer contract for one decision (decoupled from ent).
// DecisionKey is the external UUID identifier; ID/ParentDecisionID are the
// internal int64 parent-chain (ADR-3 single-parent).
type Record struct {
	ID               int64
	DecisionKey      string
	Category         Category
	Scenario         string
	Reasoning        string
	Outcome          string
	Confidence       *float64
	ActorType        ActorType
	ActorKey         string
	ParentDecisionID *int64
	RelatedEntities  []EntityRef
	SourceRef        SourceRef
	Metadata         map[string]any
	WorkspaceID      string
	CreatedAt        string // RFC3339; set explicitly by caller-side collector
	UpdatedAt        string
	// VirtualParent 仅查询面注解（设计 §5 兜底补链）：链查询把同 run 内
	// 最近前置 planner 决策作为虚拟父返回时置 true，永不持久化。
	VirtualParent bool
}

// Validate checks the Emit-time contract: category whitelist plus the minimal
// field set every category must carry. Truncation happens separately in
// Normalize; Validate rejects rather than silently fixing so adapter bugs
// surface in tests.
func (r Record) Validate() error {
	if !validCategories[r.Category] {
		return fmt.Errorf("decision: unknown category %q", r.Category)
	}
	if strings.TrimSpace(r.DecisionKey) == "" {
		return fmt.Errorf("decision: decision_key is required")
	}
	if strings.TrimSpace(r.Outcome) == "" {
		return fmt.Errorf("decision: outcome is required")
	}
	switch r.ActorType {
	case ActorAgent, ActorHuman, ActorSystem:
	default:
		return fmt.Errorf("decision: unknown actor_type %q", r.ActorType)
	}
	if strings.TrimSpace(r.ActorKey) == "" {
		return fmt.Errorf("decision: actor_key is required")
	}
	if strings.TrimSpace(r.Scenario) == "" {
		return fmt.Errorf("decision: scenario is required")
	}
	return nil
}

// Normalize applies field caps in place (truncate, never fabricate). Called by
// the Collector before enqueue so every persisted row honors the caps.
func (r *Record) Normalize() {
	r.Scenario = truncateRunes(r.Scenario, maxScenarioLen)
	r.Reasoning = truncateRunes(r.Reasoning, maxReasoningLen)
	r.Outcome = truncateRunes(r.Outcome, maxOutcomeLen)
	r.ActorKey = truncateRunes(r.ActorKey, maxActorKeyLen)
}

// truncateRunes caps s at n runes (CJK-safe; scenario/reasoning are Chinese
// text in practice — byte-level truncation would split UTF-8 sequences).
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}
