# Graph UI 优化（第二轮 ~ 第十二轮）

**日期**：2026-05-29  
**模块**：Graph (36) · 前端 + 后端  
**影响**：🟡 中 — Graph 编辑器/运行页/列表页视觉与交互优化 + 后端服务端筛选

## 变更摘要

七轮迭代优化 Graph UI：修复亮色模式适配、数据流合规、watch 性能、撤销/重做系统、无障碍访问、交互增强（Ctrl+S/Ctrl+Z/Ctrl+F/连线高亮/拖拽预览）、动画与响应式布局、SASS 拆分、共享工具函数提取、PropertyPanel 全面 undo 集成、画布缩放指示器、条件路由可视化编辑、边/节点删除 undo 集成、composable 拆分、硬编码颜色提取、批量节点操作、执行历史列表页、composable 全面响应式改造。第八轮：执行历史页面增强（状态筛选+执行时长）、批量节点移动 undoRedo 集成。第九轮：代码质量重构（P5 建议项全部修复）+ 执行历史时间范围筛选。第十轮：画布框选 + Ctrl+A 全选 + 边重连 undoRedo + 执行历史服务端筛选准备。第十一轮：后端服务端筛选实现 + 节点对齐辅助线 + 步骤详情展开/折叠/错误高亮 + Dagre 自动布局。第十二轮：代码质量优化 + UX 增强——节点尺寸常量统一提取、useSnapGuide 变量声明顺序修复、自动布局 undoRedo 集成、小地图执行状态着色、执行 Dialog 共享组件抽取、GraphRunSidebar 优化（formatJson 截断 + reactive Set + 纯函数直接 import）。

## 第二轮优化

### P0 — 必须修复

| 项 | 变更 |
|----|------|
| R2-1 | **GraphContextMenu 亮色模式修复**：scoped 硬编码深色样式迁移至全局 `_graph-pages.sass`，新增 15 个 `--graph-ctx-*` CSS 变量实现亮/暗双模式自适应 |
| R2-2 | **GraphPropertyPanel dirty 追踪修复**：缓存 toggle/TTL + State Schema 字段编辑添加 `@update:model-value="notifyChange"` |

### P1 — 重要改进

| 项 | 变更 |
|----|------|
| R2-3 | **Ctrl+S 保存快捷键**：`useGraphEditorPage.ts` 添加全局 `keydown` 监听 |
| R2-4 | **硬编码颜色统一到 CSS 变量**：新增 `--graph-status-*`/`--graph-progress-*`/`--graph-ctx-*` 变量 |
| R2-5 | **共享工具函数提取**：新建 `features/graph/utils.ts`，5 个函数，6 个文件改为 import |

### P2 — 体验优化

| 项 | 变更 |
|----|------|
| R2-6 | **节点状态过渡动画**：border-color/box-shadow 过渡 160ms→300ms |
| R2-7 | **卡片列表入场动画**：`animation: graph-card-in 0.3s ease-out both` |
| R2-8 | **编辑器响应式布局**：1024px/768px 断点适配 |

## 第三轮优化

### P1 — 重要改进

| 项 | 变更 |
|----|------|
| R3-1 | **GraphEditorCanvas watch 性能优化**：拆分为 6 个精确 watch + fingerprint 缓存去重 |
| R3-2 | **GraphPropertyPanel 数据流合规**：全部 v-model 改为 `:model-value` + `@update:model-value`，新增 `nodeChange`/`graphChange` emit |

### P2 — 体验优化

| 项 | 变更 |
|----|------|
| R3-3 | **无障碍 ARIA 标注**：右键菜单 `role="menu"` + 菜单项 `role="menuitem"` + 节点 `role="group"` |
| R3-4 | **连线目标高亮 + 拖拽视觉预览**：虚线连线样式 + 自定义拖拽幽灵元素 |
| R3-5 | **画布节点搜索（Ctrl+F）**：新建 `GraphNodeSearch.vue`，模糊匹配 + 上/下导航 |

## 第四轮优化

### P1 — 重要改进

