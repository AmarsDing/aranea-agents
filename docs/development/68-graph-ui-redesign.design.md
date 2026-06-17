# 68 — Graph 编辑器 UI 重构 · 设计文档

---

## 1. 架构概览

### 1.1 当前架构

```
┌─────────────┬──────────────────────────┬──────────────────┐
│ NodePalette │     VueFlow Canvas       │ PropertyPanel    │
│ (7 种节点)   │  GraphFlowNode (2 Handle)│ (表单字段)        │
│ (2 分组)     │  GraphFlowDiamond        │ (Agent=纯文本)    │
│ (模板选择)   │  GraphFlowEdge (动画点)   │ (Tool=平铺列表)   │
│             │  Controls + MiniMap      │ (State Schema)   │
│             │  Background              │ (Validation)     │
└─────────────┴──────────────────────────┴──────────────────┘
```

### 1.2 目标架构

```
┌──────┬──────────────┬──────────────────────────┬──────────────┬──────────────┐
│ 图标  │ 侧边栏内容    │     VueFlow Canvas       │ PropertyPanel│  RunPanel    │
│ 条    │              │                          │  (浮动)      │  (可折叠)    │
│ 40px  │ 节点类型      │  State-Aware Node        │  资源选择器   │  执行控制     │
│      │ 版本历史      │  (多端口 Handle)          │  Agent选择器  │  State 快照   │
│ 🧩   │ 设置         │  (可折叠)                 │  Tool选择器   │  Checkpoint  │
│ 📜   │              │  5 种状态视觉              │  Func选择器   │  TimeTravel  │
│ 🔧   │              │  NodeToolbar (浮动)       │  State Schema│              │
│      │              │  连线引导 (Handle 过滤)    │  校验面板     │              │
└──────┴──────────────┴──────────────────────────┴──────────────┴──────────────┘
```

### 1.3 核心变化

| 维度 | 当前 | 目标 |
|------|------|------|
| Handle 数量 | 2 个无名（左入右出） | 按 State 字段动态生成多端口 |
| Handle 渲染 | 无内容（纯圆点） | slot 显示字段名 + 类型色标 |
| Agent 选择 | `q-input` 纯文本 | 分类选择器 + 搜索 + MCP 信息 |
| Tool 选择 | `q-select` 平铺 key | 分类选择器（系统/MCP/自定义） |
| 节点折叠 | 不支持 | 所有节点可折叠 + 汇总端口 |
| 运行时状态 | 4 种（running/completed/failed/interrupted） | 5 种（+idle）+ 边动画 |
| 执行面板 | 独立页面 | 编辑器内 RunPanel |
| 侧边栏 | 单区段 | 3 区段（节点类型/版本/设置） |
| 节点操作 | 右键菜单 | NodeToolbar + 右键菜单 |
| 连线引导 | 无 | Handle 字段名过滤 + 兼容高亮 |

## 2. 类型设计

### 2.1 新增类型

```typescript
// features/graph/types.ts — 新增

/** Handle 端口定义 */
interface PortDef {
  id: string           // 'r:messages' | 'w:response'
  fieldName: string    // State 字段名
  direction: 'read' | 'write'
  fieldType: string    // string/integer/float/boolean/array/object
  reducer?: string     // 仅 write 端口：default/append/cover/merge
  connected: boolean   // 是否已有边连接
}

/** 节点运行时状态 */
type NodeExecState = 'idle' | 'running' | 'success' | 'error' | 'interrupted'

/** 节点折叠状态 */
interface NodeCollapseState {
  collapsed: boolean
  /** 折叠前的 Handle 连接快照，展开时恢复 */
  preservedConnections?: Map<string, string>
}

/** 资源选择器选项 */
interface ResourceOption {
  value: string        // agent_key / tool_key / func_ref
  label: string        // display_name
  group: string        // 分组名（自建Agent/系统Agent/系统工具/MCP工具/...）
  icon?: string
  description?: string
  disabled?: boolean   // 不可用（如 MCP 工具需 session 上下文）
  metadata?: Record<string, unknown>  // 额外信息（如 MCP 策略）
}
```

### 2.2 修改类型

```typescript
// EdgeDef 增加 Handle 引用
interface EdgeDef {
  from: string
  to: string
  kind: string
  sourceHandle?: string  // 新增：'w:messages'
  targetHandle?: string  // 新增：'r:messages'
}

// NodeDef 增加计算字段（不序列化到 Proto）
interface NodeDef {
  // ... 现有 23 个字段不变 ...

  // 以下为前端计算字段，不发送到后端
  ports?: PortDef[]           // 根据 State Schema + 节点类型计算
  execState?: NodeExecState   // 运行时状态
  collapsed?: boolean         // 折叠状态
}
```

### 2.3 Handle ID 编码方案

> **实现现状**：当前代码（`features/graph/portTypes.ts`）使用 **3-part 编码**，包含 nodeId 以支持跨节点唯一性。以下为实际实现规范。

