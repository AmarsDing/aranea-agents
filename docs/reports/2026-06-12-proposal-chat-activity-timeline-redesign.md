# 聊天会话过程展示重设计方案

> **文档地位**：聊天 UI 重设计的完整方案，融合"活动时间线"模型与 v7 设计原型的 Team 面板风格。
> **日期**：2026-06-13（v3 — 第二轮审查修复版）
> **原型参考**：`.superpowers/brainstorm/chat-ui-design/content/m59-chat-ui-v7.html`

---

## 一、问题陈述

### 1.1 当前痛点

| # | 痛点 | 根因 |
|---|------|------|
| P1 | 多轮回复合并到一个 UI 容器 | `TurnBlockGroup.assistant` 单槽位，多条 assistant 消息后者覆盖前者 |
| P2 | 思考与回复混在一起 | 非 ReAct 模式下 `CompactTimeline` 只生成 1 个 reply 节点 |
| P3 | 三套渲染路径不统一 | AgentTreeTimeline / TurnBlock / ChatMessageRow 条件分支，数据模型各异 |
| P4 | Team 协作过程不直观 | 子 Agent 嵌套在时间线中，缺乏"统一面板"视角 |
| P5 | 流式体验碎片化 | 每条消息独立流式，无法呈现"AI 正在工作"的连贯感 |

### 1.2 用户心智模型

用户跟 AI Agent 对话时，心里想的是：

```
我问了一个问题 → AI 在工作 → AI 给我答案
```

"AI 在工作"这个过程的展开，就是用户想看到的东西。用户不关心"消息"、"轮次"、"ReAct 标签"这些后端概念，用户关心的是：

1. **AI 现在在干嘛？** — 实时进度感知
2. **AI 为什么这么做？** — 推理过程可追溯
3. **AI 做了什么？** — 工具调用可见
4. **AI 告诉我什么？** — 回复内容清晰
5. **哪些是中间过程，哪些是最终结论？** — 层次分明

---

## 二、设计原则

| # | 原则 | 说明 |
|---|------|------|
| D1 | **每个回复有独立 UI 容器** | 绝不合并多条回复到一个容器 |
| D2 | **活动时间线模型** | 放弃"消息列表"，采用"活动时间线"：think → act → say 按发生顺序排列 |
| D3 | **统一渲染路径** | 一套数据模型、一套组件树，消除三套路径的条件分支 |
| D4 | **Team 面板化** | Team 协作采用 v7 原型的统一面板设计，而非嵌套时间线 |
| D5 | **渐进式披露** | 思考自动折叠、工具可折叠、回复始终可见 |
| D6 | **流式即活动** | 流式不是"消息在更新"，而是"活动在进行" |

---

## 三、核心概念：活动时间线（Activity Timeline）

### 3.1 数据模型

一个 Turn（用户提问到 AI 完成回答）内的所有内容，按时间顺序排列为一条活动时间线。时间线上的每个节点是一个 **Activity**：

```typescript
/** 一个 Turn = 用户提问 + Agent 工作过程 */
interface ConversationTurn {
  /** 使用后端 turn_id（权威 FK），不用前端推算 */
  id: string;
  userMessage: Message;
  agentWork: AgentWorkProcess;
}

/** Agent 工作过程 = 活动时间线 */
interface AgentWorkProcess {
  agentKey: string;
  agentName: string;
  agentIcon: string;
  agentColor: string;
  /** 扩展自 AgentBlockStatus，合并 tool_running/tool_blocked 到 running，partial_failure 映射为 completed */
  status: 'running' | 'completed' | 'failed';
  durationMs: number | null;

  /** 活动时间线 — 严格按发生顺序排列 */
  activities: Activity[];

  /** Team 统一面板（Team 模式时存在，见 §5.2） */
  panel?: TeamPanel;

  /** 以下字段从现有 AgentBlock 迁移，保留语义 */
  task: string | null;
  result: string | null;
  hasPartialFailure: boolean;
  plan: OrchestrationPlan | null;
  teamStatus: TeamStatusSummary | null;
  progressSections: ProgressSection[];
  startedAt: string;
  finishedAt: string | null;
}

/** 活动节点 — 时间线上的最小展示单元 */
type Activity =
  | ThinkActivity
  | ActActivity
  | SayActivity
  | DelegateActivity
  | NoticeActivity;

interface ThinkActivity {
  kind: 'think';
  id: string;
  content: string;
  /** 区分"规划"/"推理"/"重规划"/"进度" */
  label?: string;
  collapsed: boolean;
  streaming: boolean;
  durationMs: number | null;
}

interface ActActivity {
  kind: 'act';
  id: string;
  tool: ToolActivity;
}

interface SayActivity {
  kind: 'say';
  id: string;
  content: string;
  /** isFinal 判定规则见 §3.4 */
  isFinal: boolean;
  streaming: boolean;
  /** 渲染变体：默认 markdown / a2ui 结构化 UI */
  variant: 'default' | 'a2ui';
  /** A2UI 模式时的结构化数据 */
  a2uiLines?: A2UIJsonlLine[];
  durationMs: number | null;
}

interface DelegateActivity {
  kind: 'delegate';
  id: string;
  subAgent: AgentWorkProcess;
}

/** 通知/提示节点 — 对应现有 TimelineEntry.kind === 'notice' */
interface NoticeActivity {
  kind: 'notice';
  id: string;
  type: 'degradation' | 'info';
  message: string;
}

/** 工具活动 */
interface ToolActivity {
  toolName: string;
  toolLabel: string;
  status: 'running' | 'success' | 'failed' | 'blocked' | 'cancelled';
  durationMs: number | null;
  arguments: string | null;
  result: string | null;
  error: string | null;
  iconKey?: string;
  isLongRunning?: boolean;
}
```

### 3.2 Activity 语义

| Activity | 图标 | 语义 | 展示策略 |
|----------|------|------|---------|
| **think** | 🧠 | AI 在思考/规划/推理 | 可折叠，完成后自动折叠；label 区分"规划"/"推理"/"重规划"/"进度" |
| **act** | 🔧 | AI 在使用工具 | 显示工具名+状态，可展开参数/结果 |
| **say** | 💬 | AI 在回复用户 | 始终可见，不可折叠；`isFinal` 区分中间回复与最终回复；`variant` 区分 markdown 与 A2UI |
| **delegate** | 👥 | AI 委派子 Agent | 根据 `subAgent.panel` 是否存在决定渲染方式：无 panel → 嵌套时间线；有 panel → Team 面板（见 §五） |
| **notice** | ℹ️ | 系统通知/降级提示 | 轻量提示行，不可折叠；`type` 区分"降级"与"信息" |

