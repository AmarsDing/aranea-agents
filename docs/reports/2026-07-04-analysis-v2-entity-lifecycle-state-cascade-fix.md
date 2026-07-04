# v2 实体生命周期与状态级联关闭修复方案

> **类型**：分析报告 + 修复计划
> **日期**：2026-07-04
> **背景**：22:23 时刻对话数据库实证显示 v2 实体状态级联关闭失败，导致前端渲染异常

---

## 一、数据库实证（22:23 时刻 Postgres 查询）

### 1.1 正常关闭的实体

| 实体 | 数量 | 状态 | 说明 |
|------|------|------|------|
| Task | 1 | completed (22:23:54) | 根 Task 正常关闭 |
| Spirit Turn | 1 | completed | Spirit turn 正常关闭 |

### 1.2 卡在 running/pending 的实体（Task 完成后 30 秒仍未关闭）

| 实体 | 数量 | 状态 | 关键字段异常 |
|------|------|------|------------|
| TeamStage | 2 | running/executing | team_name 为空字符串 |
| TeamRun | 2 | running | - |
| MemberSession | 4 | running | - |
| PlanBoard | 2 | **planning**（从未进入 executing） | - |
| GraphStage | 2 | running | 第一个 GraphStage 无 graph_node |
| GraphNode | 2 | running | dag_node_id 为空、label 为空 |
| team Turn | 2 | running | agent_key 是 `__system_admin__` |
| Step | 6 | pending | - |

### 1.3 表数据异常

- `plan_steps_v2` 表**完全为空**（0 条记录）
- `graph_nodes_v2.dag_node_id` 为空字符串
- `graph_nodes_v2.label` 为空字符串

---

## 二、问题清单与根因分析

### 问题 D1：Spirit Task 完成时机过早（设计层）

**现象**：Spirit Task 在 22:23:54 completed，但 team 一直执行到 22:24:35。

**根因**：Spirit turn 和 team turn 之间无同步等待。Spirit turn 在 `plan_and_execute` 工具返回后立即完成，team 在后台异步执行。

**现有机制**：已实现 system-push 模式（方案B）：
1. Spirit turn #1 提前完成
2. team 后台异步执行
3. 所有 team 完成后，`checkAllTeamsCompleted` 触发 `turnGateway.ExecuteTurn` 注入合成消息
4. 启动 Spirit turn #2 做最终 conclusion

**问题本质**：system-push 机制本身设计正确，但因问题 C1（状态级联关闭失败）导致 `checkAllTeamsCompleted` 从未被触发。

**修复方向**：不阻塞 Spirit turn，而是修复 C1 让 system-push 正确工作。同时在 Spirit turn 完成时，如果检测到有 team 在执行，**延迟关闭 Task 状态**（保持 running），等所有 team 完成后再标记 completed。

### 问题 D2：PlanStep 从未持久化 + PlanBoard 卡在 planning（设计层）

**现象**：`plan_steps_v2` 表为空，PlanBoard 卡在 `planning` 状态。

**根因**：
1. `publishV2PlanBoard`（task_planner_impl.go:1340）创建 PlanBoard 时设置 `Status: PlanBoardStatusPlanning`
2. `PlanExecutor` 启动 DAG 执行时**没有更新 PlanBoard 状态**为 `executing`
3. DAG 执行完成后**没有更新 PlanBoard 状态**为 `completed`
4. PlanStep 持久化代码路径正确（publishV2PlanBoard → seq.Publish → UpsertPlanStep），但可能是旧数据问题或 Sequencer 持久化失败

**修复方向**：
1. `PlanExecutor.Subscribe` 启动 DAG 时，更新 PlanBoard 状态为 `executing`
2. `dagRun.run` 完成时，更新 PlanBoard 状态为 `completed`/`failed`
3. 清空数据库后重新测试验证 PlanStep 持久化

### 问题 C1：Team 完成后子实体状态未级联关闭（代码层，最严重）

**现象**：4 个 MemberSession、2 个 TeamRun、2 个 TeamStage 全部卡在 running。

**根因**：`HandleTeamTurnResult` → `publishV2TeamRunCompletion` 链路失败。

