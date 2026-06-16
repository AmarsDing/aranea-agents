# 聊天 UI 统一事件流重构方案

> 日期：2026-06-15（2026-06-16 核对更新 + 实施更新 + 代码审查修正）
> 状态：Phase 1-3 已完成（含审查修复），Phase 4 待实施。**2026-06-16 审查发现 Phase 1 实际为"API 骨架已完成，运行时集成未完成"**
> 范围：前端聊天 UI + 后端 Activity 类型体系

---

## 一、问题诊断

### 1.1 现状（2026-06-16 核对）

原始问题为三重复杂度叠加，经部分重构后当前状态：

| 层级 | 原始问题 | 当前状态 | 剩余工作 |
|------|---------|---------|---------|
| 双数据路径 | AF 路径 + 消息推理路径（35 个推理路径 / 5,858 行） | **推理路径已删除**，`useConversationTimeline.ts` 从 5,858 行简化到 562 行，仅保留 AF 路径 + Legacy 降级路径 | a2uiParse.ts 未删除（3 处导入仍活跃，但 `shouldUseA2UIView`/`contentLooksLikeA2UIJsonl` 为死代码） |
| 双渲染模型 | `TimelineActivity` + `TaskBoardNodeData` | **已统一**：EventStream 单模型，`activityToStreamEvent` 单映射，TaskBoard.vue/TaskBoardNode.vue 已删除 | activityTimelineTypes.ts 仍保留（ConversationTurn/DelegateActivity 仍在使用） |
| 组件膨胀 | `components/chat/` 下 70+ Vue 文件 | 60 个 Vue 文件（减少约 14%），组件变体已统一为 card 一种 | 删除 task/sub_task_board/delegate Kind 后可进一步减少 |

### 1.2 根因

**前端在替后端做推理**。后端 `ActivityProjector` 已能精确识别 thinking/action/reply/delegate 等语义事件，但前端仍保留从原始 `Message[]` 反推 Activity 的推理路径。双渲染模型则是因为 TaskBoard 和 Timeline 被设计为独立系统，而非同一事件流的不同视图。

### 1.3 核心原则

> **后端知道发生了什么，前端只负责展示。前端不应从原始消息反推 Activity。**

---

## 二、后端消息类型体系

### 2.1 Activity Kind 完整目录（2026-06-16 核对）

后端 `ActivityProjector` 当前实际产生的 Activity Kind 共 **7 种**，全部有常量定义和创建代码：

| Kind | 语义 | 创建时机 | 状态生命周期 | 前端当前渲染 |
|------|------|---------|-------------|-------------|
| `task` | 任务描述（Turn 根节点） | `OnTurnStart` | running → completed | TaskBoard 根节点 / Timeline 不渲染 |
| `thinking` | 推理/思考过程 | `OnReasoningDelta` | running → completed | ThinkingBlock（可折叠，3 变体） |
| `action` | 工具调用 | `OnToolCall` | tool_running → completed / failed | ActionBlock（可展开，2 变体） |
| `reply` | Agent 回复 | `OnTextDelta` | running → completed | ReplyBlock（Markdown，2 变体） |
| `error` | 错误信息 | `OnError` | 直接 failed | ErrorBlock |
| `sub_task_board` | 子任务板（递归） | `OnDelegate` 内部创建子 Activity | running → (无终结) | TaskBoard 嵌套 / Timeline 不渲染 |
| `delegate` | Spirit→Team 委派 | `OnDelegate` | running → (无终结) | 不渲染（两通道均过滤） |

> **勘误**：原文档称 `end`/`notice` "常量已定义，无创建代码"，实际核对发现 `ActivityKindEnd` 和 `ActivityKindNotice` **常量从未定义**。`end` 仅存在于图编译的节点类型（`NodeTypeEnd`），与 Activity Kind 无关。

> **2026-06-16 审查修正**：`OnDelegate` 方法虽已实现，但**无任何生产代码调用方**（仅有方法定义，`stream_consumer.go` 未调用）。因此 `delegate` 和 `sub_task_board` 两种 Kind 在生产环境中**不会产生数据**。

### 2.2 需要补全的 Kind（2026-06-16 核对 → 2026-06-16 审查修正）

当前后端缺少以下语义表达，需要新增：

| Kind | 语义 | 当前状态 | 方案 |
|------|------|---------|------|
| `notice` | 系统通知（进度提示、状态变更等） | **常量已定义**，`OnNotice` 方法已实现，**但无生产调用方** | 需在模型切换、配额警告、降级等场景调用（见 N-03） |
| `confirm` | 用户确认请求 | **常量已定义**，`OnConfirmRequest`/`OnConfirmResult` 方法已实现，**但无生产调用方** | 需在工具需要用户确认时调用 |
| `plan` | 执行计划 | **常量已定义**，`OnPlanStart`/`OnPlanStepUpdate` 方法已实现，**运行时集成已完成**（N-02：`processGraphNodeStart`/`processGraphNodeComplete` 从 `graph.node.*` 事件自动创建 plan Activity 和 steps） | — |

> **勘误**：原文档称 `notice`/`end` "常量已定义，无创建代码"，实际核对发现常量从未定义。原文档称 `delegate` "有 `OnDelegate` 方法但无调用点"，实际 `OnDelegate` **确实无生产调用方**（2026-06-16 审查确认：`stream_consumer.go` 未调用 `OnDelegate`，该方法仅有测试调用）。`end` Kind 从未作为 Activity Kind 存在，无需"激活"。

> **2026-06-16 审查修正**：`notice`/`confirm`/`plan` 三种 Kind 的常量和方法已实现（`OnNotice`/`OnConfirmRequest`/`OnConfirmResult`/`OnPlanStart`/`OnPlanStepUpdate`），但**均无生产代码调用方**，仅有测试调用。这意味着 Phase 1 完成的是"API 骨架"，三种新 Kind 在生产中不会产生数据，前端新组件（PlanBlock/ConfirmBlock/NoticeBlock）无法被触发。
>
> **2026-06-16 更新**：N-02 已完成，`OnPlanStart`/`OnPlanStepUpdate` 已通过 `processGraphNodeStart`/`processGraphNodeComplete` 获得生产调用方。PlanBlock 在生产中可被 graph.node.* 事件触发。`OnNotice`/`OnConfirmRequest`/`OnConfirmResult` 仍无生产调用方。

### 2.3 Activity Status 完整目录（2026-06-16 核对）

9 个 Status 常量全部已定义，但仅 4 个在 ActivityProjector 中实际使用：

| Status | 常量已定义 | ActivityProjector 中使用 | 语义 | 目标用途 |
|--------|----------|------------------------|------|---------|
| `pending` | 是 | 否 | 等待开始 | plan/confirm 的初始状态 |
| `running` | 是 | 是（task/thinking/reply/delegate/sub_task_board 创建时） | 执行中 | 通用运行中 |
| `tool_running` | 是 | 是（action 创建时） | 工具执行中 | action 专用 |
| `tool_blocked` | 是 | 否 | 等待用户确认 | confirm 专用 |
| `completed` | 是 | 是（thinking/reply/tool_result/turn_end 完成时） | 正常完成 | 通用完成 |
| `failed` | 是 | 是（error/on_tool_result 失败/stuck_tools） | 失败 | 通用失败 |
| `partial_failure` | 是 | 否 | 部分失败 | plan 中部分步骤失败 |
| `cancelled` | 是 | 否 | 用户取消 | confirm 被拒绝 |
| `interrupted` | 是 | 否 | 中断 | 外部中断 |

### 2.4 统一后的消息类型体系（7 种）

