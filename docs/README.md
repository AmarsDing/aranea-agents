# Aranea-Agents 项目文档

> **AI 入口文档**：AI 在对本项目进行代码修改或新功能实现前，**必须先阅读本文**。本文提供项目全貌、编码规则索引和文档导航。

---

## 1. 项目定位

**Aranea-Agents** 是一个多智能体任务编排平台。用户通过可视化界面组装 LLM、工具、子 Agent，形成可运行工作流；支持会话内多轮协作、可观测、可配置记忆与多模型提供商。

---

## 2. 技术栈

| 层级 | 选型 |
|------|------|
| 后端框架 | Go + **Kratos v2**（HTTP/gRPC/SSE 传输、配置、鉴权、Wire 依赖注入） |
| Agent 运行时 | **trpc-agent-go**（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team） |
| 前端 | Vue 3 + Quasar + Pinia + TypeScript |
| 数据库 | SQLite（向量/图可按阶段外挂 pgvector） |

---

## 3. 双框架分工（铁律）

| 框架 | 职责 | 禁止 |
|------|------|------|
| **Kratos v2** | 传输层（HTTP/gRPC/SSE）、配置、鉴权、中间件、Wire 依赖注入 | 不承载 Agent 编排、不实现第二套事件循环 |
| **trpc-agent-go** | Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team） | 不直接写业务数据库、不处理 HTTP 路由 |

