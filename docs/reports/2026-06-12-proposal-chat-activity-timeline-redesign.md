# 聊天会话过程展示重设计方案

> **文档地位**：聊天 UI 重设计的完整方案，融合"活动时间线"模型与 v7 设计原型的 Team 面板风格。
> **日期**：2026-06-12
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
  status: 'running' | 'completed' | 'failed';
  durationMs: number | null;

  /** 活动时间线 — 严格按发生顺序排列 */
  activities: Activity[];

  /** 编排计划（Team 模式） */
  plan?: OrchestrationPlan;
}

/** 活动节点 — 时间线上的最小展示单元 */
type Activity =
  | { kind: 'think'; id: string; content: string; label?: string; collapsed: boolean; streaming: boolean }
  | { kind: 'act';  id: string; tool: ToolActivity }
  | { kind: 'say';  id: string; content: string; isFinal: boolean; streaming: boolean }
  | { kind: 'delegate'; id: string; subAgent: AgentWorkProcess };

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
| **think** | 🧠 | AI 在思考/规划/推理 | 可折叠，完成后自动折叠；label 区分"规划"/"推理"/"重规划" |
| **act** | 🔧 | AI 在使用工具 | 显示工具名+状态，可展开参数/结果 |
| **say** | 💬 | AI 在回复用户 | 始终可见，不可折叠；`isFinal` 区分中间回复与最终回复 |
| **delegate** | 👥 | AI 委派子 Agent | 单 Agent：嵌套时间线；Team：统一面板（见 §五） |

### 3.3 Activity 构建规则

从后端消息到 Activity 的映射：

| 后端事件 | 生成的 Activity | 说明 |
|---------|----------------|------|
| assistant + `reasoning_markdown` 非空 | `think` | 思考内容 |
| assistant + ReAct `/*PLANNING*/` | `think { label: "规划" }` | ReAct 规划步骤 |
| assistant + ReAct `/*REASONING*/` | `think { label: "推理" }` | ReAct 推理步骤 |
| assistant + ReAct `/*REPLANNING*/` | `think { label: "重规划" }` | ReAct 重规划步骤 |
| assistant + ReAct `/*ACTION*/` | 不生成 | 工具由 tool 消息处理 |
| assistant + ReAct `/*FINAL_ANSWER*/` | `say { isFinal: true }` | 最终回复 |
| assistant + `content_markdown` 非空（非 ReAct） | `say { isFinal: true }` | 普通回复 |
| assistant + `content_markdown` 非空（多轮中间） | `say { isFinal: false }` | 中间回复 |
| tool 消息 | `act` | 工具调用 |
| team member 消息 | `delegate` → 子 `AgentWorkProcess` | 子 Agent 工作 |
| Ralph Loop 迭代 | 一组 `think → act → say` | 每次迭代独立 |

**去重规则**（唯一需要的去重）：
- 如果 `say` 的内容与紧邻的前一个 `think` 内容完全相同，跳过该 `say`
- 这只发生在非 ReAct 模式下 LLM 只输出了 reasoning 没有 content 的情况

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

Team 模式下 `delegate` Activity 的 `subAgent` 结构扩展为统一面板：

