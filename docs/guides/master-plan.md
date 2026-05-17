# Aranea-Agents 项目开发整体规划文档

> **⚠️ 本文档自 2026-05-17 起停止维护**。进度真相与下一步规划以 [`docs/guides/execution-plan.md`](execution-plan.md) 为唯一权威基线；本文 §4 状态表、§6 优先级清单、§8 落地分类、§10 风险、§12 配套文档索引 与代码实际现状已脱钩，仅作历史参考。新增任务请直接在 `execution-plan.md` §3 / 附录 A 登记，不要再扩展本文。
>
> ---
>
> 版本：v1.1（2026-05-17，已冻结）
> 编制依据：`docs/README.md`、`docs/guides/AI-DEVELOPMENT-SPECIFICATION.md`、`docs/guides/plan.md`、`docs/guides/trpc-agent-go-framework.md` 以及 `cmd/`、`internal/`、`api/`、`pkg/`、`web/` 全量源码与配置。
> 文档定位（已失效）：曾作为后续 N 个版本迭代的执行依据。
>
> 伴生文档（同步冻结）：
> - 实施方案总览：[docs/guides/implementation-plan.md](implementation-plan.md)（已废弃）
> - 任务追踪表：[docs/guides/task-tracker.md](task-tracker.md)（已废弃）
> - Sprint 详细计划：[S1](sprints/S1-p0-redlines.md) · [S2](sprints/S2-architecture-debt.md) · [S3](sprints/S3-observability.md) · [S4](sprints/S4-plugin-skill-planner.md) · [S5](sprints/S5-artifact-cron-tests.md) · [S6](sprints/S6-knowledge-eval-a2a.md)（已废弃）

---

## 0. 阅读路径

- §1 整体架构梳理 —— 现状全景
- §2 现存问题清单 —— 文档/代码不一致 + 缺陷 + 红线违反
- §3 优化整改方案 —— 架构/业务/工程/扩展四维
- §4 功能补全计划 —— M1~M20 + 前后端缺口
- §5 代码重构方向 —— 模块级具体动作
- §6 开发优先级划分 —— P0~P3 + 模块迭代顺序
- §7 编码规范统一标准 —— 红线 + 决策树 + 落地清单
- §8 落地分类索引 —— ① 可直接落地重构 / ② 新增开发 / ③ 仅调整优化
- §9 验收标准
- §10 风险与缓解
- §11 参考索引
- §12 配套文档索引（执行入口）

> **快速入口**：本文是全局规划基准（只读），落地执行请直接使用 [implementation-plan.md](implementation-plan.md) + [task-tracker.md](task-tracker.md)。

---

## 1. 整体架构梳理

### 1.1 顶层定位（与 `docs/README.md` 一致）

- **运行时内核**：`pkg/trpc-agent-go/`（vendored 第三方框架），提供 `Agent / Runner / Session / Memory / Tool / Event / Skill / Graph / Team / Planner / Plugin / Artifact / CodeExecutor / Knowledge / Evaluation` 全套能力。
- **业务外壳**：基于 Kratos v2 的分层架构 `api → service → biz → data`，承载租户、运行时配置、可观测、审计、前端 BFF。
- **进程入口**：`cmd/admin/main.go` 启动 Kratos `app.Run()`，附带 `event.Bus`、`internal/cron/runner`、`internal/skill/watch/runner` 三个 goroutine 级常驻组件。
- **依赖注入**：Wire 集中在 `cmd/admin/wire_gen.go`，通过 `internal/data.ProviderSet`、`internal/biz.ProviderSet`、`internal/service.ProviderSet`、`internal/server.ProviderSet` 编排。
- **前端**：Vue 3 + Quasar + Pinia + TS，proto 生成的客户端在 `web/src/services/kratos/<pkg>/v1/`。

### 1.2 后端目录骨架（按职责）

| 目录 | 当前职责 | 备注 |
|------|----------|------|
| `cmd/admin/` | 入口、Wire | `main.go` 还混入了 `event.Bus`、cron、skill watcher 启动逻辑 |
| `api/kratos/` | proto 与生成产物 | 23 个服务（admin, agent, chat, channel, cron, event, graph, mcp, memory, monitor, plugin, provider, session, settings, skill, team, tool, tool-call, usage, workspace, user, system 等） |
| `internal/server/` | HTTP/gRPC 服务器装配 | `server.go`、`grpc.go`、`http.go`、`ws.go`（违反红线 6/12） |
| `internal/service/` | RPC 实现 + ChatService 编排 | `chat.go`/`chat_native.go` 双轨制；`session.go`、`agent.go`、`team.go` 等 |
| `internal/biz/` | 业务用例 + 仓储接口 | 33 个文件；`biz.go` 与 `graph.go` 违反红线 2/8 |
| `internal/data/` | Ent 仓储实现 + DB 连接 | `data.go` 是唯一允许 `sql.Open` 的位置；目前还有两处旁路 |
| `internal/agent/` | trpc Runner / Agent 构建 | `trpc_build.go`、`trpc_runtime.go`、`trpc_options.go`、`tool_filter.go` |
| `internal/session/trpc/` | trpc SessionService 适配 | `sqlite.go` 内部再次 `sql.Open`（红线 10） |
| `internal/memory/trpc/` | trpc MemoryService 适配 | `sqlite_adapter.go` 用进程内 cache，存在逻辑漏洞 |
| `internal/graph/trpc/` | trpc StateGraph + Checkpoint | `builder.go`、`validator.go`、`checkpoint.go`（再次 `sql.Open`） |
| `internal/team/` | Team Runner 适配 | `runner_team_trpc.go` 直连 trpc |
| `internal/tools/` | Tool 注册中心 + 适配 | `registry/`、`trpc/`、`skillrouter/`、`mcpmount/` |
| `internal/event/` | EventBus + Envelope | 通用事件总线，订阅广播（buf=128，覆盖式丢弃） |
| `internal/runtimedeps/` | TurnDeps 聚合体 | 一个上帝对象，14+ 字段 |
| `internal/cron/`、`internal/skill/watch/` | 后台任务 | 独立 runner |
| `pkg/` | trpc-agent-go 源码 + 通用工具 | `trpc-agent-go/` 大量子包；`pkg/jsonutil`、`pkg/strutil` 等 |

### 1.3 前端目录骨架

- `web/src/features/{19 个业务域}`：admin / agents / avatar / channels / chat / cron / graph / heartbeat / mcp / memory / monitor / platform / plugins / session / skills / system-settings / teams / tools / usage。
- `web/src/stores/{agents,avatar}`：仅 2 个 Pinia store；其余 17 个业务域绕开 store，直接在 composable / 页面里调 `kratosApi`。
- `web/src/services/`：`axiosHandler.ts`、`index.ts`、`kratos/{chat,session,...}/v1/index.ts`（proto 生成产物，部分缺失）。

### 1.4 运行时数据流（实际）

