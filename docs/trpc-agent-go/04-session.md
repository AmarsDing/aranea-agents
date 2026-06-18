# 会话（Session）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/session/`
> 项目实现路径：`internal/biz/session/`、`internal/data/session_*`、`internal/session/`、`internal/service/session*`
> 当前对齐度：★★★☆☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `session.Service` | `CreateSession(ctx, key, stateMap, ...Option) (*Session, error)` | 创建新 Session |
| | `GetSession(ctx, key, ...Option) (*Session, error)` | 按 Key 获取 Session |
| | `ListSessions(ctx, userKey, ...Option) ([]*Session, error)` | 列出某用户下所有 Session |
| | `DeleteSession(ctx, key, ...Option) error` | 删除 Session |
| | `UpdateAppState(ctx, appName, stateMap) error` | 更新应用级状态 |
| | `DeleteAppState(ctx, appName, key) error` | 删除应用级状态键 |
| | `ListAppStates(ctx, appName) (StateMap, error)` | 列出应用级状态 |
| | `UpdateUserState(ctx, userKey, stateMap) error` | 更新用户级状态 |
| | `ListUserStates(ctx, userKey) (StateMap, error)` | 列出用户级状态 |
| | `DeleteUserState(ctx, userKey, key) error` | 删除用户级状态键 |
| | `UpdateSessionState(ctx, key, stateMap) error` | 更新 Session 级状态 |
| | `AppendEvent(ctx, session, event, ...Option) error` | 追加事件到 Session |
| | `CreateSessionSummary(ctx, sess, filterKey, force) error` | 同步创建摘要 |
| | `EnqueueSummaryJob(ctx, sess, filterKey, force) error` | 异步入队摘要任务 |
| | `GetSessionSummaryText(ctx, sess, ...SummaryOption) (string, bool)` | 获取摘要文本 |
| | `Close() error` | 关闭服务 |
| `session.SearchableService` | `SearchEvents(ctx, req) ([]EventSearchResult, error)` | 向量语义搜索事件（仅 pgvector 后端） |
| `session.WindowService` | `GetEventWindow(ctx, req) (*EventWindow, error)` | 锚点事件窗口加载 |
| `session.TrackService` | `AppendTrackEvent(ctx, sess, event, ...Option) error` | 追加 Track 事件 |
| `session.Ingestor` | `IngestSession(ctx, sess, ...IngestOption) error` | 长时记忆摄入 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `session.Session` | 核心实体：ID/AppName/UserID/State/Events/Tracks/Summaries/UpdatedAt/CreatedAt |
| `session.Key` | Session 唯一标识：AppName + UserID + SessionID |
| `session.UserKey` | 用户级标识：AppName + UserID |
| `session.StateMap` | `map[string][]byte`，Session/User/App 级 KV 存储 |
| `session.State` | Value + Delta 双缓冲状态 |
| `session.Summary` | 摘要文本 + Topics + UpdatedAt + Boundary |
| `session.SummaryBoundary` | 摘要边界：Version/FilterKey/CutoffAt/LastEventID |
| `session.Track` / `TrackEvent` / `TrackEvents` | Track 事件通道（独立于 Event 的结构化日志） |
| `session.Options` | 查询选项：EventNum/EventTime/EventPage/ListSessionPage |
| `session.EventSearchRequest` / `EventSearchResult` | 语义搜索请求/响应 |
| `session.EventWindowRequest` / `EventWindow` | 事件窗口请求/响应 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| `AppendEventHook` | 责任链，`func(ctx, next) error`，写入时拦截 | 内容过滤、审计日志、Tag 标记 |
| `GetSessionHook` | 责任链，`func(ctx, next) (*Session, error)`，读取时拦截 | 内容脱敏、连续消息合并、过滤 |
| `session.Ingestor` | 接口实现，Runner 每轮完成时调用 | 长时记忆摄入（mem0 等） |
| `summary.SessionSummarizer` | 接口实现 + Checker 工厂 | 自定义摘要触发条件和生成逻辑 |
| `summary.PreSummaryHook` / `PostSummaryHook` | 摘要前后置钩子 | 摘要内容预处理/后处理 |
| `SearchableService` | 接口实现 + 类型断言 | 向量语义搜索（需 pgvector 后端） |
| `WindowService` | 接口实现 + 类型断言 | 锚点事件窗口加载 |

### 1.4 配置选项