| Kind | 语义 | 核心字段 | 状态流转 | 视觉角色 |
|------|------|---------|---------|---------|
| `thinking` | 思考/推理 | `content`(reasoning), `label`, `collapsed` | running → completed | 过程展示（默认折叠） |
| `action` | 工具调用 | `toolName`, `toolLabel`, `toolArguments`, `toolResult`, `toolErrorCode`, `toolDurationMs` | tool_running → completed / failed | 过程展示（默认折叠） |
| `reply` | Agent 回复 | `content`, `variant`(default/a2ui) | running → completed | 结果展示（始终展开） |
| `plan` | 执行计划 | `content`(计划描述), `steps`(子 Activity 列表) | pending → running → completed / partial_failure | 结构化概览（始终展开） |
| `confirm` | 用户确认 | `content`(确认提示), `toolName`, `toolArguments` | tool_blocked → completed / cancelled | 阻塞交互（必须操作） |
| `notice` | 系统通知 | `content`, `noticeType`(info/warning/degradation) | pending → completed | 轻量提示（内联） |
| `error` | 错误 | `content`, `errorCode` | 直接 failed | 错误展示（醒目） |

**删除的 Kind**：
- `task`：作为根节点仅起分组作用，统一后由 `ConversationTurn` 承担此职责，无需独立 Activity
- `sub_task_board`：语义由 `plan` 的递归 `steps` 表达
- `delegate`：语义由 `plan`（Team DAG）+ 子 `thinking`/`action`/`reply` 表达

> **注意**：`task` 仍在后端活跃使用（`OnTurnStart` 在生产路径中），但前端 `activityToStreamEvent` 将其 fallback 为 ErrorEvent 并在 `streamEvents` computed 中过滤，属于后端产生、前端静默丢弃的数据浪费。`sub_task_board`/`delegate` 的 `OnDelegate` 无生产调用方，不会产生数据。删除需在 N-02（Plan 集成）完成后同步进行。`end` 从未作为 Activity Kind 存在，无需删除。

---

## 三、前后端交互协议

### 3.1 WS 信封类型（保持不变）

| 信封类型 | 用途 | 关键字段 |
|---------|------|---------|
| `activity_start` | Activity 创建 | 完整 Activity 字段 |
| `activity_delta` | 流式增量 | `delta_field` + `delta_chunk` |
| `activity_done` | Activity 终结 | 最终 status + result |
| `activity_child_start` | 子 Activity 创建 | 父 ID + 子 Activity |

### 3.2 前端消费流程

```
1. 用户发送消息
   → 创建 pending-user 占位消息（role=user）
   → WS 发送 user_message

2. 收到 activity_start(kind=thinking)
   → 创建 ThinkingBlock，显示"正在思考..."脉冲

3. 收到 activity_delta(delta_field="reasoning")
   → ThinkingBlock 流式渲染推理内容

4. 收到 activity_done(kind=thinking, status=completed)
   → ThinkingBlock 折叠，显示摘要

5. 收到 activity_start(kind=plan) [可选]
   → 创建 PlanBlock，展示执行计划

6. 收到 activity_start(kind=action)
   → 创建 ActionBlock，显示工具名 + "执行中..."

7. 收到 activity_done(kind=action, status=completed)
   → ActionBlock 折叠，显示工具名 + 耗时

8. 收到 activity_start(kind=confirm) [可选]
   → 创建 ConfirmBlock，阻塞等待用户操作

9. 收到 activity_start(kind=reply)
   → 创建 ReplyBlock，流式渲染回复

10. 收到 activity_done(kind=reply, status=completed)
    → ReplyBlock 完成，标记 isFinal
```

### 3.3 API 接口（2026-06-16 核对）

| 接口 | 方法 | 用途 | 实现状态 |
|------|------|------|---------|
| `/v1/sessions/{id}/activities` | GET | 加载历史 Activity（会话打开时） | **已实现**（`ListActivities` RPC） |
| `/v1/sessions/{id}/activities?turn_id={tid}` | GET | 加载单个 Turn 的 Activity | **已实现**（`ListActivities` RPC 支持 turn_id 参数） |
| `/v1/sessions/{id}/activities/{aid}/confirm` | POST | 用户确认/拒绝工具调用 | **已实现**（`ConfirmActivity` RPC + `chat_confirm.go`） |

---

## 四、前端 UI 展示设计

### 4.1 统一类型定义

```typescript
// streamEventTypes.ts

type StreamEventStatus =
  | 'pending' | 'running' | 'tool_running' | 'tool_blocked'
  | 'completed' | 'failed' | 'partial_failure' | 'cancelled';

interface StreamEventBase {
  id: string;
  status: StreamEventStatus;
  durationMs?: number;
  agentKey?: string;
  agentName?: string;
}

type StreamEvent =
  | ThinkingEvent
  | ActionEvent
  | ReplyEvent
  | PlanEvent
  | ConfirmEvent
  | NoticeEvent
  | ErrorEvent;

interface ThinkingEvent extends StreamEventBase {
  kind: 'thinking';
  content: string;           // 推理内容（Markdown）
  label?: string;            // 标签：规划 / 推理 / 重规划
  collapsed: boolean;
  streaming: boolean;
  subSteps?: ThinkingEvent[]; // 合并的相邻思考步骤
}

interface ActionEvent extends StreamEventBase {
  kind: 'action';
  toolName: string;
  toolLabel?: string;        // 人类可读的工具标签
  toolArguments?: string;    // JSON
  toolResult?: string;       // JSON 或文本
  toolErrorCode?: string;
  toolDurationMs?: number;
  iconKey?: string;          // 工具图标
  isLongRunning?: boolean;   // 长时间运行标记
}

interface ReplyEvent extends StreamEventBase {
  kind: 'reply';
  content: string;           // Markdown
  isFinal: boolean;          // 是否为最终回复
  streaming: boolean;
  variant: 'default' | 'a2ui';
  a2uiLines?: string[];
}

interface PlanEvent extends StreamEventBase {
  kind: 'plan';
  title: string;             // 计划标题
  steps: PlanStep[];         // 计划步骤
}

interface PlanStep {
  id: string;
  label: string;             // 步骤描述
  status: StreamEventStatus;
  agentName?: string;        // 执行者
  dagNodeId?: string;        // DAG 节点 ID
  dependsOn?: string[];      // 依赖步骤 ID
  children?: StreamEvent[];  // 步骤内的子事件（递归）
}

interface ConfirmEvent extends StreamEventBase {
  kind: 'confirm';
  content: string;           // 确认提示文本
  toolName: string;
  toolArguments?: string;
  autoApproveAt?: number;    // 自动批准时间戳（可选倒计时）
}

interface NoticeEvent extends StreamEventBase {
  kind: 'notice';
  noticeType: 'info' | 'warning' | 'degradation';
  message: string;
}

interface ErrorEvent extends StreamEventBase {
  kind: 'error';
  content: string;
  errorCode?: string;
}
```

### 4.2 组件树

```
ChatMessagePanel (容器)
  ├── ChatComposer (输入框)
  └── ChatMessageList
      └── ConversationTurn[]（每个 user 消息开启一个 Turn）
          ├── UserBubble（用户消息）
          └── EventStream（Agent 工作过程）
              ├── ThinkingBlock    // kind='thinking'
              ├── ActionBlock      // kind='action'
              ├── ReplyBlock       // kind='reply'
              ├── PlanBlock        // kind='plan'
              ├── ConfirmBlock     // kind='confirm'
              ├── NoticeBlock      // kind='notice'
              └── ErrorBlock       // kind='error'
```

### 4.3 各消息类型的 UI 展示设计

#### 4.3.1 ThinkingBlock — 思考过程

**视觉设计**：

```
┌─────────────────────────────────────────────────┐
│ 🧠 思考过程  ···  2.3s                    [▼]  │  ← Header：图标 + 标签 + 脉冲点 + 时长 + 折叠按钮
├─────────────────────────────────────────────────┤
│ ▎ 让我分析一下这个问题...                       │  ← Body：左侧 2px accent 竖线 + Markdown 内容
│ ▎ 首先需要考虑...                               │     流式时：闪烁光标 + accent 竖线
│ ▎ ...                                           │     完成后：竖线变 muted 色
└─────────────────────────────────────────────────┘
```

