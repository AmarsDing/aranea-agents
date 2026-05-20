# Aranea-Agents 项目文档

> **AI 入口文档**：AI 在对本项目做任何代码改动前，**必须先阅读本文**。本文提供项目全貌与文档导航，AI 按需精确读取相关指导内容。

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
| 观测 | Prometheus（`/metrics`） + OpenTelemetry（计划中） |

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
6. 后端验证：make wire && make api && make build && make test && make runtime-boundary
7. 前端验证：cd web && pnpm i && pnpm lint && pnpm test && pnpm build
8. 更新文档：changelog/ + execution-plan.md（如涉及进度变更）
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
| **当前迭代进度与任务** | [guides/execution-plan.md](./guides/execution-plan.md) | 模块接入度、Top-20、里程碑、扩展红线 |
| **系统开发计划总览** | [需求/0-system-development.md](./需求/0-system-development.md) | 架构健康度、OpenClaw 对照、路线图、代码质量评价；AI 开发前必读 |
| **模块开发计划索引** | [需求/README-development.md](./需求/README-development.md) | 全部模块接入度、近期完成、建议下一步 |

### 5.2 需求与设计

| 场景 | 读取文档 | 说明 |
|------|----------|------|
| **系统架构总览** | [需求/0 系统框图.md](./需求/0%20系统框图.md) | 分层架构图、模块关系、数据流、依赖矩阵 |
| **某功能的产品需求** | `需求/<编号> <模块名>.md` | 用户故事、功能规格、验收标准 |
| **某功能的技术设计** | `需求/<编号> <模块名>.design.md` | 架构方案、接口设计、数据模型 |
| **某功能的开发计划** | `需求/<编号>-<模块名>-development.md` | 迭代计划、任务拆分 |
| **流程日志 / 链路排障** | [需求](./需求/52-flow-logger.md) · [设计](./需求/52-flow-logger.design.md) · [开发计划](./需求/52-flow-logger-development.md) · [步骤注册表](./guides/flow-log-step-registry.md) · [Slog 移除 changelog](./changelog/2026-05-20-FlowLog-V2-SlogRemoval.md) | TraceEmitter、trace_id、severity；**禁止 slog / SlogBridge** |

### 5.3 前端参考

| 场景 | 读取文档 | 说明 |
|------|----------|------|
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
