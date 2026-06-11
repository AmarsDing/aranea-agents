package agent

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"aranea-agents/internal/agent/planner"
	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

var (
	buildCacheDefaultCap     = 128
	buildCacheTTL            = 10 * time.Minute
	buildCacheGCInterval     = 2 * time.Minute
	buildCacheGCIdleShutdown = 5 * time.Minute
)

// buildCacheEntry stores a cached agent along with its expiry time.
type buildCacheEntry struct {
	agent     trpcagent.Agent
	expiresAt time.Time
	key       string
	elem      *list.Element
	a2ui      *planner.A2UIResult
}

// BuildCache is a thread-safe LRU cache for built trpc LLMAgents.
// It is keyed by a sha256 hash of the agent's configuration fingerprint.
//
// Concurrency: singleflight coalesces concurrent builds for the same key so
// that only one BuildTRPCLLMAgent call runs per key at a time (thundering-herd
// protection). The builder callback receives a context derived via
// context.WithoutCancel so that one caller cancelling its request does not
// abort the shared build.
type BuildCache struct {
	mu      sync.Mutex
	cap     int
	ttl     time.Duration
	items   map[string]*buildCacheEntry
	lruList *list.List // front = most-recently-used

	// singleflight coalesces concurrent cache-miss builds for the same key.
	sfGroup singleflight.Group

	// GC loop fields — lazily started on first put, self-terminates when
	// the cache stays empty long enough (buildCacheGCIdleShutdown).
	started bool
	gcMu    sync.Mutex
	gcDone  chan struct{} // closed when the GC goroutine exits
	gcIdle  time.Duration
}

var globalBuildCache = newBuildCache(buildCacheDefaultCap, buildCacheTTL)

func newBuildCache(cap int, ttl time.Duration) *BuildCache {
	if cap <= 0 {
		cap = buildCacheDefaultCap
	}
	if ttl <= 0 {
		ttl = buildCacheTTL
	}
	return &BuildCache{
		cap:     cap,
		ttl:     ttl,
		items:   make(map[string]*buildCacheEntry),
		lruList: list.New(),
		gcIdle:  buildCacheGCIdleShutdown,
	}
}

// get returns the cached agent for the given key, or nil if not found / expired.
func (c *BuildCache) get(key string) trpcagent.Agent {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		c.evict(key)
		return nil
	}
	c.lruList.MoveToFront(entry.elem)
	return entry.agent
}

func (c *BuildCache) getA2UI(key string) *planner.A2UIResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		c.evict(key)
		return nil
	}
	return entry.a2ui
}

// put stores an agent under the given key, evicting LRU entries if over capacity.
func (c *BuildCache) put(key string, ag trpcagent.Agent, a2uiResult *planner.A2UIResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ag == nil {
		return // never cache a nil agent
	}
	if e, ok := c.items[key]; ok {
		c.lruList.MoveToFront(e.elem)
		e.expiresAt = time.Now().Add(c.ttl)
		e.agent = ag
		e.a2ui = a2uiResult
		return
	}
	for len(c.items) >= c.cap {
		back := c.lruList.Back()
		if back == nil {
			break
		}
		c.evict(back.Value.(*buildCacheEntry).key)
	}
	entry := &buildCacheEntry{
		key:       key,
		agent:     ag,
		expiresAt: time.Now().Add(c.ttl),
		a2ui:      a2uiResult,
	}
	entry.elem = c.lruList.PushFront(entry)
	c.items[key] = entry
	c.ensureGC()
}

// Invalidate removes all cache entries whose key contains the given agentID prefix.
func (c *BuildCache) Invalidate(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.items {
		idx := strings.Index(k, ":")
		if idx >= 0 && k[:idx] == agentID {
			c.evict(k)
		}
	}
}

// evict must be called with c.mu held.
func (c *BuildCache) evict(key string) {
	if e, ok := c.items[key]; ok {
		c.lruList.Remove(e.elem)
		delete(c.items, key)
	}
}

// sweepExpired removes all expired entries and returns the count evicted.
func (c *BuildCache) sweepExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	var evicted int
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			c.evict(k)
			evicted++
		}
	}
	return evicted
}

// Close stops the GC goroutine (if running) and clears all entries.
// It is safe to call Close multiple times.
func (c *BuildCache) Close() {
	c.mu.Lock()
	c.items = make(map[string]*buildCacheEntry)
	c.lruList.Init()
	c.mu.Unlock()

	c.gcMu.Lock()
	if c.started {
		c.started = false
		done := c.gcDone
		c.gcMu.Unlock()
		if done != nil {
			// Wait for the GC goroutine to exit so we don't leak.
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
		}
	} else {
		c.gcMu.Unlock()
	}
}