```
格式：{direction}:{fieldName}:{nodeId}
方向：r = read (target Handle, 左侧)
     w = write (source Handle, 右侧)

注意：Handle ID 包含 nodeId 以确保跨节点唯一性。
字段名可能包含冒号，因此解码时按首尾冒号分割。

示例：
  r:messages:node-1     — node-1 读取 messages 字段（左侧 Handle）
  w:response:node-1     — node-1 写入 response 字段（右侧 Handle）

折叠态汇总 Handle：
  __reads        — 汇总读取端口
  __writes       — 汇总写入端口
```

**编码/解码函数**（已实现于 `features/graph/portTypes.ts`）：

```typescript
export function encodeHandleId(port: PortInfo): string {
  const prefix = port.direction === 'reads' ? 'r' : 'w';
  return `${prefix}:${port.field}:${port.nodeId}`;
}

export function decodeHandleId(id: string): PortInfo {
  // 按首尾冒号分割，支持字段名包含冒号
  const firstColon = id.indexOf(':');
  const lastColon = id.lastIndexOf(':');
  // ... 返回 { direction, field, fieldType, nodeId }
}
```

## 3. 组件设计

### 3.1 GraphFlowNode 重构

```
┌─────────────────────────────────────────────────────┐
│ [🤖] Agent: CustomerService              [▼] [✓]   │ ← Header
│ ─────────────────────────────────────────────────── │ ← 色条
│                                                     │
│ ○ 读取: messages          ● 写入: response          │ ← READS/WRITES 区域
│ ○ 读取: step              ● 写入: approval          │
│                                                     │
│ 🔧 web_search, file (+3)                           │ ← Tool chips
│ 💬 处理客户咨询并生成回复                              │ ← Description
└─────────────────────────────────────────────────────┘

折叠态：
┌──────────────────────────────────┐
│ [🤖] Agent:CS      R2 / W2  [▼] │ ← R2=读2字段, W2=写2字段
│ ○ READS           ● WRITES      │ ← 汇总端口
└──────────────────────────────────┘
```

**核心改造点**：

1. Handle 从 2 个无名 → 动态多端口（`v-for` 渲染 `data.ports`）
2. Handle 使用默认 slot 渲染字段名标签
3. 折叠/展开切换（双击 Header）
4. 折叠态显示汇总端口（`__reads` / `__writes`）
5. 5 种状态视觉（CSS class 切换）

**Props 变更**：

```typescript
// 新增 data 字段
interface GraphFlowNodeData {
  // ... 现有字段 ...
  ports: PortDef[]          // 新增：端口列表
  execState: NodeExecState  // 新增：运行时状态
  collapsed: boolean        // 新增：折叠状态
}
```

**Langflow 精确尺寸规范**：

| 属性 | 展开态 | 折叠态 |
|------|--------|--------|
| 宽度 | `w-80` = 320px | `w-48` = 192px |
| 圆角 | `rounded-xl` = 12px | `rounded-xl` = 12px |
| 边框 | 1px solid + ring | 1px solid + ring |
| 阴影(默认) | `shadow-sm` = `0 1px 2px 0 rgb(0 0 0 / 0.05)` | 同左 |
| 阴影(hover) | `shadow-md` = `0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)` | 同左 |
| 背景 | `bg-background` = light #FFFFFF, dark #191A1D | 同左 |
| 过渡 | `transition-all` | 同左 |

**Header 精确规范**：

| 属性 | 值 |
|------|-----|
| 内边距 | `px-4 py-3` = 16px 水平, 12px 垂直 |
| 底部边框(展开) | `border-b` 1px |
| 图标尺寸 | `h-4.5 w-4.5` = 18px × 18px |
| 图标与名称间距 | `ml-3` = 12px |
| 名称字体 | `text-base font-medium` = 16px, weight 500 |
| 名称颜色 | `text-primary` |
| 间距 | `gap-2` = 8px |

**Legacy Badge**：字体 `text-xxs` = 11px，圆角 `rounded-sm` = 4px，边框 1px `border-accent-amber`，文字色 `text-accent-amber-foreground`，内边距 `px-1` = 4px

**Beta Badge**：尺寸 `h-4 w-4` = 16px × 16px，圆角 `rounded-sm` = 4px，边框 1px `border-accent-purple-foreground`，内边距 `p-0.5` = 2px

**Run Button**：尺寸 `h-6 w-6` = 24px × 24px，圆角 `rounded-sm` = 4px，背景 transparent/hover `bg-muted`，图标 `h-3.5 w-3.5` = 14px × 14px，图标色 `text-muted-foreground`/hover `text-foreground`，building 态 `animate-spin`，building hover `text-status-red`

**Edit Button**：尺寸 `h-6 w-6` = 24px × 24px，圆角 `rounded-md` = 6px，展开定位 `top-2 translate-x-[10.4rem]` = top 8px right 166.4px，折叠定位 `top-0 translate-x-[6.4rem]` = top 0px right 102.4px，默认背景 `bg-zinc-foreground`，激活背景 `bg-accent-emerald`，图标 `h-[18px] w-[18px]` = 18px × 18px

**节点状态边框**：