```typescript
/** Team 模式的 AgentWorkProcess 扩展 */
interface TeamWorkProcess extends AgentWorkProcess {
  /** 统一面板分区 */
  panel: {
    /** 任务拆解 */
    taskBoard: TaskBoardSection;
    /** 依赖关系 DAG */
    dag?: DagSection;
    /** 团队进度 */
    teamProgress: TeamProgressSection[];
  };
}

interface TaskBoardSection {
  entries: Array<{
    id: string;
    num: number;
    task: string;
    agentName: string | null;
    status: 'pending' | 'running' | 'completed' | 'failed';
  }>;
}

interface DagSection {
  nodes: Array<{
    id: string;
    label: string;
    status: 'done' | 'running' | 'pending' | 'failed';
  }>;
  edges: Array<{ from: string; to: string }>;
}

interface TeamProgressSection {
  teamId: string;
  teamName: string;
  teamIcon: string;
  status: 'running' | 'completed' | 'failed' | 'interrupted';
  progressPercent: number;
  durationMs: number | null;
  agents: AgentProgress[];
  /** 中断时的操作 */
  actions?: ('resume' | 'cancel')[];
}

interface AgentProgress {
  agentKey: string;
  agentName: string;
  agentIcon: string;
  status: 'running' | 'completed' | 'failed' | 'waiting';
  /** Agent 内部活动（thinking line / tool line / reply line） */
  details: AgentDetailLine[];
}

type AgentDetailLine =
  | { kind: 'thinking'; content: string; streaming?: boolean }
  | { kind: 'tool'; toolName: string; args: string; status: string; durationMs: number | null }
  | { kind: 'reply'; content: string };
```

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
| `panel-section` (DAG) | `DagSection.vue` | 节点+箭头流程图 |
| `panel-section` (团队进度) | `TeamProgressSection.vue` | 可折叠团队卡片 |
| `team-prog` | `TeamProgressCard.vue` | 单个团队进度卡片 |
| `team-prog-header` | 卡片头部 | 头像+名称+状态+进度条+时长 |
| `team-prog-body` | 卡片内容 | Agent 详情列表 |
| `agent-detail` | `AgentDetail.vue` | Agent 头部 + thinking/tool/reply 行 |
| `thinking-line` | `AgentThinkingLine.vue` | 斜体+左边框+光标 |
| `tool-line` | `AgentToolLine.vue` | 图标+工具名+参数+状态+时长 |
| `reply-line` | `AgentReplyLine.vue` | 正文内容 |
| `progress-bar` | 微型进度条 | 2px 高度，颜色随状态变化 |
| `pulse-dot` | 脉冲点 | 运行中状态指示 |
| `tp-action-btn` | 操作按钮 | 中断时显示恢复/取消 |