**交互规则**：

| 状态 | 默认 | 用户操作 |
|------|------|---------|
| streaming（流式中） | 展开，显示脉冲点 + 闪烁光标 | 可手动折叠 |
| completed（完成） | **折叠**，仅显示 Header + 首行摘要（80 字符截断） | 点击展开完整内容 |
| 短文本（< 30 字符纯文本） | 始终展开，无折叠按钮 | — |

**subSteps（合并思考步骤）**：相邻多个 thinking 合并为一个 ThinkingBlock，每个 subStep 显示为缩进的子行：

```
┌─────────────────────────────────────────────────┐
│ 🧠 思考过程（3 步）                    5.1s [▼] │
├─────────────────────────────────────────────────┤
│ ▎ 第一步分析...                                 │  ← subStep 1
│ ▎   第二步推理...                               │  ← subStep 2（缩进）
│ ▎   第三步决策...                               │  ← subStep 3（缩进）
└─────────────────────────────────────────────────┘
```

**设计意图**：思考过程是"过程信息"，用户关心的是 Agent 在做什么，而非推理细节。默认折叠减少视觉噪音，需要时一键展开。

---

#### 4.3.2 ActionBlock — 工具调用

**视觉设计**：

```
折叠状态（默认）：
┌─────────────────────────────────────────────────┐
│ 🔧 搜索网页  ✓  1.2s                           │  ← 工具标签 + 状态图标 + 时长
└─────────────────────────────────────────────────┘

展开状态：
┌─────────────────────────────────────────────────┐
│ 🔧 搜索网页  ✓  1.2s                     [▲]  │
├─────────────────────────────────────────────────┤
│ 参数                                            │
│ ┌─────────────────────────────────────────────┐ │
│ │ {"query": "Vue 3 composition API"}          │ │  ← 代码块，等宽字体，玻璃态背景
│ └─────────────────────────────────────────────┘ │
│ 结果                                            │
│ ┌─────────────────────────────────────────────┐ │
│ │ 找到 15 个相关结果...                       │ │  ← 最大高度 200px，可滚动
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘

失败状态：
┌─────────────────────────────────────────────────┐
│ 🔧 搜索网页  ✗  失败                    [▲]  │  ← danger 色状态图标
├─────────────────────────────────────────────────┤
│ 错误：网络超时 (tool_timeout)                    │  ← 红色边框错误区
└─────────────────────────────────────────────────┘
```

**状态图标映射**：

| 状态 | 图标 | 颜色 |
|------|------|------|
| tool_running | ⏳ | accent（脉冲动画） |
| completed | ✓ | success |
| failed | ✗ | danger |
| tool_blocked | 🔒 | warning |

**特殊工具渲染**：

| 工具名 | 特殊渲染 | 说明 |
|--------|---------|------|
| `todo_write` / `TodoWrite` | `TodoInlineList` | 结构化任务卡片列表，不显示原始 JSON |
| `read_file` / `write_file` | 文件图标 + 文件名 | 参数中提取文件名作为摘要 |
| `search` / `web_search` | 搜索图标 + 查询词 | 参数中提取 query 作为摘要 |
| 其他 | 默认工具标签 | 从 `toolLabel` 获取 |

**交互规则**：

| 状态 | 默认 | 用户操作 |
|------|------|---------|
| tool_running | 展开，显示参数 + "执行中..." | 可手动折叠 |
| completed | **折叠**，仅显示工具名 + ✓ + 时长 | 点击展开参数/结果 |
| failed | **折叠**，显示工具名 + ✗ + "失败" | 点击展开错误详情 |

**设计意图**：工具调用是"过程信息"，用户通常只关心"做了什么"和"花了多久"，默认折叠。失败时需要醒目标记但不自动展开（避免中断阅读流）。

---

#### 4.3.3 ReplyBlock — Agent 回复

**视觉设计**：

```
┌─────────────────────────────────────────────────┐
│ 💬 回复                                   ···  │  ← Header：图标 + 标签 + 流式脉冲点
├─────────────────────────────────────────────────┤
│                                                 │
│  根据 Vue 3 Composition API 的文档，            │  ← Body：玻璃态卡片 + Markdown 渲染
│  推荐使用 `ref` 和 `reactive` 来...             │     14px 字号，1.7 行高
│                                                 │     流式时：闪烁光标
│  ```typescript                                  │
│  const count = ref(0)                           │
│  ```                                            │
│                                                 │
└─────────────────────────────────────────────────┘

最终回复（isFinal=true）：
┌─────────────────────────────────────────────────┐
│ 💬 最终回复                                     │  ← 标签变为"最终回复"
├─────────────────────────────────────────────────┤
│ ...                                             │  ← 同上，但无脉冲点
└─────────────────────────────────────────────────┘
```

**交互规则**：

| 状态 | 默认 | 用户操作 |
|------|------|---------|
| streaming | 展开，脉冲点 + 闪烁光标 | — |
| completed | 展开，始终可见 | — |
| isFinal | 展开，标签为"最终回复" | — |

**设计意图**：回复是"结果信息"，用户最关心的内容，始终展开。`isFinal` 标记帮助用户区分中间回复和最终结论。

---

#### 4.3.4 PlanBlock — 执行计划（新增）

**视觉设计**：

```
┌─────────────────────────────────────────────────┐
│ 📋 执行计划                                     │
│   "分析代码库并生成测试"                         │  ← 计划标题
├─────────────────────────────────────────────────┤
│                                                 │
│  ┌─ ● 步骤 1：读取源代码          ✓  2.1s ──┐ │  ← 已完成步骤
│  │  └ 🧠 思考... ··· 0.8s                  │ │  ← 步骤内子事件（递归）
│  │  └ 🔧 read_file  ✓  1.3s                │ │
│  └──────────────────────────────────────────┘ │
│                                                 │
│  ┌─ ● 步骤 2：分析依赖关系        ⏳ ──────┐ │  ← 运行中步骤
│  │  └ 🧠 思考... ···                       │ │  ← 脉冲动画
│  └──────────────────────────────────────────┘ │
│                                                 │
│  ┌─ ○ 步骤 3：生成测试用例        等待中 ──┐ │  ← 等待步骤（依赖步骤 2）
│  └──────────────────────────────────────────┘ │
│                                                 │
└─────────────────────────────────────────────────┘
```

**步骤状态视觉**：

| 状态 | 圆点 | 颜色 | 附加 |
|------|------|------|------|
| pending | ○ | muted | 显示"等待中" |
| running | ● | accent | 脉冲动画 |
| completed | ● | success | 显示 ✓ + 时长 |
| failed | ● | danger | 显示 ✗ + 错误摘要 |
| partial_failure | ● | warning | 显示 ⚠ + 部分失败提示 |

**DAG 依赖展示**：当步骤有 `dependsOn` 时，用水平连接线表示依赖关系（简化版，不画完整 DAG 图）：

```
  ● 步骤 1：读取代码     ✓
  │
  ├── ● 步骤 2：分析依赖   ⏳
  │
  └── ○ 步骤 3：生成测试   等待步骤 2
```

**交互规则**：

| 状态 | 默认 | 用户操作 |
|------|------|---------|
| 整体 | **始终展开** | — |
| 步骤内子事件 | 默认折叠（同 ThinkingBlock/ActionBlock 规则） | 点击展开 |

**设计意图**：执行计划是"全局概览"，让用户一眼看到 Agent 的整体工作进度。步骤内的子事件默认折叠，避免信息过载。这是替代原 TaskBoard + delegate 的统一方案。

---

#### 4.3.5 ConfirmBlock — 用户确认（新增）

