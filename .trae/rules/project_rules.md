# Aranea-Agents 项目开发规则

> AI 每次开发必须遵守。本文件为**索引 + 全局约束**，详细规范见各 SKILL（冲突时以 SKILL 为准）。
> 本文件**不重复** SKILL 中的红线、编程规范、分层规范、决策树等内容——只告诉你「去哪个 SKILL 找」。

---

## 一、项目概览

Aranea-Agents 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核。

**技术栈**：Go + Kratos v2（HTTP/gRPC/WebSocket）| trpc-agent-go（Agent 运行时）| Vue 3 + Quasar + Pinia + TypeScript | SQLite（Ent ORM）| Wire（编译期 DI）

**双框架分工**：
- Kratos v2：传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire DI
- trpc-agent-go：Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team）

---

## 二、SKILL 体系（按任务选读）

> 以下 SKILL 为各领域的权威规范。本文件不再重复其内容。

### 编码类

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `aranea-coding-guide` | 后端项目编码指南（详细版） | 编写 Go 后端代码 |
| `go-oop-guide` | 通用 Go OOP 编程指导 | struct/接口/组合/工厂设计 |
| `aranea-frontend-guide` | 前端项目编码指南（详细版） | 编写 Vue 3/Quasar/Pinia/TS 代码 |
| `vue-frontend-guide` | 通用 Vue 3 编程指导 | 组件/Composable/TypeScript 设计 |

### 文档类

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `aranea-docs-guide` | docs 目录文档维护规范 | 创建/修改/移动 docs/ 下的任何文档 |

### 审查类

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `aranea-review` | 全栈代码审查（后端 + 前端） | 审查代码的架构/分层/数据流/OOP/UX 合规 |
| `go-oop-review` | Go OOP 代码审查 | 审查 Go 代码的 OOP 合规 |

### 工作流类

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `aranea-test-loop` | 自动化测试循环 | 运行测试、修复失败、生成报告 |

### Superpowers 辅助 SKILL（按需使用）

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `brainstorming` | 协作式设计探索 | 任何创造性工作前 |
| `writing-plans` | 细粒度实施计划 | 有规格后、编码前 |
| `subagent-driven-development` | 子代理驱动开发 | 执行实施计划 |
| `test-driven-development` | TDD 红绿重构 | 实现任何功能/修复 |
| `verification-before-completion` | 完成前验证 | 声明完成前必须提供证据 |
| `finishing-a-development-branch` | 分支收尾 | 实施完成后 |
| `executing-plans` | 顺序执行计划 | 无子代理时的替代方案 |
| `systematic-debugging` | 系统调试 | 遇到 bug/测试失败 |
| `requesting-code-review` | 请求代码审查 | 任务/功能完成后 |
| `receiving-code-review` | 接收审查反馈 | 收到审查意见时 |

### 各 SKILL 覆盖范围速查

| 你要查的内容 | 去哪个 SKILL |
|-------------|-------------|
| 后端 24 条有效红线（+3 条已降级为编程规范，共 27 条编号） | `aranea-coding-guide` §2 |
| 前端 11 条有效红线（+4 条已降级为编程规范，共 15 条编号） | `aranea-frontend-guide` §1 |
| 后端编程规范 CS-B1~B18 | `aranea-coding-guide` §14 |
| 前端编程规范 CS-F1~F9 | `aranea-frontend-guide` §13 |
| 依赖方向 / 分层规范 | `aranea-coding-guide` §1+§5 |
| Agent 运行时规范 | `aranea-coding-guide` §6 |
| **框架反模式清单（AI 高频踩坑）** | `aranea-coding-guide` §6.13 |
| **框架文档导航地图** | `aranea-coding-guide` §6.14 |
| 前端数据流 / 分层 | `aranea-frontend-guide` §3+§4 |
| 聊天消息分组 | `aranea-frontend-guide` §5 |
| UX 主题 / Dialog 规范 | `aranea-frontend-guide` §6+§7 |
| **业务逻辑正确性（状态机/事件可靠性/不变量/边界条件）** | `aranea-coding-guide` §15 |
| **测试约定（分层测试/mock 策略/事件流/并发测试）** | `aranea-coding-guide` §16 |
| 数据库编码规范 | `aranea-coding-guide` §5.4（Schema/访问模式/Repo/事务/读写分离/迁移）+ `project_rules.md` 数据库框架约束（连接管理/迁移体系/错误翻译/技术债务/开发红线） |
| Go OOP 设计模式 | `go-oop-guide` |
| Vue 3 组件/Composable 模式 | `vue-frontend-guide` |
| 代码审查清单 | `aranea-review`（全栈）、`go-oop-review`（Go OOP） |
| 决策树（代码该放哪） | `aranea-coding-guide` §4、`aranea-frontend-guide` §2 |
| AI 编码自检清单 | `aranea-coding-guide` §11、`aranea-frontend-guide` §10 |
| docs 文档命名/存放规范 | `aranea-docs-guide` |
| 模块文档三件套格式 | `aranea-docs-guide` §2 |
| 子模块合并规则 | `aranea-docs-guide` §2.3 |
| 架构评判标准 AS-ADR~AS-EVT | `project_rules.md` §八 + `docs/reports/2026-06-11-review-architecture-runtime-pain-points.md` |

