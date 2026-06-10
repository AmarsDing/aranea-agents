# M59: Chat UI 优化（精灵模式 + 任务看板树形嵌套 + 可观测性 + useAgentBlocks 业务逻辑修复） — 需求规格

> **版本**：2026-06-10 | **状态**：Implemented（P0/P0.5/OBS/M60 全阶段 + P1.5 + P1.6 规划中 + M69 修复全部完成）
> **合并来源**：原 M59（精灵模式 + 可观测性 + 并行编排）+ M69（时间线展示 + 团队列表修复 + useAgentBlocks 业务逻辑审查）
> **技术设计**：[59-chat-ui-optimization.design.md](./59-chat-ui-optimization.design.md)
> **开发计划**：[59-chat-ui-optimization.development.md](./59-chat-ui-optimization.development.md)
> **读者**：产品、全栈开发、运维

---

## 1. 背景与问题

Chat 页面当前面临**展示层 + 编排层 + 状态机层**三类问题，原 M59 解决了编排层（精灵/团队/编排），M69 解决了展示层（时间线/团队列表）和状态机层（useAgentBlocks 业务逻辑）。

| # | 问题 | 用户影响 | 严重度 | 来源 |
|---|------|----------|--------|------|
| P0-1 | 左侧面板团队列表无数据（`loadSpiritTeams` 从未被调用） | 用户看不到任何进行中/已完成的团队 | 致命 | M69 |
| P0-2 | WS 重连后团队数据丢失 | 网络波动后团队列表清空 | 致命 | M69 |
| P1-1 | 用户需在数十个 Agent/Team 中选择对话对象 | 认知成本高，选择困难 | 高 | M59 |
| P1-2 | Agent 和 Team 平铺展示，无层级关系 | 无法体现"管家调度→团队执行"的业务逻辑 | 高 | M59 |
| P1-3 | 消息展示不按"任务-思考-工具-回复"的树形嵌套时间线脉络 | 用户无法清晰理解 Agent 执行进展 | 高 | M69 |
| P1-4 | Team 会话禁用 TurnBlock 模式 | Team 执行过程缺乏结构化展示 | 高 | M69 |
| P2-1 | 同一精灵 Session 只能有一个活跃团队 | 无法并行推进多个任务 | 高 | M59 |
| P2-2 | 工具调用卡片全部展开，长对话时占据大量空间 | 用户需大量滚动才能找到当前活跃内容 | 中 | M59 |
| P2-3 | 精灵思考/执行时显示通用 spinner | 用户不知道 Agent 在做什么，等待体验差 | 中 | M59 |
| P2-4 | TaskExecutionPanel 未集成 TeamProgressCard / SynthesisResultCard | 团队执行面板信息不完整 | 中 | M69 |
| P2-5 | 左侧面板仍显示完整 Agent 列表 | 违反精灵为唯一入口的设计原则 | 中 | M69 |
| P3-1 | 子代理工具未完成时状态被误判为 `completed` | 用户被状态徽章误导错过工具失败 | 中 | M69 |
| P3-2 | `tool_blocked` 被合并入 `running` | 用户无法区分"系统在等"与"等用户确认" | 中 | M69 |
| P3-3 | PlanCard 在 subagents_spawn 路径下永远停在"规划中" | 用户无法感知子代理真实执行进展 | 中 | M69 |
| P3-4 | 已完成回合默认折叠 | 用户翻看历史时找不到最终答案 | 中 | M69 |
| P3-5 | 部分工具失败被掩盖为 `completed` | 用户不知道结果好但中途有工具失败 | 低 | M69 |
| P4-1 | `todo_write` 多次调用 stuck 失败 | 代办管理工具不可用 | 中 | M59 |
| P4-2 | 长对话中工具调用堆叠难追踪 | 用户无法快速定位"先 A 后 B 再 C"顺序 | 中 | M59 |

**核心原则**：
1. **用户不需要知道有哪些 Agent、如何编排**——只需与精灵对话，精灵负责调度一切
2. **可观测性强，但不影响主要内容的显示**——环境可观测性优先，完成即折叠
3. **任务看板树形嵌套**——任何 agent 的对话输出形式统一为"任务-思考-工具-回复-思考-回复"，可递归嵌入子任务看板
4. **完成即展开**——已完成回合默认展开，让用户直达最终答案

---

## 2. 核心展示模型：任务看板树形嵌套

### 2.1 单一 agent 输出形式

**任何 agent 的对话看板内容统一为**：

```
[任务] → [思考] → [工具] → [回复] → [思考] → [回复]
```

每个元素是"任务看板"中的一个节点，可折叠可展开。

### 2.2 多 agent 嵌套结构

**看板中可以嵌入子任务看板**，所有任务看板的输出形式相同；主面板是**树形结构嵌套子任务面板**。

### 2.3 典型场景

**场景 A：单任务，精灵自己完成**

```
[用户指令]
    ↓
[任务：分析问题]
    ↓
[思考：理解需求...]
    ↓
[任务：搜索资料]
    ↓
[思考：准备搜索关键词...]
    ↓
[工具：web_search("关键词")]
    ↓
[思考：基于结果整理答案...]
    ↓
[回复：最终答案]
    ↓
[思考：补充注意事项...]
    ↓
[回复：补充说明]
```

**场景 B：多任务，精灵多 agent 协作**

```
[用户指令：开发完整功能]
    ↓
[思考：需求拆解...]
    ↓
[任务：任务分析]
    ↓
[任务看板 A：后端开发]
    ├─ 任务：设计 API
    ├─ 思考：分析需求...
    ├─ 任务：实现代码
    │     └─ 任务看板 A.1：API 实现
    │           ├─ 任务：编写代码
    │           ├─ 思考：...
    │           ├─ 工具：file_write
    │           └─ 回复：实现完成
    ├─ 任务：写测试
    └─ 回复：后端完成
    ↓
[任务看板 B：前端开发]
    ├─ 任务：实现 UI
    ├─ ...
    └─ 回复：前端完成
    ↓
[任务看板 C：测试团队]（并行）
    ├─ 任务：编写测试用例
    ├─ ...
    └─ 回复：测试通过
    ↓
[任务看板状态：3/3 完成]
    ↓
[思考：汇总结论...]
    ↓
[回复：综合结果汇报]
```

### 2.4 节点类型

| 节点类型 | 图标 | 默认状态 | 说明 |
|---------|------|---------|------|
| `task` | 📋 | 不折叠 | 任务描述（用户/Agent 视角） |
| `thinking` | 🧠 | 完成后折叠 | reasoning 内容 |
| `action`（工具） | ⚡ | 完成后折叠 | 工具调用（工具名+状态+耗时） |
| `reply` | 💬 | 不折叠 | Agent 回复（含最终答案） |
| `sub_task_board` | 🗂️ | 展开 | 子任务看板（可嵌套递归） |
| `end` | ✅ | 不折叠 | 任务完成标记 |
| `error` | ❌ | 不折叠 | 错误信息 |

### 2.5 状态指示

| 状态 | 颜色 | 图标 | 说明 |
|------|------|------|------|
| `running` | 蓝色 | ⚡ | 活跃中，脉冲动画 |
| `tool_blocked` | 橙色 | ⏸ | 等待用户输入 |
| `tool_running` | 蓝色 | ⚡ | 工具执行中 |
| `completed` | 绿色 | ✓ | 已完成（默认展开） |
| `partial_failure` | 橙色 | ⚠️ | 有工具失败但最终结果成功 |
| `failed` | 红色 | ✗ | 失败 |
| `stuck`（stuck 工具） | 红色 | error_outline | 工具无返回结果 |
| `cancelled` | 灰色 | ⊘ | 已取消 |

---

## 3. 用户故事

### US-01 精灵为唯一对话入口

**作为** 用户
**我希望** Chat 页面左侧只显示精灵（大管家），不再显示所有 Agent 和 Team 列表
**以便** 我不需要理解系统中有哪些 Agent，只需与精灵对话即可

**验收**：
- 左侧列表仅显示精灵入口（`__spirit__`），不显示其他 Agent 和 Team
- 精灵始终置顶，不可拖拽排序、不可删除
- 精灵入口下方动态展示当前活跃的任务团队列表（见 US-03）
- 精灵的 Session 列表在右侧 Session 侧栏中展示