```
WebSocket /v1/ws ──┐
                   ├─► EventBus（订阅 sessionID/runID）
HTTP /v1/chat ─────┤
                   ├─► ChatService.SendMessage
                   │     ├─► biz.SessionUsecase（持久化）
                   │     ├─► RunTRPCUserTurn（构造 LLMAgent + Runner）
                   │     │     ├─► tools/trpc 注册器
                   │     │     ├─► session/trpc.SQLiteSessionService
                   │     │     └─► memory/trpc.SQLiteMemoryService（cache 失效）
                   │     └─► EventBus.Publish（envelope）
gRPC :9000 ────────┘
```

### 1.5 数据存储

- **SQLite（默认）**：Ent ORM，schema 位于 `internal/data/ent/schema/`，已覆盖 agents/sessions/messages/runs/tool_calls/skills/plugins/usage 等 60+ 表。
- **PostgreSQL（可选）**：`data.go:newPostgresPool` 用 `database/sql` 直接连接，仅作为 pgvector / 知识库后端使用。
- **Redis**：`data.go:newRedisClient`，仅作 KV / 心跳。

---

## 2. 现存问题清单

> 标记规则：`[D-x]` 文档/代码不一致；`[M-x]` 漏实现；`[R-x]` 冗余；`[B-x]` 逻辑漏洞；`[Q-x]` 代码质量缺陷；`[V-x]` 红线违反。

### 2.1 文档与代码不一致（高优先级）

| 编号 | 位置（文档） | 位置（代码） | 不一致内容 |
|------|--------------|--------------|------------|
| D-1 | `plan.md` §3.2 "MCP 集成仍使用 ADK 而非 trpc，P2 优先级迁移" | `internal/tools/mcpmount/append.go` 已 `import trpc.group/.../tool/mcp`，仓内 `import "google.golang.org/adk"` 0 处 | MCP 已完成迁移，文档未更新 |
| D-2 | `plan.md` §3.3 "Runner 的 SessionService 始终使用 inmemory" | `internal/data/data.go:NewTRPCSessionService` 通过 wire 注入 `runtimedeps.Runtime.TRPCSession`，`service/trpc_turn.go` 使用 `WithSessionService(persistent)` | 已支持 SQLite session 持久化 |
| D-3 | `plan.md` M4 "ValidateGraph ❌P0待实现" | `internal/graph/trpc/validator.go` 完整实现入度/出度/循环检测 | 已实现 |
| D-4 | `plan.md` M4 "Checkpoint SQLite 持久化 ⚠️待完善" | `internal/graph/trpc/checkpoint.go` + wire 注入 | 已实现（但实现违反红线 10） |
| D-5 | `plan.md` §3.4 / README §13 提及 `server/sse.go` 流式 | 实际只有 `internal/server/ws.go`（WebSocket）+ `features/chat/api.ts` 注释明确"不再使用 SSE" | SSE 已被 WebSocket 替代 |
| D-6 | README §6 Memory L0/L1/L2/L3/L4 多层结构 | `memory/trpc/sqlite_adapter.go` 仅一套，`data/sessionmemory/store.go` 提供 L0~L4 行类型 | 多层"读写策略"未实现 |
| D-7 | `AI-DEVELOPMENT-SPECIFICATION.md` §3.6 引用 `internal/agent/adksvc.BizSessionService` | 仓内无 `adksvc` 包 | 文档遗留 ADK 痕迹 |
| D-8 | README §3 / §13 多次提及"ADK"作为对比框架 | 仓内运行时已无 ADK 引用 | 文档历史信息 |
| D-9 | `plan.md` 模块状态表标注的 ✅ 数量 | 实际 ✅ 数量更多（M3/M4/M6/M7/M19 都需上调） | 状态表需重写 |
| D-10 | `web/src/services/index.ts` 引用 `./kratos/graph/v1/index` | `web/src/services/kratos/graph/` 目录不存在 | 前端 graph 客户端未生成 |

### 2.2 漏实现功能

| 编号 | 模块 | 描述 |
|------|------|------|
| M-1 | M6 Memory | `EnqueueAutoMemoryJob` 是 `return nil` 占位（`biz/session_usecase.go`），未对接 `memory/extraction` |
| M-2 | M6 Memory | Memory 工具（add/update/load/search/delete）未注册到 trpc Runner，trpc-agent-go `memory/tool/` 能力闲置 |
| M-3 | M9 Skill | `internal/skill/watch` 仅文件 watcher，没有 trpc `skill/repository` 接口 |
| M-4 | M10 Plugin | `biz/plugin.go` 仅 CRUD；trpc `plugin/PluginManager`、`PluginContext`、Callback Points 全部未对接 |
| M-5 | M11 Planner | 仅识别 `dialogMode==plan` 走 BuiltinPlanner，`react/`、`a2ui/` 子包未集成 |
| M-6 | M12 Artifact | `artifact/` 完全未集成；上传/下载/版本/MIME 都缺失 |
| M-7 | M13 Knowledge | 完全未实现；pgvector schema 准备好但无业务接口 |
| M-8 | M14 CodeExecutor | 仅 `code_executor/local`，沙箱与容器未集成 |
| M-9 | M15 A2A | 未实现 |
| M-10 | M16 Gateway | `pkg/trpc-agent-go/server/gateway/` 的 ManagedRunner / SteerableRunner / RunStatus 路由未暴露；`chat.proto` 无 `GetRunStatus` |
| M-11 | M17 Evaluation | 未实现 |
| M-12 | M19 Callback | 仅 `agent/trpc_callbacks.go` 中的 ToolCallbacks；Agent/Model 级 Callback、PluginManager Callback 未挂接 |
| M-13 | 前端 Graph | `services/kratos/graph/v1/index.ts` 缺失，TS 编译会失败 |
| M-14 | 前端 Chat | `features/chat/api.ts` 没有使用生成的 `createChatService()`，直接 `kratosApi.post("/v1/chat/messages", ...)` |
| M-15 | 前端 Store | 19 个业务域，只有 2 个 Pinia store，违反 README §7.1 数据流约定 |
| M-16 | 后端审计 | `cmd/admin/main.go` 注册 cron + skill watcher，缺少健康探针 RPC 和优雅退出 hook |
| M-17 | 测试 | 仅 14 个 `*_test.go`，集中在工具/团队边缘，service/biz/data 几乎无测试 |
| M-18 | CI | `scripts/check-runtime-boundary.ps1` 未挂到 CI；`Makefile` 无 `lint`/`vet` 默认目标 |

### 2.3 冗余代码

