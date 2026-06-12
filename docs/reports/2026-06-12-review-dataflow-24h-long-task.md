# 数据流全链路审查与 24H 长任务专项解决方案

> **文档类型**：架构评审 + 专项解决方案（含影响分析与修正）
> **审查范围**：Chat UI → WebSocket → Service → Biz → Agent Runtime → LLM → 事件回传 → UI 显示
> **核心目标**：验证数据流逻辑正确性，识别架构设计缺陷，确保 24H 连续长任务可行性
> **审查日期**：2026-06-12

---

## 一、问题重评估与根因分类

经过逐行代码验证，原始 14 个阻断项中：

| 原始 ID | 验证结果 | 说明 |
|---------|---------|------|
| B01 | **确认，Critical** | `MarkOrphanedRunsCancelled` WHERE 包含 `durable`，100% 复现 |
| B02 | **确认，High** | writePump drain 无迭代上限，高频场景可饿死 ping |
| B03 | **确认，High** | BlockUpTo 100ms 后 DropOldest，Critical 事件实时投递可丢 |
| B04 | **确认，High** | 服务器关闭无 graceful drain |
| B05 | **降级为 Medium** | closeSend 后 readPump 60s 内退出并清理，泄漏窗口有限 |
| B06 | **确认，Medium** | RunRegistry 3 个 sync.Map，违反 AS-COG-01 |
| B07 | **确认，High** | Runner.Run 无 MaxRunDuration 兜底 |
| B08 | **确认，Critical** | Claim stale 300s vs 执行 86400s，比例 1:288 |
| B09 | **确认，Critical** | WS 断连 → connCtx 取消 → turn 直接 Fail，无 durable 升级 |
| B10 | **确认，Medium** | pendingQueue 重连耗尽后不清理且继续追加 |
| B11 | **确认，Medium** | Buffer 200 条/30min 硬编码，高频场景不足 |
| B12 | **撤回** | deliverDropOldestLocked 在 sub.mu 锁内执行，无竞态 |
| B13 | **确认，Medium** | WAL 无自动 Purge |
| B14 | **确认，Medium** | messagesBySession 无 LRU，pendingQueue 无限增长 |

**根因归类**：上述问题不是孤立的代码 bug，而是**三个系统性架构缺陷**的表现：

| 根因 | 表现问题 | 核心矛盾 |
|------|---------|---------|
| **RC-1: Turn 生命周期与传输连接耦合** | B02, B09, B05 | Turn 的 context 绑定在 WS connCtx 上，连接断开 = 任务终止 |
| **RC-2: 事件投递模型缺乏可靠性分层执行** | B03, B04, B11, B13 | Bus 层 Critical/Important 共享同一投递路径，BlockUpTo 退化后无差异化保障 |
| **RC-3: Durable 机制不完整** | B01, B08, B07 | Durable 是"半成品"——升级路径存在，但恢复路径有致命缺陷 |

---

## 二、影响分析：方案副作用与遗漏问题

对三个根因方案 + 前端方案的逐行代码走查，发现 **7 个方案自身引入的新问题** 和 **4 个原审查遗漏问题**。以下问题已纳入修正方案（第三~六章），不再单独列出修复步骤。

### 2.1 方案引入的新问题

#### N-01: BlockUntilAcked 导致 Publish goroutine 永久阻塞 [Critical]

**关联方案**：RC-2（Critical 事件改为 BlockUntilAcked）

**问题**：如果 WS subscriber channel（BufferSize=256）满且消费者（eventPump）卡住，`deliverBlockUntilAcked` 将永久阻塞。由于 `stream_consumer` 是同步顺序调用 `Publish`，一个 Critical 事件阻塞会导致整个 turn 的流式消费停滞。

**级联链**：
```
网络慢 → writePump 卡在 WriteMessage → high 队列满
  → eventPump 卡在 enqueue → subscriber channel 满
  → BlockUntilAcked 阻塞 → stream_consumer 停滞
  → 整个 turn 卡死
```

**WBPF 交互**：`wal.go` 中 `WriteBeforePublish` 的 `publish()` 回调阻塞后，`markPublished()` 永远不执行，WAL 中事件处于 `published=0`。重启后 `Recover()` 会重复发布。

**修正**：BlockUntilAcked 加 30s 安全阀 + unhealthy 标记；WBPF publish 加 10s 超时，超时仍 markPublished 防重复。

#### N-02: Important 事件走 high 队列导致连接断开风险 [High]

**关联方案**：RC-2（Important 事件走 high 队列）

**问题**：high 队列容量仅 64，使用 block+5s timeout 策略。Important 事件频率远高于原有 high 事件，burst 场景下可能填满 high 队列，触发 5s 超时 → 连接断开 → 重连风暴。

**修正**：新增 `important` 四级优先级（容量 128，DropNewest 策略），不将 Important 事件路由到 high 队列。

#### N-03: Important 事件 BlockUpTo(500ms) 导致流式卡顿 [Medium]

**关联方案**：RC-2（Important 事件 BlockUpTo 从 100ms 提升到 500ms）

**问题**：`stream_consumer` 同步顺序调用 `Publish`，一个 session 最多 6 个 subscriber，**最坏情况单次 Publish 阻塞 3 秒**。

**修正**：Important 事件 BlockUpTo 降为 200ms；长期优化为并行投递 subscriber。

#### N-04: MarkPhase 不经过状态机校验，竞态时可覆盖终态 [Critical]

**关联方案**：RC-3（Durable 闭环）+ RC-1（auto-escalate）

**问题**：`MarkPhase` 直接调用 `repo.UpdatePhase`，不检查当前 phase 是否允许转换。竞态场景下 `Complete()` 先执行后，`MarkPhase(durable)` 会将已完成的 run 覆盖为 durable。

**修正**：将 `MarkPhase` 改为 `TransitionPhase(from, to)`，使用 CAS 保证转换合法性。

#### N-05: Auto-escalate 后 FinishSessionRunLifecycle 将 durable run 标记为 failed [High]

**关联方案**：RC-1（WS 断连时 auto-escalate）

**问题**：auto-escalate 触发 `runs.Cancel()` → turnCancel() → turn 返回 error → `FinishSessionRunLifecycle(turnErr != nil)` → `Fail()`。但 run 已被标记为 durable，`Fail()` 会覆盖 durable 状态。当前虽有 `cur.Phase == durable` 保护，但存在 TOCTOU 竞态。

**修正**：使用 CAS（TransitionPhase）替代 Get+Fail 的两步操作。

#### N-06: MaxRunDuration 硬编码 30 分钟截断 Channel 长任务 [High]

**关联方案**：RC-3（Runner 强制 MaxRunDuration=30min）

**问题**：Channel 可配置 `TurnTimeoutSec=86400`（24 小时），但 MaxRunDuration 硬编码 30 分钟会在 30 分钟时强制终止 Runner。Durable resume 的 deadline 也是 86400s，30 分钟的 MaxRunDuration 与之矛盾。

**修正**：MaxRunDuration 从 Channel 配置读取，30min 仅作最终兜底。