| 项 | 变更 |
|----|------|
| R4-1 | **撤销/重做系统**：新建 `useGraphUndoRedo.ts`，Command 模式双栈结构（最大深度 50），9 个 `push*` 命令工厂（addNode/deleteNode/duplicateNode/addEdge/deleteEdge/deleteConditionalEdge/setProperty/setGraphProperty/disconnectNode）；集成到 `useGraphEditorPage.ts`（Ctrl+Z 撤销、Ctrl+Shift+Z 重做）和 `GraphEditorCanvas.vue`（节点增删复制、连线增删、断开操作均走 undo 栈）；工具栏新增 undo/redo 按钮；删除节点确认对话框移除"此操作不可撤销"提示 |
| R4-2 | **GraphEditorCanvas undoRedo 集成**：`duplicateNode`/`deleteNode`/`disconnectNode`/`onConnect`/`onDrop` 全部通过 `undoRedo.push*()` 执行，保留 `emit("updateGraph")` 作为无 undoRedo 时的回退 |

### P2 — 体验优化

| 项 | 变更 |
|----|------|
| R4-3 | **GraphTaskDetailDrawer 硬编码默认值修复**：`agentKey`/`reviewerAgent`/`commentAuthor` 默认值从硬编码字符串改为空字符串；切换任务时重置全部表单字段 |
| R4-4 | **_graph-pages.sass 拆分**：提取 Team 编排样式（`.team-orchestrate-*`/`.team-run-observatory-*`/`.workflow-kanban-*`/`.orch-*` 等）到独立 `_team-orchestrate.sass`；`app-theme.sass` 新增 `@import ./theme/team-orchestrate`；`_graph-pages.sass` 仅保留 Graph 相关规则 |

### P3 — 视觉增强

| 项 | 变更 |
|----|------|
| R4-5 | **MiniMap 节点按类型着色**：`<MiniMap :node-color="miniMapNodeColor" />`，根据 `NODE_TYPE_STYLES[node.type].borderColor` 返回对应颜色 |

## 第五轮优化

### P2 — 重要改进

| 项 | 变更 |
|----|------|
| R5-1 | **PropertyPanel 全面 undo 集成**：`GraphPropertyPanel.vue` 新增 `undoRedo` prop，`updateNodeField`/`updateGraphField`/`updateStateField` 全部记录 oldValue 并通过 `pushSetProperty`/`pushSetGraphProperty`/`pushSetStateProperty` 入栈；`addStateField`/`removeStateField` 通过 `pushAddStateField`/`pushRemoveStateField` 入栈；`GraphEditorPage.vue` 传递 `:undo-redo="undoRedo"` |
| R5-2 | **条件路由可视化编辑**：`GraphPropertyPanel.vue` 为 router 节点新增「条件路由」折叠面板，展示 `conditionalEdges` 的 pathMap 条目（标签 + 目标节点），支持添加/删除/编辑 pathMap 条目和 condFuncRef，全部走 undoRedo；`useGraphUndoRedo.ts` 新增 `pushSetConditionalPathMap`/`pushSetCondFuncRef`/`pushAddConditionalEdge` 三个命令 |
| R5-3 | **setEntry/setFinish undo 集成**：`GraphEditorCanvas.vue` 右键菜单「设为入口/结束节点」操作改用 `pushSetGraphProperty` 入栈，可撤销 |

### P3 — 体验优化

| 项 | 变更 |
|----|------|
| R5-4 | **画布缩放指示器**：`GraphEditorCanvas.vue` 新增缩放指示器 UI（`-`/百分比/`+` 按钮），使用 VueFlow `onViewportChange` 追踪缩放级别，`zoomTo`/`fitView` 实现缩放操作；`_graph-pages.sass` 新增 `.graph-editor-canvas__zoom-indicator`/`__zoom-text` 样式 |

## 第六轮优化

### P3 — 重要改进

| 项 | 变更 |
|----|------|
| R6-1 | **画布边删除 undo 集成**：`onEdgesChange` 中普通边删除走 `pushDeleteEdge`，条件边删除走 `pushDeleteConditionalEdge`，键盘 Delete 删除边现在可撤销 |
| R6-2 | **VueFlow 内置节点删除统一**：`onNodesChange` 中 `change.type === "remove"` 改为调用 `deleteNode()` 函数，统一走 undoRedo + 确认对话框，移除了重复的删除逻辑和"此操作不可撤销"提示 |

### P4 — 代码质量

| 项 | 变更 |
|----|------|
| R6-3 | **GraphPropertyPanel 拆分**：提取条件路由编辑逻辑到 `useConditionalRoutes.ts` composable（`features/graph/`），`GraphPropertyPanel.vue` script 从 ~250 行降至 ~150 行，符合 ≤~200 行规范 |
| R6-4 | **条件边 labelBgStyle 硬编码提取**：`rgba(244,114,182,0.15)` → `var(--graph-cond-edge-label-stroke)` CSS 变量（亮色 `0.15` 透明度，暗色 `0.25` 透明度增强可见性） |

