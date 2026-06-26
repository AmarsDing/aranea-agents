# ADR-02: Activity-First 事件持久化策略（WBPF → 并行异步 + Dead-Letter 补偿）

## 状态：已接受

## 背景

Aranea-Agents 的 Chat 模块重构（详见 `docs/reports/2026-06-25-analysis-chat-module-refactor.md`）引入了 Activity-First（AF）架构：后端将运行时事件投影为 Activity 语义单元，前端零推断消费。

AF 之前，Critical 级事件（Error/Checkpoint/RunnerCompletion）使用 **WBPF（Write-Before-Publish-Fanout）** 模式（见 `internal/event/contract/reliability.go`）：事件必须先持久化到 WAL，成功后才向 Bus 发布。该模式保证「持久化成功 ↔ 推送成功」的强一致，但带来两个问题：

1. **DB I/O 阻塞 WS 推送**：用户感知延迟。postgres 单写连接（`MaxOpenConns=1`）下，持久化可能耗时数十至数百毫秒，期间前端无法收到实时事件。
2. **持久化失败导致推送被跳过**：前端丢失实时事件，只能依赖 API reload 补全，体验降级。

AF 引入后，Activity 事件流是前端渲染的唯一来源，WBPF 的阻塞问题被放大：
- 一个 turn 可能产生 10+ 个 Activity 事件（thinking start/delta×N/done, action start/done, reply start/delta×N/done, turn end）
- 每个事件都阻塞在 DB I/O 上，累计延迟可达数秒
- 串行 BlockUpTo 导致 per-activity FIFO 通道吞吐量受限

## 决策

### D1：Activity 事件采用并行异步持久化（替代 WBPF）

