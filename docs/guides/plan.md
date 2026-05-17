# Aranea Agents — trpc-agent-go 功能对齐与超越计划

> 本文档是项目对标 `pkg/trpc-agent-go` 框架的总体大纲，目标是 **完全复刻** 框架能力并 **超越**。
> 每个模块的详细需求见 `docs/需求/` 下对应文档。
> **最近一次代码审计**：2026-05-15，基于 `internal/` 全量代码扫描 + `pkg/trpc-agent-go/tool/` 逐文件能力审计。

---

## 一、总体目标

| 维度 | 目标 |
|------|------|
| **复刻** | 将 trpc-agent-go 框架的全部核心能力集成到项目中，不留功能盲区 |
| **超越** | 在复刻基础上增加产品层能力（多租户、权限、审计、可视化编排、评估平台），形成框架+平台的双层架构 |
| **架构** | 项目作为产品壳层，trpc-agent-go 作为运行时内核；产品层通过适配器桥接框架接口 |

---

## 二、功能对齐总览

> 对齐状态基于 2026-05-14 代码审计结果，反映代码库实际实现情况。

| # | 模块 | trpc-agent-go 框架能力 | 项目现有实现 | 对齐状态 | 优先级 | 需求文档 |
|---|------|----------------------|-------------|---------|--------|----------|
| M1 | Skill 运行时 | `skill.Repository` + `tool/skill/{load,run,list_docs,select_docs}` + 渐进披露 + 工作区执行 + SkillLoadMode + PromptCache | `skill/trpc/repository.go` (FSRepositoryAdapter) + `skill/trpc/tools.go` + `skill/trpc/executor.go` + `skill/trpc/filter.go` | ✅ 已对齐 | P0 | `20 skill.md` |
| M2 | Agent 构建 | `llmagent.New` + 占位符变量 + ModelInstructions + Planner + SkillLoadMode + ContextCompaction + SessionSummary | `agent/trpc_build.go` (BuildTRPCLLMAgent) + `agent/trpc_runtime.go` (NewTRPCRunner) + `agent/prompt.go` (BuildSystemPrompt+RuntimeCapabilityCue) | ✅ 已对齐 | P0 | `2 agents-create.md` |
| M3 | Team 编排 | `team.NewCoordinator` / `team.NewSwarm` + AgentTool + TransferTool + crossRequestTransfer + SwarmHandoffInput | `team/trpc_build.go` (BuildTRPCTeam: coordinator/swarm/sequential/parallel/critic_loop + WithCrossRequestTransfer + WithSwarmHandoffInput) + `team/runner.go` + `team/definition.go` | ✅ 已对齐 | P1 | `11 multi-agent.md` |
| M4 | Graph 工作流 | `graph.StateGraph` + 节点/边/条件路由 + HITL + 检查点 + 时间旅行 + 子图 + DAG引擎 + 可视化 | `graph/trpc/builder.go` (BuildStateGraph + GraphAgent + StateSchema/Reducer + 子图 + DAG) + `graph/trpc/registry.go` (NodeFunc/CondFunc 注册表) + `graph/trpc/checkpoint.go` (SQLite Checkpoint Saver) + `graph/trpc/event_bridge.go` (9种ObjectType→Envelope映射) + `graph/trpc/visualize.go` (DOT解析+结构化JSON) + `api/kratos/graph/v1/` (15个RPC端点) + `biz/graph.go` (CRUD+Execute+Resume+TimeTravel+EventBridge+ListExecutions+CancelExecution) + `data/graph.go` (GraphRepo+GraphRunRepo+CheckpointSaver) + `data/ent/schema/graph_execution.go` (执行持久化) + `service/graph.go` + 前端Vue Flow编辑器 | ✅ 已对齐（v2四维架构设计完成，P0/P1待实现） | P1 | `graph-workflow.md` |
| M5 | Session 管理 | `session.Service` + SQLite/Redis/PG/MySQL/ClickHouse + 摘要压缩 + Event分页 + Track + Ingestor | `session/trpc/sqlite.go` (NewSQLiteSessionService + NewInMemorySessionService) + `data/sessionmemory/` (SQLite Store 用于 Memory) + `service/session_compress.go` (异步压缩) | ✅ 已对齐 | P1 | `10 session.md` |
| M6 | Memory 记忆 | `memory.Service` + 自动提取(Auto) + 工具驱动(Agentic) + Mem0 + 多后端(SQLite/PG/Redis/MySQL/pgvector) + 向量搜索 | `memory/trpc/sqlite_adapter.go` (sqliteMemoryService: AddMemory/UpdateMemory/DeleteMemory/SearchMemories/ReadMemories/Tools 基础CRUD) + `data/sessionmemory/` (Store) | ❌ 严重不足 | P2 | `memory.md` + `12-16` |
| M7 | Tool 工具体系 | `tool.Tool`/`CallableTool`/`StreamableTool` + FunctionTool + MCP + 流式 + 重试 + 过滤 + Callbacks + ToolSet + 并行 + arxivsearch + awaitreply + claudecode + email + geminifetch + google/search + openapi + todo + wikipedia + workspaceexec + claudefetch(stub) | `tools/tool.go` (项目级接口别名) + `tools/toolset.go` (Registry+Assemble总入口) + `tools/trpc/toolsets.go` (向后兼容适配器) + `tools/mcpmount/` (trpc MCP) + `tools/skillruntime/` (Skill) + `agent/trpc_build.go` (buildToolFilter+buildToolCallbacks+buildToolRetryPolicy+WithEnableParallelTools) + DB字段(tools_retry_*/tools_parallel_enabled/tools_streaming_enabled) + 前端UI(重试/并行/流式配置卡片) | ✅ 已对齐 | P1 | `23 tools.md` |
| M8 | MCP 集成 | `tool/mcp.ToolSet` + `tool/mcpbroker.Broker` + STDIO/SSE/StreamableHTTP + 运行时发现 + 会话重连 | `tools/mcpmount/append.go` (trpc MCP ToolSet) + `tools/mcpmount/config.go` + `tools/mcpmount/transport.go` + `biz/mcp_server.go` | ⚠️ 部分实现（已迁移到 trpc，Broker/传输层待完善） | P2 | `19 mcp.md` |
| M9 | Model 模型层 | `model.Model` + OpenAI/Gemini/Anthropic/Ollama/Bedrock/Hunyuan/HuggingFace + Failover/Hedge + TokenTailor | `provider/trpc_llm.go` (TRPCModelForProviderModel + wrapHA: failover/hedge) + `provider/catalog.go` (CatalogConfig + HA配置) | ✅ 基本对齐 | P2 | `9 provider.md` |
| M10 | Plugin 插件 | `plugin.Plugin` + Runner级生命周期 + BeforeModel/AfterTool/OnEvent + PluginManager | `biz/plugin.go` (仅CRUD) + `data/plugin.go` (Ent Repo) + `data/ent/schema/plugin.go` | ❌ 未实现 | P2 | `22 plugin.md` |
| M11 | Planner 规划 | `planner.BuiltinPlanner` / `planner.ReActPlanner` / `planner.A2UIPlanner` + 思考链 | `agent/trpc_build.go` (仅BuiltinPlanner: dialogMode=="plan"时启用) | ⚠️ 部分实现 | P2 | `planner.md` |
| M12 | Artifact 制品 | `artifact.Service` + S3/COS/InMemory + 版本管理 + ListArtifactKeys | 无 | ❌ 未实现 | P2 | `artifact.md` |
| M13 | Knowledge 知识库 | `knowledge.Knowledge` + OCR + Query + RAG + 分块 + Source + AgenticFilter + SearchFilter | 无 | ❌ 未实现 | P3 | `knowledge.md` |
| M14 | CodeExecutor | `codeexecutor.CodeExecutor` + Local/E2B/Jupyter/Container + WorkspaceRegistry + Interactive | `skill/trpc/executor.go` (仅Local, 通过 llmagent.WithCodeExecutor 注入) | ⚠️ 部分实现 | P2 | `codeexecutor.md` |
| M15 | A2A 协议 | `a2aagent.A2AAgent` + A2AServer + AgentCard + 流式 + DataPart映射 + GraphResume | 无 | ❌ 未实现 | P3 | `a2a-protocol.md` |
| M16 | Gateway 网关 | `runner.Runner` + HTTP webhook + 会话并发控制 + status/cancel + AwaitUserReply + QueuedUserMessage | `server/ws.go` + `service/chat_native.go` (activeRuns sync.Map + pendingQueue) + `service/chat.go` (ManagedRunner引用) | ⚠️ 部分实现 | P2 | `gateway.md` |
| M17 | Evaluation 评估 | `evaluation.AgentEvaluator` + EvalSet + Metric + LLM-as-Judge + UserSimulation + MultiRun | 无 | ❌ 未实现 | P3 | `evaluation.md` |
| M18 | Event 事件 | `event.Event` + 流式 + 标签 + StateDelta + Extensions + FilterKey + Branch + Clone + Actions | `server/ws.go` (WebSocket投影) + `service/trpc_turn.go` (事件流处理: StateDelta/Extensions/Branch/FilterKey/Tag) | ✅ 已对齐 | P2 | `event-system.md` |
| M19 | Callback 回调 | `agent.Callbacks` + BeforeAgent/AfterAgent + StructuredCallback + ModelCallbacks + ToolCallbacks | `agent/trpc_build.go` (buildToolCallbacks: AfterTool error handling) + DB字段(tools_retry_enabled等) + 前端UI | ⚠️ 部分实现 | P2 | `callback.md` |
| M20 | Runner 运行器 | `runner.Runner` + ManagedRunner + SteerableRunner + AgentFactory + PluginManager + ArtifactService + SessionIngestor + AwaitUserReply | `agent/trpc_runtime.go` (NewTRPCRunner: ManagedRunner + CancelTRPCRun + EnqueueTRPCUserMessage + SessionService + MemoryService) | ✅ 已对齐 | P1 | `runner.md` |

