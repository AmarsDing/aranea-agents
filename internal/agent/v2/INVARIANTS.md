# internal/agent/v2 不变量档案（INVARIANTS）

> 事件溯源 v2（sequencer / event_router / deadletter / projector）的硬约束。每条 = 不变量 / 为什么 / 违反代价。

## INV-V2-1 事件可靠性分级（AS-EVT-01）

- **不变量**：Critical（ToolResult/Error/RunnerCompletion/Checkpoint）= WBPF（Write-Behind-Publish-Fail 前持久化）+ 重试；Important（StateDelta/TokenUsage/RunStatus/SessionStatusChanged/GraphNodeEnd/TeamRunFinished）= BlockUpTo + 异步持久化；Informational = 尽力而为。
- **为什么**：不同事件的业务后果差异极大，一刀切要么丢关键证据要么拖死热路径。
- **违反代价**：关键事件丢失导致恢复/审计失真，或热路径阻塞。

## INV-V2-2 Seq 只由 Sequencer 在发布期分配

- **不变量**：事件序号（Seq）的**分配权**独占于 Sequencer 发布期（经共享 SeqAssigner 单调分配）；生产端禁止自行编号。biz.Event 信封上的 Seq 字段（如 `deliverySeqCarrier`、RunStatusEvent.Seq / SystemNoticeEvent.Seq）仅承载已分配值用于投递/回放定位，不是编号来源。
- **为什么**：发布期统一编序是全序与重放的根基；分布式预编号必然出现空洞/乱序。
- **违反代价**：重放序错乱，projector 状态腐坏。

## INV-V2-3 deadletter ≠ 未送达

- **不变量**：死信（重试耗尽进入 deadletter）只代表「本端持久化/确认未完成」；对端是否已消费**不可判定**。重放必须幂等（幂等键/代际 fencing），禁止把死信当「确认未送」触发业务补偿。
- **为什么**：publish 与 confirm 之间的崩溃窗口天然产生「已送达但未确认」双生子（orca provider-launch 簇同结论）。
- **违反代价**：补偿动作把已送达事件再执行一次（重复扣费/重复执行）。

## INV-V2-4 终态守卫

- **不变量**：projector 与 GraphNode 一律先做终态谓词守卫（已完成实体拒绝后续状态迁移）；complete 类死信重放必须 fencing 终态 + 同事务清理。
- **为什么**：89 批次 T10–T12（FR-10/11/12）事故链——重放/迟到事件把已终结实体写活。
- **违反代价**：终态被覆写，UI 与审计出现「复活」实体。

## INV-V2-5 恢复路径只信正向证据

- **不变量**：outbox 水位/恢复扫描必须折入持久化证据（水位线、冲突重试），禁止用「进程活着」推断「事件已发完」。
- **为什么**：89 批次 T9（FR-9）——水位未折入导致恢复漏发；活性标志在挂起/休眠场景不可信。
- **违反代价**：恢复后事件缺口静默存在。