### US-02 精灵区分简单对话与任务型对话

**验收**：
- 简单对话（闲聊、知识问答、简单查询）：精灵直接回复，不组建团队
- 任务型对话（开发、分析、创作等复杂任务）：精灵调用 `plan_and_execute` 工具，经三阶段编排（Plan → Allocate → Orchestrate）组建团队执行
- 精灵在组建团队时，向用户展示任务分析结果（任务类型、所需角色、预估步骤）
- 组建过程对用户可见：精灵回复中包含"正在组建团队…"的执行卡片
- 用户可在精灵对话中下达多个任务指令，每个任务独立组建团队

### US-03 任务团队在精灵下方展示

**验收**：
- 每个任务团队在精灵下方显示为一个卡片条目，包含：
  - **团队名称**：精灵根据任务自动生成
  - **任务摘要**：一句话描述团队正在执行的任务
  - **运行状态**：`pending` / `running` / `completed` / `failed` / `cancelled` / `interrupted` / `archived`
  - **成员头像组**：最多显示 4 个成员头像，超出显示 +N
  - **编排模式标签**：sequential / parallel / hybrid / coordinator
  - **进度指示**：已完成步骤 / 总步骤
  - **DAG 依赖提示**：等待前置任务时显示依赖数量
  - **Agent 状态标签**：每个成员显示状态标签
- 团队按**活跃度排序**：running → pending → interrupted → completed → failed → cancelled
- 同状态的团队按创建时间倒序
- 已完成的团队默认折叠到"已完成"分组，可展开查看
- 团队状态变化时卡片短暂脉冲高亮

### US-04 点击团队查看任务执行面板

**验收**：
- 点击团队卡片 → 中间区域切换为该团队的**任务执行面板**
- 任务执行面板布局：

```
+----------------------------------------------------------+
| [← 返回精灵]  团队名称    状态 Badge    编排模式标签      |
+----------------------------------------------------------+
|  ┌─ 并行团队概览 ────────────────────────────────────┐   |
|  │ 进行中：2  已完成：1  并行配额：2/3               │   |
|  │ ┌─ DAG 依赖图 ──────────────────────────────┐     │   |
|  │ │ ▶ 任务A → ⏳ 任务B(依赖A) → ⏳ 任务C(依赖B)│     │   |
|  │ └────────────────────────────────────────────┘     │   |
|  └───────────────────────────────────────────────────┘   |
|  ┌─ 团队进度卡片 ────────────────────────────────────┐   |
|  │ 后端 API 开发团队    ⚡ running    2/5 步骤        │   |
|  │ ████████░░░░░░░░░░░░ 40%    耗时 1m 20s           │   |
|  │ 👤 Golang 工程师 · 代码审查员 · 测试工程师 +1     │   |
|  └───────────────────────────────────────────────────┘   |
|  ┌─ 中断恢复提示（条件显示）──────────────────────────┐   |
|  │ ⏸ 团队已中断 — 因服务器重启而中断                  │   |
|  │ 已完成 3/5 步骤  [恢复执行] [取消团队]             │   |
|  └───────────────────────────────────────────────────┘   |
|  ┌─ 综合结果 ────────────────────────────────────────┐   |
|  │ 📋 混合合成    100% 成功    耗时 3m 20s            │   |
|  │ 各团队结果摘要...                                  │   |
|  └───────────────────────────────────────────────────┘   |
|  ┌─ 执行看板（任务-思考-工具-回复 树形嵌套）─────────┐   |
|  │ 📋 任务 A1                                       │   |
|  │   🧠 思考：分析需求...                            │   |
|  │   ⚡ 工具：read_file("main.go")                   │   |
|  │   💬 回复：已读取文件                             │   |
|  │ 📋 任务 A2                                       │   |
|  │   🧠 思考：基于文件内容...                        │   │
|  │   🗂️ 子任务看板：代码实现                         │   |
|  │     📋 任务：编写代码                              │   |
|  │     ⚡ 工具：file_write                            │   │
|  │     💬 回复：代码已写入                            │   |
|  │ ✅ 完成                                           │   |
|  └───────────────────────────────────────────────────┘   |
+----------------------------------------------------------+
```

- **并行团队概览区**（`ParallelTeamOverview`）：展示多团队并行状态、并行配额、DAG 依赖图
- **团队进度卡片**（`TeamProgressCard`）：每个团队独立展示进度、状态、成员
- **执行看板**（`TaskKanbanBoard`）：**任务-思考-工具-回复 树形嵌套**展示
- **中断恢复提示**（`InterruptedTeamCard`）：interrupted 状态时显示恢复/取消按钮
- **综合结果区**（`SynthesisResultCard`）：所有团队完成后展示合成结果

### US-05 任务看板树形嵌套展示（核心展示模型）

**作为** 用户
**我希望** 任何 agent 的对话输出按**任务-思考-工具-回复**的统一结构展示，且支持子任务看板递归嵌套
**以便** 我能以一致的心智模型理解所有 agent 的工作，且能下钻到子 agent 内部查看细节

**验收**：
- **统一结构**：所有 agent（包括根精灵、子 agent）的对话输出都按"任务-思考-工具-回复-思考-回复"序列展示
- **节点类型**：
  - `task`（任务）：用户/Agent 视角的任务描述
  - `thinking`（思考）：reasoning 内容
  - `action`（工具）：工具调用（折叠态显示工具名+状态+耗时，展开显示参数和结果）
  - `reply`（回复）：Agent 回复（含最终答案）
  - `sub_task_board`（子任务看板）：可嵌套递归
  - `end`（完成标记）
  - `error`（错误）
- **递归嵌套**：
  - 主面板 = 根 agent 的任务看板
  - 看板中可嵌入子 agent 的子任务看板（sub_task_board 节点）
  - 嵌套层级无硬性限制（实际由 Session 树深度 MaxSessionDepth=2 约束）
  - 嵌套的子看板与父看板使用相同的节点类型和展示规则
- **折叠策略**：
  - `thinking` 和 `action` 完成后默认折叠，展开显示完整内容
  - `task`、`reply`、`end` 始终展开
  - 已完成回合（`status === 'completed'`）的看板**默认展开**，让用户直达最终答案
  - 全局"展开全部/折叠全部"按钮作用于所有层级
- **多场景适配**：
  - 单任务：用户指令 → 思考 → 任务 → 思考 → 工具 → 回复 → 思考 → 回复
  - 多任务：用户指令 → 思考 → 拆解任务 → 任务看板 A → 任务看板 B → 任务看板 C → 任务看板状态 → 汇总结论
  - 子 agent：父看板中嵌入 sub_task_board 节点，下钻可查看子 agent 内部细节

### US-06 展开团队查看成员树形列表

**验收**：
- 团队卡片支持展开/折叠（点击展开箭头或双击卡片）
- 展开后显示树形成员列表，每个成员显示：
  - **名称**：Agent display_name
  - **角色标签**：worker / synthesizer / generator / critic 等
  - **工作状态**：通过 `AgentStatusLabel` 展示

### US-07 点击成员查看只读对话输出

**验收**：
- 点击成员 → 中间区域切换为该成员的**只读对话输出面板**
- 面板内容：顶部成员名称+角色+返回按钮、消息流、工具调用卡片、执行统计
- **只读模式**：输入面板隐藏，消息不可编辑/不可回复
- 消息按堆栈模型分组（`groupMessagesByTurn`）

### US-08 多任务并行与 Agent 复用

**验收**：
- 用户可在精灵对话中连续下达多个任务指令
- 每个任务独立组建团队，左侧列表同时展示多个团队卡片
- 同一 Agent 可出现在多个团队中，拥有独立的 Session、Runner、状态
- **并行配额**：`ParallelConfig` 控制最大并行团队数（默认 3）和单团队最大并发数（默认 2）
- **DAG 编排**：复杂任务可分解为多个子任务，形成 DAG 依赖图，按拓扑顺序执行
- **条件路由**：编排管家可根据 Agent 输出动态路由到不同下游 Agent（`ConditionalBranch`）

### US-09 团队生命周期管理

**验收**：
- **团队状态流转**：

```
pending → running → completed
                  → failed
                  → interrupted → running（恢复）
                  → cancelled
         → cancelled
```