| 状态 | 边框 | Ring |
|------|------|------|
| 默认(未选中) | `border` 1px | `ring-[0.5px] ring-border` |
| 选中(非building) | `border` 1px | `ring-[0.75px] ring-muted-foreground` |
| BUILDING | `border-[1px]` | `ring-[0.75px] ring-foreground` |
| ERROR | `border-[1px]` | `ring-[0.75px] ring-destructive` |
| INACTIVE | `border-none` | `ring grayscale` |
| Frozen(选中) | `border-2 border-frozen-blue` | `shadow-frozen-ring` = `0 0 10px 2px rgba(128, 190, 230, 0.5)` + `::before` inset -2px |
| Frozen(未选中) | 默认 | `shadow-frozen-ring` = `0 0 10px 2px rgba(128, 190, 230, 0.5)` |
| Frozen overlay | — | `rgba(255, 255, 255, 0.5)` blur(5px) + blur(10px) opacity 0.2 |

**Wiggle 动画（BUILDING 状态）**：关键帧 0%/100% scale(100%), 50% scale(120%)，时长 150ms，缓动 ease-in-out，迭代 1 次

### 3.2 GraphFlowEdge 增强

**变更**：

1. 边样式根据 sourceHandle/targetHandle 的字段类型着色
2. 运行时边动画（波浪推进）—— 已有 `<animateMotion>` 基础，增加状态驱动
3. 循环边使用虚线（`strokeDasharray="5 5"`）

**Langflow 精确边规范**：

| 边状态 | 描边宽度 | 描边颜色 |
|--------|----------|----------|
| 默认 | 2px | `var(--connection)` = light #555, dark #6d6c6c |
| 选中 | 2px | `var(--selected)` = light #2196f3, dark #0369a1 |
| 运行中 | 2px | `hsl(var(--foreground))` |
| 非运行 | 1px | `hsl(var(--foreground))` |
| 循环边 | 2px | `strokeDasharray="5 5"` |

**边命中区域**：描边宽度 20（不可见），描边不透明度 0

**连接线**：描边宽度 2，颜色 `hsl(var(--datatype-{color}))`（数据类型色），CSS class `animated`（行军蚁），端点圆半径 5px，填充 #fff，描边 = 数据类型色，描边宽度 1.5

### 3.3 GraphPropertyPanel 重构

**核心变更**：Agent/Tool/Function 字段替换为资源选择器

```
┌─ 属性面板 ─────────────────────┐
│ 基本信息                        │
│   节点类型: [Agent ▼]           │
│   描述: [处理客户咨询...]        │
│                                 │
│ 资源配置                        │
│   Agent: [CustomerService ▼]   │ ← GraphAgentSelector
│   工具列表:                     │
│   [web_search] [file] [+添加]  │ ← GraphToolSelector
│   模型: [gpt-4o ▼]            │ ← GraphModelSelector (可选)
│                                 │
│ MCP 策略                        │ ← 选中 Agent 后显示
│   ✅ mcp_tool_set (3 个服务器)  │
│   ✅ mcp_broker                 │
│                                 │
│ 中断与审批                       │
│   ...                           │
│                                 │
│ 高级选项                         │
│   ...                           │
└─────────────────────────────────┘
```

### 3.4 GraphRunPanel（新增）

```
┌─ RunPanel ─────────────────────┐
│ [▶ 运行] [⏸ 暂停] [⏹ 停止]    │ ← 执行控制
├────────────────────────────────┤
│ [State] [Checkpoint] [Task]    │ ← q-tabs
├────────────────────────────────┤
│ State 快照:                     │
│   messages: [...] (3条)        │
│   step: 2                      │
│   approval: "pending"          │
│   response: "处理中..."         │
├────────────────────────────────┤
│ Checkpoint 列表:                │
│   #3 step=2 [TimeTravel]       │
│   #2 step=1 [TimeTravel]       │
│   #1 step=0 [TimeTravel]       │
└─────────────────────────────────┘
```

**Langflow RunPanel 精确规范**：

| 属性 | 值 |
|------|-----|
| 默认宽度 | 400px |
| 最小宽度 | 200px |
| 最大宽度 | 800px |
| 可调整大小 | 是 |
| 全屏 | 支持 |
| 过渡时长 | 0.3s（非调整大小），0s（调整大小中） |
| 过渡缓动 | easeInOut |
| 过渡属性 | width, x (滑动), opacity |

**Chat Input**：圆角 `rounded-md` = 6px，边框 1px `border-input`，背景 `bg-muted`，内边距 `p-3` = 12px，hover 边框 `hover:border-muted-foreground`，focus 边框 `focus-within:border-primary`，最小高度 24px，最大高度 200px

**Bot Message**：圆角 `rounded-md` = 6px，内边距 `px-2 py-3` = 8px 水平 12px 垂直，hover `hover:bg-muted`，头像 `h-6 w-6` = 24px，名称 `text-sm font-medium` = 14px weight 500

**User Message**：背景 `bg-muted`（<45rem）/ transparent（≥45rem），头像 `h-6 w-6` = 24px

### 3.5 GraphResourceSelector（新增）

通用资源选择器组件，支持分组、搜索、验证。

```typescript
// Props
interface GraphResourceSelectorProps {
  modelValue: string | string[]  // 单选=string, 多选=string[]
  options: ResourceOption[]
  placeholder: string
  multiple?: boolean
  groupBy?: string               // 分组字段名
  searchable?: boolean           // 是否支持搜索
  showMcpInfo?: boolean          // 是否显示 MCP 信息（Agent 选择器）
}
```

