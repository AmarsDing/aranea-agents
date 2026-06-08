# M59-OBS: Chat 精灵模式可观测性 UX — 开发计划

> **版本**：2026-06-08 | **状态**：🔄 待开发
> **需求**：[59-chat-spirit-mode-observability.md](./59-chat-spirit-mode-observability.md) · **设计**：[59-chat-spirit-mode-observability.design.md](./59-chat-spirit-mode-observability.design.md)
> **调研**：[2026-06-08-research-chat-observability-ux.md](../reports/2026-06-08-research-chat-observability-ux.md)

---

## 1. 模块定位

Chat 精灵模式可观测性 UX 增强：7 项方案，遵循"可观测性强但不影响主要内容显示"原则，基于三层可观测性架构（L1 环境层 → L2 结构层 → L3 证据层）。

**代码锚点**：

| 层级 | 路径 | 方案 |
|------|------|------|
| Composable | `web/src/composables/chat/useAutoCollapse.ts` | OBS-01 |
| Composable | `web/src/composables/chat/useContextualLoadingMessage.ts` | OBS-02 |
| Composable | `web/src/composables/chat/useStatusPulse.ts` | OBS-05 |
| Feature | `web/src/features/chat/groupMessagesByTurn.ts` | OBS-01 |
| Feature | `web/src/features/spirit/observabilityConstants.ts` | OBS-02 |
| Feature | `web/src/features/spirit/spiritUi.ts` | OBS-03 |
| Component | `web/src/components/spirit/AgentStatusLabel.vue` | OBS-03 |
| Component | `web/src/components/spirit/SpiritStatusBar.vue` | OBS-04 |
| Component | `web/src/components/spirit/InterruptedTeamCard.vue` | OBS-07 |
| Component | `web/src/components/chat/ChatExecutionCard.vue` | OBS-06 |
| Component | `web/src/components/chat/ChatMessagePanel.vue` | OBS-02/04 |
| Component | `web/src/components/spirit/TaskExecutionPanel.vue` | OBS-03/07 |
| Component | `web/src/components/spirit/TeamTaskCard.vue` | OBS-03/05 |
| Component | `web/src/components/chat/ChatEntitySidebar.vue` | OBS-05 |
| Service | `internal/service/spirit_team.go` | OBS-04 Token 扩展 |

---

## 2. 前置依赖

| 依赖 | 状态 | 说明 |
|------|------|------|
| M59 P0/P0.5 已完成 | ✅ | 精灵模式核心骨架和三阶段编排 |
| `ChatExecutionCard` 已有折叠/展开 | ✅ | 使用 `<q-expansion-item>` |
| `tool_call`/`tool_result` Envelope 携带 AgentName | ✅ | `EnvelopeToolCall.AgentName`/`AgentKey`/`ActivityKind` |
| `AgentNodeStatus` 17 种状态已定义 | ✅ | `orchestration_status.go` |
| `SpiritMember.status` 字段存在 | ✅ | 类型为 `string`，典型值 idle/running/error |
| `OptionsJSON.team_member` 字段存在 | ✅ | 成员消息过滤基础 |
| `ResumeTeamRunExecution` API 存在 | ✅ | 需要 `graph_execution_id` |
| WS 事件回放机制 | ✅ | `lastEventId` + `onReplayState` 回调 |
| `groupMessagesByTurn` 分组 | ✅ | 需扩展 `isCompleted` 字段 |

---

## 3. 开发阶段

### Phase P0 — 核心可观测性体验

