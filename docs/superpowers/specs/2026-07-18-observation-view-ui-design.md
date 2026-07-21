# 观测视图（Observation View）UI/UX 设计文档

> **Status**: Draft
> **Date**: 2026-07-18
> **Scope**: Chat UI 内 ComfyUI 风格的成员节点实时观测画布

***

## 1. 设计目标

### 1.1 业务定位

观测视图是**任务下达者的实时监控面板**。用户向团队下达任务后，在此视图看到：

* 每个成员正在做什么（状态）

* 每个成员产出了什么（媒体预览）

* 整体进展如何（依赖链 + 完成度）

### 1.2 设计原则

| 原则         | 说明                        |
| ---------- | ------------------------- |
| **一眼全局**   | 打开即看到所有节点状态和依赖关系，无需滚动或点击  |
| **状态驱动视觉** | 节点颜色/边框/动画直接反映当前状态，不需要读文字 |
| **产出即所见**  | 媒体产出直接内嵌在节点上，不需要跳转        |
| **信息分层**   | 默认显示关键信息，点击展开详细信息         |
| **流畅交互**   | 拖拽、缩放、平移零延迟，动画过渡自然        |

***

## 2. 节点抽象模型

### 2.1 节点是什么

观测画布上的每个节点代表**执行计划中的一个步骤**（PlanStep），映射关系：

```
PlanBoard (1:1) → GraphStage
PlanStep  (1:1) → GraphNode
PlanStep  (1:1) → TeamStage (组装后)
TeamStage (1:N) → MemberSession (团队成员)
```

**一个节点 = 一个子任务 = 一个执行单元**。它可能对应：

* 单个 Agent 执行任务

* 一个 Team 协作执行任务

* 一个人工审批节点

### 2.2 节点数据结构（前端）

```typescript
/** 观测节点数据 — Vue Flow node.data */
interface ObserveNodeData {
  // ── 身份 ──
  nodeId: string;            // GraphNode.ID (= PlanStep.ID)
  label: string;             // 节点标题（子任务名称）
  dagNodeId: string;         // 对应的 PlanStep.ID
  teamStageId: string;       // 对应的 TeamStage.ID（组装后非空）

  // ── 状态 ──
  status: GraphNodeStatus;   // pending | running | completed | failed | interrupted
  dependsOn: string[];       // 依赖的上游节点 ID 列表

  // ── 成员信息 ──
  members: NodeMember[];     // 执行该节点的成员列表
  activeMemberCount: number; // 当前活跃成员数

  // ── 产出 ──
  mediaOutput: MediaArtifact[]; // 已产出的媒体文件
  textOutput?: string;          // 文本产出摘要（截取前 200 字符）

  // ── 进度 ──
  progress?: NodeProgress;      // 实时进度（媒体生成等长任务）

  // ── 时间 ──
  startedAt?: string;        // 开始时间
  completedAt?: string;      // 完成时间
  durationMs?: number;       // 执行耗时

  // ── 错误 ──
  error?: string;            // 错误信息（failed 时）
}

/** 节点成员 */
interface NodeMember {
  agentKey: string;
  agentName: string;
  avatarUrl?: string;
  status: MemberSessionStatus; // pending | running | completed | failed | skipped
  currentAction?: string;      // 当前正在执行的动作描述
}

/** 节点进度 */
interface NodeProgress {
  value: number;    // 当前值
  max: number;      // 最大值
  label?: string;   // 进度描述（如 "采样中 30%"）
}
```

### 2.3 节点类型

当前系统只有**一种节点类型**（`observe`），但视觉上需要区分：

| 节点类型         | 判定条件                   | 视觉差异             |
| ------------ | ---------------------- | ---------------- |
| **Agent 节点** | members.length === 1   | 单头像，单名称          |
| **Team 节点**  | members.length > 1     | 多头像堆叠，团队名称 + 成员数 |
| **入口节点**     | dependsOn.length === 0 | 左侧绿色左边界标记        |
| **出口节点**     | 无下游节点                  | 右侧蓝色右边界标记        |

***

## 3. 状态机

### 3.1 节点状态转换

```
                    ┌──────────┐
                    │ pending  │ ← 初始状态（已创建，等待依赖完成）
                    └────┬─────┘
                         │ 所有依赖节点 completed
                         ▼
                    ┌──────────┐
           ┌───────│ running  │───────┐
           │       └────┬─────┘       │
           │            │             │
           ▼            ▼             ▼
    ┌──────────┐ ┌──────────┐ ┌────────────┐
    │completed │ │  failed  │ │interrupted │
    └──────────┘ └──────────┘ └────────────┘
     正常完成      执行出错     被用户/系统中断
```