### 3.6 GraphNodeToolbar（新增）

```
                    ┌──────────────────────────┐
                    │ [▶] [❄] [🗑] [⋯]        │  ← 节点上方
                    └──────────────────────────┘
```

**定位逻辑**：基于选中节点的 DOM 位置 + Teleport 到 VueFlow 容器

**Langflow NodeToolbar 精确规范**：

| 属性 | 值 |
|------|-----|
| 容器高度 | `h-10` = 40px |
| 容器圆角 | `rounded-xl` = 12px |
| 容器边框 | 1px `border-border` |
| 容器背景 | `bg-background` |
| 容器内边距 | `p-1` = 4px |
| 容器阴影 | `shadow-sm` = `0 1px 2px 0 rgb(0 0 0 / 0.05)` |
| 按钮内边距 | `py-[6px] px-[6px]` |
| 按钮圆角 | `rounded-md` = 6px |
| 按钮图标 | `h-4 w-4` = 16px |
| 按钮标签 | `text-mmd font-medium` = 13px, weight 500 |
| More 按钮 | `h-[2rem] w-[2rem]` = 32px × 32px |

### 3.7 GraphStatePanel（新增，P1-4）

```
┌─ State 面板 ────────────────┐
│ messages   [Message[]]      │
│   ← Agent:CS (写入)         │
│   → Agent:Review (读取)     │
│ step       [integer]        │
│   ← Agent:CS (写入)         │
│ approval   [string]         │
│   ← HITL:Approve (写入)     │
└─────────────────────────────┘
```

## 4. 数据流设计

### 4.1 State Schema → Handle 生成

> **实现现状**：端口计算逻辑已实现于 `features/graph/portTypes.ts` 的 `getNodePorts()` 函数。实际实现比原始设计更精细——根据节点类型的不同配置（inputMapperJson/outputMapperJson/instruction 模板）推导端口，而非简单的"读写全部字段"。

```
GraphDefinition.stateFields[]
        │
        ▼
  getNodePorts(node, stateFields)  ← features/graph/portTypes.ts
        │
        ├── Agent 节点: reads=inputMapperJson 的 values, writes=outputMapperJson 的 keys
        ├── LLM 节点: reads=instruction 模板中的 ${field} 引用, writes=['response']
        ├── Tool 节点: reads=[], writes=[]（前端无法内省工具签名）
        ├── Function 节点: reads=[], writes=[]（前端无法内省 Go 函数签名）
        ├── Router 节点: reads=[], writes=[]（前端无法确定 condFuncRef 读取字段）
        ├── Join 节点: reads=[], writes=[]（透传）
        └── HITL 节点: reads=[], writes=[]（前端无法确定审批字段）
        │
        ▼
  PortInfo[] → 渲染 Handle 列表
```

> **设计说明**：原始设计假设"所有可执行节点读写全部 State 字段"，但实际实现发现前端无法内省 Tool/Function/Router/HITL 的字段读写范围，因此这些节点返回空端口（使用默认 Handle）。Agent 和 LLM 节点通过解析配置（inputMapperJson/outputMapperJson/instruction 模板）推导端口。后续可通过 PropertyPanel 手动配置节点的具体读写字段范围（P2 功能）。

### 4.2 WS 事件 → 节点状态

> **实现现状**：事件→状态映射已实现于 `features/graph/runtime/useGraphExecutionStream.ts`。实际使用的状态值与原始设计略有差异——使用 `completed` 而非 `success`，使用 `failed` 而非 `error`。

```
WS Event Bus  ← features/graph/runtime/useGraphExecutionStream.ts
    │
    ├── graph_node_start → { nodeId, stepNumber }
    │       └── execNodeStates[nodeId] = 'running'
    │
    ├── graph_node_end → { nodeId, durationNs }
    │       └── execNodeStates[nodeId] = 'completed'
    │
    ├── graph_node_error → { nodeId, error }
    │       └── execNodeStates[nodeId] = 'failed'
    │
    ├── checkpoint (interrupt) → { nodeId, interruptKey }
    │       └── execNodeStates[nodeId] = 'interrupted'
    │
    └── graph_execution_done
            └── 重置所有节点状态
```

**实际状态值映射**（`features/graph/types.ts` EXECUTION_STATUS_STYLES）：

| 状态值 | 颜色 | 图标 | 标签 |
|--------|------|------|------|
| `idle` | grey | radio_button_unchecked | 等待 |
| `running` | cyan | sync | 运行中 |
| `completed` | emerald | check_circle | 完成 |
| `failed` | pink | error | 失败 |
| `interrupted` | amber | pause_circle | 中断 |
| `waiting` | grey-6 | schedule | 等待 |

### 4.3 资源选择器数据流

```
useGraphEditorPage  ← features/graph/useGraphEditorPage.ts
    │
    ├── loadAvailableAgents() → useAgentsCatalogStore.listAgents()
    │       └── agents → ResourceOption[] (grouped by kind)
    │
    ├── loadAvailableTools() → useToolsStore.loadTools()
    │       └── tools → ResourceOption[] (grouped by category + source)
    │
    ├── loadAgentMcpInfo(agentKey) → getAgentEffectiveTools(agentKey)
    │       └── effectiveTools → { mcpServers, toolCount }
    │
    └── 传入 GraphPropertyPanel → GraphAgentSelector / GraphToolSelector
```

