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
  Task 1: 类型扩展 + Handle 端口计算
  Task 2: GraphFlowNode 多端口 Handle 重构
  Task 3: 节点折叠/展开
  Task 4: 5 种运行时状态
  Task 5: EdgeDef 扩展 + 连接验证
  Task 6: 资源选择器
  Task 7: RunPanel
  Task 8: P0 集成测试 + 修复

P1 (体验打磨) ──────────────────────────────────────────
  Task 9:  侧边栏重构
  Task 10: NodeToolbar
  Task 11: 连线引导
  Task 12: State Schema 可视化
  Task 13: P1 集成测试 + 修复

P2 (高级功能) ──────────────────────────────────────────
  Task 14-20: 路径可达性/Reducer冲突/子图/State Diff/便签/对齐线/版本管理
```

## 2. P0 任务详细计划

### Task 1: 类型扩展 + Handle 端口计算

**目标**：扩展 types.ts，新增 PortDef/NodeExecState 类型，实现 `useNodePorts` composable。

**产出**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `features/graph/types.ts` | 修改 | 新增 PortDef, NodeExecState, ResourceOption；EdgeDef 增加 sourceHandle/targetHandle |
| `features/graph/composables/useNodePorts.ts` | 新增 | 根据 State Schema + 节点类型计算 PortDef[] |
| `features/graph/composables/useNodePorts.spec.ts` | 新增 | 单元测试 |

**useNodePorts 核心逻辑**：

```typescript
export function useNodePorts(graphDef: Ref<GraphDefinition>) {
  const computePorts = (node: NodeDef): PortDef[] => {
    const fields = graphDef.value.stateFields
    const ports: PortDef[] = []

    // 可执行节点：读写全部 State 字段
    if (['agent', 'llm', 'tool', 'function', 'hitl'].includes(node.type)) {
      fields.forEach(f => {
        ports.push({ id: `r:${f.name}`, fieldName: f.name, direction: 'read', fieldType: f.type, connected: false })
        ports.push({ id: `w:${f.name}`, fieldName: f.name, direction: 'write', fieldType: f.type, reducer: f.reducer, connected: false })
      })
    }

    // Router：只读不写
    if (node.type === 'router') {
      fields.forEach(f => {
        ports.push({ id: `r:${f.name}`, fieldName: f.name, direction: 'read', fieldType: f.type, connected: false })
      })
    }

    // Join：透传，无端口
    if (node.type === 'join') {
      // 仅保留默认 target/source Handle
    }

    return ports
  }

  return { computePorts }
}
```

**验收标准**：
- [ ] `useNodePorts` 对 7 种节点类型返回正确的 PortDef[]
- [ ] EdgeDef 包含 sourceHandle/targetHandle 可选字段
- [ ] PortDef 的 id 编码为 `r:{fieldName}` / `w:{fieldName}`
- [ ] 单元测试通过

**依赖**：无

---

### Task 2: GraphFlowNode 多端口 Handle 重构

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

**Handle 视觉参数**（对齐 Langflow）：

| 属性 | 值 |
|------|-----|
| Handle 圆点 | 10px 彩色圆 + 3px ring |
| Handle 点击区 | 32×32px 透明区域 |
| 字段名标签 | 12px 文字，Handle 旁 4px |
| 端口类型色标 | Handle 圆点边框色 = 字段类型色 |

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

### Task 3: 节点折叠/展开

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

### Task 4: 5 种运行时状态

**目标**：节点根据 WS 事件显示 5 种执行状态，边显示波浪动画。

**产出**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `components/graph/GraphNodeStatusBadge.vue` | 新增 | 5 种状态徽章组件 |
| `components/graph/GraphFlowNode.vue` | 修改 | 集成状态徽章 + CSS class |
| `components/graph/GraphFlowEdge.vue` | 修改 | 状态驱动的边动画 |
| `features/graph/useGraphRunStream.ts` | 修改 | 事件→状态映射 |

**状态视觉规范**：

| 状态 | 边框 | 动画 | 图标 |
|------|------|------|------|
| idle | 默认 | 无 | 无 |
| running | 蓝色 1px | 150ms wiggle | Loader2 旋转 |
| success | 绿色 0.75px ring | 无 | Check |
| error | 红色 1px | 无 | CircleAlert |
| interrupted | 橙色 1px | 无 | CirclePause |

**验收标准**：
- [ ] 5 种状态视觉正确
- [ ] WS 事件正确映射到状态
- [ ] running 状态的出边显示流动动画点
- [ ] 执行完成后所有节点恢复 idle
- [ ] 暗色模式正常

**依赖**：Task 2（可与 Task 3 并行）

---

### Task 5: EdgeDef 扩展 + 连接验证

**目标**：EdgeDef 支持 sourceHandle/targetHandle，连接时做结构性检查。

**产出**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `components/graph/GraphEditorCanvas.vue` | 修改 | onConnect 捕获 sourceHandle/targetHandle |
| `features/graph/useGraphLocalValidation.ts` | 修改 | 增加结构性连接检查 |
| `features/graph/types.ts` | 修改 | EdgeDef 增加 sourceHandle/targetHandle |

**连接验证逻辑**：

```typescript
function isValidConnection(connection: Connection): boolean {
  // 1. 自连接检查
  if (connection.source === connection.target) return false
  // 2. 重复边检查
  const exists = edges.value.some(
    e => e.source === connection.source
      && e.target === connection.target
      && e.sourceHandle === connection.sourceHandle
      && e.targetHandle === connection.targetHandle
  )
  if (exists) return false
  return true
}
```

**验收标准**：
- [ ] Edge 包含 sourceHandle/targetHandle
- [ ] 连接时禁止自连接和重复边
- [ ] 边 ID 编码包含 Handle 信息
- [ ] 现有边数据兼容（无 sourceHandle/targetHandle 时使用默认 Handle）

**依赖**：Task 1, Task 2

---

### Task 6: 资源选择器

**目标**：Agent/Tool/Function 选择器替代纯文本输入，节点创建时自动弹出选择器。

**产出**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `components/graph/GraphResourceSelector.vue` | 新增 | 通用资源选择器 |
| `components/graph/GraphAgentSelector.vue` | 新增 | Agent 分类选择器 |
| `components/graph/GraphToolSelector.vue` | 新增 | Tool 分类选择器 |
| `components/graph/GraphFunctionSelector.vue` | 新增 | Function 选择器 |
| `components/graph/GraphPropertyPanel.vue` | 修改 | 替换纯文本输入为选择器 |
| `features/graph/composables/useResourceSelectors.ts` | 新增 | 资源数据加载 + 分组 |
| `features/graph/useGraphEditorPage.ts` | 修改 | 加载 Agent/Tool/Function 列表 |

**数据源映射**：

| 选择器 | API | 分组依据 |
|--------|-----|---------|
| Agent | `listAgents()` | Kind（user/system_builtin/ecosystem_preset）+ AgentKind（llm/a2a_proxy） |
| Tool | `listTools()` | Category + Source（registry/mcp/custom） |
| Function | `listTools({ source: 'registry' })` | Category |

**节点创建引导**：
- 拖入 Agent/Tool/Function 节点后，自动弹出对应选择器
- 选择后自动填充 agentName/toolNames/funcRef
- 跳过则节点显示"未配置"警告

**验收标准**：
- [ ] Agent 选择器按 Kind 分组，支持搜索
- [ ] Tool 选择器按 Category 分组，多选
- [ ] Function 选择器列出 Registry 函数
- [ ] 选中 Agent 后显示 MCP 策略信息
- [ ] 节点创建后自动弹出选择器
- [ ] 选择的资源不存在时显示警告

**依赖**：Task 1

---

### Task 7: RunPanel

**目标**：编辑器内可折叠 RunPanel，集成执行控制、State 快照、Checkpoint 导航。

**产出**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `components/graph/GraphRunPanel.vue` | 新增 | 运行面板主组件 |
| `components/graph/GraphEditorCanvas.vue` | 修改 | 集成 RunPanel |
| `features/graph/useGraphEditorPage.ts` | 修改 | RunPanel 状态管理 |

**RunPanel 结构**：
- 顶部：执行控制（运行/暂停/停止）
- 中部：q-tabs（State / Checkpoint / Task）
- State Tab：显示当前 State 所有字段和值
- Checkpoint Tab：列出 Checkpoint + TimeTravel 按钮
- Task Tab：复用现有 GraphTaskKanban（M54 完成后集成）

**验收标准**：
- [ ] RunPanel 默认收起，点击运行按钮展开
- [ ] 执行控制按钮功能正常
- [ ] State 快照实时更新
- [ ] Checkpoint 列表 + TimeTravel 功能
- [ ] RunPanel 宽度可拖拽
- [ ] 与 PropertyPanel 共存

**依赖**：Task 4（需要运行时状态）

---

### Task 8: P0 集成测试 + 修复

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

### Task 9: 侧边栏重构

**产出**：
- `GraphNodePalette.vue` 修改：3 区段 + 快速模板
- 新增 3 个快速模板（审批流程/数据处理/条件路由）

**验收标准**：
- [ ] 40px 图标条 + 3 区段（节点类型/版本/设置）
- [ ] 快速模板卡片可拖拽创建
- [ ] 搜索功能正常

### Task 10: NodeToolbar

**产出**：
- `GraphNodeToolbar.vue` 新增：浮动工具栏
- `GraphEditorCanvas.vue` 修改：集成 NodeToolbar

**验收标准**：
- [ ] 选中节点上方显示工具栏
- [ ] 3 个直接按钮（运行到此/冻结/删除）
- [ ] 更多菜单条件性显示
- [ ] 点击画布空白区域隐藏

### Task 11: 连线引导

**产出**：
- `GraphEditorCanvas.vue` 修改：Handle 过滤 + 兼容高亮
- `GraphFlowNode.vue` 修改：Handle 霓虹脉冲动画

**验收标准**：
- [ ] 拖线时同名 Handle 绿色脉冲
- [ ] 不兼容 Handle 灰色
- [ ] 无兼容端口时显示提示
- [ ] 双击 Handle 快捷连线

### Task 12: State Schema 可视化

**产出**：
- `GraphStatePanel.vue` 新增：State 可视化面板

**验收标准**：
- [ ] 显示所有 State 字段及其读写节点
- [ ] Reducer 用自然语言显示
- [ ] 点击字段高亮相关节点

### Task 13: P1 集成测试 + 修复

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

## 6. 补充规范

### 6.1 Handle ID 编码方案

```
格式：{direction}:{fieldName}
方向：r = read (target Handle, 左侧)
     w = write (source Handle, 右侧)

