# 记忆（Memory）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/memory/`
> 项目实现路径：`internal/biz/memory*.go`、`internal/data/memory_shim_*.go`、`internal/service/memory*.go`、`internal/agent/memory_inject.go`、`internal/memory/trpc/auto_memory_queue.go`、`internal/cronrunner/jobs/memory_*.go`
> 当前对齐度：★★★☆☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `memory.Service` | `AddMemory(ctx, UserKey, content, topics, ...AddOption) error` | 添加/更新记忆（幂等） |
| | `UpdateMemory(ctx, Key, content, topics, ...UpdateOption) error` | 更新已有记忆 |
| | `DeleteMemory(ctx, Key) error` | 删除记忆 |
| | `ClearMemories(ctx, UserKey) error` | 清除用户全部记忆 |
| | `ReadMemories(ctx, UserKey, limit) ([]*Entry, error)` | 读取用户记忆列表 |
| | `SearchMemories(ctx, UserKey, query, ...SearchOption) ([]*Entry, error)` | 语义搜索记忆 |
| | `Tools() []tool.Tool` | 返回 6 个记忆工具 |
| | `EnqueueAutoMemoryJob(ctx, *session.Session) error` | 异步自动记忆提取入队 |
| | `Close() error` | 关闭服务 |
| `extractor.MemoryExtractor` | `Extract(ctx, UserKey, []Message, []*Entry) ([]Operation, error)` | LLM 驱动的记忆提取 |
| | `ShouldExtract(ctx, *MemoryJob) bool` | 判断是否需要提取 |
| | `SetPrompt(string)` | 动态更新系统提示词 |
| | `SetModel(model.Model)` | 动态更新模型 |
| | `Metadata() map[string]any` | 返回提取器元数据 |
| `auto.MemoryOperator` | `ReadMemories/SearchMemories/AddMemory/UpdateMemory/DeleteMemory/ClearMemories` | Auto Worker 的存储操作接口 |
| `session.Ingestor` | `IngestSession(ctx, *Session) error` | 会话摄入（mem0/tencentdb 模式） |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `memory.Kind` | 记忆类别常量：`KindFact="fact"` / `KindEpisode="episode"` |
| `memory.UserKey` | 用户标识：`{AppName, UserID}` |
| `memory.Key` | 记忆条目标识：`{UserKey, MemoryID}` |
| `memory.Metadata` | 事件元数据：`{Kind, EventTime, Participants, Location}` |
| `memory.Memory` | 完整记忆数据：`{ID, Content, Topics, Metadata}` |
| `memory.Entry` | 搜索结果条目：`{Memory, Score, UpdatedAt}` |
| `memory.SearchOptions` | 搜索选项：`{Kind, StartTime, EndTime, KindFallback, Deduplicate, HybridSearch, OrderByEventTime, Limit}` |
| `memory.AddOptions` | Add 操作选项：`{Metadata *Metadata}` |
| `memory.UpdateOptions` | Update 操作选项：`{Metadata *Metadata, Result *UpdateResult}` |
| `extractor.Operation` | 提取操作：`{Type, Memory, MemoryID, Topics, MemoryKind, EventTime, Participants, Location}` |
| `extractor.OperationType` | 操作类型常量：`OperationAdd/Update/Delete/Clear` |
| `auto.AutoMemoryConfig` | Auto 模式配置：`{Extractor, AsyncMemoryNum, MemoryQueueSize, MemoryJobTimeout, EnabledTools}` |
| `auto.MemoryJob` | 记忆提取任务：`{Ctx, UserKey, Session, LatestTs, Messages}` |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 实现 `memory.Service` 接口 | 自定义存储后端 | 需要 SQLite/Redis/PG 等之外的存储 |
| 实现 `extractor.MemoryExtractor` 接口 | 自定义提取器 | 需要替代默认 LLM 提取逻辑 |
| 实现 `extractor.Checker` 函数类型 | 自定义提取检查器 | 控制何时触发提取 |
| 实现 `embedder.Embedder` 接口 | 自定义向量嵌入器 | pgvector/mysqlvec 后端需要 |
| 实现 `session.Ingestor` 接口 | 会话摄入模式 | 外部记忆服务（mem0/tencentdb） |
| `WithCustomTool` / `WithToolEnabled` / `WithToolExposed` | 工具定制 | 控制暴露给 Agent 的记忆工具 |
| `plugin.Plugin` 接口 | BeforeModel 钩子 | tencentdb 的自动 recall 注入 |

### 1.4 配置选项

