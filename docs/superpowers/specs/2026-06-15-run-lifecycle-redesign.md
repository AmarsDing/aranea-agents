# Run 生命周期重新设计方案

> 日期：2026-06-15
> 状态：草案 v3（经深度逻辑审查修正）
> 影响范围：后端 ~50 文件 / ~350 行 + 前端 ~17 文件 / ~80 行 + 测试 ~8 文件 / ~120 行

---

## 一、背景与问题

当前 Run 生命周期存在三个根本性设计缺陷：

### 1.1 时间预算机制与用户需求背道而驰

**现状**：软预算 180s → `escalating`，硬预算 900s → 强制 `durable`，纯时间驱动。

**问题**：
- Agent 的核心价值是**按用户指令完成任务**，不管时间多长都应以完成任务为准
- 时间预算是为 IM 渠道的 webhook 超时设计的，但被默认套用到了 Web/WS 渠道
- Web 渠道是长连接，不存在 IM 的 3-5 秒超时问题
- `escalating` 状态对 Web 用户无意义——没有飞书卡片可点，没有 `/background` 可用
- 文档 `56-business-logic-optimization.md` 已明确记录痛点："Web 用户：我要 Agent 跑半小时深度研究 → 必须时间满 900s 才升 durable，期间网页关掉就丢"

### 1.2 Session 并发锁阻塞用户交互

**现状**：一个 Session 同时只能有一个活跃 Run，新消息只能 queue 或 reject_busy。

**问题**：
- 用户在 Agent 执行期间发新消息，只能排队等待，无法中途追加信息
- 学习 Cursor：用户应能发送消息进入 pending 列表，可选择等待 turn 完成后执行，或点击"立即发送"中断当前 turn 并将新内容注入 Agent 上下文
- 当前 `BusyInputMode` 仅支持 `queue`/`interrupt`/`followup`，缺乏用户选择权

### 1.3 自动超时处理卡死问题不合理

**现状**：硬预算 900s + 60s 宽限期后强制 Fail，stuck tool 自动发布失败信封。

**问题**：
- Agent 无响应 / Turn 卡死不应由机器自动判断，应由人判断
- 学习 Cursor：提供"停止生成"按钮，由用户决定是否中断
- 自动超时可能中断正在正常执行的长任务

---

## 二、设计目标

1. **任务完成优先**：Agent 应不受时间限制地执行任务，直到完成或用户主动停止
2. **用户控制权**：用户可在 Agent 执行期间追加信息、中断执行、转后台——一切由用户决定
3. **IM 心跳保活**：IM 渠道通过心跳维持 webhook 连接，而非超时中断
4. **智能建议**：系统可基于多信号检测建议用户操作（如"是否转后台"），但不自动执行
5. **向后兼容**：保留 durable 后台执行能力，但触发方式改为用户主动

---

## 三、统一修改方案

### 3.1 变更一：去掉时间预算，改为心跳保活 + 用户主动控制

#### 3.1.1 删除的代码

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/biz/session_run_budget.go` | **整个文件删除** | BudgetPhaseCallbacks、StartBudgetWatcher、fireSoft/fireHard、hardBudgetGracePeriod 全部移除 |
| `internal/service/chat_session_run_escalate.go` | **整个文件删除** | EscalateActiveSessionRun、EscalateSessionRun 两个方法移除。**注意**：`channelBackgroundReplyOK`/`channelBackgroundReplyAlready`/`channelBackgroundReplyNoActiveRun` 常量定义在此文件中（L13），需迁移到 `channel_ingress_background.go` |
| `internal/channel/preview/feishu_escalate_card.go` | **整个文件删除** | 软预算通知卡片不再需要 |
| `internal/channel/preview/feishu_escalate_card_test.go` | **整个文件删除** | 对应测试 |

#### 3.1.2 简化的代码

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/biz/session_run_phase_machine.go` | 移除 `PhaseEscalating`、`PhaseEventEscalate` 及相关转换规则；新增 `PhaseEventUserEscalate` 和 `interactive → durable` 直接转换规则 | 状态机从 6 阶段简化为 5 阶段。**关键**：`ParseSessionRunPhase()` 中 `PhaseEscalating` case 不能简单删除——DB 中可能存在 `phase='escalating'` 的历史记录，需映射为 `PhaseDurable`（语义最接近：已从交互模式转出） |
| `internal/biz/session_run.go` | 移除 `SessionRunBudget` struct、`DefaultSessionRunBudget()`、`SessionRunPhaseEscalating` 常量；`StartInteractive` 不再接收 budget 参数 | SessionRun struct 中 `SoftBudgetSec`/`HardBudgetSec` 字段标记 deprecated 但保留（避免 DB schema 变更）。**接口签名变更**：`StartInteractive` 移除 `budget` 参数，所有调用方和 mock 需同步更新 |
| `internal/biz/session/status.go` | 移除 `StatusReasonBudgetEscalated`，新增 `StatusReasonUserEscalated SessionStatusReason = "user_escalated"` | **同步修改**：`internal/biz/session/status_machine.go` 中的转换规则需将 `StatusReasonBudgetEscalated` 替换为 `StatusReasonUserEscalated`；`status_machine_test.go` 中引用也需更新 |
| `internal/biz/channel_config_helpers.go` | 移除 `AutoEscalateAfterSoftBudget`、`SoftEscalateConfirmSec`、`RunPolicy()`、`SoftEscalateConfirmSecOrDefault()`、`DefaultSoftEscalateConfirmSec()` | `TurnTimeoutSec` 保留但语义变更：不再影响 hard budget，仅作为 turn context 的超时上限（见 3.3）。`DurableDeadlineSec` **保留**（它是 durable 执行的 deadline，与 budget 无关）。JSON 解析结构体中对应字段移除 |
| `internal/service/session_run_escalation_notifier.go` | 移除 `NotifySoftBudget` 方法 | 保留 `NotifyDurableEscalated`、`NotifyRunCompleted`、`NotifyRunFailed`（durable worker 仍需通知）。**注意**：`NotifySoftBudget` 内部调用了 `preview.BuildFeishuEscalateCardJSON`，删除方法后该引用自动消除，不会产生编译错误 |
| `internal/service/chat_orch_session_run_lifecycle.go` | 移除 `onSessionRunSoftBudget`、`stopBudget` 机制、`OnSoftBudget`/`OnHardBudget` 回调 | **关键修改**：(1) `BeginSessionRunLifecycle` 当前返回 `(context.Context, string, context.CancelFunc)`，CancelFunc 就是 stopBudget。删除后返回值改为 `(context.Context, string)`，**接口签名变更**需同步更新所有调用方和 mock；(2) `BeginSessionRunLifecycle` 中 `budget := ltCfg.RunPolicy()` 和 `StartInteractive(budget)` 调用需删除，改为 `StartInteractive()` 无参调用；(3) `applyDurableTransition` 中 `StatusReasonBudgetEscalated` 替换为 `StatusReasonUserEscalated`；(4) `FinishSessionRunLifecycle` 中 `PhaseEscalating` 分支删除；(5) 移除 `safego` import（如果仅被 `onSessionRunSoftBudget` 使用） |
| `internal/service/chat_orchestrator_turn_phases.go` | 移除 `turnExecuteResult` 中的 `stopBudget context.CancelFunc` 字段 | `invokeTurnLLMAndStream` 不再接收 stopBudget；`assembleTurnResult` 不再接收 stopBudget 参数 |
| `internal/service/chat_orchestrator_turn.go` | 移除 `defer execResult.stopBudget()` | 不再需要取消预算计时器 |
| `internal/service/chat_orchestrator_durable.go` | 移除 `stopBudget` 相关代码 | L82 的 `stopBudget := func() {}` 空函数定义删除，durable resume 路径不再返回 stopBudget |
| `internal/service/channel_ingress_session.go` | L211 移除 `biz.SessionRunPhaseEscalating` case | 仅保留 `biz.SessionRunPhaseDurable` |
| `internal/data/session_run_repo.go` | 修改 3 处 SQL 中 `escalating` 引用 | (1) L277 `ListForJobs`：`phase IN ('escalating','durable','interactive')` → `phase IN ('durable','interactive')`；(2) L326 `GetActiveForSession`：移除 `biz.SessionRunPhaseEscalating` 参数；(3) L443 `MarkOrphanedRunsCancelled`：`phase IN ('interactive','escalating')` → `phase IN ('interactive')` |
| `internal/service/session_observability.go` | 移除 `SoftBudgetSec`/`HardBudgetSec` 字段 | — |
| `internal/service/ingress_policy.go` | 移除 `IngressSteer`/`IngressRejectBusy` 常量别名 | 这是 biz 层类型的 service 层别名，需与 biz 层同步 |
| `internal/service/channel_ingress_policy.go` | 移除 `IngressRejectBusy` 分支和 `IngressSteer` 分支 | **关键**：`steerIntoActiveTurn()` 方法（L80-100）调用 `TryEnqueueUserMessage()` 直接注入运行中 runner。移除 steer 后，**Channel 的 `BusyInputMode=interrupt` 仍需保留此能力**——改为在 `interrupt` 模式下直接调用 `InterruptAndInjectMessage`（见 3.2），而非简单删除 |
| `internal/service/channel_ingress_background.go` | 更新 `HandleBackgroundCommand` | 当前引用 `EscalateActiveSessionRun`，需改为调用 `EscalateToDurableByUser`。`channelBackgroundReply*` 常量已迁移到此文件 |
| `internal/service/channel_ingress_card_action.go` | 更新飞书卡片回调处理 | L34/L38/L70 引用 `channelBackgroundReplyDenied`/`channelBackgroundReplyNoActiveRun`，需确认常量迁移后路径正确 |
| `internal/service/channel_ingress_constants.go` | 更新 `channelStatusPhaseTemplate` | 当前 status 查询会显示"当前任务阶段：escalating"，移除后需更新模板或映射 |
| `internal/service/chat.go` | `ActiveSessionRunPhase()` 增加兼容映射 | L157-170 返回 run.Phase，移除 escalating 后前端可能收到旧的 escalating 值。需在此处做兼容映射：若 DB 中读到 `escalating`，映射为 `durable` 返回 |
| `internal/biz/turn_gateway.go` | 确认 `ActiveSessionRunPhase` 接口返回值兼容 | L39-40 接口定义，移除 escalating 后需确认返回值兼容 |
| `internal/biz/session_run_checkpoint.go` | **保留，无需修改** | `DurableCheckpointSnapshot`/`CreateDurableCheckpoint`/`GetCheckpoint` 是 durable 升格的核心依赖，完全保留 |
| `internal/service/chat_session_run_cancel.go` | 确认 `channelBackgroundReply*` 常量引用 | 删除 escalate 文件后常量已迁移，需确认引用路径正确 |
| `cmd/admin/wire.go` | **修改而非移除** Wire 绑定 | `provideChannelRunEscalationNotifier` 和 `provideChannelNotifierDeps` **不能移除**（`SessionRunEscalationNotifier` 接口仍存在，只是方法减少）。仅移除 `chat_session_run_escalate.go` 对应的任何独立 Wire 绑定（如果有的话——从搜索结果看，该文件中的方法是 ChatService 的方法，不涉及独立 Wire 绑定） |

