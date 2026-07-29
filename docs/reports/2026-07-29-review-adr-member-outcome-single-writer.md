# ADR-09: 成员终态单写者 + 版本权威带（outcome 哨兵化）

## 状态：已接受（2026-07-28 决策，2026-07-29 哨兵化修正）

## 背景

12:33 skill 安装会话暴露成员状态失真：成员返回最终文本即显示成功，实际安装失败。根因是**成员终态存在双写者且仲裁失效**：

1. **runner 投影 completed**：`team/runner_helpers.go` 在成员产出最终文本时发布 MemberSession completed——这是**消息生命周期**而非**工作结果**。
2. **service 终态兜底**：`publishV2TeamRunCompletion` 按团队状态发布成员终态，但与 runner 投影同版本（Version=1/2 混用），`UpsertMemberSession` 的 `VersionLT` 守卫与前端 `activityV2Store` 守卫把它们当重复事件丢弃，导致 failed 覆盖被静默拒绝（F10 修复一度失效）。
3. **无 step 的缺口成员**（创建失败/深度超限）由 `finalizePendingSessionActivities` 补发 completed，再次把消息生命周期升格为成员终态。

完整证据链（中断 session / 失败 step / 交付物门 / 验证门）只在**团队终态时刻**齐备，任何执行期的成员级「成功」投影都缺乏裁决依据。

## 决策

### 1. 单写者：成员终态只能由 service 终态 outcome pass 裁决

- runner 只允许发布 **created**（running）生命周期事实；成员 completed/failed/skipped 投影全部删除（含 `finalizePendingSessionActivities`）。
- service `publishV2TeamRunCompletion` 是唯一终态裁决点，证据链按序执行：
  1. **Fix-1 真实交付物门**（DAG 团队）：无交付物 → 团队翻转 failed；
  2. **F9 验证门**（definition `verification_gates`，当前唯一自动来源：skill 安装 `tool_assertion` 门）：拒绝或 infra 错误均 fail-closed 翻转 failed——「装了但不可用」不得报成功；
  3. **F10 成员级证据覆盖**：团队 completed 时 per-member 调 `MemberExecutionEvidence`（中断 session / 失败 step），单成员团队追加交付物证据；cancelled 保持 skipped。
  4. **F4 兜底**：定义成员无 agent session 时以 team session 补发终态，保证定义成员状态必达终态。

### 2. 版本权威带：Version 是写者权威层级而非任意编号

```
created = 1（runner，生命周期事实，执行期可达）
evidence = 2（预留：runner 事实性失败/取消；当前无生产写者）
outcome = 1 << 40（终态写者族：service outcome pass / Mode B finish / 崩溃 recovery）
```

- 每个版本带内只有一个写者（族）；守卫（DB `VersionLT` + 前端 `version <=`）回归幂等去重本职，不再承担跨写者仲裁。
- **outcome 取哨兵大值而非固定小值（2026-07-29 修正）**：生命周期写者（pause/resume）使用 `Version++` 单调递增，若 outcome 为固定小值（初版 V=3），pause→resume 循环可使 running 成员达到相同版本，终态事件被守卫静默拒绝（成员永久 running，100% 可复现）。哨兵保证任何递增写者现实中无法到达本带——**终态恒赢，且终态之后无写者**。
- 生命周期写者纪律：`syncMemberSessionStatus` 对已终态记录直接跳过；目标为终态（Mode B finish）携带 outcome 带；paused/running 走 `Version++`。崩溃 recovery 对非终态孤儿直接 `SetVersion(outcome)` 标 failed。

### 3. 前端零改动

哨兵值 `1<<40` < `2^53`（JS Number 安全整数），WS/JSON 数值比较语义不变。

### 4. standalone（Mode A）团队终态可达性（2026-07-29 F-1/S-1/S-2 补充）

单写者裁决点只对「能到达它的团队」有效。排查发现 standalone 团队（用户手动创建、非编排派发）存在三层可达性断点，成员同样永 running：

