# 聊天 UI 统一事件流重构方案

> 日期：2026-06-15（2026-06-16 核对更新 + 实施更新）
> 状态：Phase 1-3 已完成（含审查修复），Phase 4 待实施
> 范围：前端聊天 UI + 后端 Activity 类型体系

---

## 一、问题诊断

### 1.1 现状（2026-06-16 核对）

原始问题为三重复杂度叠加，经部分重构后当前状态：

| 层级 | 原始问题 | 当前状态 | 剩余工作 |
|------|---------|---------|---------|
| 双数据路径 | AF 路径 + 消息推理路径（35 个推理路径 / 5,858 行） | **推理路径已删除**，`useConversationTimeline.ts` 从 5,858 行简化到 562 行，仅保留 AF 路径 + Legacy 降级路径 | a2uiParse.ts 未删除（3 处导入仍活跃） |
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

### 2.2 需要补全的 Kind（2026-06-16 核对）

当前后端缺少以下语义表达，需要新增：

| Kind | 语义 | 当前状态 | 方案 |
|------|------|---------|------|
| `notice` | 系统通知（进度提示、状态变更等） | **常量未定义**，无创建代码 | **新增**：定义 `ActivityKindNotice` 常量，`ActivityProjector` 新增 `OnNotice` 方法 |
| `confirm` | 用户确认请求 | **常量未定义**，无创建代码 | **新增**：定义 `ActivityKindConfirm` 常量，工具需要用户确认时创建，状态 tool_blocked → completed / cancelled |
| `plan` | 执行计划 | **常量未定义**，无创建代码 | **新增**：定义 `ActivityKindPlan` 常量，Agent 生成执行计划时创建（如 Graph DAG 节点列表） |

> **勘误**：原文档称 `notice`/`end` "常量已定义，无创建代码"，实际核对发现常量从未定义。原文档称 `delegate` "有 `OnDelegate` 方法但无调用点"，实际 `OnDelegate` 在 `ProcessEvent` 中被调用（delegate 事件触发时），已有调用点。`end` Kind 从未作为 Activity Kind 存在，无需"激活"。

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

> **注意**：`task`/`sub_task_board`/`delegate` 当前仍在后端活跃使用，删除需在 Phase 1 实施后同步进行。`end` 从未作为 Activity Kind 存在，无需删除。

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
| `/v1/sessions/{id}/activities/{aid}/confirm` | POST | 用户确认/拒绝工具调用 | **未实现**（Proto/Service 中均无此 RPC） |

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

### 5.1 Phase 1：后端 Activity 类型补全 — **已完成**

**目标**：补全 `notice`、`confirm`、`plan` 三种 Activity Kind。

| 任务 | 说明 | 状态 |
|------|------|------|
| 新增 `notice` | 定义 `ActivityKindNotice` 常量，`ActivityProjector` 新增 `OnNotice(content, noticeType)` 方法，在状态变更、模型切换等场景调用 | **已完成** |
| 新增 `confirm` | 定义 `ActivityKindConfirm` 常量，`ActivityProjector` 新增 `OnConfirmRequest(toolName, toolArguments, content)` 方法；新增 `OnConfirmResult(approved)` 方法（含 Kind 校验） | **已完成** |
| 新增 `plan` | 定义 `ActivityKindPlan` 常量，`ActivityProjector` 新增 `OnPlanStart(title, steps)` 和 `OnPlanStepUpdate(stepId, status)` 方法（含 Kind 校验 + 自动推导计划整体状态） | **已完成** |
| 新增 `ActivityPlanStep` 类型 | biz 层新增 PlanStep 结构体（ID/Label/Status/AgentName/DependsOn） | **已完成** |
| 新增 `Meta` 字段 | `biz.Activity` 新增 `Meta map[string]any` 字段，Ent Schema 同步添加 `field.JSON("meta", ...)` | **已完成** |
| 新增 API | `POST /v1/sessions/{id}/activities/{aid}/confirm` — 用户确认/拒绝 | **未实施**（需 Proto 定义 + Service 实现） |
| 删除 `task`/`sub_task_board`/`delegate` Kind | 这三种 Kind 的语义由 ConversationTurn / plan.steps / 子 Activity 承担 | **未实施**（当前仍在活跃使用，需等 Plan 替换完成） |

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
- **a2uiParse.ts** — 仍存在于 `web/src/features/chat/a2uiParse.ts`（73 行，3 处导入仍活跃）

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
| Activity Kind | 7 种（全部活跃） | 10 种（7 原有 + 3 新增 notice/confirm/plan） | 7 种（新增 3 + 删除 3） | 部分完成（新增完成，删除待实施） |
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

