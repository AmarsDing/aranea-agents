# M59: Chat UI 优化 — 开发计划

> **版本**：2026-06-10 | **状态**：P0/P0.5/OBS-P0/OBS-P1/M60-P1/M60-P2/M60-P4/P1/P1.5 已完成 · 📋 P1.6 TK 批次规划中 · 📋 P2 规划中 · ✅ M69 P0/P1/P2/P3/P4 全部完成
> **需求**：[59-chat-ui-optimization.md](./59-chat-ui-optimization.md) · **设计**：[59-chat-ui-optimization.design.md](./59-chat-ui-optimization.design.md)
> **合并来源**：原 M59（精灵模式 + 可观测性 + 并行编排）+ M69（时间线展示 + 团队列表修复 + useAgentBlocks 业务逻辑审查）

---

## 1. 模块定位

Chat UI 优化：精灵为唯一对话入口，左侧列表重构为精灵 + 任务团队树，中间面板支持精灵对话/任务执行/成员只读三种模式。任务看板支持**树形嵌套展示**（任务-思考-工具-回复 统一结构 + sub_task_board 递归嵌入）。支持三阶段编排（Plan → Allocate → Orchestrate）、多团队并行执行、DAG 依赖调度、结果自动合成、编排策略进化。可观测性 UX 增强：对话流自动折叠、语境加载消息、Agent 状态标签、底部状态栏、侧边栏脉冲、中断恢复提示。智能增强：任务复杂度分级、Graph DAG 编排、自适应 Team 模式、编排验证门禁。useAgentBlocks 业务逻辑审查修复：状态机扩展、Reply 去重对齐、已完成回合默认展开、部分失败可观测化、progress section 独立渲染。

**核心展示模型**：
- 任何 agent 的对话看板内容统一为"任务-思考-工具-回复-思考-回复"
- 看板中可以嵌入子任务看板，所有任务看板的输出形式相同
- 主面板树形结构嵌套子任务面板
- 嵌套层级受 `MaxSessionDepth=2` 约束

---

## 2. 前置依赖

| 依赖 | 状态 | 说明 |
|------|------|------|
| 精灵 Agent 种子数据 | ✅ | `__spirit__` Agent 行 + Ownership=system_builtin + tools_profile=spirit |
| `plan_and_execute` 工具 | ✅ | 三阶段编排统一入口（Plan → Allocate → Orchestrate） |
| Session 树字段 | ✅ | ParentSessionID / RootSessionID / AgentDepth |
| ChatEntitySidebar 重构 | ✅ | 从 Agent/Team 平铺 → 精灵 + 团队树 |
| Team AutoCreated 字段 | ✅ | 区分精灵创建 vs 用户手动创建 |
| DAG 依赖字段 | ✅ | DAGNodeID / DependsOn |
| 并行配置 | ✅ | ParallelConfig（MaxConcurrentTeams / AutoArchiveSeconds 等） |
| `ChatExecutionCard` 已有折叠/展开 | ✅ | 使用 `<q-expansion-item>` |
| `tool_call`/`tool_result` Envelope 携带 AgentName | ✅ | `EnvelopeToolCall.AgentName`/`AgentKey`/`ActivityKind` |
| `AgentNodeStatus` 17 种状态已定义 | ✅ | `orchestration_status.go` |
| `SpiritMember.status` 字段存在 | ✅ | 类型为 `string`，典型值 idle/running/error |
| `OptionsJSON.team_member` 字段存在 | ✅ | 成员消息过滤基础 |
| `ResumeTeamRunExecution` API 存在 | ✅ | 需要 `graph_execution_id` |
| WS 事件回放机制 | ✅ | `lastEventId` + `onReplayState` 回调 |
| `groupMessagesByTurn` 分组 | ✅ | 需扩展 `isCompleted` 字段 |
| EvolutionUsecase | ✅ | DQ Score 计算和建议生成 |
| LearningLoopUsecase | ✅ | Pattern 检测和 Proposal 生成 |

---

## 3. 开发阶段

### Phase P0 — 核心骨架（M59 原有）

> **目标**：精灵为唯一入口，左侧列表重构，团队列表展示，任务执行面板基础版，Session 树关联。
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SP-BE-01 | Session Proto 扩展：parent_session_id / root_session_id / agent_depth | `api/kratos/session/v1` | `make api && make build` 通过 | ✅ |
| SP-BE-02 | Session Biz：ListByParentSessionID / GetRootSession 查询 | `internal/biz/session` | 单测通过 | ✅ |
| SP-BE-03 | Session Data：parent_session_id 索引 + 查询实现 | `internal/data` | 单测通过 | ✅ |
| SP-BE-04 | Team Proto 扩展：spirit_session_id / task_description / auto_created | `api/kratos/team/v1` | `make api && make build` 通过 | ✅ |
| SP-BE-05 | Team Biz：Create 支持 AutoCreated / SpiritSessionID | `internal/biz/team` | 单测通过 | ✅ |
| SP-BE-06 | spirit_team.go：AssembleTeam 流程 | `internal/service` | 集成测试通过 | ✅ |
| SP-BE-07 | chat.go：识别 `__spirit__` → buildSpiritTeam 路由 | `internal/service` | 精灵对话走 Team 路径 | ✅ → P0.5 重构 |
| SP-BE-08 | Event：spirit_team_assembled / completed / failed EnvelopeType | `internal/event` | 单测通过 | ✅ |
| SP-BE-09 | 精灵 Agent 种子数据 | `internal/data/seed` | 启动后精灵 Agent 可查 | ✅ |
| SP-FE-01 | `features/spirit/types.ts` + `api.ts` | `web/src/features/spirit` | 类型与 Proto 对齐 | ✅ |
| SP-FE-02 | `useSpiritTeamStore`：团队列表 + 面板模式 + 展开/折叠 | `web/src/stores/spirit` | Store 单测通过 | ✅ |
| SP-FE-03 | `SpiritEntry.vue`：精灵入口卡片 | `web/src/components/spirit` | 点击切换精灵对话 | ✅ |
| SP-FE-04 | `ChatEntitySidebar.vue` 重构：精灵 + 团队树 | `web/src/components/chat` | US-01 验收 | ✅ |
| SP-FE-05 | `TeamTaskCard.vue`：团队卡片 | `web/src/components/spirit` | US-03 验收 | ✅ |
| SP-FE-06 | `TaskExecutionPanel.vue` 基础版：概览 + 时间线 | `web/src/components/spirit` | US-04 验收 | ✅ |
| SP-FE-07 | `ChatMessagePanel.vue` 三模式切换 | `web/src/components/chat` | 精灵/团队/成员面板切换 | ✅ |
| SP-FE-08 | `TeamAssemblyCard.vue`：精灵对话中的团队组建卡片 | `web/src/components/spirit` | US-02 验收 | ✅ |

### Phase P0 — 致命 Bug 修复（M69 P0 合并）

> **目标**：修复团队列表数据加载缺失（致命 Bug），WS 重连后恢复团队数据。
> **状态**：✅ 已完成

| ID | 任务 | 影响文件 | 验收标准 | 状态 |
|----|------|----------|----------|------|
| T-01 | 在 useChatWorkspace 中添加 Spirit session 选择 watch，调用 loadSpiritTeams | `useChatWorkspace.ts` | AC-01：选择 Spirit session 后团队列表有数据 | ✅ |
| T-02 | 在 useChatWorkspace 中监听 wsReplaying 信号，重连后调用 reloadTeams | `useChatWorkspace.ts` | AC-02：WS 重连后团队列表恢复 | ✅ |
| T-03 | 在 session 切换时调用 spiritStore.reset() 清理旧数据 | `useChatWorkspace.ts` | AC-03：切换到非 Spirit session 时团队列表清空 | ✅ |

### Phase P0.5 — 三阶段编排