#### 通用 Session Option（查询时）

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithEventNum(num)` | 限制返回的最近事件数 | 全部 |
| `WithEventTime(time)` | 只返回指定时间之后的事件 | 无限制 |
| `WithListSessionOnlyMeta()` | ListSessions 仅返回元数据 | false |
| `WithListSessionPage(offset, limit)` | ListSessions 分页 | 无分页 |
| `WithGetSessionEventPage(offset, limit)` | GetSession 事件分页 | 无分页 |

#### SQLite 后端 Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithTablePrefix(prefix)` | 表名前缀 | 无 |
| `WithEnableAsyncPersist(bool)` | 异步持久化 | false |
| `WithAsyncPersisterNum(int)` | 异步持久化 worker 数 | 4 |
| `WithSoftDelete(bool)` | 软删除 | false |
| `WithSkipDBInit(bool)` | 跳过 DDL 初始化 | false |
| `WithSessionEventLimit(int)` | 事件上限 | 1000 |
| `WithSessionTTL(duration)` | Session TTL | 无 |
| `WithAppendEventHook(...)` | 追加事件钩子 | 无 |
| `WithGetSessionHook(...)` | 获取 Session 钩子 | 无 |

#### InMemory 后端 Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithSessionEventLimit(int)` | 事件上限 | 1000 |
| `WithSessionTTL(duration)` | Session TTL | 无 |
| `WithAppStateTTL(duration)` | App 状态 TTL | 无 |
| `WithUserStateTTL(duration)` | User 状态 TTL | 无 |
| `WithCleanupInterval(duration)` | 清理间隔 | 1 分钟 |
| `WithSummarizer(summarizer)` | LLM 摘要器 | 无 |
| `WithAsyncSummaryNum(int)` | 异步摘要 worker 数 | 4 |
| `WithSummaryQueueSize(int)` | 摘要队列大小 | 1024 |
| `WithSummaryJobTimeout(duration)` | 摘要任务超时 | 5 分钟 |
| `WithSummaryFilterAllowlist(...)` | 分支摘要过滤白名单 | 无 |
| `WithCascadeFullSessionSummary(bool)` | 分支摘要级联刷新全量 | false |
| `WithAppendEventHook(...)` | 追加事件钩子 | 无 |
| `WithGetSessionHook(...)` | 获取 Session 钩子 | 无 |

#### Summary Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithSummaryFilterKey(key)` | 指定摘要过滤键 | 无 |
| `WithPrompt(prompt)` | 自定义摘要提示词（需含 `{conversation_text}`） | 框架默认 |
| `WithSystemPrompt(prompt)` | 独立系统提示词 | 无 |
| `WithMaxSummaryWords(int)` | 摘要字数限制 | 无 |
| `WithTokenThreshold(int)` | Token 阈值触发 | 无 |
| `WithEventThreshold(int)` | 事件数阈值触发 | 无 |
| `WithTimeThreshold(duration)` | 时间阈值触发 | 无 |
| `WithContextThreshold(opts...)` | 动态感知模型 context window | 无 |
| `WithChecksAll/Any(checks)` | AND/OR 组合触发条件 | 无 |
| `WithPreSummaryHook(hook)` | 摘要前置钩子 | 无 |
| `WithPostSummaryHook(hook)` | 摘要后置钩子 | 无 |

### 1.5 框架内置实现

