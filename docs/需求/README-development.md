# 模块开发计划索引

> 本索引帮助 AI 从 `docs/README.md` 快速进入模块开发计划。系统级优先阅读 `0 系统框图.md` 与 `0-system-development.md`。

## 系统级

| 模块 | 开发计划 |
|------|----------|
| System | [0-system-development.md](./0-system-development.md) |
| **前端（页面总览）** | [frontend-pages.md](./frontend-pages.md) |
| Message | [message-development.md](./message-development.md) |
| Admin/Auth | [admin-auth.design.md](./admin-auth.design.md) · [admin-auth-development.md](./admin-auth-development.md) |

## 核心运行

| 模块 | 开发计划 |
|------|----------|
| Chat | [1-chat-development.md](./1-chat-development.md)（WS ✅；多模态 + RunStatus ✅；await 跨重启 resume MVP ✅） |
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
| Monitor | [18-monitor-development.md](./18-monitor-development.md)（Phase 1d：**方案 C** Runs+Events 🚧） |
| FlowLogger | [52-flow-logger.md](./52-flow-logger.md) · [design](./52-flow-logger.design.md) · [开发计划](./52-flow-logger-development.md) · [Slog 移除](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md) |
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

## 进度快照（2026-05-20，迭代 7 修订）

> **真相源**：[execution-plan.md](../guides/execution-plan.md) · **本轮**：[changelog/2026-05-20-Optimization-Iteration7.md](../changelog/2026-05-20-Optimization-Iteration7.md)

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
| Plugin | `model_router`：`WithModelSelector` + `ResolveModelAPI` | BeforeModel 仍可做 audit |
| Chat | `AwaitUserReply` 跨进程 resume（新 turn + `await_resumed`） | 非 mid-turn 恢复 |
| Monitor | 告警 Webhook/Channel + `cooldown_minutes` + `alert.notify` | 内存冷却 |

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
| A2A | Phase 3.5：远程 Invoke、GatewayDiscover、Graph metadata、传输文档（[Phase35](../changelog/2026-05-20-A2A-Phase35.md)） | Phase 4：网关健康 Cron |
| Hooks | `HooksPage.vue`；`AgentHooksPanel.vue`；`CallbackEditor.vue` UTF-8 修复 | — |

### 模块接入度（详表）

| 等级 | 模块 | 说明 |
|------|------|------|
| 核心可用 | Chat、Agent 全家桶、Session、Skill、Tools（含 Override/确认/统计）、Cron、Message/WS、Plugin/Callback | 可创建/运行/配置/观测 |
| 可用需闭环 | **Team**（需结构化汇总）、**Graph**（LLM/Tool 节点待补）、**MCP**（认证/重连/Broker）、**Memory**（L4/Worker 待补）、**Plugin 产品化**（UpdateScope/运行记录） | 主路径可用，生产级治理不足 |
| 有管理页、Runtime 待补 | **Knowledge**、**Artifact**、**Evaluation** | UI 入口可达，部分 RPC/Runtime 待补 |
| 有管理页、Phase 3.5 待补 | **A2A** | Phase 1–3 已闭环（Server/远程注册/mTLS/工作区策略）；流式/网关待 Phase 3.5–4 |
| 早期/占位 | Evolution、Channel 投递、Ecosystem、CLI 产品化、TTS | 不能作为可组合模块 |

### 近期已完成（迭代 7 — 2026-05-20）

| 模块 | 交付物 |
|------|--------|
| FlowLogger | Phase 3：Team TraceEmitter、Rerank fallback、EventBus error 级、chat 步骤 ID |
| **Monitor** | Logs Tab 流程/进程拆分 + LogStreamHub（[changelog](../changelog/2026-05-20-Monitor-Logs-Split.md)） |
| Evaluation | 结果对话框 CSV/JSON 报告导出 |
| **A2A** | Phase 3：A2A Server、远程注册、mTLS、Invoke 工作区策略（[changelog](../changelog/2026-05-20-A2A-Phase3.md)）；Phase 1–2 见 [Phase1-2 changelog](../changelog/2026-05-20-A2A-Phase1-2.md) |

