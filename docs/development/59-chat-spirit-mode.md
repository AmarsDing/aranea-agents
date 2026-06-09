# M59: Chat 精灵模式 — 完整需求文档（M59+OBS+M60 合并版）

> **版本**：2026-06-09 | **状态**：P0+P0.5+OBS+M60(P1~P4) + P1 交互增强 + P2 展示 + UX 人性化改进全部完成
> **合并来源**：M59 精灵模式核心需求 + M59-OBS 可观测性 UX + M60 并行编排需求
> **技术设计**：[59-chat-spirit-mode.design.md](./59-chat-spirit-mode.design.md)
> **开发计划**：[59-chat-spirit-mode.development.md](./59-chat-spirit-mode.development.md)
> **读者**：产品、全栈开发、运维

---

## 1. 背景与问题

当前 Chat 页面左侧列表同时展示**所有 Agent 和 Team**，用户需要自行理解行业、选择 Agent、组建 Team、配置编排。同时，已完成步骤与活跃步骤平铺展示，工具调用卡片全部展开占据大量空间，精灵思考/执行时显示通用 spinner。

| 问题 | 用户影响 | 来源 |
|------|----------|------|
| 用户需在数十个 Agent/Team 中选择对话对象 | 认知成本高，选择困难 | M59 |
| Agent 和 Team 平铺展示，无层级关系 | 无法体现"管家调度→团队执行"的业务逻辑 | M59 |
| 同一精灵 Session 只能有一个活跃团队 | 无法并行推进多个任务 | M60 |
| 工具调用卡片全部展开，长对话时占据大量空间 | 用户需大量滚动才能找到当前活跃内容 | OBS |
| 精灵思考/执行时显示通用 spinner | 用户不知道 Agent 在做什么，等待体验差 | OBS |
| 团队成员状态仅在展开后才能看到 | 用户无法一眼感知团队执行进度 | OBS |
| 中断团队无恢复提示 | 用户不知道可以恢复执行 | OBS |

**核心原则**：
1. **用户不需要知道有哪些 Agent、如何编排**——只需与精灵对话，精灵负责调度一切
2. **可观测性强，但不影响主要内容的显示**——环境可观测性优先，完成即折叠

---

## 2. 用户故事

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

**作为** 用户
**我希望** 与精灵对话时，简单问题精灵直接回复，复杂任务精灵自动组建团队执行
**以便** 我不需要手动判断是否需要组建团队

**验收**：

- 简单对话（闲聊、知识问答、简单查询）：精灵直接回复，不组建团队
- 任务型对话（开发、分析、创作等复杂任务）：精灵调用 `plan_and_execute` 工具，经三阶段编排（Plan → Allocate → Orchestrate）组建团队执行
- 精灵在组建团队时，向用户展示任务分析结果（任务类型、所需角色、预估步骤）
- 组建过程对用户可见：精灵回复中包含"正在组建团队…"的执行卡片
- 用户可在精灵对话中下达多个任务指令，每个任务独立组建团队

### US-03 任务团队在精灵下方展示

**作为** 用户
**我希望** 精灵组建的团队在左侧列表精灵入口下方展示
**以便** 我能一目了然看到所有进行中的任务团队

**验收**：

- 每个任务团队在精灵下方显示为一个卡片条目，包含：
  - **团队名称**：精灵根据任务自动生成
  - **任务摘要**：一句话描述团队正在执行的任务
  - **运行状态**：`pending` / `running` / `completed` / `failed` / `cancelled` / `interrupted` / `archived`
  - **成员头像组**：最多显示 4 个成员头像，超出显示 +N
  - **编排模式标签**：sequential / parallel / hybrid / coordinator（`OrchestrationModeBadge` 组件）
  - **进度指示**：已完成步骤 / 总步骤
  - **DAG 依赖提示**：等待前置任务时显示依赖数量
  - **Agent 状态标签**：每个成员显示状态标签（`AgentStatusLabel` 组件）
- 团队按**活跃度排序**：running → pending → interrupted → completed → failed → cancelled
- 同状态的团队按创建时间倒序
- 已完成的团队默认折叠到"已完成"分组，可展开查看
- 并行团队概览区显示进行中/已完成团队计数和并行配额进度条
- 团队状态变化时卡片短暂脉冲高亮（OBS-05）

