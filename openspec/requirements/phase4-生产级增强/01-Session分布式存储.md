# 01-Session 分布式存储

## 一、需求文档

### 1.1 背景

当前 Aranea-Agents 的 Session 存储使用 SQLite（通过 `internal/session/trpc/sqlite.go` 中的 `sqliteSessionFactory`），所有会话事件持久化到本地 SQLite 文件。这种方案存在以下问题：

- **单点瓶颈**：SQLite 是单机嵌入式数据库，无法水平扩展，高并发写入时 WAL 锁竞争严重
- **无法多实例部署**：多 Pod 无法共享同一 SQLite 文件，阻碍 K8s 水平伸缩
- **缺乏语义搜索**：SQLite 不支持向量索引，无法对会话事件进行语义检索
- **缺乏窗口查询**：无法基于锚点高效加载事件窗口（用于无限滚动场景）

框架 `pkg/trpc-agent-go/session/` 已提供多种后端实现：

| 后端 | 文件路径 | 实现接口 | 特性 |
|------|----------|----------|------|
| `session.Service` | `session/session.go` | 13 方法核心接口 | CreateSession/GetSession/ListSessions/DeleteSession/UpdateAppState/DeleteAppState/ListAppStates/UpdateUserState/ListUserStates/DeleteUserState/UpdateSessionState/AppendEvent/CreateSessionSummary/EnqueueSummaryJob/GetSessionSummaryText/Close |
| `session.SearchableService` | `session/session.go` | 语义搜索扩展 | SearchEvents(ctx, EventSearchRequest) ([]EventSearchResult, error) |
| `session.WindowService` | `session/session.go` | 窗口查询扩展 | GetEventWindow(ctx, EventWindowRequest) (*EventWindow, error) |
| `session.TrackService` | `session/session.go` | 轨迹事件扩展 | AppendTrackEvent(ctx, key Key, eventType string, payload []byte) error |
| Postgres 实现 | `session/postgres/service.go` | Service + TrackService | 异步持久化、TTL 清理、软/硬删除 |
| PgVector 实现 | `session/pgvector/service.go` | Service + TrackService + SearchableService + WindowService | 向量索引、Dense/Hybrid 搜索、窗口查询 |
| Redis 实现 | `session/redis/service.go` | Service + TrackService | HashIdx + ZSet 双存储、CompatMode 迁移 |
| MySQL 实现 | `session/mysql/service.go` | Service + TrackService | MySQL 方言适配 |

### 1.2 目标

1. **Postgres 迁移**：将 Session 存储从 SQLite 迁移到 PostgreSQL，支持多实例部署
2. **PgVector 增强**：在 Postgres 基础上集成 PgVector，支持会话事件语义搜索和窗口查询
3. **平滑迁移**：提供 SQLite→Postgres 数据迁移工具，零停机切换
4. **配置驱动**：通过配置文件切换存储后端，无需修改代码

### 1.3 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| S-P0-1 | Postgres Session 后端 | 实现 `session.Service` + `session.TrackService`，使用框架 `session/postgres` 包 |
| S-P0-2 | PgVector 语义搜索 | 实现 `session.SearchableService`，使用框架 `session/pgvector` 包 |
| S-P0-3 | PgVector 窗口查询 | 实现 `session.WindowService`，基于锚点加载事件窗口 |
| S-P0-4 | 配置驱动后端选择 | 通过 `config.yaml` 的 `session.backend` 字段选择 sqlite/postgres/pgvector |
| S-P0-5 | Wire 注入适配 | 根据配置动态绑定不同的 Session 实现到 Wire ProviderSet |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|------|
| S-P1-1 | SQLite→Postgres 数据迁移 | 提供离线迁移脚本，将 SQLite 中的 Session/Event 数据导入 Postgres |
| S-P1-2 | 连接池配置 | 支持配置 Postgres 连接池参数（MaxOpen/MaxIdle/MaxLifetime） |
| S-P1-3 | 嵌入模型配置 | PgVector 后端需要配置 Embedder（用于事件向量化和语义搜索） |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|------|
| S-P2-1 | Redis Session 缓存层 | 在 Postgres/PgVector 之上叠加 Redis 缓存，加速热会话读取 |
| S-P2-2 | 混合搜索调参 | 支持 `SearchMode`（Dense/Hybrid）和 RRF K 值的运行时配置 |

