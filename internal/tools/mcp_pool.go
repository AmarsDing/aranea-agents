package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// mcp_pool.go — 进程级 MCP ToolSet 连接池。
//
// 问题：agent 构建缓存 miss 时，buildMCPToolSet + Init 每次都全量建连
//（stdio 子进程启动 / TCP 握手 + initialize + tools/list），缓存驱逐时又
// 整体 Close。MCP 服务器配置变化频率远低于构建 miss 频率，连接本可复用。
//
// 设计：
//   - 池 key = sha256(规范化连接配置 + name + toolPrefix + reconnect 策略)，
//     配置相同即共享同一条连接（map 经 json.Marshal 键序无关）。
//   - HeaderInjector != nil 的配置不池化：按请求注入用户凭证的会话若跨用户
//     共享会把首个建连用户的凭证泄漏给其他用户（凭证隔离红线）。
//   - 池化 wrapper 的 Close 只做 release（引用计数 --），真实 Close 由 reaper
//     在空闲超过 idleTTL 后执行；agent 缓存驱逐因此不再断连。
//   - Init 在建 entry 时执行一次，失败仅告警不失败（Always-Ready：框架在
//     Tools() 调用时自动重连）。
//   - Pool.Close 幂等，关闭全部 entry；关闭后 Acquire 降级为非池化新建，
//     保证 shutdown 期间的兜底构建仍可用。

const (
	defaultMCPPoolIdleTTL      = 10 * time.Minute
	defaultMCPPoolReapInterval = time.Minute
)

// mcpToolSetFactory builds one MCP ToolSet for the given config. Injectable
// for tests; production default is buildMCPToolSet.
type mcpToolSetFactory func(MCPServerConfig) (ToolSet, error)

// mcpPoolEntry is one pooled connection with its reference count.
type mcpPoolEntry struct {
	ts          ToolSet
	refs        int
	lastIdleAt  time.Time // last time refs dropped to zero
	initialized bool
}

// MCPToolSetPool is a process-level pool of live MCP ToolSets keyed by
// canonical connection config. Safe for concurrent use.
type MCPToolSetPool struct {
	mu           sync.Mutex
	entries      map[string]*mcpPoolEntry
	factory      mcpToolSetFactory
	lg           loggateway.Logger
	idleTTL      time.Duration
	reapInterval time.Duration
	reaperOnce   sync.Once
	ctx          context.Context
	cancel       context.CancelFunc
	closed       bool
}

// NewMCPToolSetPool creates the production pool with default idle policy.
func NewMCPToolSetPool(lg loggateway.Logger) *MCPToolSetPool {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return newMCPToolSetPool(lg, buildMCPToolSet, defaultMCPPoolIdleTTL, defaultMCPPoolReapInterval)
}

// newMCPToolSetPoolWithFactory builds a pool for tests: custom factory, custom
// idle TTL, and NO reaper goroutine (tests drive reapIdle directly).
func newMCPToolSetPoolWithFactory(factory mcpToolSetFactory, idleTTL time.Duration) *MCPToolSetPool {
	return newMCPToolSetPool(loggateway.NewNoop(), factory, idleTTL, defaultMCPPoolReapInterval)
}

func newMCPToolSetPool(lg loggateway.Logger, factory mcpToolSetFactory, idleTTL, reapInterval time.Duration) *MCPToolSetPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &MCPToolSetPool{
		entries:      make(map[string]*mcpPoolEntry),
		factory:      factory,
		lg:           lg.With(loggateway.Domain("tools.mcp.pool")),
		idleTTL:      idleTTL,
		reapInterval: reapInterval,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetLogger injects the process logger (called once from Wire).
func (p *MCPToolSetPool) SetLogger(lg loggateway.Logger) {
	if lg == nil {
		return
	}
	p.mu.Lock()
	p.lg = lg.With(loggateway.Domain("tools.mcp.pool"))
	p.mu.Unlock()
}

// mcpPoolKey canonicalizes the effective connection parameters into a stable
// hash. The second return value is false when the config must not be pooled
// (per-request credential injection).
func mcpPoolKey(cfg MCPServerConfig) (string, bool) {
	if cfg.HeaderInjector != nil {
		return "", false
	}
	conn := cfg.ToConnectionConfig()
	keyMaterial := struct {
		Name      string      `json:"name"`
		Conn      interface{} `json:"conn"`
		Prefix    string      `json:"prefix"`
		Reconnect int         `json:"reconnect"`
	}{cfg.Name, conn, cfg.ToolPrefix, cfg.SessionReconnectMax}
	raw, err := json.Marshal(keyMaterial)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), true
}

