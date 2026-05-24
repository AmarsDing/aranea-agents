# Aranea-Agents 项目文档

> **AI 入口文档**：AI 在对本项目做任何代码改动前，**必须先阅读本文**。仓库根目录 [AGENTS.md](../AGENTS.md) 为 Cursor / Claude Code 快捷入口。本文提供项目全貌与文档导航，AI 按需精确读取相关指导内容。

---

## 1. 项目定位

**Aranea-Agents** 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核，提供可视化 Agent / Team / Graph 编排、会话管理、可观测记忆、Cron 调度、Skill / Plugin 治理与多模型接入能力。

---

## 2. 技术栈

| 层级 | 选型 |
|------|------|
| 后端 | Go + **Kratos v2**（HTTP / gRPC / WebSocket 传输、Wire DI） |
| Agent 运行时 | **trpc-agent-go**（Runner / Agent / Session / Memory / Tool / Event / Skill / Graph / Team / Planner / Plugin / Artifact / CodeExecutor / Knowledge / Evaluation） |
| 前端 | Vue 3 + Quasar + Pinia + TypeScript |
| 数据库 | SQLite（Ent ORM，单连接池）；向量 / 图可外挂 pgvector |
| 依赖注入 | Wire（编译期），`make wire` 生成；Proto 生成 `make api` |
| 观测 | Prometheus（`/metrics`）+ OTLP Trace/Metrics（`internal/telemetry`）+ FlowLog/Runs 投影 |

---

## 3. 双框架分工

