# Research: Langflow UI 复刻方案 — 设计思想适配与业务落地

> **日期**：2026-06-09
> **版本**：v1.9（可行性评估版 — Vue Flow/Quasar/后端 API 三维就绪度分析）
> **范围**：Graph 模块 UI 重构，基于 Langflow 设计思想的适配方案

---

## 摘要

本报告基于对 Langflow（GitHub 146K+ stars，v1.8/1.9）前端架构和 UI 设计的深度调研，结合 Aranea-Agents Graph 模块的业务领域模型，提出一套**以业务为中心的 UI 复刻方案**。核心结论：Langflow 的设计思想（类型化端口、节点折叠、状态指示、侧边栏 Playground、连接验证、Handle 点击过滤）值得复刻，但其具体实现（LangChain 显式枚举类型匹配、聊天式 Playground、Python 代码编辑器）不适配 Aranea 的业务模型。Aranea Graph 的本质是**确定性多 Agent 编排引擎**，数据流由 State Schema 驱动，节点间传递的是对共享 State 的读写而非 LangChain 数据类型。方案按 P0/P1/P2 三期规划，P0 聚焦节点重构（State-Aware 端口显示字段名）、5 种核心运行时状态、内嵌 RunPanel、资源分类选择器四项核心改造。关键取舍：端口显示字段名而非类型（§11.4 决策 1）、5 种状态而非 8 种（决策 2）、路径可达性降级为离线校验（决策 3）、MCP 通过 Agent 间接使用（决策 4）、节点折叠放宽约束（决策 5）。

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

> **源码验证**：以下描述基于对 `src/frontend/src/CustomNodes/GenericNode/index.tsx` 及其子组件的逐文件阅读，非文档推测。

Langflow 的 GenericNode **没有** NodeHeader/NodeContent/NodeFooter 的组件抽象，而是采用内联组合布局。这种选择是因为节点状态（折叠/展开/冻结/更新）会动态改变 DOM 结构，分层反而增加条件渲染复杂度。

```
GenericNode (React Flow 自定义节点, memo 包裹)
├── UpdateComponentModal           ← 组件版本更新弹窗
├── NodeToolbarComponent           ← 悬停/右键工具栏（绝对定位，节点上方）
│   ├── 代码编辑器按钮（Code icon, 仅 hasCode + allowCustomComponents, 自定义组件时 animate-pulse-pink）
│   ├── 高级控制按钮（SlidersHorizontal icon, 仅 nodeLength > 0 且 !inspectionPanelVisible）
│   ├── 冻结按钮（FreezeAll icon, 仅 !hasToolMode || inspectionPanelVisible, 冻结时 text-accent-indigo-foreground）
│   ├── Tool Mode 开关（Hammer icon + ToggleShad, 仅 hasToolMode 且 !isGroup）
│   └── 更多菜单（MoreHorizontal icon, Select 下拉）
├── [NodeUpdateComponent | NodeLegacyComponent]  ← 顶部横幅（组件过时/遗留时显示）
│   └── 横幅会将整个节点上移 mt-10 以腾出空间
├── Header 区域（始终可见，grid 布局，border-b 分隔线）
│   ├── 主行（flex, justify-between, px-4 py-3）
│   │   ├── 左侧（flex, items-center）
│   │   │   ├── NodeIcon（h-4.5 w-4.5, emoji 为 text-lg, 否则 Lucide icon）
│   │   │   └── NodeName（ml-3, flex-1, overflow-hidden）
│   │   │       ├── display_name（text-base font-medium truncate, cursor-grab）
│   │   │       ├── Legacy 徽章（text-xxs, border-accent-amber, rounded-sm, px-1）
│   │   │       └── Beta 徽章（h-4 w-4, FlaskConical icon, border-accent-purple）
│   │   ├── [折叠态时] 内联 RenderInputParameters + NodeOutputs（hidden 但挂载）
│   │   └── 右侧 NodeStatus（shrink-0, flex, gap-2）
│   │       ├── BuildStatusDisplay（成功时: font-mono text-xs, Token 用量 + 时长）
│   │       │   └── Coins icon (h-3 w-3) + token 数 + "|" + 时长
│   │       ├── Auth 连接按钮（h-4 w-4, rounded-sm, OAuth 状态指示）
│   │       │   ├── Loader2 (polling, animate-spin) / Link (authenticated) / AlertTriangle (error)
│   │       │   └── hover 时 Unlink icon 渐显（authenticated 状态）
│   │       └── Run 按钮（h-3.5 w-3.5, Play/Loader2/Square 三态）
│   │           ├── 默认: Play icon, text-muted-foreground
│   │           ├── 构建中: Loader2, animate-spin
│   │           └── 构建中+hover: Square (停止), text-status-red
│   ├── 编辑按钮（absolute, left-1/2, z-50, h-6 w-6 rounded-md）
│   │   ├── 展开态: top-2 translate-x-[10.4rem]
│   │   ├── 折叠态: top-0 translate-x-[6.4rem]
│   │   ├── 默认: bg-zinc-foreground, PencilLine icon
│   │   └── 编辑态: bg-accent-emerald, Check icon
│   └── [展开态时] NodeDescription（px-4 pb-3, text-xs, Markdown 渲染, 1000 字符上限）
├── Content 区域（仅 showNode=true 时渲染，nopan nodelete nodrag 容器）
│   ├── RenderInputParameters
│   │   └── NodeInputField × N（过滤 _前缀/hidden/advanced 字段，按 field_order 排序）
│   │       ├── HandleRenderComponent（左侧 target Handle）
│   │       ├── 字段标题行（名称 + 必填* + 信息提示 + 自定义标签）
│   │       └── ParameterRenderComponent（内联编辑器：文本/下拉/代码/文件/开关等）
│   ├── NodeOutputs (shown)
│   │   └── OutputParameter × N
│   │       ├── HandleRenderComponent（右侧 source Handle）
│   │       ├── [循环输出] 额外左侧 loop input Handle
│   │       ├── OutputComponent（输出名/选择器）
│   │       ├── 冻结指示（Snowflake icon，text-ice 样式）
│   │       ├── 检查按钮（TextSearchIcon → OutputModal）
│   │       └── Looping/Infinity 徽章
│   └── NodeOutputs (hidden) + _HiddenOutputsButton（展开/收起隐藏输出）
└── （无 NodeFooter — 错误信息和计时器在 NodeStatus 组件内）
```

**折叠机制**（源码关键约束）：

折叠由 `showNode` 布尔值控制，但**不是所有节点都能折叠**。Langflow 有严格的前置条件：

```
isMinimal = (hasSelectOutput || hasOnlyOneOutput) && hasMaximumOneConnectedInput
```

- 不满足 `isMinimal` 的节点**不能折叠**，否则 Handle 会重叠
- 折叠时宽度从 `w-80`(320px) 缩到 `w-48`(192px)
- 折叠时参数组件仍挂载（hidden），因为 React hooks 需要执行
- 如果节点已折叠但变为非 minimal（如新增连接），自动展开
- 折叠态只渲染 primary input Handle 的圆点 + 输出 Handle 圆点

### 2.2 整体页面布局

> **源码验证**：以下描述基于对 `pages/FlowPage/index.tsx`、`PageComponent/index.tsx`、`flowSidebarComponent/index.tsx` 的逐文件阅读。

```
FlowPage (三层 Sidebar 嵌套)
├── SimpleSidebarProvider (右侧 Playground 面板)
│   ├── SidebarProvider (左侧组件面板)
│   │   ├── SidebarSegmentedNav (左侧 40px 图标导航条) ← 新版侧边栏
│   │   │   ├── 🔍 Search
│   │   │   ├── 🧩 Components
│   │   │   ├── 🔗 MCP
│   │   │   ├── 📦 Bundles
│   │   │   ├── 📜 Versions
│   │   │   └── 📊 Traces
│   │   └── SidebarContent (17.5rem / 280px, 可折叠)
│   │       ├── 搜索框 (Fuse.js 模糊搜索, 300ms 防抖)
│   │       ├── 分类折叠组 (CategoryDisclosure)
│   │       │   └── 每组：图标 + 名称 + 左色条 + 可拖拽卡片
│   │       └── 底部按钮 (新建自定义组件 / MCP 管理)
│   ├── Main Canvas Area (flex:1, 圆角 m-2)
│   │   └── ReactFlow
│   │       ├── GenericNode × N
│   │       ├── CustomEdge × N
│   │       ├── Background (2px 点阵, 20px 间距)
│   │       ├── FlowToolbar (Panel top-right, h-11)
│   │       │   ├── Playground 按钮 (Mod+K)
│   │       │   ├── Publish 下拉
│   │       │   └── Deploy 按钮
│   │       ├── CanvasControls (Panel bottom-center)
│   │       │   ├── AI 助手按钮（含 "New" 粉色徽章）
│   │       │   ├── 缩放控制 (25%-200%, 下拉菜单)
│   │       │   ├── 便签按钮
│   │       │   ├── 帮助下拉 (辅助线开关/Inspector 开关/文档/报告 Bug)
│   │       │   └── Inspector 切换按钮
│   │       ├── InspectionPanel (Panel top-right, 320px, 选中节点时)
│   │       └── SelectionMenu (多选时 Group 按钮)
│   └── SimpleSidebar (右侧 Playground, 默认 326px, 可拖拽 15%-60%)
│       ├── ChatSidebar (全屏时显示, 236px)
│       ├── ChatHeader (会话选择 + 全屏 + 关闭)
│       ├── Messages (StickToBottom 自动滚动)
│       └── ChatInput (文本 + 文件 + 语音)
└── (无 StatusBar — 底部无状态栏)
```

**关键布局参数**：

| 区域 | 宽度 | 行为 |
|------|------|------|
| 左侧导航条 | 40px | 始终可见，图标导航 |
| 左侧面板 | 17.5rem (280px) | 可折叠，状态持久化到 cookie |
| 画布 | flex:1 | 圆角 inset (m-2)，Playground 打开时右侧无 margin |
| 右侧 Playground | 326px 默认 | 可拖拽调整 (15%-60% viewport)，支持全屏 |
| InspectionPanel | 320px | 画布内浮层，Framer Motion 滑入 |
| 节点展开 | w-80 (320px) | rounded-xl, shadow-sm |
| 节点折叠 | w-48 (192px) | rounded-xl |

**FlowToolbar 按钮**（右上角 Panel, h-11, rounded-md, bg-background）：

| 按钮 | 图标 | 标签 | 快捷键 | 条件 |
|------|------|------|--------|------|
| Playground | Play | "Playground" | Mod+K | hasIO 时可用，否则 disabled + tooltip |
| Share | ChevronDown | "Share" | — | 下拉菜单：API access / Export / MCP Server / Embed / Shareable Playground |
| Deploy | Rocket | "Deploy" | — | 仅 wxo_deployments 功能标志启用 |

**CanvasControls 按钮**（底部居中 Panel, rounded-lg, bg-background, px-2 py-1）：

| 按钮 | 图标 | 功能 | 快捷键 | 备注 |
|------|------|------|--------|------|
| AI 助手 | 彩色 SVG（hover 渐显） | 切换助手侧边栏 | A | 含粉色 "New" 徽章（Sparkles icon + "New" text, bg-pink-600） |
| 缩放控制 | 百分比 + ChevronDown | 放大/缩小/100%/适应 | Ctrl+/-/0/1 | 下拉菜单 side="top"，fitView 右侧 padding 340px |
| 便签 | sticky-note (18px) | 进入添加便签模式 | — | 激活时 icon 变 text-foreground，280×140px 阴影框跟随光标 |
| 帮助 | Circle-Help | 辅助线开关/Inspector 开关/文档/报告 Bug | — | 下拉菜单 side="top"，含 Smart Guides 开关 |
| Inspector | PanelRight / PanelRightClose | 切换 InspectorPanel | — | 仅 ENABLE_INSPECTION_PANEL 功能标志启用时显示 |

> **注意**：Langflow 的 CanvasControls **没有独立的锁定按钮**。画布锁定由 `effectiveLocked = isLocked || isAgentWorking` 驱动，锁定时通过 ReactFlow store API 禁用 `nodesDraggable/nodesConnectable/elementsSelectable`，并在画布左上角显示 Agent 工作横幅。

**键盘快捷键系统**（对齐 Langflow shortcuts store）：

| 快捷键 | 功能 | 作用域 |
|--------|------|--------|
| Mod+Z | 撤销 | 画布 |
| Mod+Y / Mod+Shift+Z | 重做 | 画布 |
| Mod+C | 复制 | 画布（noflow 区域跳过） |
| Mod+X | 剪切 | 画布 |
| Mod+V | 粘贴 | 画布 |
| Mod+D | 复制节点 | 画布 |
| Backspace / Delete | 删除 | 画布（nodelete 区域跳过） |
| Mod+G | 分组 | 画布（多选时） |
| Mod+K | 切换 Playground | 全局 |
| Mod+B | 切换侧边栏 | 全局 |
| Mod+S | 保存流程 | 全局 |
| / | 聚焦搜索 | 侧边栏 |
| P | 运行组件 | 节点 |
| Space | 代码编辑器 | 节点 |
| O | 输出检查 | 节点 |
| Mod+. | 折叠/展开 | 节点 |
| Mod+Shift+F | 冻结路径 | 节点 |
| Escape | 取消/关闭 | 全局 |

