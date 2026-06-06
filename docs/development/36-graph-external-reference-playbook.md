# Graph × Team 外部参考借鉴手册（Flowise + AgentCoord）

> **版本**：2026-05-25 | **状态**：📋 参考手册（无外部源码依赖）  
> **模块**：M36 Graph · M53 Team×Graph · M54 Hermes Kanban  
> **上级索引**：[docs/README.md §5.2 Graph/Team](../README.md#52-需求与设计) · [36 graph-workflow.md](./36%20graph-workflow.md) · [53 team-graph-orchestration.design.md](./53%20team-graph-orchestration.design.md)

---

## 1. 文档定位

本文整理 **Flowise**（节点图编辑器 + Agentflow 执行）与 **AgentCoord**（多 Agent 协调策略可视化探索）中，适合 Aranea 借鉴的模式。

**本文不依赖外部仓库源码**——在其他机器上仅 clone `aranea-agents` 即可阅读与落地；外部项目以公开链接 + 下文摘要为准。

**本文做四件事：**

1. **边界**：什么值得学、什么不要搬。
2. **对照**：外部能力与 Aranea 现有落点的映射表。
3. **启发**：按 P0/P1/P2 排列的可执行项。
4. **阅读清单**：本仓库内应读的文档与代码锚点（§8）。

**不变量（实施时不得破坏）：**

- `internal/biz` 不 import `pkg/trpc-agent-go`
- Graph 构建与运行仅在 `internal/graph/trpc` + `internal/service`
- 运行时真相源：`GraphDefinition`（Proto）/ `OrchestrationSpec`（M53）
- 前端栈：Vue 3 + Quasar + Vue Flow（不引入 React/MUI 双栈）
- M53 终态：Team Run 经 `CompileToGraphRuntimeConfig` → `GraphAgent` 单链执行

---

## 2. 外部参考速览

### 2.1 Flowise

| 项 | 说明 |
|----|------|
| 定位 | 可视化构建 AI Agent / Agentflow 的低代码平台 |
| 仓库 | https://github.com/FlowiseAI/Flowise |
| 关键包 | `packages/agentflow`（`@flowiseai/agentflow`，React 可嵌入编辑器） |
| 画布 | ReactFlow v11 |
| 后端 | Node.js + TypeORM；Agentflow 执行在 `buildAgentflow.ts`（队列调度） |
| 数据模型 | ReactFlow JSON：`nodes[].data.inputs` + `edges[].sourceHandle` 表分支 |
| 节点 | Start / Agent / LLM / Condition / Loop / HTTP / Iteration / ExecuteFlow 等 ~13 种 |
| 强项 | Schema 驱动属性面板、变量 `{{nodeId.output}}`、连接校验、AI 生成 Flow、节点运行态着色 |
| 弱项 | 无 Checkpoint/TimeTravel；执行与 LangChain 组件强绑定；非企业工作流引擎 |

**架构一句话**：画布 JSON 即定义，Node 服务器按图队列执行 LLM 组件链。

### 2.2 AgentCoord

| 项 | 说明 |
|----|------|
| 定位 | 帮助用户**可视化探索**多 Agent **协调策略**（研究原型） |
| 仓库 | https://github.com/AgentCoord/AgentCoord |
| 论文 | https://arxiv.org/abs/2404.11943 |
| 后端 | Flask + OpenAI/Groq；PlanEngine（设计）+ RehearsalEngine（排练执行） |
| 前端 | React + MUI + MobX；**四栏布局 + SVG 跨栏连线**（非节点图画布） |
| 数据模型 | **树**而非 DAG：StepTask 树 → AgentSelection → AgentAction 树（Propose/Critique/Improve/Finalize） |
| 强项 | 步骤/动作双层分支、Agent 热力图选型、Rich Collaboration Brief、增量排练 |
| 弱项 | 无 DB、Agent 仅为 role prompt、无 Tool/RAG、无生产 API |

**架构一句话**：LLM 生成协作计划，用户分支对比策略，排练式 dry-run 验证协调模式。

### 2.3 三者关系（Aranea 视角）

```
Flowise    → 教「图编辑器 UX / 动态表单 / 校验」
AgentCoord → 教「协调策略探索 / 多方案对比 / 跨视图联动」
Aranea     → 教「确定性执行 / State+Reducer / HITL+Task / Checkpoint / Agent 框架集成」
```

---

## 3. 与 Aranea 能力对照

| 能力 | Flowise | AgentCoord | Aranea 现状 | 借鉴方向 |
|------|---------|------------|-------------|----------|
| 节点图画布 | ✅ ReactFlow | ❌ 四栏文档式 | ✅ Vue Flow | Flowise：多 handle、StickyNote、schema 表单 |
| 图定义契约 | 画布 JSON | Plan 树 | ✅ `GraphDefinition` Proto | 保持 Proto，不引入外部模型 |
| 执行引擎 | Node 队列 | LLM 链排练 | ✅ trpc-agent-go BSP/DAG | 不替换 |
| 条件路由 | sourceHandle | ❌ | ✅ conditionalEdges | Flowise 条件 UI |
| State + Reducer | 弱 | 命名 Object | ✅ StateFieldDef | AgentCoord brief 分色展示 state 流 |
| HITL | HumanInput 节点 | ❌ | ✅ hitl + Task 系统 | Flowise 表单预览 UX |
| Checkpoint / TimeTravel | ❌ | ❌ | ✅ | 差异化，继续强化 |
| Task Kanban | ❌ | ❌ | ✅ M54 | — |
| Team 编译为 Graph | ❌ | ❌ | 🚧 M53 | AgentCoord 策略探索向导 |
| AI 生成拓扑 | ✅ | ✅ basePlan | 部分模板 | 生成 → Validate → 人工改 |
| 运行态 WS 投影 | SSE | ❌ | ✅ | — |
| Agent 选型 | 下拉 | ✅ 热力图 | Agent 目录 | AgentCoord 多维度推荐 |

---

## 4. 启发清单（按优先级）

### P0 — 巩固 Aranea 差异化（继续现有路线，不抄外部运行时）

| ID | 项 | 说明 |
|----|-----|------|
| GR-REF-00 | M53 执行单链 | Team → CompileToGraph → GraphAgent；见 [53-team-graph-orchestration-development.md §8](./53-team-graph-orchestration-development.md#8-终态路线图team-规格--graph-执行单链) |
| GR-REF-01 | Graph Run + Kanban + WS | 运行观测优于两参考项目；保持 `useGraphExecutionStream` |
| GR-REF-02 | Checkpoint / TimeTravel UI | 独家能力；见 [36-graph-development.md](./36-graph-development.md) Phase C |

### P1-A — 设计态（Flowise 为主 → Graph 编辑器）

| ID | 项 | 借鉴要点 | Aranea 落点 |
|----|-----|----------|------------|
| GR-REF-10 | Schema 驱动属性面板 | `InputParam[]` 动态表单 | `web/src/components/graph/GraphPropertyPanel.vue` + 新 `features/graph/schema/` |
| GR-REF-11 | VariablePicker | `{{nodeId.field}}` 引用 state | Agent/Router mapper、instruction 字段 |
| GR-REF-12 | 连接校验增强 | 环/悬空/类型/handle | 扩展 `GraphValidationPanel` + 后端 `validator.go` |
| GR-REF-13 | Router 多 handle | 条件边 label | `GraphFlowDiamond.vue` |
| GR-REF-14 | AI 生成 Graph | NL → JSON → Validate | 新 service RPC；**禁止**自动保存未校验图 |
| GR-REF-15 | StickyNote | 纯 UI 注释 | `metadata.stickyNotes` |
| GR-REF-16 | EditNodeDialog 模式 | 复杂节点弹窗编辑 | 可选：大表单从侧栏迁到 Dialog |

### P1-B — 策略态（AgentCoord 为主 → Team / Observatory）

| ID | 项 | 借鉴要点 | Aranea 落点 |
|----|-----|----------|------------|
| GR-REF-20 | 多方案对比 | branch_PlanOutline | M53 Observatory：OrchestrationSpec 候选 Tab |
| GR-REF-21 | Agent 热力图 | agentSelectModify 多维度打分 | Team member 推荐 UI |
| GR-REF-22 | PCIF 协调模板 | Propose→Critique→Improve→Finalize | Graph 内置模板 / Team preset |
| GR-REF-23 | 跨栏 SVG 联动 | ViewConnector DOM ref 连线 | Graph Run ↔ Kanban ↔ Log focus 联动 |
| GR-REF-24 | Rich Collaboration Brief | 输入/Agent/任务/输出分色 | Observatory / Kanban 卡片描述 |
| GR-REF-25 | 分步排练 | executePlan(stepsToRun=N) | Graph Execute 支持「跑 N 步」或 dry-run |

### P2 — 节点生态（需 trpc-agent-go + builder 先支持）

| ID | 项 | 条件 |
|----|-----|------|
| GR-REF-30 | Loop / Iteration | `pkg/trpc-agent-go/graph` 子图/循环 API |
| GR-REF-31 | HTTP / Retriever 节点 | 业务需求 + `internal/graph/trpc/builder.go` |
| GR-REF-32 | ExecuteSubgraph UI | 已有 `SubgraphDef`，补编辑器 |

---

## 5. 明确不做

| # | 项 | 原因 |
|---|-----|------|
| 1 | 迁移 Flowise UI 或嵌入 `@flowiseai/agentflow` | React/MUI/Flowise API；与 Quasar 栈冲突 |
| 2 | 引入 Flowise Server / flowise-components | Node 运行时与 trpc-agent-go 语义不对齐 |
| 3 | 引入 AgentCoord Flask 后端 | 计划树不能替代 `GraphDefinition` 执行 |
| 4 | 用 AgentCoord Plan 树作运行时真相源 | 与 M53 OrchestrationSpec / Graph Proto 冲突 |
| 5 | 为 UI 堆节点类型超出 builder 能力 | 设计态与运行态脱节 |
| 6 | 长期双运行时（Team Native + Graph） | M53 终态单链 |

---

## 6. 字段/概念映射（无外部源码时查表）

### 6.1 Flowise FlowData → GraphDefinition（概念级）

| Flowise | Aranea GraphDefinition | 备注 |
|---------|------------------------|------|
| `FlowNode.id` | `NodeDef.id` | — |
| `FlowNode.type` / `data.name` | `NodeDef.type` + `funcRef` / `agentName` | 需映射表，非 1:1 |
| `FlowNode.position` | `metadata.layout[id]` | 已有 |
| `FlowEdge.source/target` | `EdgeDef.from/to` | — |
| `FlowEdge.sourceHandle` | `ConditionalEdgeDef.pathMap` | Router 分支 |
| `data.inputs`（KV） | `NodeDef` 各字段 + `stateFields` | Aranea 更结构化 |
| Start 节点 | `entryPoint` | Aranea 可无视觉 Start 节点 |
| HumanInput | `hitl` + Task | Aranea 更强 |

### 6.2 AgentCoord Plan → OrchestrationSpec / Graph（概念级）

| AgentCoord | Aranea | 备注 |
|------------|--------|------|
| General Goal | Team goal / Graph metadata | — |
| StepTask（name/content/inputs/output） | Graph 线性链或 agent 节点序列 | 编译器可生成 |
| AgentSelection | Team members / Graph agent 节点 | — |
| AgentAction（PCIF） | 子图模板或 Team 内多轮 | 映射为 Graph 模板 |
| KeyObjects | `StateFieldDef` + reducer | Aranea 更正式 |
| branches（步骤/动作） | 设计态候选 spec，非运行时边 | Observatory 对比用 |
| RehearsalLog | Graph execution steps + WS | Aranea 已有 |

---

## 7. 外部公开链接（备查）

| 资源 | URL |
|------|-----|
| Flowise GitHub | https://github.com/FlowiseAI/Flowise |
| @flowiseai/agentflow npm | https://www.npmjs.com/package/@flowiseai/agentflow |
| AgentCoord GitHub | https://github.com/AgentCoord/AgentCoord |
| AgentCoord 论文 | https://arxiv.org/abs/2404.11943 |
| AgentCoord 演示视频 | https://youtu.be/s56rHJx-eqY |

> 需要对照实现细节时再 clone 外部仓库；日常开发以本文 + 本仓库代码为准。

---

## 8. 本仓库阅读清单（其他机器开发顺序）

按顺序阅读即可开工，**无需** Flowise/AgentCoord 源码。

### 8.1 必读（架构 + 红线）

| 顺序 | 文档 | 用途 |
|------|------|------|
| 1 | [docs/README.md](../README.md) | 项目入口、验证分级 |
| 2 | [guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) | 后端红线 |
| 3 | [guides/frontend-guide.md](../guides/frontend-guide.md) | 前端红线 |
| 4 | [docs/AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md) | Runner / Graph 边界 |
| 5 | **本文** | 外部参考与任务 ID |

### 8.2 Graph 模块（M36）

| 顺序 | 文档 | 用途 |
|------|------|------|
| 6 | [36 graph-workflow.md](./36%20graph-workflow.md) | 产品需求四维架构 |
| 7 | [36 graph-workflow.design.md](./36%20graph-workflow.design.md) | 技术设计 |
| 8 | [36-graph-development.md](./36-graph-development.md) | **实现差距与 Phase 真相源** |
| 9 | [frontend-pages.md](./frontend-pages.md) | Graph 相关路由与页面 |

### 8.3 Team×Graph（M53）与 Kanban（M54）

| 顺序 | 文档 | 用途 |
|------|------|------|
| 10 | [53 team-graph-orchestration.md](./53%20team-graph-orchestration.md) | 融合需求 |
| 11 | [53 team-graph-orchestration.design.md](./53%20team-graph-orchestration.design.md) | OrchestrationSpec · Observatory |
| 12 | [53-team-graph-orchestration-development.md](./53-team-graph-orchestration-development.md) | Phase 5–7 终态 |
| 13 | [54-hermes-kanban-development.md](./54-hermes-kanban-development.md) | Task 运行时与 Kanban |

### 8.4 代码锚点（改代码前 CodeGraph 查）

| 区域 | 路径 |
|------|------|
| Graph 引擎 | `internal/graph/trpc/` |
| Graph 业务 | `internal/biz/graph.go` · `internal/biz/graph_runtime.go` |
| Graph RPC | `internal/service/graph.go` · `api/kratos/graph/v1/graph.proto` |
| Team 编译 | `internal/team/graph_compile.go` |
| 前端类型/API | `web/src/features/graph/types.ts` · `api.ts` |
| 前端编辑器 | `web/src/components/graph/` · `web/src/features/graph/` |
| 框架真相源 | `pkg/trpc-agent-go/graph/` |

### 8.5 变更落地后

| 动作 | 文档 |
|------|------|
| 记录完成项 | `docs/changelog/YYYY-MM-DD-*.md` |
| 更新 Phase 状态 | 对应 `*-development.md` |
| 可选 Review | `docs/review/` |

---

## 9. 与现有开发计划的关系

| 本文 ID | 建议写入 |
|---------|----------|
| GR-REF-10 ~ 16 | [36-graph-development.md](./36-graph-development.md) 后续 Phase（设计态增强） |
| GR-REF-20 ~ 25 | [53-team-graph-orchestration-development.md](./53-team-graph-orchestration-development.md) Observatory / 向导 |
| GR-REF-30 ~ 32 | [36 graph-workflow.md](./36%20graph-workflow.md) P2 backlog |

实施某 ID 时：在 development 文档对应 Phase 增加任务块，验收标准引用本文 §4。

---

## 10. 一句话总结

**抄 UX 和策略探索，不抄运行时。** Flowise 提升 Graph 设计态；AgentCoord 提升 Team/Observatory 策略态；Aranea 以 `GraphDefinition` + trpc-agent-go 守住执行态。