## 5. CSS 变量扩展（Langflow 对齐）

> 所有 CSS 变量使用 HSL 通道格式，消费方式为 `hsl(var(--token))`。Light/Dark 双模式完整定义。实际定义文件：`web/src/css/theme/_graph-pages.sass`

```scss
// _graph-pages.sass — Langflow 完整色彩体系

// ============================================================
// 主色 (Primary Colors)
// ============================================================
--primary: 0 0% 0%                          // light: #000 | dark: 0 0% 100% (#FFF)
--primary-foreground: 0 0% 100%              // light: #FFF | dark: 0 0% 0% (#000)
--background: 0 0% 100%                      // light: #FFF | dark: 240 6% 10% (#191A1D)
--foreground: 0 0% 0%                        // light: #000 | dark: 0 0% 100% (#FFF)
--border: 240 6% 90%                         // light: #E2E2E6 | dark: 240 5% 26% (#3E3E42)
--muted: 240 5% 96%                          // light: #F4F4F7 | dark: 240 4% 16% (#24242A)
--muted-foreground: 240 4% 46%               // light: #737380 | dark: 240 5% 65% (#9696A5)
--destructive: 0 72% 51%                     // light: #E23636 | dark: 0 84% 60% (#E83636)

// ============================================================
// 强调色变体 (Accent Variants)
// ============================================================
--accent-amber-foreground: 26 90.5% 37.1%    // light | dark: 45.9 96.7% 64.5%
--accent-emerald-foreground: 161 94% 30%     // light | dark: 158 64% 52%
--accent-indigo-foreground: 243 75% 59%      // light | dark: 234 89% 74%
--accent-purple-foreground: 271 81% 56%      // light | dark: 270 95% 75%
--accent-red-foreground: 0 72% 51%           // light | dark: 0 91% 71%

// ============================================================
// 状态色 (Status Colors) — 非 HSL 通道，直接使用 HEX
// ============================================================
--status-red: #ef4444
--status-yellow: #eab308
--status-green: #4ade80
--status-blue: #2563eb
--status-gray: #6b7280

// ============================================================
// 特殊色 (Special Colors)
// ============================================================
--selected: #2196f3                          // light | dark: #0369a1
--connection: #555                           // light | dark: #6d6c6c
--canvas: 240 5% 96%                         // light | dark: 0 0% 0% (#000)
--canvas-dot: 240 5% 65%                     // light | dark: 240 5.3% 26.1%
--tooltip: 0 0% 0%                           // light: #000 | dark: 0 0% 100% (#FFF)
--tooltip-foreground: 0 0% 100%              // light: #FFF | dark: 0 0% 0% (#000)
--node-ring: 240 6% 90%                      // both modes
--frozen-blue: rgba(128, 190, 219, 0.86)

// ============================================================
// 14 种数据类型色 (Datatype Colors) — HSL 通道
// ============================================================
--datatype-pink: 333.3 71.4% 50.6%           // light | dark: 327.4 87.1% 81.8%
--datatype-rose: 346.8 77.2% 49.8%           // light | dark: 352.6 95.7% 81.8%
--datatype-yellow: 40.6 96.1% 40.4%          // light | dark: 50.4 97.8% 63.5%
--datatype-blue: 221.2 83.2% 53.3%           // light | dark: 211.7 96.4% 78.4%
--datatype-gray: 215 13.8% 34.1%             // light | dark: 216 12.2% 83.9%
--datatype-lime: 84.8 85.2% 34.5%            // light | dark: 82 84.5% 67.1%
--datatype-red: 0 72.2% 50.6%                // light | dark: 0 93.5% 81.8%
--datatype-violet: 262.1 83.3% 57.8%         // light | dark: 252.5 94.7% 85.1%
--datatype-emerald: 161.4 93.5% 30.4%        // light | dark: 156.2 71.6% 66.9%
--datatype-fuchsia: 293.4 69.5% 48.8%        // light | dark: 291.1 93.1% 82.9%
--datatype-purple: 271.5 81.3% 55.9%         // light | dark: 268.6 100% 91.8%
--datatype-cyan: 191.6 91.4% 36.5%           // light | dark: 187 92.4% 69%
--datatype-indigo: 243.4 75.4% 58.6%         // light | dark: 229.7 93.5% 81.8%
--datatype-orange: 20.5 90.2% 48.2%          // light | dark: 20.5 90.2% 48.2%

// ============================================================
// 端口类型色（映射到 14 种数据类型色）
// ============================================================
--graph-port-string: var(--datatype-blue)
--graph-port-integer: var(--datatype-indigo)
--graph-port-float: var(--datatype-violet)
--graph-port-boolean: var(--datatype-yellow)
--graph-port-array: var(--datatype-emerald)
--graph-port-object: var(--datatype-purple)

// ============================================================
// 端口状态色
// ============================================================
--graph-port-connected: hsl(var(--datatype-blue))
--graph-port-compatible: hsl(var(--datatype-emerald))   // 兼容高亮
--graph-port-incompatible: hsl(var(--datatype-gray))     // 不兼容灰化
--graph-port-muted: hsl(var(--muted-foreground))         // 折叠态汇总端口

// ============================================================
// 节点状态色（5 种）
// ============================================================
--graph-status-idle: transparent
--graph-status-running: hsl(var(--foreground))
--graph-status-success: hsl(var(--accent-emerald-foreground))
--graph-status-error: hsl(var(--destructive))
--graph-status-interrupted: #eab308

// ============================================================
// Handle 霓虹脉冲
// ============================================================
--graph-handle-glow-duration: 1.1s
```