### US-04 点击团队查看任务执行面板

**作为** 用户
**我希望** 点击任务团队后，中间聊天区域展示该团队的执行过程
**以便** 我能实时观察团队的工作进展

**验收**：

- 点击团队卡片 → 中间区域切换为该团队的**任务执行面板**
- 任务执行面板布局：

```
+----------------------------------------------------------+
| [← 返回精灵]  团队名称    状态 Badge    编排模式标签      |
+----------------------------------------------------------+
|                                                          |
|  ┌─ 并行团队概览 ────────────────────────────────────┐   |
|  │ 进行中：2  已完成：1  并行配额：2/3               │   |
|  │ ┌─ DAG 依赖图 ──────────────────────────────┐     │   |
|  │ │ ▶ 任务A → ⏳ 任务B(依赖A) → ⏳ 任务C(依赖B)│     │   |
|  │ └────────────────────────────────────────────┘     │   |
|  └───────────────────────────────────────────────────┘   |
|                                                          |
|  ┌─ 团队进度卡片 ────────────────────────────────────┐   |
|  │ 后端 API 开发团队    ⚡ running    2/5 步骤        │   |
|  │ ████████░░░░░░░░░░░░ 40%    耗时 1m 20s           │   |
|  │ 👤 Golang 工程师 · 代码审查员 · 测试工程师 +1     │   |
|  │ [AgentStatusLabel: 执行中/排队中/等待中...]        │   |
|  └───────────────────────────────────────────────────┘   |
|                                                          |
|  ┌─ 中断恢复提示（条件显示）──────────────────────────┐   |
|  │ ⏸ 团队已中断 — 因服务器重启而中断                  │   |
|  │ 已完成 3/5 步骤  [恢复执行] [取消团队]             │   |
|  └───────────────────────────────────────────────────┘   |
|                                                          |
|  ┌─ 综合结果 ────────────────────────────────────────┐   |
|  │ 📋 混合合成    100% 成功    耗时 3m 20s            │   |
|  │ 各团队结果摘要...                                  │   |
|  └───────────────────────────────────────────────────┘   |
|                                                          |
+----------------------------------------------------------+
| ⚡ 2 running │ ⏸ 1 interrupted │ 📊 2/3 quota │ ✅ Team A │ ← SpiritStatusBar
+----------------------------------------------------------+
```

- **并行团队概览区**（`ParallelTeamOverview`）：展示多团队并行状态、并行配额、DAG 依赖图
  - DAG 依赖图（`DAGDiagramCard`）：简化文本视图展示任务依赖关系 + 验证节点
  - 所有团队完成后显示"所有团队已完成"提示
- **团队进度卡片**（`TeamProgressCard`）：每个团队独立展示进度、状态、成员
  - 状态图标：running(spinner) / completed(✓) / failed(✗) / cancelled(⊘) / pending(⏳)
  - 进度条 + 执行时长
  - 取消按钮（running/pending 状态可用）
  - 依赖状态区分：有依赖的 pending 显示"等待依赖完成"，无依赖的显示"等待调度"
- **Agent 状态标签**（`AgentStatusLabel`）：每个成员显示聚合后的状态标签
- **中断恢复提示**（`InterruptedTeamCard`）：interrupted 状态时显示恢复/取消按钮
- **综合结果区**（`SynthesisResultCard`）：所有团队完成后展示合成结果
  - 合成策略标签（模板合成 / Prompt 合成 / 混合合成）
  - 成功率指标（100% 成功/部分成功/低成功率）
  - 各团队结果摘要和关键发现
- WS 实时推送：15+ 种 Spirit 信封事件实时更新面板

### US-05 展开团队查看成员树形列表

**作为** 用户
**我希望** 在左侧列表中展开团队卡片，以树形结构查看团队成员
**以便** 我能快速了解团队的人员组成和各自状态

**验收**：

- 团队卡片支持展开/折叠（点击展开箭头或双击卡片）
- 展开后显示树形成员列表，每个成员显示：
  - **名称**：Agent display_name
  - **角色标签**：worker / synthesizer / generator / critic 等
  - **工作状态**：通过 `AgentStatusLabel` 展示（排队中/执行中/等待中/已完成/失败/已跳过/已取消）
