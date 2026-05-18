# Aranea-Agents 项目文档

> **AI 入口文档**：AI 在对本项目做任何代码改动或新功能实现前，**必须先阅读本文**。本文提供项目全貌、强制规范索引、文档导航与文档治理规则。
>
> 📌 **最新执行基线**：[`guides/execution-plan.md`](./guides/execution-plan.md) — 2026-05-17 起的**唯一权威进度与路线图**。所有原 master-plan / plan / implementation-plan / task-tracker / sprints 文件已**冻结废弃**，仅作历史快照保留。

---

## 0. AI 进入项目必读顺序

按以下顺序阅读，不可跳过：

| 顺序 | 文档 | 用途 |
|------|------|------|
| **1** | 本 README（你正在阅读） | 项目全貌、文档导航、文档治理规则 |
| **2** | [guides/AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) | **唯一编码行为准则**：分层规则、运行时边界、Proto/API、Go/前端风格、UI/UX、自检清单 |
| **3** | [guides/execution-plan.md](./guides/execution-plan.md) | **当前迭代真相**：模块接入度、立即可执行 Top-20、M0–M5 里程碑、扩展红线（R10/R13–R18）、AI 协作约束 |
| **4** | [guides/trpc-agent-go-framework.md](./guides/trpc-agent-go-framework.md) | trpc-agent-go 框架工程化解读（涉及 Agent/Runner/Model/Session/Memory/Tool 时必读） |
| **5** | [需求/0 系统框图.md](./需求/0%20系统框图.md) | 系统架构总览：分层、模块关系、数据流、依赖矩阵 |
| **6** | `docs/需求/` 对应模块文档 | 产品需求、功能规格与运维指南（§6） |

**规范冲突优先级**：`execution-plan.md` 扩展红线（R10/R13–R18）> AI-DEVELOPMENT-SPECIFICATION > 需求文档 > 历史 plan/master-plan。

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
| 观测 | Prometheus（`/metrics`） + OpenTelemetry（计划中，EP-OBS-01） |

---

## 3. 双框架分工

