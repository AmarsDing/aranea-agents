# Research: Langflow UI 复刻方案 — 设计思想适配与业务落地

> **日期**：2026-06-09
> **版本**：v1.0
> **范围**：Graph 模块 UI 重构，基于 Langflow 设计思想的适配方案

---

## 摘要

本报告基于对 Langflow（GitHub 146K+ stars，v1.8/1.9）前端架构和 UI 设计的深度调研，结合 Aranea-Agents Graph 模块的业务领域模型，提出一套**以业务为中心的 UI 复刻方案**。核心结论：Langflow 的设计思想（类型化端口、节点折叠、状态指示、内嵌运行面板、连接验证）值得复刻，但其具体实现（LangChain 数据类型层级、聊天式 Playground、Python 代码编辑器）不适配 Aranea 的业务模型。Aranea Graph 的本质是**确定性多 Agent 编排引擎**，数据流由 State Schema 驱动，节点间传递的是对共享 State 的读写而非 LangChain 数据类型。方案按 P0/P1/P2 三期规划，P0 聚焦节点重构、State-Aware 端口、运行时状态指示、内嵌 RunPanel 四项核心改造。

---

## 一、Langflow 项目概览

### 1.1 基本信息

| 维度 | 详情 |
|------|------|
| GitHub | langflow-ai/langflow |
| Stars | 146K+（截至 2026-03） |
| 许可证 | MIT |
| 语言构成 | Python ~70%（后端）、TypeScript/React ~30%（前端） |
| 最新版本 | v1.8（2026-03-06）/ v1.9.x |
| 核心定位 | 低代码 AI Agent 和工作流构建平台 |
| 开发方 | DataStax（2024-04 收购） |

### 1.2 技术栈

| 层 | 技术 | 职责 |
|----|------|------|
| 前端 | React 18 + TypeScript | 可视化图编辑器、组件面板、属性编辑器 |
| 图编辑器 | React Flow (@xyflow/react) | 节点/边渲染、拖拽、缩放、连接 |
| 状态管理 | Zustand | flowStore / sidebarStore / chatStore / alertStore |
| UI 组件 | shadcn/ui (Radix UI) | 无障碍、一致的交互模式 |
| 样式 | TailwindCSS | 原子化样式 |
| 后端 | FastAPI (Python) | 流程执行引擎、组件注册、API 服务 |
| 运行时 | LangChain | LLM 调用、工具执行、链式编排 |
| 数据存储 | SQLite / PostgreSQL | 流程定义、用户数据持久化 |

### 1.3 设计哲学

1. **低代码优先**：一切操作尽量通过拖拽和配置完成
2. **组件即节点**：每个 LangChain 组件映射为一个可视节点
3. **类型化连接**：端口有明确的类型标注，只有类型兼容的端口才能连接
4. **即时预览**：支持在编辑器中直接运行流程并查看输出
5. **可扩展性**：支持自定义组件（Python 代码），用户可创建自己的节点类型

---

## 二、Langflow UI 架构详解

### 2.1 GenericNode 组件层级

```
GenericNode (React Flow 自定义节点)
├── NodeToolbar (悬停工具栏)
│   ├── 验证 / 运行 / 冻结 / 复制 / 删除 / 保存
│   └── 更多操作（编辑描述、添加/移除端口、导出、调试）
├── NodeHeader (标题栏)
│   ├── 图标 (Icon) — 来自 nodeType 映射
│   ├── 显示名称 (Display Name)
│   ├── 状态指示器 (StatusBadge)
│   │   ├── idle (灰色) / building (黄色脉冲) / running (蓝色脉冲)
│   │   └── success (绿色) / error (红色)
│   └── 冻结标识 (FrozenBadge)
├── NodeContent (内容区)
│   ├── InputHandles (左侧输入端口)
│   │   └── Handle × N (每个输入参数一个)
│   │       ├── Handle 圆点 + 参数名 + 类型标识
│   │       └── 内联编辑器（如可编辑）
│   ├── ParameterArea (参数编辑区)
│   │   ├── 文本/下拉/文件/代码/开关/滑块/键值对/表格/字典
│   │   └── 根据参数类型自动选择编辑器组件
│   └── OutputHandles (右侧输出端口)
│       └── Handle × N (每个输出参数一个)
└── NodeFooter (底部)
    ├── 错误信息展示 (ErrorBadge)
    └── 运行计时 (RunTimer)
```

### 2.2 整体页面布局