### 2.3 连接验证架构

> **源码验证**：以下描述基于对 `src/frontend/src/utils/reactflowUtils.ts` 中 `isValidConnection`、`typeIsCompatibleWith`、`typesAreCompatible` 函数的逐行阅读。

**Langflow 没有前端类型层级**。类型系统是**字符串匹配 + 显式枚举**，不是继承树：

**Handle 类型数据结构**：

```typescript
// 输出 Handle（右侧，source）
sourceHandleType = {
  dataType: string;          // 组件类型（如 "ChatOpenAI", "AstraDB"）
  id: string;                // 节点 ID
  name: string;              // 输出显示名
  output_types: string[];    // 产出的类型（如 ["Message"], ["JSON", "Table"]）
  conditionalPath?: string;  // 条件路径标识
}

// 输入 Handle（左侧，target）
targetHandleType = {
  type: string;              // 字段声明类型（如 "model", "str"）
  fieldName: string;         // 模板字段名
  id: string;                // 节点 ID
  inputTypes?: string[];     // 可接受的输入类型（如 ["LanguageModel", "Embeddings"]）
  output_types?: string[];   // 仅循环节点输入时存在
  proxy?: { field: string; id: string };  // 组节点代理
}
```

**验证规则**（`isValidConnection` 的实际逻辑，6 步 OR 链）：

1. **自连接检查**：`source === target` 直接拒绝
2. **source.dataType ∈ target.inputTypes**：组件类型匹配输入类型列表
3. **循环输入类型检查**：source 的 output_types 与 target 的 output_types 匹配（仅循环节点）
4. **source.output_types ∩ target.inputTypes ≠ ∅**：输出类型与输入类型有交集
5. **source.dataType ∈ [target.type]**：组件类型匹配字段声明类型
6. **∃ source.output_type ∈ [target.type]**：任一输出类型匹配字段声明类型

**关键事实**：
- **没有子类型关系**：`HumanMessage` 不能自动连接到 `Message` 输入，除非 `input_types` 显式包含 `"Message"`
- **没有 Any 根类型**：没有"Any 接受所有"的规则
- **类型迁移**：仅 `Data → JSON`、`DataFrame → Table` 两个向后兼容映射
- **连接槽位**：`list: true` 的输入字段允许多连接，否则只允许一个
- **循环例外**：DFS 检测有向环，但如果环路径中存在 loop input Handle（target 有 `output_types`），循环被允许

**Handle 点击过滤**（文档常见遗漏）：

Langflow 有一个强大的交互模式：点击任意 Handle 设置全局 `filterType`，画布上所有兼容 Handle 显示霓虹脉冲动画（`pulseNeon` keyframes），不兼容 Handle 变灰（null handle）。再次点击兼容 Handle 可自动连线。这是快速发现可连接目标的关键交互。

**视觉反馈**：
- **兼容 Handle**：霓虹脉冲动画 + 扩展 box-shadow（handle 颜色）
- **不兼容 Handle**：灰色边框圆点（null handle），无发光
- **Model 类型未连接**：6px 半透明圆点（muted），减少视觉噪音
- **连接线**：颜色继承 source Handle 的类型颜色（`nodeColorsName` 映射）
- **Tooltip**：显示类型徽章 + 兼容性信息 + "Drag to connect / Click to filter"

### 2.4 Playground 集成

> **源码验证**：以下描述基于对 `src/frontend/src/pages/FlowPage/index.tsx`、`playgroundStore.ts`、`flowStore.ts`、`message-event-handler.ts` 的逐文件阅读。

**Langflow 没有三种 Playground 模式**。实际是两种独立 UI + 双消息存储架构：

**1. 编辑器内 Playground（SimpleSidebar 侧边栏）**：

在 Flow 编辑器页面，Playground 是右侧可拖拽调整宽度的 `SimpleSidebar`（默认 326px），与 InspectionPanel **共存**而非互斥：
- InspectionPanel：ReactFlow 画布内的 `Panel position="top-right"` 浮层，编辑节点参数
- Playground 侧边栏：画布外部的独立面板，聊天交互
- 两者可同时显示，互不干扰

Playground 侧边栏内容：
- `ChatSidebar`：会话列表（创建/删除/重命名）
- `ChatHeader`：会话选择器 + 全屏切换 + 关闭
- `Messages` 组件：聊天消息列表（流式更新）
- `ChatInput`：输入区（文件上传 + 语音 + 文本）

**2. 独立 Playground 页面（公开分享页）**：

`/playground/:flowId` 路径，加载流程后以 `CustomIOModal`（always-open 模式）渲染纯聊天界面，无画布。

**3. 不存在的"内嵌模式"**：

ChatInput/ChatOutput 节点**没有**内嵌迷你聊天界面。实际交互是：用户在右侧 Playground 面板输入 → ChatInput 节点接收 → 流程执行 → ChatOutput 节点输出 → Playground 面板显示结果。

**双消息存储**（关键架构细节）：

Langflow 维护两套消息存储，通过 `message-event-handler.ts` 同步：
1. **React Query cache**（`useChatHistory` hook）：编辑器内 Playground 使用，支持细粒度流式更新
2. **Zustand `useMessagesStore`**：独立 Playground 页面使用，支持 `addMessage/updateMessage` 操作

每个 `add_message` / `token` 事件同时更新两套存储，确保数据一致。

**执行触发与事件流**：

```
用户发送消息 / 点击 Run
  → flowStore.buildFlow({ input_value, startNodeId, session })
    → buildFlowVerticesWithFallback() → HTTP POST (streaming/polling)
      → SSE 事件流：
        vertices_sorted → 设置节点执行计划 + TO_BUILD 状态
        build_start     → BUILDING 状态
        end_vertex      → BUILT/ERROR 状态 + flowPool 更新 + 边动画
        add_message     → 双消息存储同步更新
        token           → 流式 token 累积
        build_end       → 所有节点 BUILT
        end             → isBuilding = false
```

### 2.5 v1.8 新增特性

| 特性 | 说明 |
|------|------|
| **Traces** | 流级 + 组件级执行轨迹，可视化延迟和 Token 用量 |
| **Inspection Panel** | 实时查看组件内部状态、输入/输出、日志、Token |
| **全局模型提供商** | 集中配置 LLM 凭证，组件引用而非各自配置 |
| **V2 Workflow API** | 更简洁的响应格式 + 异步后台任务支持 |
| **Knowledge Bases** | 本地向量数据库，数据不需每次从远程重新摄取 |

### 2.6 源码验证发现的额外设计亮点

> 以下特性在 Langflow 官方文档中未重点提及，但通过源码阅读发现对 Aranea 有重要参考价值。

**1. Handle 点击过滤系统**（`handleRenderComponent/index.tsx`）

点击任意 Handle 设置全局 `filterType`，画布上所有兼容 Handle 显示霓虹脉冲动画（`pulseNeon` keyframes），不兼容 Handle 变灰（null handle）。再次点击兼容 Handle 可自动连线。这对 Aranea 的 State 字段级连接引导极其有用——点击"写入 messages"Handle，所有"读取 messages"Handle 高亮。

**2. 组件版本管理**（`NodeUpdateComponent` / `NodeLegacyComponent`）

当后端组件定义变更时，前端节点显示"需要更新"横幅（amber/green/warning 色点 + 标签 + 更新/忽略按钮）。Aranea 的 Graph 定义与后端 Registry 函数紧密绑定，当 Registry 函数签名变更时需要类似的版本提示机制。

**3. CycleEdge 循环支持**（`lfx/graph/edge/base.py`）

Langflow 支持循环节点（For Loop），通过 `CycleEdge` 实现——循环边有 `is_fulfilled` 契约机制，第一次执行时跳过循环边，后续迭代通过 `honor()` 传递结果。Aranea 的 Router 节点可能产生循环（条件路由回到之前的节点），需要类似的循环边视觉和交互设计。

**4. 双消息存储架构**（`message-event-handler.ts`）

Langflow 维护 React Query cache + Zustand store 两套消息存储，通过统一事件处理器同步。这解决了不同 UI 组件的响应式需求差异——编辑器 Playground 需要细粒度流式更新，独立 Playground 页面需要批量操作。Aranea 的 RunPanel 和 GraphRunPage 可能需要类似的差异化响应式策略。

**5. 边清理机制**（`cleanEdges` in `reactflowUtils.ts`）

加载流程时，`cleanEdges` 验证每条已有边：检查节点是否存在、Handle 是否匹配（含类型迁移容忍）、隐藏字段边是否应移除。这确保流程定义与当前组件版本一致。Aranea 在 Graph 定义加载时也需要类似的边完整性校验。

### 2.7 设计系统与样式架构

> **源码验证**：以下描述基于对 `style/index.css`、`style/applies.css`、`style/classes.css`、`tailwind.config.mjs`、`utils/styleUtils.ts` 的逐文件阅读。

**CSS 架构**（三层分离）：

| 层 | 文件 | 职责 |
|----|------|------|
| Design Tokens | `style/index.css` | CSS 变量定义（`:root` / `.dark`），HSL 通道值存储 |
| Component Classes | `style/applies.css` | `@layer components`，~150 个 `@apply` 组合类 |
| Global/Overrides | `style/classes.css` | `@tailwind` 指令、全局样式、keyframe 动画、ReactFlow 覆盖 |

**颜色系统**（HSL 通道值架构）：

所有颜色以 HSL 通道值存储（如 `240 6% 10%`），通过 `hsl(var(--token))` 消费。暗色模式通过 `.dark` 选择器重定义所有变量实现反转。

| 语义层 | Light | Dark | 用途 |
|--------|-------|------|------|
| `--background` | 白色 | 深蓝灰 | 卡片/面板背景 |
| `--canvas` | 浅灰 `240 5% 96%` | 纯黑 | 画布背景 |
| `--border` | `240 6% 90%` | `240 5% 26%` | 边框 |
| `--muted` | `240 5% 96%` | `240 4% 16%` | 次要背景 |
| `--accent-indigo` | 蓝紫 | 亮蓝紫 | 构建状态 |
| `--accent-emerald` | 绿色 | 亮绿 | 成功状态 |
| `--accent-amber` | 琥珀 | 亮琥珀 | 警告状态 |
| `--status-red/yellow/green/blue` | 硬编码 hex | 同 | 状态指示器 |

**数据类型颜色系统**（12 对，Handle/Edge 着色）：

| 类型 | Tailwind 名 | 典型用途 |
|------|------------|---------|
| Message | indigo | 消息流 |
| JSON | red | 数据 |
| Table | pink | 表格 |
| LanguageModel | fuchsia | 模型 |
| Embeddings | emerald | 嵌入 |
| Tool | cyan | 工具 |
| Agent | purple | 智能体 |

每对有 `datatype-{name}`（背景色）和 `datatype-{name}-foreground`（前景色），暗色模式自动反转。

**节点视觉规范**：

| 属性 | 值 | 说明 |
|------|-----|------|
| 圆角 | `rounded-xl` (12px) | 统一圆角 |
| 阴影 | `shadow-sm` / `hover:shadow-md` | 微阴影，hover 增强 |
| 边框 | `border ring-[0.5px] ring-border` | 0.5px ring + 1px border |
| 选中边框 | `ring-[0.75px] ring-muted-foreground` | 加粗 ring |
| 构建中边框 | `border-foreground border-[1px]` | 前景色 1px 实线 |
| 错误边框 | `border-destructive border-[1px]` | 红色 1px 实线 |
| 冻结边框 | `shadow-frozen-ring` + 毛玻璃伪元素 | 蓝色发光 + blur(5px) |
| Header 间距 | `px-4 py-3` | 16px/12px |
| 图标尺寸 | `h-4.5 w-4.5` (18px) | NodeIcon |
| 名称字号 | `text-base font-medium` | 16px 中等字重 |

**Handle 视觉规范**：

| 属性 | 值 | 说明 |
|------|-----|------|
| 可见圆点 | 10px | 彩色圆形 |
| 隐形点击区 | 32×32px | 透明，绝对定位 |
| Muted 圆点 | 6px, opacity:0 | 未连接的 model 端口 |
| Null Handle | 灰色边框圆点 | 不兼容时 |
| 霓虹脉冲 | `pulseNeon` 1.1s ease-in-out infinite | 兼容 Handle 的呼吸发光 |
| 霓虹发光范围 | 2px → 30px box-shadow | 7 层递增 shadow |
| 连接线颜色 | `hsl(var(--datatype-{name}))` | 继承 Handle 类型色 |

**动画规范**：

| 动画 | 时长 | 缓动 | 用途 |
|------|------|------|------|
| 霓虹脉冲 | 1.1s | ease-in-out infinite | Handle 兼容指示 |
| 状态 wiggle | 150ms | ease-in-out | 节点状态变化 |
| 边过渡 | 150ms | — | 边颜色/粗细变化 |
| 对话框进入 | 400ms | cubic-bezier(0.16,1,0.3,1) | Dialog 弹出 |
| InspectionPanel | 即时 | easeInOut | 右侧滑入 |
| Playground 面板 | 300ms | ease-in-out | 宽度变化 |

**Aranea 适配要点**：

