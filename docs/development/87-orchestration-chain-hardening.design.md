# 87-orchestration-chain-hardening 设计文档

> 需求文档：[87-orchestration-chain-hardening.md](./87-orchestration-chain-hardening.md) · 开发计划：[87-orchestration-chain-hardening.development.md](./87-orchestration-chain-hardening.development.md)
> 审查依据：[docs/reports/2026-10-20-review-spirit-orchestration-chain.md](../reports/2026-10-20-review-spirit-orchestration-chain.md)（25 confirmed / 8 plausible）
> 本文档仅含架构设计、契约、技术方案与测试设计，不含需求清单与进度。

## 1. 设计目标

在不改变既有架构边界（组织铁律、双 DAG 引擎归属 ADR-86、事件可靠性分级 AS-EVT-01、AS-FSM-01 显式状态机）的前提下，修复精灵编排链路的 33 项确认/疑似缺陷，使以下条件成立：

1. **取消语义可信**：用户取消在 run/session/team/plan-step/event 五个平面上语义一致（cancelled，不再伪装成 failed），且永不触发自动重跑。
2. **并发执行单跑**：同一团队、同一 PlanStep 在任何入口组合上至多一个活跃执行体。
3. **经验复用可信**：配方键自洽（归一化+工作区+契约指纹）、质量信号双向（成功升权/失败降权）、持久化不丢更新。
4. **状态显示无谎言**：前端每个状态枚举有独立正确的视觉与文案，迟到数据不能回退真实状态。

## 2. 全局不变量（所有集群共同遵守）

| 编号 | 不变量 | 落点集群 |
|------|--------|----------|
| INV-CANCEL | 取消判别三源统一：`registry 内存 cancelled 标记 → err 链 context.Canceled → ctx()/Cause`；任一为真即用户取消，任何中间层不得改写为 failed；取消路径的终态落账必须在 `context.WithoutCancel` 之后的上下文执行；`context.DeadlineExceeded` 属 infra 墙钟超时，**不**判用户取消 | D1 |
| INV-CLASSIFY | 失败分类器新增 terminal-cancel 类：用户取消的 PlanStep 语义为 terminal-cancel（不重试、不级联失败、下游按 skip 收敛），与 retryable transient 分离 | D1/D5 |
| INV-IDEMPOTENT | 每团队生命周期内 `recordTeamCompletion`/`scheduleDependentTeams` 至多生效一次；幂等键 = `teams` 表 CAS 状态转移（expectCurrent→newStatus 单次有效），不新建存储 | D1 |
| INV-MUTEX | `StartTeamTurn` 与 chat 入口共享同一互斥域（运行注册 + 会话锁 + Running 转换中止语义），双方向互可见 | D2 |
| INV-DAG | `checkDownstream` 的「就绪判定→预标记 dispatch」在锁内原子完成；`r.canceled` 写入持锁；DAG 扇出受配额闸门（槽位延后准入 + 终态/单飞重扫活性兜底） | D5 |
| INV-EVT | 终态事件先落库后外发（AS-EVT-01 Critical=WBPF）；新增 `team_run.cancelled` 事件 kind 走 Critical 通道（Sequencer→outbox→BlockUpTo→MarkPublished→CriticalJournal），取消与失败在持久化/回放/Monitor 三处可区分 | D4/D6 |
| INV-STREAM | `consumeRuntimeEvents` 的「interrupt→流关闭」不再等价于 done；observer 收口改由 FSM 终态判定；streamGen 代际守卫对称覆盖 GraphDone/NodeStart/NodeEnd | D6 |
| INV-CACHE-KEY | 配方键 = 归一化域路径 + workspace 隔离 + 契约指纹（PlaybookID/ConstraintFingerprint 写入并在全部消费点 fail-closed 校验；旧条目指纹空 → 一次性降级不淘汰）；「其他」兜底域禁止写入与命中 | D7 |
| INV-CACHE-LEARN | DQ 双向：成功升权、失败降权（含 synthesis 报错型失败）；多副本持久化分行+版本 CAS，杜绝整 blob 覆盖丢更新 | D7 |
| INV-FE-FRESH | run_status 各源仲裁：runId 绑定 + 终态粘性 + REST 更新时刻单调；错误 running 态有自愈对账入口；GraphNode 生命周期秩（pending→running→terminal）单向不退让、水合快照与 live 帧同秩仲裁 | D9 |
| INV-FE-VOCAB | 成员状态单一 v2 词表（pending/running/paused/completed/failed/skipped）全量映射；team 终态四值（completed/partial_failure/failed/cancelled）与 interrupted 在各展示组件有独立视觉与文案 | D10 |

## 3. 集群设计（10 集群 / 30 项修复）

## 3.1 集群 D1：取消语义与团队终态记账

> 集群 D1 三条外科手术修复，互不改 wire_gen.go。F1：抽取包级 isUserCancelledErr（turn_error_publish.go），三处团队回调点（team_turn_hooks.go:116、spirit_team.go:290、chat_orchestrator_turn_dispatch.go:419）按其分流——用户取消传 cancelled + errMsg 原文并经 context.WithoutCancel 落账，root Task 落 TaskStatusCancelled，跳过 failed 级联。F2：HandleTeamTurnResult 终态守卫集合补 partial_failure；recordTeamCompletion/scheduleDependentTeams 以 teams 表 CAS TransitionStatus 成功为幂等前置；cancelled 分支按当前状态条件补 pending/running→cancelled 转移（FSM 已有，容忍幂等冲突），standalone 分支同补。F3：step_failure_classifier 增 StepFailureCancel 类（最高优先级），failStepWith 对取消不重试、级联走 cascade skip 而非重跑，"deadline exceeded" 维持 transient。附完整状态/事件契约表与 TDD 红绿用例清单。