#### N-07: 前端 LRU 驱逐当前活跃会话导致 UI 空白 [Critical]

**关联方案**：前端 SessionResourceManager

**问题**：如果 ResourceManager 在后台驱逐了用户正在查看的会话，`displayMessages` 立即返回空数组，UI 瞬间清空所有消息。流式传输中途驱逐更会导致快照丢失。

**修正**：增加驱逐守卫——禁止驱逐当前活跃会话 + 禁止驱逐正在流式传输的会话；useChatInboundSync 暴露 `evictSessionResources` 方法。

### 2.2 原审查遗漏问题

#### M-01: CheckpointID 为空的 durable run 永远不被清理 [Medium]

**位置**：`internal/service/session_run_durable_worker.go`

**问题**：`processOnce` 中跳过无 checkpoint 的 durable run，但不标记为 failed。修复 B01（排除 durable 不被孤儿清理）后，这些异常 run 永远不会被清理。

**修正**：在 `processOnce` 中增加清理逻辑——无 checkpoint 的 durable run 标记为 failed。

#### M-02: Hard budget grace period 60s 可能强制 fail 已升级的 durable run [High]

**位置**：`internal/biz/session_run_budget.go`

**问题**：Hard budget 触发后，如果 checkpoint 创建耗时超过 60s，grace period goroutine 会在 run 变为 durable 之前强制 fail。

**修正**：grace period goroutine 应在检测到 `phase=durable` 时退出，而非仅检查终态。

#### M-03: Durable resume 无重试策略 [Medium]

**位置**：`internal/service/chat_durable_resume.go`

**问题**：如果 resume 因临时性错误失败，run 直接被标记为 failed，不重试。

**修正**：添加显式重试策略（最大 3 次 + 指数退避），`session_runs` 表增加 `resume_attempts` 列。

#### M-04: interactive → durable 直接跳转不合法 [Medium]

**位置**：`internal/biz/session_run_phase_machine.go`

**问题**：状态机中没有 `interactive → durable` 的直接转换规则，必须经过 `interactive → escalating → durable`。但 auto-escalate（WS 断连时）需要快速升级。

**修正**：在状态机中增加 `interactive → durable` 的直接转换规则。

---

## 三、RC-1: Turn 生命周期与传输连接解耦

### 3.1 问题本质

当前架构中，Turn 的执行 context 派生自 WS 连接的 `connCtx`：

```
WS connCtx ──→ context.WithTimeout(connCtx, TurnTimeout) ──→ Turn 执行
     │
     └── 客户端断连 → connCancel() → Turn context 取消 → 任务终止
```

**根本矛盾**：Turn 是**业务概念**（一次 Agent 推理循环），WS 是**传输概念**（客户端连接）。两者生命周期不应耦合。

### 3.2 解决方案：Turn Context 独立于传输层

#### 3.2.1 架构变更

```
                         ┌──────────────────────┐
                         │   TurnContextManager  │
                         │  (per-session state)  │
                         └──────────┬───────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
    ┌─────────▼──────────┐ ┌───────▼────────┐ ┌─────────▼──────────┐
    │  Interactive Mode   │ │ Durable Mode   │ │  Background Mode   │
    │  ctx = turnCtx      │ │ ctx = bgCtx    │ │  ctx = bgCtx       │
    │  (WS-aware cancel)  │ │ (24H timeout)  │ │  (24H timeout)     │
    └─────────┬──────────┘ └───────┬────────┘ └─────────┬──────────┘
              │                     │                     │
              │    WS 断连触发      │    硬预算触发       │    Channel 入口
              │    auto-escalate    │    auto-escalate    │    天然解耦
              └─────────────────────┴─────────────────────┘
```

**核心变更**：Turn 不再直接使用 `connCtx`，而是通过 `TurnContextManager` 获取 context。`TurnContextManager` 根据当前模式返回不同的 context：

- **Interactive 模式**：Turn context 仍与 WS 连接关联，但**断连时触发 auto-escalate 而非直接取消**
- **Durable 模式**：Turn context 独立于任何连接，使用 `context.Background()` + 24H 超时
- **Background 模式**：Channel 入口天然解耦

#### 3.2.2 具体实现

**Step 1：引入 `TurnContextManager`**

```go
// internal/runtime/turn/context_manager.go

type TurnMode string

const (
    TurnModeInteractive TurnMode = "interactive"
    TurnModeDurable     TurnMode = "durable"
    TurnModeBackground  TurnMode = "background"
)

type TurnContextManager struct {
    runs         RunRegistry
    sessionRuns  biz.SessionRunWriter
    budgetConfig func(sessionID string) BudgetConfig
    lg           loggateway.Logger
}

// AcquireTurnContext 返回 turn 执行 context 和模式
// WS 入口传入 connCtx 用于感知断连，但不直接作为 turn 的 parent
func (m *TurnContextManager) AcquireTurnContext(
    parentCtx context.Context,  // 可能是 connCtx 或 background
    sessionID string,
    mode TurnMode,
) (ctx context.Context, cancel context.CancelFunc, mode TurnMode) {

    switch mode {
    case TurnModeInteractive:
        // Interactive: 使用独立 context，但监听 parent 取消信号
        ctx, cancel = context.WithTimeout(context.Background(), m.turnTimeout(sessionID))
        // 启动断连监听器
        go m.watchDisconnect(parentCtx, ctx, cancel, sessionID)
        return ctx, cancel, TurnModeInteractive

    case TurnModeDurable, TurnModeBackground:
        // Durable/Background: 完全独立
        deadline := m.durableDeadline(sessionID)
        ctx, cancel = context.WithTimeout(context.Background(), deadline)
        return ctx, cancel, mode
    }
}

// watchDisconnect 监听 WS 断连，触发 auto-escalate 而非直接取消
// 修正(N-05)：断连后 30s 宽限期内未 escalate 成功则强制取消
func (m *TurnContextManager) watchDisconnect(
    connCtx context.Context,
    turnCtx context.Context,
    turnCancel context.CancelFunc,
    sessionID string,
) {
    select {
    case <-connCtx.Done():
        // WS 断连 → 尝试 auto-escalate 到 durable
        escalated, err := m.tryAutoEscalate(turnCtx, sessionID)
        if err != nil || !escalated {
            // 升级失败 → 30s 宽限期后取消 turn
            m.lg.Warn("auto-escalate failed, starting grace period",
                "sessionID", sessionID, "error", err)
            select {
            case <-time.After(30 * time.Second):
                m.lg.Warn("grace period expired, cancelling turn", "sessionID", sessionID)
                turnCancel()
            case <-turnCtx.Done():
            }
        }
        // 升级成功 → turn 继续执行（context 不变，只是模式切换）

    case <-turnCtx.Done():
        // Turn 正常结束或超时，无需操作
    }
}

// tryAutoEscalate 尝试将 interactive run 升级为 durable
// 修正(N-04/N-05)：使用 TransitionPhase CAS 替代 MarkPhase
func (m *TurnContextManager) tryAutoEscalate(
    ctx context.Context, sessionID string,
) (bool, error) {
    if !m.runs.HasActive(sessionID) {
        return false, nil
    }
    // 使用 CAS: interactive → durable（M-04：状态机已增加直接转换规则）
    ok, err := m.sessionRuns.TransitionPhase(ctx, sessionRunID,
        biz.SessionRunPhaseInteractive, biz.SessionRunPhaseDurable)
    if !ok {
        // CAS 失败，run 可能已终态或已升级
        return false, nil
    }
    // 创建 checkpoint
    // ...
    return true, nil
}
```