#### 3.1.3 新增的代码

| 文件 | 新增 | 说明 |
|------|------|------|
| `internal/biz/escalation_suggester.go`（新文件） | `EscalationSuggester` struct + `ShouldSuggestDurable()` 方法 | 基于多信号（工具调用数、token 数、执行时长）计算建议分数，返回建议结果。**仅建议，不自动执行**。通过 WS 推送"建议转后台"通知给前端 |
| `internal/service/chat_orch_session_run_lifecycle.go` | `EscalateToDurableByUser()` 方法 | 用户主动触发的 durable 升格（替代原 `EscalateSessionRunToDurable`，去掉 budget 相关逻辑，保留 checkpoint + cancel runner + notify）。**接口方法替换**：`sessionRunLifecycle` 接口中 `EscalateSessionRunToDurable` 改名为 `EscalateToDurableByUser`，签名简化 |
| `internal/service/channel_heartbeat.go`（新文件） | `ChannelHeartbeatSender` | IM 渠道心跳保活。**重要发现**：`ChannelLongTaskConfig` 中已有 `HeartbeatMessage`（默认"仍在处理中…"）和 `ProgressQuietSec`（默认 20s）配置，`ChannelIMRenderPolicy.HeartbeatEnabled` 已计算，但**实际发送心跳的定时器逻辑缺失**（半完成功能）。本文件补全心跳发送逻辑，利用已有的流式预览 PATCH 机制更新同一条消息（而非发送新消息），避免消息轰炸 |
| `internal/channel/preview/feishu_durable_card.go`（新文件） | 替代 `feishu_escalate_card.go` | 用户主动转后台的飞书卡片模板，包含"已转后台执行"状态展示（不再需要"确认转后台"按钮，因为是用户主动触发） |

#### 3.1.4 前端变更

**重要架构背景**：前端存在两套独立的状态类型系统：

| 类型系统 | 核心类型 | 使用场景 | 当前是否包含 escalating |
|---------|---------|---------|----------------------|
| Chat 运行时 | `RunStatusValue`（domain/types.ts） | WS 推送、ChatRunnerStatus、Composer 状态 | **否** |
| 后台任务 | `SESSION_RUN_STATUS`（sessionRunStatus.ts） | BackgroundJob 面板、轮询策略 | **是** |

本次修改只影响**后台任务类型系统**（移除 escalating），Chat 运行时类型系统无需变更（因为 `RunStatusValue` 本身就不包含 escalating）。

