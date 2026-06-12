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

const (
	filterCacheMaxEntries = 512
	filterCacheTTL        = 2 * time.Minute
)

type cacheEntry struct {
	value      map[string]bool
	accessedAt time.Time
	createdAt  time.Time
}

type filterCache struct {
	mu        sync.RWMutex
	entries   map[string]*cacheEntry
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
	ttl       time.Duration
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

	if c.ttl > 0 && time.Since(e.createdAt) > c.ttl {
		// Re-check under write lock to avoid TOCTOU: another goroutine may have
		// stored a fresh entry between our RLock and this Lock.
		c.mu.Lock()
		fresh, exists := c.entries[key]
		if !exists {
			c.mu.Unlock()
			c.misses.Add(1)
			return nil, false
		}
		if time.Since(fresh.createdAt) > c.ttl {
			delete(c.entries, key)
			c.mu.Unlock()
			c.misses.Add(1)
			return nil, false
		}
		// Fresh entry was stored by another goroutine; use it.
		e = fresh
		c.mu.Unlock()
	}

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
	c.entries[key] = &cacheEntry{value: val, accessedAt: time.Now(), createdAt: time.Now()}
}

func (c *filterCache) Stats() (hits, misses, evicts int64) {
	return c.hits.Load(), c.misses.Load(), c.evictions.Load()
}

func (c *filterCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry, filterCacheMaxEntries)
}

// AgentVisibilityFilter narrows visible skills per invocation using Layer A + Layer B
// policy from agent_runtime_settings.skill_runtime_json and the turn query in RuntimeState.
type AgentVisibilityFilter struct {
	skillUC    SkillResolver
	runtime    RuntimeSettings
	cache      filterCache
	lg         loggateway.Logger
	agentKey   string
	lastGoodMu sync.RWMutex
	lastGoodSet map[string]bool
}

func NewAgentVisibilityFilter(skillUC SkillResolver, runtime RuntimeSettings, lg loggateway.Logger, agentKey string) trpcskill.VisibilityFilter {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	f := &AgentVisibilityFilter{skillUC: skillUC, runtime: runtime, cache: filterCache{ttl: filterCacheTTL}, lg: lg, agentKey: agentKey}
	return f.allow
}

func (f *AgentVisibilityFilter) InvalidateCache() {
	f.cache.InvalidateAll()
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
	// Include agent key to prevent cross-agent cache leakage when no invocation ID is available.
	if cacheKey == "default" {
		if key := strings.TrimSpace(f.agentKey); key != "" {
			cacheKey = "agent:" + key
		}
	}
	if v, ok := f.cache.Load(cacheKey); ok {
		return v
	}
	opts := &SkillToolsetOptions{Runtime: f.runtime, UserQuery: TurnQueryFromContext(ctx)}
	slugs, err := ResolveSkillSlugsDetailed(ctx, f.skillUC, opts, f.lg)
	set := map[string]bool{}
	if err != nil {
		// Resolution failed: return the last successful result (stale-but-available)
		// so that transient errors don't hide all skills.
		f.lastGoodMu.RLock()
		cached := f.lastGoodSet
		f.lastGoodMu.RUnlock()
		if len(cached) > 0 {
			f.lg.Warn("ResolveSkillSlugs failed; returning last good set (stale-but-available)",
				loggateway.StepID("tool.skillruntime.resolve_fail_stale"),
				loggateway.Err(err))
			return cached
		}
		// No previous good result: fail-closed but do NOT cache the empty set.
		f.lg.Warn("ResolveSkillSlugs failed; no last good set available, hiding all skills",
			loggateway.StepID("tool.skillruntime.resolve_fail"),
			loggateway.Err(err))
		return set
	}
	// Successful resolution (even if result is empty): cache the result.
	for _, slug := range slugs.Slugs {
		s := strings.TrimSpace(strings.ToLower(slug))
		if s != "" {
			set[s] = true
		}
	}
	f.lastGoodMu.Lock()
	f.lastGoodSet = set
	f.lastGoodMu.Unlock()
	f.cache.Store(cacheKey, set)
	return set
}
