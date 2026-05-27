# Aranea-Agents 模块 Review 总览

> **产出时间**：2026-05-21 · **最后同步**：2026-05-27  
> **方法**：按 `docs/需求/README-development.md` 模块索引，结合需求/设计/开发计划文档链与前后端代码实际落点，对每个模块进行六维评分与风险标注。  
> **真相源优先级**：`0-system-diagram.md` + `0-system-development.md` + `execution-plan.md` > `*-development.md` > `*.design.md` > 需求正文

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
| **Session** | 10 | 83 | 17 | 22 | 16 | 13 | 5 | 9 | P1 | [10-session-review.md](./10-session-review.md) · [Phase2](./2026-05-24-Session-Phase2-Review.md) |
| **Team** | 11 | 83 | 17 | 22 | 18 | 14 | 6 | 6 | P1 | [11-team-review.md](./11-team-review.md) |
| **Memory L0–L4** | 12–16 | 74 | 15 | 18 | 16 | 12 | 7 | 8 | P1 | [memory-review.md](./memory-review.md) |
| **Channel** | 17 | 92 | 19 | 23 | 19 | 14 | 8 | 9 | P2 | [17-channel-review.md](./17-channel-review.md) · [DECO-01](./2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md) |
| **Monitor** | 18 | 78 | 16 | 21 | 17 | 13 | 5 | 6 | P1 | [18-monitor-review.md](./18-monitor-review.md) |
| **MCP** | 19 | 80 | 16 | 22 | 17 | 13 | 6 | 6 | P1 | [19-mcp-review.md](./19-mcp-review.md) |
| **Skill** | 20 | 78 | 16 | 21 | 17 | 12 | 6 | 6 | P1 | [20-skill-review.md](./20-skill-review.md) |
| **Cron** | 21 | 82 | 17 | 22 | 18 | 13 | 6 | 6 | P2 | [21-cron-review.md](./21-cron-review.md) |
| **Plugin/Callback** | 22/28 | 81 | 16 | 22 | 17 | 14 | 6 | 6 | P1 | [22-28-plugin-callback-review.md](./22-28-plugin-callback-review.md) |
| **Tools** | 23 | 86 | 18 | 23 | 18 | 13 | 7 | 7 | P2 | [23-tools-review.md](./23-tools-review.md) |
| **Telemetry** | 24 | 73 | 14 | 20 | 16 | 11 | 6 | 6 | P1 | [24-telemetry-review.md](./24-telemetry-review.md) |
| **CLI（技术预览）** | 25 | 42 | 8 | 12 | 8 | 7 | 3 | 4 | P3 | [25-cli-review.md](./25-cli-review.md) |
| **A2A** | 26 | 81 | 17 | 22 | 17 | 13 | 6 | 6 | P1 | [26-a2a-review.md](./26-a2a-review.md) |
| **Artifact** | 27 | 72 | 15 | 20 | 16 | 11 | 5 | 5 | P2 | [27-artifact-review.md](./27-artifact-review.md) |
| **Token/Usage** | 29 | 79 | 16 | 21 | 17 | 13 | 6 | 6 | P1 | [29-token-review.md](./29-token-review.md) |
| **Ecosystem** | 30 | 38 | 7 | 10 | 6 | 8 | 3 | 4 | P3 | [30-ecosystem-review.md](./30-ecosystem-review.md) |
| **CodeExecutor** | 32 | 78 | 16 | 21 | 17 | 12 | 6 | 6 | P2 | [32-codeexecutor-review.md](./32-codeexecutor-review.md) |
| **Evaluation** | 33 | 82 | 17 | 22 | 17 | 13 | 7 | 6 | P1 | [33-evaluation-review.md](./33-evaluation-review.md) |
| **Event System** | 34 | 84 | 17 | 23 | 18 | 14 | 6 | 6 | P1 | [34-event-review.md](./34-event-review.md) |
| **Gateway** | 35 | 83 | 17 | 22 | 18 | 13 | 7 | 6 | P1 | [35-gateway-review.md](./35-gateway-review.md) |
| **Graph** | 36 | 77 | 15 | 21 | 17 | 12 | 6 | 6 | P1 | [36-graph-review.md](./36-graph-review.md) |
| **Knowledge** | 37 | 76 | 15 | 21 | 17 | 12 | 5 | 6 | P1 | [37-knowledge-review.md](./37-knowledge-review.md) |
| **Planner** | 39 | 81 | 16 | 22 | 17 | 14 | 6 | 6 | P1 | [39-planner-review.md](./39-planner-review.md) |
| **Runner** | 40 | 82 | 17 | 22 | 18 | 13 | 6 | 6 | P1 | [40-runner-review.md](./40-runner-review.md) |
| **Avatar** | 50 | 70 | 14 | 20 | 15 | 11 | 5 | 5 | P2 | [50-avatar-review.md](./50-avatar-review.md) |
| **Message/WS** | 51 | 85 | 17 | 23 | 18 | 14 | 7 | 6 | P1 | [51-message-review.md](./51-message-review.md) |
| **FlowLogger** | 52 | 79 | 16 | 21 | 17 | 13 | 6 | 6 | P1 | [52-flowlogger-review.md](./52-flowlogger-review.md) |
| **TTS（技术预览）** | — | 25 | 5 | 7 | 5 | 4 | 2 | 2 | P3 | [tts-review.md](./tts-review.md) |
| **Admin/Auth** | — | 75 | 15 | 21 | 16 | 12 | 5 | 6 | P1 | [admin-auth-review.md](./admin-auth-review.md) |

