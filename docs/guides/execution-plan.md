# Aranea-Agents 执行计划

> **状态真相源**：本文记录当前架构健康度、模块接入度和下一阶段任务。详细系统图见 `docs/需求/0 系统框图.md`，综合开发计划见 `docs/需求/0-system-development.md`。
>
> **更新时间**：2026-05-19（M3 收尾：文档与 MCP 会话统计闭环；M4 待启动）

## 当前结论

- 主链路已经可用：Chat / Agent / Team / Graph 通过 `trpc-agent-go` Runner 与 EventBus / WebSocket 串联。
- 架构红线基本保持：`internal/biz` 不直接 import `trpc-agent-go`，`internal/server` 不直接调用 Agent runtime。
- 当前优先级不是新增大模块，而是修复系统真理库、Runner/Gateway 状态机、Memory 双轨、Data 运行时耦合和前端模式分裂。

## 模块接入度

| 等级 | 模块 |
|------|------|
| 核心可用 | Chat(1)、Agent 创建/列表/分类/设置/文件/标题/头像(2-6/8/50)、Provider(9)、Session(10)、Skill(20)、Tools(23)、Cron(21)、Message/Event WS(51/34) |
| 可用但需闭环 | Team(11)、Graph(36)、MCP(19)、Plugin(22)、Callback(28)、Memory(12-16)、Knowledge(37)、Artifact(27)、Evaluation(33)、A2A(26)、Monitor/Telemetry/Token(18/24/29) |
| 早期/占位 | Evolution(7)、Channel 投递(17)、Ecosystem(30)、CLI 产品化(25)、TTS |

## Top 20 任务

| 排序 | 任务 | 模块 | 优先级 |
|------|------|------|--------|
| 1 | 收敛所有 Chat/Team/Monitor 主通道为 `/v1/ws`，清理旧 SSE 口径 | Message/Monitor/Team | P0 ✅ |
| 2 | 抽 `RunRegistry` / `RunnerManager`，承接 `ChatService` 的 active run 状态 | Gateway/Runner | P0 ✅ |
| 3 | 新增细粒度 `CancelRun` / `EnqueueUserMessage` RPC 或 WS 上行闭环 | Runner/Chat | P0（EnqueueUserMessage RPC + WS ✅；独立 CancelRun RPC 仍用 StopGeneration + WS cancel） |
| 4 | Runner 注入 ArtifactService / SessionIngestor / AgentFactory / AwaitUserReplyRouting | Runner | P1 ✅ |
| 5 | 统一 `trpc-agent-go/memory.Service` 与 Aranea L0-L4 的关系 | Memory | P0（`memory.RuntimeSet`：TRPC + Admin 端口 ✅） |
| 6 | 移除 service/agent 对 `internal/data/sessionmemory` 的直接依赖 | Memory | P0（service/agent/runtime 已改；data/memory 包内保留实现） |
| 7 | 将 trpc session / graph checkpoint provider 从 data 层上移 | Data/Graph/Session | P1 ✅ |
| 8 | 拆解 `biz` ↔ `provider` 依赖环 | Provider | P1 ✅ |
| 9 | 将 Skill Import 自定义 HTTP 路由迁入 proto + service | Skill | P1 ✅ |
| 10 | Team RunTeamTest 端到端实现 | Team | P1 ✅ |
| 11 | Team member_* WS Envelope 发射与前端展示 | Team/Message | P1（后端发射 ✅；前端已订阅） |
| 12 | ToolOverride 运行时生效、工具调用统计闭环 | Tools | P1（Override + TRPC 需确认 + 调用统计 ✅） |
| 13 | MCP timeout、重连验证、认证配置、MCPBroker 默认发现 | MCP | P2（timeout 60s ✅；session `mcp_call_count` 闭环 ✅；认证 UI + 运行时 header ✅；OAuth2 待做） |
| 14 | Plugin / Callback 全链路挂载 Agent / Model / Tool 生命周期 | Plugin/Callback | P1（Chain+Hook+OnEvent ✅；内置插件除 audit_log 外待实现） |
| 15 | Memory L4、MemoryWorker、冲突检测、级联更新 | Memory | P2（L4 prompt 注入 + MemoryWorker ✅；图写入/冲突检测待做） |
| 16 | Knowledge 管理页、摄取进度、Rerank/OCR 规划 | Knowledge | P2（管理页 + Embedder UI 可配置 + 摄取 WS ✅；Rerank/OCR 待做） |
| 17 | Artifact Runner 注入、预览、签名下载、Chat 附件关联 | Artifact | P2 ✅（Runner 注入 + 管理页 + Preview + 签名下载 + CodeExecutor 自动保存 + Chat 会话制品面板） |
| 18 | Evaluation 与框架 EvalSet 边界定稿，补前端页面 | Evaluation | P2（AgentEvaluator + MultiRun + LLMJudge + 管理页 ✅） |
| 19 | 前端 feature/store/mapper 统一，拆巨型 Chat / Agent 设置文件 | Frontend | P1（Knowledge/Evaluation/A2A/AgentSettings composable ✅；Chat 已薄） |
| 20 | Ecosystem 后端 API 与模板/插件/Skill 市场模型 | Ecosystem | P3 |

