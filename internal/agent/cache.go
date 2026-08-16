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
	"time"

	"golang.org/x/sync/singleflight"

	"aranea-agents/internal/agent/planner"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
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
	// toolSets holds the ToolSets created during agent build. When the entry
	// is evicted or replaced they are retired to the graveyard and closed
	// after a delay (see retireToolSets), preventing both resource leaks
	// (MCP sessions, stdio subprocesses) and in-flight-call aborts.
	toolSets []trpctool.ToolSet
	// handle 是本代际的引用计数句柄（P0-4）：agent 字段即 handle.agent
	// （runScopedAgent 包装产物）；替换/驱逐时随 toolSets 一起进 graveyard，
	// sweeper 依据 handle.refs 精确判定在途 run 是否结束。
	handle *agentHandle
	// lastHitFlowAt throttles the cache_hit flow log: hits fire on every
	// chat request, so the user-visible flow log is emitted at most once per
	// entry per cacheHitFlowLogInterval (process logs/metrics stay per-hit).
	lastHitFlowAt time.Time
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
	bus     contract.MonitorBus

	// singleflight coalesces concurrent cache-miss builds for the same key.
	sfGroup singleflight.Group

	// graveyard holds ToolSets retired from live entries (replaced or
	// evicted). They are closed by the sweeper only after retireDelay,
	// giving in-flight requests holding the retired agent time to finish
	// their tool calls before the underlying MCP sessions / stdio
	// subprocesses are torn down.
	graveyard     []retiredToolSet
	retireDelay   time.Duration
	sweeperCancel context.CancelFunc
	sweeperDone   chan struct{} // closed when the sweeper goroutine exits
	closed        bool          // set by Close; retire/put become no-ops afterwards
}

// retiredToolSet is one graveyard entry: a group of ToolSets retired from the
// same cache key, together with the retirement timestamp used by the sweeper.
type retiredToolSet struct {
	toolSets []trpctool.ToolSet
	cacheKey string
	retireAt time.Time
	// handle 是被退役代际的引用计数句柄（P0-4）；nil 视为 refs==0
	// （仅 Close 路径构造无 handle 的临时条目，且那些条目不进 graveyard）。
	handle *agentHandle
}

const (
	// toolSetRetireDelay is how long retired ToolSets stay open before the
	// sweeper actually closes them. It must comfortably exceed the longest
	// expected in-flight tool call on a cached agent (LLM turns with tool
	// use run minutes, not seconds).
	toolSetRetireDelay = 10 * time.Minute
	// toolSetSweepInterval is the sweeper's scan period in production; the
	// effective interval is min(toolSetSweepInterval, retireDelay) so tests
	// with a tiny injected retireDelay still observe timely sweeps.
	toolSetSweepInterval = time.Minute
)

// cacheHitFlowLogInterval is the per-entry throttle window for the
// system.agent.cache_hit flow log (see buildCacheEntry.lastHitFlowAt).
const cacheHitFlowLogInterval = 5 * time.Minute

var globalBuildCache = newBuildCache(buildCacheDefaultCap)

// agentBuildFn is the agent builder invoked on cache misses. Package-level so
// tests can substitute a lightweight stub; production wiring never overrides it.
var agentBuildFn = buildTRPCLLMAgentWithToolSets

func newBuildCache(cap int) *BuildCache {
	if cap <= 0 {
		cap = buildCacheDefaultCap
	}
	return &BuildCache{
		cap:         cap,
		items:       make(map[string]*buildCacheEntry),
		lruList:     list.New(),
		lg:          loggateway.NewNoop(),
		retireDelay: toolSetRetireDelay,
	}
}

// SetLogger injects a Logger into the cache. Should be called once during
// Wire initialization (before any cache operations). The Logger is used by
// closeToolSetsNow to log ToolSet Close errors during graveyard sweeps and
// shutdown.
func (c *BuildCache) SetLogger(lg loggateway.Logger) {
	if lg == nil {
		return
	}
	c.mu.Lock()
	c.lg = lg
	c.mu.Unlock()
}

// SetMonitorBus injects the shared MonitorBus into the cache. Should be called
// once during Wire initialization (before any cache operations). The bus is
// used to publish cache hit/miss flow logs (流程日志); when unset, flow log
// emission is skipped (tests).
func (c *BuildCache) SetMonitorBus(bus contract.MonitorBus) {
	if bus == nil {
		return
	}
	c.mu.Lock()
	c.bus = bus
	c.mu.Unlock()
}

// emitCacheFlow publishes a cache hit/miss flow log via the shared monitor
// bus. Nil-safe: no-op when the bus is not wired.
func (c *BuildCache) emitCacheFlow(ctx context.Context, lg loggateway.Logger, stepID, message string, pairs ...event.Pair) {
	flow := c.cacheFlowEmitter(ctx, lg)
	if flow == nil {
		return
	}
	flow.LogDone(stepID, message, pairs...)
}