| 编号 | 文件 | 问题 |
|------|------|------|
| R-1 | `internal/tools/mcpmount/append.go` | `AppendEffectiveMCPServerToolsets` 全仓 0 处调用 |
| R-2 | `internal/agent/trpc_runtime.go:NewInMemoryTRPCSessionService` 与 `internal/session/trpc/sqlite.go:NewInMemorySessionService` | 同功能两处导出 |
| R-3 | `internal/data/agent_catalog_legacy.go` | 文件名带 legacy，但仍是当前 catalog 默认值实现，命名误导 |
| R-4 | `internal/service/chat.go` + `chat_native.go` | 同一 ChatService 拆成两份，字段/编排重复 |
| R-5 | `internal/biz/errros.go` | 文件名拼写错误（应为 `errors.go`） |
| R-6 | `internal/data/sessionmemory/entity_adk.go` | 仍以 `_adk` 后缀命名，与项目"去 ADK"立场冲突 |
| R-7 | `internal/service/trpc_turn.go` 与 `internal/team/runner_team_trpc.go` | Envelope 构造 / Usage 累计逻辑大段复制 |
| R-8 | `internal/service/agent.go` `fromProtoRuntime`/`toProtoRuntime` | 220+ 行手写双向映射，未抽工具 |
| R-9 | `internal/tools/toolset.go:toolFilterForPrefix` 与 `mcpmount` 内同名函数 | 重复实现，未提取公共包 |
| R-10 | `internal/biz/envelope.go` | 整文件只做 trpc envelope 的 re-export，violates 红线 2（间接绑定框架）；应让 biz 不感知 envelope |

### 2.4 逻辑漏洞

| 编号 | 文件 | 问题 |
|------|------|------|
| B-1 | `internal/memory/trpc/sqlite_adapter.go` | `ReadMemories` 仅读进程内 `s.cache`；`service/trpc_turn.go:NewSQLiteMemoryService` 每个 turn 重新构造，cache 必然为空 → Memory tool ReadMemories 永远返回空 |
| B-2 | `internal/server/ws.go` | 自行启动 `http.Server` 监听 `:8002`，`mux.HandleFunc("/v1/ws", ...)`；违反红线 6 + 红线 12，且绕开 Kratos middleware（日志、追踪、鉴权） |
| B-3 | `internal/session/trpc/sqlite.go:NewSQLiteSessionService` + `internal/graph/trpc/checkpoint.go:NewSQLiteCheckpointSaver` | 在 `data.NewData` 之外再次 `sql.Open("sqlite3", dsn)`，形成多池 → 与 Ent 写锁竞争（SQLite 单写者），违反红线 10 |
| B-4 | `internal/biz/graph.go` | 进程内 `executions map[string]*runtimeExec` 无 TTL/GC，长跑后内存泄漏；`ResumeExecution` 依赖该 map，重启即不可恢复 |
| B-5 | `internal/graph/trpc/builder.go:BuildStateGraphWithRegistry` | 直接修改入参切片元素的 `Func` 字段，多并发 build 同一 GraphDefinition 时 race |
| B-6 | `internal/service/chat.go:dequeuePending` | 高并发下 `CompareAndDelete` 与 `CompareAndSwap` 重试在某些路径可能丢消息（用户提交瞬间被另一线程拿走） |
| B-7 | `internal/service/chat_native.go` | 当 `enqueuePending` 因队列满返回错误，用户消息已经写库但 run 不会启动，造成"已发送未应答"幽灵消息 |
| B-8 | `internal/event/bus.go` | 订阅者 buffer 满时丢弃**旧事件**（FIFO 覆盖），高频日志会顶掉关键 `tool_result` / `error` 事件 |
| B-9 | `internal/service/trpc_turn.go:recordToolInvocationAsync` | `go func()` 内无 panic recover，单次失败可能 crash 整个进程 |
| B-10 | `internal/biz/graph.go:consumeEvents` | 同上，`go func()` 无 recover |
| B-11 | `internal/agent/trpc_build.go:loadEffectiveToolKeys` | 使用 `context.Background()` 而非 turn ctx，吞掉取消信号 |
| B-12 | `internal/biz/session_usecase.go` | `EnqueueAutoMemoryJob` 占位 → 自动记忆永远不生成 |
| B-13 | `internal/runtimedeps/deps.go:TurnDeps` | 当 `TRPCSession==nil` 时静默退化为 InMemory，部分 session 持久化失败而无告警 |
| B-14 | `internal/cron/schedule.go` | 任务执行失败仅 log，无重试/失败计数；存在"静默丢任务"风险（需结合 spec 评估） |
| B-15 | `web/src/services/index.ts` | `import { createGraphServiceClient } from './kratos/graph/v1/index'` 指向空目录，`pnpm build` 失败 |

### 2.5 代码质量缺陷

#### 2.5.1 性能

| 编号 | 位置 | 问题 |
|------|------|------|
| Q-1 | `internal/service/trpc_turn.go` | 每次 chat turn 重新执行 `BuildTRPCLLMAgent`（解析 tools、skills、MCP、callbacks），无任何缓存。建议按 agent_id+config_hash 缓存 |
| Q-2 | `internal/biz/graph.go` | `executions` map 无清理，长期内存增长 |
| Q-3 | `internal/event/bus.go` | 覆盖式丢弃 + per-subscriber goroutine（无 backpressure），高频写入会持续 GC |
| Q-4 | `internal/data/agents.go`、`sessions.go` 等 | 大对象（agent runtime config）单条查询时反复 JSON 反序列化，可在 biz 缓存 |
| Q-5 | WSServer | 没有写心跳速率限制 / 没有 max in-flight |

#### 2.5.2 耦合

| 编号 | 位置 | 问题 |
|------|------|------|
| Q-6 | `internal/biz/biz.go` | `import "aranea-agents/internal/graph/trpc"` → biz 层间接绑定 trpc-agent-go，违反红线 2 |
| Q-7 | `internal/biz/graph.go` | 直接 import `trpc.group/.../graph`、`event`、`model`、`tool`：红线 2 + 红线 8 双违反 |
| Q-8 | `internal/runtimedeps/TurnDeps` | 上帝对象（14 个字段），所有 service/team/chat 都依赖 |
| Q-9 | `internal/data/data.go` | 同时是 Wire root + 多个 trpc 适配器构造点，职责过重 |
| Q-10 | `internal/service/chat.go` | 嵌入 `td runtimedeps.TurnDeps` 字段 + `deps ChatServiceDeps` 参数，参数双轨 |

#### 2.5.3 规范不统一

| 编号 | 位置 | 问题 |
|------|------|------|
| Q-11 | 错误模型 | `biz/graph.go` 用 `ErrNotFound` 全局变量；`data/graph.go` 用 `kerrors.NotFound("GRAPH", ...)`；`session/trpc/sqlite.go` 用 `fmt.Errorf` |
| Q-12 | 命名 | `MCPCallCount` vs proto `McpCallCount`，缺统一命名约定文档 |
| Q-13 | 字符串枚举 | `RiskLevel` / `Confirmation` / `Status` 均为 raw string，无 `package types` 常量 |
| Q-14 | 文件名 | `errros.go`、`agent_catalog_legacy.go`、`entity_adk.go` 三个误导命名 |
| Q-15 | 注释 | 大部分 biz 结构体字段无 godoc；proto 生成字段缺业务释义 |
| Q-16 | 前端服务调用 | 19 个 features 域，部分用 `kratosApi.get/post`，部分用生成的 `createXxxClient()`，没有强制规范 |
| Q-17 | 前端 store 缺失 | README §7.1 规定 "features → store → composable → page"，实际只有 2 个 store |

