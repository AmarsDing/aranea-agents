package skillruntime

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// AgentVisibilityFilter narrows visible skills per invocation using Layer A + Layer B
// policy from agent_runtime_settings.skill_runtime_json and the turn query in RuntimeState.
type AgentVisibilityFilter struct {
	skillUC *biz.SkillUsecase
	runtime *biz.AgentRuntimeSettings
	cache   sync.Map // invocationID -> map[string]bool
}

// NewAgentVisibilityFilter returns a trpc-agent-go VisibilityFilter backed by ResolveSkillSlugs.
func NewAgentVisibilityFilter(skillUC *biz.SkillUsecase, runtime *biz.AgentRuntimeSettings) trpcskill.VisibilityFilter {
	f := &AgentVisibilityFilter{skillUC: skillUC, runtime: runtime}
	return f.allow
}

func (f *AgentVisibilityFilter) allow(ctx context.Context, summary trpcskill.Summary) bool {
	if f == nil || f.skillUC == nil {
		return true
	}
	allowed := f.allowedSlugs(ctx)
	if len(allowed) == 0 {
		return false
	}
	name := strings.TrimSpace(strings.ToLower(summary.Name))
	return allowed[name]
}

func (f *AgentVisibilityFilter) allowedSlugs(ctx context.Context) map[string]bool {
	cacheKey := "default"
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
		if id := strings.TrimSpace(inv.InvocationID); id != "" {
			cacheKey = id
		}
	}
	if v, ok := f.cache.Load(cacheKey); ok {
		return v.(map[string]bool)
	}
	opts := &SkillToolsetOptions{Runtime: f.runtime, UserQuery: TurnQueryFromContext(ctx)}
	slugs, err := ResolveSkillSlugs(ctx, f.skillUC, opts)
	set := map[string]bool{}
	if err == nil {
		for _, slug := range slugs {
			s := strings.TrimSpace(strings.ToLower(slug))
			if s != "" {
				set[s] = true
			}
		}
	}
	if len(set) == 0 && err == nil {
		// Policy produced no slugs while candidates exist — keep repo mount but hide all.
	} else if err != nil {
		// Fail open to enabled published keys only when resolution errors.
		keys, listErr := f.skillUC.ListEnabledPublishedSkillKeys(ctx)
		if listErr == nil {
			set = strutil.SliceToSet(normalizeSlugSlice(keys))
		}
	}
	f.cache.Store(cacheKey, set)
	return set
}

func normalizeSlugSlice(slugs []string) []string {
	out := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		s := strings.TrimSpace(strings.ToLower(slug))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
