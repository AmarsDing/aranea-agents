package biz

import "context"

// AgentFactory dynamically creates Agents when 4-layer matching fails.
// Implementations live in internal/agent. Wire binding in internal/service.
//
// Stability:evolving
type AgentFactory interface {
	// EnsureAgent returns an agent_key suitable for the given TaskProfile.
	// If a matching Agent already exists, it is reused (idempotent by AgentKey).
	// Otherwise a new Agent is generated via LLM, persisted with Source="system",
	// and an EnvelopeTypeAgentCreated event is published.
	EnsureAgent(ctx context.Context, profile TaskProfile) (string, error)
}

// TaskProfile describes the task requirements that drive AgentFactory
// generation. It is defined in biz to avoid import cycles (internal/agent
// depends on internal/biz, not the reverse).
type TaskProfile struct {
	// RequiredCapabilities lists the capability tags the Agent must cover
	// (e.g. "go-backend", "vue3-frontend"). Used for template selection and
	// LLM prompt context.
	RequiredCapabilities []string
	// Domain is the broad problem domain (e.g. "engineering", "research").
	Domain string
	// TaskDescription is the natural-language task that triggered creation.
	TaskDescription string
	// PreferredTools lists tool keys the Agent should be configured with.
	PreferredTools []string
	// PreferredModel is an optional provider/model hint (e.g. "gpt-4.1-mini").
	// Empty falls back to the default catalog model.
	PreferredModel string
	// SpiritSessionID routes orchestration progress events to the owning
	// Spirit session (P-ORCH). Empty disables progress event publishing.
	SpiritSessionID string
	// DomainPath 是任务归一化领域路径。非空时 AgentKey 按 domain 派生
	// （同域同模型复用同一 Agent）；空走旧 key 行为（兼容兜底）。
	DomainPath string
	// Mission 是任务使命提示文本（用于同域使命相似度复用检查）。空回退 TaskDescription。
	Mission string
	// DepartmentID is the org-pruned home department (M78). When set,
	// EnsureAgent occupies a position under this department. Empty keeps
	// the pre-M78 factory behavior (no position).
	DepartmentID string
	// PositionID optionally pins the birth position. Empty → pick a default
	// position under DepartmentID.
	PositionID string
}
