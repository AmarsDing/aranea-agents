# Aranea-Agents 项目文档

> **AI 入口文档**：AI 在对本项目进行代码修改或新功能实现前，**必须先阅读本文**。本文提供项目全貌、编码规则索引和文档导航。

---

## 1. 项目定位

**Aranea-Agents** 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核，提供可视化 Agent/Team/Graph 编排、会话管理、可观测记忆与多模型接入能力。

---

## 2. 技术栈

| 层级 | 选型 |
|------|------|
| 后端 | Go + **Kratos v2**（HTTP/gRPC/WebSocket 传输、Wire DI） |
| Agent 运行时 | **trpc-agent-go**（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team） |
| 前端 | Vue 3 + Quasar + Pinia + TypeScript |
| 数据库 | SQLite（Ent ORM）；向量/图可外挂 pgvector |
| 依赖注入 | Wire（编译期）；proto 代码生成 `make api` |

---

## 3. 双框架分工

Kratos v2 负责传输层，trpc-agent-go 负责 Agent 编排，二者互不越界。详细规则见 [AI-DEVELOPMENT-SPECIFICATION.md §1.1-1.3](./guides/AI-DEVELOPMENT-SPECIFICATION.md#11-双框架分工)。

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + 框架 Runner 装配
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite）
```

---

## 4. 编码前必读规范（按优先级）

AI 进行任何代码改动时，**必须**按以下顺序阅读规范文档：

| 优先级 | 文档 | 说明 |
|--------|------|------|
| **1（最高）** | [AI-DEVELOPMENT-SPECIFICATION.md](./guides/AI-DEVELOPMENT-SPECIFICATION.md) | **AI 编码唯一行为准则**：十章全覆盖——架构总纲、分层编码、Agent 运行时、Proto/API、Go 风格、模块化、前端编码、UI/UX 执行规范、自检清单、平台目标架构原则 |
| **2** | [trpc-agent-go-framework.md](./guides/trpc-agent-go-framework.md) | **trpc-agent-go 框架工程化解读**：目录导读、核心接口速查、模块实现规则、项目内框架映射、常见场景速查 |
| **3** | [plan.md](./guides/plan.md) | trpc-agent-go 功能对齐清单：18 模块现状、目标、步骤、验收标准 |

**规范冲突优先级**：AI-DEVELOPMENT-SPECIFICATION > plan

---

## 5. 系统架构总览

| 文档 | 说明 |
|------|------|
| [0 系统框图.md](./需求/0%20系统框图.md) | **系统架构总览**：分层架构图、模块关系图、数据流图、模块依赖矩阵、Web 端功能分布、Team vs Multi-Agent 区别 |

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
| M4: Graph 工作流 | ✅ 已对齐（v2四维架构：校验引擎+模板+前后端全链路已实现） | P1 |
| M5: Session 管理 | ✅ 已对齐（Turns 编排追踪/Detail 页/Restore/Archive/分页过滤已实现） | P1 |
| M6: Memory 记忆 | ❌ 严重不足 | P2 |
| M7: Tool 工具体系 | ✅ 已对齐 | P1 |
| M8: MCP 集成 | ⚠️ 部分实现（已迁移到 trpc，Broker/传输层待完善） | P2 |
| M9: Model 模型层 | ✅ 基本对齐 | P2 |
| M10: Plugin 插件 | ❌ 未实现 | P2 |
| M11: Planner 规划 | ⚠️ 部分实现 | P2 |
| M12: Artifact 制品 | ❌ 未实现 | P2 |
| M13: Knowledge 知识库 | ❌ 未实现 | P3 |
| M14: CodeExecutor | ⚠️ 部分实现 | P2 |
| M15: A2A 协议 | ❌ 未实现 | P3 |
| M16: Gateway 网关 | ⚠️ 部分实现 | P2 |
| M17: Evaluation 评估 | ❌ 未实现 | P3 |
| M18: Event 事件 | ✅ 已对齐 | P2 |
| M19: Callback 回调 | ⚠️ 部分实现 | P2 |
| M20: Runner 运行器 | ✅ 已对齐 | P1 |

---

## 7. 产品需求文档

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
| **Provider** | [9 provider.md](./需求/9%20provider.md)、[9 provider.design.md](./需求/9%20provider.design.md) |
| **Session** | [10 session.md](./需求/10%20session.md)、[10 session.design.md](./需求/10%20session.design.md) |
| **Multi-Agent/Team** | [11 multi-agent.md](./需求/11%20multi-agent.md)、[11 multi-agent.design.md](./需求/11%20multi-agent.design.md) |
| **Memory L0~L4** | [12 L0-sensory](./需求/12%20memory-L0-sensory.md)、[13 L1-working](./需求/13%20memory-L1-working.md)、[14 L2-episodic](./需求/14%20memory-L2-episodic.md)、[15 L3-semantic](./需求/15%20memory-L3-semantic.md)、[16 L4-persistent](./需求/16%20memory-L4-persistent.md)、[12-16 memory.design](./需求/12-16%20memory.design.md)、[31+38 supplement.design](./需求/31+38%20memory-supplement.design.md) |
| **Channel** | [17 channel.md](./需求/17%20channel.md)、[17 channel.design.md](./需求/17%20channel.design.md)、[channel-requirements-analysis.md](./需求/channel-requirements-analysis.md) |
| **Monitor** | [18 monitor.md](./需求/18%20monitor.md)、[18 monitor.design.md](./需求/18%20monitor.design.md) |
| **MCP** | [19 mcp.md](./需求/19%20mcp.md)、[19 mcp.design.md](./需求/19%20mcp.design.md) |
| **Skill** | [20 skill.md](./需求/20%20skill.md)、[20 skill.design.md](./需求/20%20skill.design.md)、[20 skill struct design.md](./需求/20%20skill%20struct%20design.md) |
| **Cron** | [21 cron.md](./需求/21%20cron.md)、[21 cron.design.md](./需求/21%20cron.design.md) |
| **Plugin** | [22 plugin.md](./需求/22%20plugin.md)、[22 plugin.design.md](./需求/22%20plugin.design.md) |
| **Tools** | [23 tools.md](./需求/23%20tools.md)、[23 tools.design.md](./需求/23%20tools.design.md)、[23 tools struct design.md](./需求/23%20tools%20struct%20design.md) |
| **Telemetry** | [24 telemetry.md](./需求/24%20telemetry.md)、[24 telemetry.design.md](./需求/24%20telemetry.design.md) |
| **CLI** | [25 cli.md](./需求/25%20cli.md)、[25 cli.design.md](./需求/25%20cli.design.md) |
| **A2A 协议** | [26 a2a-protocol.md](./需求/26%20a2a-protocol.md)、[26 a2a-protocol.design.md](./需求/26%20a2a-protocol.design.md) |
| **Artifact 制品** | [27 artifact.md](./需求/27%20artifact.md)、[27 artifact.design.md](./需求/27%20artifact.design.md) |
| **Callback 回调** | [28 callback.md](./需求/28%20callback.md)、[28 callback.design.md](./需求/28%20callback.design.md) |
| **Token** | [29 token.md](./需求/29%20token.md)、[29 token.design.md](./需求/29%20token.design.md) |
| **Ecosystem** | [30 ecosystem.md](./需求/30%20ecosystem.md)、[30 ecosystem.design.md](./需求/30%20ecosystem.design.md) |
| **Memory 补充** | [31 memery.md](./需求/31%20memery.md) |
| **CodeExecutor** | [32 codeexecutor.md](./需求/32%20codeexecutor.md)、[32 codeexecutor.design.md](./需求/32%20codeexecutor.design.md) |
| **Evaluation 评估** | [33 evaluation.md](./需求/33%20evaluation.md)、[33 evaluation.design.md](./需求/33%20evaluation.design.md) |
| **Event 事件** | [34 event-system.md](./需求/34%20event-system.md)、[34 event-system.design.md](./需求/34%20event-system.design.md) |
| **Gateway 网关** | [35 gateway.md](./需求/35%20gateway.md)、[35 gateway.design.md](./需求/35%20gateway.design.md) |
| **Graph 工作流** | [36 graph-workflow.md](./需求/36%20graph-workflow.md)、[36 graph-workflow.design.md](./需求/36%20graph-workflow.design.md) |
| **Knowledge 知识库** | [37 knowledge.md](./需求/37%20knowledge.md)、[37 knowledge.design.md](./需求/37%20knowledge.design.md) |
| **Memory（框架）** | [38 memory.md](./需求/38%20memory.md) |
| **Planner 规划** | [39 planner.md](./需求/39%20planner.md)、[39 planner.design.md](./需求/39%20planner.design.md) |
| **Runner 运行器** | [40 runner.md](./需求/40%20runner.md)、[40 runner.design.md](./需求/40%20runner.design.md) |
| **Avatar** | [50 Avatar.md](./需求/50%20Avatar.md)、[50 Avatar.design.md](./需求/50%20Avatar.design.md) |
| **消息机制** | [51 消息机制.md](./需求/51%20消息机制.md)、[51a 后端消息机制.md](./需求/51a%20后端消息机制.md)、[51b 前端消息机制.md](./需求/51b%20前端消息机制.md) |
| **TTS** | [tts.md](./需求/tts.md) |

---

## 9. 文档组织规则

AI 和开发者编写文档时，**必须**遵循以下职责分离原则：

| 文档类型 | 目录 | 内容边界 | 禁止包含 |
|----------|------|----------|----------|
| **需求文档** | `需求/*.md` | 纯需求内容：用户故事、功能规格、验收标准 | 技术实现、代码片段、架构设计 |
| **设计文档** | `需求/*.design.md` | 纯设计内容：架构方案、接口设计、数据模型、技术选型 | 实现细节、修复记录、待办事项 |
| **变更记录** | `changelog/` | 纯变更记录：日期、模块、变更摘要、影响范围、破坏性变更、关联文件列表 | 代码片段、修复过程、编译验证、待实现 todo |
| **开发日志** | `devlog/` | 开发过程记录：实现细节、代码片段、修复记录、编译验证、待实现 todo | — |

**核心原则**：
- **需求文档 = 纯需求**，不混入技术实现
- **设计文档 = 纯设计**，不混入实现细节和待办事项
- **changelog = 纯变更记录**，不混入实现过程和 todo
- **devlog = 开发过程**，实现细节、修复记录、待办事项统一放此处

changelog 末尾通过链接指向对应的 devlog 详细记录。

---

## 10. 变更记录

| 文档 | 说明 |
|------|------|
| [2026-05-12-Provider.md](./changelog/2026-05-12-Provider.md) | ADK → trpc-agent-go 迁移 + 多 Provider 支持 |
| [2026-05-13-Session.md](./changelog/2026-05-13-Session.md) | Session 核心数据结构重构 |
| [2026-05-16-Graph.md](./changelog/2026-05-16-Graph.md) | Graph 工作流完善（校验引擎+模板+全链路） |
| [2026-05-16-Session-Optimize.md](./changelog/2026-05-16-Session-Optimize.md) | Session 模块优化（通用更新/恢复/分页/排序/过滤） |
| [2026-05-17-Session-Turns.md](./changelog/2026-05-17-Session-Turns.md) | Session Turns 编排追踪 + Detail 页 + Restore/Archive |
| [2026-05-17-S1-Hardening.md](./changelog/2026-05-17-S1-Hardening.md) | S1 P0 红线加固：单连接池/WS接入/biz解耦/内存缓存修复/并发安全/EventBus可靠投递/前端graph客户端生成 |
| [2026-05-17-S2-Architecture.md](./changelog/2026-05-17-S2-Architecture.md) | S2 架构债清理：runtime包重构/EventBus背压策略/Agent缓存LRU/17个Pinia store/axios拦截器/统一WS客户端 |
| [2026-05-17-S3-Observability.md](./changelog/2026-05-17-S3-Observability.md) | S3 业务可观测：RunStatus RPC/Callback Chain/apierror/Workspace中间件/Prometheus metrics/lint工具/CI/测试基线 |
| [2026-05-17-S4-Plugin-Skill-Planner.md](./changelog/2026-05-17-S4-Plugin-Skill-Planner.md) | S4 功能补全：Plugin运行时(AuditLogPlugin/热重载)/Skill DB仓库/Planner多策略(builtin/react/a2ui)/Memory工具注入/AutoMemory后台任务 |

---

## 11. 参考资料与 Skill

| 路径 | 说明 |
|------|------|
| [skills/](./skills/) | Agent Skill 定义（karpathy-guidelines 等） |

---

## 12. 目录结构总览

```
docs/
├── README.md                  ← 你正在阅读的入口文档
├── guides/                    ← 编码规范（AI 必读）
│   ├── AI-DEVELOPMENT-SPECIFICATION.md   ← ★ AI 编码唯一行为准则（十章整合版）
│   ├── trpc-agent-go-framework.md        ← trpc-agent-go 框架工程化解读
│   └── plan.md                          ← trpc 功能对齐清单
├── 需求/                      ← 产品需求规格 + 设计文档
│   ├── 0 系统框图.md                      ← ★ 系统架构总览（框图/数据流/依赖矩阵）
│   ├── *.md                               ← 纯需求内容（用户故事、功能规格、验收标准）
│   └── *.design.md                        ← 纯设计内容（架构方案、接口设计、数据模型）
├── changelog/                 ← 变更记录（纯变更摘要，不含实现细节）
├── devlog/                    ← 开发日志（实现细节、修复记录、编译验证、待实现 todo）
├── frontend/                  ← 前端设计参考
│   ├── UX.md
│   └── vue-design/
└── skills/                    ← Agent Skill 定义
```

---

## 13. AI 编码工作流

当 AI 接到新功能开发或代码修改任务时，按以下流程操作：

```
1. 阅读本入口文档 → 了解项目全貌
2. 阅读 guides/AI-DEVELOPMENT-SPECIFICATION.md → 掌握唯一编码准则（含运行时边界、UI/UX、平台架构原则）
3. 阅读 guides/trpc-agent-go-framework.md → 掌握框架核心接口与项目映射（涉及 Agent/Runner/Model/Session/Memory/Tool 时必读）
4. 查找对应需求文档 → 理解功能规格（含架构融合的运行时实现与演进方向）
5. 阅读 guides/plan.md → 确认功能对齐状态
6. 按规范编码 → 遵循分层、依赖方向、命名约定
7. 后端编译验证 → make all 验证后端编译与运行
8. 前端编译验证 → npm install && quasar dev 验证前端编译与运行
9. 更新文档 → 遵循文档组织规则（§9）：changelog 写变更摘要，devlog 写实现细节和 todo
```