> **目标**：从 `assemble_team` 单步组建演进为 Plan → Allocate → Orchestrate 三阶段编排。
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| SP-BE-10 | 移除路由层拦截，精灵走 `runSingleAgentViaTRPC` + `spiritCustomTools` 注入 | `internal/service/chat_orchestrator_turn.go` | ✅ |
| SP-BE-11 | `plan_and_execute` 工具：三阶段统一入口 | `internal/tools/spirit_tools.go` | ✅ |
| SP-BE-12 | `check_progress` / `cancel_orchestration` / `synthesize_results` 工具 | `internal/tools/spirit_tools.go` | ✅ |
| SP-BE-13 | `build_orchestration_graph` 工具：DAG 图构建 | `internal/tools/orchestrator/build_graph.go` | ✅ |
| SP-BE-14 | `TaskPlannerPort` / `AgentAllocatorPort` / `TaskOrchestratorPort` 端口接口 | `internal/biz/` | ✅ |
| SP-BE-15 | `TaskDAG` 拓扑路由 | `internal/biz/spirit_task_dag.go` | ✅ |
| SP-BE-16 | `ParallelConfig`：并行配额 + 自动归档配置 | `internal/biz/spirit_parallel_config.go` | ✅ |
| SP-BE-17 | `AutoArchiveCompletedTeams` | `internal/biz/spirit_team_usecase.go` | ✅ |
| SP-BE-18 | `CancelTeam` + 级联依赖处理 | `internal/biz/spirit_team_usecase.go` | ✅ |
| SP-BE-19 | `TeamStarter`：团队生命周期管理 | `internal/service/spirit_team.go` | ✅ |
| SP-BE-20~22 | 三阶段编排事件 / Progress / AllCompleted 事件 | `internal/event/contract/envelope.go` | ✅ |
| SP-BE-23 | 旧工具标记 DEPRECATED | `internal/tools/spirit_tools.go` | ✅ |
| SP-FE-09~16 | 前端联合类型 + 进度卡片 + DAG 图 + 合成卡片 + 编排徽章 + Store 事件扩展 | `web/src/` | ✅ |

### Phase OBS-P0 — 核心可观测性

> **目标**：对话流自动折叠 + 语境加载消息 + 可折叠工具输出 + Agent 状态标签
> **状态**：✅ 已完成

| ID | 任务 | 状态 |
|----|------|------|
| OBS-FE-01~02 | `observabilityConstants.ts` + `spiritUi.ts` 状态聚合 | ✅ |
| OBS-FE-03~05 | `useAutoCollapse` + `ChatMessagePanel` 集成 | ✅ |
| OBS-FE-06~07 | `useContextualLoadingMessage` + 集成 | ✅ |
| OBS-FE-08~10 | `AgentStatusLabel` + TeamTaskCard/TaskExecutionPanel 集成 | ✅ |
| OBS-FE-11~13 | `ChatExecutionCard` 自动折叠 + 历史消息恢复 + WS 回放兼容 | ✅ |
| OBS-FE-14 | `TaskExecutionPanel` 集成 `ParallelTeamOverview` | ✅ |

### Phase OBS-P1 — 全局感知增强

> **目标**：底部状态栏 + 侧边栏脉冲 + 中断恢复提示
> **状态**：✅ 已完成

| ID | 任务 | 状态 |
|----|------|------|
| OBS-BE-01~02 | `spirit_team_completed` / `spirit_teams_all_completed` 增加 token 字段 | ✅ |
| OBS-FE-15~16 | `SpiritStatusBar.vue` + `ChatMessagePanel` 集成 | ✅ |
| OBS-FE-17~18 | `useStatusPulse` + `ChatEntitySidebar` 集成 | ✅ |
| OBS-FE-19~20 | `InterruptedTeamCard.vue` + `TaskExecutionPanel` 集成 | ✅ |
| OBS-FE-21 | 恢复执行 API 调用 | ✅ |

### Phase M60-P1 — 基础并行

> **目标**：移除单团队限制，支持多团队并行。
> **状态**：✅ 已完成

| ID | 任务 | 状态 |
|----|------|------|
| SPO-BE-01~14 | 后端：ListActiveTeams / ParallelConfig / TeamKey UUID / 工具改造 / 事件扩展 | ✅ |
| SPO-FE-01~05 | 前端：类型 / Store / 组件 / WS 事件 | ✅ |

### Phase M60-P2 — 智能编排

> **目标**：Task DAG 依赖调度、拓扑路由、Synthesis Engine、编排进化闭环。
> **状态**：✅ 已完成

| ID | 任务 | 状态 |
|----|------|------|
| SPO-BE-15~27 | 后端：TaskDAG / Topology / Synthesis / OrchestrationCache | ✅ |
| SPO-FE-06~09 | 前端：DAG 文本 / Synthesis 卡片 / 依赖调度 UI / 编排模式说明 | ✅ |
| SPO-INT-01~06 | P2 集成修复：Wire / Synthesize / DQ Score / Topology 路由 | ✅ |
| SPO-DP-01~07 | P2 深度业务实现 | ✅ |

### Phase M60-P4 — 智能增强

> **目标**：任务复杂度分级、Graph DAG 编排、自适应 Team 模式、编排验证门禁。
> **状态**：✅ 已完成

| ID | 任务 | 状态 |
|----|------|------|
| SPO-P4-01~10 | ComplexityRuleEngine / Spirit Prompt / chat_orchestrator_spirit / Graph DAG / 验证门禁 | ✅ |
| SPO-DR-S3~S5 / FS1~FS4 | 深度架构审查严重项修复 | ✅ |
| SPO-DR-M8/M11/M13 | 中等项修复 | ✅ |
| SPO-DR-L11/L17 | 轻微项修复 | ✅ |
| SPO-DR-WIRE | Wire 注入修复 | ✅ |
| SPO-RS-01~10 | 迭代建议修复 | ✅ |
| SPO-RR-01~08 | 二轮审查修复 | ✅ |

### Phase P1 — 交互增强

> **目标**：成员树/只读面板、面包屑导航、重试失败团队、手动归档 UI。
> **状态**：🔄 进行中

| ID | 任务 | 状态 |
|----|------|------|
| SP-BE-24 | `ListSpiritTeams` RPC + HTTP 端点 | ✅ |
| SP-BE-25 | `ArchiveTeam` RPC | ✅ |
| SP-BE-26 | `RetryTeam` RPC | ✅ |
| SP-FE-17~26 | 成员树 + 只读面板 + 消息过滤 + 复用标识 + 归档 UI + 重试 + 面包屑 + 双键名清理 | ✅ |

### Phase P1.5 — ChatExecutionCard 独立折叠增强

> **目标**：5s 耗时守卫、折叠摘要增强、全局展开/折叠两层联动。
> **状态**：✅ 已完成

| ID | 任务 | 状态 |
|----|------|------|
| SP-FE-27 | ChatExecutionCard 5s elapsed timer | ✅ |
| SP-FE-28 | ChatExecutionCard 折叠态摘要兜底 | ✅ |
| SP-FE-29 | ToolStrip 折叠态摘要增强 | ✅ |
| SP-FE-30 | Provide/Inject 全局控制 | ✅ |
| SP-FE-31 | `ToolUseEvent.expanded` 死代码清理 | ✅ |

### Phase P1.5-3 — Spirit 模式统一

> **目标**：TaskExecutionPanel 中的 ChatExecutionCard 自动响应全局控制信号。
> **状态**：✅ 已验证

| ID | 任务 | 状态 |
|----|------|------|
| SP-FE-30V | 验证 Spirit 模式（TaskExecutionPanel）中 ChatExecutionCard 响应全局展开/折叠信号 | ✅ |

### Phase P2 — 任务看板树形嵌套展示（M69 P1 合并）

> **目标**：实现任务看板树形嵌套展示模型（任务-思考-工具-回复 统一结构 + sub_task_board 递归嵌入）。
> **状态**：✅ 已完成
> **设计**：[59-chat-ui-optimization.design.md §6.2](./59-chat-ui-optimization.design.md)
> **纪律**：TDD — 先写失败测试，再写最小实现；两阶段审查（规格合规 + 代码质量）

#### P2 后端

> M69 P2 不涉及后端变更（M69 是纯前端 Chat UI 优化）。

#### P2 前端

