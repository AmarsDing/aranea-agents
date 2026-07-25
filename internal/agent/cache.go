package agent

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"

	"aranea-agents/internal/agent/planner"
	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

var buildCacheDefaultCap = 128

// buildCacheEntry stores a cached agent.
// TTL has been removed — entries stay until explicitly invalidated or evicted
// by LRU policy. This ensures that hot agents are always available without
// periodic cold-start penalties.
type buildCacheEntry struct {
	agent trpcagent.Agent
	key   string
	elem  *list.Element
	a2ui  *planner.A2UIResult
	dirty bool // if true, a background rebuild is in progress
	// toolSets holds the ToolSets created during agent build. They are
	// closed when the entry is evicted or the cache is shut down, preventing
	// resource leaks (MCP sessions, stdio subprocesses, HTTP connections).
	toolSets []trpctool.ToolSet
}

// BuildCache is a thread-safe LRU cache for built trpc LLMAgents.
// It is keyed by a sha256 hash of the agent's configuration fingerprint.
//
// Design principles (Always-Ready Agent):
//   - No TTL: entries persist until explicitly invalidated or LRU-evicted.
//     This eliminates the 2-15s cold-start penalty when a hot agent's TTL
//     expires between requests.
//   - MarkDirty: instead of evicting on invalidation, entries are marked
//     dirty and continue serving stale agents while a background rebuild
//     runs. This ensures zero downtime during configuration changes.
//   - singleflight: concurrent cache-miss builds for the same key collapse
//     to a single BuildTRPCLLMAgent invocation (thundering-herd protection).
type BuildCache struct {
	mu      sync.Mutex
	cap     int
	items   map[string]*buildCacheEntry
	lruList *list.List // front = most-recently-used
	lg      loggateway.Logger

	// singleflight coalesces concurrent cache-miss builds for the same key.
	sfGroup singleflight.Group
}

var globalBuildCache = newBuildCache(buildCacheDefaultCap)

func newBuildCache(cap int) *BuildCache {
	if cap <= 0 {
		cap = buildCacheDefaultCap
	}
	return &BuildCache{
		cap:     cap,
		items:   make(map[string]*buildCacheEntry),
		lruList: list.New(),
		lg:      loggateway.NewNoop(),
	}
}

// SetLogger injects a Logger into the cache. Should be called once during
// Wire initialization (before any cache operations). The Logger is used by
// closeToolSets to log ToolSet Close errors during eviction and shutdown.
func (c *BuildCache) SetLogger(lg loggateway.Logger) {
	if lg == nil {
		return
	}
	c.mu.Lock()
	c.lg = lg
	c.mu.Unlock()
}

// get returns the cached agent for the given key, or nil if not found.
func (c *BuildCache) get(key string) trpcagent.Agent {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
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
	return entry.a2ui
}

// put stores an agent under the given key, evicting LRU entries if over capacity.
func (c *BuildCache) put(key string, ag trpcagent.Agent, a2uiResult *planner.A2UIResult, toolSets []trpctool.ToolSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ag == nil {
		return // never cache a nil agent
	}
	if e, ok := c.items[key]; ok {
		// Close old ToolSets before replacing — the new agent brings its own.
		c.closeToolSets(e.toolSets, key)
		c.lruList.MoveToFront(e.elem)
		e.agent = ag
		e.a2ui = a2uiResult
		e.dirty = false
		e.toolSets = toolSets
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
		key:      key,
		agent:    ag,
		a2ui:     a2uiResult,
		toolSets: toolSets,
	}
	entry.elem = c.lruList.PushFront(entry)
	c.items[key] = entry
}

// Invalidate marks all cache entries for the given agentID as dirty.
// The stale agent continues to serve requests until the next request
// triggers a background rebuild (see BuildTRPCLLMAgentCached).
func (c *BuildCache) Invalidate(agentID string) {
	c.mu.Lock()
	for k, e := range c.items {
		idx := strings.Index(k, ":")
		if idx >= 0 && k[:idx] == agentID {
			e.dirty = true
		}
	}
	c.mu.Unlock()
}

