# ADR-10: TeamStage 运行隔离 ID 与 standalone 团队 teams.status 归属

## 状态：已接受（2026-08-05）

## 背景

S-3 评审发现团队 v2 记录链（team_stages_v2 → team_runs_v2 → member_sessions_v2）存在三类结构性缺陷：

1. **同团队多轮 turn 碰撞同一行**。`NewTeamStageActivityID(teamID)` 仅以 teamID 派生确定性 ID，standalone（Mode A）团队每轮 chat turn 都命中同一 team_stages_v2 行：第一轮终态后，第二轮的 Pending→Running 转换被 TeamStage 状态机拒绝（已终态），状态永久冻结；TeamRun/MemberSession 同理。
2. **团队 chat 入口无 run 维度**。`executeTeamTurnViaHooks` 在 RootTaskActivityID 注入点前提前 return，runner/service 派生 v2 ID 时 run 维度为空；turn 树挂到 tasks_v2 不存在的幽灵 ID，且 pending-queue 出队的团队 turn 复用上一轮 rootTaskID（loopCtx 携带），同样碰撞。
3. **standalone 团队 teams.status 被每轮 run 驱动**。teams 表状态机是 Mode B（AutoCreated）一次性编排生命周期（pending→running→completed，无 completed→running 回路）。standalone 团队是用户创建的持久实体、反复运行：一次 failed 即把团队实体永久毒化为 failed（pending→failed 合法），completed 则恒被 FSM 拒绝（pending→completed 非法）刷 Warn 噪声。

## 决策

1. **S-3：TeamStage ID 公式引入 run 维度**。`NewTeamStageActivityID(teamID, rootTaskID)`——rootTaskID 非空时按 (teamID, rootTaskID) 派生，每轮 turn 一行；为空降级 legacy teamID-only 公式（兼容重启后 DB 恢复的旧 session）。TeamRunV2ID/MemberSessionActivityID 从 teamStageID 链式派生，天然继承隔离。
2. **S-5/S-5b：团队入口统一注入 RootTaskActivityID + 根 Task v2 生命周期**。`executeTeamTurnViaHooks`（S-5）与 pending-queue 团队分支（S-5b）同一姿态：每条用户消息 = 新 run，注入全新 RootTaskActivityID；幂等 `UpsertTask` 建根（continuation turn 继承 ParentTaskID 不新建）；turn 结束 `CompleteTaskTerminal` 终态化（防重启恢复把已完成团队任务误判 interrupted）。
3. **ctx-less 路径查最新行替代公式重放**。CancelTeam / pause-resume / spirit view 等用户动作入口 ctx 无 RootTaskActivityID，新增 reader `GetLatestTeamStageByTeam(teamID)` 定位最新行；CancelTeam 把其 TaskID（Mode B 下 = rootTaskID）注入 ctx，使终态 pass 派生同一批 run-isolated ID。
4. **S-4：standalone 团队不驱动 teams.status**。`handleStandaloneTeamTurnResult` 删除 teams 表 TransitionStatus；每轮 run 终态只落 v2 三表。Mode B（AutoCreated）行为不变。

## 后果

正面：

- 同团队多轮 turn 各自拥有独立 team_stages_v2/team_runs_v2/member_sessions_v2 行，FSM 转换与版本守卫恢复正常；前端按轮次展示 run 历史成为可能。
- 团队 turn 树挂到真实 tasks_v2 根行，重启恢复语义正确。
- standalone 团队实体状态不再被单次 run 的成败毒化。

负面 / 代价：

- ID 公式依赖 ctx 传递的 rootTaskID，任何新调用点必须确保注入（S-5b 即是补上的缺口）；ctx-less 入口必须走 GetLatestTeamStageByTeam 查询（多一次读，可接受）。
- Mode A runner 事件不写 TeamStage.TaskID 时 CancelTeam 降级 legacy 公式，与 S-3 行错位——有守卫（TaskID 为空不注入），影响面限于重启恢复路径。

## 替代方案

- **v2 三表改用随机 UUID + 唯一索引**：放弃确定性派生。否决——确定性 ID 是双写者（runner/assembler 与 service）幂等收敛到同一行的机制，改 UUID 需要引入额外的查找与去重逻辑。
- **standalone 复用 Mode B teams.status 并给 FSM 加 completed→running 回路**：否决——teams.status 对持久实体表达「团队当前是否在编排」语义不清，且会让团队管理页的展示状态随每轮 chat 抖动；run 生命周期本就属于 v2 三表。
- **pending-queue 复用 loopCtx 的 rootTaskID**：否决——这正是 S-3 要消除的碰撞源。

## 关联

- ADR-09（成员终态单写者与版本权威带）——S-3/S-4 延续同一「v2 三表为 run 真相源」姿态。
- 实现锚点：`internal/agent/activity_context.go`（ID 公式）、`internal/service/team_turn_hooks.go`（S-5）、`internal/service/chat_orchestrator_turn_dispatch.go`（S-5b）、`internal/service/spirit_team.go`（CancelTeam 富化 + S-4）、`internal/data/team_stage_v2_repo.go`（GetLatestTeamStageByTeam）、`internal/team/team_graph_run_coordinator.go`（session 捕获 rootTaskID）。