- 成员按 `SortOrder` 排列
- **P1 状态**：✅ `TeamMemberTreeNode` 组件已实现

### US-06 点击成员查看只读对话输出

**作为** 用户
**我希望** 点击团队成员后，查看该成员的对话输出
**以便** 我能了解某个 Agent 具体做了什么

**验收**：

- 点击成员 → 中间区域切换为该成员的**只读对话输出面板**
- 面板内容：顶部成员名称+角色+返回按钮、消息流、工具调用卡片、执行统计
- **只读模式**：输入面板隐藏，消息不可编辑/不可回复
- 消息按堆栈模型分组（`groupMessagesByTurn`），遵循前端红线 #14
- **P1 状态**：✅ `MemberReadOnlyPanel` 已实现

### US-07 多任务并行与 Agent 复用

**作为** 用户
**我希望** 可以同时给精灵下达多个任务，且同一 Agent 可以被多个团队复用
**以便** 我可以并行推进多个任务，而不需要为每个团队配置独立的 Agent 实例

**验收**：

- 用户可在精灵对话中连续下达多个任务指令
- 每个任务独立组建团队，左侧列表同时展示多个团队卡片
- 同一 Agent 可出现在多个团队中，拥有独立的 Session、Runner、状态
- **并行配额**：`ParallelConfig` 控制最大并行团队数（默认 3）和单团队最大并发数（默认 2）
- **DAG 编排**：复杂任务可分解为多个子任务，形成 DAG 依赖图，按拓扑顺序执行
- **条件路由**：编排管家可根据 Agent 输出动态路由到不同下游 Agent（`ConditionalBranch`）

### US-08 团队生命周期管理

**作为** 用户
**我希望** 了解团队在任务完成后的状态，以及如何管理历史团队
**以便** 我能保持工作区整洁，同时保留有价值的历史信息

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
- **失败团队**：显示错误信息和失败步骤，提供"取消"按钮（✅ 已实现）、"重试"按钮（✅ 已实现）、"归档"按钮（✅ 已实现）
- **精灵可主动汇报**：团队完成后，精灵在对话中主动通知用户任务结果

### US-09 返回精灵对话

**作为** 用户
**我希望** 在查看团队或成员详情后，能方便地返回精灵对话
**以便** 我能继续给精灵下达新指令

**验收**：

- 任务执行面板和成员只读面板顶部均有"← 返回精灵"按钮 — ✅ 已实现
- 点击后切换回精灵的聊天面板 — ✅ 已实现
- 左侧列表点击精灵入口也切换回精灵对话 — ✅ 已实现
- 切换不丢失当前团队的 WS 连接和实时状态 — ✅ 已实现
- 面包屑导航：精灵 > 团队名称 > 成员名称 — ✅ 已实现

---

## 3. 可观测性 UX 用户故事（M59-OBS）

### OBS-01 对话流自动折叠

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
- **状态**：✅ P0 已完成

### OBS-02 语境加载消息

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
- **状态**：✅ P0 已完成

### OBS-03 Agent 状态标签

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
- **状态**：✅ P0 已完成

### OBS-04 底部状态栏

**作为** 用户
**我希望** 聊天面板底部固定显示全局并行状态
**以便** 我不需要切换到左侧栏就能感知整体进度

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
- **状态**：✅ P1 已完成

### OBS-05 侧边栏状态脉冲

**作为** 用户
**我希望** 左侧团队卡片在状态变化时短暂高亮
**以便** 我在阅读精灵对话时也能感知到团队状态变化

**验收**：

- 团队状态变化时卡片短暂脉冲高亮：

| 状态变化 | 脉冲颜色 | 持续时间 |
|---------|---------|---------|
| → running | 蓝色 | 1.0s |
| → completed | 绿色 | 1.5s |
| → failed | 红色 | 2.0s |
| → interrupted | 橙色 | 1.5s |

- WS 回放期间禁用脉冲动画
- **状态**：✅ P1 已完成

### OBS-06 可折叠工具输出