---

## 九、下一步优化项（2026-06-16）

> Phase 1-3 已完成，以下为后续迭代待实施项，按优先级排序。

### P0：功能完整性（阻塞生产使用）

| # | 优化项 | 说明 | 涉及文件 |
|---|--------|------|---------|
| N-01 | **Confirm API 端点** | `POST /v1/sessions/{id}/activities/{aid}/confirm` — 需 Proto 定义 + Service 实现 + Wire 绑定。当前 ConfirmBlock 的 emit 事件无后端接收方 | `api/kratos/chat/v1/*.proto`, `internal/service/chat.go`, `internal/biz/` |
| N-02 | **Plan Activity 运行时集成** | `OnPlanStart`/`OnPlanStepUpdate` 已定义但无调用方。需在 Graph/Team 运行时中调用，将 DAG 节点状态映射为 PlanStep | `internal/agent/`, `internal/session/trpc/` |
| N-03 | **Notice Activity 运行时集成** | `OnNotice` 已定义但无调用方。需在模型切换、配额警告、降级等场景调用 | `internal/agent/`, `internal/service/` |

### P1：Kind 清理（减少前端复杂度）

| # | 优化项 | 说明 | 涉及文件 |
|---|--------|------|---------|
| N-04 | **删除 `task` Kind** | 根节点分组职责由 ConversationTurn 承担，需先确保前端不依赖 task Activity | `activity_projector.go`, `useActivityTimeline.ts` |
| N-05 | **删除 `sub_task_board` Kind** | 语义由 plan.steps 递归表达，需先完成 N-02 | `activity_projector.go`, `activity.go` |
| N-06 | **删除 `delegate` Kind** | 语义由 plan + 子 thinking/action/reply 表达，需先完成 N-02 + 前端 DelegateActivity 迁移 | `activity_projector.go`, `DelegateActivity.vue`, `activityTimelineTypes.ts` |

### P2：代码清理（减少技术债务）

| # | 优化项 | 说明 | 涉及文件 |
|---|--------|------|---------|
| N-07 | **a2uiParse.ts 清理** | 73 行，3 处导入仍活跃（ChatA2UIPreview.vue、a2uiSurfaceState.ts、测试文件）。需评估是否迁移到 ReplyBlock 的 a2ui variant | `web/src/features/chat/a2uiParse.ts` |
| N-08 | **activityTimelineTypes.ts 瘦身** | ConversationTurn/AgentWorkProcess/DelegateActivity/TeamPanel 仍在使用，但 ActivityVariant/TaskBoardNodeData 已删除。长期目标：DelegateActivity 迁移到 PlanBlock 后可删除整个文件 | `web/src/features/chat/activityTimelineTypes.ts` |
| N-09 | **CSS 类名统一** | ReplyBlock 仍使用 `say-activity` CSS 类名，应统一为 `reply-block` | `ReplyBlock.vue` |
| N-10 | **DelegateActivity.vue 迁移** | 当前 EventStream 仍渲染 DelegateActivity，需在 N-06 完成后删除 | `DelegateActivity.vue` |

### P3：健壮性增强

| # | 优化项 | 说明 | 涉及文件 |
|---|--------|------|---------|
| N-11 | **Phase 4：API 失败恢复** | `loadActivitiesFromAPI` 重试机制（指数退避 2 次）+ 降级提示 | `useActivityTimeline.ts` |
| N-12 | **后端单元测试** | OnNotice/OnConfirmRequest/OnConfirmResult/OnPlanStart/OnPlanStepUpdate 缺少单元测试 | `internal/agent/activity_projector_test.go` |
| N-13 | **PlanBlock 步骤内子事件渲染** | 当前 PlanStep.children 通过递归 EventStream 渲染，需验证与后端 child_activity 的数据流对齐 | `PlanBlock.vue`, `useActivityTimeline.ts` |

### 依赖关系图

```
N-02 (Plan 集成) ──→ N-05 (删除 sub_task_board) ──→ N-10 (删除 DelegateActivity.vue)
                  ──→ N-06 (删除 delegate)     ──→ N-08 (删除 activityTimelineTypes.ts)
N-03 (Notice 集成) ──→ N-04 (删除 task)
N-01 (Confirm API) ──→ ConfirmBlock 功能闭环
N-11 (API 重试) ──→ 生产可用性
N-12 (单元测试) ──→ 代码质量
```