| ID | 任务 | 影响文件 | 验收标准 | 状态 |
|----|------|----------|----------|------|
| T-04 | 定义 TimelineElement 类型和 timelineTypes.ts | `features/chat/timelineTypes.ts`(新) | 类型编译通过 | ✅ |
| T-05 | 扩展 useChatTimeline，新增 timelineElements 计算属性 | `useChatTimeline.ts` | timelineElements 正确拆解消息为时间线元素 | ✅ |
| T-06 | 实现 TimelineNode 展示组件 | `components/chat/TimelineNode.vue`(新) | 各类型节点正确渲染 | ✅ |
| T-07 | ChatMessageList 集成时间线渲染模式 | `ChatMessageList.vue` | TurnBlock 内使用时间线渲染 | ✅ |
| T-08 | Team 会话启用时间线模式 | `useChatTimeline.ts` | Team 会话显示时间线 | ✅ |
| T-09 | 折叠交互：thinking/action 完成后自动折叠 | `TimelineNode.vue` | 完成的步骤自动折叠 | ✅ |
| T-10 | TaskExecutionPanel 集成 TeamProgressCard | `TaskExecutionPanel.vue` | 团队进度卡片显示 | ✅ |
| T-11 | TaskExecutionPanel 集成 SynthesisResultCard | `TaskExecutionPanel.vue` | 综合结果区显示 | ✅ |
| T-12 | 左侧面板简化为精灵+团队树 | `ChatEntitySidebar.vue`, `ChatPage.vue` | Spirit 模式下不显示 Agent 列表 | ✅ |
| T-13 | 实现 TaskBoard 树形嵌套展示组件 | `TaskBoard.vue`(新), `TaskBoardNode.vue`(新) | 任务-思考-工具-回复 统一结构 + sub_task_board 递归 | ✅ |

#### P2 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | T-04 | TimelineElement 类型定义 | ✅ |
| 2 | T-05 | useChatTimeline timelineElements | ✅ |
| 3 | T-06 | TimelineNode 组件 | ✅ |
| 4 | T-07 | ChatMessageList 集成 | ✅ |
| 5 | T-08 | Team 会话启用时间线 | ✅ |
| 6 | T-09 | 折叠交互 | ✅ |
| 7 | T-10 | TeamProgressCard 集成 | ✅ |
| 8 | T-11 | SynthesisResultCard 集成 | ✅ |
| 9 | T-12 | 左侧面板简化 | ✅ |
| 10 | T-13 | TaskBoard 树形嵌套组件 | ✅ |

### Phase P3 — UI 原型对齐优化（M69 P3 合并）

> **目标**：基于 HTML 原型与需求提案核对验证后，修复 UI 差距。
> **状态**：✅ 已完成

| ID | 任务 | 涉及文件 | 验收标准 | 状态 |
|----|------|----------|----------|------|
| T-15 | TurnBlock 添加 Agent Block Header（头像+名称+状态徽章+耗时+子任务数） | `TurnBlock.vue` | ✅ 完成 | ✅ |
| T-16 | ChatExecutionCard 添加 agent 首字头像 | `ChatExecutionCard.vue` | ✅ 完成 | ✅ |
| T-17 | 运行中耗时添加 `...` 后缀 | `ChatExecutionCard.vue` | ✅ 完成 | ✅ |
| T-18 | ChatReasoningPeek 添加脉冲圆点和光标闪烁指示器 | `ChatReasoningPeek.vue` | ✅ 完成 | ✅ |
| T-19 | ChatExecutionCard running 状态添加脉冲圆点 | `ChatExecutionCard.vue` | ✅ 完成 | ✅ |
| T-20 | 全局展开/折叠按钮始终可见 | `ChatMessagePanel.vue` | ✅ 完成 | ✅ |
| T-21 | Sub-Agent 嵌套缩进（左边框线+缩进） | `TurnBlock.vue` | ✅ 完成 | ✅ |
| T-22 | 执行结果区段标签 | `TurnBlock.vue` | ✅ 完成 | ✅ |

### Phase P4 — useAgentBlocks 业务逻辑审查修复（M69 P4 合并）

> **目标**：静态代码审查 `useAgentBlocks.ts` 发现的 8 项业务逻辑问题修复（F-13~F-21 / AC-17~AC-25）。
> **状态**：✅ 已完成
> **设计**：[59-chat-ui-optimization.design.md §九](./59-chat-ui-optimization.design.md)
> **纪律**：TDD — 先写失败测试，再写最小实现；两阶段审查（规格合规 + 代码质量）

| 任务 ID | 任务 | 涉及文件 | 验收标准 | TDD 提示 | 状态 |
|--------|------|----------|----------|----------|------|
| T-23 | `SubAgentBuilder.addTool` 签名扩展为 `(msg, toolEv)`，推入 `allToolMsgs` | `useAgentBlocks.ts` | AC-17 | 测试：构造"子代理有 1 个未完成 tool"用例，断言 `status === 'running'` 而非 `completed` | ✅ |
| T-24 | 状态机扩展 `tool_blocked`，`computeAgentStatus` / `SubAgentBuilder.build` 显式返回 | `useAgentBlocks.ts` + `agentTreeTypes.ts` | AC-18 | 测试：构造"tool.status === 'blocked'"，断言 `status === 'tool_blocked'` | ✅ |
| T-25 | `resolvePlanStatus` 新增 `planEntriesCount` 参数，补 `running → executing` 转换 | `useAgentBlocks.ts` | AC-19 | 测试：`(planning, running, planEntries=3)` → `'executing'` | ✅ |
| T-26 | progress sortKey 钳制 `Math.max(0, offset) - 0.5` | `useAgentBlocks.ts` | AC-20 | 测试：构造 `startedAt < turnStartTs`，断言 sortKey ≥ 0 | ✅ |
| T-27 | Reply 去重改为 `presentation.mode !== 'react'` 单条件 | `useAgentBlocks.ts` | AC-21 | 测试：ReAct + finalAnswer 不去重；非 ReAct + reasoning==body 去重 | ✅ |
| T-28 | `PlanEntry` 新增 `agentKey` 字段；`updatePlanEntryStatuses` 改用 `agentKey` 匹配 | `useAgentBlocks.ts` + `agentTreeTypes.ts` | AC-22 | 测试：构造 2 个相似 task 的 sub-agent，断言 plan entry 与对应 block 正确配对 | ✅ |
| T-29 | AgentBlock 根 + SubAgentBuilder 的 `collapsed` 默认改为 `false` | `useAgentBlocks.ts` | AC-23 | 测试：构造 `status === 'completed'` 用例，断言 `collapsed === false` | ✅ |
| T-30 | AgentBlock 新增 `hasPartialFailure` 字段；`buildRootAgentBlock` 计算 | `useAgentBlocks.ts` + `agentTreeTypes.ts` | AC-24 | 测试：构造"1 failed tool + 1 successful assistant"，断言 `hasPartialFailure === true` | ✅ |
| T-31 | AgentBlock 新增 `progressSections` 字段；`timeline` 不再含 `kind: 'progress'`；`ChatMessageList` 渲染 progressSections 在 turn 头部 | `useAgentBlocks.ts` + `agentTreeTypes.ts` + `ChatMessageList.vue` | AC-25 | 测试：构造含 progress 的回合，断言 `block.timeline` 中无 `kind: 'progress'`；`block.progressSections.length > 0` | ✅ |
| T-32 | `TurnBlock.vue` 渲染 `hasPartialFailure === true` 徽章 | `TurnBlock.vue` | AC-24 | — | ✅ |
| T-33 | `ChatExecutionCard.vue` 渲染 `tool_blocked` 徽章 | `ChatExecutionCard.vue` | AC-18 | — | ✅ |
| T-34 | `useAgentBlocks.test.ts` 新增上述 8 项测试用例 | `useAgentBlocks.test.ts`（新） | T-23~T-31 测试通过 | TDD 红灯先写 | ✅ |

#### 依赖关系

```
T-23 ──→ T-24 ──→ T-33
T-25
T-26
T-27
T-28
T-29
T-30 ──→ T-32
T-31
T-34  ← TDD 套件
```

- T-23/T-24/T-25/T-26/T-27/T-28/T-29/T-30/T-31 可独立开发
- T-32/T-33 依赖对应数据字段落地
- T-34 在所有功能完成后补全测试覆盖率