// cacheFlowEmitter builds a run-scoped flow emitter over the shared monitor
// bus, or nil when the bus is not wired (tests). One emitter instance pairs
// LogStart/LogDone timings, so multi-phase flows must share the instance.
func (c *BuildCache) cacheFlowEmitter(ctx context.Context, lg loggateway.Logger) *event.TraceEmitter {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	bus := c.bus
	c.mu.Unlock()
	if bus == nil {
		return nil
	}
	if lg == nil {
		lg = c.lg
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     lg,
		Infra:  event.NewInfraFromBus(bus),
	})
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
// Returns the new generation's handle（agent 字段为 runScopedAgent 包装产物）；
// ag 为 nil 或缓存已关闭时返回 nil（调用方自行处置原始 agent）。
func (c *BuildCache) put(key string, ag trpcagent.Agent, a2uiResult *planner.A2UIResult, toolSets []trpctool.ToolSet) *agentHandle {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ag == nil {
		return nil // never cache a nil agent
	}
	if c.closed {
		// Cache already shut down: do not cache (would leak the ToolSets,
		// since no sweeper/Close will ever reclaim them). Close the incoming
		// ToolSets immediately so the caller's resources are not leaked.
		go closeToolSetsNow(c.lg, toolSets, key)
		return nil
	}
	h := newAgentHandle(ag)
	if e, ok := c.items[key]; ok {
		// Retire old ToolSets before replacing — the new agent brings its
		// own. Retired sets carry the old generation's handle so the sweeper
		// closes them precisely once in-flight runs drain (see sweepGraveyard).
		c.retireToolSets(e.toolSets, key, e.handle)
		c.lruList.MoveToFront(e.elem)
		e.agent = h.agent
		e.a2ui = a2uiResult
		e.dirty = false
		e.toolSets = toolSets
		e.handle = h
		return h
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
		agent:    h.agent,
		a2ui:     a2uiResult,
		toolSets: toolSets,
		handle:   h,
	}
	entry.elem = c.lruList.PushFront(entry)
	c.items[key] = entry
	return h
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
		c.retireToolSets(e.toolSets, key, e.handle)
		c.lruList.Remove(e.elem)
		delete(c.items, key)
	}
}

// Close clears all entries and immediately closes every ToolSet, including
// graveyard entries whose delay has not elapsed: process shutdown has no
// in-flight requests left worth waiting for. Close blocks until the sweeper
// goroutine has fully exited, guaranteeing no concurrent ToolSet.Close runs
// after Close returns (no double-close). It is safe to call Close multiple
// times; subsequent calls are no-ops. Returns nil; the error return satisfies
// io.Closer / lifecycle.Closer so the cache can be registered with
// LifecycleManager (A3).
func (c *BuildCache) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	var all []retiredToolSet
	for key, e := range c.items {
		all = append(all, retiredToolSet{toolSets: e.toolSets, cacheKey: key})
	}
	c.items = make(map[string]*buildCacheEntry)
	c.lruList.Init()
	all = append(all, c.graveyard...)
	c.graveyard = nil
	cancel := c.sweeperCancel
	done := c.sweeperDone
	c.sweeperCancel = nil
	c.sweeperDone = nil
	lg := c.lg
	c.mu.Unlock()

	// Stop the sweeper and wait for it to fully exit before closing, so no
	// sweepGraveyard Close can race with the loop below (double-close).
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	for _, r := range all {
		closeToolSetsNow(lg, r.toolSets, r.cacheKey)
	}
	return nil
}

// GetGlobalBuildCache returns the process-level BuildCache singleton.
// It is exposed so Wire can register it with the LifecycleManager for
// orderly shutdown (A3).
func GetGlobalBuildCache() *BuildCache {
	return globalBuildCache
}

// BuildCacheKey produces a sha256 fingerprint that uniquely identifies an agent build
// configuration. The key encodes agent ID + UpdatedAt (covers all DB-level changes)
// + the build-effective dialog mode (planner selection only, see cacheKeyDialogMode)
// but NOT Provider/Model, which are resolved per-request via RunOption
// (agent.WithModel) instead of being baked into the cache key. This allows
// different provider/model combinations to share the same cached agent,
// dramatically improving cache hit rates.
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
		DialogMode:   cacheKeyDialogMode(ag, deps.DialogMode),
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

// cacheKeyDialogMode reduces the dialog mode to its build-time effect on
// planner selection (planner.Select): with an explicit planner_kind the mode
// is irrelevant; otherwise only "plan" activates the builtin planner.
// Normalizing prevents cache-key flapping between build paths that pass
// per-request modes ("" / "default" / "chat") for the same agent — the team
// runner path passes the turn's dialog mode while the graph node resolver
// path passes "".
func cacheKeyDialogMode(ag biz.Agent, dialogMode string) string {
	if !planner.DialogModeSelects(plannerKind(ag)) {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(dialogMode), "plan") {
		return "plan"
	}
	return ""
}

// cacheKeyDiscriminated is implemented by tools whose behavior varies beyond
// their declaration name (e.g. a contract-installed set_deliverable validates
// writes while the contract-free variant accepts them). The discriminator is
// folded into the build cache key so behavior variants never share a cache
// entry. Satisfaction is implicit: tool packages implement the method without
// importing this package.
type cacheKeyDiscriminated interface {
	CacheKeyDiscriminator() string
}

