# 模块开发计划索引

> 本索引帮助 AI 从 `docs/README.md` 快速进入模块开发计划。系统级优先阅读 `0 系统框图.md` 与 `0-system-development.md`。

## 系统级

| 模块 | 开发计划 |
|------|----------|
| System | [0-system-development.md](./0-system-development.md) |
| Message | [message-development.md](./message-development.md) |
| Admin/Auth | [admin-auth.design.md](./admin-auth.design.md) · [admin-auth-development.md](./admin-auth-development.md) |

## 核心运行

| 模块 | 开发计划 |
|------|----------|
| Chat | [1-chat-development.md](./1-chat-development.md)（WS ✅；工具卡片/Reasoning/run_status ✅；多模态/持久化 P3） |
| Agent Create | [2-agents-create-development.md](./2-agents-create-development.md) |
| Agent List | [3-agent-list-development.md](./3-agent-list-development.md) |
| Agent Type | [4-agent-type-development.md](./4-agent-type-development.md) |
| Agent Setting | [5-agent-setting-development.md](./5-agent-setting-development.md) |
| Agent Setting File | [6-agent-setting-file-development.md](./6-agent-setting-file-development.md) |
| Agent Evolution | [7-agent-evolution-development.md](./7-agent-evolution-development.md) |
| Agent Title | [8-agent-title-development.md](./8-agent-title-development.md) |
| Provider | [9-provider-development.md](./9-provider-development.md) |
| Session | [10-session-development.md](./10-session-development.md) |
| Multi-Agent / Team | [11-multi-agent-development.md](./11-multi-agent-development.md) |
| Runner | [40-runner-development.md](./40-runner-development.md) |

## 能力与运行时

| 模块 | 开发计划 |
|------|----------|
| Memory | [12-16 memory-development.md](./12-16%20memory-development.md) |
| Channel | [17-channel-development.md](./17-channel-development.md) |
| Monitor | [18-monitor-development.md](./18-monitor-development.md) |
| MCP | [19-mcp-development.md](./19-mcp-development.md) |
| Skill | [20-skill-development.md](./20-skill-development.md) |
| Cron | [21-cron-development.md](./21-cron-development.md) |
| Plugin | [22-plugin-development.md](./22-plugin-development.md) |
| Tools | [23-tools-development.md](./23-tools-development.md) |
| Telemetry | [24-telemetry-development.md](./24-telemetry-development.md) |
| CLI | [25-cli-development.md](./25-cli-development.md) |
| A2A | [26-a2a-development.md](./26-a2a-development.md) |
| Artifact | [27-artifact-development.md](./27-artifact-development.md) |
| Callback | [28-callback-development.md](./28-callback-development.md) |
| Token | [29-token-development.md](./29-token-development.md) |
| Ecosystem | [30-ecosystem-development.md](./30-ecosystem-development.md) |

## 高级能力

| 模块 | 开发计划 |
|------|----------|
| CodeExecutor | [32-codeexecutor-development.md](./32-codeexecutor-development.md) |
| Evaluation | [33-evaluation-development.md](./33-evaluation-development.md) |
| Event System | [34-event-development.md](./34-event-development.md) |
| Gateway | [35-gateway-development.md](./35-gateway-development.md) |
| Graph | [36-graph-development.md](./36-graph-development.md) |
| Knowledge | [37-knowledge-development.md](./37-knowledge-development.md) |
| Planner | [39-planner-development.md](./39-planner-development.md) |
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

## 进度快照（2026-05-19，修订）

> **真相源**：[execution-plan.md](../guides/execution-plan.md)。下表为索引级摘要，细节以各模块 `*-development.md` 为准。

### 里程碑

| 里程碑 | 状态 | 说明 |
|--------|------|------|
| M0 文档与边界 | ✅ 完成 | 系统框图、execution-plan、SSE 口径清理 |
| M1 Runner 与 Gateway | ✅ 完成 | RunRegistry、RunnerManager、Enqueue、ArtifactService/SessionIngestor/AgentFactory/AwaitUserReplyRouting 注入；Chat/Team/Cron/Channel 共用 RunGateway |
| M2 架构边界康复 | ✅ 完成 | Memory 端口（RuntimeSet）、Data provider 上移、Provider 拆环（llminspect）、Skill Import service 化 |
| M3 模块闭环 | ✅ 主项完成 | Evaluation/A2A/Artifact/Knowledge/Chat UX（工具卡片/run_status）已通；Rerank/OCR、Memory 图治理、前端拆分留 P2 |
| M4 平台运营 | ⏳ 未开始 | Monitor/Telemetry/Token、Channel、Ecosystem |

### 近期已完成（M3 延续）