- **任务完成后**：团队状态自动变为 `completed`，卡片移入"已完成"折叠分组
- **自动归档**：超过 `ParallelConfig.AutoArchiveSeconds`（默认 3600s）后自动归档
- **失败团队**：显示错误信息和失败步骤，提供"取消"、"重试"、"归档"按钮
- **精灵可主动汇报**：团队完成后，精灵在对话中主动通知用户任务结果

### US-10 返回精灵对话

**验收**：
- 任务执行面板和成员只读面板顶部均有"← 返回精灵"按钮
- 点击后切换回精灵的聊天面板
- 左侧列表点击精灵入口也切换回精灵对话
- 切换不丢失当前团队的 WS 连接和实时状态
- 面包屑导航：精灵 > 团队名称 > 成员名称

### US-11 对话流自动折叠（OBS-01）

**作为** 用户
**我希望** 已完成的步骤/工具调用自动折叠为单行摘要，仅当前活跃步骤展开
**以便** 我能快速定位当前工作，不被历史步骤干扰

**验收**：
- 已完成的工具调用卡片自动折叠为单行摘要（工具名 + 状态图标 + 耗时）
- 已完成的团队组建/完成卡片自动折叠为单行摘要
- 当前活跃步骤始终展开显示
- 精灵直接回复的消息不折叠
- 新消息到达时，前一个活跃步骤自动折叠
- 折叠态点击可展开查看完整内容
- 提供"展开全部"按钮
- interrupted 状态的步骤视为"已完成"并折叠，但显示中断标记（⏸）而非 ✓
- **例外**：已完成回合（`status === 'completed'`）的 `collapsed` 默认为 `false`（**展开**），让用户直达最终答案

### US-12 语境加载消息（OBS-02）

**作为** 用户
**我希望** 精灵思考/执行时显示与当前动作相关的描述性文案，而非通用 spinner
**以便** 我在等待时就知道 Agent 在做什么

**验收**：
- 三阶段编排过程中，按事件顺序展示语境加载消息：
  - `butler.orchestration.started` → "正在处理任务…"
  - `spirit_plan_created` → "正在分析任务复杂度…"
  - `spirit_allocation_created` → "正在分配 Agent 角色…"
  - `spirit_orchestration_started` → "正在编排执行流程…"
- 团队执行过程中，按 `tool_call`/`tool_result` 事件展示 Agent 级语境消息
- 语境加载消息以流式方式替换（非追加），保持 1 行
- 消息样式：浅色背景 + 左侧彩色竖线
- WS 重连事件回放期间，语境加载消息静默

### US-13 Agent 状态标签（OBS-03）

**作为** 用户
**我希望** 团队卡片和任务执行面板中每个 Agent 显示状态标签
**以便** 我能一眼感知团队执行进度

**验收**：
- 后端 `AgentNodeStatus` 17 种状态聚合为 7 种展示标签：

| 聚合标签 | 文案 | 颜色 | 图标 | 动画 |
|---------|------|------|------|------|
| Queued | "排队中" | 灰色 | ○ | 无 |
| Active | "执行中" | 蓝色 | ⚡ | 左边框呼吸动画 |
| Suspended | "等待中" | 橙色 | ⏸ | 无 |
| Done | "已完成" | 绿色 | ✓ | 无 |
| Failed | "失败" | 红色 | ✗ | 无 |
| Skipped | "已跳过" | 灰色 | ⊘ | 无 |
| Cancelled | "已取消" | 灰色 | ⊘ | 无 |

- 侧边栏团队卡片使用 `SpiritMember.status`（简单 3 值：idle/running/error）
- 任务执行面板使用 `AgentNodeStatus`（17 值聚合为 7 种标签）
- **新增 `tool_blocked` 显式状态**：UI 显示"🟡 等待您的输入"徽章（来自 M69 useAgentBlocks 修复）

### US-14 底部状态栏（OBS-04）

**验收**：
- 聊天面板底部固定一行状态栏（`SpiritStatusBar`），包含：
  - 复杂度级别：简单/中等/复杂 + tooltip 显示策略理由
  - 活跃团队数："⚡ N 运行中"
  - 中断团队数："⏸ N 已中断"
  - 编排检查点：当前步骤名称
  - 并行配额："📊 N/M 配额"
  - Token 消耗："🔵 X.Xk Token"
  - 最近事件："✅/❌ 最近完成的团队"
- 状态栏不随内容滚动
- 窄屏时自动隐藏低优先级字段
- 仅在精灵模式激活时显示

### US-15 侧边栏状态脉冲（OBS-05）

**验收**：
- 团队状态变化时卡片短暂脉冲高亮：

| 状态变化 | 脉冲颜色 | 持续时间 |
|---------|---------|---------|
| → running | 蓝色 | 1.0s |
| → completed | 绿色 | 1.5s |
| → failed | 红色 | 2.0s |
| → interrupted | 橙色 | 1.5s |

- WS 回放期间禁用脉冲动画

### US-16 可折叠工具输出（OBS-06）

**验收**：
- 工具调用卡片（`ChatExecutionCard`）完成后自动折叠
- 折叠态显示：工具名 + 状态图标 + 耗时（1 行，~32px）
- running 状态的工具调用始终展开
- failed 状态的工具调用折叠但红色高亮
- 加载历史消息时，已完成的工具卡片默认折叠

### US-17 ChatExecutionCard 独立折叠增强（OBS-08）

**验收**：
- 工具运行 ≥5s 时显示实时计时器（`5s` → `1m 12s`），≥60s 变为警告色
- `started_at` 为空时降级：`occurred_at` → `Date.now()`，始终启动计时器
- 折叠态摘要兜底：后端未提供 `summary` 时，前端根据 `tool_name` + `arguments` 生成摘要（如"修改 auth.go"、"搜索 TODO"）
- ToolStrip 折叠态显示工具类型分布（如"3 file_read · 2.5s"而非"3 tools · 2.5s"）
- 全局"展开全部/折叠全部"按钮同时作用于 TurnBlock 和 ChatExecutionCard 两层
- 运行中的工具不受"折叠全部"影响
- Spirit 模式（TaskExecutionPanel）中的 ChatExecutionCard 同样响应全局控制
- `ToolUseEvent.expanded` 死代码字段清理

### US-18 中断恢复提示（OBS-07）

**验收**：
- interrupted 状态的团队在任务执行面板中显示恢复提示卡片（`InterruptedTeamCard`）
- 如果团队无 `graph_execution_id`，显示"此团队不支持断点恢复"

### US-19 TODO 任务看板（TK-01）

**作为** 用户
**我希望** 精灵执行 `todo_write` 后，对话面板中以**三列看板**形式展示当前会话的 todo 列表
**以便** 我能一眼看到每个任务的标题、所处状态、活跃进度，而不必在工具调用卡片里挖 JSON

**验收**：
- 数据源：`todo_write` 工具调用的**最后一次成功结果**，以及会话级 session state `temp:todos[:<branch>]`
- 看板位置：`ChatMessagePanel` 顶部，`SpiritStatusBar` 上方，**折叠态**默认只显示 1 行摘要
- 三列固定：`Pending` / `In Progress` / `Completed`，列头显示计数
- 卡片内容：任务正文 `content`、执行态描述 `activeForm`、状态图标 + 颜色
- 任务变更时整列高亮脉冲 0.8s
- 空状态：未调用过 `todo_write` 时看板隐藏
- 精灵 / 团队 / 成员三种面板模式都显示

### US-20 工具调用按时间线展示（TK-02）

**作为** 用户
**我希望** 一个 turn 内的多次工具调用按**发生时间**排列成纵向时间线
**以便** 我能看到"先 A 后 B 再 C"的真实执行顺序

**验收**：
- 替换规则：`turn.tools.length >= 2` 时使用 `ToolCallTimeline`；单工具调用保持卡片不拆分
- 节点元素：左侧时间轴（`TimelineNode`）含 `HH:mm:ss` 时间戳 + 状态点；右侧节点体含工具名 + args 摘要 + result/error 摘要 + 耗时
- 排序：按 `ToolUseEvent.occurred_at` 升序；同 ms 事件按 `id` 字典序
- 节点状态：`running` / `success` / `failed` / `cancelled` / `blocked` / `stuck`
- "展开全部/折叠全部"按钮作用于时间线
- 团队组建 / 团队完成 / 中断恢复等"非工具事件"卡片不纳入时间线

