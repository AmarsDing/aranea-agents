# 03-Graph Cache 与 Retry

## 一、需求文档

### 1.1 背景

当前 Aranea-Agents 的 Graph 编排层（`internal/graph/trpc/`）已实现基本的工作流构建和断路器/故障恢复（`circuit_breaker.go`、`failure_recovery.go`），但缺乏以下关键生产级能力：

- **节点级缓存**：重复输入的 Graph 节点每次都重新执行，浪费 LLM 调用和计算资源
- **自动重试**：瞬态错误（网络超时、LLM 限流）导致整个工作流失败，缺乏自动重试
- **可配置重试策略**：无法针对不同节点配置不同的重试策略（如 LLM 节点重试 3 次、工具节点重试 1 次）

框架 `pkg/trpc-agent-go/graph/` 已提供完整的 Cache 和 Retry 实现：

| 组件 | 文件路径 | 核心接口/结构 |
|------|----------|--------------|
| Cache 接口 | `graph/cache.go` | `Cache` 接口（Get/Set/Clear）+ `CachePolicy` + `InMemoryCache` |
| Retry 接口 | `graph/retry.go` | `RetryCondition` 接口 + `RetryPolicy` 结构体 + 便捷构造函数 |

### 1.2 目标

1. **节点级缓存**：集成框架 `graph.Cache` 接口，对 Graph 节点输出进行缓存，避免重复计算
2. **自动重试**：集成框架 `graph.RetryPolicy`，对瞬态错误自动重试
3. **配置驱动**：通过 `GraphBuildConfig` 配置每个节点的缓存策略和重试策略
4. **可观测性**：缓存命中/未命中、重试次数通过 FlowLog 记录

### 1.3 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| G-P0-1 | InMemoryCache 集成 | 使用框架 `InMemoryCache` 实现节点级缓存 |
| G-P0-2 | CachePolicy 配置 | 通过 `NodeDef` 配置每个节点的 `CachePolicy`（KeyFunc + TTL） |
| G-P0-3 | RetryPolicy 集成 | 使用框架 `RetryPolicy` 实现自动重试 |
| G-P0-4 | RetryCondition 配置 | 使用框架 `RetryOnErrors`/`RetryOnPredicate` 配置重试条件 |
| G-P0-5 | NodeDef 扩展 | 在 `NodeDef` 中新增 `CachePolicy` 和 `RetryPolicy` 字段 |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|------|
| G-P1-1 | Redis Cache 后端 | 实现 `graph.Cache` 接口的 Redis 版本，支持多实例共享缓存 |
| G-P1-2 | DefaultTransientCondition | 使用框架 `DefaultTransientCondition()` 作为默认重试条件 |
| G-P1-3 | WithSimpleRetry 便捷配置 | 使用框架 `WithSimpleRetry(attempts)` 快速配置重试 |
| G-P1-4 | 缓存命名空间隔离 | 使用框架 `buildCacheNamespace(nodeID)` 隔离不同节点的缓存 |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|----------|
| G-P2-1 | 缓存指标 | 缓存命中率、缓存大小通过 Prometheus 暴露 |
| G-P2-2 | 重试指标 | 重试次数、重试延迟分布通过 Prometheus 暴露 |
| G-P2-2 | 自定义 KeyFunc | 允许用户为特定节点提供自定义缓存键生成函数 |

### 1.4 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | 缓存 Get/Set P99 < 1ms（InMemory），< 5ms（Redis） |
| 可靠性 | 重试不改变业务语义，at-least-once 执行 |
| 兼容性 | 不影响现有无缓存/无重试的 Graph 节点执行 |
| 可观测性 | 缓存命中/未命中、重试次数通过 FlowLog 记录 |
| 内存安全 | InMemoryCache 的 TTL 过期自动清理，防止内存泄漏 |

### 1.5 验收标准