### 5.5 Team 面板折叠策略

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
├── ConversationTurn (每个 Turn)
│   ├── UserMessageBubble (用户消息气泡)
│   └── AgentWorkPanel (Agent 工作面板)
│       ├── AgentHeader (头像+名称+状态+时长)
│       ├── ActivityTimeline (活动时间线 — 单 Agent)
│       │   ├── ThinkActivity (思考节点)
│       │   ├── ActActivity (工具节点)
│       │   ├── SayActivity (回复节点)
│       │   └── DelegateActivity (委派节点 — 非 Team)
│       │       └── AgentWorkPanel (递归)
│       └── TeamPanel (统一面板 — Team 模式)
│           ├── TaskBoardSection (任务拆解)
│           ├── DagSection (依赖关系)
│           └── TeamProgressSection (团队进度)
│               └── TeamProgressCard[] (每个团队)
│                   └── AgentDetail[] (每个 Agent)
│                       ├── AgentThinkingLine
│                       ├── AgentToolLine
│                       └── AgentReplyLine
```

### 6.2 组件职责

| 组件 | 职责 | 交互 |
|------|------|------|
| `ConversationTurn` | 一个完整 Turn 的容器 | 折叠/展开整个 Turn |
| `AgentWorkPanel` | Agent 工作过程面板 | 折叠/展开工作详情 |
| `ActivityTimeline` | 活动时间线，按序渲染 Activities | — |
| `ThinkActivity` | 思考节点，可折叠 | 点击展开/折叠 |
| `ActActivity` | 工具节点，显示摘要 | 点击展开参数/结果 |
| `SayActivity` | 回复节点，始终可见 | 流式时显示脉冲 |
| `DelegateActivity` | 子 Agent 委派 | 递归渲染子面板 |
| `TeamPanel` | Team 统一面板 | Section 级折叠 |
| `TaskBoardSection` | 任务拆解列表 | 折叠/展开 |
| `DagSection` | 依赖关系图 | 折叠/展开 |
| `TeamProgressSection` | 团队进度列表 | 折叠/展开 |
| `TeamProgressCard` | 单个团队卡片 | 折叠/展开 + 操作按钮 |
| `AgentDetail` | Agent 详情 | — |
| `AgentThinkingLine` | 思考行 | 流式光标 |
| `AgentToolLine` | 工具行 | 点击展开 |
| `AgentReplyLine` | 回复行 | — |

### 6.3 折叠策略

| 状态 | Think | Act | Say |
|------|-------|-----|-----|
| 流式中 | 展开显示光标 | 显示 spinner | 展开显示光标 |
| 刚完成 | 展开短暂停留 | 显示结果摘要 | 始终展开 |
| 完成后 500ms | 自动折叠 | 自动折叠 | 始终展开 |
| 用户手动展开 | 保持展开 | 保持展开 | — |

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
| `ConversationTurn` | `TurnBlockGroup` + `AgentBlock` | 合并，去掉 `assistant: Message \| null` 单槽限制 |
| `AgentWorkProcess` | `AgentBlock` | 基本不变，`timeline` 改名 `activities` |
| `Activity` | `TimelineEntry` | 基本不变，`kind` 值调整 |
| `think` | `TimelineEntry.kind === 'thinking'` | 改名 + 增加 `label` 字段 |
| `act` | `TimelineEntry.kind === 'tool'` | 改名 |
| `say` | `TimelineEntry.kind === 'reply'` | 改名 + 增加 `isFinal` 字段 |
| `delegate` | `TimelineEntry.kind === 'subagent'` | 改名，Team 模式走 `TeamPanel` |
| `TeamWorkProcess` | 无 | 新增，统一面板数据模型 |
| `TaskBoardSection` | `OrchestrationPlan` | 扩展，增加编号和 Agent 分配 |
| `TeamProgressSection` | `AgentBlock` (子 Agent) | 重构为面板卡片模型 |

### 8.2 逻辑层

| 新 Composable | 现有实现 | 改动 |
|--------------|---------|------|
| `useConversationTimeline` | `useAgentBlocks` | 重构，输出 `ConversationTurn[]` |
| `useTeamPanel` | 无 | 新增，构建 `TeamWorkProcess` |
| — | `useChatTimeline` | 简化，去掉 TurnBlock/MessageRow 分支 |
| — | `groupMessagesByTurn` | 删除，被 `useConversationTimeline` 替代 |
| — | `compactTimeline` | 删除，被 `ActivityTimeline` 替代 |

### 8.3 渲染层

| 新组件 | 现有组件 | 改动 |
|--------|---------|------|
| `ConversationTurn` | `TurnBlock` + `AgentBlock` (根) | 合并 |
| `AgentWorkPanel` | `AgentBlock` | 重构 |
| `ActivityTimeline` | `AgentBlock` 的 `.agent-timeline` | 提取为独立组件 |
| `ThinkActivity` | `AgentThinkingSection` | 改名 + label 支持 |
| `ActActivity` | `AgentToolSection` | 改名 |
| `SayActivity` | `AgentBlock` 内联 `.section--reply` | 提取为独立组件 |
| `DelegateActivity` | `AgentBlock` 内联 `.sub-agent-timeline-entry` | 提取为独立组件 |
| `TeamPanel` | 无 | 新增，v7 风格统一面板 |
| `TaskBoardSection` | `PlanCard` | 重构为 v7 风格任务行 |
| `DagSection` | 无 | 新增 |
| `TeamProgressSection` | 无 | 新增 |
| `TeamProgressCard` | 无 | 新增 |
| `AgentDetail` | 无 | 新增 |
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

1. 定义 `ConversationTurn` / `Activity` / `TeamWorkProcess` 类型
2. 重构 `useAgentBlocks` → `useConversationTimeline`
3. Activity 构建规则实现（think/act/say/delegate）

### Phase 2：单 Agent 渲染

1. 实现 `ConversationTurn` + `AgentWorkPanel` + `ActivityTimeline`
2. 实现 `ThinkActivity` / `ActActivity` / `SayActivity`
3. 替换 `ChatMessageList.vue` 为统一渲染路径
4. 删除 `TurnBlock` / `CompactTimeline` / `ChatMessageRow`

### Phase 3：Team 面板

1. 实现 `TeamPanel` + `TaskBoardSection` + `DagSection`
2. 实现 `TeamProgressSection` + `TeamProgressCard` + `AgentDetail`
3. 实现 `AgentThinkingLine` / `AgentToolLine` / `AgentReplyLine`
4. 适配 v7 原型的视觉风格（统一面板、进度条、脉冲点）

### Phase 4：流式体验

1. Activity 级流式状态管理
2. Think 自动折叠动画
3. Say 流式光标
4. 进度占位（execution_progress 融入 Activity）

### Phase 5：清理

1. 删除废弃文件
2. 更新测试
3. 性能优化（虚拟滚动适配）

---

## 十、风险与缓解

| 风险 | 缓解 |
|------|------|
| 重构范围大，可能引入回归 | Phase 分步实施，每步可独立验证 |
| 虚拟滚动与 Activity 模型不兼容 | Phase 5 专项处理，Activity 高度可预测 |
| v7 原型风格与项目 UX 规范冲突 | Team 面板内部用 v7 风格，外层遵守项目 CSS 变量 |
| Ralph Loop 迭代展示需后端配合 | 前端可基于消息时间间隔推断迭代边界 |
| A2UI 模式未覆盖 | A2UI 作为 `say` 的特殊变体，后续迭代处理 |