**视觉设计**：

```
┌─────────────────────────────────────────────────┐
│ ⚠️ 需要确认                                     │  ← warning 色背景
│                                                 │
│  Agent 请求执行以下操作：                        │
│                                                 │
│  🔧 delete_file                                 │
│  ┌─────────────────────────────────────────┐    │
│  │ {"path": "/src/legacy/utils.ts"}        │    │  ← 工具参数预览
│  └─────────────────────────────────────────┘    │
│                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │  ✓ 批准  │  │  ✗ 拒绝  │  │ ⏱ 15s 后 │      │  ← 操作按钮
│  └──────────┘  └──────────┘  └──────────┘      │     第三个按钮仅在 autoApproveAt 存在时显示
└─────────────────────────────────────────────────┘

已批准：
┌─────────────────────────────────────────────────┐
│ ✓ 已批准  ·  delete_file                        │  ← success 色左边框
└─────────────────────────────────────────────────┘

已拒绝：
┌─────────────────────────────────────────────────┐
│ ✗ 已拒绝  ·  delete_file                        │  ← danger 色左边框
└─────────────────────────────────────────────────┘
```

**交互规则**：

| 状态 | 默认 | 用户操作 |
|------|------|---------|
| tool_blocked | **始终展开**，阻塞式显示 | 点击"批准"或"拒绝" |
| completed | 折叠，显示"已批准" | — |
| cancelled | 折叠，显示"已拒绝" | — |

**设计意图**：用户确认是"阻塞交互"，必须醒目且必须操作。默认展开且不可折叠，操作完成后折叠为轻量标记。

---

#### 4.3.6 NoticeBlock — 系统通知

**视觉设计**：

```
info 类型：
┌─────────────────────────────────────────────────┐
│ ℹ️ 会话已切换到 GPT-4o 模型                     │  ← glass-surface 背景 + glass-border
└─────────────────────────────────────────────────┘

warning 类型：
┌─────────────────────────────────────────────────┐
│ ⚠️ Token 用量已达到配额的 80%                   │  ← warning 色淡背景 + warning 边框
└─────────────────────────────────────────────────┘

degradation 类型：
┌─────────────────────────────────────────────────┐
│ ⚠️ 部分功能降级：向量搜索不可用                  │  ← warning 色深背景 + warning 边框
└─────────────────────────────────────────────────┘
```

**交互规则**：始终展开，无折叠，无交互（纯展示）。

**设计意图**：通知是"轻量提示"，不打断用户阅读流，但确保信息可见。

---

#### 4.3.7 ErrorBlock — 错误

**视觉设计**：

```
┌─────────────────────────────────────────────────┐
│ ❌ 执行失败                                      │  ← danger 色左边框 + danger 背景
│                                                 │
│ 工具调用超时，请重试                             │  ← 错误描述
│ 错误码：tool_timeout                            │  ← 错误码（次要信息，小字）
└─────────────────────────────────────────────────┘
```

**交互规则**：始终展开，无折叠。如果是可重试错误，显示"重试"按钮。

**设计意图**：错误是"必须关注的信息"，醒目且不可忽略。

---

### 4.4 事件流整体视觉节奏

一个完整的 Agent Turn 事件流视觉示例：

```
┌─────────────────────────────────────────────────┐
│ 👤 用户                                         │
│ "帮我重构这个模块并添加测试"                     │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│ 🤖 Agent                                        │
│                                                 │
│  🧠 思考过程  ···  1.2s                  [▼]   │  ← 思考（折叠）
│                                                 │
│  📋 执行计划                                    │  ← 计划（展开）
│    "重构模块并添加测试"                          │
│    ● 步骤 1：读取源代码          ✓  2.1s        │
│    ● 步骤 2：分析依赖关系        ⏳              │
│    ○ 步骤 3：重构代码            等待中          │
│    ○ 步骤 4：编写测试            等待中          │
│                                                 │
│  🔧 read_file  ✓  1.3s                         │  ← 工具（折叠）
│  🔧 analyze_deps  ✓  0.8s                      │  ← 工具（折叠）
│                                                 │
│  ⚠️ 需要确认                                    │  ← 确认（展开，阻塞）
│  Agent 请求执行：delete_file                     │
│  {"path": "/src/legacy/utils.ts"}               │
│  [✓ 批准]  [✗ 拒绝]                            │
│                                                 │
│  ✓ 已批准  ·  delete_file                       │  ← 确认结果（折叠）
│                                                 │
│  🧠 思考过程  ···  3.5s                  [▼]   │  ← 第二轮思考（折叠）
│  🔧 write_file  ✓  2.0s                        │  ← 工具（折叠）
│  🔧 write_test  ✓  1.8s                        │  ← 工具（折叠）
│                                                 │
│  💬 最终回复                                    │  ← 回复（展开）
│  重构完成！主要变更：                            │
│  1. 提取了 `validateInput` 工具函数             │
│  2. 添加了 12 个单元测试                        │
│  3. 移除了遗留的 utils.ts                        │
└─────────────────────────────────────────────────┘
```

**视觉节奏原则**：

1. **过程折叠，结果展开**：thinking 和 action 默认折叠，reply 始终展开
2. **计划全局可见**：plan 始终展开，作为进度锚点
3. **确认阻塞醒目**：confirm 始终展开且不可忽略
4. **通知轻量穿插**：notice 内联显示，不打断节奏
5. **错误醒目标记**：error 始终展开，不可忽略

---

## 五、重构实施计划（2026-06-16 核对进度）

### 5.1 Phase 1：后端 Activity 类型补全 — **API 骨架已完成，plan 运行时集成已完成，notice/confirm 运行时集成未完成**

**目标**：补全 `notice`、`confirm`、`plan` 三种 Activity Kind。

> **2026-06-16 审查修正**：Phase 1 原标记为"已完成"，但审查发现 `OnNotice`/`OnConfirmRequest`/`OnConfirmResult`/`OnPlanStart`/`OnPlanStepUpdate` 五个方法**均无生产代码调用方**（仅有测试调用）。这意味着 Phase 1 完成的是"API 骨架"——常量定义、方法签名、数据模型、Ent Schema 均已就绪，但运行时不会触发这些方法，前端新组件无法被触发。实际状态应为"**API 骨架已完成，运行时集成未完成**"。

| 任务 | 说明 | 状态 |
|------|------|------|
| 新增 `notice` | 定义 `ActivityKindNotice` 常量，`ActivityProjector` 新增 `OnNotice(content, noticeType)` 方法，在状态变更、模型切换等场景调用 | **API 骨架已完成**，运行时集成未完成（见 N-03） |
| 新增 `confirm` | 定义 `ActivityKindConfirm` 常量，`ActivityProjector` 新增 `OnConfirmRequest(toolName, toolArguments, content)` 方法；新增 `OnConfirmResult(approved)` 方法（含 Kind 校验） | **API 骨架已完成**，运行时集成未完成 |
| 新增 `plan` | 定义 `ActivityKindPlan` 常量，`ActivityProjector` 新增 `OnPlanStart(title, steps)` 和 `OnPlanStepUpdate(stepId, status)` 方法（含 Kind 校验 + 自动推导计划整体状态） | **运行时集成已完成**（N-02：`processGraphNodeStart`/`processGraphNodeComplete` 从 `graph.node.*` 事件自动创建 plan Activity 和 steps） |
| 新增 `ActivityPlanStep` 类型 | biz 层新增 PlanStep 结构体（ID/Label/Status/AgentName/DependsOn） | **已完成** |
| 新增 `Meta` 字段 | `biz.Activity` 新增 `Meta map[string]any` 字段，Ent Schema 同步添加 `field.JSON("meta", ...)` | **已完成** |
| 新增 API | `POST /v1/chat/activities/{activity_id}/confirm` — 用户确认/拒绝 | **已完成**（Proto 定义 + `chat_confirm.go` Service 实现 + Wire 绑定） |
| 删除 `task`/`sub_task_board`/`delegate` Kind | 这三种 Kind 的语义由 ConversationTurn / plan.steps / 子 Activity 承担 | **未实施**（`task` 仍活跃，`sub_task_board`/`delegate` 无生产调用但代码仍存在，需等 N-02 Plan 集成完成） |