**不变量承诺**：org-invariants：不改编排所有权与建团入口——取消终态仍由 TeamStarter.HandleTeamTurnResult 单写者落账 + PlanExecutor 经 NotifyTeamCompletion 单通道感知，未新增引擎/未复活 completed Team/未动编制表与 dept_lead 治理边界。AS-FSM-01：cancelled 落地全部走既有显式状态机——teams 表 pending/running→cancelled（team_state_machine.go:84/89 已存在，无需补转移）、root Task Running→Cancelled（task_v2_state_machine.go:78）；幂等键=CAS+CanTransition，同态重放自然拒绝。AS-EVT-01：TeamStage/TeamRun 终态事件（Important 级）仍走 publishV2Event(seq 优先持久化)+终态守卫，cancelled 与 completed/failed 同通道不降级；PlanStepSkippedEvent 级联语义既有。DOC-SYNC：实施 PR 应同步 spirit DAG 编排相关模块三件套的「状态/契约」描述（cancelled 终态语义与幂等契约），本方案不改三件套归属判断。根目录规范：测试均在既有 internal/service、internal/biz 包内新增 _test.go，不建根目录、不改 cmd/admin/*。

### D1-F1 — 统一取消判别并让用户取消沿三处团队回调落地 cancelled

**修法**

抽取包级纯函数 isUserCancelledErr(ctx, err)（errors.Is(err,context.Canceled) || ctx.Err()/context.Cause(ctx) 为 context.Canceled；显式不把 context.DeadlineExceeded 判为用户取消——那是 TeamMaxLifetime 墙钟 infra 超时），放在 turn_error_publish.go 与 runWasCancelled 同文件；runWasCancelled 保留 registry(SessionRunPhaseCancelled) 优先检查后委托该函数，TeamStarter 侧直接复用同一判别。三处回调点按取消分流：①executeTeamTurnViaHooks err 分支 cancelled:=o.runWasCancelled(teamCtx,sessionID,err)，为真则根 Task 落 biz.TaskStatusCancelled（CompleteTaskTerminal FSM Running→Cancelled 合法且幂等）、跳过 SetRunStatus(failed)/transitionSessionStatus(interrupted,error)（cancelActiveRun 已落 cancelled/user_cancelled，对齐 runSingleAgentViaTRPC 的 !runWasCancelled 门）、HandleTeamTurnResult 改用 context.WithoutCancel(teamCtx)（保 RootTaskActivityID、防取消传染终态落账）、status 传 biz.TeamStatusCancelled、errMsg 传 err.Error() 原文（保留 context.Canceled 信号供分类器）；非取消维持现状。②pending 团队分支同款：cancelled→TaskStatusCancelled+transitionSessionStatus(interrupted,user_cancelled)+HandleTeamTurnResult(cancelled)，并跳过 publishTurnFailure 与 DLQ Enqueue（取消不是失败邮件）。③StartTeamTurn err 分支 cancelled:=isUserCancelledErr(turnCtx,err)，为真 sessions 转 interrupted("user_cancelled")、HandleTeamTurnResult(cancelled)。

**行为契约**

契约：取消判别三源统一且只升不降——registry 内存 cancelled 标记（chat 入口 cancelActiveRun）→ err 链 context.Canceled → ctx/Cause；任一为真即「用户取消」，任何中间层不得把 status 改写为 failed。errMsg 永远保留原始错误文本（含 "context canceled"/"user_cancel" 子串），禁止清洗成 "internal error"（下游分类器依赖该信号）。终态落账一律在 context.WithoutCancel 之后的上下文执行，保证取消不传染持久化；root Task、teams 表、TeamStage 各自的终态转移 FS M 幂等。不变量：用户取消的 chat 团队会话 → teams=cancelled（不再 failed）→ checkAllTeamsCompleted 的 CancelledTeams>0 → synthesis 跳过（修掉 spirit_team_schedule.go:173-180 不命中的失败叙事链路）。

**改动文件**

- `internal/service/turn_error_publish.go` — 新增包级函数 isUserCancelledErr(ctx context.Context, err error) bool；runWasCancelled 在 registry 检查后委托它（签名不变），注释注明 TeamStarter 侧共享该判别
- `internal/service/team_turn_hooks.go` — executeTeamTurnViaHooks err 分支：取消判别再分流——TaskStatusCancelled 终态化、跳过 failed run-status/session 写、HandleTeamTurnResult(WithoutCancel(teamCtx),…,biz.TeamStatusCancelled,err.Error(),"")
- `internal/service/chat_orchestrator_turn_dispatch.go` — pending 团队 err 分支同款取消分流；取消时跳过 publishTurnFailure 与 DLQ Enqueue
- `internal/service/spirit_team.go` — StartTeamTurn err 分支：isUserCancelledErr(turnCtx,err) 为真时 sessions 转 interrupted("user_cancelled")、HandleTeamTurnResult(biz.TeamStatusCancelled)

**测试设计（TDD：先红后绿）**

- `internal/service/turn_error_publish_test.go（新建）`：TestIsUserCancelledErr 表驱动：err=fmt.Errorf("x: %w",context.Canceled)→true；裸 ctx 取消（ctx.Err()==context.Canceled)→true；ctx.WithCancelCause 根因 context.Canceled→true；err=context.DeadlineExceeded→false（pin：墙钟超时≠用户取消）；err=errors.New("boom")→false。另加 TestRunWasCancelled_DelegatesCancelSignals 回归现有 registry 优先语义
- `internal/service/team_turn_hooks_cancel_test.go（新建）`：复用 newTeamTurnTestOrch/capturingTeamRunner/capturingTeamStarter。TestExecuteTeamTurnViaHooks_CancelledErrReportsCancelled：runner.runErr=context.Canceled→starter.status=="cancelled"、stubTaskV2Writer.terminalized 末次 Status==biz.TaskStatusCancelled（修复前红：status=="failed"）；TestExecuteTeamTurnViaHooks_GenericErrStillReportsFailed（对照 pin）：runErr=errors.New("llm boom")→starter.status=="failed"、TaskStatusFailed
- `internal/service/spirit_team_start_turn_cancel_test.go（新建）`：复用 spirit_team_handle_result_test.go 的 &TeamStarter{team:TeamOrchestrationDeps{…}} +stub 构造。TestStartTeamTurn_CancelledErrReportsCancelled：TeamsNative stub 返回 context.Canceled 且 ctx 已 cancel→stub TeamUC 收到 TransitionStatus(cancelled)（经 handle_result cancelled 分支）、controller.recordCompletionCalls==0（修复前红：收到 failed 转移）。TestStartTeamTurn_GenericErrStillFailed（对照 pin）

**风险**：①chat_orchestrator_turn.go 单 agent 路径 runWasCancelled 现仅检查 registry/ctx/err——抽取后行为不变，回归靠既有 chat_orch_run_status_test.go 钉住。②pending 路径无 registry entry（SetPendingCancel 非 StoreCancelable），只能依赖 ctx/err 信号——InterruptAndSendMessage 抢占取消也会归类 user_cancelled，语义可接受。③service 包当前被 knowledge WIP 卡编译，本 fix 测试待其恢复后运行。

---
### D1-F2 — HandleTeamTurnResult 终态守卫补 partial_failure + 同态重放幂等门

**修法**

同一函数改四处：(a) 终态守卫把 biz.TeamStatePartialFailure 加入现态终态集合，异态回调丢 stale_callback，同态放行；(b) completed/partial_failure 分支把 recordTeamCompletion+scheduleDependentTeams 门到 TransitionStatus 成功之后，CAS 拒绝只 Warn；(c) failed 分支把 scheduleDependentTeams 同门；(d) cancelled 分支破除「调用方已转换」假设——team.Status!=cancelled 时尝试 TransitionStatus(cancelled)（FSM 已有 pending/running→cancelled转移），对 BadRequest/Conflict 容忍 Warn（幂等重放/竞态取消）后继续既有成员 session interrupted(user_cancelled)+scheduleDependentTeams。spirit_team_standalone.go handleStandaloneTeamTurnResult 的 cancelled 分支同补条件 TransitionStatus（standalone 本就无 record/schedule）。幂等键不新建存储：teams 表 UpdateTeamWhereStatus 的 expectCurrent→newStatus CAS 转移天然单次有效，它就是幂等键。

**行为契约**

HandleTeamTurnResult(ctx,spirit,team,status,errMsg,chatSess) 改动后契约表（status∈{completed,partial_failure,failed,cancelled}）：
1. 终态守卫（前置）：现态∈{completed,failed,cancelled,partial_failure} 且 !=入参 → stale_callback 丢弃；同态放行进入幂等路径。守卫集合本次补 partial_failure。
2. completed/partial_failure：交付物门/验证门/F10 quorum/P1-4 salvage 收敛（既有）→TransitionStatus CAS（running→completed|partial_failure，必须成功）→成功才 recordTeamCompletion+scheduleDependentTeams→publishTerminalTeamStage→publishV2TeamRunCompletion→checkAllTeamsCompleted→NotifyTeamCompletion(success=true)。CAS 失败=同态重放/竞态取消→全部记账型副作用跳过。
3. failed：TransitionStatus CAS→成功才 scheduleDependentTeams→failed 事件→NotifyTeamCompletion(success=false,errMsg 原文)。用户取消信号永不进入本分支（F1 判别前置）。
4. cancelled：现态≠cancelled 时 TransitionStatus(pending/running→cancelled)（容忍 invalid/conflict）→成员 session interrupted("user_cancelled")→scheduleDependentTeams（dagRun 活跃时被 HasActiveRunForSession 门掉）→TeamStage cancelled→TeamRun/MemberSession outcome→checkAllTeamsCompleted（CancelledTeams>0→synthesis 跳过）→NotifyTeamCompletion(success=false,errMsg 原文含 context.Canceled)。
时序不变量：recordTeamCompletion/scheduleDependentTeams 每团队生命周期至多生效一次（幂等键=teams CAS 转移）；事件面由 TeamStage FSM 终态守卫+TeamRun 终态守卫+completion channel LoadAndDelete 各自幂等。

**改动文件**

- `internal/service/spirit_team_handle_result.go` — HandleTeamTurnResult：终态守卫集合补 biz.TeamStatePartialFailure；completed/partial_failure 分支将 recordTeamCompletion+scheduleDependentTeams 门到 TransitionStatus 成功；failed 分支将 scheduleDependentTeams 同门；cancelled 分支按当前状态条件补 TransitionStatus(cancelled)（容忍 invalid/conflict）
- `internal/service/spirit_team_standalone.go` — handleStandaloneTeamTurnResult cancelled 分支：现态≠cancelled 时同样尝试 TransitionStatus(cancelled)（容忍拒绝），保持无 record/schedule 语义

**测试设计（TDD：先红后绿）**

- `internal/service/spirit_team_handle_result_test.go（追加）`：TestHandleTeamTurnResult_CompletedReplay_SkipsRecordAndSchedule：team 现态 completed、hasRealDeliverable=true，回调 completed→recordCompletionCalls==0 且 scheduleDependentsCalls==0（修复前红=1）；TestHandleTeamTurnResult_FailedReplay_SkipsSchedule：现态 failed、回调 failed→scheduleDependentsCalls==0（修复前红=1）；TestHandleTeamTurnResult_PartialFailureGuard_DropsLateCompleted：现态 partial_failure、回调 completed→stale discard，transitions==0、record/schedule==0（修复前红：守卫集合缺 partial_failure 会走 completed 分支）；TestHandleTeamTurnResult_Cancelled_TransitionsFromRunning：现态 running、回调 cancelled→stubTeamWriter.transitions 含(id,cancelled,running)，成员 session interrupted(user_cancelled) 调用（修复前红：无 transition）；TestHandleTeamTurnResult_Cancelled_AlreadyCancelled_Idempotent：现态 cancelled、回调 cancelled→transitions==0 且无副作用重复（回归 pin）
- `internal/service/spirit_team_handle_result_part2_test.go（追加）`：TestHandleTeamTurnResult_Standalone_Cancelled_TransitionsIfRunning：standalone 团队现态 running、回调 cancelled→writer 收到 cancelled 转移（修复前红：分支不转换）；现态 cancelled→不转换不 panic（pin）

**风险**：TransitionStatus 失败不再触发 record/schedule 会改变一类边缘语义：团队真实完成但 CAS 竞态输给并发的 CancelTeam/Recover——此时副作用被跳过是正确行为（取消/重做生效优先）。既有测试 TestHandleTeamTurnResult_CompletedWithRealDeliverable_StaysCompleted 等会正常转绿，但 TestHandleTeamTurnResult_StaleCallbackAfterTerminalSkipped（现态 completed、入参 failed，异态守卫路径）不变。stubTeamWriter.UpdateTeamWhereStatus 恒 true 的旧测试若依赖「completed→completed 也记账」会被翻红，需在实施时逐条核对并改为断言新幂等语义。

---
### D1-F3 — 失败分类器新增 terminal-cancel 类，阻断用户取消的自动重跑

**修法**

step_failure_classifier.go 新增第四类 StepFailureCancel，匹配优先级置于 capability 之前（注释更新优先级链 cancel→capability→semantic→transient→默认 transient）；patterns（全小写 contains）："context canceled","context cancelled","user_cancel","user_cancelled","team_cancel","team_cancelled"。plan_executor_dag_dispatch.go failStepWith(:249-260) 对 class==StepFailureCancel 单列分支：Info 日志「step 失败分类为用户取消，不重试不重跑」后直接走末尾 cascadeSkip——不调用 maybeRetryStep（不消耗 attempts 预算、不触发 5s 退避重跑）。"deadline exceeded" 维持 transient（TeamMaxLifetime 4h 墙钟属 infra 超时而非用户取消，见 open_questions）。handleCompletion(:207) 不变——errMsg 原文来自 F1 的 cancelled 回调，已携带 cancel 信号。

**行为契约**

契约：用户取消信号沿 status=cancelled + errMsg 原文 → NotifyTeamCompletion(success=false,errMsg) → TeamCompleteEvent{Success:false,ErrorMsg} → dagRun.handleCompletion → failStepWith(team_failed,msg,true) → ClassifyStepFailure=StepFailureCancel → 不重试（attempts 不递增、无退避重跑）、下游依赖经 cascadeSkip 标 skipped(StepErrCodeCascadeSkip)（可被 ResumePlanBoard 复活）而非继续失败级联扩散；当前 step 仍标 Failed（取消语义由分类/错误文本承载，board 终态扫描归 partial/failed 既有逻辑）。时序不变量：取消的团队在 dagRun 存活期内至多在 step 失败路径上被记账一次；board 级取消（r.canceled）与该 step 级取消互不干扰、双写防护已有 wg 屏障+sweepNonTerminalSteps。

**改动文件**

- `internal/biz/step_failure_classifier.go` — 新增 StepFailureCancel 类 + 取消 patterns，匹配优先级置于 capability 之前；ClassifyStepFailure 注释同步
- `internal/service/plan_executor_dag_dispatch.go` — dagRun.failStepWith 对 StepFailureCancel 单列：日志后跳过 maybeRetryStep 直接 cascadeSkip

**测试设计（TDD：先红后绿）**

- `internal/biz/step_failure_classifier_test.go`：追加表驱动用例："context canceled"→cancel；"graph aborted: context canceled"→cancel；"context cancelled by user"→cancel；"user_cancel"→cancel；"team_cancel: stopped"→cancel；对照 pin："context deadline exceeded"→transient、"rate limit 429"→transient、"agent not found"→capability、空串→transient
- `internal/service/plan_executor_dag_cancel_test.go（新建）`：TestDagRunFailStepWith_CancelSkipsRetryAndCascadesSkip：以最小 dagRun harness（参考 plan_executor_part2_test.go 构造）驱动 r.failStepWith(ctx,step,"team_failed","context canceled",true)→断言 r.attempts[step.ID] 未递增、无二次 dispatch（orch stub 计数==0）、下游 pending step 变 skipped(StepErrCodeCascadeSkip)；对照用例 transient("connection reset")仍走 maybeRetryStep（pin 既有 F2）

**风险**：若上游调用方把非取消错误文本拼接进含 "context canceled" 子串的消息会误判为不重试——F1 已保证 cancelled 回调的 errMsg 是真实取消错误，误伤面可控；暂不把 "deadline exceeded" 归入 cancel（避免改变 F2 恢复语义，见 open_questions）。

---

**遗留问题**：
- DeadlineExceeded 的归类边界：TeamMaxLifetime(4h 墙钟 spirit_team.go:267) 与 biz RegisterTeamTimeout 是 infra 超时而非用户取消，本方案维持 transient（F2 恢复语义不变）。若产品意图「墙钟超时即终态不重试」，需单独 ADR 决定是否把 "deadline exceeded" 从 transient 迁移（牵涉 maybeRetryStep 全局策略）。
- reverse-sync 依赖调度（biz/spirit_orchestration_schedule.go:51）把 cancelled 上游视同 failed → 下游标 failed（cascade fail 语义残留）。生产主路径因 dagRun 活跃被 HasActiveRunForSession 门掉不受影响；standalone/spirit-自动团队 DAG 场景下是否也要「取消=级联 skip」超出本集群最小修复，建议另开 M-三件套变更评估。
- 历史脏数据：修复前用户取消落为 failed 的 teams 行不回填；如需对账，在 test/<cleanup-cancelled-teams>/ 下做一次性脚本按 team_runs 错误文本回填，不进本 PR。
- pending 队列 InterruptAndSendMessage 的抢占取消与用户主动 stop 目前都会归类 user_cancelled（无独立 cancel cause 通道）。语义上可接受，但若后续要在 UI 区分「插队打断」与「用户终止」，需 pendingQ 取消时携带 reason。

## 3.2 集群 D2 团队运行互斥与看门狗续链：

> 三条外科手术修复：(1) StartTeamTurn 经 chat_wire.go ProvideChatService 后置回填 SetRunGuard（RunRegistry+lockSession+DrainPending），与 chat 入口共享同一把 keyed lock+同一 RunRegistry，busy 时幂等 nil-skip、TransitionStatus 失败改为中止、defer Finish+cancel+唤醒 pending 队列——wire_gen.go 零改动；(2) failTeamOnTimeout 加 bgCtx+cfg 参数，TransitionStatus 失败分支重新 armTeamTimeout 顺延一窗（终态下次自清）；(3) 新增 biz.IsTeamSlotOccupied（pending+running）仅在 internal/tools/spirit_tools.go:214 并行配额处收窄，IsTeamStatusActive 与 reuse/synthesis/progress 消费点保持 interrupted-as-active 语义不动。TDD：service/biz/tools 三包各自新增失败先行用例。

**不变量承诺**：组织铁律：本集群仅触及团队/会话运行生命周期互斥与配额口径，不改编制/花名册/工具面/领导职责——org-invariants §1-§4 不触发；『生产建团唯一路径 PlanExecutor+RealTeamOrchestrator（65 §1.21/ADR-86）』保持，dispatch 入口仍是 RealTeamOrchestrator+TeamStarter.scheduleDependentTeams 两条既有路，仅补上互斥；TeamStarterPort 接口（biz/spirit_team_usecase.go:11-13）零变更，65 §1.21『修改接口需同步 service/team.go』无牵连。AS-FSM-01：Session/Team FSM 转换表零新增/零修改——修复只是让既有 FSM 失败从被吞（Warn-continue）升级为诚实中止，failed 落地失败从弃犬升级为顺延重试。AS-EVT-01：无新事件，事件可靠性分类不变；team_stage 仍 seq-first 持久化（spirit_team.go:209-247 不动）。AS-STA-01：新注入走内部 service 包私有 TeamRunGuard+setter 后置模式（同 SetTurnGateway/SetPlanExecutor 先例，65 §1.30『Wire 环规避』同款），biz port 表面零扩展。红线 #16：全部新日志用 loggateway.StepID/Str/Err 结构化字段。AS-COG-01：TeamStarter 字段数 15→16 已知超限（既有 TECH-DEBT(COG) 注释域），不新增拆分动作以免越出最小手术范围。

### D2-2#2-HIGH — StartTeamTurn 接入运行注册+会话锁+中止语义，消除调度/chat 双跑

**修法**

在 TeamStarter 引入可选 TeamRunGuard{Runs *rt.RunRegistry; Lock func(string) func(); DrainPending func(string)} 与 SetRunGuard setter，由 ProvideChatService（chat_wire.go，SetTurnGateway 回填块后，第41-43行同款 interface-assertion 模式）回填：Runs=cs.orch.runs（wire_gen.go:262 的同一 runRegistry 实例）、Lock=cs.orch.lockSession（=o.chatUC.LockSession 同一 keyed locker）、DrainPending 为闭包（内部 Re-Get session 后调 cs.orch.processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "", "")，镜像 team_turn_hooks.go:82-92 defer 行为）。改写 StartTeamTurn（spirit_team.go:154-306）前序流程：(1) TeamsNative nil 检查不变；(2) guard!=nil 时 unlock:=Lock(sessionID)，锁内 Runs.HasActive(sessionID)→解锁+log.Info+return nil（契约与现 :162-167 skip 完全相同，调用方三个入口——team_orchestrator_real.go:168 / spirit_team_schedule.go:121+142 / spirit_team_assembler.go:85+114——零改动）；(3) legacy sess.Status==Running 裸读保留在锁内（多副本/崩溃恢复窗口防御——DB 真值兜底），registry 空+DB running 的仲裁策略：维持现有 skip 语义但锁内判定消除 TOCTOU；(4) sess 前移到锁前，Get 失败照旧 404；(5) sessions.TransitionStatus(→Running) 失败：apierror 类型保留、改为 return err 中止（不再 Warn-continue）；同目标幂等 no-op（usecase.go:707-711）不受影响——chat 入口已置 running 时此为 nil，靠 (2) 的 registry 判定承担互斥；(6) turnCtx 由 WithTimeout(maxLifetime) 派生（原 :267-271），runID:=uuid.NewString()，Runs.StoreCancelable(sessionID, runID, cancel)——与 chat 路径 chat_orchestrator_turn.go:230 同序，unlock() 释放，再进 4h 长 RunTurnFromInput；(7) defer Runs.Finish(sessionID, runID)+cancel()+(guard.DrainPending!=nil 时调用)。guard==nil（v1-only/测试直构）走原路径但 TransitionStatus 中止仍生效。双向冲击：dispatch 对 chat——chat 入口 admission（:67）见 hasActive=true 走 admitGate/enqueue，Finish 后 DrainPending 唤醒队列杜绝 4h 积压；chat 对 dispatch——dispatch 锁内 HasActive 命中 chat 已在跑→skip nil，pending channel 由既有 run 的 HandleTeamTurnResult 终态路由喂饱（team_turn_hooks.go:116/137 同源调用）；用户 WS cancel 首次可杀 dispatch run（cancel func 已注册），RunTurnFromInput ctx.Canceled→既有 interrupted+HandleTeamTurnResult(failed) 路径诚实落 DAG。不改动 TeamStarterPort 接口（biz/spirit_team_usecase.go:11），不加构造参数，65 §1.21『修改接口需同步实现』无牵连。

**行为契约**

INV-M1（单飞）：同一 team session 任意时刻至多一个在飞 run——chat 与 dispatch 双入口共用同一 lockSession keyed lock + 同一 RunRegistry，注册顺序均为 lock→HasActive→StoreCancelable→unlock，与 chat_orchestrator_turn.go:121-231 镜像，无新并发原语引入死锁（锁持有期间仅 DB 读 + 状态转换，毫秒级）。INV-M2（可寻址）：每个 dispatch run 有 (runID,cancel) 注册项且恰好 Finish 一次（defer），会话永不残留 BUSY 窒息准入层（对照 2026-08-20 活体实证同类缺陷）。INV-M3（诚实中止）：session →Running 转换失败则启动失败、错误回传，RunTurnFromInput 不执行、HandleTeamTurnResult 不触发；不存在『团队真跑但 session 非 running』的谎言态。INV-M4（幂等 skip）：busy 时返回 nil（非错误），调度方 pending channel 不被误败（RealTeamOrchestrator.NotifyTeamCompletion 不被触发失败事件）。INV-M5（队列不淤积）：dispatch run 结束的 defer 在非 pending-loop 场景唤醒 DrinkPending，排队消息在 4h 后被消费而非永久挂起。INV-M6（状态机不变）：Session FSM/Team FSM 转换表零改动，仅收束错误路径；interrupted→running 重启轮不受影响（FSM 合法）。

**改动文件**

- `internal/service/spirit_team.go` — TeamStarter 增 runGuard *TeamRunGuard 字段+SetRunGuard setter；StartTeamTurn 前序重排为 lock/registry-busy-check/legacy-running-check/TransitionStatus-abort/StoreCancelable/unlock/defer-Finish-cancel-drain；RunTurnFromInput 调用点不变；synthesisTriggered.Reset(:185-187)、TeamStage 事件(:209-247)、RegisterTeamTimeout(:249-251) 全部不动
- `internal/service/chat_wire.go` — ProvideChatService 在 SetTurnGateway 回填块（:41-43）后新增 ~10 行：SetRunGuard 回填（Runs=cs.orch.runs、Lock=cs.orch.lockSession、DrainPending 闭包转调新增 orch 辅助方法）。函数签名不变
- `internal/service/chat_orchestrator.go` — 新增 ~12 行 drainPendingAfterDispatchTeamTurn(sessionID string)：Get session（o.td().Sessions.Get with appctx/background ctx）→ inPendingLoop 守卫 → processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "", "")，注释标注镜像 team_turn_hooks.go defer

**测试设计（TDD：先红后绿）**

- `F:/myproject/aranea-agents/internal/service/spirit_team_start_mutex_test.go`：TestStartTeamTurn_BusyRegistrySkips——预存 runs.StoreCancelable(sid, otherID, noopCancel)，调用返回 nil，stub TeamsNative.RunTurnFromInput 零调用，unlock 计数==1，registry 未被覆盖；TestStartTeamTurn_TransitionRunningErrorAborts——session repo 作者写失败：返回 err 非 nil、registry 无注册、RunTurnFromInput 零调用、HandleTeamTurnResult 零调用、unlock==1；TestStartTeamTurn_RegistersCancelableAndFinishes——stub RunTurnFromInput 阻塞在 channel：阻塞中断言 HasActive==true，放行完成后断言 HasActive==false 且 cancel 已调用；TestStartTeamTurn_ConcurrentDispatchSerialize——两个 goroutine 各调 StartTeamTurn（stub 内 sleep 50ms），RunTurnFromInput 调用次数==1，另一个 nil-skip；TestStartTeamTurn_NilGuard_LegacyPath——未 SetRunGuard 时行为同修复前（无锁无注册也正常完成）；sesssion repo 复用 spirit_team_handle_result_test.go:1247 的 f10SessionRepo+NewSessionUsecase 模式
- `F:/myproject/aranea-agents/internal/service/team_turn_hooks_test.go`：新增用例（该文件已存在 file_exists）：同 RunRegistry 上一先一后 chat 入口（模拟 executeTeamTurnViaHooks 的 StoreCancelable 顺序）与 dispatch 交错时双方只见一个 HasActive 为 false——跨入口互可见的集成断言

**风险**：1) 修复后 dispatch 运行期间用户对 team session 发消息会从『双跑』变为『入队等待』——前端 UX 变化在可接受范围（此前双跑本身就是事故）；2) DrainPending 闭包内 sess Get 失败时跳过唤醒，退化到遗留行为（消息挂到下一个 chat-path turn）——记 log，不改阻塞语义；3) legacy sess.Status==Running 锁内保留在多副本部署中仍是最后一道防，但在 crash-resume marker 已恢复活跃 run 的场景（76-run：team_orphan_recovery.go skip 由 83-长时运行韧性）如果 session 层恢复滞后一拍，可能让一次重启后的必要 start 被 skip 一次——与修复前行为一致，非回归；4) TeamStarter 字段从 15→16，AS-COG-01 上限 15 已在既有 TECH-DEBT 注释域，不新增单独拆分；5) internal/service 包测试随外部 WIP 恢复后才能跑——实现前可先在 biz 层验证 guard 逻辑可注入性。

---
### D2-P6 — 看门狗 TransitionStatus CAS 失败 re-arm，杜绝永久弃犬

**修法**

改 spirit_orchestration_timeout.go 中 failTeamOnTimeout（:151-165）签名为 failTeamOnTimeout(bgCtx, timeoutCtx context.Context, cfg ParallelConfig, team Team)：TransitionStatus 失败分支从 Warn+return（弃犬）改为 Warn + o.armTeamTimeout(bgCtx, cfg, team.ID, cfg.TeamTimeout()) + return——与 onTeamTimeout 中 Get 失败顺延（:63）/probe 失败顺延（:123）同款『顺延一个窗口』降级语义。4 个调用点（:92 max-lifetime 强终、:102 无 run 判死、:114 probe-strikes 判死、:145 idle 判死）全部传 cfg+bgCtx——它们都已在 onTeamTimeout 内有这两个参数在作用域，纯转发。TransitionStatus 成功分支行为不变（HandleTeamTimeout 路由）。CAS 语义说明：timer 已被 claim LoadAndDelete（:37）——重新 arm 是写入新条目，cancel-on-terminal（CancelTimeoutTimer）若早一拍会同被删的旧记录空操作，safe。

**行为契约**

INV-W1（不丢犬）：团队非 pending 且非终态时，任何一次超时回调链上不允许出现「claim 删除 timer+TransitionStatus 失败」组合导致该 team 在 timeouts registry 中永续缺席——失败必 re-arm 一个 window。INV-W2（终态自清）：re-arm 的 timer 下一次触发时走 onTeamTimeout :66-76 终态短路（completed/failed/cancelled/partial_failure/pending 直接 return 且不再续 arm），无空转累积，最坏代价一次额外唤醒。INV-W3（fail-closed 不降权）：判死三类场景（max-lifetime/无-run-宽限/probe-strikes）的语义不变，仅当判死的落地动作（TransitionStatus→failed）本身失败时才顺延重试——延续既有 Get 失败顺延的失败观。INV-W4（CAS 一致性）：claim→业务处理→re-arm 的产物序列与既有 armTeamTimeout 相同（timeouts.store），不新增并发面：cancel 与新 arm 的顺序歧义由『成功路径调 HandleTeamTimeout 后 RecordTeamCompletion 的 CancelTimeoutTimer 总生效』保证。

**改动文件**

- `internal/biz/spirit_orchestration_timeout.go` — failTeamOnTimeout 签名扩展（bgCtx+cfg）、失败分支 armTeamTimeout 顺延、4 个调用点同步传参；注释说明失败顺延语义对齐 :63/:123

**测试设计（TDD：先红后绿）**

- `F:/myproject/aranea-agents/internal/biz/spirit_orchestration_timeout_test.go`：TestFailTeamOnTimeout_TransitionError_Rearms——fake teamUC TransitionStatus 返回 error：调用后 timeouts.claim(teamID)==true（可 again claim，即新 timer 已存入）；TestFailTeamOnTimeout_Success_NoRearm——成功路径后 timeouts.claim==false+timeoutHandler 被调（既有断言保持）；TestOnTeamTimeout_IdleFailButTransitionError_ReArmedTimerFiresAgain——端到端：第一次窗口判死+落地失败→第二个窗口再次触发并（第二次 TransitionStatus 成功）最终转 failed——验证 recoverability；复用该文件既有 timeouts:&teamTimeoutRegistry{} struct 直构模式（:129）

**风险**：1) FSMS 拒绝（如并发路径先转 completed）导致失败时也会 re-arm 一次，下一次走终态短路自清——接受一次唤醒浪费换取终态统一自清，不引入 branch 复杂性；2）若 DB 长期故障将转为每 window 一次的 warn-retry 心跳噪音——与既有 probe 失败顺延同等级，正确性优于永久弃犬；3) 4 个调用点全在 onTeamTimeout 内，bgCtx 均为 registerTeamTimeout 的 WithoutCancel ctx——无 goroutine 泄漏面变化。

---
### D2-P2 — 并行配额改占座口径，interrupted 不再永久卡死 plan_and_execute

**修法**

全局影响面枚举（已逐点核对）：(a) internal/tools/spirit_tools.go:214-222 plan_and_execute 并行配额 len(activeTeams)>=maxParallel——窄化处：interrupted 为非执行占用，不应计座；(b) internal/tools/spirit_reuse.go:90 cohortAllCompleted——保持含 interrupted（同 cohort 未了结，follow-up 挂旧队而非开新 DAG）；(c) internal/tools/spirit_reuse.go:167 reuseNextAction active 计数——保持；(d) internal/biz/spirit_synthesis.go:254-261 synthesis 前置门 ListActiveTeams——保持含 interrupted（未完成编排不得合成，RecoverAllInterrupted 链路依赖）；(e) internal/biz/spirit_orchestration_progress.go:168 进度文案——保持。方案：IsTeamStatusActive（team_types.go:36-38）全局不动；新增 biz.IsTeamSlotOccupied(status string) bool = pending||running（同文件内，紧邻 Active 谓词，doc 注明『并行配额占座口径——interrupted 可 resume 但零执行不占座』）；tools/spirit_tools.go 配额块改为按占座计数：遍历 activeTeams 用 IsTeamSlotOccupied 累加 occupied 后与 maxParallel 比较；错误提示文案数显仍报 len(activeTeams) 还是 occupied 由实现选 occupied（用户可见的会计口径即真实阻挡者）。

**行为契约**

INV-Q1（占座口径）：parallel quota 只计『确在执行或即将执行』的 seat（pending+running）；interrupted 团队（仅产自 team_orphan_recovery.go:38 崩溃恢复）可经 ResumeTeamIfInterrupted（team_usecase.go:641-655）回到 running 后重新占座——无自动回收属有意设计（sweep 守门 comment team_graph_sweep.go:50 防止 resume 边缘场景），但不再静默卡死 plan_and_execute。INV-Q2（隔离）：cohort/reuse/synthesis/progress 四处语义保持 interrupted-as-active——用户续接旧编排走 resume 路径（resume hook），不被新配额放行撬出新 DAG。INV-Q3（失败不静默）：配额读 ListActiveTeams 失败仍沿用现有 warn+跳过检查降级（tools/spirit_tools.go:216-219），不新引入 fail-closed 路径。

**改动文件**

- `internal/biz/team_types.go` — 新增 IsTeamSlotOccupied(status string) bool（pending||running），doc 注明仅 parallel-quota 占座语义，interrupted 排除
- `internal/tools/spirit_tools.go` — quota 检查块（:211-224）由 len(activeTeams)>=maxParallel 改为 IsTeamSlotOccupied 过滤后的 occupied>=maxParallel

**测试设计（TDD：先红后绿）**

- `F:/myproject/aranea-agents/internal/biz/team_types_test.go`：TestIsTeamSlotOccupied——pending=true/running=true/interrupted=false/completed=false/failed=false/cancelled=false/partial_failure=false/archived=false/empty=false；既有 TestIsTeamStatusActive（:160-170）三 true 断言零改动保持回归
- `F:/myproject/aranea-agents/internal/tools/spirit_tools_quota_test.go`：maxParallel=1+1 interrupted 团队→quota 放行（进入 plan 阶段，不再 BadRequest）；maxParallel=1+1 running 团队→仍拒绝 BadRequest；maxParallel=1+1 pending 团队→仍拒绝

**风险**：1) 配额放行后用户发新任务且旧 interrupted 团队被 resume 链路同时激活，可能出现短暂超配——interrupted resume 是用户驱动（恢复）/recovery 驱动，频率低，quota 是最优努力护栏非硬上限；2) 提示文案从 active 数改报 occupied 数，前端无结构依赖（纯字符串）；3) tools/spirit_reuse_test.go stub 的 teamQuery 已是窄接口（spirit_tools_part2.go:278），签名零变化。

---

**遗留问题**：
- 用户 WS cancel 一个 dispatch 启动的团队 run 时，现有错误路径把 session 转 interrupted+reason=Error（team_turn_hooks.go:106 同款），是否应区分 StatusReasonUserCancelled 以提高前端语义精度？倾向本次不动（保持与团队 chat 入口一致的现有口径），另开议题
- legacy sess.Status==Running 裸读在多副本/多实例部署中是否仍须保留为兜底——若未来真水平扩展，RunRegistry 是进程内态，需把该检查升级为 DB 侧 CAS 租约；本次保留但建议记入 ADR 88 系列议题
- DrainPending 触发时 processPendingQueue 的 RootTaskActivityID 参数置空是否会让 pending 消息挂到新一轮幽灵 Task（对照 chat_orchestrator_turn.go:285-294 续跑不变量）——实现落地时走与 team_turn_hooks.go:89 相同空传参实证，如有偏差在实现期补 RootTaskActivityIDFromCtx
- FIX-D2-01 落地后 dispatch 期间用户对 team session 发消息从双跑变为入队——需要前端在上限文案/排队提示上配套吗（web/src/features/chat/api.ts 正被并行会话修改，不在本集群范围）

## 3.3 集群 D3：synthesis 生命周期与触发 ctx：

> 指控逐条核实：①发现#3成立——poller（biz/spirit_orchestration.go:169-213）只扫 Status=Running，synthesis turn 失败后会话停 interrupted（chat_orchestrator_turn_phases.go:877 等），spirit_team_schedule.go:255-263 释放的 CAS 成死路径；②P1 成立——plan_executor_dag_publish.go:20 用 context.Background()、poller 用 pollCtx，两路均无 RootTaskActivityID，spirit_team_notices.go:183-190 的 ctx 兜底为死代码，DB 未落库时 parentTaskID 空→触发文本落成新用户 Task 气泡（projector.go:326-340）且父 Task 永 Running（projector.go:1161-1173）；synthesisAlreadyProduced 读失败 fail-open（214-217）。方案：FIX-D3-1 biz poller 追加 interrupted 复活扫描（白名单 4 种 reason，CAS/守卫不下沉）；FIX-D3-2 lease 释放处以 board.TaskID 经 ctx 注入 RootTaskActivityID（不改 biz 接口）；FIX-D3-3 守卫改三态 defer、ctx rtID 权威优先、空 parent 禁触发、重试预算 3 次与兜底通知首发唯一。全程不碰 cmd/admin/* 与 knowledge/*，wire_impact 全 none。

**不变量承诺**：org-invariants：不触碰编制表/花名册/dept_lead/company_lead/编排分档与唯一建团路径（PlanExecutor+RealTeamOrchestrator 不动）；synthesis 复活只是重新武装既有 system-push 触发，无新增 LLM 调用面与工具面变更；interrupted 会话复活 dispatch 沿用既有 chat 入口与配额门禁。AS-FSM-01：不新增状态——会话 Interrupted→Running 复用 session/status_machine.go 既有合法边；PlanBoard/GraphStage 终态机不变；CAS+budget 文档化为「每轮至多一在飞、至多 N 次尝试、终态闭锁至下轮 StartTeamTurn」的显式守卫（不属于实体状态机，不引入隐藏状态：三态 guard 判定全部源自持久化+ctx 的纯函数）。AS-EVT-01：synthesis turn 走既有 sequencer 持久化链（Critical）；deliverables/fallback notice 走 publishV2Event（seq 优先持久化，Important），契约限定每轮各至多一次，兜底 notice 首发唯一避免重试刷屏。AS-ADR-01：本方案跨 biz（轮询扫描）/service（守卫与预算）/executor（ctx 透传）三模块且翻转 fail-open 语义，按规范落一份 ADR（docs/reports/2026-XX-review-adr-*，背景=发现#3+P1、决策=三态 defer+revival 白名单+ctx 权威、替代方案=整体 fail-closed/biz 反查端口）后再合入。

### D3-FIX-1 — poller 复活扫描：扫 interrupted 且可复活的精灵会话

**修法**

在 `internal/biz/spirit_orchestration.go` 扩展 `pollTeamCompletions`（现 169-213 行只扫 Status=Running 的死路径根源）：① 复用 `resolveSpiritAgentID` 与 SpiritPollerMaxPages/PageSize 分页，追加一段 `Status: SessionStatusInterrupted` 的扫描；② 纯函数 `isRevivableInterruptedReason(reason string)` 放行 error/timeout/server_shutdown/unexpected_shutdown 四种（user_cancelled/user_escalated/manual_override/clarification 系人为意图一律排除，CancelledTeams 门作为第二道防线）；③ 命中候选仍走 `CheckAllTeamsCompleted`→AllDone→`o.completionNotifier.NotifyAllTeamsCompleted(ctx, sess.ID)`。去重/CAS/budget/持久化守卫全部留在 service 侧（FIX-D3-3），poller 保持无状态：running 与 interrupted 两扫描集按 status 互斥、同一 tick 不会双触发；跨 tick 重复 Notify 由 TeamStarter CAS 吸收。重启幂等链路：startup `RecoverOrphanedRunningSessions` 把 Running 会话翻成 interrupted(unexpected_shutdown)→revival 扫描拾取；若 synthesis 已产出，持久化守卫 skip（FIX-D3-3 三态）。

**行为契约**

① revival 扫描只读不改：不重写 session 状态，synthesis turn 派发时 dispatch 链自动 transitionSessionStatus(interrupted→Running)（复用 status_machine.go 既有合法边，AS-FSM-01 不新增状态）。② execute 失败回落 interrupted(error) 后，下个 tick 再次命中 revival——重试循环上限由 FIX-D3-3 的 budget 兜底。③ 用户取消/人工介入的 interrupted 永不复活。④ 正在跑新一轮编排的会话 teams 未全终态 → AllDone=false 跳过；待其终态后走正常 CAS 路径。

**改动文件**

- `F:\myproject\aranea-agents\internal\biz\spirit_orchestration.go` — `pollTeamCompletions` 尾部追加 interrupted-revival 扫描段（Status=interrupted + AgentID + RootOnly 复用同一分页常量）；新增小函数 `isRevivableInterruptedReason`（白名单 4 reason）。候选提取抽成一个闭包复用 CheckAllTeamsCompleted→Notify 流程。不改 SessionSearchQuery/数据层仓库。

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\internal\biz\spirit_orchestration_poller_revive_test.go（新建）`：a) interrupted(error)+全终态 → notifier 恰好被调 1 次且 id 正确；b) interrupted(user_cancelled)+全终态 → 0 次；c) interrupted(unexpected_shutdown)+全终态 → 1 次（重启复活路径）；d) interrupted(error) 但 teams 含 running → 0 次；e) 既有 running 扫描回归：running 会话照常（沿用 part2_test 的 memSpiritSessionRepo，其 SearchSessions 已实现 Status 过滤）；f) context_overflow reason → 0 次（明确排除策略）。先写测试（当前即失败——revival 扫描不存在），再实现。

**风险**：revival 本意是针对 synthesis 失败的会话，但任何 error-interrupted 的精灵会话只要 teams 全终态都会进 Notify——双保险是 service 侧持久化守卫（已产出即 skip）+ budget 上限；若某会话非编排 turn 失败（如普通闲聊 turn）但历史编队恰好全终态，可能重发一轮迟来的 synthesis——由三态守卫的 produced 判定挡住。预算值 3 与轮询 30s 相乘的最坏重试窗口 90s。冲突规避：本 fix 只碰 biz/spirit_orchestration.go，与并行会话 knowledge 文件零交集。

---
### D3-FIX-2 — lease 释放触发路径注入 RootTaskActivityID

**修法**

精确方案选「沿 ctx 携带 NotifyAllTeamsCompleted 参数」，不改 biz 接口：改 `internal/service/plan_executor_dag_publish.go` `releaseLeaseAndNotifyCompletion`（15-21 行），把 `context.Background()` 换为 `notifyCtx := agent.ContextWithRootTaskActivityID(context.Background(), agent.RootTaskActivityID(r.board.TaskID))`（board.TaskID 非空时；为空保持 Background）。依据：根 Task ID 的唯一持久化真相是 `plan_boards_v2.task_id`（task_planner_publish_v2.go:22 由 RootTaskActivityIDFromCtx 写入），且 startBoardDAG（plan_executor_lifecycle.go:181-184）注入 run ctx 的正是同一值——从 board 读而非从 run ctx 透传，使重启 recover 重建的 dagRun 也携带正确值。不选「biz 侧按 board 反查」的理由：需给 biz.PlanBoardV2Reader 加 ListPlanBoardsBySession 端口方法+data 实现+wire_gen.go 注入，违反最小外科与 wire_gen 手工并合约束；poller 路径的 DB 时序间隙由 FIX-D3-3 的 defer 语义吸收（30s tick 到任务行必已落库）。

**行为契约**

① lease-release 触发路径的 checkAllTeamsCompleted ctx 必含 RootTaskActivityID==board.TaskID，2026-07-04 问题5 的 ctx fallback 在该路径复活。② v2 任务行异步落库滞后时 `resolveLatestUserTaskID`（FIX-D3-3 改为 ctx 优先）返回 rtID → `TurnInput.ParentTaskID` 非空 → projector.OnTurnStart 走 continuation 分支（projector.go:321-325）不产生新用户气泡；OnTurnEnd 关闭父 Task（projector.go:1161-1173）消除「父 Task 永久 Running」。③ `publishDeliverablesNotice` 的 parentTaskID 空 Warn 死路径消除 → deliverables notice 恢复发布。④ ExecuteTurn 透传同一 rtID 与入口 resolveRootTaskActivityID(input) 派生值相等，无二次污染。

**改动文件**

- `F:\myproject\aranea-agents\internal\service\plan_executor_dag_publish.go` — `releaseLeaseAndNotifyCompletion`：notifyCtx 由 board.TaskID 注入 RootTaskActivityID；注释补「2026-07-04 问题5 ctx fallback 死路径修复」。函数签名不变、biz.AllTeamsCompletedNotifier 接口不变。

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\internal\service\plan_executor_synthesis_gate_test.go（追加用例）`：a) TestDagRunTerminal 全程后置断言：notifier 收到的 ctx 的 RootTaskActivityIDFromCtx == "task-notify"（复用 plan_executor_synthesis_gate_test.go 的 Subscribe+completeStep 驱动）；b) board.TaskID 为空 → notifyCtx 无 rtID 且 notifier 仍被调一次（不退化、不 panic）；c) 重启 resume 场景：直接 newDagRun+run 终态后断言同一不变量。

**风险**：board.TaskID 为空的存量数据/壳失败板 → 行为与现状一致（ctx 无值），由 FIX-D3-3 的 attach-integrity defer 兜底；不影响 v1-only（planExec 为 nil 时该路径不存在）。注意 service 测试包当前被外部 WIP 卡住编译——本 fix 的测试随 WIP 恢复后通过。

---
### D3-FIX-3 — 三态持久化守卫 + 复活重试预算 + 挂靠完整性

**修法**

① `synthesisAlreadyProduced`（spirit_team_notices.go:210-223）改三态：`guardState ∈ {produced, clear, uncertain}`——taskV2Reader==nil→clear（v1 legacy 保持现状）；查询 err→uncertain；len(tasks)==0 且 ctx 无 rtID→uncertain；len==0 且有 rtID→clear（DB 滞后由 rtID 兜底）；rtID 非空且 !=latest.ID→clear；否则按 latest.Status==Completed 判 produced。fail-closed 影响面结论：不可整体翻转——读抖动直接判 produced 会在 CAS 已占位的时序下永久窃取本轮总结（已有 pinned 测试 TestCheckAllTeamsCompleted_TaskReadError_FailsOpen 证明原设计意图）；defer（释 CAS 本 tick 不触发、下 tick 重判）同时满足「不双发」与「不丢发」，代价是 DB 故障窗口内最长延迟到恢复。② `resolveLatestUserTaskID`（162-196）改 ctx rtID 优先、DB 回落：ctx 携带的是本轮权威根 Task（与 resolveRootTaskActivityID 同源），消除 DB 滞后行错挂旧 Task。③ `checkAllTeamsCompleted`（spirit_team_schedule.go:198-265）：CAS 之后插 guard 判定——produced→保持 CAS 返回；uncertain→`s.synthesisTriggered.Delete` 后返回（不发通告、不烧 budget）；clear→resolve parent，parentTaskID==""且 reader!=nil→释 CAS 返回（attach-integrity：v2 部署禁止空 ParentTaskID 触发，堵 ghost 气泡+父 Task 永 Running）。④ ExecuteTurn 同步失败：新字段 `synthesisAttempts sync.Map`（spirit_team.go TeamStarter struct 加字段，零值可用，wire_gen.go 不动）计数；attempts<reviveMaxAttempts(=3)→释 CAS；>=3→保持 CAS 终态闭锁。fallback notice 仅 attempts==1 时发（同轮多次失败不刷屏，既有断言「恰好 1 条 notice」天然兼容）。⑤ CAS 与 attempts 的锁存/重置同点：`StartTeamTurn`（spirit_team.go:185-187）在删 CAS 处同步删 attempts——新一轮编排解锁重试预算。时序契约（先抢 CAS 者得、谁发 deliverables notice）：事件路径（HandleTeamTurnResult，ctx 带 rtID）与 lease-release/poller/revival 四路共用「dagRun 门→CAS LoadOrStore」单入口——LoadOrStore 原子单胜；败者静默返回。deliverables notice 仅由 winner 在受理（queued）或阻塞完成时发布，每轮至多一条；queued 即受理即发，不等报告排空（既有语义保留）。重启幂等：进程重启 CAS/attempts 全失，由 produced 持久化判定接管；崩溃发生在 synthesis turn 中途→startup recovery 终态化会话与在飞 Task→produced=false→revival 补发（正确重新生成报告），不存在双发（旧 turn 已死）。

**行为契约**

不变量：I1 每轮合成 turn 至多一个在飞（CAS）；I2 dagRun 门在 CAS 前，波次中点不烧名额；I3 持久化真相不确定时宁可 defer 不触发、不双发、不永丢；I4 v2 部署触发必有 ParentTaskID（ctx 权威优先）；I5 每轮 UX 终态信号恰好一条（fallback notice 首发或 deliverables notice）；I6 重启后可恢复重试且已产出轮次不重发。失败重试预算 3 次/进程/轮，耗尽后 CAS 闭锁定局，下一新轮 StartTeamTurn 解锁。

**改动文件**

- `F:\myproject\aranea-agents\internal\service\spirit_team_notices.go` — synthesisAlreadyProduced→三态 guard；resolveLatestUserTaskID 改 ctx 优先；增补 doc 注释（defer 语义+rtID 权威）
- `F:\myproject\aranea-agents\internal\service\spirit_team_schedule.go` — checkAllTeamsCompleted 按 I1-I6 重排（gate→CAS→guard→attach→ExecuteTurn→budget/notice），不改函数签名
- `F:\myproject\aranea-agents\internal\service\spirit_team.go` — TeamStarter struct 增 `synthesisAttempts sync.Map` 字段；StartTeamTurn 重置 CAS 处同步 Delete attempts（struct 零值可用，newSynthesisStarter 等测试直构不受影响）
- `F:\myproject\aranea-agents\internal\service\spirit_team_handle_result_test.go` — 翻转 TestCheckAllTeamsCompleted_TaskReadError_FailsOpen 断言（DB err→ExecuteTurn 0 次+CAS 已释可下 tick 重测）；补 notice 首发唯一性断言（需随 internal/service WIP 恢复后跑通）

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\internal\service\spirit_team_synthesis_guard_test.go（新建）`：a) stubSynthesisTaskReader{err}→ExecuteTurn 0 次、CAS 已释（二次调用 reader 恢复→1 次）；b) DB 空+ctx 无 rtID→defer；DB 空+ctx 有 rtID→触发且 TurnInput.ParentTaskID==rtID；c) ExecuteTurn 连错 3 次→第 3 次后 CAS 保持，第 4 次调用不再 ExecuteTurn，兜底 notice 全程恰好 1 条；d) StartTeamTurn 后 attempts 清零可再触发；e) stale 列表+ctx rtID→ParentTaskID==rtID（ctx 优先于 DB……打开既有 StaleTaskList 用例保持绿）。先实现测试（WIP 恢复前仅编译不过，恢复后先红后绿），再改实现。

**风险**：fail-open→defer 翻转既有 pinned 测试，DB 抖动窗口首回合 summary 延迟 ≤30s——需在 ADR 记录语义变更；budget=3 耗尽后该轮总结终态静默仅留兜底 notice，属可接受 fail-safe。attempts 为内存态，重启重置——单进程崩溃循环有 budget 兜底，跨进程重启属「复活」语义内可承受。不与并行会话争文件：不碰 cmd/admin/* 与 knowledge/*。

---

**遗留问题**：
- 复活重试预算建议 3 次（≈90s 最坏 UX 静默窗）——是否接受；以及兜底通知改为「仅首次失败发」是否与 19:29 诚实信号规范完全兼容（现语义为每次失败都发）？
- context_overflow/clarification 系 interrupted 明确排除复活（重试大概率再失败且部分语义属用户待答）——是否需要例外放进白名单或暴露成配置？
- TestCheckAllTeamsCompleted_TaskReadError_FailsOpen 断言需随内部 service WIP 恢复后同步翻转（fail-open→defer）——确认这是 D3 簇授权的行为变更，不与并行会话持有的测试文件冲突（该文件为 spirit_team_handle_result_test.go。若并行也在改它，需协调）
- deliverables notice 修复后将在 lease-release 路径恢复发布（此前因空 parentTaskID 静默跳过）——前端 NoticeBlock 对迟到 deliverables 的展示是否有预期外表现需要走查？

## 3.4 集群 D4：TeamRun 取消事件契约

> D4 集群：TeamRun 取消事件契约全链路修复。核心一处发布点（spirit_team_completion.go publishV2TeamRunCompletion 末段开关）把 cancelled 终态从 FailedEvent 工厂切到新建的 EventKindTeamRunCancelled（team_run.cancelled），与 D1 的取消终态同一发布点、同一实体。biz 层新增 kind/struct/工厂/EntityID（含 deliverySeqCarrier）；IsTerminalEventKind 按 .cancelled 后缀自动进 Critical 通道 → Sequencer WBPF + event_delivery_outbox 行 + CriticalJournal 记录零代码改动自动生效（以测试钉死）；event_router.describePersist 补 case 保证落库与死信回放；WS fail-closed 出口 ws_v2_wire_convert.go 补映射；前端 v2Types/useChatEventRouter/teamRunEventFromV2Event/eventView/tracesQuery 按契约落地（type=team_run_cancelled、severity 不再误报 warn 级 failed、Runs 列表防抖刷新覆盖），展示打磨明确交 D9/D10。全部改动无 wire_gen.go 注入需求（wire_impact 均 none），不触碰并行会话文件。

**不变量承诺**：org-invariants：本集群只动 TeamRun 终态事件平面（biz 事件定义/Sequencer 路由/WS 映射/前端契约），不触碰编制表、花名册绑定、dept_lead/company_lead 治理身份、ToolsProfile、PlanExecutor 编排路径，与 M67/M78 无不变量碰撞。AS-EVT-01：team_run.cancelled 为终态事件，借用 IsTerminalEventKind 的 .cancelled 后缀判定自动进入 Critical 通道（WBPF BlockUpTo + event_delivery_outbox 行 + WS critical lane），与既有 team_run.completed/failed 同分级同链路，零策略改动；D4-1/D4-3 的测试把该分级钉为回归契约，防止未来 IsTerminalEventKind 重构静默降级。AS-FSM-01：TeamRunV2StateMachine 的 Running/Paused--cancel-->cancelled 转移与终态吸收不变量（trSM.Transition 拒绝→skip publish）原样保留，事件 kind 与实体终态一一对应，事件平面不缺 cancel 终态表达；向后兼容：历史 outbox/journal 中 kind=team_run.failed+Status=cancelled 的帧按原映射回放仍正确（实体 Status 为准），event_id 空间天然隔离新旧帧，无需迁移。

### D4-1 — biz 事件平面新增 EventKindTeamRunCancelled（kind + struct + 工厂 + EntityID）

**修法**

在 internal/biz/event.go 的 TeamRun 事件区块（EventKindTeamRunFailed 之后，现 131-134 行）新增 `EventKindTeamRunCancelled EventKind = "team_run.cancelled"`，并新增 `TeamRunCancelledEvent` struct：字段与 TeamRunFailedEvent 完全同构（taskID/spiritSessionID/TeamRun/occurredAt + 内嵌 `deliverySeqCarrier`——carrier 是 outbox 投递序必备，缺它 insertCriticalOutbox 会 Warn "critical event lacks delivery-seq carrier, outbox skipped"，sequencer.go:656-661）。在 internal/biz/event_factory.go 新增 `NewTeamRunCancelledEvent(tr TeamRun)` 工厂（复刻 NewTeamRunFailedEvent，229-236 行）+ `func (e *TeamRunCancelledEvent) EntityID() string { return e.TeamRun.ID }`（紧跟 363-364 行同族方法）。Critical 分类零代码改动：`biz.IsTerminalEventKind` 按 `.cancelled` 后缀已判真（event.go:30-37）→ `IsCriticalDeliveryEvent` 自动覆盖，WBPF（bus_v2.go publishCritical）+ outbox 行（sequencer.go insertCriticalOutbox）+ CriticalJournal Append（critical_journal.go:58-72，按 kind 字符串记录，天然区分 team_run.cancelled 与 team_run.failed）全部自动生效；本项用测试钉死该契约而非改代码。

**行为契约**

不变量：(1) `TeamRunCancelledEvent.EventKind()=="team_run.cancelled"` 且 `IsTerminalEventKind`/`IsCriticalDeliveryEvent` 对其为真；(2) kind 字符串唯一映射 cancelled 语义，不再与 failed 共用 team_run.failed（修掉 critical_journal 回放不可区分取消/失败的事实）；(3) 事件携带的 TeamRun.Status 恒为 cancelled，事件平面与实体平面（team_runs_v2.Status）一一对应。

**改动文件**

- `internal/biz/event.go` — EventKind 常量块新增 EventKindTeamRunCancelled；新增 TeamRunCancelledEvent struct（含 deliverySeqCarrier 内嵌）及 EventKind/SpiritSessionID/TaskID/OccurredAt/SetOccurredAt 方法
- `internal/biz/event_factory.go` — 新增 NewTeamRunCancelledEvent 工厂 + TeamRunCancelledEvent.EntityID()

**测试设计（TDD：先红后绿）**

- `internal/biz/event_teamrun_test.go（新建，包级可跑）`：TestNewTeamRunCancelledEvent_Contract：kind=="team_run.cancelled"、SpiritSessionID/TaskID 取自 TeamRun 字段、occurredAt 非零；TestIsTerminalEventKind_TeamRunCancelled 为真；TestIsCriticalDeliveryEvent_TeamRunCancelled 为真；TestTeamRunCancelledEvent_DeliverySeqCarrier：SetDeliverySeqValue(42) 后 DeliverySeqValue()==42（断言实现 biz.DeliverySeqEvent）

**风险**：纯增量不改既有 kind，无回归面；唯一风险是遗漏 deliverySeqCarrier→outbox 跳过（由测试用例钉死）。

---
### D4-2 — 发布点对齐 D1：cancelled 终态在 publishV2TeamRunCompletion 同一发布点改用专用 kind

**修法**

改 internal/service/spirit_team_completion.go `publishV2TeamRunCompletion` 末段发布开关（202-215 行）：新增 `case biz.TeamRunV2StatusCancelled: s.seq.Publish(ctx, biz.NewTeamRunCancelledEvent(tr))`，删除 default 分支里 cancelled 走 NewTeamRunFailedEvent 的注释与调用，default 改为防御性 return（不可达：上方 trEvent 开关已对未知态 return）。这是 TeamRun 取消终态的唯一发布点（DAG 路径 HandleTeamTurnResult:300 与 standalone 取消 spirit_team_standalone.go:214 均汇聚于此），与 D1 的 cancelled 终态判断（primaryStatus→teamRunStatus→trEvent 转换 + trSM.Transition 守卫，108-185 行）同一函数同一实体 tr——D1 不动则本改动即闭环，D1 若调整取消判定也只改上游状态映射，不重开第二个取消发布源。

**行为契约**

状态机不变量：Running/Paused --TeamRunV2EventCancel--> cancelled（team_run_v2_state_machine.go:55/58）不变；迟到回调时 trSM.Transition 拒绝→整段 skip publish（现有竞态守卫，558-601 行测试）不变。事件不变量：实体 Status=cancelled 当且仅当发布 team_run.cancelled 事件；同一 TeamRun 终态事件恰好一条。成员事件行为不变（cancelled→MemberSession skipped，不受本项影响）。

**改动文件**

- `internal/service/spirit_team_completion.go` — 发布开关：cancelled 专用 case 用 NewTeamRunCancelledEvent；移除 default 的 FailedEvent 兜底注释/调用，default 改 return

**测试设计（TDD：先红后绿）**

- `internal/service/spirit_team_handle_result_test.go（随 WIP 恢复后可跑）`：新增 TestPublishV2TeamRunCompletion_Cancelled_PublishesCancelledKind：cancelled 入参下 seq 捕获恰好 1 条 TeamRun 终态事件且类型为 *biz.TeamRunCancelledEvent、EventKind()==team_run.cancelled、内嵌 TeamRun.Status==cancelled、不再出现 *biz.TeamRunFailedEvent；Memberskip 行为沿用既有 TestPublishV2TeamRunCompletion_F10_Cancelled_StaysSkipped 断言不回归；既有竞态用例 TestPublishV2TeamRunCompletion_SkipsWhenCurrentTerminal 保持通过（不发布任何事件）

**风险**：internal/service 测试包当前受外部 WIP 卡编译——本项测试随 WIP 恢复后执行；实现本身为纯增量 case 分支，不触碰既有 completed/failed 路径，编译面仅依赖 D4-1 的工厂。

---
### D4-3 — 持久化路由 + 投递链：Sequencer Critical/outbox、死信回放、CriticalJournal 全链贯通（含测试钉）

**修法**

三处接线/钉测试，只一处改代码：(1) internal/agent/v2/event_router.go `describePersist` 在 TeamRunFailedEvent case（119-120 行）后新增 `case *biz.TeamRunCancelledEvent: return &persistDescriptor{EntityKindTeamRun, PersistOpUpsert, ev.TeamRun}`——不加此 case 事件会被 describePersist 判 nil 静默不落库；死信路径按 entityKind+op 路由，天然兼容。(2) insertCriticalOutbox/WBPF/journal 无需改代码（见 D4-1），在 sequencer 层钉一条回归：cancelled 事件走 publishCritical→outbox.Insert 且 Kind==team_run.cancelled。(3) CriticalJournal 在 internal/event/critical_journal_test.go 钉 kind 记录。

**行为契约**

持久化不变量：team_run.cancelled 与 completed/failed 同走 UpsertTeamRun，实体真实状态以 TeamRun.Status=cancelled 为准（RepoSet.persistTeamRun 与事件类型无关，现注释语义保持）；投递不变量：作为 Critical 事件恰好产生一条 outbox 行（UNIQUE(session_id,seq)），event_id 形态 v2:{session}:{seq}:team_run.cancelled:{entity}，重放与实时帧字节一致；journal 行 Kind 字段精确为 team_run.cancelled。

**改动文件**

- `internal/agent/v2/event_router.go` — describePersist 新增 TeamRunCancelledEvent case（EntityKindTeamRun/upsert）

**测试设计（TDD：先红后绿）**

- `internal/agent/v2/deadletter_replay_test.go（追加用例，包级可跑）`：新增表项：describePersist(NewTeamRunCancelledEvent(tr)) 非 nil、entityKind==team_run、op==upsert；并对 decodePersistEntity(EntityKindTeamRun,...) 回放等价性（与 team_run 既有用例同模式）
- `internal/agent/v2/sequencer_test.go（追加，仿 2026-10-20 task.cancelled 既有 B-06 用例，包级可跑）`：fakeOutbox 场景发布 NewTeamRunCancelledEvent→恰好 1 条 outbox 行、Kind==team_run.cancelled、Seq 单调、MarkPublished 一次；复用 task-cancelled 用例同构骨架
- `internal/event/critical_journal_test.go（追加）`：Append(NewTeamRunCancelledEvent(...)) 后 ReplayCritical 返回 entry.Kind==team_run.cancelled 且 NoticeType 为空；与 team_run.failed 条目可区分

**风险**：internal/event 未在可跑白名单内，但其与 WIP 文件无依赖，预计可跑；若被卡与 internal/service 同批恢复（低风险）。

---
### D4-4 — WS wire 映射补齐（fail-closed 出口）

**修法**

internal/server/ws_v2_wire_convert.go `v2EventPayloadToWire` 在 TeamRunFailedEvent case（354-355 行）后新增 `case *biz.TeamRunCancelledEvent: return teamRunEventWire{TeamRun: teamRunToWire(ev.TeamRun)}, nil`。该函数 fail-closed（无映射 kind 直接报错→ws_v2_subscriber.go:98-103 drop 并 Warn），不加 case 新 kind 在 WS 出口静默丢弃。outbox 重放无需动（marshalV2EventEnvelope 存的是带 kind 字符串的信封字节，replayOutbox 原样转发）。

**行为契约**

WS 契约：envelope{type:v2_event, kind:team_run.cancelled, payload:{TeamRun:{...,Status:cancelled}}}，payload 键与 team_run.completed/failed 完全一致（PascalCase TeamRun）；老版本前端按 default 忽略未知 kind（useChatEventRouter default 分支），不产生崩溃——旧帧（kind=team_run.failed+Status=cancelled）仍按旧映射渲染，向后兼容。

**改动文件**

- `internal/server/ws_v2_wire_convert.go` — v2EventPayloadToWire 新增 TeamRunCancelledEvent 映射到 teamRunEventWire
- `internal/server/ws_v2_wire_test.go` — wireGoldenCases 表新增 team_run.cancelled 金样用例

**测试设计（TDD：先红后绿）**

- `internal/server/ws_v2_wire_test.go（wireGoldenCases 追加一行，随 internal/server 包可跑）`：wireGoldenCases 表新增 {team_run.cancelled, NewTeamRunCancelledEvent(wireTestTeamRun()), []string{TeamRun}}；断言信封 kind 字符串与 payload 键集合

**风险**：无；fail-closed 兜底保证遗漏时仅 Warn+丢帧，不污染线上协议。

---
### D4-5 — 前端事件契约：kind 识别、路由、Monitor 视图/刷新映射（展示细节交 D9/D10）

**修法**

前端只落地事件契约触点，展示打磨交 D9/D10：(1) web/src/features/chat/v2Types.ts EventKind 联合类型在 'team_run.failed' 后增 `'team_run.cancelled'`。(2) web/src/features/chat/composables/useChatEventRouter.ts handleKind 在 team_run.* case 组加 `case 'team_run.cancelled':` 与既有三者共用 `store.upsertTeamRun`（状态平面以实体 Status 为准，无需客户端二值分支）。(3) web/src/features/teams/teamRunEventFromV2Event.ts switch 新增 `case 'team_run.cancelled'` 返回 `type: 'team_run_cancelled'`（run/payload 形态与 failed 分支一致；TeamRunEvent.type 是自由 string，见 teams/types.ts:212）。(4) web/src/features/monitor/eventView.ts：`wsEventToView` 新增 `case 'team_run_cancelled'`（title 用 i18n key `monitorPage.events.title.teamRunCancelled`，subtitle 取 run.error_message/空）；`severityForWsEvent` 确保 cancelled 不落 warn（当前 87 行只匹配 failed/error，新 type 自然为 info——用测试钉死，D9/D10 可另行调色/定级）；244 行 failed 判定仅看 error_message，cancelled 运行 Error 为空不受影响。(5) web/src/features/monitor/tracesQuery.ts `RUN_LIVE_REFRESH_EVENT_TYPES` 增 'team_run_cancelled'（契约性 wiring：保证 Runs 列表在取消事件到达时防抖刷新落定，否则取消行滞留 running——此非展示职责，属本集群）。(6) web/src/realtime/systemNoticeAdapter.ts 的短名清单属 v1 notice 适配层，本 contract 不动，留 D9/D10 视需要扩展。

**行为契约**

前端契约：(1) 收到 kind=team_run.cancelled → store TeamRun 实体置 Status=cancelled（实时帧与 outbox 重放同帧幂等）；(2) TeamRunEvent.type='team_run_cancelled' 且 run.status='cancelled'；(3) Monitor 事件流 severity != warn 且 title 不再渲染为失败；(4) Runs 列表在取消后一次防抖刷新内落定终态；(5) 未知/旧 kind 消费方不受影响（旧 team_run.failed 帧仍走旧分支）。

**改动文件**

- `web/src/features/chat/v2Types.ts` — EventKind 联合加 team_run.cancelled
- `web/src/features/chat/composables/useChatEventRouter.ts` — handleKind team_run case 组并入 team_run.cancelled
- `web/src/features/teams/teamRunEventFromV2Event.ts` — 新增 team_run.cancelled case → type team_run_cancelled
- `web/src/features/monitor/eventView.ts` — wsEventToView 新增 team_run_cancelled 标题分支；severityForWsEvent 保持 cancelled 非 warn（测试钉）
- `web/src/features/monitor/tracesQuery.ts` — RUN_LIVE_REFRESH_EVENT_TYPES 增 team_run_cancelled

**测试设计（TDD：先红后绿）**

- `web/src/features/chat/composables/__tests__/useChatEventRouter.spec.ts`：dispatch({kind:team_run.cancelled, payload:{TeamRun:{ID,Status:cancelled,...}}}) → store.upsertTeamRun 后该 run 状态为 cancelled；同 ID 重复投递幂等
- `web/src/features/teams/__tests__/teamRunEventFromV2Event.spec.ts（新建）`：team_run.cancelled 信封 → type==team_run_cancelled、run.status==cancelled；同步补 started/completed/failed 回归基线
- `web/src/features/monitor/__tests__/eventView.spec.ts`：severityForWsEvent(team_run_cancelled) 非 warn（==info）；wsEventToView 对 team_run_cancelled 产出 cancelled 标题且不取 error 摘要；isRunLifecycleEventType(team_run_cancelled)==true

**风险**：i18n key monitorPage.events.title.teamRunCancelled 的文案归属 D9/D10——若本集群先行落地可先用占位文案，由 D9/D10 终稿（见 open_questions）。

---

**遗留问题**：
- Monitor i18n 文案 key（monitorPage.events.title.teamRunCancelled）与 cancelled 的最终 severity 定级（info vs 新增 cancelled 样式）归属 D9/D10——本集群 contract 仅保证 cancelled 不再渲染为 warn 级 failed；是否由本集群先放占位文案待 D9/D10 确认
- system.notice 平面（team_summary/session_status_changed）是否需为团队取消补 notice 形态供 systemNoticeAdapter 消费，留给 D9/D10 展示需要评估
- internal/service 测试包被外部 WIP 卡住编译：D4-2 的 service 层测试在 WIP 恢复前无法执行，是否接受以 biz/agent/server 三层包级测试先行合入、service 用例随恢复补跑验证
- D1 集群产出 cancelled 终态的最终落点若与 publishV2TeamRunCompletion 的现有 primaryStatus 映射（spirit_team_completion.go:108-110）有偏差，需 D1/D4 共同确认同一发布点语义不重开第二真相源

## 3.5 集群 D5：DAG 调度并发安全与配额（plan_executor dagRun）：

> 三条指控全部核实成立。D5-1（#15 HIGH）：checkDownstream（dag_flow.go:82-88）在 r.mu 内判就绪、Unlock 后才 dispatch，dispatchStep（dispatch.go:50-57）才做 Pending→Running 转移——双完成竞态下第二个 dispatch 转移失败即 failStep→cascadeSkip→板 Failed。修法：把『就绪判定+Pending→Running 转移』收进同一临界区（admitReadySteps 锁内预标记），dispatchStep 改为断言式（非 Running 只记日志返回，绝不 fail），不新增 PlanStep 状态；maybeRetryStep 在其既有锁内补 Failed→Running 转移；全局重扫替代按 dependent 点扫。D5-2（#17 LOW）：run.go:136 无锁写 canceled vs dispatch.go:278 持锁读——写方加 r.mu（屏障后读点是 run goroutine 程序序，天然有序）。D5-3（#7 MED）：在 admitReadySteps 扇出处加 per-spirit-session 配额闸门（GetMaxParallelTeams/ListActiveTeams 同口径，延后准入不失败不阻塞），内部 Running 计数锁内 O(n) 派生、外部活跃校正 ownTeams，零槽时由步骤终态重扫+单飞 2s 重扫兜底活性；依赖经 chat_wire.go SetQuotaPort 后注入（deps.Team.SpiritAssembler 天然满足窄端口），wire_gen.go 零改动。

**不变量承诺**：组织铁律：方案不动生产建团唯一路径（PlanExecutor+RealTeamOrchestrator，ADR-2/org-invariants §4），不引入第二调度引擎、不改编排分档/花名册/工具装配，配额数据源复用精灵 parallel_config（GetMaxParallelTeams 现有口径），未新增 per-岗位/部门维度，org-invariants 全部不动。ADR-86：checkDownstream→admitReadySteps 是 PlanExecutor 内部临界区收编，前向调度所有权不变，TeamStarter reverse-sync 抑制前提不变（同步 65 手册锚点措辞）。AS-FSM-01：显式拒绝新增 Dispatching 中间态——planStepTransitions 表面零改动，仅把 Pending→Running / Failed→Running 两条既有合法边的调用点收进 r.mu 临界区并在 dispatchStep 改为断言，转换表本身仍是唯一真源；crash-recover/sweep/resume 全部沿用既有 Pending 重排队语义。AS-EVT-01：零新增事件、零删除事件；PlanStepStarted/Completed/Failed/Skipped 的发布点与分级不变；并发修复消灭的是『错误产生』的 Failed 事件而非压制正常失败路径。AS-COG-01：dagRun 仅新增 quotaRearmActive/quotaRearmDelay 两字段（runningCount 锁内派生避免计数器记账面），无 sync.Map 新增。修复后 DAG 并发不变量（设计验收清单）：(1) 单派发：一步一 dagRun 至多一次 Pending→Running，且必在 r.mu 内；(2) 无误杀：状态机断言失败仅记录日志，永不触发 failStep/cascadeSkip；(3) 锁序：dagRun 全部共享态（步态/runningCount 视图/attempts/canceled/quotaRearmActive）读写持 r.mu，锁内零 DB/事件 I/O；(4) 配额：runningCount+external≤max（max>0），瞬态偏保守；(5) 活性：就绪 pending 步骤在有限次终态重扫或单飞重扫后必被准入或被 cancel-sweep 终结；(6) 取消序：canceled 持锁写 → wg 屏障 → 终态判定，无在飞写者（C-17 延续）。

### 发现#15 — check-and-dispatch 原子化：锁内预标记 Pending→Running，消灭双 dispatch 误杀链

**修法**

核心重构：把『判定就绪 → 转换 Running』收进同一 r.mu 临界区（in-lock 预标记），不新增 PlanStep 状态（保 AS-FSM-01 转换表面不动；拒绝 Ready→Dispatching 新态方案：持久化中间态会带来 crash-recover/sweep/resume 矩阵扩散，互斥由 r.mu 提供即可）。

具体改动按函数落位：
1) internal/service/plan_executor_dag_flow.go：删除 dispatchReadyPending(:23-39) 与 checkDownstream(:65-90)，合并为新函数 `admitReadySteps(ctx)`——单次扫描 r.board.Steps（按下标序，保根先于下游确定性），每步在同一个 r.mu Hold 内完成：status==Pending 检查 → dependenciesCompleted 检查 → 配额空余检查（见 #7）→ `s.Transition(Running)`（Pending→Running 唯一调度入口）→ wg.Add(1) → 收集出参；临界区只碰内存，锁外逐个 r.dispatch(ctx, s)。dependencySatisfied/dependenciesCompleted 原样保留。
2) internal/service/plan_executor_dag_dispatch.go：
   - dispatchStep 第 1 步(:48-64) 由『Transition(Running)』改为『预标记断言』：r.mu 内校验 step.Status==Running（不是自己预标记的条目一律 log 后 return，严禁再调 failStep——这是发现#15 的误杀源头），随后照常填 StartedAt/Version++/Mode 默认值。holdPlaybookConfirm(:38-47) 保持在前（确认等待期间 step 处于内存 Running；deny 走 Running→Skipped 合法边；cancel 由 sweep Running→Skipped 合法边兜底）。
   - maybeRetryStep(:276-311) 在其既有 r.mu 临界区（canceled 检查 + attempts 递增处）追加 `step.Transition(Failed→Running)`，重试路径的 Running 预标记同样发生在锁内；转移失败则打日志且不 wg.Add（防御，不再触发 fail 链）。
   - handleCompletion(:182-208) 成功分支 `r.checkDownstream(ctx, step.ID)` 改为 `r.admitReadySteps(ctx)`。
   - failStepWith(:227-261) 在 cascadeSkip 之后补 `r.admitReadySteps(ctx)`（释放的槽位可准入其他就绪 pending；cascade 子树内部已被全量 skipped，不会产生子树内准入）。
3) internal/service/plan_executor_dag_run.go:123：`r.dispatchReadyPending(ctx)` 改为 `r.admitReadySteps(ctx)`。
4) internal/service/plan_executor_playbook_confirm.go skipPlaybookConfirmDenied(:261) cascadeSkip 后补 `r.admitReadySteps(ctx)`。

效果：任一 dagRun 实例内，一个 PlanStep 的 Pending→Running 至多发生一次且必发生在调度临界区；重复就绪判定（并发完成、admission 重扫）被锁内状态检查自然吞掉；dispatchStep 不再因状态机错误 fail 任何步骤。

**行为契约**

状态机不变量：planStepTransitions 表不动（无新状态/新边）；Pending→Running 只发生在 admitReadySteps 临界区，Failed→Running 只发生在 maybeRetryStep 临界区，两处均在 r.mu 下；dispatchStep 是断言方不是转移方。事件契约不变：PlanStepStarted 仍在 Upsert 成功后由 dispatchStep 发布（AS-EVT-01 分级不动）；单步骤在一次 dagRun 内至多一条 Started。时序不变量：并发完成两个最后依赖时，下游步骤恰好 dispatch 一次，零 PlanStepFailed 误报，零 cascadeSkip 误级联；终端判定仍在 wg 屏障之后（C-17 不动）。恢复语义不变：崩溃后 DB 中步骤仍为 Pending/Failed，resume/recover 走原有 Pending 重排队，预标记纯内存。

**改动文件**

- `internal/service/plan_executor_dag_flow.go` — 删除 dispatchReadyPending(:23-39) 与 checkDownstream(:65-90)；新增 admitReadySteps(ctx)：锁内 Pending 检查+依赖判定+配额闸门+Transition(Running)+wg.Add，锁外 dispatch；新增 maybeRearmQuotaSweep（见 #7 的单飞重扫）
- `internal/service/plan_executor_dag_dispatch.go` — dispatchStep 第 1 段(:48-64) Transition→Running 断言（非 Running 打日志 return，不调 failStep）；maybeRetryStep(:277-291) 锁内追加 Failed→Running Transition；handleCompletion(:202) 改调 admitReadySteps；failStepWith(:260) cascade 后补 admitReadySteps
- `internal/service/plan_executor_dag_run.go` — run() 调用点(:123) 改为 admitReadySteps
- `internal/service/plan_executor_playbook_confirm.go` — skipPlaybookConfirmDenied(:261) cascadeSkip 后补 admitReadySteps
- `docs/development/65-module-cross-reference-full.md` — §781 行内『与 checkDownstream 双重激活』措辞同步为 admitReadySteps（DOC-SYNC-6 锚点真实性）

**测试设计（TDD：先红后绿）**

- `internal/service/plan_executor_dag_admission_test.go`：TestDagRun_ConcurrentLastDeps_DispatchOnce（菱形 A→{B,C}→D：completeStep B/C 并发，waitForCallCount(D,1) 成功后短停再断言 calls 计数仍==1、无 PlanStepFailedEvent、run 返回 nil、board Completed）；TestDagRun_ConcurrentAdmitReadySteps_NoDoubleDispatch（8 goroutine 并发直接调 admitReadySteps，每步骤 orch 调用数==1，-race 跑）；TestDagRun_DispatchStepRejectsNonPremarked（直接对 Pending 步骤调 dispatchStep：状态保持 Pending、零 orch 调用、零 failed/skipped 事件）；TestDagRun_RetryPreMarksFailedToRunning（注入 transient 失败、stepRetryBackoff=0：同一 step dispatch 恰好 2 次、第二次可 complete、cascade 未触发、下游照常准入）。复用现有 fakeOrchestrator/fakeReposForExecutor/fakeSeq（已 mutex-guarded，plan_executor_test.go:16/119）

**风险**：dispatchStep 改为断言式后，任何绕过 admitReadySteps/maybeRetryStep 直接调 dispatchStep 的未来调用方会静默 no-op——用返回值/日志留痕，并在断言分支打 Error 级日志便于排查；并行会话正在改 cmd/admin 三件套，本方案不触碰；internal/service 包当前被外部 WIP 卡住编译，测试先写好待 WIP 恢复后跑。

---
### 发现#17 — r.canceled 无锁写改持锁写，补齐与 maybeRetryStep 的 happens-before

**修法**

最小修法（不引入 atomic.Bool，不做无关替换）：internal/service/plan_executor_dag_run.go:136 改为
```
r.mu.Lock()
r.canceled = true
r.mu.Unlock()
```
并补一行注释说明锁契约。maybeRetryStep 的持锁读被写方同锁覆盖，race 消除；publish 路径读与写同 goroutine，程序序天然提供 happens-before，无需改动。

**行为契约**

不变量：r.canceled 的全部写发生在 r.mu 临界区内，且在 wg 屏障 `<-done` 之前（保持 run() 既定顺序）；全部读要么持锁（maybeRetryStep）、要么在屏障后的 run goroutine 程序序内（publishPlanBoardTerminal/publishGraphStageTerminal）。终端分类语义不变：canceled → PlanBoardEventFail + Interrupted GraphStage + termCtx 5s 预算 + sweepNonTerminalSteps，顺序与现行一致（run.go:131-167 不动除该行加锁）。

**改动文件**

- `internal/service/plan_executor_dag_run.go` — :136 写 canceled 加 r.mu 保护 + 锁契约注释（读点分布注明）

**测试设计（TDD：先红后绿）**

- `internal/service/plan_executor_dag_run_cancel_test.go`：TestDagRun_CancelDuringRetryBackoff_RaceClean（失败注入 transient 错误使 maybeRetryStep 进入 5s backoff，100ms 后 cancel ctx：断言 run 返回 ctx.Canceled、board 终态 Failed、卡在 Running 的步骤被 sweep 成 Skipped；该用例在修复前于 `go test -race` 下必然报 data race：run goroutine 无锁写 canceled vs retry goroutine 持锁读），TestDagRun_CancelTerminalClassificationUnchanged（取消后 publishPlanBoardTerminal 走 Fail 分支、signalStartup(false, canceled reason) 首发有效）

**风险**：无行为变化；唯一注意点是后续新增 canceled 读点必须遵守『持锁或屏障后程序序』契约，注释里已写明。

---
### 发现#7 — DAG 扇出配额闸门：per-session 槽位延后准入 + 终态重扫与单飞重扫活性兜底

**修法**

语义裁定：配额维度=per-spirit-session（与 GetMaxParallelTeams + ListActiveTeams 既有口径严格对齐：上限来自 spirit agent parallel_config 的 max_concurrent_teams，活跃计数按 spiritSessionID），超限执行语义=延后准入（不阻塞不失败）：就绪但无槽步骤保持 Pending，每次步骤终态转移触发全局重扫，且在『本 run 在飞=0 且就绪被全数延滞且仅外部占用』时由单飞 2s 重扫循环兜底活性（否则外部团队完成无人能唤醒本 run，board 会带着 pending 步骤误判 Completed）。

落位：
1) internal/service/plan_executor.go：新增窄端口
```go
type planExecutorQuotaPort interface {
    GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int
    ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
}
```
（tools.SpiritTeamQueryPort 的 2 方法子集，*SpiritTeamAssembler 天然满足：spirit_team_assembler.go:123/161）；PlanExecutor 加字段 quota planExecutorQuotaPort + 后注入方法 SetQuotaPort（与 SetTaskPlanUpdater 同风格）；newDagRun 透传 pe 引用即可，dagRun 不加新锁。
2) admitReadySteps（#15 新函数，dag_flow.go）配额闸门实现：锁外先算 external——max<=0 或 port==nil → 不限（对齐入口门 maxParallel>0 的 fail-open 语义，ListActiveTeams 出错同样 warn+按 0 处理）；否则 ownTeams=锁内统计『Status==Running 且 MappedTeamStageID 非空』步数，external=max(0, len(active)-ownTeams)（Orchestrate 已建团到 MappedTeamStageID 回填的微窗口会短暂双计，偏保守永不欠计，下一重扫自愈）；available=max-external-runningCount。然后在同一 r.mu 临界区按 board 下标序准入：available>0 才允许预标记，runningCount 由锁内 O(n) 计数派生（不写计数器字段，根除 retry/泄漏记账面）；每准入一步 available--。confirm_before 步骤预标记后持槽等待人工确认（保守语义，防批量批准瞬间超发；拒绝即释放并经重扫准入后续步骤）。
3) 活性兜底：admitReadySteps 结束时若『本次准入 0 且仍存在就绪 pending 且 runningCount==0 且配额受限』，则 r.mu 内置单飞标志 quotaRearmActive 并 safego.Go：timer 2s（测试可收窄字段 quotaRearmDelay）→ Done/清标志 → 重调 admitReadySteps；ctx.Done 立即退出。cascade 终态/cancel sweep 路径无需准入调用（run 收尾）。
4) internal/service/chat_wire.go ProvideChatService：在既有 `if planExec != nil { ... }` 块内追加 `if deps.Team.SpiritAssembler != nil { planExec.SetQuotaPort(deps.Team.SpiritAssembler) }`（deps.Team.SpiritAssembler 类型 *SpiritTeamAssembler，chat_orchestrator.go:329）。

**行为契约**

配额不变量：任一时刻 runningCount(dagRun)+externalActive(session) ≤ max（max>0 时），瞬态双计只多不少；超限时步骤滞留 Pending 不产生任何事件/持久化写。活性不变量：每次步骤终态转移（Completed/Failed-terminal/Skipped-denied）后必有一次全局 admitReadySteps；在飞为 0 且仍有就绪延滞步骤时，单飞重扫循环以 quotaRearmDelay 周期重试直至准入或 ctx 取消，cancel 时由 sweepNonTerminalSteps 统一置 Skipped（Pending→Skipped 合法）。口径对齐声明：per-spirit-session 上限与入口门 plan_and_execute(spirit_tools.go:210-223) 同源（GetMaxParallelTeams/ListActiveTeams），dagRun 不做 per-board 独立额度；重试路径不新占槽（Running 计数在重试期间持续为 1）。

**改动文件**

- `internal/service/plan_executor.go` — 新增 planExecutorQuotaPort 窄端口 + quota 字段 + SetQuotaPort 后注入（紧邻 SetTaskPlanUpdater :314）
- `internal/service/plan_executor_dag_flow.go` — admitReadySteps 内嵌配额闸门（external 锁外计算、available 锁内扣减、O(n) runningCount 派生）+ maybeRearmQuotaSweep 单飞重扫 + quotaRearmActive/quotaRearmDelay 字段
- `internal/service/chat_wire.go` — ProvideChatService 内 planExec!=nil 块追加 SetQuotaPort(deps.Team.SpiritAssembler)

**测试设计（TDD：先红后绿）**

- `internal/service/plan_executor_dag_quota_test.go`：TestDagRun_FanOutQuotaDefersExcessRoots（stub port：max=1、active=nil；3 根步骤：并发 Running 峰值==1，全部完成，board Completed）；TestDagRun_QuotaSlotFreedAdmitsNext（首根完成后断言第二根在超时内被 auto 准入，验证终态转移→全局重扫链路）；TestDagRun_QuotaExternalActiveBlocksThenRearmAdmits（active 固定返回 1 个外部团队、max=1：首轮准入 0；随后清零 active，断言 quotaRearmDelay=20ms 的单飞重扫在 1s 内准入根步骤——覆盖活性兜底）；TestDagRun_QuotaFailureReleasesSlotToSibling（max=1，根 X 注入 transient 错误连挂 2 次耗尽重试，级联后独立根 Y 被准入并完成；board 终态 Failed 但 Y 的 Started 事件已发）；TestDagRun_QuotaNilPortUnlimited（port=nil 时 3 根并发 Running 峰值==3）

**风险**：external 计数依赖 ListActiveTeams 的及时性（与入口门同一数据源，风险同源）；confirm_before 步骤持槽等待是保守权衡，已在契约中声明，若产品侧要求确认等待不占额度，需再评审（会引入批量批准超发窗口）；单飞重扫是轮询（无跨 board 完成信号可订阅），2s 周期对 DB 压力可忽略。

---

**遗留问题**：
- confirm_before 步骤预标记后持配额槽位等待人工确认（防批量批准超发），产品侧是否接受该保守语义？若不接受需改为『批准后再竞争槽位』，会引入超发窗口，需另行评审
- 单飞重扫采用 2s 轮询（quotaRearmDelay，生产默认）兜底外部团队占用场景——当前无跨 board 完成事件可订阅；是否接受轮询，还是要为 PlanExecutor 间加 session 级完成信号（属新架构面，超出本集群最小修复范围）？
- ListActiveTeams 出错时配额 fail-open（与 spirit_tools.go 入口门 warn+跳过完全对齐）是否符合预期？若要求 fail-closed 会改变扇入任务的饿死特征，需确认

## 3.6 集群 D6 - Graph 运行时事件流与终态落库：

> 四条外科手术修复全部落在 internal/biz(+data/service 各一处)，零 wire 改动。F1：consumeRuntimeEvents 收口时按 FSM 终态（IsGraphExecutionTerminal）判定是否触发 onComplete，interrupt 的 waiting_human 不再误触发 projector 停播/telemetry Finish，四个调用点一处修复全继承。F2：为 GraphDone/NodeStart/NodeEnd 补与 NodeError/ExecutionError/Interrupt 对称的 streamGen 守卫，杜绝旧流副作用。F3：终态/等待态统一「先 UpdateRun(有界重试) 后 sink 外发」，持久化耗死记 graph.record_dead_letter 并跳过外发，满足 AS-EVT-01。F4：biz 新增窄端口 GraphOrphanRunScanner（data raw-SQL 排除 team_graph_sessions），FinalizeOrphanedStandaloneExecutions 复用幂等 FinalizeTeamGraphExecution，由 ProvideGraphUsecase 启动调用一次；team 行管辖边界不动。所有核心测试放 internal/biz/internal/data（可独立跑），service 测试一处标注随 WIP 恢复后运行。

**不变量承诺**：AS-FSM-01：D6-F1/F3 把「流关闭」从终态判定中彻底剔除，终态唯一真相源是 GraphExecutionStateMachine（IsGraphExecutionTerminal/applyExecTransition），CANCEL 仍收口保证 observer 解绑，与现有 N1 done-driven 收敛注释一致。AS-EVT-01：D6-F3 将 OnRunCompleted/OnRunFailed/OnRunWaitingApproval 对齐 Critical 级「先持久化后外发+有界重试」，持久化耗死记入 step graph.record_dead_letter 的 Error 日志（死信可见）；NodeStart/NodeEnd 等进度件保持 Important/Informational 级语义不变。org-invariants/ADR-86：D6-F4 的扫描以 SQL 排除 team_graph_sessions 行，team 图执行继续由 TeamGraphRunCoordinator/team_orphan_recovery 单点管辖，F-B 缝与双引擎边界不被突破；未触碰 organizations/dept_lead/花名册。DOC-SYNC：落地后需同步 83 韧性三件套（docs/development/83-* 若档）与 65-module-cross-reference-full.md §1.9 事件可靠性行（standalone 启动终态化归属）。

### D6-F1 — interrupt 暂停流不再被当作 done：observer 收口改由 FSM 终态判定

**修法**

单一收口点修复，不改四个调用点：在 internal/biz/graph_execution_usecase.go 的 consumeRuntimeEvents 中，把末尾 `if onComplete != nil { onComplete() }`（现 730-732 行）改为：在 execMu 收敛块内捕获收敛后状态 `terminal := IsGraphExecutionTerminal(ParseGraphExecutionState(exec.Status))`，仅在 terminal 为 true 时才调用 onComplete()。interrupt 使 exec 进入 waiting_human（IsGraphExecutionTerminal=false）：投影器(GraphOrchestrationProjector.OnGraphExecutionComplete)与遥测(GraphExecutionTelemetry.OnGraphExecutionComplete→bridge.Finish)不再提前收口，resume 后新 generation 的 consumer 沿用仍在运行的 projector/telemetry，真实终局时一次性收口。cancel 路径（Cancelled 是终态）仍触发 onComplete，保证 observer 解绑不泄漏。流关闭本身永不作为 done 信号——延续现 N1 done-driven 设计，终态判定全部走 FSM。四个 consumeRuntimeEvents 调用点（ExecuteGraph:348-350、ExecuteGraphBuildConfig:408-410、ResumeExecution:546-548、graph_execution_recover.go:85-87）传nil或共享闭包，自动继承修复。

**行为契约**

消费循环退出后：(1) 若 gen 失配 → 直接返回（既有）；(2) 状态收敛后按 FSM 判定：Completed/Failed/Cancelled → 触发 observer.onComplete 恰好一次；WaitingHuman/Running(evicted) → 不触发；(3) 流正常结束但无 done 且非 interrupt → 仍按既有 N1 fail-closed 转 Failed 并触发。不变量：observer.projector/telemetry 生命周期 == 执行 running…terminal 全周期；resume 期间不重建 projector（Start 时已用 context.Background + execID 键）。

**改动文件**

- `F:\myproject\aranea-agents\internal\biz\graph_execution_usecase.go` — consumeRuntimeEvents：收敛块捕获 terminal 布尔；onComplete 调用改为 `if terminal && onComplete != nil`；删除/不再依赖 sawDone 对 onComplete 的语义（sawDone 继续仅用于 fail-closed 判定）

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\internal\biz\graph_execution_usecase_stream_test.go（新增）`：TestConsumeRuntimeEvents_InterruptSkipsOnComplete（interrupt→关流：onComplete 未调用、status=waiting_human、UpdateRun 已持久化 waiting_human）；TestConsumeRuntimeEvents_DoneFiresOnCompleteAfterPersist（done→completed：onComplete 恰好一次且在 sink 之后）；TestConsumeRuntimeEvents_CancelledStreamStillFiresOnComplete（cancel 后流关：onComplete 触发，observer 解绑不泄漏）

**风险**：风险极低：唯一行为变化是 interrupt 后不再触发 onComplete；现有测试 TestConsumeRuntimeEvents_InterruptThenStreamEnd_StaysWaitingHuman 传 nil onComplete 不受影响；cancel 路径由新增用例覆盖防回归。

---
### D6-F2 — streamGen 防护对称补全：GraphDone/NodeStart/NodeEnd 同代际守卫

**修法**

对称补全：在 internal/biz/graph_execution_usecase.go 的 updateExecutionFromRuntimeEvent 中，为 DomainEventGraphDone(761-767)、DomainEventGraphNodeStart(768-793)、DomainEventGraphNodeEnd(794-810) 三个 case 在取得 execMu 后第一步加入与 NodeError(814)/ExecutionError(849)/Interrupt(871) 完全一致的守卫 `if exec.streamGen != gen { unlock; return }`。NodeStart 的守卫必须包覆 taskCoord.OnGraphNodeStart 副作用调用与 runEventSink.OnNodeStarted；NodeEnd 守卫包覆 Steps upsert/UpdateRun/OnNodeCompleted；GraphDone 守卫包覆 CurrentState 覆写。与 FSM 的交互：三 case 本就不做 FSM 迁移，gen 守卫杜绝旧流把 waiting_human/running 的新流状态污染（如旧 GraphDone 的 FinalState 覆写新流 CurrentState、旧 NodeEnd 向新流 Steps 追加幽灵步骤、旧 NodeStart 重复建 task 看板任务），与 graph.go:220-224 自述契约对齐。同时更新 GraphExecution.streamGen 字段注释，明确守卫覆盖全部状态/副作用变更新件。

**行为契约**

同 generation 事件按既有语义处理；任何 gen != exec.streamGen 的事件在进入 execMu 后立即丢弃，不产生：状态字段写、FSM 迁移、DB UpdateRun、runEventSink 通知、taskCoord 看板副作用。waiting_human 期间允许同窗的迟到 NodeEnd（gen 相同）照常落库——interrupt 前的已完成步骤不丢。

**改动文件**

- `F:\myproject\aranea-agents\internal\biz\graph_execution_usecase.go` — updateExecutionFromRuntimeEvent 三 case 内补 gen 守卫；GraphExecution.streamGen 注释更新
- `F:\myproject\aranea-agents\internal\biz\graph.go` — streamGen 字段注释同步（仅注释行，若与 usecase 注释重复可省略此条改动）

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\internal\biz\graph_execution_usecase_stream_test.go（新增）`：TestUpdateExecutionFromRuntimeEvent_StaleStream_IgnoresNodeStart（gen 失配：CurrentNode 不变、taskCoord 未调用、sink 未通知）；_IgnoresNodeEnd（Steps 未追加、UpdateRun 未调用）；_IgnoresGraphDoneFinalState（CurrentState 不被旧 FinalState 覆写），复用现有 stale 测试骨架（exec.streamGen=2, 调 gen=1）

**风险**：老流事件被整件丢弃可能使 resume 瞬间少一条旧 NodeEnd 步骤——但该 step 属于已取消旧 runtime，丢弃正是设计意图；onComplete 时序不变。

---
### D6-F3 — 终态「先落库后外发」重排 + 落库有界重试与死信日志（AS-EVT-01）

**修法**

终态重排+重试。(1) consumeRuntimeEvents 收敛段：现顺序 706-715(sink)→726(UpdateRun) 重排为 UpdateRun(带重试)→sink(OnRunOutput/OnRunCompleted/OnRunFailed)→terminal 时 Finally onComplete；UpdateRun 失败重试 2 次（25ms/100ms backoff，persistCtx 已 detach），终败则 lg.Error(step graph.record_dead_letter) 并跳过 sink，onComplete 仍触发（内存终态已收敛，observer 负责解绑属进程内事实）。(2) updateExecutionFromRuntimeEvent：NodeError !Retrying(838)、ExecutionError(863)、Interrupt(884) 三处把 runEventSink 调用移到 UpdateRun 之后且仅在持久化成功时发送；新增私有方法 persistRunWithRetry(ctx, snap, stepID) error 供四处复用；NodeStart/NodeEnd/Retrying-NodeError 为 Important/Informational 级进度件，不改顺序、不重试、落库失败仅 Warn（既有），缩小 diff。

**行为契约**

不变量：任何 OnRunCompleted/OnRunFailed/OnRunWaitingApproval 外发之前，对应终态/等待态必已写库成功（可观察失败=重试耗尽，且此时不外发）。UpdateRun 重试在消费 goroutine 内同步进行（上限 ~150ms/event），不向已 detach 的执行追加副作用；FSM 迁移与 FinalizedAt 设在持久化重试之前完成（内存权威先行、DB 必须追上）。

**改动文件**

- `F:\myproject\aranea-agents\internal\biz\graph_execution_usecase.go` — consumeRuntimeEvents 终态段重排+persistRunWithRetry；NodeError/ExecutionError/Interrupt 三 case sink 后置

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\internal\biz\graph_execution_usecase_stream_test.go（新增）`：TestPersistBeforeSink_TerminalDone（顺序 [UpdateRun(ok),OnRunCompleted,onComplete]）；TestPersistRunWithRetry_FailThenSuccess（首次失败重试成功，sink 仍精确一次）；TestTerminatePersistAllFail_SkipsSink（UpdateRun 恒败：无 OnRunCompleted、Error 日志含 graph.record_dead_letter、onComplete 仍触发）；TestInterruptPersistsBeforeWaitingApprovalSink（顺序 [UpdateRun(waiting_human),OnRunWaitingApproval]）

**风险**：终态 sink 因持久化失败被跳过：twin 侧收不到完成 webhook——这正是 AS-EVT-01 想要的 fail-visible（Error 日志可告警）；后续可挂 EventWAL(Critical 既有基建) 做 webhook 侧重放，非本次范围。

---
### D6-F4 — standalone graph_executions 启动终态化扫描（biz 窄端口 + provider 触发）

**修法**

扫描放入 biz，触发点复用 service 层现有 provider，零 wire 改动。(1) internal/biz/graph.go 新增窄端口 GraphOrphanRunScanner{ ListOrphanedStandaloneRuns(ctx, status string, limit int) }（Stability:evolving 新接口，不侵入 GraphRunRepo 大接口，存量 fakes 不受影响）。(2) internal/data/graph.go 实现该方法：raw SQL `SELECT ... FROM graph_executions WHERE status=? AND id NOT IN (SELECT exec_id FROM team_graph_sessions) ORDER BY started_at LIMIT ?`（team_graph_sessions 表已由 EnsureTeamGraphSessionSchema 保证存在；NOT IN 排除受 team coordinator 管辖的行，契合 ADR-86 F-B 边界铁律——team 行归 RecoverSessions/team_orphan_recovery 终态化，standalone 行归本扫描）。(3) internal/biz/graph_orphan_sweep.go 新增 GraphExecutionUsecase.FinalizeOrphanedStandaloneExecutions(ctx)(int,error)：runRepo nil 或不实现 scanner 接口→(0,nil) 幂等空转；对每条 running standalone 行调用既有 FinalizeTeamGraphExecution(ctx,id,true,"server restart: orphaned graph execution finalized")（其终态守卫天然幂等，二次启动空转）；逐条失败仅 Warn 并累计，不中断。(4) 触发：internal/service/graph_wire.go ProvideGraphUsecase 在 biz.NewGraphUsecase 返回后以 context.Background()+10s 超时同步调用一次，失败仅 Warn（对齐 chat_orchestrator.go:630 RecoverSessions 构造期对账先例，且 ProvideGraphUsecase 不在并行会话禁区，无需改 wire.go/wire_gen.go）。与 recover 的关系：本扫描只做终态化不续跑（standalone 无 resume 入口的责任人）；team 行被 SQL 排除，随后 ChatOrchestrator 构造期的 crash-resume 照常接管，两者无竞态。

**行为契约**

进程每次启动恰好执行一次全量扫描；仅 status=running 且无 team_graph_sessions 行的执行被收敛为 failed(reason=server restart)；waiting_human/completed/failed/cancelled/team-owned 一律不动；多次启动/多副本交叠安全（终态守卫幂等）；扫描失败不影响 server 启动（非致命，日志+流程告警通道可后续挂）。

**改动文件**

- `F:\myproject\aranea-agents\internal\biz\graph_orphan_sweep.go（新增）` — 新增 GraphExecutionUsecase.FinalizeOrphanedStandaloneExecutions + GraphUsecase 委托
- `F:\myproject\aranea-agents\internal\biz\graph.go` — GraphRunRepo 附近新增 GraphOrphanRunScanner 窄接口
- `F:\myproject\aranea-agents\internal\data\graph.go` — graphRunRepo 实现 ListOrphanedStandaloneRuns（raw SQL 排除 team_graph_sessions）
- `F:\myproject\aranea-agents\internal\service\graph_wire.go` — ProvideGraphUsecase 构造后调用一次启动扫描（超时+仅 Warn）

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\internal\biz\graph_orphan_sweep_test.go（新增）`：TestFinalizeOrphanedStandaloneExecutions_SkipsTeamAndNonRunning（fake scanner 返回混合表：仅 running-standalone 被 finalize 为 failed 且 UpdateRun 一次）；_IdempotentSecondRunNoOp；_NilRepoOrNoScannerNoOp
- `F:\myproject\aranea-agents\internal\data\graph_pagination_test.go（同包新增用例，internal/data 可跑）`：TestGraphRunRepo_ListOrphanedStandaloneRuns_ExcludesTeamSessionRows（seed 两 running 行，其一挂 team_graph_sessions：查询只返回 standalone 行；另验 limit 生效）
- `F:\myproject\aranea-agents\internal\service\graph_wire_test.go（新增，受 WIP 阻塞）`：TestProvideGraphUsecase_InvokesOrphanSweepOnce（stub GraphRunRepo 记录调用；标注：随 internal/service WIP 编译恢复后运行）

**风险**：多副本部署同时启动会争用同一批 running 行：FinalizeTeamGraphExecution 先 FSM 终态守卫、UpdateRun 幂等覆盖同值，多个失败仅告警——可接受；team_graph_sessions 为 raw-SQL 表（非 ent），故查询须走 raw DB 而非 ent predicate，实现时注意方言占位符(现有 repo 已按 dialect 处理)。

---

**遗留问题**：
- F-B（team graph）路径终态由 FinalizeTeamGraphExecution 收敛但从不触发 execObserver——GraphOrchestrationProjector/telemetry 对 team run 是否依赖别的解绑钩子？若 team 图也起过 projector，存在同类泄漏/提前收口，应单列发现复核（非本集群范围）。
- D6-F3 终态 sink 持久化失败后直接跳过——twin webhook 是否需要接 EventWAL(Critical) 做重放补偿？需离决策（改 contract 基建，超最小修复）。
- standalone 崩溃续跑（HasCheckpoint+RecoverOrphanedExecution）当前仅 team 触发；未来若给精灵入口 standalone 续跑能力，本扫描应为带 checkpoint 的行放行还是仍判死？需在 83 续期文档中记录决策。

## 3.7 集群 D7 — 经验复用缓存契约（OrchestrationCache 全链路：DQ 学习信号 / 约束指纹 / 读写键对称 / 持久化并发 / workspace 隔离）：

> 六条外科手术修复覆盖 7 项指控：(F1) DQ 双向学习——失败以 ×0.8 乘法衰减、下限 0.1 触底下架，Synthesize err 分支新增 learnRecipeFailure 复用同一降权路径，与 AgentPerformance 分域不双罚；(F2) 指纹真正写入（RecipeProvenance 随 learn 落 entry）+ planner/L0 双消费闸，RecipeKeysReusable 改 want 非空时 require have==want（旧条目一次性降级、无约束任务放行）；(F3) 读侧 NormalizeDomainPath 对齐写侧，"其他"域读写双封禁；(F4) 成员收缩有效 DQ 折算+同分全序确定性；(F5) 持久化按 pattern 分行（新表 20261287+一次性 legacy 导入+EvictStale 接线），评估后否定单行 CAS；(F6) 配方按 workspace 隔离（Entry.WorkspaceID+scoped 内存键+复合主键），能力表全局语义按 P2-B 保留。全部修复零 wire_gen.go 改动（构造函数/装配线不动），零禁区文件触碰。

**不变量承诺**：org-invariants 合规：本组修复不触碰公司树/部门/岗位编制、不把配方当作编排权威——花名册绑定（bindRosterSpecialist）与 IsHeuristicAssignable/FilterRecipeAgentKeys 的治理岗排除保持为唯一权威，L0 配方仅是带 fail-closed 闸门的缓存加速层；shared 能力目录（BuildAll 无 WorkspaceID，P2-B）与配方按 workspace 隔离两个域显式区分，不回退“全员共用精灵工具箱”。AS-FSM-01：配方 DQ 生命周期在 D7-F1 给出显式四态状态机（Serving/Demoted/Evicted/TTL-Evicted）及全部转移条件，后续应回写 65 交叉参考 §1.42/B.10.21 三件套（DOC-SYNC 触发：RecordDomainRecipe 契约、RecipeKeysReusable 语义、OrchestrationCacheRepo 接口均变化）。AS-EVT-01：配方持久化维持 Important 级（异步落库、失败 Warn 不阻断编排主链路）不升级 Critical；D7-F5 分行 upsert 把持久化失败的爆炸半径从“整份缓存 JSON”缩小到单条目，与事件可靠性分级精神一致且不引入新的重试义务。降权信号与 AgentPerformance 既有半负反馈严格分域（配方条目 vs 员工履历），同一失败事实不双罚同一实体。落地顺序建议 D7-F3→F1→F2→F4→F5→F6（F5/F6 可互换但 F6 依赖 F5 的复合主键实现 workspace 列落库）。

### D7-F1 — DQ 双向学习信号：失败降权 + 合成失败纳入学习（发现#8）

**修法**

双向学习信号设计：在 biz 的两个写入入口统一引入结果分支——`dqScore < DQSuccessThreshold(0.5)` 视为失败信号：`existing.DQScore = max(DQScoreMin(0.1), existing.DQScore × 0.8)`（新增常量 DQRecipeFailureDecay=0.8），保留 AgentKeys/Topology 不动、刷新 UpdatedAt；触底（≤DQScoreMin）整条目删除（下架）。成功路径维持现有 strict-upgrade 语义（existing>=new 早退不动）。条目不存在时失败信号 no-op（不造负面首条）。
闭中找到 #8 的另一半：Synthesize 的 `synthesis.SynthesizeResults` err 分支（task_orchestrator_impl.go:179-182）当前直接 return 绕过了 :206 的 learnFromOrchestration——新增小方法 `learnRecipeFailure(ctx, handle)`：resolvePlanLearningHint 解析主域后，以 dq=0 调用 RecordDomainRecipe/RecordCompletionWithAgents，复用同一降权分支（不改 learnFromOrchestration 签名）。computeDQScoreFromSynthesis 对失败合成已产出 0.2 → 自动走降权。
与 AgentPerformance 既有半负反馈不重复：perf 按 (agent,taskType) 记 successCount=0 的运行均值，本修复只动配方条目 DQ（不同实体维度）；learnRecipeFailure 刻意不碰 perfRepo（合成 infra 失败≠员工失败，避免重复惩罚同一信号源）。降多少/下限：×0.8 乘法衰减、下限 0.1、触底淘汰。

**行为契约**

配方 DQ 状态机（满足 AS-FSM-01 显式化）：Serving(DQ≥0.7，可被 L0/记忆命中消费) --成功且 dq>existing--> Serving(DQ=dq，严格升级)；--失败 dq<0.5 且 DQ×0.8≥0.1--> Serving|Demoted(DQ 衰减)；--失败且衰减至 ≤0.1--> Evicted(条目删除+repo DeleteEntry)；Demoted(DQ∈(0.1,0.7)，停止服务但保留配方体) --成功 dq>existing--> Serving；--TTL 30d--> Evicted。失败信号永不改变 AgentKeys/拓扑体（降权不毁配方结构）；UpdatedAt 任一信号都刷新（TTL 从最近信号起算）。空缓存上的失败信号为零操作。

**改动文件**

- `internal/biz/spirit_orchestration_cache.go` — RecordCompletionWithAgents（:286-312）与 RecordDomainRecipe（:406-437）的写前分支改造：加失败降权+触底淘汰；新增 DQRecipeFailureDecay 常量；淘汰时调 repo DeleteEntry（签名在 D7-F5 落地，本轮先内联注释点位）
- `internal/agent/task_orchestrator_impl.go` — Synthesize 的 synthesis.SynthesizeResults err 分支（:179-182）增加 o.learnRecipeFailure(ctx, handle) 调用；新增 learnRecipeFailure 方法（紧邻 learnFromOrchestration :441）

**测试设计（TDD：先红后绿）**

- `internal/biz/spirit_orchestration_cache_test.go`：TestRecordDomainRecipe_FailureDecaysDQ（0.9 连续失败→0.72/0.576/...单调降至触底下架、下架后 Get miss）；TestRecordDomainRecipe_FailurePreservesAgentKeys；TestRecordDomainRecipe_LowerSuccessKeepsExisting（0.9 vs 0.75 → 保持 0.9，防回归）；TestRecordDomainRecipe_DemotedStopsServing（衰减至 <0.7 后 BestRecipeForDomain miss、≥0.1 时 QueryByTaskPattern 仍可列出）；TestRecordCompletion_FailureEvictsAtFloor
- `internal/agent/task_orchestrator_learn_test.go（新建）`：TestSynthesize_SynthesisError_DowngradesRecipe（stub synthesis 返回 err → 既有 0.9 配方 DQ 下降且 AgentKeys 不变；perfRepo 收到零次 Upsert——证明不与 AgentPerformance 重复计数）；复用 task_orchestrator_fsm_test.go 的 stub 风格

**风险**：合成 infra 错误（LLM 超时等）会误伤健康配方的 DQ——缓解：降权仅 ×0.8 且不毁配方体，连续 3 次健康成功即可回到原位；Warn 日志含 orchestration_id+reason 供观测回滚。乘法衰减是单调的，不会 oscillate。

---
### D7-F2 — ConstraintFingerprint/PlaybookID 真正写入 + 消费侧双闸 fail-closed（发现#9）

**修法**

写入侧（两处落点）：biz 新增 `RecipeProvenance{PlaybookID, ConstraintFingerprint}`，RecordDomainRecipe/RecordCompletionWithAgents 保持现签名（内部委托零值 provenance，兼容现有测试与 spirit_orchestration_progress.go:226 调用），新增 `RecordDomainRecipeWithProvenance`/`RecordCompletionWithAgentsWithProvenance`；orchestrator 的 `resolvePlanLearningHint`（task_orchestrator_impl.go:556-572）扩展第三返回值 provenance（来源 plan.MemoryHit.ConstraintFingerprint/PlaybookID，nil-safe），learnFromOrchestration/learnRecipeFailure 调用新 provenance 版写入。
消费侧两个校验点（fail-open→fail-closed 的兼容策略）：① planner 既有守卫 task_planner_impl.go:228 语义无改代码、语义随函数变化生效；② L0 缺口 tryDomainRecipe——playbook_plan.go 提取包级函数 `playbookConstraintFingerprintForTask(ctx, org, taskText)`（现 currentConstraintFingerprint 的 eachCompanyMetadata+TryPlaybookForTask 循环下沉共享，taskPlannerImpl 方法改为委托）；agentAllocatorImpl 在 Allocate（agent_allocator_impl.go:86）用 taskPlan.UserMessage 计算一次 wantFp（impl.capBuilder==nil 或 org==nil → ""，fail-open 保旧测试路径），经 runPhaseAMatch(:121)→matchSubTask(agent_allocator_match.go:54)→tryDomainRecipe(agent_domain_match.go:38) 透传，校验 `biz.RecipeKeysReusable(entry.ConstraintFingerprint, wantFp)` 不符则 miss 落 roster/L1。
RecipeKeysReusable 新契约（一次性降级策略）：want=="" → true（当前任务不受剧本约束，放行）；want!="" → 仅 have==want 放行，have==""（旧条目/无 provenance 条目）→ false（旧条目 keys 不被剧本任务注入=一次性降级，Topology/Specialties 仍可复用，planner 侧 :228-230 已有的 keys 置 nil 逻辑自然承接）。AllocateExplicit 不加指纹拦截（显式点名=调用方意图，MemoryHit keys 已被①过滤）。

**行为契约**

指纹语义：ConstraintFingerprint=支配该次编排的剧本约束哈希（无剧本任务恒空）。不变量：受剧本约束的任务（want≠""）只复用同指纹配方体的 AgentKeys；无约束任务不受限。降级路径：旧条目对剧本任务失去 keys 复用权、其余字段照旧服役——不删数据、下次成功学习自动写入正确指纹完成自愈。治理岗排除（FilterRecipeAgentKeys）在显式路径继续生效，本修复不改。

**改动文件**

- `internal/biz/spirit_orchestration_cache.go` — 新增 RecipeProvenance 类型；OrchestrationCacheEntry 两字段真正落 entry；新增 WithProvenance 两个方法
- `internal/biz/playbook.go` — RecipeKeysReusable（:273-280）改 fail-closed 契约（want≠"" 时 require have==want）+ 注释
- `internal/agent/playbook_plan.go` — 提取包级 playbookConstraintFingerprintForTask；currentConstraintFingerprint 委托
- `internal/agent/task_orchestrator_impl.go` — resolvePlanLearningHint 增第三返回值 RecipeProvenance；learnFromOrchestration 改调 WithProvenance 写入
- `internal/agent/agent_domain_match.go` — tryDomainRecipe/agent_domain_match.go:38 增 wantFp 参数与守卫；matchSubTask/runPhaseAMatch 透传；Allocate 计算 wantFp
- `internal/agent/agent_allocator_match.go` — matchSubTask 签名与透传点
- `internal/agent/agent_allocator_impl.go` — Allocate(:86)/runPhaseAMatch 调用点(:121) 透传 wantFp

**测试设计（TDD：先红后绿）**

- `internal/biz/playbook_recipe_fingerprint_test.go（新建）`：TestRecipeKeysReusable：("","")→true、("a","")→true、("", "a")→false（一次性降级）、("a","a")→true、("a","b")→false
- `internal/agent/task_orchestrator_learn_test.go（新建）`：TestLearnFromOrchestration_WritesProvenance（plan.MemoryHit 带 fingerprint → entry 落库字段非空）；TestLearnFromOrchestration_NoMemoryHit_ZeroProvenance
- `internal/agent/agent_domain_match_test.go`：TestTryDomainRecipe_FingerprintMismatch_FallsToRoster（条目 fp=A、当前剧本 fp=B → L0 miss）；TestTryDomainRecipe_EmptyCurrentFingerprint_Permissive（非剧本任务仍复用 playbook 条目 keys=新契约）
- `internal/agent/task_planner_impl_part2_test.go`：指纹不符时 memoryHit.AgentKeysUsed 被置 nil、Specialties 保留（:228-234 行为锁定）

**风险**：RecipeKeysReusable 收紧后剧本任务对旧配方 keys 复用能力消失（预期内的一次性降级）；in-flight 旧 plan 的 MemoryHit keys 仍按生成时指纹放行（与现状一致，闭合需 allocator 挂 org reader=改 wire，本轮冻结不做）。

---
### D7-F3 — 读写键归一化对齐 + “其他”域读写双封禁（发现#10）

**修法**

读侧归一化：agent_allocator_match.go matchSubTask（:67-68）把 `subTask.DomainPath != ""` 改为 `spec := NormalizeDomainPath(subTask.DomainPath); if spec != "" && spec != domainLexiconOther`，传 spec 进 tryDomainRecipe（BestRecipeForDomain 的"调用方必须已归一化"契约由此兑现）。
解析侧落盘即归一：task_planner_decompose_llm.go:366 parseStreamSubTask 的 `DomainPath: raw.DomainPath` 改 `DomainPath: NormalizeDomainPath(raw.DomainPath)`（"" 恒等不变、词表外深路径如归一策略、下游 bindRosterSpecialist 内部本就做同类归并，行为中性偏正）。
"其他"域写封禁：learnFromOrchestration（task_orchestrator_impl.go:468）的 RecordDomainRecipe 前置守卫增加 `primaryDomainPath != domainLexiconOther`（与 collectSpecialtySlots:579 既有排除对齐——collectSpecialtySlots 已排除但 PrimaryDomainPath 不排除导致 "domain：其他" 污染）；读封禁即上文 matchSubTask 的 spec!=domainLexiconOther。历史 "其他" 条目依赖 30d TTL + D7-F5 的 EvictStale 接线自然出清，不做主动迁移。

**行为契约**

读写键对称不变量：任何进入 BestRecipeForDomain 的查询与 RecordDomainRecipe 的键均经过同一 NormalizeDomainPath；"其他" 桶永不写入配方、永不服务配方（两条独立的 fail-closed 闸门）。词表外路径在 subtask 落盘时点即归并到词表最近前缀，下游所有消费方（L0/roster/L1/学习回写）共享同一规范化视图。空 DomainPath 维持现状跳过 L0。

**改动文件**

- `internal/agent/agent_allocator_match.go` — matchSubTask L0 入口归一化+“其他”跳过
- `internal/agent/task_planner_decompose_llm.go` — parseStreamSubTask DomainPath 落盘归一化
- `internal/agent/task_orchestrator_impl.go` — learnFromOrchestration 写侧“其他”守卫

**测试设计（TDD：先红后绿）**

- `internal/agent/agent_domain_match_test.go`：未归一查询 "创作 / 文学"（含空格）命中已存 "创作/文学" 配方；"其他"/"随便" 归一类查询不命中任何 recipe（含已预置的 domain：其他 污染条目）；空域仍走 roster/L1
- `internal/agent/task_planner_decompose_llm_test.go（新建或并入现有 planner 测试文件）`：TestParseStreamSubTask_NormalizesDomainPath（"创作/诗歌"→"创作"、""→""、词表内原样）
- `internal/agent/task_orchestrator_learn_test.go（新建）`：TestLearnFromOrchestration_SkipsOtherDomain（主域=其他 → RecordDomainRecipe 零调用、走 :479 旧 key 回退）

**风险**：parseStreamSubTask 归一会改变 SubTask.DomainPath 的持久化值（词表外路径被归并）——这本来就是 lexicon 设计意图（注释：词表是约束非封闭集），validateRecipeSubTasks/roster 不受影响；前端按 DomainPath 展示的子任务将显示归一后路径（更一致）。

---
### D7-F4 — 成员收缩有效 DQ 重算 + 同分确定性（发现#13）

**修法**

两段：① biz BestRecipeForDomain（spirit_orchestration_cache.go:360-392）选择比较由“第一个严格更大者”改为全序：DQScore 降序 → UpdatedAt(RFC3339 解析，不可解析=零值) 降序 → TaskPattern 字典序升序。② agent tryDomainRecipe（agent_domain_match.go:57-72）成员过滤后计算 retained=1+len(members)、total=len(entry.AgentKeys)，retained<total 时 effectiveDQ=entry.DQScore×retained/total；effectiveDQ<biz.DomainRecipeMinDQ(0.7) → miss（Warn 日志带 retained/total/原 DQ 供审计），否则以 effectiveDQ 作为 MatchScore/MatchReason 返回。完整的成队配方 ratio=1 行为不变。

**行为契约**

配方有效性=DQScore×成员存活率，服务质量阈值恒为 DomainRecipeMinDQ：任何成员收缩后的配方若折算后低于阈值则不再以历史高分放行（落 roster/L1 重新匹配）。同分排序全序确定：同一缓存状态、同一查询必命中同一条目（跨进程、跨 map 迭代次序）。

**改动文件**

- `internal/biz/spirit_orchestration_cache.go` — BestRecipeForDomain 确定性全序比较
- `internal/agent/agent_domain_match.go` — tryDomainRecipe 成员收缩有效 DQ 折算与阈值闸

**测试设计（TDD：先红后绿）**

- `internal/biz/spirit_orchestration_cache_test.go`：TestBestRecipeForDomain_DeterministicTieBreak（两等分条目：新 UpdatedAt 者胜；同 ts 时 TaskPattern 字典序小者胜；多次迭代结果一致——防 map 随机序回归）
- `internal/agent/agent_domain_match_test.go`：TestTryDomainRecipe_MemberShrink_RecomputesDQ（0.9×3 keys 仅存 lead → 0.3 → miss 走 roster）；TestTryDomainRecipe_PartialShrink_PassesWhenAboveThreshold（0.9 存 2/3 → 0.6...调整阈值用例：0.95 存 2/2+... 折算 ≥0.7 放行且 MatchScore=折算值）

**风险**：部分成员被删的配方从“照旧服役”变为“折算降级/淘汰”，短期 L0 命中率下降（预期方向的收紧，由 MatchReason 可观测）；ratio 线性折算是刻意保守选择，未引入成员权重（无数据来源），如需角色加权另立 ADR。

---
### D7-F5 — 持久化分行 + 一次性导入 + EvictStale 接活（发现#11）

**修法**

评估结论：选“按 pattern 分行”而非“单行版本列 CAS”。CAS(在 system_settings 加 version 列+乐观锁重试) 仍需 reload-merge-retry 做全 map 权威合并，跨实例并发各自持有全量 map 时同键取舍逻辑复杂且仍会丢“另一实例新增的不同 pattern”；分行把并发冲突面收窄到单 pattern（同 pattern 并发写值语义近等价、DQ 取高天然收敛），并顺手消掉“每次完成 2 次全量序列化落库”（Record* 与 SetEntrySpecialties 各自 ToJSON+整行 upsert）。迁移成本可控：OrchestrationCacheRepo 接口仅 data 一处实现+biz 内部消费。
落地：新表 orchestration_cache_entries(workspace_id TEXT NOT NULL DEFAULT '', task_pattern TEXT NOT NULL, entry_json TEXT NOT NULL, dq_score REAL NOT NULL DEFAULT 0, updated_at TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, task_pattern))（SQLite/PG 双方言通用、幂等 CREATE IF NOT EXISTS，沿用 20261280 文件风格）；接口扩展为 LoadCacheJSON(保留=legacy 一次性导入源)+LoadEntries/UpsertEntry/DeleteEntry；biz persistToRepo 改 persistEntry（每次只 upsert 变动的单条 entry）；InitFromRepo 流程=LoadEntries 非空直接 seed→为空则读 legacy LoadCacheJSON→LoadFromJSON 解析→逐条 UpsertEntry 导入（幂等，重跑安全）→尾部调用 EvictStale 一次（接活死代码，逐条 DeleteEntry）；Get 的惰性过期删除维持内存侧即可。registry 在 ddl_migration_table_late.go 追加 {Version:20261287, Name:"orchestration_cache_entries", SQL:...}（当前最大 20261286/20261279）。

**行为契约**

持久化粒度=单条配方：任一 Record*/SetEntrySpecialties 只 upsert 本条目行；淘汰只删本条目行；不同 pattern/不同 workspace 的并发写零相互覆盖。legacy system_settings.orchestration_cache_json 列保留不删（回滚安全），仅在新表为空时充当一次性导入源。EvictStale 从死代码变为 InitFromRepo 启动路径的标准清理步（不含周期任务——渐进最小改动）。

