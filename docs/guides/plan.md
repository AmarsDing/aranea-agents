# Aranea Agents — trpc-agent-go 功能对齐与超越计划

> 本文档是项目对标 `pkg/trpc-agent-go` 框架的总体大纲，目标是 **完全复刻** 框架能力并 **超越**。
> 每个模块的详细需求见 `docs/需求/` 下对应文档。

---

## 一、总体目标

| 维度 | 目标 |
|------|------|
| **复刻** | 将 trpc-agent-go 框架的全部核心能力集成到项目中，不留功能盲区 |
| **超越** | 在复刻基础上增加产品层能力（多租户、权限、审计、可视化编排、评估平台），形成框架+平台的双层架构 |
| **架构** | 项目作为产品壳层，trpc-agent-go 作为运行时内核；产品层通过适配器桥接框架接口 |

---

## 二、功能对齐总览

| # | 模块 | trpc-agent-go 框架能力 | 项目现有实现 | 对齐状态 | 优先级 | 需求文档 |
|---|------|----------------------|-------------|---------|--------|----------|
| M1 | Skill 运行时 | `skill.Repository` + `tool/skill/{load,run,list_docs,select_docs}` + 渐进披露 + 工作区执行 + SkillLoadMode + PromptCache | `skill/trpc/repository.go` + `skill/trpc/tools.go` + `skill/trpc/executor.go` + `skill/trpc/filter.go` | ✅ 已对齐 | P0 | `20 skill.md` |
| M2 | Agent 构建 | `llmagent.New` + 占位符变量 + ModelInstructions + Planner + SkillLoadMode + ContextCompaction + SessionSummary | `agent/trpc_build.go` + `agent/trpc_runtime.go` | ⚠️ 部分实现 | P0 | `2 agents-create.md` |
| M3 | Team 编排 | `team.NewCoordinator` / `team.NewSwarm` + AgentTool + TransferTool + crossRequestTransfer + SwarmHandoffInput | `team/trpc_build.go` + `team/runner.go` | ⚠️ 部分实现 | P1 | `11 multi-agent.md` |
| M4 | Graph 工作流 | `graph.StateGraph` + 节点/边/条件路由 + HITL + 检查点 + 时间旅行 + 子图 + DAG引擎 + 可视化 | `graph/trpc/builder.go` | ⚠️ 部分实现 | P1 | `graph-workflow.md` |
| M5 | Session 管理 | `session.Service` + SQLite/Redis/PG/MySQL/ClickHouse + 摘要压缩 + Event分页 + Track + Ingestor | `session/trpc/sqlite.go` (仅inmemory) | ❌ 严重不足 | P1 | `10 session.md` |
| M6 | Memory 记忆 | `memory.Service` + 自动提取(Auto) + 工具驱动(Agentic) + Mem0 + 多后端(SQLite/PG/Redis/MySQL/pgvector) + 向量搜索 | `memory/trpc/sqlite_adapter.go` (基础CRUD) | ❌ 严重不足 | P2 | `memory.md` + `12-16` |
| M7 | Tool 工具体系 | `tool.Tool`/`CallableTool`/`StreamableTool` + FunctionTool + MCP + 流式 + 重试 + 过滤 + ToolSet + 并行 | `tools/trpc/toolsets.go` (基础filesystem+shell) | ⚠️ 部分实现 | P1 | `23 tools.md` |
| M8 | MCP 集成 | `tool/mcp.ToolSet` + `tool/mcpbroker.Broker` + STDIO/SSE/StreamableHTTP + 运行时发现 + 会话重连 | `tools/mcpmount/` + `biz/mcp_server.go` | ⚠️ 部分实现 | P2 | `19 mcp.md` |
| M9 | Model 模型层 | `model.Model` + OpenAI/Gemini/Anthropic/Ollama/Bedrock/Hunyuan/HuggingFace + Failover/Hedge + TokenTailor | `provider/trpc_llm.go` + `provider/catalog.go` | ⚠️ 部分实现 | P2 | `9 provider.md` |
| M10 | Plugin 插件 | `plugin.Plugin` + Runner级生命周期 + BeforeModel/AfterTool/OnEvent + PluginManager | `biz/plugin.go` (仅CRUD) | ❌ 未实现 | P2 | `22 plugin.md` |
| M11 | Planner 规划 | `planner.BuiltinPlanner` / `planner.ReActPlanner` / `planner.A2UIPlanner` + 思考链 | `agent/trpc_build.go` (仅Builtin) | ⚠️ 部分实现 | P2 | `planner.md` |
| M12 | Artifact 制品 | `artifact.Service` + S3/COS/InMemory + 版本管理 + ListArtifactKeys | 无 | ❌ 未实现 | P2 | `artifact.md` |
| M13 | Knowledge 知识库 | `knowledge.Knowledge` + OCR + Query + RAG + 分块 + Source + AgenticFilter + SearchFilter | 无 | ❌ 未实现 | P3 | `knowledge.md` |
| M14 | CodeExecutor | `codeexecutor.CodeExecutor` + Local/E2B/Jupyter/Container + WorkspaceRegistry + Interactive | `skill/trpc/executor.go` (仅Local) | ⚠️ 部分实现 | P2 | `codeexecutor.md` |
| M15 | A2A 协议 | `a2aagent.A2AAgent` + A2AServer + AgentCard + 流式 + DataPart映射 + GraphResume | 无 | ❌ 未实现 | P3 | `a2a-protocol.md` |
| M16 | Gateway 网关 | `runner.Runner` + HTTP webhook + 会话并发控制 + status/cancel + AwaitUserReply + QueuedUserMessage | `server/sse.go` + `service/chat_native.go` | ⚠️ 部分实现 | P2 | `gateway.md` |
| M17 | Evaluation 评估 | `evaluation.AgentEvaluator` + EvalSet + Metric + LLM-as-Judge + UserSimulation + MultiRun | 无 | ❌ 未实现 | P3 | `evaluation.md` |
| M18 | Event 事件 | `event.Event` + 流式 + 标签 + StateDelta + Extensions + FilterKey + Branch + Clone + Actions | `server/sse.go` (SSE投影) | ⚠️ 部分实现 | P2 | `event-system.md` |
| M19 | Callback 回调 | `agent.Callbacks` + BeforeAgent/AfterAgent + StructuredCallback + ModelCallbacks + ToolCallbacks | 无 | ❌ 未实现 | P2 | `callback.md` |
| M20 | Runner 运行器 | `runner.Runner` + AgentFactory + PluginManager + ArtifactService + SessionIngestor + AwaitUserReply | `agent/trpc_runtime.go` (基础) | ⚠️ 部分实现 | P1 | `runner.md` |