注意：Handle ID 只需在节点内唯一，不需要跨节点唯一。
Vue Flow 的 Edge 已通过 source/target 字段区分不同节点的 Handle，
因此 2-part 编码足够。3-part 编码（r:messages:node1）是冗余的。

示例：
  r:messages     — 读取 messages 字段（左侧 Handle）
  w:response     — 写入 response 字段（右侧 Handle）

折叠态汇总 Handle：
  __reads        — 汇总读取端口
  __writes       — 汇总写入端口
```

### 6.2 节点视觉参数（完全对齐 Langflow）

| 属性 | 值 | Langflow 来源 |
|------|-----|--------------|
| 展开宽度 | 320px (`w-80`) | GenericNode |
| 折叠宽度 | 192px (`w-48`) | GenericNode |
| 圆角 | 12px (`rounded-xl`) | GenericNode |
| 阴影默认 | `0 1px 2px 0 rgb(0 0 0 / 0.05)` (`shadow-sm`) | GenericNode |
| 阴影 hover | `0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)` (`shadow-md`) | GenericNode |
| 阴影选中 hover | `0 0px 15px -3px rgb(0 0 0 / 0.1), 0 0px 6px -4px rgb(0 0 0 / 0.1)` (`shadow-node`) | GenericNode |
| 背景 | light #FFFFFF / dark #191A1D (`bg-background`) | GenericNode |
| Header 间距 | 16px 水平, 12px 垂直 (`px-4 py-3`) | NodeHeader |
| Header 底部边框 | 1px `border-b`（仅展开态） | NodeHeader |
| 图标尺寸 | 18px (`h-4.5 w-4.5`) | NodeIcon |
| 图标左边距 | 12px (`ml-3`) | NodeIcon |
| 名称字号 | 16px, weight 500 (`text-base font-medium`) | NodeName |
| Legacy 徽章 | 11px, 4px 圆角, 1px amber 边框, 4px 水平内边距 | NodeName |
| Beta 徽章 | 16px×16px, 4px 圆角, 1px purple 边框, 2px 内边距 | NodeName |
| Run 按钮 | 24px×24px, 4px 圆角, 14px 图标, building 时 spin | NodeStatus |
| Edit 按钮 | 24px×24px, 6px 圆角, 18px 图标, 展开时 top-2 translate-x-[10.4rem], 折叠时 top-0 translate-x-[6.4rem] | NodeName |
| Handle 圆点 | 10px 彩色圆, muted 时 6px opacity:0 | handleRenderComponent |
| Handle 点击区 | 32×32px 透明 | handleRenderComponent |
| Handle 位置 | 左: `left:0; transform:translate(-50%,-50%)`, 右: `right:0; transform:translate(50%,-50%)` | classes.css |
| Handle 过渡 | `all 0.2s` | handleRenderComponent |
| 字段名标签 | 12px，Handle 旁 4px | 适配（Langflow 无此元素） |
| Wiggle 动画 | 0%/100% scale(100%), 50% scale(120%), 150ms ease-in-out ×1 | BUILDING 状态 |

**节点状态边框（完全对齐 Langflow）**：

| 状态 | 边框 | Ring | 额外 |
|------|------|------|------|
| 默认（未选中） | 1px `border` | 0.5px `ring-border` | — |
| 选中（未构建） | 1px `border` | 0.75px `ring-muted-foreground` | hover:shadow-node |
| BUILDING | 1px `border-foreground` | 0.75px `ring-foreground` | wiggle 动画 |
| ERROR | 1px `border-destructive` | 0.75px `ring-destructive` | — |
| INACTIVE | `border-none` | `ring` | grayscale 滤镜 |
| Frozen（选中） | 2px `border-frozen-blue` | — | shadow-frozen-ring + ::before inset -2px |
| Frozen（未选中） | 1px `border` | — | shadow-frozen-ring |

**Handle 霓虹脉冲动画（完全对齐 Langflow）**：

```css
/* 0% 和 100% */
box-shadow: 0 0 0 3px hsl(var(--node-ring)),
            0 0 2px {handleColor},
            0 0 4px {handleColor},
            0 0 6px {handleColor},
            0 0 8px {handleColor},
            0 0 10px {handleColor},
            0 0 15px {handleColor},
            0 0 20px {handleColor};