## 第七轮优化

### P3 — 重要改进

| 项 | 变更 |
|----|------|
| R7-1 | **批量节点操作**：`GraphEditorCanvas.vue` 新增 `deleteSelectedNodes()` 函数，支持多选节点批量删除（确认对话框 + undoRedo `pushDeleteNodes` 批量命令）；画布空白区域右键菜单（多选时显示"删除选中 N 个节点"）；`onNodesChange` 多节点删除走 `deleteSelectedNodes` |
| R7-2 | **执行历史列表页**：新建 `useGraphExecutionsPage.ts` composable + `GraphExecutionsPage.vue` 页面；Store 新增 `loadExecutionHistory` action + `executionHistory`/`executionHistoryLoading`/`executionHistoryNextToken` 状态；路由 `graphs/:id/executions`；Graph 编辑器工具栏新增"执行历史"按钮；`_graph-pages.sass` 新增执行历史卡片列表样式 |

### P4 — 代码质量

| 项 | 变更 |
|----|------|
| R7-3 | **useConditionalRoutes 全面响应式改造**：所有参数（`graphDef`/`selectedNodeId`/`undoRedo`/`destinationOptions`）改为 `MaybeRef<T>` 类型 + `toValue()` 解包，computed 正确追踪响应式依赖变化 |

## 第八轮优化

### P4 — 重要改进

| 项 | 变更 |
|----|------|
| R8-1 | **执行历史页面增强**：`GraphExecutionsPage.vue` 新增状态筛选（`q-btn-toggle`：全部/已完成/运行中/失败/已中断），`filteredHistory` computed 客户端筛选，`execDuration()` 执行时长格式化（ms/s/m），记录数显示，空筛选结果提示；`useGraphExecutionsPage.ts` 新增 `statusFilter` ref |
| R8-2 | **批量节点移动 undoRedo 集成**：`useGraphUndoRedo.ts` 新增 `pushMoveNodes` 命令（记录 `{nodeId, oldPos, newPos}[]`，undo/redo 通过 `writeGraphNodePosition` 恢复位置）；`GraphEditorCanvas.vue` 新增 `onNodeDragStart` 事件处理（追踪拖拽前所有选中节点位置），`onNodeDragStop` 改为对比新旧位置并走 undoRedo，支持单节点和多节点拖拽移动撤销 |

## 第九轮优化

### P5 — 代码质量（修复第八轮审查建议项）

| 项 | 变更 |
|----|------|
| R9-1 | **filteredHistory 迁入 composable**：`filteredHistory` computed 从 Page 迁入 `useGraphExecutionsPage.ts`，与 `statusFilter`/`timeRangeFilter` 放在一起，Page 仅解构使用 |
| R9-2 | **UI 常量提取**：新建 `features/graph/graphExecutionsUi.ts`，包含 `STATUS_FILTER_OPTIONS`/`TIME_RANGE_OPTIONS` 常量 + `statusColor`/`statusLabel`/`timeRangeStart` 函数；Page 改为 import |
| R9-3 | **execDuration 迁入 utils**：`execDuration(startedAt, finishedAt)` 从 Page 迁入 `features/graph/utils.ts`，签名改为接收两个字符串参数（更通用） |

### P4 — 功能增强

| 项 | 变更 |
|----|------|
| R9-4 | **执行历史时间范围筛选**：`useGraphExecutionsPage.ts` 新增 `timeRangeFilter` ref + `filteredHistory` 支持状态+时间双重筛选；`graphExecutionsUi.ts` 新增 `TIME_RANGE_OPTIONS`（全部时间/今天/最近7天/最近30天）+ `timeRangeStart()` 函数；`GraphExecutionsPage.vue` 新增第二个 `q-btn-toggle` 时间范围筛选 |

## 第十轮优化

### P3 — 画布交互增强