**通用选项（所有后端共享）**：

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithMemoryLimit(n)` | 记忆条数上限 | 无限制 |
| `WithMinSearchScore(score)` | 最低搜索分数 | 0 |
| `WithMaxResults(n)` | 最大返回结果数 | 无限制 |
| `WithCustomTool(tool)` | 自定义工具 | 无 |
| `WithToolEnabled(name)` | 启用指定工具 | add/update/search/load |
| `WithToolExposed(name)` | 暴露指定工具到 Agent | add/update/search/load |
| `WithAutoMemoryExposedTools(names)` | Auto 模式暴露的工具 | 同上 |
| `WithExtractor(e)` | 设置提取器（启用 Auto 模式） | nil（Agentic 模式） |
| `WithAsyncMemoryNum(n)` | 异步 worker 数量 | 1 |
| `WithMemoryQueueSize(n)` | 任务队列大小 | 10 |
| `WithMemoryJobTimeout(d)` | 任务超时时间 | 30s |

**SQLite 特有选项**：

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithTableName(name)` | 自定义表名 | `memories` |
| `WithSoftDelete()` | 启用软删除 | false |
| `WithSkipDBInit()` | 跳过数据库初始化 | false |

**pgvector 特有选项**：

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithEmbedder(e)` | 向量嵌入器 | 无（必须提供） |
| `WithIndexDimension(dim)` | 向量维度 | 1536 |
| `WithSimilarityThreshold(t)` | 相似度阈值 | 0 |
| `WithHNSWIndexParams(params)` | HNSW 索引参数 | 默认值 |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| `inmemory.NewMemoryService()` | `memory/inmemory/` | 内存存储，BM25 关键词搜索 |
| `sqlite.NewService()` | `memory/sqlite/` | SQLite 存储，JSON 序列化，支持 soft delete |
| `sqlitevec.NewService()` | `memory/sqlitevec/` | SQLite + 向量搜索 |
| `redis.NewService()` | `memory/redis/` | Redis 存储 |
| `postgres.NewService()` | `memory/postgres/` | PostgreSQL 存储 |
| `pgvector.NewService()` | `memory/pgvector/` | PostgreSQL + pgvector 向量搜索 |
| `mysql.NewService()` | `memory/mysql/` | MySQL 存储 |
| `mysqlvec.NewService()` | `memory/mysqlvec/` | MySQL + 向量搜索 |
| `mem0.NewService()` | `memory/mem0/` | mem0 外部服务集成（Ingestor + 只读工具） |
| `tencentdb.NewService()` | `memory/tencentdb/` | 腾讯云网关 sidecar（Ingestor + Plugin + 原生工具） |
| `extractor.NewExtractor()` | `memory/extractor/` | 默认 LLM 提取器，含 ~770 行系统提示词 |
| `auto.NewAutoMemoryWorker()` | `memory/internal/memory/auto.go` | 异步记忆提取 Worker，含 reconcile 去重 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `memory.Service` 接口 | `sqliteMemoryService`（框架内置） | ✅ 完全实现 | 通过 Wire 注入到 `MemorySet.TRPC` 字段 |
| `AddMemory` | 完全使用 | ✅ 完全合规 | — |
| `SearchMemories` | 完全使用 | ✅ 完全合规 | — |
| `ReadMemories` | 完全使用 | ✅ 完全合规 | — |
| `UpdateMemory/DeleteMemory/ClearMemories` | 完全使用 | ✅ 完全合规 | — |
| `Tools()` | 完全使用 | ✅ 完全合规 | 6 个标准工具作为 Agent 可选工具暴露 |
| `EnqueueAutoMemoryJob` | 完全使用 | ✅ 完全合规 | 框架 runner 层调用 |
| Agentic 模式 | 完全使用 | ✅ 完全合规 | Agent 通过工具主动管理记忆 |
| Auto 模式 | **未使用** | ❌ 未启用 | 项目使用自建 Cron Worker 替代 |
| 框架 9 种后端 | **未使用** | ❌ 自建替代 | 项目自建 SQLite + pgvector 双存储 |
| `extractor.MemoryExtractor` | **未使用** | ❌ 自建替代 | 项目自建 MemoryConsolidator + EnhancedTextExtractor |
| `auto.AutoMemoryWorker` | **未使用** | ❌ 自建替代 | 项目自建三优先级队列 + Cron Worker |
| 框架 reconcile 去重 | **未使用** | ❌ 自建替代 | 项目自建 SHA-256 fingerprint + 跨层去重 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| **L0 上下文组装快照** | `internal/data/memory_shim_l0.go` | 无对应 | 框架无分层概念，项目需要记录每次 prompt 组装的 token 预算/实际用量 |
| **L1 工作记忆（Task/Field）** | `internal/data/memory_shim_l1.go` | 无对应 | 框架无结构化工作记忆，项目需要任务/字段模型、budget 控制、TTL、字段历史、schema 验证 |
| **L2 情景记忆（Episode）** | `internal/data/memory_shim_l2.go` | 部分：`memory.Service` 的 `KindEpisode` | 框架 Episode 是扁平条目，项目需要混合评分召回（keyword + vector + importance + session boost + cross-encoder rerank） |
| **L3 语义事实（Fact）** | `internal/data/memory_shim_l3.go` | 部分：`memory.Service` 的 `KindFact` | 框架 Fact 是扁平条目，项目需要冲突检测、PII 审核、pgvector 级联删除、embedding 状态标记 |
| **L4 知识图谱（Entity/Relation）** | `internal/data/memory_shim_l4.go` | 无对应 | 框架无图谱能力，项目需要实体/关系图结构、BFS 邻域查询、衰减/强化信号 |
| **级联提案** | `internal/data/memory_shim_cascade.go`、`internal/biz/memory_l4_cascade.go` | 无对应 | 框架无级联机制，项目需要名称冲突检测 → Saga 步骤 → 人工审批 → 自动执行 |
| **PII 检测** | `internal/biz/memory_pii.go` | 无对应 | 框架无内置 PII 保护，项目需要 10 种 PII 模式正则扫描 + 脱敏 + 审核 |
| **跨层融合召回** | `internal/biz/memory_composite_recall.go` | 无对应 | 框架搜索是单层扁平的，项目需要 L2+L3 统一排序去重后注入 prompt |
| **策略审计引擎** | `internal/biz/memory_policy.go` | 无对应 | 框架无审计，项目需要所有记忆变更走审计路径，strict mode 可阻断写入 |
| **运行时策略** | `internal/biz/agent_memory_runtime_policy.go` | 无对应 | 框架无按 Agent 控制记忆层的机制，项目需要每个 Agent 独立控制哪些层启用读/写/注入 |
| **BeforeModel 注入** | `internal/agent/memory_inject.go` | 无对应（tencentdb 有类似 Plugin） | 框架不自动注入记忆到 prompt，项目需要 L1/L2/L3/L4 cue 自动注入 system message |
| **上下文压缩集成** | `internal/session/memory_compact.go` | 无对应 | 框架无压缩集成，项目需要压缩时使用 L1+L3 生成结构化摘要 |
| **Agent 演化** | `internal/data/memory_shim_l4.go`（Identity/Strategy/Evolution） | 无对应 | 框架无 Agent 演化概念，项目需要 Identity/Strategy Profile + Evolution Events |
| **三优先级队列** | `internal/memory/trpc/auto_memory_queue.go` | `auto.AutoMemoryWorker.jobChans` | 框架队列是单通道 per-user hash 路由，项目需要 High/Normal/Low 三优先级 + 租户配额 + 死信持久化 |
| **多策略提取器** | `internal/biz/memory_consolidator.go`、`internal/service/memory_llm_extractor.go`、`internal/service/memory_enhanced_extractor.go` | `extractor.MemoryExtractor` | 框架提取器是单一 LLM 提取，项目需要 Heuristic + LLM + Feedback + Chain 降级 + Enhanced（Episode+Entity+Relation） |
| **SHA-256 去重** | `internal/biz/memory_dedup.go` | `auto.reconcileOps`（Score + Jaccard） | 框架去重基于 Score+Jaccard，项目需要跨层 SHA-256 fingerprint 去重 |
| **Reranker** | `internal/biz/memory_rerank.go`、`internal/data/memory_rerank_adapter.go` | 无对应 | 框架无独立 Reranker，项目需要 Bigram Jaccard + 外部 API 适配 |
| **6 个 Cron 维护 Job** | `internal/cronrunner/jobs/memory_*.go` | 无对应 | 框架无维护 Job，项目需要 L1 归档、L2/L3/L4 衰减、索引修复、回填、迁移、死信重放 |
| **死信队列** | `internal/data/memory_job_deadletter.go`、`internal/cronrunner/jobs/memory_dead_letter_replayer.go` | 无对应 | 框架无死信机制，项目需要持久化 + 重放 + 放弃 |
| **事实/Episode 索引同步** | `internal/data/memory_fact_index_sync.go`、`internal/data/memory_episode_sync.go` | 无对应 | 框架无索引一致性修复，项目需要 SQLite ↔ pgvector 一致性修复 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| Auto 模式（`WithExtractor`） | 项目自建了更强大的 Cron Worker + 多策略提取器，Auto 模式的单 LLM 提取 + reconcile 不足以满足 L0-L4 多层提取需求 | 否（自建方案更完善） |
| 框架 9 种后端 | 项目需要 SQLite + pgvector 双存储（结构化数据 + 向量搜索），框架后端是单存储扁平模型，无法满足分层需求 | 否（架构差异根本性） |
| `extractor.MemoryExtractor` | 项目需要多策略提取（Heuristic + LLM + Feedback + Chain 降级 + Enhanced），框架提取器是单一 LLM 提取 | 否（自建方案更完善） |
| `auto.AutoMemoryWorker` | 项目需要三优先级队列 + 租户配额 + 死信，框架 Worker 是单通道 + per-user hash 路由 | 否（自建方案更完善） |
| `SearchOptions.HybridSearch` / `HybridRRFK` | 项目有自建的混合评分召回（每层独立评分），不使用框架的 RRF 合并 | 否（自建方案更精确） |
| `SearchOptions.KindFallback` | 项目 L2/L3 有独立的召回接口，不需要 Kind 回退 | 否 |
| `SearchOptions.Deduplicate` | 项目使用 SHA-256 fingerprint 去重，不使用框架的 Jaccard 去重 | 否 |
| `mem0` / `tencentdb` 集成 | 项目不使用外部记忆服务 | 否 |
| `WithSoftDelete` | 项目自建了独立的删除逻辑（含级联删除） | 否 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **Agentic 模式标准工具**：6 个内置工具（add/update/delete/clear/search/load），Agent 可直接调用 | 项目完全使用框架工具，但仅作为"可选暴露"，核心记忆流程走自建 L1 working_memory 工具 | 低（已在使用，但 L1 工具未对齐框架工具模式） |
| 2 | **双模式切换**：Agentic ↔ Auto 一键切换，通过 `WithExtractor` 配置 | 项目仅使用 Agentic 模式，Auto 模式未启用 | 低（自建 Cron Worker 功能更强，但框架 Auto 模式更轻量，适合简单场景） |
| 3 | **9 种存储后端即插即用**：通过 Option 函数切换后端 | 项目硬编码 SQLite + pgvector 双存储 | 中（如果未来需要支持 Redis/MySQL 等后端，框架后端可直接复用） |
| 4 | **标准化的记忆 ID 生成**：`GenerateMemoryID` 基于 SHA256(content+appName+userID+metadata) | 项目自建了 `FactFingerprint`（SHA-256）去重，但 ID 生成逻辑分散 | 低（ID 生成逻辑已各自实现） |
| 5 | **框架文档和示例**：完整的中文文档 + 示例代码，降低学习成本 | 项目记忆系统复杂度高（20+ biz 文件、18+ data 文件），无对应文档 | 中（框架文档可作为简化版记忆的参考） |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **L0-L4 五层记忆体系**：Snapshot/Task+Field/Episode/Fact/Entity+Relation | 框架是扁平 KV（UserKey → []Entry），仅有 fact/episode 两种 Kind | 贡献回框架（分层抽象） |
| 2 | **混合评分召回**：每层独立评分（keyword + vector + importance + recency + cross-encoder rerank） | 框架搜索是 BM25 + 可选 vector + RRF | 贡献回框架（评分策略抽象） |
| 3 | **多策略提取器**：Heuristic + LLM + Feedback + Chain 降级 + Enhanced（Episode+Entity+Relation） | 框架是单一 LLM 提取器 | 贡献回框架（提取器链） |
| 4 | **PII 检测与脱敏**：10 种 PII 模式正则扫描 + 脱敏 + 审核 | 框架无内置 PII 保护 | 贡献回框架（PII Scanner 接口） |
| 5 | **策略审计引擎**：所有记忆变更走审计路径，strict mode 可阻断写入 | 框架无审计 | 贡献回框架（Audit Hook） |
| 6 | **运行时策略**：每个 Agent 独立控制哪些层启用读/写/注入 | 框架无按 Agent 控制记忆层的机制 | 贡献回框架（MemoryPolicy 接口） |
| 7 | **BeforeModel 自动注入**：L1/L2/L3/L4 cue 自动注入 system message | 框架不自动注入（tencentdb 有类似 Plugin 但不通用） | 贡献回框架（通用 MemoryInject Plugin） |
| 8 | **三优先级队列 + 租户配额 + 死信** | 框架是单通道 + per-user hash 路由 | 贡献回框架（PriorityQueue 接口） |
| 9 | **知识图谱（L4）**：Entity/Relation 图结构、BFS 邻域查询、衰减/强化 | 框架无图谱能力 | 贡献回框架（GraphMemory 接口） |
| 10 | **级联提案 + Saga**：名称冲突检测 → Saga 步骤 → 人工审批 → 自动执行 | 框架无级联机制 | 保持自建（过于业务特定） |
| 11 | **6 个 Cron 维护 Job**：归档/衰减/索引修复/回填/迁移/死信重放 | 框架无维护 Job | 贡献回框架（MaintenanceJob 接口） |
| 12 | **上下文压缩集成**：压缩时使用 L1+L3 生成结构化摘要 | 框架无压缩集成 | 保持自建（与项目压缩管道紧耦合） |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| **扁平 vs 分层** | 框架设计为通用扁平 KV 存储（fact/episode），项目业务需要五层结构化记忆（Snapshot/Task/Episode/Fact/Graph），这是架构设计理念的差异 | 全部记忆模块（20+ biz 文件、18+ data 文件） |
| **单存储 vs 双存储** | 框架后端是单存储（SQLite 或 pgvector 二选一），项目需要 SQLite（结构化数据）+ pgvector（向量搜索）双存储协同 | 所有 L2/L3/L4 的 data 层实现 |
| **单提取 vs 多策略** | 框架提取器是单一 LLM 提取，项目需要多策略（Heuristic + LLM + Feedback + Chain 降级 + Enhanced），因为不同层需要不同提取策略 | `memory_consolidator.go`、`memory_llm_extractor.go`、`memory_enhanced_extractor.go` |
| **单队列 vs 三优先级** | 框架 Auto Worker 是单通道 + per-user hash 路由，项目需要三优先级（High=反馈/Normal=轮次/Low=回填）+ 租户配额 + 死信，因为多租户场景需要公平调度 | `auto_memory_queue.go`、`memory_job_deadletter.go` |
| **无注入 vs BeforeModel 注入** | 框架不自动注入记忆到 prompt（仅提供工具），项目需要在每次模型调用前自动注入 L1/L2/L3/L4 cue，因为 Agent 需要被动获取上下文而非主动查询 | `memory_inject.go` |
| **无治理 vs 策略审计** | 框架无审计/PII/策略控制，项目需要严格治理（PII 脱敏、审计日志、strict mode 阻断），因为生产环境有合规要求 | `memory_policy.go`、`memory_pii.go` |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | L2/L3 记忆存储适配框架 `memory.Service` 接口 | 新增适配层 | P2 | `internal/data/memory_shim_l2.go`、`memory_shim_l3.go` | 代码减少约 200 行；可复用框架后端 |
| 2 | 提取器链适配框架 `extractor.MemoryExtractor` 接口 | 新增适配层 | P2 | `internal/biz/memory_consolidator.go` | 接口合规；可复用框架 Checker 机制 |
| 3 | 贡献 L0-L4 分层抽象回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/memory/` | 框架增强；长期减少自建维护 |
| 4 | 贡献 PII Scanner 接口回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/memory/` | 框架增强；通用安全能力 |
| 5 | 贡献 MemoryInject Plugin 回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/memory/` | 框架增强；通用注入能力 |
| 6 | 贡献 PriorityQueue 接口回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/memory/` | 框架增强；通用队列能力 |
| 7 | 贡献 Audit Hook 接口回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/memory/` | 框架增强；通用审计能力 |
| 8 | 评估启用框架 Auto 模式用于简单场景 | 启用框架功能 | P3 | `internal/runtime/` | 简单场景减少自建代码 |

