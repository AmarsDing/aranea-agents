package skillruntime

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

const filterCacheMaxEntries = 512

type filterCache struct {
	mu      sync.Mutex
	entries map[string]map[string]bool
	count   atomic.Int64
}

func (c *filterCache) Load(key string) (map[string]bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[key]
	return v, ok
}

func (c *filterCache) Store(key string, val map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]map[string]bool, filterCacheMaxEntries)
	}
	if _, exists := c.entries[key]; !exists {
		n := c.count.Add(1)
		if n > filterCacheMaxEntries {
			c.entries = make(map[string]map[string]bool, filterCacheMaxEntries)
			c.count.Store(1)
		}
	}
	c.entries[key] = val
}

// AgentVisibilityFilter narrows visible skills per invocation using Layer A + Layer B
// policy from agent_runtime_settings.skill_runtime_json and the turn query in RuntimeState.
type AgentVisibilityFilter struct {
	skillUC *biz.SkillUsecase
	runtime *biz.AgentRuntimeSettings
	cache   filterCache
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
	// TPM-P1-06: Summary.Name is now the canonical slug (DB adapter aligned in
	// internal/skill/trpc/db_repository.go). Canonicalize defensively so framework
	// FS repositories that still emit display name keep working when display==slug.
	key := strings.ToLower(strings.TrimSpace(summary.Name))
	return allowed[key]
}

func (f *AgentVisibilityFilter) allowedSlugs(ctx context.Context) map[string]bool {
	cacheKey := "default"
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
		if id := strings.TrimSpace(inv.InvocationID); id != "" {
			cacheKey = id
		}
	}
	if v, ok := f.cache.Load(cacheKey); ok {
		return v
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
	} else if err != nil {
		slog.Warn("skillruntime: ResolveSkillSlugs failed; hiding all skills (fail-closed)", "error", err)
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