---

## 三、代码审计发现

### 3.1 已实现但未充分利用的框架能力

| 能力 | 框架支持 | 项目现状 | 差距 |
|------|----------|----------|------|
| Session SQLite 持久化 | `session/sqlite.NewSessionService` | `session/trpc/sqlite.go` 已导出 `NewSQLiteSessionService` + `NewInMemorySessionService`，通过 Wire 注入 Runner | ✅ 已对齐 |
| Memory 自动提取 | `memory.Service.EnqueueAutoMemoryJob` + `memory/extractor` | `sqliteMemoryService` 有 `EnqueueAutoMemoryJob` 方法但未实现自动提取逻辑 | 需集成 `memory/extractor` |
| Memory 工具 | `memory/tool` 提供 memory_add/search/load 等工具 | `sqliteMemoryService.Tools()` 返回 `trpcmemtool.New(s)` 但未确认工具是否正确注入 Agent | 需验证工具注入链路 |
| Model Failover/Hedge | `model/failover` + `model/hedge` | `provider/trpc_llm.go` 已实现 `wrapHA` 支持 failover/hedge 模式 | ✅ 已对齐 |
| Model TokenTailor | `trpcprovider.WithEnableTokenTailoring` | `provider/trpc_llm.go` 已支持 `EnableTokenTailoring` 配置 | ✅ 已对齐 |
| Team Swarm | `team.NewSwarm` | `team/trpc_build.go` 已使用 `trpcteam.NewSwarm` | ✅ 基本对齐 |
| Team Coordinator | `team.New` | `team/trpc_build.go` 已使用 `trpcteam.New` | ✅ 基本对齐 |
| Graph 条件路由 | `graph.AddConditionalEdges` | `graph/trpc/builder.go` 已支持 `ConditionalEdgeDef` | ✅ 基本对齐 |
| Skill Repository | `skill.NewFSRepository` | `skill/trpc/repository.go` 已实现 `FSRepositoryAdapter` | ✅ 已对齐 |
| CodeExecutor Local | `codeexecutor/local` | `skill/trpc/executor.go` 通过 `llmagent.WithCodeExecutor` 注入 | ✅ 基本对齐 |