### 4.2 对齐项详情

#### 对齐项 #1：L2/L3 记忆存储适配框架 `memory.Service` 接口

**类型**：新增适配层

**现状**：
- 项目当前实现：L2（`memory_shim_l2.go`）和 L3（`memory_shim_l3.go`）使用 Raw SQL 直接操作 SQLite + pgvector，不实现框架 `memory.Service` 接口
- 框架提供能力：`memory.Service` 接口 + 9 种后端，支持 Add/Update/Delete/Clear/Read/Search 完整 CRUD

**对齐方案**：
1. 创建 `L2MemoryServiceAdapter` 和 `L3MemoryServiceAdapter`，实现 `memory.Service` 接口，内部委托给现有的 `L2EpisodeWriter`/`L2RecallStore`/`L3FactWriter`/`L3FactReader`
2. 适配层将框架的扁平 `Entry` 映射到项目的 L2 Episode / L3 Fact 数据模型
3. 保留项目自建的混合评分召回作为 `SearchMemories` 的实现（不使用框架的 BM25 + RRF）
4. 现有 Raw SQL 实现不变，适配层仅做接口桥接

**代码变更范围**：
- 新增：`internal/adapter/memory_l2_service.go`、`internal/adapter/memory_l3_service.go`
- 修改：`internal/runtime/memory_set.go`（可选：通过适配层统一 `TRPC` 字段）
- 删除：无

