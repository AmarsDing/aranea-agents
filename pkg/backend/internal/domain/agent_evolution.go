// Package domain – 智能体自进化领域类型，见 `aranea/docs/16 memory-L4-persistent.md` §3.2、§4.2。
// 这些结构表示智能体稳定身份、策略 / 偏好画像、不可变进化事件日志、待处理提案队列及按工具的技能统计。
package domain

import mem "arenea/backend/internal/memory/domain"

// 持久化在 `agent_identity.current_phase` 的智能体身份阶段。
const (
	AgentPhaseColdStart   = "cold-start"
	AgentPhaseWarming     = "warming"
	AgentPhaseMature      = "mature"
	AgentPhaseSpecialized = "specialized"
)

// 持久化在 `agent_identity.tone` 的语气取值。
const (
	AgentToneFormal   = "formal"
	AgentToneCasual   = "casual"
	AgentTonePlayful  = "playful"
	AgentToneStrict   = "strict"
	AgentToneAcademic = "academic"
)

// 持久化在 `agent_evolution_events` 的触发种类与事件种类。
const (
	EvoTriggerAuto     = "auto"
	EvoTriggerProposal = "proposal"
	EvoTriggerUser     = "user"
	EvoTriggerCritic   = "critic"
	EvoTriggerPlugin   = "plugin"
	EvoTriggerRollback = "rollback"

	EvoKindIdentityUpdate      = "identity_update"
	EvoKindPersonaUpdate       = "persona_update"
	EvoKindToneChange          = "tone_change"
	EvoKindSystemPromptAppend  = "system_prompt_append"
	EvoKindSystemPromptReplace = "system_prompt_replace"
	EvoKindToolEnable          = "tool_enable"
	EvoKindToolDisable         = "tool_disable"
	EvoKindToolPrefUpdate      = "tool_pref_update"
	EvoKindProviderPrefUpdate  = "provider_pref_update"
	EvoKindModelPrefUpdate     = "model_pref_update"
	EvoKindStrategyParamUpdate = "strategy_param_update"
	EvoKindDomainAdded         = "domain_added"
	EvoKindPhaseChange         = "phase_change"
	EvoKindRollback            = "rollback"
	EvoKindRestore             = "restore"
)

// 持久化在 `agent_evolution_proposals.status` 的提案生命周期状态。
const (
	EvoProposalPending    = "pending"
	EvoProposalApproved   = "approved"
	EvoProposalRejected   = "rejected"
	EvoProposalApplied    = "applied"
	EvoProposalSuperseded = "superseded"
	EvoProposalExpired    = "expired"
)

// 持久化在 `agent_evolution_proposals.risk_level` 的风险级别。
const (
	EvoRiskLow    = "low"
	EvoRiskMedium = "medium"
	EvoRiskHigh   = "high"
)

// 持久化在 `agent_evolution_proposals.source` 的提案来源取值。
const (
	EvoSourceConsolidator  = "consolidator"
	EvoSourceCritic        = "critic"
	EvoSourceRuntimeSignal = "runtime_signal"
	EvoSourceUser          = "user"
)

// AgentIdentity 为 `agent_identity` 表的持久化行，描述智能体稳定人设、价值观、语气与生命周期阶段。
type AgentIdentity struct {
	AgentID          string         `json:"agent_id"`
	Persona          string         `json:"persona"`
	Values           []string       `json:"values"`
	Tone             string         `json:"tone,omitempty"`
	Domains          []string       `json:"domains"`
	UserExpectations string         `json:"user_expectations,omitempty"`
	CurrentPhase     string         `json:"current_phase"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Version          int            `json:"version"`
	CreatedAt        string         `json:"created_at,omitempty"`
	UpdatedAt        string         `json:"updated_at,omitempty"`
}

// AgentStrategyProfile 为 `agent_strategy_profile` 表的持久化行，描述智能体决策风格及工具 / 模型 / 提供商偏好。
// 所有标量字段取值于 [0,1]。
type AgentStrategyProfile struct {
	AgentID            string             `json:"agent_id"`
	Exploration        float64            `json:"exploration"`
	Conciseness        float64            `json:"conciseness"`
	Caution            float64            `json:"caution"`
	Delegation         float64            `json:"delegation"`
	ToolPreference     map[string]float64 `json:"tool_preference,omitempty"`
	ToolBlacklist      []string           `json:"tool_blacklist,omitempty"`
	ProviderPreference map[string]float64 `json:"provider_preference,omitempty"`
	ModelPreference    map[string]float64 `json:"model_preference,omitempty"`
	Stats              map[string]any     `json:"stats,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
	Version            int                `json:"version"`
	CreatedAt          string             `json:"created_at,omitempty"`
	UpdatedAt          string             `json:"updated_at,omitempty"`
}