### 3.2 状态判定规则

| 状态          | 判定条件                                      | 来源                                      |
| ----------- | ----------------------------------------- | --------------------------------------- |
| pending     | PlanStep 已创建，但至少一个依赖未完成                   | `MapPlanStepToGraphNodeStatus(pending)` |
| running     | 对应 TeamStage 正在执行，或至少一个 MemberSession 在运行 | TeamStage.status = running              |
| completed   | 所有成员执行成功，产出已生成                            | TeamStage.status = completed            |
| failed      | 至少一个成员执行失败（且策略不是 skip/continue）           | TeamStage.status = failed               |
| interrupted | 用户取消或系统超时中断                               | TeamStage.status = cancelled            |

### 3.3 状态视觉映射

| 状态          | 边框颜色                   | 背景                       | 动画           | 图标                |
| ----------- | ---------------------- | ------------------------ | ------------ | ----------------- |
| pending     | `--color-border` (灰)   | `--color-surface`        | 无            | `hourglass_empty` |
| running     | `--color-warning` (橙)  | `--color-surface`        | 边框脉冲 + 进度条动画 | `bolt`            |
| completed   | `--color-positive` (绿) | `color-mix(positive 5%)` | 无            | `check_circle`    |
| failed      | `--color-negative` (红) | `color-mix(negative 5%)` | 无            | `error`           |
| interrupted | `--color-warning` (橙)  | `--color-surface`        | 无            | `pause_circle`    |

***

## 4. 视觉设计规范

### 4.1 节点卡片结构

```
┌─────────────────────────────────┐
│ ●── AgentName          [STATUS] │  ← Header: 状态指示灯 + 名称 + 状态徽章
│    ┌─────┐                       │
│    │ AV  │  Sub-task description │  ← Body: 头像 + 描述
│    └─────┘                       │
│    ▓▓▓▓▓▓▓▓░░░░  60%  采样中   │  ← Progress (仅 running)
│    ┌────┐ ┌────┐ ┌────┐         │  ← Media Preview (最多 3 个缩略图)
│    │img1│ │img2│ │ +2 │         │
│    └────┘ └────┘ └────┘         │
│    ⚡ Generating image...        │  ← Current Action (仅 running)
│    2.3s                          │  ← Duration (运行中/已完成)
└─────────────────────────────────┘
```

### 4.2 节点尺寸

| 属性   | 值                            | 说明                 |
| ---- | ---------------------------- | ------------------ |
| 最小宽度 | 200px                        | 保证名称可读             |
| 最大宽度 | 260px                        | 防止过宽影响布局           |
| 最小高度 | 80px                         | 至少显示 header + 一行信息 |
| 内边距  | 10px                         | 舒适的内容间距            |
| 圆角   | 10px                         | 现代感                |
| 边框宽度 | 2px                          | 状态色边框              |
| 阴影   | `0 2px 8px rgba(0,0,0,0.08)` | 浮起感                |

### 4.3 颜色系统

使用项目已有的 CSS 变量体系，不引入新颜色：

```sass
// 节点基础
--observe-node-bg: var(--color-surface)
--observe-node-border: var(--color-border)
--observe-node-text: var(--color-text-primary)
--observe-node-text-secondary: var(--color-text-secondary)
--observe-node-text-tertiary: var(--color-text-tertiary)

// 状态色（复用全局变量）
--observe-pending: var(--color-text-tertiary)
--observe-running: var(--color-warning)
--observe-completed: var(--color-positive)
--observe-failed: var(--color-negative)
--observe-interrupted: var(--color-warning)
```

### 4.4 字体规范

| 元素   | 字号   | 字重  | 颜色                       |
| ---- | ---- | --- | ------------------------ |
| 节点标题 | 13px | 600 | `--color-text-primary`   |
| 节点描述 | 11px | 400 | `--color-text-secondary` |
| 状态徽章 | 10px | 500 | 对应状态色                    |
| 进度标签 | 10px | 400 | `--color-text-tertiary`  |
| 当前动作 | 11px | 400 | `--color-text-tertiary`  |
| 耗时   | 10px | 400 | `--color-text-tertiary`  |
| 成员名  | 11px | 500 | `--color-text-primary`   |

### 4.5 动画