```
FlowPage
├── TopBar (顶部工具栏)
│   ├── 流程名称（可编辑）
│   ├── 保存状态指示
│   ├── 撤销/重做 / 运行 / 部署 / 分享 / 设置
├── MainLayout (水平三栏)
│   ├── Sidebar (左侧, 280px, 可折叠)
│   │   ├── 搜索框
│   │   ├── 分类标签 (All/Favorites/LLMs/Prompts/Data/Helpers/Custom)
│   │   └── 组件卡片列表 (draggable)
│   ├── CanvasArea (中间, flex:1)
│   │   └── ReactFlow
│   │       ├── GenericNode × N
│   │       ├── CustomEdge × N
│   │       ├── MiniMap / Controls / Background
│   │       └── 画布工具栏
│   └── RightPanel (右侧, 400px, 可折叠)
│       ├── NodeDetailPanel (选中节点时)
│       └── PlaygroundPanel (运行时)
└── StatusBar (底部状态栏)
```

### 2.3 连接验证架构

**类型层级**：
```
Any (根类型)
├── Data → Text / DataFrame / File / Image
├── Message → HumanMessage / AIMessage / SystemMessage
├── Tool
├── Embeddings
├── VectorStore
└── BaseLanguageModel → LLM / ChatModel
```

**验证规则**：
1. 子类型兼容：`HumanMessage` 可连接到 `Message` 输入
2. Any 规则：`Any` 输入接受所有类型
3. Data 规则：`Data` 输入接受所有 Data 子类型
4. 多类型输入：`input_types: ["Message", "Data"]` 接受两种
5. 循环检测：DFS 检测有向环
6. 重复连接检测：同一 source-target 对不可重复

**视觉反馈**：
- 有效连接：绿色高亮 + 吸附效果
- 无效连接：红色高亮 + 拒绝动画 + 错误提示
- 拖拽中：半透明预览线 + 目标 Handle 高亮

### 2.4 Playground 集成

**三种模式**：
1. **右侧面板模式**：点击节点"运行"后，右侧面板切换到 Playground
2. **全屏模式**：顶部"Run Flow"按钮进入全屏 Playground
3. **内嵌模式**：ChatInput/ChatOutput 节点内显示迷你聊天界面

**数据流**：用户输入 → ChatInput 节点 → 流程执行 → ChatOutput 节点 → Playground 显示

### 2.5 v1.8 新增特性

| 特性 | 说明 |
|------|------|
| **Traces** | 流级 + 组件级执行轨迹，可视化延迟和 Token 用量 |
| **Inspection Panel** | 实时查看组件内部状态、输入/输出、日志、Token |
| **全局模型提供商** | 集中配置 LLM 凭证，组件引用而非各自配置 |
| **V2 Workflow API** | 更简洁的响应格式 + 异步后台任务支持 |
| **Knowledge Bases** | 本地向量数据库，数据不需每次从远程重新摄取 |

---

## 三、Aranea Graph 模块现状

### 3.1 技术栈

| 层面 | 技术 |
|------|------|
| 图编辑器 | @vue-flow/core ^1.48.2 |
| 插件 | @vue-flow/background / controls / minimap |
| 自动布局 | dagre (LR, nodesep=60, ranksep=120) |
| 状态管理 | Pinia (stores/graph/index.ts) |
| UI 组件 | Quasar |
| 后端 | Kratos v2 (HTTP/gRPC) + trpc-agent-go (Graph Agent) |

### 3.2 前端组件清单（23 个）

| 组件 | 功能 |
|------|------|
| GraphEditorCanvas.vue | 画布主组件，VueFlow 集成 |
| GraphFlowNode.vue | 自定义节点（矩形，按 NodeType 着色） |
| GraphFlowDiamond.vue | 自定义菱形节点（Router/Join） |
| GraphFlowEdge.vue | 自定义边，含流动光点动画 |
| GraphPropertyPanel.vue | 属性面板（节点/Graph/State Schema） |
| GraphNodePalette.vue | 节点面板（拖拽添加） |
| GraphNodeSearch.vue | 节点搜索 (Ctrl+F) |
| GraphContextMenu.vue | 右键菜单 |
| GraphRunDialog.vue | 执行对话框 |
| GraphRunInspector.vue | 运行监控检查器 |
| GraphRunSidebar.vue | 运行监控侧边栏 |
| GraphCheckpointPanel.vue | 检查点面板 |
| GraphTimeTravelPanel.vue | TimeTravel 面板 |
| GraphHitlDialog.vue | HITL 中断对话框 |
| GraphTaskKanban.vue | 任务看板 |
| GraphTaskDetailDrawer.vue | 任务详情抽屉 |
| GraphVersionPanel.vue | 版本历史面板 |
| GraphValidationPanel.vue | 校验结果面板 |
| GraphVariablePicker.vue | 变量引用选择器 |
| GraphTemplatePicker.vue | 模板选择器 |
| GraphRunDialog.vue | 执行对话框 |
| GraphCardContextMenu.vue | 列表页卡片右键菜单 |
| GraphDetailPanel.vue | 详情面板 |

