# Memory L0–L4 文档索引

> **模块编号**：12–16 · **最后整合**：2026-05-28
> **进度真相**：以 [`memory-development.md`](./memory-development.md) 与各层 `*-development.md` 为准；需求/设计正文不写修复记录。

---

## 1. 文档分类与边界

| 类型 | 文件名模式 | 写什么 | 不写什么 |
|------|------------|--------|----------|
| **需求** | `memory.md`、`L*.md` | 用户故事、功能/非功能需求、UX、验收标准、权限 | Proto 字段、Go 接口、SQL DDL、代码路径、任务状态 |
| **设计** | `memory.design.md`、`L*.design.md` | 架构、数据模型、API/Proto 摘要、存储、层间数据流、对接点 | 迭代排期、✅/❌ 进度、Phase 任务表 |
| **开发计划** | `memory-development.md`、`L*-development.md` | 代码锚点、现状表、差距、Phase、任务清单 | 重复粘贴需求全文、UX 交互细节 |
| **理论参考** | [`theory.md`](./theory.md) | Ledger/Views/Policy、System 1/2、文献观点 | **非验收标准**；规格以各层需求为准 |

**阅读顺序（AI 编码）**：

1. 本文 → [`memory.design.md`](./memory.design.md) §一、§二（架构与存储）
2. 涉及层 → 对应 `L*.md` + `L*.design.md`
3. 动手前 → 对应 `*-development.md` 查代码锚点与差距

---

## 2. 文档地图

### Memory 总（跨层）

| 类型 | 文档 | 说明 |
|------|------|------|
| 需求 | [`memory.md`](./memory.md) | 五层产品定位、Memory Center UX、权限、信息架构 |
| 需求 | [`neural-memory.md`](./neural-memory.md) | **神经记忆系统**：时间感知、联动更新、仿生生命周期、Memory-Agent |
| 设计 | [`neural-memory.design.md`](./neural-memory.design.md) | 神经记忆系统设计：数据模型、接口、核心流程、Proto 扩展 |
| 开发计划 | [`neural-memory-development.md`](./neural-memory-development.md) | 神经记忆系统开发计划：48 项任务、3 Phase、验收标准 |
| 设计 | [`memory.design.md`](./memory.design.md) | 目标架构、存储拓扑、双轨、Policy、MemoryWorker、Proto 索引 |
| 开发计划 | [`memory-development.md`](./memory-development.md) | 模块定位、分层包表、全局现状、Phase、跨层任务 |
| 优化方案 | [`memory-optimization-2026-05-26.md`](./memory-optimization-2026-05-26.md) | 6 项业务逻辑优化（L3 双轨、L4 衰减、队列隔离、PII、提取协议、Cascade Saga） |
| 理论 | [`theory.md`](./theory.md) | 知识体系思辨（原 `38 memory.md`） |

### 分层

| 层 | 用户名称 | 需求 | 设计 | 开发计划 |
|----|----------|------|------|----------|
| L0 | 上下文窗口 | [`L0.md`](./L0.md) | [`L0.design.md`](./L0.design.md) | [`L0-development.md`](./L0-development.md) |
| L1 | 工作记忆 | [`L1.md`](./L1.md) | [`L1.design.md`](./L1.design.md) | [`L1-development.md`](./L1-development.md) |
| L2 | 会话事件 | [`L2.md`](./L2.md) | [`L2.design.md`](./L2.design.md) | [`L2-development.md`](./L2-development.md) |
| L3 | 知识记忆 | [`L3.md`](./L3.md) | [`L3.design.md`](./L3.design.md) | [`L3-development.md`](./L3-development.md) |
| L4 | 图谱与进化 | [`L4.md`](./L4.md) | [`L4.design.md`](./L4.design.md) | [`L4-development.md`](./L4-development.md) |

---

## 3. 关联文档

| 场景 | 文档 |
|------|------|
| 运行时边界 | [`AGENT_RUNTIME_BOUNDARY.md`](../../AGENT_RUNTIME_BOUNDARY.md) |
| Session 与 L0 压缩 | [`10 session.md`](../10%20session.md) · [`10 session.design.md`](../10%20session.design.md) |
| Agent 设置记忆 Tab | [`5 agent-setting.md`](../5%20agent-setting.md) · **记忆 Tab**：`l0_compress_*`、`memory_worker_*`（巩固 Worker 模型） |
| Agent 进化 | [`7 agent-evolution.md`](../7%20agent-evolution.md) |
| 模块 Review | [`review/memory-review.md`](../../review/memory-review.md) |
| 行业差距分析 | [`review/memory-module-gap-analysis.md`](../../review/memory-module-gap-analysis.md) |
| 优化提案（学术） | [`review/memory-optimization-proposal.md`](../../review/memory-optimization-proposal.md) |
| SQL 迁移 | [`internal/data/sql/memory_chain.sql`](../../../internal/data/sql/memory_chain.sql) |
| Legacy 迁移（旧 trpc_memory） | [`memory.design.md`](./memory.design.md) §3.1、§十一 · [`L3.design.md`](./L3.design.md) §3.8 · [changelog](../../changelog/2026-05-24-Memory-Legacy-Backfill-Startup.md) |

---

## 4. 历史路径（已删除）

原 `12-16 memory*.md`、`12–16 memory-L*.md`、`38 memory.md` 已删除；内容已迁入本目录。Git 历史可查旧版。