---

## 日志架构约束

### 红线（强制）

- **红线 #16**：禁止 `log/slog`，统一使用 `pkg/loggateway.Logger`
- **Global() deprecated**：`loggateway.Global()` 已废弃，新代码必须通过构造注入 `loggateway.Logger`
- **CtxFlowLog\***：`internal/event/flow_context.go` 中的 `WithFlowLogger`/`FlowLoggerFromContext`/`NewFlowLogger` 为遗留 API（已标记 Deprecated），新代码应使用 `loggateway.Logger` + `With()` 预设字段
- **RuntimeLogAdapter**：trpc-agent-go 运行时日志已通过 `RuntimeLogAdapter` 桥接到 loggateway Pipeline，无需额外处理

### 架构概览

```
业务代码 → loggateway.Logger → Gateway.emitToPipeline() → logpipeline.Pipeline.Emit()
    → pipeline.dispatchLoop() → SinkGroup.Emit() → Sink.Write()
        ├── FileSink (JSON + lumberjack 轮转)
        ├── StdoutSink (JSON → stdout)
        └── EventBusSink (→ event.Bus → WebSocket/持久化，含熔断器)

trpc-agent-go 运行时 → RuntimeLogAdapter → loggateway.Logger → 同上
```

### 核心组件

| 组件 | 包路径 | 职责 |
|------|--------|------|
| `Logger` 接口 | `pkg/loggateway/logger.go` | 统一日志接口：Debug/Info/Warn/Error/With |
| `Gateway` | `pkg/loggateway/gateway.go` | Logger 实现，桥接 Pipeline，nil-safe |
| `Pipeline` | `pkg/logpipeline/pipeline.go` | 异步日志分发（channel + 多 SinkGroup 隔离） |
| `SinkGroup` | `pkg/logpipeline/sink_group.go` | Sink 隔离（独立 goroutine/buffer/DropPolicy） |
| `FileSink` | `pkg/logpipeline/file_sink.go` | 文件输出（JSON + lumberjack 轮转） |
| `StdoutSink` | `pkg/logpipeline/stdout_sink.go` | 标准输出（JSON + 级别过滤） |
| `EventBusSink` | `pkg/logpipeline/eventbus_sink.go` | 事件总线输出（熔断器：5次失败开启，10秒恢复，3次探测关闭） |
| `RuntimeLogAdapter` | `internal/adapter/runtime_log.go` | trpc-agent-go 桥接（Fatal 同步写 stderr，其余异步） |

### 使用规则

1. **构造注入**：所有 struct 通过构造函数参数 `lg loggateway.Logger` 获取 Logger，禁止在构造函数外调用 `loggateway.Global()`
2. **With() 预设字段**：需要固定上下文字段时，在构造函数中用 `lg.With()` 创建子 Logger 存入 struct 字段
   ```go
   // 正确：构造时预设字段
   func NewXxxUsecase(lg loggateway.Logger, ...) *XxxUsecase {
       return &XxxUsecase{lg: lg.With(loggateway.Domain("xxx"))}
   }
   // 错误：每次调用都 With()
   func (u *XxxUsecase) Do() { u.lg.With(loggateway.Domain("xxx")).Info("msg") }
   ```