### 3.3 Activity 构建规则

从后端消息到 Activity 的映射：

| 后端事件 | 生成的 Activity | 说明 |
|---------|----------------|------|
| assistant + `reasoning_markdown` 非空 | `think` | 思考内容 |
| assistant + ReAct `/*PLANNING*/` | `think { label: "规划" }` | ReAct 规划步骤 |
| assistant + ReAct `/*REASONING*/` | `think { label: "推理" }` | ReAct 推理步骤 |
| assistant + ReAct `/*REPLANNING*/` | `think { label: "重规划" }` | ReAct 重规划步骤 |
| assistant + ReAct `/*ACTION*/` | 不生成 | 工具由 tool 消息处理 |
| assistant + ReAct `/*FINAL_ANSWER*/` | `say { isFinal: true, variant: 'default' }` | 最终回复 |
| assistant + `content_markdown` 非空（非 ReAct） | `say { isFinal: ?, variant: 'default' }` | 普通回复，isFinal 判定见 §3.4 |
| assistant + A2UI `a2uiLines` 非空 | `say { isFinal: ?, variant: 'a2ui', a2uiLines }` | A2UI 结构化回复 |
| tool 消息 | `act` | 工具调用 |
| team member 消息 | `delegate` → 子 `AgentWorkProcess` | 子 Agent 工作 |
| `execution_progress` 信封 | `think { label: "进度" }` | LLM 调用等待等进度信息 |
| notice 消息（降级/信息提示） | `notice` | 系统通知，对应现有 TimelineEntry.kind === 'notice' |
| Ralph Loop 迭代 | 一组 `think → act → say` | 迭代边界判定见 §3.5 |

**去重规则**（唯一需要的去重）：
- 如果 `say` 的内容与紧邻的前一个 `think` 内容完全相同，跳过该 `say`
- 这只发生在非 ReAct 模式下 LLM 只输出了 reasoning 没有 content 的情况

### 3.4 `isFinal` 判定规则

前端无法直接从单条 `text_done` 信封判断是否为最终回复。判定规则如下：

**规则 1：Turn 内最后一条 assistant 消息的 `say` 为 `isFinal: true`，其余为 `false`。**

具体实现：
1. `useConversationTimeline` 在构建 Activity 时，先收集当前 Turn 内所有 assistant 消息
2. 最后一条 assistant 消息产生的 `say` Activity 标记 `isFinal: true`
3. 其余 `say` Activity 标记 `isFinal: false`
4. **流式期间**：当前正在流式的 `say` 临时标记为 `isFinal: true`（因为尚不知道是否还有后续）；当新的 assistant 消息到达时，前一个 `say` 降级为 `isFinal: false`

**规则 2：ReAct 模式的 `/*FINAL_ANSWER*/` 始终为 `isFinal: true`。**

ReAct 模式下，`/*FINAL_ANSWER*/` 标签是后端循环终止信号，语义上就是最终回复。

**视觉区分**：
- `isFinal: true` → 标签显示"回复"
- `isFinal: false` → 标签显示"中间回复"

### 3.5 Ralph Loop 迭代边界判定

前端区分 Ralph Loop 迭代与普通多轮的规则：

**规则：同一 Turn 内，`runner_completion` 事件后紧接新的 `text_delta`（无中间 user 消息），视为 Ralph Loop 的新一次迭代。**

具体实现：
1. `useConversationTimeline` 维护一个 `lastEventType` 状态
2. 当 `lastEventType === 'runner_completion'` 且新到 `text_delta` 无中间 user 消息 → 标记为 Ralph 迭代
3. Ralph 迭代产生的 Activity 前插入一个 `think { label: "第 N 次尝试" }` 分隔节点
4. 迭代完成后（`runner_completion` 后无新 `text_delta`）→ 插入"验证通过"或"验证失败"标记

**长期方案**：后端在 `state_delta` 信封中增加 `ralph_iteration` 字段，前端直接读取。

---

## 四、单 Agent 模式展示设计

### 4.1 纯对话

```
┌──────────────────────────────────────────┐
│ 👤 你好，帮我写一首诗                      │
├──────────────────────────────────────────┤
│                                          │
│  💬 回复                                  │
│  ┌────────────────────────────────────┐  │
│  │ 春风拂面花自开，细雨润物燕归来...    │  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

### 4.2 带思考

```
┌──────────────────────────────────────────┐
│ 👤 这段代码有什么问题？                    │
├──────────────────────────────────────────┤
│                                          │
│  🧠 思考                           ▼ 折叠 │
│  ┌────────────────────────────────────┐  │
│  │ 让我分析这段代码的结构...             │  │
│  │ 变量命名不规范，循环逻辑有边界问题...  │  │
│  └────────────────────────────────────┘  │
│                                          │
│  💬 回复                                  │
│  ┌────────────────────────────────────┐  │
│  │ 这段代码有两个主要问题：              │  │
│  │ 1. 变量命名不符合规范...             │  │
│  │ 2. 循环边界条件有 off-by-one 错误... │  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

### 4.3 带工具

```
┌──────────────────────────────────────────┐
│ 👤 北京今天天气怎么样？                    │
├──────────────────────────────────────────┤
│                                          │
│  🧠 思考                           ▼ 折叠 │
│  ┌────────────────────────────────────┐  │
│  │ 用户想知道天气，我需要搜索...         │  │
│  └────────────────────────────────────┘  │
│                                          │
│  🔧 search("北京天气")           ✓ 1.8s  │
│  ┌────────────────────────────────────┐  │
│  │ 北京：晴天，25°C，湿度45%...         │  │
│  └────────────────────────────────────┘  │
│                                          │
│  💬 回复                                  │
│  ┌────────────────────────────────────┐  │
│  │ 北京今天是晴天，气温25°C，           │  │
│  │ 湿度45%，适合户外活动。              │  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

### 4.4 ReAct 循环

```
┌──────────────────────────────────────────┐
│ 👤 帮我对比北京和上海的天气                │
├──────────────────────────────────────────┤
│                                          │
│  🧠 规划                           ▼ 折叠 │
│  ┌────────────────────────────────────┐  │
│  │ 需要分别搜索两个城市的天气，          │  │
│  │ 然后进行对比分析。                   │  │
│  └────────────────────────────────────┘  │
│                                          │
│  🔧 search("北京天气")           ✓ 1.8s  │
│                                          │
│  🧠 推理                           ▼ 折叠 │
│  ┌────────────────────────────────────┐  │
│  │ 北京数据已获取，继续搜索上海...       │  │
│  └────────────────────────────────────┘  │
│                                          │
│  🔧 search("上海天气")           ✓ 1.6s  │
│                                          │
│  🧠 推理                           ▼ 折叠 │
│  ┌────────────────────────────────────┐  │
│  │ 两个城市数据都拿到了，可以对比了。    │  │
│  └────────────────────────────────────┘  │
│                                          │
│  💬 回复                                  │
│  ┌────────────────────────────────────┐  │
│  │ 北京 vs 上海天气对比：               │  │
│  │ - 北京：晴天 25°C                   │  │
│  │ - 上海：多云 22°C                   │  │
│  │ 北京比上海高3°C，天气更晴朗。        │  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