1. 配置了 `CachePolicy` 的节点，相同输入第二次执行直接返回缓存结果
2. 配置了 `RetryPolicy` 的节点，瞬态错误自动重试，最终成功返回结果
3. 未配置 Cache/Retry 的节点行为与现有完全一致
4. `InMemoryCache` 的 TTL 过期后缓存自动失效
5. `make build && make test` 全部通过

---

## 二、设计文档

### 2.1 框架参考

#### Cache 接口 — `pkg/trpc-agent-go/graph/cache.go`

```go
type Cache interface {
    Get(ns, key string) (val any, ok bool)
    Set(ns, key string, val any, ttl time.Duration)
    Clear(ns string)
}
```

#### CachePolicy 结构体

```go
type CachePolicy struct {
    KeyFunc func(input any) ([]byte, error)
    TTL     time.Duration
}
```

#### InMemoryCache 实现

```go
type InMemoryCache struct { ... }

func NewInMemoryCache() *InMemoryCache
```

特性：

- TTL 过期自动清理
- `deepCopy` 深拷贝防止缓存值被外部修改
- 命名空间隔离：`buildCacheNamespace(nodeID)` → `"__writes__:" + nodeID`

#### RetryCondition 接口 — `pkg/trpc-agent-go/graph/retry.go`

```go
type RetryCondition interface {
    Match(err error) bool
}
```

#### RetryPolicy 结构体

```go
type RetryPolicy struct {
    MaxAttempts       int
    InitialInterval   time.Duration
    BackoffFactor     float64
    MaxInterval       time.Duration
    Jitter            time.Duration
    RetryOn           []RetryCondition
    MaxElapsedTime    time.Duration
    PerAttemptTimeout time.Duration
}

func (p RetryPolicy) NextDelay(attempt int) time.Duration
func (p RetryPolicy) ShouldRetry(err error) bool
```

#### 便捷构造函数

```go
func WithSimpleRetry(attempts int) RetryPolicy

func RetryOnErrors(targets ...error) RetryCondition
func RetryOnPredicate(match func(error) bool) RetryCondition
func DefaultTransientCondition() RetryCondition
```

`NextDelay` 计算逻辑：指数退避 + 抖动

```
delay = min(InitialInterval * BackoffFactor^attempt, MaxInterval)
delay = delay + rand(0, Jitter)
```

### 2.2 当前项目现状

当前 Graph 编排在 `internal/graph/trpc/` 目录下：

| 文件 | 职责 |
|------|------|
| `builder.go` | `NodeDef`/`GraphBuildConfig`/`SubgraphDef` 类型定义，构建 `trpcgraph.Workflow` |
| `circuit_breaker.go` | 断路器模式实现 |
| `failure_recovery.go` | 故障恢复策略 |

`NodeDef` 当前字段：

```go
type NodeDef struct {
    ID             string
    FuncRef        string
    Type           string
    RetryMaxAttempts int
    FailureAction  string
}
```

已有 `RetryMaxAttempts` 字段但未与框架 `RetryPolicy` 集成，仅作为简单重试次数使用。无 `CachePolicy` 相关字段。

### 2.3 架构设计

#### 2.3.1 整体架构

```
internal/graph/trpc/builder.go
  └─ NodeDef 扩展
       ├─ CachePolicy *graph.CachePolicy  ← 新增
       └─ RetryPolicy *graph.RetryPolicy  ← 新增（替换 RetryMaxAttempts）
       │
       ▼
internal/graph/trpc/cache.go  ← 新增：Cache 工厂
  ├─ NewInMemoryCache()  → graph.InMemoryCache
  └─ NewRedisCache(...)  → RedisCache (P1)

internal/graph/trpc/retry.go  ← 新增：Retry 策略构建
  ├─ BuildRetryPolicy(def NodeDef) → graph.RetryPolicy
  └─ DefaultRetryPolicy() → graph.RetryPolicy
```

#### 2.3.2 NodeDef 扩展