### 5.2 Phase 2：前端删除消息推理路径 — **大部分完成**

**目标**：删除 35 个推理路径文件 / 5,858 行代码。

| 任务 | 说明 | 状态 |
|------|------|------|
| 删除推理路径独占组件 | CompactTimeline, TurnBlock, ChatMessageRow, ToolStrip, ChatReactSteps, ToolCallTimeline, ToolCallTimelineItem, TimelineNode, AgentBlock, AgentTreeTimeline（10 个 Vue 组件） | **已完成**（10/10 已删除） |
| 删除推理路径独占 TS 文件 | groupMessagesByTurn, useChatTimeline, useTurnBlock, useChatMessageRow, reactPlannerParse, reactPlannerTypes, reactToolLinkIndex, reactPlannerToolLink, useToolCallTimeline, useToolDisplayMode, timelineTypes, compactTimeline, useAgentBlocks, messagePlannerPresentation, a2uiParse（15 个 TS 文件） | **大部分完成**（14/15 已删除，a2uiParse.ts 仍存在） |
| 删除测试文件 | 10 个推理路径测试文件 | **已完成** |
| 简化 ChatMessageList | 三路分支 → 单路（仅 ConversationTurn） | **已完成** |
| 简化 ChatMessagePanel | 移除 useChatTimeline、虚拟滚动等推理相关逻辑 | **已完成** |
| 删除 useActivityFirstFlag | AF 成为唯一路径，功能开关无意义 | **已完成** |
| 重构共享文件 | ChatExecutionCard（删除推理导入）、executionCardHelpers（删除推理导出）、useAutoCollapse（重写为基于 StreamEvent）、useChatMessageScroll（移除 TurnBlockGroup 依赖） | 未验证 |

### 5.3 Phase 3：前端合并双渲染模型 — **已完成**

**目标**：统一 TimelineActivity + TaskBoardNodeData 为 StreamEvent，统一 ActivityTimeline + TaskBoard 为 EventStream。

| 任务 | 说明 | 状态 |
|------|------|------|
| 新增 `streamEventTypes.ts` | StreamEvent 联合类型定义（含 ConfirmEvent、NoticeEvent） | **已完成** |
| 新增 `EventStream.vue` | 统一事件流渲染组件，按 kind 分发到 7 个子组件 | **已完成** |
| 新增 `ErrorBlock.vue` | 错误渲染组件（从 NoticeActivity 拆分） | **已完成** |
| 新增 `PlanBlock.vue` | 执行计划渲染（替代 TaskBoard + TaskBoardNode + DelegateActivity） | **已完成**（含递归子事件渲染） |
| 新增 `ConfirmBlock.vue` | 用户确认渲染（含批准/拒绝按钮 + 自动批准倒计时） | **已完成** |
| 重构 `AgentWorkPanel.vue` | 互斥分支 → 统一 EventStream | **已完成** |
| 重构 `ChatMessageList.vue` | 三路分支 → 单路 | **已完成** |
| 重构 `ChatMessagePanel.vue` | 移除推理路径逻辑 | **已完成** |
| 重构 `ThinkingBlock.vue` | 三种变体 → 统一一种（card 变体） | **已完成**（移除 inline/compact，仅保留 card） |
| 重构 `ActionBlock.vue` | 两种变体 → 统一一种（card 变体） | **已完成**（移除 compact，仅保留 card） |
| 重构 `ReplyBlock.vue` | 两种变体 → 统一一种（card 变体） | **已完成**（移除 compact，仅保留 card） |
| 重写 `useActivityTimeline.ts` | 双映射 → 单映射 `activityToStreamEvent`（含 notice/confirm 映射） | **已完成** |
| 删除 `TaskBoard.vue` + `TaskBoardNode.vue` | 由 PlanBlock 替代 | **已完成**（文件已删除） |
| 删除 `activityTimelineTypes.ts` 中 TaskBoardNodeData | 移除双渲染类型定义 | **已完成**（ActivityVariant 已删除，TaskBoardNodeData 字段已移除） |
| 删除 `agentTreeTypes.ts` 中 TaskBoardNodeData | 由 StreamEvent 替代 | **已完成**（TaskBoardNodeKind/TaskBoardToolStatus/TaskBoardNodeData 已删除） |
| 前端 ActivityKind 同步 | 添加 `notice`/`confirm` 到前端类型 | **已完成** |
| 前端 Activity.meta 字段 | 添加 meta 字段到前端接口 + WS 解析 | **已完成** |

### 5.4 Phase 4：API 失败恢复 — **未实施**

| 任务 | 说明 |
|------|------|
| 重试机制 | `loadActivitiesFromAPI` 失败时自动重试 2 次（指数退避） |
| 降级提示 | 重试失败后显示"会话数据加载失败"提示 + 重试按钮 |

---

## 六、删除/新增/重构文件清单（2026-06-16 核对进度）

### 6.1 删除文件（原计划 40 个，已完成 37 个）

| 类别 | 原计划文件数 | 已删除 | 未删除 | 行数 |
|------|------------|--------|--------|------|
| 推理路径 Vue 组件 | 10 | 10 | 0 | ~2,746 |
| 推理路径 TS 文件 | 15 | 14 | 1（a2uiParse.ts） | ~2,267 |
| 推理路径测试文件 | 10 | 10 | 0 | ~1,512 |
| 双渲染模型文件 | 4 | 2（TaskBoard.vue, TaskBoardNode.vue） | 2（activityTimelineTypes.ts, agentTreeTypes.ts 部分） | ~600 |
| 功能开关 | 1 | 1 | 0 | ~15 |
| **合计** | **40** | **37** | **3** | **~7,140** |

详细清单：

**推理路径独占组件（10）— 全部已删除**：
- ~~CompactTimeline.vue, TurnBlock.vue, ChatMessageRow.vue, ToolStrip.vue, ChatReactSteps.vue, ToolCallTimeline.vue, ToolCallTimelineItem.vue, TimelineNode.vue, AgentBlock.vue, AgentTreeTimeline.vue~~

**推理路径独占 TS（15）— 14 已删除，1 未删除**：
- ~~groupMessagesByTurn.ts, useChatTimeline.ts, useTurnBlock.ts, useChatMessageRow.ts, reactPlannerParse.ts, reactPlannerTypes.ts, reactToolLinkIndex.ts, reactPlannerToolLink.ts, useToolCallTimeline.ts, useToolDisplayMode.ts, timelineTypes.ts, compactTimeline.ts, useAgentBlocks.ts, messagePlannerPresentation.ts~~
- **a2uiParse.ts** — 仍存在于 `web/src/features/chat/a2uiParse.ts`（73 行，3 处导入仍活跃）。**审查发现**：`shouldUseA2UIView`/`contentLooksLikeA2UIJsonl` 两个函数在生产代码中无消费者（死代码），可安全删除

**双渲染模型文件（4）— 2 已删除，2 部分清理**：
- ~~**TaskBoard.vue** — 已删除~~
- ~~**TaskBoardNode.vue** — 已删除~~
- **activityTimelineTypes.ts** — 仍存在（ActivityVariant 已删除，TaskBoardNodeData 字段已移除，但 ConversationTurn/AgentWorkProcess/DelegateActivity/TeamPanel 仍在使用）
- **agentTreeTypes.ts** — TaskBoardNodeKind/TaskBoardToolStatus/TaskBoardNodeData 已删除，其余保留

