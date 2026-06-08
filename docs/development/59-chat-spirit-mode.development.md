# M59: Chat 精灵模式 — 开发计划（M59+OBS+M60 合并版）

> **版本**：2026-06-09 | **状态**：✅ P0/P0.5/OBS-P0/OBS-P1/M60-P1/M60-P2/M60-P4/P1 已完成 · 🔄 P1.5 规划中 · 📋 P2 规划中
> **需求**：[59-chat-spirit-mode.md](./59-chat-spirit-mode.md) · **设计**：[59-chat-spirit-mode.design.md](./59-chat-spirit-mode.design.md)

---

## 1. 模块定位

Chat 精灵模式：精灵为唯一对话入口，左侧列表重构为精灵 + 任务团队树，中间面板支持精灵对话/任务执行/成员只读三种模式。支持三阶段编排（Plan → Allocate → Orchestrate）、多团队并行执行、DAG 依赖调度、结果自动合成、编排策略进化。可观测性 UX 增强：对话流自动折叠、语境加载消息、Agent 状态标签、底部状态栏、侧边栏脉冲、中断恢复提示。智能增强：任务复杂度分级、Graph DAG 编排、自适应 Team 模式、编排验证门禁。

**代码锚点**：

| 层级 | 路径 | 阶段 |
|------|------|------|
| Service 精灵工具注入 | `internal/service/chat_orchestrator_turn.go` | P0 |
| Service 团队生命周期 | `internal/service/spirit_team.go` | P0-P0.5 / M60-P1 / OBS-P1 |
| Service Team Turn 回调 | `internal/service/team_turn_hooks.go` | P0 / M60-P1 |
| Service 合成 | `internal/service/spirit_synthesis.go` | M60-P2 |
| Service 精灵编排 | `internal/service/chat_orchestrator_spirit.go` | M60-P4 |
| Tools 三阶段编排 | `internal/tools/spirit_tools.go` | P0.5 / M60-P1-P2-P4 |
| Tools 复杂度评估 | `internal/tools/spirit_complexity.go` | M60-P4 |
| Tools DAG 图构建 | `internal/tools/orchestrator/build_graph.go` | P0.5 / M60-P4 |
| Tools 编排验证 | `internal/tools/orchestrator/verification.go` + `verify_funcs.go` | M60-P4 |
| Biz 三阶段端口 | `internal/biz/task_planner.go` / `agent_allocator.go` / `task_orchestrator.go` | P0.5 |
| Biz Session 树 | `internal/biz/session/usecase.go` | P0 |
| Biz Team 扩展 | `internal/biz/team_usecase.go` | P0 |
| Biz 精灵团队用例 | `internal/biz/spirit_team_usecase.go` | P0-P0.5 / M60-P1 |
| Biz DAG 拓扑 | `internal/biz/spirit_task_dag.go` | P0.5 / M60-P2 |
| Biz 并行配置 | `internal/biz/spirit_parallel_config.go` | P0.5 / M60-P1 |
| Biz 合成引擎 | `internal/biz/spirit_synthesis.go` | M60-P2 |
| Biz 编排缓存 | `internal/biz/spirit_orchestration_cache.go` | M60-P2 |
| Biz 精灵模式 | `internal/biz/spirit_mode.go` | M60-P4 |
| Event | `internal/event/contract/envelope.go` | P0-P0.5 / M60-P1-P2 |
| Composable 自动折叠 | `web/src/composables/chat/useAutoCollapse.ts` | OBS-P0 |
| Composable 语境消息 | `web/src/composables/chat/useContextualLoadingMessage.ts` | OBS-P0 |
| Composable 脉冲 | `web/src/composables/chat/useStatusPulse.ts` | OBS-P1 |
| Feature 可观测常量 | `web/src/features/spirit/observabilityConstants.ts` | OBS-P0 |
| Feature 状态映射 | `web/src/features/spirit/spiritUi.ts` | OBS-P0 |
| 前端 Store | `web/src/stores/spirit/index.ts` | P0-P0.5 / M60-P1-P2 / OBS-P0-P1 |
| 前端组件 | `web/src/components/spirit/` | P0-P1 / M60-P1-P2 / OBS-P0-P1 |
| Proto | `api/kratos/session/v1/session.proto` | P0 |
| Proto | `api/kratos/team/v1/team.proto` | P0-P0.5 / M60-P2 |

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
| M59 P0/P0.5 已完成 | ✅ | 精灵模式核心骨架和三阶段编排 |
| `ChatExecutionCard` 已有折叠/展开 | ✅ | 使用 `<q-expansion-item>` |
| `tool_call`/`tool_result` Envelope 携带 AgentName | ✅ | `EnvelopeToolCall.AgentName`/`AgentKey`/`ActivityKind` |
| `AgentNodeStatus` 17 种状态已定义 | ✅ | `orchestration_status.go` |
| `SpiritMember.status` 字段存在 | ✅ | 类型为 `string`，典型值 idle/running/error |
| `OptionsJSON.team_member` 字段存在 | ✅ | 成员消息过滤基础 |
| `ResumeTeamRunExecution` API 存在 | ✅ | 需要 `graph_execution_id` |
| WS 事件回放机制 | ✅ | `lastEventId` + `onReplayState` 回调 |
| `groupMessagesByTurn` 分组 | ✅ | 需扩展 `isCompleted` 字段 |
| M59 精灵工具注入 | ✅ | CustomTools 机制 |
| M59 团队生命周期事件 | ✅ | spirit_team_completed/failed |
| SpiritTeamAssemblerPort | ✅ | assemble_team 工具接口 |
| EvolutionUsecase | ✅ | DQ Score 计算和建议生成 |
| LearningLoopUsecase | ✅ | Pattern 检测和 Proposal 生成 |

---

## 3. 开发阶段

### Phase P0 — 核心骨架

> **目标**：精灵为唯一入口，左侧列表重构，团队列表展示，任务执行面板基础版，Session 树关联。
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SP-BE-01 | Session Proto 扩展：parent_session_id / root_session_id / agent_depth | `api/kratos/session/v1` | `make api && make build` 通过 | ✅ |
| SP-BE-02 | Session Biz：ListByParentSessionID / GetRootSession 查询 | `internal/biz/session` | 单测通过 | ✅ |
| SP-BE-03 | Session Data：parent_session_id 索引 + 查询实现 | `internal/data` | 单测通过 | ✅ |
| SP-BE-04 | Team Proto 扩展：spirit_session_id / task_description / auto_created | `api/kratos/team/v1` | `make api && make build` 通过 | ✅ |
| SP-BE-05 | Team Biz：Create 支持 AutoCreated / SpiritSessionID | `internal/biz/team` | 单测通过 | ✅ |
| SP-BE-06 | spirit_team.go：AssembleTeam 流程（创建 Team + 创建 Session + 发射 Envelope） | `internal/service` | 集成测试通过 | ✅ |
| SP-BE-07 | chat.go：识别 `__spirit__` → buildSpiritTeam 路由 | `internal/service` | 精灵对话走 Team 路径 | ✅ → P0.5 重构 |
| SP-BE-08 | Event：spirit_team_assembled / completed / failed EnvelopeType | `internal/event` | 单测通过 | ✅ |
| SP-BE-09 | 精灵 Agent 种子数据（`__spirit__` + Ownership=system_builtin） | `internal/data/seed` | 启动后精灵 Agent 可查 | ✅ |
| SP-FE-01 | `features/spirit/types.ts` + `api.ts` | `web/src/features/spirit` | 类型与 Proto 对齐 | ✅ |
| SP-FE-02 | `useSpiritTeamStore`：团队列表 + 面板模式 + 展开/折叠 | `web/src/stores/spirit` | Store 单测通过 | ✅ |
| SP-FE-03 | `SpiritEntry.vue`：精灵入口卡片 | `web/src/components/spirit` | 点击切换精灵对话 | ✅ |
| SP-FE-04 | `ChatEntitySidebar.vue` 重构：精灵 + 团队树 | `web/src/components/chat` | SP-01 验收 | ✅ |
| SP-FE-05 | `TeamTaskCard.vue`：团队卡片（名称/状态/成员/进度） | `web/src/components/spirit` | SP-03 验收 | ✅ |
| SP-FE-06 | `TaskExecutionPanel.vue` 基础版：概览 + 时间线 | `web/src/components/spirit` | SP-04 验收 | ✅ |
| SP-FE-07 | `ChatMessagePanel.vue` 三模式切换 | `web/src/components/chat` | 精灵/团队/成员面板切换 | ✅ |
| SP-FE-08 | `TeamAssemblyCard.vue`：精灵对话中的团队组建卡片 | `web/src/components/spirit` | SP-02 验收 | ✅ |