3. **结构化字段优先**：使用预定义字段构造函数（`loggateway.StepID`/`SessionID`/`RunID`/`AgentKey`/`Domain`/`Phase`/`Err` 等），禁止拼接字符串到 msg 中
4. **错误记录**：使用 `loggateway.Err(err)` 记录错误（自动解包错误链），不要用 `loggateway.Str("error", err.Error())`
5. **日志级别语义**：
   - `Debug`：开发调试信息，生产环境默认关闭
   - `Info`：正常业务流程关键节点
   - `Warn`：可恢复的异常/降级/兼容处理
   - `Error`：需要关注的错误，但不影响进程存活
6. **测试中使用 Noop**：测试代码中优先使用 `loggateway.NewNoop()` 创建静默 Logger，避免 `loggateway.Global()`（Global 是 deprecated API）
7. **data 层日志访问**：data 层存在两种模式——独立 Repo 持有 `lg loggateway.Logger`（推荐）或通过 `r.data.lg` 访问共享 Logger。新增 Repo 优先使用独立持有模式
8. **禁止 fmt.Fprintf(os.Stderr, ...)**：除 `RuntimeLogAdapter.Fatal` 和 Pipeline 内部 panic 恢复外，业务代码禁止直接写 stderr

### 初始化流程（cmd/admin/logging.go）

1. 创建 `logpipeline.Pipeline`（缓冲区 4096）
2. 根据配置创建 Sink（file/stdout/eventbus），eventbus sink 延迟到 BeforeStart
3. 创建 `loggateway.Gateway`（注入 Pipeline）
4. 创建 `RuntimeLogAdapter` 并替换 `agentlog.Default`/`agentlog.ContextDefault`
5. 返回 Logger + Pipeline + Sink 配置供 Wire 使用

### 限流机制

Pipeline 支持基于 stepID 前缀匹配的令牌桶限流（`ThrottleRule{Prefix, MaxPerSec}`），按前缀最长匹配，空闲 5 分钟自动清理桶。被限流的日志计入 `Throttled()` 计数，不进入 Sink。

### 前端日志

前端无日志框架，仅使用原生 `console.warn/info`（共约 13 处），集中在 chat 实时通信模块。禁止使用 `console.log`（无法按级别过滤），优先使用 `console.warn`（异常）或 `console.info`（关键流程）。

---

## 数据库框架约束（SQLite + Ent ORM）

> 详细规范见 `aranea-coding-guide` §5.4。本节为**架构分析 + 审查验证后的实战规则**，补充 SKILL 中未覆盖的注意事项。

### 架构概览

```
internal/biz           ← Repo 接口定义（窄接口：Reader/Writer/Mutator）
        ↓
internal/data          ← Repo 实现
    ├── Data struct     ← 连接管理（双连接读写分离 + 可选 Postgres）
    ├── ReadWriteClient ← 事务感知的 Ent 读写选择器
    ├── ReadWriteDB     ← 事务感知的 Raw SQL 读写选择器
    ├── ent/schema/     ← 76 个 Ent Schema（唯一真相源）
    ├── ent/            ← go generate 生成物（禁止手动修改）
    ├── sql/migrations/ ← DDL 迁移 SQL 文件（28 个版本化迁移）
    └── *_repo.go       ← Repo 实现（Ent / Raw SQL / 混合）
```

### 连接管理

| 连接 | 用途 | MaxOpenConns | 访问方式 |
|------|------|-------------|---------|
| `entClient` | Ent 写 | 1（SQLite 单写） | `d.RW().Write(ctx)` |
| `readClient` | Ent 读 | 2（WAL 并发读） | `d.RW().Read(ctx)` |
| `rawDB` | Raw SQL 写 | 1 | `d.RWDB().WriteDB(ctx)` |
| `readDB` | Raw SQL 读 | 2 | `d.RWDB().ReadDB(ctx)` |
| `pg` | Postgres（pgvector） | 8 | `d.Postgres()` |

**SQLite PRAGMAs**（写连接 + 读连接均设置）：
- `foreign_keys=ON`、`journal_mode=WAL`、`busy_timeout=30000`
- `synchronous=NORMAL`、`wal_autocheckpoint=500`

### 读写分离规则