1. **Quasar 不支持 HSL 通道值架构**：Quasar 有自己的颜色系统，Aranea 需要定义等价的 CSS 变量体系，或在 Quasar 主题中映射
2. **Vue Flow Handle 样式**：Vue Flow 的 Handle 渲染机制与 React Flow 不同，霓虹脉冲动画需要用 Vue 的 `<Transition>` 或 CSS 动画实现
3. **7 种节点类型的颜色映射**：Langflow 用 12 对 datatype 颜色，Aranea 只需 7 种节点类型颜色 + State 字段类型颜色（string/integer/float/boolean/array/object）

### 2.8 边渲染与连接交互

> **源码验证**：以下描述基于对 `CustomEdges/index.tsx`、`ConnectionLineComponent/index.tsx`、`PageComponent/index.tsx` 的逐文件阅读。

**边路径计算**（两种贝塞尔曲线）：

| 边类型 | 路径算法 | 视觉 |
|--------|---------|------|
| 普通边 | React Flow `getBezierPath()` | 实线，2px |
| 循环边（target 有 `output_types`） | 手工三次贝塞尔，控制点偏移 200px+ | **虚线** `strokeDasharray="5 5"`，2px |

关键细节：
- 源点 X 偏移到 `sourceNode.width + 7`（节点右边缘 +7px）
- 目标点 X 偏移到 `targetNode.position.x - 7`（节点左边缘 -7px）
- 循环边的控制点距离动态计算：`distance = 200 + 0.1 * (horizontalGap / 2)`

**连接线（拖拽中）**：

| 属性 | 值 |
|------|-----|
| 路径 | 固定贝塞尔 `M{fromX},{fromY} C {fromX} {toY} {fromX} {toY} {toX},{toY}` |
| 线宽 | 2px |
| 颜色 | `hsl(var(--datatype-{handleColor}))` |
| 动画 | React Flow 内置 marching-ants（`className="animated"`） |
| 端点圆 | 白色填充，5px 半径，Handle 类型色描边 1.5px |

**边状态视觉**：