**Step 2：修改 `handleUserMessage`**

```go
// internal/server/ws_message_handler.go

func (s *WSServer) handleUserMessage(wc *wsConn, up wsUpstream) {
    connCtx := wc.contextOrBackground()

    safego.Go(appctx.Ctx(), "ws-user-message", func() {
        // 不再: ctx, cancel := context.WithTimeout(connCtx, turnTimeout)
        // 改为: 通过 TurnContextManager 获取独立 context
        ctx, cancel, mode := s.turnCtxMgr.AcquireTurnContext(
            connCtx, wc.sessionID, runtime.TurnModeInteractive,
        )
        defer cancel()

        input := s.buildTurnInput(ctx, wc, up)
        if err := s.turnExecutor.ExecuteTurn(ctx, input); err != nil {
            // 错误处理...
        }
    })
}
```

**Step 3：修改 `FinishSessionRunLifecycle`**

```go
// internal/service/chat_orch_session_run_lifecycle.go
// 修正(N-05)：使用 CAS 替代 Get+Fail，避免 TOCTOU 竞态

func (l *chatSessionRunLifecycle) FinishSessionRunLifecycle(
    ctx context.Context, sessionID, sessionRunID string, turnErr error,
) {
    l.runStatus.DeleteBinding(sessionID)

    if turnErr != nil {
        // 尝试 CAS: interactive → failed
        ok, _ := l.sessionRuns.TransitionPhase(ctx, sessionRunID,
            biz.SessionRunPhaseInteractive, biz.SessionRunPhaseFailed)
        if !ok {
            ok2, _ := l.sessionRuns.TransitionPhase(ctx, sessionRunID,
                biz.SessionRunPhaseEscalating, biz.SessionRunPhaseFailed)
            if !ok2 {
                // phase 已是 durable 或终态，不 Fail
                l.lg.Info("skip Fail: run already in non-failable phase",
                    "sessionRunID", sessionRunID)
                return
            }
        }
        // CAS 成功，记录错误信息
        l.sessionRuns.UpdateErrorMessage(ctx, sessionRunID, turnErr.Error())
        return
    }
    // 正常完成逻辑...
}
```

**Step 4：状态机增加 `interactive → durable` 直接转换（M-04）**

```go
// internal/biz/session_run_phase_machine.go

var sessionRunPhaseTransitionRules = []shared.TransitionRule[SessionRunPhase, SessionRunPhaseEvent]{
    // ... 原有规则 ...
    // 新增：interactive 可直接升级为 durable（WS 断连 auto-escalate 场景）
    {From: PhaseInteractive, Event: PhaseEventDurable, To: PhaseDurable},
}
```

#### 3.2.3 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B09 | WS 断连时 auto-escalate 到 durable，而非直接取消 turn |
| B02 | Turn context 独立于 connCtx，即使 ping 饿死导致连接断开，turn 仍可继续执行 |
| B05 | Turn context 不依赖 connCtx，closeSend 不影响 turn 执行 |
| N-04 | TransitionPhase + CAS 替代 MarkPhase，防止终态覆盖 |
| N-05 | FinishSessionRunLifecycle 使用 CAS，auto-escalate 后不会 Fail 覆盖 durable |
| M-04 | 状态机增加 interactive → durable 直接转换 |

#### 3.2.4 对 writePump ping 饥饿的补充修复

即使 Turn 解耦后，writePump 的 ping 饥饿仍需修复（连接稳定性问题）：

```go
// internal/server/ws_io_pump.go — writePump drain 循环增加迭代保护

func (s *WSServer) writePump(wc *wsConn) {
    ticker := time.NewTicker(cfg.PingPeriod)
    // ...

    for {
        // drain 循环增加迭代计数器
        drainIter := 0
        maxDrainIter := 1000  // 每轮最多 drain 1000 条消息

        for drainIter < maxDrainIter {
            data, prio, ok := wc.queues.drainSelect()
            if !ok {
                break
            }
            drainIter++

            // 写消息到 WS...
            // ...（原有逻辑）

            // 每 100 次迭代检查是否该发 ping
            if drainIter%100 == 0 {
                select {
                case <-ticker.C:
                    wc.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(cfg.WriteWait))
                default:
                }
            }
        }

        // 阻塞 select（原有逻辑）
        select {
        case _, ok := <-wc.send:
            // ...
        case <-ticker.C:
            // ping
        case <-bpTicker.C:
            // backpressure
        }
    }
}
```

---

## 四、RC-2: 事件投递可靠性分层执行

### 4.1 问题本质

当前事件投递模型的核心矛盾：

```
Critical 事件 (ToolResult/Error/RunnerCompletion/Checkpoint)
    │
    ├── WBPF: 先写 WAL 再发布 ✓
    ├── Bus: BlockUpTo 100ms → DropOldest ✗  (实时投递可丢)
    ├── WS: normal 队列 DropNewest ✗          (投递可丢)
    └── Buffer: 200 条/30min ✗                (重连补发不足)

Important 事件 (RunStatus/StateDelta/TokenUsage/...)
    │
    ├── WBPF: 不走 WAL ✗                      (崩溃不可恢复)
    ├── Bus: BlockUpTo 100ms → DropOldest ✗   (同 Critical)
    └── WS: normal 队列 DropNewest ✗

Informational 事件 (TextDelta/FlowLog/...)
    │
    ├── WBPF: 不走 WAL (合理)
    ├── Bus: DropNewest (合理)
    └── WS: low 队列 DropNewest (合理)
```

**根本矛盾**：事件可靠性分级在**分类**层面已实现，但在**投递执行**层面没有差异化。

### 4.2 解决方案：三级投递管道

#### 4.2.1 架构变更

```
                    ┌─────────────────────────────────┐
                    │         EventBus.Publish         │
                    └────────────┬────────────────────┘
                                 │
                    ┌────────────▼────────────────────┐
                    │     ReliabilityRouter            │
                    │  (按事件分级路由到不同管道)        │
                    └──┬──────────┬──────────┬────────┘
                       │          │          │
            ┌──────────▼──┐ ┌────▼──────┐ ┌─▼───────────┐
            │ Critical    │ │ Important │ │ Informational│
            │ Pipeline    │ │ Pipeline  │ │ Pipeline     │
            │             │ │           │ │              │
            │ WBPF+       │ │ WBPF+     │ │ 尽力而为     │
            │ BlockUntil  │ │ BlockUpTo │ │ DropNewest   │
            │ Acked       │ │ 200ms     │ │              │
            │ (30s安全阀) │ │           │ │              │
            └──────┬──────┘ └─────┬─────┘ └──────┬──────┘
                   │              │               │
            ┌──────▼──────┐ ┌────▼──────┐ ┌──────▼──────┐
            │ WS: high    │ │ WS:       │ │ WS: low     │
            │ queue(64)   │ │ important │ │ queue(256)  │
            │             │ │ queue(128)│ │             │
            └─────────────┘ └───────────┘ └─────────────┘
```