#### 验证命令

```bash
cd web && pnpm lint && pnpm test -- useAgentBlocks && pnpm build
```

> **范围约束**：本 Phase 不修改 `SynthesisResultCard.vue` 或 `ChatMessagePanel.vue` 的卡片互斥逻辑。`AgentBlock.result` 降级为兼容字段，timeline 末尾的 `kind: 'reply'` 作为最终答案的单一来源。

### Phase P1.6 — TODO 看板 + 工具时间线（TK 批次，M59 原有）

> **目标**：解决"代办管理工具多次 stuck"和"工具调用堆叠难追踪"两个痛点。
> **状态**：📋 规划中

#### P1.6 后端

| ID | 任务 | 状态 |
|----|------|------|
| SP-BE-27 | `todo_write` 工具调用 result 平铺 | 📋 |
| SP-BE-28 | `stuckToolResultReason` 文案改为可配置 i18n 文案 | 📋 |
| SP-BE-29 | `EnvelopeToolCall` `error_code` 字段校验 | 📋 |
| SP-BE-30 | `todo_write` 工具 LLM Prompt 检查：拦截把 todos 参数错给非 todo 工具的请求（**根因修复**） | 📋 |

#### P1.6 前端

| ID | 任务 | 状态 |
|----|------|------|
| TK-FE-01~02 | 类型扩展 + `useTodoBoard.ts` composable | 📋 |
| TK-FE-03~05 | `TodoCard` / `TodoColumn` / `TodoKanbanBoard` 组件 | 📋 |
| TK-FE-06 | `ChatMessagePanel.vue` 集成看板 | 📋 |
| TK-FE-07 | i18n 新增键（zh-CN + en-US） | 📋 |
| TK-FE-08 | `isStuckTool.ts` 工具函数 | 📋 |
| TK-FE-09~12 | `useToolCallTimeline` composable + 组件 + TurnBlock 集成 | 📋 |
| TK-FE-13~14 | `ToolStuckBadge.vue` + TaskExecutionPanel 集成 | 📋 |
| TK-FE-15~17 | 单元测试 + 端到端验证 | 📋 |
| **TK-FE-18** | `useUiConfigStore` + `TOOL_DISPLAY_KEY` 注入（TK-04） | 📋 |
| **TK-FE-19** | `UiConfigToggle.vue` + ChatMessagePanel 顶部集成（TK-04） | 📋 |
| **TK-FE-20** | `detectCodeLanguage.ts` + highlight.js 注册（TK-05） | 📋 |
| **TK-FE-21** | `CodeBlock.vue` 组件（自动检测 + 高亮 + 复制 + 折叠）（TK-05） | 📋 |
| **TK-FE-22** | `MarkdownView.vue` 集成 CodeBlock（TK-05） | 📋 |
| **TK-FE-23** | 思考节点流式 + 完成态折叠细化（TK-06） | 📋 |
| **TK-FE-24** | i18n 键补齐：`uiConfig` / `codeBlock`（TK-04/05） | 📋 |
| **TK-FE-25** | 关闭工具时降级单测（TK-04） | 📋 |
| **TK-FE-26** | CodeBlock 组件单测（TK-05） | 📋 |

#### P1.6 验收标准

- [ ] 精灵执行 `todo_write` 后，看板在 1s 内出现
- [ ] 看板三列内容正确（pending / in_progress / completed）
- [ ] 任务切换状态时对应列脉冲 0.8s
- [ ] 折叠态默认只显示 1 行摘要
- [ ] 未调用过 `todo_write` 时看板不占布局
- [ ] turn.tools.length >= 2 时切换到时间线
- [ ] turn.tools.length < 2 时保留原 ChatExecutionCard
- [ ] 时间线节点按 occurred_at 升序，同 ms 按 id 兜底
- [ ] 全局"展开/折叠"按钮作用于时间线
- [ ] running 节点不响应"折叠全部"
- [ ] 团队组建/完成/中断卡片不进入时间线
- [ ] `error_code === 'tool_timeout'` 节点显示"工具无返回结果"
- [ ] `TaskExecutionPanel` 顶部显示"⚠ N 工具未返回"徽章
- [ ] ChatPanel 顶部开关按钮可切换工具显示（TK-04）
- [ ] 关闭工具显示后，ChatExecutionCard / ToolCallTimeline / ToolStrip / ToolStuckBadge 全部不渲染（TK-04）
- [ ] 关闭后 TodoKanbanBoard、思考节点、纯文本回复仍正常显示（TK-04）
- [ ] 开关状态持久化到 localStorage，刷新后保留（TK-04）
- [ ] 代码块自动检测语言，未指定时调用 highlightAuto（TK-05）
- [ ] 代码块 12 种候选语言外的代码 fallback 到 plaintext（TK-05）
- [ ] 代码 >10KB 跳过 auto 检测，性能保护（TK-05）
- [ ] 代码 >20 行默认折叠，点击展开（TK-05）
- [ ] 复制按钮 2s 反馈"已复制"（TK-05）
- [ ] 思考节点流式态有脉冲边框 + 闪烁光标（TK-06）
- [ ] 思考节点完成态自动折叠为 span，点击展开（TK-06）
- [ ] 思考节点字体/字号/颜色与主文本区分但与回复文本同字号（TK-06）
- [ ] i18n zh-CN + en-US 翻译完整
- [ ] 虚拟滚动回收后看板/时间线状态正确恢复
- [ ] `cd web && pnpm lint && pnpm test && pnpm build` 通过

#### P1.6 任务依赖图

```
TK-FE-18 (useUiConfigStore)
  ├─→ TK-FE-19 (UiConfigToggle) ─→ TK-FE-25 (关闭降级单测)
  └─→ 各消费方（ChatMessageList/TurnBlock/SpiritStatusBar）v-if 适配

TK-FE-20 (detectCodeLanguage)
  └─→ TK-FE-21 (CodeBlock.vue) ─→ TK-FE-22 (MarkdownView 集成) ─→ TK-FE-26 (单测)

TK-FE-23 (思考节点细化)
  └─→ 与 TaskBoardNode 集成（已有 M69 P2 基础）

TK-FE-24 (i18n) — 与上述三项并行，最后补齐
```

### Phase P2 — 进化闭环（M59 原有）

> **目标**：Session 数据 → 技能/记忆/编排分析，Agent 能力画像 → 团队组建优化。

| ID | 任务 | 状态 |
|----|------|------|
| SP-EVO-01~06 | Session 执行轨迹 → 技能管家 / 记忆管家 / 编排效率分析 / Agent 画像 / 知识图谱 / 失败模式分析 | 📋 |

---

## 4. 验收标准

### Phase P0（核心骨架）

- [x] `make api && make wire && make build` 通过
- [x] `go test ./internal/biz/session/... ./internal/service/... -count=1` 通过
- [x] 精灵 Agent 种子数据启动后可查
- [x] 左侧列表仅显示精灵 + 团队（US-01）
- [x] 精灵区分简单/任务型对话（US-02）
- [x] 团队卡片展示名称/状态/成员/进度（US-03）
- [x] 任务执行面板三区布局（US-04）
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase P0（M69 致命 Bug 修复）

- [x] AC-01：选择 Spirit session 后，左侧面板显示团队列表
- [x] AC-02：WS 重连后团队列表自动恢复
- [x] AC-03：切换到非 Spirit session 时团队列表清空

### Phase P0.5（三阶段编排）

- [x] 精灵走 `runSingleAgentViaTRPC`，不再硬编码路由
- [x] `plan_and_execute` 三阶段编排工具可调用
- [x] DAG 拓扑自动路由（parallel / sequential / hybrid / coordinator）
- [x] 并行团队支持（ParallelConfig + DAG 依赖调度）
- [x] 综合结果合成（synthesize_results + SynthesisResultCard）
- [x] 自动归档已完成团队（AutoArchiveCompletedTeams）
- [x] 取消团队 + 级联依赖处理
- [x] 三阶段编排事件
- [x] 团队进度和全完成事件
- [x] 前端联合类型约束
- [x] `make api && make wire && make build` 通过
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase OBS-P0（核心可观测性）