**改动文件**

- `internal/biz/spirit_orchestration_cache.go` — OrchestrationCacheRepo 接口扩展（LoadEntries/UpsertEntry/DeleteEntry）；persistToRepo→persistEntry 单条落库；InitFromRepo 导入+EvictStale 接线
- `internal/data/orchestration_cache_repo.go` — orchestrationCacheRepo 实现三个新方法（RenumberPlaceholders 双方言 upsert/delete/select）；保留 LoadCacheJSON
- `internal/data/sql/migrations/20261287_orchestration_cache_entries.sql` — 新建 20261287_orchestration_cache_entries.sql
- `internal/data/ddl_migration_table_late.go` — ddlMigrationsLate 追加注册项

**测试设计（TDD：先红后绿）**

- `internal/data/orchestration_cache_repo_test.go（新建）`：TestOrchestrationCacheRepo_UpsertLoadRoundTrip（同一 workspace+pattern 两次 upsert 取最新）；TestOrchestrationCacheRepo_DeleteEntry；TestOrchestrationCacheRepo_RowIsolation（不同 pattern/不同 ws 互不覆盖=并发丢更新根修的回归锁）；沿用 data 包现有 sqlite harness（参照 self_check_report_repo_test.go 模式）
- `internal/biz/spirit_orchestration_cache_test.go`：TestInitFromRepo_ImportsLegacyJSONOnce（repo 只给 legacy JSON → seed 成功且触发逐条 UpsertEntry）；TestInitFromRepo_RunsEvictStale（预置过期条目→启动后被踢且 DeleteEntry 被调）；fake repo 记录调用

