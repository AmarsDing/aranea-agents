# 68 — Graph 编辑器 UI 重构 · 开发计划

---

## 1. 开发策略

### 1.1 原则

- **增量改造**：每个 Task 产出可独立验证的增量，不破坏现有功能
- **TDD**：先写失败测试，再写最小实现
- **后端零改造**：所有 API 已就绪，仅前端改造
- **不回退**：现有 Graph 编辑器功能必须保留

### 1.2 分期策略

```
P0 (核心体验) ──────────────────────────────────────────
  Task 1: 类型扩展 + Handle 端口计算          🟡 部分完成
  Task 2: GraphFlowNode 多端口 Handle 重构     ⏳ 未开始
  Task 3: 节点折叠/展开                        ⏳ 未开始
  Task 4: 5 种运行时状态                       🟡 部分完成
  Task 5: EdgeDef 扩展 + 连接验证              🟡 部分完成
  Task 6: 资源选择器                           ✅ 已完成
  Task 7: RunPanel                             ✅ 已完成
  Task 8: P0 集成测试 + 修复                   ⏳ 未开始

P1 (体验打磨) ──────────────────────────────────────────
  Task 9:  侧边栏重构                          🟡 部分完成
  Task 10: NodeToolbar                         ✅ 已完成
  Task 11: 连线引导                            ⏳ 未开始
  Task 12: State Schema 可视化                 ⏳ 未开始
  Task 13: P1 集成测试 + 修复                  ⏳ 未开始

P2 (高级功能) ──────────────────────────────────────────
  Task 14-20: 路径可达性/Reducer冲突/子图/State Diff/便签/对齐线/版本管理
```

## 2. P0 任务详细计划

### Task 1: 类型扩展 + Handle 端口计算 🟡

**状态**：部分完成 — `portTypes.ts` 已实现端口计算逻辑（与原始设计不同），但 `types.ts` 尚未扩展 PortDef/NodeExecState/ResourceOption 类型，EdgeDef 尚未增加 sourceHandle/targetHandle。

**目标**：扩展 types.ts，新增 PortDef/NodeExecState 类型，实现端口计算逻辑。

**产出**：

| 文件 | 操作 | 状态 | 说明 |
|------|------|------|------|
| `features/graph/types.ts` | 修改 | ⏳ 待完成 | 新增 PortDef, NodeExecState, ResourceOption；EdgeDef 增加 sourceHandle/targetHandle |
| `features/graph/portTypes.ts` | 已新增 | ✅ 已完成 | `getNodePorts()` 根据 State Schema + 节点类型计算端口；`encodeHandleId`/`decodeHandleId` 实现 3-part 编码 |
| `features/graph/portTypes.spec.ts` | 新增 | ⏳ 待完成 | 单元测试 |

**实际实现与设计差异**（`features/graph/portTypes.ts`）：

```typescript
// 实际实现：getNodePorts 根据节点配置推导端口，而非"读写全部字段"
export function getNodePorts(node: NodeDef, stateFields: StateFieldDef[]): { reads: PortInfo[]; writes: PortInfo[] } {
  switch (node.type) {
    case 'agent':
      // reads = inputMapperJson 的 values, writes = outputMapperJson 的 keys
    case 'llm':
      // reads = instruction 模板中的 ${field} 引用, writes = ['response']
    case 'tool'/'function'/'router'/'join'/'hitl':
      // 返回空（前端无法内省）
  }
}
```