---

### Phase P0.5 — 三阶段编排

> **目标**：从 `assemble_team` 单步组建演进为 Plan → Allocate → Orchestrate 三阶段编排，支持 DAG 依赖、并行团队、结果合成、自动归档。
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SP-BE-10 | 移除路由层拦截，精灵走 `runSingleAgentViaTRPC` + `spiritCustomTools` 注入 | `internal/service/chat_orchestrator_turn.go` | 精灵不再硬编码路由 | ✅ |
| SP-BE-11 | `plan_and_execute` 工具：三阶段统一入口 | `internal/tools/spirit_tools.go` | Plan → Allocate → Orchestrate 流程 | ✅ |
| SP-BE-12 | `check_progress` / `cancel_orchestration` / `synthesize_results` 工具 | `internal/tools/spirit_tools.go` | 工具可调用 | ✅ |
| SP-BE-13 | `build_orchestration_graph` 工具：DAG 图构建 | `internal/tools/orchestrator/build_graph.go` | 4+ Agent 时构建 DAG | ✅ |
| SP-BE-14 | `TaskPlannerPort` / `AgentAllocatorPort` / `TaskOrchestratorPort` 端口接口 | `internal/biz/` | 三阶段解耦 | ✅ |
| SP-BE-15 | `TaskDAG` 拓扑路由：parallel / sequential / hybrid / coordinator | `internal/biz/spirit_task_dag.go` | 自动选择拓扑 | ✅ |
| SP-BE-16 | `ParallelConfig`：并行配额 + 自动归档配置 | `internal/biz/spirit_parallel_config.go` | 可配置并行数和归档时间 | ✅ |
| SP-BE-17 | `AutoArchiveCompletedTeams`：自动归档已完成团队 | `internal/biz/spirit_team_usecase.go` | 超时后自动归档 | ✅ |
| SP-BE-18 | `CancelTeam`：取消团队 + 级联依赖处理 | `internal/biz/spirit_team_usecase.go` + `internal/service/spirit_team.go` | 取消后依赖团队处理 | ✅ |
| SP-BE-19 | `TeamStarter`：团队生命周期管理（启动/完成/超时/依赖调度） | `internal/service/spirit_team.go` | 全完成检查 + 事件发布 | ✅ |
| SP-BE-20 | 三阶段编排事件：plan_created / allocation_created / orchestration_started 等 | `internal/event/contract/envelope.go` | 前端可感知编排进度 | ✅ |
| SP-BE-21 | `SpiritTeamProgress` / `SpiritTeamsAllCompleted` 事件 | `internal/event/contract/envelope.go` | 团队进度和全完成通知 | ✅ |
| SP-BE-22 | Team Schema 索引：`idx_teams_spirit_session` | `internal/data/ent/schema/team.go` | 查询性能 | ✅ |
| SP-BE-23 | 旧工具标记 DEPRECATED：assemble_team / assess_complexity / check_team_progress / cancel_team | `internal/tools/spirit_tools.go` | 旧工具委托到新流程 | ✅ |
| SP-FE-09 | `SpiritTeamStatus` / `SpiritTeamMode` / `TopologyType` 联合类型 | `web/src/features/spirit/types.ts` | 编译期类型约束 | ✅ |
| SP-FE-10 | `TeamProgressCard.vue`：进度卡片（进度条/取消按钮/依赖提示） | `web/src/components/spirit` | SP-12 验收 | ✅ |
| SP-FE-11 | `ParallelTeamOverview.vue`：并行团队概览 | `web/src/components/spirit` | SP-07 验收 | ✅ |
| SP-FE-12 | `DAGDiagramCard.vue`：DAG 依赖图文本视图 | `web/src/components/spirit` | SP-12 验收 | ✅ |
| SP-FE-13 | `SynthesisResultCard.vue`：综合结果卡片 | `web/src/components/spirit` | SP-13 验收 | ✅ |
| SP-FE-14 | `OrchestrationModeBadge.vue`：编排模式徽章 | `web/src/components/spirit` | 4 种拓扑标签 | ✅ |
| SP-FE-15 | Store 事件处理扩展：三阶段编排事件 + synthesis 事件 | `web/src/stores/spirit/index.ts` | WS 事件正确处理 | ✅ |
| SP-FE-16 | `spiritUi.ts`：状态映射和标签函数 | `web/src/features/spirit` | UI 文案统一 | ✅ |

---

### Phase OBS-P0 — 核心可观测性

> **目标**：对话流自动折叠 + 语境加载消息 + 可折叠工具输出 + Agent 状态标签
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| OBS-FE-01 | `observabilityConstants.ts`：语境消息映射表 + 脉冲颜色配置 | `web/src/features/spirit` | 常量定义完整 | ✅ |
| OBS-FE-02 | `spiritUi.ts` 扩展：`AGENT_NODE_STATUS_MAP` + `STATUS_LABEL_CONFIG` | `web/src/features/spirit` | 17→7 聚合映射正确 | ✅ |
| OBS-FE-03 | `groupMessagesByTurn.ts` 扩展：`TurnBlockGroup.isCompleted` 计算属性 | `web/src/features/chat` | 已完成 block 正确标记 | ✅ |
| OBS-FE-04 | `useAutoCollapse` composable：折叠/展开状态管理 | `web/src/features/chat/composables` | 折叠逻辑正确，性能 < 16ms | ✅ |
| OBS-FE-05 | `ChatMessagePanel.vue` 集成自动折叠：已完成 block 渲染为折叠态 | `web/src/components/chat` | OBS-01 验收 | ✅ |
| OBS-FE-06 | `useContextualLoadingMessage` composable：语境加载消息逻辑 | `web/src/features/chat/composables` | 事件→消息映射正确 | ✅ |
| OBS-FE-07 | `ChatMessagePanel.vue` 集成语境加载消息：精灵对话面板顶部显示 | `web/src/components/chat` | OBS-02 验收 | ✅ |
| OBS-FE-08 | `AgentStatusLabel.vue`：Agent 状态标签组件 | `web/src/components/spirit` | 7 种标签正确渲染 | ✅ |
| OBS-FE-09 | `TeamTaskCard.vue` 增加 AgentStatusLabel：折叠态色点 + 展开态标签 | `web/src/components/spirit` | OBS-03 验收（侧边栏） | ✅ |
| OBS-FE-10 | `TaskExecutionPanel.vue` 增加 AgentStatusLabel：成员列表标签 | `web/src/components/spirit` | OBS-03 验收（执行面板） | ✅ |
| OBS-FE-11 | `ChatExecutionCard.vue` 增加自动折叠：completed/failed 时 `expanded=false` | `web/src/components/chat` | OBS-06 验收 | ✅ |
| OBS-FE-12 | 历史消息折叠态恢复：从 `OptionsJSON.tool_event.status` 判断初始折叠 | `web/src/features/chat` | 加载历史消息时已完成工具默认折叠 | ✅ |
| OBS-FE-13 | WS 回放兼容：语境消息和脉冲在 `onReplayState(true)` 期间静默 | `web/src/features/chat/composables` | 回放期间无闪烁 | ✅ |
| OBS-FE-14 | `TaskExecutionPanel.vue` 集成 `ParallelTeamOverview`：替换简化版布局 | `web/src/components/spirit` | 三区布局完整展示 | ✅ |

---

### Phase OBS-P1 — 全局感知增强