**风险**：同 pattern 跨实例并发写仍是 last-writer-wins（记录在案的可接受残余：值语义等价且 DQ 取高收敛）；导入赛跑——两实例同时启动都见空表都导入 legacy， upsert 同键幂等无分叉；data 包已有 sqlite 测试 harness 覆盖 ensureAllSchemas 全套 migration，新表自动入检。

---
### D7-F6 — 配方按 workspace 隔离（发现#14）

**修法**

语义划界（回应任务要求）：BuildAll 不设 WorkspaceID 是 P2-B 共享能力目录的有意语义，本修复不动；隔离只作用于“配方复用”域。
实现：OrchestrationCacheEntry 增 `WorkspaceID string json:"workspace_id,omitempty"`；biz 内部 `wsOf(ctx)`=workspace.IDFromContext(ctx)，SystemWorkspaceID 归一为 DefaultWorkspaceID（biz 引 internal/workspace 已有先例 agent_duplicate.go:36，无循环）；内存 map 键改为 scoped 键 `ws+"\x1f"+pattern`（Entry.TaskPattern 保持原样，JSON 往返无损）；写侧（各 Record*/SetEntrySpecialties）以调用 ctx 的 ws 落 entry 并组键；读侧 QueryByTaskPattern 用 ctx ws 组键、Get 拆出私有 getForWorkspace；三个无 ctx 导出方法改签名补 ctx：SuggestTopology(ctx,...)/SuggestBestAlternativeTopology(ctx,...)/BestRecipeForDomain(ctx,...)。调用点：task_planner_impl.go:762、spirit_team_assembler.go:228（该方法已有 ctx）、spirit_orchestration_progress.go:235（同函数 ctx 在作用域）、agent_domain_match.go:42（tryDomainRecipe 经 matchSubTask 透传 ctx——随 D7-F2 的签名触碰同点落地）。BestRecipeForDomain 扫描时过滤 `e.WorkspaceID==wsOf(ctx)`。加载兼容：LoadFromJSON/LoadEntries seed 时 entry.WorkspaceID==""→归一为 DefaultWorkspaceID（与 workspace.AssertWorkspace 的 empty→default 语义对齐）。跨 workspace 历史数据处置：旧 JSON 条目全部落 default 桶=对 default/system 调用方可见如旧、对其余租户即刻不可见；不向具体租户做归属迁移（无从推断，方向 fail-closed）。