// ensureGC lazily starts the background GC loop on the first put.
func (c *BuildCache) ensureGC() {
	c.gcMu.Lock()
	defer c.gcMu.Unlock()
	if c.started {
		return
	}
	c.started = true
	c.gcDone = make(chan struct{})
	safego.Go(context.Background(), "agent.cache.gc", func() { c.runGC() })
}

// runGC periodically sweeps expired entries. It self-terminates when the cache
// stays empty for buildCacheGCIdleShutdown consecutive intervals, so that
// long-lived processes don't leak a goroutine when the cache is idle.
func (c *BuildCache) runGC() {
	defer close(c.gcDone)
	emptyStreak := 0
	ticker := time.NewTicker(buildCacheGCInterval)
	defer ticker.Stop()
	for {
		<-ticker.C
		c.gcMu.Lock()
		if !c.started {
			c.gcMu.Unlock()
			return
		}
		c.gcMu.Unlock()

		c.sweepExpired()

		c.mu.Lock()
		isEmpty := len(c.items) == 0
		c.mu.Unlock()

		if isEmpty {
			emptyStreak++
		} else {
			emptyStreak = 0
		}
		if emptyStreak >= int(c.gcIdle/buildCacheGCInterval) {
			c.gcMu.Lock()
			c.started = false
			c.gcMu.Unlock()
			return
		}
	}
}

// BuildCacheKey produces a sha256 fingerprint that uniquely identifies an agent build
// configuration. The key encodes agent ID + UpdatedAt (covers all DB-level changes)
// + provider / model / dialog_mode so that per-request option overrides produce their
// own cache slot.
func BuildCacheKey(ag biz.Agent, deps TRPCBuilderDeps, toolHash, skillHash, mcpHash string) string {
	type fingerprint struct {
		AgentID      string
		AgentUpdated string
		ConfigJSON   string
		Provider     string
		Model        string
		DialogMode   string
		SettingsJSON string
		ToolHash     string
		SkillHash    string
		MCPHash      string
	}
	fp := fingerprint{
		AgentID:      ag.ID,
		AgentUpdated: ag.UpdatedAt,
		ConfigJSON:   strings.TrimSpace(ag.ConfigJSON),
		Provider:     deps.Provider,
		Model:        deps.Model,
		DialogMode:   deps.DialogMode,
		ToolHash:     toolHash,
		SkillHash:    skillHash,
		MCPHash:      mcpHash,
	}
	if ag.Settings != nil {
		if b, err := json.Marshal(ag.Settings); err == nil {
			fp.SettingsJSON = string(b)
		}
		// ConfigJSON may carry additional configuration not captured in Settings;
		// always include it so that a ConfigJSON-only change invalidates the cache.
		if fp.ConfigJSON == "" {
			fp.ConfigJSON = strings.TrimSpace(ag.ConfigJSON)
		}
	}
	raw, _ := json.Marshal(fp)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%s:%x", ag.ID, sum)
}

// InvalidateAgentCache evicts all cached agents for the given agentID from the global cache.
// Call this whenever an agent, tool, or skill is updated.
func InvalidateAgentCache(agentID string) {
	globalBuildCache.Invalidate(agentID)
}

// BuildTRPCLLMAgentCached wraps BuildTRPCLLMAgent with the global LRU cache
// and singleflight coalescing. Concurrent cache-miss calls for the same key
// collapse to a single BuildTRPCLLMAgent invocation; the builder receives a
// context derived via context.WithoutCancel so that one caller cancelling does
// not abort the shared build.
func BuildTRPCLLMAgentCached(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, error) {
	key := BuildCacheKey(ag, deps, deps.ToolVersionHash, deps.SkillVersionHash, deps.MCPVersionHash)
	if cached := globalBuildCache.get(key); cached != nil {
		arametrics.AgentBuildCacheHits.Inc()
		lg.Info("Agent 构建缓存命中", loggateway.StepID("agent.cache_hit"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))
		return cached, nil
	}
	arametrics.AgentBuildCacheMisses.Inc()
	lg.Info("Agent 构建缓存未命中", loggateway.StepID("agent.cache_miss"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))

	// singleflight: coalesce concurrent builds for the same key.
	// Use context.WithoutCancel so that one caller's cancellation does not
	// abort the build shared by other callers.
	v, err, _ := globalBuildCache.sfGroup.Do(key, func() (interface{}, error) {
		buildCtx := context.WithoutCancel(ctx)
		built, buildErr := BuildTRPCLLMAgent(buildCtx, ag, deps, lg)
		if buildErr != nil {
			return nil, buildErr
		}
		globalBuildCache.put(key, built, nil)
		return built, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(trpcagent.Agent), nil
}

// LookupA2UIByAgentKey returns the cached A2UI result for the given agent key.
// The agentKey must match the full cache key format produced by BuildCacheKey.
func LookupA2UIByAgentKey(agentKey string) *planner.A2UIResult {
	if agentKey == "" {
		return nil
	}
	return globalBuildCache.getA2UI(agentKey)
}