**Dark 模式覆盖**（在 `[data-theme="dark"]` 或 `.body--dark` 下）：

```scss
[data-theme="dark"]
  --primary: 0 0% 100%
  --primary-foreground: 0 0% 0%
  --background: 240 6% 10%          // #191A1D
  --foreground: 0 0% 100%
  --border: 240 5% 26%              // #3E3E42
  --muted: 240 4% 16%               // #24242A
  --muted-foreground: 240 5% 65%    // #9696A5
  --destructive: 0 84% 60%          // #E83636
  --accent-amber-foreground: 45.9 96.7% 64.5%
  --accent-emerald-foreground: 158 64% 52%
  --accent-indigo-foreground: 234 89% 74%
  --accent-purple-foreground: 270 95% 75%
  --accent-red-foreground: 0 91% 71%
  --selected: #0369a1
  --connection: #6d6c6c
  --canvas: 0 0% 0%
  --canvas-dot: 240 5.3% 26.1%
  --tooltip: 0 0% 100%
  --tooltip-foreground: 0 0% 0%
  --datatype-pink: 327.4 87.1% 81.8%
  --datatype-rose: 352.6 95.7% 81.8%
  --datatype-yellow: 50.4 97.8% 63.5%
  --datatype-blue: 211.7 96.4% 78.4%
  --datatype-gray: 216 12.2% 83.9%
  --datatype-lime: 82 84.5% 67.1%
  --datatype-red: 0 93.5% 81.8%
  --datatype-violet: 252.5 94.7% 85.1%
  --datatype-emerald: 156.2 71.6% 66.9%
  --datatype-fuchsia: 291.1 93.1% 82.9%
  --datatype-purple: 268.6 100% 91.8%
  --datatype-cyan: 187 92.4% 69%
  --datatype-indigo: 229.7 93.5% 81.8%
  --datatype-orange: 20.5 90.2% 48.2%
```

## 6. Langflow 视觉规范完全对齐

> 本节为 Langflow UI 的像素级精确规范，所有实现必须严格遵循。

### 6.1 Handle 规格

**点击区域**：

| 属性 | 值 |
|------|-----|
| 尺寸 | 32px × 32px |
| 定位 | absolute, top 50% |
| 背景 | transparent（不可见命中区域） |
| z-index | 30 |

**可见圆点**：

| 属性 | 值 |
|------|-----|
| 正常尺寸 | 10px × 10px |
| 静默尺寸 | 6px × 6px（opacity 0，实际不可见） |
| Null Handle 背景 | `hsl(var(--border))` |
| Null Handle 边框 | `2px solid hsl(var(--muted))` |
| 圆角 | 50%（正圆） |
| 过渡 | `all 0.2s`（200ms） |
| 定位 | 在 32px 点击区域内居中，absolute + translate |

**霓虹脉冲动画**：

- 关键帧按节点动态生成：`pulseNeon-{nodeId}`
- 0%（及 100%）：
  ```
  box-shadow: 0 0 0 3px hsl(var(--node-ring)),
              0 0 2px {color}, 0 0 4px {color}, 0 0 6px {color},
              0 0 8px {color}, 0 0 10px {color}, 0 0 15px {color}, 0 0 20px {color}
  ```
- 50%：
  ```
  box-shadow: 0 0 0 3px hsl(var(--node-ring)),
              0 0 4px {color}, 0 0 8px {color}, 0 0 12px {color},
              0 0 16px {color}, 0 0 20px {color}, 0 0 25px {color}, 0 0 30px {color}
  ```
- 时长：1.1s
- 缓动：ease-in-out
- 迭代：infinite

**静态霓虹阴影（active/hovered）**：

```
box-shadow: 0 0 0 1px hsl(var(--border)),
            0 0 2px {color}, 0 0 4px {color}, 0 0 6px {color},
            0 0 8px {color}, 0 0 10px {color}, 0 0 15px {color}, 0 0 20px {color}
```

**Handle 定位 CSS**：

```css
/* 右侧 Handle */
.right_handle {
  right: 0 !important;
  transform: translate(50%, -50%) !important;
}

/* 左侧 Handle */
.left_handle {
  left: 0 !important;
  transform: translate(-50%, -50%) !important;
}
```

### 6.2 Sidebar 规格

| 属性 | 值 |
|------|-----|
| 展开宽度 | 19rem (304px) |
| 折叠/图标宽度 | 4rem (64px) |
| 分段图标宽度 | 40px |
| 过渡 | `transition-[width] duration-300 ease-in-out` |