- [x] 已完成工具调用卡片自动折叠为单行摘要
- [x] 已完成团队组建/完成卡片自动折叠
- [x] interrupted 状态折叠显示 ⏸ 标记
- [x] "展开全部"按钮可用
- [x] 三阶段编排过程显示语境加载消息
- [x] Agent 级语境消息显示"{agent_name} 正在{display_label}…"
- [x] WS 回放期间语境消息静默
- [x] 侧边栏团队卡片显示 Agent 状态色点
- [x] 任务执行面板显示 7 种 Agent 状态标签
- [x] Active 状态标签有呼吸动画
- [x] ChatExecutionCard completed/failed 时自动折叠
- [x] running 状态工具调用始终展开
- [x] 加载历史消息时已完成工具默认折叠
- [x] TaskExecutionPanel 集成 ParallelTeamOverview 三区布局
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase OBS-P1（全局感知增强）

- [x] 底部状态栏显示活跃团队数/中断数/配额/Token
- [x] 底部状态栏固定 24px，不随内容滚动
- [x] 侧边栏团队卡片状态变化时脉冲高亮
- [x] 脉冲颜色和时长正确
- [x] WS 回放期间脉冲禁用
- [x] interrupted 团队显示恢复提示卡片
- [x] "恢复执行"按钮调用 ResumeTeamRunExecution API
- [x] 不支持断点恢复的团队显示禁用提示
- [x] `make api && make wire && make build` 通过
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase M60-P1（基础并行）

- [x] 同一精灵 Session 可创建多个并行团队
- [x] 并行度超限时精灵提示用户等待
- [x] 团队进度实时监控 + 精灵主动通知
- [x] 取消团队 + 释放配额
- [x] `make api && make wire && make build` 通过
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase M60-P2（智能编排）

- [x] Task DAG 依赖调度正确执行
- [x] 拓扑路由自动选择编排模式
- [x] Synthesis Engine 结果合成
- [x] DQ Score 驱动编排缓存
- [x] 编排策略进化闭环

### Phase M60-P4（智能增强）

- [x] `assess_complexity` 工具正确评估 simple/moderate/complex 三级
- [x] Spirit 强制先调用 assess_complexity 再路由
- [x] Team 模式选择：simple→Direct, moderate→Direct, complex→Coordinator
- [x] `build_orchestration_graph` 生成正确的 Graph DAG
- [x] 验证节点注入：output_format/task_completion/human_approval
- [x] `make api && make wire && make build` 通过
- [x] `go test ./internal/biz/... ./internal/service/... ./internal/tools/... -count=1` 通过

### Phase P1（交互增强）

- [x] 成员树形展开 + 状态
- [x] 成员只读面板无输入框
- [x] Agent 复用标识可见
- [x] 团队归档/重试/放弃
- [x] 面包屑导航 + 返回精灵
- [x] ListSpiritTeams API 闭环
- [x] ListChildSessions RPC Proto 定义 + Service 实现
- [x] SynthesizeResults RPC Proto 定义 + Service 实现
- [x] ArchiveTeam RPC Proto 定义 + Service 实现
- [x] RetryTeam RPC Proto 定义 + Service 实现
- [x] api.ts 双键名兼容清理
- [x] spirit_teams_all_completed 载荷分项统计
- [x] SpiritTeamMode 前后端语义映射确认对齐

### Phase P1.5（ChatExecutionCard 折叠增强）

- [x] 工具运行 ≥5s 时显示实时计时器，≥60s 变为警告色
- [x] `started_at` 为空时降级 `occurred_at` → `Date.now()`
- [x] 折叠态摘要兜底：后端未提供 summary 时前端生成
- [x] ToolStrip 折叠态显示工具类型分布
- [x] 全局"展开全部/折叠全部"同时作用于 TurnBlock + ChatExecutionCard
- [x] 运行中工具不受"折叠全部"影响
- [x] Spirit 模式 ChatExecutionCard 同样响应全局控制
- [x] ToolUseEvent.expanded 死代码清理

### Phase P2（M69 时间线树形嵌套展示）

- [x] 任务看板树形嵌套展示（任务-思考-工具-回复 统一结构）
- [x] sub_task_board 节点递归渲染（受 MaxSessionDepth=2 约束）
- [x] 单 Agent 会话按时间线展示思考-动作-总结-结束
- [x] Team 会话按时间线展示思考-动作-总结-结束
- [x] 思考和动作元素完成后自动折叠
- [x] TaskExecutionPanel 展示三区布局
- [x] 左侧面板仅显示精灵+团队树
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase P3（UI 原型对齐优化）

- [x] TurnBlock 显示 Agent Block Header
- [x] ChatExecutionCard 显示 agent 首字头像
- [x] 运行中工具耗时显示 `...` 后缀
- [x] 思考流式输出显示脉冲圆点指示器和光标闪烁
- [x] 运行中工具卡片显示脉冲圆点指示器
- [x] 全局展开/折叠按钮始终可见（右对齐）
- [x] Sub-Agent 嵌套缩进
- [x] 执行结果区段显示"📊 执行结果"标签

### Phase P4（useAgentBlocks 业务逻辑修复）

- [x] AC-17：子代理工具未完成时不被误判为 `completed`
- [x] AC-18：`tool_blocked` 显式状态，UI 显示"等待您的输入"徽章
- [x] AC-19：PlanCard 在 subagents_spawn 路径下从"规划中"转为"执行中"
- [x] AC-20：progress sortKey 钳制，时钟漂移不会插到 user 之前
- [x] AC-21：ReAct 模式下 reply 去重与 `resolveReplyContent` 语义对齐
- [x] AC-22：Plan entry 匹配改用 `agentKey`
- [x] AC-23：已完成回合 `collapsed: false`（默认展开）
- [x] AC-24：`hasPartialFailure` 字段，UI 显示"⚠️ 部分工具失败"徽章
- [x] AC-25：progress section 移到 turn 头部，与 timeline 主线视觉分离
- [x] `useAgentBlocks.test.ts` 8 项测试用例通过
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase P1.6（TODO 看板 + 工具时间线）

- [ ] 精灵执行 `todo_write` 后，看板在 1s 内出现
- [ ] 看板三列内容正确
- [ ] 任务切换状态时对应列脉冲 0.8s
- [ ] 折叠态默认只显示 1 行摘要
- [ ] 未调用过 `todo_write` 时看板不占布局
- [ ] turn.tools.length >= 2 时切换到时间线
- [ ] turn.tools.length < 2 时保留原 ChatExecutionCard
- [ ] 时间线节点按 occurred_at 升序
- [ ] 全局"展开/折叠"按钮作用于时间线
- [ ] running 节点不响应"折叠全部"
- [ ] 团队组建/完成/中断卡片不进入时间线
- [ ] `error_code === 'tool_timeout'` 节点显示"工具无返回结果"
- [ ] `TaskExecutionPanel` 顶部显示"⚠ N 工具未返回"徽章
- [ ] **TK-04** ChatPanel 顶部开关按钮可切换工具显示
- [ ] **TK-04** 关闭后 ChatExecutionCard / ToolCallTimeline / ToolStrip / ToolStuckBadge 全部不渲染
- [ ] **TK-04** 关闭后 TodoKanbanBoard、思考节点、纯文本回复仍正常显示
- [ ] **TK-04** 开关状态持久化到 localStorage，刷新后保留
- [ ] **TK-05** 代码块自动检测语言，未指定时调用 highlightAuto
- [ ] **TK-05** 12 种候选语言外的代码 fallback 到 plaintext
- [ ] **TK-05** 代码 >10KB 跳过 auto 检测
- [ ] **TK-05** 代码 >20 行默认折叠，点击展开
- [ ] **TK-05** 复制按钮 2s 反馈"已复制"
- [ ] **TK-06** 思考节点流式态有脉冲边框 + 闪烁光标
- [ ] **TK-06** 思考节点完成态自动折叠为 span，点击展开
- [ ] **TK-06** 思考节点字体/字号/颜色与回复文本同字号但低一档亮度
- [ ] i18n zh-CN + en-US 翻译完整
- [ ] 虚拟滚动回收后看板/时间线状态正确恢复
- [ ] `cd web && pnpm lint && pnpm test && pnpm build` 通过