// InvalidateAll marks every entry as dirty.
func (c *BuildCache) InvalidateAll() {
	c.mu.Lock()
	for _, e := range c.items {
		e.dirty = true
	}
	c.mu.Unlock()
}

// evict must be called with c.mu held.
func (c *BuildCache) evict(key string) {
	if e, ok := c.items[key]; ok {
		c.closeToolSets(e.toolSets, key)
		c.lruList.Remove(e.elem)
		delete(c.items, key)
	}
}

// Close clears all entries. It is safe to call Close multiple times.
// Returns nil; the error return satisfies io.Closer / lifecycle.Closer so
// the cache can be registered with LifecycleManager (A3).
func (c *BuildCache) Close() error {
	c.mu.Lock()
	for key, e := range c.items {
		c.closeToolSets(e.toolSets, key)
	}
	c.items = make(map[string]*buildCacheEntry)
	c.lruList.Init()
	c.mu.Unlock()
	return nil
}

// closeToolSets closes each ToolSet in the slice, logging any errors.
// It is best-effort: a Close error on one ToolSet does not prevent the
// others from being closed. This function is called during cache eviction
// and shutdown to prevent resource leaks (MCP sessions, stdio subprocesses).
// Caller must hold c.mu.
func (c *BuildCache) closeToolSets(toolSets []trpctool.ToolSet, cacheKey string) {
	for _, ts := range toolSets {
		if ts == nil {
			continue
		}
		if err := ts.Close(); err != nil {
			c.lg.Warn("ToolSet Close 失败",
				loggateway.Domain("agent.cache"),
				loggateway.Str("cache_key", cacheKey),
				loggateway.Str("toolset", ts.Name()),
				loggateway.Err(err))
		}
	}
}

// GetGlobalBuildCache returns the process-level BuildCache singleton.
// It is exposed so Wire can register it with the LifecycleManager for
// orderly shutdown (A3).
func GetGlobalBuildCache() *BuildCache {
	return globalBuildCache
}

// BuildCacheKey produces a sha256 fingerprint that uniquely identifies an agent build
// configuration. The key encodes agent ID + UpdatedAt (covers all DB-level changes)
// + DialogMode (affects Planner selection at build time) but NOT Provider/Model,
// which are resolved per-request via RunOption (agent.WithModel) instead of being
// baked into the cache key. This allows different provider/model combinations to
// share the same cached agent, dramatically improving cache hit rates.
func BuildCacheKey(ag biz.Agent, deps TRPCBuilderDeps, toolHash, skillHash, mcpHash string) string {
	type fingerprint struct {
		AgentID      string
		AgentUpdated string
		ConfigJSON   string
		DialogMode   string
		SettingsJSON string
		ToolHash     string
		SkillHash    string
		MCPHash      string
		CustomTools  []string
	}
	fp := fingerprint{
		AgentID:      ag.ID,
		AgentUpdated: ag.UpdatedAt,
		ConfigJSON:   strings.TrimSpace(ag.ConfigJSON),
		DialogMode:   deps.DialogMode,
		ToolHash:     toolHash,
		SkillHash:    skillHash,
		MCPHash:      mcpHash,
		CustomTools:  customToolNames(deps.CustomTools),
	}
	if ag.Settings != nil {
		if b, err := json.Marshal(ag.Settings); err == nil {
			fp.SettingsJSON = string(b)
		}
	}
	raw, _ := json.Marshal(fp)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%s:%x", ag.ID, sum)
}

// customToolNames returns the sorted declaration names of the given tools.
// CustomTools must participate in the cache key: an agent built with extra
// tools (e.g. deliverable tools in EnableStateDeliverable team graphs) must
// not share a cache entry with the same agent built without them, otherwise
// the toolset leaks across builds depending on build order. Sorting makes the
// key order-insensitive.
func customToolNames(tools []trpctool.Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		names = append(names, t.Declaration().Name)
	}
	sort.Strings(names)
	return names
}