> 详细设计说明详见 [设计文档 §4.1 State Schema → Handle 生成](./68-graph-ui-redesign.design.md#41-state-schema--handle-生成)

**验收标准**：
- [x] `portTypes.ts` 对 7 种节点类型返回正确的 PortInfo[]
- [ ] `types.ts` 中 EdgeDef 包含 sourceHandle/targetHandle 可选字段
- [x] Handle ID 编码为 3-part：`{direction}:{fieldName}:{nodeId}`
- [ ] 单元测试通过

**依赖**：无

---

### Task 2: GraphFlowNode 多端口 Handle 重构 ⏳

**状态**：未开始 — 当前 `GraphFlowNode.vue` 仍使用 2 个无名 Handle（`components/graph/GraphFlowNode.vue` 第 18、51 行）。

**目标**：将 2 个无名 Handle 替换为动态多端口 Handle，Handle slot 显示字段名，Handle Tooltip 显示详细信息。

**产出**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `components/graph/GraphFlowNode.vue` | 修改 | 多端口 Handle + 字段名标签 + Handle Tooltip |
| `css/theme/_graph-pages.sass` | 修改 | 端口色变量 + Handle 样式 + Tooltip 样式 |

**核心改造**：

```vue
<!-- 替换原来的 2 个无名 Handle -->
<template v-for="port in data.ports" :key="port.id">
  <!-- 左侧 READS Handle -->
  <Handle
    v-if="port.direction === 'read'"
    :id="port.id"
    type="target"
    :position="Position.Left"
    :style="portStyle(port)"
  >
    <span class="port-label port-label--read">{{ port.fieldName }}</span>
    <q-tooltip :delay="1000" anchor="center right" self="center left">
      <div>{{ port.fieldName }} <span class="text-muted">[{{ port.fieldType }}]</span></div>
      <div v-if="port.reducer">Reducer: {{ reducerLabel(port.reducer) }}</div>
    </q-tooltip>
  </Handle>

  <!-- 右侧 WRITES Handle -->
  <Handle
    v-if="port.direction === 'write'"
    :id="port.id"
    type="source"
    :position="Position.Right"
    :style="portStyle(port)"
  >
    <span class="port-label port-label--write">{{ port.fieldName }}</span>
    <q-tooltip :delay="1000" anchor="center left" self="center right">
      <div>{{ port.fieldName }} <span class="text-muted">[{{ port.fieldType }}]</span></div>
      <div v-if="port.reducer">Reducer: {{ reducerLabel(port.reducer) }}</div>
    </q-tooltip>
  </Handle>
</template>
```

**Handle 布局**：
- 左侧 READS Handle 垂直排列，间距 24px
- 右侧 WRITES Handle 垂直排列，间距 24px
- 节点高度根据 Handle 数量自适应
- 字段名标签位于 Handle 圆点旁，使用 CSS `position: absolute`
- State 字段超过 5 个时，显示前 3 个 + "更多 N 个"折叠

**Handle Tooltip 4 种状态**：

| 状态 | 内容 |
|------|------|
| 默认（悬停 1s） | 字段名 + 类型 + Reducer |
| 连线中+兼容 | "连接到" + 字段名 + 类型徽章 |
| 连线中+不兼容 | "不兼容" + 字段名 |
| 同节点 | "不能连接到同一节点" |

> Handle 视觉参数（圆点尺寸、点击区、字段名标签、端口类型色标）详见 [设计文档 §6.1 Handle 规格](./68-graph-ui-redesign.design.md#61-handle-规格)

**验收标准**：
- [ ] Agent 节点显示所有 State 字段的 READS/WRITES Handle
- [ ] Handle 旁显示字段名标签
- [ ] Handle Tooltip 悬停 1s 显示字段名+类型+Reducer
- [ ] Router 节点只显示 READS Handle
- [ ] Join 节点保持原有 2 个默认 Handle
- [ ] 节点高度自适应 Handle 数量
- [ ] State 字段超过 5 个时折叠显示
- [ ] 7 种节点类型各有独立色标驱动 Handle 边框色和 Header 色条
- [ ] 暗色模式正常

**依赖**：Task 1

---

### Task 3: 节点折叠/展开 ⏳

**状态**：未开始

**目标**：所有节点可折叠，折叠后显示汇总端口。

**产出**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `components/graph/GraphFlowNode.vue` | 修改 | 折叠/展开逻辑 + 汇总端口 |
| `features/graph/useGraphEditorPage.ts` | 修改 | 折叠状态持久化到 metadata.layout |

**折叠逻辑**：

1. 双击 Header → 切换 `data.collapsed`
2. 折叠时：隐藏所有字段 Handle，显示 `__reads` / `__writes` 汇总 Handle
3. 汇总 Handle 标签：`R{count} / W{count}`（如 R3/W2）
4. 已连接的边：折叠时连接到汇总 Handle，展开时恢复到原始 Handle
5. 调用 `updateNodeInternals(nodeId)` 刷新 Handle 边界

**验收标准**：
- [ ] 双击 Header 切换折叠
- [ ] 折叠后显示汇总端口（R{count}/W{count}）
- [ ] 已连接的边在折叠/展开时正确映射
- [ ] `updateNodeInternals()` 在切换后调用
- [ ] 折叠状态持久化到 layout metadata

**依赖**：Task 2

---

### Task 4: 5 种运行时状态 🟡

**状态**：部分完成 — `EXECUTION_STATUS_STYLES` 已定义 6 种状态（`features/graph/types.ts` 第 232 行），`GraphNodeStatusBadge.vue` 已存在，WS 事件映射已实现（`features/graph/runtime/useGraphExecutionStream.ts`）。但 `GraphFlowNode.vue` 仍使用 `execStatus` 而非文档定义的 `execState`，且状态值使用 `completed`/`failed` 而非 `success`/`error`。

**目标**：节点根据 WS 事件显示 5 种执行状态，边显示波浪动画。

**产出**：

| 文件 | 操作 | 状态 | 说明 |
|------|------|------|------|
| `components/graph/GraphNodeStatusBadge.vue` | 已新增 | ✅ | 5 种状态徽章组件 |
| `components/graph/GraphFlowNode.vue` | 修改 | ⏳ | 集成状态徽章 + CSS class（当前使用 execStatus） |
| `components/graph/GraphFlowEdge.vue` | 修改 | ⏳ | 状态驱动的边动画 |
| `features/graph/runtime/useGraphRunStream.ts` | 已修改 | ✅ | 事件→状态映射 |

**实际状态值映射**（`features/graph/types.ts` EXECUTION_STATUS_STYLES）：

| 状态值 | 颜色 | 图标 | 标签 | WS 事件 |
|--------|------|------|------|---------|
| `idle` | grey | radio_button_unchecked | 等待 | （默认） |
| `running` | cyan | sync | 运行中 | graph_node_start |
| `completed` | emerald | check_circle | 完成 | graph_node_end |
| `failed` | pink | error | 失败 | graph_node_error |
| `interrupted` | amber | pause_circle | 中断 | checkpoint.interrupt |
| `waiting` | grey-6 | schedule | 等待 | （扩展状态） |

> 注意：实际实现使用 `completed`/`failed` 而非设计文档中的 `success`/`error`。详见 [设计文档 §4.2 WS 事件 → 节点状态](./68-graph-ui-redesign.design.md#42-ws-事件--节点状态)

**验收标准**：
- [x] 状态徽章组件已实现
- [x] WS 事件正确映射到状态
- [ ] running 状态的出边显示流动动画点
- [ ] 执行完成后所有节点恢复 idle
- [ ] 暗色模式正常

**依赖**：Task 2（可与 Task 3 并行）

---

### Task 5: EdgeDef 扩展 + 连接验证 🟡

**状态**：部分完成 — `isValidConnectionQuick()` 已实现于 `features/graph/portTypes.ts`（含自连接/重复边检查 + 字段名匹配警告），但 `EdgeDef` 尚未扩展 sourceHandle/targetHandle，`GraphEditorCanvas.vue` 的 `onConnect` 未使用 sourceHandle/targetHandle。

**目标**：EdgeDef 支持 sourceHandle/targetHandle，连接时做结构性检查。

**产出**：

| 文件 | 操作 | 状态 | 说明 |
|------|------|------|------|
| `components/graph/GraphEditorCanvas.vue` | 修改 | ⏳ | onConnect 捕获 sourceHandle/targetHandle（当前仅用 from/to） |
| `features/graph/useGraphLocalValidation.ts` | 修改 | ⏳ | 增加结构性连接检查（当前 portTypes.ts 已有 isValidConnectionQuick） |
| `features/graph/types.ts` | 修改 | ⏳ | EdgeDef 增加 sourceHandle/targetHandle |
| `features/graph/portTypes.ts` | 已新增 | ✅ | `isValidConnectionQuick()` 连接验证函数 |

**实际连接验证逻辑**（`features/graph/portTypes.ts`）：

```typescript
export function isValidConnectionQuick(
  sourceNodeId: string,
  sourceHandleId: string | null,
  targetNodeId: string,
  targetHandleId: string | null,
  existingEdges: Array<{ from: string; to: string }>,
): ConnectionValidationResult {
  // 1. 自连接检查
  if (sourceNodeId === targetNodeId) return { valid: false, reason: '...' };
  // 2. 重复边检查
  if (existingEdges.some(e => e.from === sourceNodeId && e.to === targetNodeId))
    return { valid: false, reason: 'Duplicate edge' };
  // 3. 字段名匹配警告（非阻断）
  if (sourceHandleId && targetHandleId) {
    const sourcePort = decodeHandleId(sourceHandleId);
    const targetPort = decodeHandleId(targetHandleId);
    if (sourcePort.field !== targetPort.field)
      return { valid: true, warning: `Field name mismatch: ...` };
  }
  return { valid: true };
}
```

**验收标准**：
- [ ] Edge 包含 sourceHandle/targetHandle
- [x] 连接时禁止自连接和重复边（portTypes.ts 已实现）
- [ ] 边 ID 编码包含 Handle 信息
- [ ] 现有边数据兼容（无 sourceHandle/targetHandle 时使用默认 Handle）

**依赖**：Task 1, Task 2

---

### Task 6: 资源选择器 ✅

**状态**：已完成 — `GraphAgentSelector.vue`、`GraphToolSelector.vue`、`GraphFunctionSelector.vue` 均已存在。`GraphResourceSelector.vue` 未创建（通用组件被各专用选择器替代）。

**目标**：Agent/Tool/Function 选择器替代纯文本输入，节点创建时自动弹出选择器。

**产出**：

| 文件 | 操作 | 状态 | 说明 |
|------|------|------|------|
| `components/graph/GraphResourceSelector.vue` | 未新增 | ➖ | 通用资源选择器（被专用选择器替代） |
| `components/graph/GraphAgentSelector.vue` | 已新增 | ✅ | Agent 分类选择器 |
| `components/graph/GraphToolSelector.vue` | 已新增 | ✅ | Tool 分类选择器 |
| `components/graph/GraphFunctionSelector.vue` | 已新增 | ✅ | Function 选择器 |
| `components/graph/GraphPropertyPanel.vue` | 修改 | ✅ | 替换纯文本输入为选择器 |
| `features/graph/useGraphEditorPage.ts` | 修改 | ✅ | 加载 Agent/Tool/Function 列表 |

> 数据源映射详见 [设计文档 §8 API 与数据源](./68-graph-ui-redesign.design.md#8-api-与数据源)

**验收标准**：
- [x] Agent 选择器按 Kind 分组，支持搜索
- [x] Tool 选择器按 Category 分组，多选
- [x] Function 选择器列出 Registry 函数
- [ ] 选中 Agent 后显示 MCP 策略信息（待验证）
- [ ] 节点创建后自动弹出选择器（待验证）
- [ ] 选择的资源不存在时显示警告（待验证）

**依赖**：Task 1

---

### Task 7: RunPanel ✅

**状态**：已完成 — `GraphRunPanel.vue` 已存在，使用 `q-drawer` 实现，包含执行控制、State 快照、Checkpoint 导航。

**目标**：编辑器内可折叠 RunPanel，集成执行控制、State 快照、Checkpoint 导航。

**产出**：

| 文件 | 操作 | 状态 | 说明 |
|------|------|------|------|
| `components/graph/GraphRunPanel.vue` | 已新增 | ✅ | 运行面板主组件（q-drawer 实现） |
| `components/graph/GraphEditorCanvas.vue` | 修改 | ✅ | 集成 RunPanel |
| `features/graph/useGraphEditorPage.ts` | 修改 | ✅ | RunPanel 状态管理 |

**RunPanel 结构**：
- 顶部：执行控制（运行/停止）
- 中部：q-tabs（State / Checkpoint / Task）
- State Tab：显示当前 State 所有字段和值
- Checkpoint Tab：列出 Checkpoint + TimeTravel 按钮
- Task Tab：复用现有 GraphTaskKanban（M54 完成后集成）

**验收标准**：
- [x] RunPanel 默认收起，点击运行按钮展开
- [x] 执行控制按钮功能正常
- [x] State 快照实时更新
- [x] Checkpoint 列表 + TimeTravel 功能
- [x] RunPanel 宽度可拖拽
- [x] 与 PropertyPanel 共存

**依赖**：Task 4（需要运行时状态）

---

### Task 8: P0 集成测试 + 修复 ⏳

**状态**：未开始

**目标**：P0 全部功能集成验证，修复发现的问题。

**验收标准**：
- [ ] 50 节点图编辑器操作流畅
- [ ] 所有现有功能不回退
- [ ] 暗色模式正常
- [ ] `pnpm lint` 通过
- [ ] `pnpm build` 通过
- [ ] `pnpm test` 通过

**依赖**：Task 1-7 全部完成

## 3. P1 任务详细计划

### Task 9: 侧边栏重构 🟡

**状态**：部分完成 — `GraphNodePalette.vue` 已存在，支持模板选择，但尚未实现 3 区段导航（节点类型/版本历史/设置）。

**产出**：
- `components/graph/GraphNodePalette.vue` 修改：3 区段 + 快速模板
- 新增 3 个快速模板（审批流程/数据处理/条件路由）

**验收标准**：
- [ ] 40px 图标条 + 3 区段（节点类型/版本/设置）
- [x] 快速模板卡片可拖拽创建（部分完成）
- [x] 搜索功能正常

### Task 10: NodeToolbar ✅

**状态**：已完成 — `GraphNodeToolbar.vue` 已存在。

**产出**：
- `components/graph/GraphNodeToolbar.vue` 已新增：浮动工具栏
- `components/graph/GraphEditorCanvas.vue` 修改：集成 NodeToolbar

**验收标准**：
- [x] 选中节点上方显示工具栏
- [x] 3 个直接按钮（运行到此/冻结/删除）
- [x] 更多菜单条件性显示
- [x] 点击画布空白区域隐藏

### Task 11: 连线引导 ⏳

**状态**：未开始

**产出**：
- `components/graph/GraphEditorCanvas.vue` 修改：Handle 过滤 + 兼容高亮
- `components/graph/GraphFlowNode.vue` 修改：Handle 霓虹脉冲动画

**验收标准**：
- [ ] 拖线时同名 Handle 绿色脉冲
- [ ] 不兼容 Handle 灰色
- [ ] 无兼容端口时显示提示
- [ ] 双击 Handle 快捷连线

### Task 12: State Schema 可视化 ⏳

**状态**：未开始 — `GraphStatePanel.vue` 尚未创建。

**产出**：
- `components/graph/GraphStatePanel.vue` 新增：State 可视化面板

**验收标准**：
- [ ] 显示所有 State 字段及其读写节点
- [ ] Reducer 用自然语言显示
- [ ] 点击字段高亮相关节点

### Task 13: P1 集成测试 + 修复 ⏳

同 Task 8 模式。

## 4. 验证命令

| 改动类型 | 命令 |
|----------|------|
| 前端 lint | `cd web && pnpm lint` |
| 前端 build | `cd web && pnpm build` |
| 前端测试 | `cd web && pnpm test` |
| 全量前端 | `cd web && pnpm lint && pnpm test && pnpm build` |

## 5. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|:----:|------|
| 多端口 Handle 布局复杂 | 中 | 参考 Langflow `groupHandlesByType()` + CSS Grid |
| 折叠/展开时边连接错位 | 中 | `updateNodeInternals()` + 边映射逻辑 |
| NodeToolbar 自建定位 | 低 | 基于 Vue Flow 节点 DOM 位置 + Teleport |
| 大量工具时 q-select 性能 | 低 | `q-virtual-scroll` + 分页 |
| Handle 霓虹脉冲动画性能 | 低 | CSS `will-change: box-shadow`，仅活跃 Handle 启用 |
| State Schema 变更时 Handle 动态更新 | 中 | watch stateFields 变化 → `updateNodeInternals()` |
| RunPanel 与 GraphRunPage 逻辑重复 | 中 | 提取 `useGraphPlayground` composable 共享执行逻辑 |
| State 字段数量爆炸（>10 个字段节点过高） | 中 | 分组折叠——显示前 3 个字段，其余 "更多 N 个" |
| RunPanel 宽度挤压画布空间 | 低 | 默认收起 + 运行时自动展开 + 宽度可拖拽 |
| Graph 事件需 session_id 关联 | 低 | ExecuteGraph 返回 execution_id，前端从执行上下文获取 session_id |
| 循环图（Router 回边）视觉设计 | 低 | 循环边使用虚线 `strokeDasharray="5 5"` + 自定义贝塞尔控制点偏移 |
| 边重连行为（无效重连恢复原位） | 低 | `edgeUpdateEnd` 时检查新连接有效性，无效则恢复原边 |
| 画布执行锁定 | 低 | 执行时 `nodesDraggable=false` + 禁用连接 + 显示执行横幅 |

### 5.1 已修复 UX 问题（2026-06-10）

| 问题 | 根因 | 修复 | 涉及文件 |
|------|------|------|----------|
| 拖拽连线时无视觉反馈 | `#connection-line` slot 被替换为空模板 | 恢复 `GraphConnectionLine` 组件渲染 | `components/graph/GraphEditorCanvas.vue` |
| Handle 点击目标太小（10px） | 可视尺寸仅 10px，实际可点击区域约 6px | 增大至 12px + `::before` 24px 热区 + hover scale | `css/theme/_graph-pages.sass` |
| 已连接的线删除困难 | 边 stroke-width 仅 1px 难以点击，无右键菜单 | 12px 透明交互路径 + 边右键菜单 + Delete 键支持 | `components/graph/GraphFlowEdge.vue`、`components/graph/GraphEditorCanvas.vue` |
| 对齐辅助线只画线不吸附 | `useSnapGuide` 只计算辅助线不修正位置；`snap-to-grid` 16px 与对齐线冲突 | `computeSnapLines` 返回 `delta` 修正量，`onNodeDragStop` 应用吸附；关闭 `snap-to-grid` | `features/graph/useSnapGuide.ts`、`components/graph/GraphEditorCanvas.vue` |
| 对齐辅助线严重错位 | SVG 放在 VueFlow 默认 slot 中，不随 viewport transform 缩放/平移，导致坐标不匹配 | 将 SVG 移入 `#zoom-pane` slot（在 Transform div 内部），坐标自动对齐；添加 `vector-effect: non-scaling-stroke` 保持线宽不变 | `components/graph/GraphEditorCanvas.vue`、`css/theme/_graph-pages.sass` |
| `deleteEdgeById` 与 `onEdgesChange` 重复逻辑 | 两处独立实现边删除 | 提取 `deleteEdgeById` 共享函数，`onEdgesChange` 复用 | `components/graph/GraphEditorCanvas.vue` |

## 6. 改动文件清单

> 以下文件清单从设计文档迁移，包含阶段标记。设计文档中的组件设计引用了这些文件。

### 6.1 新增文件

| 文件 | 用途 | 阶段 | 状态 |
|------|------|------|------|
| `components/graph/GraphAgentSelector.vue` | Agent 分类选择器 | P0 | ✅ 已完成 |
| `components/graph/GraphToolSelector.vue` | Tool 分类选择器 | P0 | ✅ 已完成 |
| `components/graph/GraphFunctionSelector.vue` | Function 选择器 | P0 | ✅ 已完成 |
| `components/graph/GraphRunPanel.vue` | 运行面板 | P0 | ✅ 已完成 |
| `components/graph/GraphNodeStatusBadge.vue` | 节点状态徽章 | P0 | ✅ 已完成 |
| `features/graph/portTypes.ts` | Handle 端口计算 + 编码/解码 + 连接验证 | P0 | ✅ 已完成 |
| `components/graph/GraphResourceSelector.vue` | 通用资源选择器 | P0 | ➖ 未创建（被专用选择器替代） |
| `features/graph/composables/useResourceSelectors.ts` | 资源选择器数据 | P0 | ➖ 未创建（逻辑内联于 useGraphEditorPage） |
| `features/graph/composables/useGraphPlayground.ts` | RunPanel 执行逻辑（与 GraphRunPage 共享） | P0 | ➖ 未创建 |
| `components/graph/GraphNodeToolbar.vue` | 节点浮动工具栏 | P1 | ✅ 已完成 |
| `components/graph/GraphStatePanel.vue` | State 可视化面板 | P1 | ⏳ 未开始 |

### 6.2 修改文件

| 文件 | 变更 | 阶段 | 状态 |
|------|------|------|------|
| `components/graph/GraphFlowNode.vue` | 多端口 Handle + 折叠 + 5 种状态 | P0 | ⏳ 未开始（当前仍 2 个无名 Handle） |
| `components/graph/GraphFlowEdge.vue` | sourceHandle/targetHandle + 状态动画 | P0 | ⏳ 未开始 |
| `components/graph/GraphPropertyPanel.vue` | 资源选择器替换纯文本输入 | P0 | ✅ 已完成 |
| `components/graph/GraphEditorCanvas.vue` | isValidConnection + sourceHandle/targetHandle + NodeToolbar + 画布锁定 | P0/P1 | 🟡 部分完成（NodeToolbar 已集成，连接验证未集成） |
| `pages/GraphEditorPage.vue` | 集成 RunPanel + 布局调整 | P0 | ✅ 已完成 |
| `components/graph/GraphNodePalette.vue` | 3 区段 + 快速模板 | P1 | 🟡 部分完成（模板已有，3 区段未实现） |
| `features/graph/types.ts` | PortDef + EdgeDef 扩展 + NodeExecState | P0 | ⏳ 未开始（EXECUTION_STATUS_STYLES 已有） |
| `features/graph/useGraphEditorPage.ts` | 资源加载 + 节点创建引导 | P0 | ✅ 已完成 |
| `features/graph/useGraphLocalValidation.ts` | 结构性连接检查 | P0 | ⏳ 未开始（portTypes.ts 已有 isValidConnectionQuick） |
| `features/graph/runtime/useGraphRunStream.ts` | 事件→状态映射 + 批量边动画 | P0 | ✅ 已完成 |
| `css/theme/_graph-pages.sass` | Langflow 完整色彩体系 + 端口色 + 状态色 + Handle 动画 | P0 | ✅ 已完成 |

## 7. 里程碑

| 里程碑 | 包含任务 | 交付物 | 状态 |
|--------|---------|--------|------|
| M1 - 类型基础 | Task 1 | types.ts 扩展 + portTypes.ts 端口计算 | 🟡 部分完成 |
| M2 - 节点重构 | Task 2, 3, 4 | 多端口 Handle + 折叠 + 5 种状态 | ⏳ 未开始 |
| M3 - 连接增强 | Task 5 | sourceHandle/targetHandle + 连接验证 | 🟡 部分完成 |
| M4 - 资源选择 | Task 6 | Agent/Tool/Function 选择器 | ✅ 已完成 |
| M5 - 执行闭环 | Task 7 | RunPanel | ✅ 已完成 |
| M6 - P0 完成 | Task 8 | 集成测试通过 | ⏳ 未开始 |
| M7 - P1 完成 | Task 9-13 | 侧边栏 + NodeToolbar + 连线引导 + State 面板 | 🟡 部分完成 |
