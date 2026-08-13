package skillruntime

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

type SkillResolver interface {
	ListEnabledPublishedSkillCandidates(ctx context.Context) ([]biz.SkillRuntimeCandidate, error)
	ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error)
	ScoreByEmbedding(ctx context.Context, query string, candidates []biz.SkillRuntimeCandidate) (map[string]float64, error)
}

type RuntimeSettings interface {
	GetSkillRuntimeJSON() string
}

// AgentVisibilityFilter narrows visible skills using ONLY Layer A policy
// (allowed_slugs / denied_slugs) from agent_runtime_settings.skill_runtime_json.
//
// Layer A-only is deliberate: this filter feeds the framework's skill overview
// injection (system prompt prefix via SummariesForContext), which must stay
// byte-stable across turns for prompt-cache hits. Per-turn dynamic routing
// (Layer B: intent/tags/scoring/health) lives in the progressive guidance
// injection path (internal/agent/skill_guidance_inject.go) and never hides
// skills from the overview.
//
// The filter is pure config: no DB access, no cache, no turn-query dependency.
// The base repository already restricts to enabled+published rows, so skill
// enable/disable takes effect without rebuilding this filter.
type AgentVisibilityFilter struct {
	allowed map[string]bool // empty = no allow-list configured → allow all not denied
	denied  map[string]bool
}

// NewAgentVisibilityFilter builds a Layer A-only visibility filter from the
// agent's static skill runtime policy. The returned filter produces a stable
// visible set for the lifetime of the built agent.
func NewAgentVisibilityFilter(runtime RuntimeSettings) trpcskill.VisibilityFilter {
	raw := "{}"
	if runtime != nil && strings.TrimSpace(runtime.GetSkillRuntimeJSON()) != "" {
		raw = runtime.GetSkillRuntimeJSON()
	}
	// Policy slugs are already normalized (lowercase, trimmed) by the parser.
	policy := biz.ParseSkillRuntimePolicy(raw)
	f := &AgentVisibilityFilter{
		allowed: strutil.SliceToSet(policy.AllowedSlugs),
		denied:  strutil.SliceToSet(policy.DeniedSlugs),
	}
	return f.allow
}

func (f *AgentVisibilityFilter) allow(_ context.Context, summary trpcskill.Summary) bool {
	if f == nil {
		return true
	}
	// Summary.Name is the canonical slug (DB adapter aligned in
	// internal/skill/trpc/db_repository.go). Canonicalize defensively so
	// framework FS repositories that still emit display name keep working
	// when display==slug.
	key := strings.ToLower(strings.TrimSpace(summary.Name))
	if key == "" {
		return false
	}
	if f.denied[key] {
		return false
	}
	if len(f.allowed) > 0 && !f.allowed[key] {
		return false
	}
	return true
}