#### 4.2.2 具体实现

**Step 1：Bus 层投递策略按可靠性分级**

```go
// internal/event/bus.go — 修改 deliverToSubscriber

func (b *bus) deliverToSubscriber(sub *subscriber, env Envelope, requiresBlockUpTo bool) {
    reliability := contract.ClassifyEventReliability(env.Type)

    switch reliability {
    case contract.ReliabilityCritical:
        // Critical: BlockUntilAcked + 30s 安全阀（修正 N-01）
        b.deliverBlockUntilAcked(sub, env)

    case contract.ReliabilityImportant:
        // Important: BlockUpTo 200ms（修正 N-03：从 500ms 降为 200ms）
        b.deliverBlockUpTo(sub, env, 200*time.Millisecond)

    case contract.ReliabilityInformational:
        // Informational: DropNewest — 满即丢
        b.deliverDropNewest(sub, env)
    }
}

// deliverBlockUntilAcked 阻塞直到成功投递或 subscriber 关闭
// 修正(N-01)：加 30s 安全阀，防止 goroutine 永久阻塞
const maxBlockUntilAcked = 30 * time.Second

func (b *bus) deliverBlockUntilAcked(sub *subscriber, env Envelope) {
    sub.mu.Lock()
    defer sub.mu.Unlock()
    if sub.closed {
        return
    }
    // 先尝试直接写入
    select {
    case sub.ch <- env:
        return
    default:
    }
    // 阻塞等待，带安全阀
    timer := time.NewTimer(maxBlockUntilAcked)
    defer timer.Stop()
    select {
    case sub.ch <- env:
    case <-sub.done:
        b.logDrop(env, "subscriber_closed")
    case <-timer.C:
        // 超时降级为 DropOldest
        b.deliverDropOldestLocked(sub, env)
        // 标记 subscriber 为 unhealthy
        sub.unhealthy.Store(true)
    }
}
```

**Step 2：WBPF publish 超时保护（N-01）**

```go
// internal/event/wal.go — WBPF publish 加超时

func (w *EventWAL) WriteBeforePublish(ctx context.Context, env contract.Envelope, publish func()) error {
    if !IsCriticalWBPFType(env.Type) {
        publish()
        return nil
    }
    // WAL 写入
    w.db.ExecContext(ctx, insertSQL, ...)
    // 带超时的 publish
    done := make(chan struct{})
    go func() { publish(); close(done) }()
    select {
    case <-done:
        w.markPublished(ctx, env.ID)
    case <-time.After(10 * time.Second):
        // publish 超时，仍标记为 published（避免 Recover 重复发布）
        w.markPublished(ctx, env.ID)
        w.lg.Warn("WBPF publish timeout, marked as published to prevent duplicate", ...)
    }
    return nil
}
```

**Step 3：WS 层四级优先级（修正 N-02）**

```go
// internal/server/ws_priority.go — 四级优先级

type wsPriority int

const (
    wsPriorityHigh     wsPriority = iota  // AlertNotify, MCPHealthAlert
    wsPriorityImportant                    // RunnerCompletion, RunStatus, StateDelta, etc.
    wsPriorityNormal                       // text_delta, tool_call, etc.
    wsPriorityLow                          // flow_log, log
)

// 队列容量
HighCap      = 64    // 不变
ImportantCap = 128   // 新增，DropNewest 策略
NormalCap    = 128   // 不变
LowCap       = 256   // 不变

func classifyWSPriority(env event.Envelope) wsPriority {
    reliability := contract.ClassifyEventReliability(env.Type)

    switch reliability {
    case contract.ReliabilityCritical:
        return wsPriorityHigh
    case contract.ReliabilityImportant:
        return wsPriorityImportant  // 不走 high 队列，避免连接断开
    case contract.ReliabilityInformational:
        // 原有逻辑：flow_log → low, 其他 → normal
        // ...
    }
}

// drainSelect 优先级：high → important → normal → low
func (q *connQueues) drainSelect() ([]byte, wsPriority, bool) {
    select {
    case data := <-q.high:
        return data, wsPriorityHigh, true
    default:
    }
    select {
    case data := <-q.important:
        return data, wsPriorityImportant, true
    default:
    }
    // ... normal, low
}
```

**Step 4：Buffer 容量可配置化 + Critical 事件独立持久化队列**

```go
// internal/event/buffer.go — Buffer 容量从配置读取

type BufferConfig struct {
    Cap         int           // 默认 500（从 200 提升）
    TTL         time.Duration // 默认 30min
    CriticalCap int           // Critical 事件独立缓冲，默认 1000
}

func NewBufferWithConfig(cfg BufferConfig) *Buffer {
    // ...
}

// Append 区分 Critical 和普通事件
func (b *Buffer) Append(env Envelope) {
    reliability := contract.ClassifyEventReliability(env.Type)
    if reliability == contract.ReliabilityCritical {
        b.appendToCritical(env)  // 独立缓冲，更大容量
    } else {
        b.appendToNormal(env)    // 普通缓冲
    }
}
```

**Step 5：WAL 自动 Purge**

```go
// internal/event/wal.go — 添加定时 Purge

func (w *EventWAL) StartPurgeLoop(ctx context.Context, interval, retention time.Duration) {
    safego.Go(ctx, "wal-purge", func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                w.PurgePublished(ctx, retention)
            }
        }
    })
}

func (w *EventWAL) PurgePublished(ctx context.Context, retention time.Duration) error {
    cutoff := time.Now().UTC().Add(-retention).Format(time.RFC3339)
    _, err := w.db.ExecContext(ctx,
        `DELETE FROM event_wal WHERE published=1 AND created_at < ?`, cutoff)
    return err
}
```

**Step 6：服务器关闭 Graceful Drain**

```go
// internal/server/ws.go — 修改 Stop()

func (s *WSServer) Stop() {
    s.closed.Store(true)

    // 1. 广播 shutdown
    s.broadcastShutdown()

    // 2. 等待所有连接 drain 完成（最多 5s）
    drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer drainCancel()

    s.connMu.Lock()
    conns := make([]*wsConn, 0, len(s.connStore))
    for _, sessionConns := range s.connStore {
        conns = append(conns, sessionConns...)
    }
    s.connMu.Unlock()

    for _, wc := range conns {
        wc.close()
    }

    // 3. 等待 goroutine 退出
    <-drainCtx.Done()
}
```