| 实现 | 路径 | Service | TrackService | WindowService | SearchableService |
|------|------|---------|-------------|---------------|-------------------|
| inmemory | `session/inmemory/` | Yes | Yes | Yes | - |
| sqlite | `session/sqlite/` | Yes | Yes | Yes | - |
| redis | `session/redis/` | Yes | Yes | Yes | - |
| postgres | `session/postgres/` | Yes | Yes | Yes | - |
| mysql | `session/mysql/` | Yes | Yes | Yes | - |
| clickhouse | `session/clickhouse/` | Yes | Yes | - | - |
| pgvector | `session/pgvector/` | Yes | Yes | Yes | Yes |
| noop | `session/noop/` | Yes | Yes | - | - |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `session.Service`（核心 16 方法） | ✅ 完全使用，Postgres 后端 + InMemory 降级 | ✅ | 工厂 `NewTRPCSessionService` 优先 Postgres，失败降级 InMemory（A6 后移除 SQLite 生产路径） |
| `session.Service` Option | ⚠️ 仅使用 3 个 Option | ⚠️ | 仅 `WithTablePrefix("trpc_")` + `WithEnableAsyncPersist(false)` + `WithSoftDelete(true)`，未使用 EventLimit/TTL/Hook/Summarizer 等 |
| `AppendEventHook` | ❌ 未使用 | ❌ | 创建 Session Service 时未注入任何 Hook |
| `GetSessionHook` | ❌ 未使用 | ❌ | 创建 Session Service 时未注入任何 Hook |
| `session.Ingestor` | ⚠️ 空壳实现 | ⚠️ | `BizSessionIngestor` 实现接口但 `IngestSession` 仅记日志，无实际摄入 |
| `SearchableService` | ❌ 未使用 | ❌ | SQLite 后端不实现此接口，项目也未使用 pgvector 后端 |
| `WindowService` | ❌ 未使用 | ❌ | SQLite 后端已实现但项目业务代码未直接引用 |
| `TrackService` | ❌ 未使用 | ❌ | SQLite 后端已实现但项目业务代码未直接引用 |
| `CreateSessionSummary` | ❌ 未使用 | ❌ | 项目自建压缩管道替代 |
| `EnqueueSummaryJob` | ⚠️ 定义但未调用 | ⚠️ | `Runtime.EnqueueFrameworkSummary` 已定义但无调用者 |
| `GetSessionSummaryText` | ❌ 未使用 | ❌ | 项目自建摘要管理 |
| `session.Service` 的 `GetSession`/`UpdateSessionState` | ✅ 通过 Runtime 封装使用 | ✅ | `session.Runtime` 封装了 Get/SyncRunnerSnapshot/SyncStateDelta 等 |
| `session.Service` 的 `AppendEvent` | ✅ Runner 内部自动调用 | ✅ | 框架 Runner 在每轮对话中自动 AppendEvent |
| `session.Service` 的 `CreateSession` | ✅ Runner 内部自动调用 | ✅ | 框架 Runner 在首次 Run 时自动创建 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| **完整压缩管道**（4 级：none/micro_compact/memory_compact/llm_compact） | `internal/session/compressor.go`、`internal/session/memory_compact.go` | 框架仅提供 `Summary`（LLM 单级摘要） | 框架 Summary 是简单的 LLM 摘要，项目需要多级压缩策略 + 快照重写 + 记忆提取 |
| **Session 状态机**（5 状态 6 事件） | `internal/biz/session/session_state_machine.go` | 框架无状态机 | 框架 Session 无生命周期状态管理，项目需要 idle/running/completed/interrupted/awaiting_confirmation |
| **SessionRun 生命周期**（Phase 状态机 + Durable Checkpoint） | `internal/biz/session_run.go`、`session_run_phase_machine.go`、`session_run_checkpoint.go` | 框架无对应功能 | 框架 Runner 无 Run 实体概念，项目需要 Run 级别的持久化、恢复、升级 |
| **SessionRun Durable Worker**（5s 轮询恢复） | `internal/service/session_run_durable_worker.go` | 框架无对应功能 | Durable Run 恢复是项目特有需求 |
| **SessionLockManager**（进程内 per-session 互斥锁） | `internal/biz/session_lock.go` | 框架无锁机制 | 框架 Session 无并发控制，项目需要防止同一 Session 并发 Run |
| **SessionMetricsUsecase**（增量聚合 + 200ms 定时刷盘） | `internal/biz/session/metrics.go`、`metrics_flush.go` | 框架无指标聚合 | 框架 Session 不管理业务指标（token 数/消息数等） |
| **SessionStatusGuard**（启动恢复 + 关闭中断） | `internal/service/session_status_guard.go` | 框架无对应功能 | 框架无进程级 Session 状态守卫 |
| **SessionProjection**（运行时状态投影） | `internal/biz/session_projection.go` | 框架无对应功能 | 项目需要为 Channel/Monitor/Team 提供 Session 运行时状态视图 |
| **ChannelPeerSession**（渠道对等端绑定） | `internal/biz/channel_peer_session.go` | 框架无对应功能 | 外部渠道集成是项目特有需求 |
| **Session 标题生成**（snippet + 异步 LLM） | `internal/biz/session/title.go`、`internal/service/session_title_llm.go` | 框架无对应功能 | 框架 Session 无标题概念 |
| **Session 导出**（Markdown/JSON） | `internal/biz/session/export.go` | 框架无对应功能 | 框架 Session 无导出功能 |
| **Session Pin/Archive/Batch 操作** | `internal/biz/session/pin.go`、`batch.go` | 框架无对应功能 | 框架 Session 仅有 Create/Get/List/Delete |
| **消息 CRUD + 修订版管理** | `internal/biz/session/message_usecase.go`、`internal/data/session_message_repo.go` | 框架 Event 是扁平列表，无消息级 CRUD | 框架 Event 与业务消息模型差异大，项目需要消息级操作 |
| **Turn 管理** | `internal/biz/session/turn_usecase.go`、`internal/data/session_turn_repo.go` | 框架无 Turn 概念 | 框架 Session 以 Event 为单位，项目需要 Turn 粒度管理 |
| **时间线组装** | `internal/biz/session/timeline_usecase.go`、`internal/data/session_timeline.go` | 框架无对应功能 | 框架 Event 是扁平列表，项目需要消息+工具+技能 UNION 查询 |
| **参与者管理** | `internal/biz/session/participant_usecase.go` | 框架无对应功能 | 框架 Session 无参与者概念 |
| **State Key 约定**（`aranea:runner_snapshot`/`aranea:compressed_summary`/`aranea:state:*`） | `internal/session/runtime.go` | 框架 `StateMap` 是通用 KV | 框架不定义具体 key，项目需要业务级 key 约定 |
| **FTS5 消息搜索** | `internal/data/message_search.go` | 框架 `SearchableService`（需 pgvector） | 项目使用 SQLite + FTS5，框架 SearchableService 需要 pgvector 后端 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `AppendEventHook` | 项目未识别到使用场景；框架示例展示了内容过滤和审计场景 | 评估中 — 可用于事件审计/内容过滤/Tag 标记 |
| `GetSessionHook` | 项目未识别到使用场景；框架示例展示了内容脱敏和连续消息合并 | 评估中 — 可用于敏感信息脱敏 |
| `SearchableService` | 需要 pgvector 后端，项目使用 SQLite | 否 — 项目使用 FTS5 满足搜索需求，且 SQLite 后端不实现此接口 |
| `WindowService` | 项目未使用锚点事件窗口加载 | 评估中 — 可能用于上下文窗口优化 |
| `TrackService` | 项目未使用 Track 事件通道 | 否 — 项目通过 EventBus + EventStore 管理结构化日志 |
| `CreateSessionSummary` / `EnqueueSummaryJob` | 项目自建了更完整的压缩管道 | 评估中 — 可作为 micro_compact 的补充或替代 |
| `GetSessionSummaryText` | 项目自建摘要管理 | 评估中 — 与 EnqueueSummaryJob 联动 |
| `WithSessionEventLimit` | 项目通过自建压缩管道控制上下文长度 | 评估中 — 可作为兜底保护 |
| `WithSessionTTL` | 项目通过 SessionStatusGuard 管理生命周期 | 否 — 项目 Session 生命周期由业务状态机驱动 |
| `WithSummarizer` | 项目自建压缩管道 | 评估中 — 可作为框架级摘要的触发入口 |
| `session.Service.UpdateAppState/DeleteAppState/ListAppStates` | 项目未使用应用级状态 | 否 — 项目通过 Ent Schema 管理应用配置 |
| `session.Service.UpdateUserState/ListUserStates/DeleteUserState` | 项目未使用用户级状态 | 评估中 — 可用于用户偏好存储 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **AppendEventHook 责任链**：框架提供标准的事件写入拦截机制，支持内容过滤、审计、Tag 标记 | 项目无事件写入拦截，所有事件直接 AppendEvent | 可统一事件审计逻辑，减少散落的日志代码 |
| 2 | **GetSessionHook 责任链**：框架提供标准的 Session 读取拦截机制，支持内容脱敏、连续消息合并 | 项目无读取拦截，所有读取直接 GetSession | 可统一敏感信息脱敏逻辑 |
| 3 | **Summary 子系统**：框架提供完整的异步摘要 Worker + Checker 工厂 + FilterKey + 分支摘要 + 级联刷新 | 项目自建压缩管道，框架 Summary 未使用 | 可减少自建摘要代码，利用框架异步 Worker 和触发条件 |
| 4 | **WindowService**：框架提供锚点事件窗口加载，支持高效的事件分页和定位 | 项目通过 `WithEventNum` 限制事件数，无窗口加载 | 可优化长 Session 的事件加载性能 |
| 5 | **TrackService**：框架提供独立于 Event 的结构化日志通道 | 项目通过 EventBus + EventStore 管理结构化日志 | 可简化结构化日志路径 |
| 6 | **UserState/AppState**：框架提供 Session 外的 KV 存储 | 项目通过 Ent Schema 管理应用配置和用户偏好 | 可减少部分 Ent Schema 和 Repo 代码 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **多级压缩管道**（none/micro_compact/memory_compact/llm_compact + 快照重写 + 记忆提取） | 框架仅提供 LLM 单级 Summary | 贡献回框架 — 压缩策略可抽象为 `Compressor` 接口 |
| 2 | **Session 状态机**（5 状态 6 事件 + 受保护状态） | 框架 Session 无生命周期状态 | 贡献回框架 — 状态机是 Session 生命周期管理的基础能力 |
| 3 | **SessionRun 实体**（Phase 状态机 + Durable Checkpoint + 恢复抢占） | 框架 Runner 无 Run 实体概念 | 保持自建 — 与项目 Durable Run 业务强相关 |
| 4 | **SessionLockManager**（进程内 per-session 互斥锁 + 空闲 GC） | 框架无并发控制 | 贡献回框架 — 并发 Run 防护是通用需求 |
| 5 | **SessionMetrics 增量聚合**（内存 map + 定时刷盘 + 强制刷盘） | 框架无指标聚合 | 保持自建 — 指标模型与项目业务强相关 |
| 6 | **消息级 CRUD + 修订版管理** | 框架 Event 是扁平列表 | 保持自建 — 消息模型与框架 Event 模型差异大 |
| 7 | **Turn 粒度管理** | 框架无 Turn 概念 | 保持自建 — Turn 是项目特有的对话轮次抽象 |
| 8 | **FTS5 消息搜索** | 框架 SearchableService 需要 pgvector | 保持自建 — SQLite + FTS5 是项目的合理选择 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| **Hook 未使用** | 认知缺失 — 项目未识别到 Hook 的使用场景，创建 Session Service 时未注入 | 事件审计和内容过滤逻辑散落在各处 |
| **Summary 未使用** | 功能缺失 — 框架 Summary 是简单的 LLM 摘要，项目需要多级压缩策略 + 快照重写 | 自建了完整压缩管道（~800 行），但框架 Summary 能力被浪费 |
| **SearchableService 未使用** | 架构决策 — 项目使用 SQLite 而非 pgvector，SQLite 不实现此接口 | 消息搜索走 FTS5 自建方案 |
| **WindowService 未使用** | 认知缺失 — 项目未识别到窗口加载的使用场景 | 长 Session 事件加载性能可能不佳 |
| **TrackService 未使用** | 认知缺失 — 项目未识别到 Track 通道的使用场景 | 结构化日志走 EventBus 自建方案 |
| **Session 状态机自建** | 功能缺失 — 框架 Session 无生命周期状态管理 | 自建了完整状态机 + 状态守卫 |
| **消息/Turn/时间线自建** | 模型差异 — 框架 Event 是扁平列表，项目需要消息级 CRUD + Turn 粒度 + 时间线组装 | 自建了完整的消息/turn/时间线体系 |
| **SessionRun 自建** | 功能缺失 — 框架 Runner 无 Run 实体概念 | 自建了 Run 生命周期 + Phase 状态机 + Durable Checkpoint |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用 AppendEventHook | 启用框架功能 | P2 | `internal/session/trpc/sqlite.go`、事件审计相关代码 | 统一事件审计入口，减少散落日志代码约 50 行 |
| 2 | 启用 GetSessionHook | 启用框架功能 | P2 | `internal/session/trpc/sqlite.go`、敏感信息处理相关代码 | 统一脱敏逻辑，减少散落处理代码约 30 行 |
| 3 | 评估 Summary 子系统替代 micro_compact | 替换自建实现 | P2 | `internal/session/compressor.go`、`internal/biz/session/compression.go` | 可减少 micro_compact 自建代码约 200 行，利用框架异步 Worker |
| 4 | 评估 WindowService 优化事件加载 | 启用框架功能 | P3 | `internal/session/runtime.go` | 长 Session 事件加载性能优化 |
| 5 | 启用 WithSessionEventLimit 兜底保护 | 启用框架功能 | P2 | `internal/session/trpc/sqlite.go` | 防止极端场景下 Session 事件无限增长 |
| 6 | 贡献 Session 状态机回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/session/` | 框架获得 Session 生命周期管理能力 |
| 7 | 贡献 SessionLockManager 回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/session/` | 框架获得并发 Run 防护能力 |
| 8 | 贡献多级压缩管道回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/session/` | 框架获得多级压缩策略能力 |

### 4.2 对齐项详情

#### 对齐项 #1：启用 AppendEventHook

**类型**：启用框架功能

**现状**：
- 项目当前实现：创建 Session Service 时未注入任何 AppendEventHook，事件直接 AppendEvent
- 框架提供能力：责任链模式 Hook，支持内容过滤、审计日志、Tag 标记，通过 `WithAppendEventHook` 注入

**对齐方案**：
1. 在 `NewSQLiteSessionService` 中添加 `WithAppendEventHook` 配置
2. 实现审计 Hook：在 AppendEvent 时记录事件类型和关键信息到 loggateway
3. 实现内容过滤 Hook（可选）：标记违规内容 Tag，配合 GetSessionHook 过滤

**代码变更范围**：
- 修改：`internal/session/trpc/sqlite.go`（添加 Hook Option）
- 新增：`internal/session/trpc/hooks.go`（Hook 实现）
- 修改：`internal/session/trpc/factory.go`（传递 Hook 配置）

**兼容性风险**：
- 低 — Hook 是追加逻辑，不影响现有事件流

**回退方案**：
- 移除 `WithAppendEventHook` Option 即可回退

**验证方法**：
- 单元测试：验证 Hook 在 AppendEvent 时被调用
- 集成测试：验证事件审计日志正确输出

**预期收益**：
- 代码减少：约 50 行散落的事件日志代码可统一到 Hook
- 维护成本：事件审计逻辑集中维护，减少散落代码

---

#### 对齐项 #2：启用 GetSessionHook

**类型**：启用框架功能

**现状**：
- 项目当前实现：创建 Session Service 时未注入任何 GetSessionHook，所有读取直接 GetSession
- 框架提供能力：责任链模式 Hook，支持内容脱敏、连续消息合并、过滤，通过 `WithGetSessionHook` 注入

**对齐方案**：
1. 在 `NewSQLiteSessionService` 中添加 `WithGetSessionHook` 配置
2. 实现脱敏 Hook：在 GetSession 时对敏感字段进行脱敏处理
3. 评估是否需要连续消息合并 Hook（框架示例展示了 merge/placeholder/skip 三种策略）

**代码变更范围**：
- 修改：`internal/session/trpc/sqlite.go`（添加 Hook Option）
- 新增：`internal/session/trpc/hooks.go`（Hook 实现，与 #1 合并）

**兼容性风险**：
- 中 — GetSessionHook 修改返回的 Session 内容，可能影响下游消费者

**回退方案**：
- 移除 `WithGetSessionHook` Option 即可回退

**验证方法**：
- 单元测试：验证 Hook 在 GetSession 时被调用
- 集成测试：验证脱敏后的 Session 内容正确

**预期收益**：
- 代码减少：约 30 行散落的脱敏处理代码可统一到 Hook
- 维护成本：脱敏逻辑集中维护

---

#### 对齐项 #3：评估 Summary 子系统替代 micro_compact

**类型**：替换自建实现

**现状**：
- 项目当前实现：自建 4 级压缩管道（none/micro_compact/memory_compact/llm_compact），micro_compact 是轻量级 LLM 摘要
- 框架提供能力：完整的 Summary 子系统，包括异步 Worker、Checker 工厂（Token/Event/Time/Context 阈值）、FilterKey、分支摘要、级联刷新、前后置 Hook

**对齐方案**：
1. 评估框架 Summary 是否可替代 micro_compact（轻量 LLM 摘要）
2. 如果可以：在 `NewSQLiteSessionService` 中配置 `WithSummarizer`，使用框架 Checker 触发条件
3. 如果不可以：保持 micro_compact 自建，但利用框架 `EnqueueSummaryJob` 作为补充触发入口
4. 无论如何：启用 `Runtime.EnqueueFrameworkSummary`（当前已定义但无调用者），在压缩后触发框架摘要

**代码变更范围**：
- 修改：`internal/session/trpc/sqlite.go`（添加 Summarizer Option）
- 修改：`internal/session/compressor.go`（在压缩后调用 EnqueueFrameworkSummary）
- 可能删除：micro_compact 相关代码（如果完全替代）

**兼容性风险**：
- 中 — 框架 Summary 的输出格式和触发条件可能与项目 micro_compact 不一致
- 需要验证框架 Summary 的 Prompt 是否满足项目需求

**回退方案**：
- 移除 Summarizer Option，恢复 micro_compact 自建逻辑

**验证方法**：
- 对比测试：框架 Summary vs micro_compact 的输出质量和触发时机
- 性能测试：框架异步 Worker vs 自建同步压缩的性能差异

**预期收益**：
- 代码减少：如果替代 micro_compact，约 200 行自建代码可删除
- 维护成本：利用框架异步 Worker 和 Checker，减少自建触发逻辑维护
- 功能增强：自动获得框架 Summary 的 FilterKey、分支摘要、级联刷新等能力

---

#### 对齐项 #4：评估 WindowService 优化事件加载

**类型**：启用框架功能

**现状**：
- 项目当前实现：通过 `WithEventNum` 限制返回事件数，无锚点事件窗口加载
- 框架提供能力：`WindowService.GetEventWindow` 支持基于锚点事件的高效窗口加载

**对齐方案**：
1. 评估长 Session 场景下 WindowService 的性能收益
2. 如果有收益：在 `session.Runtime` 中添加 WindowService 调用
3. 需要类型断言：`svc.(session.WindowService)` 检查后端是否支持

**代码变更范围**：
- 修改：`internal/session/runtime.go`（添加 WindowService 调用方法）
- 修改：Runner 上下文加载逻辑（使用窗口加载替代全量加载）

**兼容性风险**：
- 中 — 窗口加载改变事件加载逻辑，可能影响上下文完整性

**回退方案**：
- 移除 WindowService 调用，恢复 WithEventNum 限制

**验证方法**：
- 性能测试：长 Session（1000+ 事件）的加载时间对比
- 功能测试：窗口加载后上下文完整性验证

**预期收益**：
- 性能影响：长 Session 事件加载延迟可能降低 30-50%
- 内存影响：减少不必要的事件反序列化

---

#### 对齐项 #5：启用 WithSessionEventLimit 兜底保护

**类型**：启用框架功能

**现状**：
- 项目当前实现：未设置 EventLimit，Session 事件数无上限（依赖压缩管道控制）
- 框架提供能力：`WithSessionEventLimit` 设置事件上限，超出时自动淘汰旧事件

**对齐方案**：
1. 在 `NewSQLiteSessionService` 中添加 `WithSessionEventLimit(5000)` 作为兜底保护
2. 设置较大值（5000），仅在压缩管道失效时触发

**代码变更范围**：
- 修改：`internal/session/trpc/sqlite.go`（添加 EventLimit Option）

**兼容性风险**：
- 低 — EventLimit 是兜底保护，正常场景不会触发

**回退方案**：
- 移除 `WithSessionEventLimit` Option 即可回退

**验证方法**：
- 单元测试：验证事件数超过限制时的淘汰行为

**预期收益**：
- 维护成本：防止极端场景下 Session 事件无限增长导致的内存/存储问题

---

#### 对齐项 #6：贡献 Session 状态机回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：自建 5 状态 6 事件 Session 状态机，基于 `shared.GenericStateMachine` 泛型实现
- 框架现状：Session 无生命周期状态管理，仅有 Create/Get/List/Delete 操作

**对齐方案**：
1. 提取项目 Session 状态机为独立包 `session/statemachine/`
2. 定义 `Status` 枚举 + `Transition` 函数 + 守卫条件
3. 在框架 `session.Service` 接口中添加可选的状态管理方法
4. 提交 PR 到框架仓库

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/session/statemachine/`（状态机包）
- 修改：`pkg/trpc-agent-go/session/session.go`（添加状态相关字段和方法）