**作为** 用户
**我希望** 已完成的工具调用自动折叠，只显示工具名和耗时
**以便** 多个工具调用连续展示时不影响对话可读性

**验收**：

- 工具调用卡片（`ChatExecutionCard`）完成后自动折叠
- 折叠态显示：工具名 + 状态图标 + 耗时（1 行，~32px）
- running 状态的工具调用始终展开
- failed 状态的工具调用折叠但红色高亮
- 加载历史消息时，已完成的工具卡片默认折叠
- **状态**：✅ P0 已完成

### OBS-08 ChatExecutionCard 独立折叠增强

> 增强 OBS-06 折叠态内容，扩展全局控制范围至 ChatExecutionCard 级别。设计：§6.8

**作为** 用户
**我希望** 工具卡片有实时耗时显示、折叠摘要更丰富、全局展开/折叠可同时作用于工具卡片
**以便** 长任务可判断是否卡死，折叠后仍能了解工具做了什么，一键操作即可控制所有内容

**验收**：

- 工具运行 ≥5s 时显示实时计时器（`5s` → `1m 12s`），≥60s 变为警告色
- `started_at` 为空时降级：`occurred_at` → `Date.now()`，始终启动计时器
- 折叠态摘要兜底：后端未提供 `summary` 时，前端根据 `tool_name` + `arguments` 生成摘要（如"修改 auth.go"、"搜索 TODO"）
- ToolStrip 折叠态显示工具类型分布（如"3 file_read · 2.5s"而非"3 tools · 2.5s"）
- 全局"展开全部/折叠全部"按钮同时作用于 TurnBlock 和 ChatExecutionCard 两层
- 运行中的工具不受"折叠全部"影响
- Spirit 模式（TaskExecutionPanel）中的 ChatExecutionCard 同样响应全局控制
- `ToolUseEvent.expanded` 死代码字段清理 — ✅ 不存在此字段，各组件 `expanded` 为独立 UI 状态
- **状态**：✅ P1.5 已完成

### OBS-07 中断恢复提示

**作为** 用户
**我希望** 中断的团队显示恢复提示卡片
**以便** 我知道可以恢复执行，而不是以为任务丢失了

**验收**：

- interrupted 状态的团队在任务执行面板中显示恢复提示卡片（`InterruptedTeamCard`）：
  - 团队名称 + 中断原因
  - 已完成步骤 / 总步骤
  - "恢复执行"按钮（调用 `ResumeTeamRunExecution` API）
  - "取消团队"按钮
- 如果团队无 `graph_execution_id`，显示"此团队不支持断点恢复"
- **状态**：✅ P1 已完成

---

## 4. 并行编排用户故事（M60 核心）

### SPO-01 多团队并行执行

**作为** 用户
**我希望** 可以同时给精灵下达多个任务，每个任务独立组建团队并行执行
**以便** 我可以同时推进多个独立任务

**验收**：

- 精灵可在同一对话中连续调用 `plan_and_execute` 组建多个团队
- 每个团队拥有独立的 Session、Runner、状态
- 并行度可配置（默认最大 3 个并行团队），超出时精灵提示用户等待
- **状态**：✅ P1 已完成

### SPO-02 任务依赖调度

**作为** 用户
**我希望** 可以表达任务之间的依赖关系
**以便** 有依赖关系的任务按正确顺序执行，无依赖的任务并行推进

**验收**：

- 精灵在分析复杂需求时，自动识别任务间的依赖关系
- 依赖团队在前置团队完成后自动启动
- 无依赖的团队立即并行启动
- 精灵回复中展示任务依赖图（文本形式，`DAGDiagramCard`）
- **状态**：✅ P2 已完成

### SPO-03 编排模式智能选择

**作为** 用户
**我希望** 精灵能根据任务特征自动选择最优编排模式
**以便** 不同类型的任务使用最合适的编排策略

**验收**：

- 精灵根据任务 DAG 结构自动选择编排拓扑：parallel / sequential / hybrid / coordinator
- 选择依据包含历史 DQ Score 数据（如有）
- 精灵回复中说明选择该编排模式的理由（`OrchestrationModeBadge` tooltip）
- **状态**：✅ P2 已完成

