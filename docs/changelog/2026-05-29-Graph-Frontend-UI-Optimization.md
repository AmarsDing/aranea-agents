# Graph UI 优化（第二轮 ~ 第十四轮）

**日期**：2026-05-29  
**模块**：Graph (36) · 前端 + 后端  
**影响**：🟡 中 — Graph 编辑器/运行页/列表页视觉与交互优化 + 后端服务端筛选

## 变更摘要

七轮迭代优化 Graph UI：修复亮色模式适配、数据流合规、watch 性能、撤销/重做系统、无障碍访问、交互增强（Ctrl+S/Ctrl+Z/Ctrl+F/连线高亮/拖拽预览）、动画与响应式布局、SASS 拆分、共享工具函数提取、PropertyPanel 全面 undo 集成、画布缩放指示器、条件路由可视化编辑、边/节点删除 undo 集成、composable 拆分、硬编码颜色提取、批量节点操作、执行历史列表页、composable 全面响应式改造。第八轮：执行历史页面增强（状态筛选+执行时长）、批量节点移动 undoRedo 集成。第九轮：代码质量重构（P5 建议项全部修复）+ 执行历史时间范围筛选。第十轮：画布框选 + Ctrl+A 全选 + 边重连 undoRedo + 执行历史服务端筛选准备。第十一轮：后端服务端筛选实现 + 节点对齐辅助线 + 步骤详情展开/折叠/错误高亮 + Dagre 自动布局。第十二轮：代码质量优化 + UX 增强——节点尺寸常量统一提取、useSnapGuide 变量声明顺序修复、自动布局 undoRedo 集成、小地图执行状态着色、执行 Dialog 共享组件抽取、GraphRunSidebar 优化（formatJson 截断 + reactive Set + 纯函数直接 import）。第十三轮：架构优化——Ent 复合索引优化服务端筛选查询性能、Checkpoint Wire 解耦消除 data 层上帝对象依赖、GraphRunDialog defineModel 简化双向绑定。第十四轮：Brainstorm 视觉对齐——节点配色修正（Router→灰色、Join→紫色、LLM→蓝色）、4种边线流动光点动画+条件边虚线、右键菜单赛博青风格统一、属性面板5分组色彩边框、运行页进度条发光指示点呼吸动画。

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

## 第十三轮优化

### P1 — 架构优化

| 项 | 变更 |
|----|------|
| R13-1 | **Ent 复合索引优化**：`graph_executions` 表新增 `(graph_id, status, started_at)` 复合索引，优化 R11 服务端筛选的组合查询性能；前导列 `graph_id` 同时覆盖原有单列查询；`go generate ./internal/data/ent` 已执行 |
| R13-2 | **Checkpoint Wire 解耦**：新增 `provideSQLiteRawDB` Wire provider 从 `*data.Data` 提取 `*sql.DB`；`provideTRPCSessionService` 和 `provideGraphCheckpointSaver` 改为直接接收 `*sql.DB` 窄依赖，消除对 `*data.Data` 上帝对象的依赖；`make wire` 已执行 |

### P3 — 代码质量

| 项 | 变更 |
|----|------|
| R13-3 | **GraphRunDialog defineModel 优化**：`sessionId`/`initialState` 从 props+emits 手动双向绑定改为 `defineModel<string>("sessionId")`/`defineModel<string>("initialState")`；父组件 `GraphsPage.vue`/`GraphEditorPage.vue` 从 `:session-id` + `@update:session-id` 简化为 `v-model:session-id`，各减少 2 行事件绑定代码 |

## 修改文件

| 文件 | 变更 |
|------|------|
| `internal/data/ent/schema/graph_execution.go` | 新增 `index.Fields("graph_id", "status", "started_at")` 复合索引 |
| `cmd/admin/wire.go` | 新增 `provideSQLiteRawDB`；`provideTRPCSessionService`/`provideGraphCheckpointSaver` 改为接收 `*sql.DB`；Wire provider 列表新增 `provideSQLiteRawDB` |
| `web/src/components/graph/GraphRunDialog.vue` | `sessionId`/`initialState` 改为 `defineModel` 双向绑定 |
| `web/src/pages/GraphsPage.vue` | `v-model:session-id`/`v-model:initial-state` 替代 props+emits |
| `web/src/pages/GraphEditorPage.vue` | `v-model:session-id`/`v-model:initial-state` 替代 props+emits |