### 3.3 Composable 清单（16 个）

| Composable | 功能 |
|------------|------|
| useGraphEditorPage.ts | 编辑器页面主逻辑 |
| useGraphsPage.ts | 列表页逻辑 |
| useGraphRunPage.ts | 运行监控页逻辑 |
| useGraphExecutionsPage.ts | 执行历史页逻辑 |
| useGraphExecute.ts | 执行逻辑 |
| useGraphUndoRedo.ts | 撤销/重做（50 步栈） |
| useGraphLocalValidation.ts | 前端实时校验（8 种规则） |
| useGraphEditorAssets.ts | 版本/模板/导入导出 |
| useGraphRunHitl.ts | HITL 中断/恢复 |
| useGraphRunTasks.ts | 任务系统 |
| useConditionalRoutes.ts | 条件路由编辑 |
| useSnapGuide.ts | 对齐辅助线 |
| useGraphStream.ts | Graph WS 流基础 |
| useGraphRunStream.ts | 运行流 |
| useGraphExecutionStream.ts | 执行流 |
| useGraphTimeTravel.ts | TimeTravel 逻辑 |

### 3.4 7 种节点类型及其运行时语义

| 节点类型 | 业务语义 | 运行时接线 | 与 Langflow 差异 |
|----------|---------|-----------|-----------------|
| **Function** | 纯逻辑处理，不涉及 LLM | `sg.AddNode(id, func)` 通过 Registry FunctionResolver 解析 | Langflow 的 Python 节点是任意代码执行；Aranea 必须通过 Registry 注册 |
| **LLM** | 轻量级 LLM 调用，无完整 Agent 生命周期 | `sg.AddLLMNode(id, model, instruction, toolMap)` | Langflow 的 LLM 节点通常带完整配置；Aranea 是单轮推理，无 Memory/Skill |
| **Tool** | 确定性直接调用工具，不经过 LLM 推理 | `sg.AddToolsNode(id, toolMap)` | Langflow 的 Tool 节点通常是 LLM 驱动的工具调用 |
| **Agent** | 引用系统 Agent，作为子图节点嵌入 | `sg.AddAgentNode(id)` 通过 AgentResolver 解析 | Langflow 无"Agent 作为子图"概念；Aranea 的 Agent 节点拥有完整生命周期 |
| **Router** | 条件路由，根据 State 选择分支 | `sg.AddConditionalEdges(from, condFunc, pathMap)` | Langflow 条件分支通常是 LLM 判断；Aranea 使用注册的 Go 函数保证确定性 |
| **Join** | 汇聚并行分支 | BSP/DAG 引擎自动处理 | Langflow 无显式 Join 概念 |
| **HITL** | 人工审批，强制中断等待 | `sg.AddNode(id, func, WithInterruptAfter())` | Langflow 无内置 HITL 支持 |

### 3.5 边类型与流转规则

| 边类型 | Proto 定义 | 运行时语义 | 视觉 |
|--------|-----------|-----------|------|
| **Runtime Edge** | `EdgeDef { from, to, kind }` | A 完成后必定到 B | kind="" 实线 / kind="transfer"/"dispatch" 虚线动画 |
| **Conditional Edge** | `ConditionalEdgeDef { from, condFuncRef, pathMap }` | CondFunc 返回值选择分支 | 每条 pathMap 虚线箭头 + 标签 |
| **Command Edge** | `NodeDef.destinations` | Agent 运行时动态决定下一步 | 运行时通过 WS 事件动态高亮 |

### 3.6 State Schema 与状态流

State Schema 是 Graph 的核心数据模型，定义所有节点共享的全局状态结构：

```
StateFieldDef {
  name: string           -- 字段名
  type: string           -- 类型 (string/integer/float/boolean/array/object)
  reducer: ReducerType   -- 聚合策略
  defaultValue: any      -- 默认值
  required: boolean      -- 是否必填
}
```

四种 Reducer：

| Reducer | 框架映射 | 语义 | 典型场景 |
|---------|---------|------|----------|
| `default` | DefaultReducer | 完全替换旧值 | 单值覆盖 |
| `append` | AppendReducer | 追加到列表 | 消息历史、执行日志 |
| `cover` | CoverReducer | 仅非零值覆盖 | 可选更新 |
| `merge` | MergeReducer | 深度合并 Map | 多节点并发写入 |