**兼容性风险**：
- 低：适配层是纯新增，不影响现有逻辑
- 框架 `memory.Service` 的 `SearchMemories` 签名与项目混合评分召回的参数不完全匹配（框架用 `SearchOption`，项目用独立参数），需要映射

**回退方案**：
- 适配层是可选的，不使用时直接删除即可

**验证方法**：
- 单元测试：验证适配层实现 `memory.Service` 接口的所有方法
- 集成测试：通过适配层执行 L2/L3 的 CRUD + 搜索，结果与直接调用一致

**预期收益**：
- 代码减少：约 200 行（可复用框架工具和后端）
- 性能影响：无（适配层仅做接口桥接）
- 维护成本：中等降低（框架接口变更时只需更新适配层）

---

#### 对齐项 #2：提取器链适配框架 `extractor.MemoryExtractor` 接口

**类型**：新增适配层

**现状**：
- 项目当前实现：`MemoryConsolidator`（Heuristic + LLM + Feedback + Chain 降级）+ `EnhancedTextExtractor`（Episode + Entity + Relation），不实现框架 `extractor.MemoryExtractor` 接口
- 框架提供能力：`extractor.MemoryExtractor` 接口 + `Checker` 机制 + 默认 LLM 提取器

**对齐方案**：
1. 创建 `ConsolidatorExtractorAdapter`，将项目的 `MemoryConsolidator` 适配为 `extractor.MemoryExtractor` 接口
2. 适配层将框架的 `Operation`（add/update/delete/clear）映射到项目的 `MemoryProposal`
3. 利用框架 `Checker` 机制替代项目自建的提取触发判断逻辑
4. 保留项目自建的多策略降级链作为核心逻辑