| 项 | 变更 |
|----|------|
| R10-1 | **画布框选支持**：`GraphEditorCanvas.vue` 添加 `SelectionMode.Partial`（部分选择模式，支持框选多个节点），`elements-selectable=true`，VueFlow 原生框选功能启用 |
| R10-2 | **Ctrl+A 全选节点快捷键**：`onCanvasKeydown` 新增 `Ctrl+A` / `Cmd+A` 处理，遍历 `getNodes.value` 设置 `selected=true` |
| R10-3 | **边重连 undoRedo 集成**：`useGraphUndoRedo.ts` 新增 `pushReconnectEdge(edgeIdx, oldFrom, oldTo, newFrom, newTo)` 命令（第 22 个 push* 命令）；`GraphEditorCanvas.vue` 新增 `@edge-update` 事件 + `onEdgeUpdate` 处理函数，支持拖拽边的 source/target handle 重连到新节点 |

### P4 — 服务端筛选准备

| 项 | 变更 |
|----|------|
| R10-4 | **执行历史服务端筛选参数传递**：`api.ts` 的 `listGraphExecutions` 新增 `filters?: { status?: string; startedAfter?: string }` 可选参数，传递给 `svc.ListGraphExecutions`；`stores/graph/index.ts` 的 `loadExecutionHistory` 新增 `filters` 参数透传；`useGraphExecutionsPage.ts` 新增 `serverFilters` computed + `watch([statusFilter, timeRangeFilter], reload)` 筛选条件变化时重新加载，当前客户端筛选作为回退 |

## 第十一轮优化

### P4 — 后端服务端筛选实现

| 项 | 变更 |
|----|------|
| R11-1 | **Proto 筛选字段**：`graph.proto` 的 `ListGraphExecutionsRequest` 新增 `optional string status = 4` + `optional google.protobuf.Timestamp started_after = 5`；`make api` 重新生成 Go + TS |
| R11-2 | **Biz 层筛选参数**：`graph.go` 新增 `GraphRunListOption` struct（`Status string` + `StartedAfter *time.Time`）；`GraphRunRepo.ListRunsByGraph` 签名改为 `...opts ...GraphRunListOption`；`GraphUsecase.ListExecutions` 透传 opts |
| R11-3 | **Data 层筛选实现**：`graph.go` 的 `ListRunsByGraph` 新增 `graphexecution.StatusEQ(opt.Status)` + `graphexecution.StartedAtGTE(*opt.StartedAfter)` 条件 |
| R11-4 | **Service 层参数提取**：`graph_execution_service.go` 从 `req.Status`/`req.StartedAfter` 提取筛选参数，构建 `GraphRunListOption` 传给 Usecase |
| R11-5 | **测试 mock 同步**：`graph_team_execution_test.go` 的 `memGraphRunRepo.ListRunsByGraph` 签名同步更新 |

### P3 — 画布交互增强

| 项 | 变更 |
|----|------|
| R11-6 | **节点对齐辅助线**：新建 `useSnapGuide.ts` composable，计算拖拽节点与静态节点的中心/边缘对齐线（6px 阈值），返回 `SnapLine[]`；`GraphEditorCanvas.vue` 集成 `@node-drag` 事件 + SVG 渲染对齐线；`_graph-pages.sass` 新增 `.snap-guide-layer`/`.snap-guide-line` 样式（虚线 + `var(--color-accent)`） |
| R11-7 | **Graph 运行页步骤详情优化**：`GraphRunSidebar.vue` 新增步骤展开/折叠（`expandedSteps` Set + `toggleStep`/`toggleAll`），展开后显示 `inputState`/`outputState` JSON 预览；错误步骤高亮（脉冲动画 + 红色左边框 + 浅红背景）；`_graph-pages.sass` 新增 `.graph-run-step--error`/`__error`/`__detail`/`__json` 样式 + `@keyframes graph-step-error-pulse`/`graph-step-expand` |

### P2 — 自动布局增强

| 项 | 变更 |
|----|------|
| R11-8 | **Dagre 自动布局**：安装 `dagre` + `@types/dagre`；`graphLayout.ts` 的 `applyAutoLayout` 从手写 BFS 拓扑排序替换为 Dagre 分层布局算法（`rankdir: "LR"`，`nodesep: 60`，`ranksep: 120`），支持条件路由边（`conditionalEdges.pathMap` 目标节点），布局更紧凑专业 |

## 新增 CSS 变量