| 状态 | 描边色 | 线宽 | 动画 |
|------|--------|------|------|
| 默认 | `var(--connection)` (#555/#6d6c6c) | 2px | 无 |
| 选中 | `var(--selected)`（动态设为 Handle 类型色） | 2px | 无 |
| 运行中 | `hsl(var(--foreground))` | 2px | marching-ants |
| 非运行中 | `hsl(var(--foreground))` | 1px | 无 |
| 已运行 | `hsl(var(--foreground))` | 2px | 无 |

**选中边颜色**：点击边时，`--selected` CSS 变量被动态设为源 Handle 输出类型对应的 `datatype-*` 颜色，而非固定蓝色。

**边重连**：拖拽边端点到新 Handle 时验证连接有效性。**无效重连会删除边**（不是恢复原位），这是 Langflow 的设计决策。

**边右键菜单**：只有"删除"一个选项，使用 20px 宽的隐形交互路径作为点击区。

**辅助对齐线**：

| 属性 | 值 |
|------|-----|
| 颜色 | `hsl(var(--primary))` |
| 线宽 | 1px |
| 虚线模式 | `4 4` (4px dash, 4px gap) |
| 透明度 | 0.8 |
| 发光 | `drop-shadow(0 0 2px hsl(var(--primary) / 0.3))` |
| 吸附距离 | 5px |

### 2.9 画布级交互功能

> **源码验证**：以下描述基于对 `CanvasControls.tsx`、`FlowBuildingComponent/index.tsx`、`NoteNode/index.tsx`、`flowsManagerStore.ts` 的逐文件阅读。

**画布控制栏**（底部居中，`rounded-lg bg-background px-2 py-1`）：

| 按钮 | 图标 | 功能 |
|------|------|------|
| AI 助手 | 彩色 SVG（hover 渐显）+ 粉色 "New" 徽章 | 切换助手侧边栏 |
| 缩放控制 | 当前百分比 + 下拉 | 放大/缩小/100%/适应（25%-200%） |
| 便签 | `sticky-note` (18px) | 进入添加便签模式，点击画布放置 280×140px 阴影框 |
| 帮助 | `circle-help` | 辅助线开关/Inspector 开关/文档/报告 Bug（下拉菜单 side="top"） |
| Inspector | `PanelRight` / `PanelRightClose` | 切换 InspectorPanel（仅 ENABLE_INSPECTION_PANEL 时显示） |

**便签节点**（NoteNode）：

| 属性 | 值 |
|------|-----|
| 最小尺寸 | 280×140px |
| 可调整大小 | 是（NodeResizer，选中时显示） |
| 颜色 | 6 预设（amber/neutral/rose/blue/lime/transparent）+ 自定义取色器 |
| 文字对比度 | WCAG 亮度公式自动切换黑/白文字 |
| Markdown | 支持，2500 字符上限 |
| 占位文字 | "Double-click to start typing or enter Markdown..." |

**构建进度指示器**（FlowBuildingComponent）：

| 状态 | 视觉 | 位置 |
|------|------|------|
| 构建中 | `TextShimmer` 动画 + 计时器 + Stop 按钮 | 底部居中, 530px 宽 |
| 成功 | 绿色边框 + "Flow built successfully" + 2s 自动消失 | 同 |
| 失败 | 红色边框 + 错误 Markdown + Retry/Dismiss 按钮 | 同 |

**撤销/重做**：

| 属性 | 值 |
|------|-----|
| 最大历史 | 100 条 |
| 快照内容 | `{ nodes, edges }` 深拷贝 |
| 触发时机 | 连接/拖拽/删除/粘贴/剪切/分组/放置/更新组件前 |
| 去重 | 连续相同状态不重复入栈 |
| 禁用条件 | 版本预览模式 / 画布锁定 |

**自动保存**：

| 属性 | 值 |
|------|-----|
| 防抖 | 300ms |
| 触发 | 节点拖拽结束 / 任何 flowStore 变更 |
| 去重 | `customStringify` 比较当前 vs 已保存 |
| 持久化 | `nodes + edges + viewport` |

**画布锁定**（`effectiveLocked = isLocked || isAgentWorking`）：

锁定时通过 ReactFlow store API 禁用：`nodesDraggable=false`, `nodesConnectable=false`, `elementsSelectable=false`, `nodesFocusable=false`, `edgesFocusable=false`。同时禁用：`onConnect`, `onReconnect*`, `onDragOver/Drop`, 所有键盘快捷键（undo/redo/copy/paste/delete/group/duplicate），右键菜单，边点击。保留：缩放/平移/控制栏。

当 `isAgentWorking` 时，画布左上角显示 Agent 工作横幅（CanvasBadge）：旋转 Loader2 icon + "Agent is working on this flow..."，退出动画 350ms 延迟。

**版本预览模式**：

只读覆盖层，禁用所有编辑操作，保留缩放/平移。顶部显示紫色标签 + "(Read-Only)"，底部提供"保存快照"或"恢复版本"操作。

### 2.10 节点内部组件

> **源码验证**：以下描述基于对 `ParameterRenderComponent`、`OutputComponent`、`HandleTooltipComponent`、`NodeName`、`NodeDescription` 的逐文件阅读。

**参数编辑器类型**（20 种）：

| 类型 | 编辑器 | 说明 |
|------|--------|------|
| str (无 options) | InputGlobalComponent | 内联文本 + 全局变量选择 |
| str (multiline) | TextAreaComponent | 多行文本 |
| str (有 options) | DropdownComponent | 下拉选择 |
| bool | ToggleShadComponent | 开关 |
| int/float | IntComponent/FloatComponent | 数字输入 + 范围 |
| code | CodeAreaComponent | 代码编辑器 |
| prompt | AccordionPromptComponent | 提示词 + `{变量}` 高亮 |
| mustache | MustachePromptAreaComponent | 提示词 + `{{变量}}` 高亮 |
| dict | KeypairListComponent | 键值对列表 |
| NestedDict | DictComponent | 嵌套字典 |
| file | CustomInputFileComponent | 文件上传 |
| table | TableNodeComponent | 表格编辑 |
| slider | SliderComponent | 滑块 + min/max |
| tools | ToolsComponent | 工具选择 |
| sortableList | SortableListComponent | 拖拽排序列表 |
| tab | TabComponent | 标签选择 |
| connect | CustomConnectionComponent | 连接选择 |
| query | QueryComponent | 查询构建 |
| mcp | McpComponent | MCP 服务器选择 |
| model | ModelInputComponent | 模型选择下拉 |

**Aranea 适配**：Aranea 的节点参数远少于 Langflow（Agent 节点只有 instruction/inputMapper/outputMapper 等），不需要 20 种编辑器。需要的是：
- 文本编辑器（instruction/description）
- JSON 编辑器（inputMapper/outputMapper）
- 下拉选择（agentName/funcRef/toolName）
- 开关（frozen/enabled）
- 键值对编辑器（State 字段映射）

**输出选择器**（OutputComponent）：

当节点有多个输出时，显示 Popover + Command 下拉选择器（`min-w-[200px] max-w-[250px]`）。单输出时只显示 `<span>` 文本。

**Handle Tooltip**（悬停 1000ms 后显示）：

| 状态 | 内容 |
|------|------|
| 空闲 | 类型徽章 + "Drag to connect / Click to filter" |
| 连接中 + 兼容 | "Connect to" + 类型徽章 |
| 连接中 + 不兼容 | "Incompatible with" + 类型徽章 |
| 同一节点 | "Can't connect to the same node" |

**内联编辑**：

- 名称：点击铅笔图标（24×24 圆形），Enter 保存，Escape 取消
- 描述：点击铅笔图标，Textarea 编辑，1000 字符上限，接近上限时显示计数器

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

### 3.11 Agent/Tool/MCP 选择与项目对齐分析

> **核心问题**：Langflow 的组件卡片直接对应 Python 类（ChatOpenAI、AstraDB 等），拖到画布即实例化。Aranea 的节点是**引用型**——Agent 节点通过 `agent_name` 引用系统 Agent，Tool 节点通过 `tool_names` 引用工具目录，Function 节点通过 `func_ref` 引用注册函数。这种引用型架构决定了 UI 选择器的设计方式与 Langflow 完全不同。

#### 3.11.1 Agent 选择：当前是纯文本输入，需升级为选择器

**现状**：

| 维度 | 当前状态 | Langflow 对标 |
|------|---------|--------------|
| 选择方式 | `q-input` 纯文本输入 `agentName` | 下拉选择器 + 搜索 |
| 数据源 | 无（用户手动输入 agent_key） | 组件目录（所有可用组件） |
| 验证 | 无（输入错误 key 运行时才报错） | 拖拽时即验证类型兼容性 |
| Agent 信息 | 不显示（用户不知道有哪些 Agent 可选） | 显示描述、类型、Beta/Legacy 标记 |

**后端已具备的能力**：

- `AgentUsecase` 提供 `ListAgents()` / `GetAgentByAgentKey()` 完整 CRUD
- `CatalogAgentResolver` 支持按 ID / agent_key / 数据库 ID 三种方式解析
- `AgentKind` 区分 `llm` / `a2a_proxy` 两种类型
- `Agent.Kind` 区分 `user` / `system_builtin` / `ecosystem_preset` / `marketplace` / `certified`
- 前端已有 `useAgentsCatalogStore`（轻量级下拉数据源）

**需要升级为**：

```
Agent 选择器（借鉴 Langflow ModelInputComponent）：
┌─────────────────────────────┐
│ 🔍 搜索 Agent...            │  ← Fuse.js 模糊搜索
├─────────────────────────────┤
│ ▼ 🤖 自建 Agent             │  ← Kind=user
│   CustomerService           │
│   DataAnalyzer              │
├─────────────────────────────┤
│ ▼ 🏗️ 系统 Agent             │  ← Kind=system_builtin
│   DefaultAssistant          │
│   CodeReviewer              │
├─────────────────────────────┤
│ ▼ 🌐 A2A Proxy              │  ← AgentKind=a2a_proxy
│   ExternalService           │
└─────────────────────────────┘
```

#### 3.11.2 Tool 选择：已有下拉但缺少分类和 MCP 工具

**现状**：

| 维度 | 当前状态 | Langflow 对标 |
|------|---------|--------------|
| 选择方式 | `q-select` 多选下拉 | 分类卡片 + 搜索 |
| 数据源 | `toolsStore.loadTools({ page_size: 200 })` → `tool.key[]` | 组件目录（按类型分组） |
| 分类 | 无（平铺列表） | 按 Category 分组（inputs/outputs/models/tools/...） |
| 原生工具 | 不包含（`datetime`/`web_search` 等不在 catalog 中） | 所有组件都可见 |
| MCP 工具 | 不包含（MCP 工具通过 Agent 间接使用） | MCP 组件类型 |
| 工具信息 | 只显示 key | 显示描述、类型、风险等级 |

**后端已具备的能力**：

- `ToolUsecase` 提供 `ListTools()` + `RuntimeStatus`（available/registered_only/disabled）
- `ToolRegistry` 注册了 20+ 工具，每个有 `Category`/`Tags`/`RiskLevel`/`EnabledByDefault`
- `AgentMCPTooling.EffectiveServersForAgent()` 解析 Agent 的 MCP 工具策略
- MCP 工具通过 `mcp_tool_set` / `mcp_broker` 两个入口注入 Agent 运行时
- `ToolCatalogRuntime` 区分 `function`/`streaming`/`approval` 三种运行时类型

**需要升级为**：

```
Tool 选择器（借鉴 Langflow ToolsComponent + 分类卡片）：
┌─────────────────────────────┐
│ 🔍 搜索工具...              │
├─────────────────────────────┤
│ ▼ 🔧 系统工具               │  ← Registry 注册的工具
│   ☑ web_search              │
│   ☑ file                    │
│   ☐ send_email              │
├─────────────────────────────┤
│ ▼ 🔌 MCP 工具               │  ← MCP 服务器提供的工具
│   ☐ mcp_tool_set            │  ← 整个 MCP 工具集
│   ☐ mcp_broker              │  ← MCP Broker（动态发现）
├─────────────────────────────┤
│ ▼ 🧩 自定义工具             │  ← OpenAPI Spec 注册的工具
│   ☐ my_api_service          │
└─────────────────────────────┘
```

#### 3.11.3 MCP 工具：当前不是 Graph 的一等公民

**现状**：

| 维度 | 当前状态 | Langflow 对标 |
|------|---------|--------------|
| MCP 节点类型 | 不存在 | MCP 组件类型 |
| MCP 工具选择 | 不存在（只能选 `mcp_tool_set`/`mcp_broker` 整体） | MCP 服务器浏览 + 单工具选择 |
| MCP 服务器管理 | 独立页面 `/mcp-servers` | 侧边栏 MCP 区段 |
| MCP 运行时 | 通过 Agent 间接使用（Agent 的 effective tools 包含 MCP） | MCP 组件直接在画布上 |

**关键架构差异**：

Langflow 的 MCP 是**组件级**——每个 MCP 服务器是一个组件，可以拖到画布上作为节点。Aranea 的 MCP 是**Agent 级**——MCP 服务器在 Agent 的 effective tools 中配置，Graph 的 Agent 节点继承其引用 Agent 的 MCP 能力。

这意味着：
1. **Graph 不需要 MCP 节点类型**——MCP 工具通过 Agent 节点间接使用
2. **Tool 节点可以选 `mcp_tool_set`/`mcp_broker`**——但需要注入 MCP 服务器配置（当前 `CatalogToolResolver` 不注入）
3. **Agent 节点需要显示 MCP 信息**——选中 Agent 后，显示该 Agent 配置的 MCP 服务器列表

**建议方案**：

```
方案 A（推荐）：MCP 通过 Agent 间接使用
  - Agent 节点：选择 Agent 后自动显示其 MCP 工具策略
  - Tool 节点：不直接支持 MCP，MCP 工具通过 Agent 节点使用
  - 理由：与现有架构一致，MCP 服务器配置在 Agent 级别管理

方案 B：Tool 节点支持 MCP 工具集
  - Tool 节点：可选 mcp_tool_set + 指定 MCP 服务器
  - 需要：CatalogToolResolver 增加 MCP 服务器配置注入
  - 风险：MCP 工具需要 session 上下文，独立 Tool 节点可能运行时失败
```

#### 3.11.4 Graph 在本项目中的定位

**定位明确**：Graph 是 Aranea 的**确定性编排引擎**，与 Agent（自主推理）和 Team（模式化协作）并列：

| 维度 | Agent | Team | Graph |
|------|-------|------|-------|
| 哲学 | 自主推理 | 模式化协作 | 确定性编排 |
| 控制 | Agent 自主决策 | 框架决定执行顺序 | 用户定义流程骨架 |
| 状态 | 隐式（消息历史） | 隐式（消息传递） | 显式 State Schema + Reducer |
| 分支 | 无 | 无（固定模式） | Conditional Edge + Command Edge |
| 可观测性 | Session 历史 | TeamRunStep | Checkpoint + TimeTravel |
| 适用场景 | 单 Agent 任务 | 简单协作 | 复杂业务流程 |

**Team-Graph 收敛（M53）**：所有 Team 运行已编译为 Graph 运行时，GraphAgent 是唯一执行引擎。Team 只是 Graph 的简化编辑视图。

**Graph 的引用型架构**：

```
GraphNodeResolverSet {
    Models    ModelResolver      → model_name → trpcmodel.Model
    Tools     ToolResolver       → tool_names[] → map[string]trpctool.Tool
    Agents    AgentResolver      → agent_name → trpcagent.Agent
    Functions FunctionResolver   → func_ref → trpctool.CallableTool
}
```

所有节点类型都是**按名称引用**后端资源，运行时通过 Resolver 解析为具体实例。这与 Langflow 的**组件即实例**模型根本不同——Langflow 拖到画布的就是一个 Python 类实例，Aranea 拖到画布的是一个**引用槽位**，需要后续填充具体资源。

#### 3.11.5 对 UI 复刻方案的影响

| Langflow 设计 | Aranea 差异 | UI 适配方案 |
|--------------|------------|------------|
| 组件卡片 = Python 类实例 | 节点 = 引用槽位 | 侧边栏按 7 种节点类型分组，而非按组件类型分组 |
| 拖拽即实例化 | 拖拽创建空节点 → 属性面板选择资源 | 节点创建后需在 PropertyPanel 中选择 Agent/Tool/Function |
| 组件目录平铺 | 资源按类型分层（Agent/Tool/MCP） | 侧边栏卡片只创建节点骨架，资源选择在 PropertyPanel |
| MCP 组件可拖到画布 | MCP 通过 Agent 间接使用 | 无需 MCP 节点类型，Agent 选择器显示 MCP 策略 |
| 所有组件有类型 | Agent/Tool/Function 是引用 | 选择器需要验证引用有效性（Agent 是否存在、Tool 是否可用） |
| 组件参数 20 种编辑器 | 节点参数只需 5 种 | 大幅简化 ParameterRenderComponent |

**关键结论**：Langflow 的侧边栏组件卡片模式**不能直接复刻**。Aranea 需要的是**双层选择**：

1. **第一层**：侧边栏选择节点类型（Agent/LLM/Tool/Function/Router/Join/HITL）
2. **第二层**：PropertyPanel 选择具体资源（哪个 Agent、哪些 Tool、哪个 Function）

Langflow 只有一层选择（组件卡片 = 类型 + 实例合一），Aranea 必须两层分离。

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
| **类型化端口** | 连接时防止语义错误 | **State 字段名标注**：端口显示字段名（非类型），连接时验证字段名匹配（详见 §11.4 决策 1） |
| **节点折叠** | 复杂图的空间管理 | 适配折叠，**放宽约束**：所有节点可折叠，折叠后显示 Header + 汇总端口（详见 §11.4 决策 5） |
| **节点状态指示** | 运行时可视化 | 适配 Aranea 的 **5 种核心状态**（idle/running/success/error/interrupted）+ HITL 中断态（详见 §11.4 决策 2） |
| **侧边栏 Playground** | 编辑→测试闭环 | **内嵌 RunPanel**：State 初始化 → 执行 → 实时节点状态 + State 变迁 + Checkpoint |
| **Inspection Panel** | 节点级调试 | **State Diff 面板**：节点执行前后 State 变化 |
| **NodeToolbar** | 快捷操作 | 适配：**精简为 3 项**（运行到此/冻结/删除），其余放右键菜单（详见 §11.1） |
| **侧边栏分段导航** | 组件发现 + 功能分区 | 40px 图标导航条 + **3 区段**（节点类型/版本历史/设置）（详见 §11.1） |
| **连接验证** | 防止错误连接 | **离线校验面板**：连接时只做结构性检查，路径可达性作为校验面板的离线检查项（详见 §11.4 决策 3） |
| **Handle 点击过滤** | 快速发现可连接目标 | **State 字段级过滤**：点击"写入 messages"Handle，所有"读取 messages"Handle 高亮 |
| **边动画波浪推进** | 执行前沿可视化 | 批量边动画更新：每个节点完成后清除旧动画 → 设置下一批 |
| **组件版本管理** | 后端变更时前端提示 | Registry 函数签名变更时，引用该函数的节点显示"需更新"横幅 |
| **边重连** | 灵活调整连接 | 适配，但需注意 Langflow 无效重连会删除边，Aranea 可选择恢复原位 |
| **便签节点** | 画布标注 | 适配：Graph 便签节点，支持 Markdown + 颜色 |
| **构建进度指示** | 执行状态反馈 | 适配：底部居中进度条，构建中/成功/失败三态 |
| **Handle Tooltip** | 端口信息提示 | 适配：悬停 1s 显示 State 字段类型 + "Drag to connect / Click to filter" |
| **内联编辑** | 快速修改节点属性 | 适配：名称/描述铅笔图标编辑，Enter 保存/Escape 取消 |
| **辅助对齐线** | 精确布局 | 适配：1px 虚线 + 发光 + 5px 吸附距离 |

### 5.2 不应复刻的 Langflow 特性

| Langflow 特性 | 不复刻原因 |
|---------------|-----------|
| Python 代码编辑器节点 | Aranea 的 Function 节点通过 Registry 注册 Go 函数，不支持运行时代码编辑 |
| 组件市场/Store | Aranea 的节点类型是固定的 7 种，不支持用户自定义组件 |
| LLM 模型选择下拉 | Aranea 的 Model 通过 ModelResolver 解析，不在节点内配置凭证 |
| VectorStore 组件 | Aranea 不直接暴露向量库操作，通过 Tool/Agent 间接使用 |
| MCP Server 分享 | Aranea 的分享机制不同，通过 API 端点暴露 |
| 全局模型提供商设置 | Aranea 的模型配置在 Agent 目录中管理，不在 Graph 层面 |
| LangChain 数据类型层级 | Aranea 的数据流由 State Schema 驱动，不是 LangChain 组件数据类型。Langflow 实际也没有类型层级，是显式枚举匹配 |
| 类型继承树（Any/Data/Message 层级） | Langflow 实际没有前端类型继承，是 `input_types`/`output_types` 显式枚举 + 字符串匹配。Aranea 应基于 State Schema 字段语义设计验证规则 |
| 内嵌迷你聊天界面 | Langflow 实际没有此功能，ChatInput/ChatOutput 节点不内嵌聊天 UI，交互在侧边栏完成 |

---

## 六、复刻方案

### P0 — 核心体验升级

#### P0-1：节点重构 — State-Aware Node

**目标**：将 GraphFlowNode 从"固定高度矩形"重构为"State-Aware 可折叠节点"，显示每个节点对 State 的读写字段。

**节点视觉设计**（借鉴 Langflow GenericNode 的视觉规范）：

| 属性 | Langflow 值 | Aranea 适配 |
|------|------------|-------------|
| 圆角 | `rounded-xl` (12px) | 保留，与 Quasar `rounded-borders-xl` 对齐 |
| 阴影 | `shadow-sm` / `hover:shadow-md` | 保留 |
| 默认边框 | `border ring-[0.5px] ring-border` | 用 CSS 变量 `--graph-node-border` |
| 选中边框 | `ring-[0.75px] ring-muted-foreground` | 用 CSS 变量 `--graph-node-selected` |
| 宽度（展开） | `w-80` (320px) | 保留 |
| 宽度（折叠） | `w-48` (192px) | 保留 |
| Header 间距 | `px-4 py-3` | 保留 |
| 图标尺寸 | `h-4.5 w-4.5` (18px) | 保留 |
| 名称字号 | `text-base font-medium` | 保留 |
| Handle 圆点 | 10px 彩色圆形 | 保留，颜色按 State 字段类型 |
| Handle 点击区 | 32×32px 透明 | 保留 |
| 节点类型色 | 左侧 3px 色条（`borderLeftColor`） | 替代当前 accent bar |

**节点视觉结构**：

```
展开态 (w-80, 320px)：
┌─────────────────────────────────────────────┐
│ [🤖]  Agent: CustomerService  [⏱0.3s] [▶] │  ← Header: px-4 py-3, border-b
│       [Legacy] [Beta]                       │     Icon(h-4.5) + Name(ml-3,text-base) + Status(shrink-0,gap-2)
│ 双击编辑描述...                              │  ← Description: px-4 pb-3, text-xs, Markdown
├─────────────────────────────────────────────┤
│ READS                                       │  ← 输入端口区（从 State 读取）
│  ○ messages        [Message[]]              │  ← Handle(10px) + 字段名 + 类型徽章
│  ○ user_input      [string]                 │
├─────────────────────────────────────────────┤
│ WRITES                                      │  ← 输出端口区（写入 State）
│  ● response        [Message]                │
│  ● task_status     [string]                 │
├─────────────────────────────────────────────┤
│ InputMapper: {messages←messages}            │  ← Agent 节点特有：映射关系
│ OutputMapper: {response→response}           │
└─────────────────────────────────────────────┘

折叠态 (w-48, 192px)：
┌──────────────────────────┐
│ [🤖] Agent:CS  ○ ●      │  ← Header only, 无 border-b
│                          │     折叠态: 名称 truncate, 内联 Handle 圆点
└──────────────────────────┘
  ○ = primary input Handle  ● = output Handle(s)
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

> **关键修正**（详见 §11.4 决策 3）：Aranea 的数据流是**通过共享 State** 的（节点读写 State，边只决定执行顺序），而非 Langflow 的点对点数据传递。连接验证采用**离线校验面板**模式——连接时只做结构性检查（自连接/重复边），路径可达性作为校验面板的离线检查项。

```typescript
// Aranea 的连接验证：两层分离
// 1. 实时检查（连接时）：仅结构性检查，轻量快速
// 2. 离线校验（点击"校验"按钮时）：路径可达性 + Reducer 冲突

// 实时检查：连接时执行
function isValidConnectionQuick(
  sourceNode: NodeDef,
  targetNode: NodeDef,
  graphDef: GraphDefinition
): { valid: boolean; reason?: string } {
  // 1. 自连接检查
  if (sourceNode.id === targetNode.id) return { valid: false, reason: '不能自连接' }
  // 2. 重复边检查
  const exists = graphDef.edges.some(e => e.from === sourceNode.id && e.to === targetNode.id)
  if (exists) return { valid: false, reason: '连接已存在' }
  // 3. 字段名匹配提示（非阻断，仅 tooltip 显示）
  const fieldMatch = checkFieldNameOverlap(sourceNode, targetNode)
  return { valid: true }  // 连接允许，但可能有字段不匹配的警告
}

// 离线校验：校验面板执行（详见 §11.4 决策 3）
interface GraphValidationResult {
  structuralErrors: StructuralError[]      // 自连接、重复边、有向环
  unreachableFields: UnreachableField[]    // 路径不可达的 State 字段
  reducerConflicts: ReducerConflict[]      // Reducer 冲突（区分 BSP/DAG 引擎）
}
```

**Handle 布局策略**：

Langflow 的 Handle 与组件参数一一对应，位置由参数行高决定。Aranea 的 State 字段数量可能远多于 Langflow 参数（一个 Agent 节点可能读写 5-10 个 State 字段），需要专门的布局策略：

| 策略 | 说明 | 适用场景 |
|------|------|----------|
| **分组折叠** | READS/WRITES 各显示前 3 个字段，其余折叠到"更多 N 个" | 字段 > 3 的节点 |
| **按 Reducer 分组** | append 字段一组、default 字段一组，组内折叠 | 多 Reducer 混合节点 |
| **Handle ID 编码** | 使用 `r:fieldName` / `w:fieldName` 简洁编码，避免 JSON 序列化 | 所有节点 |
| **折叠态 Handle** | 保留 primary input + primary output Handle，其余隐藏 | 折叠态 |
| **折叠约束** | **放宽**：所有节点均可折叠，折叠后只保留汇总 READS/WRITES Handle（详见 §11.4 决策 5） | 所有节点 |

**Handle ID 编码方案**：

```typescript
// Aranea Handle ID 编码（简洁，避免 JSON 序列化带来的 DOM ID 问题）
type HandleId = {
  direction: 'r' | 'w'    // r=READS(输入), w=WRITES(输出)
  field: string            // State 字段名
  nodeId: string           // 节点 ID
}

// 编码格式：r:messages:node1 / w:response:node1
function encodeHandleId(h: HandleId): string {
  return `${h.direction}:${h.field}:${h.nodeId}`
}

function decodeHandleId(id: string): HandleId {
  const [direction, ...rest] = id.split(':')
  const nodeId = rest.pop()!
  const field = rest.join(':')  // 支持字段名含冒号
  return { direction: direction as 'r' | 'w', field, nodeId }
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

> **源码修正**：Langflow 的 `BuildStatus` 枚举是 `TO_BUILD / BUILDING / BUILT / INACTIVE / ERROR`，没有 "idle"。`INACTIVE` 表示被条件路由排除的节点，这是 Aranea 文档未覆盖的场景。状态更新是批量的：`vertices_sorted` 事件一次性设置所有待执行节点为 TO_BUILD。

**状态映射**（5 种核心状态，详见 §11.4 决策 2）：

| 运行时事件 | 节点视觉 | 说明 |
|-----------|---------|------|
| 无事件 | `idle` — 默认边框 | 未执行，无特殊标记 |
| `graph_step` / `graph_node_start` | `running` — 蓝色脉冲 + 旋转图标 | 正在执行（合并 queued） |
| `graph_node_end` | `success` — 绿色边框 + 勾号 | 执行完成 |
| `graph_node_error` | `error` — 红色边框 + 错误图标 | 执行失败 |
| `checkpoint.interrupt` | `interrupted` — 橙色边框 + 暂停图标 | HITL 等待审批 |

> **简化说明**：`queued` 合并到 `running`（用户不关心"排队中"），`inactive`（条件路由排除）通过灰化节点表示，`active`（Command 动态路由）合并到 `running`。如需区分，可通过节点 tooltip 显示详细子状态。

**节点状态视觉规范**（5 种核心状态，借鉴 Langflow 的边框/阴影/动画体系）：

| 状态 | 边框 | 阴影/发光 | 动画 | 图标 |
|------|------|----------|------|------|
| `idle` | `ring-[0.5px] ring-border` | `shadow-sm` | 无 | 无 |
| `running` | `border-[1px] border-accent-indigo` | `shadow-sm` | 150ms wiggle | Loader2 旋转 |
| `success` | `ring-[0.75px] ring-accent-emerald` | `shadow-sm` | 无 | Check 圆形 |
| `error` | `border-[1px] border-destructive` | `shadow-sm` | 无 | CircleAlert 红色 |
| `interrupted` | `border-[1px] border-accent-amber` | `shadow-sm` | 无 | CirclePause 橙色 |

**Handle 状态视觉**（借鉴 Langflow 的霓虹脉冲系统）：

| Handle 状态 | 视觉 | 触发条件 |
|------------|------|----------|
| 默认 | 10px 彩色圆点 + 3px 同色 ring | 无交互 |
| 兼容高亮 | 霓虹脉冲动画（1.1s 呼吸，2px→30px box-shadow） | 点击其他 Handle 过滤时，类型匹配 |
| 不兼容灰化 | 灰色边框圆点（null handle） | 点击其他 Handle 过滤时，类型不匹配 |
| Muted | 6px, opacity:0 | 未连接且非交互状态 |
| 已连接 | 连接边源类型色圆点 | 有 edge 连接到此 Handle |

**边的状态**：

| 运行时事件 | 边视觉 | 说明 |
|-----------|--------|------|
| 节点完成 → 下一个节点开始 | 蓝色流动光点 | State 正在传递 |
| Conditional Edge 被选中 | 绿色高亮 + 标签 | 实际走的分支 |
| Conditional Edge 未被选中 | 灰色虚线 | 未走的分支 |
| Command Edge 动态激活 | 紫色流动光点 | Agent 运行时决定的路径 |

**边动画批量更新机制**（借鉴 Langflow 的 `clearAndSetEdgesRunning`）：

Langflow 在每个 `end_vertex` 事件后执行批量边动画更新：
1. 清除所有边的 running 状态
2. 只设置下一批待执行节点相关的边为 running

这产生"波浪式"视觉效果——执行前沿逐层推进。Aranea 需要在 `useGraphRunStream` 中增加边级动画控制：

```typescript
// 在 useGraphRunStream.ts 中增加边动画批量更新
function onNodeEnd(nodeId: string, nextNodeIds: string[]) {
  // 1. 更新节点状态
  updateNodeState(nodeId, 'success')

  // 2. 批量更新边动画：清除所有 → 设置下一批
  clearAndSetEdgesRunning(nextNodeIds)

  // 3. 更新下一批节点状态为 queued
  nextNodeIds.forEach(id => updateNodeState(id, 'queued'))
}
```

**改造范围**：
- 新增：`web/src/components/graph/GraphNodeStatusBadge.vue`（状态徽章组件）
- 修改：`web/src/components/graph/GraphFlowNode.vue`（集成状态徽章 + 5 种状态样式）
- 修改：`web/src/components/graph/GraphFlowEdge.vue`（边状态视觉 + 批量动画控制）
- 修改：`web/src/features/graph/runtime/useGraphRunStream.ts`（事件→状态映射 + 边动画批量更新）

#### P0-4：资源选择器 — Agent/Tool/Function 分类选择

**目标**：将 GraphPropertyPanel 中的纯文本输入（agentName/funcRef）和平铺下拉（toolNames）升级为分类选择器，实现"拖入节点 → 自动弹出选择器"的引导式创建流程（详见 §11.2.1）。

**三种选择器**：

1. **Agent 选择器**（替代 `q-input` agentName）

```
┌─────────────────────────────┐
│ 🔍 搜索 Agent...            │  ← Fuse.js 模糊搜索
├─────────────────────────────┤
│ ▼ 🤖 自建 Agent             │  ← Kind=user
│   CustomerService           │
│   DataAnalyzer              │
├─────────────────────────────┤
│ ▼ 🏗️ 系统 Agent             │  ← Kind=system_builtin
│   DefaultAssistant          │
├─────────────────────────────┤
│ ▼ 🌐 A2A Proxy              │  ← AgentKind=a2a_proxy
│   ExternalService           │
└─────────────────────────────┘
选择后 → 自动填充 agentName + 显示该 Agent 的 MCP 策略信息
```

2. **Tool 选择器**（增强 `q-select` toolNames，增加分类）

```
┌─────────────────────────────┐
│ 🔍 搜索工具...              │
├─────────────────────────────┤
│ ▼ 🔧 系统工具               │  ← Registry 注册的工具（含 Category 分组）
│   ☑ web_search              │
│   ☑ file                    │
│   ☐ send_email              │
├─────────────────────────────┤
│ ▼ 🔌 MCP 工具               │  ← mcp_tool_set / mcp_broker
│   ☐ mcp_tool_set            │
│   ☐ mcp_broker              │
├─────────────────────────────┤
│ ▼ 🧩 自定义工具             │  ← OpenAPI Spec 注册的工具
│   ☐ my_api_service          │
└─────────────────────────────┘
```

3. **Function 选择器**（替代 `q-input` funcRef）

```
┌─────────────────────────────┐
│ 🔍 搜索函数...              │
├─────────────────────────────┤
│ ▼ ⚡ Registry 函数           │  ← 已注册的 Go 函数
│   if_else_router            │
│   sentiment_classifier      │
└─────────────────────────────┘
```

**节点创建引导**（详见 §11.2.1）：

拖入节点到画布后，自动弹出对应的资源选择器：
- Agent 节点 → 弹出 Agent 选择器
- Tool 节点 → 弹出 Tool 选择器
- Function 节点 → 弹出 Function 选择器
- Router/Join/HITL → 不弹选择器（无需引用外部资源）

选择完成后节点自动填充，跳过则显示"未配置"警告。

**数据源**：

| 选择器 | API | Store | 说明 |
|--------|-----|-------|------|
| Agent | `listAgents()` | `useAgentsCatalogStore` | 已有，需增加 Kind 分组 |
| Tool | `listTools()` | `useToolsStore` | 已有，需增加 Category 分组 + MCP 工具 |
| Function | `listTools()` (filter source=registry) | `useToolsStore` | 复用 Tool API，过滤 Registry 来源 |

**改造范围**：
- 新增：`web/src/components/graph/GraphResourceSelector.vue`（通用资源选择器）
- 新增：`web/src/components/graph/GraphAgentSelector.vue`（Agent 分类选择器）
- 新增：`web/src/components/graph/GraphToolSelector.vue`（Tool 分类选择器）
- 新增：`web/src/components/graph/GraphFunctionSelector.vue`（Function 选择器）
- 修改：`web/src/components/graph/GraphPropertyPanel.vue`（替换纯文本输入为选择器）
- 修改：`web/src/features/graph/useGraphEditorPage.ts`（节点创建后触发选择器）

#### P0-3：内嵌 RunPanel — State 驱动的运行时面板

**目标**：在编辑器内嵌入轻量运行面板，实现"编辑→执行→调试"闭环，无需跳转页面。

> **架构修正**：借鉴 Langflow 的 Playground 侧边栏 + InspectionPanel **共存**模式（而非互斥切换）。RunPanel 是画布外部的可拖拽侧边栏，与 PropertyPanel 共存。

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

**RunPanel 与 GraphRunPage 的职责边界**：

> 明确区分两者功能，避免重复和用户困惑。借鉴 Langflow 的"编辑器 Playground vs 独立 Playground 页面"分离模式。

| 功能 | RunPanel（编辑器内） | GraphRunPage（全屏监控） |
|------|---------------------|------------------------|
| 执行控制（启动/取消） | ✅ | ✅ |
| 实时节点状态 | ✅（画布节点高亮） | ✅（节点状态列表） |
| State 快照 | ✅（当前 State） | ✅（历史对比） |
| Checkpoint 导航 | ✅（快速跳转） | ✅（完整 TimeTravel + 编辑 + 重执行） |
| HITL 审批 | ✅（弹窗） | ✅（侧边栏） |
| 任务看板 | ❌ | ✅ |
| 执行历史 | ❌ | ✅ |
| State Diff | ❌（P2 补充） | ✅ |

**与 PropertyPanel 的共存**：

借鉴 Langflow 的 InspectionPanel + Playground 共存模式：
- PropertyPanel：画布内浮层（选中节点时显示），编辑节点属性
- RunPanel：画布外部侧边栏，运行时交互
- 两者可同时显示，互不干扰
- RunPanel 默认收起，执行时自动展开；宽度可拖拽调整

**RunPanel 视觉设计**（借鉴 Langflow Playground 侧边栏）：

| 属性 | Langflow 值 | Aranea 适配 |
|------|------------|-------------|
| 默认宽度 | 326px | 326px |
| 可拖拽范围 | 15%-60% viewport | 同 |
| 全屏模式 | 100% 宽度 + 左侧 236px 会话列表 | 同 |
| 面板样式 | `rounded-xl border bg-background shadow-lg` | Quasar `rounded-borders` + 阴影 |
| 过渡动画 | 300ms ease-in-out | 同 |
| 消息气泡（Bot） | `rounded-md px-2 py-3 hover:bg-muted` | Quasar Chat 消息样式 |
| 消息气泡（User） | `rounded-md px-2 py-3 bg-muted` | Quasar Chat 消息样式 |
| 输入区 | 底部固定 `flex-shrink-0 p-4` | 同 |
| State View | JSON/表格视图，diff 高亮 | 新增，Langflow 无此功能 |
| Checkpoint 导航 | 时间线列表 | 新增，Langflow 无此功能 |
| HITL Queue | 待审批卡片列表 | 新增，Langflow 无此功能 |

**改造范围**：
- 新增：`web/src/components/graph/GraphRunPanel.vue`（RunPanel 主组件）
- 新增：`web/src/components/graph/GraphRunPanelChat.vue`（State 视图 + 执行控制）
- 新增：`web/src/features/graph/useGraphPlayground.ts`（Playground composable）
- 修改：`web/src/pages/GraphEditorPage.vue`（集成 RunPanel 侧边栏）
- 修改：`web/src/features/graph/useGraphEditorPage.ts`（RunPanel 状态管理）

---

### P1 — 体验打磨

#### P1-1：侧边栏重构 — 分段导航 + 7 种节点类型分组

**目标**：借鉴 Langflow 的分段导航侧边栏（SidebarSegmentedNav），按 Aranea 的 7 种节点类型分组，提供搜索、分类、拖拽添加能力。

> **源码验证**：Langflow 的侧边栏使用 40px 图标导航条 + 可折叠内容面板的分段模式，搜索使用 Fuse.js 模糊搜索（300ms 防抖），组件卡片使用 `bg-muted` 背景 + 左侧 8px 色条 + 18px 图标 + 14px 文字。

**布局**（借鉴 Langflow 分段导航）：

```
┌──┬──────────────────────┐
│🔍│ 搜索节点...       [/] │  ← Fuse.js 模糊搜索, 300ms 防抖
│🧩│──────────────────────│
│📜│ ▼ 🧠 智能体 (Agent)  │  ← 分类折叠组
│🔧│   ┃ Agent 节点       │  ← 左 8px 色条 + 18px 图标 + 14px 文字
│📊│   ┃ (引用系统 Agent)  │
│  │──────────────────────│
│  │ ▼ 💬 语言模型 (LLM)  │
│  │   ┃ LLM 节点         │
│  │   ┃ (轻量 LLM 调用)   │
│  │──────────────────────│
│  │ ▼ 🔧 工具 (Tool)     │
│  │   ┃ Tool 节点         │
│  │──────────────────────│
│  │ ▼ ⚡ 函数 (Function)  │
│  │   ┃ Function 节点     │
│  │──────────────────────│
│  │ ▼ 🔀 路由 (Router)    │
│  │   ┃ Router / Join     │
│  │──────────────────────│
│  │ ▼ 👤 人机协同 (HITL)  │
│  │   ┃ HITL 节点         │
│  │──────────────────────│
│  │ [+ 新建自定义组件]    │  ← 底部按钮
└──┴──────────────────────┘
```

**分段导航**（左侧 40px 图标条）：

| 图标 | 区段 | 说明 |
|------|------|------|
| 🧩 | 节点类型 | 7 种节点类型分组 + 快速模板 |
| 📜 | 版本历史 | 复用现有 GraphVersionPanel |
| 🔧 | 设置 | Graph 设置（entryPoint/finishPoint/engine/checkpoint） |

**组件卡片视觉规范**（借鉴 Langflow SidebarDraggableComponent）：

| 属性 | Langflow 值 | Aranea 适配 |
|------|------------|-------------|
| 卡片背景 | `bg-muted` (hsl(240,5%,96%)) | Quasar `bg-grey-2` / dark `bg-grey-9` |
| 卡片间距 | `p-1 px-2` (4px/8px) | 同 |
| 左色条 | `border-l-8 borderLeftColor={color}` | 同，8px 色条 |
| 图标 | `h-[18px] w-[18px]` | 18px |
| 文字 | `text-sm font-normal truncate` | 14px, 正常字重, 截断 |
| 悬停 | `hover:bg-secondary-hover/75` | Quasar hover 色 |
| 拖拽光标 | `cursor-grab` | 同 |
| Plus 按钮 | 桌面端默认隐藏（sm:opacity-0），hover/focus 显示 | 同 |
| GripVertical | 拖拽手柄，hover 时 text-primary | 同 |
| 拖拽 ghost | 离屏 clone 节点 (215px 宽) | 同 |
| 双击添加 | 视口中心 + 192px 偏移 | 同 |
| 右键菜单 | Download + Delete（仅非官方组件） | 适配：下载配置 / 删除自定义节点 |
| 分类图标 | `h-4 w-4`, 展开时变粉色 | 16px |
| 分类文字 | 展开时 `font-semibold` | 同 |
| 分类箭头 | `ChevronRight`, 展开旋转 90° | 同 |

**搜索系统**（借鉴 Langflow Fuse.js 搜索）：

| 属性 | Langflow 值 | Aranea 适配 |
|------|------------|-------------|
| 搜索引擎 | Fuse.js 模糊搜索 | 同 |
| 搜索键 | display_name, description, type, category | id, description, instruction, agentName, type |
| 阈值 | 0.2 (严格) | 同 |
| 防抖 | 300ms | 同 |
| 快捷键 | `/` 聚焦搜索 | 同 |
| 结果高亮 | 无（只显示截断文字） | 可选：高亮匹配文字 |

**改造范围**：
- 重构：`web/src/components/graph/GraphNodePalette.vue`（分段导航 + 搜索 + 卡片）
- 新增：`web/src/components/graph/GraphSidebarSegmentedNav.vue`（40px 图标导航条）
- 新增：`web/src/features/graph/useGraphSidebarSearch.ts`（Fuse.js 搜索 composable）

#### P1-2：NodeToolbar — 节点级快捷操作

**目标**：悬停/选中节点显示操作工具栏，适配 Aranea 业务操作。

> **源码验证**：Langflow 的 NodeToolbar 位于节点上方 48px（`-top-12`），水平居中，高度 40px（`h-10`），`rounded-xl border bg-background p-1 shadow-sm`。按钮使用 `ghost` 变体 + `node-toolbar` 尺寸（24×24px 图标 + 13px 文字）。右键节点自动打开"更多"下拉菜单。

**工具栏布局**（精简版，详见 §11.1 取舍）：

```
                    ┌──────────────────────────┐
                    │ [▶] [❄] [🗑] [⋯]        │  ← 节点上方 48px
                    └──────────────────────────┘
                    
直接按钮（精简为 3 项核心操作）：
  ▶ 运行到此      — Play icon, 触发 buildFlow({ stopNodeId })
  ❄ 冻结          — FreezeAll icon, 冻结时 text-accent-indigo-foreground
  � 删除          — Trash2 icon, text-status-red

"更多"下拉菜单（⋯ 按钮, MoreHorizontal icon）：
  ├── 复制节点       — Copy icon
  ├── 设为入口       — 适配 Aranea entryPoint 设置
  ├── HITL 配置     — 仅 HITL 节点
  ├── 重试策略       — 仅非 HITL 节点
  ├── 缓存策略       — 仅非 HITL 节点
  ├── Fallback Agent — 仅 Agent 节点
  └── 查看运行时日志
```

**工具栏视觉规范**：

| 属性 | Langflow 值 | Aranea 适配 |
|------|------------|-------------|
| 位置 | `absolute -top-12 left-1/2 -translate-x-1/2` | 同 |
| 容器 | `h-10 rounded-xl border bg-background p-1 shadow-sm` | Quasar `rounded-borders-xl` |
| 按钮尺寸 | `ghost` 变体, `node-toolbar` 尺寸 (24×24) | Quasar `flat round size=sm` |
| 图标 | `h-4 w-4` (16px) | 同 |
| 文字 | `text-mmd font-medium` (13px) | 同 |
| 过渡 | `transition-all duration-300 ease-out` | 同 |
| 触发 | 选中节点 OR 右键节点 | 同 |
| 编辑按钮 | 24×24 圆形，编辑态变绿色 Check | 同 |

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
| P0 | 9 个（portTypes.ts, GraphNodeStatusBadge.vue, GraphRunPanel.vue, GraphRunPanelChat.vue, useGraphPlayground.ts, GraphResourceSelector.vue, GraphAgentSelector.vue, GraphToolSelector.vue, GraphFunctionSelector.vue） | 8 个（GraphFlowNode.vue, GraphFlowEdge.vue, GraphEditorCanvas.vue, GraphEditorPage.vue, types.ts, useGraphEditorPage.ts, useGraphRunStream.ts, GraphPropertyPanel.vue） | ~3000 行 |
| P1 | 1 个（GraphNodeToolbar.vue） | 3 个（GraphNodePalette.vue, GraphFlowNode.vue, GraphPropertyPanel.vue） | ~1000 行 |
| P2 | 3 个（GraphSubgraphBreadcrumb.vue, useGraphSubgraph.ts, GraphStateDiffPanel.vue） | 3 个（GraphEditorPage.vue, useGraphEditorPage.ts, types.ts） | ~1500 行 |

---

## 八、关键风险与对策

| 风险 | 对策 |
|------|------|
| Vue Flow 的 Handle 自定义渲染能力不如 React Flow | Vue Flow 支持自定义 slot，端口类型标签可通过 slot 插入，实测可行 |
| 节点折叠后连线可能错位 | 折叠时保留输出端口 Handle 的位置，或使用 Vue Flow 的 `updateNodeInternals` 强制刷新 |
| 内嵌 RunPanel 与现有 GraphRunPage 逻辑重复 | 提取 `useGraphPlayground` composable，两个页面共享；明确职责边界（见 P0-3 功能矩阵） |
| State 字段级连接验证与现有边类型（normal/transfer/dispatch/conditional）的兼容 | 端口验证是数据层（State 字段可达性），边类型是业务层分类，两者正交互不影响 |
| 子图嵌套的 undo/redo 栈管理复杂 | 每层子图独立 undo/redo 栈，退出子图时合并到父栈 |
| RunPanel 宽度挤压画布空间 | 面板可收起/展开，默认收起，运行时自动展开；宽度可拖拽调整 |
| **State 字段数量爆炸** | 一个复杂 Graph 可能有 20+ State 字段，每个节点显示所有读写字段会导致节点过高。对策：分组折叠——READS/WRITES 各显示前 3 个字段，其余折叠到"更多 N 个"；或按 Reducer 类型分组 |
| **路径可达性验证的计算成本** | 每次拖拽连线时计算路径可达性，复杂图可能卡顿。对策：增量计算 + 缓存——只在图结构变更时重算可达性，拖拽时查缓存；或降级为警告（不阻断连接） |
| **Vue Flow Handle 的 DOM ID 限制** | Vue Flow 的 Handle ID 是字符串，Aranea 需要编码 State 字段信息（方向+字段名+节点ID），可能与 Vue Flow 内部机制冲突。对策：使用简洁编码（`r:messages:node1`），避免 JSON 序列化；实测 Vue Flow Handle ID 长度限制 |
| **Reducer 冲突在运行时才能发现** | 前端验证只能检查静态冲突（同一 default reducer 字段被多个节点写入），动态冲突（并行节点的写入顺序）需要运行时检测。对策：前端做静态检查 + 运行时增加 Reducer 冲突告警事件（`graph_state_conflict`） |
| **循环图（Router 回边）的视觉设计** | Aranea 的 Router 节点可能产生循环（条件路由回到之前的节点），当前没有循环边的视觉设计。对策：借鉴 Langflow 的 CycleEdge 机制，循环边使用特殊视觉样式（虚线 + 循环标记），并在连接验证中允许 Router 循环 |
| **折叠约束与多端口节点的矛盾** | Agent 节点通常有多个 WRITES Handle，不满足 `isMinimal` 条件，无法折叠。对策：放宽折叠约束——允许折叠但折叠后只保留 primary Handle，其他 Handle 的连线自动隐藏并在展开时恢复 |

---

## 九、执行建议

1. **P0-1（节点重构）优先级最高** — 这是所有其他改进的基础，当前 GraphFlowNode 的固定高度、无端口标注是最大的体验瓶颈
2. **P0-2（状态指示）成本最低、收益最高** — 只需在现有 WS 事件流基础上增加视觉映射
3. **P0-3（RunPanel）业务价值最大** — 让用户在编辑器内完成"编辑→执行→调试"闭环
4. P0-1 和 P0-2 可以并行开发（节点重构和状态指示是相对独立的改动）
5. P0-3 依赖 P0-1 和 P0-2（RunPanel 需要新的节点组件和状态系统）
6. P1/P2 根据业务优先级排期，建议 P0 全部完成后再启动

---

## 十一、业务取舍与用户友好度优化

> **核心原则**：Graph 的用户是**业务流程设计者**（非 Python 开发者），他们关心的是"流程是否正确"而非"数据类型是否兼容"。UI 设计应降低认知负担，让用户聚焦业务逻辑。

### 11.1 功能取舍矩阵

基于 Graph 在项目中的定位（确定性编排引擎、引用型架构、双层选择），对 Langflow 的每个设计思想做取舍：

| Langflow 设计 | 取舍 | 理由 |
|--------------|------|------|
| **类型化端口（Handle 显示数据类型）** | **简化** → 端口显示 State 字段名 | 用户关心"这个节点读写哪些字段"，不关心"字段是什么类型"。字段名比类型徽章更直观 |
| **Handle 点击过滤** | **保留** → 字段名级过滤 | 点击"写入 messages"Handle，高亮所有"读取 messages"Handle，这是最直观的连线引导 |
| **Handle 霓虹脉冲** | **简化** → 单色呼吸发光 | 7 种节点类型色 + 6 种 State 字段类型色 = 13 种颜色太多。简化为：兼容=绿色脉冲、不兼容=灰色、默认=节点类型色 |
| **节点折叠** | **保留但放宽** | Langflow 的 `isMinimal` 规则太严格（Agent 节点几乎无法折叠）。放宽为：所有节点可折叠，折叠后只显示 Header + primary Handle |
| **8 种节点状态** | **简化** → 5 种核心状态 | idle/running/success/error/interrupted 足够。`queued` 合并到 `running`（用户不关心"排队中"），`inactive` 合并到 `idle`（灰化即可），`active` 合并到 `running`（紫色区分度低） |
| **路径可达性验证** | **降级** → 警告不阻断 | 路径可达性计算成本高，且对初学者过于复杂。改为：连接时只做结构性检查（自连接/重复边），路径可达性作为"校验面板"的离线检查项 |
| **Reducer 冲突检测** | **延后** → P2 | 初学者不会遇到 Reducer 冲突（只有并行分支写同一字段时才冲突），等用户有高级需求再补 |
| **侧边栏分段导航** | **简化** → 3 区段 | Langflow 6 区段（Search/Components/MCP/Bundles/Versions/Traces）太多。Aranea 只需：节点类型/版本历史/设置 |
| **侧边栏组件卡片** | **重新设计** → 节点骨架卡片 | Langflow 卡片=实例，Aranea 卡片=骨架。卡片只显示节点类型图标+名称，资源选择在 PropertyPanel |
| **RunPanel** | **保留** | 编辑→执行→调试闭环是核心价值，但简化为：执行控制 + State 快照 + Checkpoint 导航 |
| **NodeToolbar** | **保留但精简** | 只保留：运行到此/冻结/删除。其他操作（复制/配置）放右键菜单 |
| **边动画波浪推进** | **保留** | 执行可视化是 Graph 的核心差异化能力 |
| **便签节点** | **延后** → P2 | 便签是锦上添花，非核心 |
| **构建进度指示器** | **保留** | 执行反馈必需 |
| **辅助对齐线** | **延后** → P2 | 对齐是布局优化，非核心 |
| **内联编辑** | **保留** | 双击节点名直接编辑，降低操作路径 |
| **Handle Tooltip** | **保留** | 悬停显示字段信息，降低学习成本 |
| **边重连** | **保留但改行为** | 无效重连恢复原位（不删除边），更安全 |
| **组件版本管理** | **延后** → P2 | 等 Registry 函数签名变更场景成熟再补 |
| **Inspection Panel** | **重新定位** → 运行时 State Diff | 只在运行后显示，编辑时不显示（编辑时用 PropertyPanel） |

### 11.2 用户友好度优化

#### 11.2.1 降低入门门槛

**问题**：当前 Graph 编辑器对新手不友好——拖节点到画布后不知道下一步做什么，Agent 名称要手动输入，工具选择没有分类。

**优化方案**：

1. **节点创建引导**：拖节点到画布后，自动弹出资源选择器（而非等用户点 PropertyPanel）

```
拖入 Agent 节点 → 自动弹出 Agent 选择器
┌─────────────────────────────┐
│ 选择一个 Agent               │
│ 🔍 搜索...                  │
├─────────────────────────────┤
│ 🤖 CustomerService          │  ← 自建 Agent
│ 🤖 DataAnalyzer             │
│ 🏗️ DefaultAssistant         │  ← 系统 Agent
└─────────────────────────────┘
选择后 → 节点自动填充 agentName + 显示 READS/WRITES
跳过 → 节点显示 "未配置 Agent" 警告
```

2. **智能默认值**：创建节点时根据类型自动填充

| 节点类型 | 自动填充 | 依据 |
|----------|---------|------|
| Agent | 默认 Agent（系统第一个可用 Agent） | 降低空节点率 |
| LLM | 默认 Model（系统配置的默认模型） | 用户通常不关心模型选择 |
| Tool | 空（必须手动选择） | 工具选择是核心操作 |
| Function | 空（必须手动选择） | 函数引用是核心操作 |
| Router | 默认条件函数（if/else） | 最常见的路由模式 |
| Join | 无需配置 | 透传节点 |
| HITL | 默认审批角色（admin） | 最常见的审批场景 |

3. **连线引导**：拖拽连线时，只高亮可连接的 Handle

```
从 "写入 messages" Handle 拖线 → 所有 "读取 messages" Handle 绿色脉冲
                              → 其他 Handle 灰色
                              → 无可连接目标时显示 "无兼容端口" 提示
```

#### 11.2.2 降低认知负担

**问题**：State Schema 是 Graph 的核心概念，但对新手来说"State 字段"、"Reducer"、"路径可达性"太抽象。

**优化方案**：

1. **State Schema 可视化**：在画布右侧显示 State 面板，实时显示当前 State 的字段和流向

```
┌─ State 面板 ──────────────┐
│ messages   [Message[]]     │  ← append, 被写入 3 次
│   ← Agent:CS (写入)        │  ← 显示哪些节点写入此字段
│   → Agent:Review (读取)    │  ← 显示哪些节点读取此字段
│ step       [integer]       │  ← default, 被写入 1 次
│ approval   [string]        │  ← cover, 被 HITL 节点写入
└────────────────────────────┘
```

2. **节点端口用字段名而非类型**：

```
当前方案（技术视角）：
  ○ READS: messages [Message[]]     ← 类型徽章，技术用户才懂
  ● WRITES: response [string]

优化方案（业务视角）：
  ○ 读取: messages                  ← 字段名，一看就懂
  ● 写入: response                  ← 不显示类型，减少噪音
```

3. **Reducer 用自然语言而非术语**：

| Reducer | 技术描述 | 用户友好描述 |
|---------|---------|------------|
| default | 完全替换旧值 | "覆盖" |
| append | 追加到列表 | "追加" |
| cover | 仅非零值覆盖 | "可选更新" |
| merge | 深度合并 Map | "合并" |

#### 11.2.3 操作效率优化

**问题**：当前操作路径太长——创建节点 → 点 PropertyPanel → 选 Agent → 选 Tool → 配参数。

**优化方案**：

1. **拖拽即配置**：从侧边栏拖入节点时，如果该节点类型只有一个可选资源，自动填充

```
侧边栏只有一个 Agent "CustomerService" → 拖入 Agent 节点自动填充
侧边栏有多个 Agent → 拖入后弹出选择器
```

2. **快捷连线**：双击 Handle 自动连接到最近的兼容 Handle

```
双击 "写入 messages" Handle → 自动连接到最近的 "读取 messages" Handle
无兼容 Handle → 显示 "无兼容端口" 提示
```

3. **批量操作**：多选节点后批量配置

```
选中 3 个 Tool 节点 → 右键 "批量添加工具" → 弹出工具选择器 → 所有选中节点添加相同工具
```

4. **模板快速创建**：侧边栏提供常用流程模板

```
┌──┬──────────────────────┐
│🧩│ ▼ 📋 快速模板         │  ← 新增区段
│📜│   审批流程             │  ← 预置: Agent→HITL→Agent
│🔧│   数据处理管线         │  ← 预置: Function→LLM→Function
│📊│   条件路由             │  ← 预置: Agent→Router→2×Agent
│  │──────────────────────│
│  │ ▼ 🧠 节点类型         │  ← 原有区段
└──┴──────────────────────┘
```

### 11.3 修订后的分期方案

基于取舍结果，重新排列优先级：

#### P0 — 核心体验（必须做）

| 编号 | 功能 | 用户价值 | 复杂度 |
|------|------|---------|--------|
| P0-1 | **节点重构**：State-Aware 可折叠节点，端口显示字段名 | 从"看不懂"到"一目了然" | 高 |
| P0-2 | **状态指示**：5 种核心状态（idle/running/success/error/interrupted） | 从"黑盒"到"实时可见" | 低 |
| P0-3 | **RunPanel**：执行控制 + State 快照 + Checkpoint 导航 | 从"跳转页面"到"编辑器内闭环" | 中 |
| P0-4 | **资源选择器**：Agent/Tool/Function 分类选择器（替代纯文本输入） | 从"手动输入"到"可视化选择" | 中 |

#### P1 — 体验打磨（应该做）

| 编号 | 功能 | 用户价值 | 复杂度 |
|------|------|---------|--------|
| P1-1 | **侧边栏重构**：3 区段（节点类型/版本/设置）+ 节点骨架卡片 + 快速模板 | 从"不知道拖什么"到"模板引导" | 中 |
| P1-2 | **NodeToolbar**：运行到此/冻结/删除 + 右键菜单 | 从"找 PropertyPanel"到"一键操作" | 低 |
| P1-3 | **连线引导**：Handle 字段名级过滤 + 兼容高亮 + 快捷连线 | 从"盲连"到"引导连线" | 中 |
| P1-4 | **State Schema 可视化**：右侧面板显示字段流向 | 从"抽象概念"到"可视化" | 中 |

#### P2 — 高级功能（可以做）

| 编号 | 功能 | 用户价值 | 复杂度 |
|------|------|---------|--------|
| P2-1 | **路径可达性校验**：离线校验面板（非实时阻断） | 防止运行时错误 | 高 |
| P2-2 | **Reducer 冲突检测**：静态检查 + 运行时告警 | 防止并发写入冲突 | 高 |
| P2-3 | **子图嵌套**：双击 Agent 进入子图 | 大型流程模块化 | 高 |
| P2-4 | **State Diff**：运行时节点级 State 变化 | 调试利器 | 中 |
| P2-5 | **便签节点** | 画布标注 | 低 |
| P2-6 | **辅助对齐线** | 精确布局 | 低 |
| P2-7 | **组件版本管理** | Registry 变更提示 | 中 |

### 11.4 关键设计决策

#### 决策 1：端口显示字段名而非类型

```
选择 A（技术视角）：○ messages [Message[]]  ← Langflow 风格，显示类型徽章
选择 B（业务视角）：○ messages              ← 只显示字段名

推荐 B，理由：
1. Aranea 的 State 字段类型只有 6 种（string/integer/float/boolean/array/object），
   类型信息对用户价值低（string 和 integer 的连线行为没有区别）
2. 字段名本身就是最好的语义说明（messages 比 Message[] 更直观）
3. 减少视觉噪音，节点更紧凑
4. 类型信息放在 Handle Tooltip 中（悬停 1s 显示），需要时才看
```

#### 决策 2：5 种状态而非 8 种

```
选择 A：8 种状态（idle/queued/running/success/error/interrupted/active/inactive）
选择 B：5 种状态（idle/running/success/error/interrupted）

推荐 B，理由：
1. queued 和 running 对用户无区别——都是"正在处理"
2. active（Command Edge 动态路由）是高级场景，初学者不会遇到
3. inactive（条件路由排除）可通过灰化节点表示，不需要独立状态
4. 5 种状态 → 5 种颜色，用户记忆负担低
5. 后端事件映射：
   - graph_step → running（合并 queued）
   - graph_node_start → running
   - graph_node_end → success
   - graph_node_error → error
   - checkpoint.interrupt → interrupted
   - 无事件 → idle
```

#### 决策 3：路径可达性降级为离线校验

```
选择 A：实时路径可达性验证（每次连线时计算）
选择 B：离线校验面板（点击"校验"按钮时计算）

推荐 B，理由：
1. 实时验证增加连线延迟，复杂图可能卡顿
2. 初学者不理解"路径可达性"概念，实时报错反而困惑
3. 离线校验更灵活——可以显示完整的校验报告（哪些字段不可达、哪些 Reducer 冲突）
4. 与现有 useGraphLocalValidation（8 种规则）整合，统一校验入口
5. 校验结果面板：
   ┌─ 校验结果 ─────────────────┐
   │ ✅ 结构检查通过              │
   │ ⚠️ 2 个字段不可达           │
   │   • Agent:Review 读取 approval，但上游无写入
   │   • HITL:Approve 读取 task_id，但上游无写入
   │ ✅ Reducer 无冲突           │
   └────────────────────────────┘
```

#### 决策 4：MCP 通过 Agent 间接使用

```
选择 A：Graph 增加 MCP 节点类型
选择 B：MCP 通过 Agent 间接使用（推荐）

推荐 B，理由：
1. 与现有架构一致——MCP 服务器在 Agent 级别配置
2. 避免引入新的节点类型（7 种已经够多）
3. Agent 选择器可以显示 MCP 策略信息（"此 Agent 配置了 3 个 MCP 服务器"）
4. 降低实现复杂度——不需要修改 CatalogToolResolver
```

#### 决策 5：节点折叠放宽约束

```
选择 A：严格 isMinimal 规则（≤1 output + ≤1 connected input 才能折叠）
选择 B：所有节点可折叠（推荐）

推荐 B，理由：
1. Langflow 的严格规则是因为 Handle 重叠问题，但 Aranea 的 State 字段端口
   可以在折叠时统一为一个 "READS" / "WRITES" 汇总端口
2. Agent 节点通常有多个 WRITES 字段，严格规则下几乎无法折叠
3. 折叠是空间管理需求，不应因端口数量受限
4. 折叠态视觉：
   ┌──────────────────────────┐
   │ [🤖] Agent:CS   R3 / W2  │  ← R3=读3字段, W2=写2字段
   │ ○ READS          ● WRITES│  ← 汇总端口
   └──────────────────────────┘
```

---

## 十二、可行性评估

> **结论：可行，无阻塞性技术障碍。** Vue Flow v1.48.2 提供所有核心能力，Quasar v2.19.3 覆盖全部 UI 组件需求，后端 API 全面就绪。主要工作量在前端组件重构和交互逻辑实现。

### 12.1 Vue Flow 能力评估

**当前版本**：`@vue-flow/core` v1.48.2 + `@vue-flow/background` v1.3.2 + `@vue-flow/controls` v1.1.3 + `@vue-flow/minimap` v1.5.4

| 方案所需能力 | Vue Flow 支持 | 当前项目使用 | 差距 |
|-------------|:------------:|:----------:|------|
| 自定义节点（组件化） | ✅ | ✅ GraphFlowNode + GraphFlowDiamond | 无 |
| 自定义边（组件化） | ✅ | ✅ GraphFlowEdge | 无 |
| 多端口 Handle（id 属性） | ✅ | ⚠️ 仅 Router 节点用了命名 Handle | 需扩展所有节点类型 |
| Handle 自定义渲染（slot） | ✅ | ❌ 未使用 | 需新增 Handle slot 内容 |
| `updateNodeInternals` | ✅ | ❌ 未使用 | 动态 Handle 变更时需调用 |
| `isValidConnection` | ✅ | ❌ 未使用 | 需实现连接验证逻辑 |
| 边重连（`edgeUpdate*` 事件） | ✅ | ✅ 已实现 `@edge-update` | 无 |
| Panel 组件（6 种定位） | ✅ | ❌ 未使用 | 需用于 CanvasControls |
| 动画边（内置 + 自定义） | ✅ | ✅ 自定义 `<animateMotion>` | 无 |
| 节点拖拽约束（extent/dragHandle） | ✅ | ⚠️ 仅 readOnly 开关 | 需增加 dragHandle |
| Minimap / Controls / Background | ✅ | ✅ 全部使用 | 无 |
| NodeToolbar 组件 | ❌ 无内置 | ❌ 用右键菜单替代 | **需自建**（非阻塞） |
| NodeResizer 组件 | ❌ 无内置 | ❌ 未使用 | 便签节点需自建（P2） |

**关键差距分析**：

1. **从 2 个 Handle 到多端口**（P0-1 核心改造）

   当前 GraphFlowNode 只有 2 个无名 Handle（target-left, source-right），需要改为按 State 字段动态生成多个命名 Handle。Vue Flow 完全支持——只需给 Handle 加 `id` 属性，在 Edge 上用 `sourceHandle`/`targetHandle` 引用，动态变更时调用 `updateNodeInternals()`。

2. **Handle 自定义渲染**（P0-1 端口显示字段名）

   Vue Flow Handle 组件支持默认 slot `{ id }`，可在 slot 中渲染字段名标签。这是实现"端口显示字段名"的核心机制。

3. **NodeToolbar 需自建**（P1-2）

   Vue Flow 不像 React Flow 那样内置 NodeToolbar 组件。需要自建一个浮动工具栏，定位逻辑基于选中节点的 DOM 位置。实现复杂度中等，不阻塞。

### 12.2 Quasar 组件能力评估

**当前版本**：Quasar v2.19.3 + @quasar/app-vite v2.6.0

| Langflow 组件（shadcn/ui） | Quasar 等价物 | 图编辑器已用 | 评估 |
|---------------------------|-------------|:----------:|------|
| Dropdown Menu | `q-menu` | ✅ | 直接等价，支持锚点定位 |
| Toggle/Switch | `q-toggle` | ✅ | 直接等价 |
| Tooltip | `q-tooltip` | ✅ | 支持 delay/anchor，Handle Tooltip 可用 |
| Popover（输出选择器） | `q-menu` | ✅ | 用 q-menu + q-list 实现 |
| Command（搜索下拉） | `q-select` + `use-input` | ✅ | 支持 filter/chips/multiple |
| Sheet/Sidebar（RunPanel） | `q-drawer` | ⚠️ 其他页面用了 | 支持 overlay/resize/mini |
| Badge | `q-badge` | ✅ | 支持 color/rounded/floating |
| Tabs | `q-tabs`/`q-tab-panels` | ✅ | 支持 dense/animated |

**8/8 组件有直接等价物，6/8 已在图编辑器中使用。**

**CSS/主题能力**：

| 能力 | 状态 | 说明 |
|------|------|------|
| CSS 自定义属性 | ✅ 已有 577+ 变量 | `_css-vars-light.sass` + `_css-vars-dark.sass` |
| 暗色模式 | ✅ 双主题 | 暖琥珀（浅色）/ 科技青（暗色） |
| 拖放 | ✅ 已实现 | HTML5 DnD + vuedraggable |
| 虚拟滚动 | ✅ 可用 | `q-virtual-scroll`，图编辑器未用但可引入 |
| 动画 | ✅ 已有 | 自定义 @keyframes（pulse/flow/expand） |

**Quasar 无阻塞性限制。** 唯一需注意：`q-select` 下拉不支持内置虚拟滚动，工具/Agent 列表超 200 项时需配合 `q-virtual-scroll` 或分页加载。

### 12.3 后端 API 就绪度评估

| 方案所需 API | 就绪？ | 端点 | 说明 |
|-------------|:-----:|------|------|
| Graph CRUD | ✅ | `POST/GET/PUT/DELETE /v1/graphs` | 完整 CRUD + 排序 |
| 运行 Graph | ✅ | `POST /v1/graphs/{id}/executions` | 返回 execution_id + status |
| 校验 Graph | ✅ | `POST /v1/graphs/{id}/validate` | 返回 errors[] + warnings[] |
| 获取执行状态 | ✅ | `GET /v1/graph/executions/{id}` | 含 current_state + steps[] |
| State 快照 | ✅ | `GET /v1/graph/executions/{id}/state-snapshot` | Checkpoint 级别 |
| Checkpoint 列表 | ✅ | `GET /v1/graph/executions/{id}/checkpoints` | TimeTravel 支持 |
| 编辑 State | ✅ | `POST /v1/graph/executions/{id}/edit-state` | 运行时 State 修补 |
| TimeTravel | ✅ | `POST /v1/graph/executions/{id}/time-travel` | 回溯到指定步骤 |
| 恢复执行 | ✅ | `ResumeGraph` | HITL 审批后恢复 |
| 取消执行 | ✅ | `CancelGraphExecution` | 取消运行中执行 |
| Agent 列表 | ✅ | `GET /v1/agents` | 含 agent_key/display_name/kind/agent_kind |
| Agent 有效工具 | ✅ | `GET /v1/agents/{id}/tools/effective` | 含 source/category/state |
| 工具列表 | ✅ | `GET /v1/tools` | 含 key/display_name/category/runtime_status/source |
| MCP 服务器列表 | ✅ | `GET /v1/mcp-servers` | 含 key/name/status/enabled |
| 版本管理 | ✅ | `ListGraphVersions` + `RollbackGraphVersion` | 版本回滚 |
| 模板系统 | ✅ | `ListGraphTemplates` + `CreateGraphFromTemplate` | 6 种内置模板 |
| 导入/导出 | ✅ | `ExportGraph` + `ImportGraph` | JSON 导出导入 |
| 任务管理 | ✅ | 完整 Task 生命周期 | Claim/Submit/Heartbeat/Review |

**WebSocket 事件**（运行时状态驱动）：

| 事件 | 就绪 | UI 映射 |
|------|:----:|---------|
| `graph_node_start` | ✅ | → `running` 状态 |
| `graph_node_end` | ✅ | → `success` 状态 |
| `graph_node_error` | ✅ | → `error` 状态 |
| `checkpoint`（interrupt） | ✅ | → `interrupted` 状态 |
| `graph_step` | ✅ | → 边动画波浪推进 |
| `state_delta` | ✅ | → State 面板实时更新 |
| `graph_execution_done` | ✅ | → 执行完成通知 |

**后端 API 全面就绪，无缺口。** 唯一需注意：Graph 执行事件通过 session WS 通道推送，前端需知道 execution 关联的 session_id。

### 12.4 当前前端与方案的差距

| 当前状态 | 方案目标 | 改造量 |
|---------|---------|--------|
| 2 个无名 Handle | 按 State 字段动态生成多端口 Handle | **高**（核心改造） |
| Handle 无渲染内容 | Handle slot 显示字段名 + 类型色标 | 中 |
| Agent 名称纯文本输入 | 分类选择器 + 搜索 + MCP 信息 | 中 |
| Tool 平铺 key 列表 | 分类选择器（系统/MCP/自定义） | 中 |
| 无连接验证 | 结构性检查 + 离线校验面板 | 中 |
| 无节点折叠 | 所有节点可折叠 + 汇总端口 | 中 |
| 无 NodeToolbar | 自建浮动工具栏（3 按钮 + 更多菜单） | 中 |
| 无 RunPanel | 侧边栏 RunPanel（执行/State/Checkpoint） | 中 |
| 5 种边类型色 | 5 种状态色 + 波浪动画 | 低 |
| 3 区段侧边栏 | 3 区段 + 快速模板 | 低 |

### 12.5 风险与缓解

| 风险 | 等级 | 缓解措施 |
|------|:----:|---------|
| 多端口 Handle 布局复杂 | 中 | 参考 Langflow 源码的 `groupHandlesByType()` + CSS Grid 布局 |
| NodeToolbar 自建定位 | 低 | 基于 Vue Flow 节点 DOM 位置 + Teleport |
| 大量工具/Agent 时 q-select 性能 | 低 | `q-virtual-scroll` + 分页加载 |
| Graph 事件需 session_id | 低 | ExecuteGraph 返回 execution_id，前端从执行上下文获取 session_id |
| Handle 霓虹脉冲动画性能 | 低 | CSS `@keyframes` + `will-change: box-shadow`，仅活跃 Handle 启用 |
| State Schema 变更时 Handle 动态更新 | 中 | `updateNodeInternals()` + watch State Schema 变化 |

### 12.6 总体评估

```
┌─────────────────────────────────────────────────────────────────┐
│                    可行性评估总结                                 │
├─────────────┬──────────┬────────────────────────────────────────┤
│   维度       │  就绪度   │  说明                                  │
├─────────────┼──────────┼────────────────────────────────────────┤
│ Vue Flow    │  ★★★★☆  │  核心能力齐全，NodeToolbar 需自建       │
│ Quasar      │  ★★★★★  │  全组件覆盖，主题系统成熟               │
│ 后端 API    │  ★★★★★  │  全面就绪，无缺口                      │
│ WebSocket   │  ★★★★☆  │  事件齐全，需处理 session_id 关联       │
│ 前端改造量   │  ★★★☆☆  │  P0 约 3000 行新代码，核心是 Handle    │
├─────────────┼──────────┼────────────────────────────────────────┤
│ 总体可行性   │  ★★★★☆  │  可行，无阻塞障碍                      │
└─────────────┴──────────┴────────────────────────────────────────┘
```

**核心结论**：

1. **Langflow 的设计思想和 UI 优点可以在本项目中实现**——Vue Flow + Quasar + 后端 API 三层能力均满足需求
2. **最大改造点是多端口 Handle**——从 2 个无名 Handle 到按 State 字段动态生成多端口，这是整个方案的核心
3. **唯一需自建的组件是 NodeToolbar**——Vue Flow 不内置，但实现复杂度不高
4. **后端零改造**——所有 API 已就绪，前端只需消费更多字段
5. **P0 预估 ~3000 行新代码**——4 个新增组件 + 8 个修改组件

---

## 十三、参考资料

- [Langflow GitHub](https://github.com/langflow-ai/langflow)
- [Langflow 官方文档 - Visual Editor](https://docs.langflow.org/concepts-overview)
- [Langflow 1.8 Release Notes](https://www.langflow.org/blog/langflow-1-8)
- [Langflow GenericNode 深度分析](https://deepwiki.com/langflow-ai/langflow/5.3-genericnode-component)
- [Langflow Connection Validation](https://deepwiki.com/langflow-ai/langflow/5.4-connection-validation)
- [React Flow 官方文档](https://reactflow.dev/learn/customization/custom-nodes)
- [Vue Flow 官方文档](https://vue-flow.dev/)