> **目标**：对话流自动折叠 + 语境加载消息 + 可折叠工具输出 + Agent 状态标签
> **验收**：OBS-01 / OBS-02 / OBS-03 / OBS-06 全部通过

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| OBS-FE-01 | `observabilityConstants.ts`：语境消息映射表 + 脉冲颜色配置 | `web/src/features/spirit` | 常量定义完整 | ✅ |
| OBS-FE-02 | `spiritUi.ts` 扩展：`AGENT_NODE_STATUS_MAP` + `STATUS_LABEL_CONFIG` | `web/src/features/spirit` | 17→7 聚合映射正确 | ✅ |
| OBS-FE-03 | `groupMessagesByTurn.ts` 扩展：`TurnBlockGroup.isCompleted` 计算属性 | `web/src/features/chat` | 已完成 block 正确标记 | ✅ |
| OBS-FE-04 | `useAutoCollapse` composable：折叠/展开状态管理 | `web/src/features/chat/composables` | 折叠逻辑正确，性能 < 16ms | ✅ 修复：ChatMessagePanel 调用 useAutoCollapse，ChatMessageList 传递 :collapsed 到 TurnBlock（OBS-R01） |
| OBS-FE-05 | `ChatMessagePanel.vue` 集成自动折叠：已完成 block 渲染为折叠态 | `web/src/components/chat` | OBS-01 验收 | ✅ |
| OBS-FE-06 | `useContextualLoadingMessage` composable：语境加载消息逻辑 | `web/src/features/chat/composables` | 事件→消息映射正确 | ✅ 修复：skill 模式 displayLabel 正确提取 tc.summary（OBS-R06） |
| OBS-FE-07 | `ChatMessagePanel.vue` 集成语境加载消息：精灵对话面板顶部显示 | `web/src/components/chat` | OBS-02 验收 | ✅ |
| OBS-FE-08 | `AgentStatusLabel.vue`：Agent 状态标签组件 | `web/src/components/spirit` | 7 种标签正确渲染 | ✅ |
| OBS-FE-09 | `TeamTaskCard.vue` 增加 AgentStatusLabel：折叠态色点 + 展开态标签 | `web/src/components/spirit` | OBS-03 验收（侧边栏） | ✅ |
| OBS-FE-10 | `TaskExecutionPanel.vue` 增加 AgentStatusLabel：成员列表标签 | `web/src/components/spirit` | OBS-03 验收（执行面板） | ✅ |
| OBS-FE-11 | `ChatExecutionCard.vue` 增加自动折叠：completed/failed 时 `expanded=false` | `web/src/components/chat` | OBS-06 验收 | ✅ |
| OBS-FE-12 | 历史消息折叠态恢复：从 `OptionsJSON.tool_event.status` 判断初始折叠 | `web/src/features/chat` | 加载历史消息时已完成工具默认折叠 | ✅ |
| OBS-FE-13 | WS 回放兼容：语境消息和脉冲在 `onReplayState(true)` 期间静默 | `web/src/features/chat/composables` | 回放期间无闪烁 | ✅ |
| OBS-FE-14 | `TaskExecutionPanel.vue` 集成 `ParallelTeamOverview`：替换简化版布局 | `web/src/components/spirit` | 三区布局完整展示 | ✅ |

---

### Phase P1 — 全局感知增强

> **目标**：底部状态栏 + 侧边栏脉冲 + 中断恢复提示
> **验收**：OBS-04 / OBS-05 / OBS-07 全部通过

| ID | 任务 | 影响域 | 验收 | 状态 |
|----|------|--------|------|------|
| OBS-BE-01 | `spirit_team_completed` 事件增加 `total_token_in` / `total_token_out` 字段 | `internal/service/spirit_team.go` | 事件 payload 含 token 统计 | ✅ 修复：从 session 提取实际 InputTokens/OutputTokens，不再硬编码为 0（OBS-R04） |
| OBS-BE-02 | `spirit_teams_all_completed` 事件增加 token 汇总字段 | `internal/service/spirit_team.go` | 事件 payload 含汇总 token | ✅ 修复：biz 层 AllTeamsCompletedResult 增加 token 字段，Service 层使用聚合值（OBS-R05 + OBS-S07） |
| OBS-FE-15 | `SpiritStatusBar.vue`：底部状态栏组件 | `web/src/components/spirit` | 4 个字段正确显示 | ✅ 修复：ChatPage 传递 spiritStatusBar computed 到 ChatMessagePanel（OBS-R02），改用 CSS 变量（OBS-S05），消费 token 数据（OBS-S06） |
| OBS-FE-16 | `ChatMessagePanel.vue` 集成 SpiritStatusBar：底部固定 24px | `web/src/components/chat` | OBS-04 验收 | ✅ 修复：ChatPage 传递 spirit-status-bar prop（OBS-R02） |
| OBS-FE-17 | `useStatusPulse` composable：侧边栏脉冲逻辑 | `web/src/features/chat/composables` | 脉冲颜色和时长正确 | ✅ 修复：useChatWorkspace 调用 useStatusPulse，ChatPage 传递 pulseTeamColors + watcher 触发脉冲（OBS-R03），添加 onUnmounted 清理（OBS-S04） |
| OBS-FE-18 | `ChatEntitySidebar.vue` 集成脉冲：团队卡片状态变化时脉冲高亮 | `web/src/components/chat` | OBS-05 验收 | ✅ |
| OBS-FE-19 | `InterruptedTeamCard.vue`：中断恢复提示卡片 | `web/src/components/spirit` | 显示恢复/取消按钮 | ✅ |
| OBS-FE-20 | `TaskExecutionPanel.vue` 集成 InterruptedTeamCard：interrupted 团队显示恢复提示 | `web/src/components/spirit` | OBS-07 验收 | ✅ |
| OBS-FE-21 | 恢复执行 API 调用：`ResumeTeamRunExecution` + 前端状态更新 | `web/src/features/spirit` | 恢复后团队状态变为 running | ✅ |

