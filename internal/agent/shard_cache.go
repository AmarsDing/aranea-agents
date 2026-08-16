package agent

import (
	"container/list"
	"context"
	"sync"
	"time"

	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// shard_cache.go — P0-2 阶段A：分片构建产物缓存。
//
// 背景：agent 构建冷路径实测 3-7s（report-05 FR-2）。BuildCache 以整体配置
// 指纹为 key，任何 full_rebuild 字段变化都整实例重建。本缓存把构建产物按
// 装配组切片（core / mcp:<server> / knowledge / media / memory / twinops /
// officecli / custom / mcp_broker），每片独立指纹、独立缓存：
// 配置变更时仅指纹变化的片重建，其余片 100% 复用（FR-2.2）；同一 MCP
// server 被多 agent 引用时第二 agent 直接命中（AC-4）。
//
// 所有权模型（与 BuildCache graveyard + P0-4 代际引用计数衔接）：
//   - 分片产物可被多个存活 agent 代际共享 → BuildCache entry 不再直接拥有
//     toolsets；每次 acquire 配发一个 shardHoldToolSet 占位符进入
//     entry.toolSets（retire 单元），entry 换代/驱逐时经 graveyard 在
//     在途 run 排空后 Close 占位符 = 释放分片引用（refs--）。
//   - 分片条目 refs==0 才被 LRU 淘汰，淘汰即同步 Close 产物（此时无任何
//     存活或未排空代际引用它，在途调用不可能存在，无需 graveyard 延迟）。
//   - Close（进程退出）：refs==0 立即关闭；refs>0 标记 closing 移出索引，
//     最后一次 release 时关闭（与 MCPToolSetPool.Close 同一先例）。
//
// 缓存的产物是「原始」工具集：确认门 / 装饰器 / 消歧 / 延迟包装 / MCP 治理
// 全部在合并期以非变异包装器形式每次构建重放（均已核实不原地修改共享
// 产物），因此共享安全。

// shardProduct 是一片装配组的原始构建产物（未过门/装饰器/治理/延迟包装）。
type shardProduct struct {
	toolSets []trpctool.ToolSet
	tools    []trpctool.Tool
}

// shardSpec 描述一个待获取分片：稳定 ID、组名（指标标签）、输入指纹、
// 是否可缓存（凭证注入型 MCP server 不可共享，见 tools.MCPServerConfigShareable）
// 以及未命中时的构建函数。
type shardSpec struct {
	id        string
	group     string
	fp        string
	cacheable bool
	build     func(ctx context.Context) (*shardProduct, error)
}

// shardCacheEntry 是一条缓存分片。
type shardCacheEntry struct {
	fp      string
	group   string
	id      string
	prod    *shardProduct
	refs    int
	elem    *list.Element
	closing bool // Close 后摘除索引、等待 refs 归零的条目
}

// shardCache 是分片产物的进程级 LRU。并发安全。
type shardCache struct {
	mu      sync.Mutex
	cap     int
	items   map[string]*shardCacheEntry
	lruList *list.List // front = most-recently-used
	lg      loggateway.Logger
	closed  bool
}

var shardCacheDefaultCap = 256

var globalShardCache = newShardCache(shardCacheDefaultCap)

// ShardCache 是分片构建缓存的对外句柄（Wire 生命周期注册用）。具体类型
// 不导出：构建路径经包内 globalShardCache 直接访问。
type ShardCache interface {
	// Close 关闭全部空闲分片；被引用分片标记 closing 并在最后一次 release
	// 时关闭。幂等。
	Close() error
	// SetLogger 注入进程日志（Wire 启动时调用一次）。
	SetLogger(lg loggateway.Logger)
}

func newShardCache(cap int) *shardCache {
	if cap <= 0 {
		cap = shardCacheDefaultCap
	}
	return &shardCache{
		cap:     cap,
		items:   make(map[string]*shardCacheEntry),
		lruList: list.New(),
		lg:      loggateway.NewNoop(),
	}
}

// GetGlobalShardCache returns the process-level shard cache singleton so Wire
// can register it with the LifecycleManager for orderly shutdown.
func GetGlobalShardCache() ShardCache {
	return globalShardCache
}

// SetLogger injects the process logger (called once from Wire).
func (c *shardCache) SetLogger(lg loggateway.Logger) {
	if lg == nil {
		return
	}
	c.mu.Lock()
	c.lg = lg
	c.mu.Unlock()
}

// acquire 取回分片产物与释放函数。调用方持有产物期间必须持有引用；
// 释放函数幂等，由 retire 单元（shardHoldToolSet.Close）触发。
//
// 并发去重：不加 singleflight——同 fp 并发构建时后到者丢弃自己的构建产物
// （关闭之）并引用先到者的条目；agent 级 singleflight 已吸收绝大多数
// 同 key 并发，此处仅兜底跨 agent 的偶然撞车。
func (c *shardCache) acquire(ctx context.Context, spec shardSpec) (*shardProduct, func(), error) {
	if spec.build == nil {
		return &shardProduct{}, func() {}, nil
	}
	if !spec.cacheable {
		// 不可共享产物（如凭证注入型 MCP server）：每次构建新建，释放即关闭。
		prod, err := spec.build(ctx)
		if err != nil {
			return nil, nil, err
		}
		return prod, c.directReleaseFunc(prod), nil
	}

	c.mu.Lock()
	if !c.closed {
		if e, ok := c.items[spec.fp]; ok {
			e.refs++
			c.lruList.MoveToFront(e.elem)
			c.mu.Unlock()
			arametrics.AgentBuildShardHits.WithLabelValues(spec.group).Inc()
			return e.prod, c.releaseFunc(e), nil
		}
	}
	c.mu.Unlock()

	arametrics.AgentBuildShardMisses.WithLabelValues(spec.group).Inc()
	start := time.Now()
	prod, err := spec.build(ctx)
	if err != nil {
		return nil, nil, err
	}
	arametrics.AgentBuildShardBuildSeconds.WithLabelValues(spec.group).Observe(time.Since(start).Seconds())

	c.mu.Lock()
	if c.closed {
		// 关闭后降级：不进缓存，释放时直接关闭（进程退出期的兜底构建仍可用）。
		c.mu.Unlock()
		return prod, c.directReleaseFunc(prod), nil
	}
	if e, ok := c.items[spec.fp]; ok {
		// 并发撞车：先到者已入库，丢弃本地构建产物（关闭释放其池引用）。
		e.refs++
		c.lruList.MoveToFront(e.elem)
		lg := c.lg
		c.mu.Unlock()
		go closeToolSetsNow(lg, prod.toolSets, spec.fp)
		return e.prod, c.releaseFunc(e), nil
	}
	e := &shardCacheEntry{fp: spec.fp, group: spec.group, id: spec.id, prod: prod, refs: 1}
	e.elem = c.lruList.PushFront(e)
	c.items[spec.fp] = e
	evicted := c.evictLocked()
	lg := c.lg
	c.mu.Unlock()
	for _, ev := range evicted {
		closeToolSetsNow(lg, ev.prod.toolSets, ev.fp)
	}
	c.lg.Info("分片构建完成并入缓存",
		loggateway.StepID("agent.shard_built"),
		loggateway.Str("shard_id", spec.id),
		loggateway.Str("shard_group", spec.group),
		loggateway.Int64("build_ms", time.Since(start).Milliseconds()))
	return prod, c.releaseFunc(e), nil
}

// evictLocked 在容量超限时从 LRU 尾部淘汰 refs==0 的条目，返回待关闭列表
// （调用方须在锁外关闭）。refs>0 的条目被存活 agent 代际引用，不可淘汰。
func (c *shardCache) evictLocked() []*shardCacheEntry {
	var evicted []*shardCacheEntry
	for len(c.items) > c.cap {
		back := c.lruList.Back()
		for back != nil && back.Value.(*shardCacheEntry).refs > 0 {
			back = back.Prev()
		}
		if back == nil {
			break // 全部被引用，允许暂超容量
		}
		e := back.Value.(*shardCacheEntry)
		c.lruList.Remove(back)
		delete(c.items, e.fp)
		evicted = append(evicted, e)
	}
	return evicted
}

// releaseFunc 返回条目的引用释放闭包（幂等）。
func (c *shardCache) releaseFunc(e *shardCacheEntry) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			e.refs--
			drain := e.refs <= 0 && e.closing
			lg := c.lg
			c.mu.Unlock()
			if drain {
				closeToolSetsNow(lg, e.prod.toolSets, e.fp)
			}
		})
	}
}