### 1.4 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | 单会话 AppendEvent P99 < 50ms（Postgres 异步持久化） |
| 可靠性 | 异步持久化 worker 保证 at-least-once 语义 |
| 兼容性 | `session.Service` 13 方法全部实现，不破坏现有调用方 |
| 可观测性 | 连接池指标、持久化队列深度通过 Prometheus 暴露 |
| 迁移安全 | 迁移脚本支持断点续传，不丢失事件 |

### 1.5 验收标准

1. `session.backend=pgvector` 配置下，所有现有 Session 相关功能正常
2. `SearchEvents` 返回语义相关结果，Dense 搜索 cosine similarity 阈值可配
3. `GetEventWindow` 支持锚点前后的双向加载
4. SQLite→Postgres 迁移脚本执行后数据完整，事件顺序不变
5. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 框架参考

#### 核心接口 — `pkg/trpc-agent-go/session/session.go`

```go
type Service interface {
    CreateSession(ctx context.Context, key Key) (*Session, error)
    GetSession(ctx context.Context, key Key) (*Session, error)
    ListSessions(ctx context.Context, userID string) ([]*Session, error)
    DeleteSession(ctx context.Context, key Key) error
    UpdateAppState(ctx context.Context, key Key, appState string) error
    DeleteAppState(ctx context.Context, key Key, appState string) error
    ListAppStates(ctx context.Context, key Key) ([]string, error)
    UpdateUserState(ctx context.Context, key Key, userState string) error
    ListUserStates(ctx context.Context, key Key) ([]string, error)
    DeleteUserState(ctx context.Context, key Key, userState string) error
    UpdateSessionState(ctx context.Context, key Key, state map[string]any) error
    AppendEvent(ctx context.Context, key Key, event Event) error
    CreateSessionSummary(ctx context.Context, key Key, summary string) error
    EnqueueSummaryJob(ctx context.Context, key Key) error
    GetSessionSummaryText(ctx context.Context, key Key) (string, error)
    Close() error
}

type SearchableService interface {
    SearchEvents(ctx context.Context, req EventSearchRequest) ([]EventSearchResult, error)
}

type WindowService interface {
    GetEventWindow(ctx context.Context, req EventWindowRequest) (*EventWindow, error)
}

type TrackService interface {
    AppendTrackEvent(ctx context.Context, key Key, eventType string, payload []byte) error
}
```

#### Postgres 实现 — `pkg/trpc-agent-go/session/postgres/service.go`

```go
type Service struct { ... }

func NewService(options ...ServiceOpt) (*Service, error)
```

构造选项：

```go
func WithDSN(dsn string) ServiceOpt
func WithHost(host string) ServiceOpt
func WithInstanceName(name string) ServiceOpt
```

DSN 优先级：`WithDSN` > `WithHost` > `WithInstanceName`。

#### PgVector 实现 — `pkg/trpc-agent-go/session/pgvector/service.go`

```go
type Service struct { ... }

func NewService(options ...ServiceOpt) (*Service, error)
```

关键扩展：

- 需要 `embedder model.Embedder` 用于事件向量化
- `asyncIndexEvent` 异步嵌入生成
- `indexEventAfterPersist` 支持同步/异步索引模式

#### 搜索实现 — `pkg/trpc-agent-go/session/pgvector/search.go`

```go
func (s *Service) SearchEvents(ctx context.Context, req session.EventSearchRequest) ([]session.EventSearchResult, error)
```

搜索模式：

```go
type SearchMode int

const (
    Dense SearchMode = iota
    Hybrid
)
```

- `executeDenseSearch`：cosine similarity `1 - (embedding <=> $1)`
- `executeKeywordSearch`：PostgreSQL 全文搜索 `ts_rank` + `plainto_tsquery`
- `mergeHybridEventResults`：Reciprocal Rank Fusion (RRF)

#### 窗口查询 — `pkg/trpc-agent-go/session/pgvector/window.go`

```go
func (s *Service) GetEventWindow(ctx context.Context, req session.EventWindowRequest) (*session.EventWindow, error)
```

- `loadWindowAnchor`：定位锚点事件
- `loadWindowNeighbors`：加载锚点前后事件

### 2.2 当前项目现状

当前 Session 后端在 `internal/session/trpc/` 目录下：

| 文件 | 职责 |
|------|------|
| `factory.go` | `SessionFactory` 接口 + `sqliteSessionFactory` 实现 |
| `sqlite.go` | SQLite 后端，创建 `sqliteSessionService` |
| `rollback.go` | 事件回滚逻辑 |

`internal/session/provider.go` 提供 Wire ProviderSet：