### SPO-04 多团队结果合成

**作为** 用户
**我希望** 所有并行团队完成后，精灵自动合成各团队结果
**以便** 我能一次性看到所有任务的汇总结果

**验收**：

- 所有活跃团队完成后，精灵自动调用 Synthesis Engine 合成结果
- 合成结果包含：每个团队的任务摘要、执行状态、关键产出
- 部分团队失败时，合成结果标注失败团队及原因
- 成功率指标：100% 成功/部分成功/低成功率（颜色区分）
- **状态**：✅ P2 已完成

### SPO-05 编排策略进化

**作为** 用户
**我希望** 精灵的编排策略能从历史执行中学习优化
**以便** 同类任务越做越好，减少次优编排

**验收**：

- 每次团队执行完成后计算 DQ Score（三元分解：Validity×0.4 + Specificity×0.3 + Correctness×0.3）
- DQ Score > 0.7 缓存编排拓扑，相似任务优先复用
- DQ Score < 0.5 生成编排优化建议
- 进化护栏确保策略变更幅度可控
- **状态**：✅ P2 全栈已完成

### SPO-06 任务复杂度智能评估

**作为** 用户
**我希望** 精灵在委派任务前先评估任务复杂度
**以便** 简单问题得到快速响应，复杂任务得到充分编排

**验收**：

- 精灵收到用户消息后，先评估复杂度（simple/moderate/complex）
- 规则引擎优先判断（零 Token 消耗），无法判断时返回 moderate
- 复杂度评估结果在 `SpiritStatusBar` 中展示（简单/中等/复杂 + 策略理由 tooltip）
- **状态**：✅ P4 后端已完成，✅ 前端已展示

### SPO-07 Graph DAG 编排

**作为** 用户
**我希望** 编排管家能将复杂任务拆解为 Graph DAG 结构执行
**以便** 无依赖的 Agent 并行执行，有依赖的 Agent 按序执行

**验收**：

- 编排管家新增 `build_orchestration_graph` 工具，动态生成 `GraphBuildConfig`
- Graph DAG 支持并行节点、汇合节点、条件路由（`ConditionalBranch`）
- 验证节点可注入（output_format / task_completion / human_approval）
- **状态**：✅ P4 全栈已完成

### SPO-08 编排验证门禁

**作为** 用户
**我希望** 编排管家的 Graph DAG 中包含自动验证节点
**以便** 质量不达标时自动回退/重试

**验收**：

- Graph DAG 中可注入验证节点（Verification Node）
- 验证节点类型：output_format / task_completion / human_approval
- 验证失败时根据 FailureAction 处理：Skip / RetryThenBlock / FailFast
- HITL 验证节点使用 Graph 的 interrupt 机制
- 验证节点在 `DAGDiagramCard` 中展示（类型 + 状态 + 失败原因）
- **状态**：✅ P4 后端已完成，🟡 前端验证节点展示已准备类型定义

---

## 5. 功能规格

### 5.1 三层可观测性架构

```
┌─────────────────────────────────────────────────────┐
│ L1 环境层 (Ambient) — 始终可见，零干扰              │
│ OBS-02 语境加载消息  OBS-03 Agent 状态标签           │
│ OBS-05 侧边栏脉冲    OBS-04 底部状态栏              │
│ SPO-06 复杂度标签    SPO-07 检查点步骤               │
├─────────────────────────────────────────────────────┤
│ L2 结构层 (Structural) — 按需查看，不遮挡            │
│ OBS-01 对话流自动折叠  OBS-06 可折叠工具输出         │
│ OBS-07 中断恢复提示    SPO-02 DAG 依赖图             │
│ SPO-03 编排模式标签    SPO-08 验证门禁节点           │
├─────────────────────────────────────────────────────┤
│ L3 证据层 (Evidential) — 主动展开才可见              │
│ ChatExecutionCard 展开态  ChatDiffViewer             │
│ TeamRunObservatory Timeline  SynthesisResultCard     │
└─────────────────────────────────────────────────────┘
```

### 5.2 设计原则