**行为契约**

隔离不变量：任一配方条目的写、读、淘汰、导入都在单一 workspace 桶内闭合；ctx 无 workspace 的调用（含全部既有单租户部署与测试 background ctx）统一落 default 桶——单租户行为与今天完全一致；SystemWorkspaceID 读写等同 default（后台 cron 学习无跨租户泄漏通道）。能力目录（BuildAll）保持全局共享语义不变。

**改动文件**

- `internal/biz/spirit_orchestration_cache.go` — wsOf/scoped key、Entry.WorkspaceID 字段、三方法加 ctx、读侧过滤、Load seed 归一
- `internal/agent/task_planner_impl.go` — queryMemory 调用点改 SuggestTopology(ctx,...)
- `internal/agent/agent_domain_match.go` — tryDomainRecipe 内 BestRecipeForDomain(ctx,...)（ctx 由 matchSubTask 传入）
- `internal/service/spirit_team_assembler.go` — SpiritTeamAssembler.SuggestTopology(:228) 传 ctx——仅一行，唯一 internal/service 触碰点
- `internal/biz/spirit_orchestration_progress.go` — SuggestBestAlternativeTopology 调用点(:235)传 ctx

**测试设计（TDD：先红后绿）**

- `internal/biz/spirit_orchestration_cache_test.go`：TestWorkspaceIsolation_WriteReadScoped（ws-a ctx 写、ws-b ctx 读 miss、ws-a 再读 hit）；TestLegacyEntryNoWorkspace_FallsIntoDefaultBucket（LoadFromJSON 旧 JSON→ ctx background(default) 可见、ws-a 不可见）；TestSystemWorkspace_NormalizesToDefault
- `internal/agent/task_planner_impl_part2_test.go`：TestQueryMemory_CrossTenant_NoHit（orchCache 预置他租户同文本条目 → queryMemory nil=跨租户同文本直命中消除）