| 变量 | 亮色模式 | 暗色模式 | 用途 |
|------|----------|----------|------|
| `--graph-status-running` | `#0891b2` | `#22d3ee` | 运行中状态色 |
| `--graph-status-completed` | `#059669` | `#34d399` | 已完成状态色 |
| `--graph-status-failed` | `#db2777` | `#f472b6` | 失败状态色 |
| `--graph-status-interrupted` | `#d97706` | `#fbbf24` | 中断状态色 |
| `--graph-progress-from` | `#059669` | `#34d399` | 进度条渐变起点 |
| `--graph-progress-to` | `#0891b2` | `#22d3ee` | 进度条渐变终点 |
| `--graph-ctx-bg` | `rgba(255,253,245,0.97)` | `rgba(3,7,18,0.97)` | 右键菜单背景 |
| `--graph-ctx-border` | `rgba(212,137,26,0.18)` | `rgba(34,211,238,0.12)` | 右键菜单边框 |
| `--graph-ctx-text` | `#3A322C` | `#a5f3fc` | 右键菜单文字 |
| `--graph-ctx-accent` | `#D4891A` | `#22d3ee` | 右键菜单强调色 |
| `--graph-ctx-danger` | `#E55C5C` | `#f472b6` | 右键菜单危险色 |
| `--graph-ctx-success` | `#4CAF7C` | `#34d399` | 右键菜单成功色 |
| `--graph-ctx-glow` | `rgba(212,137,26,0.4)` | `rgba(34,211,238,0.6)` | 右键菜单发光 |
| `--graph-ctx-danger-glow` | `rgba(229,92,92,0.4)` | `rgba(244,114,182,0.6)` | 右键菜单危险发光 |
| `--graph-ctx-success-glow` | `rgba(76,175,124,0.4)` | `rgba(52,211,153,0.6)` | 右键菜单成功发光 |
| `--graph-cond-edge-label-stroke` | `rgba(244,114,182,0.15)` | `rgba(244,114,182,0.25)` | 条件边标签描边 |

## 新增文件

| 文件 | 说明 |
|------|------|
| `web/src/features/graph/utils.ts` | 共享工具函数（truncate/formatTime/relativeTime/stepIcon/stepColor/execDuration） |
| `web/src/features/graph/useGraphUndoRedo.ts` | 撤销/重做 Command 模式 composable（22 个 push* 命令） |
| `web/src/features/graph/useConditionalRoutes.ts` | 条件路由编辑 composable（7 个方法，全面 MaybeRef 响应式） |
| `web/src/features/graph/useGraphExecutionsPage.ts` | 执行历史列表页 composable |
| `web/src/components/graph/GraphNodeSearch.vue` | 画布节点搜索浮层组件 |
| `web/src/pages/GraphExecutionsPage.vue` | 执行历史列表页 |
| `web/src/css/theme/_team-orchestrate.sass` | Team 编排页面独立样式 |
| `web/src/features/graph/graphExecutionsUi.ts` | 执行历史 UI 常量 + 辅助函数（STATUS_FILTER_OPTIONS/TIME_RANGE_OPTIONS/statusColor/statusLabel/timeRangeStart） |
| `web/src/features/graph/useSnapGuide.ts` | 节点对齐辅助线 composable（computeSnapLines/clearSnapLines/snapLines） |

## 修改文件