### 4.5 多轮交互（关键场景）

一个 Turn 内 AI 产生了多条 assistant 消息，每条都可能包含思考+回复：

```
┌──────────────────────────────────────────┐
│ 👤 帮我调研 React vs Vue，然后给建议       │
├──────────────────────────────────────────┤
│                                          │
│  🧠 思考                           ▼ 折叠 │
│  ┌────────────────────────────────────┐  │
│  │ 需要分别调研两个框架的生态和性能...   │  │
│  └────────────────────────────────────┘  │
│                                          │
│  🔧 search("React 2024 生态")    ✓ 2.1s  │
│                                          │
│  💬 中间回复                              │
│  ┌────────────────────────────────────┐  │
│  │ React 方面的调研结果：               │  │
│  │ 生态成熟，Next.js 是主流框架...      │  │
│  └────────────────────────────────────┘  │
│                                          │
│  🧠 思考                           ▼ 折叠 │
│  ┌────────────────────────────────────┐  │
│  │ React 部分已完成，继续调研 Vue...    │  │
│  └────────────────────────────────────┘  │
│                                          │
│  🔧 search("Vue 2024 生态")      ✓ 1.9s  │
│                                          │
│  💬 最终回复                              │
│  ┌────────────────────────────────────┐  │
│  │ Vue 方面的调研结果：                 │  │
│  │ 学习曲线平缓，Nuxt 是主流框架...     │  │
│  │                                     │  │
│  │ ─── 综合建议 ───                    │  │
│  │ 如果团队偏好的是...推荐 React；      │  │
│  │ 如果追求开发效率...推荐 Vue。        │  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

### 4.6 Ralph 验证循环

```
┌──────────────────────────────────────────┐
│ 👤 写一个快速排序函数并用测试验证           │
├──────────────────────────────────────────┤
│                                          │
│  🔄 第 1 次尝试                           │
│  │                                       │
│  🧠 思考                           ▼ 折叠 │
│  💬 回复                                  │
│  ┌────────────────────────────────────┐  │
│  │ ```python                           │  │
│  │ def quicksort(arr):                 │  │
│  │   ...                               │  │
│  │ ```                                 │  │
│  └────────────────────────────────────┘  │
│  🔧 run_test("quicksort")        ✗ 失败  │
│  ┌────────────────────────────────────┐  │
│  │ AssertionError: 边界情况未处理       │  │
│  └────────────────────────────────────┘  │
│                                          │
│  🔄 第 2 次尝试                           │
│  │                                       │
│  🧠 思考                           ▼ 折叠 │
│  💬 回复                                  │
│  ┌────────────────────────────────────┐  │
│  │ 修复了边界情况：                     │  │
│  │ ```python                           │  │
│  │ def quicksort(arr):                 │  │
│  │   if len(arr) <= 1: return arr      │  │
│  │   ...                               │  │
│  │ ```                                 │  │
│  └────────────────────────────────────┘  │
│  🔧 run_test("quicksort")        ✓ 通过  │
│                                          │
│  ✅ 验证通过                              │
└──────────────────────────────────────────┘
```

---

## 五、Team 模式展示设计（v7 原型风格）

### 5.1 设计来源

Team 模式的 UI 设计遵循 v7 设计原型（`m59-chat-ui-v7.html`）的风格，核心要素：

1. **统一面板（Unified Panel）**：所有 Team 信息在一个卡片内，分 Section 展示，不用 Tab
2. **任务拆解（Task Board）**：编号任务行，显示分配的 Agent 和状态
3. **依赖关系（DAG）**：节点+箭头的流程图，标注完成/运行/等待状态
4. **团队进度（Team Progress）**：每个子团队一个可折叠卡片，内含 Agent 详情
5. **Agent 详情**：头像+名称+状态+thinking line+tool line+reply line
6. **进度条**：每个团队卡片有微型进度条
7. **操作按钮**：中断的团队显示"恢复"/"取消"按钮

### 5.2 数据模型扩展

Team 模式下 `AgentWorkProcess.panel` 存在时，渲染为统一面板：

```typescript
/** Team 统一面板数据 */
interface TeamPanel {
  /** 任务拆解 — 复用现有 PlanEntry 类型 */
  taskBoard: TaskBoardSection;
  /** 依赖关系 DAG */
  dag?: DagSection;
  /** 团队进度 */
  teamProgress: TeamProgressSection[];
}

/** 任务拆解 — 扩展现有 PlanEntry */
interface TaskBoardSection {
  /** 复用现有 OrchestrationPlan 的 PlanEntry，增加序号和 Agent 分配 */
  entries: Array<PlanEntry & {
    num: number;
    agentName: string | null;
  }>;
}

/** DAG 依赖关系 */
interface DagSection {
  nodes: Array<{
    id: string;
    label: string;
    status: 'done' | 'running' | 'pending' | 'failed';
  }>;
  edges: Array<{ from: string; to: string }>;
}

/** 团队进度 */
interface TeamProgressSection {
  teamId: string;
  teamName: string;
  teamIcon: string;
  status: 'running' | 'completed' | 'failed' | 'interrupted';
  progressPercent: number;
  durationMs: number | null;
  /** Agent 详情 — 复用 Activity 模型而非独立的 Line 模型 */
  agents: AgentProgress[];
  /** 中断时的操作 */
  actions?: ('resume' | 'cancel')[];
}

