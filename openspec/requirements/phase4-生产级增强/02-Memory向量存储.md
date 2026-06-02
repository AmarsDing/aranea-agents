# 02-Memory 向量存储

## 一、需求文档

### 1.1 背景

当前 Aranea-Agents 的 Memory 存储使用 SQLite 适配器（`internal/memory/trpc/sqlite_adapter.go` 中的 `sqliteMemoryService`），通过 `sessionmemory.Store` 进行关键词搜索。存在以下问题：

- **搜索精度低**：仅支持关键词匹配，无法理解语义相似性
- **缺乏向量索引**：SQLite 无向量搜索能力，无法执行相似度检索
- **缺乏混合搜索**：无法结合向量搜索和关键词搜索的优势
- **缺乏外部记忆平台集成**：无法接入 Mem0 等专业记忆管理平台

框架 `pkg/trpc-agent-go/memory/` 已提供多种后端实现：

| 后端 | 文件路径 | 实现接口 | 特性 |
|------|----------|----------|------|
| `memory.Service` | `memory/memory.go` | 8 方法核心接口 | AddMemory/UpdateMemory/DeleteMemory/ClearMemories/ReadMemories/SearchMemories/Tools/EnqueueAutoMemoryJob/Close |
| PgVector 实现 | `memory/pgvector/service.go` | memory.Service | 向量搜索 + 关键词搜索 + 混合搜索（RRF）、memoryLimit 淘汰、AutoMemoryWorker |
| Mem0 实现 | `memory/mem0/service.go` | memory.Service | Ingest-first 模式、mem0 平台 API、IngestSession 异步摄入 |

### 1.2 目标

1. **PgVector 记忆后端**：集成框架 `memory/pgvector`，支持语义搜索和混合搜索
2. **Mem0 平台集成**：集成框架 `memory/mem0`，接入专业记忆管理平台
3. **配置驱动**：通过配置文件切换 Memory 后端（sqlite/pgvector/mem0）
4. **向后兼容**：现有 Memory 工具（add/update/delete/clear/search/load）行为不变

### 1.3 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| M-P0-1 | PgVector Memory 后端 | 实现 `memory.Service`，使用框架 `memory/pgvector` 包 |
| M-P0-2 | 语义搜索 | 通过 `SearchMemories` 支持向量相似度搜索 |
| M-P0-3 | 混合搜索 | 支持 `SearchOptions.HybridSearch=true`，RRF 融合向量+关键词结果 |
| M-P0-4 | 配置驱动后端选择 | 通过 `config.yaml` 的 `memory.backend` 字段选择 sqlite/pgvector/mem0 |
| M-P0-5 | Memory 工具兼容 | 6 个 Memory 工具（add/update/delete/clear/search/load）行为不变 |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|------|
| M-P1-1 | Mem0 平台集成 | 使用框架 `memory/mem0` 包，接入 mem0 云端记忆管理 |
| M-P1-2 | IngestSession | 通过 mem0 的 `IngestSession` 异步摄入会话记录 |
| M-P1-3 | AutoMemoryJob | PgVector 后端的 `EnqueueAutoMemoryJob` 自动记忆提取 |
| M-P1-4 | 记忆淘汰 | PgVector 后端的 `memoryLimit` 配置，超限自动淘汰旧记忆 |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|------|
| M-P2-1 | 搜索调参 | 运行时配置 `HybridRRFK`、`KindFallback`、`OrderByEventTime` 等参数 |
| M-P2-2 | 双写模式 | 同时写入 PgVector 和 Mem0，PgVector 负责实时搜索，Mem0 负责长期管理 |

### 1.4 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | SearchMemories P99 < 100ms（PgVector 向量索引） |
| 可靠性 | AutoMemoryJob 至少执行一次语义 |
| 兼容性 | `memory.Service` 8 方法全部实现，Memory 工具行为不变 |
| 可观测性 | 搜索延迟、记忆数量、淘汰事件通过 FlowLog 记录 |
| 数据安全 | 记忆删除操作不可逆，需确认机制 |