### US-21 Stuck 工具调用可观测化（TK-03）

**作为** 用户
**我希望** 当工具调用变成 stuck（`error_code=tool_timeout`）时，时间线节点显示明确提示
**以便** 我能区分"工具真的失败了"和"工具没有返回结果"

**验收**：
- 当 `ToolUseEvent.error_code === 'tool_timeout'` 时，时间线节点右侧显示错误图标 + 红色文案
- `TaskExecutionPanel` 顶部如果**有**任意 stuck 工具，则在状态栏旁显示橙色"⚠ 1 工具未返回"徽章
- 后端 `stuckToolResultReason` 文案暴露为前端可识别的 i18n key
- 范围：仅在 Spirit 模式 + 工具时间线启用时生效

### US-22 工具显示开关（TK-04）

**作为** 用户（特别是生产环境部署方）
**我希望** 在 ChatPanel 顶部可一键切换"是否显示工具调用"，发布到生产时关闭让对话面板更聚焦
**以便** 测试阶段我能完整看到工具执行过程，正式发布时让用户只看到思考和回复

**验收**：
- **入口位置**：`ChatMessagePanel.vue` 顶部操作栏（与"展开全部/折叠全部"按钮同行）一个 `<q-btn>` 切换按钮，图标 `build_circle` / `build_circle_outlined`
- **状态持久化**：开关状态写入 `localStorage.chat.ui.showToolCalls`，默认 `true`
- **关闭时降级**：
  - `ChatExecutionCard` / `ToolCallTimeline` / `TodoKanbanBoard` 整组不挂载
  - `SpiritStatusBar` 不再显示 `toolCount` 字段
  - 任务看板 TK-01 仍然显示（业务必需，工具调用是 todo 的载体）
  - 思考节点保留显示（用户语义理解必需）
  - 工具调用前后的纯文本回复不受影响
- **开启时**：完全等同 TK-01/TK-02 行为
- **i18n 键**：`chat.uiConfig.showToolCalls` / `chat.uiConfig.hideToolCallsTooltip`
- **后端协议无关**：纯前端控制，不涉及 Envelope 过滤（按 §6.5.4 决策走"前端 store + 路由配置"路径）

### US-23 代码块自动识别语言与高亮（TK-05）

**作为** 用户
**我希望** 精灵/Agent 回复中嵌入的代码块能**自动识别编程语言**并**高亮显示**
**以便** 我能快速阅读代码、识别语言类型、复制可执行片段，且视觉上不喧宾夺主

**验收**：
- **技术选型**：`markdown-it`（已用）+ `highlight.js`（轻量、auto 检测、零配置）
- **自动检测策略**（优先级递减）：
  1. **fenced info string** 显式指定（` ```python `）— 100% 准确
  2. **未指定时**：`hljs.highlightAuto(text, LANG_CANDIDATES)`，候选语言限制在 12 种常用语言（ts/js/go/python/bash/json/yaml/sql/rust/java/markdown/shell）
  3. **置信度 < 0.5** 或代码长度 > 10KB：fallback 到 `plaintext`（性能保护）
- **组件**：`web/src/components/chat/CodeBlock.vue`
  - 头部一行：`{displayLang}` 标签 + `content_copy` 复制按钮（右对齐）
  - 主体：`<pre><code v-html="highlightedHtml" />` 渲染
  - 行号：可选（默认关闭，避免视觉噪声）
- **折叠策略**：
  - 代码行数 ≤ 20：默认展开
  - 代码行数 > 20：默认折叠为 `▶ 展开代码 (N 行)`，点击展开
- **视觉规范**：
  - 字体：`var(--font-family-mono)`（与项目其他代码块一致）
  - 主题：跟随 `var(--color-bg-elevated)` token，不引入新色
  - 工具栏：仅"复制"按钮（不喧宾夺主）
  - 字号：与主文本一致（`var(--font-size-base)`），不放大
- **集成位置**：`MarkdownView.vue` 中替换默认的 `<pre><code>` 为 `<CodeBlock :code :lang>`
- **范围**：作用于所有 Agent 回复（不仅精灵），包括思考节点 `kind: 'thinking'` 中的代码块
- **i18n 键**：`chat.codeBlock.copy` / `chat.codeBlock.copied` / `chat.codeBlock.expandLine` / `chat.codeBlock.collapseLine` / `chat.codeBlock.plaintext`

### US-24 思考节点"UI 不喧宾夺主"细化（TK-06，M59 P1.5 增强）

**作为** 用户
**我希望** 思考节点在流式输出和已完成两种状态下的展示都"不喧宾夺主"
**以便** 思考是辅助信息，回复才是主内容；无论思考是否完成，视觉权重都不应超过回复

**验收**：
- **流式输出中**：
  - 思考内容在**固定宽度容器**（`max-width: var(--content-max-width)`）中**实时刷新**追加
  - 不使用 modal/popover，不阻塞用户滚动
  - 当前活动思考节点有**细微脉冲边框**（`border-left: 2px solid var(--color-primary); animation: pulse 1.5s ease-in-out infinite`）
- **完成后**：
  - **自动折叠为 1 行 span**，文案：`🧠 {firstSentence}`（截取第一个句号前的内容，超过 60 字加 `…`）
  - 点击 span 展开完整 reasoning
  - 展开态保持**与回复文本相同字号**（`var(--font-size-base)`），不放大
  - 展开态背景色透明，区别于 ChatExecutionCard 的卡片样式
- **状态切换**：
  - 折叠 → 展开：CSS transition 200ms
  - 展开 → 折叠：CSS transition 200ms + 内容高度坍缩动画
- **统一性**：
  - 思考节点字体 = 主文本字体（`var(--font-family-base)`），不复用 mono
  - 行高 = 主文本行高（`var(--line-height-base)`）
  - 颜色 = `var(--color-text-secondary)`，比回复文本低一档亮度
- **范围**：适用于 `TaskBoardNode` 中所有 `kind: 'thinking'` 节点，包括嵌套子任务看板
- **例外**：当 `reasoning` 长度 < 30 字符时，直接内联显示不折叠（信息密度过低，折叠反而干扰）

---

## 4. 编排用户故事（M60 核心）

### SPO-01 多团队并行执行

**验收**：
- 精灵可在同一对话中连续调用 `plan_and_execute` 组建多个团队
- 每个团队拥有独立的 Session、Runner、状态
- 并行度可配置（默认最大 3 个并行团队），超出时精灵提示用户等待

### SPO-02 任务依赖调度

**验收**：
- 精灵在分析复杂需求时，自动识别任务间的依赖关系
- 依赖团队在前置团队完成后自动启动
- 无依赖的团队立即并行启动
- 精灵回复中展示任务依赖图（文本形式，`DAGDiagramCard`）

### SPO-03 编排模式智能选择

**验收**：
- 精灵根据任务 DAG 结构自动选择编排拓扑：parallel / sequential / hybrid / coordinator
- 选择依据包含历史 DQ Score 数据（如有）
- 精灵回复中说明选择该编排模式的理由（`OrchestrationModeBadge` tooltip）

### SPO-04 多团队结果合成

**验收**：
- 所有活跃团队完成后，精灵自动调用 Synthesis Engine 合成结果
- 合成结果包含：每个团队的任务摘要、执行状态、关键产出
- 部分团队失败时，合成结果标注失败团队及原因
- 成功率指标：100% 成功/部分成功/低成功率

### SPO-05 编排策略进化

**验收**：
- 每次团队执行完成后计算 DQ Score（三元分解：Validity×0.4 + Specificity×0.3 + Correctness×0.3）
- DQ Score > 0.7 缓存编排拓扑，相似任务优先复用
- DQ Score < 0.5 生成编排优化建议
- 进化护栏确保策略变更幅度可控

### SPO-06 任务复杂度智能评估

**验收**：
- 精灵收到用户消息后，先评估复杂度（simple/moderate/complex）
- 规则引擎优先判断（零 Token 消耗），无法判断时返回 moderate
- 复杂度评估结果在 `SpiritStatusBar` 中展示

### SPO-07 Graph DAG 编排

**验收**：
- 编排管家新增 `build_orchestration_graph` 工具，动态生成 `GraphBuildConfig`
- Graph DAG 支持并行节点、汇合节点、条件路由（`ConditionalBranch`）
- 验证节点可注入（output_format / task_completion / human_approval）

### SPO-08 编排验证门禁

**验收**：
- Graph DAG 中可注入验证节点（Verification Node）
- 验证失败时根据 FailureAction 处理：Skip / RetryThenBlock / FailFast
- HITL 验证节点使用 Graph 的 interrupt 机制
- 验证节点在 `DAGDiagramCard` 中展示

---

## 5. 功能规格

### 5.1 任务看板树形嵌套数据结构

```typescript
type TaskBoardNodeKind =
  | 'task'           // 任务
  | 'thinking'       // 思考
  | 'action'         // 工具
  | 'reply'          // 回复
  | 'sub_task_board' // 子任务看板（递归）
  | 'end'            // 结束
  | 'error'          // 错误