/** Agent 进度 — 复用 Activity 而非独立 Line 类型 */
interface AgentProgress {
  agentKey: string;
  agentName: string;
  agentIcon: string;
  status: 'running' | 'completed' | 'failed' | 'waiting';
  /** 复用 Activity 类型，通过 variant 控制紧凑/展开渲染 */
  activities: Activity[];
}
```

**关键设计决策**：

1. **复用 `PlanEntry`**：`TaskBoardSection.entries` 直接扩展 `PlanEntry`，增加 `num` 和 `agentName`，不新建类型
2. **复用 `Activity`**：`AgentProgress.activities` 使用 `Activity` 类型（think/act/say/delegate），而非独立的 `AgentDetailLine` 类型。渲染时通过 `variant` prop 控制紧凑行风格（Team 面板内）vs 折叠卡片风格（单 Agent 时间线）
3. **DAG 构建逻辑归属**：DAG 边关系和进度百分比的计算收敛到 `features/chat/buildTeamDag.ts` 纯函数中，`useTeamPanel` composable 只做响应式编排

### 5.3 Team 展示布局

```
┌──────────────────────────────────────────────────────────────┐
│ 👤 帮我开发用户认证和权限管理的后端 API                        │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  🧠 思考                                               ▼ 折叠 │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ 用户需要认证和权限管理 API，涉及 JWT token 签发、       │  │
│  │ RBAC 权限模型、HTTP 中间件拦截。需要拆分为后端 API      │  │
│  │ 实现、代码审查、测试编写三个子任务…                      │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌─ 统一面板 ─────────────────────────────────────────────┐  │
│  │                                                        │  │
│  │  📋 任务拆解                                     ▼ 5   │  │
│  │  ┌──────────────────────────────────────────────────┐  │  │
│  │  │ ① 实现 JWT 认证中间件和 RBAC 权限模型    Golang 工程师  ⚡ │  │  │
│  │  │ ② 代码审查和安全检查                      代码审查员  ⏳ │  │  │
│  │  │ ③ 编写单元测试和集成测试                  测试工程师  ⏳ │  │  │
│  │  │ ④ 实现登录页面和权限管理界面              Vue 工程师   ⚡ │  │  │
│  │  │ ⑤ 用户行为数据分析                        数据分析师  ✓ │  │  │
│  │  └──────────────────────────────────────────────────┘  │  │
│  │                                                        │  │
│  │  🔀 依赖关系                                     ▼     │  │
│  │  ┌──────────────────────────────────────────────────┐  │  │
│  │  │ ✓ 数据分析 → ⚡ 后端 API → ⏳ 代码审查 → ⏳ 测试   │  │  │
│  │  │                              | ⚡ 前端 UI          │  │  │
│  │  └──────────────────────────────────────────────────┘  │  │
│  │                                                        │  │
│  │  📊 团队进度                                     ▼ 4   │  │
│  │  ┌──────────────────────────────────────────────────┐  │  │
│  │  │                                                  │  │  │
│  │  │  ┌─ API 后端团队 ──── ⚡ 运行中 ──── ████░░ 40% ── 1m42s ▼ ┐  │  │
│  │  │  │  G Golang 工程师                          ⚡ 运行中  │  │  │
│  │  │  │  │ 基于需求设计 JWT 中间件和 RBAC 模型…▍       │  │  │
│  │  │  │  │ ⚡ file_read  auth.go          ✓ 0.3s   │  │  │
│  │  │  │  │ ⚡ file_write  jwt.go           ⏳ 5s…   │  │  │
│  │  │  │  │                                              │  │  │
│  │  │  │  R 代码审查员                             ⏳ 等待中  │  │  │
│  │  │  │    等待 Golang 工程师完成…                    │  │  │
│  │  │  │  │                                              │  │  │
│  │  │  │  T 测试工程师                             ⏳ 等待中  │  │  │
│  │  │  │    等待代码审查通过…                          │  │  │
│  │  │  └──────────────────────────────────────────────┘  │  │
│  │  │                                                  │  │  │
│  │  │  ┌─ UI 前端团队 ──── ⚡ 运行中 ──── ███░░░ 33% ── 0m55s ▼ ┐  │  │
│  │  │  │  V Vue 工程师                            ⚡ 运行中  │  │  │
│  │  │  │  │ 基于API文档设计登录表单组件…              │  │  │
│  │  │  │  │ ⚡ file_write  LoginPage.vue     ⏳ 12s…  │  │  │
│  │  │  │  │                                              │  │  │
│  │  │  │  D 设计师                                 ⏳ 等待中  │  │  │
│  │  │  │    等待 Vue 工程师完成…                      │  │  │
│  │  │  └──────────────────────────────────────────────┘  │  │
│  │  │                                                  │  │  │
│  │  │  ┌─ 数据分析团队 ──── ✓ 已完成 ──── ██████ 100% ── 3m20s ▶ ┐  │  │
│  │  │  │  D 数据分析师                           ✓ 已完成  │  │  │
│  │  │  │    已完成数据提取和清洗，共处理 12,847 条记录。  │  │  │
│  │  │  │  │                                              │  │  │
│  │  │  │  M ML 工程师                             ✓ 已完成  │  │  │
│  │  │  │    完成统计建模，用户留存率 68%。              │  │  │
│  │  │  └──────────────────────────────────────────────┘  │  │
│  │  │                                                  │  │  │
│  │  │  ┌─ 部署团队 ──── ⏸ 已中断 ──── █████░ 50% ── 4m05s ▶ ┐  │  │
│  │  │  │  [恢复] [取消]                                    │  │  │
│  │  │  │  D DevOps 工程师                        ⏸ 已中断  │  │  │
│  │  │  │  │ ⚡ bash  docker build…         ✓ 45s    │  │  │
│  │  │  │  │ ⚡ bash  docker push…          ✗ 2m03s  │  │  │
│  │  │  │  │ ❌ Docker registry 认证错误                    │  │  │
│  │  │  └──────────────────────────────────────────────┘  │  │
│  │  │                                                  │  │  │
│  │  └──────────────────────────────────────────────────┘  │  │
│  │                                                        │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                              │
│  💬 回复                                                     │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ 已为你组建了 4 个团队，正在并行执行：                     │  │
│  │ 1. **后端 API 开发团队** — Golang 工程师正在实现 JWT 中间件 │  │
│  │ 2. **前端 UI 开发团队** — Vue 工程师正在实现登录页面       │  │
│  │ 3. **数据分析团队** — ✅ 已完成                           │  │
│  │ 4. **部署团队** — ⏸ 已中断，可点击恢复继续执行            │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

### 5.4 v7 原型风格要素映射