**风险**：internal/service 当前被外部 WIP 卡编译，spirit_team_assembler.go 的一行签名适配需与其恢复同批验证（biz/agent/data 包独立可测不受影响）；写读 ctx 来源不一致（如 learn 在后台 ctx default、前台租户读 ws-a）会表现为 miss——方向 fail-closed 不产生泄漏，配方在正确 ctx 下首单即重新学习。

---

**遗留问题**：
- AllocateExplicit 对 LLM/用户显式点名的 agent_keys 不做指纹拦截（语义=显式意图优先），仅对 MemoryHit 来源 keys 经 planner 闸过滤——是否认可此边界？
- learnRecipeFailure 把合成 infra 错误也计为配方降权信号（×0.8、可恢复），还是只限“合成成功但 DQ<0.5”降权？默认实现前者，如需收敛为一行开关请确认。
- workspace 隔离上线后，历史单 JSON 条目统一归 default 桶（对非 default 租户一次性不可见），是否需要针对已知租户做人工归属迁移脚本？默认不做（无从推断归属、方向 fail-closed）。
- 历史“其他”域配方依赖 30d TTL 自然出清（读侧已封禁不再服役），是否需要随 20261287 迁移顺手 DELETE 清理？默认不做主动数据删除。
- RecipeKeysReusable 收紧会导致 playbook 任务对全部旧配方 keys 的一次性降级（topology/specialties 仍复用）——是否需要补一条向 65/B.10.21 三件套的文档同步任务单？

## 3.8 集群 D8：组队归一化时序与 AutoStart 死路径：

> 逐条核实指控：D8-6 成立——`spirit_assembly.go:94-99` 归一化先于 `ResolveGraphTemplateRoute`，单成员 + dispatch/adaptive 模板被翻回 coordinator 并授 RoleSynthesizer（零工具调用假成功）；修复采用时序法：路由先行、归一化殿后（保留 review_loop 单成员响亮失败、`graph_template_route.go` 纯映射不动），并在 `spirit_assembly_part2.go` 的合成者赋予处加 `len>1` 守卫与 parallel 末成员规则对齐。D8-P3 成立——`spirit_team_assembler.go:80-93` AutoStart 死路径零生产调用方（唯一调用方恒传 false 且有竞态顾虑注释）、绕过 BuildTeamTurnInput；推荐删除（含同类死方法 FindTeamSessionAndStartTurn），不接线，接线需重构 orchestrator pending-map 注册时序、超出最小手术；保留 AutoStart 字段与 teamStarter（CancelTeam 在用）。两修复均不碰 wire_gen.go/chat_wire.go；biz 测试即跑、service 测试按稳定导出 API 设计随 WIP 恢复通过。

**不变量承诺**：org-invariants：§4「生产建团唯一路径：PlanExecutor + RealTeamOrchestrator」与交叉手册 §1.21 ADR-2（2026-08-20 死路径整删先例）共同支撑 D8-P3 的删除决策；两修复均不触碰编制表/花名册/专项工具面（ToolsProfile/Allow/Deny 未动），任务路径不创建组织实体。AS-EVT-01：删除绕过 BuildTeamTurnInput 的启动路径，确保 DAG 首轮输入恒走「上游前缀 + Fix 2b 强制交付协议」唯一组装点，保住 Fix-1 零交付门（Critical 级成败语义）与 F10 quorum 收敛的证据链；Critical 事件（ToolResult/Error/RunnerCompletion/Checkpoint）路径不变、无新增发布点。AS-FSM-01：Team 状态机不增状态、不改迁移表——修复反而强化不变量「初始恒 pending、pending→running 唯一迁移点=StartTeamTurn」（spirit_assembly.go:128-131 原注释语义），消除一条绕过迁移点的隐式路径。AS-COG-01：净效果为减分支（-2 死路径），无 struct 字段膨胀（teamStarter 保留因 CancelTeam 在用）。AS-STA-01：tools.SpiritTeamAssemblerPort（Stability:evolving）方法集不变；biz.SpiritTeamParams.AutoStart 字段保留为占位语义，不删字段、不动既有 6 处测试赋值。DOC-SYNC 豁免判断：两修复均为缺陷修复与死路径清理，不改变对外的 RPC/Schema/事件形状；D8-P3 删除方法不在任何 port 接口内，建议 PR 描述注明「与 ADR-2 同类的死路径清理」；65 手册 §1.21 条目可在下一文档维护批次补记一笔（非红线阻断项）。

### D8-6 — 归一化移到 ResolveGraphTemplateRoute 之后 + builder 单成员禁授 synthesizer

**修法**

指控核实：`internal/biz/spirit_assembly.go:94-99` —— 2026-07-28 单成员归一化（`if mode == TeamModeCoordinator && len(agentIDs) <= 1 { mode = TeamModeSequential }`）在 `ResolveGraphTemplateRoute`（`graph_template_route.go:19-38`）之前执行；builtin `dispatch`/`adaptive`（`normalizeBuiltinGraphTemplate` 归入 dispatch 族）把 mode 重置为 `coordinator`，单成员团队在 `buildSpiritTeamDefinitionJSON`（`spirit_assembly_part2.go:22-24`）被授 RoleSynthesizer（提示词禁止执行成员工作 → 零工具调用假成功）。多成员路径不受影响。复核 `template_registry.go:62-64`：`dispatchTemplate.BuildEdges` 对 len(ids)<2 已退化为 pipeline（start→member→end，无边），即单节点 sequential 与 dispatch 边集相同——修复不动该断言，只纠正 mode 标签与角色赋予。

方案：选【时序法】——路由先行、归一化殿后（放弃“路由对 ≤1 成员放弃覆盖”备选）。理由：
1) `ResolveGraphTemplateRoute` 是纯 id→mode 映射，route 单测（`graph_template_route_test.go` 3 例）与 `CompileTemplateIDPreferringDefinition`（`team/graph_compile.go:374-380`）消费方零改动；归一化=团队形状关注，落在组装点单一职责；
2) 备选法改调用点条件 `&& len(agentIDs) > 1`，会把 parallel_review/review_loop 的单成员覆盖也一并豁免——review_loop 单成员今日在 `validateTeamDefinition`（`team_usecase.go:284-288` critic_loop 要求 generator+critic）响亮失败，豁免则静默退化为 sequential，掩盖剧本配置错误；时序法保留该响亮失败；
3) 时序法只搬动一个既有代码块（+注释），外科范围最小。

具体改动：
- `spirit_assembly.go`：把归一化块移到 route 覆盖之后，作用于终态 mode；注释更新为「终态 mode 不变量：单成员团队无协调对象，builtin 模板对 ≤1 成员的 coordinator 覆盖同样被归一化」；`graph_template_id` 仍经 `ApplyAssembleOrgFaces`（spirit_assembly.go:110）入 defJSON 作来源凭证（snapshot TemplateId=dispatch 不变）。
- `spirit_assembly_part2.go:22-24`：coordinator 分支加 `&& len(agentKeys) > 1`，与 25-27 行 parallel 末成员规则对齐（防御纵深：任何不经 AssembleTeam 归一化的调用方也不会给单成员授 synthesizer）。
不改：`graph_template_route.go`、`template_registry.go`、`team/graph_compile.go`。

**行为契约**

不变量 I1：`len(agentIDs) <= 1` ⇒ 终态 `def.mode == Team.Topology ≠ coordinator`（dispatch/adaptive/coordinator 输入或模板覆盖一律归一为 sequential；sequential/parallel 终态保持单成员可运行 pipeline）。成员身份：单成员恒 RoleWorker；coordinator 模式 RoleSynthesizer 仅在成员数 ≥2 时存在（与 parallel 末成员合成者同规则）。多成员 + dispatch 模板：mode=coordinator、members[0]=synthesizer 不变。review_loop 单成员：维持 `validateTeamDefinition` 响亮失败（不静默退化）。graph_template_id 仅来源凭证：snapshot `TemplateId=dispatch` 保留，单节点边集仍走 pipeline 退化（既有行为）。DAG 队 `enable_state_deliverable==true` 与严格交付协议语义不受归一化影响（`requireDeliverable = DagNodeID != ""`）。缓解项保留：`spirit_team_handle_result.go:81` 的 Fix-1 零交付门不做任何改动，继续作为 fail-closed 兜底；F10 quorum/salvage 流程不动。

