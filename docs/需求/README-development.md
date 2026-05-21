# 模块开发计划索引

> 本索引帮助 AI 从 `docs/README.md` 快速进入模块开发计划。系统级优先阅读 `0 系统框图.md` 与 `0-system-development.md`。

## 系统级

| 模块 | 开发计划 |
|------|----------|
| System | [0-system-development.md](./0-system-development.md) |
| **前端（页面总览）** | [frontend-pages.md](./frontend-pages.md) |
| Message | [51 消息机制](./51%20消息机制.md) · [51a](./51a%20后端消息机制.md) · [51b](./51b%20前端消息机制.md) · [开发计划](./message-development.md) |
| Admin/Auth | [admin-auth.design.md](./admin-auth.design.md) · [admin-auth-development.md](./admin-auth-development.md) |

## 核心运行

| 模块 | 开发计划 | 接入度（2026-05-21） |
|------|----------|----------------------|
| Chat | [1-chat-development.md](./1-chat-development.md) | WS ✅；多模态 + RunStatus ✅ |
| Agent Create | [2-agents-create-development.md](./2-agents-create-development.md) | ✅ 创建/A2A/查重/模型检查/模板全字段/结构化错误 |
| **Agent 2–8 横切** | [2-8-agent-modules-development.md](./2-8-agent-modules-development.md) | ✅ 迭代 8–10 主项；🟡 AGT-15/16、批量/迁移 |
| Agent List | [3-agent-list-development.md](./3-agent-list-development.md) | ✅ 列表 UX + 运行态 + 复制 + `created_by` 筛选；❌ 批量/迁移 |
| Agent Type | [4-agent-type-development.md](./4-agent-type-development.md) | ✅ Platform 分类树；❌ 拖拽/关联统计 |
| Agent Setting | [5-agent-setting-development.md](./5-agent-setting-development.md) | ✅ 全 Tab + Tab 子组件；🟡 页壳 ~488 行 |
| Agent Setting File | [6-agent-setting-file-development.md](./6-agent-setting-file-development.md) | ✅ CRUD/注入 + AI 编辑 RPC |
| Agent Evolution | [7-agent-evolution-development.md](./7-agent-evolution-development.md) | ✅ API+指标+Scanner；❌ 趋势图 |
| Agent Title | [8-agent-title-development.md](./8-agent-title-development.md) | ✅ 顶栏+预览；❌ 标题自动生成 |
| Provider | [9-provider-development.md](./9-provider-development.md) |
| Session | [10-session-development.md](./10-session-development.md) |
| Multi-Agent / Team | [11-multi-agent-development.md](./11-multi-agent-development.md) | ✅ 编排+RunTest+Summary+WS；P3 闭环完成 |
| Runner | [40-runner-development.md](./40-runner-development.md) |

## 能力与运行时

| 模块 | 开发计划 |
|------|----------|
| Memory | [12-16 memory-development.md](./12-16%20memory-development.md) |
| Channel | [17-channel-development.md](./17-channel-development.md) |
| Monitor | [18-monitor-development.md](./18-monitor-development.md)（运维 `/monitor/logs`）· [Dashboard 概览](./18-monitor-dashboard-development.md)（`/overview`） |
| FlowLogger | [52-flow-logger.md](./52-flow-logger.md) · [design](./52-flow-logger.design.md) · [开发计划](./52-flow-logger-development.md) · [Slog 移除](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md) |
| MCP | [19-mcp-development.md](./19-mcp-development.md) | 🟢 CRUD+探活+ToolSet/Broker+OAuth+重连 ✅；🟡 统计闭环 |
| Skill | [20-skill-development.md](./20-skill-development.md) | ✅ 管理面 + Layer A/B 运行时；❌ 版本回滚/RBAC/Preview 原因 |
| Cron | [21-cron-development.md](./21-cron-development.md) |
| Plugin | [22-plugin-development.md](./22-plugin-development.md) |
| Tools | [23-tools-development.md](./23-tools-development.md) |
| Telemetry | [24-telemetry-development.md](./24-telemetry-development.md) | 🟢 Chat/Team/Graph + 采样(HTTP) ✅；gRPC 采样 / per-span otel_id 待办 |
| CLI | [25-cli-development.md](./25-cli-development.md) |
| A2A | [26-a2a-development.md](./26-a2a-development.md) | 🟢 Phase 1–3.5 ✅；❌ 网关健康 Cron / 速率限制 |
| Artifact | [27-artifact-development.md](./27-artifact-development.md) |
| Callback | [28-callback-development.md](./28-callback-development.md) |
| Token | [29-token-development.md](./29-token-development.md) |
| Ecosystem | [30-ecosystem-development.md](./30-ecosystem-development.md) |