### 依赖方向

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + 框架 Runner 装配
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite）
```

**跨层只允许向内依赖。违反即停。**

---

## 4. 编码前必读规范（按优先级）

AI 进行任何代码改动时，**必须**按以下顺序阅读规范文档：

| 优先级 | 文档 | 说明 |
|--------|------|------|
| **1（最高）** | [AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) | **AI 编码唯一行为准则**：十章全覆盖——架构总纲、分层编码、Agent 运行时、Proto/API、Go 风格、模块化、前端编码、UI/UX 执行规范、自检清单、平台目标架构原则 |
| **2** | [plan.md](./guides/plan.md) | trpc-agent-go 功能对齐清单：18 模块现状、目标、步骤、验收标准 |

**规范冲突优先级**：AI-DEVELOPMENT-SPECIFICATION > plan

> **整合说明**：`AI-DEVELOPMENT-SPECIFICATION.md` 已整合以下文档的全部内容：
> - 原 `runtime-boundary.md` → 第一章架构总纲
> - 原 `AI-全栈新功能开发规范.md` → 第四章迁移硬约束/横切边界/用量双写 + 第七章前端迭代 + 第八章 UI/UX
> - 原 `接口与数据库开发规范.md` → 第四章 Proto/API 规范增强
> - 原 `platform-architecture.md` 第三篇 → 第十章平台目标架构原则

---

## 5. 架构设计融合说明

> 原 `architecture/` 目录下的所有文档已分别融合至对应的需求设计文档中。以下为融合映射：

| 原架构文档 | 融合目标 | 融合内容 |
|------------|----------|----------|
| `trpc-agent-go-implementation-plan.md` | [11 multi-agent.md](./需求/11%20multi-agent.md) | trpc-agent-go 标准化实施计划 |
| `agent-team-design.md` | [11 multi-agent.md](./需求/11%20multi-agent.md) | Agent & Team 运行时设计 |
| `agent-orchestration-roadmap.md` | [11 multi-agent.md](./需求/11%20multi-agent.md) | Agent 编排系统发展方向 |
| `agent-orchestration-total-design.md` | [11 multi-agent.md](./需求/11%20multi-agent.md) | Agent 编排总体设计 |
| `session-context-compression.md` | [10 session.md](./需求/10%20session.md) | 会话上下文压缩设计 |
| `agent-skills-tools-mcp-memory.md` | [20 skill.md](./需求/20%20skill.md)、[23 tools.md](./需求/23%20tools.md)、[19 mcp.md](./需求/19%20mcp.md)、[12 memory-L0-sensory.md](./需求/12%20memory-L0-sensory.md) | Skill/Tools/MCP/Memory 运行时装配 |
| `agent-repo-retrieval-context-engineering.md` | [23 tools.md](./需求/23%20tools.md) | 代码库检索与上下文工程实施工单 |
| `platform-architecture.md` 第一/二篇 | [9 provider.md](./需求/9%20provider.md) | LLM Gateway 三层架构与演进 |
| `platform-architecture.md` 第三篇 | [AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) | 平台目标架构原则 |
| `runtime-boundary.md` | [AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) | 运行时边界 |

---

## 6. 功能对齐与优化清单

| 文档 | 说明 |
|------|------|
| [plan.md](./guides/plan.md) | **trpc-agent-go 功能对齐清单**：18 个模块的现状、目标、步骤、验收标准。AI 按此清单自主执行优化 |

### 当前对齐状态

| 模块 | 状态 | 优先级 |
|------|------|--------|
| M1: Skill 运行时 | ✅ 已对齐 | P0 |
| M2: Agent 构建 | ✅ 已对齐 | P0 |
| M3: Team 编排 | ✅ 已对齐 | P1 |
| M4: Graph 工作流 | ✅ 已对齐 | P1 |
| M5: Session 管理 | ✅ 已对齐 | P1 |
| M6: Memory 记忆 | ✅ 已对齐 | P2 |
| M7: Tool 工具体系 | ✅ 已对齐 | P1 |
| M8: MCP 集成 | ⚠️ 部分实现 | P2 |
| M9: Model 模型层 | ✅ 已对齐 | P2 |
| M10: Plugin 插件 | ⚠️ 部分实现 | P2 |
| M11: Planner 规划 | ✅ 已对齐 | P2 |
| M12: Artifact 制品 | ⚠️ 部分实现 | P2 |
| M13: Knowledge 知识库 | ❌ 未实现 | P3 |
| M14: CodeExecutor | ⚠️ 部分实现 | P2 |
| M15: A2A 协议 | ❌ 未实现 | P3 |
| M16: Gateway 网关 | ⚠️ 部分实现 | P3 |
| M17: Evaluation 评估 | ❌ 未实现 | P3 |
| M18: Event 事件 | ⚠️ 部分实现 | P2 |

---

## 7. 产品需求文档

需求文档入口：[产品需求总览.md](./需求/产品需求总览.md)

### 按功能域索引

| 功能域 | 文档 |
|--------|------|
| **Chat** | [1 chat.md](./需求/1%20chat.md) |
| **Agent 创建** | [2 agents-create.md](./需求/2%20agents-create.md) |
| **Agent 列表** | [3 agent-list.md](./需求/3%20agent-list.md) |
| **Agent 分类** | [4.agent-type.md](./需求/4.agent-type.md) |
| **Agent 设置** | [5 agent-setting.md](./需求/5%20agent-setting.md) |
| **Agent 提示文件** | [6 agent-setting-file.md](./需求/6%20agent-setting-file.md) |
| **Agent 进化** | [7 agent-evolution.md](./需求/7%20agent-evolution.md) |
| **Agent 标题** | [8 agent-title.md](./需求/8%20agent-title.md) |
| **Provider** | [9 provider.md](./需求/9%20provider.md)（含 LLM Gateway 演进） |
| **Session** | [10 session.md](./需求/10%20session.md)（含上下文压缩） |
| **Multi-Agent/Team** | [11 multi-agent.md](./需求/11%20multi-agent.md)（含编排/Team/trpc 实施计划）、[team.md](./需求/team.md) |
| **Memory** | [memory.md](./需求/memory.md)、[12-16 memory-L0~L4](./需求/12%20memory-L0-sensory.md)、[31 memery.md](./需求/31%20memery.md) |
| **Channel** | [17 channel.md](./需求/17%20channel.md)、[channel-requirements-analysis.md](./需求/channel-requirements-analysis.md) |
| **Monitor** | [18 monitor.md](./需求/18%20monitor.md) |
| **MCP** | [19 mcp.md](./需求/19%20mcp.md)（含运行时装配） |
| **Skill** | [20 skill.md](./需求/20%20skill.md)（含运行时装配）、[20 skill struct design.md](./需求/20%20skill%20struct%20design.md) |
| **Cron** | [21 cron.md](./需求/21%20cron.md) |
| **Plugin** | [22 plugin.md](./需求/22%20plugin.md) |
| **Tools** | [23 tools.md](./需求/23%20tools.md)（含代码库检索实施工单）、[23 tools struct design.md](./需求/23%20tools%20struct%20design.md) |
| **Telemetry** | [24 telemetry.md](./需求/24%20telemetry.md) |
| **CLI** | [25 cli.md](./需求/25%20cli.md) |
| **Token** | [29 token.md](./需求/29%20token.md) |
| **Ecosystem** | [30 ecosystem.md](./需求/30%20ecosystem.md) |
| **Avatar** | [50 Avatar.md](./需求/50%20Avatar.md) |
| **Hook** | [hook.md](./需求/hook.md) |
| **TTS** | [tts.md](./需求/tts.md) |

---

## 8. 前端设计文档（历史参考）

> 前端规范已整合至 [AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) 第七章至第八章。以下文档保留供历史参考。

| 文档 | 说明 |
|------|------|
| [vue-design.md](./frontend/vue-design/vue-design.md) | 前端编码规范原始版（Vue 3 / Quasar / Pinia 分层与自检） |
| [vue-design-agent-rules.md](./frontend/vue-design/vue-design-agent-rules.md) | AI 系统提示精简版（MUST/MUST NOT） |
| [UX.md](./frontend/UX.md) | UI 执行规范原始版（奶油昼·玻璃夜，token 与数值唯一权威） |

---

## 9. 变更记录

| 文档 | 说明 |
|------|------|
| [2026-05-12-Provider.md](./changelog/2026-05-12-Provider.md) | ADK → trpc-agent-go 迁移 + 多 Provider 支持 |
| [2026-05-13-Session.md](./changelog/2026-05-13-Session.md) | Session 核心数据结构重构 |

---

## 10. 参考资料与 Skill

| 路径 | 说明 |
|------|------|
| [reference/](./reference/) | 外部资料整理（知乎 AI Agent 开发攻略等） |
| [skills/](./skills/) | Agent Skill 定义（karpathy-guidelines 等） |

---

## 11. 目录结构总览

```
docs/
├── README.md                  ← 你正在阅读的入口文档
├── guides/                    ← 编码规范（AI 必读）
│   ├── AI-DEVELOPMENT-SPECIFICATION.md   ← ★ AI 编码唯一行为准则（十章整合版）
│   └── plan.md                          ← trpc 功能对齐清单
├── 需求/                      ← 产品需求规格（含架构融合内容）
│   ├── 产品需求总览.md                    ← 需求入口
│   ├── 1 chat.md ~ 50 Avatar.md
│   └── ...
├── frontend/                  ← 前端设计（历史参考，已整合至 guides/）
│   ├── UX.md
│   └── vue-design/
├── changelog/                 ← 变更记录
├── reference/                 ← 外部参考资料
└── skills/                    ← Agent Skill 定义
```

---

## 12. AI 编码工作流

当 AI 接到新功能开发或代码修改任务时，按以下流程操作：

```
1. 阅读本入口文档 → 了解项目全貌
2. 阅读 guides/AI-DEVELOPMENT-SPECIFICATION.md → 掌握唯一编码准则（含运行时边界、UI/UX、平台架构原则）
3. 查找对应需求文档 → 理解功能规格（含架构融合的运行时实现与演进方向）
4. 阅读 guides/plan.md → 确认功能对齐状态
5. 按规范编码 → 遵循分层、依赖方向、命名约定
6. 编译验证 → go build + go vet
```