#### 2.5.4 异常处理 / 注释 / 可维护性

| 编号 | 位置 | 问题 |
|------|------|------|
| Q-18 | `service/trpc_turn.go`、`biz/graph.go` | 多处 `go func()` 无 panic recover |
| Q-19 | `cron/runner.go`、`skill/watch/runner.go` | 退出路径无完整 cleanup（关闭 watcher、cancel ctx 顺序未文档化） |
| Q-20 | `biz/session_usecase.go` | 单文件 24K+，包含 CRUD/Timeline/Summary/State/Title/AutoMemory 多职责 |
| Q-21 | `service/agent.go` | 单文件 250+ 行 proto↔biz 双向映射 |
| Q-22 | `biz/AgentRuntimeSettings` | 100+ 平铺字段，建议拆 sub-struct |

### 2.6 红线（来自 AI-DEVELOPMENT-SPECIFICATION.md）违反汇总

| 编号 | 红线 | 现场 |
|------|------|------|
| V-1 | 红线 2「biz 不得 import pkg/trpc-agent-go」 | `internal/biz/biz.go`、`internal/biz/graph.go`、`internal/biz/envelope.go`（间接 re-export） |
| V-2 | 红线 6「不得为框架运行时另起独立 HTTP 监听」 | `internal/server/ws.go` 自启 `:8002` |
| V-3 | 红线 8「biz 不得依赖具体框架对象」 | `biz/graph.go` 直接 use `graph.StateGraph`、`tool.Tool` |
| V-4 | 红线 10「SQLite 只允许 NewData 内一次 sql.Open」 | `session/trpc/sqlite.go`、`graph/trpc/checkpoint.go` 各一处 |
| V-5 | 红线 12「Server 层不得手写 HandleFunc」 | `server/ws.go:mux.HandleFunc("/v1/ws", ...)` |
| V-6 | 红线 13「不得修改 proto 生成代码」 | 暂无证据违反（保持监控） |
| V-7 | 红线（隐性）「文档与代码同步」 | `plan.md` / README / AI-DEV-SPEC 多处描述滞后于代码 |

---

## 3. 优化整改方案（四维）

### 3.1 架构层

1. **引入 `internal/runtime` 中间层（新增）**
   - 把 `runtimedeps.TurnDeps` 拆为四个领域聚合：`RuntimeKernel`（trpc Runner/Agent factory）、`PersistenceSet`（Session/Memory/Checkpoint persistence）、`EventPipeline`（Bus + subscribers）、`Catalog`（Agent/Team/Tool/Skill registry view）。
   - service 层只能依赖 `RuntimeKernel` 接口，禁止直接 import `pkg/trpc-agent-go/*`。
2. **biz 层去框架化**
   - 把 `biz/envelope.go` 改为 biz 自有事件模型 `biz.DomainEvent`，由 `internal/event/projection` 双向投影到 trpc envelope。
   - `biz/graph.go` 通过 `biz.GraphRuntime` 接口反向依赖（实现放到 `internal/graph/trpc/`）。
3. **统一 WS 接入到 Kratos**
   - 用 Kratos `transport/http` 的 WebSocket middleware（或挂一个 `http.HandlerFunc`，由 Kratos `http.Server` 路由）替代独立监听；`config.yaml:server.ws` 改为 `path`/`subprotocol` 配置项。
4. **单 SQLite 连接池**
   - 在 `data.NewData` 维护 `*sql.DB`（与 Ent 共享 underlying driver）；`session/trpc`、`graph/trpc` 通过 `data.Data.RawDB()` 注入，杜绝二次 `sql.Open`。
5. **EventBus 背压**
   - 把"覆盖式丢弃"改为"按订阅者策略"（reliable=block / lossy=drop oldest），关键事件类型默认 reliable。
6. **代码分层依赖检查 CI**
   - `scripts/check-runtime-boundary.ps1` 改写为跨平台 Go 程序，挂到 `make lint`，并加入 GitHub Actions / Drone。

### 3.2 业务层

1. **Memory 完整闭环**
   - 修复 B-1：`memory/trpc/sqlite_adapter.go` 在初始化时 `LoadFromStore()`；或直接抛弃内存 cache，全部走 store。
   - 实现 `EnqueueAutoMemoryJob`：写入 `auto_memory_jobs` 表（schema 已存在）由 cron 消费。
   - 注册 `memory/tool` 五件套到 trpc Runner。
2. **Plugin 接入**
   - 在 `biz/plugin.go` 增加 `Runtime` 接口（Apply、CallbackPoints、Permissions），实现 `internal/plugin/trpc/` 适配 `pkg/trpc-agent-go/plugin`。
   - 在 `BuildTRPCLLMAgent` 注入 PluginManager。
3. **Planner 切换**
   - 改 `internal/agent/trpc_build.go:buildPlanner`，按 `dialogMode + plannerKind` 选择 Builtin / ReAct / A2UI。
4. **Artifact 最小实现**
   - 落地 `internal/artifact/{biz,data,service}`，对接 `pkg/trpc-agent-go/artifact/inmemory + local`。
5. **Knowledge / Evaluation / A2A**
   - 列入 P2 阶段，暂保留接口占位（`biz/knowledge.go` 等 stub）。
6. **Graph 持久化恢复**
   - `biz/graph.go` 改为基于 `checkpoint.Saver` 列表恢复执行；进程内 `executions` map 仅用于活跃 run。
7. **审计与可观测**
   - 所有 RPC 入口在 `service` 层添加 `kerrors.Wrap` + traceID；新增 `internal/server/metrics.go` 暴露 `/metrics`。

### 3.3 工程化层

1. **目录规范**
   - 重命名：`biz/errros.go → errors.go`、`data/agent_catalog_legacy.go → agent_catalog.go`、`data/sessionmemory/entity_adk.go → entity.go`。
2. **统一错误模型**
   - `pkg/apierror`：所有 biz/data/service 使用 `apierror.NotFound / BadRequest / Internal` 包装；禁止裸 `fmt.Errorf` 跨层。
3. **统一 Logger / Tracer / Metrics**
   - 全部从 `kratos/log` + `kratos/tracing` 取，禁止 `log.Default()`。
4. **统一配置**
   - `configs/config.yaml` 增加 schema（用 `pkg/conf/schema.go` 校验），启动时 fail-fast。
5. **统一 Makefile**
   - `make api`（proto + ts client）、`make wire`、`make lint`（含红线检查 + golangci-lint）、`make test`（go test + vitest）、`make ci`。
6. **测试矩阵**
   - service：每个 RPC 至少 1 个 happy path + 1 个 error；biz：usecase 输入边界；data：Ent 集成测试用 `t.TempDir()` + SQLite；前端：composable 单测 + 关键页面 e2e。