```go
var ProviderSet = wire.NewSet(
    NewRuntime,
    NewCompressor,
    wire.Bind(new(biz.NativeTurnCompressor), new(*Compressor)),
    wire.Bind(new(biz.RunnerSnapshotSync), new(*Runtime)),
)
```

当前 `sqliteSessionFactory` 创建的 Session 服务仅实现 `session.Service`，未实现 `SearchableService` 和 `WindowService`。

### 2.3 架构设计

#### 2.3.1 整体架构

```
config.yaml
  └─ session.backend: sqlite | postgres | pgvector
       │
       ▼
internal/session/trpc/factory.go  ← 工厂根据配置选择后端
  ├─ sqliteFactory     → session/sqlite (现有)
  ├─ postgresFactory   → session/postgres (新增)
  └─ pgvectorFactory   → session/pgvector (新增)
       │
       ▼
internal/session/provider.go  ← Wire 注入点
  └─ wire.Bind(session.Service, ...)
  └─ wire.Bind(session.SearchableService, ...)  ← 可选
  └─ wire.Bind(session.WindowService, ...)       ← 可选
```

#### 2.3.2 配置结构

在 `internal/conf/conf.proto` 中扩展：

```protobuf
message Session {
  string backend = 1;  // sqlite | postgres | pgvector
  Postgres postgres = 2;
  PgVector pgvector = 3;
}

message Postgres {
  string dsn = 1;
  string host = 2;
  int32 port = 3;
  string user = 4;
  string password = 5;
  string database = 6;
  int32 max_open_conns = 7;
  int32 max_idle_conns = 8;
  int64 conn_max_lifetime_ms = 9;
}

message PgVector {
  Postgres postgres = 1;
  string embedder_provider = 2;  // openai | azure | local
  string embedder_model = 3;
  int32 embedding_dimension = 4;
  string search_mode = 5;  // dense | hybrid
  int32 hybrid_rrf_k = 6;
}
```

#### 2.3.3 工厂重构

`internal/session/trpc/factory.go` 扩展 `SessionFactory`：

```go
type SessionFactory interface {
    NewSessionService(ctx context.Context) (session.Service, error)
    NewSearchableService(ctx context.Context) (session.SearchableService, error)
    NewWindowService(ctx context.Context) (session.WindowService, error)
    NewTrackService(ctx context.Context) (session.TrackService, error)
}
```

新增 `postgresSessionFactory` 和 `pgvectorSessionFactory`：

```go
type postgresSessionFactory struct {
    opts []postgressession.ServiceOpt
}

func (f *postgresSessionFactory) NewSessionService(ctx context.Context) (session.Service, error) {
    return postgressession.NewService(f.opts...)
}

type pgvectorSessionFactory struct {
    opts        []pgvectorsession.ServiceOpt
    embedder    model.Embedder
}

func (f *pgvectorSessionFactory) NewSessionService(ctx context.Context) (session.Service, error) {
    return pgvectorsession.NewService(f.opts...)
}

func (f *pgvectorSessionFactory) NewSearchableService(ctx context.Context) (session.SearchableService, error) {
    svc, _ := pgvectorsession.NewService(f.opts...)
    return svc, nil
}

func (f *pgvectorSessionFactory) NewWindowService(ctx context.Context) (session.WindowService, error) {
    svc, _ := pgvectorsession.NewService(f.opts...)
    return svc, nil
}
```

#### 2.3.4 Embedder 集成

PgVector 后端需要 `model.Embedder` 用于事件向量化。通过 `internal/provider` 获取：

```go
type EmbedderProvider interface {
    GetEmbedder(ctx context.Context, providerName, modelName string) (model.Embedder, error)
}
```

在 `internal/provider` 中新增 `GetEmbedder` 方法，复用现有的 LLM Provider 基础设施。

#### 2.3.5 Wire 注入适配

`internal/session/provider.go` 重构：

```go
var ProviderSet = wire.NewSet(
    NewRuntime,
    NewCompressor,
    NewSessionFactory,
    wire.Bind(new(biz.NativeTurnCompressor), new(*Compressor)),
    wire.Bind(new(biz.RunnerSnapshotSync), new(*Runtime)),
)
```

`NewSessionFactory` 根据配置返回不同的 `SessionFactory` 实现。对于 `SearchableService` 和 `WindowService`，采用可选注入模式——仅在 `pgvector` 后端下绑定。

### 2.4 与框架的集成方式