**代码变更范围**：
- 新增：`internal/adapter/memory_extractor.go`
- 修改：无（适配层是纯新增）
- 删除：无

**兼容性风险**：
- 中：框架 `MemoryExtractor.Extract` 的输入是 `[]session.Message` + `[]*memory.Entry`，项目 Consolidator 的输入是 `[]biz.Message` + 自建上下文，需要类型转换
- 框架 `Operation` 类型与项目 `MemoryProposal` 类型的映射可能不完全一一对应

**回退方案**：
- 适配层是可选的，不使用时直接删除即可

**验证方法**：
- 单元测试：验证适配层实现 `extractor.MemoryExtractor` 接口
- 对比测试：通过适配层提取的结果与直接调用 Consolidator 一致

**预期收益**：
- 代码减少：约 50 行（可复用框架 Checker 机制）
- 性能影响：无
- 维护成本：低降低（框架接口变更时只需更新适配层）

---

#### 对齐项 #3：贡献 L0-L4 分层抽象回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：L0-L4 五层记忆体系，每层有独立的表结构、读写接口、评分逻辑、维护 Job
- 框架提供能力：扁平 KV 存储（`UserKey → []Entry`），仅有 fact/episode 两种 Kind

**对齐方案**：
1. 在框架 `memory` 包中定义 `Layer` 接口（或 `TieredService` 接口），支持注册多层记忆后端
2. 每层实现 `memory.Service` 的子集（Read/Write/Search），框架提供统一的跨层搜索和注入
3. 项目将 L0-L4 各层注册为框架 Layer 实现
4. 分阶段贡献：先贡献接口定义和注册机制，再贡献内置 Layer 实现（如 EpisodeLayer、FactLayer）

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/memory/tiered/`（框架侧）
- 修改：`pkg/trpc-agent-go/memory/memory.go`（添加 Layer 注册方法）
- 删除：无

**兼容性风险**：
- 高：框架需要接受新的分层抽象，需要与框架维护者协商
- 分层抽象可能与框架现有的扁平设计理念冲突

**回退方案**：
- 框架不接受贡献时，项目保持自建

**验证方法**：
- 框架侧单元测试：验证 Layer 注册、跨层搜索、注入机制
- 项目侧集成测试：验证 L0-L4 作为 Layer 注册后功能不变

**预期收益**：
- 代码减少：约 500 行（框架提供分层基础设施后，项目可删除自建的分层管理代码）
- 性能影响：无
- 维护成本：显著降低（框架维护分层抽象，项目只需实现各层逻辑）
- 功能增强：框架用户可获得分层记忆能力

---

#### 对齐项 #4：贡献 PII Scanner 接口回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`internal/biz/memory_pii.go`，10 种 PII 模式正则扫描 + 脱敏 + 审核
- 框架提供能力：无内置 PII 保护

**对齐方案**：
1. 在框架 `memory` 包中定义 `PIIScanner` 接口：`Scan(content string) ([]PIIMatch, error)` + `Redact(content string, matches []PIIMatch) string`
2. 提供内置实现 `regexpi.NewScanner()`，包含常见 PII 模式（email/phone/ID card/bank/credit/SSN 等）
3. 添加 `WithPIIScanner(scanner)` Option，在 AddMemory/UpdateMemory 时自动扫描和脱敏
4. 项目将自建 PII 扫描器适配为框架 `PIIScanner` 接口

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/memory/pii/`（框架侧）
- 修改：`pkg/trpc-agent-go/memory/inmemory/options.go`（添加 `WithPIIScanner` Option）
- 删除：无