| 操作类型 | Ent Repo | Raw SQL Repo |
|---------|----------|-------------|
| 读 | `r.data.RW().Read(ctx)` | `r.data.RWDB().ReadDB(ctx)` |
| 写 | `r.data.RW().Write(ctx)` | `r.data.RWDB().WriteDB(ctx)` |
| DDL | `r.data.RW().Write(ctx)` | `r.data.RWDB().WriteHandle()` |

**已废弃的访问器**（禁止新增调用）：`RawDB()`、`ReadDB()`、`ReadEnt()`、`ReadClient()`

### 事务管理

**统一入口**：`Data.ExecInTx(ctx, fn func(ctx) error) error`

关键行为：
1. **嵌套事务检测**：context 中已有事务时复用，不创建 savepoint
2. **分离 context**：事务在 `context.Background()` + 30s 硬超时上执行，防止 HTTP 取消中断 SQLite 操作
3. **提交前检查**：`fn()` 成功后检查原始调用方 context 是否已取消，已取消则回滚
4. **双 key 注入**：`txClientKey{}`（Ent 客户端）+ `rawTxKey{}`（Raw SQL execer），确保 Ent 和 Raw SQL 在同一事务中

**Biz 层事务接口**：
- `AgentTxRepo.ExecInTx` — Agent 原子创建/更新
- `CompressRepo.CompressSessionInTx` — Session 压缩（含版本 CAS 守卫）
- `TxProvider.ExecInTx` — Pack 导入原子操作

### Schema 管理

- **76 个 Ent Schema**，73 个使用 `entsql.Annotation{Table: ...}` 显式映射表名
- **仅 Eval 域使用 Ent Edge**（4 个 Schema 有 8 条边），其余全部使用手动 FK 字段
- **FTS5 全文搜索**（`messages_fts`）和 **pgvector 向量搜索**（`vector_embeddings`）不在 Ent Schema 中，通过 DDL 迁移管理
- **Ent 生成命令**：`go generate ./internal/data/ent`（启用 `sql/execquery` + `sql/upsert` 特性）

### 三层迁移体系

| 层级 | 机制 | 范围 |
|------|------|------|
| L1: Ent Auto-Migration | `Schema.Create()` | 核心表结构（Ent Schema 定义的所有表） |
| L2: DDL Migration Registry | `ddl_migration_registry.go` + `sql/migrations/*.sql` | FTS5、索引、列补丁等 Ent 不支持的特性（28 个版本化迁移） |
| L3: Data Migration | `runPendingDataMigrations` | 一次性数据转换（TRPC 记忆回填、turn_index 迁移等） |

**迁移门控**：所有迁移通过 `schema_migrations` 表去重，已应用的跳过。

**启动就绪门**：`ReadinessGate` 确保 P1 步骤（DDL 迁移 + Postgres Schema + 数据迁移）完成后才接受流量。

### 错误翻译

**唯一出口**：`entErrToBizErr(err, domain)` — 所有 Repo 方法的数据库错误必须经过此函数。

| 输入 | 输出 Code |
|------|----------|
| `nil` | `nil` |
| 已是 `*apierror.Error` | 透传 |
| `ent.IsNotFound` / `sql.ErrNoRows` | `CodeNotFound` |
| `ent.IsConstraintError` / `shared.ErrMessageDuplicate` / `shared.ErrAgentKeyConflict` | `CodeConflict` |
| `ent.IsNotLoaded` | `CodeBadRequest` |
| 其他 | `CodeInternal` |

### 类型转换

- **方向**：Ent → Biz（单向），无反向转换函数
- **命名**：`entXxxToBiz` / `entToBizXxx`
- **写路径**：直接从 Biz 模型字段构造 Ent create/update builder

### 已知技术债务（审查发现）