| 文件 | 变更 | 说明 |
|------|------|------|
| `web/src/features/chat/sessionRunStatus.ts` | 移除 `ESCALATING: 'escalating'`，从 `ACTIVE_RUN_STATUSES` 中移除 | 不再有 escalating 状态 |
| `web/src/features/chat/jobFormatters.ts` | 移除 `case 'escalating': return 'Escalating'` | 不再需要 escalating 标签 |
| `web/src/components/chat/chatUi.ts` | 移除 `case 'escalating':` 分支 | 不再需要 escalating 颜色 |
| `web/src/features/chat/useChatBackgroundJobs.ts` | 从 `ACTIVE_STATUSES` 中移除 `'escalating'` | — |
| `web/src/components/sessions/SessionRunsPanel.vue` | 移除 `phase === 'escalating'` 分支 | — |
| `web/src/components/sessions/SessionStatusBadge.vue` | 移除 `budget_escalated: '预算超限'`，新增 `user_escalated: '用户转后台'` | — |
| `web/src/features/session/types.ts` | 移除 `'budget_escalated'`，新增 `'user_escalated'`；`soft_budget_sec`/`hard_budget_sec` 标记 optional | — |
| `web/src/features/chat/composables/useChatRunStatus.ts` | 无变更 | runStatus 机制不变，只是不再收到 escalating 状态 |
| `web/src/features/chat/envelopeRunStatus.ts` | **新增兼容映射** | `runStatusFromEnvelope()` 中，如果后端 WS 推送 `escalating` 状态（旧信封），映射为 `running`。当前代码用 `as RunStatusValue` 强制转换，不会编译报错但运行时展示错误，需显式处理 |
| `web/src/features/chat/conversationEventDispatcher.ts` | **新增兼容映射** | `turnStatusFromEnvelope()` 中，如果 metadata.status/phase 包含 `escalating`，`runStatusToTurnStatus` 应映射为 `running` |

#### 3.1.5 数据库兼容

- `session_runs` 表的 `soft_budget_sec` / `hard_budget_sec` 列**保留不删**（SQLite 不支持 DROP COLUMN）
- 新写入的 run 记录这两个字段写 0
- Ent Schema 中字段标记 `Deprecated` 注释（需找到 `internal/data/ent/schema/` 下对应的 SessionRun schema）
- Proto 中字段标记 `deprecated`

**历史数据迁移**：DB 中可能存在 `phase='escalating'` 的活跃记录。处理策略：
1. **查询层兼容**：`ParseSessionRunPhase()` 将 `"escalating"` 映射为 `PhaseDurable`（语义最接近）
2. **数据迁移**（可选）：在 DDL Migration Registry 中新增一条迁移，将 `phase='escalating'` 的记录更新为 `phase='durable'`，`status_reason` 从 `budget_escalated` 更新为 `user_escalated`
3. **SQL 查询兼容**：`GetActiveForSession` 和 `ListForJobs` 移除 `'escalating'` 后，历史 escalating 记录仍能被查到（因为 `ParseSessionRunPhase` 映射后走 durable 路径）

#### 3.1.6 潜在风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 去掉硬预算后，Run 可能永远不结束 | 资源泄漏 | 保留 `CleanupOrphanedRuns`（进程重启时清理）；新增用户主动"停止执行"按钮；durable worker 保留；保留 1-2 小时的 turnTimeout 安全上限（见 3.3） |
| IM 渠道 webhook 超时 | 消息丢失 | 心跳保活机制确保连接不超时；如果心跳也失败，IM 平台会重试 |
| 前端收到旧的 `escalating` 状态信封 | 显示异常 | 前端 `runStatusFromEnvelope` 将 `escalating` 映射为 `running`（兼容旧信封）；`turnStatusFromEnvelope` 同理 |
| DB 中 `phase='escalating'` 的历史记录 | 查询遗漏 | `ParseSessionRunPhase` 映射为 `PhaseDurable`；可选数据迁移 |
| `sessionRunLifecycle` 接口签名变更 | 编译错误 | `BeginSessionRunLifecycle` 返回值从 3 个改为 2 个，所有调用方和 mock 需同步更新 |
| `StartInteractive` 签名变更 | 编译错误 | 移除 budget 参数，所有调用方和 mock 需同步更新 |
| `NotifySoftBudget` 删除后 `BuildFeishuEscalateCardJSON` 引用 | 编译错误 | 不会出错——`BuildFeishuEscalateCardJSON` 仅在 `NotifySoftBudget` 内部调用，删除方法后引用自动消除 |

---

### 3.2 变更二：Session 并发锁改为 Pending 列表 + 用户选择

#### 3.2.1 核心设计

**当前流程**：
```
用户发消息 → HasActiveRun? → Yes → IngressQueue/IngressSteer/IngressRejectBusy
                                  → No  → IngressAdmit（直接执行）
```

**新流程**：
```
用户发消息 → HasActiveRun? → Yes → 消息进入 Pending 列表
                                  → 用户可选择：
                                    A. 等待当前 turn 完成后自动执行（默认行为）
                                    B. 点击"立即发送"→ 中断当前 turn + 启动新 turn 包含新消息
                                    C. 删除 pending 消息
                                  → No  → IngressAdmit（直接执行）
```

#### 3.2.2 "立即发送"流程重设计（关键修正）

**v2 方案的严重逻辑漏洞**（经代码审查发现）：

| 漏洞 | 说明 | 严重度 |
|------|------|--------|
| **Cancel 与 processPendingQueue 竞态** | Cancel 删除 activeRuns → 当前 turn 的 defer 链中 `processPendingQueue` 可能先于"立即发送"的新消息入队就执行 → 消息丢失 | 高 |
| **Steer 路径语义冲突** | 当前 `EnqueueUserMessage` 的 Steer 是注入到**当前 turn** 的 LLM 上下文（不中断 turn），而"立即发送"期望**中断 turn + 新 turn** | 高 |
| **Pending Queue FIFO 语义冲突** | 用户对消息 C 点击"立即发送"，但 B 在 C 前面（FIFO），B 会先执行 | 中 |
| **Cancel 后 Runner 回滚竞态** | Cancel 后 defer 链执行 `rollbackRunnerSession()`，新 turn 在回滚完成前创建 runner 可能状态不一致 | 中 |
| **Session Lock 竞争** | Cancel 后的清理和新 turn 启动都需要 session lock，可能死锁 | 中 |

**修正方案：两阶段"立即发送"**：

学习 Cursor 的实际行为：Cursor 的"立即发送"不是"Cancel + 新 turn"，而是**先 Cancel 当前生成，等 run 结束后自动处理 pending 中的消息**。这避免了所有竞态问题。

```
"立即发送"流程：
1. 用户点击"立即发送"
2. 后端将 pending 消息标记为 `priority=high`（提升优先级）
3. 后端 Cancel 当前 turn（与 StopGeneration 相同的 Cancel 路径）
4. 当前 turn 的 defer 链执行：
   a. o.runs.Finish(sessionID) — 从 activeRuns 删除
   b. runner.Close() — 关闭 runner
   c. processPendingQueue() — 取出队首消息
      → 如果是 priority=high 的消息，立即启动新 turn
      → 如果是普通消息，也立即启动新 turn（原有逻辑）
5. 新 turn 正常执行，包含用户的新消息
```

**关键设计决策**：
- "立即发送" = **标记优先级 + Cancel**，不是"Cancel + 注入"
- 新 turn 通过 `processPendingQueue` 正常启动，不需要特殊的注入逻辑
- 避免了所有竞态问题，因为复用了已有的 Cancel → Finish → processPendingQueue 流程
- `priority=high` 标记确保：如果 pending 队列中有多条消息，被标记的消息排在队首（优先出队）

**FIFO 语义修正**：
- 当用户对某条 pending 消息点击"立即发送"时，该消息被移到队首（`PromoteToFront`）
- 其他消息保持原序
- 这确保了用户的意图（"这条消息现在就要执行"）得到满足