Activity 事件流不再使用 WBPF，改为 **并行异步** 模式（见 [activity_event_sequencer.go:300-316](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go#L300-L316)）：

```
processTask(activityID, task):
  1. 持久化：投递到 persistChan（buffered channel, 非阻塞）
     - channel 满时回退到同步 persistWithRetry（极端场景兜底）
  2. 推送：同步调用 eventBus.Publish（保留 per-activity FIFO）
```

- **持久化 fire-and-forget**：consume goroutine 不等待持久化完成，吞吐量由 WS 推送延迟（~5ms）决定，而非 DB I/O。
- **推送同步**：WS 推送通常 < 5ms，同步执行保留 per-activity FIFO 顺序，无需额外协调。
- **persist worker 独立 goroutine**：从 `persistChan` 消费，FIFO 处理，保证 start→done 顺序。

### D2：持久化失败补偿三重保障

1. **重试预算**（[activity_event_sequencer.go:38-39](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go#L38-L39)）：`persistMaxRetries=5`，`persistInitialBackoffMs=100`，指数退避（100/200/400/800/1600ms），总预算 3100ms。对齐 postgres `busy_timeout=30000ms` 的 1/10，避免占用写连接过久。
2. **Dead-Letter 环形缓冲**（[activity_event_sequencer.go:357-367](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go#L357-L367)）：重试耗尽后，失败的 Activity 进入 `deadLetter` 环形缓冲（容量 512，FIFO 淘汰）。通过 `ListDeadLetterActivities(sessionID)` 暴露给 WS 重连补偿路径。
3. **API Backfill**：前端在 WS 重连或显式 reload 时，通过 `listActivities(sessionId)` API 拉取最新持久化状态，作为最终一致兜底。

### D3：OnError 语义重构（删除 ActivityKindError）

旧模型：错误产生独立的 `ActivityKindError` Activity，与 root task Activity 并行存在，前端需合并两者。

新模型（[activity_projector.go:856-914](file:///f:/aranea-agents/internal/agent/activity_projector.go#L856-L914)）：
- **存在 root task**：将 root task Activity 转换为 `status=failed`，错误信息存入 `Meta.error_message/error_type/error_code`。
- **无 root task**：创建一个最小化的 failed task Activity 兜底。
- **OnTurnEnd 终态保护**（[activity_projector.go:1349-1368](file:///f:/aranea-agents/internal/agent/activity_projector.go#L1349-L1368)）：若 root 已是终态（Failed/Cancelled/Interrupted），OnTurnEnd 不覆盖状态，仅附加 token usage。

这统一了「turn 失败」的表达：`task.failed` 即代表整 turn 失败，无需 parallel error kind。

### D4：Legacy ActivityKind 清理

删除后端常量（[activity.go](file:///f:/aranea-agents/internal/biz/activity.go)）：
- `ActivityKindSubTaskBoard`（前端不再使用 sub_task_board 渲染）
- `ActivityKindError`（被 D3 的 task.failed 模型替代）
- `ActivityKindDelegate`（OnDelegate 方法删除，无调用方）

保留 `ChildBoardId` 字段以兼容 DB/proto，但不再有写入路径。

前端类型（[activityTypes.ts](file:///f:/aranea-agents/web/src/features/chat/activityTypes.ts)）同步移除 `'sub_task_board' | 'error' | 'delegate'`。

## 后果

### 正面

- **延迟降低 10x+**：consume 不再阻塞在 DB I/O，吞吐量由 WS 推送决定（~5ms/event vs 旧 ~50-200ms/event）。
- **persist 失败不阻塞 UI**：前端仍有实时事件，dead-letter + API backfill 保证最终一致。
- **错误模型简化**：单一 `task.failed` 表达 turn 失败，消除 parallel error kind 的合并复杂度。
- **状态机一致性**：OnTurnEnd 终态保护避免 Failed 被覆盖为 Completed 的状态机违规。

### 负面

- **临时不一致窗口**：persist 失败时，前端实时状态与 DB 不一致，最长持续到下次 API backfill（通常 < 5s，极端情况依赖 dead-letter 重放）。
- **Dead-letter 容量有限**：512 条，超出后 FIFO 淘汰最旧记录。极端场景（DB 长时间不可用）可能丢失部分 persist 失败记录。
- **persist worker 单线程**：per-activity FIFO 要求串行处理，无法水平扩展。高并发 turn（多 agent 同时运行）下可能成为瓶颈。

### 风险缓解

| 风险 | 缓解措施 |
|------|---------|
| Dead-letter 丢失 | 容量 512 覆盖单 turn 最多 ~50 事件的 10 倍冗余；API backfill 作为最终兜底 |
| persist worker 瓶颈 | 监控 `persistChan` 满载率；超过阈值时告警，可横向扩展为 per-session worker pool |
| 临时不一致 | 前端关键操作（如 token 用量统计）以 API 数据为准，不依赖实时事件 |

## 替代方案

### A1：保留 WBPF（Status Quo）

- 优点：强一致，无补偿机制复杂度
- 缺点：DB I/O 阻塞 WS 推送，AF 场景下累计延迟不可接受
- 否决原因：AF 引入后事件密度上升 10x+，WBPF 的阻塞问题被放大

### A2：完全 fire-and-forget（无重试无 dead-letter）

- 优点：实现最简，consume 永不阻塞
- 缺点：persist 失败即数据丢失，无补偿路径
- 否决原因：违反 AS-EVT-01 的 Critical 事件可靠性要求

### A3：sync.WaitGroup 等待 persist + publish 双 goroutine

- 优点：persist 失败可感知，consume 仍比串行快
- 缺点：consume 仍等 max(persist, publish)，吞吐量提升有限（~2x）
- 否决原因：收益不足以抵消复杂度

### A4：per-session persist worker pool

- 优点：水平扩展，解决单 worker 瓶颈
- 缺点：per-activity FIFO 跨 worker 难以保证；实现复杂度高
- 否决原因：当前单 worker 未达瓶颈，过早优化

## 适用范围

- **本 ADR 适用**：Activity 事件流（`biz.ActivityEventBus`，AF 架构）
- **不适用**：Legacy envelope-based Critical 事件（Error/Checkpoint）仍使用 WBPF，直到 legacy envelope 系统完全退役（见 Phase F defer 说明）

## 关联文档

- 重构主文档：[`docs/reports/2026-06-25-analysis-chat-module-refactor.md`](file:///f:/aranea-agents/docs/reports/2026-06-25-analysis-chat-module-refactor.md) §5（持久化与推送解耦）
- 架构评判标准：`project_rules.md` §AS-EVT-01（事件可靠性分级）
- 代码锚点：
  - [activity_event_sequencer.go](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go) — persist worker + retry + dead-letter
  - [activity_projector.go](file:///f:/aranea-agents/internal/agent/activity_projector.go) — OnError 重构 + OnTurnEnd 终态保护
  - [activity.go](file:///f:/aranea-agents/internal/biz/activity.go) — ActivityKind 常量清理

## 遗留项（defer 到后续迭代）

以下重构因影响面大、需独立迁移而 defer，不阻塞本 ADR 落地：

1. **Legacy spirit 面板删除**（TaskExecutionPanel.vue / MemberReadOnlyPanel.vue）：仍服务于 team/member 模式，需先完成 AF 管线对 team/member 模式的覆盖后方可删除。
2. **Legacy envelope 文件删除**（envelope.ts / useEnvelopeStream.ts / inboundSyncRouting.ts 等）：59+ 文件仍依赖 envelope 类型，需逐步迁移至 ActivityEvent 后方可整体删除。
3. **event_store/event_wal 表删除**：DDL 迁移已就绪（`20260901_drop_event_store_subsystem.sql`），但需等待 legacy envelope 系统完全退役后方可执行。