> **目标**：底部状态栏 + 侧边栏脉冲 + 中断恢复提示
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| OBS-BE-01 | `spirit_team_completed` 事件增加 `total_token_in` / `total_token_out` 字段 | `internal/service/spirit_team.go` | 事件 payload 含 token 统计 | ✅ |
| OBS-BE-02 | `spirit_teams_all_completed` 事件增加 token 汇总字段 | `internal/service/spirit_team.go` | 事件 payload 含汇总 token | ✅ |
| OBS-FE-15 | `SpiritStatusBar.vue`：底部状态栏组件 | `web/src/components/spirit` | 4 个字段正确显示 | ✅ |
| OBS-FE-16 | `ChatMessagePanel.vue` 集成 SpiritStatusBar：底部固定 24px | `web/src/components/chat` | OBS-04 验收 | ✅ |
| OBS-FE-17 | `useStatusPulse` composable：侧边栏脉冲逻辑 | `web/src/features/chat/composables` | 脉冲颜色和时长正确 | ✅ |
| OBS-FE-18 | `ChatEntitySidebar.vue` 集成脉冲：团队卡片状态变化时脉冲高亮 | `web/src/components/chat` | OBS-05 验收 | ✅ |
| OBS-FE-19 | `InterruptedTeamCard.vue`：中断恢复提示卡片 | `web/src/components/spirit` | 显示恢复/取消按钮 | ✅ |
| OBS-FE-20 | `TaskExecutionPanel.vue` 集成 InterruptedTeamCard：interrupted 团队显示恢复提示 | `web/src/components/spirit` | OBS-07 验收 | ✅ |
| OBS-FE-21 | 恢复执行 API 调用：`ResumeTeamRunExecution` + 前端状态更新 | `web/src/features/spirit` | 恢复后团队状态变为 running | ✅ |

---

### Phase M60-P1 — 基础并行

> **目标**：移除单团队限制，支持多团队并行，并行度可配置，进度监控，团队取消。
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SPO-BE-01 | `SpiritTeamUsecase.ListActiveTeams()`：按 spirit_session_id 查询所有 active 团队 | `internal/biz/spirit_team_usecase.go` | ✅ |
| SPO-BE-02 | `SpiritTeamUsecase.GetMaxParallelTeams()`：从 AgentRuntimeSettings 读取并行度配置 | `internal/biz/spirit_team_usecase.go` | ✅ |
| SPO-BE-03 | `ParallelConfig` 结构体 + 默认值 + 存储到 Agent MetadataJSON | `internal/biz/spirit_parallel_config.go` | ✅ |
| SPO-BE-04 | TeamKey UUID 后缀：`"spirit_" + sessionID + "_" + uuid[:8]` | `internal/biz/spirit_team_usecase.go` | ✅ |
| SPO-BE-05 | `assemble_team` 工具改造：移除 `GetActiveTeam()` 短路，改用 `ListActiveTeams()` + 并行度检查 | `internal/tools/spirit_tools.go` | ✅ |
| SPO-BE-06 | `check_team_progress` 工具：查询所有活跃团队进度 | `internal/tools/spirit_tools.go` | ✅ |
| SPO-BE-07 | `cancel_team` 工具：取消指定团队 | `internal/tools/spirit_tools.go` | ✅ |
| SPO-BE-08 | `SpiritTeamAssemblerPort` 接口扩展：ListActiveTeams / GetMaxParallelTeams / CancelTeam / CheckTeamProgress | `internal/tools/spirit_tools.go` | ✅ |
| SPO-BE-09 | `SpiritTeamAssembler` 实现扩展：ListActiveTeams / CancelTeam / CheckTeamProgress | `internal/service/spirit_team.go` | ✅ |
| SPO-BE-10 | 精灵 CustomTools 扩展：注入 check_team_progress + cancel_team | `internal/service/cli_admin_tools.go` | ✅ |
| SPO-BE-11 | 新增 EnvelopeType：`spirit_team_progress` / `spirit_teams_all_completed` / `spirit_synthesis_completed` | `internal/event/contract/envelope.go` | ✅ |
| SPO-BE-12 | 精灵 Observer：订阅子团队完成事件，全部完成时发布 `spirit_teams_all_completed` | `internal/service/team_turn_hooks.go` | ✅ |
| SPO-BE-13 | Data 层：`ListBySpiritSessionID` 查询已实现（M59 已完成） | `internal/data/team_repo.go` | ✅ |
| SPO-BE-14 | 团队状态扩展：前端新增 `waiting_deps` / `cancelled` 状态 | `internal/biz/team_types.go` + 前端 types.ts | ✅ |
| SPO-FE-01 | `ParallelConfig` / `TeamProgressView` 类型 | `web/src/features/spirit/types.ts` | ✅ |
| SPO-FE-02 | `useSpiritTeamStore` 扩展：并行团队列表 + 进度查询 + 取消 + runningTeamCount | `web/src/stores/spirit/index.ts` | ✅ |
| SPO-FE-03 | `ParallelTeamOverview.vue`：精灵对话中的并行团队总览卡片 | `web/src/components/spirit/` | ✅ |
| SPO-FE-04 | `TeamProgressCard.vue`：单团队进度卡片 | `web/src/components/spirit/` | ✅ |
| SPO-FE-05 | WS 事件处理：`spirit_team_progress` / `spirit_teams_all_completed` | `web/src/stores/spirit/index.ts` + `useChatInboundSync.ts` + `realtime/envelope.ts` | ✅ |

---

### Phase M60-P2 — 智能编排

> **目标**：Task DAG 依赖调度、拓扑路由、Synthesis Engine、编排进化闭环。
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SPO-BE-15 | `TaskNode` / `TaskDAG` 数据模型 + 校验（环检测、依赖完整性） | `internal/biz/spirit_task_dag.go` | ✅ |
| SPO-BE-16 | `TaskDAG.RouteTopology()`：拓扑路由算法（parallel/sequential/hybrid/coordinator） | `internal/biz/spirit_task_dag.go` | ✅ |
| SPO-BE-17 | `DependencyScheduler`：前置团队完成后自动启动依赖团队 | `internal/biz/spirit_dependency_scheduler.go` | ✅ 已创建后删除（调度逻辑由 SpiritTeamController.ScheduleDependentTeams 承担） |
| SPO-BE-18 | `SynthesisEngine`：结果合成引擎（模板/LLM/混合策略） | `internal/biz/spirit_synthesis.go` | ✅ |
| SPO-BE-19 | `SynthesisPort` 接口 + `SpiritSynthesisService` 实现 | `internal/service/spirit_synthesis.go` | ✅ |
| SPO-BE-20 | `synthesize_results` 工具 | `internal/tools/spirit_tools.go` | ✅ |
| SPO-BE-21 | 精灵 CustomTools 扩展：注入 synthesize_results | `internal/service/cli_admin_tools.go` | ✅ |
| SPO-BE-22 | `OrchestrationCache`：DQ Score 驱动的编排拓扑缓存 | `internal/biz/spirit_orchestration_cache.go` | ✅ |
| SPO-BE-23 | 编排进化闭环：团队完成后计算 DQ Score → 缓存/建议 | `internal/biz/spirit_orchestration_cache.go` | ✅ |
| SPO-BE-24 | `assemble_team` 增强：先查编排缓存，命中则复用历史最优拓扑 | `internal/tools/spirit_tools.go` + `internal/service/spirit_team.go` | ✅ |
| SPO-BE-25 | DAG 存储到 Team 记录：dag_node_id / depends_on 字段 | `internal/data/team_repo.go` | ✅ |
| SPO-BE-26 | 新增 EnvelopeType：`spirit_synthesis_completed` | `internal/event/contract/envelope.go` | ✅ |
| SPO-BE-27 | Team Proto 扩展：dag_node_id / depends_on / parallel_config_json | `api/kratos/team/v1` | ✅ |
| SPO-FE-06 | `TaskDAG` 类型 + DAG 可视化（文本形式） | `web/src/features/spirit/types.ts` | ✅ |
| SPO-FE-07 | `SynthesisResult` 类型 + 合成结果卡片 | `web/src/features/spirit/types.ts` + `components/spirit/SynthesisResultCard.vue` | ✅ |
| SPO-FE-08 | 依赖调度 UI：团队卡片显示依赖状态（等待中 → 运行中） | `web/src/components/spirit/TeamProgressCard.vue` | ✅ |
| SPO-FE-09 | 编排模式说明：精灵回复中展示拓扑选择理由 | `web/src/components/spirit/OrchestrationModeBadge.vue` | ✅ |