// directReleaseFunc 用于未入缓存的产物：释放即关闭。
func (c *shardCache) directReleaseFunc(prod *shardProduct) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			closeToolSetsNow(c.lg, prod.toolSets, "uncached-shard")
		})
	}
}

// Close 关闭全部空闲分片；仍被引用的分片标记 closing 并移出索引，最后一
// 次 release 时关闭（shutdown use-after-close 防护）。幂等，满足 io.Closer /
// lifecycle.Closer 以便注册进 LifecycleManager。
func (c *shardCache) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	var idle []*shardCacheEntry
	for fp, e := range c.items {
		delete(c.items, fp)
		if e.refs <= 0 {
			idle = append(idle, e)
		} else {
			e.closing = true
		}
	}
	c.lruList.Init()
	lg := c.lg
	c.mu.Unlock()
	for _, e := range idle {
		closeToolSetsNow(lg, e.prod.toolSets, e.fp)
	}
	return nil
}

// shardHoldToolSet 是分片引用的 retire 单元占位符：随 agent 缓存 entry 的
// toolSets 一起进 graveyard，在途 run 排空（P0-4 代际引用计数）后被
// sweeper Close = 释放一次分片引用。它永不注册进 LLM agent。
type shardHoldToolSet struct {
	name    string
	release func()
	once    sync.Once
}

var _ trpctool.ToolSet = (*shardHoldToolSet)(nil)

func newShardHoldToolSet(shardID string, release func()) *shardHoldToolSet {
	return &shardHoldToolSet{name: "shard_hold:" + shardID, release: release}
}

func (h *shardHoldToolSet) Name() string { return h.name }

// Tools 恒返回空——占位符不参与工具面。
func (h *shardHoldToolSet) Tools(context.Context) []trpctool.Tool { return nil }

// Close 幂等释放分片引用。
func (h *shardHoldToolSet) Close() error {
	h.once.Do(func() {
		if h.release != nil {
			h.release()
		}
	})
	return nil
}