> **平均分**：~76 / 100（占位/早期模块 CLI/Ecosystem/TTS 显著拉低，排除后 ~79.8）

---

## 风险清单

### P0 — 须立即核查（已全部闭合）

| ID | 模块 | 风险描述 | 状态 |
|----|------|---------|------|
| P0-001 | Memory | biz import trpc-agent-go | ✅ 已迁至 `internal/runtime/memory_set.go` |
| P0-002 | Chat | `useChatWorkspace.ts` 编排过重 | ✅ 已拆至 ~500 行 + 子 composable |
| P0-003 | Session | `sessionmemory.Store` 直连 service | ✅ 主链路经 data/runtime/wire 注入 |

### P1 — 当前迭代修复

| ID | 模块 | 风险描述 | 状态 |
|----|------|---------|------|
| P1-001 | 系统/Chat | PendingMessageQueue 在 Service | ✅ 已下沉 `internal/runtime` |
| P1-003 | 前端 | 大 Page 直连 API | 🟡 部分已迁 composable |
| P1-004 | Provider | biz↔provider 双向依赖 | 🚧 待收敛 |
| P1-011 | 前端测试 | E2E 薄弱 | 🚧 续补中 |
| P1-012 | Session | Participants List 全量 Sync | 🚧 → F6-a |
| P1-013 | Session | Export / Chat Timeline 无界内存 | 🚧 → ARCH-03 · FE-TL-01 |
| P1-014 | Channel/M55 | Global hub 与 session WS 双 patch | 🚧 DECO-01 |

### P2 — 下迭代修复

| ID | 模块 | 风险描述 | 状态 |
|----|------|---------|------|
| P2-004 | Graph | Agent/Router 节点 | 🟡 CatalogAgentResolver ✅ |
| P2-007 | Evolution | 趋势图/diff/护栏 | 🚧 backlog |
| P2-008 | Monitor | latency 聚合 / 自动刷新 | 🟡 30s 刷新 ✅ |
| P2-009 | Token | 定价未配置 UX | ✅ `pricingWarning.ts` |
| P2-010 | Plugin | 沙箱/版本 Phase 4 | 🚧 类型占位 |

---

## 架构红线核查结果

| 红线 | 状态 | 说明 |
|------|------|------|
| `internal/biz` 不 import `trpc-agent-go` | ✅ | MemorySet 在 `internal/runtime` |
| `internal/server` 不调 Runner/Agent/LLM | ✅ | 传输注册正常 |
| Chat/Team/Monitor 主实时通道为 `/v1/ws` | ✅ | SSE 仅限 A2A/MCP 外部协议 |
| 前端分层 Page→store→feature API | 🟡 | 部分页面存在直连漂移 |
| `make runtime-boundary` 通过 | ✅ | CI 已覆盖 |

---

## 各批次 Review 文档

### 第一批：系统主链路

- [00-system-review.md](./00-system-review.md) — 系统架构总览
- [01-chat-review.md](./01-chat-review.md) — Chat / 对话主链路
- [51-message-review.md](./51-message-review.md) — Message / WebSocket / Envelope
- [34-event-review.md](./34-event-review.md) — Event System / EventBus
- [52-flowlogger-review.md](./52-flowlogger-review.md) — FlowLogger
- [10-session-review.md](./10-session-review.md) — Session（基线）
- [2026-05-24-Session-Phase2-Review.md](./2026-05-24-Session-Phase2-Review.md) — Session Phase 2
- [35-gateway-review.md](./35-gateway-review.md) — Gateway / Runner / RunRegistry
- [40-runner-review.md](./40-runner-review.md) — Runner