interface TaskBoardNode {
  kind: TaskBoardNodeKind
  id: string
  timestamp: string
  collapsed: boolean
  /** task: 任务描述 */
  content?: string
  /** thinking: reasoning 内容 */
  reasoning?: string
  /** action: 工具调用信息 */
  toolName?: string
  toolStatus?: string
  toolDuration?: number
  toolCallId?: string
  toolArguments?: string  // 展开态显示
  toolResult?: string     // 展开态显示
  /** sub_task_board: 嵌套的子看板 */
  childBoard?: AgentBlock
  /** 节点状态 */
  status?: 'running' | 'tool_running' | 'tool_blocked' | 'completed' | 'failed' | 'partial_failure' | 'cancelled'
  /** 错误信息 */
  errorMessage?: string
  /** turn 完成状态 */
  turnStatus?: string
}

interface AgentBlock {
  /** 根节点或子看板的根节点 */
  id: string
  /** Agent 标识 */
  agentKey: string
  agentName: string
  /** 当前 turn 的根看板 */
  rootBoard: TaskBoardNode[]
  /** 子 agent 子任务看板（递归结构） */
  childBoards: AgentBlock[]
  /** 整体状态 */
  status: AgentBlockStatus
  /** 是否有工具失败但最终结果成功 */
  hasPartialFailure: boolean
  /** progress section（从 timeline 拆出，渲染在 turn 头部） */
  progressSections: ProgressSection[]
  /** 兼容字段：result 不再驱动 UI 单一来源 */
  result?: string
}

type AgentBlockStatus =
  | 'running'
  | 'tool_running'
  | 'tool_blocked'
  | 'completed'
  | 'failed'
  | 'partial_failure'
  | 'cancelled'
```

### 5.2 三层可观测性架构

```
┌─────────────────────────────────────────────────────┐
│ L1 环境层 (Ambient) — 始终可见，零干扰              │
│ OBS-02 语境加载消息  OBS-03 Agent 状态标签           │
│ OBS-05 侧边栏脉冲    OBS-04 底部状态栏              │
│ SPO-06 复杂度标签    SPO-07 检查点步骤               │
├─────────────────────────────────────────────────────┤
│ L2 结构层 (Structural) — 按需查看，不遮挡            │
│ OBS-01 对话流自动折叠  OBS-06 可折叠工具输出         │
│ OBS-07 中断恢复提示    OBS-08 折叠增强              │
│ SPO-02 DAG 依赖图      SPO-03 编排模式标签          │
│ SPO-08 验证门禁节点    US-20 工具调用时间线         │
├─────────────────────────────────────────────────────┤
│ L3 证据层 (Evidential) — 主动展开才可见              │
│ ChatExecutionCard 展开态  ChatDiffViewer             │
│ TeamRunObservatory Timeline  SynthesisResultCard     │
│ TaskBoard 展开态  SubTaskBoard 嵌套下钻             │
└─────────────────────────────────────────────────────┘
```

### 5.3 设计原则

| # | 原则 | 说明 |
|---|------|------|
| DP-1 | 环境可观测性优先 | 状态信息以颜色、图标、微动画呈现，不占用主内容区空间 |
| DP-2 | 渐进式信息披露 | 默认只展示 L1，用户主动交互才展开 L2/L3 |
| DP-3 | 完成即折叠 | 已完成的步骤/团队/工具调用自动收起，保持视觉焦点在活跃内容 |
| DP-4 | 状态即视觉 | 颜色、图标、动画三位一体传达状态 |
| DP-5 | 证据后置 | 过程信息轻量展示，详细证据仅在用户主动查看时展开 |
| DP-6 | **完成即展开** | 已完成回合的看板默认展开，让用户直达最终答案 |
| DP-7 | **树形嵌套** | 子 agent 子任务看板与父看板使用相同结构，递归可下钻 |

### 5.4 中间面板状态机

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  精灵对话面板  │     │  任务执行面板  │     │ 成员只读面板   │
│              │     │              │     │              │
│ L1: 语境消息  │     │ L1: Agent标签 │     │ L1: 状态标签  │
│ L2: 自动折叠  │     │ L2: 进度卡片  │     │ L2: 工具折叠  │
│ L2: 工具折叠  │     │ L2: 恢复提示  │     │              │
│ L1: 底部状态栏│     │ L1: 底部状态栏│     │ L1: 底部状态栏│
│              │     │ L2: 任务看板   │     │ L2: 任务看板   │
└──────────────┘     └──────────────┘     └──────────────┘
```

### 5.5 并行团队管理

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `MaxConcurrentTeams` | 3 | 同一精灵 Session 最大并行团队数 |
| `MaxTeamConcurrency` | 2 | 单团队内最大并发成员数 |
| `TeamTimeout` | 10min | 单团队超时 |
| `AutoArchiveAfter` | 1h | 完成后自动归档时间 |
| `MaxSessionDepth` | 2 | Session 树最大深度（看板嵌套层级） |

### 5.6 Task DAG 模型

```
TaskNode:
  id: TaskNodeID
  task_name: string
  description: string
  depends_on: []TaskNodeID
  mode: string
  agent_keys: []string

TaskDAG:
  nodes: map[TaskNodeID]*TaskNode
  roots: []TaskNodeID
```

**拓扑路由规则**：

| DAG 特征 | 路由结果 |
|----------|---------|
| 单节点 | `sequential` |
| 所有节点无依赖 | `parallel` |
| 存在依赖但宽度 > 1 | `hybrid` |
| 依赖链深度 > 3 | `coordinator` |

### 5.7 Synthesis Engine

| 场景 | 策略 |
|------|------|
| 全部成功 | 完整合成 |
| 部分失败 | 部分合成，标注失败团队 |
| 全部失败 | 失败报告 |
| 依赖链中断 | 级联标注 |

### 5.8 精灵工具扩展

| 工具 | 功能 | 状态 |
|------|------|------|
| `plan_and_execute` | 三阶段编排（Plan→Allocate→Orchestrate） | ✅ 活跃 |
| `check_progress` | 查询编排进度 | ✅ 活跃 |
| `cancel_orchestration` | 取消编排 | ✅ 活跃 |
| `synthesize_results` | 合成已完成团队的结果 | ✅ 活跃 |
| `build_orchestration_graph` | 生成 Graph DAG | ✅ 活跃 |
| `assemble_team` | 组建团队（旧） | DEPRECATED |
| `assess_complexity` | 评估复杂度（旧） | DEPRECATED |
| `check_team_progress` | 查询进度（旧） | DEPRECATED |
| `cancel_team` | 取消团队（旧） | DEPRECATED |

### 5.9 事件驱动模型

新增 EnvelopeType（15+）：

| EnvelopeType | 触发时机 |
|-------------|---------|
| `spirit_team_assembled` | 团队组建完成 |
| `spirit_team_completed` | 团队执行完成 |
| `spirit_team_failed` | 团队执行失败 |
| `spirit_team_interrupted` | 团队执行中断 |
| `spirit_team_progress` | 团队进度更新 |
| `spirit_teams_all_completed` | 所有并行团队完成 |
| `spirit_synthesis_completed` | 合成完成 |
| `spirit_plan_created` | 任务计划创建 |
| `spirit_allocation_created` | Agent 分配完成 |
| `spirit_orchestration_started` | 编排启动 |
| `spirit_orchestration_checkpoint` | 编排检查点 |
| `spirit_orchestration_interrupted` | 编排中断 |
| `butler.orchestration.started` | Butler 编排启动 |
| `butler.orchestration.completed` | Butler 编排完成 |
| `butler.orchestration.failed` | Butler 编排失败 |