**组件卡片**：

| 属性 | 值 |
|------|-----|
| 圆角 | `rounded-md` = 6px |
| 背景 | `bg-muted`，hover: `bg-secondary-hover/75` |
| 内边距 | `p-1 px-2` = 4px 全方向 + 8px 水平 |
| 左侧色条 | inline style `borderLeftColor` |
| 图标 | `h-[18px] w-[18px]` = 18px |
| 文字 | `truncate text-sm font-normal` = 14px |
| Plus 按钮图标 | `h-4 w-4` = 16px（sm 下 opacity 0，hover 时 100） |
| Grip 图标 | `h-4 w-4` = 16px |
| 项目间距 | `gap-1` = 4px |

**搜索输入**：

| 属性 | 值 |
|------|-----|
| 字体 | `text-sm` = 14px |
| 圆角 | `rounded-lg` = 8px |
| 背景 | `bg-background` |

### 6.3 FlowToolbar 规格（右上角）

**容器**：

| 属性 | 值 |
|------|-----|
| 高度 | `h-11` = 44px |
| 间距 | `gap-7` = 28px |
| 圆角 | `rounded-md` = 6px |
| 边框 | 1px |
| 背景 | `bg-background` |
| 内边距 | `px-1.5` = 6px |
| 阴影 | `shadow` |

**Playground 按钮**：

| 属性 | 值 |
|------|-----|
| 尺寸 | `h-8 w-[7.2rem]` = 32px 高 × 115.2px 宽 |
| 间距 | `gap-1.5` = 6px |
| 字体 | `text-sm` = 14px |

### 6.4 CanvasControls 规格（底部居中）

**容器**：

| 属性 | 值 |
|------|-----|
| 圆角 | `rounded-lg` = 8px |
| 背景 | `bg-background` |
| 内边距 | `px-2 py-1` = 8px 水平, 4px 垂直 |
| 间距 | `gap-1` = 4px |

**单个按钮**：

| 属性 | 值 |
|------|-----|
| 尺寸 | `h-8 w-8` = 32px × 32px |
| 圆角 | `rounded-md` = 6px |
| Hover | `hover:bg-muted` |
| 图标尺寸 | 18px (assistant/note), 20px (inspector/zoom) |

### 6.5 Build Progress Indicator 规格

| 属性 | 值 |
|------|-----|
| 定位 | absolute, bottom 64px, center |
| 宽度 | 530px |
| 圆角 | `rounded-lg` = 8px |
| 边框 | 1px |
| 背景 | `bg-background` |
| 内边距 | `px-4 py-2` = 16px 水平, 8px 垂直 |
| 阴影 | `shadow-md` |
| 字体 | `text-sm` = 14px |
| 错误态 | 边框/文字 `accent-red-foreground` |
| 成功态 | 边框/文字 `accent-emerald-foreground` |
| 自动消失 | 2000ms |

### 6.6 Tooltip 规格

| 属性 | 值 |
|------|-----|
| 背景 | `bg-tooltip` = light #000, dark #FFF |
| 文字 | `text-tooltip-foreground` = light #FFF, dark #000 |
| 字体 | `text-xs` = 12px |
| 圆角 | `rounded-md` = 6px |
| 边框 | 1px |
| 内边距 | `px-3 py-1.5` = 12px 水平, 6px 垂直 |
| 阴影 | `shadow-md` |
| 侧偏移 | 4px |
| 最大宽度 | 384px (`max-w-96`) |
| 最大高度 | 25vh |
| 延迟 | 1000ms (Handle), 500ms (默认) |

### 6.7 Typography

| 属性 | 值 |
|------|-----|
| Sans 字体 | "Inter", sans-serif |
| Mono 字体 | "JetBrains Mono", monospace |
| 自定义尺寸 xxs | 11px |
| 自定义尺寸 mmd | 13px |
| 基础圆角 --radius | 0.5rem (8px) |

### 6.8 Scrollbar

| 属性 | 值 |
|------|-----|
| 宽度/高度 | 8px |
| 轨道 | `hsl(var(--muted))` |
| 滑块 | `hsl(var(--border))` |
| 滑块 hover | `hsl(var(--placeholder-foreground))` |
| 滑块圆角 | 999px (pill) |

### 6.9 动画时序

| 动画 | 时长 | 缓动 | 迭代 |
|------|------|------|------|
| overlayShow | 400ms | cubic-bezier(0.16, 1, 0.3, 1) | 1 |
| overlayHide | 500ms | cubic-bezier(0.16, 1, 0.3, 1) | 1 |
| contentShow | 400ms | cubic-bezier(0.16, 1, 0.3, 1) | 1 |
| contentHide | 500ms | cubic-bezier(0.16, 1, 0.3, 1) | 1 |
| wiggle | 150ms | ease-in-out | 1 |
| slow-wiggle | 500ms | ease-in-out | 1 |
| jiggle | 150ms | ease-in-out | infinite |
| pulse-pink | 2s | linear | infinite |
| Sidebar 宽度 | 300ms | ease-in-out | — |
| Panel 滑动 | 300ms | easeInOut | — |

### 6.10 Frozen 状态完整规范