---

## 4. 任务板

### P0 任务板（核心可观测性）

| 排序 | ID | 任务 | 依赖 | 状态 |
|------|-----|------|------|------|
| 1 | OBS-FE-01 | observabilityConstants.ts | 无 | ✅ |
| 2 | OBS-FE-02 | spiritUi.ts 状态聚合映射 | 无 | ✅ |
| 3 | OBS-FE-03 | groupMessagesByTurn.ts isCompleted | 无 | ✅ |
| 4 | OBS-FE-04 | useAutoCollapse composable | OBS-FE-03 | ✅ 修复：OBS-R01 |
| 5 | OBS-FE-05 | ChatMessagePanel 自动折叠集成 | OBS-FE-04 | ✅ |
| 6 | OBS-FE-06 | useContextualLoadingMessage composable | OBS-FE-01 | ✅ 修复：OBS-R06 |
| 7 | OBS-FE-07 | ChatMessagePanel 语境消息集成 | OBS-FE-06 | ✅ |
| 8 | OBS-FE-08 | AgentStatusLabel.vue | OBS-FE-02 | ✅ |
| 9 | OBS-FE-09 | TeamTaskCard 增加 AgentStatusLabel | OBS-FE-08 | ✅ |
| 10 | OBS-FE-10 | TaskExecutionPanel 增加 AgentStatusLabel | OBS-FE-08 | ✅ |
| 11 | OBS-FE-11 | ChatExecutionCard 自动折叠 | 无 | ✅ |
| 12 | OBS-FE-12 | 历史消息折叠态恢复 | OBS-FE-11 | ✅ |
| 13 | OBS-FE-13 | WS 回放兼容 | OBS-FE-06 + OBS-FE-17 | ✅ |
| 14 | OBS-FE-14 | TaskExecutionPanel 集成 ParallelTeamOverview | OBS-FE-10 | ✅ |

**并行分组**：

- **A 组**（可并行）：OBS-FE-01 + OBS-FE-02 + OBS-FE-03 + OBS-FE-08 + OBS-FE-11
- **B 组**（依赖 A）：OBS-FE-04 + OBS-FE-06 + OBS-FE-09 + OBS-FE-10 + OBS-FE-12
- **C 组**（依赖 B）：OBS-FE-05 + OBS-FE-07 + OBS-FE-13 + OBS-FE-14

### P1 任务板（全局感知增强）

| 排序 | ID | 任务 | 依赖 | 状态 |
|------|-----|------|------|------|
| 1 | OBS-BE-01 | spirit_team_completed 增加 token 字段 | 无 | ✅ 修复：OBS-R04 |
| 2 | OBS-BE-02 | spirit_teams_all_completed 增加 token 字段 | 无 | ✅ 修复：OBS-R05 + OBS-S07 |
| 3 | OBS-FE-15 | SpiritStatusBar.vue | OBS-BE-01 | ✅ 修复：OBS-R02 + OBS-S05 + OBS-S06 |
| 4 | OBS-FE-16 | ChatMessagePanel 集成 SpiritStatusBar | OBS-FE-15 | ✅ 修复：OBS-R02 |
| 5 | OBS-FE-17 | useStatusPulse composable | 无 | ✅ 修复：OBS-R03 + OBS-S04 |
| 6 | OBS-FE-18 | ChatEntitySidebar 集成脉冲 | OBS-FE-17 | ✅ |
| 7 | OBS-FE-19 | InterruptedTeamCard.vue | 无 | ✅ |
| 8 | OBS-FE-20 | TaskExecutionPanel 集成 InterruptedTeamCard | OBS-FE-19 | ✅ |
| 9 | OBS-FE-21 | 恢复执行 API 调用 | OBS-FE-20 | ✅ |

**并行分组**：

