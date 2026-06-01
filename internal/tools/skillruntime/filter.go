package skillruntime

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
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

const filterCacheMaxEntries = 512

type cacheEntry struct {
	value      map[string]bool
	accessedAt time.Time
}

type filterCache struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry
	hits     atomic.Int64
	misses   atomic.Int64
	evictions atomic.Int64
}

func (c *filterCache) Load(key string) (map[string]bool, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	if !ok {
		c.mu.RUnlock()
		c.misses.Add(1)
		return nil, false
	}
	c.mu.RUnlock()
	c.mu.Lock()
	e.accessedAt = time.Now()
	c.mu.Unlock()
	c.hits.Add(1)
	return e.value, true
}

func (c *filterCache) Store(key string, val map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]*cacheEntry, filterCacheMaxEntries)
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= filterCacheMaxEntries {
		var oldest string
		var oldestTime time.Time
		for k, e := range c.entries {
			if oldest == "" || e.accessedAt.Before(oldestTime) {
				oldest = k
				oldestTime = e.accessedAt
			}
		}
		delete(c.entries, oldest)
		c.evictions.Add(1)
	}
	c.entries[key] = &cacheEntry{value: val, accessedAt: time.Now()}
	c.misses.Add(1)
}

func (c *filterCache) Stats() (hits, misses, evicts int64) {
	return c.hits.Load(), c.misses.Load(), c.evictions.Load()
}

// AgentVisibilityFilter narrows visible skills per invocation using Layer A + Layer B
// policy from agent_runtime_settings.skill_runtime_json and the turn query in RuntimeState.
type AgentVisibilityFilter struct {
	skillUC SkillResolver
	runtime RuntimeSettings
	cache   filterCache
	lg      loggateway.Logger
}

func NewAgentVisibilityFilter(skillUC SkillResolver, runtime RuntimeSettings, lg loggateway.Logger) trpcskill.VisibilityFilter {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	f := &AgentVisibilityFilter{skillUC: skillUC, runtime: runtime, lg: lg}
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
	slugs, err := ResolveSkillSlugsDetailed(ctx, f.skillUC, opts, f.lg)
	set := map[string]bool{}
	if err == nil {
		for _, slug := range slugs.Slugs {
			s := strings.TrimSpace(strings.ToLower(slug))
			if s != "" {
				set[s] = true
			}
		}
	}
	if err != nil {
		f.lg.Warn("ResolveSkillSlugs failed; hiding all skills (fail-closed)",
			loggateway.StepID("tool.skillruntime.resolve_fail"),
			loggateway.Err(err))
	}
	f.cache.Store(cacheKey, set)
	return set
}