### 1.5 验收标准

1. `memory.backend=pgvector` 配置下，6 个 Memory 工具全部正常工作
2. `SearchMemories` 语义搜索返回相关结果，cosine similarity 阈值可配
3. `HybridSearch=true` 时，RRF 融合结果优于纯向量或纯关键词搜索
4. `memory.backend=mem0` 配置下，`IngestSession` 成功摄入会话记录
5. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 框架参考

#### 核心接口 — `pkg/trpc-agent-go/memory/memory.go`

```go
type Service interface {
    AddMemory(ctx context.Context, userID string, content string, opts ...MemoryOption) (*Memory, error)
    UpdateMemory(ctx context.Context, userID string, memoryID string, content string, opts ...MemoryOption) (*Memory, error)
    DeleteMemory(ctx context.Context, userID string, memoryID string) error
    ClearMemories(ctx context.Context, userID string) error
    ReadMemories(ctx context.Context, userID string, opts ...ReadOption) ([]*Memory, error)
    SearchMemories(ctx context.Context, userID string, query string, opts ...SearchOption) ([]*Memory, error)
    Tools() []tool.Tool
    EnqueueAutoMemoryJob(ctx context.Context, userID string, sessionKey session.Key) error
    Close() error
}
```

#### Memory 结构体

```go
type Memory struct {
    ID           string
    UserID       string
    Content      string
    Kind         Kind
    EventTime    time.Time
    Participants []string
    Location     string
    Embedding    []float64
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type Kind string

const (
    KindFact    Kind = "fact"
    KindEpisode Kind = "episode"
)
```

#### 搜索选项

```go
type SearchOptions struct {
    Kind            Kind
    TimeAfter       *time.Time
    TimeBefore      *time.Time
    HybridSearch    bool
    HybridRRFK      int
    Deduplicate     bool
    KindFallback    bool
    OrderByEventTime bool
    Limit           int
}
```

#### PgVector 实现 — `pkg/trpc-agent-go/memory/pgvector/service.go`

```go
type Service struct { ... }

func NewService(options ...ServiceOpt) (*Service, error)
```

关键特性：

- 需要 `embedder model.Embedder` 用于记忆向量化
- `executeVectorSearch`：cosine similarity 向量搜索
- `executeKeywordSearch`：PostgreSQL 全文搜索
- `mergeHybridResults`：RRF 融合
- `memoryLimit` 配置：超限自动淘汰最旧记忆
- `AutoMemoryWorker`：通过 `imemory.AutoMemoryWorker` 自动提取记忆

#### Mem0 实现 — `pkg/trpc-agent-go/memory/mem0/service.go`

```go
type Service struct { ... }

func NewService(options ...ServiceOpt) (*Service, error)
```

关键特性：

- Ingest-first 模式：先摄入会话记录，mem0 平台负责记忆管理
- `IngestSession(ctx, userID, sessionKey, transcript)` 异步摄入
- `ReadMemories` + `SearchMemories` 通过 mem0 API 查询
- 不需要本地 Embedder（mem0 平台内部处理嵌入）

### 2.2 当前项目现状

当前 Memory 后端在 `internal/memory/trpc/sqlite_adapter.go`：

```go
type sqliteMemoryService struct {
    store              sessionmemory.Store
    vectorFactSearcher vectorFactSearcher
    autoMemoryQueue    *AutoMemoryQueue
}
```

- `SearchMemories`：优先尝试 `vectorFactSearcher`（如果可用），否则回退到关键词搜索
- `Tools()`：返回 6 个工具（add, update, delete, clear, search, load）
- `EnqueueAutoMemoryJob`：通过 `AutoMemoryQueue` 排队
- `vectorFactSearcher` 接口：可选的向量搜索能力，当前未实现

`internal/memory/provider.go` 提供 Wire ProviderSet。