**改动文件**

- `internal/biz/spirit_assembly.go` — AssembleTeam 内 94-99 行重排：ResolveGraphTemplateRoute 覆盖块上移、单成员归一化块下移并更新注释；条件不变（mode==TeamModeCoordinator && len(agentIDs)<=1），作用于终态 mode
- `internal/biz/spirit_assembly_part2.go` — buildSpiritTeamDefinitionJSON 角色赋予：coordinator 分支改为 `mode == TeamModeCoordinator && i == 0 && len(agentKeys) > 1`，附注与 parallel 末成员规则同源（合成者前提=存在其他成员产出）
- `internal/biz/spirit_assembly_normalize_test.go` — 新增 TDD 测试文件（package biz，复用 newMemSpiritTeamRepo/newMemSpiritSessionRepo/memSpiritTransactor/memSpiritAgentResolver 既有 fixtures）

**测试设计（TDD：先红后绿）**

- `internal/biz/spirit_assembly_normalize_test.go`：① TestAssembleTeam_SingleMember_DispatchTemplate_NormalizesToSequential：Mode=coordinator + AgentKeys=[agent-1] + GraphTemplateID=dispatch ⇒ def.mode==sequential、members[0].role==worker、Team.Topology==sequential（pre-fix RED：终态 coordinator+synthesizer）；② TestAssembleTeam_SingleMember_SequentialMode_PlusDispatchTemplate_Normalizes：Mode=sequential + dispatch ⇒ 同上（pre-fix RED：route 曾把 sequential 翻回 coordinator）；③ TestAssembleTeam_SingleMember_EmptyMode_PlusAdaptiveTemplate_Normalizes：Mode=""（默认 coordinator）+ GraphTemplateID=adaptive ⇒ sequential+worker（pre-fix RED，adaptive 归 dispatch 族）；④ TestAssembleTeam_MultiMember_DispatchTemplate_KeepsCoordinator：Mode=coordinator + 2 成员 + dispatch ⇒ coordinator+synthesizer[0]（pre/post GREEN 回归锁）；⑤ TestAssembleTeam_SingleMember_ReviewLoop_FailsLoudly：review_loop + 1 成员 ⇒ AssembleTeam 返回校验错误（pre/post GREEN，锁定策略差异）；⑥ TestBuildSpiritTeamDefinitionJSON_CoordinatorSingleMember_NeverSynthesizer：直调 builder（coordinator, 1 member）⇒ role==worker（pre-fix RED）；⑦ TestAssembleTeam_DAGSingleMember_DispatchTemplate_KeepsDeliverableChannel：DagNodeID!=“” + dispatch ⇒ enable_state_deliverable==true 且 mode==sequential（通道不变量 + 归一化并存）

**风险**：1) playbook 若故意对单成员 stage 用 dispatch 期望 coordinator 拓扑标签——其边集本就退化 pipeline，行为不变仅标签/角色变化；2) service 侧 `publishSpiritTeamAssembled` 事件 mode 用 params.Mode（未归一化原值），与 Team.Topology 的标签不一致为预先存在现象（pre-fix 已如此，当 step.Mode=coordinator 且单成员时），列入 open questions 不在本修；3) `CompileTemplateIDPreferringDefinition` 对单成员 dispatch 仍报 TemplateId=dispatch（凭证语义）与 def.mode=sequential 的标签差异保持现状并文档化。biz 包测试可在当前 WIP 状态下直接运行（约束4）。

---
### D8-P3 — 删除 AutoStart 死路径（不接线），启动唯一入口归位 RealTeamOrchestrator

**修法**

指控核实：`internal/service/spirit_team_assembler.go:80-93` AutoStart 分支以裸 `params.TaskDescription` 调 `teamStarter.StartTeamTurn`，绕过 `BuildTeamTurnInput`（`biz/spirit_delivery_upstream.go:193-204`：materializeUpstreamInbox + 上游前缀 + Fix 2b 强制交付协议后缀）。【删除】决策，理由四点：
1) 唯一生产调用方 `RealTeamOrchestrator.Orchestrate`（`team_orchestrator_real.go:112-129`）恒传 `AutoStart:false`，且注释明确原因：completion channel 必须先于 StartTeamTurn 注册进 pending map（`team_orchestrator_real.go:99-100`）——把启动挪回 AssembleTeam 内部会复活“快团队完成事件早于 LoadOrStore”的竞态；正确接线必须重构 orchestrator 注册时序，超出最小手术范围；
2) org-invariants §4 + 交叉手册 §1.21（65-module-cross-reference-full.md:783）：「生产建团唯一路径：PlanExecutor + RealTeamOrchestrator」，且 ADR-2（2026-08-20）已整删 `TaskOrchestratorPort.Orchestrate` 死路径——AutoStart=true 的历史调用方（旧 Path A，1-chat.design.md:4385 记载）正是该次删除的孤儿；本次为同类清理，有项目先例；
3) 现状分支缺 BuildTeamTurnInput：任何未来误用会让 DAG 队首轮输入不符合 Fix 2b 契约，叠 Fix-1 门直接误判 failed——删除优于留陷阱；
4) grep 全仓：`SpiritTeamParams{` 构造点仅 `team_orchestrator_real.go:112` 一处 + biz 测试；同文件的 `FindTeamSessionAndStartTurn`（104-121）亦零调用方、同一绕过缺陷（全仓 grep 验证，不在任何 port 接口内），同缺陷类一并清理。

保留：`SpiritTeamParams.AutoStart` 字段（biz 语义“初始状态恒 pending” + T1.1 测试继续使用）；`teamStarter` 字段与 `NewSpiritTeamAssembler` 签名（CancelTeam:214-216 经 HandleTeamTurnResult 仍用）；biz 层 `spirit_assembly.go:128-131` 状态机注释不动。

**行为契约**

组装≠启动：`SpiritTeamAssembler.AssembleTeam` 永不调用 StartTeamTurn（AutoStart=true 亦然）；团队初始状态恒 pending；pending→running 唯一迁移点 = StartTeamTurn，且首轮输入唯一组装点 = `SpiritDelivery.BuildTeamTurnInput`，仅经 RealTeamOrchestrator.Orchestrate（team_orchestrator_real.go:163-164）与 lazy DAG 激活 `scheduleDependentTeams`（biz/spirit_orchestration_schedule.go:91）进入。`AutoStart` 字段语义降级为“文档化占位：初始状态恒 pending，启动由编排调用方负责”。事件面：publishSpiritTeamAssembled 序列不变（AutoStart 分支本不增发事件）；TSM pending 状态不变。删除后 `strings`/`safego` 未用 import 按实清理（`time` 仍被 CancelTeam 使用，保留）。

**改动文件**

- `internal/service/spirit_team_assembler.go` — 删除 AssembleTeam 中 80-93 行 AutoStart 分支（含 safego.Go 块）与 104-121 行 FindTeamSessionAndStartTurn；方法头注释明确「组装不启动 Turn；唯一启动入口 RealTeamOrchestrator.Orchestrate → StartTeamTurn（完整 BuildTeamTurnInput）」；清理失效 import
- `internal/service/spirit_team_assembler_autostart_test.go` — 新增 TDD 回归守卫测试文件（package service，复用既有 stub 模式）

**测试设计（TDD：先红后绿）**

- `internal/service/spirit_team_assembler_autostart_test.go`：TestAssembleTeam_AutoStart_DoesNotStartTurn：以 biz 导出窄接口 stub（biz.SpiritTeamAssembler/SpiritSessionAccessor/SpiritAgentResolver/SpiritTransactor 各一）构造真实 biz.NewSpiritTeamUsecase，spy biz.TeamStarterPort（原子计数 StartTeamTurn），eventBus/seq/orchCache 传 nil（v2EventReady=false 走 early-return 安全路径）；调用 a.AssembleTeam(AutoStart=true, TaskDescription 非空, AgentKeys=[agent-1], session Get 返回 spirit-1)；断言 500ms 有界轮询窗口内 StartTeamTurn 调用数==0。pre-fix RED（当前分支 safego.Go 近乎立即置 1），删除后恒 0 GREEN

**风险**：1) 删除行为对生产零影响（零调用方实证），但若存在仓库外依赖 service 自动启动历史的调用方（如旧脚本），其团队将只创建不启动——由生产唯一路径约束公开背书，风险可接受；2) 若评审认为 FindTeamSessionAndStartTurn 删除超出指控范围，可降级为仅删 80-93 + 对 FindTeamSessionAndStartTurn 加 deprecate 注记，但建议同 PR 清理（同文件同缺陷类，且有 ADR-2 先例）；3) internal/service 测试包当前被外部 WIP 卡住编译——新测试仅引用稳定导出 API（biz.NewSpiritTeamUsecase 窄接口 + NewSpiritTeamAssembler 现有签名），WIP 恢复后应直接通过；4) safego/strings import 移除若手滑会影响编译，依赖编译器兜底。

---

**遗留问题**：
- service 侧 publishSpiritTeamAssembled 事件 mode 参数用 params.Mode（未归一化原值）：单成员+coordinator 时事件拓扑标签与 Team.Topology 不一致（pre-fix 即存在）。是否另起小修改为传 result.Team.Topology？本集群不动。
- graph_template_id 对 ≤1 成员队继续写入 defJSON，snapshot TemplateId=dispatch 与实际 pipeline 边集存在标签/拓扑差异。是否让 CompileTemplateIDPreferringDefinition 对单成员也放弃 builtin 模板名（需传入成员数）？当前保持凭证语义不变。
- SpiritTeamParams.AutoStart 字段本体是否最终删除：涉及 biz 层 6+ 处测试赋值与字段语义迁移，纯清理且无行为收益，建议留待公共清理批次，不在本次外科范围。
- SkillInstallAssertionGate（spirit_assembly.go:105-107）与单成员归一化无交集，但该 gate 依赖任务关键词匹配——若 planner 对单成员 install 任务复选 dispatch 模板，修复后 gate 仍正常随 def 生效，无需联动，请评审确认无其他模板×gate 组合失效场景。

## 3.9 集群 D9 前端事件接入层修复：

> 五条外科修复全部落在 web 前端、零 wire 改动：D9-19 在 useChatRunStatus 建 runId 绑定+终态粘性+REST updatedAt 单调的接受谓词（WS 与 REST 对账统一仲裁：同 run 终态恒赢，跨 run 仅在上 run 终态后放行），并在 useChatSender 增加 markSendingDone 后非终态孤儿再对账入口，修复迟到 running 覆写终态→停止键复现/Enter 误路由/错误态残留；D9-20 新增 patchMemberSessionStatusByChatSession（终态汇点、不伪造 Version），复活被版本守卫吞掉的 pause/resume 迟到补丁；D9-21 为 GraphNode 引入 live/snapshot 双源生命周期秩守卫（snapshot 秩单调不降、completed 全源汇点、resume 复位经 live 放行）；D9-22 router 对未知 kind/非 v2_event 加 60s 节流 console.warn；D9-T 补 TeamRunStatus 'partial_failure'（消费面已核查安全）。每条附 TDD 失败先行规格（vitest，复用既有 spec 骨架），绕开内部 service WIP 影响面。