### 5.10 编排进化闭环

```
用户需求 → 精灵判断 → 组建团队（基于历史 DQ Score 选择编排模式）
    → 团队执行 → Session 执行轨迹记录
        ├──→ DQ Score 计算 → 编排策略优化
        ├──→ 工具调用模式检测 → Skill 提议
        └──→ Agent 能力画像更新 → tool_weight 调整
```

**DQ Score 三元分解**：`Validity(×0.4) + Specificity(×0.3) + Correctness(×0.3)`

### 5.11 验证门禁

| 验证类型 | 触发时机 | FailureAction |
|---------|---------|---------------|
| output_format | merge 后 | Skip |
| task_completion | merge 后 | RetryThenBlock |
| human_approval | 关键节点前 | interrupt_before |

### 5.12 TODO 任务看板数据契约

```
TodoBoardState:
  todos: TodoItem[]
  lastUpdated: ISO8601
  source: 'tool_result' | 'session_state' | 'merged'

TodoItem:
  id: string
  content: string
  activeForm: string
  status: 'pending' | 'in_progress' | 'completed'
  updatedAt: ISO8601

TodoColumn:
  key: 'pending' | 'in_progress' | 'completed'
  label: i18n
  items: TodoItem[]
```

### 5.13 工具调用时间线节点模型

```
ToolCallTimelineNode:
  event: ToolUseEvent
  timestamp: HH:mm:ss
  statusPoint: { color, icon, animated }
  summary: string
  argsPreview?: string
  resultPreview?: string
  errorText?: string
  durationLabel: string
  isStuck: boolean   // event.error_code === 'tool_timeout'
```

### 5.14 Stuck 工具检测与提示

| 字段 | 取值 |
|------|------|
| 触发条件 | `event.error_code === 'tool_timeout'` |
| 错误文案 | `chat.activity.stuckTool`（i18n） |
| 范围 | Spirit 模式 + 工具时间线 |
| 徽章 | `TaskExecutionPanel` 顶部 `SpiritStatusBar` 旁，橙色 chip |

---

## 6. 非功能需求

| 项 | 要求 |
|----|------|
| 架构 | `internal/biz` 不 import `pkg/trpc-agent-go`；精灵构建仅在 `internal/service` |
| 性能 | 任务执行面板首屏 10 成员 < 500ms；WS 状态更新 < 200ms；折叠/展开 < 16ms；语境消息替换 < 100ms |
| 并发 | `MaxConcurrentTeams` 可配置，默认 3 |
| 进化 | 编排策略变更受 `GuardrailMaxChangePerPeriod` 约束；DQ Score < 0.3 触发回滚 |
| 兼容 | 精灵模式为 Chat 页面新模式，原有 Agent/Team 列表模式保留为"专家模式"入口 |
| 前端 | 遵循 UX 规范 token；复用 `ChatExecutionCard` / `SessionStatusBadge` |
| WS 回放 | 语境消息和脉冲动画在回放期间静默 |
| 历史恢复 | 加载历史消息时正确恢复折叠态 |
| 嵌套深度 | Session 树最大深度 2（看板嵌套层级受此约束） |

---

## 7. 模块边界

| 模块 | 本需求中的职责 |
|------|----------------|
| Chat (1) | 精灵对话面板、团队组建卡片、任务执行面板、成员只读面板、对话流自动折叠、语境加载消息、底部状态栏、任务看板树形嵌套渲染 |
| Team (11) | 精灵自动创建 Team、TeamRun 状态追踪、多团队并行创建、TeamKey UUID、依赖调度 |
| Orchestration (53) | Task DAG 拓扑路由、依赖感知调度、Agent 节点状态投影、执行时间线 |
| Session (10) | Session 树状关联、ParentSessionID / RootSessionID、深度限制（看板嵌套层级） |
| Agent (2-8) | 精灵 Agent 种子数据、`plan_and_execute` 三阶段编排工具 |
| Evolution (7) | DQ Score 驱动编排缓存、编排策略进化 |
| Memory (L0-L4) | Team Session 记忆提取、Agent 复用时 L3/L4 共享 |
| Builtin Agents | 精灵/编排管家定义、三阶段编排工具 |

**不在范围**：Agent 目录 CRUD、非精灵模式的 Team 编辑、Observatory 页面重构。

---

## 8. 开放问题（已决议）

| # | 问题 | 决议 | 状态 |
|---|------|------|------|
| 1 | 已完成团队是否自动归档？ | C：`ParallelConfig.AutoArchiveSeconds`（默认 3600s） | ✅ |
| 2 | 团队失败后是否自动重试？ | B：提供取消按钮，重试由精灵决策 | ✅ |
| 3 | 成员只读面板是否支持"接管对话"？ | A（后续迭代考虑 B） | ✅ |
| 4 | 精灵对话中是否显示历史团队列表？ | A：仅显示活跃团队 | ✅ |
| 5 | Agent 复用上限？ | C：`ParallelConfig.MaxConcurrentTeams`（默认 3） | ✅ |
| 6 | Session 树最大深度？ | C：`ParallelConfig.MaxSessionDepth`（默认 2） | ✅ |
| 7 | 依赖团队自动启动还是精灵确认后启动？ | C（默认自动，关键任务需确认） | ✅ |
| 8 | Synthesis Engine 用 LLM 合成还是模板合成？ | C（简单场景模板，复杂场景 LLM） | ✅ |
| 9 | 编排拓扑缓存存储位置？ | A：AgentRuntimeSettings JSON | ✅ |
| 10 | 并行团队间是否共享 Memory L3/L4？ | A：共享 | ✅ |
| 11 | 团队超时后是否自动重试？ | B：仅通知精灵 | ✅ |
| 12 | 任务看板采用 turn 级时间线还是工具级时间线？ | 树形嵌套统一结构（任务-思考-工具-回复） | ✅ |
| 13 | 已完成回合默认折叠还是展开？ | 展开，让用户直达最终答案 | ✅ |

---

## 9. 前端组件架构

### 9.1 组件清单

| 组件 | 路径 | 职责 | 状态 |
|------|------|------|------|
| SpiritEntry.vue | `components/spirit/` | 精灵入口卡片 | ✅ |
| SpiritStatusBar.vue | `components/spirit/` | 底部状态栏 | ✅ |
| TeamTaskCard.vue | `components/spirit/` | 侧边栏团队卡片 | ✅ |
| AgentStatusLabel.vue | `components/spirit/` | Agent 状态标签（7 种聚合 + tool_blocked） | ✅ |
| OrchestrationModeBadge.vue | `components/spirit/` | 编排模式标签 | ✅ |
| TeamProgressCard.vue | `components/spirit/` | 团队进度卡片 | ✅ |
| ParallelTeamOverview.vue | `components/spirit/` | 并行团队概览 | ✅ |
| DAGDiagramCard.vue | `components/spirit/` | DAG 依赖图 | ✅ |
| SynthesisResultCard.vue | `components/spirit/` | 综合结果卡片 | ✅ |
| TaskExecutionPanel.vue | `components/spirit/` | 任务执行面板（集成所有子组件） | ✅ |
| TeamAssemblyCard.vue | `components/spirit/` | 团队组建卡片 | ✅ |
| InterruptedTeamCard.vue | `components/spirit/` | 中断恢复提示卡片 | ✅ |
| TeamMemberTreeNode.vue | `components/spirit/` | 成员树形节点 | ✅ |
| MemberReadOnlyPanel.vue | `components/spirit/` | 成员只读面板 | ✅ |
| TaskBoardNode.vue | `components/chat/` | 任务看板单个节点（task/thinking/action/reply/sub_task_board/end/error） | ✅ |
| TaskBoard.vue | `components/chat/` | 任务看板壳（支持树形嵌套渲染） | ✅ |
| ChatExecutionCard.vue | `components/chat/` | 工具执行卡片（增强折叠 + 5s 计时） | ✅ |
| ChatReasoningPeek.vue | `components/chat/` | 思考预览（脉冲+光标闪烁） | ✅ |
| TodoKanbanBoard.vue | `components/chat/` | TODO 三列任务看板 | 📋 P1.6 |
| TodoColumn.vue | `components/chat/` | TODO 单列 | 📋 P1.6 |
| TodoCard.vue | `components/chat/` | TODO 单卡 | 📋 P1.6 |
| ToolCallTimeline.vue | `components/chat/` | 工具调用纵向时间线 | 📋 P1.6 |
| ToolCallTimelineItem.vue | `components/chat/` | 时间线单个节点 | 📋 P1.6 |
| ToolStuckBadge.vue | `components/spirit/` | 任务执行面板 stuck 工具徽章 | 📋 P1.6 |
| CodeBlock.vue | `components/chat/` | 代码块自动识别语言 + 高亮 + 复制 + 折叠 | 📋 P1.6 |
| UiConfigToggle.vue | `components/chat/` | 工具调用显示开关按钮（ChatPanel 顶部） | 📋 P1.6 |