/* 50% */
box-shadow: 0 0 0 3px hsl(var(--node-ring)),
            0 0 4px {handleColor},
            0 0 8px {handleColor},
            0 0 12px {handleColor},
            0 0 16px {handleColor},
            0 0 20px {handleColor},
            0 0 25px {handleColor},
            0 0 30px {handleColor};

/* 动画参数 */
duration: 1.1s;
timing-function: ease-in-out;
iteration: infinite;
```

**Handle 静态霓虹阴影（hover/active 时）**：

```css
box-shadow: 0 0 0 1px hsl(var(--border)),
            0 0 2px {handleColor},
            0 0 4px {handleColor},
            0 0 6px {handleColor},
            0 0 8px {handleColor},
            0 0 10px {handleColor},
            0 0 15px {handleColor},
            0 0 20px {handleColor};
```

### 6.3 边视觉状态（完全对齐 Langflow）

| 状态 | 线宽 | 颜色 | 说明 |
|------|------|------|------|
| 默认 | 2px | `var(--connection)` = light #555, dark #6d6c6c | 普通边 |
| 选中 | 2px | `var(--selected)` = light #2196f3, dark #0369a1 | 选中高亮 |
| 运行中 | 2px | `hsl(var(--foreground))` | 黑/白 |
| 非运行 | 1px | `hsl(var(--foreground))` | 灰化 |
| 已运行 | 2px | `hsl(var(--foreground))` | 正常 |
| 循环边 | 2px | `var(--connection)` + `strokeDasharray="5 5"` | 虚线 |
| 过渡 | — | `color 150ms` | — |

**连接线（完全对齐 Langflow）**：

| 属性 | 值 |
|------|-----|
| 线宽 | 2px |
| 颜色 | `hsl(var(--datatype-{color}))`（数据类型色） |
| 动画 | marching ants（CSS `animated` class） |
| 端点圆 | 半径 5px, 填充 #fff, 描边=数据类型色, 描边宽度 1.5px |

**边点击区域**：20px 透明描边（opacity: 0），用于右键菜单定位

### 6.4 边重连行为

```
Langflow 行为：无效重连 → 删除边
Aranea 行为：无效重连 → 恢复原边（更安全）

实现：在 edgeUpdateEnd 事件中：
1. 检查新连接是否有效（isValidConnection）
2. 有效 → 更新边的 source/target/sourceHandle/targetHandle
3. 无效 → 恢复原始边数据（不删除）
```

### 6.5 画布执行锁定

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

## 7. 里程碑

| 里程碑 | 包含任务 | 交付物 |
|--------|---------|--------|
| M1 - 类型基础 | Task 1 | types.ts 扩展 + useNodePorts composable |
| M2 - 节点重构 | Task 2, 3, 4 | 多端口 Handle + 折叠 + 5 种状态 |
| M3 - 连接增强 | Task 5 | sourceHandle/targetHandle + 连接验证 |
| M4 - 资源选择 | Task 6 | Agent/Tool/Function 选择器 |
| M5 - 执行闭环 | Task 7 | RunPanel |
| M6 - P0 完成 | Task 8 | 集成测试通过 |
| M7 - P1 完成 | Task 9-13 | 侧边栏 + NodeToolbar + 连线引导 + State 面板 |