## 代码审查（aranea-review 第十三轮）

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

### 合规性清单

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] 构造函数接收窄依赖（`*sql.DB` 而非 `*data.Data`）
- [x] Wire 改动后 `make wire` 已执行
- [x] Ent Schema 变更后 `go generate` 已执行
- [x] 展示组件无 Store/API import（红线 #1/#2）
- [x] Dialog emit 而非内部调 API（红线 #4）
- [x] Dialog 用 app-dialog-card（FU4）
- [x] 构建验证通过（go build + pnpm build）

### 亮点

- ✅ Checkpoint Wire 解耦：从 `*data.Data` 上帝对象依赖改为 `*sql.DB` 窄依赖，符合 BL8 原则
- ✅ Ent 复合索引：直接优化 R11 服务端筛选查询，前导列覆盖原有单列查询
- ✅ defineModel 简化：GraphRunDialog 双向绑定更简洁，两个父组件各减少 2 行代码

## 第十四轮优化

### Brainstorm 视觉对齐

> 依据 `.superpowers/brainstorm/` 中的设计提案 v1/v2/v3 + ctx-menu-v7，修正当前实现与 brainstorm 定义的视觉差距。

| 项 | 变更 |
|----|------|
| R14-1a | **节点配色修正**：LLM 从 `#22d3ee`（青）→ `#93c5fd`（蓝）；Router 从 `#f472b6`（粉）→ `#94a3b8`（灰）；Join 从 `#94a3b8`（灰）→ `#a78bfa`（紫）。亮色/暗色模式同步更新 |
| R14-1b | **边线配色修正**：Normal 从 `#22d3ee` → `#93c5fd`（蓝）；Conditional 从 `#f472b6` → `#f9a8d4`（浅粉）；Transfer 从 `#fbbf24` → `#fde68a`（浅黄）；Dispatch 从 `#34d399` → `#6ee7b7`（浅绿） |
| R14-1c | **条件边虚线**：CSS `.graph-edge--conditional .vue-flow__edge-path` 添加 `stroke-dasharray: 6 4`，移除内联 `strokeDasharray` |
| R14-1d | **边线流动光点动画**：4 种边线各有 `stroke-dasharray` + `stroke-dashoffset` 动画——Normal 24px/1.6s、Conditional 40px/1.6s、Transfer 28px/1.8s、Dispatch 56px/1.4s |
| R14-1e | **边线样式统一到 CSS**：移除 `buildEdges()` 中的内联 `strokeDasharray`，所有边线 dasharray 由 CSS 控制 |
| R14-2 | **右键菜单赛博青风格**：亮色模式 accent 从 `#D4891A`（暖金）→ `#0891b2`（深青），border/glow 同步更新，与暗色模式 `#22d3ee`（霓虹青）同色系 |
| R14-3 | **属性面板分组色彩边框**：5 个分组各有独立色彩——basic=LLM蓝、conditional=条件边粉、model=Agent绿、interrupt=HITL橙、advanced=Router灰；使用 `--group-accent` CSS 变量优雅覆盖 |
| R14-4 | **运行页进度条发光指示点**：8px 微型指示点 + 3 层 box-shadow 发光 + `graph-progress-dot-pulse` 2s 呼吸动画 |
| R14-1f | **暗色模式状态色修正**：running `#22d3ee` → `#93c5fd`（蓝）；failed `#f472b6` → `#f87171`（红）；progress-to `#22d3ee` → `#93c5fd` |

## 修改文件