// InvalidateAgentCache marks all cached agents for the given agentID as dirty,
// triggering background rebuilds. The stale agent continues serving until the
// rebuild completes. Call this whenever an agent, tool, or skill is updated.
func InvalidateAgentCache(agentID string) {
	globalBuildCache.Invalidate(agentID)
}

// InvalidateAllAgentCaches marks every cached agent as dirty, triggering
// background rebuilds. Call this when a platform-wide resource (tool catalog,
// skill list, MCP servers) changes and potentially affects all agents.
func InvalidateAllAgentCaches() {
	globalBuildCache.InvalidateAll()
}

// BuildTRPCLLMAgentCached wraps BuildTRPCLLMAgent with the global LRU cache
// and singleflight coalescing. Concurrent cache-miss calls for the same key
// collapse to a single BuildTRPCLLMAgent invocation; the builder receives a
// context derived via context.WithoutCancel so that one caller cancelling does
// not abort the shared build.
//
// When a dirty entry is found, the stale agent is returned immediately (zero
// latency) and a background rebuild is triggered to refresh the cache for
// subsequent requests.
func BuildTRPCLLMAgentCached(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, error) {
	key := BuildCacheKey(ag, deps, deps.ToolVersionHash, deps.SkillVersionHash, deps.MCPVersionHash)

	// Snapshot cache entry fields under lock to avoid races with concurrent
	// Invalidate calls that may set dirty=true or replace the agent.
	globalBuildCache.mu.Lock()
	entry, ok := globalBuildCache.items[key]
	var cachedAgent trpcagent.Agent
	var dirty bool
	if ok {
		cachedAgent = entry.agent
		dirty = entry.dirty
	}
	globalBuildCache.mu.Unlock()

	if ok {
		if dirty {
			// Dirty entry: serve stale agent immediately for zero latency,
			// but also trigger a fresh build in the background so the next
			// request gets the updated agent. The stale agent is safe to use
			// because the configuration change only affects tool/skill/MCP
			// composition, not the LLM agent's core behavior.
			arametrics.AgentBuildCacheHits.Inc()
			lg.Info("Agent 构建缓存命中（脏，后台重建中）", loggateway.StepID("agent.cache_hit_dirty"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))

			// Kick off a background build to refresh this cache entry.
			// Use singleflight to coalesce if multiple requests hit the same dirty key.
			safego.Go(context.Background(), "agent.cache.dirty_rebuild", func() {
				globalBuildCache.sfGroup.Do(key, func() (interface{}, error) {
					buildCtx := context.WithoutCancel(ctx)
					built, toolSets, buildErr := buildTRPCLLMAgentWithToolSets(buildCtx, ag, deps, lg)
					if buildErr != nil {
						lg.Warn("Agent 后台重建失败", loggateway.StepID("agent.cache_rebuild_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(buildErr))
						return nil, buildErr
					}
					globalBuildCache.put(key, built, nil, toolSets)
					lg.Info("Agent 后台重建完成", loggateway.StepID("agent.cache_rebuild_done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("cache_key", key))
					return built, nil
				})
			})

			return cachedAgent, nil
		}

		arametrics.AgentBuildCacheHits.Inc()
		lg.Info("Agent 构建缓存命中", loggateway.StepID("agent.cache_hit"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))
		return cachedAgent, nil
	}

	arametrics.AgentBuildCacheMisses.Inc()
	lg.Info("Agent 构建缓存未命中", loggateway.StepID("agent.cache_miss"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))

	// singleflight: coalesce concurrent builds for the same key.
	// Use context.WithoutCancel so that one caller's cancellation does not
	// abort the build shared by other callers.
	v, err, _ := globalBuildCache.sfGroup.Do(key, func() (interface{}, error) {
		buildCtx := context.WithoutCancel(ctx)
		built, toolSets, buildErr := buildTRPCLLMAgentWithToolSets(buildCtx, ag, deps, lg)
		if buildErr != nil {
			return nil, buildErr
		}
		globalBuildCache.put(key, built, nil, toolSets)
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