| v7 原型元素 | 方案对应 | 实现说明 |
|------------|---------|---------|
| `unified-panel` | `TeamPanel.vue` | 统一面板卡片，分 Section |
| `panel-section` (任务拆解) | `TaskBoardSection.vue` | 编号任务行 + Agent 分配 + 状态 |
| `panel-section` (DAG) | `DagSection.vue` | 节点+箭头流程图（简单版用 flex 布局，不引入 SVG/Canvas） |
| `panel-section` (团队进度) | `TeamProgressSection.vue` | 可折叠团队卡片 |
| `team-prog` | `TeamProgressCard.vue` | 单个团队进度卡片 |
| `team-prog-header` | 卡片头部 | 头像+名称+状态+进度条+时长 |
| `team-prog-body` | 卡片内容 | Agent 详情列表 |
| `agent-detail` | `AgentDetail.vue` | Agent 头部 + 复用 Activity 组件（variant="compact"） |
| `thinking-line` | `ThinkActivity` (variant="compact") | 紧凑行风格：斜体+左边框+光标 |
| `tool-line` | `ActActivity` (variant="compact") | 紧凑行风格：图标+工具名+参数+状态+时长 |
| `reply-line` | `SayActivity` (variant="compact") | 紧凑行风格：正文内容 |
| `progress-bar` | 微型进度条 | 2px 高度，颜色用 `var(--color-accent)` |
| `pulse-dot` | 脉冲点 | 统一使用项目现有实现，颜色用 `var(--color-accent)` |
| `tp-action-btn` | 操作按钮 | 中断时显示恢复/取消 |

### 5.5 v7 原型色值映射

v7 原型使用硬编码色值，必须映射到项目 CSS 变量：

| v7 原型色值 | 项目 CSS 变量 | 日间值 | 夜间值 | 用途 |
|------------|-------------|--------|--------|------|
| `--color-primary: #5b8af5` | `--color-accent` | `#DCA03E` | `#4DD8E8` | 主强调色 |
| `--bg: #1a1a2e` | `--canvas-base` | `#FEFBF4` | `#0A1018` | 画布底色 |
| `--surface: #16213e` | `--glass-surface` | `rgba(255,253,245,0.88)` | `rgba(18,26,40,0.68)` | 玻璃表面 |
| `--surface-hover: #1a2744` | `--glass-surface-hover` | `rgba(255,253,245,0.78)` | `rgba(22,28,40,0.75)` | 悬停表面 |
| `--border: rgba(255,255,255,0.08)` | `--glass-border` | `rgba(235,220,200,0.7)` | `rgba(120,160,220,0.12)` | 边框 |
| `--text: #ebebf0` | `--color-text-primary` | `#2C2218` | `#D0DCE8` | 主文本 |
| `--text-muted: #9ca0b0` | `--color-text-secondary` | `#6B5B4D` | `#94A5BE` | 辅助文本 |
| `--success: #3fe0a0` | `--color-success` | `#4CAF7C` | `#3FE0A0` | 成功 |
| `--warning: #ffaf4d` | `--color-warning` | `#F09B54` | `#FFAF4D` | 警告 |
| `--danger: #ff5e7a` | `--color-danger` | `#E55C5C` | `#FF5E7A` | 危险 |

**规则**：Team 面板内所有颜色必须使用 CSS 变量，禁止硬编码 hex。v7 原型的视觉风格（布局、间距、圆角）保留，但色值全部替换。

### 5.6 Team 面板折叠策略

| Section | 默认状态 | 折叠条件 |
|---------|---------|---------|
| 任务拆解 | 展开 | 用户手动折叠 |
| 依赖关系 | 展开 | 用户手动折叠 |
| 团队进度 | 展开 | 用户手动折叠 |
| 单个团队卡片 | 运行中展开，完成/中断折叠 | 用户手动切换 |

---

## 六、组件架构

### 6.1 组件树

```
ConversationView (页面)
├── ConversationTurn (每个 Turn — 纯布局编排)
│   ├── UserMessageBubble (用户消息气泡)
│   └── AgentWorkPanel (Agent 工作面板 — 渲染分支入口)
│       ├── [panel 不存在] ActivityTimeline (活动时间线 — 单 Agent)
│       │   ├── ThinkActivity (思考节点, variant="card"|"compact")
│       │   ├── ActActivity (工具节点, variant="card"|"compact")
│       │   ├── SayActivity (回复节点, variant="card"|"compact")
│       │   ├── NoticeActivity (通知节点)
│       │   └── DelegateActivity (委派节点)
│       │       └── AgentWorkPanel (递归)
│       └── [panel 存在] TeamPanel (统一面板 — Team 模式)
│           ├── TaskBoardSection (任务拆解)
│           ├── DagSection (依赖关系)
│           └── TeamProgressSection (团队进度)
│               └── TeamProgressCard[] (每个团队)
│                   └── AgentDetail[] (每个 Agent)
│                       └── ActivityTimeline (variant="compact")
│                           ├── ThinkActivity (variant="compact")
│                           ├── ActActivity (variant="compact")
│                           └── SayActivity (variant="compact")
```

### 6.2 组件路径与分层合规

| 组件 | 路径 | 层级 | 说明 |
|------|------|------|------|
| `ConversationTurn` | `components/chat/ConversationTurn.vue` | 展示 | 纯布局编排，props 驱动 |
| `UserMessageBubble` | `components/chat/UserMessageBubble.vue` | 展示 | 用户消息气泡 |
| `AgentWorkPanel` | `components/chat/AgentWorkPanel.vue` | 展示 | 根据 `panel` 是否存在分支渲染 |
| `ActivityTimeline` | `components/chat/ActivityTimeline.vue` | 展示 | 按序渲染 Activities |
| `ThinkActivity` | `components/chat/ThinkActivity.vue` | 展示 | variant prop 控制卡片/紧凑行 |
| `ActActivity` | `components/chat/ActActivity.vue` | 展示 | variant prop 控制卡片/紧凑行 |
| `SayActivity` | `components/chat/SayActivity.vue` | 展示 | variant prop 控制卡片/紧凑行，含 A2UI 渲染 |
| `NoticeActivity` | `components/chat/NoticeActivity.vue` | 展示 | 轻量通知行，type 区分降级/信息 |
| `DelegateActivity` | `components/chat/DelegateActivity.vue` | 展示 | 递归渲染 AgentWorkPanel |
| `TeamPanel` | `components/chat/TeamPanel.vue` | 展示 | v7 风格统一面板 |
| `TaskBoardSection` | `components/chat/TaskBoardSection.vue` | 展示 | 任务拆解列表 |
| `DagSection` | `components/chat/DagSection.vue` | 展示 | 依赖关系图 |
| `TeamProgressSection` | `components/chat/TeamProgressSection.vue` | 展示 | 团队进度列表 |
| `TeamProgressCard` | `components/chat/TeamProgressCard.vue` | 展示 | 单个团队卡片 |
| `AgentDetail` | `components/chat/AgentDetail.vue` | 展示 | Agent 头部 + ActivityTimeline(variant="compact") |

**类型引入规则**：所有展示组件从 `features/chat/types.ts` 引入类型（type-only import），符合红线 #12。

**容器逻辑归属**：
- `useConversationTimeline` → `features/chat/composables/useConversationTimeline.ts`
- `useTeamPanel` → `features/chat/composables/useTeamPanel.ts`
- DAG 构建纯函数 → `features/chat/buildTeamDag.ts`