---

## 5. 任务板

### P0 任务板（M59 原有 + M69 修复）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SP-BE-01 | Session Proto 扩展 | ✅ |
| 2 | SP-BE-02 | Session Biz 树查询 | ✅ |
| 3 | SP-BE-03 | Session Data 查询实现 | ✅ |
| 4 | SP-BE-04 | Team Proto 扩展 | ✅ |
| 5 | SP-BE-05 | Team Biz AutoCreated | ✅ |
| 6 | SP-BE-06 | spirit_team.go AssembleTeam | ✅ |
| 7 | SP-BE-07 | chat.go 精灵路由 | ✅ |
| 8 | SP-BE-08 | Event 新增 EnvelopeType | ✅ |
| 9 | SP-BE-09 | 精灵 Agent 种子数据 | ✅ |
| 10 | SP-FE-01 | 前端 types + api | ✅ |
| 11 | SP-FE-02 | useSpiritTeamStore | ✅ |
| 12 | SP-FE-03 | SpiritEntry.vue | ✅ |
| 13 | SP-FE-04 | ChatEntitySidebar 重构 | ✅ |
| 14 | SP-FE-05 | TeamTaskCard.vue | ✅ |
| 15 | SP-FE-06 | TaskExecutionPanel.vue | ✅ |
| 16 | SP-FE-07 | ChatMessagePanel 三模式 | ✅ |
| 17 | SP-FE-08 | TeamAssemblyCard.vue | ✅ |
| 18 | T-01 | useChatWorkspace Spirit session watch | ✅ |
| 19 | T-02 | useChatWorkspace wsReplaying watch | ✅ |
| 20 | T-03 | useChatWorkspace session 切换 reset | ✅ |

### P2 任务板（M69 时间线树形嵌套展示）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | T-04 | TimelineElement 类型定义 | ✅ |
| 2 | T-05 | useChatTimeline timelineElements | ✅ |
| 3 | T-06 | TimelineNode 组件 | ✅ |
| 4 | T-07 | ChatMessageList 集成 | ✅ |
| 5 | T-08 | Team 会话启用时间线 | ✅ |
| 6 | T-09 | 折叠交互 | ✅ |
| 7 | T-10 | TeamProgressCard 集成 | ✅ |
| 8 | T-11 | SynthesisResultCard 集成 | ✅ |
| 9 | T-12 | 左侧面板简化 | ✅ |
| 10 | T-13 | TaskBoard 树形嵌套组件 | ✅ |

### P4 任务板（M69 useAgentBlocks 业务逻辑修复）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | T-23 | SubAgentBuilder.addTool 签名扩展 | ✅ |
| 2 | T-24 | 状态机扩展 tool_blocked | ✅ |
| 3 | T-33 | ChatExecutionCard 渲染 tool_blocked 徽章 | ✅ |
| 4 | T-25 | resolvePlanStatus 新增 planEntriesCount | ✅ |
| 5 | T-26 | progress sortKey 钳制 | ✅ |
| 6 | T-27 | Reply 去重改为 presentation.mode 条件 | ✅ |
| 7 | T-28 | PlanEntry 新增 agentKey | ✅ |
| 8 | T-29 | AgentBlock 根 collapsed 默认 false | ✅ |
| 9 | T-30 | AgentBlock hasPartialFailure 字段 | ✅ |
| 10 | T-32 | TurnBlock 渲染 hasPartialFailure 徽章 | ✅ |
| 11 | T-31 | AgentBlock progressSections 字段 | ✅ |
| 12 | T-34 | useAgentBlocks.test.ts 测试套件 | ✅ |

### P1.6 任务板（M59 原有 TK 批次）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SP-BE-29 | EnvelopeToolCall error_code 字段校验 | 📋 |
| 2 | SP-BE-28 | stuckToolResultReason i18n 化 | 📋 |
| 3 | SP-BE-27 | todo_write result 平铺 | 📋 |
| 4 | SP-BE-30 | Prompt 检查拦截 LLM 误用工具（根因） | 📋 |
| 5 | TK-FE-01 | 类型扩展 | 📋 |
| 6 | TK-FE-07 | i18n 键补齐 | 📋 |
| 7 | TK-FE-08 | isStuckTool 工具函数 | 📋 |
| 8 | TK-FE-02 | useTodoBoard composable | 📋 |
| 9 | TK-FE-03 | TodoCard 组件 | 📋 |
| 10 | TK-FE-04 | TodoColumn 组件 | 📋 |
| 11 | TK-FE-05 | TodoKanbanBoard 组件 | 📋 |
| 12 | TK-FE-06 | ChatMessagePanel 集成看板 | 📋 |
| 13 | TK-FE-09 | useToolCallTimeline composable | 📋 |
| 14 | TK-FE-10 | ToolCallTimelineItem 组件 | 📋 |
| 15 | TK-FE-11 | ToolCallTimeline 组件 | 📋 |
| 16 | TK-FE-12 | TurnBlock/ChatMessageRow 集成时间线 | 📋 |
| 17 | TK-FE-13 | ToolStuckBadge 组件 | 📋 |
| 18 | TK-FE-14 | TaskExecutionPanel 集成徽章 | 📋 |
| 19 | TK-FE-15 | composable/lib 单测 | 📋 |
| 20 | TK-FE-16 | 组件单测 | 📋 |
| 21 | TK-FE-17 | 端到端验证 | 📋 |
| **22** | **TK-FE-18** | **useUiConfigStore + TOOL_DISPLAY_KEY（TK-04）** | 📋 |
| **23** | **TK-FE-19** | **UiConfigToggle.vue + ChatMessagePanel 集成（TK-04）** | 📋 |
| **24** | **TK-FE-20** | **detectCodeLanguage.ts + highlight.js 注册（TK-05）** | 📋 |
| **25** | **TK-FE-21** | **CodeBlock.vue 组件（TK-05）** | 📋 |
| **26** | **TK-FE-22** | **MarkdownView.vue 集成 CodeBlock（TK-05）** | 📋 |
| **27** | **TK-FE-23** | **思考节点流式 + 折叠细化（TK-06）** | 📋 |
| **28** | **TK-FE-24** | **i18n 键补齐（uiConfig/codeBlock，TK-04/05）** | 📋 |
| **29** | **TK-FE-25** | **关闭工具降级单测（TK-04）** | 📋 |
| **30** | **TK-FE-26** | **CodeBlock 组件单测（TK-05）** | 📋 |

### P1.6 任务分层（按依赖深度）

#### 零依赖（可并行启动）
- TK-FE-01（类型扩展）、TK-FE-08（isStuckTool）、TK-FE-18（useUiConfigStore）、TK-FE-20（detectCodeLanguage）、TK-FE-23（思考节点细化）

#### 一层依赖
- TK-FE-02 (← TK-FE-01)
- TK-FE-19 (← TK-FE-18)
- TK-FE-21 (← TK-FE-20)
- TK-FE-25 (← TK-FE-19)

#### 二层依赖
- TK-FE-03, TK-FE-04 (← TK-FE-01, TK-FE-02)
- TK-FE-09, TK-FE-10 (← TK-FE-01, TK-FE-08)
- TK-FE-22 (← TK-FE-21, TK-FE-24)
- TK-FE-26 (← TK-FE-22, TK-FE-21)

#### 三层依赖（集成）
- TK-FE-05 (← TK-FE-03, TK-FE-04)
- TK-FE-06 (← TK-FE-05, TK-FE-07)
- TK-FE-11 (← TK-FE-09, TK-FE-10)
- TK-FE-12 (← TK-FE-11, TK-FE-08)
- TK-FE-14 (← TK-FE-13, TK-FE-12)

#### 收尾
- TK-FE-07、TK-FE-16、TK-FE-17（与 i18n 同步）

---

## 6. 依赖与风险