7. **前端代码生成**
   - 修复 `make api` 的 ts 子任务，确保所有 proto 都生成到 `web/src/services/kratos/<pkg>/v1/`；`services/index.ts` 自动 barrel export。

### 3.4 扩展性层

1. **Tool/Skill/MCP 注册中心抽象**
   - `internal/tools/registry/Registry` 改为 generic `Registry[T]`；Skill、MCP、Plugin 复用同一接口。
2. **Provider 矩阵**
   - 抽 `provider.Selector`（Failover/Hedge/TokenTailor 策略）接入到 `model/`，配置驱动。
3. **多租户 / Workspace 一致性**
   - 所有 biz 查询统一 `WithWorkspace(ctx, id)` middleware；data 层在 Ent hook 强制注入 workspace_id 谓词。
4. **插件 / 扩展点白名单**
   - `docs/guides/extension-points.md`（新增）明确每个 CallbackPoint 的输入/输出/优先级。
5. **国际化**
   - 后端 `kerrors` 错误码 + locale；前端 i18n 资源文件。

---

## 4. 功能补全计划（M1~M20）

> 状态符号：✅完成 / 🟡部分 / ❌未实现 / 🆕本规划新增
> 任务编号（Txx）对应 [task-tracker.md](task-tracker.md)，可通过该表追踪完成进度。

| ID | 模块 | 当前实状（基于代码） | 计划动作 | 阶段 | 任务 |
|----|------|-----------------------|----------|------|------|
| M1 | Agent | ✅ trpc LLMAgent 构建 | 引入 Agent 构建缓存（Q-1）；拆分 Settings sub-struct（Q-22） | P1 | T16 / T36 |
| M2 | Runner | ✅ Runner + Turn 流程 | 抽 `RuntimeKernel`；接入 PluginManager；接入 ManagedRunner/SteerableRunner（M-10） | P0 | T14 / T21 |
| M3 | Session | ✅ SQLite SessionService（含红线违反） | 单连接池（V-4）；增加 SessionIngestor；完善 RunStatus | P0 | T1 / T21 |
| M4 | Graph | ✅ Validator / Checkpoint | 修复 builder 并发 race（B-5）；Checkpoint 基于 Data.RawDB；恢复 ResumeExecution（B-4） | P0 | T1 / T6 / T7 |
| M5 | Team | ✅ Team Runner | 抽 Envelope 投影到 `event/projection`（R-7） | P1 | T3 |
| M6 | Memory | 🟡 SQLite 适配器但 cache 失效 | 修复 ReadMemories（B-1）；接入 5 个 memory tool（M-2）；EnqueueAutoMemoryJob（M-1） | P0 | T5 / T32 / T33 |
| M7 | Tool | ✅ Registry/Skillrouter/MCPMount | 删除未用函数（R-1）；提取公共 filter（R-9） | P1 | T20 |
| M8 | MCP | ✅ trpcmcp 集成 | 文档同步；接入 MCP 健康检查 | P2 | T13 |
| M9 | Skill | 🟡 文件 watcher | 实现 `skill/repository` 接口适配；UI 一致化 | P2 | T30 |
| M10 | Plugin | ❌ 仅 CRUD | 适配 PluginManager + Callback Points（M-4） | P1 | T29 |
| M11 | Planner | 🟡 仅 Builtin | 接入 ReAct / A2UI（M-5） | P2 | T31 |
| M12 | Artifact | ❌ | 最小实现 inmemory/local + REST（M-6） | P2 | T34 |
| M13 | Knowledge | ❌ | 设计 + pgvector 集成（M-7） | P3 | T38 |
| M14 | CodeExecutor | 🟡 仅 local | 加 docker / sandbox 选项（M-8） | P3 | T41 |
| M15 | A2A | ❌ | 长期规划（M-9） | P3 | T40 |
| M16 | Gateway | 🟡 chat + ws | 暴露 `GetRunStatus` / `AwaitUserReply` RPC（M-10） | P1 | T21 |
| M17 | Evaluation | ❌ | P3 占位 | P3 | T39 |
| M18 | Cron | ✅ | 增加失败重试 + metrics（B-14） | P2 | T35 |
| M19 | Callback | 🟡 仅 Tool | Agent/Model Callback；统一 Callback 配置（M-12） | P1 | T22 |
| M20 | Event | ✅ EventBus | 背压策略改造（B-8） | P0 | T8 / T15 |

### 4.1 前端缺口

| ID | 项 | 动作 | 任务 |
|----|----|------|------|
| F-1 | Graph TS 客户端 | 修复 `make api`；生成 `web/src/services/kratos/graph/v1/`（M-13） | T11 |
| F-2 | Chat 客户端使用 | 改 `features/chat/api.ts` 用生成的 `createChatService()`（M-14） | T12 |
| F-3 | Pinia store 缺失 | 17 个域补 store，遵循 README §7.1（M-15） | T17 |
| F-4 | 统一 axios handler | `services/axiosHandler.ts` 抽错误码映射 + WS 重连策略 | T18 |
| F-5 | i18n / theme tokens | 抽 `web/src/design/`，对接设计 token | S6+ |

---

## 5. 代码重构方向（模块级）

### 5.1 `internal/runtimedeps` → `internal/runtime`

```text
internal/runtime/
  kernel.go        // RuntimeKernel 接口 + 实现（trpc 适配）
  persistence.go   // Session/Memory/Checkpoint 抽象
  pipeline.go      // EventPipeline 抽象
  catalog.go       // Agent/Team/Tool/Skill 只读视图
```

迁移规则：service 层只能依赖 `runtime` 包导出的接口；`runtime` 包内部允许 import `pkg/trpc-agent-go/*`。

### 5.2 `internal/event`

- 新增 `event/projection/`：负责把 trpc `event.Event` 投影到 `biz.DomainEvent`；biz 不再 import envelope。
- `bus.go`：增加 `SubscribeOption{ Reliable bool, Buffer int }`；reliable=true 时使用 `time.After` + 退避，不丢事件。

### 5.3 `internal/biz`

- 拆分 `session_usecase.go`：`session_crud.go`、`session_timeline.go`、`session_summary.go`、`session_state.go`、`session_title.go`、`session_auto_memory.go`。
- 删除 `envelope.go`；新增 `domain_event.go`。
- `graph.go`：仅保留 GraphDefinition 业务用例；运行时控制下沉到 `internal/graph/trpc/runtime.go`。
- 拆分 `AgentRuntimeSettings`：
  ```
  type AgentRuntimeSettings struct {
      Identity   IdentityCfg
      Reasoning  ReasoningCfg
      Memory     MemoryCfg
      Tools      ToolsCfg
      Skills     SkillsCfg
      Plugins    PluginsCfg
      Evolution  EvolutionCfg
      Context    ContextCfg
  }
  ```

### 5.4 `internal/data`