**兼容性风险**：
- 低 — 新增功能，不影响现有接口

**回退方案**：
- 框架不接受 PR 则保持自建

**验证方法**：
- 框架测试：状态机单元测试 + 转换校验测试

**预期收益**：
- 维护成本：框架统一维护状态机逻辑，项目减少自建代码
- 功能增强：框架其他使用者获得 Session 生命周期管理能力

---

#### 对齐项 #7：贡献 SessionLockManager 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：进程内 per-session 互斥锁，懒分配 + 30 分钟空闲 GC + 5 分钟扫描周期
- 框架现状：无并发控制，同一 Session 可能被多个 Runner 并发操作

**对齐方案**：
1. 提取项目 SessionLockManager 为独立包 `session/lock/`
2. 定义 `LockManager` 接口 + 进程内实现
3. 在框架 Runner 中集成锁检查
4. 提交 PR 到框架仓库

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/session/lock/`（锁管理包）
- 修改：`pkg/trpc-agent-go/runner/runner.go`（集成锁检查）

**兼容性风险**：
- 中 — Runner 集成锁检查可能影响现有行为

**回退方案**：
- 框架不接受 PR 则保持自建

**验证方法**：
- 框架测试：并发 Run 场景下的锁竞争测试

**预期收益**：
- 代码减少：项目可删除自建 SessionLockManager（约 100 行）
- 功能增强：框架获得并发 Run 防护能力

---

#### 对齐项 #8：贡献多级压缩管道回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：4 级压缩管道（none/micro_compact/memory_compact/llm_compact）+ 快照重写 + 记忆提取
- 框架现状：仅提供 LLM 单级 Summary

**对齐方案**：
1. 提取项目压缩策略为 `session/compressor/` 包
2. 定义 `Compressor` 接口 + `Strategy` 枚举 + 多级实现
3. 与框架 Summary 子系统整合，Compressor 作为 Summary 的增强版
4. 提交 PR 到框架仓库

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/session/compressor/`（压缩管道包）
- 修改：`pkg/trpc-agent-go/session/summary/`（整合压缩策略）