- **A 组**（可并行）：OBS-BE-01 + OBS-BE-02 + OBS-FE-17 + OBS-FE-19
- **B 组**（依赖 A）：OBS-FE-15 + OBS-FE-18 + OBS-FE-20
- **C 组**（依赖 B）：OBS-FE-16 + OBS-FE-21

---

## 5. 验收标准

### Phase P0

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

### Phase P1

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

---

## 6. 依赖与风险

| 风险 | 缓解 |
|------|------|
| `TurnBlockGroup.isCompleted` 计算影响消息分组性能 | 仅在 block 内工具状态变化时重新计算，使用 computed 缓存 |
| `ChatExecutionCard` 自动折叠可能干扰用户正在查看的工具 | 仅在用户未主动展开该卡片时自动折叠；用户主动展开的卡片不自动折叠 |
| WS 回放期间语境消息闪烁 | 统一通过 `isReplaying` ref 控制，回放期间所有 L1 方案静默 |
| `AgentNodeStatus` 数据源可能延迟 | 侧边栏使用 `SpiritMember.status`（实时），执行面板使用 `AgentNodeStatus`（可能延迟 500ms） |
| Token 统计事件扩展需后端修改 | P1 阶段实施，P0 阶段底部状态栏不显示 Token 字段 |
| `ResumeTeamRunExecution` 需 `graph_execution_id` | 无 Graph 执行的团队显示"不支持断点恢复"，不阻塞用户 |

---

## 7. 关联文档

| 文档 | 关系 |
|------|------|
| [59-chat-spirit-mode.md](./59-chat-spirit-mode.md) | 父需求 |
| [59-chat-spirit-mode.design.md](./59-chat-spirit-mode.design.md) | 父设计 |
| [59-chat-spirit-mode.development.md](./59-chat-spirit-mode.development.md) | 父开发计划 |
| [2026-06-08-research-chat-observability-ux.md](../reports/2026-06-08-research-chat-observability-ux.md) | 调研基础 |

---

## 8. 审查修复记录

> 2026-06-08 代码审查发现并修复的问题

### 阻断项修复

| ID | 问题 | 修复 |
|----|------|------|
| OBS-R01 | useAutoCollapse composable 是死代码，TurnBlock.collapsed 未传递 | ChatMessagePanel 调用 useAutoCollapse，ChatMessageList 传递 :collapsed 到 TurnBlock |
| OBS-R02 | SpiritStatusBar 数据流断链，ChatPage 未传递 spirit-status-bar prop | ChatPage 计算 spiritStatusBar computed 并传递给 ChatMessagePanel |
| OBS-R03 | useStatusPulse composable 是死代码，pulseTeamColors 未传递 | useChatWorkspace 调用 useStatusPulse，ChatPage 传递 pulseTeamColors + watcher 触发脉冲 |
| OBS-R04 | spirit_team_completed 事件 token 字段硬编码为 0 | Service 层从 session 提取实际 InputTokens/OutputTokens |
| OBS-R05 | spirit_teams_all_completed 事件 token 字段硬编码为 0 | biz 层 AllTeamsCompletedResult 增加 token 字段，Service 层使用聚合值 |
| OBS-R06 | useContextualLoadingMessage skill 模式缺少 {summary} 占位符 | 修复 displayLabel 逻辑，正确提取 tc.summary |
| OBS-R07 | M59-OBS 测试覆盖率为零 | 新增 useAutoCollapse/useContextualLoadingMessage/useStatusPulse 单元测试 |

### 建议项修复

| ID | 问题 | 修复 |
|----|------|------|
| OBS-S01 | canResume 使用 dagNodeId 而非 graphExecutionId | SpiritTeam 类型增加 graphExecutionId，canResume 检查 graphExecutionId \|\| dagNodeId |
| OBS-S02 | interruptReason 硬编码为 '服务器重启' | SpiritTeam 类型增加 interruptReason，从事件元数据读取，默认 '执行中断' |
| OBS-S04 | useStatusPulse 未自动清理 setTimeout | 添加 onUnmounted(() => cleanup()) |
| OBS-S05 | SpiritStatusBar 使用 Quasar 色名 | 改用 CSS 变量 (var(--color-accent) 等) |
| OBS-S06 | 前端未消费 token 数据 | SpiritTeam 增加 tokenIn/tokenOut，store 消费事件 token 字段，ChatPage 聚合到 spiritStatusBar |
| OBS-S07 | AllTeamsCompletedResult 缺少 token 字段 | 增加 TotalTokenIn/TotalTokenOut，CheckAllTeamsCompleted 聚合 session metrics |