- **F-1**：`HandleTeamTurnResult` 对非 AutoCreated 团队直接早退 → 拆出 `handleStandaloneTeamTurnResult` 精简终态 pass（复用同一 `publishV2TeamRunCompletion` 证据链与哨兵带；剔除编排专属职责：交付物门/F9 验证门/recordTeamCompletion/依赖调度/synthesis/自动归档）。
- **F-3/S-1 聚合根回退**：standalone 团队 `ParentSessionID` 与 `team.SpiritSessionID` 均为空（`CreateTeam` 不落该字段）。回退语义统一为 **team session ID 即聚合根**（runner `deriveSpiritSessionID` 回退 `sess.ID`、hooks/dispatch 调用点回退、`HandleTeamTurnResult` standalone 分支内 `resolveStandaloneSpiritSessionID` 兜底——`CancelTeam` 等无 session 上下文入口的唯一可达路径）。
- **S-2 取消对齐**：standalone cancelled 时 running 成员 session 一并转 `interrupted`（与 AutoCreated 路径同姿态），DB session 层与 MemberSession v2 skipped 保持一致。

## 后果

**正面**：
- 成员状态以执行证据为据，「出文本 ≠ 工作成功」从结构上杜绝；
- 终态事件版本单调恒赢，failed 覆盖（F10/F9 翻转、团队失败传播）不再被静默丢弃；
- 版本带语义文档化（`biz/member_session.go` `MemberSessionVersion*` 常量），新增写者有明确归属带；
- 回归测试锁定：`TestMemberSessionV2Repo_Upsert_OutcomeSentinelAlwaysWins`（pause→resume→outcome 链 + 终态后迟到事件拒绝）。

**负面/残差**：
- TeamRun v2 完成事件在 `GetTeamRun` 读失败时降级 `Version=2`：成员曾 pause（V≥2）且恰逢 DB 读故障时终态可能丢失（已有 Warn 日志；概率低，stale-callback 守卫已过滤主要竞态）。记录为已知残差，不在本 ADR 修复。
- runner `publishTeamStepActivity` 的 updated 分支（evidence 带）保留 completed→Completed 的状态映射但无生产调用方；未来新增调用方须遵守「终态禁止落入 evidence 带」纪律。

## 替代方案

| 方案 | 未选原因 |
|------|---------|
| 所有写者 read-then-`max(band, existing+1)` | runner 当前不读库，侵入大；引入读放大与竞态窗 |
| pause/resume 固定写 evidence 带（V=2） | 第二次 pause→resume 循环被守卫拒绝，状态持久化丢失，破坏单调性 |
| outcome 固定小值（V=3，初版） | 与 `Version++` 写者必然碰撞（本 ADR 修正的 P0 回归） |
| 保留 runner completed 投影 + 提高其版本 | 消息生命周期冒充工作结果，根因未除 |

## 代码锚点

- 版本带常量 + `IsMemberSessionTerminal`：`internal/biz/member_session.go`
- outcome pass（证据链 ①~④）：`internal/service/spirit_team.go` `HandleTeamTurnResult` / `publishV2TeamRunCompletion` / `resolveMemberOutcomeStatus`
- F9 验证门：`internal/biz/verification_gate.go`（`tool_assertion`）+ `cmd/admin/wire.go` `provideVerificationGateExecutor`
- runner 只写 created：`internal/team/runner_helpers.go` `publishTeamStepActivity`
- 生命周期写者纪律：`internal/service/chat_pause.go` `syncMemberSessionStatus`、`internal/service/team_pause.go` `syncV2TeamRunStatus`
- 崩溃 recovery 终态：`internal/data/v2_recovery_repo.go`
- standalone 终态 pass：`internal/service/spirit_team.go` `handleStandaloneTeamTurnResult` / `resolveStandaloneSpiritSessionID`；调用点回退 `team_turn_hooks.go` / `chat_orchestrator_turn_dispatch.go` / `runner_team_trpc_phases.go` `deriveSpiritSessionID`
- 守卫：`internal/data/member_session_v2_repo.go` `UpsertMemberSession`、`web/src/stores/chat/activityV2Store.ts` `upsertMemberSession`