### 9.2 数据流

```
API / Service（features/spirit/api.ts）
        ↓
Pinia Store（stores/spirit/index.ts — useSpiritTeamStore）
        ↓
Composable（features/chat/composables/useChatWorkspace.ts / useChatTimeline.ts / useAgentBlocks.ts）
        ↓
Page（pages/ChatPage.vue）
        ↓ props
Component（components/spirit/*.vue + components/chat/*.vue）
```

### 9.3 WS 事件路由

```
WebSocket Envelope
  → useChatInboundSync.handleInboundEnvelope()
    → env.type.startsWith('spirit_') 或 env.type.startsWith('butler.orchestration.')
      → useSpiritTeamStore.handleSpiritEnvelope(env)
        → 更新 teams/planCreated/allocationCreated/orchestrationStarted/lastCheckpoint/synthesisResult 等
          → ChatPage.vue (spiritStore.sortedTeams / spiritStatusBar computed)
            → ChatEntitySidebar: SpiritEntry + TeamTaskCard 列表
            → ChatMessagePanel: TaskExecutionPanel / SynthesisResultCard / SpiritStatusBar
            → TaskBoard: 任务看板树形嵌套渲染
```

---

## 10. 验收标准索引

### 10.1 核心展示

| ID | 摘要 | 阶段 | 状态 |
|----|------|------|------|
| US-01 | 左侧列表仅显示精灵 + 团队树 | P0 | ✅ |
| US-02 | 精灵区分简单/任务型对话 | P0 | ✅ |
| US-03 | 团队卡片展示名称/状态/成员/进度/Agent标签 | P0 | ✅ |
| US-04 | 任务执行面板三区布局 + 任务看板 | P0 | ✅ |
| US-05 | 任务看板树形嵌套展示 | M69 P1 | ✅ |
| US-06 | 成员树形展开 + 状态 | P1 | ✅ |
| US-07 | 成员只读面板（无输入框） | P1 | ✅ |
| US-08 | 多任务并行 + Agent 复用隔离 | P0.5 | ✅ |
| US-09 | 团队生命周期（归档/取消/重试） | P1 | ✅ |
| US-10 | 面包屑导航 + 返回精灵 | P1 | ✅ |
| US-11 | 对话流自动折叠 | OBS P0 | ✅ |
| US-12 | 语境加载消息 | OBS P0 | ✅ |
| US-13 | Agent 状态标签（含 tool_blocked） | OBS P0 + M69 P4 | ✅ |
| US-14 | 底部状态栏 | OBS P1 | ✅ |
| US-15 | 侧边栏状态脉冲 | OBS P1 | ✅ |
| US-16 | 可折叠工具输出 | OBS P0 | ✅ |
| US-17 | ChatExecutionCard 折叠增强 | OBS P1.5 | ✅ |
| US-18 | 中断恢复提示 | OBS P1 | ✅ |
| US-19 | TODO 任务看板 | P1.6 | 📋 |
| US-20 | 工具调用时间线 | P1.6 | 📋 |
| US-21 | Stuck 工具可观测化 | P1.6 | 📋 |

### 10.2 编排

| ID | 摘要 | 阶段 | 状态 |
|----|------|------|------|
| SPO-01 | 多团队并行执行 | M60 P1 | ✅ |
| SPO-02 | 任务依赖调度 | M60 P2 | ✅ |
| SPO-03 | 编排模式智能选择 | M60 P2 | ✅ |
| SPO-04 | 多团队结果合成 | M60 P2 | ✅ |
| SPO-05 | 编排策略进化 | M60 P2 | ✅ |
| SPO-06 | 任务复杂度智能评估 | M60 P4 | ✅ |
| SPO-07 | Graph DAG 编排 | M60 P4 | ✅ |
| SPO-08 | 编排验证门禁 | M60 P4 | ✅ |

### 10.3 M69 修复

| ID | 摘要 | 阶段 | 状态 |
|----|------|------|------|
| M69-01 | 团队列表数据加载（loadSpiritTeams） | P0 | ✅ |
| M69-02 | WS 重连团队数据恢复 | P0 | ✅ |
| M69-03 | 单 Agent 会话按时间线展示 | P1 | ✅ |
| M69-04 | Team 会话按时间线展示 | P1 | ✅ |
| M69-05 | 思考和动作元素完成后自动折叠 | P1 | ✅ |
| M69-06 | TaskExecutionPanel 展示三区布局 | P2 | ✅ |
| M69-07 | 左侧面板仅显示精灵+团队树 | P2 | ✅ |
| M69-08 | UI 原型对齐优化（Agent Block Header、嵌套缩进、脉冲圆点等） | P3 | ✅ |
| M69-09 | useAgentBlocks 业务逻辑修复（F-13~F-21） | P4 | ✅ |

### 10.4 useAgentBlocks 业务逻辑审查（子模块，2026-06-10）

| ID | 验收项 | 优先级 | 文件 |
|----|--------|--------|------|
| AC-17 | `SubAgentBuilder.addTool` 必须将对应 message 推入 `allToolMsgs`，使 `allToolsDone` 检查在工具未完成时不返回 `true` | P0 | `useAgentBlocks.ts` |
| AC-18 | 子代理 block 状态机增加 `tool_blocked` 显式状态；`tool_blocked` 不应被合并到 `running` 中 | P1 | `useAgentBlocks.ts` |
| AC-19 | `resolvePlanStatus` 在 `planStatus === 'planning' && agentStatus === 'running' && planEntries.length > 0` 时必须返回 `'executing'` | P0 | `useAgentBlocks.ts` |
| AC-20 | progress envelope 的 sortKey 不得小于 user 消息对应 sortKey；时钟漂移场景使用钳制（`Math.max(0, offset) - 0.5`） | P1 | `useAgentBlocks.ts` |
| AC-21 | Reply 去重判断必须在 ReAct 模式下也走 `hasExplicitFinalAnswer` 判定（与 `resolveReplyContent` 语义对齐） | P1 | `useAgentBlocks.ts` |
| AC-22 | `updatePlanEntryStatuses` 的 plan entry 与 sub-agent block 匹配改用 `agentKey` 而非 `agentName \|\| task` | P1 | `useAgentBlocks.ts` |
| AC-23 | 已完成回合（`status === 'completed'`）的 `collapsed` 默认为 `false`（展开态），让用户直达最终答案 | P2 | `useAgentBlocks.ts` |
| AC-24 | AgentBlock 暴露 `hasPartialFailure` 字段（`hasFailedTool && hasSuccessfulResult`），UI 在回合头显示"⚠️ 部分工具失败"徽章 | P2 | `useAgentBlocks.ts` |
| AC-25 | progress section 整体移到 turn 头部（user 消息之后、第一条 timeline 条目之前），与 timeline 主线视觉分离 | P3 | `useAgentBlocks.ts` + `TimelineNode.vue` |

> **文档边界说明（2026-06-10）**：本节 AC 仅约束 M59/M69 整合后时间线展示层（`useAgentBlocks.ts` 构建的 AgentBlock 树）。**AgentBlock.result 与 timeline reply 与 SynthesisResultCard 三方重复展示问题**，因 `SynthesisResultCard` 由 M59 精灵模式拥有，仅在 M59 端给出"单一来源"约束（`AgentBlock.result` 降级为兼容字段），合成卡片的去重属于 M59 范围。

---

## 11. 遗留技术债

