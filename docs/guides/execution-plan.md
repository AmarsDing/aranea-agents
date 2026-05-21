# Aranea-Agents 执行计划

> **状态真相源**：本文记录当前架构健康度、模块接入度与下一阶段任务。AI 编码前须先读 [docs/README.md](../README.md)，再按场景读取规范与需求文档。
>
> **关联文档**：[0 系统框图.md](../需求/0%20系统框图.md) · [0-system-development.md](../需求/0-system-development.md) · [README-development.md](../需求/README-development.md)
>
> **更新时间**：2026-05-21（Review 优化 Phase D：ResourceManager composable · Agent 页壳 · A2A/Team/Graph 测 · LIST-04 biz · Chat i18n）

---

## 当前结论

- **主链路可用**：Chat / Agent / Team / Graph 经 `trpc-agent-go` Runner 与 EventBus + `/v1/ws` 串联；RunRegistry + RunnerManager + RunGateway 已落地。
- **架构红线保持**：`internal/biz` 不 import `trpc-agent-go`；`internal/server` 不直接调 Agent runtime；实时主通道为 `/v1/ws`（SSE 仅限 A2A/MCP 等外部协议）。
- **当前优先级（backlog）**：Provider biz↔provider 收敛 · Evolution 趋势图 · Artifact Chat 引用 · Graph HITL UI · LIST-04 列表 UI · pgvector 多租户测 · Telemetry gRPC 采样。

---

## AI 快速入口

| 步骤 | 文档 |
|------|------|
| 1. 项目全貌 | [docs/README.md](../README.md) |
| 2. 后端编码 | [AI-DEVELOPMENT-SPECIFICATION.md](./AI-DEVELOPMENT-SPECIFICATION.md) |
| 3. 前端编码 | [frontend-guide.md](./frontend-guide.md) |
| 4. 框架速查 | [kratos-framework-guide.md](./kratos-framework-guide.md) · [trpc-agent-go-framework.md](./trpc-agent-go-framework.md) |
| 5. 验证命令 | 后端：`make wire && make api && make build && make test && make runtime-boundary`；前端：`cd web && pnpm i && pnpm lint && pnpm test && pnpm build` |

**文档状态优先级**：`0 系统框图.md` + `0-system-development.md` + **本文** > 模块 `*-development.md` > `*.design.md` > 历史需求正文。

---

## 模块接入度