#### 4.2.3 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B03 | Critical 事件使用 BlockUntilAcked（30s 安全阀），不退化到 DropOldest |
| B04 | Stop() 主动 close 所有连接 + drain 等待 |
| B11 | Buffer 容量可配置（默认 500）+ Critical 事件独立缓冲（1000） |
| B13 | WAL 添加定时 Purge（默认每 1h 清理 >24h 的已发布记录） |
| N-01 | BlockUntilAcked 30s 安全阀 + WBPF publish 10s 超时 |
| N-02 | 新增 important 四级优先级（容量 128，DropNewest），避免连接断开 |
| N-03 | Important 事件 BlockUpTo 降为 200ms，避免流式卡顿 |

---

## 五、RC-3: Durable 机制完整性补全

### 5.1 问题本质

Durable 机制的当前状态是"半成品"：

| 环节 | 状态 | 问题 |
|------|------|------|
| 升级路径 | ✅ 完整 | 软预算→escalating→硬预算→durable |
| Checkpoint 持久化 | ✅ 完整 | 保存到 session_run_checkpoints 表 |
| 进程内 Runner 取消 | ✅ 完整 | runs.Cancel() 清理内存状态 |
| IM 通知 | ✅ 完整 | NotifyDurableEscalated |
| **重启恢复** | ❌ 致命缺陷 | MarkOrphanedRunsCancelled 误杀 durable run |
| **Resume Claim** | ❌ 设计缺陷 | 300s stale vs 86400s 执行，无进度心跳 |
| **Runner 无超时兜底** | ❌ 缺失 | MaxRunDuration=0 时 Run 无限执行 |

### 5.2 解决方案：Durable 生命周期闭环

#### 5.2.1 架构变更

```
┌─────────────────────────────────────────────────────────────────┐
│                    Durable Run 生命周期                           │
│                                                                 │
│  Interactive ──→ Escalating ──→ Durable ──→ Resuming ──→ Completed│
│       │              │             │            │          │     │
│       ▼              ▼             ▼            ▼          ▼     │
│   [BudgetWatcher]  [Checkpoint]  [Worker]   [Claim+Heartbeat]  │
│   soft→hard       保存快照       轮询恢复    原子claim+进度心跳  │
│                                                                 │
│  异常路径：                                                      │
│  ┌─ 服务器重启 ──→ CleanupOrphanedRuns(排除durable) ──→ Worker恢复│
│  ├─ Claim过期 ──→ 心跳检测 ──→ 活跃则跳过/僵死则重新claim       │
│  ├─ Resume失败 ──→ 重试(最多3次,指数退避) ──→ 超限则Fail         │
│  └─ Runner无响应 ──→ MaxRunDuration兜底 ──→ 强制完成             │
└─────────────────────────────────────────────────────────────────┘
```

#### 5.2.2 具体实现

**Step 1：修复 MarkOrphanedRunsCancelled（B01）+ 清理无 checkpoint durable run（M-01）**

```go
// internal/data/session_run_repo.go

func (r *sessionRunRepo) MarkOrphanedRunsCancelled(ctx context.Context) (int, error) {
    now := time.Now().UTC()
    nowStr := now.Format(time.RFC3339)

    // 修复：排除 durable 阶段（保留有 checkpoint 的 durable run）
    // 但清理"卡在 durable 但无 checkpoint"的异常数据（M-01）
    res, err := db.ExecContext(ctx, `
UPDATE session_runs
SET phase=?, error_message='orphaned: process restarted',
    finished_at=?, phase_changed_at=?, updated_at=?
WHERE (
    phase IN ('interactive','escalating')
    OR (phase='durable' AND (checkpoint_id IS NULL OR checkpoint_id=''))
  )
  AND (finished_at IS NULL OR finished_at='')`,
        biz.SessionRunPhaseCancelled, now, now, nowStr,
    )
    // ...
}
```

```go
// internal/service/session_run_durable_worker.go — processOnce 增加清理（M-01）

if strings.TrimSpace(run.CheckpointID) == "" {
    // 无 checkpoint 的 durable run 是异常数据，标记为 failed
    w.runs.Fail(ctx, run.ID, "durable run without checkpoint")
    w.lg.Warn("durable run without checkpoint, marking as failed",
        "sessionRunID", run.ID)
    continue
}
```

**Step 2：MarkPhase 改为 TransitionPhase + CAS（N-04）**

```go
// internal/data/session_run_repo.go

func (r *sessionRunRepo) TransitionPhase(ctx context.Context, id, fromPhase, toPhase string) (bool, error) {
    now := time.Now().UTC().Format(time.RFC3339)
    res, err := r.db.ExecContext(ctx, `
UPDATE session_runs SET phase=?, phase_changed_at=?, updated_at=?
WHERE id=? AND phase=?`,
        toPhase, now, now, id, fromPhase,
    )
    if err != nil { return false, err }
    return res.RowsAffected() > 0, nil
}

// internal/biz/session_run.go

func (u *SessionRunUsecase) TransitionPhase(ctx context.Context, id string, event SessionRunPhaseEvent) error {
    cur, err := u.repo.Get(ctx, id)
    if err != nil { return err }
    toPhase, err := sessionRunPhaseMachine.Transition(ParseSessionRunPhase(cur.Phase), event)
    if err != nil { return fmt.Errorf("invalid phase transition: %w", err) }
    ok, err := u.repo.TransitionPhase(ctx, id, cur.Phase, string(toPhase))
    if err != nil { return err }
    if !ok { return fmt.Errorf("CAS failed: phase changed concurrently") }
    return nil
}
```

**Step 3：Resume Claim 心跳机制（B08）**

```go
// internal/service/chat_durable_resume.go — 添加心跳

func (w *SessionRunDurableWorker) ResumeDurableSessionRun(
    ctx context.Context, run biz.SessionRun,
) {
    // ... claim 成功后 ...

    // 创建可取消的 resume context
    resumeCtx, resumeCancel := context.WithTimeout(
        context.Background(),
        time.Duration(biz.DefaultDurableDeadlineSec())*time.Second,
    )
    defer resumeCancel()

    // 启动心跳 goroutine
    heartbeatDone := make(chan struct{})
    go w.resumeHeartbeat(resumeCtx, run.ID, heartbeatDone)

    // 执行 resume
    turnErr := w.RunNativeTurn(resumeCtx, ...)

    // 停止心跳
    close(heartbeatDone)

    // 处理结果（含重试策略，见 Step 5）...
}

// resumeHeartbeat 定期更新 resume_started_at，表示"我还活着"
func (w *SessionRunDurableWorker) resumeHeartbeat(
    ctx context.Context, runID string, done chan struct{},
) {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-done:
            return
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.runs.TouchResumeClaim(ctx, runID)
        }
    }
}
```

```go
// internal/data/session_run_repo.go — 添加 TouchResumeClaim

func (r *sessionRunRepo) TouchResumeClaim(ctx context.Context, id string) error {
    now := time.Now().UTC().Format(time.RFC3339)
    _, err := r.db.ExecContext(ctx,
        `UPDATE session_runs SET resume_started_at=?, updated_at=? WHERE id=?`,
        now, now, id,
    )
    return err
}
```

**Step 4：Hard budget grace period 检测 durable phase（M-02）**