### 6.3 组件职责

| 组件 | 职责 | 交互 |
|------|------|------|
| `ConversationTurn` | 纯布局编排：用户消息 + Agent 面板 | 折叠/展开整个 Turn |
| `UserMessageBubble` | 用户消息气泡 | — |
| `AgentWorkPanel` | 渲染分支入口：根据 `panel` 是否存在选择 ActivityTimeline 或 TeamPanel | 折叠/展开工作详情 |
| `ActivityTimeline` | 按序渲染 Activities，传递 variant 给子组件 | — |
| `ThinkActivity` | 思考节点；variant="card" 折叠卡片，variant="compact" 紧凑行 | 点击展开/折叠 |
| `ActActivity` | 工具节点；variant="card" 卡片，variant="compact" 紧凑行 | 点击展开参数/结果 |
| `SayActivity` | 回复节点；variant="card" 卡片，variant="compact" 紧凑行；含 A2UI 渲染分支 | 流式时显示脉冲 |
| `NoticeActivity` | 通知节点；轻量提示行，type 区分降级/信息 | — |
| `DelegateActivity` | 子 Agent 委派，递归渲染 AgentWorkPanel | — |
| `TeamPanel` | Team 统一面板 | Section 级折叠 |
| `TaskBoardSection` | 任务拆解列表 | 折叠/展开 |
| `DagSection` | 依赖关系图（flex 布局 + 箭头） | 折叠/展开 |
| `TeamProgressSection` | 团队进度列表 | 折叠/展开 |
| `TeamProgressCard` | 单个团队卡片 | 折叠/展开 + 操作按钮 |
| `AgentDetail` | Agent 头部 + ActivityTimeline(variant="compact") | — |

### 6.4 折叠策略

| 状态 | Think | Act | Say | Notice |
|------|-------|-----|-----|--------|
| 流式中 | 展开显示光标 | 显示 spinner | 展开显示光标 | 始终可见 |
| 刚完成 | 展开短暂停留 | 显示结果摘要 | 始终展开 | 始终可见 |
| 完成后 500ms | 自动折叠 | 自动折叠 | 始终展开 | 始终可见 |
| 用户手动展开 | 保持展开 | 保持展开 | — | — |

**整个 Turn 的折叠**：
- 当 Turn 内所有 Activity 都完成且用户滚动离开时，Turn 折叠为一行摘要
- 摘要内容：Agent 名称 + "使用了 N 个工具" / "回复了 N 次" + 时长

---

## 七、流式体验设计

```
用户发送消息后：

1. [0-2s] 显示进度占位
   ⏳ 正在思考...

2. [2-5s] Think Activity 流式展开
   🧠 思考
   ┌────────────────────────────────────┐
   │ 用户想知道天气，我需要搜索...▌       │  ← 光标闪烁
   └────────────────────────────────────┘

3. [5-7s] Act Activity 出现
   🔧 search("北京天气")           ⏳ 运行中

4. [7-8s] Act Activity 完成
   🔧 search("北京天气")           ✓ 1.8s  ← 状态变化
   ┌────────────────────────────────────┐
   │ 北京：晴天，25°C...                  │  ← 结果出现
   └────────────────────────────────────┘

5. [8-10s] Say Activity 流式展开
   💬 回复
   ┌────────────────────────────────────┐
   │ 北京今天是晴天，气温25°C，▌          │  ← 光标闪烁
   └────────────────────────────────────┘

6. [10s+] 完成
   Think 自动折叠，Say 保持展开
```

---

## 八、与现有代码的映射

### 8.1 数据层

| 新概念 | 现有实现 | 改动 |
|--------|---------|------|
| `ConversationTurn` | `TurnBlockGroup` + `AgentBlock` | 合并，`id` 使用后端 `turn_id`，去掉 `assistant: Message \| null` 单槽限制 |
| `AgentWorkProcess` | `AgentBlock` | 基本不变，`timeline` 改名 `activities`，增加 `panel?` 字段 |
| `Activity` | `TimelineEntry` | 基本不变，`kind` 值调整，增加 `variant` 和 A2UI 支持 |
| `think` | `TimelineEntry.kind === 'thinking'` | 改名 + 增加 `label` 字段 |
| `act` | `TimelineEntry.kind === 'tool'` | 改名 |
| `say` | `TimelineEntry.kind === 'reply'` | 改名 + 增加 `isFinal`/`variant`/`a2uiLines` 字段 |
| `delegate` | `TimelineEntry.kind === 'subagent'` | 改名，渲染分支由 `AgentWorkPanel` 根据 `panel` 决定 |
| `notice` | `TimelineEntry.kind === 'notice'` | 改名 + 增加 `type` 字段（degradation/info） |
| `TeamPanel` | 无 | 新增，统一面板数据模型 |
| `TaskBoardSection` | `OrchestrationPlan` / `PlanEntry` | 扩展 `PlanEntry`，增加 `num` 和 `agentName`，不新建类型 |
| `TeamProgressSection` | `AgentBlock` (子 Agent) | 重构为面板卡片模型，`AgentProgress.activities` 复用 `Activity` |

### 8.2 逻辑层

| 新 Composable/函数 | 现有实现 | 改动 |
|-------------------|---------|------|
| `useConversationTimeline` | `useAgentBlocks` | 重构，输出 `ConversationTurn[]`；内含 `execution_progress` → `think { label: "进度" }` 转换 |
| `useTeamPanel` | 无 | 新增，构建 `TeamPanel`；调用 `buildTeamDag` 纯函数 |
| `buildTeamDag` | 无 | 新增纯函数，DAG 边关系和进度百分比计算，放 `features/chat/buildTeamDag.ts` |
| `buildActivityFromMessage` | `messagePlannerPresentation` | 重构，从 Message 构建 Activity（含 isFinal 判定、A2UI 分支） |
| — | `useChatTimeline` | 简化，去掉 TurnBlock/MessageRow 分支 |
| — | `groupMessagesByTurn` | 删除，被 `useConversationTimeline` 替代 |
| — | `compactTimeline` | 删除，被 `ActivityTimeline` 替代 |

### 8.3 渲染层