// EvolutionEvent 为 `agent_evolution_events` 中不可变历史行。保留 `BeforeJSON` 供回滚。
type EvolutionEvent struct {
	ID                string         `json:"id"`
	AgentID           string         `json:"agent_id"`
	WorkspaceID       string         `json:"workspace_id,omitempty"`
	Kind              string         `json:"event_kind"`
	TargetField       string         `json:"target_field,omitempty"`
	BeforeJSON        string         `json:"before_json,omitempty"`
	AfterJSON         string         `json:"after_json,omitempty"`
	DiffJSON          string         `json:"diff_json,omitempty"`
	TriggerKind       string         `json:"trigger_kind"`
	TriggerSource     string         `json:"trigger_source,omitempty"`
	Evidence          []mem.EvidenceRef `json:"evidence,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	Applied           bool           `json:"applied"`
	Reverted          bool           `json:"reverted"`
	RevertedByEventID string         `json:"reverted_by_event_id,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         string         `json:"created_at,omitempty"`
	AppliedAt         string         `json:"applied_at,omitempty"`
	RevertedAt        string         `json:"reverted_at,omitempty"`
}

// EvolutionProposal 为 `agent_evolution_proposals` 中的候选变更行。
// 已批准提案通过 `AppliedEventID` 关联产生的 EvolutionEvent。
type EvolutionProposal struct {
	ID                string         `json:"id"`
	AgentID           string         `json:"agent_id"`
	WorkspaceID       string         `json:"workspace_id,omitempty"`
	Kind              string         `json:"proposal_kind"`
	TargetField       string         `json:"target_field,omitempty"`
	ProposedValueJSON string         `json:"proposed_value_json,omitempty"`
	CurrentValueJSON  string         `json:"current_value_json,omitempty"`
	DiffJSON          string         `json:"diff_json,omitempty"`
	Rationale         string         `json:"rationale,omitempty"`
	Evidence          []mem.EvidenceRef `json:"evidence,omitempty"`
	ExpectedImpact    string         `json:"expected_impact,omitempty"`
	RiskLevel         string         `json:"risk_level"`
	ApprovalRequired  bool           `json:"approval_required"`
	Status            string         `json:"status"`
	ReviewedBy        string         `json:"reviewed_by,omitempty"`
	ReviewedAt        string         `json:"reviewed_at,omitempty"`
	AppliedEventID    string         `json:"applied_event_id,omitempty"`
	ExpiresAt         string         `json:"expires_at,omitempty"`
	Source            string         `json:"source"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         string         `json:"created_at,omitempty"`
	UpdatedAt         string         `json:"updated_at,omitempty"`
}

// AgentSkillStat 为 `agent_skill_stats` 中按工具的技能遥测行。
// EvolutionWorker 用它推导工具偏好 / 黑名单提案。
type AgentSkillStat struct {
	AgentID         string         `json:"agent_id"`
	Scope           string         `json:"scope"`
	ScopeValue      string         `json:"scope_value,omitempty"`
	ToolKey         string         `json:"tool_key"`
	Invocations     int            `json:"invocations"`
	Successes       int            `json:"successes"`
	Failures        int            `json:"failures"`
	UserOverrides   int            `json:"user_overrides"`
	AvgLatencyMS    float64        `json:"avg_latency_ms"`
	AvgTokens       float64        `json:"avg_tokens"`
	PreferenceScore float64        `json:"preference_score"`
	LastUsedAt      string         `json:"last_used_at,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	UpdatedAt       string         `json:"updated_at,omitempty"`
}
