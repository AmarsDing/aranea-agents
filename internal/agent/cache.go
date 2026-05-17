package agent

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"

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
}

// BuildCache is a thread-safe LRU cache for built trpc LLMAgents.
// It keyed by a sha256 hash of the agent's configuration fingerprint.
type BuildCache struct {
	mu       sync.Mutex
	cap      int
	ttl      time.Duration
	items    map[string]*buildCacheEntry
	lruList  *list.List // front = most-recently-used
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

// put stores an agent under the given key, evicting LRU entries if over capacity.
func (c *BuildCache) put(key string, ag trpcagent.Agent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.lruList.MoveToFront(e.elem)
		e.expiresAt = time.Now().Add(c.ttl)
		e.agent = ag
		return
	}
	for len(c.items) >= c.cap {
		// Evict least-recently-used (back of the list).
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
	}
	entry.elem = c.lruList.PushFront(entry)
	c.items[key] = entry
}

// Invalidate removes all cache entries whose key contains the given agentID prefix.
func (c *BuildCache) Invalidate(agentID string) {
	prefix := agentID + ":"
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.items {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
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
func BuildCacheKey(ag biz.Agent, deps TRPCBuilderDeps) string {
	type fingerprint struct {
		AgentID      string
		AgentUpdated string
		ConfigJSON   string
		Provider     string
		Model        string
		DialogMode   string
		SettingsJSON string
	}
	fp := fingerprint{
		AgentID:      ag.ID,
		AgentUpdated: ag.UpdatedAt,
		ConfigJSON:   ag.ConfigJSON,
		Provider:     deps.Provider,
		Model:        deps.Model,
		DialogMode:   deps.DialogMode,
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

// InvalidateAgentCache evicts all cached agents for the given agentID from the global cache.
// Call this whenever an agent, tool, or skill is updated.
func InvalidateAgentCache(agentID string) {
	globalBuildCache.Invalidate(agentID)
}

// BuildTRPCLLMAgentCached wraps BuildTRPCLLMAgent with the global LRU cache.
// Cache hits avoid the cost of assembling tools, skills, and the model client.
func BuildTRPCLLMAgentCached(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	key := BuildCacheKey(ag, deps)
	if cached := globalBuildCache.get(key); cached != nil {
		arametrics.AgentBuildCacheHits.Inc() // EP-OBS-04
		return cached, nil
	}
	arametrics.AgentBuildCacheMisses.Inc() // EP-OBS-04
	built, err := BuildTRPCLLMAgent(ctx, ag, deps)
	if err != nil {
		return nil, err
	}
	globalBuildCache.put(key, built)
	return built, nil
}
