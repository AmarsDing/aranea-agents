package biz

import (
	"context"
	"encoding/json"
	"time"
)

// RuntimeProfile is a per-agent runtime configuration profile that overrides
// agent defaults at run time. It maps to the framework's runtimeprofile.Profile
// but lives in the biz layer (no framework dependency).
type RuntimeProfile struct {
	ID               string
	AgentID          string
	Name             string
	Description      string
	Version          string
	IsActive         bool
	Priority         int
	PromptConfig     PromptConfig
	ToolPolicy       ToolPolicy
	SkillPolicy      SkillPolicy
	KnowledgePolicy  KnowledgePolicy
	WorkspacePolicy  WorkspacePolicy
	CredentialPolicy CredentialPolicy
	IsolationPolicy  IsolationPolicy
	ExtraModelConfig map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PromptConfig defines prompt overrides for one run.
type PromptConfig struct {
	Instruction  string `json:"instruction,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// ToolPolicy defines name-based tool visibility and execution policy.
type ToolPolicy struct {
	Include          []string          `json:"include,omitempty"`
	Exclude          []string          `json:"exclude,omitempty"`
	ExecutionInclude []string          `json:"execution_include,omitempty"`
	ExecutionExclude []string          `json:"execution_exclude,omitempty"`
	ToolSets         []string          `json:"toolsets,omitempty"`
	CredentialRefs   map[string]string `json:"credential_refs,omitempty"`
}

// SkillPolicy defines profile-scoped skill visibility and repositories.
type SkillPolicy struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
	Roots   []string `json:"roots,omitempty"`
}

// KnowledgePolicy defines per-run knowledge query policy.
type KnowledgePolicy struct {
	Indexes []string       `json:"indexes,omitempty"`
	Filter  map[string]any `json:"filter,omitempty"`
}

// WorkspacePolicy defines profile-scoped filesystem boundaries.
type WorkspacePolicy struct {
	Workdir      string   `json:"workdir,omitempty"`
	AllowedRoots []string `json:"allowed_roots,omitempty"`
}

// CredentialPolicy defines profile-scoped credential references.
type CredentialPolicy struct {
	AllowedRefs []string `json:"allowed_refs,omitempty"`
}

// IsolationPolicy defines optional hard-isolation contracts.
type IsolationPolicy struct {
	Mode         string `json:"mode,omitempty"`
	AgentCache   bool   `json:"agent_cache,omitempty"`
	ToolSetCache bool   `json:"toolset_cache,omitempty"`
	ServiceMode  string `json:"service_mode,omitempty"`
}

// MarshalJSON serializes the policy fields as JSON strings for DB storage.
func (p RuntimeProfile) PolicyJSON() (map[string]string, error) {
	out := make(map[string]string, 8)
	pairs := []struct {
		key string
		val any
	}{
		{"prompt_config", p.PromptConfig},
		{"tool_policy", p.ToolPolicy},
		{"skill_policy", p.SkillPolicy},
		{"knowledge_policy", p.KnowledgePolicy},
		{"workspace_policy", p.WorkspacePolicy},
		{"credential_policy", p.CredentialPolicy},
		{"isolation_policy", p.IsolationPolicy},
	}
	for _, pair := range pairs {
		b, err := json.Marshal(pair.val)
		if err != nil {
			return nil, err
		}
		out[pair.key] = string(b)
	}
	if p.ExtraModelConfig != nil {
		b, err := json.Marshal(p.ExtraModelConfig)
		if err != nil {
			return nil, err
		}
		out["extra_model_config"] = string(b)
	} else {
		out["extra_model_config"] = "{}"
	}
	return out, nil
}

// RuntimeProfileReader defines read operations for runtime profiles.
// Stability:evolving
type RuntimeProfileReader interface {
	List(ctx context.Context, agentID string, activeOnly bool) ([]RuntimeProfile, error)
	GetByID(ctx context.Context, id string) (RuntimeProfile, error)
	GetActive(ctx context.Context, agentID string) (*RuntimeProfile, error)
}

// RuntimeProfileWriter defines write operations for runtime profiles.
// Stability:evolving
type RuntimeProfileWriter interface {
	Create(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error)
	Update(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error)
	Delete(ctx context.Context, id string) error
	SetActive(ctx context.Context, id string, active bool) (RuntimeProfile, error)
}

// RuntimeProfileReadWriter is the composite interface for Wire binding.
type RuntimeProfileReadWriter interface {
	RuntimeProfileReader
	RuntimeProfileWriter
}