### 2.3 架构设计

#### 2.3.1 整体架构

```
config.yaml
  └─ memory.backend: sqlite | pgvector | mem0
       │
       ▼
internal/memory/trpc/factory.go  ← 工厂根据配置选择后端
  ├─ sqliteFactory      → sqliteMemoryService (现有)
  ├─ pgvectorFactory    → memory/pgvector (新增)
  └─ mem0Factory        → memory/mem0 (新增)
       │
       ▼
internal/memory/provider.go  ← Wire 注入点
  └─ wire.Bind(trpcmemory.Service, ...)
```

#### 2.3.2 配置结构

在 `internal/conf/conf.proto` 中扩展：

```protobuf
message Memory {
  string backend = 1;  // sqlite | pgvector | mem0
  PgVectorMemory pgvector = 2;
  Mem0Memory mem0 = 3;
}

message PgVectorMemory {
  string dsn = 1;
  string embedder_provider = 2;
  string embedder_model = 3;
  int32 embedding_dimension = 4;
  bool hybrid_search = 5;
  int32 hybrid_rrf_k = 6;
  int32 memory_limit = 7;
}

message Mem0Memory {
  string api_key = 1;
  string base_url = 2;
  string user_id = 3;
}
```

#### 2.3.3 工厂设计

新增 `internal/memory/trpc/factory.go`：

```go
type MemoryFactory interface {
    NewMemoryService(ctx context.Context) (trpcmemory.Service, error)
}

type pgvectorMemoryFactory struct {
    opts     []pgvectormemory.ServiceOpt
    embedder model.Embedder
}

func (f *pgvectorMemoryFactory) NewMemoryService(ctx context.Context) (trpcmemory.Service, error) {
    return pgvectormemory.NewService(f.opts...)
}

type mem0MemoryFactory struct {
    opts []mem0memory.ServiceOpt
}

func (f *mem0MemoryFactory) NewMemoryService(ctx context.Context) (trpcmemory.Service, error) {
    return mem0memory.NewService(f.opts...)
}
```

#### 2.3.4 工具兼容性

框架 `memory/pgvector` 和 `memory/mem0` 的 `Tools()` 方法已返回标准 6 个工具（add, update, delete, clear, search, load），与现有 `sqliteMemoryService.Tools()` 行为一致。无需额外适配。

#### 2.3.5 AutoMemoryJob 适配

- **PgVector 后端**：框架内置 `AutoMemoryWorker`，`EnqueueAutoMemoryJob` 直接使用
- **Mem0 后端**：`IngestSession` 替代 AutoMemoryJob，在会话结束时摄入完整记录
- **SQLite 后端**：保持现有 `AutoMemoryQueue` 逻辑

### 2.4 与框架的集成方式

| 集成点 | 框架包 | 项目适配层 | 说明 |
|--------|--------|-----------|------|
| Memory 核心 | `memory/pgvector` | `internal/memory/trpc/pgvector_adapter.go` | 直接使用 `pgvector.NewService`，传入 Embedder |
| Mem0 平台 | `memory/mem0` | `internal/memory/trpc/mem0_adapter.go` | 直接使用 `mem0.NewService`，传入 API Key |
| 记忆嵌入 | `model.Embedder` | `internal/provider` | 复用 Provider 层 Embedder（与 Session PgVector 共享） |
| AutoMemory | `imemory.AutoMemoryWorker` | 框架内部使用 | PgVector 后端内置，无需额外适配 |
| 存储客户端 | `storage/postgres` | 框架内部使用 | PgVector Service 内部创建 |

**关键原则**：Memory 的 `Tools()` 方法由框架实现直接提供，项目不做工具注册的二次封装。`EnqueueAutoMemoryJob` 的调度逻辑由框架管理。

### 2.5 错误处理