**集成修复**（P2 Review 后）：

| ID | 任务 | 状态 |
|----|------|------|
| SPO-INT-01 | Wire 注入链修复：provideChatServiceDeps 填充 SpiritAssembler/SpiritSynthesis/OrchCache | ✅ |
| SPO-INT-02 | SynthesizeResults 查询逻辑修复：ListActiveTeams→ListCompletedTeams | ✅ |
| SPO-INT-03 | DQ Score 进化闭环接入：团队完成时 ComputeDQScore + RecordCompletion | ✅ |
| SPO-INT-04 | TaskDAG 拓扑路由接入：assemble_team 集成 ParseTaskDAG + RouteTopology | ✅ |
| SPO-INT-05 | LLM 策略重命名为 Prompt 策略（SynthesisStrategyLLM→SynthesisStrategyPrompt） | ✅ |
| SPO-INT-06 | TeamProgressCard Mode→Topology 类型映射修复 | ✅ |

**深度业务实现**（P1/P2 差距修复）：

| ID | 任务 | 状态 |
|----|------|------|
| SPO-DP-01 | DAGDiagramCard.vue 前端 DAG 文本展示组件 | ✅ |
| SPO-DP-02 | SynthesisResultCard.vue 展示 summary/keyFindings | ✅ |
| SPO-DP-03 | 编排优化建议生成 EvolutionSuggestionRepo 接入 | ✅ |
| SPO-DP-04 | 前端取消团队调用后端 API（cancelSpiritTeam） | ✅ |
| SPO-DP-05 | 团队超时 TeamTimeout 实现（time.AfterFunc + safego） | ✅ |
| SPO-DP-06 | 自动归档 AutoArchiveAfter 实现（AutoArchiveCompletedTeams） | ✅ |
| SPO-DP-07 | Session 树深度限制 MaxSessionDepth 实现 | ✅ |

---

### Phase M60-P4 — 智能增强

> **目标**：任务复杂度分级、Graph DAG 编排、自适应 Team 模式、编排验证门禁。
> **状态**：✅ 已完成

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SPO-P4-01 | `ComplexityRuleEngine` 规则引擎 + `assess_complexity` 工具 | `internal/tools/spirit_complexity.go` + `spirit_tools.go` (DEPRECATED) | ✅ 增强 plan_and_execute 内部 ComplexityRuleEngine |
| SPO-P4-02 | Spirit Prompt 强制决策规则（assess_complexity 优先） | `internal/scenario/system/prompts/spirit.md` | ✅ DECISION.md + CAPABILITIES.md 更新 |
| SPO-P4-03 | `chat_orchestrator_spirit.go`：Spirit Team 构建逻辑 + 模式选择 | `internal/service/chat_orchestrator_spirit.go` | ✅ SelectSpiritMode + ResolveSpiritMode |
| SPO-P4-04 | `runSingleAgentViaTRPC` 集成 Spirit 模式选择 | `internal/service/chat_orchestrator_turn.go` | ✅ 注释标记 + 模式选择可用 |
| SPO-P4-05 | `build_orchestration_graph` 工具定义 | `internal/tools/orchestrator/build_graph.go` | ✅ GraphBuilderPort 接口替代 OrchestratorGraphDeps |
| SPO-P4-06 | `BuildGraphConfig` DAG 生成逻辑（并行/串行/混合拓扑） | `internal/tools/orchestrator/build_graph.go` | ✅ |
| SPO-P4-07 | 验证节点类型定义 + 验证函数（output_format/task_completion） | `internal/tools/orchestrator/verification.go` + `verify_funcs.go` | ✅ |
| SPO-P4-08 | 验证节点注入到 Graph（injectVerificationNodes） | `internal/tools/orchestrator/verification.go` | ✅ |
| SPO-P4-09 | `GraphBuilderPort` 依赖注入 + Wire 绑定 | `internal/service/cli_admin_tools.go` + `cmd/admin/wire.go` | ✅ |
| SPO-P4-10 | 编排管家 Prompt Graph 编排决策规则 | `internal/scenario/system/prompts/orchestrator.md` | ✅ |

---

### Phase P1 — 交互增强

> **目标**：成员树/只读面板、面包屑导航、重试失败团队、手动归档 UI、ListSpiritTeams API 闭环。
> **状态**：🔄 进行中

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SP-BE-24 | `ListSpiritTeams` RPC：按 spirit_session_id 查团队列表，暴露为 HTTP 端点 | `api/kratos/team/v1` | 前端团队列表数据源闭环 | ✅ |
| SP-BE-25 | `ArchiveTeam` RPC：手动归档已完成团队 | `api/kratos/team/v1` | 归档后列表不显示 | ✅ |
| SP-BE-26 | `RetryTeam` RPC：重试失败团队 | `api/kratos/team/v1` | 失败团队可重新启动 | ✅ |
| SP-FE-17 | `TeamMemberTreeNode.vue`：成员树节点（名称/角色/状态） | `web/src/components/spirit` | SP-05 验收 | ✅ |
| SP-FE-18 | `TeamTaskCard.vue` 展开成员树 | `web/src/components/spirit` | 展开/折叠 + 成员状态 | ✅ |
| SP-FE-19 | `MemberReadOnlyPanel.vue`：只读面板（无输入框） | `web/src/components/spirit` | SP-06 验收 | ✅ |
| SP-FE-20 | 成员消息过滤：按 `OptionsJSON.team_member` 过滤 | `web/src/stores/spirit` | 成员只看自己的消息 | ✅ |
| SP-FE-21 | Agent 复用标识：团队卡片标注"共用 Agent" | `web/src/components/spirit` | 复用 Agent 可见标识 | ✅ |
| SP-FE-22 | 团队归档 UI：手动归档按钮 | `web/src/components/spirit` | SP-08 验收 | ✅ |
| SP-FE-23 | 失败团队：重试/放弃按钮 | `web/src/components/spirit` | SP-08 验收 | ✅ |
| SP-FE-24 | 面包屑导航：精灵 > 团队 > 成员 | `web/src/features/spirit` | SP-09 验收 | ✅ |
| SP-FE-25 | 返回精灵按钮 + WS 连接保持 | `web/src/components/spirit` | 切换不丢 WS | ✅ 已在 P0 实现 |
| SP-FE-26 | `api.ts` 双键名兼容清理 | `web/src/features/spirit/api.ts` | 统一 camelCase | ✅ |

---

### Phase P1.5 — ChatExecutionCard 独立折叠增强

> **目标**：5s 耗时守卫、折叠摘要增强、全局展开/折叠两层联动。
> **状态**：📋 规划中
> **设计文档**：§6.8（权威） · [proposal](../../reports/2026-06-09-proposal-chat-execution-card-folding.md)（详细参考）
> **分阶段交付**：SP-FE-27~29 可独立交付（Phase 1），SP-FE-30~31 为后续增强（Phase 2）

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| SP-FE-27 | ChatExecutionCard 5s elapsed timer：running ≥5s 显示实时计时器，≥60s 警告色；`started_at` 为空时降级 `occurred_at` → `Date.now()`；`onBeforeUnmount` 清理 timer | `web/src/components/chat/ChatExecutionCard.vue` | OBS-08 验收1+2 | ❌ |
| SP-FE-28 | ChatExecutionCard 折叠态摘要兜底：`event.summary` 为空时前端根据 `tool_name`+`arguments` 生成 | `web/src/components/chat/ChatExecutionCard.vue` | OBS-08 验收3 | ❌ |
| SP-FE-29 | ToolStrip 折叠态摘要增强：显示工具类型分布（如"3 file_read · 2.5s"） | `web/src/components/chat/ToolStrip.vue` | OBS-08 验收4 | ❌ |
| SP-FE-30 | Provide/Inject 全局控制：`ExecutionCollapseControl` 接口 + Signal；运行中工具不响应 collapseAll；Spirit 模式自动生效 | `web/src/features/chat/types.ts` + `ChatMessagePanel.vue` + `ChatExecutionCard.vue` | OBS-08 验收5-7 | ❌ |
| SP-FE-31 | `ToolUseEvent.expanded` 死代码清理 | `web/src/features/chat/types.ts` | OBS-08 验收8 | ❌ |