```go
// internal/biz/session_run_budget.go

safego.Go(ctx, "session-run-hard-budget-grace", func() {
    select {
    case <-ctx.Done():
        return
    case <-time.After(hardBudgetGracePeriod):
        dbc := context.WithoutCancel(ctx)
        run, err := u.repo.Get(dbc, runID)
        phase := ParseSessionRunPhase(run.Phase)
        // 已升级为 durable，不强制 fail
        if phase == PhaseDurable {
            u.lg.Info("hard budget grace: run already escalated to durable, skipping force fail")
            return
        }
        if !IsSessionRunPhaseTerminal(phase) {
            u.lg.Warn("hard budget grace: forcing run to failed", ...)
            u.repo.MarkTerminal(dbc, runID, SessionRunPhaseFailed, "hard budget grace period exceeded")
        }
    }
})
```

**Step 5：Resume 重试策略（M-03）**

```go
// internal/biz/session_run.go

const (
    DefaultDurableMaxResumeAttempts = 3
    DefaultDurableResumeBackoffSec  = 60  // 指数退避基础 60s
)

// session_runs 表增加 resume_attempts 列
// TryClaimDurableResume WHERE 条件增加:
//   AND (resume_attempts IS NULL OR resume_attempts < ?)
```

```go
// internal/service/chat_durable_resume.go

func (w *SessionRunDurableWorker) ResumeDurableSessionRun(ctx context.Context, run biz.SessionRun) {
    // ... claim 成功后 ...

    _, _, turnErr := s.RunNativeTurn(bgCtx, req)
    if turnErr != nil {
        attempts := run.ResumeAttempts + 1
        if attempts < biz.DefaultDurableMaxResumeAttempts {
            // 重试：增加 resume_attempts，清空 resume_started_at
            w.runs.PrepareRetry(ctx, run.ID, attempts)
            return
        }
        // 超过最大重试次数，标记为 failed
        w.runs.Fail(persistCtx, run.ID, turnErr.Error())
    }
}
```

**Step 6：MaxRunDuration 从 Channel 配置读取（N-06）**

```go
// internal/agent/trpc_runtime.go

func NewTRPCRunner(root, deps TRPCRunnerDeps, opts ...TRPCRunnerOption) (ManagedRunner, error) {
    maxDuration := deps.MaxRunDuration
    if maxDuration <= 0 {
        // 从 Channel 配置读取
        maxDuration = time.Duration(deps.ChannelTurnTimeoutSec) * time.Second
    }
    if maxDuration <= 0 {
        maxDuration = 30 * time.Minute  // 最终兜底
    }
    // ...
}
```

Durable resume 场景：

```go
// internal/service/chat_durable_resume.go

func (w *SessionRunDurableWorker) ResumeDurableSessionRun(ctx context.Context, run biz.SessionRun) {
    deadline := w.getChannelDurableDeadline(ctx, run.SessionID)
    if deadline <= 0 {
        deadline = biz.DefaultDurableDeadlineSec()
    }
    // MaxRunDuration = deadline（而非 30 分钟）
    resumeCtx, resumeCancel := context.WithTimeout(context.Background(),
        time.Duration(deadline)*time.Second)
    // ...
}
```

#### 5.2.3 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B01 | MarkOrphanedRunsCancelled 排除 durable 阶段（保留无 checkpoint 异常清理） |
| B08 | Resume 心跳机制 + stale 判定基于心跳间隔 |
| B07 | Runner MaxRunDuration 从 Channel 配置读取，30min 仅作最终兜底 |
| N-04 | MarkPhase 改为 TransitionPhase + CAS |
| N-06 | MaxRunDuration 不硬编码，从 Channel 配置读取 |
| M-01 | 无 checkpoint 的 durable run 在 processOnce 中标记为 failed |
| M-02 | Hard budget grace period 检测 durable phase 时跳过 force fail |
| M-03 | Resume 重试策略（最大 3 次 + 指数退避） |

---

## 六、前端内存管理方案

### 6.1 问题本质

前端内存问题不是单一 bug，而是**缺少资源生命周期管理**：

| 资源 | 当前行为 | 问题 |
|------|---------|------|
| `messagesBySession` | 永不淘汰 | 24H 使用后内存持续增长 |
| `inboundWriters` | 组件卸载时整体销毁 | 切换 session 不清理旧 writer |
| `sealedTurnBySession` | unseal 时删除 | 异常路径泄漏 |
| `pendingQueue` | disconnect 时清空 | 重连耗尽后不清空且继续追加 |

### 6.2 解决方案：Session 资源池化管理（含驱逐守卫）

#### 6.2.1 架构变更

引入 `SessionResourceManager`，统一管理所有 per-session 资源的生命周期：

```typescript
// web/src/stores/chat/sessionResourceManager.ts

interface SessionResource<T> {
  value: T
  lastAccessedAt: number
}

export function createSessionResourceManager(deps: {
  messageStore: ReturnType<typeof useMessageStore>
  streamingSnapshots: ReturnType<typeof useChatStreamingSnapshots>
  sessionStore: ReturnType<typeof useSessionStore>
  inboundSync: { evictSessionResources: (sid: string) => void }
  sender: { sending: Ref<boolean> }
}) {
  const MAX_ACTIVE_SESSIONS = 5
  const IDLE_SESSION_TTL_MS = 30 * 60 * 1000
  const resources = new Map<string, SessionResource<any>>()

  function get<T>(sessionId: string, factory: () => T): T {
    const existing = resources.get(sessionId)
    if (existing) {
      existing.lastAccessedAt = Date.now()
      return existing.value as T
    }

    const value = factory()
    resources.set(sessionId, { value, lastAccessedAt: Date.now() })

    if (resources.size > MAX_ACTIVE_SESSIONS) {
      evictOldest()
    }

    return value
  }

  // 修正(N-07)：增加驱逐守卫
  function canEvict(sessionId: string): boolean {
    // 规则 1：不驱逐当前活跃会话
    if (deps.sessionStore.currentSessionId() === sessionId) return false
    // 规则 2：不驱逐正在流式传输的会话
    if (deps.sender.sending.value) return false
    return true
  }

  function evictOldest() {
    let oldest: string | null = null
    let oldestTime = Infinity

    for (const [sid, res] of resources) {
      if (!canEvict(sid)) continue  // 跳过不可驱逐的会话
      if (res.lastAccessedAt < oldestTime) {
        oldest = sid
        oldestTime = res.lastAccessedAt
      }
    }

    if (oldest) {
      evict(oldest)
    }
  }

  function evict(sessionId: string): void {
    if (!canEvict(sessionId)) return

    // 清理完整资源链
    deps.messageStore.deleteSessionMessages(sessionId)
    deps.streamingSnapshots.clear(sessionId)
    deps.inboundSync.evictSessionResources(sessionId)
    resources.delete(sessionId)
  }

  function release(sessionId: string) {
    const res = resources.get(sessionId)
    if (!res) return
    deps.messageStore.deleteSessionMessages(sessionId)
    deps.streamingSnapshots.clear(sessionId)
    deps.inboundSync.evictSessionResources(sessionId)
    resources.delete(sessionId)
  }

  function touch(sessionId: string) {
    const res = resources.get(sessionId)
    if (res) {
      res.lastAccessedAt = Date.now()
    }
  }

  return { get, release, touch, evict }
}
```