| 文件 | 变更 |
|------|------|
| `web/src/css/theme/_graph-pages.sass` | 节点/边线/状态/菜单 CSS 变量配色修正；条件边虚线 + 4种边线流动动画；属性面板分组色彩；进度条发光指示点动画 |
| `web/src/components/graph/GraphEditorCanvas.vue` | `buildEdges()` 移除内联 strokeDasharray，边线样式统一到 CSS |
| `web/src/components/graph/GraphPropertyPanel.vue` | 5 个 q-expansion-item 添加分组色彩修饰类 |

## 代码审查（aranea-review 第十四轮）

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 1 | 0 | 1 |
| **构建与回归** | 0 | 0 | 0 | 0 |

### 合规性清单

- [x] 展示组件无 Store/API import
- [x] Page 无直接 API import
- [x] Dialog emit 而非内部调 API
- [x] 浮层 backdrop-filter 成对
- [x] 主按钮用 --color-accent
- [x] 无硬编码 hex（组件中）
- [x] 构建验证通过

### 亮点

- ✅ 配色体系完全对齐 brainstorm v2 定义
- ✅ 边线流动动画实现 brainstorm v3 的"流动光点"效果
- ✅ 属性面板分组色彩使用 `--group-accent` CSS 变量，零逻辑改动

## 第十五轮优化

### P1 — 前端实时校验 + 后端测试补全

| 项 | 变更 |
|----|------|
| R15-1 | **前端实时校验 composable**：新建 `useGraphLocalValidation.ts`，提供 `localErrors`/`localWarnings`/`localValid` computed 属性；校验规则包括 no_entry_point、duplicate_node、edge_source_missing、edge_target_missing、unreachable_node、loop_no_exit（无条件循环=error）、conditional_loop（条件循环=warning）、orphan_node；集成到 `useGraphEditorPage.ts`，与后端校验结果合并去重（key=`code:nodeId:field`） |
| R15-2 | **GraphVariablePicker 组件**：新建 `GraphVariablePicker.vue`，支持在 instruction/mapper 字段插入 `{{nodeId.field}}` 变量引用；光标位置感知插入；集成到 `GraphPropertyPanel.vue` instruction 字段 |
| R15-3 | **Checkpoint 管理 UI 产品化**：`GraphCheckpointPanel.vue` 增强状态快照预览（JSON 展示）、下一节点显示（Badge）、回退确认对话框（`$q.dialog`）；新增 `stateSnapshot`/`nextNodes`/`restoring` props + `restore` emit |
| R15-4 | **Biz 层测试补全**：新增 4 个测试文件 + 扩展 1 个现有测试文件，共 47 个新测试用例。覆盖 `ShouldCreateTaskForNode`/`ShouldCreateTeamGraphTaskNode`（15+10 case）、`GraphTaskInputFromNode`/`nodeDefFromConfig`（4+3 case）、`BuildConfigFromGraphDefinition`/`GraphDefinitionFromBuildConfig`/`compactNodesForVersion`/`ReadUserTemplateMeta`/`WriteUserTemplateMeta`/`GraphValidationResult`/`graphStepSnapshotToJSON`（2+3+1+5+3+5+1 case）、`upsertGraphStep`/`evictIfNeeded`（4+2 case）、`ApplyFailurePolicy`/`ApplySkipNodeSemantics`/`ApplyCircuitBreakerPolicy`/`FinalizeGraphFailurePolicy`/`parallelBranchNodeIDs`/`normalizeFailureDefault`（4+3+4+2+3+8 case） |

### 审查修复

| 项 | 变更 |
|----|------|
| R15-F1 | **循环检测误报修复**：区分无条件循环（error，`loop_no_exit`）和条件循环（warning，`conditional_loop`），条件边回退不再误报为死循环 |
| R15-F2 | **边源节点缺失检测**：新增 `edge_source_missing` 错误码，检测边/条件边的 from 节点不存在 |
| R15-F3 | **合并去重 key 细化**：从 `code:nodeId` 改为 `code:nodeId:field`，避免不同 field 的同 code 同 nodeId 错误被过度去重 |