```sass
// Running 节点边框脉冲
@keyframes observe-pulse
  0%, 100%
    box-shadow: 0 0 0 0 rgba(255, 152, 0, 0.4)
  50%
    box-shadow: 0 0 0 8px rgba(255, 152, 0, 0)

// Running 节点进度条呼吸
@keyframes observe-progress-breathe
  0%, 100%
    opacity: 1
  50%
    opacity: 0.7

// 节点出现（新节点加入时）
@keyframes observe-node-enter
  from
    opacity: 0
    transform: scale(0.9)
  to
    opacity: 1
    transform: scale(1)

// 状态变化过渡
.observe-node
  transition: border-color 0.3s ease, box-shadow 0.3s ease, background 0.3s ease
```

***

## 5. 信息展示策略

### 5.1 按状态展示不同信息

| 信息     | pending | running | completed | failed | interrupted |
| ------ | ------- | ------- | --------- | ------ | ----------- |
| 节点标题   | ✅       | ✅       | ✅         | ✅      | ✅           |
| 状态徽章   | ✅       | ✅       | ✅         | ✅      | ✅           |
| 头像     | ✅       | ✅       | ✅         | ✅      | ✅           |
| 成员列表   | ✅       | ✅       | ✅         | ✅      | ✅           |
| 进度条    | ❌       | ✅       | ❌         | ❌      | ❌           |
| 当前动作   | ❌       | ✅       | ❌         | ❌      | ❌           |
| 媒体预览   | ❌       | ✅ (部分)  | ✅         | ❌      | ❌           |
| 文本产出摘要 | ❌       | ❌       | ✅         | ❌      | ❌           |
| 耗时     | ❌       | ✅ (累计)  | ✅ (总耗时)   | ❌      | ❌           |
| 错误信息   | ❌       | ❌       | ❌         | ✅      | ❌           |
| 依赖列表   | 悬停显示    | 悬停显示    | 悬停显示      | 悬停显示   | 悬停显示        |

### 5.2 媒体预览策略

* 最多显示 **3 个缩略图**（64x64px），超过显示 `+N`

* 图片直接显示；视频显示 poster 帧 + 播放图标

* 点击缩略图打开 MediaLightbox 全屏预览

* 缩略图圆角 6px，间距 4px

### 5.3 成员展示策略

* **单成员**：显示头像 + 名称

* **多成员（Team）**：头像堆叠（最多 3 个）+ `+N` + 团队名称

* 成员状态指示点（头像右下角）：

  * 绿点 = completed

  * 橙点 = running

  * 红点 = failed

  * 灰点 = pending/skipped

***

## 6. 交互设计

### 6.1 画布交互

| 交互   | 操作         | 效果                         |
| ---- | ---------- | -------------------------- |
| 平移   | 拖拽空白区域     | 画布跟随移动                     |
| 缩放   | 滚轮         | 以鼠标为中心缩放（0.2x - 2.0x）      |
| 适应视图 | 双击空白区域     | 自动缩放至所有节点可见                |
| 节点拖拽 | 拖拽节点       | 节点跟随移动（仅影响视觉布局，不影响 DAG 结构） |
| 框选   | Shift + 拖拽 | 选中多个节点（预留，暂不实现多选操作）        |

### 6.2 节点交互

| 交互   | 操作    | 效果                             |
| ---- | ----- | ------------------------------ |
| 点击节点 | 单击    | 打开 ObserveNodeDetail 侧边栏       |
| 双击节点 | 双击    | 居中并放大到该节点                      |
| 悬停节点 | 鼠标悬停  | 节点高亮（边框加粗 + 阴影加深）+ 显示依赖tooltip |
| 点击媒体 | 单击缩略图 | 打开 MediaLightbox               |
| 拖拽节点 | 拖拽    | 更新节点位置（Vue Flow 内置，不影响 DAG 数据） |

### 6.3 边交互

| 交互  | 操作   | 效果                     |
| --- | ---- | ---------------------- |
| 悬停边 | 鼠标悬停 | 边高亮（颜色加深）              |
| 点击边 | 单击   | 高亮显示该依赖链（上游 → 当前 → 下游） |

### 6.4 节点位置持久化

* 节点拖拽后的新位置**仅保存在前端内存**，不持久化到后端

* 刷新页面后恢复 DAG 自动布局

* 未来可扩展为保存到 localStorage

***

## 7. 布局算法

### 7.1 DAG 自动布局

使用项目已有的 `usePlanDAGLayout` 算法，基于 DAG 拓扑排序：

```
输入: GraphNode[] (含 DependsOn)
输出: Map<nodeId, {x, y}> + computedWidth
```

布局参数：