| 编号 | 问题 | 位置 | 严重度 |
|------|------|------|--------|
| DB-DEBT-01 | `AgentRuntimeSetting` Schema 约 140 个字段，严重超标 | `ent/schema/agent_runtime_setting.go` | 高 |
| DB-DEBT-02 | 8 个窄接口方法数 >5：`AnalyticsRepo`(10)、`QuotaRepo`(8)、`SystemSettingRepo`(11)、`plugin.Repo`(9)、`RemoteAgentRepo`(7)、`TeamReader`(6)、`TeamRunWriter`(6)、`TraceRepo`(6) | 各 biz 接口文件 | 中 |
| DB-DEBT-03 | 部分窄接口缺少 Stability 标注（`AnalyticsRepo`、`QuotaRepo`、`plugin.Repo`、`SystemSettingRepo`、`TraceRepo`） | 各 biz 接口文件 | 低 |
| DB-DEBT-04 | Data 层 `fmt.Errorf` 使用不一致：`evolution_suggestion_repo.go`、`background_job.go` 等对 Ent 错误使用 `fmt.Errorf` 而非 `entErrToBizErr` | `internal/data/` | 中 |
| DB-DEBT-05 | 多个复合 Repo 接口方法数远超 5（`SessionRepo`~40+、`skill.Repo`23、`tool.ToolRepo`18、`usage.Repo`22），已标记 Deprecated/TECH-DEBT 但仍用于 Wire 绑定 | 各 biz 接口文件 | 已知 |

### 数据库开发规则（红线 + 注意事项）

#### 红线（强制）

| # | 规则 | 说明 |
|---|------|------|
| DB-R1 | **禁止在 `NewData` 外另开 SQLite 连接** | 仅通过 `d.RW()`/`d.RWDB()` 访问，对应红线 #10 |
| DB-R2 | **禁止修改 Ent 生成代码** | 改 Schema → `go generate` → 提交生成物，对应红线 #11 |
| DB-R3 | **禁止野生表** | 所有表必须进 Ent Schema，FTS5/pgvector 等通过 DDL 迁移补充但不另建表 |
| DB-R4 | **禁止散落 `*_patch.go` 迁移** | 所有 Schema 变更纳入 DDL Migration Registry，有版本号、有依赖顺序 |
| DB-R5 | **禁止 Repo 方法直接返回 Ent 错误** | 所有错误必须经 `entErrToBizErr(err, domain)` 翻译 |
| DB-R6 | **禁止使用已废弃的连接访问器** | `RawDB()`/`ReadDB()`/`ReadEnt()`/`ReadClient()` 已废弃，使用 `RW()`/`RWDB()` |

#### 注意事项（建议）

| # | 规则 | 说明 |
|---|------|------|
| DB-N1 | **新增 Repo 优先使用 Ent API** | 仅在 Ent 无法覆盖时（FTS5/pgvector/复杂 SQL）使用 Raw SQL |
| DB-N2 | **Raw SQL 必须走事务感知路径** | 读用 `RW().Read(ctx).QueryContext` 或 `RWDB().ReadDB(ctx)`，写用 `RW().Write(ctx).ExecContext` 或 `RWDB().WriteDB(ctx)` |
| DB-N3 | **Repo 接口方法 ≤ 5** | 超过按读写职责拆分为 `XxxReader`/`XxxWriter`，复合接口仅用于 Wire 绑定 |
| DB-N4 | **新增 Schema 必须加 `entsql.Annotation{Table: ...}`** | 避免依赖 Ent 默认复数化规则，保持表名可控 |
| DB-N5 | **Schema 中禁止 import `pkg/trpc-agent-go`** | Schema 属于 data 层，不得依赖框架运行时 |
| DB-N6 | **DDL 迁移 SQL 必须幂等** | 使用 `IF NOT EXISTS` / `IF NOT NULL`，"duplicate column" 和 "already exists" 错误视为成功 |
| DB-N7 | **事务内操作必须从 ctx 获取连接** | 使用 `EntClientFromCtx` / `TxExecerFromCtx`，确保参与同一事务 |
| DB-N8 | **敏感字段必须标记 `.Sensitive()`** | 如 `credential_encryption_key`、`api_key` 等，防止日志泄漏 |
| DB-N9 | **JSON 字段优先使用 `field.JSON()`** | 避免手动 `json.Marshal`/`Unmarshal`，利用 Ent 的类型安全 JSON 支持 |
| DB-N10 | **新增迁移必须在 `ddl_migration_registry.go` 注册** | 包含版本号（YYYYMMDD 格式）+ 名称 + SQL 路径或 Go 函数 |

---

## 三、验证命令

| 改动类型 | 最小验证 |
|----------|----------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz / Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 前端 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前（全量）** | 后端：`make api && make wire && make build && make test && make lint`；前端：`cd web && pnpm lint && pnpm test && pnpm build` |