**可能原因**：
1. `RunTurnFromInput` 异步返回，`HandleTeamTurnResult` 在 team 实际完成前就被调用
2. `publishV2TeamRunCompletion` 的 DB 查询 `sessions.Search(TeamID=teamID)` 返回空结果
3. `publishV2TeamRunCompletion` 发布的 updated 事件未正确持久化

**修复方向**：
1. 在 `publishV2TeamRunCompletion` 中增加 DB 查询失败日志
2. 确保 `HandleTeamTurnResult` 在 team turn 真正完成后才调用
3. 增加 MemberSession 状态关闭的兜底机制（超时强制关闭）

### 问题 C2：pending queue 路径 RootTaskActivityID 丢失（代码层）

**位置**：[chat_orchestrator_turn_dispatch.go:253](file:///f:/aranea-agents/internal/service/chat_orchestrator_turn_dispatch.go#L253)

**根因**：`bgCtx` 来自 `appctx.Ctx()`，不携带 RootTaskActivityID。

**后果**：`publishV2TeamRunCompletion` 取到 `rootTaskID=""`，MemberSession updated 事件的 TaskID 为空。

**修复方向**：在 dispatch.go 的 bgCtx 中注入 RootTaskActivityID（从 PlanBoard.TaskID 或 ctx 恢复）。

### 问题 C3：team_name 为空（代码层）

**现象**：数据库 team_stages_v2.team_name 为空字符串。

**根因**：代码路径正确（spirit_team.go:1069 携带了 team.DisplayName），但可能是旧数据问题（修复前创建的 Team 没有 DisplayName）。

**修复方向**：
1. 清空数据库后重新测试
2. 在 `publishSpiritTeamAssembled` 增加防御性检查：如果 `team.DisplayName` 为空，记录 Warn 日志

### 问题 C5：多处绕过 seq.Publish 不持久化（代码层）

**位置**：
- [chat_event_publisher.go:72](file:///f:/aranea-agents/internal/service/chat_event_publisher.go#L72) PublishTurnFailure
- [chat_orch_await.go:145](file:///f:/aranea-agents/internal/service/chat_orch_await.go#L145)
- [pre_planning_gate.go:138](file:///f:/aranea-agents/internal/service/pre_planning_gate.go#L138)
- [chat_run_gateway.go:316](file:///f:/aranea-agents/internal/service/chat_run_gateway.go#L316)

**后果**：这些 Notice step 刷新后消失。

**修复方向**：将 `eventBus.Publish` 改为 `seq.Publish`（或通过 `publishV2Event` 兜底）。

### 问题 C7：TaskCard.vue 过滤掉了 planning 状态的 PlanBoard（代码层，新发现）

**位置**：[TaskCard.vue:110-115](file:///f:/aranea-agents/web/src/components/chat/v2/TaskCard.vue#L110-L115)

**现象**：PlanBoard 卡在 `planning` 状态（因问题 D2），且 `plan_steps_v2` 表为空（steps.length === 0），被过滤逻辑排除，导致整个执行计划面板不显示。

**根因**：我之前添加的过滤逻辑 `return steps.length > 0 || pb.Status === 'completed' || pb.Status === 'failed'` 把 `planning` 状态的 PlanBoard 过滤掉了。

**修复方向**：放宽过滤条件，`planning` 状态的 PlanBoard 也应显示（因为 team 可能已经在执行）。

---

## 三、修复方案（按优先级）

### Phase 1：修复状态级联关闭（C1 + C2）— 最关键

**目标**：team 完成后所有子实体正确关闭，system-push 正确触发。

| # | 修复点 | 文件 | 说明 |
|---|--------|------|------|
| 1.1 | 修复 pending queue RootTaskActivityID | chat_orchestrator_turn_dispatch.go:253 | bgCtx 注入 RootTaskActivityID |
| 1.2 | 增加 publishV2TeamRunCompletion 日志 | spirit_team.go:1440 | DB 查询失败时记录 Warn，便于定位 |
| 1.3 | 增加 MemberSession 超时兜底关闭 | spirit_team.go | team 完成后如果 MemberSession 仍 running，强制关闭 |

### Phase 2：修复 PlanBoard 状态转换（D2）

**目标**：PlanBoard 状态正确从 planning → executing → completed 转换。

| # | 修复点 | 文件 | 说明 |
|---|--------|------|------|
| 2.1 | PlanExecutor 启动 DAG 时更新 PlanBoard 为 executing | plan_executor.go | Subscribe 后发布 PlanBoardUpdatedEvent(Status=executing) |
| 2.2 | DAG 完成时更新 PlanBoard 为 completed | plan_executor.go | dagRun.run 结束时发布 PlanBoardUpdatedEvent(Status=completed) |
| 2.3 | 验证 PlanStep 持久化 | - | 清空 DB 后重新测试 |

### Phase 3：修复 TaskCard.vue 过滤逻辑（C7）

**目标**：planning 状态的 PlanBoard 也能显示。

| # | 修复点 | 文件 | 说明 |
|---|--------|------|------|
| 3.1 | 放宽 PlanBoard 过滤条件 | TaskCard.vue:110-115 | planning 状态也显示 |

### Phase 4：修复绕过 seq.Publish 的 4 处（C5）

**目标**：所有 v2 事件都经过 Sequencer 持久化。

| # | 修复点 | 文件 | 说明 |
|---|--------|------|------|
| 4.1 | PublishTurnFailure 改用 seq.Publish | chat_event_publisher.go:72 | - |
| 4.2 | chat_orch_await 改用 seq.Publish | chat_orch_await.go:145 | - |
| 4.3 | pre_planning_gate 改用 seq.Publish | pre_planning_gate.go:138 | - |
| 4.4 | chat_run_gateway 改用 seq.Publish | chat_run_gateway.go:316 | - |

### Phase 5：Task 状态延迟关闭（D1）

**目标**：Spirit turn 完成时，如果有 team 在执行，Task 保持 running。

| # | 修复点 | 文件 | 说明 |
|---|--------|------|------|
| 5.1 | Spirit turn 完成时检查 team 状态 | projector.go OnTurnEnd | 如果有 team running，Task 保持 running |
| 5.2 | 所有 team 完成后关闭 Task | spirit_team.go checkAllTeamsCompleted | system-push 触发前先关闭 Task |

### Phase 6：team_name 防御性检查（C3）

**目标**：确保 team_name 不为空。

| # | 修复点 | 文件 | 说明 |
|---|--------|------|------|
| 6.1 | publishSpiritTeamAssembled 防御性检查 | spirit_team.go:1065 | team.DisplayName 为空时记录 Warn |
| 6.2 | 清空 DB 后验证 | - | 确认新数据 team_name 正确 |

---

## 四、文档同步计划

| 文档 | 同步内容 | 触发规则 |
|------|---------|---------|
| docs/development/1-chat.design.md §3.6.3 | 渲染顺序补充"planning 状态 PlanBoard 也显示" | DOC-SYNC-1 |
| docs/development/1-chat.design.md §B.4 | PlanBoard 状态机补充 planning→executing→completed | DOC-SYNC-1 |
| docs/development/1-chat.development.md | 记录本次修复任务清单与状态 | DOC-SYNC-4 |

---

## 五、验证方式

1. **清空数据库**：删除所有 v2 实体记录，确保无旧数据干扰
2. **运行新对话**：发起一个需要 team 编排的复杂任务
3. **数据库验证**：
   - Task completed 后，所有 TeamStage/TeamRun/MemberSession 是否 completed
   - PlanBoard 是否经历 planning→executing→completed
   - plan_steps_v2 表是否有记录
   - team_stages_v2.team_name 是否非空
   - graph_nodes_v2.dag_node_id 和 label 是否非空
4. **前端验证**：
   - 执行计划面板是否显示（planning 状态也显示）
   - 团队名称是否正确
   - agent 执行内容是否显示
   - agent 状态是否从执行中变为完成
5. **编译验证**：`go build ./...` + `pnpm build` + `pnpm lint`
6. **测试验证**：`go test ./internal/service/... ./internal/agent/... ./internal/biz/...` + `pnpm test`