- 新增 `Data.RawDB()` 方法返回 Ent underlying `*sql.DB`，所有需要 sql.DB 的适配器走该方法。
- `agent_catalog_legacy.go → agent_catalog.go`，`sessionmemory/entity_adk.go → entity.go`。

### 5.5 `internal/service`

- 合并 `chat.go` + `chat_native.go` → `chat.go` + `chat_pending_queue.go`（按职责拆分而非按"native/non-native"）。
- 抽 `service/internal/proto_mapper`：替代 250+ 行手写映射。

### 5.6 `internal/server`

- 删除 `ws.go` 中 `http.Server` 字段，改成 `Handler() http.Handler` + Kratos `http.Server` 挂载 `/v1/ws`。
- 集中 middleware：`logging`、`tracing`、`recovery`、`auth`（缺失需补）、`workspace`（多租户）。

### 5.7 `internal/session/trpc` & `internal/graph/trpc`

- 构造函数签名改为 `New(...)(svc, error)`，参数仅 `*sql.DB`（来自 Data.RawDB）。
- 提供 `Migrate(ctx, db)` 函数，把 raw SQL DDL 转为 ent migration 或单独的 migrations 目录。

### 5.8 `internal/memory/trpc`

- 删除进程内 cache；ReadMemories/SearchMemories/UpdateMemory 全部走 store；DeleteMemory 同步 store。
- 抽 `MemoryStore` 接口，便于将来切到 pgvector。

### 5.9 `internal/agent`

- `BuildTRPCLLMAgent` 拆为 `Compose(cfg)` 纯函数 + `Cache` 装饰器。
- `trpc_callbacks.go`：把 ToolCallback 提取为 `CallbackChain`，便于挂 Agent/Model/Plugin 级别。

### 5.10 `web`

- `services/index.ts`：barrel 自动生成（脚本写入 `make api` 末尾）。
- 每个 `features/<x>/`：必须有 `api.ts`（生成客户端封装）+ `store.ts`（Pinia）+ `composables/` + `pages/` + `components/`。
- 全局错误显示：`services/axiosHandler.ts` 接 `Notify`（Quasar）。

---

## 6. 开发优先级划分

### 6.1 优先级定义

- **P0（立刻整改）**：红线违反 + 阻塞前端构建 + 数据正确性 bug。
- **P1（一个迭代内）**：架构债务（耦合、缓存、可观测）。
- **P2（两个迭代内）**：功能补全（Plugin/Planner/Artifact/Cron 等）。
- **P3（长期路线图）**：Knowledge/Evaluation/A2A/CodeExecutor 升级。

### 6.2 P0 清单（红线 + 阻塞）

1. V-4 单 SQLite 连接池 —— 改造 `data.go`、`session/trpc/sqlite.go`、`graph/trpc/checkpoint.go`。
2. V-2 / V-5 WS 接入 Kratos —— 删除 `server/ws.go` 独立监听。
3. V-1 / V-3 biz 去框架化 —— 拆 `biz/envelope.go`、`biz/graph.go`。
4. B-1 Memory cache 修复 + B-12 EnqueueAutoMemoryJob 落地。
5. B-2/B-5/B-8 修复（Server/Graph/Bus）。
6. M-13 / M-14 前端 graph 客户端 + chat 使用生成客户端。
7. D-1 ~ D-10 文档校正（`plan.md`、README、AI-DEV-SPEC、master-plan 状态表）。

### 6.3 P1 清单

1. RuntimeKernel 抽象 + service 层迁移。
2. EventBus 背压策略。
3. BuildTRPCLLMAgent 缓存（Q-1）。
4. M16 Gateway RunStatus / AwaitUserReply 路由。
5. M19 Callback Chain（Agent/Model 级）。
6. 错误模型 / 命名 / 文件名统一（R-3/R-5/R-6、Q-11/Q-12/Q-14）。
7. 前端 store 补齐 + Pinia 规范。

### 6.4 P2 清单

1. M10 Plugin 接入 PluginManager。
2. M11 Planner ReAct / A2UI。
3. M12 Artifact 最小实现。
4. M9 Skill repository 接入。
5. M18 Cron 失败重试 + metrics。
6. 测试矩阵搭建（service/biz 单测、e2e）。

### 6.5 P3 清单

1. M13 Knowledge（pgvector pipeline）。
2. M17 Evaluation。
3. M15 A2A。
4. M14 CodeExecutor 沙箱 / docker。
5. 多租户/i18n 强化。

### 6.6 模块迭代顺序（Sprint 视角）

| Sprint | 主线 | 关键产出 | 详细计划 |
|--------|------|----------|----------|
| S1（2 周） | P0 红线 + 数据正确性（T1~T13） | 单连接池；WS 改造；Memory 修复；前端 graph 客户端；文档同步 | [S1-p0-redlines.md](sprints/S1-p0-redlines.md) |
| S2（2 周） | P1 架构债（T14~T20） | RuntimeKernel；EventBus 背压；Agent 构建缓存；前端 store | [S2-architecture-debt.md](sprints/S2-architecture-debt.md) |
| S3（2 周） | P1 业务可观测（T21~T28） | RunStatus RPC；Callback Chain；统一错误模型；测试基线 | [S3-observability.md](sprints/S3-observability.md) |
| S4（2 周） | P2 功能补全（一）（T29~T33） | Plugin 接入；Skill 接入；Planner ReAct；Memory 工具五件套 | [S4-plugin-skill-planner.md](sprints/S4-plugin-skill-planner.md) |
| S5（2 周） | P2 功能补全（二）（T34~T37） | Artifact；Cron 重试；测试矩阵覆盖 60% | [S5-artifact-cron-tests.md](sprints/S5-artifact-cron-tests.md) |
| S6+（开放窗口） | P3 / 长期（T38~T41） | Knowledge / Evaluation / A2A / CodeExecutor 沙箱 | [S6-knowledge-eval-a2a.md](sprints/S6-knowledge-eval-a2a.md) |

---

## 7. 编码规范统一标准

> 本节是 `AI-DEVELOPMENT-SPECIFICATION.md` 的精炼复述 + 本规划新增。所有团队成员、AI 协作工具必须遵守。

### 7.1 不可逾越红线（强制 CI 检查）

| 编号 | 红线 | 检查方式 |
|------|------|----------|
| R1 | `internal/server/*` 不得直接 `runner.Runner{}` / `llmagent.New` | `make lint` AST 检查 |
| R2 | `internal/biz/*` 不得 import `pkg/trpc-agent-go/*` 及 `internal/*/trpc/` | grep + AST |
| R3 | `internal/data/*` 不得依赖 `internal/biz/*` | go list -deps |
| R4 | `internal/service/*` 不得直接读 Ent client，必须经 biz 仓储接口 | grep |
| R5 | proto 生成代码不得手工修改（`*.pb.go`、`*_grpc.pb.go`、`*.ts`） | git diff CI |
| R6 | 不得为框架运行时另起独立 HTTP 监听 | grep `http.Server{` + 白名单 |
| R7 | 不得手写 `mux.HandleFunc` / `http.HandleFunc` | grep |
| R8 | SQLite `sql.Open` 仅允许 `internal/data/data.go:NewData` 一处 | grep + 文件名白名单 |
| R9 | 不得在业务包内裸用 `log.Default()` | grep |
| R10 | `cmd/admin/main.go` 不得写业务逻辑（仅 Wire + lifecycle） | 行数 + AST |