// Acquire returns a ToolSet for the given config. Poolable configs share the
// underlying connection across acquisitions; non-poolable configs (credential
// injection) build a fresh ToolSet per call. Init failures degrade to a
// warning — the ToolSet is still returned and the framework retries at
// Tools() call time (Always-Ready semantics).
func (p *MCPToolSetPool) Acquire(ctx context.Context, cfg MCPServerConfig) (ToolSet, error) {
	key, poolable := mcpPoolKey(cfg)
	if !poolable {
		return p.factory(cfg)
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		// Shutdown race: degrade to unpooled so late builds still work.
		return p.factory(cfg)
	}
	if e, ok := p.entries[key]; ok {
		e.refs++
		p.mu.Unlock()
		p.lg.Debug("MCP 连接池命中",
			loggateway.StepID("tools.mcp.pool_hit"),
			loggateway.Str("server", cfg.Name))
		return &pooledMCPToolSet{pool: p, key: key, inner: e.ts}, nil
	}
	p.mu.Unlock()

	// Build outside the lock: Init may block on process spawn / TCP connect.
	ts, err := p.factory(cfg)
	if err != nil {
		return nil, err
	}
	if ts == nil {
		return nil, nil
	}
	if init, ok := ts.(interface{ Init(context.Context) error }); ok {
		if initErr := init.Init(ctx); initErr != nil {
			p.lg.Warn("MCP ToolSet 初始化失败（降级运行，将在调用时重试）",
				loggateway.StepID("tools.mcp.init_fail"),
				loggateway.Str("server", cfg.Name),
				loggateway.Err(initErr))
		}
	}

	p.mu.Lock()
	// Double-check: a concurrent Acquire may have created the same entry while
	// we were connecting. Keep the first (already-shared) connection and close
	// the duplicate we just built.
	if e, ok := p.entries[key]; ok && !p.closed {
		e.refs++
		p.mu.Unlock()
		if err := ts.Close(); err != nil {
			p.lg.Warn("MCP 重复连接关闭失败", loggateway.StepID("tools.mcp.pool_dup_close"), loggateway.Str("server", cfg.Name), loggateway.Err(err))
		}
		return &pooledMCPToolSet{pool: p, key: key, inner: e.ts}, nil
	}
	if p.closed {
		p.mu.Unlock()
		if err := ts.Close(); err != nil {
			p.lg.Warn("MCP 连接关闭失败", loggateway.StepID("tools.mcp.pool_close"), loggateway.Str("server", cfg.Name), loggateway.Err(err))
		}
		return ts, nil
	}
	p.entries[key] = &mcpPoolEntry{ts: ts, refs: 1, initialized: true}
	p.mu.Unlock()

	p.lg.Info("MCP 连接池新建连接",
		loggateway.StepID("tools.mcp.pool_create"),
		loggateway.Str("server", cfg.Name))
	p.startReaperOnce()
	return &pooledMCPToolSet{pool: p, key: key, inner: ts}, nil
}

// release decrements the reference count for key. Called by the pooled
// wrapper's Close.
func (p *MCPToolSetPool) release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[key]
	if !ok {
		return
	}
	e.refs--
	if e.refs <= 0 {
		e.refs = 0
		e.lastIdleAt = time.Now()
	}
}

// reapIdle closes entries that have been unreferenced for longer than the
// idle TTL. Safe to call directly from tests.
func (p *MCPToolSetPool) reapIdle(now time.Time) {
	p.mu.Lock()
	var victims []struct {
		key string
		ts  ToolSet
	}
	for key, e := range p.entries {
		if e.refs == 0 && now.Sub(e.lastIdleAt) >= p.idleTTL {
			victims = append(victims, struct {
				key string
				ts  ToolSet
			}{key, e.ts})
			delete(p.entries, key)
		}
	}
	p.mu.Unlock()
	for _, v := range victims {
		if err := v.ts.Close(); err != nil {
			p.lg.Warn("MCP 空闲连接关闭失败", loggateway.StepID("tools.mcp.pool_reap"), loggateway.Err(err))
			continue
		}
		p.lg.Info("MCP 空闲连接已回收", loggateway.StepID("tools.mcp.pool_reap"), loggateway.Str("pool_key", v.key[:12]))
	}
}

