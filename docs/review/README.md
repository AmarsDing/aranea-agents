# Aranea-Agents 模块 Review 总览

> **产出时间**：2026-05-21  
> **方法**：按 `docs/需求/README-development.md` 模块索引，结合需求/设计/开发计划文档链与前后端代码实际落点，对每个模块进行六维评分与风险标注。  
> **真相源优先级**：`0 系统框图.md` + `0-system-development.md` + `execution-plan.md` > `*-development.md` > `*.design.md` > 需求正文

---

## 评分标准（满分 100）

| 维度 | 权重 | 说明 |
|------|------|------|
| 需求符合度 | 20 | 验收项是否落地、功能边界是否清晰、用户故事是否闭环 |
| 架构一致性 | 25 | Kratos/tRPC-Agent 边界、依赖方向、运行时组装位置是否正确 |
| 后端实现质量 | 20 | API、biz/data、并发/状态、错误处理、持久化与可观测性 |
| 前端实现质量 | 15 | 页面分层、store/composable、wire 映射、UX/i18n 与交互闭环 |
| 测试与验证 | 10 | 单测、集成测试、mapper 测试、边界脚本覆盖 |
| 文档一致性 | 10 | 需求/设计/开发计划/README/execution-plan 是否同步 |

### 风险分级

| 级别 | 含义 |
|------|------|
| **P0** | 架构红线违规或主链路破坏，须立即修复 |
| **P1** | 影响稳定性或扩展性的结构性问题，当前迭代修复 |
| **P2** | 功能缺口或代码质量问题，下一迭代计划修复 |
| **P3** | 优化建议，不影响可用性 |

---

## 模块评分汇总

| 模块 | 编号 | 总分 | 需求 | 架构 | 后端 | 前端 | 测试 | 文档 | 风险等级 | Review 文件 |
|------|------|------|------|------|------|------|------|------|----------|-------------|
| **系统架构** | 0 | 82 | 18 | 23 | 18 | 11 | 6 | 6 | P1 | [00-system-review.md](./00-system-review.md) |
| **Chat** | 1 | 84 | 17 | 22 | 18 | 14 | 7 | 6 | P1 | [01-chat-review.md](./01-chat-review.md) |
| **Agent 2–8** | 2–8 | 80 | 16 | 21 | 17 | 14 | 6 | 6 | P1 | [02-08-agent-modules-review.md](./02-08-agent-modules-review.md) |
| **Provider** | 9 | 81 | 17 | 22 | 18 | 12 | 6 | 6 | P1 | [09-provider-review.md](./09-provider-review.md) |
| **Session** | 10 | 79 | 16 | 21 | 17 | 13 | 6 | 6 | P1 | [10-session-review.md](./10-session-review.md) |
| **Team / Multi-Agent** | 11 | 83 | 17 | 22 | 18 | 14 | 6 | 6 | P1 | [11-team-review.md](./11-team-review.md) |
| **Memory L0–L4** | 12–16 | 74 | 15 | 18 | 16 | 12 | 7 | 6 | P1 | [12-16-memory-review.md](./12-16-memory-review.md) |
| **Channel** | 17 | 76 | 15 | 21 | 17 | 12 | 5 | 6 | P1 | [17-channel-review.md](./17-channel-review.md) |
| **Monitor / Dashboard** | 18 | 78 | 16 | 21 | 17 | 13 | 5 | 6 | P1 | [18-monitor-review.md](./18-monitor-review.md) |
| **MCP** | 19 | 80 | 16 | 22 | 17 | 13 | 6 | 6 | P1 | [19-mcp-review.md](./19-mcp-review.md) |
| **Skill** | 20 | 78 | 16 | 21 | 17 | 12 | 6 | 6 | P1 | [20-skill-review.md](./20-skill-review.md) |
| **Cron** | 21 | 82 | 17 | 22 | 18 | 13 | 6 | 6 | P2 | [21-cron-review.md](./21-cron-review.md) |
| **Plugin / Callback** | 22/28 | 81 | 16 | 22 | 17 | 14 | 6 | 6 | P1 | [22-28-plugin-callback-review.md](./22-28-plugin-callback-review.md) |
| **Tools** | 23 | 80 | 16 | 22 | 17 | 13 | 6 | 6 | P1 | [23-tools-review.md](./23-tools-review.md) |
| **Telemetry** | 24 | 73 | 14 | 20 | 16 | 11 | 6 | 6 | P1 | [24-telemetry-review.md](./24-telemetry-review.md) |
| **CLI** | 25 | 42 | 8 | 12 | 8 | 7 | 3 | 4 | P3 | [25-cli-review.md](./25-cli-review.md) |
| **A2A** | 26 | 81 | 17 | 22 | 17 | 13 | 6 | 6 | P1 | [26-a2a-review.md](./26-a2a-review.md) |
| **Artifact** | 27 | 72 | 15 | 20 | 16 | 11 | 5 | 5 | P2 | [27-artifact-review.md](./27-artifact-review.md) |
| **Token / Usage** | 29 | 79 | 16 | 21 | 17 | 13 | 6 | 6 | P1 | [29-token-review.md](./29-token-review.md) |
| **Ecosystem** | 30 | 38 | 7 | 10 | 6 | 8 | 3 | 4 | P3 | [30-ecosystem-review.md](./30-ecosystem-review.md) |
| **CodeExecutor** | 32 | 78 | 16 | 21 | 17 | 12 | 6 | 6 | P2 | [32-codeexecutor-review.md](./32-codeexecutor-review.md) |
| **Evaluation** | 33 | 82 | 17 | 22 | 17 | 13 | 7 | 6 | P1 | [33-evaluation-review.md](./33-evaluation-review.md) |
| **Event System** | 34 | 84 | 17 | 23 | 18 | 14 | 6 | 6 | P1 | [34-event-review.md](./34-event-review.md) |
| **Gateway** | 35 | 83 | 17 | 22 | 18 | 13 | 7 | 6 | P1 | [35-gateway-review.md](./35-gateway-review.md) |
| **Graph** | 36 | 77 | 15 | 21 | 17 | 12 | 6 | 6 | P1 | [36-graph-review.md](./36-graph-review.md) |
| **Knowledge** | 37 | 76 | 15 | 21 | 17 | 12 | 5 | 6 | P1 | [37-knowledge-review.md](./37-knowledge-review.md) |
| **Planner** | 39 | 81 | 16 | 22 | 17 | 14 | 6 | 6 | P1 | [39-planner-review.md](./39-planner-review.md) |
| **Runner** | 40 | 82 | 17 | 22 | 18 | 13 | 6 | 6 | P1 | [40-runner-review.md](./40-runner-review.md) |
| **Message / WS** | 51 | 85 | 17 | 23 | 18 | 14 | 7 | 6 | P1 | [51-message-review.md](./51-message-review.md) |
| **FlowLogger** | 52 | 79 | 16 | 21 | 17 | 13 | 6 | 6 | P1 | [52-flowlogger-review.md](./52-flowlogger-review.md) |
| **Avatar** | 50 | 70 | 14 | 20 | 15 | 11 | 5 | 5 | P2 | [50-avatar-review.md](./50-avatar-review.md) |
| **TTS** | — | 25 | 5 | 7 | 5 | 4 | 2 | 2 | P3 | [tts-review.md](./tts-review.md) |
| **Admin / Auth** | — | 75 | 15 | 21 | 16 | 12 | 5 | 6 | P1 | [admin-auth-review.md](./admin-auth-review.md) |