状态流经 Graph 的路径：
```
初始 State → EntryPoint → 节点执行(读写 State) → 边规则决定下一节点 → ... → FinishPoint → 最终 State
```

Agent 节点的特殊映射：
- **InputMapper**：将 Graph State 投影为 Agent 运行时状态
- **OutputMapper**：将 Agent 输出写回 Graph State
- 这是 Graph 与 Agent 运行时的**状态桥梁**

### 3.7 执行模型

| 引擎 | 运行时 | 特点 | 适用场景 |
|------|--------|------|----------|
| **BSP** (默认) | ExecutionEngineBSP | Pregel 模型，超步同步，确定性可复现 | 审批流、合规流程 |
| **DAG** | ExecutionEngineDAG | 依赖分析，无依赖节点并行，高吞吐 | 数据处理管线 |

### 3.8 运行时事件桥接

| trpc-agent-go 事件 | Aranea EnvelopeType | 推送目标 |
|---------------------|---------------------|----------|
| `graph.node.start` | `graph_node_start` | WS → 前端 |
| `graph.node.complete` | `graph_node_end` | WS → 前端 |
| `graph.node.error` | `graph_node_error` | WS → 前端 |
| `graph.node.custom` | `graph_node_custom` | WS → 前端 |
| `graph.pregel.step` | `graph_step` | WS → 前端 |
| `checkpoint.interrupt` | `checkpoint` | WS → 前端 HITL 对话框 |
| `checkpoint.created` | `checkpoint` | WS → 前端 |
| `graph.state.update` | `state_delta` | WS → 前端 |
| `graph.execution` (done) | `graph_execution_done` | WS → 前端 |

### 3.9 当前 UI 已知问题

| # | 严重度 | 问题 | 状态 |
|---|--------|------|------|
| 11 | Medium | `nextNodes` prop 永远无数据 | 待定 |
| 16 | Low | GraphPropertyPanel 使用 `as any` 绕过类型检查 | 待定 |

### 3.10 未实现功能

| 功能 | 状态 | 说明 |
|------|------|------|
| 子图嵌套编辑器 | 未实现 | 后端 SubgraphDef 已支持，前端待补 |
| 动态任务节点插入 | 未实现 | 无 DynamicNodeInsert 事件 |
| 熔断策略 UI | 未实现 | Proto CircuitBreakerPolicy 已定义未接入 |

---

## 四、关键差异分析

### 4.1 确定性 vs 灵活性

| 维度 | Aranea Graph | Langflow |
|------|-------------|----------|
| 路由逻辑 | Go 函数（Registry 注册），确定性 | LLM 判断或 Python 代码，非确定性 |
| 状态管理 | 显式 State Schema + Reducer，类型安全 | 隐式字典传递，无类型约束 |
| 执行保证 | BSP 引擎保证可复现 | 无此保证 |
| 审计追踪 | Checkpoint + TimeTravel，任意状态回放 | 无内置支持 |

### 4.2 Agent 一等公民

| 维度 | Aranea Graph | Langflow |
|------|-------------|----------|
| Agent 概念 | Agent 是独立实体，拥有 Session/Memory/Tool/Skill | 无独立 Agent 概念，只有 LLM 节点 |
| 子图嵌套 | 子图编译为 Agent，天然递归组合 | 支持子流程但非 Agent 语义 |
| 状态隔离 | InputMapper/OutputMapper 显式映射 | 无隔离机制 |

### 4.3 人机协同

| 维度 | Aranea Graph | Langflow |
|------|-------------|----------|
| 人工审批 | HITL 节点 + InterruptBefore/After + Checkpoint | 无内置支持 |
| 任务派工 | 完整任务系统（状态机/派工/审核/超时/心跳） | 无 |
| 状态回放 | TimeTravel（任意步骤回放 + 编辑 + 重执行） | 无 |

### 4.4 运行时治理

| 维度 | Aranea Graph | Langflow |
|------|-------------|----------|
| 重试策略 | 节点级 RetryPolicy + FallbackAgent | 无内置支持 |
| 缓存策略 | 节点级 CachePolicy（KeyFunc + TTL） | 无内置支持 |
| 失败恢复 | failureAction + FallbackAgent | 无内置支持 |
| 熔断 | CircuitBreakerPolicy 已定义（待接入） | 无 |
| 心跳超时 | Agent 心跳上报 + 租约延期 | 无 |

---

## 五、设计思想映射

### 5.1 核心原则：复刻思想，适配业务