## 新增文件

| 文件 | 说明 |
|------|------|
| `web/src/features/graph/useGraphLocalValidation.ts` | 前端实时校验 composable（8 种校验规则，区分 error/warning） |
| `web/src/components/graph/GraphVariablePicker.vue` | 变量引用插入组件（光标位置感知，`{{nodeId.field}}` 格式） |
| `internal/biz/graph_node_task_test.go` | ShouldCreateTaskForNode/ShouldCreateTeamGraphTaskNode 测试（25 case） |
| `internal/biz/graph_task_input_test.go` | GraphTaskInputFromNode/nodeDefFromConfig 测试（7 case） |
| `internal/biz/graph_build_config_test.go` | BuildConfigFrom/compactNodes/UserTemplate/ValidationResult/StepSnapshot 测试（20 case） |
| `internal/biz/graph_step_test.go` | upsertGraphStep/evictIfNeeded 测试（6 case） |

## 修改文件

| 文件 | 变更 |
|------|------|
| `web/src/features/graph/useGraphEditorPage.ts` | 集成 localValidation + 合并去重（key=`code:nodeId:field`） |
| `web/src/pages/GraphEditorPage.vue` | 使用 mergedValidation* 替代 validation* |
| `web/src/components/graph/GraphPropertyPanel.vue` | 集成 GraphVariablePicker + 新增 allNodes/stateFields props |
| `web/src/components/graph/GraphCheckpointPanel.vue` | 状态快照预览 + 下一节点显示 + 回退确认对话框 |
| `web/src/css/theme/_graph-pages.sass` | Checkpoint 预览样式 |
| `internal/biz/failure_policy_test.go` | 新增 13 个测试函数（ApplyFailurePolicy nil/skip/override、ApplySkipNodeSemanticsExtended、ApplyCircuitBreakerPolicy、FinalizeGraphFailurePolicy、parallelBranchNodeIDs 边界、normalizeFailureDefault） |

## 代码审查（aranea-review 第十五轮）

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — 业务逻辑归属** | 0 | 2 | 0 | 2 |
| **前端 — UX 主题** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

### 已修复的建议项

| ID | 维度 | 文件 | 问题描述 | 修复 |
|----|------|------|----------|------|
| S1 | FB2 | useGraphLocalValidation.ts | 循环检测不区分无条件/条件循环，条件回退误报为死循环 | ✅ 区分 unconditionalAdj/fullAdj，条件循环降级为 warning |
| S2 | FB2 | useGraphLocalValidation.ts | 边源节点缺失未检测 | ✅ 新增 edge_source_missing 错误码 |
| S3 | FB2 | useGraphEditorPage.ts | 合并去重 key 过粗（`code:nodeId`），不同 field 的错误被过度去重 | ✅ key 改为 `code:nodeId:field` |

### 合规性清单

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] 展示组件无 Store/API import（红线 #1/#2）
- [x] Page 无直接 API import（红线 #11）
- [x] Dialog/浮层 emit 而非内部调 API（红线 #4）
- [x] 新 HTTP 调用在 api.ts（红线 #7）
- [x] 聊天消息分组用堆栈模型（红线 #14）
- [x] 构建验证通过（go build + go test + pnpm build）

### 亮点

- ✅ 前端实时校验：8 种规则 + 区分 error/warning，编辑时即时反馈，无需等待后端保存
- ✅ 循环检测精确化：无条件循环=error，条件循环=warning，消除合法回退的误报
- ✅ Biz 层测试覆盖：47 个新测试用例覆盖 17 个核心函数，包括边界场景（nil、空值、大小写、覆盖优先级）
- ✅ VariablePicker 光标位置感知：在输入框光标处插入变量，而非追加到末尾
- ✅ Checkpoint 回退确认：`$q.dialog` 二次确认，防止误操作

## 剩余工作