| 风险 | 缓解 |
|------|------|
| 精灵 Agent 种子数据与现有 Agent 冲突 | `agent_key=__spirit__` + `Ownership=system_builtin` 唯一约束 |
| ChatEntitySidebar 重构影响现有 Chat 功能 | feature flag 控制精灵/专家模式切换 |
| Session 树查询性能 | `parent_session_id` 加索引；深度限制 2 层 |
| Agent 复用时 L3/L4 共享导致记忆污染 | L3/L4 只读共享，写入仍按 Session 隔离 |
| WS 事件量增大（每个团队独立 WS） | 复用 Team Session WS，前端按 team_id 过滤 |
| 团队自动归档误删 | 归档仅移除列表显示，Session/TeamRun 保留 |
| 前端 `/v1/spirit/{id}/teams` API 断裂 | P1 阶段补齐 ListSpiritTeams HTTP 端点 |
| 旧工具 DEPRECATED 但仍可调用 | 旧工具委托到新流程，不删除确保向后兼容 |
| 多 Team 并行导致 Token 配额耗尽 | `MaxConcurrentTeams` 硬限制 + 配额预检 |
| 多 Team 同时写数据库导致锁竞争 | 读写分离 + 乐观锁 |
| 团队间隐式依赖导致结果不一致 | Task DAG 显式声明依赖 + 拓扑路由 |
| 前端 WS 消息风暴（多 Team 同时推送） | 事件聚合 + 节流 + 按团队分组 |
| Session 树过深导致上下文丢失 | `MaxSessionDepth=2` 强制限制 |
| 编排进化策略退化 | `GuardrailMaxChangePerPeriod` 约束 + DQ Score < 0.3 回滚 |
| Synthesis Engine LLM 调用增加成本 | 简单场景用模板合成，仅复杂场景调用 LLM |
| 规则引擎覆盖不全导致误判 | P0 使用安全默认值 moderate；P1 引入历史数据优化 |
| Graph DAG 生成不合理 | P0 保留 assemble_team 回退；P1 增加模板缓存 |
| `TurnBlockGroup.isCompleted` 计算影响消息分组性能 | 仅在 block 内工具状态变化时重新计算，使用 computed 缓存 |
| `ChatExecutionCard` 自动折叠可能干扰用户正在查看的工具 | 仅在用户未主动展开该卡片时自动折叠 |
| WS 回放期间语境消息闪烁 | 统一通过 `isReplaying` ref 控制 |
| `AgentNodeStatus` 数据源可能延迟 | 侧边栏使用 `SpiritMember.status`（实时），执行面板使用 `AgentNodeStatus` |
| Token 统计事件扩展需后端修改 | P1 阶段实施 |
| `ResumeTeamRunExecution` 需 `graph_execution_id` | 无 Graph 执行的团队显示"不支持断点恢复" |
| **M69 P4 风险**：子代理状态机变更可能影响现有 UI 状态展示 | 状态机扩展前向兼容（保留 running 状态语义），新状态作为可选展示 |
| **M69 P4 风险**：progress 移到 turn 头部可能影响现有 timeline 渲染 | `ChatMessageList` 双模式兼容（带 progressSections / 不带），分阶段切换 |
| **M69 P4 风险**：已完成回合默认展开可能让首屏变高 | 全局折叠按钮兜底，用户可一键折叠 |
| **M69 P2 风险**：任务看板树形嵌套增加渲染复杂度 | 虚拟滚动已支持；超过 50 节点触发默认折叠态 |
| **M69 P2 风险**：嵌套深度过深导致 UI 堆叠 | 受 `MaxSessionDepth=2` 约束，深度可控 |
| TK 工具调用 stuck 误判 | `isStuckTool` 只判 `error_code === 'tool_timeout'`，不解析 `ResultJSON` 文本 |
| TK 看板仅根精灵 todo | P1.6 范围限制；分支 todo 走 TD-TK-5 后续 |
| TK 时间线与 TeamAssemblyCard 视觉割裂 | 时间线只对 `turn.tools` 启用 |
| **TK-04 风险**：开关状态在多 Tab 间不同步 | 使用 `localStorage` `storage` 事件监听 + 单例 Pinia store |
| **TK-04 风险**：关闭后开发/调试无法看到工具 | 快捷键 `Ctrl+Alt+T` 临时切换（dev-only），生产构建移除 |
| **TK-05 风险**：highlight.js bundle 体积膨胀 | 仅注册 12 种常用语言（~80KB gzip），不全量引入 |
| **TK-05 风险**：hljs.highlightAuto 在长代码上耗时 | >10KB 跳过 auto，仅渲染 plaintext（性能保护） |
| **TK-05 风险**：MarkdownView 解析 `<pre>` 后丢失原始 ref | 在 markdown-it 渲染时嵌入 `data-lang` 属性，CodeBlock 通过 ref 接管 |
| **TK-06 风险**：流式思考节点可能引发布局抖动 | 使用 `contain: layout` 容器 + 固定高度占位 |
| **TK-06 风险**：折叠 summary 截断丢失重要前缀 | 截取第一个句号前，保留语义完整；>60 字加 `…` |

---

## 7. 关联文档更新

| 文档 | 更新内容 | 时机 |
|------|---------|------|
| [1-chat.md](./1-chat.md) | 新增精灵模式章节 | P0 |
| [10-session.md](./10-session.md) | Session 树状模型 | P0 |
| [11-multi-agent.md](./11-multi-agent.md) | 精灵自动创建 Team + 并行编排扩展 | P0 / M60-P1 |
| [architecture-blueprint.md](../architecture-blueprint.md) | 精灵模块卡片 + SPO 模块卡片 | P0 / M60-P1 |
| [module-cross-reference.md](../module-cross-reference.md) | M59 + M60 模块卡片 | P0 / M60-P1 |
| [7-agent-evolution.md](./7-agent-evolution.md) | 进化指标数据源扩展 + 编排进化闭环 | P2 / M60-P2 |
| [memory/L4.md](./memory/L4.md) | 协作关系 → 实体-关系 | P2 |
| [23-tools.md](./23-tools.md) | `isLongRunning` UI 联动 / stuck 工具检测 | P1.6 |

---

## 8. 审查修复记录

### 2026-06-01 P0 Review 修复（M59 原有）

> 修复范围：S1 / S2 / S3 / M1 / M2 / M3 / M4 / M5 / M6 / M8

| 编号 | 问题 | 修复方案 | 状态 |
|------|------|---------|------|
| S1 | 每次 Turn 都创建 Team | 移除路由拦截，精灵走 `runSingleAgentViaTRPC` | ✅ |
| S2 | Composer 不渲染 | `v-if="!panelMode \|\| panelMode === 'spirit'"` | ✅ |
| S3 | Completed/Failed 事件未实现 | `team_turn_hooks.go` 发布生命周期事件 | ✅ |
| M1 | appendChildSessionID 并发保护 | 移除 `appendChildSessionID` 方法 | ✅ |
| M2 | child_session_ids 冗余 | 移除，统一使用 `ListByParentSessionID` 查询 | ✅ |
| M3 | spiritAssembler 静默降级 | 移除 `spiritAssembler` 字段和路由拦截逻辑 | ✅ |
| M4 | SpiritTeam 类型约束 | 定义联合类型 | ✅ |
| M5 | 类型导入红线 | 统一从 `types.ts` 导入类型 | ✅ |
| M6 | API 双键名兼容 | 部分清理，仍有残留（TD-1） | ⚠️ → P1 清理 |
| M8 | Team Schema 索引 | 添加 `idx_teams_spirit_session` 复合索引 | ✅ |

### 2026-06-06 P0.5 三阶段编排实施

关键变更：移除路由拦截 → `plan_and_execute` 三阶段工具 → DAG 拓扑路由 → 并行配置 → 自动归档 → 取消+级联 → TeamStarter 生命周期 → 三阶段事件。

### 2026-06-08 OBS Review 修复

**阻断项**：OBS-R01~R07
**建议项**：OBS-S01~S07

### M60 深度架构审查修复

> 修复范围：S3~S5 严重项 + FS1~FS4 前端严重项 + M8/M11/M13 中等项 + L11/L17 轻微项 + Wire 注入修复