---

## 三、实施阶段

### 阶段一：核心运行时完善（P0-P1）

**目标**：让 Agent 的构建、运行、编排能力完全对齐框架

| 步骤 | 模块 | 关键任务 | 验收标准 |
|------|------|----------|----------|
| 1.1 | M2 Agent构建 | 启用占位符变量、ModelInstructions、ContextCompaction、SessionSummary | Agent Instruction 中 `{key}` 被正确替换；长对话自动压缩 |
| 1.2 | M5 Session | 集成 trpc `session/sqlite` 替换 inmemory；集成摘要压缩 | Session 持久化到 SQLite；长对话自动摘要 |
| 1.3 | M20 Runner | 完善 Runner：AgentFactory、PluginManager、ArtifactService | Runner 支持动态 Agent 创建和插件注入 |
| 1.4 | M3 Team | 使用 trpc `team.NewCoordinator`/`team.NewSwarm`；集成 AgentTool + TransferTool + crossRequestTransfer | Coordinator/Swarm 模式通过 trpc team 包运行 |
| 1.5 | M4 Graph | 完善 Graph：条件路由、HITL、检查点、子图、API端点 | 能通过 API 定义并执行 Graph 工作流 |
| 1.6 | M7 Tool | 迁移到 trpc Tool 接口；集成 FunctionTool、流式工具、工具重试、工具过滤 | 所有内置工具通过 trpc Tool 接口注册 |
| 1.7 | M18 Event | 完善 Event：StateDelta、Extensions、FilterKey、Branch、Actions | SSE 推流包含完整事件元数据 |

### 阶段二：能力扩展（P2）

**目标**：补齐 Memory、MCP、Model、Plugin、Planner、Artifact、CodeExecutor、Callback、Gateway