> **推迟到后续迭代**：ToolStrip `<details>` → `q-expansion-item` 统一折叠动画；`aria-expanded`/`aria-controls` 无障碍属性；虚拟滚动兼容验证

---

### Phase P2 — 进化闭环

> **目标**：Session 数据 → 技能/记忆/编排分析，Agent 能力画像 → 团队组建优化，知识图谱数据积累。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| SP-EVO-01 | Session 执行轨迹 → 技能管家分析输入 | `internal/biz` | 技能管家可消费 Team Session 数据 |
| SP-EVO-02 | Session 执行轨迹 → 记忆管家分析输入 | `internal/biz` | 记忆管家 dream_cycle 可消费 |
| SP-EVO-03 | 编排效率分析：模式成功率 + DQ Score | `internal/biz` | 精灵可参考历史编排效率选择模式 |
| SP-EVO-04 | Agent 能力画像 → 团队组建优化 | `internal/agent` | assemble_team 参考 Agent 历史表现 |
| SP-EVO-05 | 知识图谱数据积累：协作关系 + 产出物 | `internal/memory` | L4 实体-关系图谱丰富 |
| SP-EVO-06 | 编排进化：失败模式分析 → FailurePolicy 调整 | `internal/team` | 基于历史失败调整默认 FailurePolicy |

---

## 4. 任务板

### P0 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SP-BE-01 | Session Proto 扩展 | ✅ |
| 2 | SP-BE-02 | Session Biz 树查询 | ✅ |
| 3 | SP-BE-03 | Session Data 查询实现 | ✅ |
| 4 | SP-BE-04 | Team Proto 扩展 | ✅ |
| 5 | SP-BE-05 | Team Biz AutoCreated | ✅ |
| 6 | SP-BE-06 | spirit_team.go AssembleTeam | ✅ |
| 7 | SP-BE-07 | chat.go 精灵路由 | ✅ → P0.5 重构 |
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

### P0.5 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SP-BE-10 | 移除路由层拦截，spiritCustomTools 注入 | ✅ |
| 2 | SP-BE-11 | plan_and_execute 三阶段工具 | ✅ |
| 3 | SP-BE-12 | check_progress / cancel_orchestration / synthesize_results | ✅ |
| 4 | SP-BE-13 | build_orchestration_graph DAG 工具 | ✅ |
| 5 | SP-BE-14 | 三阶段端口接口 | ✅ |
| 6 | SP-BE-15 | TaskDAG 拓扑路由 | ✅ |
| 7 | SP-BE-16 | ParallelConfig 并行配置 | ✅ |
| 8 | SP-BE-17 | AutoArchiveCompletedTeams | ✅ |
| 9 | SP-BE-18 | CancelTeam + 级联处理 | ✅ |
| 10 | SP-BE-19 | TeamStarter 生命周期管理 | ✅ |
| 11 | SP-BE-20 | 三阶段编排事件 | ✅ |
| 12 | SP-BE-21 | Progress / AllCompleted 事件 | ✅ |
| 13 | SP-BE-22 | Team Schema 索引 | ✅ |
| 14 | SP-BE-23 | 旧工具标记 DEPRECATED | ✅ |
| 15 | SP-FE-09 | 联合类型定义 | ✅ |
| 16 | SP-FE-10 | TeamProgressCard.vue | ✅ |
| 17 | SP-FE-11 | ParallelTeamOverview.vue | ✅ |
| 18 | SP-FE-12 | DAGDiagramCard.vue | ✅ |
| 19 | SP-FE-13 | SynthesisResultCard.vue | ✅ |
| 20 | SP-FE-14 | OrchestrationModeBadge.vue | ✅ |
| 21 | SP-FE-15 | Store 事件处理扩展 | ✅ |
| 22 | SP-FE-16 | spiritUi.ts 状态映射 | ✅ |

### OBS-P0 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | OBS-FE-01 | observabilityConstants.ts | ✅ |
| 2 | OBS-FE-02 | spiritUi.ts 状态聚合映射 | ✅ |
| 3 | OBS-FE-03 | groupMessagesByTurn.ts isCompleted | ✅ |
| 4 | OBS-FE-04 | useAutoCollapse composable | ✅ |
| 5 | OBS-FE-05 | ChatMessagePanel 自动折叠集成 | ✅ |
| 6 | OBS-FE-06 | useContextualLoadingMessage composable | ✅ |
| 7 | OBS-FE-07 | ChatMessagePanel 语境消息集成 | ✅ |
| 8 | OBS-FE-08 | AgentStatusLabel.vue | ✅ |
| 9 | OBS-FE-09 | TeamTaskCard 增加 AgentStatusLabel | ✅ |
| 10 | OBS-FE-10 | TaskExecutionPanel 增加 AgentStatusLabel | ✅ |
| 11 | OBS-FE-11 | ChatExecutionCard 自动折叠 | ✅ |
| 12 | OBS-FE-12 | 历史消息折叠态恢复 | ✅ |
| 13 | OBS-FE-13 | WS 回放兼容 | ✅ |
| 14 | OBS-FE-14 | TaskExecutionPanel 集成 ParallelTeamOverview | ✅ |

### OBS-P1 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | OBS-BE-01 | spirit_team_completed 增加 token 字段 | ✅ |
| 2 | OBS-BE-02 | spirit_teams_all_completed 增加 token 字段 | ✅ |
| 3 | OBS-FE-15 | SpiritStatusBar.vue | ✅ |
| 4 | OBS-FE-16 | ChatMessagePanel 集成 SpiritStatusBar | ✅ |
| 5 | OBS-FE-17 | useStatusPulse composable | ✅ |
| 6 | OBS-FE-18 | ChatEntitySidebar 集成脉冲 | ✅ |
| 7 | OBS-FE-19 | InterruptedTeamCard.vue | ✅ |
| 8 | OBS-FE-20 | TaskExecutionPanel 集成 InterruptedTeamCard | ✅ |
| 9 | OBS-FE-21 | 恢复执行 API 调用 | ✅ |

### M60-P1 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SPO-BE-01 | ListActiveTeams 查询 | ✅ |
| 2 | SPO-BE-02 | GetMaxParallelTeams 配置读取 | ✅ |
| 3 | SPO-BE-03 | ParallelConfig 结构体 | ✅ |
| 4 | SPO-BE-04 | TeamKey UUID 后缀 | ✅ |
| 5 | SPO-BE-05 | assemble_team 工具改造 | ✅ |
| 6 | SPO-BE-06 | check_team_progress 工具 | ✅ |
| 7 | SPO-BE-07 | cancel_team 工具 | ✅ |
| 8 | SPO-BE-08 | SpiritTeamAssemblerPort 接口扩展 | ✅ |
| 9 | SPO-BE-09 | SpiritTeamAssembler 实现扩展 | ✅ |
| 10 | SPO-BE-10 | 精灵 CustomTools 扩展 | ✅ |
| 11 | SPO-BE-11 | 新增 EnvelopeType | ✅ |
| 12 | SPO-BE-12 | 精灵 Observer | ✅ |
| 13 | SPO-BE-13 | Data 层查询实现 | ✅ |
| 14 | SPO-BE-14 | 团队状态扩展 | ✅ |
| 15 | SPO-FE-01 | 前端类型 + API | ✅ |
| 16 | SPO-FE-02 | Store 扩展 | ✅ |
| 17 | SPO-FE-03 | ParallelTeamOverview 组件 | ✅ |
| 18 | SPO-FE-04 | TeamProgressCard 组件 | ✅ |
| 19 | SPO-FE-05 | WS 事件处理 | ✅ |