// customToolNames returns the sorted declaration names of the given tools.
// CustomTools must participate in the cache key: an agent built with extra
// tools (e.g. deliverable tools in EnableStateDeliverable team graphs) must
// not share a cache entry with the same agent built without them, otherwise
// the toolset leaks across builds depending on build order. Sorting makes the
// key order-insensitive. Tools implementing cacheKeyDiscriminated append
// their discriminator to the name so behavior variants get distinct keys.
func customToolNames(tools []trpctool.Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		name := t.Declaration().Name
		if d, ok := t.(cacheKeyDiscriminated); ok {
			if disc := d.CacheKeyDiscriminator(); disc != "" {
				name += "|" + disc
			}
		}
		names = append(names, name)
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
	var emitHitFlow bool
	if ok {
		cachedAgent = entry.agent
		dirty = entry.dirty
		// 命中是每请求级高频事件：流程日志按 entry 节流（进程日志/指标仍逐次打）。
		if time.Since(entry.lastHitFlowAt) >= cacheHitFlowLogInterval {
			entry.lastHitFlowAt = time.Now()
			emitHitFlow = true
		}
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
			if emitHitFlow {
				globalBuildCache.emitCacheFlow(ctx, lg, "system.agent.cache_hit", "Agent 缓存命中（脏，后台重建中）",
					event.P("agent_id", ag.ID), event.P("agent_key", ag.AgentKey), event.P("dirty", true))
			}

			// Kick off a background build to refresh this cache entry.
			// Use singleflight to coalesce if multiple requests hit the same dirty key.
			safego.Go(context.Background(), "agent.cache.dirty_rebuild", func() {
				globalBuildCache.sfGroup.Do(key, func() (interface{}, error) {
					buildCtx := context.WithoutCancel(ctx)
					built, toolSets, buildErr := agentBuildFn(buildCtx, ag, deps, lg)
				if buildErr != nil {
					lg.Warn("Agent 后台重建失败", loggateway.StepID("agent.cache_rebuild_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(buildErr))
					return nil, buildErr
				}
				// 后台重建只负责换代；返回值无人消费，put 内部完成旧代际退役。
				globalBuildCache.put(key, built, nil, toolSets)
					lg.Info("Agent 后台重建完成", loggateway.StepID("agent.cache_rebuild_done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("cache_key", key))
					return built, nil
				})
			})

			return cachedAgent, nil
		}

		arametrics.AgentBuildCacheHits.Inc()
		lg.Info("Agent 构建缓存命中", loggateway.StepID("agent.cache_hit"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))
		if emitHitFlow {
			globalBuildCache.emitCacheFlow(ctx, lg, "system.agent.cache_hit", "Agent 缓存命中",
				event.P("agent_id", ag.ID), event.P("agent_key", ag.AgentKey))
		}
		return cachedAgent, nil
	}

	arametrics.AgentBuildCacheMisses.Inc()
	lg.Info("Agent 构建缓存未命中", loggateway.StepID("agent.cache_miss"), loggateway.Phase("done"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("cache_key", key))
	globalBuildCache.emitCacheFlow(ctx, lg, "system.agent.cache_miss", "Agent 缓存未命中",
		event.P("agent_id", ag.ID), event.P("agent_key", ag.AgentKey))

	// singleflight: coalesce concurrent builds for the same key.
	// Use context.WithoutCancel so that one caller's cancellation does not
	// abort the build shared by other callers.
	v, err, _ := globalBuildCache.sfGroup.Do(key, func() (interface{}, error) {
		buildCtx := context.WithoutCancel(ctx)
		// K1: build start/done flow logs make the (potentially multi-second)
		// cold build visible in the 流程日志 tab. One emitter instance pairs
		// the start/done timing under step system.agent.build.
		flow := globalBuildCache.cacheFlowEmitter(buildCtx, lg)
		if flow != nil {
			flow.LogStart("system.agent.build", "Agent 构建开始",
				event.P("agent_id", ag.ID), event.P("agent_key", ag.AgentKey))
		}
		built, toolSets, buildErr := agentBuildFn(buildCtx, ag, deps, lg)
		if buildErr != nil {
			if flow != nil {
				// K2: error path flow log carries the original error.
				flow.LogError("system.agent.build", "Agent 构建失败",
					event.P("agent_id", ag.ID), event.P("agent_key", ag.AgentKey), event.P("error", buildErr.Error()))
			}
			lg.Warn("Agent 构建失败",
				loggateway.StepID("agent.cache_build_fail"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Str("cache_key", key),
				loggateway.Err(buildErr))
			return nil, buildErr
		}
		if h := globalBuildCache.put(key, built, nil, toolSets); h != nil {
			// 返回代际句柄持有的包装 agent（Run 作用域引用计数，P0-4）；
			// put 返回 nil 仅发生在缓存已关闭（进程退出中），退回原始构建产物。
			built = h.agent
		}
		if flow != nil {
			flow.LogDone("system.agent.build", "Agent 构建完成",
				event.P("agent_id", ag.ID), event.P("agent_key", ag.AgentKey))
		}
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