### 3.2 MCP 已迁移到 trpc

`tools/mcpmount/append.go` 已使用 `trpc.group/trpc-go/trpc-agent-go/tool/mcp`，ADK 依赖已清除。剩余待完善项：`tool/mcpbroker.Broker` 运行时发现、STDIO/SSE/StreamableHTTP 传输层、会话重连。

### 3.3 Session 管理现状

当前 Runner 的 SessionService 已通过 Wire 注入支持 SQLite 持久化（`session/trpc/sqlite.go:NewSQLiteSessionService`），不再始终使用 inmemory。`data/sessionmemory/` 包实现了 SQLite Store 用于 Memory 实体的存储。

### 3.4 Runner 能力缺口

当前 Runner 仅使用基础 `trpcrunner.NewRunner`，未使用：
- `ManagedRunner`（Cancel/RunStatus）
- `SteerableRunner`（EnqueueUserMessage）
- `runner.WithPlugins`
- `runner.WithArtifactService`

### 3.5 Tool 框架能力审计（2026-05-15）

对 `pkg/trpc-agent-go/tool/` 逐文件审计发现以下框架能力此前未被项目集成：

| 框架文件 | 框架能力 | 集成状态 |
|----------|----------|----------|
| `tool/callbacks.go` | BeforeTool/AfterTool 结构化回调链 + panic 恢复 + ContinueOnError | ✅ 已集成（buildToolCallbacks + WithToolCallbacks） |
| `tool/filter.go` | FilterFunc + FilterTools + FilterToolSet + Include/Exclude 名称过滤器 | ✅ 已集成（buildToolFilter + NewExcludeToolNamesFilter） |
| `tool/retry.go` | RetryPolicy + 指数退避 + Jitter + DefaultRetryOn | ✅ 已集成（buildToolRetryPolicy + DB字段 tools_retry_*） |
| `tool/merge.go` | Merge 函数（字符串/数字/切片/Map/结构体/Mergeable） | ⚠️ 未集成（无业务场景需要合并多工具结果） |
| `tool/stream.go` | Stream/StreamReader/StreamWriter + StreamChunk + FinalResultChunk | ⚠️ 预留字段（tools_streaming_enabled），待有 StreamableCall 实现后启用 |
| `tool/context.go` | ToolCallID 传播 + 结构化错误流 + FinalResult 标记 | ⚠️ 未显式使用（框架内部自动处理） |
| `tool/agent_tool.go` | AgentTool（Agent 包装为 Tool）+ HistoryScope + ResponseMode | ⚠️ 未集成（Team 编排使用框架内置 TransferTool） |
| `tool/codeexecutor_tool.go` | CodeExecutionTool + 语言验证 + 结果处理 | ✅ 已集成（通过 llmagent.WithCodeExecutor） |
| `tool/transfer_tool.go` | TransferTool（Agent 间转移）+ 上下文保持 | ✅ 已集成（Team 编排中自动注入） |

**框架 Tool 子包完整对照**（2026-05-15 补充）：

