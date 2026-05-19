# Aranea-Agents 执行计划

> **状态真相源**：本文记录当前架构健康度、模块接入度与下一阶段任务。AI 编码前须先读 [docs/README.md](../README.md)，再按场景读取规范与需求文档。
>
> **关联文档**：[0 系统框图.md](../需求/0%20系统框图.md) · [0-system-development.md](../需求/0-system-development.md) · [README-development.md](../需求/README-development.md)
>
> **更新时间**：2026-05-20（迭代 2 启动：飞书 Channel 闭环 ✅；Team `team_summary` ✅；Graph LLM/Tool 进行中）

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

## 当前结论

- **主链路可用**：Chat / Agent / Team / Graph 经 `trpc-agent-go` Runner 与 EventBus + `/v1/ws` 串联；RunRegistry + RunnerManager + RunGateway 已落地。
- **架构红线保持**：`internal/biz` 不 import `trpc-agent-go`；`internal/server` 不直接调 Agent runtime；实时主通道为 `/v1/ws`（SSE 仅限 A2A/MCP 等外部协议）。
- **当前优先级**：迭代 2（M4-P1）— Graph LLM/Tool 节点与 ExecutionSummary；多平台 Channel 适配；P2 前端治理与 Memory 图治理。

---

## 模块接入度

| 等级 | 模块 | 说明 |
|------|------|------|
| **核心可用** | Chat(1)、Agent 全家桶(2–8/50)、Provider(9)、Session(10)、Skill(20)、Tools(23)、Cron(21)、Message/WS(51/34)、Plugin/Callback(22/28)、Gateway/Runner(35/40) | 可创建、运行、配置、观测 |
| **可用需闭环** | Graph(36)、MCP(19)、Memory(12–16)、Monitor/Token(18/29) | Graph 节点类型待补；MCP 重连可观测；Memory 冲突/级联 |
| **可用需闭环** | Team(11)、Channel(17) | Team `team_summary` WS ✅；飞书入站+出站 ✅；其他平台适配待补 |
| **有页、Runtime 已通主项** | Knowledge(37)、Artifact(27)、Evaluation(33)、A2A(26) | 管理页 + Runner 注入 ✅；Rerank/OCR、高级评估、远程 A2A 待补 |
| **早期/占位** | Evolution(7)、Ecosystem(30)、CLI(25)、TTS、Telemetry UI(24) | 不可作为可组合模块；文档须标 API-only 或 mock |

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
| 13 | MCP 60s 超时、OAuth2、Broker 挂载、会话 `*_call_count` | MCP | ✅ P2（重连可观测待补） |
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
| I2-FE-01 | Knowledge/Evaluation/A2A `page-to-components` | P2 | ⏳ | 单文件 <300 行 |
| I2-MEM-01 | L4 冲突检测、级联、衰减 | P2 | ⏳ | AutoMemory 写入治理 |
| I2-PLG-01 | Plugin UpdateScope + 运行记录 | P2 | ⏳ | 管理端可审计 |
| I2-CHAT-01 | 多模态附件、RunStatus 持久化 | P3 | ⏳ | 重启后可恢复 awaiting |

**当前冲刺焦点**：I2-FE-01（前端治理）→ I2-MEM-01 → 企微 Channel（I2-CH-03）。

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
- [x] MCP 会话 `mcp_call_count` / `tool_call_count` / `skill_call_count`

**待做**

- [x] Channel 飞书入站 + 出站（`internal/service/channel_ingress.go` + `internal/channel/lark`）
- [x] Channel 钉钉入站 + 出站（I2-CH-02）
- [ ] Channel 企微等平台（I2-CH-03）
- [x] Graph LLM/Tool 节点 + ExecutionSummary（I2-GRAPH-01/02）
- [x] Team 结构化汇总 `team_summary` Envelope（`internal/team/summary.go`）
- [ ] Ecosystem 后端与市场模型（P3）
- [ ] Telemetry 业务 Span / OTel UI（P3）
- [ ] Monitor Dashboard 与告警规则（P2）
- [ ] 前端 page-to-components + mapper 单测（P2）

---

## 系统级验收（节选）

| 项 | 状态 |
|----|------|
| `internal/biz` 不 import `trpc-agent-go` | ✅ |
| `internal/server` 不调 Runner / Agent / LLM | ✅ |
| `internal/data` 不绑定 Runner/Graph runtime 组装 | ✅ |
| Chat/Team/Monitor 实时主链路 `/v1/ws` | ✅ |
| RunRegistry：status / cancel / enqueue / artifact / ingest | ✅ |
| Memory L0–L4 与 MemoryService 主从（`RuntimeSet`） | ✅ |
| 核心模块五面定义完整（Graph/Channel/Ecosystem） | ⏳ |
| `internal/service` 无复杂运行状态机（await/pending 可接受短期） | ⏳ |
| 前端 feature 模板 + mapper 单测 | ⏳ |
| TTS/Ecosystem/CLI 文档标占位 | ⏳ |

---

## 扩展红线

- 不在 `internal/biz` import `pkg/trpc-agent-go`。
- 不在 `internal/server` 调 Runner / Agent / LLM。
- 不复制 `pkg/trpc-agent-go` 内部实现到业务目录。
- 不新增 Chat / Team 独立 SSE 主链路；实时主通道是 `/v1/ws`。
- 不在未定义边界的情况下新增第二套 Memory / Evaluation / Gateway 实现。
- 进度变更时同步更新 **本文** + 相关 `changelog/*.md`（见 [README.md §4](../README.md)）。
