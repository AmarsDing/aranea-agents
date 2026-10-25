# 审查报告：精灵编排链路深审（入口→组队→经验复用→graph→状态→UI）

> 日期：2026-10-20 · 类型：review（代码审计） · 范围：精灵会话分配任务后的全链路
> 方法：7 子系统并行深读（60 agents）→ 53 条候选发现 → 双轮独立对抗验证 → 16 条分歧经第三轮 16 个高 effort 裁判终裁。
> **终审结论：25 confirmed / 8 plausible / ~20 refuted（已驳回项不计入修复范围）。**
> 修复方案与实施：见 [87-orchestration-chain-hardening](../development/87-orchestration-chain-hardening.md) 三件套。

---

## 1. 全链路机制总览（已核实）

```
用户消息 → ChatOrchestrator.runNativeAgentTurnBody（锁+准入+OwnerType 分流）
  → Intent Pass → PrePlanningGate（QuickAssess 分档 simple/moderate/complex）
  → resolveSpiritTurnOrchestration（SpiritPhase Idle/Ready/Orchestrating）
  → plan_and_execute：Plan → Allocate（三层匹配 exact/semantic/llm_cold_start + roster_miss 重规划一次）
  → PublishV2Board → PlanExecutor.startBoardDAG（lease）
  → dispatchStep → Orchestrate → AssembleTeam（复用检查→组队→组织面 R17→事务建队）
  → publishSpiritTeamAssembled（TeamStage V=1）→ StartTeamTurn（Pending→Running）
  → RunTurnFromInput → HandleTeamTurnResult（交付物门/F9 验证门/F10 部分失败+quorum 收敛）
  → scheduleDependentTeams / PlanExecutor.checkDownstream → 终态事件（Sequencer Critical=WBPF）
  → checkAllTeamsCompleted → synthesis turn → deliverables notice
前端：WS(v2_event，33 种 kind 前后端一一对应）→ useChatV2EventHandlers → activityV2Store（Version 守卫）
  → stores/spirit → UnifiedExecutionPanel/TeamProgressCard/AgentSidebarCard/ObserveNode
```

机制骨架完备：CAS 终态守卫、DAG 环检测、checkpoint 崩溃恢复（fail-closed）、事件可靠性分级（AS-EVT-01）均有实现。

---

## 2. Confirmed 缺陷清单（25 条）

### 2.1 入口分发 — 取消语义是全链路最重的伤

| # | 级别 | 发现 | 位置 |
|---|------|------|------|
| C1 | HIGH | 用户取消被硬编码 failed（3 处分支），无 `runWasCancelled` 判别；级联：cancel-skip 失效触发失败叙事 synthesis；`context.Canceled` 被失败分类器判 transient → 用户点停止 ~5s 后团队被自动重跑 | `internal/service/team_turn_hooks.go:116`、`spirit_team.go:290`、`chat_orchestrator_turn_dispatch.go:419`、`step_failure_classifier.go:73` |
| C2 | HIGH | `StartTeamTurn` 无 run 注册/会话锁，调度入口与 chat 入口可双跑同一团队（双 run 行、双 token、双回调） | `internal/service/spirit_team.go:154-176` |
| C3 | MEDIUM | synthesis 失败后 30s 轮询复活不可达（poller 只扫 running 会话），报告只能等用户下一输入 | `internal/service/spirit_team_schedule.go:255-263` |
| C4 | MEDIUM | `recordTeamCompletion` 同状态重放非幂等：守卫只丢异态回调，`TransitionStatus` 被拒后仍无条件记账 → orchCache 重复计数、交付物重落、演化建议重复 | `internal/service/spirit_team_handle_result.go:43-54,196-205` |
| C5 | LOW | TeamRun cancelled 走 FailedEvent 工厂；事件平面缺 `EventKindTeamRunCancelled`，Monitor/CriticalJournal 无法区分取消与失败 | `internal/service/spirit_team_completion.go:208-215`、`internal/biz/event.go:131-134` |

### 2.2 组队与花名册（铁律守住，2 处缺陷）

| # | 级别 | 发现 | 位置 |
|---|------|------|------|
| C6 | MEDIUM | 单成员 mode 归一化在内建模板路由重映射之前 → 单成员团队被错误授予 RoleSynthesizer、mode 被翻成 coordinator | `internal/biz/spirit_assembly.go:94-99`、`graph_template_route.go:28-52` |
| C7 | MEDIUM | 并行团队配额只在 `plan_and_execute` 入口检查；DAG 扇出无配额 → 深/扇形 DAG 可超 `max_concurrent_teams` | `internal/tools/spirit_tools.go:210-223`、`internal/service/plan_executor_dag_flow.go:65-90` |