| 文件 | 变更 |
|------|------|
| `GraphContextMenu.vue` | 移除 scoped 样式（迁移至全局 SASS），添加 ARIA 属性 |
| `GraphPropertyPanel.vue` | v-model→:model-value+emit；新增 undoRedo prop；updateNodeField/updateGraphField/updateStateField 全面 undo 集成；addStateField/removeStateField undo 集成；新增条件路由编辑面板（router 节点）；条件路由逻辑拆分到 useConditionalRoutes composable |
| `GraphEditorCanvas.vue` | watch 拆分优化，连线高亮，Ctrl+F 搜索，undoRedo 集成，MiniMap 着色，缩放指示器，setEntry/setFinish undo 集成，边删除 undo 集成，节点删除统一走 deleteNode，批量删除（deleteSelectedNodes + pushDeleteNodes），画布空白区域右键菜单，批量节点移动 undoRedo（onNodeDragStart + onNodeDragStop pushMoveNodes），画布框选（SelectionMode.Partial），Ctrl+A 全选，边重连 undoRedo（onEdgeUpdate + pushReconnectEdge），节点对齐辅助线（useSnapGuide + @node-drag + SVG 渲染） |
| `GraphRunSidebar.vue` | 步骤展开/折叠（expandedSteps Set + toggleStep/toggleAll + inputState/outputState JSON 预览），错误步骤高亮（脉冲动画 + 红色左边框 + 浅红背景），状态 Badge |
| `GraphFlowNode.vue` | ARIA 属性，import truncate from utils |
| `GraphFlowDiamond.vue` | ARIA 属性 |
| `GraphNodePalette.vue` | 自定义拖拽幽灵元素 |
| `GraphTaskDetailDrawer.vue` | 移除硬编码默认值，切换任务重置表单 |
| `GraphCheckpointPanel.vue` | import formatTime from utils |
| `GraphVersionPanel.vue` | import formatTime from utils |
| `GraphTaskKanbanCard.vue` | import truncate from utils |
| `GraphEditorPage.vue` | undo/redo 工具栏按钮，传入 undoRedo 到 PropertyPanel，执行历史按钮 |
| `useGraphEditorPage.ts` | Ctrl+S/Ctrl+Z/Ctrl+Shift+Z 快捷键，undoRedo 初始化与导出，goToExecutions |
| `useGraphRunPage.ts` | import formatTime/stepIcon/stepColor from utils |
| `useGraphsPage.ts` | import relativeTime from utils |
| `useGraphUndoRedo.ts` | 新增 pushSetStateProperty/pushAddStateField/pushRemoveStateField/pushSetConditionalPathMap/pushSetCondFuncRef/pushAddConditionalEdge/pushDeleteNodes/pushMoveNodes/pushReconnectEdge |
| `graphLayout.ts` | 自动布局从 BFS 拓扑排序替换为 Dagre 分层布局（支持条件路由边） |
| `stores/graph/index.ts` | 新增 loadExecutionHistory action + executionHistory/executionHistoryLoading/executionHistoryNextToken 状态 |
| `_graph-pages.sass` | CSS 变量体系、右键菜单全局样式、搜索浮层样式、动画、响应式断点、缩放指示器样式、条件边标签描边变量；移除 Team 编排样式 |
| `app-theme.sass` | 新增 `@import ./theme/team-orchestrate` |

## 代码审查（aranea-review 第七轮 ~ 第十一轮）

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — 业务逻辑归属** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

### 审查中发现的阻断项（已修复）

| ID | 文件 | 问题描述 | 修复 |
|----|------|----------|------|
| B1 | GraphPropertyPanel.vue (R5) | `updateCondFuncRef` 错误使用 `pushSetProperty`（节点属性命令）修改条件边属性 | 新增 `pushSetCondFuncRef` 专用命令 |
| B2 | useConditionalRoutes.ts (R6) | `selectedNodeId` 参数类型为 `string \| null`，但传入 `computed(() => ...)` 导致类型不匹配 + `routerConditionalEdges` computed 永远返回空数组 | 改为 `MaybeRef<string \| null>` 类型 + `toValue()` 解包 |
| B3 | GraphEditorCanvas.vue (R8) | `onNodeDragStop` 中 `writeGraphNodePosition` 被冗余调用两次（直接调用 + pushMoveNodes→execute→redo 内调用） | 移除直接调用，仅在无 undoRedo 回退路径中调用 |

### 已修复的建议项（第九轮全部修复）

| ID | 维度 | 原问题 | 修复 |
|----|------|--------|------|
| S1 | FB2 | `filteredHistory` computed 在 Page 中而非 Composable | ✅ 迁入 `useGraphExecutionsPage.ts` |
| S2 | FB6 | `statusFilterOptions`/`statusColor`/`statusLabel` UI 常量定义在 Page 中 | ✅ 提取到 `graphExecutionsUi.ts` |
| S3 | FB7 | `execDuration` 纯工具函数定义在 Page 中 | ✅ 迁入 `features/graph/utils.ts` |

### 合规性清单

- [x] 展示组件无 Store/API import（红线 #1/#2）
- [x] Page 无直接 API import（红线 #11）— GraphExecutionsPage 通过 composable + Store
- [x] Dialog/浮层 emit 而非内部调 API（红线 #4）
- [x] 新 HTTP 调用在 api.ts（红线 #7）— `listGraphExecutions` 已存在于 api.ts
- [x] 聊天消息分组用堆栈模型（红线 #14）— 无涉及
- [x] 浮层 backdrop-filter 成对（FU3）
- [x] 主按钮用 --color-accent（FU7）
- [x] Dialog 用 app-dialog-card（FU4）— 无新增
- [x] Page script ≤~200 行 — GraphEditorPage ✅；GraphPropertyPanel ✅（~150 行）；GraphExecutionsPage ✅（~20 行）
- [x] 组件类型从 types.ts 引入（FL4）
- [x] 硬编码颜色已全部提取为 CSS 变量（FU1）
- [x] UI 常量在 *Ui.ts（FB6）— graphExecutionsUi.ts ✅
- [x] 筛选逻辑在 Composable（FB2）— filteredHistory 在 useGraphExecutionsPage ✅
- [x] 构建验证通过（pnpm build 成功）