> **平均分**：~76 / 100（占位/早期模块 CLI/Ecosystem/TTS 显著拉低）

---

## 全局风险清单

### P0 — 须立即核查

| ID | 模块 | 风险描述 |
|----|------|---------|
| P0-001 | Memory | `internal/biz/memory_runtime_set.go` 疑似 import `pkg/trpc-agent-go/memory`，违反 biz 不 import trpc-agent-go 红线；需用 `make runtime-boundary` 验证 |
| P0-002 | Session | Session 服务若有遗留 `sessionmemory.Store` 直连（未经 biz Usecase 端口）需核查 |
| P0-003 | Chat | `useChatWorkspace.ts` 约 1500 行，若含业务状态机逻辑须拆分，否则 bug 传播风险高 |

### P1 — 当前迭代修复

| ID | 模块 | 风险描述 |
|----|------|---------|
| P1-001 | 系统架构 | `PendingMessageQueue` 仍在 Service 层，应下沉 runtime 或 Usecase |
| P1-002 | Agent Settings | `AgentSettingsPage.vue` 约 488 行，功能高度集中，测试缺失 |
| P1-003 | Frontend 分层 | `SystemSettingsPage`、`EcosystemPage`、`MonitorPage`、`PluginsPage` 等直连 API，违反 Page→composable/store→feature API 分层 |
| P1-004 | Provider | `internal/biz` 与 `internal/provider` 概念双向依赖（模型 inspect 与模型目录边界不稳） |
| P1-005 | Memory | L0-L4 产品层与 `trpc-agent-go/memory.Service` 双轨共存，未明确主从 |
| P1-006 | Data | `internal/data.go` 绑定 trpc session / graph checkpoint，应上移至 `internal/runtime` 或 Wire |
| P1-007 | Skill | `server/skill_import_http.go` 绕过 proto/service 层鉴权与观测 |
| P1-008 | Monitor | FlowLogger Phase 2 落库（`ListFlowLogs` HTTP 查询）尚未实现 |
| P1-009 | Telemetry | Span 语义（per-span otel_id、gRPC 采样）待补；Trace UI 体验仍弱 |
| P1-010 | 前端测试 | 多数域仅有 mapper 单测，E2E 几乎为零 |

### P2 — 下迭代修复