// startReaperOnce launches the background idle reaper exactly once.
func (p *MCPToolSetPool) startReaperOnce() {
	p.reaperOnce.Do(func() {
		safego.Go(p.ctx, "tools.mcp.pool_reaper", func() {
			p.lg.Info("MCP 连接池回收器启动", loggateway.StepID("tools.mcp.pool_reaper_start"))
			ticker := time.NewTicker(p.reapInterval)
			defer ticker.Stop()
			for {
				select {
				case <-p.ctx.Done():
					p.lg.Info("MCP 连接池回收器退出", loggateway.StepID("tools.mcp.pool_reaper_stop"))
					return
				case now := <-ticker.C:
					p.reapIdle(now)
				}
			}
		})
	})
}

// Close stops the reaper and closes every pooled connection. Idempotent.
// Satisfies io.Closer for LifecycleManager registration.
func (p *MCPToolSetPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.cancel()
	entries := p.entries
	p.entries = make(map[string]*mcpPoolEntry)
	p.mu.Unlock()

	for key, e := range entries {
		if err := e.ts.Close(); err != nil {
			p.lg.Warn("MCP 连接池关闭连接失败", loggateway.StepID("tools.mcp.pool_close"), loggateway.Str("pool_key", key[:12]), loggateway.Err(err))
		}
	}
	p.lg.Info("MCP 连接池已关闭", loggateway.StepID("tools.mcp.pool_close"), loggateway.Int("entries", len(entries)))
	return nil
}

// pooledMCPToolSet wraps a pooled ToolSet. Close releases the pool reference
// instead of closing the connection; Tools/Name delegate. Init delegates so
// the assembly-time resilience probe (connectivity check + warn) still works —
// the framework's Init is a no-op when the session is already connected.
type pooledMCPToolSet struct {
	pool  *MCPToolSetPool
	key   string
	inner ToolSet
	once  sync.Once
}

var _ ToolSet = (*pooledMCPToolSet)(nil)

func (w *pooledMCPToolSet) Name() string { return w.inner.Name() }

func (w *pooledMCPToolSet) Tools(ctx context.Context) []trpctool.Tool {
	return w.inner.Tools(ctx)
}

func (w *pooledMCPToolSet) Close() error {
	w.once.Do(func() { w.pool.release(w.key) })
	return nil
}

// Init implements the optional Init interface probed by assembleMCPTools.
func (w *pooledMCPToolSet) Init(ctx context.Context) error {
	if init, ok := w.inner.(interface{ Init(context.Context) error }); ok {
		return init.Init(ctx)
	}
	return nil
}

var globalMCPToolSetPool = NewMCPToolSetPool(nil)

// GetGlobalMCPToolSetPool returns the process-level pool singleton so Wire can
// register it with the LifecycleManager for orderly shutdown.
func GetGlobalMCPToolSetPool() *MCPToolSetPool {
	return globalMCPToolSetPool
}

// acquireMCPToolSet routes MCP ToolSet construction through the process-level
// pool. Credential-injected configs bypass the pool automatically.
func acquireMCPToolSet(ctx context.Context, cfg MCPServerConfig) (ToolSet, error) {
	return globalMCPToolSetPool.Acquire(ctx, cfg)
}

// PrewarmMCPToolSet establishes (or reuses) the pooled connection for cfg and
// immediately releases the reference; the connection stays warm in the pool
// until the idle reaper collects it. Callers must skip credential-injected
// (RequireUserCredentials) servers — they are never poolable and per-user
// credentials are unavailable at startup.
func PrewarmMCPToolSet(ctx context.Context, cfg MCPServerConfig) error {
	ts, err := globalMCPToolSetPool.Acquire(ctx, cfg)
	if err != nil || ts == nil {
		return err
	}
	return ts.Close()
}