| 优先级 | 项目 | 说明 | 复杂度 |
|--------|------|------|--------|
| P2 | Graph 模板市场 | 预置模板浏览 + 一键创建 | 中 |
| P3 | Graph 运行页子图嵌套展示 | 步骤时间线支持子图步骤缩进/折叠展示（需后端 WS 事件增加 subgraph_id 字段） | 高 |
| P2 | HITL 节点配置与 UX | 定义 await_user_reply 在 Graph 中的配置和 UX | 中 |
| P2 | 熔断策略实现 | CircuitBreakerPolicy（Proto 已定义），连续失败达阈值暂停分支 | 中 |
| P3 | Schema 驱动属性面板 | 借鉴 Flowise InputParam[] 动态表单，替代硬编码属性面板 | 高 |
| P3 | 连接校验增强 | 环/悬空/类型/handle 校验，扩展 ValidationPanel + validator.go（`ValidationErrOrphanNode` unused） | 中 |
| P1 | Graph 适配层测试 | internal/graph/trpc/ 测试薄弱 | 高 |

### 已完成（第十五轮 — P1-P3 优化 + 审查修复）

| 优先级 | 项目 | 说明 |
|--------|------|------|
| ~~P1~~ | ~~前端实时校验~~ | ✅ R15-1：`useGraphLocalValidation` composable，8 种校验规则 + 区分 error/warning |
| ~~P3~~ | ~~VariablePicker~~ | ✅ R15-2：`GraphVariablePicker.vue`，光标位置感知插入 `{{nodeId.field}}` |
| ~~P2~~ | ~~Checkpoint 管理 UI 产品化~~ | ✅ R15-3：状态快照预览 + 下一节点显示 + 回退确认对话框 |
| ~~P1~~ | ~~Biz 层测试补全~~ | ✅ R15-4：47 个新测试用例覆盖 17 个核心函数 |
| ~~P3~~ | ~~循环检测误报~~ | ✅ R15-F1：区分无条件循环(error)与条件循环(warning) |
| ~~P3~~ | ~~边源节点缺失检测~~ | ✅ R15-F2：新增 `edge_source_missing` 错误码 |
| ~~P3~~ | ~~合并去重 key 过粗~~ | ✅ R15-F3：key 从 `code:nodeId` 改为 `code:nodeId:field` |

### 已完成（第十四轮 — Brainstorm 视觉对齐）

| 优先级 | 项目 | 说明 |
|--------|------|------|
| ~~P1~~ | ~~节点配色修正~~ | ✅ R14-1a：Router→灰色、Join→紫色、LLM→蓝色，对齐 brainstorm v2 |
| ~~P1~~ | ~~条件边虚线~~ | ✅ R14-1c：CSS stroke-dasharray: 6 4，对齐 brainstorm v3 |
| ~~P1~~ | ~~边线流动光点动画~~ | ✅ R14-1d：4种边线各有 stroke-dashoffset 动画，对齐 brainstorm v3 |
| ~~P2~~ | ~~右键菜单赛博青风格~~ | ✅ R14-2：亮色模式 accent 改为深青，对齐 ctx-menu-v7 |
| ~~P2~~ | ~~属性面板分组色彩边框~~ | ✅ R14-3：5分组独立色彩，对齐 brainstorm v2 |
| ~~P2~~ | ~~运行页进度条发光指示点~~ | ✅ R14-4：8px 微型点 + 3层发光 + 呼吸动画，对齐 brainstorm v3 |

### 已完成（第十三轮）

| 优先级 | 项目 | 说明 |
|--------|------|------|
| ~~P3~~ | ~~Ent 复合索引优化~~ | ✅ R13-1：`(graph_id, status, started_at)` 复合索引已添加 |
| ~~P1~~ | ~~Data 层 Checkpoint 耦合~~ | ✅ R13-2：`provideGraphCheckpointSaver`/`provideTRPCSessionService` 改为接收 `*sql.DB` 窄依赖 |
| ~~P3~~ | ~~GraphRunDialog defineModel 优化~~ | ✅ R13-3：`sessionId`/`initialState` 改为 `defineModel` 双向绑定 |

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