| 场景 | 错误类型 | 处理方式 |
|------|----------|----------|
| PgVector 连接失败 | `kerrors.InternalServer("MEMORY", ...)` | 启动时 Fail Fast |
| Embedder 初始化失败 | `kerrors.InternalServer("MEMORY", ...)` | 启动时 Fail Fast |
| Mem0 API Key 无效 | `kerrors.InternalServer("MEMORY", ...)` | 启动时 Fail Fast |
| SearchMemories 无结果 | 返回空切片 | 不视为错误 |
| AddMemory 淘汰旧记忆 | FlowLog 记录 | 静默淘汰，不报错 |
| Mem0 IngestSession 失败 | FlowLog 记录 | 异步重试，不影响主流程 |
| AutoMemoryJob 执行失败 | FlowLog 记录 | 重试队列，不阻塞 |

---

## 三、开发计划

### 3.1 任务拆解

| # | 任务 | 涉及文件 | 依赖 | 预估 |
|---|------|----------|------|------|
| T1 | 扩展 Proto 配置结构 | `internal/conf/conf.proto` | 无 | 0.5d |
| T2 | 新增 PgVector Memory 工厂 | `internal/memory/trpc/pgvector_adapter.go` | T1, T5 | 1.5d |
| T3 | 新增 Mem0 Memory 工厂 | `internal/memory/trpc/mem0_adapter.go` | T1 | 1d |
| T4 | 新增 MemoryFactory 接口与实现 | `internal/memory/trpc/factory.go` | T2, T3 | 0.5d |
| T5 | Provider 层 Embedder 共享 | `internal/provider/embedder.go` | 无（与 01-Session T6 共享） | 0.5d |
| T6 | 重构 Wire ProviderSet | `internal/memory/provider.go` | T4 | 0.5d |
| T7 | 配置解析与后端选择 | `internal/memory/trpc/factory.go` | T1 | 0.5d |
| T8 | AutoMemoryJob 适配 | `internal/memory/trpc/pgvector_adapter.go` | T2 | 0.5d |
| T9 | 集成测试 | `internal/memory/trpc/pgvector_test.go`, `mem0_test.go` | T2, T3 | 1d |
| T10 | `make api && make wire && make build` 验证 | 全局 | T1-T8 | 0.5d |

### 3.2 开发顺序

```
Phase 1 — 基础设施（T1 → T5）
  ├─ T1: Proto 配置扩展
  └─ T5: Provider Embedder 共享（与 01-Session 并行）

Phase 2 — PgVector 后端（T2 → T8）
  ├─ T2: PgVector Memory 适配器
  └─ T8: AutoMemoryJob 适配

Phase 3 — Mem0 后端（T3）
  └─ T3: Mem0 Memory 适配器

Phase 4 — 工厂与注入（T4 → T6 → T7）
  ├─ T4: MemoryFactory 接口
  ├─ T6: Wire ProviderSet 重构
  └─ T7: 配置解析与后端选择

Phase 5 — 验证（T9 → T10）
  ├─ T9: 集成测试
  └─ T10: 全量构建验证
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| PgVector 语义搜索 | `go test ./internal/memory/trpc/... -run TestPgVectorSearch -count=1` | SearchMemories 返回语义相关结果 |
| PgVector 混合搜索 | `go test ./internal/memory/trpc/... -run TestPgVectorHybrid -count=1` | RRF 融合结果优于单一搜索 |
| PgVector 记忆淘汰 | `go test ./internal/memory/trpc/... -run TestPgVectorEviction -count=1` | 超限后自动淘汰最旧记忆 |
| Mem0 摄入 | `go test ./internal/memory/trpc/... -run TestMem0Ingest -count=1` | IngestSession 成功 |
| Memory 工具兼容 | `go test ./internal/memory/trpc/... -run TestTools -count=1` | 6 个工具全部可用 |
| 配置切换 | 修改 `config.yaml` 的 `memory.backend` | 不同后端正确加载 |
| 全量构建 | `make api && make wire && make build && make test` | 零错误 |