```go
type NodeDef struct {
    ID            string
    FuncRef       string
    Type          string
    CachePolicy   *CachePolicyDef
    RetryPolicy   *RetryPolicyDef
    FailureAction string
}

type CachePolicyDef struct {
    Enabled bool
    TTL     time.Duration
}

type RetryPolicyDef struct {
    MaxAttempts       int
    InitialIntervalMs int64
    BackoffFactor     float64
    MaxIntervalMs     int64
    JitterMs          int64
    PerAttemptTimeoutMs int64
}
```

#### 2.3.3 Cache 工厂

新增 `internal/graph/trpc/cache.go`：

```go
type CacheFactory interface {
    NewCache(ctx context.Context) (graph.Cache, error)
}

type inMemoryCacheFactory struct{}

func (f *inMemoryCacheFactory) NewCache(ctx context.Context) (graph.Cache, error) {
    return graph.NewInMemoryCache(), nil
}
```

P1 阶段新增 Redis Cache：

```go
type redisCacheFactory struct {
    client *redis.Client
}

func (f *redisCacheFactory) NewCache(ctx context.Context) (graph.Cache, error) {
    return NewRedisCache(f.client), nil
}
```

#### 2.3.4 Retry 策略构建

新增 `internal/graph/trpc/retry.go`：

```go
func BuildRetryPolicy(def *RetryPolicyDef) graph.RetryPolicy {
    if def == nil {
        return graph.RetryPolicy{}
    }
    return graph.RetryPolicy{
        MaxAttempts:       def.MaxAttempts,
        InitialInterval:   time.Duration(def.InitialIntervalMs) * time.Millisecond,
        BackoffFactor:     def.BackoffFactor,
        MaxInterval:       time.Duration(def.MaxIntervalMs) * time.Millisecond,
        Jitter:            time.Duration(def.JitterMs) * time.Millisecond,
        PerAttemptTimeout: time.Duration(def.PerAttemptTimeoutMs) * time.Millisecond,
        RetryOn:           []graph.RetryCondition{graph.DefaultTransientCondition()},
    }
}
```

#### 2.3.5 Builder 集成

在 `builder.go` 的 Graph 构建流程中，将 `CachePolicy` 和 `RetryPolicy` 注入到框架的节点配置：

```go
func (b *Builder) buildNode(def NodeDef, cache graph.Cache) trpcgraph.NodeOption {
    opts := []trpcgraph.NodeOption{}

    if def.CachePolicy != nil && def.CachePolicy.Enabled {
        opts = append(opts, trpcgraph.WithCache(cache, graph.CachePolicy{
            KeyFunc: defaultKeyFunc,
            TTL:     def.CachePolicy.TTL,
        }))
    }

    if def.RetryPolicy != nil {
        opts = append(opts, trpcgraph.WithRetry(BuildRetryPolicy(def.RetryPolicy)))
    }

    return trpcgraph.WithNodeOptions(opts...)
}
```

#### 2.3.6 Wire 注入

`internal/graph/trpc/provider.go` 新增 Cache 工厂绑定：

```go
var ProviderSet = wire.NewSet(
    NewBuilder,
    NewCacheFactory,
    wire.Bind(new(CacheFactory), new(*inMemoryCacheFactory)),
)
```

### 2.4 与框架的集成方式

| 集成点 | 框架包 | 项目适配层 | 说明 |
|--------|--------|-----------|------|
| Cache 接口 | `graph.Cache` | `internal/graph/trpc/cache.go` | 直接使用 `InMemoryCache`，P1 扩展 Redis |
| CachePolicy | `graph.CachePolicy` | `NodeDef.CachePolicy` | 配置转换：`CachePolicyDef` → `graph.CachePolicy` |
| RetryPolicy | `graph.RetryPolicy` | `internal/graph/trpc/retry.go` | 配置转换：`RetryPolicyDef` → `graph.RetryPolicy` |
| RetryCondition | `graph.RetryCondition` | 同上 | 使用 `DefaultTransientCondition()` + `RetryOnErrors()` |
| 命名空间 | `buildCacheNamespace` | 框架内部使用 | 节点 ID 自动隔离 |
| 便捷构造 | `WithSimpleRetry` | `BuildRetryPolicy` | 简单场景可使用 `WithSimpleRetry` |