### 7.2 决策树（写代码前必读）

```
是「定义业务模型/用例」？    → internal/biz
是「访问数据库/外部存储」？  → internal/data
是「实现 RPC 接口」？        → internal/service
是「装配 transport」？       → internal/server
是「Agent 构建/Runner 适配」？→ internal/agent | internal/team | internal/graph/trpc
是「工具/插件/技能/MCP」？   → internal/tools/{registry,trpc,skillrouter,mcpmount,...}
是「跨模块的通用工具」？     → pkg/<utility>
是「框架（trpc-agent-go）功能扩展」？ → 先在 internal/*/trpc 适配，再考虑 PR 上游
```

### 7.3 命名约定

- Go 包名小写单数；文件名 `snake_case.go`；测试 `*_test.go`。
- 业务结构体字段：`MCPCallCount`（首字母连续大写缩写也用 `MCP` 而非 `Mcp`）。
- proto 字段：`snake_case`，生成 Go 字段 `Mcp`（保留 proto 默认）；biz↔proto 在 `service` 层映射时显式声明。
- 文件名不得带 `legacy`、`adk`、`new`、`v2`、`final` 等无意义后缀。

### 7.4 错误处理

- 所有跨层错误使用 `kerrors`（`go-kratos/kratos/v2/errors`）：
  - `kerrors.NotFound("DOMAIN", ...)` / `BadRequest` / `Internal` / `Unauthorized`。
- biz 层只返回业务语义错误；data 层使用 `kerrors.Wrap` 包装原始 ent 错误；service 层只决定 HTTP/gRPC code。
- 不得在跨层接口暴露 `database/sql` 的 `sql.ErrNoRows`。

### 7.5 注释

- biz 模型 struct 必须有结构体级 godoc + 关键字段注释。
- 公共函数必须有"参数 / 返回值 / 副作用"三段式注释。
- TODO 必须带 `// TODO(@owner, YYYY-MM-DD): ...`。

### 7.6 测试

- biz/service：每个公开方法 ≥ 1 happy + 1 error 单测。
- data：与 Ent 集成测试用 `t.TempDir()` + `entx.Open` 内存 SQLite。
- 前端：composable 单测；关键流程 vitest + Cypress e2e。
- CI 阈值：unit ≥ 60% line coverage（S5 起强制）。

### 7.7 文档

- 任何改变行为/接口的 PR 必须同步：`docs/changelog/YYYY-MM-DD-Topic.md` + 影响到的 guide。
- 状态表（`master-plan.md` §4 + `plan.md` 模块表）每个 Sprint 收尾刷新。

### 7.8 前端

- 数据流：`features/<x>/api.ts → stores/<x>.ts (Pinia) → composables/use<X>.ts → pages → components`。
- 不得在页面/组件直接 `kratosApi.post`；必须用生成的客户端 + store action。
- 样式：Tailwind / Quasar token；禁止 inline style 与硬编码颜色。

---

## 8. 落地分类索引

### 8.1 ① 可直接落地重构（已有代码 / 接口稳定，仅需手术）

| 项 | 涉及文件 | 说明 |
|----|-----------|------|
| V-4 单 SQLite 连接池 | `internal/data/data.go`、`internal/session/trpc/sqlite.go`、`internal/graph/trpc/checkpoint.go` | 改构造函数签名，删除二次 sql.Open |
| V-2/V-5 WS 接入 Kratos | `internal/server/ws.go`、`internal/server/http.go`、`configs/config.yaml` | 删独立 server，挂到 Kratos HTTP |
| V-1/V-3 biz 去框架 | `internal/biz/biz.go`、`internal/biz/envelope.go`、`internal/biz/graph.go` | 移除 trpc import，envelope 投影下沉 |
| B-1 Memory cache 修复 | `internal/memory/trpc/sqlite_adapter.go` | 删 cache，全走 store |
| B-5 Graph builder race | `internal/graph/trpc/builder.go` | 改不可变 config + 函数式 build |
| B-4 executions GC | `internal/biz/graph.go` | TTL + 重启不依赖内存 map |
| R-3/R-5/R-6 文件名 | `internal/biz/errros.go`、`internal/data/agent_catalog_legacy.go`、`internal/data/sessionmemory/entity_adk.go` | 直接 rename，更新 import |
| R-1 删未用函数 | `internal/tools/mcpmount/append.go:AppendEffectiveMCPServerToolsets` | 删 + 测试通过 |
| R-2/R-9 合并重复实现 | `internal/agent/trpc_runtime.go`、`internal/session/trpc/sqlite.go`、`internal/tools/{toolset.go,mcpmount/}` | 抽公共，删旧 |
| M-13 前端 graph 客户端 | `Makefile`、`api/kratos/graph/v1/`、`web/src/services/index.ts` | 修生成脚本 |
| M-14 前端 chat 用生成客户端 | `web/src/features/chat/api.ts`、`web/src/services/index.ts` | 替换 axios 调用 |
| Q-14 命名清理 | 同 R-3/R-5/R-6 | — |
| D-1~D-9 文档同步 | `docs/guides/plan.md`、`docs/README.md`、`docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` | 删 SSE / ADK 残留，刷新状态表 |

### 8.2 ② 新增开发（需要写新代码 / 新模块）

| 项 | 目标位置 | 说明 |
|----|----------|------|
| RuntimeKernel 抽象 | `internal/runtime/` | 取代 runtimedeps 角色 |
| EventBus 背压策略 | `internal/event/bus.go` 扩展 + `event/projection/` | reliable/lossy 模式 |
| Agent 构建缓存 | `internal/agent/cache.go` | LRU+config hash |
| Memory 五件套工具 | `internal/tools/memory/`（新建） | 注册到 Runner |
| EnqueueAutoMemoryJob | `internal/biz/session_auto_memory.go`、`internal/cron/jobs/auto_memory.go` | 用现有 schema |
| Plugin 运行时 | `internal/plugin/trpc/`、`biz/plugin.go` 扩展 | PluginManager 适配 |
| Planner 多策略 | `internal/agent/planner/` | 按 plannerKind 选择 |
| Artifact 模块 | `internal/artifact/{biz,data,service}`、`api/kratos/artifact/v1/` | 最小可用 |
| Skill repository 适配 | `internal/skill/trpc/` | 接入 trpc skill API |
| RunStatus / AwaitUserReply | `api/kratos/chat/v1/chat.proto`、`internal/service/chat.go` | 暴露 gateway 能力 |
| Callback Chain | `internal/agent/callbacks/` | Agent/Model/Tool/Plugin 统一 |
| 错误模型 | `pkg/apierror/` | 跨层错误包 |
| Workspace middleware | `internal/server/middleware/workspace.go` | 多租户 |
| Metrics endpoint | `internal/server/metrics.go` | Prometheus |
| 跨平台 lint 工具 | `cmd/araneactl/lint`、`Makefile` | 替代 ps1 |
| 前端 Pinia store（17 个） | `web/src/stores/<域>.ts` | 按 README §7.1 |
| 前端 axios 错误处理 | `web/src/services/axiosHandler.ts` 升级 | Notify + WS 重连 |
| 测试矩阵 | `internal/*/...*_test.go`、`web/src/**/__tests__/` | service/biz/前端 |
| Knowledge / Evaluation / A2A 接口占位 | `internal/knowledge/`、`internal/evaluation/`、`internal/a2a/` | stub + proto |