Langflow 的 UI 设计思想值得学习，但其具体实现绑定 LangChain 生态，不能照搬。核心映射关系：

| Langflow 设计思想 | 本质 | Aranea 适配方式 |
|-------------------|------|-----------------|
| **类型化端口** | 连接时防止语义错误 | **State 字段引用标注**：端口标注读/写 State 的哪些字段，连接时验证字段可达性 |
| **节点折叠** | 复杂图的空间管理 | 直接复刻，折叠时保留 State 读写摘要 |
| **节点状态指示** | 运行时可视化 | 适配 Aranea 的 5 种节点状态 + HITL 中断态 |
| **内嵌 Playground** | 编辑→测试闭环 | **内嵌 RunPanel**：State 初始化 → 执行 → 实时节点状态 + State 变迁 + Checkpoint |
| **Inspection Panel** | 节点级调试 | **State Diff 面板**：节点执行前后 State 变化 |
| **NodeToolbar** | 快捷操作 | 适配：运行到此/设为入口/HITL 配置/冻结 |
| **侧边栏分组** | 组件发现 | 按 7 种节点类型分组 |
| **连接验证** | 防止错误连接 | **State 字段级验证**：下游 READS 必须在上游 WRITES 或 State Schema 中有定义 |

### 5.2 不应复刻的 Langflow 特性

| Langflow 特性 | 不复刻原因 |
|---------------|-----------|
| Python 代码编辑器节点 | Aranea 的 Function 节点通过 Registry 注册 Go 函数，不支持运行时代码编辑 |
| 组件市场/Store | Aranea 的节点类型是固定的 7 种，不支持用户自定义组件 |
| LLM 模型选择下拉 | Aranea 的 Model 通过 ModelResolver 解析，不在节点内配置凭证 |
| VectorStore 组件 | Aranea 不直接暴露向量库操作，通过 Tool/Agent 间接使用 |
| MCP Server 分享 | Aranea 的分享机制不同，通过 API 端点暴露 |
| 全局模型提供商设置 | Aranea 的模型配置在 Agent 目录中管理，不在 Graph 层面 |
| LangChain 数据类型层级 | Aranea 的数据流由 State Schema 驱动，不是 LangChain 组件数据类型 |

---

## 六、复刻方案

### P0 — 核心体验升级

#### P0-1：节点重构 — State-Aware Node

**目标**：将 GraphFlowNode 从"固定高度矩形"重构为"State-Aware 可折叠节点"，显示每个节点对 State 的读写字段。

**节点视觉结构**：

```
展开态：
┌─────────────────────────────────────────┐
│ [🤖] Agent: CustomerService    [▼] [⚡] │  ← 标题栏：图标+名称+折叠+状态
├─────────────────────────────────────────┤
│ READS                                   │  ← 输入端口区（从 State 读取）
│  ○ messages        [Message[]]          │  ← 端口 + State 字段名 + 类型
│  ○ user_input      [string]             │
├─────────────────────────────────────────┤
│ WRITES                                  │  ← 输出端口区（写入 State）
│  ● response        [Message]            │
│  ● task_status     [string]             │
├─────────────────────────────────────────┤
│ InputMapper: {messages←messages}        │  ← Agent 节点特有：映射关系
│ OutputMapper: {response→response}       │
└─────────────────────────────────────────┘

折叠态：
┌──────────────────────────────────┐
│ [🤖] CustomerService   R2 / W2  │  ← R2=读2字段, W2=写2字段
│ ○ in              ● out          │  ← 简化端口（只保留首尾）
└──────────────────────────────────┘
```

**不同节点类型的端口显示**：

| 节点类型 | READS 端口 | WRITES 端口 | 特殊显示 |
|----------|-----------|-------------|----------|
| Function | NodeFunc 的输入参数 | NodeFunc 的返回字段 | funcRef 标签 |
| LLM | instruction 模板引用的 State 字段 | 生成结果写入的 State 字段 | model 名称 |
| Tool | Tool 输入参数引用的 State 字段 | Tool 输出写入的 State 字段 | toolNames 列表 |
| Agent | InputMapper 映射的 State 字段 | OutputMapper 映射的 State 字段 | agentName + 映射关系 |
| Router | CondFunc 读取的 State 字段 | （透传，不写入） | pathMap 分支标签 |
| Join | （无显式读取） | （透传，不写入） | 等待前驱数 |
| HITL | 审批输入字段 | 审批结果写入字段 | requiredRole + 状态 |

**连接验证逻辑**：