| 步骤 | 模块 | 关键任务 | 验收标准 |
|------|------|----------|----------|
| 2.1 | M6 Memory | 集成 trpc `memory.Service`；启用自动提取；集成记忆工具；支持 pgvector 向量搜索 | 对话后自动提取记忆；Agent 可调用 memory_search |
| 2.2 | M8 MCP | 集成 `tool/mcp.ToolSet` + `tool/mcpbroker.Broker`；运行时发现；会话重连 | Agent 可通过 MCP Broker 动态发现和调用远程 MCP 工具 |
| 2.3 | M9 Model | 集成 Failover/Hedge；支持 Gemini/Anthropic/Ollama/Bedrock；TokenTailor | 多模型自动切换；token 超限自动裁剪 |
| 2.4 | M10 Plugin | 实现 `plugin.Plugin` 接口；Runner 级生命周期钩子 | BeforeModel/AfterTool/OnEvent 钩子生效 |
| 2.5 | M11 Planner | 集成 ReActPlanner、A2UIPlanner | 复杂任务先规划再执行 |
| 2.6 | M12 Artifact | 集成 `artifact.Service`；S3/COS 后端；版本管理 | Agent 可保存/加载/列出制品 |
| 2.7 | M14 CodeExecutor | 集成 E2B/Jupyter/Container 执行器；Interactive 模式 | 代码在沙箱中执行并返回结果 |
| 2.8 | M19 Callback | 实现 BeforeAgent/AfterAgent StructuredCallback；ModelCallbacks；ToolCallbacks | 回调在 Agent/Model/Tool 各阶段正确触发 |
| 2.9 | M16 Gateway | 完善 Runner：会话并发控制、status/cancel、AwaitUserReply、QueuedUserMessage | 支持中断/恢复、排队消息 |

### 阶段三：超越层（P3）

**目标**：在复刻基础上增加框架不具备的产品层能力

| 步骤 | 模块 | 关键任务 | 验收标准 |
|------|------|----------|----------|
| 3.1 | M13 Knowledge | 集成 `knowledge.Knowledge` + OCR + RAG + AgenticFilter；超越：多租户知识库隔离 | Agent 可搜索知识库；不同租户知识隔离 |
| 3.2 | M15 A2A | 集成 `a2aagent.A2AAgent` + A2AServer；超越：A2A 网关注册中心 | Agent 可通过 A2A 协议与其他 Agent 通信 |
| 3.3 | M17 Evaluation | 集成 `evaluation.AgentEvaluator`；超越：可视化评估平台 + A/B 测试 | 可对 Agent 进行自动化评估 |
| 3.4 | 超越-可视化编排 | Graph 可视化编辑器（拖拽节点/边） | 前端可拖拽构建 Graph 工作流 |
| 3.5 | 超越-多租户 | 全模块多租户隔离（Session/Memory/Knowledge/Artifact） | 不同租户数据完全隔离 |
| 3.6 | 超越-审计 | 全链路审计日志（Agent调用/Tool调用/Memory变更） | 可追溯任何操作的完整链路 |
| 3.7 | 超越-可观测 | OpenTelemetry 集成 + Metrics + Trace Dashboard | 可在 Grafana 查看 Agent 运行指标 |

---

## 四、模块详细计划索引

每个模块的详细需求、现状分析、trpc框架参照、具体步骤、涉及文件、验收标准，
均记录在 `docs/需求/` 下对应文档中：

| 模块 | 需求文档 | 状态 |
|------|----------|------|
| M1 Skill 运行时 | `20 skill.md` + `20 skill struct design.md` | 已有，需补充 SkillLoadMode/PromptCache 细节 |
| M2 Agent 构建 | `2 agents-create.md` + `4.agent-type.md` + `5 agent-setting.md` | 已有，已补充占位符/ContextCompaction/SessionSummary（§8） |
| M3 Team 编排 | `11 multi-agent.md` + `team.md` | 已有，已补充 crossRequestTransfer/SwarmHandoff（§15） |
| M4 Graph 工作流 | `graph-workflow.md` | ✅ 已创建 |
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

## 五、架构原则

1. **框架即内核**：trpc-agent-go 是运行时内核，项目是产品壳层，通过适配器桥接
2. **适配器模式**：每个模块通过 `internal/{module}/trpc/` 适配器桥接框架接口
3. **渐进迁移**：新功能直接使用 trpc 接口，旧功能通过适配器逐步迁移
4. **产品层增值**：多租户、权限、审计、可视化等是产品层能力，不修改框架代码
5. **测试先行**：每个适配器必须有单元测试，验证接口契约

---

## 六、依赖关系

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

---

## 七、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| trpc-agent-go 接口不稳定 | 适配器频繁修改 | 锁定 go.mod 版本；接口变更时评估影响范围 |
| 多后端 Session 迁移 | 数据丢失 | 先实现 SQLite，逐步增加 Redis/PG；提供迁移工具 |
| Memory 自动提取质量 | 提取无关记忆 | 提供提取 prompt 可配置；增加 checker 机制 |
| Graph 可视化复杂度 | 开发周期长 | 先实现 API 端点，可视化作为超越层 |
| A2A 协议兼容性 | 与外部 Agent 通信失败 | 严格遵循 A2A 规范；增加兼容性测试 |
