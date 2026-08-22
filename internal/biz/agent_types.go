package biz

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
)

const (
	SpiritAgentKey      = "__spirit__"
	MemoryAgentKey      = "__memory__"
	SkillsAgentKey      = "__skills__"
	SystemAdminAgentKey = "__system_admin__"
	// VoiceButlerAgentKey 语音助手（M74 V9）：语音前台轻量 agent，快答/委派/播报，
	// 复杂任务经 delegate_to_spirit 异步委派精灵执行（设计 74 §15）。
	VoiceButlerAgentKey = "__voice_butler__"

	// Default compression trigger ratios (single source of truth).
	DefaultCompressionBufferRatio = 0.15
	DefaultSoftTriggerRatio       = 0.70
	DefaultHardTriggerRatio       = 0.90

	// DefaultToolsDenyFrameworkMemory lists the framework memory tools that are denied
	// by default for new agents (working_memory mode). Agents using "both" mode
	// should clear these from their ToolsDenyJSON.
	DefaultToolsDenyFrameworkMemory = `["memory_add","memory_update","memory_delete","memory_search","memory_load"]`
)

// IsSystemAgentKey reports whether the given key is a built-in system agent
// that should never participate in business task teams. System agents are
// infrastructure-level (spirit orchestrator, memory/skills/system admin) and
// must not be selected as team members by the allocator.
//
// 2026-07-04 问题 3 修复：统一系统 Agent 判断逻辑，避免每个过滤点重复写常量。
func IsSystemAgentKey(key string) bool {
	switch key {
	case SpiritAgentKey, SystemAdminAgentKey, MemoryAgentKey, SkillsAgentKey, VoiceButlerAgentKey:
		return true
	}
	return false
}

// IsCatalogAgentAssignable reports whether a catalog agent may be selected
// for matching, allocation, or team assembly. Inactive/archived/system
// agents stay in the directory but must not be assigned work.
//
// Department leads (agent_variant=dept_lead / __dept_lead_*__ keys) are
// governance roles, not business executors. Heuristic matching must not
// assign them as Lead or complementary members. Explicit plan_and_execute
// agent_keys may still name a dept_lead (AllocateExplicit bypasses this).
func IsCatalogAgentAssignable(a Agent) bool {
	if IsSystemAgentKey(a.AgentKey) {
		return false
	}
	if IsDeptLeadAgent(a) {
		return false
	}
	return NormalizeAgentStatus(a.Status) == AgentStatusActive
}

// AgentStatus enumerates the valid lifecycle statuses for a catalog agent.
// Using typed constants instead of free-form strings prevents invalid status values.
type AgentStatus string

const (
	AgentStatusActive   AgentStatus = "active"
	AgentStatusInactive AgentStatus = "inactive"
	AgentStatusArchived AgentStatus = "archived"
)

// ValidAgentStatuses is the set of all valid agent statuses.
var ValidAgentStatuses = map[AgentStatus]bool{
	AgentStatusActive:   true,
	AgentStatusInactive: true,
	AgentStatusArchived: true,
}

// ValidateAgentStatus returns an error if the given status string is not a valid AgentStatus.
func ValidateAgentStatus(status string) error {
	if status == "" {
		return nil // empty is allowed (defaults to active on creation)
	}
	if ValidAgentStatuses[AgentStatus(status)] {
		return nil
	}
	return apierror.BadRequest("AGENT", "invalid agent status: "+status)
}

// NormalizeAgentStatus returns the normalized AgentStatus, defaulting to active for empty.
func NormalizeAgentStatus(status string) AgentStatus {
	s := AgentStatus(strings.TrimSpace(strings.ToLower(status)))
	if s == "" {
		return AgentStatusActive
	}
	if ValidAgentStatuses[s] {
		return s
	}
	return AgentStatusActive
}

// agentEventForTarget determines the AgentEvent needed to transition to the given target state.
// This is used by AgentUsecase to validate status transitions via the state machine.
func agentEventForTarget(target AgentState) AgentEvent {
	switch target {
	case AgentStateActive:
		return AgentEventActivate
	case AgentStateInactive:
		return AgentEventDeactivate
	case AgentStateArchived:
		return AgentEventArchive
	default:
		return AgentEvent(target) // fallback; will fail transition validation
	}
}

// BoolVal dereferences a *bool, returning false for nil.
func BoolVal(p *bool) bool { return p != nil && *p }