**兼容性风险**：
- 低：PII Scanner 是纯新增功能，不影响现有逻辑

**回退方案**：
- 框架不接受贡献时，项目保持自建

**验证方法**：
- 单元测试：验证 PII Scanner 接口和内置实现
- 集成测试：验证 AddMemory 时自动扫描和脱敏

**预期收益**：
- 代码减少：约 100 行（框架提供 PII 基础设施）
- 维护成本：中等降低
- 功能增强：框架用户可获得 PII 保护能力

---

#### 对齐项 #5：贡献 MemoryInject Plugin 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`internal/agent/memory_inject.go`，BeforeModel Hook 自动注入 L1/L2/L3/L4 cue 到 system message
- 框架提供能力：`plugin.Plugin` 接口（tencentdb 有类似 `recallPlugin` 但不通用）

**对齐方案**：
1. 在框架 `memory` 包中定义通用的 `MemoryInjectPlugin`，实现 `plugin.Plugin` 接口
2. Plugin 在 `BeforeModel` 钩子中调用 `memory.Service.ReadMemories`/`SearchMemories` 获取记忆，注入到 system message
3. 支持配置注入策略（哪些 Kind 注入、注入位置、token 预算）
4. 项目将自建的 `memoryInjectBeforeHook` 替换为框架 Plugin

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/memory/plugin/`（框架侧）
- 修改：`internal/agent/memory_inject.go`（替换为框架 Plugin）
- 删除：部分自建注入逻辑

**兼容性风险**：
- 中：项目注入逻辑复杂（L1/L2/L3/L4 各自不同的 cue 构建逻辑），框架 Plugin 需要足够的扩展性
- 框架 Plugin 的 `BeforeModel` 钩子签名需要与项目使用方式兼容

**回退方案**：
- 框架 Plugin 不满足需求时，保留自建注入逻辑

**验证方法**：
- 单元测试：验证 Plugin 的 BeforeModel 钩子正确注入记忆
- 集成测试：验证注入后的 Agent 行为与自建注入一致

**预期收益**：
- 代码减少：约 150 行（框架提供注入基础设施）
- 维护成本：中等降低
- 功能增强：框架用户可获得自动记忆注入能力

---

#### 对齐项 #6：贡献 PriorityQueue 接口回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`internal/memory/trpc/auto_memory_queue.go`，三优先级队列（High/Normal/Low）+ 租户配额 + 死信持久化
- 框架提供能力：`auto.AutoMemoryWorker`，单通道 + per-user hash 路由

**对齐方案**：
1. 在框架 `memory` 包中定义 `PriorityQueue` 接口：`Enqueue(job, priority)` / `Chan() <-chan Job` / `Close()`
2. 提供内置实现 `priorityqueue.New()`，支持多优先级 + 配额
3. 添加 `WithPriorityQueue(q)` Option，替代框架的 `WithMemoryQueueSize`
4. 项目将自建 `MemoryJobQueue` 适配为框架 `PriorityQueue` 接口

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/memory/priorityqueue/`（框架侧）
- 修改：`pkg/trpc-agent-go/memory/internal/memory/auto.go`（支持 PriorityQueue）
- 删除：无