### 2.3 经验复用（7 连击，全链路最薄弱）

| # | 级别 | 发现 | 位置 |
|---|------|------|------|
| C8 | HIGH | 配方只升不降（`>=` 早退）；失败零降权信号（仅 output!=nil 学习）；服役配方对质量劣化失明 | `internal/biz/spirit_orchestration_cache.go:289,413`、`internal/agent/task_orchestrator_impl.go:206` |
| C9 | HIGH | `ConstraintFingerprint`/`PlaybookID` 恒空；`RecipeKeysReusable` 因 playbook fail-open 恒放行；`tryDomainRecipe`/`AllocateExplicit` 无指纹检查 → 剧本换约后旧团队照复用 | `spirit_orchestration_cache.go:26-27,293-302,417-427`、`agent_domain_match.go:42-72`、`spirit_tools_part2.go:96-100` |
| C10 | HIGH | 读写键不对称：写归一化/读 raw `DomainPath` → "创作 / 文学"类变体永远读不回；归一化"其他"域被跨无关任务命中（配方串用） | `agent_allocator_match.go:67-68`、`task_planner_decompose_llm.go:366`、`task_orchestrator_impl.go:571` |
| C11 | MEDIUM | 单行 id=1 全量 JSON upsert 无版本/CAS → 多实例丢更新；每次完成 2 次全量序列化落库 | `internal/data/orchestration_cache_repo.go:43-48` |
| C12 | MEDIUM | 缓存命中 notice 是死代码（`SuggestTopology` 零调用方，且不补齐 Step 路由字段）→ UI 对经验复用零可见性 | `internal/service/spirit_team_assembler.go:220-257` |
| C13 | MEDIUM | L0 配方成员静默收缩仍沿用历史 DQ（只校验 lead）；同分取 map 随机序 | `internal/agent/agent_domain_match.go:57-72` |
| C14 | MEDIUM | 配方/graph 路由与 workspace 完全解耦：跨租户同文本任务直接命中，历史 agent_keys 可落他租户 | `spirit_orchestration_cache.go:17-29`、`orchestration_cache_repo.go:21-22,44-46`、`task_planner_impl.go:756-792` |

### 2.4 Graph 执行与状态事件

| # | 级别 | 发现 | 位置 |
|---|------|------|------|
| C15 | HIGH | `checkDownstream` 判定后先 Unlock 再 dispatch → 同一 PlanStep 双 dispatch，健康步骤被覆盖 Failed → 级联杀板 | `internal/service/plan_executor_dag_flow.go:65-90`、`plan_executor_dag_dispatch.go:50-57` |
| C16 | MEDIUM | interrupt 后被当 done 误触发 onComplete → 投影器提前停、trace 提前 Finish，resume 后 UI 冻结/遥测断链 | `internal/biz/graph_execution_usecase.go:730-732`、`internal/service/graph_orchestration_projector.go:69-82` |
| C17 | MEDIUM | GraphExecution 终态 sink 事件先于 DB 落库（违反 AS-EVT-01 WBPF）；standalone 执行卡 running 无启动终态化扫描 | `graph_execution_usecase.go:706-728,835-843,862-867` |
| C18 | LOW | `r.canceled` 无锁写/持锁读 → data race | `internal/service/plan_executor_dag_run.go:136`、`plan_executor_dag_dispatch.go:277-281` |

### 2.5 前端接入

| # | 级别 | 发现 | 位置 |
|---|------|------|------|
| C19 | HIGH | `run_status` 迟到帧无 runId/updatedAt 新鲜度守卫 → REST 对账覆盖 WS 终态，停止按钮错误重现且永不自愈；Enter 键误路由 enqueue | `web/src/features/chat/composables/useChatRunStatus.ts:47-62,124-139`、`useChatSender.ts:139` |
| C20 | HIGH | MemberSession pause/resume 补丁 Version 与存储全等 → 被 `upsertMemberSession` 守卫恒丢弃（死代码） | `web/src/features/chat/composables/useChatV2EventHandlers.ts:253`、`web/src/stores/chat/activityV2Store.ts:304` |
| C21 | MEDIUM | GraphNode 无版本守卫，水合 REST 旧快照可把终态节点回退 running，且无法自愈 | `activityV2Store.ts:356-371` |
| C22 | LOW | 未知事件 kind 静默丢弃，契约漂移零观测（有意前向兼容设计，缺告警） | `web/src/features/chat/composables/useChatEventRouter.ts:16,135-137` |