#### 6.2.2 useChatInboundSync 暴露 evictSessionResources（N-07）

```typescript
// web/src/features/chat/composables/useChatInboundSync.ts

function evictSessionResources(sessionId: string): void {
  // 1. flush 并销毁 inboundWriter
  const writer = inboundWriters.get(sessionId)
  if (writer) {
    writer.flushSync()
    inboundWriters.delete(sessionId)
  }
  // 2. 清理 seal
  sealedTurnBySession.delete(sessionId)
  // 3. 清理 in-flight 标记
  hydrateInFlight.delete(sessionId)
  focusInFlight.delete(sessionId)
}

return { ..., evictSessionResources }
```

#### 6.2.3 pendingQueue 重连耗尽处理

```typescript
// web/src/realtime/ws-transport.ts

function scheduleReconnect(): void {
    if (reconnectTimer) return
    if (reconnectAttempts >= WS_MAX_RECONNECT_ATTEMPTS) {
        opts.onReconnectFailed?.()
        // 清理 pendingQueue，防止无限增长
        const undelivered = [...pendingQueue]
        pendingQueue.length = 0
        opts.onMessagesLost?.(undelivered)
        return
    }
    // ... 原有重连逻辑
}

function send(upstream: WsUpstream): void {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(upstream))
        return
    }

    // 重连已耗尽，不再入队
    if (reconnectAttempts >= WS_MAX_RECONNECT_ATTEMPTS) {
        opts.onMessagesLost?.([upstream])
        return
    }

    pendingQueue.push(upstream)
}
```

#### 6.2.4 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B14 | SessionResourceManager LRU 淘汰，最多保留 5 个 session |
| B10 | 重连耗尽后清空 pendingQueue + 通知上层标记消息 failed |
| N-07 | 驱逐守卫——禁止驱逐当前活跃会话 + 禁止驱逐正在流式传输的会话 |
| S09 | inboundWriters 纳入 SessionResourceManager 统一管理 |
| S10 | sealedTurnBySession 纳入 SessionResourceManager 统一管理 |

---

## 七、实施路线图

### Phase 1：紧急修复（1-2 天）— 不引入新问题

| 任务 | 修复项 | 风险 |
|------|--------|------|
| MarkOrphanedRunsCancelled 排除 durable + 无 checkpoint 清理 | B01, M-01 | 低 |
| MarkPhase 改为 TransitionPhase + CAS | N-04 | 中，需修改 repo 接口 |
| writePump drain 迭代保护 | B02 | 低 |
| Hard budget grace period 检测 durable | M-02 | 低 |

### Phase 2：架构优化（3-5 天）— 需充分测试竞态

| 任务 | 修复项 | 风险 |
|------|--------|------|
| TurnContextManager + auto-escalate | B09 | 高，需测试竞态 |
| FinishSessionRunLifecycle CAS 保护 | N-05 | 中 |
| 状态机增加 interactive→durable | M-04 | 低 |
| Bus 层三级投递（带安全阀） | B03, N-01 | 高，需压测 |
| WS 四级优先级 | N-02 | 中 |
| WBPF publish 超时保护 | N-01 | 中 |
| 服务器关闭 graceful drain | B04 | 中 |

### Phase 3：资源管理（2-3 天）— 需测试 UX

| 任务 | 修复项 | 风险 |
|------|--------|------|
| SessionResourceManager（带驱逐守卫） | B14, N-07 | 中，需测试回切 UX |
| useChatInboundSync 暴露 evictSessionResources | N-07 | 低 |
| pendingQueue 重连耗尽处理 | B10 | 低 |
| Buffer 容量可配置化 | B11 | 低 |
| WAL 自动 Purge | B13 | 低 |

### Phase 4：加固完善（2-3 天）

| 任务 | 修复项 | 风险 |
|------|--------|------|
| Resume heartbeat + 重试策略 | B08, M-03 | 中 |
| MaxRunDuration 从 Channel 配置读取 | N-06 | 中 |
| RunRegistry 子管理器提取 | B06 | 低 |
| runSingleAgentViaTRPC 拆分 | S08 | 中 |

---

## 八、端到端场景验证

### 场景 1：WS 客户端发送消息 → LLM 回复 → UI 显示

```
用户点击发送
  → useChatSender.onSend()
  → createPlaceholderMessage('pending-user-{uuid}')
  → messageStore.setMessages(sid, [...msgs, placeholder])
  → ensureChatStream(sid)
  → sendChatViaWs(stream, WsUpstream)

WSServer.handleUserMessage()
  → TurnContextManager.AcquireTurnContext(connCtx, sid, Interactive)
    → ctx = context.WithTimeout(Background, TurnTimeout)  // 独立于 connCtx
    → go watchDisconnect(connCtx, ctx, cancel, sid)       // 监听断连
  → turnExecutor.ExecuteTurn(ctx, input)
    → ChatOrchestrator.RunNativeAgentTurnWithOutcome()
      → BuildTRPCAgentCached → Provider → Model
      → Runner.Run(ctx) → LLM API (streaming)
      → stream_consumer.consume(events)
        → EventProjector → Envelope
        → Infra.Publish(ctx, env)
          → [Critical] WBPF + BlockUntilAcked(30s安全阀)
          → [Important] BlockUpTo(200ms)
          → [Informational] DropNewest
        → WS eventPump → 四级优先级队列 → writePump → WebSocket

前端收到事件
  → EnvelopeDispatcher.dispatch(env)
  → streamHandlers: text_delta/tool_call/tool_result/run_status/runner_completion
  → messageStoreBatch.update() → RAF → messageStore.setMessages()
  → groupMessagesByTurn() → TurnBlockGroup[] → ChatMessageList 渲染
```

**验证点**：
- [x] Turn context 独立于 WS connCtx
- [x] Critical 事件有 30s 安全阀，不会无限阻塞
- [x] Important 事件走 important 队列（DropNewest），不会导致连接断开
- [x] 流式消费不会因 BlockUpTo 卡顿（200ms 而非 500ms）

### 场景 2：WS 断连后重连

```
WS 连接断开
  → readPump 退出 → wc.close() → connCancel()
  → TurnContextManager.watchDisconnect 检测到 connCtx.Done()
    → 尝试 auto-escalate: TransitionPhase(interactive, durable)
    → 如果 CAS 成功：创建 checkpoint → runs.Cancel() → turn 返回
    → 如果 CAS 失败（run 已终态）：调用 turnCancel() 取消 turn

客户端重连
  → createWsTransport → connect()
  → 携带 last_event_id → replayEvents()
  → Buffer.Replay(sid, lastEventID) → 补发缺失事件
  → eventPump 恢复实时推送

Durable Worker 轮询
  → ListDurablePending → 发现 phase=durable 的 run
  → TryClaimDurableResume → SQL CAS claim
  → ResumeDurableSessionRun(bgCtx, run)
    → heartbeat goroutine 每 60s 更新 resume_started_at
    → RunNativeTurn(bgCtx, req) → LLM 调用
    → 成功: TransitionPhase(durable, completed)
    → 失败: resume_attempts++ → 重试 or Fail
```