### M60-P2 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SPO-BE-15 | TaskDAG 数据模型 + 环检测 | ✅ |
| 2 | SPO-BE-16 | 拓扑路由算法 | ✅ |
| 3 | SPO-BE-17 | DependencyScheduler | ✅ 已创建后删除 |
| 4 | SPO-BE-18 | SynthesisEngine | ✅ |
| 5 | SPO-BE-19 | SynthesisPort + Service | ✅ |
| 6 | SPO-BE-20 | synthesize_results 工具 | ✅ |
| 7 | SPO-BE-21 | CustomTools 注入 synthesize_results | ✅ |
| 8 | SPO-BE-22 | OrchestrationCache | ✅ |
| 9 | SPO-BE-23 | 编排进化闭环 | ✅ |
| 10 | SPO-BE-24 | assemble_team 缓存增强 | ✅ |
| 11 | SPO-BE-25 | DAG 存储到 Team 记录 | ✅ |
| 12 | SPO-BE-26 | spirit_synthesis_completed EnvelopeType | ✅ |
| 13 | SPO-BE-27 | Team Proto 扩展 | ✅ |
| 14 | SPO-FE-06 | TaskDAG 类型 | ✅ |
| 15 | SPO-FE-07 | SynthesisResult 卡片 | ✅ |
| 16 | SPO-FE-08 | 依赖调度 UI | ✅ |
| 17 | SPO-FE-09 | 编排模式说明 UI | ✅ |
| 18 | SPO-INT-01 | Wire 注入链修复 | ✅ |
| 19 | SPO-INT-02 | SynthesizeResults 查询逻辑修复 | ✅ |
| 20 | SPO-INT-03 | DQ Score 进化闭环接入 | ✅ |
| 21 | SPO-INT-04 | TaskDAG 拓扑路由接入 | ✅ |
| 22 | SPO-INT-05 | LLM→Prompt 策略重命名 | ✅ |
| 23 | SPO-INT-06 | TeamProgressCard 类型映射修复 | ✅ |
| 24 | SPO-DP-01 | DAGDiagramCard.vue | ✅ |
| 25 | SPO-DP-02 | SynthesisResultCard.vue 展示增强 | ✅ |
| 26 | SPO-DP-03 | EvolutionSuggestionRepo 接入 | ✅ |
| 27 | SPO-DP-04 | 前端取消团队 API 调用 | ✅ |
| 28 | SPO-DP-05 | TeamTimeout 实现 | ✅ |
| 29 | SPO-DP-06 | AutoArchiveAfter 实现 | ✅ |
| 30 | SPO-DP-07 | MaxSessionDepth 实现 | ✅ |

### M60-P4 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SPO-P4-01 | ComplexityRuleEngine + assess_complexity | ✅ |
| 2 | SPO-P4-02 | Spirit Prompt 强制决策规则 | ✅ |
| 3 | SPO-P4-03 | chat_orchestrator_spirit.go Team 模式选择 | ✅ |
| 4 | SPO-P4-04 | runSingleAgentViaTRPC 集成 | ✅ |
| 5 | SPO-P4-05 | build_orchestration_graph 工具 | ✅ |
| 6 | SPO-P4-06 | buildGraphConfig DAG 生成 | ✅ |
| 7 | SPO-P4-07 | 验证节点类型 + 验证函数 | ✅ |
| 8 | SPO-P4-08 | 验证节点注入到 Graph | ✅ |
| 9 | SPO-P4-09 | GraphBuilderPort 依赖注入 | ✅ |
| 10 | SPO-P4-10 | 编排管家 Prompt 决策规则 | ✅ |
| 11 | SPO-DR-S3 | OrchestrationCache ToJSON 递归 RLock 死锁修复 | ✅ |
| 12 | SPO-DR-S4 | 超时回调不触发依赖调度修复 | ✅ |
| 13 | SPO-DR-S5 | interrupted 被错误视为终态修复 | ✅ |
| 14 | SPO-DR-FS1 | 前后端 SpiritTeamMode 枚举对齐 | ✅ |
| 15 | SPO-DR-FS2 | 前后端 SpiritTeamStatus 枚举对齐 | ✅ |
| 16 | SPO-DR-FS3 | SynthesisResultCard XSS 修复 | ✅ |
| 17 | SPO-DR-FS4 | cancelTeam 改为更新状态而非移除 | ✅ |
| 18 | SPO-DR-M11 | HandleTeamTurnResult 入口统一取消超时定时器 | ✅ |
| 19 | SPO-DR-M13 | BuildGraphConfig 循环检测 + 依赖验证 | ✅ |
| 20 | SPO-DR-M8 | 前端 spirit_team_progress 状态回退防护 | ✅ |
| 21 | SPO-DR-L11 | AutoArchiveCompletedTeams 错误日志记录 | ✅ |
| 22 | SPO-DR-L17 | checkAllTeamsCompleted 循环外统一调用优化 | ✅ |
| 23 | SPO-DR-WIRE | provideFailurePatternSyncJob 接口注入修复 | ✅ |
| 24 | SPO-RS-01 | NewSpiritTeamUsecase 7参数→Options 模式 | ✅ |
| 25 | SPO-RS-02 | TeamGraphSessionRepo 6方法→Reader+Writer 嵌入组合 | ✅ |
| 26 | SPO-RS-04 | RecordCompletionWithAgents Get+Put 非原子→合并单 Lock | ✅ |
| 27 | SPO-RS-05 | app.go 混用 logger→统一 loggateway | ✅ |
| 28 | SPO-RS-06 | SetTimeoutHandler 并发安全→sync.Once | ✅ |
| 29 | SPO-RS-07 | ComputeDQScoreBreakdown 魔法数字→命名常量 | ✅ |
| 30 | SPO-RS-08 | BuildGraphConfig 95行→拆分 4 个子函数 | ✅ |
| 31 | SPO-RS-09 | buildSpiritTeamDefinitionJSON 魔法数字→命名常量 | ✅ |
| 32 | SPO-RS-10 | AutoArchiveCompletedTeams 逐条DB→批量操作 | ✅ |
| 33 | SPO-RR-01 | TeamOrchestrationDeps 移除未使用 Teams 字段 | ✅ |
| 34 | SPO-RR-02 | TeamRepository Deprecated 注释 + 迁移窄接口 | ✅ |
| 35 | SPO-RR-03 | 超时回调 context.Background()→30s 超时控制 | ✅ |
| 36 | SPO-RR-04 | TeamTaskCard.vue as any→mappedStatus 计算属性 | ✅ |
| 37 | SPO-RR-05 | TeamProgressCard 废弃状态值清理 | ✅ |
| 38 | SPO-RR-06 | TeamProgressView/TeamSynthesisResult.status string→SpiritTeamStatus | ✅ |
| 39 | SPO-RR-07 | updateTeamStatus string→SpiritTeamStatus + 类型守卫 | ✅ |
| 40 | SPO-RR-08 | SpiritTeamMode 移除后端不存在的 direct 值 | ✅ |

### P1 当前冲刺

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SP-BE-24 | ListSpiritTeams RPC + HTTP 端点 | ✅ |
| 2 | SP-BE-25 | ArchiveTeam RPC | ✅ |
| 3 | SP-BE-26 | RetryTeam RPC | ✅ |
| 4 | SP-FE-17 | TeamMemberTreeNode.vue | ✅ |
| 5 | SP-FE-18 | TeamTaskCard 展开成员树 | ✅ |
| 6 | SP-FE-19 | MemberReadOnlyPanel.vue | ✅ |
| 7 | SP-FE-20 | 成员消息过滤 | ✅ |
| 8 | SP-FE-21 | Agent 复用标识 | ✅ |
| 9 | SP-FE-22 | 团队归档 UI | ✅ |
| 10 | SP-FE-23 | 失败团队重试/放弃 | ✅ |
| 11 | SP-FE-24 | 面包屑导航 | ✅ |
| 12 | SP-FE-25 | 返回精灵 + WS 保持 | ✅ |
| 13 | SP-FE-26 | api.ts 双键名清理 | ✅ |

### P1.5 任务板

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SP-FE-27 | ChatExecutionCard 5s elapsed timer | ❌ |
| 2 | SP-FE-28 | ChatExecutionCard 折叠态摘要兜底 | ❌ |
| 3 | SP-FE-29 | ToolStrip 折叠态摘要增强 | ❌ |
| 4 | SP-FE-30 | Provide/Inject 全局控制 | ❌ |
| 5 | SP-FE-31 | ToolUseEvent.expanded 死代码清理 | ❌ |

---

## 5. 验收标准

### Phase P0