## 高级能力

| 模块 | 开发计划 | 接入度（2026-05-21） |
|------|----------|----------------------|
| CodeExecutor | [32-codeexecutor-development.md](./32-codeexecutor-development.md) | 🟢 Factory + Agent 配置 + capabilities ✅；🟡 E2B/Container lazy；❌ Jupyter/Workspace |
| Evaluation | [33-evaluation-development.md](./33-evaluation-development.md) |
| Event System | [34-event-development.md](./34-event-development.md) | 🟢 Bus/Envelope/WS/event_store ✅；Monitor RealtimeEvents ✅；Chat Inspector Dialog 双 Tab ✅ |
| Gateway | [35-gateway-development.md](./35-gateway-development.md) | ✅ RunRegistry/ChatUsecase/Webhook；🟡 Follow-up Queue 前端 UX |
| Graph | [36-graph-development.md](./36-graph-development.md) |
| Knowledge | [37-knowledge-development.md](./37-knowledge-development.md) |
| Planner | [39-planner-development.md](./39-planner-development.md) | 🟢 持久化 + 设置 UI + Chat ReAct/A2UI 组件树 + tool 去重 + Review 打磨 ✅；🟡 表单可编辑 / 长尾组件 |
| Avatar | [50-avatar-development.md](./50-avatar-development.md) |
| TTS | [tts-development.md](./tts-development.md) |

## 当前开发顺序

以 [0-system-development.md](./0-system-development.md) 和 [../guides/execution-plan.md](../guides/execution-plan.md) 为准：

1. 文档真理库和 WS/SSE 口径统一。
2. Gateway / RunRegistry（✅ 已落地）/ RunnerManager / Runner 框架能力补齐。
3. Memory、Data、Provider、Skill Import 边界康复。
4. Team、Tools/MCP、Plugin/Callback、Knowledge、Artifact、Evaluation、A2A 闭环。
5. 前端 feature/store/mapper 治理。
6. Monitor、Telemetry、Token、Channel、Ecosystem 平台能力。

---

## 进度快照（2026-05-21，迭代 10 Agent）

> **真相源**：[execution-plan.md](../guides/execution-plan.md) · [2-8-agent-modules-development.md](./2-8-agent-modules-development.md) · **迭代 10**：[changelog/2026-05-21-Agent-Iteration10.md](../changelog/2026-05-21-Agent-Iteration10.md) · **详案**：[devlog/2026-05-21-Agent-Iteration10-Plan.md](../devlog/2026-05-21-Agent-Iteration10-Plan.md) · **LIST-02**：[CreatedBy-Templates-Errors](../changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md) · **迭代 8–9**：[Modules-2-8-DocSync](../changelog/2026-05-21-Agent-Modules-2-8-DocSync.md) · [Optimization](../changelog/2026-05-21-Agent-Optimization.md) · [Iteration9](../changelog/2026-05-21-Agent-Iteration9.md)

### 里程碑

| 里程碑 | 状态 | 说明 |
|--------|------|------|
| M0 文档与边界 | ✅ 完成 | 系统框图、execution-plan、SSE 口径清理 |
| M1 Runner 与 Gateway | ✅ 完成 | RunRegistry、RunnerManager、Enqueue、ArtifactService/SessionIngestor/AgentFactory/AwaitUserReplyRouting 注入；Chat/Team/Cron/Channel 共用 RunGateway |
| M2 架构边界康复 | ✅ 完成 | Memory 端口（RuntimeSet）、Data provider 上移、Provider 拆环（llminspect）、Skill Import service 化 |
| M3 模块闭环 | ✅ 主项完成 | Evaluation/A2A/Artifact/Knowledge/Chat UX（工具卡片/run_status）已通；Rerank/OCR、Memory 图治理、前端拆分留 P2 |
| M4 平台运营 | 🚧 进行中 | Monitor/Token/Quota 部分已通；飞书/钉钉/企微 Channel ✅；Ecosystem/Telemetry UI 待补 |

### 近期已完成（迭代 4 — 2026-05-20）