| # | 原则 | 说明 |
|---|------|------|
| DP-1 | 环境可观测性优先 | 状态信息以颜色、图标、微动画呈现，不占用主内容区空间 |
| DP-2 | 渐进式信息披露 | 默认只展示 L1，用户主动交互才展开 L2/L3 |
| DP-3 | 完成即折叠 | 已完成的步骤/团队/工具调用自动收起，保持视觉焦点在活跃内容 |
| DP-4 | 状态即视觉 | 颜色、图标、动画三位一体传达状态 |
| DP-5 | 证据后置 | 过程信息轻量展示，详细证据仅在用户主动查看时展开 |

### 5.3 中间面板状态机

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  精灵对话面板  │     │  任务执行面板  │     │ 成员只读面板   │
│              │     │              │     │              │
│ L1: 语境消息  │     │ L1: Agent标签 │     │ L1: 状态标签  │
│ L2: 自动折叠  │     │ L2: 进度卡片  │     │ L2: 工具折叠  │
│ L2: 工具折叠  │     │ L2: 恢复提示  │     │              │
│ L1: 底部状态栏│     │ L1: 底部状态栏│     │ L1: 底部状态栏│
└──────────────┘     └──────────────┘     └──────────────┘
```

### 5.4 并行团队管理

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `MaxConcurrentTeams` | 3 | 同一精灵 Session 最大并行团队数 |
| `MaxTeamConcurrency` | 2 | 单团队内最大并发成员数 |
| `TeamTimeout` | 10min | 单团队超时 |
| `AutoArchiveAfter` | 1h | 完成后自动归档时间 |
| `MaxSessionDepth` | 2 | Session 树最大深度 |

### 5.5 Task DAG 模型

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

**条件路由**（P4 增强）：

```
ConditionalBranch:
  source_agent: string
  condition_func: string
  path_map: map[string]string
  default_path: string
```

### 5.6 Synthesis Engine

| 场景 | 策略 |
|------|------|
| 全部成功 | 完整合成 |
| 部分失败 | 部分合成，标注失败团队 |
| 全部失败 | 失败报告 |
| 依赖链中断 | 级联标注 |

### 5.7 精灵工具扩展

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

### 5.8 事件驱动模型

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

### 5.9 编排进化闭环

```
用户需求 → 精灵判断 → 组建团队（基于历史 DQ Score 选择编排模式）
    → 团队执行 → Session 执行轨迹记录
        ├──→ DQ Score 计算 → 编排策略优化
        ├──→ 工具调用模式检测 → Skill 提议
        └──→ Agent 能力画像更新 → tool_weight 调整