| 框架子包 | 工具类型 | Registry 注册名 | 集成方式 |
|----------|----------|-----------------|----------|
| `tool/agent/` | Agent-as-Tool | `agent` | ✅ Registry+Assemble（AgentToolConfig） |
| `tool/arxivsearch/` | Arxiv 搜索 ToolSet | `arxiv_search` | ✅ Registry+Assemble |
| `tool/awaitreply/` | 等待用户回复 | `await_user_reply` | ✅ Registry+Assemble |
| `tool/claudecode/` | Claude Code ToolSet | `claudecode` | ✅ Registry+Assemble |
| `tool/codeexec/` | 代码执行 | — | ✅ 通过 llmagent.WithCodeExecutor 注入 |
| `tool/duckduckgo/` | DuckDuckGo 搜索 | `duckduckgo` | ✅ Registry+Assemble |
| `tool/email/` | 邮件 ToolSet | `email` | ✅ Registry+Assemble |
| `tool/file/` | 文件操作 ToolSet | `file` | ✅ Registry+Assemble |
| `tool/function/` | FunctionTool 泛型 | — | ✅ custom/demo.go 使用 |
| `tool/google/search/` | Google 搜索 ToolSet | `google_search` | ✅ Registry+Assemble |
| `tool/hostexec/` | 主机命令执行 ToolSet | `hostexec` | ✅ Registry+Assemble |
| `tool/mcp/` | MCP 协议 ToolSet | `mcp` | ✅ Registry+Assemble（MCPServerConfig + trpc_build.go 接通） |
| `tool/mcpbroker/` | MCP 服务发现 | `mcpbroker` | ✅ Registry+Assemble（MCPBrokerConfig + trpc_build.go 接通） |
| `tool/openapi/` | OpenAPI Spec ToolSet | `openapi` | ✅ Registry+Assemble |
| `tool/skill/` | Skill 加载/执行/文档 | — | ✅ 通过 llmagent.WithSkills 注入 |
| `tool/todo/` | Todo 管理 | `todo` | ✅ Registry+Assemble |
| `tool/transfer/` | Agent 转移工具 | — | ✅ Team 编排自动注入 |
| `tool/webfetch/claudefetch/` | Claude 网页抓取 | `claudefetch` | ⚠️ 框架空壳包（无导出函数），Registry 占位 |
| `tool/webfetch/geminifetch/` | Gemini 网页抓取 | `geminifetch` | ✅ Registry+Assemble |
| `tool/webfetch/httpfetch/` | HTTP 网页抓取 | `httpfetch` | ✅ Registry+Assemble |
| `tool/wikipedia/` | Wikipedia 搜索 | `wikipedia` | ✅ Registry+Assemble |
| `tool/workspaceexec/` | 工作区执行 | `workspace_exec` | ✅ Registry+Assemble |

**目录重构**（2026-05-15）：学习 trpc 框架目录结构，在 `internal/tools/` 根目录建立：
- `tool.go` — 项目级接口定义（type alias 到 trpc-agent-go/tool）
- `toolset.go` — 中央注册表 `Registry()` + 总入口 `Assemble(ctx, AssemblyConfig)`
- `trpc/toolsets.go` — 向后兼容适配器（ToolsetConfig → AssemblyConfig → tools.Assemble）

**教训**：后续分析框架时必须逐文件扫描，建立 `[文件] → [能力] → [项目集成状态]` 矩阵，避免遗漏。

### 3.6 S1 架构加固成果（2026-05-17）

Sprint 1 完成了以下架构加固，已合并到主干：

| 任务 | 变更 | 验收 |
|------|------|------|
| T1 单 SQLite 连接池 | `data.RawDB()` 共享底层连接池；session/graph trpc 适配器接收注入的 `*sql.DB` | `grep -rn "sql.Open" internal/ \| grep -v data/data.go` 空 |
| T2 WS 接入 Kratos | `WSServer.RegisterOnKratos(srv)` 挂载到 Kratos HTTP；进程仅监听 8000+9000 | 无独立 :8002 进程 |
| T3 biz 去框架 envelope | `biz/domain_event.go` 定义纯业务 DomainEvent；`biz/domain_event_adapter.go` 提供投影适配 | `go list -deps .../biz/... \| rg trpc-agent-go` 空 |
| T4 biz 去框架 graph | `biz/graph.go` 直接定义 `StateFieldDef`/`NodeDef`/`GraphBuildConfig` 等业务类型（无 trpc 引用）；`biz/graph_runtime.go` 定义 `GraphBuilderFactory` 纯接口；`adapter/graph/runtime_adapter.go` 实现 bizCfgToTrpc/trpcCfgToBiz 双向转换 | `go list -deps .../biz/... \| rg trpc-agent-go` 空 |
| T5 Memory cache 修复 | `sqlite_adapter.go` 删除进程内缓存，所有读写直通 SQLite Store | 跨 turn 记忆可见 |
| T6 Graph builder race 修复 | `BuildStateGraphWithRegistry` 函数内深拷贝入参切片 | `go test -race ./internal/graph/...` 通过 |
| T7 Graph executions GC | `gcLoop` 每 5min 清理超过 30min 的完成态执行 | 进程内存稳定 |
| T8 EventBus 可靠投递 | `reliableTypes` 6 类关键事件阻塞最多 100ms 再投递 | tool_result 不被 text_delta 覆盖 |
| T9 panic recover | `pkg/safego.Go` 统一 recover；goroutine 内 panic 不中断进程 | 单测通过 |
| T10 ctx 修复 | `loadEffectiveToolKeys` 使用 turn ctx 而非 Background | ctx 取消可传播 |
| T11 前端 graph 客户端 | `make api` 生成 `web/src/services/kratos/graph/v1/index.ts` | `pnpm build` 通过 |
| T12 前端 chat 客户端 | `features/chat/api.ts` 使用 `createChatService()` | 无裸 `kratosApi.post` |

