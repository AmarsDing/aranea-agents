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
6. 验证 → 迭代中用 §4.2 分级验证；提交 / PR 前跑全量链
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

索引未初始化时：先询问是否执行 `codegraph init -i`。细则见 [AI-DEVELOPMENT-SPECIFICATION.md §速查卡](../docs/guides/AI-DEVELOPMENT-SPECIFICATION.md#代码探索约束codegraph) 与 `.cursor/rules/codegraph.mdc`。

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

---

## 5. 文档索引（按场景精确读取）

### 5.1 编码规范

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **任何后端编码** | [AI-DEVELOPMENT-SPECIFICATION.md](../docs/guides/AI-DEVELOPMENT-SPECIFICATION.md) | ★ **唯一后端编码行为准则**：红线、决策树、分层规则、运行时规范、API/Proto、Go 风格、自检清单 |
| **CLI 快速上手（aranea 命令行）** | [cli-quickstart.md](../docs/guides/cli-quickstart.md) | 安装、登录、agent/skill/tool 管理、全局选项、常见问题 |
| **探索 / 查询代码结构** | 本文 [§4.1](./README.md#41-代码探索约束codegraph) + `.cursor/rules/codegraph.mdc` | CodeGraph 优先于 grep；符号、调用链、影响面、模块上下文 |
| **判断代码该放 Kratos 哪层** | [kratos-framework-guide.md](../docs/guides/kratos-framework-guide.md) | Kratos 各层职责边界、依赖方向、Proto/Wire/中间件/错误处理/配置 |
| **使用 trpc-agent-go 框架** | [trpc-agent-go-framework.md](../docs/guides/trpc-agent-go-framework.md) | 框架目录结构、核心接口、项目内桥接函数、常见实现场景、官方文档索引 |
| **Agent Prompt 组装 / 排障** | [prompt/README.md](../docs/guides/prompt/README.md) · [assembly.md](../docs/guides/prompt/assembly.md) | 构建期 System Instruction、运行时 Processor 链、L2/L3 记忆、Intent Pass、附件与源码入口 |
| **任何前端编码** | [frontend-guide.md](../docs/guides/frontend-guide.md) | ★ **唯一前端编码行为准则**：数据流、分层、红线、UX 主题、迁移剧本 |
| **跨模块解耦 / 架构优化** | [0-module-decoupling-architecture.md](./requirements/0-module-decoupling-architecture.md) · [**§3.1 四层目标架构**](./requirements/0-module-decoupling-architecture.md#31-推荐目标架构channel--chat--agent) | Chat / Channel / Agent 边界、Ingress/Policy/Turn/Projector、端口化路线 |
| **当前迭代进度与任务** | [execution-plan.md](../docs/guides/execution-plan.md) | 模块接入度、Top-20、里程碑、扩展红线 |
| **Chat × Channel 企业级蓝图** | [55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南](./requirements/55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南) | ★ 主链路架构 / UX 蓝图；**§1.5** 双投影 + Tier 评估；**§2.6** Run 升格（P-1 根本解）；**§13** 设计评审；AI 任务卡 |
| **Graph × Team × Multi-Agent 企业级蓝图** | [53%20team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南](./requirements/53%20team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南) | ★ M53 编排融合下一阶段（执行单链 / OrchestrationSpec v2 / Activity 时间线 / FailurePolicy 完整化）AI 落地任务卡 |
| **系统开发计划总览** | [0-system-development.md](./requirements/0-system-development.md) | 架构健康度、OpenClaw 对照、路线图、代码质量评价；AI 开发前必读 |
| **模块开发计划索引** | [README-development.md](./requirements/README-development.md) | 全部模块接入度、近期完成、建议下一步 |

### 5.2 需求与设计

> **命名规范**：需求目录文件统一使用 `<编号>-<模块名>.md` 格式（连字符分隔）。每个模块通常包含三件套：需求 `*.md`、设计 `*.design.md`、开发计划 `*-development.md`。

| 模块 | 编号 | 需求 | 设计 | 开发计划 |
|------|------|------|------|----------|
| 系统架构总览 | 0 | [0-system-diagram.md](./requirements/0-system-diagram.md) | — | [0-system-development.md](./requirements/0-system-development.md) |
| Chat 对话 | 1 | [1-chat.md](./requirements/1-chat.md) | [1-chat.design.md](./requirements/1-chat.design.md) | [1-chat-development.md](./requirements/1-chat-development.md) |
| Agent 创建 | 2 | [2-agents-create.md](./requirements/2-agents-create.md) | [2-agents-create.design.md](./requirements/2-agents-create.design.md) | [2-agents-create-development.md](./requirements/2-agents-create-development.md) |
| Agent 列表 | 3 | [3-agent-list.md](./requirements/3-agent-list.md) | [3-agent-list.design.md](./requirements/3-agent-list.design.md) | [3-agent-list-development.md](./requirements/3-agent-list-development.md) |
| Agent 类型 | 4 | [4-agent-type.md](./requirements/4-agent-type.md) | [4-agent-type.design.md](./requirements/4-agent-type.design.md) | [4-agent-type-development.md](./requirements/4-agent-type-development.md) |
| Agent 设置 | 5 | [5-agent-setting.md](./requirements/5-agent-setting.md) | [5-agent-setting.design.md](./requirements/5-agent-setting.design.md) | [5-agent-setting-development.md](./requirements/5-agent-setting-development.md) |
| Agent 文件 | 6 | [6-agent-setting-file.md](./requirements/6-agent-setting-file.md) | [6-agent-setting-file.design.md](./requirements/6-agent-setting-file.design.md) | [6-agent-setting-file-development.md](./requirements/6-agent-setting-file-development.md) |
| Agent 进化 | 7 | [7-agent-evolution.md](./requirements/7-agent-evolution.md) | [7-agent-evolution.design.md](./requirements/7-agent-evolution.design.md) | [7-agent-evolution-development.md](./requirements/7-agent-evolution-development.md) |
| Agent 顶栏 | 8 | [8-agent-title.md](./requirements/8-agent-title.md) | [8-agent-title.design.md](./requirements/8-agent-title.design.md) | [8-agent-title-development.md](./requirements/8-agent-title-development.md) |
| Provider | 9 | [9-provider.md](./requirements/9-provider.md) | [9-provider.design.md](./requirements/9-provider.design.md) | [9-provider-development.md](./requirements/9-provider-development.md) |
| Session | 10 | [10-session.md](./requirements/10-session.md) | [10-session.design.md](./requirements/10-session.design.md) | [10-session-development.md](./requirements/10-session-development.md) |
| Team / Multi-Agent | 11 | [11-multi-agent.md](./requirements/11-multi-agent.md) | [11-multi-agent.design.md](./requirements/11-multi-agent.design.md) | [11-multi-agent-development.md](./requirements/11-multi-agent-development.md) |
| Memory L0–L4 | 12–16 | [memory/](./requirements/memory/) | [memory/](./requirements/memory/) | [memory-development.md](./requirements/memory/memory-development.md) |
| Channel | 17 | [17-channel.md](./requirements/17-channel.md) | [17-channel.design.md](./requirements/17-channel.design.md) | [17-channel-development.md](./requirements/17-channel-development.md) |
| Monitor | 18 | [18-monitor.md](./requirements/18-monitor.md) | [18-monitor.design.md](./requirements/18-monitor.design.md) | [18-monitor-development.md](./requirements/18-monitor-development.md) |
| MCP | 19 | [19-mcp.md](./requirements/19-mcp.md) | [19-mcp.design.md](./requirements/19-mcp.design.md) | [19-mcp-development.md](./requirements/19-mcp-development.md) |
| Skill | 20 | [20-skill.md](./requirements/20-skill.md) | [20-skill.design.md](./requirements/20-skill.design.md) | [20-skill-development.md](./requirements/20-skill-development.md) |
| Cron | 21 | [21-cron.md](./requirements/21-cron.md) | [21-cron.design.md](./requirements/21-cron.design.md) | [21-cron-development.md](./requirements/21-cron-development.md) |
| Plugin | 22 | [22-plugin.md](./requirements/22-plugin.md) | [22-plugin.design.md](./requirements/22-plugin.design.md) | [22-plugin-development.md](./requirements/22-plugin-development.md) |
| Tools | 23 | [23-tools.md](./requirements/23-tools.md) | [23-tools.design.md](./requirements/23-tools.design.md) | [23-tools-development.md](./requirements/23-tools-development.md) |
| Telemetry | 24 | [24-telemetry.md](./requirements/24-telemetry.md) | [24-telemetry.design.md](./requirements/24-telemetry.design.md) | [24-telemetry-development.md](./requirements/24-telemetry-development.md) |
| CLI（技术预览） | 25 | [25-cli.md](./requirements/25-cli.md) | [25-cli.design.md](./requirements/25-cli.design.md) | [25-cli-development.md](./requirements/25-cli-development.md) |
| A2A | 26 | [26-a2a-protocol.md](./requirements/26-a2a-protocol.md) | [26-a2a-protocol.design.md](./requirements/26-a2a-protocol.design.md) | [26-a2a-development.md](./requirements/26-a2a-development.md) |
| Artifact | 27 | [27-artifact.md](./requirements/27-artifact.md) | [27-artifact.design.md](./requirements/27-artifact.design.md) | [27-artifact-development.md](./requirements/27-artifact-development.md) |
| Callback | 28 | [28-callback.md](./requirements/28-callback.md) | [28-callback.design.md](./requirements/28-callback.design.md) | [28-callback-development.md](./requirements/28-callback-development.md) |
| Token / Usage | 29 | [29-token.md](./requirements/29-token.md) | [29-token.design.md](./requirements/29-token.design.md) | [29-token-development.md](./requirements/29-token-development.md) |
| Ecosystem | 30 | [30-ecosystem.md](./requirements/30-ecosystem.md) | [30-ecosystem.design.md](./requirements/30-ecosystem.design.md) | [30-ecosystem-development.md](./requirements/30-ecosystem-development.md) |
| CodeExecutor | 32 | [32-codeexecutor.md](./requirements/32-codeexecutor.md) | [32-codeexecutor.design.md](./requirements/32-codeexecutor.design.md) | [32-codeexecutor-development.md](./requirements/32-codeexecutor-development.md) |
| Evaluation | 33 | [33-evaluation.md](./requirements/33-evaluation.md) | [33-evaluation.design.md](./requirements/33-evaluation.design.md) | [33-evaluation-development.md](./requirements/33-evaluation-development.md) |
| Event System | 34 | [34-event-system.md](./requirements/34-event-system.md) | [34-event-system.design.md](./requirements/34-event-system.design.md) | [34-event-development.md](./requirements/34-event-development.md) |
| Gateway | 35 | [35-gateway.md](./requirements/35-gateway.md) | [35-gateway.design.md](./requirements/35-gateway.design.md) | [35-gateway-development.md](./requirements/35-gateway-development.md) |
| Graph 工作流 | 36 | [36-graph-workflow.md](./requirements/36-graph-workflow.md) | [36-graph-workflow.design.md](./requirements/36-graph-workflow.design.md) | [36-graph-development.md](./requirements/36-graph-development.md) |
| Knowledge | 37 | [37-knowledge.md](./requirements/37-knowledge.md) | [37-knowledge.design.md](./requirements/37-knowledge.design.md) | [37-knowledge-development.md](./requirements/37-knowledge-development.md) |
| Planner | 39 | [39-planner.md](./requirements/39-planner.md) | [39-planner.design.md](./requirements/39-planner.design.md) | [39-planner-development.md](./requirements/39-planner-development.md) |
| Runner | 40 | [40-runner.md](./requirements/40-runner.md) | [40-runner.design.md](./requirements/40-runner.design.md) | [40-runner-development.md](./requirements/40-runner-development.md) |
| Avatar | 50 | [50-avatar.md](./requirements/50-avatar.md) | [50-avatar.design.md](./requirements/50-avatar.design.md) | [50-avatar-development.md](./requirements/50-avatar-development.md) |
| 消息机制 | 51 | [51-message-mechanism.md](./requirements/51-message-mechanism.md) | [51a-backend-message-mechanism.md](./requirements/51a-backend-message-mechanism.md) · [51b-frontend-message-mechanism.md](./requirements/51b-frontend-message-mechanism.md) | [message-development.md](./requirements/message-development.md) |
| FlowLogger | 52 | [52-flow-logger.md](./requirements/52-flow-logger.md) | [52-flow-logger.design.md](./requirements/52-flow-logger.design.md) | [52-flow-logger-development.md](./requirements/52-flow-logger-development.md) |
| Team × Graph (M53) | 53 | [53-team-graph-orchestration.md](./requirements/53-team-graph-orchestration.md) | [53-team-graph-orchestration.design.md](./requirements/53-team-graph-orchestration.design.md) | [53-team-graph-orchestration-development.md](./requirements/53-team-graph-orchestration-development.md) |
| Hermes Kanban (M54) | 54 | [54-hermes-kanban.md](./requirements/54-hermes-kanban.md) | [54-hermes-kanban.design.md](./requirements/54-hermes-kanban.design.md) | [54-hermes-kanban-development.md](./requirements/54-hermes-kanban-development.md) |
| Chat×Channel (M55) | 55 | [55-chat-channel-cursor-solution.md](./requirements/55-chat-channel-cursor-solution.md) | — | [55-chat-channel-cursor-development.md](./requirements/55-chat-channel-cursor-development.md) |
| TTS（技术预览） | — | [tts.md](./requirements/tts.md) | — | [tts-development.md](./requirements/tts-development.md) |
| Admin / Auth | — | [admin-auth.md](./requirements/admin-auth.md) | [admin-auth.design.md](./requirements/admin-auth.design.md) | [admin-auth-development.md](./requirements/admin-auth-development.md) |

### 5.3 前端参考

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **页面与功能总览** | [frontend-pages.md](./requirements/frontend-pages.md) | ★ 路由、侧栏、各页能力、features/stores 对照 |
| **UX 主题与视觉规范** | `aranea-frontend-guide` SKILL §6 | 日夜双模、玻璃材质、CSS 变量、排版、组件数值 |
| **Vue 架构与分层** | `aranea-frontend-guide` SKILL §3 | 数据流、目录映射、各层细则 |
| **AI 系统提示精简版** | `aranea-frontend-guide` SKILL §1（红线） | MUST/MUST NOT |
| **Registry 表格规范** | `aranea-frontend-guide` SKILL §8 | q-table 列定义、对齐、列宽与样式落点 |

### 5.4 运维与历史

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **数据库初始化** | `sql/00_init.sql` → `sql/99_indexes.sql`（[../docs/sql/](../docs/sql/)） | 按编号顺序执行 |
| **Grafana 仪表盘** | [grafana-aranea.json](../docs/observability/grafana-aranea.json) | 导入 Grafana |
| **变更记录** | [changelog/](./changelog/) | 只读历史，按日期-主题命名 |
| **开发日志** | [devlog/](./devlog/) | 实现细节、审计快照 |
| **模块 Review** | [review/](../docs/review/) | 模块评分、风险清单、架构红线核查 |
| **废弃文档** | [_deprecated/](../docs/_deprecated/) | 已冻结的历史快照，仅供参考 |

### 5.5 分析报告

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **竞品分析** | [competitive-analysis/](./reports/competitive-analysis/) | 竞品功能对比、市场定位分析 |
| **竞品差距与需求** | [competitive-gap-requirements/](./reports/competitive-gap-requirements/) | 竞品差距识别、需求优先级映射 |
| **前端审计报告** | [frontend-audit-report/](./reports/frontend-audit-report/) | 前端架构审计、技术债务评估、优化建议 |

### 5.6 问题与方案

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **日志问题** | [logging-issues.md](./issues/logging-issues.md) | 日志系统已知问题与解决方案 |
| **SQLite 问题** | [sqlite-issues.md](./issues/sqlite-issues.md) | SQLite 使用中的问题与优化方案 |
| **Team / Graph 问题** | [team-graph-issues.md](./issues/team-graph-issues.md) | Team 与 Graph 编排中的问题与方案 |
| **Tools UI 问题** | [tools-ui-issues.md](./issues/tools-ui-issues.md) | 工具管理界面问题与改进方案 |

---

## 6. 文档组织规则

| 文档类型 | 目录 | 内容边界 | 禁止包含 |
|----------|------|----------|----------|
| **编码规范** | `../docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` | 后端红线、决策树、分层规则、运行时规范 | 进度 / 前端内容 |
| **前端规范** | `../docs/guides/frontend-guide.md` | 前端红线、数据流、分层、UX 主题 | 后端内容 |
| **Kratos 框架** | `../docs/guides/kratos-framework-guide.md` | Kratos 各层职责与约束 | 通用教程 / 示例代码 |
| **trpc 框架** | `../docs/guides/trpc-agent-go-framework.md` | 框架接口与项目映射 | 通用教程 |
| **Prompt 组装** | `../docs/guides/prompt/` | 构建期 Instruction、运行时 Processor、记忆/Intent/附件 | 单次需求进度 |
| **解耦指导** | `./requirements/0-module-decoupling-architecture.md` | 跨模块边界、依赖方向、端口化路线 | 单次需求进度 / 具体修复记录 |
| **进度真相** | `../docs/guides/execution-plan.md` | 模块接入度 / 里程碑 / 红线扩展 | 已冻结的 sprint 节奏 |
| **需求文档** | `./requirements/*.md` | 用户故事、功能规格、验收标准 | 实现细节、代码片段 |
| **设计文档** | `./requirements/*.design.md` | 架构方案、接口设计、数据模型 | 修复记录、待办 |
| **变更记录** | `./changelog/` | 日期、模块、变更摘要 | 代码片段、修复过程 |
| **开发日志** | `./devlog/` | 实现细节、修复记录、审计快照 | — |
| **废弃文档** | `../docs/_deprecated/` | 已冻结的历史快照 | 任何新增 / 修改内容 |
| **分析报告** | `./reports/` | 竞品分析、差距需求、审计报告 | 实现细节、代码片段 |
| **问题与方案** | `./issues/` | 已知问题、根因分析、解决方案 | 需求规格、设计文档 |

---

## 7. 目录结构总览

```
项目根/
├── openspec/                             ← 文档主枢纽 + OpenSpec 变更管理
│   ├── README.md                         ← 你正在阅读的入口文档
│   ├── config.yaml                       ← OpenSpec 配置（项目上下文 + 规则）
│   ├── specs/                            ← 主规格库（稳定参考文档）
│   │   ├── README.md                     ← 规格库索引
│   │   ├── architecture-blueprint.md     ← 架构蓝图（真相源）
│   │   ├── backend-layers.md             ← 后端分层规则速查
│   │   ├── frontend-layers.md            ← 前端分层规则速查
│   │   ├── module-cross-reference.md     ← 模块交叉参考
│   │   ├── review-dimension-checklists.md ← 代码审查维度清单
│   │   └── built-in-tools-guide.md       ← 内置工具指南
│   ├── requirements/                     ← 产品需求 + 设计文档 + 开发计划（原 需求/）
│   ├── reports/                          ← 分析报告
│   │   ├── competitive-analysis/         ← 竞品分析
│   │   ├── competitive-gap-requirements/ ← 竞品差距与需求
│   │   └── frontend-audit-report/        ← 前端审计报告
│   ├── issues/                           ← 问题与方案
│   │   ├── logging-issues.md             ← 日志问题
│   │   ├── sqlite-issues.md              ← SQLite 问题
│   │   ├── team-graph-issues.md          ← Team / Graph 问题
│   │   └── tools-ui-issues.md            ← Tools UI 问题
│   ├── changelog/                        ← 变更摘要（只读历史）
│   ├── devlog/                           ← 开发日志
│   ├── changes/                          ← 活跃变更（每个变更一个目录）
│   │   ├── <change-name>/
│   │   │   ├── .openspec.yaml            ← 变更元数据
│   │   │   ├── proposal.md               ← 为什么做（目标 + 非目标）
│   │   │   ├── design.md                 ← 怎么做（架构方案）
│   │   │   ├── specs/                    ← 做成什么样（需求规格）
│   │   │   └── tasks.md                  ← 分几步做（实施计划 + DoD）
│   │   └── archive/                      ← 已归档变更
│   └── (由 openspec CLI 管理)
├── docs/
│   ├── guides/                           ← 编码规范 + 执行基线
│   │   ├── AI-DEVELOPMENT-SPECIFICATION.md ← ★ 后端编码唯一行为准则
│   │   ├── frontend-guide.md             ← ★ 前端编码唯一行为准则
│   │   ├── kratos-framework-guide.md     ← Kratos 框架层职责速查
│   │   ├── trpc-agent-go-framework.md    ← trpc-agent-go 框架接口速查
│   │   ├── prompt/                       ← Agent Prompt 组装指南
│   │   └── execution-plan.md             ← 当前迭代进度与任务
│   ├── review/                           ← 模块 Review 评分与风险清单
│   ├── observability/                    ← Grafana 仪表盘
│   ├── sql/                              ← 数据库初始化 SQL
│   ├── scenarios/                        ← 场景示例
│   ├── frontend/                         ← 前端相关文档
│   ├── _deprecated/                      ← 已冻结的历史快照
│   └── (其他技术文档)
└── .trae/skills/                         ← Trae AI 技能
    ├── aranea-coding-guide/              ← 后端编码指南
    ├── aranea-frontend-guide/            ← 前端编码指南
    ├── aranea-frontend-review/           ← 前端代码审查
    ├── aranea-review/                    ← 全栈代码审查
    ├── aranea-test-loop/                 ← 测试循环
    ├── go-oop-guide/                     ← Go OOP 编程指导
    ├── go-oop-review/                    ← Go OOP 代码审查
    ├── vue-frontend-guide/               ← Vue 3 编程指导
    ├── openspec-explore/                 ← OpenSpec 探索模式
    ├── openspec-propose/                 ← OpenSpec 提案创建
    ├── openspec-apply-change/            ← OpenSpec 实施执行
    ├── openspec-archive-change/          ← OpenSpec 归档
    └── superpowers-workflow/             ← Superpowers 开发纪律（TDD+审查+验证）
```

---

## 8. Tools 运维配置（摘录）

### 8.1 Tool 结果缓存（Catalog）

对幂等、只读类工具，可在 Tool 目录行的 `metadata_json` 或 `config_json` 中启用进程内结果缓存（TTL 默认 300 秒）：

```json
{"cache_enabled": true, "cache_ttl_sec": 300}
```

命中缓存时跳过实际工具执行；适用于 search、fetch 等重复查询场景。详见 [23-tools-development.md](./requirements/23-tools-development.md)。

### 8.2 MCP Broker AdHoc HTTP

生产环境默认**禁止** MCP Broker 通过 `mcp_call` 连接未预注册的 HTTP 端点。需同时满足：

1. MCP Server 配置中 `allow_adhoc_http: true`
2. 平台 **系统设置**（`/settings`）开启「允许 MCP Broker AdHoc HTTP」→ 写入 `system_settings.mcp_allow_adhoc_http`

保存后立即作用于后续 Agent 工具装配，无需重启。