#### 3.2.3 IngressPolicy 重设计

**关键修正**：不能简单地将 `IngressSteer` 改为 `IngressQueue`——两者的行为完全不同：

| 决策 | 行为 | 适用场景 |
|------|------|----------|
| `IngressQueue` | 消息进入 pending 队列，等待当前 turn 完成后执行 | Web 渠道默认行为 |
| `IngressSteer`（旧） | 调用 `TryEnqueueUserMessage()` 直接注入运行中 runner 的当前 turn | Channel 的 `BusyInputMode=interrupt` |

**新设计**：

| 决策 | 行为 | 适用场景 |
|------|------|----------|
| `IngressQueue` | 消息进入 pending 队列，等待当前 turn 完成后执行 | Web 渠道默认、Channel `BusyInputMode=queue` |
| `IngressInterrupt`（新，替代 Steer） | 消息进入 pending 队列（priority=high）+ Cancel 当前 turn → processPendingQueue 自动启动新 turn | Channel `BusyInputMode=interrupt`、Web 用户点击"立即发送" |

**与 v2 方案的关键区别**：
- v2 的 `IngressInject` 试图在 Cancel 后立即注入新消息到新 turn，存在竞态
- v3 的 `IngressInterrupt` 复用 Cancel → processPendingQueue 流程，无竞态
- `IngressInterrupt` 的实际行为 = `Enqueue(priority=high)` + `Cancel`，两步操作

**移除的决策**：
- `IngressRejectBusy`：不再 reject，所有消息都进入 pending 或 interrupt
- `IngressSteer`：被 `IngressInterrupt` 替代（语义更清晰：不是"转向"而是"中断后执行"）

**`context_pressure` 场景处理**：
- 当前：`contextPressure + hasActiveRun` → `IngressRejectBusy`（拒绝并发送 overflow 提示）
- 新方案：`contextPressure + hasActiveRun` → `IngressQueue`（进入队列，但标记为低优先级）。不 reject 是因为用户消息不应被丢弃，但 context 满时执行队列消息可能加剧 overflow，因此在 turn 完成后处理 pending 时检查 context 压力，如果仍满则延迟执行

#### 3.2.4 修改的代码

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/biz/ingress_policy.go` | 移除 `IngressRejectBusy`、`IngressSteer`；新增 `IngressInterrupt`；`HasActiveRun=true` 时根据 `BusyInputMode` 返回 `IngressQueue` 或 `IngressInterrupt` | `EvaluateIngressPolicy` 纯函数逻辑调整：`contextPressure` 场景改为 `IngressQueue`；`IngressDecisionNeedsTurn()` 移除 `IngressSteer` case，新增 `IngressInterrupt` case |
| `internal/biz/chat_usecase.go` | `EnqueueUserMessage` 已返回 `pendingID`，无需修改签名；新增 `InterruptAndSendMessage(sessionID, pendingEntryID)` 方法 | "立即发送"的核心方法：(1) 将 pending 消息标记为 `priority=high` 并移到队首；(2) Cancel 当前 turn。后续由 processPendingQueue 自动处理 |
| `internal/runtime/pending_queue.go` | 新增 `PromoteToFront(sessionID, pendingID)` 方法和 `SetPriority(sessionID, pendingID, priority)` 方法 | 将指定消息移到队首（用于"立即发送"时优先出队）。新增 `priority` 字段到 `PendingMessage` struct |
| `internal/runtime/run_registry.go` | `Cancel` 方法增加 `reason` 参数 | 区分"用户中断"（`user_interrupt`）和"系统取消"（`system_cancel`）。**签名变更**：`Cancel(sessionID string) (bool, string)` → `Cancel(sessionID, reason string) (bool, string)`。所有调用方需更新 |
| `internal/service/chat_orchestrator_turn_dispatch.go` | `processPendingQueue` 逻辑微调 | 取出队首消息时，优先取出 `priority=high` 的消息（如果 PromoteToFront 已实现，则 FIFO 自然保证高优先级先出队） |
| `internal/service/channel_ingress_policy.go` | 移除 `IngressRejectBusy` 分支；`IngressSteer` 分支改为 `IngressInterrupt` 分支 | **关键**：`steerIntoActiveTurn()` 方法重命名为 `interruptActiveTurn()`，内部调用 `InterruptAndSendMessage`（Enqueue priority=high + Cancel）替代 `TryEnqueueUserMessage` |
| `internal/biz/channel_config_helpers.go` | `BusyInputMode` 保留但 `interrupt` 模式改为"中断后执行"语义 | `queue` 模式不变；`followup` 模式不变 |
| `internal/service/ingress_policy.go` | 同步移除 `IngressSteer`/`IngressRejectBusy` 常量别名，新增 `IngressInterrupt` | — |

#### 3.2.5 新增的代码

| 文件 | 新增 | 说明 |
|------|------|------|
| `api/kratos/chat/v1/chat.proto` | 新增 `InterruptAndSendMessage` RPC | 前端调用"立即发送"。**注意**：`CancelRun` 功能已通过 `StopGeneration` RPC（HTTP POST `/v1/chat/stop`）实现，无需新增 |
| `web/src/components/chat/ChatPendingQueue.vue` | 增加"立即发送"按钮 | 当前已有完整的 pending 列表 UI（显示/编辑/取消），只需增加"立即发送"按钮 |

#### 3.2.6 前端变更

| 文件 | 变更 | 说明 |
|------|------|------|
| `web/src/components/chat/ChatPendingQueue.vue` | 增加"立即发送"按钮（点击后调用 `interruptAndSend` API） | 用户可对 pending 消息执行操作。当前组件已有 `cancel-pending` emit，新增 `interrupt-pending` emit |
| `web/src/features/chat/api.ts` | 新增 `interruptAndSendMessage(sessionID, pendingEntryID)` API | 调用后端 RPC |
| `web/src/features/chat/types.ts` | `EnqueueUserMessageResult` 增加 `pendingEntryID` 字段 | 前端需要 ID 来操作 pending 消息。**注意**：后端 `EnqueueUserMessage` 已返回 `pendingID`，需确认前端类型定义是否已包含 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | `isRunnerActive` 逻辑确认 | 当前只判断 `running/pending`（L612-614），`IngressInterrupt` 不影响此逻辑（interrupt 后 run 先变为 idle 再变为 running，由 processPendingQueue 驱动） |
| `web/src/components/chat/ChatComposer.vue` | 统一 Enqueue 入口 | **当前问题**：主输入框 Enter 键和 ChatEnqueueMessage 专用输入框两套入口并存，用户困惑。**修正**：移除 ChatEnqueueMessage 独立输入框，统一在主输入框中处理（run 活跃时 Enter = enqueue，placeholder 变为"消息将在当前任务完成后执行"） |
| `web/src/components/chat/ChatEnqueueMessage.vue` | **删除此组件** | 功能合并到 ChatComposer 主输入框，减少用户困惑 |
| `web/src/components/chat/ChatRunnerStatus.vue` | 增加 pending 消息计数 badge | 在 run 状态旁显示排队消息数量，让用户感知 pending 消息存在 |
| `web/src/features/chat/composables/useFollowUpQueue.ts` | session 切换时立即清空 pendingMessages | 避免旧 session 的 pending 消息短暂显示在新 session 中 |

#### 3.2.7 潜在风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| "立即发送" Cancel 后 processPendingQueue 未及时启动 | 用户等待时间稍长 | processPendingQueue 在 defer 链中同步调用，Cancel 后立即执行，延迟极小（< 100ms） |
| Cancel 后部分回复丢失 | 用户体验 | 当前 Cancel 机制已有"超时降级"路径：如果 `result.HasContent=true`，PERSIST 阶段会保存已有回复。新 turn 的 prompt 中包含"上次执行被用户中断，已生成内容：..." |
| 高频消息导致 pending 列表过长 | 性能 | 限制 pending 列表最大长度（当前 `MaxPendingPerSession = 32`，可降为 10）；超出时提示用户"消息队列已满" |
| Channel 渠道无法显示 pending 列表 UI | 功能缺失 | Channel 的 `BusyInputMode=interrupt` 直接执行中断+执行（`IngressInterrupt`），不需要 UI 选择；`BusyInputMode=followup` 保持现有合并行为；`BusyInputMode=queue` 进入 pending 队列 |
| `Cancel` 签名变更导致编译错误 | 编译 | 所有调用方需更新：`chat_orch_session_run_lifecycle.go:309`、`chat_usecase.go:142` 等 |
| `IngressSteer` → `IngressInterrupt` 行为变更 | Channel 用户体验 | `steerIntoActiveTurn` 调用 `TryEnqueueUserMessage`（注入到当前 turn，不中断），`interruptActiveTurn` 调用 `InterruptAndSendMessage`（Cancel 当前 turn + 新 turn 执行）。行为从"注入当前 turn"变为"中断后新 turn"，**语义变更需在文档中明确说明** |
| `PromoteToFront` 并发安全 | 数据一致性 | `PendingMessageQueue` 已有 `mu sync.Mutex` 保护，PromoteToFront 在锁内操作，线程安全 |
| Durable turn 被中断后上下文丢失 | 数据丢失 | 如果当前 turn 是 durable resume turn，Cancel 后新 turn 不会继承 durable 上下文。**缓解**：InterruptAndSendMessage 应检查当前 run phase，如果是 durable 则拒绝中断（返回错误提示"后台任务无法中断，请等待完成"） |

---

### 3.3 变更三：去掉自动超时，改为纯人工判断

#### 3.3.1 核心设计

**当前行为**：
- 硬预算 900s + 60s 宽限期 → 强制 Fail
- Stuck tool → 自动发布失败信封
- Turn timeout → 自动 fail

**新行为**：
- **无自动 fail**：Run 不会因为时间而自动失败
- **用户主动停止**：UI 提供"停止生成"按钮，IM 提供 `/cancel` 命令
- **Stuck tool 保留**：stuck tool detection 保留（这是合理的——turn 结束时 tool_call 没有 tool_result 是异常，需要清理），但改为"清理 + 通知用户"而非"静默失败"
- **Orphan 清理保留**：进程重启时的孤儿 Run 清理保留（这是安全兜底，不是超时机制）

#### 3.3.2 修改的代码

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/biz/session_run_budget.go` | **整个文件删除**（已在 3.1 中覆盖） | 移除所有自动超时逻辑 |
| `internal/agent/stream_consumer.go` | `finalize()` 中 stuck tool 处理改为：发布失败信封（保留）+ 通过 WS 推送"卡住工具"通知给用户 | stuck tool 的失败信封仍需发布（否则 tool_call 永远无结果），但增加用户可见的通知 |
| `internal/agent/activity_publish.go` | `stuckToolResultFallback` 改为更友好的消息："工具执行未返回结果，已自动标记为失败。如需重试请重新发送指令" | 保留自动清理，但消息更友好 |
| `internal/service/chat_orchestrator_turn_phases.go` | turn timeout 不再自动 fail，改为推送通知 + 等待用户操作 | L336-342 的 turn timeout 处理：从"直接 fail"改为"通过 WS 推送超时提醒 + 继续等待"。context.WithTimeout 改为极大值 |
| `internal/service/chat_orchestrator_turn.go` | `context.WithTimeout(ctx, o.turnTimeout())` 改为 `context.WithTimeout(ctx, o.turnTimeout() * 12)` | 实际超时设为 1 小时（5min * 12），仅防止无限挂起。**这是安全上限，不是业务截止时间** |
| `internal/biz/session/status.go` | `StatusReasonTimeout` 保留，但仅用于用户主动停止时 | — |