**兼容性风险**：
- 中：框架 Auto Worker 的队列机制需要重构以支持 PriorityQueue

**回退方案**：
- 框架不接受贡献时，项目保持自建

**验证方法**：
- 单元测试：验证 PriorityQueue 接口和内置实现
- 压力测试：验证多优先级调度和配额控制

**预期收益**：
- 代码减少：约 100 行（框架提供优先级队列基础设施）
- 维护成本：中等降低
- 功能增强：框架用户可获得优先级队列能力

---

#### 对齐项 #7：贡献 Audit Hook 接口回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`internal/biz/memory_policy.go`，策略审计引擎（strict mode + action log）
- 框架提供能力：无审计

**对齐方案**：
1. 在框架 `memory` 包中定义 `AuditHook` 接口：`BeforeAdd/BeforeUpdate/BeforeDelete(ctx, key, content) error` / `AfterAdd/AfterUpdate/AfterDelete(ctx, key, result) error`
2. 添加 `WithAuditHook(hook)` Option，在 CRUD 操作前后调用 Hook
3. 项目将自建 `MemoryPolicyEngine` 适配为框架 `AuditHook` 接口

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/memory/audit/`（框架侧）
- 修改：`pkg/trpc-agent-go/memory/inmemory/options.go`（添加 `WithAuditHook` Option）
- 删除：无

**兼容性风险**：
- 低：Audit Hook 是纯新增功能，不影响现有逻辑

**回退方案**：
- 框架不接受贡献时，项目保持自建

**验证方法**：
- 单元测试：验证 Audit Hook 接口和调用时机
- 集成测试：验证 strict mode 阻断写入

**预期收益**：
- 代码减少：约 80 行
- 维护成本：低降低
- 功能增强：框架用户可获得审计能力

---

#### 对齐项 #8：评估启用框架 Auto 模式用于简单场景

**类型**：启用框架功能

**现状**：
- 项目当前实现：所有自动记忆提取都走自建 Cron Worker（`internal/cronrunner/jobs/auto_memory.go`），功能强大但复杂
- 框架提供能力：Auto 模式（`WithExtractor`），轻量级自动记忆提取，适合简单场景

**对齐方案**：
1. 评估是否需要在简单场景（如单 Agent 无分层需求）下启用框架 Auto 模式
2. 如果启用，在 `MemorySet` 中添加 Auto 模式配置选项
3. 保留自建 Cron Worker 作为复杂场景的默认选择

**代码变更范围**：
- 新增：无
- 修改：`internal/runtime/memory_set.go`（添加 Auto 模式配置）
- 删除：无

**兼容性风险**：
- 低：Auto 模式是可选的，不影响现有逻辑

**回退方案**：
- 不启用 Auto 模式即可

**验证方法**：
- 集成测试：验证 Auto 模式下记忆提取和注入正常工作
- 对比测试：Auto 模式 vs 自建 Cron Worker 的提取质量对比

**预期收益**：
- 代码减少：0 行（不减少现有代码，但简单场景可避免使用复杂 Cron Worker）
- 维护成本：低降低（简单场景使用框架标准模式）
- 功能增强：简单场景可获得框架 Auto 模式的持续优化

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1（L2/L3 适配层）、#2（提取器适配层） | Event 对齐完成 | 中 |
| Phase 2 | #3（分层抽象贡献）、#4（PII Scanner 贡献）、#7（Audit Hook 贡献） | Phase 1 | 大 |
| Phase 3 | #5（MemoryInject Plugin 贡献）、#6（PriorityQueue 贡献） | Phase 2 | 大 |
| Phase 4 | #8（评估 Auto 模式） | Phase 1 | 小 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架不接受分层抽象贡献 | 中 | 高 | 先在项目内实现适配层，积累使用经验后再贡献 |
| 适配层性能开销 | 低 | 中 | 适配层仅做接口桥接，无额外计算 |
| 框架 `memory.Service` 接口变更导致适配层失效 | 低 | 中 | 适配层隔离变更影响，更新适配层即可 |
| L2/L3 混合评分召回无法映射到框架 `SearchMemories` | 中 | 低 | 保留自建搜索，适配层的 `SearchMemories` 委托给自建实现 |
| 贡献回框架的代码需要大量重构以满足框架代码规范 | 高 | 中 | 提前了解框架贡献规范，按规范编写 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| Memory Service 工厂 | `examples/memory/util.go` | `NewMemoryServiceByType()`、`memorysqlite.NewService()`、`memorypgvector.NewService()` | 按类型创建后端 + Option 配置 | 项目不使用框架后端工厂，自建 SQLite + pgvector 双存储 |
| Memory Runner 集成 | `examples/memory/util.go` | `runner.WithMemoryService()`、`llmagent.WithTools(memoryService.Tools())` | Runner 注入 MemoryService + Agent 暴露工具 | 项目通过 `MemorySet` 组合框架 Service + 自建 Admin/Recall，工具暴露方式一致 |
| Auto 模式配置 | `examples/memory/util.go` | `WithExtractor()`、`WithAsyncMemoryNum()`、`WithMemoryQueueSize()`、`WithMemoryJobTimeout()` | 配置 Extractor 启用 Auto 模式 | 项目未使用 Auto 模式，自建 Cron Worker + 三优先级队列替代 |
| SQLite 后端 | `examples/memory/util.go` | `memorysqlite.NewService(db, WithSoftDelete(), WithExtractor())` | 传入 `*sql.DB` + Option | 项目自建 SQLite 实现（Raw SQL），不使用框架 SQLite 后端 |
| pgvector 后端 | `examples/memory/util.go` | `memorypgvector.NewService(WithHost(), WithEmbedder(), WithSoftDelete())` | Option 配置 + Embedder | 项目自建 pgvector 实现（Raw SQL），不使用框架 pgvector 后端 |
| In-memory 后端 | `examples/memory/util.go` | `memoryinmemory.NewMemoryService(opts...)` | 纯 Option 配置 | 项目不使用 in-memory 后端 |

**对齐方案必须以示例代码的用法为目标状态**：
- 对齐项 #1 的目标状态：L2/L3 适配层创建的 `memory.Service` 实例，可像示例中一样通过 `runner.WithMemoryService()` 注入
- 对齐项 #2 的目标状态：项目 Consolidator 适配的 `extractor.MemoryExtractor` 实例，可像示例中一样通过 `WithExtractor()` 配置
- 对齐项 #8 的目标状态：简单场景下像示例一样使用 `WithExtractor(extractor.NewExtractor(model))` 启用 Auto 模式

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Memory 使用指南（中文） | `docs/mkdocs/zh/memory.md` |
| Memory API 参考 | `pkg/trpc-agent-go/memory/memory.go` |
| Extractor API 参考 | `pkg/trpc-agent-go/memory/extractor/extractor.go`、`memory/extractor/memory.go` |
| Auto Worker API 参考 | `pkg/trpc-agent-go/memory/internal/memory/auto.go` |
| Tool API 参考 | `pkg/trpc-agent-go/memory/tool/tool.go`、`memory/tool/types.go` |
| Checker API 参考 | `pkg/trpc-agent-go/memory/extractor/checker.go` |