```

**DQ Score 三元分解**：`Validity(×0.4) + Specificity(×0.3) + Correctness(×0.3)`

### 5.10 验证门禁

| 验证类型 | 触发时机 | FailureAction |
|---------|---------|---------------|
| output_format | merge 后 | Skip |
| task_completion | merge 后 | RetryThenBlock |
| human_approval | 关键节点前 | interrupt_before |

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

---

## 7. 模块边界

| 模块 | 本需求中的职责 |
|------|----------------|
| Chat (1) | 精灵对话面板、团队组建卡片、任务执行面板、成员只读面板、对话流自动折叠、语境加载消息、底部状态栏 |
| Team (11) | 精灵自动创建 Team、TeamRun 状态追踪、多团队并行创建、TeamKey UUID、依赖调度 |
| Orchestration (53) | Task DAG 拓扑路由、依赖感知调度、Agent 节点状态投影、执行时间线 |
| Session (10) | Session 树状关联、ParentSessionID / RootSessionID、深度限制 |
| Agent (2-8) | 精灵 Agent 种子数据、`plan_and_execute` 三阶段编排工具 |
| Evolution (7) | DQ Score 驱动编排缓存、编排策略进化 |
| Memory (L0-L4) | Team Session 记忆提取、Agent 复用时 L3/L4 共享 |
| Builtin Agents | 精灵/编排管家定义、三阶段编排工具 |

**不在范围**：Agent 目录 CRUD、非精灵模式的 Team 编辑、Observatory 页面重构。

---

## 8. 开放问题

| # | 问题 | 决议 | 状态 |
|---|------|------|------|
| 1 | 已完成团队是否自动归档？ | C：`ParallelConfig.AutoArchiveSeconds`（默认 3600s） | ✅ 已决议 |
| 2 | 团队失败后是否自动重试？ | B：提供取消按钮，重试由精灵决策 | ✅ 已决议 |
| 3 | 成员只读面板是否支持"接管对话"？ | A（后续迭代考虑 B） | ✅ 已决议 |
| 4 | 精灵对话中是否显示历史团队列表？ | A：仅显示活跃团队 | ✅ 已决议 |
| 5 | Agent 复用上限？ | C：`ParallelConfig.MaxConcurrentTeams`（默认 3） | ✅ 已决议 |
| 6 | Session 树最大深度？ | C：`ParallelConfig.MaxSessionDepth`（默认 2） | ✅ 已决议 |
| 7 | 依赖团队自动启动还是精灵确认后启动？ | C（默认自动，关键任务需确认） | ✅ 已决议 |
| 8 | Synthesis Engine 用 LLM 合成还是模板合成？ | C（简单场景模板，复杂场景 LLM） | ✅ 已决议 |
| 9 | 编排拓扑缓存存储位置？ | A：AgentRuntimeSettings JSON | ✅ 已决议 |
| 10 | 并行团队间是否共享 Memory L3/L4？ | A：共享 | ✅ 已决议 |
| 11 | 团队超时后是否自动重试？ | B：仅通知精灵 | ✅ 已决议 |

---

## 9. 前端组件架构

### 9.1 组件清单

| 组件 | 路径 | 职责 | 状态 |
|------|------|------|------|
| SpiritEntry.vue | `components/spirit/` | 精灵入口卡片 | ✅ |
| SpiritStatusBar.vue | `components/spirit/` | 底部状态栏（复杂度+配额+Token+检查点+最近事件） | ✅ |
| TeamTaskCard.vue | `components/spirit/` | 侧边栏团队卡片（含 AgentStatusLabel + 脉冲） | ✅ |
| AgentStatusLabel.vue | `components/spirit/` | Agent 状态标签（7 种聚合状态） | ✅ |
| OrchestrationModeBadge.vue | `components/spirit/` | 编排模式标签（含 tooltip 理由） | ✅ |
| TeamProgressCard.vue | `components/spirit/` | 团队进度卡片（含依赖状态区分） | ✅ |
| ParallelTeamOverview.vue | `components/spirit/` | 并行团队概览 | ✅ |
| DAGDiagramCard.vue | `components/spirit/` | DAG 依赖图（含验证节点展示） | ✅ |
| SynthesisResultCard.vue | `components/spirit/` | 综合结果卡片（含成功率指标） | ✅ |
| TaskExecutionPanel.vue | `components/spirit/` | 任务执行面板（集成所有子组件） | ✅ |
| TeamAssemblyCard.vue | `components/spirit/` | 团队组建卡片 | ✅ |
| InterruptedTeamCard.vue | `components/spirit/` | 中断恢复提示卡片 | ✅ |
| TeamMemberTreeNode.vue | `components/spirit/` | 成员树形节点 | ✅ |
| MemberReadOnlyPanel.vue | `components/spirit/` | 成员只读面板 | ✅ |

### 9.2 数据流

```
API / Service（features/spirit/api.ts）
        ↓
Pinia Store（stores/spirit/index.ts — useSpiritTeamStore）
        ↓