### M60 迭代建议修复（RS-01~RS-10, RR-01~RR-08）

### 2026-06-09 审查修复记录

> 修复范围：死代码删除 + A2A/OpenAI 端点 Bug 修复 + 注释修正 + 前端类型安全增强

| ID | 修复摘要 | 状态 |
|----|---------|------|
| REV-01 | 删除 `biz/spirit_mode.go` 死代码 | ✅ |
| REV-02 | 删除 `service/chat_orchestrator_spirit.go` 死代码别名层 | ✅ |
| REV-03 | A2A 端点补齐 CustomTools 注入 | ✅ |
| REV-04 | OpenAI 兼容端点补齐 CustomTools 注入 | ✅ |
| REV-05 | `resolveVerificationGates` 注释修正 | ✅ |
| REV-06 | `WriteDeliverablesToSession` 添加 TECH-DEBT 注释 | ✅ |
| REV-07 | `chat_orchestrator_turn.go` + `cli_admin_tools.go` 注释更新 | ✅ |
| REV-08 | 前端 `api.ts` 添加 isValidTeamStatus/isValidTeamMode 运行时校验 | ✅ |
| REV-09 | 前端提取 `SpiritStatusBarData` 共享类型 | ✅ |
| REV-10 | 前端 Store 中 isValidTeamStatus 改为从 types.ts 导入 | ✅ |

### 2026-06-10 M69 审查修复记录

> **修复范围**：F-1~F-12 UI 细节修复 + F-13~F-21 useAgentBlocks 业务逻辑修复 + UI 原型对齐 8 项

**M69 UI 审查修复（F-1~F-12）**：

| ID | 问题 | 根因 | 修复 |
|----|------|------|------|
| F-1 | 左侧面板团队列表始终无数据 | API 响应字段名不匹配 | `data?.items` → `data?.teams` |
| F-2 | 工具名显示一长串原始名称 | 未使用 `resolveDisplayLabel` | 三处 `toolEv?.tool_name` → `resolveDisplayLabel` |
| F-3 | 工具名过长溢出 | 无截断逻辑 | 添加 `.timeline-node__tool-name` 样式 |
| F-4 | `subagents_spawn` 不被识别为子代理 | `classifyActivityKind` 匹配错误 | 添加 `subagents_spawn` 到分类列表 |
| F-5 | 4 个工具显示原始名称 | `builtinLabels` 缺少友好名 | 添加 4 个工具的中文友好名 |
| F-6 | 非 ReAct 模式下 thinking 不显示 | `reactSteps` 为空时无 fallback | 添加 fallback 逻辑 |
| F-7 | TeamTaskCard 折叠态缺少进度条和成员头像 | 仅在 expanded 状态渲染 | 添加迷你进度条+头像到折叠态 |
| F-8 | taskSummary 被 durationText 覆盖 | `v-else-if` 导致 | 改为两个独立 `v-if` |
| F-9 | action 展开态缺少工具参数和结果 | 字段缺失 | 添加 `toolArguments`/`toolResult` 字段 |
| F-10 | error 元素从未被生成 | 无生成逻辑 | 添加 error 元素生成逻辑 |
| F-11 | InterruptedTeamCard 未导入 | 模板使用但未 import | 添加 import |
| F-12 | 搜索框未对团队列表过滤 | 未引用 `search` prop | 添加搜索过滤 |

**M69 useAgentBlocks 业务逻辑修复（F-13~F-21）**：见 Phase P4

### 架构决策记录（2026-06-09）

#### AD-01: Spirit 模式选择机制

**决策**：删除 `biz/spirit_mode.go` 中的 `SelectSpiritMode` / `ResolveSpiritMode` 死代码。
**原因**：类型系统冲突 + 实际路由由 `TaskPlanner.Plan()` 执行
**影响**：未来需要在 `TaskPlannerPort` 接口上增加 `PreRoute()` 方法

#### AD-02: A2A/OpenAI 端点 CustomTools 注入

**决策**：为 A2A 和 OpenAI 兼容端点补齐 CustomTools 注入。
**提示**：修改任何 Runner 构建路径时，必须确保 CustomTools 注入与 Chat 主流程一致。

### 架构决策记录（2026-06-10，M69 P2）

#### AD-03: 任务看板树形嵌套展示模型

**决策**：统一为"任务-思考-工具-回复"树形嵌套结构。

**理由**：
- 任何 agent 的对话输出心智模型统一
- 看板可递归嵌入子任务看板，支持任意深度的执行细节下钻
- 与现实项目管理中"任务→子任务→子子任务"的心智模型对齐

**范围约束**：
- 嵌套深度受 `MaxSessionDepth=2` 约束（精灵 → 团队 → 子 agent）
- 已完成回合默认展开，让用户直达最终答案（修复 F-19）
- 折叠策略：thinking/action 完成后折叠，task/reply/end 始终展开

### 未修复项（后续迭代）

| 编号 | 原因 |
|------|------|
| TD-11 | WriteDeliverablesToSession 使用 ParallelConfigJSON 存储交付物输出，语义不匹配 | P2 |
| TD-12 | resolveVerificationGates 未实现 LinkedGraphID 查询路径 | P2 |
| TD-13 | 废弃 Spirit 工具代码残留约 400 行 | P2 |
| TD-14 | spiritSessionIDFromCtx 耦合 trpc-agent-go 运行时 API | P3 |
| TD-15 | 借调逻辑 submitBorrowRequests 为 best-effort，无回调确认机制 | P3 |
| TD-16 | 精灵 Prompt 决策规则无法 system-side 强制执行 | P3 |
| TD-TK-1 | `todo_write` 工具结果在前端展示为嵌套 JSON | P1.6 |
| TD-TK-2 | `stuckToolResultReason` 文案为 Go 常量硬编码 | P1.6 |
| TD-TK-3 | `ChatExecutionCard` 与 `ToolCallTimeline` 在多工具时并存 | P2 |
| TD-TK-4 | 看板变更脉冲在虚拟滚动回收后可能丢失 | P2 |
| TD-TK-5 | 分支 Agent todo 列表前端显示混乱 | P1.6 后续 |
| TD-TK-6 | LLM 误用工具导致 stuck 的根因 | 后续 |

### 程序提示

#### 提示 1: 修改 Runner 构建路径时的检查清单

当新增或修改任何 Runner 构建路径（Chat / A2A / OpenAI / Team）时，必须检查：
1. CustomTools 注入是否完整
2. `ToolResultGate` / `SubAgentService` / `Organization` 字段是否设置
3. 与 `chat_orchestrator_turn.go` L583-595 的注入逻辑保持一致

#### 提示 2: Spirit 事件发布完整性

当前 15 种 Spirit EnvelopeType 中，5 种三阶段编排事件由 `internal/agent/` 下的实现发布（task_planner_impl.go / agent_allocator_impl.go / task_orchestrator_impl.go），而非 Service 层。

#### 提示 3: 前端类型守卫使用

`features/spirit/types.ts` 中提供了 `isValidTeamStatus` 和 `isValidTeamMode` 运行时类型守卫。所有从 WS/API 接收的状态值必须经过守卫校验后再断言为具体类型。

#### 提示 4: SpiritStatusBarData 共享类型

`features/spirit/types.ts` 中定义了 `SpiritStatusBarData` 类型，`ChatMessagePanel.vue` 和 `SpiritStatusBar.vue` 应统一使用此类型。

#### 提示 5（M69 新增）: useAgentBlocks 状态机扩展

修改 `useAgentBlocks.ts` 时，必须检查：
1. 新增 `tool_blocked` 状态后，UI 是否有对应渲染
2. `hasPartialFailure` 字段需配合 `TurnBlock.vue` 徽章渲染
3. `progressSections` 不再走 `timeline.sort()`，由 `ChatMessageList` 单独渲染
4. `collapsed: false`（已完成回合默认展开）需要与全局折叠按钮协同

#### 提示 6（M69 新增）: 任务看板树形嵌套深度

`TaskBoard` 组件的 `depth` prop 受 `MaxSessionDepth=2` 约束，禁止无限递归。组件内部应有最大深度守卫。