| 新组件 | 现有组件 | 改动 |
|--------|---------|------|
| `ConversationTurn` | `TurnBlock` + `AgentBlock` (根) | 合并，纯布局编排 |
| `UserMessageBubble` | `TurnBlock` 内联用户消息 | 提取为独立组件 |
| `AgentWorkPanel` | `AgentBlock` | 重构，增加渲染分支（panel 存在 → TeamPanel） |
| `ActivityTimeline` | `AgentBlock` 的 `.agent-timeline` | 提取为独立组件，传递 variant |
| `ThinkActivity` | `AgentThinkingSection` | 改名 + label 支持 + variant prop |
| `ActActivity` | `AgentToolSection` | 改名 + variant prop |
| `SayActivity` | `AgentBlock` 内联 `.section--reply` | 提取为独立组件 + variant prop + A2UI 渲染分支 |
| `DelegateActivity` | `AgentBlock` 内联 `.sub-agent-timeline-entry` | 提取为独立组件，递归 AgentWorkPanel |
| `NoticeActivity` | `AgentBlock` 内联 `.notice-timeline-entry` | 提取为独立组件，type 区分降级/信息 |
| `TeamPanel` | 无 | 新增，v7 风格统一面板 |
| `TaskBoardSection` | `PlanCard` | 重构为 v7 风格任务行 |
| `DagSection` | 无 | 新增，flex 布局+箭头 |
| `TeamProgressSection` | 无 | 新增 |
| `TeamProgressCard` | 无 | 新增 |
| `AgentDetail` | 无 | 新增，Agent 头部 + ActivityTimeline(variant="compact") |
| — | `CompactTimeline` | 删除 |
| — | `ChatMessageRow` | 删除（聊天场景） |
| — | `ChatReactSteps` | 删除，ReAct 融入 Think+Act |

### 8.4 需要删除的文件

| 文件 | 原因 |
|------|------|
| `groupMessagesByTurn.ts` | 被 `useConversationTimeline` 替代 |
| `compactTimeline.ts` | 被 `ActivityTimeline` 替代 |
| `TurnBlock.vue` | 被 `ConversationTurn` 替代 |
| `CompactTimeline.vue` | 被 `ActivityTimeline` 替代 |
| `ChatMessageRow.vue` | 被 `ConversationTurn` 替代（聊天场景） |
| `ChatReactSteps.vue` | ReAct 步骤融入 `ThinkActivity` + `ActActivity` |

### 8.5 需要修改的核心文件

| 文件 | 改动 |
|------|------|
| `useAgentBlocks.ts` | 重构为 `useConversationTimeline`，输出 `ConversationTurn[]` |
| `agentTreeTypes.ts` | 更新类型定义（Activity 替代 TimelineEntry） |
| `ChatMessageList.vue` | 简化为只渲染 `ConversationTurn[]` |
| `ChatMessagePanel.vue` | 去掉三套路径的条件分支 |
| `messagePlannerPresentation.ts` | 适配新 Activity 构建逻辑 |

---

## 九、实施路线

### Phase 1：数据模型统一

1. 定义 `ConversationTurn` / `Activity` / `TeamPanel` 类型（`features/chat/types.ts`）
2. 实现 `buildActivityFromMessage` 纯函数（含 isFinal 判定、A2UI 分支、execution_progress 转换）
3. 重构 `useAgentBlocks` → `useConversationTimeline`，输出 `ConversationTurn[]`
4. 实现 `buildTeamDag` 纯函数（`features/chat/buildTeamDag.ts`）
5. 实现 `useTeamPanel` composable

### Phase 2：单 Agent 渲染

1. 定义核心组件接口（`defineProps<T>()` / `defineEmits<T>()` 签名）
2. 实现 `ConversationTurn` + `UserMessageBubble` + `AgentWorkPanel` + `ActivityTimeline`
3. 实现 `ThinkActivity` / `ActActivity` / `SayActivity` / `NoticeActivity`（含 variant prop 和 A2UI 渲染分支）
4. 替换 `ChatMessageList.vue` 为统一渲染路径
5. **虚拟滚动兼容**：为 `ConversationTurn` 提供高度估算 hint（基于 Activity 数量和类型），或采用非虚拟滚动 + 懒加载策略
6. 删除 `TurnBlock` / `CompactTimeline` / `ChatMessageRow`

### Phase 3：Team 面板

1. 实现 `TeamPanel` + `TaskBoardSection` + `DagSection`（flex 布局+箭头）
2. 实现 `TeamProgressSection` + `TeamProgressCard` + `AgentDetail`
3. 适配 v7 原型的视觉风格（统一面板、进度条、脉冲点），所有色值使用 CSS 变量
4. Activity 组件 variant="compact" 紧凑行风格实现

### Phase 4：流式体验

1. Activity 级流式状态管理
2. Think 自动折叠动画
3. Say 流式光标
4. isFinal 动态降级（新 assistant 消息到达时前一个 say 降级为 isFinal=false）

### Phase 5：清理

1. 删除废弃文件
2. 更新测试
3. 性能优化

---

## 十、风险与缓解

| 风险 | 缓解 |
|------|------|
| 重构范围大，可能引入回归 | Phase 分步实施，每步可独立验证 |
| 虚拟滚动与 Activity 模型不兼容 | Phase 2 即处理：高度估算 hint 或非虚拟滚动+懒加载 |
| v7 原型风格与项目 UX 规范冲突 | Team 面板内部用 v7 布局风格，色值全部使用项目 CSS 变量（见 §5.5 映射表） |
| Ralph Loop 迭代边界判定需后端配合 | 前端先用 `runner_completion` + `text_delta` 时序推断（§3.5），长期后端增加 `ralph_iteration` 字段 |
| `mergeSessionMessages` 排序在 Activity 模型下是否兼容 | Activity 模型不改变消息合并逻辑。`mergeSessionMessages` 仍在 Message 层工作，`useConversationTimeline` 在合并后的 Message[] 上构建 Activity |
| `isFinal` 流式期间临时标记可能闪烁 | 流式期间当前 say 标记 `isFinal: true`，新 assistant 消息到达时前一个降级为 `false`。UI 用过渡动画避免闪烁 |

---

## 附录 A：方案审查报告（v1 → v2 修复记录）

> 审查日期：2026-06-12
> 审查依据：`aranea-review` SKILL（前端数据流合规、组件分层、聊天消息分组、UX 主题）

### A.1 v1 审查概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **前端 — 数据流合规** | 1 | 2 | 0 | 3 |
| **前端 — 组件分层** | 1 | 3 | 1 | 5 |
| **前端 — 聊天消息分组** | 0 | 2 | 0 | 2 |
| **前端 — UX 主题** | 1 | 2 | 0 | 3 |
| **架构 — 正确性** | 1 | 2 | 0 | 3 |
| **架构 — 可维护性** | 0 | 2 | 1 | 3 |
| **合计** | **4** | **13** | **2** | **19** |

### A.2 阻断项修复状态