```typescript
// Aranea 的连接验证：基于 State 字段可达性
function isValidConnection(sourceNode, targetNode, stateSchema): ValidationResult {
  // 1. 下游的 READS 字段必须可达：
  //    - 在上游的 WRITES 中定义，或
  //    - 在 State Schema 中有 defaultValue
  const unreachable = targetNode.reads.filter(field =>
    !sourceNode.writes.includes(field) &&
    !stateSchema.hasDefault(field)
  )

  // 2. 同一 State 字段的 Reducer 兼容性
  //    - append reducer 的字段：多写不冲突
  //    - default reducer 的字段：只能有一个写入者
  const conflicts = checkReducerConflicts(sourceNode, targetNode, stateSchema)

  return { valid: unreachable.length === 0 && conflicts.length === 0, ... }
}
```

**改造范围**：
- 新增：`web/src/features/graph/portTypes.ts`（端口类型定义 + 连接验证）
- 重构：`web/src/components/graph/GraphFlowNode.vue`（State-Aware 节点）
- 修改：`web/src/components/graph/GraphEditorCanvas.vue`（isValidConnection 接入）
- 修改：`web/src/components/graph/GraphFlowEdge.vue`（边类型视觉增强）
- 修改：`web/src/features/graph/types.ts`（新增端口相关类型）

#### P0-2：节点状态指示 — 适配运行时事件

**目标**：将 trpc-agent-go 的图事件映射为节点视觉状态，运行时实时反馈。

**状态映射**：

| 运行时事件 | 节点视觉 | 说明 |
|-----------|---------|------|
| 无事件 | `idle` — 默认边框 | 未执行 |
| `graph_node_start` | `running` — 蓝色脉冲 + 旋转图标 | 正在执行 |
| `graph_node_end` | `success` — 绿色边框 + 勾号 | 执行完成 |
| `graph_node_error` | `error` — 红色边框 + 错误图标 | 执行失败 |
| `checkpoint.interrupt` | `interrupted` — 橙色边框 + 暂停图标 | HITL 等待审批 |
| `graph_node_custom` | `active` — 紫色高亮 | Command 动态路由激活 |

**边的状态**：

| 运行时事件 | 边视觉 | 说明 |
|-----------|--------|------|
| 节点完成 → 下一个节点开始 | 蓝色流动光点 | State 正在传递 |
| Conditional Edge 被选中 | 绿色高亮 + 标签 | 实际走的分支 |
| Conditional Edge 未被选中 | 灰色虚线 | 未走的分支 |
| Command Edge 动态激活 | 紫色流动光点 | Agent 运行时决定的路径 |

**改造范围**：
- 新增：`web/src/components/graph/GraphNodeStatusBadge.vue`（状态徽章组件）
- 修改：`web/src/components/graph/GraphFlowNode.vue`（集成状态徽章）
- 修改：`web/src/components/graph/GraphFlowEdge.vue`（边状态视觉）
- 修改：`web/src/features/graph/runtime/useGraphRunStream.ts`（事件→状态映射）

#### P0-3：内嵌 RunPanel — State 驱动的运行时面板

**目标**：在编辑器内嵌入轻量运行面板，实现"编辑→执行→调试"闭环，无需跳转页面。

**布局**：

```
┌────────┬──────────────────────┬──────────────────┐
│ 节点面板│      画布             │   RunPanel       │
│        │                      │ ┌──────────────┐ │
│        │   [节点状态高亮]       │ │ ▶ Run  ⏸ ⏹  │ │  ← 执行控制
│        │                      │ ├──────────────┤ │
│        │                      │ │ State View   │ │  ← 当前 State 快照
│        │                      │ │ messages: [...]│ │
│        │                      │ │ step: 3       │ │
│        │                      │ ├──────────────┤ │
│        │                      │ │ Checkpoints  │ │  ← 检查点导航
│        │                      │ │ ● Step 0     │ │
│        │                      │ │ ● Step 1     │ │
│        │                      │ │ ○ Step 2 ←  │ │  ← 当前位置
│        │                      │ │ ● Step 3     │ │
│        │                      │ ├──────────────┤ │
│        │                      │ │ HITL Queue   │ │  ← 待审批项
│        │                      │ │ ⚠ Node:Review │ │
│        │                      │ └──────────────┘ │
└────────┴──────────────────────┴──────────────────┘
```

**RunPanel 三个核心视图**：

1. **State View**：当前 State 的 JSON/表格视图，节点执行后实时 diff 高亮变化字段
2. **Checkpoint Navigator**：时间线视图，点击任意 Checkpoint 可 TimeTravel 回去
3. **HITL Queue**：待审批列表，点击弹出审批对话框（复用现有 GraphHitlDialog）