| ID | 描述 | 优先级 | 状态 |
|----|------|--------|------|
| TD-1 | api.ts 双键名兼容（teamKey/team_key） | P1 | ✅ 已修复 |
| TD-2 | ListSpiritTeams HTTP 端点未暴露 | P1 | ✅ 已暴露（GET /v1/spirit/{id}/teams） |
| TD-3 | ArchiveTeam RPC 未定义 | P1 | ✅ 已定义（POST /v1/teams/{id}/archive） |
| TD-4 | MemberReadOnlyPanel 占位符 | P1 | ✅ 已实现 |
| TD-5 | TeamMemberTreeNode 未实现 | P1 | ✅ 已实现 |
| TD-6 | 面包屑导航未实现 | P1 | ✅ 已实现 |
| TD-7 | 重试失败团队未实现 | P1 | ✅ 已实现 |
| TD-8 | DQ Score 前端展示 | P2 | ✅ 已实现 |
| TD-9 | 验证门禁结果前端展示 | P2 | ✅ 已实现 |
| TD-10 | 条件路由 UI 展示 | P2 | ✅ 已实现 |
| TD-11 | WriteDeliverablesToSession 使用 ParallelConfigJSON 存储交付物输出，语义不匹配 | P2 | ⚠️ 已标记 TECH-DEBT |
| TD-12 | resolveVerificationGates 未实现 LinkedGraphID 查询路径 | P2 | ⚠️ |
| TD-13 | 废弃 Spirit 工具代码残留约 400 行 | P2 | ⚠️ |
| TD-TK-1 | `todo_write` 工具结果在前端展示为 `result` 字段嵌套 JSON | P1 | 📋 P1.6 |
| TD-TK-2 | `stuckToolResultReason` 文案为 Go 常量硬编码，前端无法 i18n | P1 | 📋 P1.6 |
| TD-TK-3 | `ChatExecutionCard` 与 `ToolCallTimeline` 在多工具时并存 | P2 | 📋 |
| TD-TK-4 | 看板变更脉冲在虚拟滚动回收后可能丢失 | P2 | 📋 P1.6 验证 |
| TD-TK-5 | 工具显示开关当前未实现，测试环境与生产环境无法差异化展示 | P1 | 📋 P1.6 (TK-04) |
| TD-TK-6 | 代码块无高亮，依赖默认 markdown 渲染，长代码无折叠 | P2 | 📋 P1.6 (TK-05) |
| TD-TK-7 | 思考节点视觉权重过重，与"不喧宾夺主"目标冲突 | P2 | 📋 P1.6 (TK-06) |

## 12. UX 人性化改进（2026-06-09 实施）

| ID | 改进项 | 优先级 | 状态 |
|----|--------|--------|------|
| UX-1 | TeamProgressCard 增加 ETA 预计完成时间 | P1 | ✅ |
| UX-2 | MemberReadOnlyPanel 增加"返回精灵"快捷按钮 | P1 | ✅ |
| UX-3 | TeamTaskCard 侧边栏增加实时耗时显示 | P1 | ✅ |
| UX-4 | TeamTaskCard/TeamProgressCard 失败时显示错误摘要 | P1 | ✅ |
| UX-5 | ToolStrip `<details>` → `q-expansion-item` 统一折叠动画 | P2 | ✅ |
| UX-6 | SpiritStatusBar 并行配额迷你进度条 | P2 | ✅ |
| UX-7 | ChatExecutionCard `aria-expanded`/`aria-controls` 无障碍 | P2 | ✅ |
| UX-8 | Provide `readonly()` 运行时包装 signal | P3 | ✅ |
| UX-9 | Summary fallback 语言改中文 | P3 | ✅ |
| UX-10 | TurnBlock 显示 Agent Block Header（头像+名称+状态徽章+耗时+子任务数） | P1 | ✅（M69） |
| UX-11 | ChatExecutionCard 显示 agent 首字头像 | P1 | ✅（M69） |
| UX-12 | 运行中工具耗时显示 `...` 后缀（如 `8s...`） | P2 | ✅（M69） |
| UX-13 | 思考流式输出显示脉冲圆点指示器和光标闪烁 | P2 | ✅（M69） |
| UX-14 | 运行中工具卡片显示脉冲圆点指示器 | P2 | ✅（M69） |
| UX-15 | 全局展开/折叠按钮始终可见（右对齐） | P2 | ✅（M69） |
| UX-16 | Sub-Agent 嵌套缩进（左边框线+缩进） | P1 | ✅（M69） |
| UX-17 | 执行结果区段显示"📊 执行结果"标签 | P2 | ✅（M69） |

---

## 13. 学术参考索引

| 论文 | 对本项目的贡献 |
|------|--------------|
| LAMaS (arXiv:2601.10560) | Task DAG 层级调度、关键路径优化 |
| AdaptOrch (arXiv:2602.16873) | 拓扑路由算法 O(\|V\|+\|E\|) |
| Maestro (arXiv:2511.06134) | 探索-合成分离、Synthesis Engine |
| M1-Parallel (arXiv:2507.08944) | 多团队并行竞速（远期参考） |
| APWA (arXiv:2605.15132) | Manager-Worker-Executor 三层分离 |
| DTA-Llama (arXiv:2501.12432) | Divide-Then-Aggregate 范式 |
| ParaCook (arXiv:2510.11608) | 时间效率感知并行规划 |
| Puppeteer (NeurIPS 2025) | RL 动态编排、DQ Score 驱动策略优化 |

---

## 14. 子模块：useAgentBlocks 业务逻辑审查（2026-06-10）

> **范围**：本前端时间线展示层（`web/src/features/chat/composables/useAgentBlocks.ts`）
> **触发**：用户报告 CHAT UI 最终回复重复展示 → 排查 → 静态代码审查发现 8 项业务逻辑问题
> **关联**：[59-chat-ui-optimization.design.md §D5](./59-chat-ui-optimization.design.md)

### 14.1 背景

`useAgentBlocks.ts` 是任务看板树形嵌套展示的核心 composable，负责将消息流构建为 AgentBlock 树。随 M59 第四轮审查与 M69 修复，timeline 渲染逐步稳定；但**消息→AgentBlock 的构建层**未做系统性审查，遗留若干状态机缺陷与 UX 友好度问题。

### 14.2 问题清单

| # | 类型 | 严重度 | 描述 |
|---|------|--------|------|
| F-13 | **Bug** | 🔴 P0 | `SubAgentBuilder.addTool` 漏写 `allToolMsgs`，`allToolsDone` 恒为 `true`，子代理在工具未完成时被误判为 `completed` |
| F-14 | **Bug** | 🔴 P0 | 状态机无 `tool_blocked` 显式状态，被合并入 `running`，与"等用户确认"语义混淆 |
| F-15 | **Bug** | 🟠 P1 | `resolvePlanStatus` 漏 `running` 转换分支，`subagents_spawn` 路径下 PlanCard 永远停在"规划中" |
| F-16 | **Bug** | 🟠 P1 | progress sortKey 未钳制，时钟漂移导致 progress 卡片插到 user 消息之前 |
| F-17 | **Bug** | 🟠 P1 | Reply 去重仅在非 ReAct 模式生效，与 `resolveReplyContent` 语义不一致 |
| F-18 | **Bug** | 🟠 P1 | Plan entry ↔ sub-agent block 匹配用 `agentName \|\| task`，多子代理相似任务时错配 |
| F-19 | **UX** | 🟡 P2 | 已完成回合默认折叠，用户看不到 AI 最终答案 |
| F-20 | **UX** | 🟡 P2 | 部分工具失败被掩盖为 `completed`，无可观测信号 |
| F-21 | **UX** | 🟢 P3 | progress section 与 timeline 主线混排，破坏线性叙事 |

### 14.3 不纳入范围（边界说明）

| 项 | 原因 | 归属 |
|----|------|------|
| AgentBlock.result 与 SynthesisResultCard 重复展示 | `SynthesisResultCard` 由 M59 拥有 | M59 |
| `findUserTurns` 无 user 消息边界 | 属于消息分组层（`groupMessagesByTurn`） | M1 Chat |
| `isLongRunning` UI 联动 | 属工具层（`activityPresentation`） | M23 Tools |

### 14.4 验收追溯

| 子模块 AC | 完整 AC 定义 |
|-----------|--------------|
| AC-17~AC-25 | 见 §10.4 表格 |
| F-13~F-21 | 见 §14.2 表格，对应修复点见设计 §D5 和开发计划 Phase P4 |