---

## 四、实施阶段

### 阶段一：核心运行时完善（P0-P1）

**目标**：让 Agent 的构建、运行、编排能力完全对齐框架

| 步骤 | 模块 | 关键任务 | 涉及文件 | 验收标准 |
|------|------|----------|----------|----------|
| 1.1 | M2 Agent构建 | ~~1. 启用占位符变量替换（`{key}` 在 Instruction 中被正确替换）~~ ✅（框架内置）<br>~~2. 集成 ContextCompaction（长对话自动压缩）~~ ✅<br>~~3. 集成 SessionSummary（会话摘要注入）~~ ✅ | `agent/trpc_build.go`<br>`agent/prompt.go`<br>`biz/session_summary.go` | ✅ 占位符由框架渲染；长对话自动压缩；会话摘要注入 |
| 1.2 | M5 Session | ~~1. 实现 `session/trpc/sqlite.go` SQLite SessionService 适配器（基于 `session/sqlite.NewSessionService`）~~ ✅<br>~~2. Wire 注入 SQLite SessionService 替换 inmemory~~ ✅<br>~~3. 集成摘要压缩（已有 `service/session_compress.go`）~~ ✅ | `session/trpc/sqlite.go`<br>`service/trpc_turn.go`<br>`cmd/admin/wire.go` | ✅ Session 持久化到 SQLite；重启后会话不丢失；长对话自动摘要 |
| 1.3 | M20 Runner | ~~1. 升级为 ManagedRunner（支持 Cancel/RunStatus）~~ ✅<br>~~2. 升级为 SteerableRunner（支持 EnqueueUserMessage）~~ ✅<br>3. 集成 PluginManager（预留接口）<br>4. 集成 SessionIngestor | `agent/trpc_runtime.go`<br>`service/chat.go`<br>`service/trpc_turn.go` | ✅ Runner 支持取消运行中请求；支持排队消息；支持运行状态查询 |
| 1.4 | M3 Team | ~~1. 集成 TransferTool（Agent 间转移控制权）~~ ✅<br>~~2. 集成 crossRequestTransfer（跨请求上下文转移）~~ ✅<br>~~3. 集成 SwarmHandoffInput（自定义转移输入）~~ ✅ | `team/trpc_build.go`<br>`team/runner.go` | ✅ Swarm 模式支持 transfer_to_agent；跨请求保持 Agent 上下文 |
| 1.5 | M4 Graph | ~~1. 实现 HITL（人机中断：interrupt_before/interrupt_after）~~ ✅<br>~~2. 实现检查点（Checkpoint 持久化/恢复）~~ ✅（InMemory）<br>~~3. 实现子图（Subgraph 嵌套）~~ ✅<br>~~4. 实现 DAG 并行执行器~~ ✅<br>~~5. 暴露 Graph API 端点~~ ✅（15个RPC）<br>~~6. 实现 State Schema + Reducer~~ ✅<br>~~7. 实现 Node Func 注册表（运行时解析）~~ ✅<br>~~8. 实现 ResumeExecution 集成 trpc Executor.Resume~~ ✅<br>~~9. 实现 TimeTravel API 集成框架 GetState/History/EditState~~ ✅<br>~~10. 补充 UpdateGraph RPC + 实现~~ ✅<br>~~11. 实现 EventBridge 事件桥接~~ ✅<br>~~12. 实现 Checkpoint API（ListCheckpoints/GetStateSnapshot/EditState）~~ ✅<br>~~13. 实现 DOT 可视化增强（结构化 JSON）~~ ✅<br>~~14. 前端 Vue Flow 编辑器~~ ✅<br>~~15. 前端执行监控（节点状态高亮 + useGraphStream）~~ ✅<br>~~16. v2 四维架构需求/设计文档重构~~ ✅<br>~~17. Checkpoint SQLite 持久化~~ ✅（`graph/trpc/checkpoint.go:NewSQLiteCheckpointSaver`，通过 Wire 注入 `*sql.DB`）<br>~~18. 设计时校验引擎（ValidateGraph）~~ ✅（`graph/trpc/validator.go:ValidateGraph`，入度/出度/循环/Agent引用/FuncRef检测）<br>19. Agent 引用校验 ❌P0待实现<br>20. 设计模式模板（6种内置模板） ❌P1待实现<br>21. 节点属性配置完善（LLM/Tool/Agent面板） ❌P1待实现<br>22. State Schema 校验 ❌P1待实现<br>23. 执行摘要与时间线 ❌P1待实现<br>24. 子图复用 ❌P1待实现 | `graph/trpc/builder.go`<br>`graph/trpc/registry.go`<br>`graph/trpc/event_bridge.go`<br>`graph/trpc/visualize.go`<br>`graph/trpc/checkpoint.go`<br>`api/kratos/graph/v1/graph.proto`<br>`internal/biz/graph.go`<br>`internal/data/graph.go`<br>`internal/service/graph.go`<br>`internal/server/register_graph.go`<br>`web/src/features/graph/` | ✅ HITL 中断/恢复；✅ 检查点恢复（SQLite + InMemory）；✅ 子图嵌套；✅ DAG 引擎；✅ API 端点（15个RPC）；✅ State Schema/Reducer；✅ Node Func 注册表；✅ TimeTravel API；✅ EventBridge；✅ 可视化增强；✅ 前端编辑器/监控；✅ v2四维架构设计；✅ Checkpoint SQLite 持久化；✅ ValidateGraph 校验引擎；❌ 模板/节点配置/执行摘要待实现 |
| 1.6 | M7 Tool | ~~1. 统一工具注册到 trpc Tool 接口（消除 ADK 残留）~~ ✅<br>~~2. 集成 FunctionTool（`tool/function`）~~ ✅<br>~~3. 集成全部框架 ToolSet: arxivsearch/awaitreply/claudecode/email/geminifetch/google-search/openapi/todo/wikipedia/workspaceexec~~ ✅<br>~~4. 集成工具重试策略~~ ✅（buildToolRetryPolicy + DB字段 tools_retry_* + 前端UI）<br>~~5. 集成工具过滤（allow/deny）~~ ✅（buildToolFilter + NewExcludeToolNamesFilter）<br>~~6. 集成工具回调（BeforeTool/AfterTool）~~ ✅（buildToolCallbacks + WithToolCallbacks）<br>~~7. 集成并行工具调用~~ ✅（WithEnableParallelTools + DB字段 tools_parallel_enabled + 前端UI）<br>8. 集成流式工具（StreamableTool）— 预留字段 tools_streaming_enabled，待有 StreamableCall 实现后启用 | `tools/tool.go`<br>`tools/toolset.go`<br>`tools/trpc/toolsets.go`<br>`agent/trpc_build.go`<br>`internal/data/ent/schema/`<br>`web/src/pages/AgentSettingsPage.vue` | ✅ 工具注册/ToolSet/重试/过滤/回调/并行已集成；⚠️ 流式工具待框架侧有 StreamableCall 实现后启用 |
| 1.7 | M18 Event | ~~1. WebSocket 推流包含 StateDelta~~ ✅<br>~~2. WebSocket 推流包含 Extensions~~ ✅<br>~~3. WebSocket 推流包含 FilterKey/Branch~~ ✅<br>~~4. WebSocket 推流包含 Tag~~ ✅ | `server/ws.go`<br>`service/trpc_turn.go`<br>`team/runner_team_trpc.go` | ✅ WebSocket 推流包含完整事件元数据；前端可按 Branch/Tag 过滤事件 |