### 8.3 ③ 仅调整优化（小手术 / 标准化 / 文档化）

| 项 | 涉及位置 | 说明 |
|----|----------|------|
| `AgentRuntimeSettings` 拆 sub-struct | `internal/biz/agent.go`、`internal/service/agent.go`、proto 文档 | 不改字段名 |
| `service/agent.go` proto 映射抽工具 | `internal/service/internal/proto_mapper/` | 减少重复 |
| `session_usecase.go` 拆分 | `internal/biz/session_*.go` | 按职责 |
| `chat.go` / `chat_native.go` 合并 | `internal/service/chat*.go` | 按职责拆，而非按 native |
| `event/bus.go` 注释 + 文档 | 注释 + `docs/guides/event-bus.md` | 解释丢弃策略 |
| `Makefile` 标准目标 | `Makefile` | api / wire / lint / test / ci |
| `scripts/check-runtime-boundary.ps1` 迁移 | `cmd/araneactl/lint` | 跨平台 |
| 全局 logger 注入 | `cmd/admin/wire.go`、各包 | 移除 log.Default |
| 注释补全 | `internal/biz/*`、`internal/agent/*` | godoc |
| `pkg/jsonutil`、`pkg/strutil` 文档化 | 各包 README/godoc | 工具集说明 |
| `docs/changelog/` 补全 | `docs/changelog/` | 每个 Sprint 末刷新 |
| `docs/README.md §13` AI 工作流 | README §13 | 改 SSE → WS |
| `plan.md` 状态表 | `docs/guides/plan.md` | 按 §4 更新 |

---

## 9. 验收标准（每个 Sprint 收尾必过）

1. **CI 全绿**：`go vet`、`golangci-lint`、`make lint`（红线检查）、`go test ./...`、`pnpm build`、`pnpm test`。
2. **红线零违反**：grep 检查脚本输出空。
3. **文档同步**：本规划文档 §4 状态表、`plan.md`、`changelog/` 与本 Sprint 改动一致。
4. **覆盖率**：S5 起 line coverage ≥ 60%（go），关键 composable + page e2e ≥ 80% 通过率。
5. **运行时 Smoke**：`make smoke`（启动 + 一次 chat turn + tool 调用 + memory 写入 + graph 执行）零错误。

---

## 10. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| SQLite 单写者瓶颈 | 高并发性能下降 | 通过单连接池 + 写队列；为生产推荐 Postgres |
| trpc-agent-go 上游 API 变更 | 适配层频繁返工 | `internal/*/trpc` 适配层加版本兼容测试；锁定 `go.mod` 版本 |
| 重构期间业务回归 | 用户感知功能丢失 | Sprint 内"feature freeze"；每 PR 必跑 smoke |
| 前端代码生成不稳 | 编译失败 | `make api` 输出 diff 检查；CI 中断阻断合并 |
| Memory 切换后历史数据兼容 | 旧 cache 行不可读 | 写 migration job；保留 fallback 读路径 |
| 文档与代码再次脱钩 | 同样问题重现 | 文档同步纳入 CI（每个 PR 检查 changelog + 状态表 diff） |

---

## 11. 参考索引

- `docs/README.md`
- `docs/guides/AI-DEVELOPMENT-SPECIFICATION.md`
- `docs/guides/plan.md`
- `docs/guides/trpc-agent-go-framework.md`
- `docs/changelog/2026-05-12-Provider.md` / `2026-05-13-Session.md` / `2026-05-16-Graph.md` / `2026-05-16-Session-Optimize.md`
- `internal/server/ws.go`、`internal/session/trpc/sqlite.go`、`internal/graph/trpc/checkpoint.go`、`internal/memory/trpc/sqlite_adapter.go`、`internal/biz/graph.go`、`internal/biz/envelope.go`、`internal/service/chat.go`、`internal/service/chat_native.go`、`web/src/services/index.ts`、`web/src/features/chat/api.ts`

---

## 12. 配套文档索引（执行入口）

> 本节文档由 master-plan 衍生，提供逐任务的可执行落地方案。本文（master-plan.md）是**只读基准**，所有执行动作以下列文档为准。

| 文档 | 路径 | 用途 |
|------|------|------|
| 实施方案总览 | [docs/guides/implementation-plan.md](implementation-plan.md) | 跨 Sprint 依赖图、风险地图、角色分工、PR 命名规范、度量阈值 |
| 任务追踪表 | [docs/guides/task-tracker.md](task-tracker.md) | T1~T41 全量勾选表 + 32 个 PR 索引 + 周报模板 + 阻塞登记 |
| S1 详细计划 | [docs/guides/sprints/S1-p0-redlines.md](sprints/S1-p0-redlines.md) | P0 红线 + 数据正确性（T1~T13，8 PR，2 周） |
| S2 详细计划 | [docs/guides/sprints/S2-architecture-debt.md](sprints/S2-architecture-debt.md) | P1 架构债（T14~T20，5 PR，2 周） |
| S3 详细计划 | [docs/guides/sprints/S3-observability.md](sprints/S3-observability.md) | P1 业务可观测 + 测试基线（T21~T28，6 PR，2 周） |
| S4 详细计划 | [docs/guides/sprints/S4-plugin-skill-planner.md](sprints/S4-plugin-skill-planner.md) | P2 功能补全（一）（T29~T33，5 PR，2 周） |
| S5 详细计划 | [docs/guides/sprints/S5-artifact-cron-tests.md](sprints/S5-artifact-cron-tests.md) | P2 功能补全（二）+ 测试矩阵 60%（T34~T37，4 PR，2 周） |
| S6 详细计划 | [docs/guides/sprints/S6-knowledge-eval-a2a.md](sprints/S6-knowledge-eval-a2a.md) | P3 长期能力（T38~T41，4~8 PR，开放窗口） |

---

> 文档维护：每个 Sprint 收尾由值班 Tech Lead 刷新本文 §4 状态表（`任务`列对应任务编号）与 §8 落地清单；新增的红线一律同步进 §7.1 并加 CI 检查；同步更新 [task-tracker.md](task-tracker.md) 对应任务状态。