### 亮点

- ✅ 批量节点操作：多选 + 右键菜单 + 批量删除 undo，完整交互闭环
- ✅ 执行历史列表页：从 API → Store → Composable → Page 完整数据流，分页加载
- ✅ useConditionalRoutes 全面响应式改造：所有参数 MaybeRef + toValue，消除响应式丢失风险
- ✅ 批量节点移动 undoRedo：onNodeDragStart 追踪位置 + onNodeDragStop pushMoveNodes，单节点/多节点拖拽均支持撤销
- ✅ 执行历史双重筛选：状态 + 时间范围，客户端零延迟
- ✅ 代码质量全面合规：Page 精简至 ~20 行，筛选逻辑在 composable，UI 常量在 *Ui.ts，工具函数在 utils.ts
- ✅ 九轮优化后，Graph 编辑器核心功能完整：撤销/重做覆盖全部操作（增删改移）、批量操作、执行历史（双重筛选）、条件路由可视化
- ✅ 十轮优化后，画布交互全面增强：框选、全选、边重连、批量移动全部支持 undoRedo；执行历史服务端筛选参数已就绪
- ✅ 十一轮优化后，全栈功能完整：后端服务端筛选、节点对齐辅助线、步骤详情展开/折叠/错误高亮、Dagre 专业自动布局

## 第十二轮优化

### P1 — Bug 修复

| 项 | 变更 |
|----|------|
| R12-1 | **useSnapGuide 变量声明顺序修复**：`internalNodes` 声明移到 `useSnapGuide` 调用之前，修复 `internalNodes` 在声明前被使用导致可能传入 undefined 的潜在运行时 bug |

### P2 — 功能增强

| 项 | 变更 |
|----|------|
| R12-2 | **自动布局 undoRedo 集成**：`applyAutoLayout` 返回 `NodeMoveInfo[]`，编辑器更多菜单新增「自动布局」选项，通过 `pushMoveNodes` 记录操作，支持 Ctrl+Z 撤销自动布局 |
| R12-3 | **小地图执行状态着色**：`miniMapNodeColor` 增强为优先显示执行状态颜色（running/completed/failed/interrupted），无执行状态时回退到节点类型颜色 |

### P3 — 代码质量

| 项 | 变更 |
|----|------|
| R12-4 | **节点尺寸常量统一提取**：`NODE_DEFAULT_WIDTH=180`/`NODE_DEFAULT_HEIGHT=80` 提取到 `types.ts`，`graphLayout.ts`/`useSnapGuide.ts`/`GraphEditorCanvas.vue` 统一引用，消除 3 处硬编码 |
| R12-5 | **emptyExecNodeStates 常量化**：提取为模块级 `EMPTY_EXEC_NODE_STATES` 常量，避免每次组件渲染创建新 Map 实例 |
| R12-6 | **deleteNode 重复 findIndex 修复**：`onOk` 回调内 `findIndex` 结果使用 early return 模式，消除冗余的 `if (idx >= 0)` 嵌套 |
| R12-7 | **执行 Dialog 共享组件抽取**：新建 `GraphRunDialog.vue`，GraphsPage 和 GraphEditorPage 中的重复执行 Dialog 模板替换为共享组件 |
| R12-8 | **GraphRunSidebar 优化**：`formatJson` 添加 4096 字符截断保护；`expandedSteps` 从 `ref(new Set())` 改为 `reactive(new Set())` 避免每次 toggle 创建新实例；`formatTime`/`stepIcon`/`stepColor` 改为直接 import 而非 props 传递，减少 3 个不必要的 props |

## 新增文件

| 文件 | 说明 |
|------|------|
| `web/src/components/graph/GraphRunDialog.vue` | 执行 Graph 共享 Dialog 组件（props/emits 模式，app-dialog-card + app-glass-dialog） |

## 修改文件