Composable（features/chat/composables/useChatWorkspace.ts 等）
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
```

---

## 10. 验收标准索引

| ID | 摘要 | 阶段 | 状态 |
|----|------|------|------|
| SP-01 | 左侧列表仅显示精灵 + 团队树 | P0 | ✅ |
| SP-02 | 精灵区分简单/任务型对话 | P0 | ✅ |
| SP-03 | 团队卡片展示名称/状态/成员/进度/Agent标签 | P0+OBS | ✅ |
| SP-04 | 任务执行面板三区布局 | P0 | ✅ |
| SP-05 | 成员树形展开 + 状态 | P1 | ✅ |
| SP-06 | 成员只读面板（无输入框） | P1 | ✅ |
| SP-07 | 多任务并行 + Agent 复用隔离 | P0.5 | ✅ |
| SP-08 | 团队生命周期（归档/取消/重试） | P1 | ✅ |
| SP-09 | 面包屑导航 + 返回精灵 | P1 | ✅ |
| SP-10 | Session 数据 → 进化体系闭环 | P2 | — |
| SP-11 | 三阶段编排（Plan→Allocate→Orchestrate） | P0.5 | ✅ |
| SP-12 | DAG 编排图 + 并行团队概览 | P0.5 | ✅ |
| SP-13 | 综合结果合成 | P0.5 | ✅ |
| OBS-01 | 对话流自动折叠 | OBS P0 | ✅ |
| OBS-02 | 语境加载消息 | OBS P0 | ✅ |
| OBS-03 | Agent 状态标签 | OBS P0 | ✅ |
| OBS-04 | 底部状态栏 | OBS P1 | ✅ |
| OBS-05 | 侧边栏状态脉冲 | OBS P1 | ✅ |
| OBS-06 | 可折叠工具输出 | OBS P0 | ✅ |
| OBS-07 | 中断恢复提示 | OBS P1 | ✅ |
| SPO-01 | 多团队并行执行 | M60 P1 | ✅ |
| SPO-02 | 任务依赖调度 | M60 P2 | ✅ |
| SPO-03 | 编排模式智能选择 | M60 P2 | ✅ |
| SPO-04 | 多团队结果合成 | M60 P2 | ✅ |
| SPO-05 | 编排策略进化 | M60 P2 | ✅ |
| SPO-06 | 任务复杂度智能评估 | M60 P4 | ✅ |
| SPO-07 | Graph DAG 编排 | M60 P4 | ✅ |
| SPO-08 | 编排验证门禁 | M60 P4 | ✅ |

---

## 11. 遗留技术债

| ID | 描述 | 优先级 | 状态 |
|----|------|--------|------|
| TD-1 | api.ts 双键名兼容（teamKey/team_key） | P1 | ✅ 已修复（字段不存在） |
| TD-2 | ListSpiritTeams HTTP 端点未暴露 | P1 | ✅ 已暴露（GET /v1/spirit/{id}/teams） |
| TD-3 | ArchiveTeam RPC 未定义 | P1 | ✅ 已定义（POST /v1/teams/{id}/archive） |
| TD-4 | MemberReadOnlyPanel 占位符 | P1 | ✅ 已实现 |
| TD-5 | TeamMemberTreeNode 未实现 | P1 | ✅ 已实现 |
| TD-6 | 面包屑导航未实现 | P1 | ✅ 已实现 |
| TD-7 | 重试失败团队未实现 | P1 | ✅ 已实现（POST /v1/teams/{id}/retry） |
| TD-8 | DQ Score 前端展示 | P2 | ✅ 已实现（StatusBar + SynthesisCard） |
| TD-9 | 验证门禁结果前端展示 | P2 | ✅ 已实现（DAGDiagramCard） |
| TD-10 | 条件路由 UI 展示 | P2 | ✅ 已实现（Graph 编辑器） |

## 12. UX 人性化改进（2026-06-09 实施）

| ID | 改进项 | 优先级 | 状态 |
|----|--------|--------|------|
| UX-1 | TeamProgressCard 增加 ETA 预计完成时间 | P1 | ✅ 已实现 |
| UX-2 | MemberReadOnlyPanel 增加"返回精灵"快捷按钮 | P1 | ✅ 已实现 |
| UX-3 | TeamTaskCard 侧边栏增加实时耗时显示 | P1 | ✅ 已实现 |
| UX-4 | TeamTaskCard/TeamProgressCard 失败时显示错误摘要 | P1 | ✅ 已实现 |
| UX-5 | ToolStrip `<details>` → `q-expansion-item` 统一折叠动画 | P2 | ✅ 已实现 |
| UX-6 | SpiritStatusBar 并行配额迷你进度条 | P2 | ✅ 已实现 |
| UX-7 | ChatExecutionCard `aria-expanded`/`aria-controls` 无障碍 | P2 | ✅ 已实现 |
| UX-8 | Provide `readonly()` 运行时包装 signal | P3 | ✅ 已实现 |
| UX-9 | Summary fallback 语言改中文 | P3 | ✅ 已实现 |

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