**功能开关（1）— 已删除**：
- ~~useActivityFirstFlag.ts~~

### 6.2 新增文件（原计划 5 个，已完成 5 个）

| 文件 | 说明 | 状态 |
|------|------|------|
| `streamEventTypes.ts` | StreamEvent 统一类型定义（含 ConfirmEvent、NoticeEvent） | **已创建**（144 行） |
| `EventStream.vue` | 统一事件流渲染组件（7 种 kind 分发） | **已创建**（~100 行） |
| `ErrorBlock.vue` | 错误渲染组件 | **已创建** |
| `PlanBlock.vue` | 执行计划渲染（含递归子事件 + 步骤状态圆点 + DAG 依赖） | **已创建**（~199 行） |
| `ConfirmBlock.vue` | 用户确认渲染（含批准/拒绝按钮 + 自动批准倒计时） | **已创建**（~220 行） |
| `NoticeBlock.vue` | 系统通知渲染（info/warning/success 三种类型） | **已创建**（~55 行） |

### 6.3 重构文件（原计划 8 个，全部已完成）

| 文件 | 重构内容 | 状态 |
|------|---------|------|
| `AgentWorkPanel.vue` | 互斥分支 → 统一 EventStream | **已完成** |
| `ChatMessageList.vue` | 三路分支 → 单路 | **已完成** |
| `ChatMessagePanel.vue` | 移除推理路径逻辑 | **已完成** |
| `ActionBlock.vue` | 两种变体 → 统一 card 变体 | **已完成**（移除 compact，仅保留 card） |
| `ReplyBlock.vue` | 两种变体 → 统一 card 变体 | **已完成**（移除 compact，仅保留 card） |
| `NoticeBlock.vue` | 重命名 + 拆分 ErrorBlock + CSS 类名统一为 notice-block | **已完成** |
| `useActivityTimeline.ts` | 双映射 → 单映射 `activityToStreamEvent`（含 notice/confirm/plan 映射 + plan steps 提取 + status 映射） | **已完成** |
| `ThinkingBlock.vue` | 三种变体 → 统一 card 变体 | **已完成**（移除 inline/compact，仅保留 card） |

---

## 七、净效果（2026-06-16 核对）

### 当前已达成

| 指标 | 变更前 | 当前 | 目标 | 进度 |
|------|--------|------|------|------|
| 聊天组件文件数 | 70+ | 60 | ~35 | 大部分完成（-14%） |
| useConversationTimeline 行数 | 5,858 | 562 | ~500 | 基本达成（-90%） |
| 数据路径 | 双路径（AF + 推理） | 单路径（AF）+ Legacy 降级 | 单路径（AF） | 基本达成 |
| 渲染模型 | 双模型（Timeline + TaskBoard） | 单模型（EventStream） | 单模型（EventStream） | **已达成** |
| 类型映射函数 | 2 个 | 1 个（activityToStreamEvent） | 1 个 | **已达成** |
| 前端推理逻辑 | 13 层 / 35 路径 | 0 层（推理路径已删除） | 0 层 | **已达成** |
| Activity Kind | 7 种（全部活跃） | 10 种（7 原有 + 3 新增 notice/confirm/plan，但 3 种新增无生产调用方） | 7 种（新增 3 + 删除 3） | 部分完成（新增 API 骨架完成，运行时集成未完成） |
| 组件变体 | 3 种（inline/card/compact） | 1 种（card） | 1 种 | **已达成** |
| CSS 类名一致性 | say-activity/notice-activity 混用 | 全部统一为 *-block | 全部统一 | **已达成** |

### 目标净效果（全部完成后）

| 指标 | 变更前 | 变更后 | 变化 |
|------|--------|--------|------|
| 聊天组件文件数 | 70+ | ~35 | -50% |
| 代码行数 | ~12,000+ | ~5,000+ | -58% |
| 数据路径 | 双路径（AF + 推理） | 单路径（AF） | -1 |
| 渲染模型 | 双模型（Timeline + TaskBoard） | 单模型（EventStream） | -1 |
| 类型映射函数 | 2 个 | 1 个 | -1 |
| 前端推理逻辑 | 13 层 | 0 层 | -13 |
| Activity Kind | 7 种 | 7 种（新增 3 + 删除 3） | 精简 |
| 组件变体 | 3 种（inline/card/compact） | 1 种 | -2 |

---

## 八、风险与缓解

| 风险 | 严重度 | 缓解措施 |
|------|--------|---------|
| 旧会话无 Activity 数据，删除推理路径后空白 | 高 | **不考虑兼容**（用户已确认），旧会话显示"此会话使用旧格式，无法展示详情"提示 |
| PlanBlock 的 DAG 依赖渲染复杂度 | 中 | 第一版仅用缩进 + 连接线表示线性依赖，不画完整 DAG 图 |
| ConfirmBlock 的确认 API 需要后端配合 | 中 | Phase 1 先实现后端 API，Phase 3 再实现前端组件 |
| 删除推理路径后无法回退 | 低 | Git 分支保护，重构在新分支进行 |
| **Confirm API 越权风险** | ~~高~~ 已修复 | ✅ 已添加 `session.UserID != userID` 校验 + Warn 日志 |
| **`activityToStreamEvent` 遗漏 `autoApproveAt` 字段** | ~~高~~ 已修复 | ✅ 已从 `node.meta.autoApproveAt` 提取 |
| **`TrySendAwaitChannel` 返回值被忽略** | ~~中~~ 已修复 | ✅ 已检查返回值，失败返回 `accepted: false` |
| **`TrySendAwaitChannel` 传入空 RunID** | ~~中~~ 已修复 | ✅ 通过 `ActiveRunner` 获取实际 RunID |
| **`GetActivity`/`UpdateActivity` 错误未翻译** | ~~中~~ 已确认无需修改 | data 层 `activity_repo.go` 已使用 `entErrToBizErr`，service 层收到的已是翻译后的 apierror |
| **新增 Kind 无生产调用方** | 高 | ~~`OnNotice`/`OnConfirmRequest`/`OnConfirmResult`/`OnPlanStart`/`OnPlanStepUpdate` 仅有测试调用~~ → `OnPlanStart`/`OnPlanStepUpdate` 已有生产调用方（N-02 已完成）。`OnNotice`/`OnConfirmRequest`/`OnConfirmResult` 仍无生产调用方，需完成 N-03/N-21 运行时集成 |
| **`ErrorEvent.type='info'` 与 `NoticeEvent.type='info'` 语义重叠** | ~~低~~ 已修复 | ✅ 移除 ErrorEvent.type 的 'info' 选项 |

---

## 九、下一步优化项（2026-06-16 更新）

> Phase 1-3 + P2/P3 部分已完成，以下为后续迭代待实施项，按优先级排序。

### 已完成项