| 文件 | 变更 |
|------|------|
| `types.ts` | 新增 `NODE_DEFAULT_WIDTH`/`NODE_DEFAULT_HEIGHT` 常量 |
| `graphLayout.ts` | 使用共享常量；`applyAutoLayout` 返回 `NodeMoveInfo[]`；新增 `NodeMoveInfo` 类型导出 |
| `useSnapGuide.ts` | 使用共享常量；移除未使用的 `watch` import |
| `GraphEditorCanvas.vue` | `internalNodes`/`internalEdges` 声明移到 `useSnapGuide` 之前；`EMPTY_EXEC_NODE_STATES` 常量化；`deleteNode` early return；`miniMapNodeColor` 执行状态着色增强 |
| `useGraphEditorPage.ts` | 新增 `autoLayout` 方法（undoRedo 集成）；导入 `applyAutoLayout` |
| `GraphEditorPage.vue` | 更多菜单新增「自动布局」选项；执行 Dialog 替换为 `GraphRunDialog` 组件 |
| `GraphsPage.vue` | 执行 Dialog 替换为 `GraphRunDialog` 组件 |
| `GraphRunSidebar.vue` | `formatJson` 截断；`expandedSteps` 改为 `reactive`；直接 import `formatTime`/`stepIcon`/`stepColor` |
| `GraphRunInspector.vue` | 移除 `formatTime`/`stepIcon`/`stepColor` props |
| `GraphRunPage.vue` | 移除传递给 `GraphRunInspector` 的 `formatTime`/`stepIcon`/`stepColor` props |

## 代码审查（aranea-review 第十二轮）

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 1 | 0 | 1 |
| **前端 — 业务逻辑归属** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 0 | 1 | 1 |
| **构建与回归** | 0 | 0 | 0 | 0 |

### 建议项

| ID | 维度 | 文件 | 问题描述 |
|----|------|------|----------|
| S1 | FL3 | GraphRunDialog.vue | `sessionId`/`initialState` 可改为 `defineModel` 双向绑定，减少 Page 中事件绑定代码 |

### 合规性清单

- [x] 展示组件无 Store/API import（红线 #1/#2）
- [x] Page 无直接 API import（红线 #11）
- [x] Dialog/浮层 emit 而非内部调 API（红线 #4）
- [x] 新 HTTP 调用在 api.ts（红线 #7）
- [x] 聊天消息分组用堆栈模型（红线 #14）
- [x] 浮层 backdrop-filter 成对（FU3）
- [x] Dialog 用 app-dialog-card（FU4）
- [x] Page script ≤~200 行
- [x] 组件类型从 types.ts 引入（FL4）
- [x] 硬编码颜色已全部提取为 CSS 变量（FU1）
- [x] 构建验证通过（pnpm build 成功）

## 剩余工作

| 优先级 | 项目 | 说明 | 复杂度 |
|--------|------|------|--------|
| P3 | Graph 运行页子图嵌套展示 | 步骤时间线支持子图步骤缩进/折叠展示（需后端 WS 事件增加 subgraph_id 字段） | 高 |
| P3 | Ent 复合索引优化 | 为 `(graph_id, status, started_at)` 添加复合索引优化组合查询 | 低 |
| P2 | Graph 模板市场 | 预置模板浏览 + 一键创建 | 中 |

### 已完成（第十二轮）

| 优先级 | 项目 | 说明 |
|--------|------|------|
| ~~P2~~ | ~~Graph 编辑器小地图节点着色增强~~ | ✅ R12-3：小地图执行状态着色增强，优先显示执行状态颜色 |
| ~~P4~~ | ~~节点尺寸硬编码~~ | ✅ R12-4：`NODE_DEFAULT_WIDTH`/`NODE_DEFAULT_HEIGHT` 统一提取到 types.ts |
| ~~P4~~ | ~~useSnapGuide 变量声明顺序~~ | ✅ R12-1：`internalNodes` 声明移到 `useSnapGuide` 调用之前 |
| ~~P4~~ | ~~执行 Dialog 重复~~ | ✅ R12-7：抽取 `GraphRunDialog.vue` 共享组件 |
| ~~P4~~ | ~~formatJson 无截断~~ | ✅ R12-8：添加 4096 字符截断保护 |
| ~~P3~~ | ~~自动布局不可撤销~~ | ✅ R12-2：`applyAutoLayout` undoRedo 集成 |

## 验证

```bash
cd web && pnpm build
```