- [x] `make api && make wire && make build` 通过
- [x] `go test ./internal/biz/session/... ./internal/service/... -count=1` 通过
- [x] 精灵 Agent 种子数据启动后可查
- [x] 左侧列表仅显示精灵 + 团队（SP-01）
- [x] 精灵区分简单/任务型对话（SP-02）
- [x] 团队卡片展示名称/状态/成员/进度（SP-03）
- [x] 任务执行面板三区布局（SP-04）
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase P0.5

- [x] 精灵走 `runSingleAgentViaTRPC`，不再硬编码路由
- [x] `plan_and_execute` 三阶段编排工具可调用
- [x] DAG 拓扑自动路由（parallel / sequential / hybrid / coordinator）
- [x] 并行团队支持（ParallelConfig + DAG 依赖调度）
- [x] 综合结果合成（synthesize_results + SynthesisResultCard）
- [x] 自动归档已完成团队（AutoArchiveCompletedTeams）
- [x] 取消团队 + 级联依赖处理
- [x] 三阶段编排事件（plan_created / allocation_created / orchestration_started 等）
- [x] 团队进度和全完成事件（spirit_team_progress / spirit_teams_all_completed）
- [x] 前端联合类型约束（SpiritTeamStatus / SpiritTeamMode / TopologyType）
- [x] `make api && make wire && make build` 通过
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase OBS-P0

- [x] 已完成工具调用卡片自动折叠为单行摘要（OBS-01）
- [x] 已完成团队组建/完成卡片自动折叠（OBS-01）
- [x] interrupted 状态折叠显示 ⏸ 标记（OBS-01）
- [x] "展开全部"按钮可用（OBS-01）
- [x] 三阶段编排过程显示语境加载消息（OBS-02）
- [x] Agent 级语境消息显示"{agent_name} 正在{display_label}…"（OBS-02）
- [x] WS 回放期间语境消息静默（OBS-02）
- [x] 侧边栏团队卡片显示 Agent 状态色点（OBS-03）
- [x] 任务执行面板显示 7 种 Agent 状态标签（OBS-03）
- [x] Active 状态标签有呼吸动画（OBS-03）
- [x] ChatExecutionCard completed/failed 时自动折叠（OBS-06）
- [x] running 状态工具调用始终展开（OBS-06）
- [x] 加载历史消息时已完成工具默认折叠（OBS-06）
- [x] TaskExecutionPanel 集成 ParallelTeamOverview 三区布局（OBS-FE-14）
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase OBS-P1

- [x] 底部状态栏显示活跃团队数/中断数/配额/Token（OBS-04）
- [x] 底部状态栏固定 24px，不随内容滚动（OBS-04）
- [x] 侧边栏团队卡片状态变化时脉冲高亮（OBS-05）
- [x] 脉冲颜色和时长正确（OBS-05）
- [x] WS 回放期间脉冲禁用（OBS-05）
- [x] interrupted 团队显示恢复提示卡片（OBS-07）
- [x] "恢复执行"按钮调用 ResumeTeamRunExecution API（OBS-07）
- [x] 不支持断点恢复的团队显示禁用提示（OBS-07）
- [x] `make api && make wire && make build` 通过
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase M60-P1

- [x] 同一精灵 Session 可创建多个并行团队（SPO-01）
- [x] 并行度超限时精灵提示用户等待（SPO-02）
- [x] 团队进度实时监控 + 精灵主动通知（SPO-03）
- [x] 取消团队 + 释放配额（SPO-04）
- [x] `make api && make wire && make build` 通过
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase M60-P2

- [x] Task DAG 依赖调度正确执行（SPO-05）
- [x] 拓扑路由自动选择编排模式（SPO-06）
- [x] Synthesis Engine 结果合成（SPO-07）
- [x] DQ Score 驱动编排缓存（SPO-08）
- [x] 编排策略进化闭环（SPO-09）

### Phase M60-P4

- [x] `assess_complexity` 工具正确评估 simple/moderate/complex 三级
- [x] Spirit 强制先调用 assess_complexity 再路由
- [x] Team 模式选择：simple→Direct, moderate→Direct, complex→Coordinator
- [x] `build_orchestration_graph` 生成正确的 Graph DAG（并行/串行/混合）
- [x] 验证节点注入：output_format/task_completion/human_approval
- [x] `make api && make wire && make build` 通过
- [x] `go test ./internal/biz/... ./internal/service/... ./internal/tools/... -count=1` 通过

### Phase P1

- [x] 成员树形展开 + 状态（SP-05）
- [x] 成员只读面板无输入框（SP-06）
- [x] Agent 复用标识可见（SP-07 补充）
- [x] 团队归档/重试/放弃（SP-08）— 归档+重试+放弃全部实现
- [x] 面包屑导航 + 返回精灵（SP-09）
- [x] ListSpiritTeams API 闭环（前端 `/v1/spirit/{id}/teams` 有后端 handler）
- [x] ListChildSessions RPC Proto 定义 + Service 实现
- [x] SynthesizeResults RPC Proto 定义 + Service 实现
- [x] ArchiveTeam RPC Proto 定义 + Service 实现
- [x] RetryTeam RPC Proto 定义 + Service 实现
- [x] api.ts 双键名兼容清理
- [x] spirit_teams_all_completed 载荷分项统计（total_teams/completed_teams/failed_teams）
- [x] SpiritTeamMode 前后端语义映射确认对齐

### Phase P1.5

- [ ] 工具运行 ≥5s 时显示实时计时器，≥60s 变为警告色（OBS-08 验收1）
- [ ] `started_at` 为空时降级 `occurred_at` → `Date.now()`，始终启动计时器（OBS-08 验收2）
- [ ] 折叠态摘要兜底：后端未提供 summary 时前端生成（OBS-08 验收3）
- [ ] ToolStrip 折叠态显示工具类型分布（OBS-08 验收4）
- [ ] 全局"展开全部/折叠全部"同时作用于 TurnBlock + ChatExecutionCard（OBS-08 验收5）
- [ ] 运行中工具不受"折叠全部"影响（OBS-08 验收6）
- [ ] Spirit 模式 ChatExecutionCard 同样响应全局控制（OBS-08 验收7）
- [ ] ToolUseEvent.expanded 死代码清理（OBS-08 验收8）

### Phase P2

- [ ] Session 数据可被技能管家消费（SP-10）
- [ ] 编排效率分析可输出 DQ Score
- [ ] Agent 能力画像可被 assemble_team 参考

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
| `AgentNodeStatus` 数据源可能延迟 | 侧边栏使用 `SpiritMember.status`（实时），执行面板使用 `AgentNodeStatus`（可能延迟 500ms） |
| Token 统计事件扩展需后端修改 | P1 阶段实施，P0 阶段底部状态栏不显示 Token 字段 |
| `ResumeTeamRunExecution` 需 `graph_execution_id` | 无 Graph 执行的团队显示"不支持断点恢复" |

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
| [59-chat-spirit-mode-observability.md](./59-chat-spirit-mode-observability.md) | 可观测性需求 | OBS-P0 |
| [59-chat-spirit-mode-observability.design.md](./59-chat-spirit-mode-observability.design.md) | 可观测性设计 | OBS-P0 |
| [60-spirit-parallel-orchestrator.md](./60-spirit-parallel-orchestrator.md) | 并行编排需求 | M60-P1 |
| [60-spirit-parallel-orchestrator.design.md](./60-spirit-parallel-orchestrator.design.md) | 并行编排设计 | M60-P1 |

---

## 8. 审查修复记录

### 2026-06-01 P0 Review 修复

> **修复范围**：S1 / S2 / S3 / M1 / M2 / M3 / M4 / M5 / M6 / M8