| # | 优化项 | 完成日期 | 说明 |
|---|--------|---------|------|
| N-01 | Confirm API 端点 | 2026-06-16 | `POST /v1/chat/activities/{activity_id}/confirm` — Proto 定义 + `chat_confirm.go` Service 实现 + Wire 绑定。含审查修复：writer nil 返回错误、拒绝时也发 await channel |
| N-07 | a2uiParse.ts 评估 | 2026-06-16 | 评估后保留——3 处导入仍活跃（ChatA2UIPreview.vue、a2uiSurfaceState.ts、测试文件），属于 A2UI 功能模块而非推理路径遗留 |
| N-09 | CSS 类名统一 | 2026-06-16 | `say-activity` → `reply-block`，全部组件 CSS 类名统一为 `*-block` |
| N-11 | API 失败恢复 | 2026-06-16 | `loadActivitiesFromAPI` 添加 `loadError` 状态 + `retryLoad` 方法，`useChatWorkspace` 利用 `loadError` 判断降级 |
| N-12 | 后端单元测试 | 2026-06-16 | 14 个测试用例覆盖 OnNotice/OnConfirmRequest/OnConfirmResult/OnPlanStart/OnPlanStepUpdate |
| N-13 | PlanBlock 子事件渲染 | 2026-06-16 | PlanBlock 新增 `childActivities` prop + EventStream `activityTree` 传递链，与后端 `parentActivityId` 数据流对齐 |
| N-14 | ConfirmBlock 前端 API 调用 | 2026-06-16 | 完整事件链 ConfirmBlock → EventStream → AgentWorkPanel → ConversationTurn → ChatMessageList → ChatMessagePanel → ChatPage → `confirmActivity` API。`api.ts` 新增 `confirmActivity` 函数 |
| N-15 | ConfirmActivity 鉴权 | 2026-06-16 | `chat_confirm.go` 添加 `ctxuser.FromContext` 校验，拒绝 `default_user` 匿名确认请求。**A-01 已修复**：添加 `session.UserID != userID` 归属校验 + Warn 日志 |
| N-02 | Plan Activity 运行时集成 | 2026-06-16 | `activity_projector.go` 新增 `processGraphNodeStart`/`processGraphNodeComplete`，从 `graph.node.*` 事件自动创建 plan Activity 和 steps。使用本地常量镜像避免传递依赖编译错误 |
| N-16 | EventStream.getChildActivities 性能 | 2026-06-16 | 递归树搜索 → `computed` + `Map<parentId, children[]>` O(1) 查找 |
| N-17 | ConfirmBlock 加载状态 | 2026-06-16 | ConfirmBlock 添加 `confirming` ref + disabled 按钮 + 超时重置（5s），防止重复点击 |
| N-18 | ConfirmBlock 错误提示 | 2026-06-16 | ChatPage 添加 `$q.notify` 错误/警告提示，API 调用失败或返回 `accepted: false` 时通知用户 |
| A-01 | Confirm API 越权修复 | 2026-06-16 | `chat_confirm.go` 添加 `session.UserID != userID` 归属校验 + `apierror.Forbidden` + Warn 日志（含 owner_id/user_id）。sessions==nil 时记录 Warn 日志 |
| A-02 | autoApproveAt 映射修复 | 2026-06-16 | `useActivityTimeline.ts` confirm 分支添加 `autoApproveAt: (node.meta?.autoApproveAt as string) ?? null`，前端倒计时功能可工作 |
| B-01 | TrySendAwaitChannel 返回值处理 | 2026-06-16 | `chat_confirm.go` 检查 `TrySendAwaitChannel` 返回值，发送失败返回 `accepted: false`，前端可感知运行时未收到信号 |
| B-02 | TrySendAwaitChannel 空 RunID 修复 | 2026-06-16 | `chat_confirm.go` 通过 `s.orch.ActiveRunner(sessionID)` 获取实际 RunID，避免并发场景信号错发 |
| B-03 | GetActivity/UpdateActivity 错误翻译 | 2026-06-16 | **确认无需修改**：`activity_repo.go` 的 `GetActivity`/`UpdateActivity` 已在 data 层使用 `entErrToBizErr(err, "ACTIVITY")` 翻译，service 层收到的已是翻译后的 apierror |
| B-05 | ErrorEvent.type='info' 语义重叠修复 | 2026-06-16 | `streamEventTypes.ts` 移除 `ErrorEvent.type` 的 `'info'` 选项（仅保留 `'degradation'`），`useActivityTimeline.ts` fallback 改用 `type: 'degradation'`。`NoticeEvent.type` 保留 `'info'` |
| B-06 | shouldUseA2UIView/contentLooksLikeA2UIJsonl 死代码删除 | 2026-06-16 | `a2uiParse.ts` 删除两个无消费者的导出函数（`shouldUseA2UIView`、`contentLooksLikeA2UIJsonl`），保留 `parseA2UIJsonl` 和 `A2UIParseLine` 类型 |
| N-07b | streamEventTypes.ts 注释更新 | 2026-06-16 | 头部注释从"5 种 Activity Kind"更新为"7 种"，移除过时的映射关系说明 |

### 已评估暂不实施

| # | 优化项 | 原因 |
|---|--------|------|
| N-03 | Notice Activity 运行时集成 | 需 ADR：插件层（cost_guard、model_router）无法直接访问 ActivityProjector，属于跨模块架构变更，需设计事件桥接机制 |
| N-04 | 删除 `task` Kind | 前端 8+ 处活跃依赖（ChatPage.vue:83、streamHandlers.ts:450/627、useConversationTimeline.ts:257、activityMessageAdapter.ts:182、api.ts:581），删除不安全 |
| N-05 | 删除 `sub_task_board` Kind | 前端 20+ 处依赖（activityTimelineTypes.ts、DelegateActivity.vue、TeamPanel.vue 等），删除不安全。需先完成 DelegateActivity → PlanBlock 迁移 |
| N-06 | 删除 `delegate` Kind | 同 N-05，前端依赖广泛，需先完成 DelegateActivity 迁移 |
| N-19 | 旧 awaitUserReply 工具确认迁移 | 现有 `useAwaitReply.submitToolConfirm` 使用特殊字符串走 `awaitUserReply` API，与新 `confirmActivity` API 是两套路径并存。需评估统一方案，涉及运行时协议变更 |

### ~~P0：安全性修复（阻塞生产使用）~~ — 已全部完成

| # | 优化项 | 状态 | 说明 |
|---|--------|------|------|
| A-01 | **Confirm API 越权风险** | ✅ 已修复 | 添加 `session.UserID != userID` 校验 + Warn 日志 |
| A-02 | **`autoApproveAt` 字段映射遗漏** | ✅ 已修复 | `activityToStreamEvent` confirm 分支提取 `node.meta.autoApproveAt` |

### ~~P1：代码健壮性（建议修复）~~ — 已全部完成

| # | 优化项 | 状态 | 说明 |
|---|--------|------|------|
| B-01 | **TrySendAwaitChannel 返回值被忽略** | ✅ 已修复 | 检查返回值，失败返回 `accepted: false` |
| B-02 | **TrySendAwaitChannel 传入空 RunID** | ✅ 已修复 | 通过 `ActiveRunner` 获取实际 RunID |
| B-03 | **GetActivity/UpdateActivity 错误未翻译** | ✅ 确认无需修改 | data 层 `activity_repo.go` 已使用 `entErrToBizErr` |

### P2：代码清理（减少技术债务）

| # | 优化项 | 说明 | 涉及文件 | 状态 |
|---|--------|------|---------|------|
| N-08 | **activityTimelineTypes.ts 瘦身** | ConversationTurn/AgentWorkProcess/DelegateActivity/TeamPanel 仍在使用，但 ActivityVariant/TaskBoardNodeData 已删除。长期目标：DelegateActivity 迁移到 PlanBlock 后可删除整个文件 | `web/src/features/chat/activityTimelineTypes.ts` | 待实施 |
| N-10 | **DelegateActivity.vue 迁移** | 当前 EventStream 仍渲染 DelegateActivity，需在 N-05/N-06 完成后删除 | `DelegateActivity.vue` | 待实施 |
| B-04 | **PlanStep.children 映射未处理** | `PlanStep.children`（递归 StreamEvent）在映射中未填充，PlanBlock 的递归子事件渲染依赖 `activityTree` 而非 `steps.children`，存在数据流不一致 | `useActivityTimeline.ts` | 待实施 |
| B-05 | ~~ErrorEvent.type='info' 与 NoticeEvent.type='info' 语义重叠~~ | 已修复：移除 ErrorEvent.type 的 'info' 选项 | `streamEventTypes.ts` | ✅ 已修复 |
| B-06 | ~~shouldUseA2UIView/contentLooksLikeA2UIJsonl 死代码~~ | 已删除 | `a2uiParse.ts` | ✅ 已修复 |