#### 3.3.3 新增的代码

| 文件 | 新增 | 说明 |
|------|------|------|
| `api/kratos/chat/v1/chat.proto` | `CancelRun` RPC 已存在（`StopGeneration`，HTTP POST `/v1/chat/stop`） | 无需新增，确认可用即可 |
| `web/src/components/chat/ChatRunnerStatus.vue` | `visible` computed 扩展到包含 `durable` 状态 | 当前 `visible` 判断 `running/pending/awaiting_user`，需增加 `durable`（用户可在 durable 状态下停止后台任务）。**注意**：`escalating` 已在 3.1 中移除，无需处理 |
| `web/src/components/chat/ChatRunnerStatus.vue` | `showCancel` computed 扩展到所有活跃状态 | 当前 `showCancel` 只在 `running/pending` 时显示，需扩展到 `durable` 状态 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | 长时间无事件提示 | 当 run 处于 `running` 状态但超过 5 分钟无新事件时，自动提示"似乎没有进展，是否停止？" |

#### 3.3.4 保留的机制

| 机制 | 保留原因 |
|------|----------|
| **Stuck tool detection** | turn 结束时 tool_call 没有 tool_result 是协议异常，必须清理。但改为"清理 + 通知用户"而非"静默失败" |
| **Orphan run cleanup** | 进程重启时清理孤儿 Run 是必要的安全兜底，与超时无关 |
| **FirstByteTimeout** | 模型 30 秒无首字节大概率是配置问题，保留提示是合理的 |
| **TurnTimeout（作为安全上限）** | 保留极大值（1 小时）的安全上限，防止真正的无限挂起。但不作为业务逻辑的截止时间 |

#### 3.3.5 潜在风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Run 真正卡死且用户不操作 | 资源泄漏 | 保留 1 小时的安全上限作为最后兜底；进程重启时 orphan cleanup 清理；前端 5 分钟无事件提示 |
| 用户不知道 Run 卡住了 | 体验差 | 前端显示实时执行状态（工具调用数、token 数、已执行时间）；长时间无事件时自动提示"似乎没有进展，是否停止？" |
| LLM API 无限等待 | HTTP 连接泄漏 | 保留 HTTP 层面的超时（如 10 分钟），这是网络层超时，不是业务超时 |
| turnTimeout 增大到 1 小时后，真正的卡死需要更长时间才能被安全上限清理 | 资源占用时间更长 | 这是可接受的权衡——宁可多等一会儿，也不要误杀正常长任务。1 小时安全上限足以覆盖绝大多数场景 |

---

## 四、状态机变更

### 4.1 当前状态机