**关键原则**：Cache 和 Retry 的核心逻辑全部由框架提供，项目只做配置转换和 Wire 装配。不复制框架的指数退避、深拷贝、TTL 清理等内部逻辑。

### 2.5 错误处理

| 场景 | 错误类型 | 处理方式 |
|------|----------|----------|
| Cache KeyFunc 执行失败 | FlowLog 记录，跳过缓存 | 降级为无缓存执行 |
| Cache Get/Set 失败 | FlowLog 记录，跳过缓存 | 降级为无缓存执行 |
| 重试耗尽仍失败 | 返回最后一次错误 | 由 `FailureAction` 决定后续处理 |
| PerAttemptTimeout 超时 | `context.DeadlineExceeded` | 触发重试条件 |
| Redis 连接失败 | `kerrors.InternalServer("GRAPH", ...)` | 启动时 Fail Fast |

---

## 三、开发计划

### 3.1 任务拆解

| # | 任务 | 涉及文件 | 依赖 | 预估 |
|---|------|----------|------|------|
| T1 | 扩展 NodeDef 结构 | `internal/graph/trpc/builder.go` | 无 | 0.5d |
| T2 | 新增 CachePolicyDef/RetryPolicyDef | `internal/graph/trpc/builder.go` | T1 | 0.5d |
| T3 | 新增 Cache 工厂 | `internal/graph/trpc/cache.go` | T1 | 1d |
| T4 | 新增 Retry 策略构建 | `internal/graph/trpc/retry.go` | T2 | 1d |
| T5 | Builder 集成 Cache/Retry | `internal/graph/trpc/builder.go` | T3, T4 | 1d |
| T6 | Wire ProviderSet 适配 | `internal/graph/trpc/provider.go` | T3 | 0.5d |
| T7 | 默认 KeyFunc 实现 | `internal/graph/trpc/cache.go` | T3 | 0.5d |
| T8 | 集成测试 | `internal/graph/trpc/cache_test.go`, `retry_test.go` | T3, T4, T5 | 1d |
| T9 | `make wire && make build && make test` 验证 | 全局 | T1-T7 | 0.5d |

### 3.2 开发顺序

```
Phase 1 — 类型定义（T1 → T2）
  ├─ T1: NodeDef 扩展
  └─ T2: CachePolicyDef/RetryPolicyDef 定义

Phase 2 — 核心实现（T3 → T4 → T7）
  ├─ T3: Cache 工厂 + InMemoryCache
  ├─ T4: Retry 策略构建
  └─ T7: 默认 KeyFunc

Phase 3 — 集成（T5 → T6）
  ├─ T5: Builder 集成 Cache/Retry
  └─ T6: Wire ProviderSet 适配

Phase 4 — 验证（T8 → T9）
  ├─ T8: 集成测试
  └─ T9: 全量构建验证
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| InMemoryCache 缓存命中 | `go test ./internal/graph/trpc/... -run TestCacheHit -count=1` | 相同输入第二次返回缓存 |
| InMemoryCache TTL 过期 | `go test ./internal/graph/trpc/... -run TestCacheTTL -count=1` | TTL 过期后缓存失效 |
| InMemoryCache 命名空间隔离 | `go test ./internal/graph/trpc/... -run TestCacheNamespace -count=1` | 不同节点缓存互不干扰 |
| RetryPolicy 指数退避 | `go test ./internal/graph/trpc/... -run TestRetryBackoff -count=1` | 延迟按指数增长 |
| RetryPolicy 瞬态重试 | `go test ./internal/graph/trpc/... -run TestRetryTransient -count=1` | 瞬态错误自动重试成功 |
| RetryPolicy 耗尽失败 | `go test ./internal/graph/trpc/... -run TestRetryExhausted -count=1` | 重试耗尽返回最后错误 |
| 无配置向后兼容 | `go test ./internal/graph/trpc/... -run TestNoCacheNoRetry -count=1` | 未配置时行为不变 |
| 全量构建 | `make wire && make build && make test` | 零错误 |