### 阶段二：能力扩展（P2）

**目标**：补齐 Memory、MCP、Plugin、Planner、Artifact、CodeExecutor、Callback、Gateway

| 步骤 | 模块 | 关键任务 | 涉及文件 | 验收标准 |
|------|------|----------|----------|----------|
| 2.1 | M6 Memory | 1. 实现 `EnqueueAutoMemoryJob`（集成 `memory/extractor` 自动提取）<br>2. 集成 `memory/tool` 完整工具链（memory_add/search/load/update/delete）<br>3. 实现 pgvector 向量搜索后端<br>4. 集成 Mem0 平台适配 | `memory/trpc/sqlite_adapter.go`<br>`memory/trpc/pgvector_adapter.go`<br>`memory/trpc/extractor.go` | 对话后自动提取记忆；Agent 可调用 memory_search；支持向量相似度搜索 |
| 2.2 | M8 MCP | ~~1. 迁移 `tools/mcpmount/` 从 ADK 到 trpc `tool/mcp.ToolSet`~~ ✅<br>2. 集成 `tool/mcpbroker.Broker`（运行时发现）<br>3. 实现 STDIO/SSE/StreamableHTTP 传输<br>4. 实现会话重连 | `tools/mcpmount/append.go`<br>`tools/mcpmount/broker.go`<br>`tools/mcpmount/transport.go` | Agent 可通过 MCP Broker 动态发现和调用远程 MCP 工具；MCP 传输层完整支持 |
| 2.3 | M9 Model | 1. 集成 Bedrock 适配器<br>2. 验证 Gemini/Anthropic/Ollama 完整功能<br>3. Failover/Hedge 已对齐，补充集成测试 | `provider/trpc_llm.go`<br>`provider/catalog.go` | 多模型自动切换；token 超限自动裁剪；Bedrock 可用 |
| 2.4 | M10 Plugin | 1. 实现 `plugin.Plugin` 接口适配器<br>2. Runner 级生命周期钩子（BeforeModel/AfterTool/OnEvent）<br>3. PluginManager 注册与调度<br>4. 日志插件和全局指令插件 | `agent/plugin.go`<br>`biz/plugin.go`<br>`agent/trpc_runtime.go` | BeforeModel/AfterTool/OnEvent 钩子生效；可动态注册/卸载插件 |
| 2.5 | M11 Planner | 1. 集成 ReActPlanner（标签约束规划）<br>2. 集成 A2UIPlanner（JSONL 协议规划）<br>3. 在 Agent 设置中暴露 Planner 选择 | `agent/trpc_build.go`<br>`agent/planner.go` | 复杂任务先规划再执行；用户可选择 Planner 类型 |
| 2.6 | M12 Artifact | 1. 集成 `artifact.Service`<br>2. 实现 InMemory 后端<br>3. 实现 S3/COS 后端<br>4. 版本管理 + ListArtifactKeys | `artifact/trpc/service.go`<br>`artifact/trpc/s3.go`<br>`artifact/trpc/cos.go` | Agent 可保存/加载/列出制品；支持 S3/COS 存储 |
| 2.7 | M14 CodeExecutor | 1. 集成 E2B 沙箱执行器<br>2. 集成 Jupyter 执行器<br>3. 集成 Container 执行器<br>4. Interactive 模式 | `skill/trpc/executor.go`<br>`skill/trpc/executor_e2b.go`<br>`skill/trpc/executor_jupyter.go` | 代码在沙箱中执行并返回结果；支持 E2B/Jupyter/Container |
| 2.8 | M19 Callback | ~~1. 实现 ToolCallbacks（BeforeTool/AfterTool）~~ ✅（buildToolCallbacks 已集成）<br>2. 实现 BeforeAgent/AfterAgent StructuredCallback<br>3. 实现 ModelCallbacks（BeforeModel/AfterModel）<br>4. 与 Plugin 系统协调 | `agent/callback.go`<br>`agent/trpc_build.go` | ✅ ToolCallbacks 已集成；⚠️ Agent/Model 级回调待实现 |
| 2.9 | M16 Gateway | 1. 实现会话并发控制（基于 ManagedRunner）<br>2. 实现 status/cancel API<br>3. 实现 AwaitUserReply（基于 SteerableRunner）<br>4. 实现 QueuedUserMessage | `service/chat.go`<br>`service/trpc_turn.go`<br>`api/kratos/chat/v1/chat.proto` | 支持中断/恢复；支持排队消息；支持运行状态查询 |