### P3：功能增强

| # | 优化项 | 说明 | 涉及文件 |
|---|--------|------|---------|
| N-20 | **Plan step 可读标签** | 当前 plan step label 使用 `graphNodeMetadata.NodeID`（如 `node_1`），用户不可读。需从 graph node 事件中提取人类可读标签（如节点名称/描述） | `activity_projector.go` |
| N-21 | **Confirm API Confirm Activity 运行时集成** | `OnConfirmRequest`/`OnConfirmResult` 已定义但无生产调用方。需在 tool_confirm 场景中调用，将工具确认请求映射为 confirm Activity | `internal/agent/`, `internal/session/trpc/` |

### 依赖关系图

```
~~A-01 (Confirm API 越权修复)~~ ✅ 已完成
~~A-02 (autoApproveAt 映射修复)~~ ✅ 已完成
~~B-01~B-02 (Confirm API 健壮性)~~ ✅ 已完成
~~B-03 (错误翻译)~~ ✅ 确认无需修改
~~B-05 (ErrorEvent.type 重叠)~~ ✅ 已完成
~~B-06 (死代码删除)~~ ✅ 已完成

N-21 (Confirm Activity 运行时集成) ──→ ConfirmBlock 生产触发
N-20 (Plan step 可读标签) ──→ PlanBlock 用户体验
B-04 (PlanStep.children 映射) ──→ 数据流一致性
N-08 (activityTimelineTypes 瘦身) ──→ N-10 (DelegateActivity 迁移) ──→ N-05/N-06 (删除 sub_task_board/delegate)
```

---

## 十、代码审查发现（2026-06-16）

> 本节为 2026-06-16 对文档方案与实际代码的交叉审查结果。

### 10.1 文档与代码不一致

| # | 不一致 | 文档描述 | 代码实际 | 修正 |
|---|--------|---------|---------|------|
| 1 | `delegate` 调用点 | "OnDelegate 在 ProcessEvent 中被调用，已有调用点" | `OnDelegate` **无生产调用方**，`stream_consumer.go` 未调用 | 已修正 §2.2 勘误 |
| 2 | Phase 1 完成状态 | "已完成" | 后端方法已实现但**无生产调用方**，严格来说 Phase 1 仅完成 API 骨架 | 已修正 §5.1 |
| 3 | Confirm API 实现状态 | "未实现" | 已实现（Proto + `chat_confirm.go` + Wire 绑定） | 已修正 §3.3 |
| 4 | `sub_task_board`/`delegate` 活跃状态 | "当前仍在后端活跃使用" | `OnDelegate` 无生产调用方，不会产生数据 | 已修正 §2.4 |
| 5 | `streamEventTypes.ts` 头部注释 | 文档未提及 | 文件仍写"5 种 Activity Kind"，实际已 7 种 | ✅ 已更新为"7 种" |
| 6 | `autoApproveAt` 映射 | §4.1 定义了字段 | `activityToStreamEvent` confirm 分支**未从 `node.meta` 提取** | ✅ 已修复映射函数 |

### 10.2 阻断级问题（必须修复）— 已全部修复

| # | 问题 | 位置 | 状态 |
|---|------|------|------|
| A-01 | **Confirm API 越权风险** | `internal/service/chat_confirm.go` | ✅ 已修复：添加 `session.UserID != userID` 校验 + Warn 日志 |
| A-02 | **`autoApproveAt` 字段映射遗漏** | `useActivityTimeline.ts` | ✅ 已修复：confirm 分支提取 `node.meta.autoApproveAt` |
| A-03 | **部分 Kind 仍无生产调用方** | `internal/agent/activity_projector.go` | 部分修复：`OnPlanStart`/`OnPlanStepUpdate` 已有生产调用方。`OnNotice`/`OnConfirmRequest`/`OnConfirmResult` 仍无生产调用方，需完成 N-03/N-21 |

### 10.3 建议级问题（推荐修复）— 已全部修复或确认

| # | 问题 | 位置 | 状态 |
|---|------|------|------|
| B-01 | `TrySendAwaitChannel` 返回值被忽略 | `chat_confirm.go` | ✅ 已修复：检查返回值，失败返回 `accepted: false` |
| B-02 | `TrySendAwaitChannel` 传入空 RunID | `chat_confirm.go` | ✅ 已修复：通过 `ActiveRunner` 获取实际 RunID |
| B-03 | `GetActivity`/`UpdateActivity` 错误未翻译 | `chat_confirm.go` | ✅ 确认无需修改：data 层已使用 `entErrToBizErr` |
| B-04 | `PlanStep.children` 映射未处理 | `useActivityTimeline.ts` | 待实施：当前 PlanBlock 通过 `activityTree` 渲染子事件，功能正常但数据流不一致 |
| B-05 | `ErrorEvent.type='info'` 与 `NoticeEvent.type='info'` 语义重叠 | `streamEventTypes.ts` | ✅ 已修复：移除 ErrorEvent.type 的 'info' 选项 |
| B-06 | `shouldUseA2UIView`/`contentLooksLikeA2UIJsonl` 死代码 | `a2uiParse.ts` | ✅ 已删除 |

### 10.4 过度设计评估

| 组件/设计 | 行数 | 评估 | 理由 |
|----------|------|------|------|
| PlanBlock.vue | 206 | **不过度设计** | 递归步骤渲染 + 状态图标 + DAG 依赖 + 子事件嵌套，复杂度合理 |
| ConfirmBlock.vue | 220 | **不过度设计** | 三种状态视图 + 倒计时 + 交互按钮，复杂度合理 |
| NoticeBlock.vue | 55 | **边缘** | 仅图标+文本，可在 EventStream 中内联 3-5 行。但保持 kind→组件一致性模式 |
| ErrorBlock.vue | 48 | **边缘** | 与 NoticeBlock 结构几乎一致，可合并为 `StatusMessageBlock`。但保持一致性 |
| 9 种 Status 常量 | — | **轻微** | 5 种 Status（pending/tool_blocked/partial_failure/cancelled/interrupted）从未在 ActivityProjector 中使用，属于提前定义 |
| PlanStep.dependsOn + dagNodeId | — | **轻微** | 文档标注"第一版仅用缩进+连接线"，但类型定义已包含完整 DAG 字段，YAGNI 轻微违反 |

**总体判断**：方案**不存在严重过度设计**。核心架构决策（单数据路径 + 单渲染模型 + 后端驱动 Activity）是正确的简化方向。

### 10.5 修复优先级（2026-06-16 更新）

1. ~~🔴 **A-01**：Confirm API 越权风险 → 添加 session 归属校验~~ ✅ 已修复
2. ~~🔴 **A-02**：`autoApproveAt` 映射遗漏 → 修复 `activityToStreamEvent` confirm 分支~~ ✅ 已修复
3. ~~🟡 **B-01~B-02**：Confirm API 健壮性 → TrySendAwaitChannel 返回值处理 + RunID~~ ✅ 已修复
4. ~~🟡 **B-03**：错误翻译 → 确认 data 层已翻译，无需修改~~ ✅ 已确认
5. ~~🟡 **B-05~B-06**：前端数据一致性 → type 重叠清理 + 死代码删除~~ ✅ 已修复
6. 🟡 **A-03**（降级）：Notice/Confirm 运行时集成 → 完成 N-03/N-21（Plan 已完成）
7. 🟡 **B-04**：PlanStep.children 映射 → 当前通过 activityTree 渲染，功能正常但数据流不一致
8. 🟡 **N-08/N-10**：activityTimelineTypes 瘦身 + DelegateActivity 迁移 → N-05/N-06 前置
9. 🟡 **N-20**：Plan step 可读标签 → 用户体验优化
10. 🟡 **N-21**：Confirm Activity 运行时集成 → ConfirmBlock 生产触发