**Frozen 选中态**：
```css
.frozen-selected {
  border: 2px solid rgba(128, 190, 219, 0.86);  /* --frozen-blue */
  box-shadow: 0 0 10px 2px rgba(128, 190, 230, 0.5);  /* shadow-frozen-ring */
}
.frozen-selected::before {
  content: '';
  position: absolute;
  inset: -2px;
  /* additional ring pseudo-element */
}
```

**Frozen 未选中态**：
```css
.frozen-unselected {
  box-shadow: 0 0 10px 2px rgba(128, 190, 230, 0.5);  /* shadow-frozen-ring */
}
```

**Frozen 叠加层**：
```css
.frozen-overlay {
  background: rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(5px);
  filter: blur(10px);
  opacity: 0.2;
}
```

## 7. 技术约束

> 以下技术约束从需求文档迁移，属于实现层面的约束规范。

### 7.1 P0-1 State-Aware 节点技术约束

- Vue Flow Handle 支持 `id` 属性和默认 slot，可渲染字段名标签
- `updateNodeInternals()` 在 Handle 动态变更时必须调用
- Handle id 编码方案：`{direction}:{fieldName}:{nodeId}`（3-part，详见 §2.3），避免特殊字符

### 7.2 P0-3 RunPanel 技术约束

- 复用现有 `useGraphExecutionStream`（`features/graph/runtime/useGraphExecutionStream.ts`）和 `useGraphTimeTravel`（`features/graph/runtime/useGraphTimeTravel.ts`）composable
- RunPanel 使用 `q-drawer` 或自定义 flex 面板

### 7.3 边重连行为

```
Langflow 行为：无效重连 → 删除边
Aranea 行为：无效重连 → 恢复原边（更安全）

实现：在 edgeUpdateEnd 事件中：
1. 检查新连接是否有效（isValidConnection）
2. 有效 → 更新边的 source/target/sourceHandle/targetHandle
3. 无效 → 恢复原始边数据（不删除）
```

### 7.4 画布执行锁定

```
执行时：
  - nodesDraggable = false
  - nodesConnectable = false
  - elementsSelectable = false（可选，保留选择查看状态）
  - 显示执行横幅（CanvasBadge + Loader2 旋转）
  - 快捷键禁用（Delete/Ctrl+D 等）

执行完成后：
  - 恢复所有交互
  - 移除横幅（350ms 退出动画）
```

## 8. API 与数据源

> 以下 API/数据源映射从需求文档迁移，属于实现层面的接口契约。

### 8.1 资源选择器数据源映射

| 选择器 | 前端 API 函数 | Store | 已有？ |
|--------|--------------|-------|--------|
| Agent | `listAgents()` | `useAgentsCatalogStore` | 是，需增加 Kind 分组 |
| Tool | `listTools()` | `useToolsStore` | 是，需增加 Category 分组 |
| Function | `listTools()` (filter) | `useToolsStore` | 复用 Tool API |
| Agent MCP | `getAgentEffectiveTools()` | 新建 | API 已有，需封装 |

### 8.2 Graph API 函数映射

> 实际实现位于 `web/src/features/graph/api.ts`

| 功能 | 前端 API 函数 | 说明 |
|------|--------------|------|
| 执行 Graph | `executeGraph()` | 启动图执行 |
| 恢复执行 | `resumeGraph()` | 恢复中断的执行 |
| 取消执行 | `cancelGraphExecution()` | 取消正在运行的执行 |
| 状态快照 | `getStateSnapshot()` | 获取当前 State 快照 |
| 编辑状态 | `editState()` | 修改 State 字段 |
| 时间旅行 | `timeTravelGraph()` | 回退到指定 Checkpoint |
| 图校验 | `validateGraph()` | 校验图结构合法性 |
| 列出 Checkpoint | `listCheckpoints()` | 获取 Checkpoint 列表 |

### 8.3 数据源分组依据

| 选择器 | API | 分组依据 |
|--------|-----|---------|
| Agent | `listAgents()` | Kind（user/system_builtin/ecosystem_preset）+ AgentKind（llm/a2a_proxy） |
| Tool | `listTools()` | Category + Source（registry/mcp/custom） |
| Function | `listTools({ source: 'registry' })` | Category |

## 9. 与现有模块的交互

### 9.1 与 M36 Graph Workflow 的关系

- **M36** 定义了 Graph 的后端架构和基础前端（已完成）
- **M68** 是 M36 前端的 UI 重构，不改变后端 API 和数据模型
- M68 的 `EdgeDef` 扩展（sourceHandle/targetHandle）需要与 M36 的 Proto `EdgeDef` 对齐

### 9.2 与 M54 Hermes Kanban 的关系

- M54 的 Task 系统在 Graph 编辑器中有 UI（GraphTaskKanban + GraphTaskDetailDrawer）
- M68 的 RunPanel 需要集成 Task 视图（Tab 之一）
- 不阻塞，M54 完成后集成

### 9.3 与 Agent/Tool/MCP 管理的关系

- Agent 选择器依赖 `useAgentsCatalogStore`（已有）
- Tool 选择器依赖 `useToolsStore`（已有）
- MCP 信息依赖 `getAgentEffectiveTools` API（已有）
- 不需要新的后端 API