Kratos v2 负责传输层，trpc-agent-go 负责 Agent 编排，二者互不越界。详细规则见 [AI-DEVELOPMENT-SPECIFICATION.md §1.1–1.3](./guides/AI-DEVELOPMENT-SPECIFICATION.md#11-双框架分工)。

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + 框架 Runner 装配
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义（禁止 import trpc-agent-go）
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite，单 sql.Open）
```

**强制红线汇总**（详见 `AI-DEVELOPMENT-SPECIFICATION.md` + `execution-plan.md` §6）：
- R1：biz 层不得 import `trpc.group/trpc-go/trpc-agent-go/**`
- R2：SQLite 全进程**唯一** `sql.Open`，所有 Repo / trpc 适配器复用同一 `*sql.DB`
- R3：除测试外不得手写 `http.ListenAndServe` / `http.HandleFunc`
- R4：`internal/server/` 不得手写 `HandleFunc`，必须使用 `api/**` 注册函数
- R10（强化）：所有 `go func()` **必须**走 `pkg/safego.Go` / `pkg/safego.GoRecover`
- R13–R18：见 `execution-plan.md` §6（新增 Callback 链 / Workspace 透传 / Plugin Pre*+Post* 配对等）

---

## 4. 系统架构总览

| 文档 | 说明 |
|------|------|
| [需求/0 系统框图.md](./需求/0%20系统框图.md) | **系统架构总览**：分层架构图、模块关系图、数据流图、模块依赖矩阵、Web 端功能分布、Team vs Multi-Agent 区别 |

---

## 5. 当前迭代状态（摘要）

> **以下表格仅作快速参考。最终口径以 [`guides/execution-plan.md`](./guides/execution-plan.md) §1.3 + 附录 A "模块状态矩阵" 为准。**

| 模块 | 状态 | 备注 |
|------|------|------|
| M1 Skill 运行时 | ✅ 已对齐 | FS + DB Repo 双适配器 |
| M2 Agent 构建 | ✅ 已对齐 | trpc_build + LRU 缓存 |
| M3 Team 编排 | ✅ 已对齐 | runner / trpc_build / runner_team_trpc 全链路 |
| M4 Graph 工作流 | ✅ 已对齐 | 校验引擎 + 模板 + executions GC + 前后端全链路 |
| M5 Session 管理 | ✅ 已对齐 | Turns 编排 / Restore / Archive / 分页过滤 |
| M6 Memory 记忆 | 🟡 已通 L1–L3 | L4 进化记忆未实装；Memory tools 已注入 |
| M7 Tool 工具体系 | ✅ 已对齐 | 含 toolset / DeclarableTool / a2a / knowledge |
| M8 MCP 集成 | 🟡 部分实现 | trpc 已迁；Broker / 传输层待完善 |
| M9 Model 模型层 | ✅ 基本对齐 | 5 种原生 Provider + 4 种 Variant + Failover/Hedge + Pricing；待：Inspect 扩展字段 / 前端四步表单 / 凭据加密 |
| **M10 Plugin** | 🟡 部分实现 | 已注入 Runner；Agent/Model 全链路 Callback 待 EP-CB-01 |
| M11 Planner 规划 | ✅ 已对齐 | builtin / react / a2ui selector |
| **M12 Artifact** | 🟡 部分实现 | Service 已注册；Runner 制品闭环 / S3 后端待完善（EP-RT-08） |
| **M13 Knowledge** | 🟡 部分实现 | 工具已装配；**EnsureKnowledgeSchema / Embedder / 摄取异步**待 EP-DATA-01、EP-KN-01/02 |
| **M14 CodeExecutor** | 🟡 部分实现 | Docker Sandbox 已写；**skill 路径仍走本地 LocalExec**（EP-BIZ-04） |
| **M15 A2A 协议** | 🟡 部分实现 | call_agent / pb / service 已写；**未挂到 Agent 装配链 + 远端鉴权未做** |
| M16 Gateway 网关 | 🟡 部分实现 | RunStatus RPC 已通；统一 Gateway 抽象未做 |
| **M17 Evaluation** | 🟡 部分实现 | usecase / repo / runner 已写；**wireApp 处 runner 仍传 `nil`** |
| M18 Event 事件 | ✅ 已对齐 | 背压策略 / Backpressure 计数 / WS 统一传输 |
| **M19 Callback 回调** | 🟡 部分实现 | Tool/Plugin 回调已通；**LLMAgent/Model Chain 未挂**（EP-CB-01） |
| M20 Runner 运行器 | ✅ 已对齐 | trpc Runner 装配走 `internal/runtime/deps.go` |

图例：✅ 端到端可用 | 🟡 代码骨架完成但未端到端接入主链路 | ❌ 未启动

---

## 6. 文档组织规则

AI 和开发者编写文档时**必须**遵循以下职责分离原则（与 `execution-plan.md` §11 一致）：

| 文档类型 | 目录 | 内容边界 | 禁止包含 |
|----------|------|----------|----------|
| **进度真相** | [`guides/execution-plan.md`](./guides/execution-plan.md) | 模块接入度 / Top-20 / 里程碑 / 红线扩展 | 已被冻结的 sprint 节奏 / S1–S6 / T1–T41 |
| **编码规范** | `guides/AI-DEVELOPMENT-SPECIFICATION.md` | 分层规则、命名、运行时边界、UI/UX、自检清单 | 进度 / 任务清单 |
| **需求文档** | `需求/*.md` | 用户故事、功能规格、验收标准、运维指南（§6） | 实现细节、代码片段、架构方案 |
| **设计文档** | `需求/*.design.md` | 架构方案、接口设计、数据模型、技术选型 | 修复记录、待办、实现细节 |
| **变更记录** | `changelog/` | 日期、模块、变更摘要、影响范围、破坏性变更 | 代码片段、修复过程、待实现 todo |
| **开发日志** | `devlog/` | 实现细节、修复记录、编译验证、待办、审计快照 | — |
| **运维参考** | `observability/`、`sql/` | Grafana 仪表盘 JSON、初始化 SQL | 业务逻辑 |
| **前端参考** | `frontend/` | UX 总纲、Vue 设计系统 | — |
| **运维指南** | `需求/*.md` §6 | 单模块上手与运维要点（已合入需求文档） | 进度 / 完工承诺 |
| **废弃文档** | `_deprecated/` | 已冻结的历史快照，仅供参考 | 任何新增 / 修改内容 |

**核心原则**：
- **进度真相只在 `execution-plan.md`**，其它文档（含 `master-plan` / `plan` / `implementation-plan` / `task-tracker` / `sprints/*`）已冻结废弃，不得再扩展。
- **需求 / 设计文档保持稳态**：被代码反超时只追加 "现状对齐" 注解，不重写历史结论。
- **changelog 是只读历史**：写完不可改；如有新发现需修正，写新 devlog 补丁，不去改旧 changelog。
- **devlog 是写一次的过程证据**：完工后归档，不持续维护。

---

## 7. 产品需求文档索引

| 功能域 | 文档 |
|--------|------|
| **Chat** | [1 chat.md](./需求/1%20chat.md)、[1 chat.design.md](./需求/1%20chat.design.md) |
| **Agent 创建** | [2 agents-create.md](./需求/2%20agents-create.md)、[2 agents-create.design.md](./需求/2%20agents-create.design.md) |
| **Agent 列表** | [3 agent-list.md](./需求/3%20agent-list.md)、[3 agent-list.design.md](./需求/3%20agent-list.design.md) |
| **Agent 分类** | [4 agent-type.md](./需求/4.agent-type.md)、[4 agent-type.design.md](./需求/4.agent-type.design.md) |
| **Agent 设置** | [5 agent-setting.md](./需求/5%20agent-setting.md)、[5 agent-setting.design.md](./需求/5%20agent-setting.design.md) |
| **Agent 提示文件** | [6 agent-setting-file.md](./需求/6%20agent-setting-file.md)、[6 agent-setting-file.design.md](./需求/6%20agent-setting-file.design.md) |
| **Agent 进化** | [7 agent-evolution.md](./需求/7%20agent-evolution.md)、[7 agent-evolution.design.md](./需求/7%20agent-evolution.design.md) |
| **Agent 标题** | [8 agent-title.md](./需求/8%20agent-title.md)、[8 agent-title.design.md](./需求/8%20agent-title.design.md) |
| **Provider** | [9 provider.md](./需求/9%20provider.md)、[9 provider.design.md](./需求/9%20provider.design.md)、[9-provider-development.md](./需求/9-provider-development.md) |
| **Session** | [10 session.md](./需求/10%20session.md)、[10 session.design.md](./需求/10%20session.design.md) |
| **Multi-Agent / Team** | [11 multi-agent.md](./需求/11%20multi-agent.md)、[11 multi-agent.design.md](./需求/11%20multi-agent.design.md) |
| **Memory L0~L4** | [12 L0-sensory](./需求/12%20memory-L0-sensory.md)、[13 L1-working](./需求/13%20memory-L1-working.md)、[14 L2-episodic](./需求/14%20memory-L2-episodic.md)、[15 L3-semantic](./需求/15%20memory-L3-semantic.md)、[16 L4-persistent](./需求/16%20memory-L4-persistent.md)、[12-16 memory.design](./需求/12-16%20memory.design.md)（含原 supplement）、[38 memory.md（框架对齐）](./需求/38%20memory.md)、[31 memery.md（拼写遗留→记忆管理 UX）](./需求/31%20memery.md) |
| **Channel** | [17 channel.md](./需求/17%20channel.md)、[17 channel.design.md](./需求/17%20channel.design.md)、[17-channel-development.md](./需求/17-channel-development.md) |
| **Monitor** | [18 monitor.md](./需求/18%20monitor.md)、[18 monitor.design.md](./需求/18%20monitor.design.md) |
| **MCP** | [19 mcp.md](./需求/19%20mcp.md)、[19 mcp.design.md](./需求/19%20mcp.design.md) |
| **Skill** | [20 skill.md](./需求/20%20skill.md)、[20 skill.design.md](./需求/20%20skill.design.md)、[20 skill struct design.md](./需求/20%20skill%20struct%20design.md) |
| **Cron** | [21 cron.md](./需求/21%20cron.md)、[21 cron.design.md](./需求/21%20cron.design.md) |
| **Plugin** | [22 plugin.md](./需求/22%20plugin.md)、[22 plugin.design.md](./需求/22%20plugin.design.md) |
| **Tools** | [23 tools.md](./需求/23%20tools.md)、[23 tools.design.md](./需求/23%20tools.design.md)、[23 tools struct design.md](./需求/23%20tools%20struct%20design.md) |
| **Telemetry** | [24 telemetry.md](./需求/24%20telemetry.md)（占位）、[24 telemetry.design.md](./需求/24%20telemetry.design.md) |
| **CLI** | [25 cli.md](./需求/25%20cli.md)、[25 cli.design.md](./需求/25%20cli.design.md) |
| **A2A 协议** | [26 a2a-protocol.md](./需求/26%20a2a-protocol.md)、[26 a2a-protocol.design.md](./需求/26%20a2a-protocol.design.md) |
| **Artifact 制品** | [27 artifact.md](./需求/27%20artifact.md)、[27 artifact.design.md](./需求/27%20artifact.design.md) |
| **Callback 回调** | [28 callback.md](./需求/28%20callback.md)、[28 callback.design.md](./需求/28%20callback.design.md) |
| **Token** | [29 token.md](./需求/29%20token.md)、[29 token.design.md](./需求/29%20token.design.md) |
| **Ecosystem** | [30 ecosystem.md](./需求/30%20ecosystem.md)、[30 ecosystem.design.md](./需求/30%20ecosystem.design.md) |
| **CodeExecutor** | [32 codeexecutor.md](./需求/32%20codeexecutor.md)、[32 codeexecutor.design.md](./需求/32%20codeexecutor.design.md) |
| **Evaluation 评估** | [33 evaluation.md](./需求/33%20evaluation.md)、[33 evaluation.design.md](./需求/33%20evaluation.design.md) |
| **Event 事件** | [34 event-system.md](./需求/34%20event-system.md)、[34 event-system.design.md](./需求/34%20event-system.design.md) |
| **Gateway 网关** | [35 gateway.md](./需求/35%20gateway.md)、[35 gateway.design.md](./需求/35%20gateway.design.md) |
| **Graph 工作流** | [36 graph-workflow.md](./需求/36%20graph-workflow.md)、[36 graph-workflow.design.md](./需求/36%20graph-workflow.design.md) |
| **Knowledge 知识库** | [37 knowledge.md](./需求/37%20knowledge.md)、[37 knowledge.design.md](./需求/37%20knowledge.design.md) |
| **Planner 规划** | [39 planner.md](./需求/39%20planner.md)、[39 planner.design.md](./需求/39%20planner.design.md) |
| **Runner 运行器** | [40 runner.md](./需求/40%20runner.md)、[40 runner.design.md](./需求/40%20runner.design.md) |
| **Avatar** | [50 Avatar.md](./需求/50%20Avatar.md)、[50 Avatar.design.md](./需求/50%20Avatar.design.md) |
| **消息机制** | [51 消息机制.md](./需求/51%20消息机制.md)、[51a 后端消息机制.md](./需求/51a%20后端消息机制.md)、[51b 前端消息机制.md](./需求/51b%20前端消息机制.md) |
| **TTS** | [tts.md](./需求/tts.md)（占位） |

> 旧文件 `31 memery.md`（拼写错误，应为 *memory*）仅作历史保留；新内容请写入 `38 memory.md` 或合入 `12-16 memory.design.md`。

### 7.1 模块开发计划（2026-05-17）

各功能域迭代路线见 [`需求/README-development.md`](./需求/README-development.md)（索引）及 `需求/*-development.md`（39+ 篇）。横切数据启动项见 [`需求/data-platform-development.md`](./需求/data-platform-development.md)。**进度仍以 `guides/execution-plan.md` 附录 A 为准**；开发计划描述「做什么」，不替代 EP 状态。

---

## 8. 核心规范与指南（guides/）

| 文档 | 说明 |
|------|------|
| [guides/AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) | ★ AI 编码唯一行为准则（十章整合版） |
| [guides/execution-plan.md](./guides/execution-plan.md) | ★ 当前迭代真相 + Top-20 + M0–M5 + 扩展红线 |
| [guides/trpc-agent-go-framework.md](./guides/trpc-agent-go-framework.md) | trpc-agent-go 框架工程化解读 |
| **已废弃**：[_deprecated/guides/](./_deprecated/guides/) | 含 master-plan / plan / implementation-plan / task-tracker / sprints S1–S6，历史快照仅供参考 |

> 单模块运维指南已合入对应需求文档（如 `需求/26 a2a-protocol.md` §6、`需求/27 artifact.md` §6 等），不再单独维护 `guides/` 下的运维文件。

---

## 9. 参考资料

| 路径 | 说明 |
|------|------|
| [observability/grafana-aranea.json](./observability/grafana-aranea.json) | Grafana 仪表盘 JSON（导入 Grafana 即用） |
| [sql/](./sql/) | 数据库初始化 SQL 脚本（按模块拆分） |
| [frontend/UX.md](./frontend/UX.md) | 前端 UX 总纲（玻璃态 / 主题 / 间距 / 动效） |
| [frontend/vue-design/](./frontend/vue-design/) | Vue 设计系统与 agent rules |
| [skills/](./skills/) | Agent Skill 定义（karpathy-guidelines 等） |

---

## 10. 目录结构总览

```
docs/
├── README.md                          ← 你正在阅读的入口文档
├── guides/                            ← 编码规范 + 当前执行基线
│   ├── AI-DEVELOPMENT-SPECIFICATION.md     ← ★ AI 编码唯一行为准则
│   ├── execution-plan.md                   ← ★ 当前迭代真相（2026-05-17 起）
│   └── trpc-agent-go-framework.md          ← trpc 框架工程化解读
├── 需求/                              ← 产品需求 + 设计文档 + 运维指南（§6）
│   ├── 0 系统框图.md                       ← ★ 系统架构总览
│   ├── README-development.md               ← 模块开发计划索引
│   ├── *-development.md                    ← 各模块开发计划
│   ├── *.md                                ← 纯需求内容
│   └── *.design.md                         ← 纯设计内容
├── changelog/                         ← 变更摘要（只读历史，索引见 README.md）
├── devlog/                            ← 开发日志（实现细节 / 审计快照，索引见 README.md）
├── observability/                     ← Grafana 仪表盘等运维资产
├── sql/                               ← 数据库初始化 SQL
├── frontend/                          ← 前端 UX 与 Vue 设计系统
├── skills/                            ← Agent Skill 定义
└── _deprecated/                       ← ⚠ 已废弃文档（历史快照，不可扩展）
    ├── guides/                             ← 废弃规划文档
    │   ├── master-plan.md
    │   ├── plan.md
    │   ├── implementation-plan.md
    │   ├── task-tracker.md
    │   └── sprints/                        ← S1–S6 Sprint 详细计划
    └── 需求/                               ← 废弃/非正式需求文档
        ├── 23 tools todo.md                     ← 验证报告（非需求）
        └── 随心记.md                             ← 个人头脑风暴笔记
```

---

## 11. AI 编码工作流

```
1. 阅读本入口 README → 了解项目全貌 + 文档治理规则
2. 阅读 guides/AI-DEVELOPMENT-SPECIFICATION.md → 掌握唯一编码准则
3. 阅读 guides/execution-plan.md → 确认当前里程碑 / 模块接入度 / 立即可执行任务 / 扩展红线
4. （涉及 Agent / Runner / Model / Session / Memory / Tool 时）阅读 guides/trpc-agent-go-framework.md
5. 阅读对应需求文档与 *.design.md → 理解功能规格 + 运维要点（§6）
6. 按规范编码 → 遵循分层、依赖方向、红线（R1–R18）、命名约定
7. 后端验证：make wire && make api && make build && make test && make runtime-boundary
8. 前端验证：cd web && pnpm i && pnpm lint && pnpm test && pnpm build
9. 更新文档：
   - 必更：guides/execution-plan.md §1 / §3 / 附录 A（关闭对应 EP-* 编号）
   - 必更：changelog/<date>-<topic>.md（变更摘要，不写过程）
   - 可选：devlog/<date>-<topic>.md（实现过程、审计、修复记录）
```

---

## 12. 文档治理变更说明（2026-05-17）

为消除"changelog 宣称已完成 vs 代码未接入主链路"的长期不一致，本次治理执行了以下硬性约束：

1. **进度真相单点化**：`guides/execution-plan.md` 是**唯一**反映"代码现实 + 下一步规划"的文档。
2. **废弃旧规划文档**：`master-plan.md` / `plan.md` / `implementation-plan.md` / `task-tracker.md` / `sprints/S1–S6` 全部冻结，顶部已加废弃声明，不再维护。
3. **需求文档防误读**：被代码反超的需求文档（如 `22 plugin.md` / `27 artifact.md` / `37 knowledge.md` / `34 event-system.md` / `16 memory-L4-persistent.md` / `51*.md`）在"现状分析"段追加 *2026-05-17 现状对齐* 注解，避免 AI 把过时现状当现实。
4. **废弃文档归档**：已冻结的规划文档（`master-plan.md` / `plan.md` / `implementation-plan.md` / `task-tracker.md` / `sprints/S1–S6`）及非正式文档（`随心记.md` / `23 tools todo.md`）统一迁移至 `_deprecated/` 目录，与活跃文档物理隔离。
5. **空文件标注占位**：`24 telemetry.md` / `tts.md` 等空文件加占位说明，避免误以为已废弃。
6. **变更与计划同更**：写 `changelog/` 时**必须**同步更新 `execution-plan.md` 对应 EP-* 行，否则视为未完成。
7. **运维指南合入需求文档**：`guides/` 下的单模块运维文件（a2a-protocol / artifact / codeexecutor / cron / evaluation / knowledge）内容已合入对应 `需求/*.md` 的 §6（或 §7），不再单独维护。
8. **changelog / devlog 索引独立**：变更记录和开发日志的索引表从 README 移至各自目录的 `README.md`，README 仅保留核心规范与指南链接。