| 模块 | 交付物 | 备注 |
|------|--------|------|
| MCP | `ReconnectObserver` → `mcp.session.reconnect` + 前端重连 chip | 见 [changelog](../changelog/2026-05-20-Iteration4-PLATFORM-P2.md) |
| Plugin | Phase 3 产品化 ✅：P0 架构收敛、Runs 筛选、前端 scope/sort/runs | Phase 5：rules/ConfirmGate/Schema 表单 |
| Chat | `AwaitUserReply` 跨进程 resume（新 turn + `await_resumed`） | 非 mid-turn 恢复 |
| Monitor | 告警 Webhook/Channel + `cooldown_minutes` + `alert.notify` | 内存冷却 |
| Monitor Dashboard | `/overview` ECharts、Runner 条、Monitor Usage 去重 | [changelog](../changelog/2026-05-21-Monitor-Dashboard-DocSync.md) |

### 近期已完成（M3 延续）

| 模块 | 交付物 | 备注 |
|------|--------|------|
| Plugin/Callback | EP-CB-01 Phase 1–3：Chain 挂 Agent/Model/Tool；PluginManager + Hook 桥接；OnEvent；CallbackEditor；StatsRecorder | 9 内置插件均有 `builtin()` 实现 |
| Plugin | Phase 2：9 内置插件回调实现；种子同步；`config_schema_json` 校验；`PluginsForAgent` scope 过滤 | model_router/retry_reflect 为策略层，产品化配置待补 |
| Tools | ToolOverride 运行时；`requires_confirmation` 策略；调用统计；TestTool；Agent 覆盖面板；MCP 默认超时 60s | MCP 认证/重连/Broker 默认发现仍待补 |
| Session/MCP | `IncrementInvocationCounts`：工具/MCP/Skill 调用同步 `sessions.*_call_count` | 动态 MCP 挂载工具名未入 catalog 时仅计 `mcp_call` |
| Team | RunTeamTest、CancelTeamRun、member_* WS Envelope；Team Runner 使用 `PluginsForAgent` | — |
| Knowledge | Phase 1 ✅ + Phase 2 分块/解析/EmbedBatch ✅ | OCR / 多租户 / AgenticFilter 待补 |
| Artifact | `ArtifactsPage.vue`；路由 `/artifacts`；Runner 注入（`WithArtifactService`） | PreviewArtifact RPC/Chat 附件/签名下载待补 |
| Evaluation | Phase 5 ✅：扩展指标 + LLM UserSim + 趋势/A/B 前端 + Eval LLM 系统配置 | — |
| A2A | Phase 1–3.5：call_agent、Proxy/Endpoint、公开 HTTP、联邦 Gateway（[DocSync](../changelog/2026-05-21-A2A-DocSync.md)） | Phase 4：网关健康 Cron、速率限制 |
| Hooks | `HooksPage.vue`；`AgentHooksPanel.vue`；`CallbackEditor.vue` UTF-8 修复 | — |

### 模块接入度（详表）

| 等级 | 模块 | 说明 |
|------|------|------|
| 核心可用 | Chat、Agent 全家桶、Session、Skill、Tools（含 Override/确认/统计）、Cron、Message/WS、Plugin/Callback | 可创建/运行/配置/观测 |
| 可用需闭环 | **Team**（`team_summary` WS ✅；RunTest UI / step_started / Summary RPC 待补）、**Graph**（LLM/Tool 节点待补）、**MCP**（认证/重连/Broker）、**Memory**（L0–L3 + L4 启发式注入 ✅；LLM Worker/级联审核待补）、**Plugin Phase 5**（rules/ConfirmGate/Schema 表单） | 主路径可用，生产级治理不足 |
| 半闭环 | **Knowledge**、**Artifact** | Knowledge 管理页+Rerank+Embedder ✅；OCR/多租户待补 |
| 核心可用 | **Evaluation** | Phase 5 ✅：FrameworkBridge + AfterTurn + 趋势/A/B + 扩展指标 + Eval LLM 系统配置 |
| 有管理页、Phase 4 待补 | **A2A** | Phase 1–3.5 ✅（Server/远程/联邦/Graph metadata/流式）；网关 Cron/限流待 Phase 4 |
| 早期/占位 | Channel 投递、Ecosystem、CLI 产品化、TTS | 不能作为可组合模块 |
| 可用需闭环 | **Evolution**（Scanner 首版 ✅；趋势图/diff/护栏待补） | 见 [7-agent-evolution-development.md](./7-agent-evolution-development.md) |

### 近期已完成（Evaluation Phase 5 — 2026-05-21）

| 模块 | 交付物 | 备注 |
|------|--------|------|
| Evaluation | 扩展指标、LLM UserSim、scores_json、趋势/A/B 前端 | [Phase5-Extended](../changelog/2026-05-21-Evaluation-Phase5-Extended.md) |
| Evaluation | Eval LLM → system_settings + Settings 页 | env 优先；默认 Sim openai/gpt-4o-mini |