## 里程碑

### M0：文档与边界

- [x] 补 `docs/需求/0 系统框图.md`
- [x] 补 `docs/需求/0-system-development.md`
- [x] 补本文
- [x] 清理旧 SSE 主链路表述
- [x] 修复 Memory 断链与模块索引

### M1：Runner 与 Gateway

- [x] `RunRegistry`（`internal/runtime/run_registry.go`：active run、pending cancel、run status；`ChatService` 已接入）
- [x] `RunnerManager`（`internal/runtime/runner_manager.go`：统一 `NewTurnRunner` 装配；可选 `RegistryKey` 长生命周期实例；Chat/Team 已接入）
- [x] `EnqueueUserMessage` 对外入口（`POST /v1/chat/enqueue`；WS `enqueue_message`；`SendChatMessage` 在 active run 时优先 steerable enqueue）
- [x] 取消路径（HTTP `StopGeneration` + WS `cancel` → `RunRegistry.Cancel`；Team cancelable run 已登记）
- [x] ArtifactService 注入（`PersistenceSet.Artifact` → `TRPCRunnerDeps` → `WithArtifactService`；Wire `provideArtifactRuntimeService`）
- [x] GetRunStatus 与 `ManagedRunner.RunStatus` 对齐（Proto 扩展 + `RunRegistry.ActiveRunner` + 合并框架字段）
- [x] SessionIngestor 注入（`BizSessionIngestor` + `WithSessionIngestor`；外部摄入待扩展）
- [x] AgentFactory 注入（`BizAgentFactoryOptions` 按 agent_key 注册；Chat/Team turn 已接入）
- [x] AwaitUserReplyRouting 注入（`AwaitHook` 配置时启用 `WithAwaitUserReplyRouting`；与 ServiceTool `MarkAwaitingUserReply` 配合）
- [x] Chat / Team / Cron / Channel 共用 `RunGateway`（`RunRegistry` + `ChatService.RunGateway`；Cron/Channel 经 `RunNativeTurnUnary` / `RunCronTurn`）

### M2：架构边界康复

- [x] Memory 端口统一（`internal/memory.RuntimeSet` + `SessionAdminStore`；`PersistenceSet.Memory`；Wire `provideMemoryService`）
- [x] Data 运行时 provider 上移（`runtime.NewTRPCSessionService` / `NewGraphCheckpointSaver`；Wire `provideTRPCSessionService` / `provideGraphCheckpointSaver`）
- [x] Provider inspect 拆环（`internal/llminspect`；`provider.CatalogFromModel` 不再 import `biz`）
- [x] Skill Import service 化（`skill.proto` Get/Apply/Refine RPC；`SkillService` + multipart `RegisterSkillImportMultipart`）

### M3：模块闭环（✅ 主项已通，Rerank/OCR 与 MCP 认证/重连留待 P2）