**与 GraphRunPage 的关系**：
- GraphRunPage 保留为全屏运行监控视图（含任务看板、执行历史）
- RunPanel 是编辑器内的轻量运行面板
- 两者共享 `useGraphExecute` 和 `useGraphRunStream` composable

**改造范围**：
- 新增：`web/src/components/graph/GraphRunPanel.vue`（RunPanel 主组件）
- 新增：`web/src/components/graph/GraphRunPanelChat.vue`（State 视图 + 执行控制）
- 新增：`web/src/features/graph/useGraphPlayground.ts`（Playground composable）
- 修改：`web/src/pages/GraphEditorPage.vue`（集成 RunPanel）
- 修改：`web/src/features/graph/useGraphEditorPage.ts`（RunPanel 状态管理）

---

### P1 — 体验打磨

#### P1-1：侧边栏重构 — 7 种节点类型分组

**目标**：按 Aranea 的 7 种节点类型分组，每组显示该类型可配置的关键属性。

```
┌─ 节点面板 ──────────────┐
│ 🔍 搜索节点...          │
├─────────────────────────┤
│ ▼ 🧠 智能体 (Agent)     │
│   Agent 节点            │  ← 拖拽到画布
│   (引用系统 Agent)       │
├─────────────────────────┤
│ ▼ 💬 语言模型 (LLM)     │
│   LLM 节点              │
│   (轻量 LLM 调用)       │
├─────────────────────────┤
│ ▼ 🔧 工具 (Tool)        │
│   Tool 节点             │
│   (确定性工具调用)       │
├─────────────────────────┤
│ ▼ ⚡ 函数 (Function)    │
│   Function 节点         │
│   (纯逻辑处理)          │
├─────────────────────────┤
│ ▼ 🔀 路由 (Router)      │
│   Router 节点           │
│   (条件分支)            │
│   Join 节点             │
│   (分支汇聚)            │
├─────────────────────────┤
│ ▼ 👤 人机协同 (HITL)    │
│   HITL 节点             │
│   (人工审批)            │
└─────────────────────────┘
```

**改造范围**：
- 重构：`web/src/components/graph/GraphNodePalette.vue`

#### P1-2：NodeToolbar — 节点级快捷操作

**目标**：悬停节点显示操作工具栏，适配 Aranea 业务操作。

```
悬停节点时显示：
┌──────────────────────────────────────┐
│ [▶ 运行到此] [📍 设为入口] [❄ 冻结]  │
│ [📋 复制]    [🗑 删除]    [⋯ 更多]   │
└──────────────────────────────────────┘

"更多"菜单：
├── HITL 配置（仅 HITL 节点）
├── 重试策略配置
├── 缓存策略配置
├── Fallback Agent 配置（仅 Agent 节点）
└── 查看运行时日志
```

**改造范围**：
- 新增：`web/src/components/graph/GraphNodeToolbar.vue`
- 修改：`web/src/components/graph/GraphFlowNode.vue`（集成 NodeToolbar）

#### P1-3：State Schema 编辑器增强

**目标**：State Schema 是 Graph 的核心数据模型，需要专门的编辑器，当前 GraphPropertyPanel 中的 Schema 编辑体验粗糙。

```
┌─ State Schema ─────────────────────────────┐
│ 字段名      类型        Reducer    默认值    │
│ ─────────────────────────────────────────── │
│ messages   Message[]   append     []        │
│ step       integer     default    0         │
│ context    object      merge      {}        │
│ approval   string      cover      ""        │
│ ─────────────────────────────────────────── │
│ [+ 添加字段]  [从 JSON 导入]                 │
└─────────────────────────────────────────────┘

Reducer 说明（悬停提示）：
- append: 追加到列表（消息历史、日志）
- default: 完全替换（当前步骤、状态标记）
- cover: 仅非零值覆盖（可选更新）
- merge: 深度合并（多节点并发写入）
```

**改造范围**：
- 修改：`web/src/components/graph/GraphPropertyPanel.vue`（Schema 编辑器重构）

---

### P2 — 高级功能

#### P2-1：子图嵌套编辑器

**目标**：Agent 节点双击进入其内部 Graph 编辑（如果该 Agent 是 GraphAgent）。

```
面包屑：Graph:OrderFlow > Agent:CustomerService > Graph:SubProcess

双击 Agent 节点 → 如果该 Agent 是 GraphAgent，进入其内部 Graph 编辑
                  → 如果该 Agent 是普通 Agent，打开 Agent 详情页
```