**兼容性风险**：
- 高 — 压缩管道涉及 Session 事件重写，逻辑复杂

**回退方案**：
- 框架不接受 PR 则保持自建

**验证方法**：
- 框架测试：各级压缩策略的输出质量测试
- 性能测试：压缩耗时和上下文完整性验证

**预期收益**：
- 代码减少：项目可删除自建压缩管道（约 800 行）
- 功能增强：框架获得多级压缩策略能力

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #5（EventLimit 兜底）、#1（AppendEventHook）、#2（GetSessionHook） | Event 对齐完成 | 中 |
| Phase 2 | #3（Summary 替代 micro_compact 评估） | Phase 1 | 中 |
| Phase 3 | #4（WindowService 评估） | Phase 2 | 小 |
| Phase 4 | #6（贡献状态机）、#7（贡献锁管理）、#8（贡献压缩管道） | Phase 2 验证通过 | 大 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架 Summary 输出质量不满足项目需求 | 中 | 中 | 先评估再决定，不盲目替换 |
| Hook 注入影响现有事件流性能 | 低 | 低 | Hook 逻辑保持轻量，耗时操作走异步 |
| WindowService 改变事件加载逻辑导致上下文不完整 | 中 | 高 | 充分测试后再启用，设置回退开关 |
| 框架不接受贡献 PR | 中 | 低 | 保持自建，不影响项目功能 |
| 压缩管道贡献回框架后接口不兼容 | 中 | 中 | 定义清晰的 Compressor 接口，保持向后兼容 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| simple | `examples/session/simple/` | `Service`, `ListSessions`, `SearchableService.SearchEvents` | 工厂模式 `NewSessionServiceByType("sqlite", cfg)` | 项目使用直接构造 `sqlite.NewService(db, opts...)`，未使用工厂；项目未使用 SearchEvents |
| persona | `examples/session/persona/` | `UpdateSessionState`, `GetSession`, `agent.WithGlobalInstruction` | 工厂模式 | 项目通过 `Runtime.SyncStateDelta` 更新状态，未使用 `agent.WithGlobalInstruction` 动态注入 |
| appendevent | `examples/session/appendevent/` | `AppendEvent`, `event.NewResponseEvent`, `sess.GetEventCount` | 直接构造 `inmemory.NewSessionService()` | 项目通过 Runner 自动 AppendEvent，不直接操作 |
| eventlimit | `examples/session/eventlimit/` | `WithSessionEventLimit`, `sess.Events` | 工厂模式 | 项目未使用 `WithSessionEventLimit`，通过压缩管道控制 |
| ttl | `examples/session/ttl/` | `WithSessionTTL`, `GetSession`（过期返回 nil） | 工厂模式 | 项目未使用 `WithSessionTTL`，通过 SessionStatusGuard 管理生命周期 |
| hook | `examples/session/hook/` | `AppendEventHook`, `GetSessionHook`, `Event.Tag` | 工厂模式 | **项目未使用任何 Hook** — 这是最大的对齐差距 |
| graph | `examples/session/graph/` | `graphagent`, `agent.WithGraphEmitFinalModelResponses`, `sess.State` | 直接构造 `inmemory.NewSessionService()` | 项目使用 `sess.State` 存储 `aranea:*` key，与示例的 `normalized_input` 等 key 约定不同 |

**关键发现**：框架示例中 Hook 示例展示了内容过滤 + 连续消息处理两种典型用法，项目完全未使用这些能力。这是 Session 模块对齐度从 ★★★★☆ 降到 ★★★☆☆ 的主要原因之一。

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Session 完整文档 | `docs/mkdocs/zh/session.md` |
| Session 英文文档 | `docs/mkdocs/en/session.md` |