---

## 四、代码审查纪律

- 代码审查**必须使用项目 SKILL**（`aranea-review` / `go-oop-review`），不可仅依赖内置通用审查
- 通用审查（如 `TRAE-code-review`）只能作为补充，项目红线和业务规则检查以 SKILL 为准
- **维度审查**：按变更范围动态加载审查维度，详见 `docs/review-dimension-checklists.md`
  - 所有变更：维度 1（架构）、2（质量）、3（正确性）、8（错误处理）
  - 涉及 DB：+ 维度 4（性能）
  - 涉及外部输入/API：+ 维度 5（安全）
  - 涉及 Usecase：+ 维度 6（可测试性）、11（业务逻辑）、14（业务逻辑正确性：状态机/事件/不变量）
  - 涉及状态变更：+ 维度 14（业务逻辑正确性：状态机审查）
  - 涉及事件：+ 维度 14（业务逻辑正确性：事件可靠性审查）
  - 涉及跨模块：+ 维度 7（可维护性）、12（文档同步）
  - 涉及测试代码：+ 维度 15（测试审查：分层/Mock/覆盖率）

---

## 五、任务执行纪律

- 有任务 ID 时：只读对应 development.md / blueprint 中该 ID 块
- 列假设 → 编码 → 分级验证 → 通过后再扩 scope
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块
- 实现前显式声明假设，困惑时停下来问，不静默选择（Karpathy: Think Before Coding）
- 最小代码解决问题：不添加未请求的功能/抽象/灵活性/可配置性（Karpathy: Simplicity First）
- 每行改动必须可追溯到用户请求；发现无关死代码只提不删（Karpathy: Surgical Changes）

### 5.1 开发流程纪律

**新变更推荐流程**（非强制工具链，但纪律必须遵守）：

```
需求探索 → 规格设计 → TDD 实施 → 验证归档
```

**阶段门控（红线）**：
1. **需求/设计阶段禁止写代码** — 只能读文件、搜索、讨论、写规格文档
2. **实施阶段必须 TDD** — 先写失败测试，再写最小实现
3. **完成阶段必须验证** — 全量测试 + build + lint 通过才能声明完成
4. **需求变更必须回退到设计** — 禁止在实施阶段直接改代码适应新需求

**实施纪律**（强制）：
1. **TDD 铁律**：无失败测试不写生产代码。先写代码后补测试 = 删掉重来
2. **两阶段审查**：规格合规审查优先，代码质量审查其次
3. **验证前置**：无新鲜验证证据不做完成声明。证据先于断言，永远
4. **YAGNI**：不添加未请求的功能，不过度工程

> 可选的 Superpowers 辅助 SKILL（按需使用）：`brainstorming`、`writing-plans`、`subagent-driven-development`、`test-driven-development`、`verification-before-completion`、`systematic-debugging`、`executing-plans`、`requesting-code-review`、`receiving-code-review`、`finishing-a-development-branch`。

---

## 六、docs 目录规范

> **操作 docs/ 目录下的任何文件前，必须先读 `aranea-docs-guide` SKILL。**

| 目录 | 用途 | 命名规范 |
|------|------|----------|
| `docs/development/` | 模块开发文档 | `<N>-<name>.md` / `.design.md` / `.development.md` |
| `docs/testing/` | 测试文档 | 见各子目录 README.md |
| `docs/scenarios/` | 专业场景文档 | 每场景一个子目录，kebab-case |
| `docs/reports/` | 调研报告 | `YYYY-MM-DD-<type>-<topic>.md` |
| `docs/notes/` | 个人笔记 | 用户自维护，AI 不主动修改 |

**关键红线**：
1. 同一模块的子功能文档必须合并到主文档，禁止创建独立子文档文件
2. development 文档后缀必须用点号分隔（`.development.md`），禁止连字符（`-development.md`）
3. 禁止文件名中使用空格
4. 禁止同一编号下放置不同主题的模块

### 三件套内容边界（强制）

每个模块的三件套必须严格按以下边界组织内容，禁止跨类混写：