| 集成点 | 框架包 | 项目适配层 | 说明 |
|--------|--------|-----------|------|
| Session 核心 | `session/postgres` | `internal/session/trpc/postgres.go` | 直接使用 `postgres.NewService` |
| 语义搜索 | `session/pgvector` | `internal/session/trpc/pgvector.go` | 直接使用 `pgvector.NewService`，传入 Embedder |
| 窗口查询 | `session/pgvector` | 同上 | `GetEventWindow` 开箱即用 |
| 事件嵌入 | `model.Embedder` | `internal/provider` | 复用 Provider 层获取 Embedder 实例 |
| 存储客户端 | `storage/postgres` | 框架内部使用 | `pgvector.Service` 内部创建 `storage.Client` |

**关键原则**：不复制框架内部逻辑，只做配置转换和 Wire 装配。框架的异步持久化 worker、TTL 清理、嵌入索引等机制全部由框架管理。

### 2.5 错误处理

| 场景 | 错误类型 | 处理方式 |
|------|----------|----------|
| Postgres 连接失败 | `kerrors.InternalServer("SESSION", ...)` | 启动时 Fail Fast |
| Embedder 初始化失败 | `kerrors.InternalServer("SESSION", ...)` | 启动时 Fail Fast |
| AppendEvent 持久化失败 | 框架内部重试 | 异步 worker 保证 at-least-once |
| SearchEvents 无结果 | 返回空切片 | 不视为错误 |
| GetEventWindow 锚点不存在 | `kerrors.NotFound("SESSION", ...)` | 返回明确错误 |
| 迁移脚本数据冲突 | `kerrors.Conflict("SESSION", ...)` | 跳过或覆盖，可配置 |

---

## 三、开发计划

### 3.1 任务拆解

| # | 任务 | 涉及文件 | 依赖 | 预估 |
|---|------|----------|------|------|
| T1 | 扩展 Proto 配置结构 | `internal/conf/conf.proto` | 无 | 0.5d |
| T2 | 新增 Postgres Session 工厂 | `internal/session/trpc/postgres.go` | T1 | 1d |
| T3 | 新增 PgVector Session 工厂 | `internal/session/trpc/pgvector.go` | T1, T6 | 1.5d |
| T4 | 重构 SessionFactory 接口 | `internal/session/trpc/factory.go` | T2, T3 | 0.5d |
| T5 | 重构 Wire ProviderSet | `internal/session/provider.go` | T4 | 0.5d |
| T6 | Provider 层新增 GetEmbedder | `internal/provider/embedder.go` | 无 | 1d |
| T7 | 配置解析与后端选择逻辑 | `internal/session/trpc/factory.go` | T1 | 0.5d |
| T8 | SQLite→Postgres 迁移脚本 | `cmd/migrate-session/main.go` | T2 | 1d |
| T9 | 集成测试 | `internal/session/trpc/postgres_test.go`, `pgvector_test.go` | T2, T3 | 1d |
| T10 | `make api && make wire && make build` 验证 | 全局 | T1-T7 | 0.5d |

### 3.2 开发顺序

```
Phase 1 — 基础设施（T1 → T6）
  ├─ T1: Proto 配置扩展
  └─ T6: Provider Embedder 支持

Phase 2 — Postgres 后端（T2 → T4 → T5）
  ├─ T2: Postgres 工厂
  ├─ T4: SessionFactory 接口重构
  └─ T5: Wire 注入适配

Phase 3 — PgVector 增强（T3 → T7）
  ├─ T3: PgVector 工厂（依赖 T6 的 Embedder）
  └─ T7: 配置解析与后端选择

Phase 4 — 迁移与验证（T8 → T9 → T10）
  ├─ T8: 迁移脚本
  ├─ T9: 集成测试
  └─ T10: 全量构建验证
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| Postgres 后端功能 | `go test ./internal/session/trpc/... -run TestPostgres -count=1` | Session CRUD + AppendEvent 正常 |
| PgVector 语义搜索 | `go test ./internal/session/trpc/... -run TestPgVectorSearch -count=1` | SearchEvents 返回语义相关结果 |
| PgVector 窗口查询 | `go test ./internal/session/trpc/... -run TestPgVectorWindow -count=1` | GetEventWindow 正确返回锚点前后事件 |
| 配置切换 | 修改 `config.yaml` 的 `session.backend` | 不同后端正确加载，Wire 注入无冲突 |
| 迁移完整性 | 运行迁移脚本后对比源/目标数据 | 事件数量一致、顺序一致、内容一致 |
| 全量构建 | `make api && make wire && make build && make test` | 零错误 |