| 模块 | 交付物 | 备注 |
|------|--------|------|
| Plugin/Callback | EP-CB-01 Phase 1–3：Chain 挂 Agent/Model/Tool；PluginManager + Hook 桥接；OnEvent；CallbackEditor；StatsRecorder | 9 内置插件均有 `builtin()` 实现 |
| Plugin | Phase 2：9 内置插件回调实现；种子同步；`config_schema_json` 校验；`PluginsForAgent` scope 过滤 | model_router/retry_reflect 为策略层，产品化配置待补 |
| Tools | ToolOverride 运行时；`requires_confirmation` 策略；调用统计；TestTool；Agent 覆盖面板；MCP 默认超时 60s | MCP 认证/重连/Broker 默认发现仍待补 |
| Session/MCP | `IncrementInvocationCounts`：工具/MCP/Skill 调用同步 `sessions.*_call_count` | 动态 MCP 挂载工具名未入 catalog 时仅计 `mcp_call` |
| Team | RunTeamTest、CancelTeamRun、member_* WS Envelope；Team Runner 使用 `PluginsForAgent` | — |
| Knowledge | EP-DATA-01 `EnsureKnowledgeSchema`；EmbeddingService 框架注入；`KnowledgePage` + 文档入库轮询 | PG 工程化/多 Embedder UI 待补 |
| Artifact | `ArtifactsPage.vue`；路由 `/artifacts`；Runner 注入（`WithArtifactService`） | PreviewArtifact RPC/Chat 附件/签名下载待补 |
| Evaluation | `EvaluationPage.vue`；路由 `/evaluation`；异步 Runner 通过 ChatService 执行 4 指标 | 框架 EvalSet 对齐/LLMJudge 注入待补（EP-RT-08） |
| A2A | `A2APage.vue`；路由 `/a2a`；Discover/Audit/UpdateCard API 完整；`call_agent` trpc 工具已定义 | HTTP Invoke 返回 pending（invoker 未注入，EP-A2A-01）；鉴权待补（EP-A2A-02） |
| Hooks | `HooksPage.vue`；`AgentHooksPanel.vue`；`CallbackEditor.vue` UTF-8 修复 | — |

### 模块接入度（详表）

| 等级 | 模块 | 说明 |
|------|------|------|
| 核心可用 | Chat、Agent 全家桶、Session、Skill、Tools（含 Override/确认/统计）、Cron、Message/WS、Plugin/Callback | 可创建/运行/配置/观测 |
| 可用需闭环 | **Team**（需结构化汇总）、**Graph**（LLM/Tool 节点待补）、**MCP**（认证/重连/Broker）、**Memory**（L4/Worker 待补）、**Plugin 产品化**（UpdateScope/运行记录） | 主路径可用，生产级治理不足 |
| 有管理页、Runtime 待补 | **Knowledge**（Embedder 提供商 UI/摄取 WS 推送）、**Artifact**（PreviewRPC/Chat 附件）、**Evaluation**（EvalSet 对齐/LLMJudge）、**A2A**（Invoke 派发/鉴权） | UI 入口可达，但核心能力不完整 |
| 早期/占位 | Evolution、Channel 投递、Ecosystem、CLI 产品化、TTS | 不能作为可组合模块 |

### 当前开发顺序（下一迭代起点）

> **待优化项完整表**：[0-system-development.md § 8](./0-system-development.md)。

1. **P1 — Channel**：Webhook 入站 + 出站投递 + 平台适配器（EP-BIZ-08）。
2. **P1 — Graph**：LLM/Tool 节点 + ExecutionSummary。
3. **P1 — Team**：结构化汇总 Envelope。
4. **P2 — 前端治理**：Knowledge/Evaluation/A2A `page-to-components`；store 策略；mapper 单测。
5. **P2 — Memory**：L4 冲突检测、级联、衰减。
6. **P2 — Plugin 产品化**：UpdateScope、运行记录、`model_router` 真路由。
7. **P2 — MCP**：生产级重连与连接状态可观测。
8. **P3 — Chat**：多模态附件、RunStatus 持久化、模型选项统一。
9. **P3 — Ecosystem / Telemetry UI / CLI / TTS**：占位模块补边界或实现。

### 模块文档同步状态（本轮）

| 文档 | 本次更新 |
|------|----------|
| [0-system-development.md](./0-system-development.md) | **§8 待优化项总览**（按模块/优先级）；M3/M4 状态与 PR 表同步 |
| [README-development.md](./README-development.md)（本文） | 里程碑/近期完成/模块接入度/建议下一步 修订 |
| [22-plugin-development.md](./22-plugin-development.md) | T2 种子、T3 Schema、T1/T4 状态表（上轮已对齐） |
| [28-callback-development.md](./28-callback-development.md) | Phase 1–3 完成；后续差距（上轮已对齐） |
| [37-knowledge-development.md](./37-knowledge-development.md) | 管理页 G0（上轮已对齐） |
| [27-artifact-development.md](./27-artifact-development.md) | Runner 注入、ArtifactsPage（上轮已对齐） |
| [../guides/execution-plan.md](../guides/execution-plan.md) | M3 分项勾选（上轮已对齐）；本轮无新增 |
| [33-evaluation-development.md](./33-evaluation-development.md) | 待本轮同步（Runner 已实现，EP-RT-08 仍待做） |
| [26-a2a-development.md](./26-a2a-development.md) | 待本轮同步（Invoke stub 状态需明确） |
| [40-runner-development.md](./40-runner-development.md) | 待本轮同步（P1/P2 验收项已通） |
| [23-tools-development.md](./23-tools-development.md) | 待本轮同步（MCP 待补项需更新） |
