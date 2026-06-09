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

	"aranea-agents/internal/agent/planner"
	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

const (
	buildCacheDefaultCap = 128
	buildCacheTTL        = 10 * time.Minute
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
// It keyed by a sha256 hash of the agent's configuration fingerprint.
type BuildCache struct {
	mu      sync.Mutex
	cap     int
	ttl     time.Duration
	items   map[string]*buildCacheEntry
	lruList *list.List // front = most-recently-used
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
		return nil
	}
	return entry.a2ui
}

// put stores an agent under the given key, evicting LRU entries if over capacity.
func (c *BuildCache) put(key string, ag trpcagent.Agent, a2uiResult *planner.A2UIResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	} else {
		fp.ConfigJSON = ag.ConfigJSON
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

// BuildTRPCLLMAgentCached wraps BuildTRPCLLMAgent with the global LRU cache.
// Cache hits avoid the cost of assembling tools, skills, and the model client.
func BuildTRPCLLMAgentCached(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, error) {
	key := BuildCacheKey(ag, deps, deps.ToolVersionHash, deps.SkillVersionHash, deps.MCPVersionHash)
	if cached := globalBuildCache.get(key); cached != nil {
		arametrics.AgentBuildCacheHits.Inc()
		lg.Info("Agent 构建缓存命中", loggateway.StepID("agent.cache_hit"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))
		return cached, nil
	}
	arametrics.AgentBuildCacheMisses.Inc()
	lg.Info("Agent 构建缓存未命中", loggateway.StepID("agent.cache_miss"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))
	built, err := BuildTRPCLLMAgent(ctx, ag, deps, lg)
	if err != nil {
		return nil, err
	}
	globalBuildCache.put(key, built, nil)
	return built, nil
}

// LookupA2UIByAgentKey returns the cached A2UI result for the given agent key.
// The agentKey must match the full cache key format produced by BuildCacheKey.
func LookupA2UIByAgentKey(agentKey string) *planner.A2UIResult {
	if agentKey == "" {
		return nil
	}
	globalBuildCache.mu.Lock()
	defer globalBuildCache.mu.Unlock()
	if entry, ok := globalBuildCache.items[agentKey]; ok {
		if entry.a2ui != nil && !time.Now().After(entry.expiresAt) {
			return entry.a2ui
		}
	}
	return nil
}