```
interactive --escalate--> escalating
interactive --complete--> completed
interactive --fail--> failed
interactive --cancel--> cancelled
escalating --durable--> durable
escalating --complete--> completed
escalating --fail--> failed
escalating --cancel--> cancelled
durable --complete--> completed
durable --fail--> failed
durable --cancel--> cancelled
```

### 4.2 新状态机

```
interactive --user_escalate--> durable      (用户主动转后台)
interactive --complete--> completed
interactive --fail--> failed
interactive --cancel--> cancelled           (用户主动停止)
durable --complete--> completed
durable --fail--> failed
durable --cancel--> cancelled
```

**注意**：`user_inject`（用户追加消息）**不是状态机事件**——它不改变 run 的 phase，run 保持在 `interactive`。它是一个独立的行为触发器（中断当前 turn + 注入新消息 + 启动新 turn），在 `TurnInterrupt` 模块中处理，不经过状态机。

**关键变更**：
1. 移除 `escalating` 阶段和 `escalate` 事件
2. 新增 `user_escalate` 事件（用户主动转后台，`interactive → durable`）
3. `interactive → durable` 直接可达，无需经过 escalating

### 4.3 SessionRunPhase 常量变更

```go
// 移除
PhaseEscalating  SessionRunPhase = "escalating"
PhaseEventEscalate SessionRunPhaseEvent = "escalate"

// 新增
PhaseEventUserEscalate SessionRunPhaseEvent = "user_escalate"
```

### 4.4 ParseSessionRunPhase 兼容处理

```go
func ParseSessionRunPhase(s string) SessionRunPhase {
    switch s {
    case string(PhaseInteractive):
        return PhaseInteractive
    // case string(PhaseEscalating):  // 已移除
    //     return PhaseEscalating
    case string(PhaseDurable):
        return PhaseDurable
    case string(PhaseCompleted):
        return PhaseCompleted
    case string(PhaseFailed):
        return PhaseFailed
    case string(PhaseCancelled):
        return PhaseCancelled
    default:
        // 兼容：DB 中历史 escalating 记录映射为 PhaseDurable
        if s == "escalating" {
            return PhaseDurable
        }
        return PhaseInteractive
    }
}
```

---

## 五、影响面汇总

### 5.1 需要修改的文件清单

#### 后端（按依赖层级排序）

**删除文件（4 个）**：

| 文件 | 说明 |
|------|------|
| `internal/biz/session_run_budget.go` | 整个文件 |
| `internal/service/chat_session_run_escalate.go` | 整个文件（常量迁移到 channel_ingress_background.go） |
| `internal/channel/preview/feishu_escalate_card.go` | 整个文件 |
| `internal/channel/preview/feishu_escalate_card_test.go` | 整个文件 |

**biz 层修改（6 个）**：

| 文件 | 变更内容 |
|------|----------|
| `internal/biz/session_run_phase_machine.go` | 移除 PhaseEscalating/PhaseEventEscalate，新增 PhaseEventUserEscalate，ParseSessionRunPhase 兼容映射 |
| `internal/biz/session_run.go` | 移除 SessionRunBudget/DefaultSessionRunBudget/SessionRunPhaseEscalating，StartInteractive 签名变更 |
| `internal/biz/session/status.go` | 移除 StatusReasonBudgetEscalated，新增 StatusReasonUserEscalated |
| `internal/biz/session/status_machine.go` | 转换规则中 StatusReasonBudgetEscalated → StatusReasonUserEscalated |
| `internal/biz/ingress_policy.go` | 移除 IngressRejectBusy/IngressSteer，新增 IngressInject，调整 EvaluateIngressPolicy 逻辑 |
| `internal/biz/channel_config_helpers.go` | 移除 budget 相关字段和方法，保留 DurableDeadlineSec |

**service 层修改（10 个）**：

| 文件 | 变更内容 |
|------|----------|
| `internal/service/chat_orch_session_run_lifecycle.go` | 移除 budget 回调/stopBudget/onSessionRunSoftBudget，新增 EscalateToDurableByUser，接口签名变更 |
| `internal/service/chat_orchestrator_turn_phases.go` | 移除 stopBudget 字段，turn timeout 改为通知 |
| `internal/service/chat_orchestrator_turn.go` | 移除 stopBudget，增大 context timeout，IngressRejectBusy 分支修改 |
| `internal/service/chat_orchestrator_durable.go` | 移除 stopBudget |
| `internal/service/session_run_escalation_notifier.go` | 移除 NotifySoftBudget |
| `internal/service/channel_ingress_policy.go` | 移除 reject_busy 分支，steer 改为 inject |
| `internal/service/channel_ingress_session.go` | 移除 PhaseEscalating case |
| `internal/service/channel_ingress_background.go` | 更新 HandleBackgroundCommand，迁移 channelBackgroundReply* 常量 |
| `internal/service/channel_ingress_card_action.go` | 更新飞书卡片回调处理 |
| `internal/service/channel_ingress_constants.go` | 更新 channelStatusPhaseTemplate |

**其他后端修改（8 个）**：

| 文件 | 变更内容 |
|------|----------|
| `internal/service/ingress_policy.go` | 同步移除 IngressSteer/IngressRejectBusy，新增 IngressInject |
| `internal/service/session_observability.go` | 移除 SoftBudgetSec/HardBudgetSec |
| `internal/service/chat.go` | ActiveSessionRunPhase 兼容映射 |
| `internal/data/session_run_repo.go` | 3 处 SQL 移除 escalating |
| `internal/agent/stream_consumer.go` | stuck tool 增加用户通知 |
| `internal/agent/activity_publish.go` | stuck tool 消息更友好 |
| `internal/runtime/run_registry.go` | Cancel 增加 reason 参数 |
| `internal/runtime/pending_queue.go` | 增加 Peek 方法 |
| `internal/biz/chat_usecase.go` | 新增 InterruptAndInjectMessage |
| `internal/biz/turn_gateway.go` | 确认接口兼容 |
| `cmd/admin/wire.go` | 修改（非移除）Wire 绑定 |

**新增文件（4 个）**：

| 文件 | 说明 |
|------|------|
| `internal/biz/escalation_suggester.go` | 多信号升级建议器 |
| `internal/biz/turn_interrupt.go` | Turn 中断逻辑封装 |
| `internal/service/channel_heartbeat.go` | IM 心跳保活 |
| `internal/channel/preview/feishu_durable_card.go` | 替代 feishu_escalate_card |

**Proto 修改（2 个）**：

| 文件 | 变更内容 |
|------|----------|
| `api/kratos/session/v1/session.proto` | soft_budget_sec/hard_budget_sec 标记 deprecated |
| `api/kratos/chat/v1/chat.proto` | 新增 InterruptAndInjectMessage RPC |

**Ent Schema 修改（1 个）**：

| 文件 | 变更内容 |
|------|----------|
| `internal/data/ent/schema/session_run.go`（需确认文件名） | soft_budget_sec/hard_budget_sec 字段添加 Deprecated 注释 |

#### 前端（17 个）