| 文档类型 | 后缀 | 允许的内容 | 禁止的内容 |
|---------|------|-----------|-----------|
| 需求文档 | `.md` | 用户故事、功能需求清单、验收标准、非功能需求、交互规格（用户视角） | 代码分层、文件结构、Proto 定义、数据模型、API 实现细节、开发进度/状态 |
| 设计文档 | `.design.md` | 架构设计、代码分层、Proto/API 契约、数据模型、接口定义、技术选型、状态机、序列图、前端组件设计、UX 规范 | 用户故事、功能需求清单、开发进度/任务清单/状态标记 |
| 开发计划 | `.development.md` | 模块定位、代码锚点、现状评估、差距与优化、Phase 划分、任务清单（含状态）、验收标准、改动文件清单 | 用户故事、功能需求、架构设计、Proto/API 契约 |

**迁移规则**：整理时发现内容错位，必须将其迁移到对应类型的文档中，而非删除。迁移后在原位置保留一行指引：`> 详见 [xxx.design.md §N](./xxx.design.md#n-标题)`。

### 文档同步纪律（红线）

**核心原则**：代码改动必须在对应的 `docs/development/` 文档中同步更新，文档与代码偏差视为技术债务。

| # | 规则 | 说明 |
|---|------|------|
| DOC-SYNC-1 | **代码改动必须同步文档** | 任何影响模块行为/接口/数据结构的代码改动，必须在同一 PR/commit 中更新对应模块的三件套文档 |
| DOC-SYNC-2 | **需求文档只列功能需求** | `.md` 文件只包含用户故事、功能需求、验收标准；架构/协议/实现细节迁移到 `.design.md` |
| DOC-SYNC-3 | **设计文档体现设计与实现** | `.design.md` 文件记录架构设计、API 契约、数据模型、技术选型；不含需求清单和开发进度 |
| DOC-SYNC-4 | **开发计划记录进度** | `.development.md` 文件记录代码锚点、现状评估、任务清单与状态、验收标准；不含需求和架构设计 |
| DOC-SYNC-5 | **状态标记必须与代码一致** | 文档中的 ✅/⏳/🟡/📋 状态标记必须反映代码真实状态；已完成的待办项必须标记 ✅，未实现的已声明功能必须标记 ⏳ |
| DOC-SYNC-6 | **代码锚点必须有效** | `.development.md` 中的「代码锚点」引用的文件路径必须真实存在；删除/重命名代码文件时必须同步更新引用 |
| DOC-SYNC-7 | **API 端点必须与 Proto 一致** | 设计文档中的 API 端点表必须与 `api/kratos/` 下的 Proto 定义一致；新增/删除 RPC 必须同步更新 |
| DOC-SYNC-8 | **数据表必须与 Schema 一致** | 设计文档中的数据表结构必须与 `internal/data/ent/schema/` 一致；Schema 变更必须同步更新设计文档 |

**触发条件**（必须同步文档的代码改动）：
- 新增/删除/重命名 RPC 方法或 HTTP 端点
- 新增/删除/修改 Ent Schema 或数据表字段
- 修改模块的核心数据流或架构分层
- 新增/删除/重命名核心文件（service/biz/data/agent 层）
- 完成开发计划中的待办任务（更新状态标记）
- 新增模块功能（在需求文档追加需求 + 设计文档追加设计 + 开发计划追加任务）

**豁免条件**（无需同步文档）：
- 纯 bug 修复（不改变接口和行为契约）
- 重构（不改变外部行为，如提取函数、重命名局部变量）
- 测试代码变更
- 配置文件调整（如调参）

**审查检查点**：代码审查时，维度 12（文档同步）必须检查上述 8 条规则。

---

### AS-ADR-01：架构决策记录

**要求**：每个影响跨模块的架构决策必须记录 ADR。

**触发条件**：新增模块/包、修改依赖方向、引入新框架/库、修改核心数据结构、性能关键路径的权衡决策。

**格式**：
```markdown
# ADR-NN: <标题>
## 状态：提议 | 已接受 | 已废弃 | 已替代
## 背景：<为什么需要做决策>
## 决策：<做了什么决策>
## 后果：<正面和负面影响>
## 替代方案：<考虑过但未选择的方案及原因>
```

**存放**：`docs/reports/YYYY-MM-DD-review-adr-<topic>.md`

### AS-COG-01：认知复杂度量化