### 阶段三：超越层（P3）

**目标**：在复刻基础上增加框架不具备的产品层能力

| 步骤 | 模块 | 关键任务 | 涉及文件 | 验收标准 |
|------|------|----------|----------|----------|
| 3.1 | M13 Knowledge | 1. 集成 `knowledge.Knowledge` + OCR + RAG<br>2. 实现 AgenticFilter + SearchFilter<br>3. 超越：多租户知识库隔离 | `knowledge/trpc/service.go`<br>`knowledge/trpc/rag.go`<br>`data/pgvector/` | Agent 可搜索知识库；不同租户知识隔离 |
| 3.2 | M15 A2A | 1. 集成 `a2aagent.A2AAgent` + A2AServer<br>2. 实现 AgentCard + 流式 + DataPart映射<br>3. 实现 GraphResume<br>4. 超越：A2A 网关注册中心 | `a2a/trpc/agent.go`<br>`a2a/trpc/server.go`<br>`a2a/trpc/registry.go` | Agent 可通过 A2A 协议与其他 Agent 通信 |
| 3.3 | M17 Evaluation | 1. 集成 `evaluation.AgentEvaluator`<br>2. 实现 EvalSet + Metric + LLM-as-Judge<br>3. 实现 UserSimulation + MultiRun<br>4. 超越：可视化评估平台 + A/B 测试 | `evaluation/trpc/evaluator.go`<br>`evaluation/trpc/metrics.go` | 可对 Agent 进行自动化评估 |
| 3.4 | 超越-可视化编排 | Graph 可视化编辑器（拖拽节点/边） | 前端 `web/src/features/graph/` | 前端可拖拽构建 Graph 工作流 |
| 3.5 | 超越-多租户 | 全模块多租户隔离（Session/Memory/Knowledge/Artifact） | `biz/tenant.go` + 各模块适配 | 不同租户数据完全隔离 |
| 3.6 | 超越-审计 | 全链路审计日志（Agent调用/Tool调用/Memory变更） | `biz/audit.go` + `data/audit.go` | 可追溯任何操作的完整链路 |
| 3.7 | 超越-可观测 | OpenTelemetry 集成 + Metrics + Trace Dashboard | `internal/telemetry/` | 可在 Grafana 查看 Agent 运行指标 |

---

## 五、模块详细计划索引

每个模块的详细需求、现状分析、trpc框架参照、具体步骤、涉及文件、验收标准，
均记录在 `docs/需求/` 下对应文档中：