**改造范围**：
- 新增：`web/src/components/graph/GraphSubgraphBreadcrumb.vue`
- 新增：`web/src/features/graph/useGraphSubgraph.ts`
- 修改：`web/src/pages/GraphEditorPage.vue`（子图栈管理）
- 修改：`web/src/features/graph/useGraphEditorPage.ts`

#### P2-2：Inspection Panel — State Diff 视图

**目标**：选中已执行节点时，显示节点执行前后 State 的 diff。

```
┌─ State Diff: Agent:CustomerService ────────┐
│ Step 3 → Step 4                            │
├────────────────────────────────────────────┤
│ messages   [+2 items]                      │
│   before: [..., msg_5]                     │
│   after:  [..., msg_5, msg_6, msg_7]       │
├────────────────────────────────────────────┤
│ step       3 → 4                           │
├────────────────────────────────────────────┤
│ response   (new) "I can help you with..."  │
├────────────────────────────────────────────┤
│ task_status  "" → "claimed"                │
└────────────────────────────────────────────┘
```

**改造范围**：
- 新增：`web/src/components/graph/GraphStateDiffPanel.vue`
- 新增：`web/src/features/graph/useGraphStateDiff.ts`

---

## 七、文件变更预估

| 分期 | 新增文件 | 修改文件 | 预估代码量 |
|------|---------|---------|-----------|
| P0 | 5 个（portTypes.ts, GraphNodeStatusBadge.vue, GraphRunPanel.vue, GraphRunPanelChat.vue, useGraphPlayground.ts） | 7 个（GraphFlowNode.vue, GraphFlowEdge.vue, GraphEditorCanvas.vue, GraphEditorPage.vue, types.ts, useGraphEditorPage.ts, useGraphRunStream.ts） | ~2000 行 |
| P1 | 1 个（GraphNodeToolbar.vue） | 3 个（GraphNodePalette.vue, GraphFlowNode.vue, GraphPropertyPanel.vue） | ~1000 行 |
| P2 | 3 个（GraphSubgraphBreadcrumb.vue, useGraphSubgraph.ts, GraphStateDiffPanel.vue） | 3 个（GraphEditorPage.vue, useGraphEditorPage.ts, types.ts） | ~1500 行 |

---

## 八、关键风险与对策

| 风险 | 对策 |
|------|------|
| Vue Flow 的 Handle 自定义渲染能力不如 React Flow | Vue Flow 支持自定义 slot，端口类型标签可通过 slot 插入，实测可行 |
| 节点折叠后连线可能错位 | 折叠时保留输出端口 Handle 的位置，或使用 Vue Flow 的 `updateNodeInternals` 强制刷新 |
| 内嵌 RunPanel 与现有 GraphRunPage 逻辑重复 | 提取 `useGraphPlayground` composable，两个页面共享 |
| State 字段级连接验证与现有边类型（normal/transfer/dispatch/conditional）的兼容 | 端口验证是数据层（State 字段可达性），边类型是业务层分类，两者正交互不影响 |
| 子图嵌套的 undo/redo 栈管理复杂 | 每层子图独立 undo/redo 栈，退出子图时合并到父栈 |
| RunPanel 宽度挤压画布空间 | 面板可收起/展开，默认收起，运行时自动展开；宽度可拖拽调整 |

---

## 九、执行建议

1. **P0-1（节点重构）优先级最高** — 这是所有其他改进的基础，当前 GraphFlowNode 的固定高度、无端口标注是最大的体验瓶颈
2. **P0-2（状态指示）成本最低、收益最高** — 只需在现有 WS 事件流基础上增加视觉映射
3. **P0-3（RunPanel）业务价值最大** — 让用户在编辑器内完成"编辑→执行→调试"闭环
4. P0-1 和 P0-2 可以并行开发（节点重构和状态指示是相对独立的改动）
5. P0-3 依赖 P0-1 和 P0-2（RunPanel 需要新的节点组件和状态系统）
6. P1/P2 根据业务优先级排期，建议 P0 全部完成后再启动

---

## 十、参考资料

- [Langflow GitHub](https://github.com/langflow-ai/langflow)
- [Langflow 官方文档 - Visual Editor](https://docs.langflow.org/concepts-overview)
- [Langflow 1.8 Release Notes](https://www.langflow.org/blog/langflow-1-8)
- [Langflow GenericNode 深度分析](https://deepwiki.com/langflow-ai/langflow/5.3-genericnode-component)
- [Langflow Connection Validation](https://deepwiki.com/langflow-ai/langflow/5.4-connection-validation)
- [React Flow 官方文档](https://reactflow.dev/learn/customization/custom-nodes)
- [Vue Flow 官方文档](https://vue-flow.dev/)