- [x] Team RunTeamTest（临时 session + `team.Runner.RunTurn` + 清理）
- [x] CancelTeamRun 实际取消（`RunRegistry.Cancel` + 持久化 `cancelled`）
- [x] Team member_* WS Envelope（`ProjectMeta.MemberAgentKeys` + `EventProjector` 成员流）
- [x] ToolOverride 运行时（`ListToolAgentOverridesByAgent` + `ApplyAgentToolOverrides` + `ApplyRuntimeConfigMaps`）
- [x] TRPC `requires_confirmation`（Declaration 标注 + BeforeTool 阻断 + `blocked` 记录；`KRATOS_TOOL_AUTO_APPROVE=1` 可跳过）
- [x] Tools 调用统计闭环（`duration_ms`、Prometheus `aranea_tool_invocation_total`、列表 SQL 聚合）
- [x] Tools `TestTool` 在线测试（`POST /v1/tools/{id}/test` + 工具详情页）
- [x] MCP 默认超时 60s（`timeout_sec` 未配置时）
- [x] Plugin / Callback EP-CB-01 Phase 1（Chain → WithAgent/Model/ToolCallbacks；Runner WithPlugins 保留 OnEvent）
- [x] PluginManager + Hook 桥接（Phase 2）
- [x] OnEvent Runner 接入 + Hook modify + CallbackEditor（Phase 3）
- [x] Plugin StatsRecorder（`IncrementStats` + AuditLog 回写）+ Hook 非 block 错误隔离
- [x] Plugin 种子同步（`registry.go` + `seedBuiltinPlugins`；9 内置 key 幂等写入 DB）
- [x] Plugin `config_schema_json` 校验（`gojsonschema` + `UpdateConfig`/`Create`）
- [x] Knowledge 管理界面（`KnowledgePage.vue` + `/knowledge` + 侧栏/i18n）
- [x] Artifact 管理界面（`ArtifactsPage.vue` + `/artifacts` + 侧栏/i18n）
- [x] Evaluation / A2A 管理界面
- [x] Knowledge EP-DATA-01：`NewData()` 启动调用 `EnsureKnowledgeSchema`；无 PG 时 `ErrKnowledgeUnavailable` fail-fast
- [x] Evaluation EP-DATA-01：`NewData()` 启动调用 `EnsureEvalSchema`
- [x] A2A EP-A2A-01：Invoke 实际派发 + call_agent 上下文注入（`RunAgentTurn` + `injectA2AContext`）
- [x] Knowledge EP-KN-01/02：GetEmbedderConfig + 摄取 WS 推送 + Embedder 面板
- [x] Evaluation EP-RT-08：LLMJudge 注入 + BizCasesToEvalSet 框架对齐
- [x] Artifact PreviewArtifact RPC + 签名下载 + CodeExecutor 自动保存 + Chat 会话制品面板
- [x] A2A EP-A2A-01/02：Invoke 派发 + admin 鉴权
- [x] Plugin Phase 2：9 个内置插件均有 `builtin()` 实现（`audit_log` 全生命周期 + OnEvent；其余治理类插件）
- [x] MCP 会话统计：`sessions.mcp_call_count` / `tool_call_count` / `skill_call_count` 随工具调用递增

### M4：平台运营（进行中）

- [x] Provider Prometheus 指标：`ProviderRequestTotal` / `ProviderRequestDuration` 包装 LLM Model
- [x] Monitor 持久化：`audit_logs` / `monitor_events` 写入；`runner.completion` 事件落库
- [x] Token：`/usage/events` 明细页；EventBus 流式 run 用量记录 status
- [x] MCP：`mcp_broker` 或 `mcp_tool_set` 可解析服务器；有服务器时自动挂载 Broker；`auth` + `session_reconnect_max` 配置
- [x] MemoryWorker：`TurnMemoryWorker` 订阅 `runner_completion` → 自动记忆队列（30s 去重）
- [x] MCP 认证 UI：`McpServerFormDialog` API Key / Bearer + `config.auth` 写入
- [x] Admin Audit：Tool / MCP / Agent Create·Update·Delete → `audit_logs`（`RecordAdminAudit`）
- [x] Memory L4 图注入：`L4MemoryCue` + `TRPCBuilderDeps.MemoryAdmin` 拼入 system prompt
- [x] Quota MVP：`usage_quotas` 表 + `Get/Set/CheckUsageQuota` RPC；Chat turn 前 `CheckQuota`
- [ ] Channel 投递与多平台适配
- [ ] Ecosystem 从 mock 到后端模型
- [ ] Telemetry 自定义 Span / OTel UI
- [x] Quota 前端配置页 `/usage/quotas`
- [x] MCP OAuth2（client_credentials / refresh / static token）
- [x] L4 图自动写入（AutoMemory → `L4GraphWriter`）

## 扩展红线

- 不在 `internal/biz` import `pkg/trpc-agent-go`。
- 不在 `internal/server` 调 Runner / Agent / LLM。
- 不复制 `pkg/trpc-agent-go` 内部实现到业务目录。
- 不新增 Chat / Team 独立 SSE 主链路；实时主通道是 `/v1/ws`。
- 不在未定义边界的情况下新增第二套 Memory / Evaluation / Gateway 实现。