| 编号 | 问题 | 修复方案 | 状态 |
|------|------|---------|------|
| S1 | 每次 Turn 都创建 Team | 移除路由拦截，精灵走 `runSingleAgentViaTRPC`，LLM 自主决策 | ✅ → P0.5 升级为 `plan_and_execute` |
| S2 | Composer 不渲染 | `v-if="!panelMode \|\| panelMode === 'spirit'"` | ✅ |
| S3 | Completed/Failed 事件未实现 | `team_turn_hooks.go` 发布生命周期事件 | ✅ |
| M1 | appendChildSessionID 并发保护 | 移除 `appendChildSessionID` 方法 | ✅ |
| M2 | child_session_ids 冗余 | 移除，统一使用 `ListByParentSessionID` 查询 | ✅ |
| M3 | spiritAssembler 静默降级 | 移除 `spiritAssembler` 字段和路由拦截逻辑 | ✅ |
| M4 | SpiritTeam 类型约束 | 定义联合类型 | ✅ |
| M5 | 类型导入红线 | 统一从 `types.ts` 导入类型 | ✅ |
| M6 | API 双键名兼容 | 部分清理，仍有残留（TD-1） | ⚠️ P1 继续 |
| M8 | Team Schema 索引 | 添加 `idx_teams_spirit_session` 复合索引 | ✅ |

### 2026-06-06 P0.5 三阶段编排实施

> **修复范围**：S1 深度修复 + 新增三阶段编排架构

关键变更：移除路由拦截 → `plan_and_execute` 三阶段工具 → DAG 拓扑路由 → 并行配置 → 自动归档 → 取消+级联 → TeamStarter 生命周期 → 三阶段事件 → 前端联合类型 + 进度卡片 + DAG 图 + 合成卡片 + 编排徽章 + Store 事件扩展 + spiritUi 状态映射。

### 2026-06-08 OBS Review 修复

> **修复范围**：OBS-R01~R07 阻断项 + OBS-S01~S07 建议项

**阻断项**：

| ID | 问题 | 修复 |
|----|------|------|
| OBS-R01 | useAutoCollapse 死代码，TurnBlock.collapsed 未传递 | ChatMessagePanel 调用 useAutoCollapse，ChatMessageList 传递 :collapsed |
| OBS-R02 | SpiritStatusBar 数据流断链 | ChatPage 计算 spiritStatusBar computed 并传递 |
| OBS-R03 | useStatusPulse 死代码，pulseTeamColors 未传递 | useChatWorkspace 调用 useStatusPulse，ChatPage 传递 + watcher |
| OBS-R04 | spirit_team_completed 事件 token 硬编码为 0 | Service 层从 session 提取实际 InputTokens/OutputTokens |
| OBS-R05 | spirit_teams_all_completed 事件 token 硬编码为 0 | biz 层增加 token 字段，Service 层使用聚合值 |
| OBS-R06 | useContextualLoadingMessage skill 模式缺少 {summary} | 修复 displayLabel 逻辑 |
| OBS-R07 | M59-OBS 测试覆盖率为零 | 新增 3 个 composable 单元测试 |

**建议项**：

| ID | 修复 |
|----|------|
| OBS-S01 | canResume 检查 graphExecutionId \|\| dagNodeId |
| OBS-S02 | interruptReason 从事件元数据读取，默认"执行中断" |
| OBS-S04 | useStatusPulse 添加 onUnmounted 清理 |
| OBS-S05 | SpiritStatusBar 改用 CSS 变量 |
| OBS-S06 | 前端消费 token 数据 |
| OBS-S07 | AllTeamsCompletedResult 增加 TotalTokenIn/TotalTokenOut |

### M60 深度架构审查修复

> **修复范围**：S3~S5 严重项 + FS1~FS4 前端严重项 + M8/M11/M13 中等项 + L11/L17 轻微项 + Wire 注入修复

| ID | 修复摘要 |
|----|---------|
| SPO-DR-S3 | OrchestrationCache ToJSON 递归 RLock 死锁修复 |
| SPO-DR-S4 | 超时回调不触发依赖调度修复（TimeoutHandler 接口） |
| SPO-DR-S5 | interrupted 被错误视为终态修复 |
| SPO-DR-FS1~FS4 | 前后端枚举对齐 + XSS 修复 + cancelTeam 行为修复 |
| SPO-DR-M8/M11/M13 | 状态回退防护 + 超时定时器统一取消 + 循环检测 |
| SPO-DR-L11/L17 | 错误日志 + 循环外统一调用优化 |
| SPO-DR-WIRE | provideFailurePatternSyncJob 接口注入修复 |

### M60 迭代建议修复（RS-01~RS-10, RR-01~RR-08）

> **修复范围**：Options 模式重构 + 接口拆分 + 并发安全 + 命名常量 + 批量操作 + 类型安全

| ID | 修复摘要 |
|----|---------|
| SPO-RS-01 | NewSpiritTeamUsecase 7参数→Options 模式 |
| SPO-RS-02 | TeamGraphSessionRepo 6方法→Reader+Writer 嵌入组合 |
| SPO-RS-04 | RecordCompletionWithAgents 非原子→合并单 Lock |
| SPO-RS-05 | app.go 统一 loggateway |
| SPO-RS-06 | SetTimeoutHandler→sync.Once |
| SPO-RS-07 | ComputeDQScoreBreakdown 魔法数字→命名常量 |
| SPO-RS-08 | BuildGraphConfig 拆分 4 个子函数 |
| SPO-RS-09 | buildSpiritTeamDefinitionJSON 魔法数字→命名常量 |
| SPO-RS-10 | AutoArchiveCompletedTeams→批量操作 |
| SPO-RR-01~03 | 后端未使用字段清理 + 超时回调 context 修复 |
| SPO-RR-04~08 | 前端类型安全修复（as any 消除 + 废弃状态值清理 + 类型守卫） |

### 未修复项（P1 待处理）

| 编号 | 原因 |
|------|------|
| ~~TD-1~~ | ~~api.ts 双键名兼容需与后端对齐~~ ✅ 已清理为统一 camelCase |
| TD-2 | ListSpiritTeams HTTP 端点未暴露（需后端新增 `/v1/spirit/{id}/teams` 路由） |
| TD-3 | ArchiveTeam RPC 未定义（需后端 proto 定义） |
| ~~TD-4~~ | ~~MemberReadOnlyPanel 仅有占位符~~ ✅ 已增强：执行统计 + assistant 消息展示 + renderMarkdown |
| ~~TD-5~~ | ~~TeamMemberTreeNode 未实现~~ ✅ 已创建 TeamMemberTreeNode.vue + TeamTaskCard 集成 |
| ~~TD-6~~ | ~~面包屑导航未实现~~ ✅ ChatMessagePanel 添加精灵 > 团队 > 成员面包屑 |
| ~~TD-7~~ | ~~重试失败团队功能未实现~~ ✅ TeamProgressCard 添加 retry 按钮，复用 resumeTeam action |
| ~~F-6~~ | ~~成员头像数量不符~~ ✅ slice(0,5)→slice(0,4) |
| ~~F-7~~ | ~~DAG 依赖数量提示缺失~~ ✅ TeamTaskCard 添加前置任务数量 |
| ~~F-4~~ | ~~DQ Score 前端展示~~ ✅ SpiritStatusBar + SynthesisResultCard 展示 |
| ~~F-5~~ | ~~验证门禁前端增强~~ ✅ DAGDiagramCard 验证节点状态渲染 |
| L1-L9 | 轻微问题，后续迭代清理 |

### P2 待实现（需后端配合）

| 编号 | 内容 | 依赖 |
|------|------|------|
| ~~B-1~~ | ~~ListSpiritTeams HTTP handler~~ ✅ Proto + Service + HTTP 路由已实现 | — |
| ~~B-2~~ | ~~ListChildSessions RPC Proto~~ ✅ Proto + Service 已实现 | — |
| ~~B-3~~ | ~~SpiritTeamView/SpiritMemberView Proto~~ ✅ Proto 消息类型已定义 | — |
| ~~B-4~~ | ~~SynthesizeResults Proto~~ ✅ Proto + Service 已实现 | — |
| ~~E-2~~ | ~~前后端验证门禁类型对齐~~ ✅ 已确认一致 | — |
| ~~E-1~~ | ~~spirit_teams_all_completed 载荷分项统计~~ ✅ AllTeamsCompletedResult 增加 TotalTeams/CompletedTeams/FailedTeams | — |
| ~~E-3~~ | ~~SpiritTeamMode 前后端语义映射~~ ✅ 前端 SpiritTeamMode 6种与后端 team_graph_constants.go 完全一致，TopologyType 映射正确 | — |
| E-4 | ChatExecutionCard 级别独立折叠 | 后续迭代 |
| E-5 | i18n 国际化 | 后续迭代 |