// BoolEqual reports whether two *bool values are semantically equal.
// nil and false are treated as equivalent (both mean "not set / false").
func BoolEqual(a, b *bool) bool {
	return BoolVal(a) == BoolVal(b)
}

type Agent struct {
	ID                    string
	AgentKey              string
	DisplayName           string
	Provider              string
	Model                 string
	Status                string
	IsDefault             *bool // nil = not set (Proto3 zero-value ambiguity); explicit true/false for merge
	IsFavorite            *bool // nil = not set (Proto3 zero-value ambiguity); explicit true/false for merge
	Icon                  string
	AgentDescription      string
	PositionID            string
	PositionKey           string
	AgentVariant          string
	VariantDescription    string
	SystemPromptMode      string
	ContextWindow         int
	BudgetMonthlyCents    int
	ConfigJSON            string
	MetadataJSON          string
	Roles                 []string
	Kind                  string // user | system_builtin | ecosystem_preset | marketplace | certified (ownership, maps from DB kind column)
	AgentKind             string // llm | a2a_proxy (technical type, derived from config_json by HydrateAgentKind)
	A2AProxy              *A2AProxyConfig
	A2AEndpointEnabled    bool   // list/get enrichment from a2a_agent_cards.enabled
	LastRunStatus         string // list enrichment: latest session runtime.status or idle/completed
	LastRunAt             string
	PendingEvolutionCount int
	CreatedBy             string
	Readonly              bool
	Source                string // user | system | imported (origin, maps from DB source column)
	CreatedAt             string
	UpdatedAt             string
	DeletedAt             string
	Settings              *AgentRuntimeSettings
	Files                 []AgentPromptFile
	// CategoryResponsibilityPreview is a transient field populated by the prompt
	// preview handler to display the injected role_responsibility block.
	// It is never persisted to DB. PGO-1-BIZ-03.
	CategoryResponsibilityPreview string
	// WorkspaceID is the owning workspace ID for tenant isolation (P2-B).
	// empty = shared/legacy (visible to all workspaces); non-empty = tenant-private.
	WorkspaceID string
	// MissionStatement 是 Agent 的长期使命（出生登记于 AgentFactory，手工 Agent 可空）。
	// 匹配时为空回退 AgentDescription（不变量 2）。
	MissionStatement string
	// DomainPath 是归一化领域路径（如 "创作/文学"），空 = 未分类（走旧匹配管线）。
	DomainPath string
}

// SkipCategoryResponsibility returns true when the agent's metadata_json
// contains {"skip_category_responsibility": true}, allowing power users to
// opt out of the automatic L1 岗位职责 injection. PGO-1-BIZ-05.
func (a Agent) SkipCategoryResponsibility() bool {
	if strings.TrimSpace(a.MetadataJSON) == "" {
		return false
	}
	var m struct {
		Skip bool `json:"skip_category_responsibility"`
	}
	if err := json.Unmarshal([]byte(a.MetadataJSON), &m); err != nil {
		return false
	}
	return m.Skip
}

// AgentPromptFile is one row in agent_prompt_files (API name field maps to file_name).
type AgentPromptFile struct {
	ID        string
	AgentID   string
	Name      string
	Body      string
	SortOrder int
	CreatedAt string
	UpdatedAt string
}

// AgentListQuery filters the agent catalog list.
type AgentListQuery struct {
	Keyword   string
	Status    string
	Provider  string
	OrgNodeID string
	CreatedBy string
	Role      string
	Kind      string // filter by ownership classification (user | system_builtin | ecosystem_preset | marketplace | certified)
	// WorkspaceID filters by tenant visibility (P2-B):
	// empty = system caller (see all); non-empty = tenant caller (see shared + own).
	WorkspaceID string
	Limit       int
	Offset      int
}

// AgentCreator is a distinct agent creator for list filters.
type AgentCreator struct {
	UserID string
	Label  string
}

// AgentListResult is a page of agents without per-row hydration unless noted.
type AgentListResult struct {
	Items  []Agent
	Total  int
	Limit  int
	Offset int
}

// FileTokenEstimate is the token estimate for a single prompt file.
type FileTokenEstimate struct {
	FileID          string
	FileName        string
	EstimatedTokens int
}

// FileTokenEstimates is the aggregate token estimate for all prompt files of an agent.
type FileTokenEstimates struct {
	TotalTokens   int
	FileEstimates []FileTokenEstimate
}