### 2.6 前端展示

| # | 级别 | 发现 | 位置 |
|---|------|------|------|
| C23 | HIGH | 成员状态双词表漂移：live 全为 v2 词表，映射表只认 legacy → failed/skipped/paused 成员全显示「排队中」，与同卡「部分成员失败」警告矛盾 | `web/src/features/spirit/spiritUi.ts:140-150`、`TeamProgressCard.vue:130,254`、`TeamMemberTreeNode.vue:36` |
| C24 | MEDIUM | AgentSidebarCard：cancelled/interrupted 并入 failed（红+重试按钮）；无 partial_failure 分支 → 成员永久"待开始" | `web/src/components/chat/AgentSidebarCard.vue:127,133` |
| C25 | LOW | ObserveNodeDetail 成员映射缺 paused：英文原文直出、无 CSS 角标 | `web/src/components/chat/observe/ObserveNodeDetail.vue:139-150,260-317` |

---

## 3. Plausible（8 条：机制属实、场景窄，纳入加固）

| # | 发现 | 定性 |
|---|------|------|
| P1 | poller/lease 触发 synthesis 缺 `RootTaskActivityID` ctx → 触发文本可能落成新用户气泡、父 Task 永久 Running | `plan_executor_dag_publish.go:20` 等两路径无注入 |
| P2 | 配额把 interrupted 计为活跃，无自动回收（仅崩溃/重启产生该态，可软删释放） | `internal/biz/team_types.go:36-38` |
| P3 | AutoStart 分支绕过 `BuildTeamTurnInput`（缺交付协议/上游 inbox），未接线死路径 | `internal/service/spirit_team_assembler.go:80-93` |
| P4 | streamGen 防护不对称：NodeStart/NodeEnd/GraphDone 无 gen 检查，与 `graph.go:220` 自述设计矛盾 | `graph_execution_usecase.go:761-810` |
| P5 | `HandleTeamTurnResult` stale 守卫缺 partial_failure；后续交互 turn 致重复记账 | `spirit_team_handle_result.go:43-54` |
| P6 | 超时判死 CAS 失败后 idle 看门狗永久摘除（有 4h maxLifetime 封顶兜底） | `internal/biz/spirit_orchestration_timeout.go:151-160` |
| P7 | 团队中断/取消后加载态文案无清除分支（主流路径有 run_status 兜底，仅边界残留） | `useContextualLoadingMessage.ts:46-47,204-239` |
| P8 | 团队 partial_failure 无侧栏脉冲反馈（PULSE 映射缺键） | `observabilityConstants.ts:239-251`、`useStatusPulse.ts:34-35` |

---

## 4. 已驳回的怀疑（~20 条，三轮验证证明正确，免修）

- **组队**：并发组队 TOCTOU（复用原子性兜底）、复用团队孤儿 TeamStage（可达路径派生 ID 必同，VersionLT 幂等）、跨部门借调静默丢失（双路径识别+生产装配保证 org 非 nil）。
- **graph 恢复**：`ResumeExecution` 静默全图重跑（框架 lineage 兜底链 + `ErrCheckpointNotFound` fail-closed）；PlanStep `partial_failure` 两视图映射矛盾（无生产者，死代码）。
- **状态**：`CheckTeamProgress` partial_failure 进度归零（全链无生产调用者，工具已软删）；团队进度"按历史成功率恒 0%"（实际按成员完成数）。
- **前端**：token 累计双倍（DeltaSeq/Version 幂等）、TeamRun paused/cancelled 无事件映射（有）、`TeamProgressCard` 进度恒 0%/ObserveNode NaN%（数据通道无生产者、当前不可达的死代码）、`TeamRunStatus` 类型缺 partial_failure（纯类型完备性，纳入 D9 顺手补齐）。

---

## 5. 结论

机制骨架正确（分档/组队/花名册/FSM/崩溃恢复/事件分级成立）。薄弱点：

1. **用户级断点（P0）**：C1 取消=failed+自动重跑、C2 双跑、C15 健康板被级联杀。
2. **UI 层谎言（P0）**：C23 失败成员显示「排队中」、C19 停止按钮复活、C21 节点终态回退。
3. **静默风险源（P0/P1）**：经验缓存 C8–C14——复用越"聪明"错得越远。
4. **死路径/债务（P2）**：C3/C12/C22/P1–P8 加固项。