### 近期已完成（A2A 文档 — 2026-05-21）

| 模块 | 交付物 | 备注 |
|------|--------|------|
| A2A | 需求/设计/开发计划与 Phase 1–3.5 代码对齐；SRP 分层说明 | [DocSync](../changelog/2026-05-21-A2A-DocSync.md) |

### 近期已完成（Gateway — 2026-05-21）

| 模块 | 交付物 | 备注 |
|------|--------|------|
| Gateway | `ChatUsecase` 接入 `ChatService`；入队/排队/锁委托 biz | [DocSync](../changelog/2026-05-21-Gateway-DocSync-ChatUsecase.md) |
| Gateway | 出站 Webhook CRUD + HMAC 终态回调；`chat_native` 入队拒绝码 | [Webhook Phase3](../changelog/2026-05-21-Gateway-Webhook-Phase3.md) |

### 近期已完成（迭代 10 — Agent — 2026-05-21）

| 模块 | 交付物 |
|------|--------|
| Agent 设置 | `AgentSettings*Tab.vue` 拆分；`useAgentSettingsPage` 不变 |
| Agent 文件 | `EditPromptFileByAI` + `PromptFileAIEditor` |
| Agent 进化 | `evolution_scan.go` + `EvolutionScanner` worker |
| Agent 创建 | `ListAgentTemplates` API |
| Agent 列表 | `DuplicateAgent`；`ListExtrasForAgents` 批量 + 终态 status |
| 审查加固 | Apply prompt、Duplicate 深拷贝、`mapPromptFileAIError`、单测 |

### 近期已完成（LIST-02 — Agent 创建/列表 — 2026-05-21）

| 模块 | 交付物 |
|------|--------|
| Agent 列表 | `created_by` 列/筛选；`ListAgentCreators`（`mine`）；迁移索引 |
| Agent 创建 | 模板全字段；结构化 `reason` → inline；`kratosError.spec.ts` |
| 审查修正 | `Duplicate` 副本 `created_by`；`CreateAgent` skipErrorNotify |

### 近期已完成（迭代 8–9 — Agent）

| 模块 | 交付物 |
|------|--------|
| Agent Builder | `builder_deps.go`；`system.agent.build` FlowLog |
| Agent 创建 | `CheckAgentKey` + 防抖查重 |
| Agent 设置 | `MergeAgentConfigJSON` |
| Agent 列表 | `last_run_status` / `pending_evolution_count` / `EstimateTokens` |

### Agent 2–8 待优化速览（详见 §8.11）

| 优先级 | 项 |
|--------|-----|
| P2 | 设置页再瘦身（&lt;300 行）、Scanner TTL、LIST-04 批量 |
| P3 | 批量操作、进化趋势图（AGT-16）、`GenerateAgentTitle`、迁移 |

### 近期已完成（迭代 7 — 2026-05-20）

| 模块 | 交付物 |
|------|--------|
| FlowLogger | Phase 3：Team TraceEmitter、Rerank fallback、EventBus error 级、chat 步骤 ID |
| **Monitor** | Logs 流程/进程拆分（[changelog](../changelog/2026-05-20-Monitor-Logs-Split.md)）；方案 C Runs+Events（[Phase1d](../changelog/2026-05-20-Monitor-Phase1d-PlanC.md)） |
| Evaluation | 结果对话框 CSV/JSON 报告导出 |
| **A2A** | Phase 3：A2A Server、远程注册、mTLS、Invoke 工作区策略（[changelog](../changelog/2026-05-20-A2A-Phase3.md)）；Phase 1–2 见 [Phase1-2 changelog](../changelog/2026-05-20-A2A-Phase1-2.md) |

### 当前开发顺序（下一迭代起点）

> **待优化项完整表**：[0-system-development.md § 8](./0-system-development.md)。

1. **P2 — FlowLogger Phase 2**：`ListFlowLogs` 落库 + HTTP 历史查询。
2. **P2 — Monitor 运维页**：全局 latency 聚合、`tab=runs` 命名（[18-monitor-development.md](./18-monitor-development.md) §3）；Dashboard Phase 4 见 [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md)。
3. **P2 — Knowledge**：OCR pipeline 规划与实现。
4. **P2 — A2A Phase 4**：网关健康 Cron + 速率限制（见 [26-a2a-development.md](./26-a2a-development.md)）。
5. **P2 — Telemetry**：Span 语义合并、Usage 三路径运维手册。

### 模块文档同步状态（本轮）