| 模块 | 需求文档 | 状态 |
|------|----------|------|
| M1 Skill 运行时 | `20 skill.md` + `20 skill struct design.md` | ✅ 已对齐，需补充 SkillLoadMode/PromptCache 细节 |
| M2 Agent 构建 | `2 agents-create.md` + `4.agent-type.md` + `5 agent-setting.md` | 已有，已补充占位符/ContextCompaction/SessionSummary（§8） |
| M3 Team 编排 | `11 multi-agent.md` + `team.md` | 已有，已补充 crossRequestTransfer/SwarmHandoff（§15） |
| M4 Graph 工作流 | `graph-workflow.md` + `graph-workflow.design.md` | ✅ 已创建（v2四维架构重构完成） |
| M5 Session 管理 | `10 session.md` | 已有，已补充 trpc session 集成/多后端/摘要压缩（§12） |
| M6 Memory 记忆 | `memory.md` + `12-16 memory-L*.md` + `31 memery.md` | 已有，需补充自动提取/Mem0/向量搜索 |
| M7 Tool 工具体系 | `23 tools.md` + `23 tools struct design.md` | 已有，已补充流式/重试/过滤/ToolSet（§15） |
| M8 MCP 集成 | `19 mcp.md` | 已有，已补充 MCPBroker/运行时发现（§10） |
| M9 Model 模型层 | `9 provider.md` | 已有，已补充 Failover/Hedge/多模型（§12） |
| M10 Plugin 插件 | `22 plugin.md` | 已有，已补充 Plugin 接口/生命周期钩子（§11） |
| M11 Planner 规划 | `planner.md` | ✅ 已创建 |
| M12 Artifact 制品 | `artifact.md` | ✅ 已创建 |
| M13 Knowledge 知识库 | `knowledge.md` | ✅ 已创建 |
| M14 CodeExecutor | `codeexecutor.md` | ✅ 已创建 |
| M15 A2A 协议 | `a2a-protocol.md` | ✅ 已创建 |
| M16 Gateway 网关 | `gateway.md` | ✅ 已创建 |
| M17 Evaluation 评估 | `evaluation.md` | ✅ 已创建 |
| M18 Event 事件 | `event-system.md` | ✅ 已创建 |
| M19 Callback 回调 | `callback.md` | ✅ 已创建 |
| M20 Runner 运行器 | `runner.md` | ✅ 已创建 |

---

## 六、架构原则

1. **框架即内核**：trpc-agent-go 是运行时内核，项目是产品壳层，通过适配器桥接
2. **适配器模式**：每个模块通过 `internal/{module}/trpc/` 适配器桥接框架接口
3. **渐进迁移**：新功能直接使用 trpc 接口，旧功能通过适配器逐步迁移
4. **产品层增值**：多租户、权限、审计、可视化等是产品层能力，不修改框架代码
5. **测试先行**：每个适配器必须有单元测试，验证接口契约
6. **ADK 残留清理**：所有 `google.golang.org/adk` import 必须逐步替换为 `trpc.group/trpc-go/trpc-agent-go` 对应包

---

## 七、依赖关系

```
M2 Agent构建 ← M1 Skill运行时
M3 Team编排  ← M2 Agent构建 + M7 Tool
M4 Graph工作流 ← M2 Agent构建
M5 Session   ← M20 Runner
M6 Memory    ← M5 Session + M9 Model
M8 MCP       ← M7 Tool
M10 Plugin   ← M20 Runner + M19 Callback
M12 Artifact ← M20 Runner + M14 CodeExecutor
M13 Knowledge ← M9 Model + M6 Memory
M15 A2A      ← M2 Agent构建 + M4 Graph
M17 Evaluation ← M20 Runner + M9 Model
M16 Gateway  ← M20 Runner + M5 Session
```

**关键路径**：M2 → M20 → M5 → M6 → M13（Agent构建 → Runner → Session → Memory → Knowledge）

**阶段一内部执行顺序**（考虑依赖）：
1. M2 Agent构建（1.1）→ 无前置依赖
2. M5 Session（1.2）→ 依赖 M20 Runner 基础（已有）
3. M20 Runner（1.3）→ 依赖 M5 Session（1.2）
4. M3 Team（1.4）→ 依赖 M2 Agent构建（1.1）+ M7 Tool（1.6）
5. M4 Graph（1.5）→ 依赖 M2 Agent构建（1.1）
6. M7 Tool（1.6）→ 依赖 M2 Agent构建（1.1）
7. M18 Event（1.7）→ 依赖 M20 Runner（1.3）

**建议执行顺序**：1.1 → 1.6 → 1.2 → 1.3 → 1.4 → 1.5 → 1.7

---

## 八、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| trpc-agent-go 接口不稳定 | 适配器频繁修改 | 锁定 go.mod 版本；接口变更时评估影响范围 |
| 多后端 Session 迁移 | 数据丢失 | 先实现 SQLite，逐步增加 Redis/PG；提供迁移工具 |
| Memory 自动提取质量 | 提取无关记忆 | 提供提取 prompt 可配置；增加 checker 机制 |
| Graph 可视化复杂度 | 开发周期长 | 先实现 API 端点，可视化作为超越层 |
| A2A 协议兼容性 | 与外部 Agent 通信失败 | 严格遵循 A2A 规范；增加兼容性测试 |
| ADK 残留清理 | MCP 等模块迁移风险 | 逐模块迁移，保持 ADK 和 trpc 双轨运行直到验证完毕 |
| Session 从 inmemory 切换到 SQLite | 运行中会话丢失 | 提供平滑迁移路径；新会话用 SQLite，旧会话保持 inmemory |

---

## 九、变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-14 | 基于代码审计重新梳理：更新 M5/M6/M8/M9/M16 对齐状态；新增第三节"代码审计发现"；细化实施步骤涉及文件和验收标准；补充 ADK 残留清理原则；调整阶段一执行顺序 |