### 第二批：Agent 编排

- [02-08-agent-modules-review.md](./02-08-agent-modules-review.md) — Agent Create/List/Type/Setting/File/Evolution/Title
- [11-team-review.md](./11-team-review.md) — Team / Multi-Agent
- [36-graph-review.md](./36-graph-review.md) — Graph 工作流
- [39-planner-review.md](./39-planner-review.md) — Planner
- [50-avatar-review.md](./50-avatar-review.md) — Avatar

### 第三批：能力运行时

- [memory-review.md](./memory-review.md) — Memory L0–L4
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
- [2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md](./2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md) — DECO-01 飞书/Web 同步
- [21-cron-review.md](./21-cron-review.md) — Cron
- [09-provider-review.md](./09-provider-review.md) — Provider
- [33-evaluation-review.md](./33-evaluation-review.md) — Evaluation
- [26-a2a-review.md](./26-a2a-review.md) — A2A
- [27-artifact-review.md](./27-artifact-review.md) — Artifact
- [30-ecosystem-review.md](./30-ecosystem-review.md) — Ecosystem
- [25-cli-review.md](./25-cli-review.md) — CLI
- [tts-review.md](./tts-review.md) — TTS
- [admin-auth-review.md](./admin-auth-review.md) — Admin / Auth

### 补充 Review

- [2026-05-22-Channel-Inbound-Review.md](./2026-05-22-Channel-Inbound-Review.md) — Channel 入站审查
- [2026-05-22-Tools-Phase4-Fragment-Edit-Review.md](./2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) — Tools Phase 4
- [2026-05-22-Tools-Phase5-Workspace-Unification-Review.md](./2026-05-22-Tools-Phase5-Workspace-Unification-Review.md) — Tools Phase 5
- [2026-05-23-Channel-IM-Preview-Review.md](./2026-05-23-Channel-IM-Preview-Review.md) — Channel IM Preview
- [2026-05-23-Chat-Flow-Full-Review.md](./2026-05-23-Chat-Flow-Full-Review.md) — Chat 全链路
- [2026-05-23-Graph-Phase-A-D-Review.md](./2026-05-23-Graph-Phase-A-D-Review.md) — Graph Phase A–D
- [2026-05-23-M55-Phase-R-UX-Review.md](./2026-05-23-M55-Phase-R-UX-Review.md) — M55 R-UX
- [2026-05-23-M55-Run-Lifecycle-Review.md](./2026-05-23-M55-Run-Lifecycle-Review.md) — M55 Run Lifecycle
- [2026-05-23-Team-Graph-M53-Phase7-Review.md](./2026-05-23-Team-Graph-M53-Phase7-Review.md) — M53 Phase 7
- [2026-05-24-Channel-External-Reference-Playbook-Review.md](./2026-05-24-Channel-External-Reference-Playbook-Review.md) — Channel 外部参考
- [2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md](./2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md) — Channel/Chat/AgentTeam Flow
- [2026-05-26-MASTER-IMPLEMENTATION-PLAN.md](./2026-05-26-MASTER-IMPLEMENTATION-PLAN.md) — 主实施计划
- [2026-05-26-Memory-Code-Review.md](./2026-05-26-Memory-Code-Review.md) — Memory 代码审查
- [2026-05-26-Monitor-Code-Review.md](./2026-05-26-Monitor-Code-Review.md) — Monitor 代码审查
- [2026-05-26-Overview-Model-Hook-Knowledge-Artifact-Eval-Review.md](./2026-05-26-Overview-Model-Hook-Knowledge-Artifact-Eval-Review.md) — 概览/Model/Hook/Knowledge/Artifact/Eval
- [2026-05-26-Team-Graph-Code-Review.md](./2026-05-26-Team-Graph-Code-Review.md) — Team/Graph 代码审查
- [2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md](./2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md) — Tools/Plugin/Skill/MCP 代码审查
- [2026-05-26-Wave3-P0-Code-Review.md](./2026-05-26-Wave3-P0-Code-Review.md) — Wave3 P0 代码审查
- [2026-05-27-Business-Logic-Review.md](./2026-05-27-Business-Logic-Review.md) — 业务逻辑审查
- [2026-05-27-Full-Architecture-Code-Quality-Review.md](./2026-05-27-Full-Architecture-Code-Quality-Review.md) — 全架构代码质量审查
- [full-project-review.md](./full-project-review.md) — 全项目审查

---

*本文档由 AI 代码 Review 自动生成，截至 2026-05-27。所有分数和风险条目基于文档分析与代码结构扫描，需结合实际运行验证。*