### 当前开发顺序（下一迭代起点）

> **待优化项完整表**：[0-system-development.md § 8](./0-system-development.md)。

1. **P1 — Monitor Phase 1d（方案 C）**：Runs 主排障 + Events 过滤 completion + correlation（[18-monitor-development.md](./18-monitor-development.md)）。
2. **P2 — FlowLogger Phase 2**：`ListFlowLogs` 落库 + HTTP 历史查询。
3. **P2 — Knowledge**：OCR pipeline 规划与实现。
4. **P2 — A2A Phase 3.5**：流式 Proxy/Endpoint + Graph resume（见 [26-a2a-development.md](./26-a2a-development.md)）。
5. **P2 — Telemetry**：Span 语义合并、Usage 三路径运维手册。
6. **P3 — Evaluation**：DeleteRun API、AfterTurn 自动评估、服务端聚合报告。

### 模块文档同步状态（本轮）

| 文档 | 本次更新 |
|------|----------|
| [0-system-development.md](./0-system-development.md) | **§8 待优化项总览**（按模块/优先级）；M3/M4 状态与 PR 表同步 |
| [README-development.md](./README-development.md)（本文） | 里程碑/近期完成/模块接入度/建议下一步 修订 |
| [22-plugin-development.md](./22-plugin-development.md) | T2 种子、T3 Schema、T1/T4 状态表（上轮已对齐） |
| [28-callback-development.md](./28-callback-development.md) | Phase 1–3 完成；后续差距（上轮已对齐） |
| [37-knowledge-development.md](./37-knowledge-development.md) | 管理页 G0（上轮已对齐） |
| [27-artifact-development.md](./27-artifact-development.md) | Runner 注入、ArtifactsPage（上轮已对齐） |
| [../guides/execution-plan.md](../guides/execution-plan.md) | I8-MON-02 Events `runner.completion` 增强 🚧 |
| [18 monitor.md](./18%20monitor.md) / [18 monitor.design.md](./18%20monitor.design.md) | §3–§4 方案 C；§九 设计 |
| [18-monitor-development.md](./18-monitor-development.md) | Phase 1d MON-1d-01～10（方案 C） |
| [changelog/2026-05-20-Monitor-Events-RunnerCompletion-Plan.md](../changelog/2026-05-20-Monitor-Events-RunnerCompletion-Plan.md) | 方案 C 决策摘要 |
| [33-evaluation-development.md](./33-evaluation-development.md) | 待本轮同步（Runner 已实现，EP-RT-08 仍待做） |
| [26-a2a-development.md](./26-a2a-development.md) | **2026-05-20**：Phase 1–3 完成；Phase 3.5–4 待实现 |
| [changelog/2026-05-20-A2A-Phase3.md](../changelog/2026-05-20-A2A-Phase3.md) | **2026-05-20**：Phase 3 交付摘要 |
| [changelog/2026-05-20-A2A-Phase1-2.md](../changelog/2026-05-20-A2A-Phase1-2.md) | **2026-05-20**：Phase 1–2 交付摘要 |
| [26 a2a-protocol.md](./26%20a2a-protocol.md) | **2026-05-20**：§2.5 产品模型、§3.10–§3.11 需求 |
| [26 a2a-protocol.design.md](./26%20a2a-protocol.design.md) | **2026-05-20**：§十一 Agent Kind 运行时 |
| [2 agents-create.md](./2%20agents-create.md) / [2-agents-create-development.md](./2-agents-create-development.md) | **2026-05-20**：§9 A2A Proxy 创建 |
| [5 agent-setting.md](./5%20agent-setting.md) / [5-agent-setting-development.md](./5-agent-setting-development.md) | **2026-05-20**：§10 A2A Tab |
| [frontend-pages.md](./frontend-pages.md) | **2026-05-20**：Agent 创建/设置/A2A 页分工 |
| [40-runner-development.md](./40-runner-development.md) | 待本轮同步（P1/P2 验收项已通） |
| [23-tools-development.md](./23-tools-development.md) | 待本轮同步（MCP 待补项需更新） |