| 文件 | 变更类型 | 变更内容 |
|------|----------|----------|
| `web/src/features/chat/sessionRunStatus.ts` | 修改 | 移除 ESCALATING |
| `web/src/features/chat/jobFormatters.ts` | 修改 | 移除 escalating case |
| `web/src/components/chat/chatUi.ts` | 修改 | 移除 escalating case |
| `web/src/features/chat/useChatBackgroundJobs.ts` | 修改 | 移除 escalating |
| `web/src/components/sessions/SessionRunsPanel.vue` | 修改 | 移除 escalating |
| `web/src/components/sessions/SessionStatusBadge.vue` | 修改 | budget_escalated → user_escalated |
| `web/src/features/session/types.ts` | 修改 | 类型更新 |
| `web/src/components/chat/ChatPendingQueue.vue` | 重构 | 增加"立即发送"按钮 |
| `web/src/components/chat/ChatRunnerStatus.vue` | 修改 | 扩展 stop 按钮到 durable 状态 + 增加 pending 计数 badge |
| `web/src/features/chat/api.ts` | 修改 | 新增 interruptAndSend API |
| `web/src/features/chat/envelopeRunStatus.ts` | 修改 | 新增 escalating → running 兼容映射 |
| `web/src/features/chat/conversationEventDispatcher.ts` | 修改 | 新增 escalating 兼容映射 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | 修改 | 长时间无事件提示 |
| `web/src/features/chat/composables/useChatRunStatus.ts` | 确认 | 无变更，确认兼容 |
| `web/src/domain/conversation.ts` | 确认 | runStatusToTurnStatus 不含 escalating，无需修改 |
| `web/src/components/chat/ChatComposer.vue` | 修改 | 统一 Enqueue 入口，run 活跃时 placeholder 变化 |
| `web/src/components/chat/ChatEnqueueMessage.vue` | 删除 | 功能合并到 ChatComposer |
| `web/src/features/chat/composables/useFollowUpQueue.ts` | 修改 | session 切换时立即清空 pendingMessages |

#### 测试文件（8 个）

| 文件 | 变更类型 |
|------|----------|
| `internal/biz/session_run_budget_test.go` 相关 | 删除 |
| `internal/biz/session_run_phase_machine_test.go` | 修改（移除 escalating 测试用例，新增 user_escalate 测试） |
| `internal/service/chat_session_run_escalate_test.go` | 删除 |
| `internal/biz/channel_config_helpers_more_test.go` | 修改（移除 budget 相关测试） |
| `internal/biz/ingress_policy_test.go` | 修改（移除 reject_busy/steer 测试，新增 inject 测试） |
| `internal/service/ingress_policy_test.go` | 修改（移除 reject_busy/steer 测试，新增 inject 测试） |
| `internal/biz/session/status_machine_test.go` | 修改（StatusReasonBudgetEscalated → StatusReasonUserEscalated） |
| `web/src/features/chat/__tests__/conversationPresentation.spec.ts` | 确认（当前不涉及 escalating） |

---

## 六、实施顺序

### Phase 1：去掉时间预算（3.1 变更）

**原则**：先建后删——先实现 `EscalateToDurableByUser` 和心跳保活，再删除 budget 机制。

1. **新增** `EscalateToDurableByUser()` 方法（在 `chat_orch_session_run_lifecycle.go` 中）
2. **新增** `feishu_durable_card.go`（替代 `feishu_escalate_card.go`）
3. **新增** `channel_heartbeat.go`（IM 心跳保活）
4. **新增** `escalation_suggester.go`（多信号建议器）
5. **迁移** `channelBackgroundReply*` 常量到 `channel_ingress_background.go`
6. **更新** `channel_ingress_background.go`（`HandleBackgroundCommand` 改调 `EscalateToDurableByUser`）
7. **更新** `channel_ingress_card_action.go`（飞书卡片回调更新）
8. **简化** `session_run_phase_machine.go`（移除 escalating，新增 user_escalate，ParseSessionRunPhase 兼容映射）
9. **修改** `session_run.go`（移除 budget 相关，StartInteractive 签名变更）
10. **修改** `session/status.go` + `status_machine.go`（StatusReason 替换）
11. **修改** `chat_orch_session_run_lifecycle.go`（移除 budget 回调，接口签名变更）
12. **修改** `chat_orchestrator_turn_phases.go` + `chat_orchestrator_turn.go` + `chat_orchestrator_durable.go`（移除 stopBudget）
13. **修改** `channel_config_helpers.go`（移除 budget 字段）
14. **修改** `session_run_escalation_notifier.go`（移除 NotifySoftBudget）
15. **修改** `session_run_repo.go`（SQL 移除 escalating）
16. **修改** `channel_ingress_session.go`、`session_observability.go`、`chat.go`、`channel_ingress_constants.go`
17. **删除** `session_run_budget.go`、`chat_session_run_escalate.go`、`feishu_escalate_card.go`、`feishu_escalate_card_test.go`
18. **修改** `wire.go`（确认 Wire 绑定兼容）
19. **修改** Proto（标记 deprecated + 确认 Ent Schema 注释）
20. **修改** 前端（移除 escalating 状态 + 兼容映射）
21. **验证**：`make api && make wire && make build && make test && make lint`

### Phase 2：Session 并发锁改为 Pending 列表（3.2 变更）

**原则**：先建后删——先实现 `InterruptAndInjectMessage` 和 `IngressInject`，再移除 `IngressSteer`/`IngressRejectBusy`。

1. **新增** `turn_interrupt.go`（TurnInterruptReason + InterruptCurrentTurn）
2. **修改** `run_registry.go`（Cancel 增加 reason 参数）
3. **修改** `pending_queue.go`（增加 Peek 方法）
4. **新增** `InterruptAndInjectMessage` 到 `chat_usecase.go`
5. **修改** `ingress_policy.go`（新增 IngressInject，调整 EvaluateIngressPolicy）
6. **修改** `channel_ingress_policy.go`（steer → inject）
7. **修改** `chat_orchestrator_turn.go`（turn 中断时检查 pending 列表）
8. **新增** `InterruptAndInjectMessage` RPC（chat.proto）
9. **修改** 前端 `ChatPendingQueue.vue`（增加按钮）
10. **修改** 前端 `api.ts`（新增 API）
11. **移除** `IngressRejectBusy`/`IngressSteer`（biz + service 层同步）
12. **验证**：`make api && make wire && make build && make test && make lint && cd web && pnpm lint && pnpm test && pnpm build`

### Phase 3：去掉自动超时（3.3 变更）

1. **修改** turn timeout 逻辑（`chat_orchestrator_turn_phases.go`：改为通知而非 fail）
2. **修改** `chat_orchestrator_turn.go`（增大 context.WithTimeout 到 1 小时）
3. **修改** `stream_consumer.go`（stuck tool 增加用户通知）
4. **修改** `activity_publish.go`（stuck tool 消息更友好）
5. **修改** 前端 `ChatRunnerStatus.vue`（扩展 stop 按钮到 durable 状态）
6. **修改** 前端 `useChatWorkspace.ts`（长时间无事件提示）
7. **验证**：全量测试

---

## 七、向后兼容性