```typescript
const LAYOUT_OPTS = {
  width: 800,        // 画布参考宽度
  nodeWidth: 220,    // 节点宽度（含间距）
  nodeHeight: 120,   // 节点高度（含间距）
  gapX: 60,          // 水平间距
  gapY: 80,          // 垂直间距
  padX: 40,          // 左边距
};
```

### 7.2 布局规则

1. **拓扑排序**：按依赖关系分层，入口节点在第 0 层
2. **同层居中**：同一层的节点水平居中对齐
3. **线性收缩**：如果只有一条链，节点宽度收缩到最小
4. **并行展开**：并行分支的节点水平展开，宽度由最宽层决定

### 7.3 动态更新

* 新节点加入时，重新计算布局

* 节点状态变化不触发重新布局（避免跳动）

* 用户拖拽后切换回自动布局需手动触发（预留"重置布局"按钮）

***

## 8. 组件架构

```
ObservationPanel (容器)
├── Toolbar (刷新 + 实时状态指示)
├── ObservationCanvas (Vue Flow 画布)
│   ├── VueFlow
│   │   ├── Background (网格背景)
│   │   ├── ObserveNode[] (自定义节点)
│   │   │   ├── NodeHeader (头像 + 名称 + 状态徽章)
│   │   │   ├── NodeProgress (进度条，仅 running)
│   │   │   ├── NodeMediaPreview (媒体缩略图)
│   │   │   ├── NodeMembers (成员列表/堆叠)
│   │   │   ├── NodeAction (当前动作，仅 running)
│   │   │   └── NodeFooter (耗时/错误信息)
│   │   └── Edge[] (依赖边)
├── ObserveNodeDetail (侧边栏，点击节点时显示)
│   ├── NodeInfo (节点基本信息)
│   ├── MemberList (成员列表 + 各自状态)
│   ├── MediaGrid (媒体产出网格)
│   ├── ActivityTimeline (该节点的活动时间线)
│   └── ErrorPanel (错误详情，仅 failed)
└── MediaLightbox (全屏媒体预览)
```

***

## 9. 数据流

```
Backend (Sequencer)
  → GraphStageCreatedEvent → WS → activityV2Store.graphStages.set()
  → GraphNodeUpdatedEvent  → WS → activityV2Store.graphNodes.set()
  → TeamStageCreatedEvent  → WS → activityV2Store.teamStages.set()
  → StepUpdatedEvent       → WS → activityV2Store.steps.set()

Frontend
  activityV2Store
    → useObserveGraph(spiritSessionId)
      → graphStage (computed)
      → nodes (computed: 优先 graphNodes map → fallback graphStage.Nodes)
      → flowNodes (computed: GraphNode → Vue Flow Node)
      → flowEdges (computed: DependsOn → Vue Flow Edge)
    → ObservationCanvas (props: flowNodes, flowEdges)
    → ObserveNode (props: data = ObserveNodeData)
      → NodeMediaPreview (props: artifacts = mediaOutput)
      → NodeProgress (props: progress)
```

***

## 10. 当前实现差距分析

| 差距      | 当前状态                                    | 目标状态                    | 优先级 |
| ------- | --------------------------------------- | ----------------------- | --- |
| 节点不可拖动  | `:nodes-draggable="false"`              | 启用拖拽，提升交互灵活性            | P0  |
| UI 过于简单 | 只有 header + progress + media + activity | 丰富的节点卡片：头像/成员/耗时/错误/描述  | P0  |
| 无成员信息   | 不显示执行者                                  | 显示成员头像+名称+状态点           | P1  |
| 无耗时显示   | 不显示执行时间                                 | 显示累计耗时/总耗时              | P1  |
| 无错误详情   | 失败节点无错误信息                               | 失败时显示错误摘要               | P1  |
| 无文本产出   | 只显示媒体产出                                 | completed 节点显示文本产出摘要    | P2  |
| 边样式单一   | 所有边一样                                   | 依赖边有方向箭头 + 颜色区分         | P2  |
| 无节点类型区分 | 所有节点一样                                  | Agent 节点 vs Team 节点视觉差异 | P2  |

***

## 11. 验收标准

1. 节点可自由拖拽，拖拽后位置在画布内保持
2. 每种状态有明确的视觉区分（颜色 + 图标 + 动画）
3. running 节点显示进度条和当前动作
4. completed 节点显示媒体产出和耗时
5. failed 节点显示错误信息
6. 多成员节点显示成员头像堆叠和状态点
7. 点击节点打开详情侧边栏
8. 画布支持滚轮缩放和拖拽平移
9. 新节点加入时有入场动画
10. 状态变化有平滑过渡动画