| ID | 模块 | 风险描述 |
|----|------|---------|
| P2-001 | A2A | Phase 4：网关健康 Cron + 速率限制尚未实现 |
| P2-002 | Team | `team_summary` WS ✅；RunTest UI / step_started / Summary RPC 仍待补 |
| P2-003 | Session | 批量治理（批量删除/归档）前端 UI 缺失 |
| P2-004 | Knowledge | OCR pipeline、多租户 pgvector 稳定性待规划 |
| P2-005 | Artifact | Chat 内附件引用、跨会话制品检索、签名下载待补 |
| P2-006 | Graph | LLM/Tool 节点待补全；Checkpoint/HITL 产品化 |
| P2-007 | Evolution | 趋势图/diff/护栏待补；`GenerateAgentTitle` 未实现 |
| P2-008 | Monitor | latency 聚合 / Phase 4 自动刷新 / Grafana 集成待补 |
| P2-009 | Token | 定价规则未配置时 `total_cost_micro_usd=0`，配额 SUM 失效，需文档化 |
| P2-010 | Plugin | rules[] 沙箱/版本（Phase 4）未实现 |

---

## 架构红线核查结果

| 红线 | 状态 | 说明 |
|------|------|------|
| `internal/biz` 不 import `trpc-agent-go` | ⚠️ 待确认 | `memory_runtime_set.go` 需用 `make runtime-boundary` 验证 |
| `internal/server` 不调 Runner/Agent/LLM | ✅ | 传输注册正常；`skill_import_http.go` 有旁路但不直调 Runner |
| Chat/Team/Monitor 主实时通道为 `/v1/ws` | ✅ | SSE 仅限 A2A/MCP 外部协议 |
| 前端分层 Page→store→feature API | 🟡 | SystemSettings/Ecosystem/Monitor/Plugins 存在直连漂移 |
| `make runtime-boundary` 通过 | ⚠️ 待确认 | 需运行脚本确认 biz 边界 |

---

## 文档质量问题

| 问题 | 涉及模块 |
|------|---------|
| 缺少完整三件套（需求/设计/开发计划）| Admin/Auth（缺需求）、TTS（缺设计）|
| 命名不统一（点号/空格/连字符混用）| `4.agent-type.*`、`52-flow-logger*`、`50 Avatar.*`、`message-development.md` |
| 模块编号存在跳号（31 缺失，41–49 未用）| Memory（内部说"31 记忆管理界面"但无 31 文件）|
| 开发计划待同步（Runner、Tools）| `40-runner-development.md`、`23-tools-development.md` 标注"待同步" |
| 部分需求正文含实现细节 | `50 Avatar.md` 内容偏向设计规范 |

---

## 各批次 Review 文档

### 第一批：系统主链路

- [00-system-review.md](./00-system-review.md) — 系统架构总览
- [01-chat-review.md](./01-chat-review.md) — Chat / 对话主链路
- [51-message-review.md](./51-message-review.md) — Message / WebSocket / Envelope
- [34-event-review.md](./34-event-review.md) — Event System / EventBus
- [52-flowlogger-review.md](./52-flowlogger-review.md) — FlowLogger
- [10-session-review.md](./10-session-review.md) — Session
- [35-gateway-review.md](./35-gateway-review.md) — Gateway / Runner / RunRegistry
- [40-runner-review.md](./40-runner-review.md) — Runner

### 第二批：Agent 编排

- [02-08-agent-modules-review.md](./02-08-agent-modules-review.md) — Agent Create/List/Type/Setting/File/Evolution/Title
- [11-team-review.md](./11-team-review.md) — Team / Multi-Agent
- [36-graph-review.md](./36-graph-review.md) — Graph 工作流
- [39-planner-review.md](./39-planner-review.md) — Planner
- [50-avatar-review.md](./50-avatar-review.md) — Avatar

### 第三批：能力运行时

- [12-16-memory-review.md](./12-16-memory-review.md) — Memory L0–L4
- [37-knowledge-review.md](./37-knowledge-review.md) — Knowledge / RAG
- [23-tools-review.md](./23-tools-review.md) — Tools
- [19-mcp-review.md](./19-mcp-review.md) — MCP
- [20-skill-review.md](./20-skill-review.md) — Skill
- [22-28-plugin-callback-review.md](./22-28-plugin-callback-review.md) — Plugin / Callback
- [32-codeexecutor-review.md](./32-codeexecutor-review.md) — CodeExecutor

### 第四批：平台运营

- [18-monitor-review.md](./18-monitor-review.md) — Monitor / Dashboard
- [24-telemetry-review.md](./24-telemetry-review.md) — Telemetry
- [29-token-review.md](./29-token-review.md) — Token / Usage / Quota
- [17-channel-review.md](./17-channel-review.md) — Channel
- [21-cron-review.md](./21-cron-review.md) — Cron
- [09-provider-review.md](./09-provider-review.md) — Provider
- [33-evaluation-review.md](./33-evaluation-review.md) — Evaluation
- [26-a2a-review.md](./26-a2a-review.md) — A2A
- [27-artifact-review.md](./27-artifact-review.md) — Artifact
- [30-ecosystem-review.md](./30-ecosystem-review.md) — Ecosystem
- [25-cli-review.md](./25-cli-review.md) — CLI
- [tts-review.md](./tts-review.md) — TTS
- [admin-auth-review.md](./admin-auth-review.md) — Admin / Auth

---

*本文档由 AI 代码 Review 自动生成，所有分数和风险条目均基于截至 2026-05-21 的代码与文档状态。*