| 文档 | 本次更新 |
|------|----------|
| [0-system-development.md](./0-system-development.md) | **§8 待优化项总览**（按模块/优先级）；M3/M4 状态与 PR 表同步 |
| [README-development.md](./README-development.md)（本文） | 里程碑/近期完成/模块接入度/建议下一步 修订 |
| [22-plugin-development.md](./22-plugin-development.md) | T2 种子、T3 Schema、T1/T4 状态表（上轮已对齐） |
| [28-callback-development.md](./28-callback-development.md) | Phase 1–3 + P2/P3 ✅ |
| [37-knowledge-development.md](./37-knowledge-development.md) | 管理页 G0（上轮已对齐） |
| [27-artifact-development.md](./27-artifact-development.md) | Runner 注入、ArtifactsPage（上轮已对齐） |
| [../guides/execution-plan.md](../guides/execution-plan.md) | I8-MON-02 方案 C ✅（2026-05-21 文档对齐） |
| [18 monitor.md](./18%20monitor.md) / [18 monitor.design.md](./18%20monitor.design.md) | **2026-05-21**：Usage Tab 去重、文件树与 Dashboard 分工 |
| [18-monitor-development.md](./18-monitor-development.md) | **2026-05-21**：Dashboard Phase 2 ✅；验收项同步 |
| [18 monitor-dashboard.md](./18%20monitor-dashboard.md) 三件套 | **2026-05-21**：Phase 0～3b 完成、Store/composable 分层 |
| [frontend-pages.md](./frontend-pages.md) / [29-token-development.md](./29-token-development.md) | **2026-05-21**：概览 ECharts/Runner；趋势组件路径更正 |
| [changelog/2026-05-21-Event-System-DocSync.md](../changelog/2026-05-21-Event-System-DocSync.md) | Event M18 文档与代码对齐摘要 |
| [34-event-development.md](./34-event-development.md) | **2026-05-21**：Bus SRP 拆分、RealtimeEvents、优化表 O1–O7 |
| [changelog/2026-05-20-Monitor-Events-RunnerCompletion-Plan.md](../changelog/2026-05-20-Monitor-Events-RunnerCompletion-Plan.md) | 方案 C 决策摘要 |
| [33-evaluation-development.md](./33-evaluation-development.md) | **2026-05-21**：Phase 5 完整 + Eval LLM system_settings |
| [33 evaluation.md / design](./33%20evaluation.md) | **2026-05-21**：US-7/8、扩展指标、系统配置 §3.4 |
| [changelog/2026-05-21-Evaluation-DocSync-Phase5.md](../changelog/2026-05-21-Evaluation-DocSync-Phase5.md) | Evaluation 文档与代码对齐摘要 |
| [26-a2a-development.md](./26-a2a-development.md) | **2026-05-21**：Phase 1–3.5 ✅；架构 SRP §2；Phase 4 待实现 |
| [changelog/2026-05-21-A2A-DocSync.md](../changelog/2026-05-21-A2A-DocSync.md) | **2026-05-21**：三件套与代码对齐 |
| [changelog/2026-05-20-A2A-Phase35.md](../changelog/2026-05-20-A2A-Phase35.md) | Phase 3.5 联邦/Graph/远程 Invoke |
| [changelog/2026-05-20-A2A-Phase3.md](../changelog/2026-05-20-A2A-Phase3.md) | Phase 3 交付摘要 |
| [changelog/2026-05-20-A2A-Phase1-2.md](../changelog/2026-05-20-A2A-Phase1-2.md) | Phase 1–2 交付摘要 |
| [26 a2a-protocol.md](./26%20a2a-protocol.md) | **2026-05-21**：现状对齐；需求边界（无实现代码块） |
| [26 a2a-protocol.design.md](./26%20a2a-protocol.design.md) | **2026-05-21**：分层/SRP/传输 §十二 |
| [2 agents-create.md](./2%20agents-create.md) / [2-agents-create-development.md](./2-agents-create-development.md) | **2026-05-20**：§9 A2A Proxy 创建 |
| [5 agent-setting.md](./5%20agent-setting.md) / [5-agent-setting-development.md](./5-agent-setting-development.md) | **2026-05-20**：§10 A2A Tab |
| [frontend-pages.md](./frontend-pages.md) | **2026-05-20**：Agent 创建/设置/A2A 页分工 |
| [40-runner-development.md](./40-runner-development.md) | 待本轮同步（P1/P2 验收项已通） |
| [23-tools-development.md](./23-tools-development.md) | 待本轮同步（MCP 待补项需更新） |