Kratos v2 负责传输层，trpc-agent-go 负责 Agent 编排，二者互不越界。

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + 框架 Runner 装配
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义（禁止 import trpc-agent-go）
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite，单 sql.Open）
```

---

## 4. AI 编码工作流

```
1. 阅读本 README → 了解项目全貌 + 文档索引
2. 阅读对应规范文档（按 §5 索引精确读取）
3. 阅读对应需求文档与 *.design.md → 理解功能规格
4. 探索 / 查询代码 → 优先 CodeGraph（见 §4.1）；禁止未查文档就先 grep 扫符号
5. 按规范编码 → 遵循分层、依赖方向、红线、命名约定
6. 验证 → 迭代中用 [§4.2 分级验证](#42-分级验证按改动选跑)；提交 / PR 前跑全量链（见 §4.2 末行）
7. 更新文档：changelog/ + execution-plan.md（如涉及进度变更）
```

### 4.1 代码探索约束（CodeGraph）

本项目已初始化 CodeGraph（`.codegraph/` 存在）。AI **在动手改代码前**，对**结构性**问题必须优先走 CodeGraph MCP，而不是先用 grep / glob / 全库 Read 扫文件。

| 问题类型 | 优先工具 | 仍可用 grep / Read 的场景 |
|----------|----------|---------------------------|
| 符号在哪定义、签名是什么 | `codegraph_search` / `codegraph_node` | — |
| 谁调用了 X、X 调用了谁 | `codegraph_callers` / `codegraph_callees` | — |
| 改 X 会影响哪些代码 | `codegraph_impact` | — |
| 理解某模块 / 任务上下文 | `codegraph_context` / `codegraph_explore` | — |
| 查字符串字面量、注释、日志文案 | — | grep |
| 已打开文件内的局部阅读 | — | Read |

**禁止**：按符号名检索时 **grep 先于 CodeGraph**；CodeGraph 已返回的结构信息 **不得再用 grep 重复验证**。

索引未初始化时：先询问是否执行 `codegraph init -i`。细则见 [guides/AI-DEVELOPMENT-SPECIFICATION.md §速查卡](./guides/AI-DEVELOPMENT-SPECIFICATION.md#代码探索约束codegraph) 与 `.cursor/rules/codegraph.mdc`。

### 4.2 分级验证（按改动选跑）

> 开发迭代中用 **最小验证集** 缩短反馈循环；**提交 / PR 前** 仍须全量 CI（见下表末行）。

| 改动类型 | 最小验证 | 何时加码 |
|----------|----------|----------|
| 仅 `internal/service` + 单测 | `go test ./internal/service/... -run TestXxx -count=1` | 涉及 Runner / Turn / WS → `make runtime-boundary` |
| 仅 `internal/biz` / `internal/data` | `go test ./internal/biz/... ./internal/data/... -count=1` | 改 Ent schema → `go generate ./internal/data/ent && go build ./...` |
| `api/**/*.proto` | `make api && go build ./...` | 改 HTTP 注解 → `cd web && pnpm lint` |
| Wire 注入（`cmd/*/wire.go`、ProviderSet） | `make wire && go build ./cmd/admin` | — |
| 跨层 / Chat·Channel·Runner 边界 | 上表对应项 + `make runtime-boundary` | 对照 AI-DEVELOPMENT-SPECIFICATION 红线 |
| 前端 `web/src/**` | `cd web && pnpm lint && pnpm test && pnpm build` | 改 API 类型 → 先 `make api` |
| **提交前（全量）** | 后端：`make api && make wire && make build && make test && make lint && make runtime-boundary`；前端：`cd web && pnpm lint && pnpm test && pnpm build` | proto / wire 有改动时必须前置 `make api` / `make wire` |

单任务闭环示例：`CC-A-01` 改 service → `go test ./internal/service/... -run TestChannelIngress -count=1` → 通过后再跑 `make runtime-boundary`。

---

## 5. 文档索引（按场景精确读取）

### 5.1 编码规范

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **任何后端编码** | [guides/AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) | ★ **唯一后端编码行为准则**：红线、决策树、分层规则、运行时规范、API/Proto、Go 风格、自检清单 |
| **探索 / 查询代码结构** | 本文 [§4.1](./README.md#41-代码探索约束codegraph) + `.cursor/rules/codegraph.mdc` | CodeGraph 优先于 grep；符号、调用链、影响面、模块上下文 |
| **判断代码该放 Kratos 哪层** | [guides/kratos-framework-guide.md](./guides/kratos-framework-guide.md) | Kratos 各层职责边界、依赖方向、Proto/Wire/中间件/错误处理/配置 |
| **使用 trpc-agent-go 框架** | [guides/trpc-agent-go-framework.md](./guides/trpc-agent-go-framework.md) | 框架目录结构、核心接口、项目内桥接函数、常见实现场景、官方文档索引 |
| **任何前端编码** | [guides/frontend-guide.md](./guides/frontend-guide.md) | ★ **唯一前端编码行为准则**：数据流、分层、红线、UX 主题、迁移剧本 |
| **跨模块解耦 / 架构优化** | [需求/0-module-decoupling-architecture.md](./需求/0-module-decoupling-architecture.md) · [**§3.1 四层目标架构**](./需求/0-module-decoupling-architecture.md#31-推荐目标架构channel--chat--agent) | Chat / Channel / Agent 边界、Ingress/Policy/Turn/Projector、端口化路线 |
| **当前迭代进度与任务** | [guides/execution-plan.md](./guides/execution-plan.md) | 模块接入度、Top-20、里程碑、扩展红线 |
| **Chat × Channel 企业级蓝图** | [需求/55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南](./需求/55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南) | ★ 主链路架构 / UX 蓝图；**§1.5** 双投影 + Tier 评估；**§2.6** Run 升格（P-1 根本解）；**§13** 设计评审；AI 任务卡 |
| **Graph × Team × Multi-Agent 企业级蓝图** | [需求/53%20team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南](./需求/53%20team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南) | ★ M53 编排融合下一阶段（执行单链 / OrchestrationSpec v2 / Activity 时间线 / FailurePolicy 完整化）AI 落地任务卡 |
| **系统开发计划总览** | [需求/0-system-development.md](./需求/0-system-development.md) | 架构健康度、OpenClaw 对照、路线图、代码质量评价；AI 开发前必读 |
| **模块开发计划索引** | [需求/README-development.md](./需求/README-development.md) | 全部模块接入度、近期完成、建议下一步 |

### 5.2 需求与设计

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **系统架构总览** | [需求/0 系统框图.md](./需求/0%20系统框图.md) | 分层架构图、模块关系、数据流、依赖矩阵 |
| **某功能的产品需求** | `需求/<编号> <模块名>.md` | 用户故事、功能规格、验收标准 |
| **某功能的技术设计** | `需求/<编号> <模块名>.design.md` | 架构方案、接口设计、数据模型 |
| **某功能的开发计划** | `需求/<编号>-<模块名>-development.md` | 迭代计划、任务拆分（**实现差距以 `*-development.md` 为准**，需求/设计正文不写修复记录） |
| **Agent 模块 2–8 待办总览** | [需求/0-system-development.md](./需求/0-system-development.md) §8.11 · [2-8-agent-modules-development.md](./需求/2-8-agent-modules-development.md) · [README-development.md](./需求/README-development.md) | 创建/列表/分类/设置/文件/进化/顶栏 |
| **Session 会话** | [需求](./需求/10%20session.md) · [设计](./需求/10%20session.design.md) · [开发计划](./需求/10-session-development.md) · [**Phase2 Review**](./review/2026-05-24-Session-Phase2-Review.md) · [Review 基线](./review/10-session-review.md) | Pin/Export/Timeline UNION/Runs ✅；Participants 读时 Sync · P1 F6-a |
| **MCP 服务器 / Broker** | [需求](./需求/19%20mcp.md) · [设计](./需求/19%20mcp.design.md) · [开发计划](./需求/19-mcp-development.md) | CRUD、`internal/mcp/*`、统计/告警/用户凭据/validate；changelog [P3–P4](./changelog/2026-05-21-MCP-P3-P4.md)；AdHoc §8.2 |
| **Telemetry / OTLP / Prometheus** | [需求](./需求/24%20telemetry.md) · [设计](./需求/24%20telemetry.design.md) · [开发计划](./需求/24-telemetry-development.md) | 双轨观测；`telemetry.Init`；Runs Waterfall；EP-OBS / I6-TEL |
| **消息机制 / WebSocket** | [需求](./需求/51%20消息机制.md) · [后端设计](./需求/51a%20后端消息机制.md) · [前端设计](./需求/51b%20前端消息机制.md) · [开发计划](./需求/message-development.md) · [**Chat 全链路 Review**](./review/2026-05-23-Chat-Flow-Full-Review.md) · [trpc Review P1–P3](./changelog/2026-05-21-Message-Trpc-Review-P1-P3.md) · [Event 系统](./需求/34-event-development.md) | Envelope + EventBus；工具记录 `source=trpc`/`event_bus`；Team 成员历史与 WS 契约对齐 |
| **Channel × Agent × Team 业务集成** | [业务说明](./需求/17-channel-agent-team-integration.md) · [集成设计](./需求/17-channel-agent-team-integration.design.md) · [**外部参考借鉴手册**](./需求/17-channel-external-reference-playbook.md) · [M55 方案 §1.5](./需求/55-chat-channel-cursor-solution.md#15-架构收敛模型1-turn--2-projections--3-anchors) · [IM Preview](./changelog/2026-05-23-Channel-IM-Preview.md) · [IM Preview Review](./review/2026-05-23-Channel-IM-Preview-Review.md) · [IM Preview E2E](./需求/17-channel-development.md#12-im-preview--e2e-验收清单lt-0107) · [Chat 同步](./changelog/2026-05-22-Channel-Chat-Sync.md) · [入站根因](./changelog/2026-05-22-Channel-Inbound-Root-Cause.md) · [审查](./review/2026-05-22-Channel-Inbound-Review.md) · [**DECO-01 Holistic Fix Review**](./review/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md) · [Channel](./需求/17%20channel.md) · [**长任务 Phase E**](./需求/17-channel-development.md#10-长任务异步执行phase-e) | 飞书 IM Preview transcript + Card upsert + Session 深链；长任务 ACK/Job；GoClaw/OpenClaw 借鉴见 playbook |
| **M55 Chat×Channel×Cursor 对标** | [**整体方案**](./需求/55-chat-channel-cursor-solution.md) · [**开发计划**](./需求/55-chat-channel-cursor-development.md) · [**企业级蓝图与 AI 落地指南**](./需求/55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南) · [execution-plan §迭代 CC](./guides/execution-plan.md) · [Run Lifecycle Review](./review/2026-05-23-M55-Run-Lifecycle-Review.md) · [Run OPT 计划](./changelog/2026-05-23-M55-Run-Lifecycle-Optimization-Plan.md) · [**R-UX 格式化/思考 UX**](./changelog/2026-05-23-M55-Phase-R-UX-Channel-Format-Reasoning.md) · [changelog Phase A–D](./changelog/2026-05-23-M55-Phase-ABCD-Review-Fixes.md) | Phase A–D + R P0–P1 ✅；R-UX 格式化/思考/Session 同步 ✅ |
| **流程日志 / 链路排障** | [需求](./需求/52-flow-logger.md) · [设计](./需求/52-flow-logger.design.md)（含 §5.1 步骤注册表）· [开发计划](./需求/52-flow-logger-development.md) · [Phase 2 changelog](./changelog/2026-05-21-Message-FlowLogger-Phase2-P3.md) · [Slog 移除](./changelog/2026-05-20-FlowLog-V2-SlogRemoval.md) | TraceEmitter、trace_id、severity；**禁止 slog / SlogBridge**；`ListFlowLogs` + 落库 ✅ |
| **Memory L0–L4** | [**索引**](./需求/memory/README.md) · [总需求](./需求/memory/memory.md) · [总设计](./需求/memory/memory.design.md) · [开发计划](./需求/memory/memory-development.md) · [理论](./需求/memory/theory.md) · [Review](./review/memory-review.md) · [L0–L4](./需求/memory/L0.md) · [Legacy 迁移](./changelog/2026-05-24-Memory-Legacy-Backfill-Startup.md) | 五层 + Ledger/Views/Policy；Legacy（旧 trpc_memory）见 design §3.1/§十一 |
| **Tools 片段编辑** | [需求](./需求/23%20tools-fragment-edit.md) · [设计](./需求/23%20tools-fragment-edit.design.md) · [开发计划 Phase 4](./需求/23-tools-development.md#phase-4片段级文件编辑p1) · [Tools 总览](./需求/23%20tools.md) · [Review Phase 4](./review/2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) · [Review Phase 5](./review/2026-05-22-Tools-Phase5-Workspace-Unification-Review.md) · [23-tools-review](./review/23-tools-review.md) | Phase 4+5 ✅ |
| **Team × Graph 编排融合** | [需求](./需求/53%20team-graph-orchestration.md) · [设计](./需求/53%20team-graph-orchestration.design.md) · [开发计划 §8 终态](./需求/53-team-graph-orchestration-development.md#8-终态路线图team-规格--graph-执行单链) · [**企业级蓝图与 AI 落地指南**](./需求/53%20team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南) · [Phase 4 changelog](./changelog/2026-05-23-Team-Graph-M53-Phase4.md) · [Phase 4 优化](./changelog/2026-05-23-Team-Graph-M53-Phase4-Optimization.md) | OrchestrationSpec · 编译/观测单链 · 执行收敛 Phase 5–7 |
| **Hermes Kanban 适配（M54）** | [需求](./需求/54-hermes-kanban.md) · [设计](./需求/54-hermes-kanban.design.md) · [开发计划](./需求/54-hermes-kanban-development.md) | Graph Task 运行时 ✅ · kanban_* tools ✅ · TaskDispatcher ✅ · 双 Kanban UI ✅ · **G14 spawn + Hermes UI 对标 📋** |
| **Runner / Gateway** | [需求](./需求/40%20runner.md) · [设计](./需求/40%20runner.design.md) · [开发计划](./需求/40-runner-development.md) | RunRegistry / RunnerManager / RunGateway / ChatUsecase / Webhook |
| **Planner 规划器** | [需求](./需求/39%20planner.md) · [设计](./需求/39%20planner.design.md) · [开发计划](./需求/39-planner-development.md) | planner_kind / ReAct / A2UI 组件树 / tool 去重 |
| **CodeExecutor** | [需求](./需求/32%20codeexecutor.md) · [设计](./需求/32%20codeexecutor.design.md) · [开发计划](./需求/32-codeexecutor-development.md) | Factory / Agent 配置 / capabilities / lazy E2B |
| **Callback / Hook** | [需求](./需求/28%20callback.md) · [设计](./需求/28%20callback.design.md) · [开发计划](./需求/28-callback-development.md) | Callback 规则 / Hook 投递 / Phase 1–3 |
| **Avatar 头像** | [开发计划](./需求/50-avatar-development.md) | Agent 头像选择器 / Channel avatar seed |
| **CLI 技术预览** | [需求](./需求/25%20cli.md) · [设计](./需求/25%20cli.design.md) · [开发计划](./需求/25-cli-development.md) | 非核心可用；早期占位 |
| **TTS 技术预览** | [需求](./需求/tts.md) · [开发计划](./需求/tts-development.md) | 占位模块；无生产 SLA |
| **Event 事件系统** | [开发计划](./需求/34-event-development.md) | EventBus 注册/分发；与 WS Envelope 协同；Monitor/Team/Chat 事件源 |

### 5.3 前端参考

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **页面与功能总览** | [需求/frontend-pages.md](./需求/frontend-pages.md) | ★ 路由、侧栏、各页能力、features/stores 对照（基于代码梳理） |
| **UX 主题与视觉规范** | [frontend/UX.md](./frontend/UX.md) | 日夜双模、玻璃材质、CSS 变量、排版、组件数值 |
| **Vue 架构与分层** | [frontend/vue-design/vue-design.md](./frontend/vue-design/vue-design.md) | 数据流、目录映射、各层细则、迁移剧本、端到端示例 |
| **AI 系统提示精简版** | [frontend/vue-design/vue-design-agent-rules.md](./frontend/vue-design/vue-design-agent-rules.md) | MUST/MUST NOT 英文版，便于粘贴到系统提示 |

### 5.4 运维与历史

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **数据库初始化** | `sql/00_init.sql` → `sql/99_indexes.sql` | 按编号顺序执行 |
| **Grafana 仪表盘** | [observability/grafana-aranea.json](./observability/grafana-aranea.json) | 导入 Grafana |
| **变更记录** | `changelog/*.md` | 只读历史，按日期-主题命名 |
| **开发日志** | `devlog/*.md` | 实现细节、审计快照 |
| **废弃文档** | `_deprecated/` | 已冻结的历史快照，仅供参考 |

---

## 6. 文档组织规则

| 文档类型 | 目录 | 内容边界 | 禁止包含 |
|----------|------|----------|----------|
| **编码规范** | `guides/AI-DEVELOPMENT-SPECIFICATION.md` | 后端红线、决策树、分层规则、运行时规范 | 进度 / 前端内容 |
| **前端规范** | `guides/frontend-guide.md` | 前端红线、数据流、分层、UX 主题 | 后端内容 |
| **Kratos 框架** | `guides/kratos-framework-guide.md` | Kratos 各层职责与约束 | 通用教程 / 示例代码 |
| **trpc 框架** | `guides/trpc-agent-go-framework.md` | 框架接口与项目映射 | 通用教程 |
| **解耦指导** | `需求/0-module-decoupling-architecture.md` | 跨模块边界、依赖方向、端口化路线、AI 迁移模板 | 单次需求进度 / 具体修复记录 |
| **进度真相** | `guides/execution-plan.md` | 模块接入度 / 里程碑 / 红线扩展 | 已冻结的 sprint 节奏 |
| **需求文档** | `需求/*.md` | 用户故事、功能规格、验收标准 | 实现细节、代码片段 |
| **设计文档** | `需求/*.design.md` | 架构方案、接口设计、数据模型 | 修复记录、待办 |
| **变更记录** | `changelog/` | 日期、模块、变更摘要 | 代码片段、修复过程 |
| **开发日志** | `devlog/` | 实现细节、修复记录、审计快照 | — |
| **废弃文档** | `_deprecated/` | 已冻结的历史快照 | 任何新增 / 修改内容 |

---

## 7. 目录结构总览

```
docs/
├── README.md                              ← 你正在阅读的入口文档
├── guides/                                ← 编码规范 + 执行基线
│   ├── AI-DEVELOPMENT-SPECIFICATION.md     ← ★ 后端编码唯一行为准则
│   ├── frontend-guide.md                   ← ★ 前端编码唯一行为准则
│   ├── kratos-framework-guide.md           ← Kratos 框架层职责速查
│   ├── trpc-agent-go-framework.md          ← trpc-agent-go 框架接口速查
│   └── execution-plan.md                   ← 当前迭代进度与任务
├── frontend/                              ← 前端参考文档
│   ├── UX.md                               ← UX 主题与视觉规范
│   └── vue-design/
│       ├── vue-design.md                   ← Vue 架构与分层详细规范
│       └── vue-design-agent-rules.md       ← AI 系统提示精简版
├── 需求/                                  ← 产品需求 + 设计文档 + 开发计划
│   ├── 0 系统框图.md                       ← ★ 系统架构总览
│   ├── 0-module-decoupling-architecture.md ← 跨模块解耦 + 后端优化路线
│   ├── README-development.md               ← 模块开发计划索引
│   ├── *-development.md                    ← 各模块开发计划
│   ├── *.md                                ← 纯需求内容
│   └── *.design.md                         ← 纯设计内容
├── changelog/                             ← 变更摘要（只读历史）
├── devlog/                                ← 开发日志
├── observability/                         ← Grafana 仪表盘
├── sql/                                   ← 数据库初始化 SQL
├── scenarios/                             ← 场景示例
├── skills/                                ← 技能模板
└── _deprecated/                           ← 已冻结的历史快照
```

---

## 8. Tools 运维配置（摘录）

### 8.1 Tool 结果缓存（Catalog）

对幂等、只读类工具，可在 Tool 目录行的 `metadata_json` 或 `config_json` 中启用进程内结果缓存（TTL 默认 300 秒）：

```json
{"cache_enabled": true, "cache_ttl_sec": 300}
```

命中缓存时跳过实际工具执行；适用于 search、fetch 等重复查询场景。详见 [23-tools-development.md](./需求/23-tools-development.md)。

### 8.2 MCP Broker AdHoc HTTP

生产环境默认**禁止** MCP Broker 通过 `mcp_call` 连接未预注册的 HTTP 端点。需同时满足：

1. MCP Server 配置中 `allow_adhoc_http: true`
2. 平台 **系统设置**（`/settings`）开启「允许 MCP Broker AdHoc HTTP」→ 写入 `system_settings.mcp_allow_adhoc_http`

保存后立即作用于后续 Agent 工具装配，无需重启。