**不变量承诺**：AS-EVT-01：终态 run_status 属 Critical 车道（outbox+Seq+重放，event_system.go:22-26），本方案不削弱该契约——客户端仲裁只在有证据（runId 绑定/终态粘性/服务端时间）时丢弃「可证过期」帧，REST 对账仍就位为安全网且恒能接受新鲜终态；WS Meta 不带服务端时间属上游合约缺口，已在 open_questions 提出 ADR 候选。AS-FSM-01：原本隐式的客户端状态机被显式化并测试化——run_status 接受表（terminal 汇点/跨 run 门控）、GraphNode 生命周期秩（pending<running<terminal、completed 全源汇点）、MemberSession 补丁终态汇点，均与后端 machines（plan_executor_resume 只复活 failed/cascade-skipped、chat_pause 已终态跳过）同口径。org-invariants：不触碰编制表/编排/专项链，纯前端事件接入层修复。日志红线：仅用 console.warn（项目允许 warn/info、禁 console.log）。任务约束：未依赖/修改 wire.go、wire_part2.go、workers.go、biz/knowledge/*、data/knowledge_*、internal/knowledge/*、internal/service/knowledge_asof_test.go、api.ts、pnpm-lock.yaml 中任何文件；全部测试为 vitest 纯前端，与 internal/service Go 包 WIP 编译卡死无交集。

### D9-19 — run_status 新鲜度守卫（runId 绑定 + 终态粘性 + REST 更新时刻单调）与孤儿态自愈对账

**修法**

在 useChatRunStatus.ts 引入接受谓词与新鲜度追踪器（lastRunId/lastStatus/lastServerMs），统一裁决 WS 帧与 REST 对账响应：R1 终态粘性（runId 相同且已接受 terminal 时拒绝非终态回落，idle 除外）、R2 runId 绑定（跨 runId 仅在上个 run 已 terminal/idle/未知时接受）、R3 REST 服务端 updatedAt 单调（同 runId 且双方带服务端时间时拒绝旧快照）。谓词同时接入 applyRunStatusFields（WS 入口）、refreshRunStatusForced 与 hydrateFromHttpIfNeeded（REST 入口）；forceSetRunStatus 同步重置追踪器。另在 useChatSender.ts 增加孤儿再对账入口：markSendingDone 时若 runStatus 仍非终态，延迟 CHAT_RUN_ORPHAN_RECONCILE_DELAY_MS 单次触发已有 reconcile 通道（复用 watchdogReconciling 重入锁，不发恢复通知），修复 terminal WS、eWS 帧丢失而 task.completed 已停表后的无限期残留。

**行为契约**

run_status 接受表（lastStatus→candidate）：terminal(completed|failed|cancelled) + 同 runId → 仅接受 terminal/idle；terminal + 异 runId → 接受（新 run）；non-terminal + 异 runId → 丢弃；non-terminal + 同 runId → 接受，但若双方有服务端时间且 candidate 更旧 → 丢弃；runId 为空的 legacy 帧按当前 runId 处理且不回写 lastRunId。仲裁结论：同 run 内终端 WS 帧恒赢迟到 REST running；REST 晚到 terminal 帧永远不被拒（终态、同 run、或上 run 已终态的新 run 均可接受）。自愈：markSendingDone 后 runStatus 非终态 ⇒ 恰好一次 orphan REST 对账（guard 保证只接受真新鲜状态）；sending=true 期间由既有 90s watchdog 负责，不叠加。

**改动文件**

- `F:\myproject\aranea-agents\web\src\features\chat\composables\useChatRunStatus.ts` — 新增模块级 TERMINAL_RUN_STATUSES 集合与 shouldAcceptRunStatus 谓词；applyRunStatusFields/hydrateFromHttpIfNeeded/refreshRunStatusForced 三个入口统一过谓词；被丢弃时 refreshRunStatusForced 返回 null（延续 REG-2 null 语义）；forceSetRunStatus 重置 lastRunId/lastStatus；resetRunStatus 清空全部追踪器。
- `F:\myproject\aranea-agents\web\src\features\chat\composables\useChatSender.ts` — markSendingDone 末尾：若 deps.runStatus.value 非 {idle,completed,failed,cancelled} 则 setTimeout(or orphan reconcile, CHAT_RUN_ORPHAN_RECONCILE_DELAY_MS)；markSending/组件销毁路径清理该定时器；orphan 路径与 watchdog 共用 watchdogReconciling 锁，成功对账后不发「已同步最新运行状态」通知。
- `F:\myproject\aranea-agents\web\src\features\constants\timeouts.ts` — 新增 CHAT_RUN_ORPHAN_RECONCILE_DELAY_MS = 10_000（watchdog 90s 之外的短自愈节拍）。

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\web\src\features\chat\composables\__tests__\useChatRunStatusForced.spec.ts`：① WS terminal(run1,completed) 后迟到 REST running(run1,T_old) → 返回 null、runStatus 保持 completed、applyAwaitRunStatus 不二次调用；② WS running(run1) 后 REST completed(run1) → 接受并返回 completed；③ REST running(T2) 后 REST running(T1<T2) → 第二次返回 null；④ completed(run1) 后 running(run2) → 接受；⑤ running(run1) 后 running(run2) → 丢弃；⑥ completed 后 idle 接受（reset 语义）；⑦ WS completed 后 WS running（回放/乱序兜底）→ 丢弃；⑧ forceSetRunStatus('idle') 后旧 runId 的 terminal 帧不复活旧状态。
- `F:\myproject\aranea-agents\web\src\features\chat\composables\__tests__\useChatSenderWatchdog.spec.ts`：⑨ markSending→markSendingDone 且 runStatus='running' → 推进 10s → reconcileRunStatus 恰好调用一次（返回 completed 时由桩更新 runStatus，再次推进不再触发、无恢复通知）；⑩ runStatus 已 terminal 时 markSendingDone → 定时器不注册、reconcile 不调用；⑪ orphan 定时器挂起时再 markSending → 旧定时器清除、窗口重计；⑫ orphan 对账期间 watchdog 对账被拒入（watchdogReconciling 复用）。

**风险**：R2 的保守丢弃在「终态帧+running 帧全丢且换 run」组合下会多丢一次 REST——由 orphan 对账与下次切会话/reconnect hydrate 兜回，不会死锁；runMeta.updatedAt 自此带真实服务端时间（WS 帧仍为空串，与现状一致），无新 UI 依赖面。

---
### D9-20 — MemberSession pause/resume 迟到补丁走专用状态补丁方法（终态汇点、免伪造 Version）

**修法**

在 activityV2Store.ts 新增 patchMemberSessionStatusByChatSession(chatSessionId, status)：按 ex.SessionID 匹配成员，ex.Status 属终态集合 {completed,failed,skipped} 或相等时直接 no-op；否则仅覆盖 Status、保持 ex.Version 不伪造版本（避免越过后端 sentinel 版本带纪律 chat_pause.go:136-140，后续权威 member_session.updated 的 Version++/哨兵带仍能通过既有守卫改写）。useChatV2EventHandlers.ts handleV2RunStatus 中的循环+upsertMemberSession 调用替换为该方法，复活被死代码吞掉的迟到 paused/running 补丁。

**行为契约**

补丁仅作用于 'paused'|'running' 候选；MemberSession 终态（completed/failed/skipped）为汇点不可被补丁回退（与后端「已终态记录直接跳过」同口径）；Version 永不被本路径修改；补丁与后续权威事件的仲裁完全复用既有 Version 守卫（等同版本无特权、更高版本恒赢）。

**改动文件**

- `F:\myproject\aranea-agents\web\src\stores\chat\activityV2Store.ts` — 新增 patchMemberSessionStatusByChatSession 并在 store return 中导出；upsertMemberSession 既有逻辑不改。
- `F:\myproject\aranea-agents\web\src\features\chat\composables\useChatV2EventHandlers.ts` — handleV2RunStatus 内 245-257 行的 memberSessions 遍历 + upsertMemberSession 替换为一次 patchMemberSessionStatusByChatSession 调用（保持 chatSessionId 提取与 status 过滤条件不变）。

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\web\src\stores\chat\__tests__\activityV2Store.memberSessionPatch.spec.ts`：① running→paused 补丁生效且 Version 不变；② paused→running 补丁生效；③ ex 为 completed/failed/skipped 时补丁被汇点拒绝；④ SessionID 不存在 / status 相同 → no-op；⑤ 补丁后再 upsertMemberSession 携带更高 Version 的 completed → 权威终态覆盖补丁（验证仲裁复用既有守卫）。

**风险**：补丁绕开 Version 守卫是故意的受控例外；仅限 paused/running 两个生命周期态且有终态汇点保护，无法复活终态成员卡。

---
### D9-21 — GraphNode 终态粘性守卫：live/snapshot 双源生命周期秩仲裁

**修法**

给 upsertGraphNode/upsertGraphStage 增加可选 source 参数（默认 'live'，保持 router 与既有测试调用零改动）：生命周期秩 rank(pending)=0 < rank(running)=1 < rank(terminal: completed|failed|interrupted)=2。snapshot 来源仅允许 rank 单调不降（Status 回退时保留 ex.Status，但元数据 TeamStageID/DependsOn/Label/DagNodeID 照常合并，等价秩间终态细调允许）；live 来源放行全部后端合法转移（含 resume 的 failed→pending、interrupted→pending），仅防御性拒绝 completed→非 completed（后端 machines 中 completed 为真汇点，见 plan_executor_resume.go:80-101 只复活 failed 与 cascade-skipped）。hydrateTask 两个 graph 列表循环显式传 'snapshot'；upsertGraphStage 内嵌 Nodes 以同一 source 透传走同一合并函数，消除直写盲区。

**行为契约**

不变量：live 单信道按 Sequencer 序可信（WS 帧无重排）；snapshot 不得让任何节点秩回退；completed 对一切来源为汇点；terminal graph_stage 事件不带 Nodes 不再构成缺口，因为回退在合并点被截停；resume 复位路径经 live 帧放行，水合快照若恰好滞后于 resume 位点，冲突由「下一帧 live 事件前向治愈」保证（pending→running→completed 均为前向）。

**改动文件**

- `F:\myproject\aranea-agents\web\src\stores\chat\activityV2Store.ts` — upsertGraphNode/upsertGraphStage 增加 source?: 'live'|'snapshot'（默认 'live'）；抽出共享 mergeGraphNode(ex, gn, source) 守卫；hydrateTask 中 listGraphStagesV2/listGraphNodesV2 两处 upsert 传 'snapshot'；fetchSessionHistory 不动（不触 graph）。

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\web\src\stores\chat\__tests__\activityV2Store.graphNode.spec.ts`：① live pending→running→completed 全部生效；② snapshot completed→running：Status 保持 completed 但 TeamStageID/Label 合并更新；③ snapshot running→pending 被拒；④ snapshot pending→running、running→completed 前向放行；⑤ live resume failed→pending→running 放行；⑥ live completed→running 被汇点防御拒；⑦ upsertGraphStage(source='snapshot') 内嵌 Nodes 走同一粘性守卫、source='live'（router 路径）直放行；⑧ 同秩终态调整（failed→interrupted）经 snapshot 允许。

**风险**：resume 复位与首屏水合同窗竞态时，snapshot 的 pending 可能被 live 已知的 failed 挡下（表现为短暂停留 failed），由下一 live dispatch 帧前向治愈；属可接受的瞬态而非永久错位。既有测试单参调用经默认 'live' 全兼容。

---
### D9-22 — 未知 v2 kind / 非 v2_event 帧的节流 console.warn 可观测性

**修法**

在 useChatEventRouter.ts 内部加 60s 逐键节流 warn（模块内 lastWarnAt Map + WARN_INTERVAL_MS 常量）：dispatch 收到非 v2_event 帧 console.warn('[chat] ws frame dropped: non-v2_event', {type, kind})；handleKind default 分支 console.warn('[chat] unknown v2 event kind', kind)。键 = `${type}:${kind}` / kind，节流防止异常帧洪泛刷屏；不改任何分发语义（仍忽略+前向兼容）。

**行为契约**

未知 kind 与非 v2_event 包均产生恰好每 60s 一条 warn（dev/test 可见、同键去抖）；已知 kind 永不触发 warn；无任何行为/状态变更——纯可观测性增强。

**改动文件**

- `F:\myproject\aranea-agents\web\src\features\chat\composables\useChatEventRouter.ts` — 新增模块级节流表与 warnDroppedFrame 辅助函数；dispatch 非 v2_event 早退前打点；switch default 打点；不引入新依赖。

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\web\src\features\chat\composables\__tests__\useChatEventRouter.spec.ts`：① vi.spyOn(console,'warn')：未知 kind 首次 dispatch 产生 1 次含 kind 的 warn；② 60s 窗口内同 kind 重复帧被节流（仍 1 次）；③ fake timers 推进 60s 后同 kind 再次 warn；④ task.created 等已知 kind 全程 0 次 warn；⑤ 非 v2_event type 帧 warn 一次且不进入 store。

**风险**：节流 Map 为模块级单例随页面生命周期存在，键集合有界（kind 枚举 + type）无泄漏风险。

---
### D9-T — TeamRunStatus 联合类型补 'partial_failure'

**修法**

v2Types.ts:19 TeamRunStatus 联合补 'partial_failure'（对齐 internal/biz/team_run.go:38 TeamRunV2StatusPartialFailure 与 PlanStatus/PlanStepStatus 既有口径）。消费面核查结论：v2Api.ts:482 为裸 as 断言直接透传、appStatusMeta.ts:86 已有 partial_failure tone、stores/spirit 走独立 SpiritTeamStatus 联合、无任何 Record<TeamRunStatus,…> 穷尽映射——纯加宽编译安全。

**行为契约**

后端 team_run.* 事件携带 partial_failure 时前端类型不再说谎；渲染 tone 走 appStatusMeta 既有 warning 条目；不改变任何运行时行为。

**改动文件**

- `F:\myproject\aranea-agents\web\src\features\chat\v2Types.ts` — TeamRunStatus 联合追加 'partial_failure'（仅一行类型改动）。

**测试设计（TDD：先红后绿）**

- `F:\myproject\aranea-agents\web\src\features\chat\__tests__\v2Types.teamRunStatus.spec.ts`：① 类型断言用例：const s: TeamRunStatus = 'partial_failure' 编译通过；② v2Api 映射回归：listTeamRunsV2 DTO status='partial_failure' 透传为 TeamRun.Status='partial_failure'（不经 'running' 兜底）。

**风险**：无运行时风险；若后续新增 TeamRunStatus 穷尽 switch 需在编译期补分支。

---

**遗留问题**：
-  WS run_status Meta 无服务端 updated_at，跨 runId 仲裁只能用「上 run 已终态」作门控；是否在后端 publishRunStatus Meta 加 updated_at（internal/service/run_status_publish.go、chat_pause.go）让客户端升级为纯服务端时间仲裁——建议走 ADR（AS-ADR-01）+另行小手术，不进本集群。
- orphan reconcile 延迟取 10s 为经验值（介于 60s 停滞提醒与 90s watchdog 之间）；是否需要用户可见的「正在核对运行状态」轻提示由产品定，目前按静默自愈实现。
- onReconnectHydrate（useChatWorkspace.ts:297）是否追加一次 refreshRunStatusForced 作为重连附加自愈——为控制爆炸半径未纳入，可作为 follow-up。
- D9-20 的 handler 级集成用例（handleV2RunStatus→patch）未建独立 spec（useChatV2EventHandlers 无既有测试骨架、需重 mock 多个 pinia store）；以 store 层契约测试等价覆盖，若要补 handler 骨架请评审拍板。

## 3.10 集群 D10：前端展示层状态映射修复（发现#23/#24/#25 + P7/P8）：

> D10 五条修复全部落在前端展示层、纯增量映射补全，零后端/wire 改动。F1(#23 HIGH)：spiritMemberStatusToLabel 重写为 v2 词表（pending/running/paused/completed/failed/skipped）主键+legacy 别名层，TeamProgressCard 的 waitingForPredecessor 改认 pending，复用既有 STATUS_LABEL_CONFIG 聚合标签与 chip/CSS 键（零新增）。F2(#24 MEDIUM):AgentSidebarCard displayStatus 扩为 9 值 union 并文档化 6 级优先级（阻塞>成员终态>团队终态回填>成员活动态>团队活动态>pending),cancelled/interrupted/partial_failure 独立视觉与文案，重试按钮因不再并入 failed 而自然只对真 failed 出现，补 4 个 i18n 键与 Sass 类。F3(#25 LOW):ObserveNodeDetail MEMBER_STATUS_KEY_MAP 补 paused/cancelled,ObserveNodeDetail/ObserveNode 成员点与状态文本补 --paused(warning)/--cancelled(tertiary),observe.statusPaused/statusCancelled 双语键。F4(P7)：加载文案清除列表追加 spirit_team_interrupted/cancelled 二字面量。F5(P8):PULSE_COLOR/DURATION_MAP 补 partial_failure/cancelled/paused/pending 四键。每条均配 TDD 失败先行用例：spiritUi.spec.ts 扩枚举、新建 AgentSidebarCard/TeamProgressCard/ObserveNodeDetail 组件 mount 测（仿 MemberSessionPanel.spec.ts stub 模式）、扩 useContextualLoadingMessage/useStatusPulse 两现有 spec。全部测试为 web vitest，不受 internal/service WIP 编译阻塞影响；禁碰文件（chat/api.ts、pnpm-lock、wire 系、knowledge 系）均不在改动清单。

**不变量承诺**：(1) org-invariants 完全无涉：本集群纯前端展示层映射修复，不改编排、编制表、专项工具面、花名册或 Agent 热路径（org-invariants.md §1-§5 均不触发）。(2) AS-EVT-01：五条 fix 均为"已送达事件的展示映射补全"，不新增/降级任何事件的可靠性等级（Critical/Important/Informational 分类不动）；F4 的清除语义反而修正了 Informational 加载文案的边界残留，与分级一致。(3) AS-FSM-01（>3 状态实体须显式状态机）：MemberSessionStatus 六值状态机（biz/member_session.go:28-35）此前在展示层只有隐式零散映射；F1/F2/F3 把成员级、团队级、观测级三张映射表各自收敛为单一权威 Record 常量 + F2 的 displayStatus 六级优先级契约（成员终态优先于团队回填），实质上是把展示状态机显式化，规格中作为 behavior_contract 固化并有全枚举测试防新增状态漏配（spiritUi.spec.ts 既有 :40-46 防漏配模式延伸）。F5 同理让 PULSE 键与 SpiritTeamStatus 九转移态一一对应。(4) 红线：仅 console.warn/info——本条目不新增日志；未知状态兜底路径沿用现有静默回退风格，不引入 console.log。(5) 禁碰约束达成：web/src/features/chat/api.ts 与 web/pnpm-lock.yaml 零接触；cmd/admin/* 与 Go 侧零接触（wire_impact 全 none);internal/service WIP 编译破裂与前端 vitest 无涉；doc comment(:139）及 TeamStage Members 写入口径（stores/spirit/index.ts:971-1048,updateMemberFromV2MemberSession:1058+）仅阅读未改。(6) i18n 双语 parity 由 web/scripts/check-i18n.mjs 守护，全部新键 zh-CN/en-US 成对给出。(7) DOC-SYNC:5 条均为纯 bug 修复级映射补全（不改模块行为契约/接口/数据结构），命中豁免条款，无需动三件套；如评审认定 displayStatus 优先级契约值得沉淀，建议在 docs/development/1-chat.design.md 观测章节补一小节（实现阶段可选）。

### D10-F1 — 发现#23(HIGH):spiritMemberStatusToLabel 补 v2 成员词表，waitingForPredecessor 认 pending

**修法**

重写 web/src/features/spirit/spiritUi.ts:139-150 的 spiritMemberStatusToLabel：映射表以 v2 词表（后端 internal/biz/member_session.go:28-35 的 MemberSessionStatus：pending/running/paused/completed/failed/skipped，经 stores/spirit/index.ts:979/1004 upsertTeamFromV2TeamStage 与 updateMemberFromV2MemberSession 原样落入 SpiritMember.status）为主键，legacy 词表（idle/error/waiting/blocked，兼容 API 旧行）作为别名层保留；未知字符串仍兜底 'queued' 保持与 agentNodeStatusToLabel:136 的静默回退风格一致。同步修 TeamProgressCard.vue:132 的 waitingForPredecessor 条件 member.status === 'idle' → （'pending' || 'idle')，v2 下 pending 成员在团队 running 时恢复显示'等待前置任务'注释。doc comment(:139)改为 v2 词表权威描述。不动 STATUS_LABEL_CONFIG(:88-132)——paused→suspended 复用既有'等待中'文案图标、skipped→skipped 复用'已跳过'、failed→failed 复用'失败'，chip/css 键零新增。

**行为契约**

对任意 member.status ∈ v2MemberStatus(6值） ∪ legacy(4值）,spiritMemberStatusToLabel 返回非 'queued' 的确定性聚合标签；团队 running 且成员 pending/idle 时 waitingForPredecessor 提示可见；failed/skipped/paused 成员的 chip 与 TeamProgressCard.vue:107 partialMemberFailure 警告不再自相矛盾；未知空字符串行为不变（queued)。

**改动文件**

- `web/src/features/spirit/spiritUi.ts` — spiritMemberStatusToLabel(:140-150）双词表映射：pending|'idle'→queued,running→active,paused|'waiting'→suspended,blocked→tool_blocked,completed→done,failed|'error'→failed,skipped→skipped；更新函数 JSDoc 为 v2 词表权威
- `web/src/components/spirit/TeamProgressCard.vue` — :132 条件 member.status === 'idle' 改为 member.status === 'pending' || member.status === 'idle'（等待前置任务提示兼容 v2)

**测试设计（TDD：先红后绿）**

- `web/src/features/spirit/__tests__/spiritUi.spec.ts / web/src/components/spirit/__tests__/TeamProgressCard.spec.ts`：(1) v2 六值全枚举：pending→queued/running→active/paused→suspended/completed→done/failed→failed/skipped→skipped，且对 failed/skipped/paused 断言结果 ≠ 'queued'（复现#23 失败先行）;(2) legacy 键回归：idle/error/waiting/blocked 映射不变；(3) 未知字符串→queued 兜底不变；(4) 新组件 spec TeamProgressCard.spec.ts（仿 MemberSessionPanel.spec.ts 的 mount+q-* stub):team running+member failed 渲染'失败'chip 而非'排队中';member pending+team running 显示 waitingForPredecessor 文案行

**风险**：低。只新增映射键，不改已有键的结果；TeamProgressCard/TeamMemberTreeNode 之外的消费方（agentNodeStatusToLabel 链路）不受影响。legacy 键保留是故意的——spirit/api.ts:156 mapSpiritMember 对 REST 旧数据原样透传。

---
### D10-F2 — 发现#24(MEDIUM):AgentSidebarCard displayStatus 终态分支重设计

**修法**

重设计 AgentSidebarCard.vue:117-134 displayStatus computed：union 扩为 'running'|'blocked'|'completed'|'failed'|'pending'|'cancelled'|'interrupted'|'partial_failure'|'skipped'。优先级契约（自上而下）:(1)blockedInfo.blocked→blocked;(2）成员自身终态优先：completed/ok/done→completed、failed/error→failed、skipped→skipped（修成员永久 pending/running 的另一洞）;(3）团队终态回填：completed/archived→completed、failed→failed、partial_failure→partial_failure（新增，修:133 永久待开始）、cancelled→cancelled（新增，脱离:127 的 failed 归因）、interrupted→interrupted（新增）;(4)running/executing→running、blocked/stuck→blocked、paused→blocked（成员暂停=可恢复挂起，复用 resume 按钮语义，emit 'resume' 与既有 blocked 路径同 RPC);(5)ts running/pending→running;(6)else pending。statusText switch 增 4 个 case 用新 i18n 键。重试按钮保持 v-if="displayStatus==='failed'"(:46)——cancelled/interrupted/partial_failure 不再并入 failed 后，重试自然只对真 failed（成员 failed/error 或团队 failed）出现。Sass 增 --cancelled/--interrupted/--partial_failure/--skipped 四组（card/status/status-dot 三个选择器）:cancelled、skipped 用 var(--color-text-tertiary) 中性灰+opacity,interrupted/partial_failure 用 var(--color-warning)。i18n 四键 zh-CN/en-US 同步加（check-i18n.mjs 校验双语 parity)。

**行为契约**

(a) 成员自身终态永远优先于团队状态回填（既有语义保留）;(b) 团队 cancelled→成员卡独立'已取消'灰样式无重试；interrupted→'已中断'橙样式无重试；partial_failure→'部分失败'橙警示无重试（团队级重试由 TeamProgressCard canRetry 承担，职责不重复）;(c) 终态事件缺失时（成员停留 pending)+团队任意终态→卡片显示团队终态而非'待开始';(d）重试按钮出现的充要条件=displayStatus==='failed' 且 failed 仅源自成员 failed/error 或团队 failed;(e)skipped 成员为中性终态不再挂 spinner/暂停键。

**改动文件**

- `web/src/components/chat/AgentSidebarCard.vue` — displayStatus computed(:117-134）按上列 6 级优先级重写 + statusText(:136-149）增 cancelled/interrupted/partial_failure/skipped 四 case + Sass 增 --cancelled/--interrupted/--partial_failure/--skipped（卡片底、status 胶囊、status-dot 三处）
- `web/src/i18n/locales/zh-CN.ts` — chat.agentSidebar 段（:321-334）新增 statusCancelled:'已取消'/statusInterrupted:'已中断'/statusPartialFailure:'部分失败'/statusSkipped:'已跳过'
- `web/src/i18n/locales/en-US.ts` — chat.agentSidebar 段（:328-341）新增 statusCancelled:'Cancelled'/statusInterrupted:'Interrupted'/statusPartialFailure:'Partial failure'/statusSkipped:'Skipped'

**测试设计（TDD：先红后绿）**

- `web/src/components/chat/__tests__/AgentSidebarCard.spec.ts（新建，mount+quasar stub 模式仿 MemberSessionPanel.spec.ts)`：(1) member pending+teamStatus cancelled→文案=statusCancelled、class 含 --cancelled、无 refresh 按钮（复现#24 失败先行）;(2) member pending+teamStatus interrupted→--interrupted 无重试；(3) member pending+teamStatus partial_failure→statusPartialFailure 文案、--partial_failure、无重试（修:133 永久待开始）;(4) member failed+teamStatus cancelled→成员终态优先=failed 且有重试；member error→同；(5) member skipped→statusSkipped 无重试无 pause;(6) member paused→blocked 视样式+resume 按钮；(7) member running→pause 按钮；blockedInfo.blocked=true 覆盖一切→blocked

**风险**：(1) 成员 paused→blocked 会让 resume 按钮出现在暂停成员上，emit 与 blocked 同路径（ResumeSession by agentKey),spirit/api.ts:105 注明该路径本就用于 AgentCard resume，语义吻合但需 review 确认父层 useChatSpiritPanel 的 resume 处理器对非 blockedInfo 成员不短路；(2) displayStatus union 扩大后，模板 :4 的 BEM 类名集合变化，若外部 e2e 断言了旧类名需同步（未发现外层消费 displayStatus 类名）;(3) 不动 blockedInfo 优先权，工具确认阻塞 UX 不变。

---
### D10-F3 — 发现#25(LOW):Observe 视图 paused/cancelled 文案键与成员点样式补全

**修法**

ObserveNodeDetail.vue:(1) MEMBER_STATUS_KEY_MAP(:139-145）补 paused→'observe.statusPaused'、cancelled→'observe.statusCancelled'(MemberSessionStatus 无 cancelled，属防御性补全，与 Finding 要求一致）;(2) Sass .observe-node-detail__member-dot(:260-280）与 .observe-node-detail__member-status(:302-317）各增 &--paused(var(--color-warning)，与 ObserveStatusBadge 中断/暂停族一致）与 &--cancelled(var(--color-text-tertiary))。ObserveNode.vue:.observe-node__member-dot(:282-302）同样补 &--paused/&--cancelled。成员 status 数据源已确认为 v2 词表（useObserveGraph.ts:135 extractMembers 取 MemberSession.Status）,paused 直出英文原文即由此产生。i18n:zh-CN observe 段（:4989-5007）加 statusPaused:'已暂停'/statusCancelled:'已取消';en-US observe 段（:5063起）加 statusPaused:'Paused'/statusCancelled:'Cancelled'。

**行为契约**

members 列表对任意 v2 MemberSessionStatus 既出 i18n 文案又有 dot/status 两个 CSS 类；未知状态仍走 memberStatusLabel(:147-150）的原文透传兜底（不变）；其余状态渲染结果逐字节不变。

**改动文件**

- `web/src/components/chat/observe/ObserveNodeDetail.vue` — MEMBER_STATUS_KEY_MAP(:139）+paused/cancelled 两键；member-dot(:260-280)/member-status(:302-317）各补 &--paused(warning)/&--cancelled(tertiary)
- `web/src/components/chat/observe/ObserveNode.vue` — observe-node__member-dot(:282-302）补 &--paused(warning)/&--cancelled(tertiary)
- `web/src/i18n/locales/zh-CN.ts` — observe 段新增 statusPaused:'已暂停'、statusCancelled:'已取消'
- `web/src/i18n/locales/en-US.ts` — observe 段新增 statusPaused:'Paused'、statusCancelled:'Cancelled'

**测试设计（TDD：先红后绿）**

- `web/src/components/chat/observe/__tests__/ObserveNodeDetail.spec.ts（新建）`：mount ObserveNodeDetail(GraphNode 带 Members:[{status:'paused'}])，断言成员行文本 = t('observe.statusPaused')（失败先行：当前直出'paused'原文），并断言 .observe-node-detail__member-dot--paused 与 .observe-node-detail__member-status--paused 类存在于 DOM；skipped 成员回归仍显示'已跳过';GraphNode 构造自足，useObserveNodeEnrichment 为纯函数可在 jsdom 跑

**风险**：极低。纯增量键；CSS 类名与模板 :35/:41 的动态绑定 `observe-node-detail__member-*--${m.status}` 天然对接，无选择器冲突。

---
### D10-F4 — P7：加载文案对 team interrupted/cancelled 补清除分支

**修法**

activityEventToLoadingType(:46-47）已把 team_stage interrupted/cancelled 映射为 spirit_team_interrupted/spirit_team_cancelled，但 onSpiritActivityEvent 三个消费分支全部漏接：ORCHESTRATION_LOADING_MAP(observabilityConstants.ts:25-50）无匹配项（不设新文案——中断/取消是边界终态，应采用'清除'而非'提示'),:204-212 清除列表无此二 envType,:237-239 notice 路径不涉及。最小修法：在 :204-212 的清除条件数组中追加 'spirit_team_interrupted' 与 'spirit_team_cancelled' 两个字符串字面量。不加 MAP 项、不动 notice 路径、不改 activityEventToLoadingType。

**行为契约**

收到 team_stage interrupted/cancelled ActivityEvent 后 loadingMessage.value===null（陈旧'正在…'清除）;completed/failed/all_completed/butler 终态清除行为不变；replay(isReplaying）抑制不变；interrupted/cancelled 自身不产生新 loading 文案（MAP 无项）。

**改动文件**

- `web/src/features/chat/composables/useContextualLoadingMessage.ts` — :204-212 清除列表追加 'spirit_team_interrupted'、'spirit_team_cancelled'

**测试设计（TDD：先红后绿）**

- `web/src/features/chat/composables/__tests__/useContextualLoadingMessage.spec.ts（扩展现有文件）`：(1) 先注入 plan(stage=plan_created）置 loading 非空，再注入 team_stage interrupted→loadingMessage.value 为 null（失败先行）;(2) 同模式 cancelled→null;(3) 回归：team_stage completed/failed 清除仍有效；interrupted/cancelled 事件自身不设置任何新 loadingMessage

**风险**：极低。仅扩大清除集合；若后端未来对 team interrupt 发 pause-hint 类 notice，走的是 onSpiritNoticeType 独立路径，不受影响。

---
### D10-F5 — P8:useStatusPulse/PULSE_COLOR_MAP 对 partial_failure/cancelled/paused/pending 补脉冲键

**修法**

脉冲触发链已确认：useChatSpiritPanel.ts:84-101 watch 全量 SpiritTeamStatus 转移（含 partial_failure/cancelled/paused/pending)→useStatusPulse.ts:31 onTeamStatusChanged→:34 PULSE_COLOR_MAP[newStatus] 未命中静默 return。只需扩 PULSE_COLOR_MAP/PULSE_DURATION_MAP(observabilityConstants.ts:239-251）四键：partial_failure→var(--color-warning)/2000ms（部分失败终态警示，时长与 failed 对齐）;cancelled→var(--color-text-tertiary)/1500ms（中性终态）;paused→var(--color-warning)/1000ms（瞬时挂起，与 running 同短档）;pending→var(--color-text-tertiary)/800ms（团队首次出现的弱提示；若产品认为 pending 不该脉冲，备选方案是显式在 useStatusPulse 注释声明 pending 有意不设——规格按补全写，评审可降级）。文案：脉冲纯视觉无文案，不新增 i18n 键；部分失败的文字警示已由 TeamProgressCard.vue:103-107 partialMemberFailure 承担，不重复。注：Finding 路径 web/src/features/orchestration/useStatusPulse.ts 有误，实际为 web/src/features/chat/composables/useStatusPulse.ts。

**行为契约**

SpiritTeamStatus 十值中除 archived（纯展示折叠，人为动作）外，其余九值的状态转移均产生确定性脉冲（颜色/时长如上）；未知字符串仍在 :35 静默 return（防御层保留）；同一 teamId 新脉冲替换旧脉冲、duration 到期自动清除等行为不变（既有测试覆盖）。

**改动文件**

- `web/src/features/spirit/observabilityConstants.ts` — PULSE_COLOR_MAP(:239-244)+partial_failure/cancelled/paused/pending 四键；PULSE_DURATION_MAP(:246-251)+2000/1500/1000/800 对应四值；type 注释更新为'覆盖全部 SpiritTeamStatus 转移(archived 除外)'

**测试设计（TDD：先红后绿）**

- `web/src/features/chat/composables/__tests__/useStatusPulse.spec.ts（扩展现有文件）`：(1) onTeamStatusChanged('t1','partial_failure')→isPulsing=true、pulseColor=var(--color-warning)、2000ms 后自动清除（失败先行）;(2) cancelled→tertiary 1500;(3) paused→warning 1000;(4) pending→tertiary 800;(5) 回归：四旧键行为与时长不变；unknown_status 仍无脉冲

**风险**：低。仅常量表增量；pending 脉冲会在团队首次 addTeam 后的旧状态→新状态 diff 中触发一次，若觉吵闹可在实现评审时把 pending 改为有意缺省并加注释——规格默认补全。

---

**遗留问题**：
- F2 中成员 paused→'blocked'（出现 resume 按钮）需要确认父层 resume 处理器（useChatSpiritPanel/侧栏宿主的 onResume）对该 agentKey 走 ResumeSession 无 blockedInfo 前置校验；若有校验则降级为 paused→'suspended'式纯展示（此时需再增一个 displayStatus 键与 i18n,本规格默认前者）。
- F5 中 pending 转移是否发脉冲：规格默认补 800ms 弱脉冲；若产品认为团队卡片创建进场动画已足够，则 pending 有意缺省并在 PULSE_COLOR_MAP 注释声明，相应测试用例改为断言无脉冲。
- F1 的未知字符串兜底是否要在 dev 环境加 console.warn 以便尽早发现后端新状态词（当前与 agentNodeStatusToLabel 一样静默→queued)；加则属新增策略，默认不加。
- Finding #24 原文把 AgentSidebarCard.vue 路径写成 web/src/components/chat/（属实），P8 把 useStatusPulse 写成 web/src/features/orchestration/（实为 web/src/features/chat/composables/useStatusPulse.ts)——已按真实路径落位，无第二处同名文件。

## 4. 兼容性与迁移

- **事件契约**：新增 `team_run.cancelled` kind 为纯增量；旧 outbox/CriticalJournal 重放不含该 kind，回放兼容。前端对未知 kind 的既有行为是静默忽略（D9 补节流告警），旧前端不会因新 kind 崩溃。
- **缓存迁移**：经验缓存由 system_settings 单行整 blob 迁移为分行存储（DDL 见 D7-F5 迁移脚本）；启动一次性导入旧 JSON 行，导入失败保留原行不丢数据。旧条目指纹为空 → fail-closed 校验下按「一次性降级」处理（可命中但降权），不做强制淘汰。
- **行为变化**：用户取消的团队终态由 failed 改为 cancelled；依赖「取消计为失败」的下游（synthesis 失败叙事文案、监控告警级别）语义修正，属预期内变更，验收见开发计划。
- **wire**：全部修复经 internal/service/chat_wire.go、graph_wire.go 现有 provider 透传完成，不 regen cmd/admin/wire_gen.go（手工并合约定）。

## 5. 文档同步点（DOC-SYNC）

- 65-module-cross-reference-full.md：编排链路「取消语义」「DAG 预标记派发」「配额闸门」「缓存分行」四处行级更新（收尾阶段执行）。
- 78-org-aware-orchestration：终态语义补 cancelled 说明行。
- 其他模块文档不涉及接口/表结构变化，豁免。