| 等级 | 模块 | 说明 |
|------|------|------|
| **核心可用** | Chat(1)、Agent 全家桶(2–8/50)、Provider(9)、Session(10)、Skill(20)、Tools(23)、Cron(21)、Message/WS(51/34)、Plugin/Callback(22/28)、Gateway/Runner(35/40) | 可创建、运行、配置、观测 |
| **可用需闭环** | Graph(36)、MCP(19)、Memory(12–16)、Monitor/Token(18/29) | Graph 节点类型待补；MCP 重连可观测；Memory 冲突/级联 |
| **可用需闭环** | Team(11)、Channel(17) | Team `team_summary` WS ✅；飞书/钉钉/企微入站+出站 ✅；更多平台待补 |
| **有页、Runtime 已通主项** | Knowledge(37)、Artifact(27)、Evaluation(33)、A2A(26) | A2A Phase 1–3.5 ✅（联邦 Gateway、远程 Invoke、Graph metadata）；网关 Cron/Admin 流式待 Phase 4 |
| **Skill 子能力** | CodeExecutor(32) | Phase 1–2 + Review ✅：Factory / Agent 配置 / capabilities / lazy E2B — [开发计划](../需求/32-codeexecutor-development.md) · [设计架构图](../需求/32%20codeexecutor.design.md#21-当前架构已实现-phase-12--review-修复) |
| **早期/占位** | Evolution(7)、CLI(25)、TTS | Ecosystem MVP ✅（`/v1/ecosystem/products`）；Telemetry Span 已通 turn，Trace UI 待补 |

---

## Top 20 任务（历史 + 当前）

> ✅ = 已验收；🚧 = 进行中；空白 = 下一迭代。细节见 [0-system-development.md §8](../需求/0-system-development.md)。

| 排序 | 任务 | 模块 | 状态 |
|------|------|------|------|
| 1 | 收敛 Chat/Team/Monitor 主通道为 `/v1/ws` | Message/Monitor/Team | ✅ P0 |
| 2 | `RunRegistry` / `RunnerManager` | Gateway/Runner | ✅ P0 |
| 3 | `EnqueueUserMessage` + WS steer；Cancel 经 StopGeneration + WS | Runner/Chat | ✅ P0（独立 `CancelRun` RPC 非必须） |
| 4 | ArtifactService / SessionIngestor / AgentFactory / AwaitUserReplyRouting 注入 | Runner | ✅ P1 |
| 5 | `memory.RuntimeSet`：TRPC MemoryService + L0–L4 Admin 端口 | Memory | ✅ P0 |
| 6 | 移除 service/agent 对 `sessionmemory` 直连 | Memory | ✅ P0 |
| 7 | trpc session / graph checkpoint provider 上移出 data | Data/Graph/Session | ✅ P1 |
| 8 | Provider 拆环（`internal/llminspect`） | Provider | ✅ P1 |
| 9 | Skill Import 迁入 proto + `SkillService` | Skill | ✅ P1 |
| 10 | Team RunTeamTest / CancelTeamRun / member_* WS | Team | ✅ P1 |
| 11 | ToolOverride + `requires_confirmation` + 调用统计 + TestTool | Tools | ✅ P1 |
| 12 | Plugin Chain + Hook + OnEvent；9 内置 `builtin()` | Plugin/Callback | ✅ P1 |
| 13 | MCP 60s 超时、OAuth2、Broker 挂载、会话 `*_call_count`、重连可观测 | MCP | ✅ P2 |
| 14 | Knowledge / Artifact / Evaluation / A2A 管理页 + Runtime 主项 | 多模块 | ✅ P2 |
| 15 | Memory L4 注入 + MemoryWorker + AutoMemory 图写入 | Memory | ✅ P2（冲突/级联/衰减待补） |
| 16 | Monitor 落库 + Provider 指标 + Quota MVP + Usage 事件 | Monitor/Token | ✅ M4 部分 |
| 17 | Chat 工具卡片 / Reasoning / `run_status` WS / Team 分栏 | Chat/Frontend | ✅ 2026-05-19 |
| 18 | **Channel** 飞书 + 钉钉 Webhook 入站/出站 | Channel | ✅ P1 |
| 19 | **Graph** LLM/Tool 节点 + ExecutionSummary | Graph | ✅ P1/P2 |
| 20 | **Team** 结构化汇总 `team_summary` Envelope | Team | ✅ P1（`EnvelopeTypeTeamSummary` + `BuildTeamRunSummary`） |

### 迭代 2（M4-P1）任务板 — 2026-05-20 起

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I2-CH-01 | 飞书 Webhook 入站 → RunGateway → 回复出站 | P1 | ✅ | `POST /webhooks/{channel_key}` + `FeishuTextSender` |
| I2-CH-02 | 钉钉 Webhook 入站 + 出站（第二平台） | P1 | ✅ | `channel_ingress` 按 `type=dingtalk` 分发；`internal/channel/dingtalk` |
| I2-TEAM-01 | `team_summary` Envelope（成员 token/耗时/状态） | P1 | ✅ | WS `team_summary`；Monitor 可订阅 |
| I2-GRAPH-01 | Graph `AddLLMNode` / `AddToolsNode` builder 接线 | P1 | ✅ | `BuildDeps` + `wireNode`；`SetEntryPoint`/`SetFinishPoint` |
| I2-GRAPH-02 | ExecutionSummary 写入 `graph_execution_done` | P2 | ✅ | `execution_summary` metadata on WS done event |
| I2-FE-01 | Knowledge/Evaluation/A2A `page-to-components` | P2 | ✅ | A2A 拆组件 + mapper 单测；Knowledge/Evaluation 已 <300 行 |
| I2-MEM-01 | L4 冲突检测、级联、衰减 | P2 | ✅ | `l4_governance.go` + `entity_lookup` |
| I2-PLG-01 | Plugin UpdateScope + 运行记录 | P2 | ✅ | `PATCH scope` + `plugin_runs` + `GET /v1/plugins/runs` |
| I2-CH-03 | 企微 Webhook 入站 + 出站 | P1 | ✅ | `internal/channel/wecom` + `channel_ingress_wecom.go` |
| I2-CHAT-01 | 多模态附件、RunStatus 持久化 | P3 | ✅ | Artifact parts + `state_json`；await 通道仍进程内 |
| MON-01 | Monitor 告警规则 + Alerts UI | P2 | ✅ | `alert-rules` API + `runner.error_rate` 评估 |

### 迭代 4（Platform P2）任务板 — 2026-05-20

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I4-OBS-01 | 统一 Envelope + Prometheus：`mcp.session.reconnect` / `alert.notify` | P2 | ✅ | Monitor Events + counter |
| I4-MCP-01 | MCP `ReconnectObserver` + 默认 `session_reconnect_max` + 前端 chip | P2 | ✅ | 断线重连可见 |
| I4-PLG-02 | `model_router` → `WithModelSelector` 真路由 | P2 | ✅ | Usage/audit model 切换 |
| I4-CHAT-02 | `AwaitUserReply` 跨进程 resume（新 turn） | P3 | ✅ | 重启后提交回复继续 |
| I4-MON-02 | 告警 Webhook/Channel 出站 + 冷却 | P2 | ✅ | POST + `alert.notify` |

### 迭代 5（Platform P2 收尾）任务板 — 2026-05-20

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I5-MCP-01 | MCP `metadata_json` 重连计数持久化 | P2 | ✅ | `RecordReconnectMetadata` + 单测 + 前端 chip |
| I5-MON-01 | Runner 指标 Dashboard | P2 | ✅ | `GET /v1/monitor/runner-metrics` + `RunnerMetricsPanel` |
| I5-MON-02 | 告警 Channel 下拉 | P2 | ✅ | `MonitorAlertRules` q-select |
| I5-SYS-02 | StopGeneration → `run_status` cancelled | P2 | ✅ | `chat_stop_generation_test` |
| I5-PLG-03 | `cost_guard` ModelSelector | P2 | ✅ | `ChainedModelSelector` + `CostGuardConfigForAgent` |
| I5-SYS-03 | EventBusConsumer 拆分 | P2 | ✅ | buffer / runner / state 三 handler |
| I5-FE-02 | knowledge/evaluation mapper 模块化 | P2 | ✅ | `features/*/mappers.ts` |
| I6-ECO-01 | Ecosystem proto + MVP | P3 | ✅ | List/Publish/Install + `EcosystemPage` |
| I6-TEL-01 | Chat turn OTel Span | P3 | ✅ | `chat.turn` in `trpc_turn` |

| I6-TEL-02 | Monitor Trace 瀑布图 + usage spans | P2 | ✅ | `turn_spans` + `TraceWaterfall.vue` |
| KN-01 | Knowledge Rerank（trpc reranker） | P2 | ✅ | `KRATOS_KNOWLEDGE_RERANKER` + Retriever |
| EVAL-02 | Evaluation 人工标注 API + UI | P2 | ✅ | `AnnotateCaseResult` + Results 对话框 |
| EVAL-05 | Evaluation Phase 5（扩展指标/UserSim/趋势/Eval LLM 系统配置） | P3 | ✅ | [changelog](../changelog/2026-05-21-Evaluation-Phase5-Extended.md) |

**迭代 6 备注（观测）**：I6-TEL-02 / KN-01 / EVAL-02 review 已合入；`chat.usage_record` 失败用 FlowLogger（见 [changelog Iteration6](../changelog/2026-05-20-Iteration6-TRACE-EVAL-KN.md) §后续优化）。

**FlowLogger v2**：📋 [需求](../需求/52-flow-logger.md) · [设计](../需求/52-flow-logger.design.md) · [开发计划](../需求/52-flow-logger-development.md) — Phase 1a/1b/2/3 ✅。

### 迭代 7（Plugin P0–P3）任务板 — 2026-05-21

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I7-PLG-01 | 回调编排边界 + orchestration.go | P0 | ✅ | 四层分工文档化 |
| I7-PLG-02 | model_router 单一路由（ModelSelector） | P0 | ✅ | BeforeModel 不 patch model |
| I7-PLG-03 | permission_guard 仅 deny_tools | P0 | ✅ | confirm_tools 不阻断 |
| I7-PLG-04 | OnEvent scope + hook agent_id | P0 | ✅ | Manager + AgentKeyResolver |
| I7-PLG-05 | audit PluginSafeLogger + telemetry enrich | P1 | ✅ | Run 含 session/agent |
| I7-PLG-06 | ListPluginRuns 扩展筛选 | P2 | ✅ | proto + data 层 |
| I7-PLG-07 | 前端 scope/sort/runs | P1 | ✅ | `/plugins/runs` |
| I7-PLG-08 | Plugin 沙箱 / 版本 | P3 | 📋 | Phase 4 backlog |
| I7-PLG-09 | ConfirmGate + AwaitUserReply 统一 | P2 | ✅ | Chain 合并 confirmation_guard |
| I7-PLG-10 | model_router rules[] | P2 | ✅ | priority + contains/regex |
| I7-PLG-11 | cost_guard 日预算持久化 | P2 | ✅ | `plugin_cost_guard_usage` |
| I7-PLG-12 | Schema 配置表单 | P2 | ✅ | PluginSchemaForm + 双模式 |
| I7-PLG-13 | retry_and_reflect 事件流反思 | P2 | ✅ | CustomResult + plugin.retry_reflect |
| I7-PLG-14 | cost_guard Agent scope 分桶 | P2 | ✅ | CostGuardBudgetRegistry |
| I7-PLG-15 | 工具确认专用 UI | P2 | ✅ | await_kind + Approve/Deny |
| I7-PLG-16 | rules[] 可视化编辑器 | P2 | ✅ | ModelRouterRulesEditor |

**当前冲刺焦点**：Team 五模式 E2E · Graph Agent/Router · Artifact Chat 引用 · A2A 速率限制 · Monitor latency · Plugin P4 沙箱。

### 迭代 7（优化升级）任务板 — 2026-05-20

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I7-FL-01 | Team `TraceEmitter`（`team.run.*`） | P2 | ✅ | `runner_team_trpc.go` + Monitor Logs |
| I7-FL-02 | Rerank fallback → `knowledge.rerank.fallback` | P2 | ✅ | 无 slog；向量序静默降级 |
| I7-FL-03 | EventBus 失败 `SessionSysLogError` | P2 | ✅ | usage/state/monitor 持久化 |
| I7-FL-04 | `chat.turn.enter` 步骤 ID 对齐 | P2 | ✅ | `chat_native` + 注册表别名 |
| I7-EVAL-01 | 评估报告 CSV/JSON 导出 | P2 | ✅ | `EvaluationResultsDialog` 按钮 |
| I7-DOC-01 | 进度文档同步 | P2 | ✅ | 本文 + changelog Iteration7 |
| I7-A2A-01 | A2A Phase 3：Server + 远程注册 + mTLS + Invoke 工作区策略 | P1 | ✅ | [changelog Phase3](../changelog/2026-05-20-A2A-Phase3.md) |
| I7-A2A-02 | A2A Phase 3.5：Graph metadata + 远程 Invoke + GatewayDiscover + 传输文档 | P2 | ✅ | [changelog Phase35](../changelog/2026-05-20-A2A-Phase35.md) |
| I8-MON-01 | Monitor Logs 流程/进程 Tab 拆分 + LogStreamHub + WS enable_log 修复 | P1 | ✅ | [changelog Monitor-Logs-Split](../changelog/2026-05-20-Monitor-Logs-Split.md) |
| I8-MON-02 | Monitor 方案 C：Runs 主排障 + Events 收窄 + `runner.completion` correlation | P1 | ✅ | [changelog](../changelog/2026-05-20-Monitor-Phase1d-PlanC.md) · [18-monitor-development](../需求/18-monitor-development.md) Phase 1d |

### 迭代 8（Agent 优化）任务板 — 2026-05-21

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I8-AGT-01 | `TRPCBuilderDeps` 分组类型（`builder_deps.go`） | P2 | ✅ | `TRPC*Deps` + 扁平字面量兼容 |
| I8-AGT-02 | `system.agent.build` FlowLog | P2 | ✅ | `trpc_build_router.go` 开始/失败/完成 |
| I8-AGT-03 | `config_json` PATCH 浅合并 | P2 | ✅ | `MergeAgentConfigJSON` + `agent_usecase.Update` |
| I8-AGT-04 | `CheckAgentKey` RPC + 创建弹窗防抖查重 | P3 | ✅ | `GET /v1/agent-keys/check` |
| I8-AGT-05 | Agent 开发计划与 §8.11 文档同步 | P2 | ✅ | [Optimization](../changelog/2026-05-21-Agent-Optimization.md) |
| I8-DOC-02 | Agent 模块 2–8 开发计划与代码对齐 | P2 | ✅ | [Modules-2-8-DocSync](../changelog/2026-05-21-Agent-Modules-2-8-DocSync.md) |

### 迭代 9（Agent 列表/文件/进化标签）任务板 — 2026-05-21

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I9-AGT-07 | 列表 `last_run_status` / `last_run_at` 聚合 | P2 | ✅ | `ListExtrasForAgents` + `formatLastRunContext` |
| I9-AGT-12 | `EstimateTokens` 前端对接 | P2 | ✅ | `estimateAgentTokens` + `AgentFilesPanel` |
| I9-AGT-14 | 进化 chip + `pending_evolution_count` | P2 | ✅ | `isAgentEvolving` 列表/设置顶栏 |
| I9-DOC-01 | 开发计划与 §8.11 同步 | P2 | ✅ | [Iteration9](../changelog/2026-05-21-Agent-Iteration9.md) |

### 迭代 10（Agent 拆分 / Scanner / AI 编辑 / 模板 / 复制）任务板 — 2026-05-21

> 详案：[devlog/2026-05-21-Agent-Iteration10-Plan.md](../devlog/2026-05-21-Agent-Iteration10-Plan.md)

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I10-AGT-08 | `AgentSettingsPage` 按 Tab 拆分 | P2 | ✅ | 页壳 ~488 行；三 Tab 子组件 |
| I10-AGT-09 | EvolutionScanner 30min + 阈值建议 | P2 | ✅ | `evolution_scan.go` + `evolution_scanner.go` |
| I10-AGT-11 | `EditPromptFileByAI` RPC + 前端 | P2 | ✅ | 替换 placeholder |
| I10-AGT-06 | `ListAgentTemplates` API | P2 | ✅ | `GET /v1/agent-templates` |
| I10-AGT-10 | `DuplicateAgent` RPC + 列表入口 | P3 | ✅ | `POST /v1/agents/{id}/duplicate` |
| I10-DOC-01 | changelog + 迭代 10 计划 | P2 | ✅ | [Iteration10](../changelog/2026-05-21-Agent-Iteration10.md) |
| I10-REVIEW | 审查 P0–P2 加固 + 模块 `*-development.md` 同步 | P2 | ✅ | ListExtras 批量、终态 status、Apply prompt、Duplicate 深拷贝 |
| I10-LIST-02 | `created_by` + 模板全字段 + 结构化创建错误 + 审查修正 | P2 | ✅ | [CreatedBy-Templates-Errors](../changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md) |

### 迭代 11（CodeExecutor Phase 1–2 + Review）— 2026-05-21

| ID | 任务 | 优先级 | 状态 | 验收 |
|----|------|--------|------|------|
| I11-CEX-01 | CodeExecutor 三文档 + 交叉引用与代码对齐 | P2 | ✅ | [CodeExecutor-DocSync](../changelog/2026-05-21-CodeExecutor-DocSync.md) |
| I11-CEX-02 | Agent `code_executor_type` + Factory（Wire 单例） | P1 | ✅ | [P0-P2](../changelog/2026-05-21-CodeExecutor-P0-P2.md) · [Review-Fixes](../changelog/2026-05-21-CodeExecutor-Review-Fixes.md) |
| I11-CEX-03 | E2B / Container lazy + capabilities API | P2 | ✅ | Monitor `code-executor-capabilities` |
| I11-CEX-04 | 架构图写入 design 文档 | P2 | ✅ | [32 codeexecutor.design.md §2.1](../需求/32%20codeexecutor.design.md) |
| I11-CEX-05 | WorkspaceRegistry + InputSpec/OutputSpec | P3 | 📋 | Phase 4 |

---

## 里程碑

### M0：文档与边界 ✅

- [x] `docs/需求/0 系统框图.md`、`0-system-development.md`、本文
- [x] `docs/README.md` 文档索引与 AI 工作流
- [x] WS/SSE 口径统一；Memory 断链修复

### M1：Runner 与 Gateway ✅

- [x] `RunRegistry`（`internal/runtime/run_registry.go`）+ `RunnerManager`
- [x] `EnqueueUserMessage`（`POST /v1/chat/enqueue`、WS `enqueue_message`）
- [x] 取消路径（HTTP `StopGeneration` + WS `cancel`）
- [x] ArtifactService / SessionIngestor / AgentFactory / AwaitUserReplyRouting 注入
- [x] Chat / Team / Cron / Channel 共用 `RunGateway`
- [x] 出站 Webhook：`GatewayService` CRUD + `WebhookDispatcher` 终态回调（2026-05-21）

### M2：架构边界康复 ✅

- [x] Memory 端口（`memory.RuntimeSet` + `SessionAdminStore`）
- [x] Data provider 上移（`provideTRPCSessionService` / `provideGraphCheckpointSaver`）
- [x] Provider 拆环（`internal/llminspect`）
- [x] Skill Import service 化

### M3：模块闭环 ✅

- [x] Team / Tools / Plugin / MCP 统计 / Knowledge / Artifact / Evaluation / A2A 主项
- [x] Chat UX：工具卡片、Reasoning、`run_status` WS、制品面板
- [x] 9 内置 Plugin `builtin()`；MCP OAuth2；EvalSet + LLMJudge

### M4：平台运营 🚧

**已完成**

- [x] Provider Prometheus 指标
- [x] Monitor：`audit_logs` / `monitor_events`；`runner.completion` 落库
- [x] Token：`/usage/events`；EventBus 流式用量
- [x] MCP：Broker 自动挂载、`auth`、`session_reconnect_max`、OAuth2、认证 UI
- [x] MemoryWorker + L4 prompt 注入 + L4 图自动写入（`L4GraphWriter`）
- [x] Quota MVP + 前端 `/usage/quotas`；Admin Audit（Tool/MCP/Agent CRUD）
- [x] Token §8.6：Ent quota/alert/hourly、异步预算告警、低性价比模型、user/global quota API、`team_turn` 用量（2026-05-20，见 [changelog](../changelog/2026-05-20-Usage-Quota-Events.md) 迭代三）
- [x] Token §9 billable 读层：`chat_turn`+`team_member` 聚合，排除 `team_turn`；明细 `team_id`/`usage_kind` 筛选（见 changelog 迭代七、`29-token-development.md` §9）
- [x] MCP 会话 `mcp_call_count` / `tool_call_count` / `skill_call_count`

**待做**

- [x] Channel 飞书入站 + 出站（`internal/service/channel_ingress.go` + `internal/channel/lark`）
- [x] Channel 钉钉入站 + 出站（I2-CH-02）
- [x] Channel 企微入站 + 出站（I2-CH-03，`internal/channel/wecom`）
- [x] Graph LLM/Tool 节点 + ExecutionSummary（I2-GRAPH-01/02）
- [x] Team 结构化汇总 `team_summary` Envelope（`internal/team/summary.go`）
- [ ] Ecosystem 后端与市场模型（P3） — MVP ✅ 见迭代 5
- [ ] Telemetry 业务 Span / OTel UI（P3）
- [x] Monitor 告警规则 + Alerts UI（MON-01）
- [x] Monitor 方案 C：Runs/Events 分工 + completion 关联（I8-MON-02 / Phase 1d）
- [x] Monitor 概览 Dashboard `/overview`（ECharts、Runner 条、Usage Tab 去重）— [18-monitor-dashboard-development.md](../需求/18-monitor-dashboard-development.md)
- [ ] Monitor latency 聚合 / Phase 4（自动刷新、Grafana）
- [x] 前端 page-to-components + A2A mapper 单测（P2）；Knowledge/Evaluation 此前已拆分

---

## 系统级验收（节选）

| 项 | 状态 |
|----|------|
| `internal/biz` 不 import `trpc-agent-go` | ✅（`MemorySet` 已移至 `internal/runtime`） |
| `internal/server` 不调 Runner / Agent / LLM | ✅ |
| `internal/data` 不绑定 Runner/Graph runtime 组装 | ✅ |
| Chat/Team/Monitor 实时主链路 `/v1/ws` | ✅ |
| RunRegistry：status / cancel / enqueue / artifact / ingest | ✅ |
| Memory L0–L4 与 MemoryService 主从（`MemorySet`） | ✅ |
| `make runtime-boundary` CI 门禁 | ✅ |
| 核心模块五面定义完整（Graph/Channel/Ecosystem） | ⏳ |
| `internal/service` 无复杂运行状态机（await meta/resume 可接受短期） | ⏳ ChatUsecase 已接入；PendingQueue 已下沉至 `internal/runtime` |
| 前端 feature 模板 + mapper 单测 | 🟡（A2A mapper 单测 ✅；其余 feature 待补） |
| FlowLogger Phase 2 落库 + TraceList | ✅ |
| TTS/Ecosystem/CLI 文档标占位 | 🟡（见 README-development 技术预览） |

---

## 扩展红线

- 不在 `internal/biz` import `pkg/trpc-agent-go`。
- 不在 `internal/server` 调 Runner / Agent / LLM。
- 不复制 `pkg/trpc-agent-go` 内部实现到业务目录。
- 不新增 Chat / Team 独立 SSE 主链路；实时主通道是 `/v1/ws`。
- 不在未定义边界的情况下新增第二套 Memory / Evaluation / Gateway 实现。
- 进度变更时同步更新 **本文** + 相关 `changelog/*.md`（见 [README.md §4](../README.md)）。