| ID | 问题 | 修复位置 | 状态 |
|----|------|---------|------|
| R-01 | DAG 构建逻辑未明确归属层 | §5.2 关键设计决策 #3 + §8.2 `buildTeamDag` 纯函数 | ✅ 已修复 |
| R-02 | Team 面板组件路径未明确 | §6.2 组件路径与分层合规表 | ✅ 已修复 |
| R-03 | v7 原型硬编码色值违反 UX 规范 | §5.5 v7 原型色值映射表 | ✅ 已修复 |
| R-04 | `isFinal` 判定逻辑未定义 | §3.4 `isFinal` 判定规则 | ✅ 已修复 |

### A.3 建议项修复状态

| ID | 问题 | 修复位置 | 状态 |
|----|------|---------|------|
| S-01 | execution_progress 如何融入 Activity | §3.3 构建规则表 + §8.2 `useConversationTimeline` | ✅ 已修复 |
| S-02 | ConversationTurn 职责偏重 | §6.1 组件树拆分为 ConversationTurn + UserMessageBubble + AgentWorkPanel | ✅ 已修复 |
| S-03 | AgentDetail Line 组件与 Activity 组件重叠 | §5.2 关键设计决策 #2 + §6.1 组件树复用 Activity + variant | ✅ 已修复 |
| S-04 | delegate 渲染分支逻辑位置 | §5.3 渲染分支规则 + §6.1 组件树 | ✅ 已修复 |
| S-05 | ConversationTurn.id 应使用 turn_id | §3.1 数据模型 `id` 注释 + §8.1 数据层映射 | ✅ 已修复 |
| S-06 | mergeSessionMessages 在 Activity 模型下如何工作 | §十 风险与缓解 | ✅ 已修复 |
| S-07 | pulse-dot 动画颜色不统一 | §5.4 v7 原型风格要素映射表 | ✅ 已修复 |
| S-08 | thinking-line 与 AgentThinkingSection 风格不统一 | §5.4 要素映射表 variant="compact" + §6.3 组件职责 | ✅ 已修复 |
| S-09 | Ralph Loop 迭代边界判定 | §3.5 Ralph Loop 迭代边界判定 | ✅ 已修复 |
| S-10 | TaskBoardSection 与 PlanEntry 关系 | §5.2 关键设计决策 #1 + §8.1 数据层映射 | ✅ 已修复 |
| S-11 | 组件接口未定义 | §9 Phase 2 步骤 1 | ✅ 已修复 |
| S-12 | 虚拟滚动适配推迟 | §9 Phase 2 步骤 5 + §十 风险与缓解 | ✅ 已修复 |
| S-13 | A2UI 模式未覆盖 | §3.1 SayActivity variant/a2uiLines + §3.3 构建规则表 + §6.3 SayActivity 组件职责 | ✅ 已修复 |

### A.4 提示项处理状态

| ID | 描述 | 处理 | 状态 |
|----|------|------|------|
| T-01 | DagSection 复杂度 | §5.4 明确"简单版用 flex 布局，不引入 SVG/Canvas" | ✅ 已处理 |
| T-02 | Activity.kind 改名迁移成本 | 保留 think/act/say/delegate 命名，迁移成本在 Phase 分步实施中消化 | ✅ 已处理 |

---

## 附录 B：第二轮系统性评审报告（v2 → v3 修复记录）

> 审查日期：2026-06-13
> 审查方法：交叉验证方案文档与实际代码（类型定义、CSS 变量、组件结构、后端事件）

### B.1 评审概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **数据模型与代码一致性** | 1 | 4 | 0 | 5 |
| **文档内部一致性** | 0 | 2 | 1 | 3 |
| **合计** | **1** | **6** | **1** | **8** |

### B.2 阻断项修复状态

| ID | 问题 | 修复位置 | 状态 |
|----|------|---------|------|
| R2-01 | §5.5 色值映射表 7 个色值与实际 CSS 变量定义不一致（`--color-accent` 日间 `#E9A23B`→`#DCA03E`，夜间 `#00E5FF`→`#4DD8E8`；`--canvas-base` 夜间 `#090D14`→`#0A1018`；`--glass-surface` 日间 0.65→0.88，夜间 `rgba(18,24,34,0.65)`→`rgba(18,26,40,0.68)`；`--glass-border` 夜间 `rgba(255,255,255,0.08)`→`rgba(120,160,220,0.12)`；`--color-text-primary` 日间 `#3A322C`→`#2C2218`，夜间 `#EBEBF0`→`#D0DCE8`；`--color-text-secondary` 日间 `#8B7A6B`→`#6B5B4D`，夜间 `#9CA0B0`→`#94A5BE`） | §5.5 色值映射表 | ✅ 已修复 |

### B.3 建议项修复状态

| ID | 问题 | 修复位置 | 状态 |
|----|------|---------|------|
| R2-02 | Activity 类型缺少 `notice`（现有 TimelineEntry 有 5 种 kind，方案只有 4 种） | §3.1 增加 `NoticeActivity` + §3.2 语义表 + §3.3 构建规则表 + §6.1 组件树 + §6.2 组件表 + §6.3 职责表 + §6.4 折叠策略 + §8.1/§8.3 映射表 | ✅ 已修复 |
| R2-03 | `AgentWorkProcess` 缺少 `task`/`result`/`hasPartialFailure`/`plan`/`teamStatus`/`progressSections`/`startedAt`/`finishedAt` 字段映射 | §3.1 `AgentWorkProcess` 增加迁移字段 | ✅ 已修复 |
| R2-04 | `AgentWorkProcess.status` 只有 3 种，实际 `AgentBlockStatus` 有 6 种（含 `tool_running`/`tool_blocked`/`partial_failure`） | §3.1 `AgentWorkProcess.status` 注释说明映射关系 | ✅ 已修复 |
| R2-05 | 附录 A.3 S-13 引用不存在的 §4.7 章节 | 附录 A.3 S-13 修复位置改为 §6.3 | ✅ 已修复 |
| R2-06 | `ThinkActivity` 缺少 `durationMs` 字段（现有 `ThinkingSection` 有此字段） | §3.1 `ThinkActivity` 增加 `durationMs` | ✅ 已修复 |
| R2-07 | `SayActivity` 缺少 `durationMs` 字段（现有 `ReplySection` 有此字段） | §3.1 `SayActivity` 增加 `durationMs` | ✅ 已修复 |

### B.4 提示项处理状态

| ID | 描述 | 处理 | 状态 |
|----|------|------|------|
| R2-08 | `ToolActivity.status` 已包含 `blocked`/`cancelled`，与现有 `ToolSectionStatus` 一致，无需额外修改 | 确认一致 | ✅ 已确认 |