| 兼容项 | 处理方式 |
|--------|----------|
| DB `soft_budget_sec` / `hard_budget_sec` 列 | 保留不删，新写入 0 |
| Proto `soft_budget_sec` / `hard_budget_sec` 字段 | 标记 `deprecated`，不删除，字段号不变 |
| DB `phase='escalating'` 历史记录 | `ParseSessionRunPhase` 映射为 `PhaseDurable`；可选数据迁移 |
| DB `status_reason='budget_escalated'` 历史记录 | 前端兼容：显示为"已转后台" |
| 前端收到旧的 `escalating` 状态信封 | `runStatusFromEnvelope` 映射为 `running`；`turnStatusFromEnvelope` 同理 |
| `IngressRejectBusy` 决策 | 后端保留常量但不再使用（避免 Proto 变更），前端无需变更 |
| `IngressSteer` 决策 | 被 `IngressInject` 替代，后端移除常量 |
| `BusyInputMode` 配置 | 保留字段，`interrupt` 语义变更为"立即发送"（行为升级，不降级） |
| Durable Worker | 完全保留，仅触发方式变更 |
| Checkpoint 机制 | 完全保留 |
| `/background` 命令 | 保留，改为调用 `EscalateToDurableByUser` |
| 飞书卡片回调 | 新卡片模板（`feishu_durable_card.go`），包含"已转后台"状态展示 |
| `CancelRun` / `StopGeneration` | 已有实现，无需新增 |
| `sessionRunLifecycle` 接口签名变更 | 返回值从 3 个改为 2 个，所有调用方和 mock 需同步更新 |
| `StartInteractive` 签名变更 | 移除 budget 参数，所有调用方和 mock 需同步更新 |
| `Cancel` 签名变更 | 增加 reason 参数，所有调用方需更新 |

---

## 八、关键设计决策记录

| 决策 | 选择 | 理由 |
|------|------|------|
| 是否保留 stuck tool detection | 保留 | turn 结束时 tool_call 无 tool_result 是协议异常，必须清理 |
| 是否保留 orphan run cleanup | 保留 | 进程重启时清理是安全兜底，与超时无关 |
| 是否保留 firstByteTimeout | 保留 | 模型无首字节大概率是配置问题，提示合理 |
| 是否保留 turnTimeout | 保留但改为安全上限（1 小时） | 防止真正的无限挂起，但不作为业务截止时间 |
| 是否保留 durable 模式 | 保留 | 后台执行是合理需求，但触发方式改为用户主动 |
| "立即发送"是否中断当前 turn | 是 | 学习 Cursor，用户追加的内容需要立即生效 |
| 中断 turn 时是否保存部分回复 | 是 | 避免丢失已生成的内容 |
| IM 心跳频率 | 60 秒 | 平衡保活需求与消息频率 |
| `IngressSteer` 替代方案 | `IngressInterrupt`（v3 修正） | v2 的 `IngressInject` 试图 Cancel + 注入新消息，存在竞态。v3 改为 `IngressInterrupt`：Enqueue(priority=high) + Cancel，复用 processPendingQueue 流程，无竞态 |
| "立即发送"实现方式 | 标记优先级 + Cancel（v3 修正） | v2 试图 Cancel + 直接注入新消息到新 turn，存在 5 个竞态漏洞。v3 学习 Cursor 实际行为：Cancel 当前 turn → processPendingQueue 自动启动新 turn。复用已有流程，零竞态 |
| 心跳发送方式 | 流式预览 PATCH（非新消息） | 飞书不支持 typing indicator；发送新消息会造成消息轰炸；利用已有的 StreamSender PATCH 机制更新同一条消息，最优体验 |
| 前端 Enqueue 入口 | 统一为主输入框（v3 修正） | 当前两套入口（主输入框 + ChatEnqueueMessage）并存导致用户困惑。统一为主输入框，run 活跃时 placeholder 变化提示 |
| Pending 消息感知 | ChatRunnerStatus 增加 badge | Pending 列表在消息底部不够显眼，在 run 状态旁显示计数 badge 提升感知 |
| Durable turn 中断保护 | 拒绝中断 | Durable turn 被中断后上下文丢失不可恢复，InterruptAndSendMessage 应拒绝中断 durable 状态的 run |
| `context_pressure` 场景处理 | `IngressQueue`（低优先级） | 不 reject 用户消息，但标记为低优先级，turn 完成后检查 context 压力决定是否执行 |
| DB 历史 `escalating` 记录映射 | `PhaseDurable` | 语义最接近：已从交互模式转出 |
| `ParseSessionRunPhase` 兼容策略 | default 分支中特殊处理 | 不在 switch case 中保留 escalating（避免新代码误用），但在 default 中做兼容映射 |
| Wire 绑定处理 | 修改而非移除 | `SessionRunEscalationNotifier` 接口仍存在（方法减少），其 Wire 绑定必须保留 |

---

## 九、编译错误预防清单

以下为实施时**必定会遇到**的编译错误，按修复顺序排列：

| 优先级 | 错误 | 位置 | 修复方式 |
|--------|------|------|----------|
| P0 | undefined: `StartBudgetWatcher` / `BudgetPhaseCallbacks` | `chat_orch_session_run_lifecycle.go:151` | 删除引用 |
| P0 | undefined: `SessionRunBudget` | `chat_orch_session_run_lifecycle.go:115`, `session_run.go:34-36` | 删除类型和引用 |
| P0 | undefined: `SessionRunPhaseEscalating` / `PhaseEscalating` | `session_run_phase_machine.go`, `channel_ingress_session.go:211`, `chat_orch_session_run_lifecycle.go`, `session_run_repo.go:326` | 删除常量和引用 |
| P0 | undefined: `PhaseEventEscalate` | `session_run_phase_machine.go:49,59` | 删除常量和转换规则 |
| P0 | undefined: `StatusReasonBudgetEscalated` | `chat_orch_session_run_lifecycle.go:313`, `status_machine.go` | 替换为 `StatusReasonUserEscalated` |
| P1 | `StartInteractive` 参数数量不匹配 | `chat_orch_session_run_lifecycle.go:116` | 移除 budget 参数 |
| P1 | `RunPolicy()` undefined | `chat_orch_session_run_lifecycle.go:115` | 删除调用 |
| P1 | `BeginSessionRunLifecycle` 返回值数量不匹配 | 所有调用方 | 从 3 个返回值改为 2 个 |
| P1 | `EscalateSessionRunToDurable` undefined | `chat_session_run_escalate.go` 调用方 | 替换为 `EscalateToDurableByUser` |
| P1 | `channelBackgroundReply*` undefined | `channel_ingress_card_action.go`, `chat_session_run_cancel.go` | 常量迁移到 `channel_ingress_background.go` |
| P2 | `IngressRejectBusy` / `IngressSteer` undefined | `channel_ingress_policy.go`, `chat_orchestrator_turn.go:91`, `service/ingress_policy.go` | 移除/替换为 IngressInterrupt |
| P2 | `stopBudget` undefined | `chat_orchestrator_turn_phases.go`, `chat_orchestrator_turn.go:383`, `chat_orchestrator_durable.go:82` | 全部移除 |
| P2 | `Cancel` 参数数量不匹配 | `chat_orch_session_run_lifecycle.go:309`, `chat_usecase.go:142` | 增加 reason 参数 |
| P2 | SQL 中 `'escalating'` 字符串残留 | `session_run_repo.go:277,326,443` | 修改 SQL |
| P3 | 测试编译失败 | budget_test, escalate_test, phase_machine_test, status_machine_test | 删除/重写 |