**验证点**：
- [x] WS 断连不直接取消 turn，而是 auto-escalate
- [x] TransitionPhase CAS 防止竞态覆盖
- [x] Resume heartbeat 防止 claim 过期
- [x] Resume 重试策略（最多 3 次）

### 场景 3：服务器重启

```
服务器重启
  → SessionRunDurableWorker.Start()
    → CleanupOrphanedRuns()
      → MarkOrphanedRunsCancelled: WHERE phase IN ('interactive','escalating')
        OR (phase='durable' AND checkpoint_id IS NULL)  // 排除有效 durable
    → 轮询 goroutine 启动
    → processOnce() → ListDurablePending
    → TryClaimDurableResume → heartbeat → ResumeDurableSessionRun
```

**验证点**：
- [x] Durable run 不被孤儿清理误杀
- [x] 无 checkpoint 的异常 durable run 被清理
- [x] Resume 正常执行

### 场景 4：24H 连续长任务

```
Channel 客户端发送消息
  → TurnContextManager.AcquireTurnContext(bgCtx, sid, Background)
    → ctx = context.WithTimeout(Background, ChannelTurnTimeoutSec)
  → Runner.Run(ctx) → MaxRunDuration = ChannelTurnTimeoutSec
  → BudgetWatcher: soft=180s, hard=ChannelTurnTimeoutSec
  → Turn 正常完成 or 超时升级 durable

Durable Worker 恢复
  → ResumeDurableSessionRun(bgCtx, run)
    → deadline = ChannelDurableDeadlineSec (默认 86400s)
    → MaxRunDuration = deadline
    → heartbeat 每 60s
    → 最多重试 3 次
```

**验证点**：
- [x] MaxRunDuration 从 Channel 配置读取，不截断长任务
- [x] Durable resume deadline 与 MaxRunDuration 协调
- [x] 前端内存有 LRU 淘汰（MAX_ACTIVE_SESSIONS=5）
- [x] WAL 有自动 Purge（防止磁盘增长）

---

## 九、验证标准

### 9.1 功能验证

| 场景 | 预期结果 | 验证方法 |
|------|---------|---------|
| WS 客户端发送消息 → LLM 回复 → UI 显示 | 端到端数据流完整 | 手动测试 |
| WS 断连后重连 | 事件通过 Buffer 重放补发，无丢失 | 模拟网络断连 |
| WS 断连时 turn 正在执行 | Auto-escalate 到 durable，turn 继续执行 | 关闭浏览器后重新打开 |
| 服务器重启 | Durable run 被 Worker 恢复，非 durable run 被清理 | `kill -9` 后重启 |
| 24H 连续执行 | 内存稳定，无泄漏 | 长时间压测 + 内存监控 |
| 高频事件场景 | Critical 事件无丢失，ping 正常 | 10+ 并发 Agent 压测 |

### 9.2 架构合规验证

| 检查项 | 标准 | 验证方法 |
|--------|------|---------|
| Turn context 独立于 WS connCtx | 断连不取消 turn | 代码审查 + 单元测试 |
| Critical 事件投递保证 | BlockUntilAcked（30s 安全阀），不退化 | 代码审查 + 压测 |
| Durable run 不被孤儿清理 | MarkOrphanedRunsCancelled 排除 durable | SQL 验证 |
| Phase 转换 CAS 保护 | TransitionPhase 替代 MarkPhase | 代码审查 + 竞态测试 |
| 前端内存上限 | MAX_ACTIVE_SESSIONS=5 + 驱逐守卫 | 内存分析 |

### 9.3 回归测试

```bash
# 后端全量验证
make api && make wire && make build && make test && make lint

# 前端全量验证
cd web && pnpm lint && pnpm test && pnpm build

# 关键路径单测
go test ./internal/service/... -run TestDurable -count=1
go test ./internal/event/... -run TestBus -count=1
go test ./internal/server/... -run TestWS -count=1
```

---

## 十、风险矩阵

| 变更 | 修正前风险 | 修正后风险 | 关键缓解 |
|------|-----------|-----------|---------|
| Critical BlockUntilAcked | 极高（goroutine 永久阻塞） | 低 | 30s 安全阀 + unhealthy 标记 |
| Important 走 high 队列 | 高（连接断开） | 低 | 新增 important 四级队列 |
| Important BlockUpTo(500ms) | 中高（流式卡顿） | 低 | 降为 200ms |
| MarkPhase 无 CAS | Critical（终态覆盖） | 低 | TransitionPhase + CAS |
| Auto-escalate 后 Fail | 高（durable 被覆盖） | 低 | CAS 保护 |
| MaxRunDuration 30min | 高（截断长任务） | 低 | 从 Channel 配置读取 |
| LRU 驱逐活跃会话 | Critical（UI 空白） | 低 | 驱逐守卫 |
| WBPF publish 阻塞 | 高（WAL 不一致） | 低 | 10s 超时 + markPublished |
| Grace period vs durable | 高（强制 fail） | 低 | 检测 durable phase |

---

## 十一、关键文件变更清单

| 文件 | 变更类型 | 关联修复项 |
|------|---------|-----------|
| `internal/runtime/turn/context_manager.go` | 新增 | B09, B05, N-04, N-05 |
| `internal/server/ws_message_handler.go` | 修改 | B09 |
| `internal/server/ws_io_pump.go` | 修改 | B02 |
| `internal/server/ws.go` | 修改 | B04 |
| `internal/server/ws_priority.go` | 修改 | N-02 |
| `internal/event/bus.go` | 修改 | B03, N-01, N-03 |
| `internal/event/buffer.go` | 修改 | B11 |
| `internal/event/wal.go` | 修改 | B13, N-01 |
| `internal/event/infra.go` | 修改 | B03 |
| `internal/data/session_run_repo.go` | 修改 | B01, N-04, M-01 |
| `internal/biz/session_run.go` | 修改 | N-04, M-03 |
| `internal/biz/session_run_phase_machine.go` | 修改 | M-04 |
| `internal/biz/session_run_budget.go` | 修改 | M-02 |
| `internal/service/session_run_durable_worker.go` | 修改 | B08, M-01 |
| `internal/service/chat_durable_resume.go` | 修改 | B08, N-06, M-03 |
| `internal/service/chat_orch_session_run_lifecycle.go` | 修改 | B09, N-05 |
| `internal/agent/trpc_runtime.go` | 修改 | B07, N-06 |
| `pkg/trpc-agent-go/runner/runner.go` | 修改 | B07 |
| `web/src/stores/chat/sessionResourceManager.ts` | 新增 | B14, N-07 |
| `web/src/stores/chat/messageStore.ts` | 修改 | B14 |
| `web/src/realtime/ws-transport.ts` | 修改 | B10 |
| `web/src/features/chat/composables/useChatInboundSync.ts` | 修改 | S09, S10, N-07 |