**要求**：以下指标不得超过上限，超标必须拆分并标记 `// TECH-DEBT(COG): <指标>=<当前值>, 上限=<上限>`。

| 指标 | 上限 | 检测方式 |
|------|------|---------|
| struct 注入字段数 | 15 | 代码审查 |
| 单方法行数 | 80 | linter（CS-B5） |
| 单方法圈复杂度 | 15 | linter（CS-B6） |
| biz 层依赖数（单 struct） | 8 | 代码审查 |
| sync.Map 数（单 struct） | 1（2+ 必须提取为子管理器；惰性初始化场景优先用 sync.OnceValue） | 代码审查 |
| 文件总行数 | 500 | linter |
| 包级导出类型数 | 20 | 代码审查 |

**超标处理**：标记 TECH-DEBT → 下一迭代安排拆分 → 不阻断当前开发但禁止继续堆叠。

### AS-FSM-01：状态机显式化要求

**要求**：任何实体拥有 >3 种状态时，必须定义显式状态机。

**定义位置**：与实体同包，文件名 `*_state_machine.go`

**必须包含**：状态枚举（const）+ 合法转换表（var transitions）+ 转换校验函数（`Transition(from, event) (to, error)`）+ 可选守卫条件（`Guard func(ctx) bool`）。

**现有实体需补全**：Run（5 种状态）、Session（已有，需统一接口）、TeamRun（6 种状态）、GraphExecution（5 种状态）。

### AS-STA-01：接口稳定性分级

**要求**：biz 层 port 接口必须标注稳定性等级。

| 等级 | 标注 | 含义 | 变更规则 |
|------|------|------|---------|
| Stable | `// Stability:stable` | 生产依赖，不可破坏兼容 | 只能新增方法，不能修改/删除 |
| Evolving | `// Stability:evolving` | 活跃开发中，可能变 | 可修改，但需 ADR 记录 |
| Internal | `// Stability:internal` | 包内使用，不对外 | 自由变更 |

**检查方式**：代码审查时校验 `Stable` 接口变更是否有 ADR。

### AS-FIT-01：架构 Fitness Function

**要求**：以下架构不变量必须通过自动化测试验证。

| Fitness Function | 验证内容 | 实现方式 |
|-----------------|---------|---------|
| 依赖方向 | biz 不依赖 pkg/trpc-agent-go | `go vet` + 自定义 linter |
| 分层隔离 | service 不直接访问 data | import 检查脚本 |
| 接口窄化 | biz port 接口方法 ≤ 5 | 静态分析 |
| 状态机覆盖 | >3 状态实体有显式状态机 | 测试枚举 |
| 认知复杂度 | struct 字段 ≤ 15 | 静态分析 |

**实现路径**：短期 `make archlint` → 中期集成 CI → 长期 `golangci-lint` 自定义规则。

### AS-EVT-01：事件可靠性分级

**要求**：事件按业务关键性分级，不同级别有不同的可靠性保证。

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| Critical | ToolResult / Error / RunnerCompletion / Checkpoint | WBPF（先写后发）+ 重试 | SQLite WAL |
| Important | StateDelta / TokenUsage / RunStatus / SessionStatusChanged / GraphNodeEnd / TeamRunFinished | BlockUpTo + 异步持久化 | SQLite EventStore |
| Informational | TextDelta / FlowLog / Log / MemberDelta | 尽力而为 | 不持久化 |

**检测方式**：代码审查时校验新增事件类型的分级是否正确。

---

## 九、模块关联强制读取（违反即停）

> **任何模块开发前必须先读关联文档。** 模块不是孤岛，改一处必知影响面。

| 文档 | 路径 | 定位 |
|------|------|------|
| **模块交叉参考** | `docs/development/65-module-cross-reference-full.md` | "改模块 X 时必须注意谁"（动态关联、影响面） |
| **系统架构总览** | `docs/development/0-system-diagram.md` | "每个模块是什么"（静态结构、全貌） |
| **数据库架构** | `docs/development/66-database-architecture.md` | 数据库设计与访问模式 |

**开发任何模块时**：
1. 定位目标模块 → 读系统架构总览对应章节
2. 读交叉参考手册 → 找到目标模块卡片
3. 查变更影响表 → 确定需要同步修改的文件清单
4. 按依赖方向逐层修改 → 验证时覆盖所有影响面
