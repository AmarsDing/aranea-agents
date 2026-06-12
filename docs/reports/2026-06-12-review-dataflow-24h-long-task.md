# 数据流全链路审查与 24H 长任务专项解决方案

> **文档类型**：架构评审 + 专项解决方案
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

以下按根因给出根本性解决方案。

---

## 二、RC-1: Turn 生命周期与传输连接解耦

### 2.1 问题本质

当前架构中，Turn 的执行 context 派生自 WS 连接的 `connCtx`：

```
WS connCtx ──→ context.WithTimeout(connCtx, TurnTimeout) ──→ Turn 执行
     │
     └── 客户端断连 → connCancel() → Turn context 取消 → 任务终止
```

这导致两个问题：
1. **B09**：客户端断连时 interactive 阶段的 turn 直接失败，无 durable 升级
2. **B02**：writePump 的 ping ticker 被 drain 饿死，连接断开 → turn 被取消

**根本矛盾**：Turn 是**业务概念**（一次 Agent 推理循环），WS 是**传输概念**（客户端连接）。两者生命周期不应耦合。

### 2.2 解决方案：Turn Context 独立于传输层

#### 2.2.1 架构变更

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

#### 2.2.2 具体实现

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
            // 升级失败 → 取消 turn（降级行为）
            m.lg.Warn("auto-escalate failed, cancelling turn",
                "sessionID", sessionID, "error", err)
            turnCancel()
        }
        // 升级成功 → turn 继续执行（context 不变，只是模式切换）

    case <-turnCtx.Done():
        // Turn 正常结束或超时，无需操作
    }
}

// tryAutoEscalate 尝试将 interactive run 升级为 durable
func (m *TurnContextManager) tryAutoEscalate(
    ctx context.Context, sessionID string,
) (bool, error) {
    // 检查是否有活跃 run
    if !m.runs.HasActive(sessionID) {
        return false, nil
    }
    // 检查 run 是否已经有 checkpoint（避免重复升级）
    // 调用 biz 层 EscalateSessionRunToDurable
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

func (l *chatSessionRunLifecycle) FinishSessionRunLifecycle(
    ctx context.Context, sessionID, sessionRunID string, turnErr error,
) {
    if turnErr != nil {
        // 新增：检查是否由 context 取消引起（客户端断连）
        if errors.Is(turnErr, context.Canceled) {
            // 尝试 auto-escalate（如果还没升级的话）
            run, _ := l.sessionRuns.Get(ctx, sessionRunID)
            if run != nil && run.Phase == biz.SessionRunPhaseInteractive {
                l.EscalateSessionRunToDurable(ctx, sessionID, sessionRunID)
                return  // 升级成功，不标记为 failed
            }
        }
        // 非断连取消，正常 fail
        l.sessionRuns.Fail(ctx, sessionRunID, turnErr.Error())
        return
    }
    // 正常完成...
}
```

#### 2.2.3 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B09 | WS 断连时 auto-escalate 到 durable，而非直接取消 turn |
| B02 | Turn context 独立于 connCtx，即使 ping 饿死导致连接断开，turn 仍可继续执行 |
| B05 | Turn context 不依赖 connCtx，closeSend 不影响 turn 执行 |

#### 2.2.4 对 writePump ping 饥饿的补充修复

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
                    // 发 ping
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

## 三、RC-2: 事件投递可靠性分层执行

### 3.1 问题本质

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

**根本矛盾**：事件可靠性分级（Critical/Important/Informational）在**分类**层面已实现，但在**投递执行**层面没有差异化——所有事件共享同一投递路径，BlockUpTo 退化后 Critical 和 Informational 的保障相同。

### 3.2 解决方案：三级投递管道

#### 3.2.1 架构变更

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
            │ Acked       │ │ 500ms     │ │              │
            └──────┬──────┘ └─────┬─────┘ └──────┬──────┘
                   │              │               │
            ┌──────▼──────┐ ┌────▼─────┐ ┌──────▼──────┐
            │ WS: high    │ │ WS: high │ │ WS: low     │
            │ queue       │ │ queue    │ │ queue       │
            └─────────────┘ └──────────┘ └─────────────┘
```

#### 3.2.2 具体实现

**Step 1：Bus 层投递策略按可靠性分级**

```go
// internal/event/bus.go — 修改 deliverToSubscriber

func (b *bus) deliverToSubscriber(sub *subscriber, env Envelope, requiresBlockUpTo bool) {
    reliability := contract.ClassifyEventReliability(env.Type)

    switch reliability {
    case contract.ReliabilityCritical:
        // Critical: BlockUntilAcked — 阻塞直到投递成功或 subscriber 关闭
        b.deliverBlockUntilAcked(sub, env)

    case contract.ReliabilityImportant:
        // Important: BlockUpTo 500ms — 比 Informational 更长阻塞
        blockFor := sub.opts.BlockFor
        if blockFor <= 0 {
            blockFor = 500 * time.Millisecond  // 从 100ms 提升到 500ms
        }
        b.deliverBlockUpTo(sub, env, blockFor)

    case contract.ReliabilityInformational:
        // Informational: DropNewest — 满即丢
        b.deliverDropNewest(sub, env)
    }
}

// deliverBlockUntilAcked 阻塞直到成功投递或 subscriber 关闭
// 不设超时，因为 Critical 事件不可丢失
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
    // 阻塞等待，不设超时
    // 但如果 subscriber channel 已关闭，退出
    select {
    case sub.ch <- env:
    case <-sub.done:
        b.logDrop(env, "subscriber_closed")
    }
}
```

**Step 2：WS 层 Important 事件走 high 队列**

```go
// internal/server/ws_priority.go — 修改优先级路由

func classifyWSPriority(env event.Envelope) wsPriority {
    reliability := contract.ClassifyEventReliability(env.Type)

    switch reliability {
    case contract.ReliabilityCritical:
        return wsPriorityHigh   // 原有：alert, mcp.health
    case contract.ReliabilityImportant:
        return wsPriorityHigh   // 新增：RunnerCompletion, RunStatus 等走 high
    case contract.ReliabilityInformational:
        // 原有逻辑：flow_log → low, 其他 → normal
        // ...
    }
}
```

**Step 3：Buffer 容量可配置化 + Critical 事件独立持久化队列**

```go
// internal/event/buffer.go — Buffer 容量从配置读取

type BufferConfig struct {
    Cap           int           // 默认 500（从 200 提升）
    TTL           time.Duration // 默认 30min
    CriticalCap   int           // Critical 事件独立缓冲，默认 1000
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

**Step 4：WAL 自动 Purge**

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

// PurgePublished 清理已发布且超过 retention 时间的 WAL 记录
func (w *EventWAL) PurgePublished(ctx context.Context, retention time.Duration) error {
    cutoff := time.Now().UTC().Add(-retention).Format(time.RFC3339)
    _, err := w.db.ExecContext(ctx,
        `DELETE FROM event_wal WHERE published=1 AND created_at < ?`, cutoff)
    return err
}
```

**Step 5：服务器关闭 Graceful Drain**

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
        // 取消连接 context，触发所有 goroutine 退出
        wc.close()
    }

    // 3. 等待 goroutine 退出
    <-drainCtx.Done()
}
```

#### 3.2.3 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B03 | Critical 事件使用 BlockUntilAcked，不退化到 DropOldest |
| B04 | Stop() 主动 close 所有连接 + drain 等待 |
| B11 | Buffer 容量可配置（默认 500）+ Critical 事件独立缓冲（1000） |
| B13 | WAL 添加定时 Purge（默认每 1h 清理 >24h 的已发布记录） |

---

## 四、RC-3: Durable 机制完整性补全

### 4.1 问题本质

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

### 4.2 解决方案：Durable 生命周期闭环

#### 4.2.1 架构变更

```
┌─────────────────────────────────────────────────────────────────┐
│                    Durable Run 生命周期                           │
│                                                                 │
│  Interactive ──→ Escalating ──→ Durable ──→ Resuming ──→ Completed│
│       │              │             │            │          │     │
│       │              │             │            │          │     │
│       ▼              ▼             ▼            ▼          ▼     │
│   [BudgetWatcher]  [Checkpoint]  [Worker]   [Claim+Heartbeat]  │
│   soft→hard       保存快照       轮询恢复    原子claim+进度心跳  │
│                                                                 │
│  异常路径：                                                      │
│  ┌─ 服务器重启 ──→ CleanupOrphanedRuns(排除durable) ──→ Worker恢复│
│  ├─ Claim过期 ──→ 心跳检测 ──→ 活跃则跳过/僵死则重新claim       │
│  └─ Runner无响应 ──→ MaxRunDuration兜底 ──→ 强制完成             │
└─────────────────────────────────────────────────────────────────┘
```

#### 4.2.2 具体实现

**Step 1：修复 MarkOrphanedRunsCancelled（B01）**

```go
// internal/data/session_run_repo.go

func (r *sessionRunRepo) MarkOrphanedRunsCancelled(ctx context.Context) (int, error) {
    now := time.Now().UTC()
    nowStr := now.Format(time.RFC3339)

    // 修复：排除 durable 阶段
    // durable run 的生命周期由 DurableWorker 管理，不应被孤儿清理干预
    // 但清理"卡在 durable 但无 checkpoint"的异常数据
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

**Step 2：Resume Claim 心跳机制（B08）**

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

    // 处理结果...
}

// resumeHeartbeat 定期更新 resume_started_at，表示"我还活着"
func (w *SessionRunDurableWorker) resumeHeartbeat(
    ctx context.Context, runID string, done chan struct{},
) {
    ticker := time.NewTicker(60 * time.Second)  // 每 60s 心跳
    defer ticker.Stop()

    for {
        select {
        case <-done:
            return
        case <-ctx.Done():
            return
        case <-ticker.C:
            // 更新 resume_started_at 为当前时间
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

**修改 Claim stale 判定**：从固定 300s 改为基于心跳间隔的动态判定：

```go
// internal/biz/session_run.go

const (
    DefaultDurableResumeClaimStaleSec = 300  // 保留，但含义变为"无心跳超过 5 分钟"
    DurableResumeHeartbeatIntervalSec = 60   // 心跳间隔 60s
)

// TryClaimDurableResume 的 staleBefore 逻辑不变
// 但由于心跳每 60s 更新 resume_started_at，
// 只有真正僵死的 resume 才会超过 300s 无更新
```

**Step 3：Runner MaxRunDuration 兜底（B07）**

```go
// internal/agent/trpc_runtime.go — 强制注入 MaxRunDuration

func NewTRPCRunner(root, deps TRPCRunnerDeps, opts ...TRPCRunnerOption) (ManagedRunner, error) {
    // 从配置获取默认 MaxRunDuration
    maxDuration := deps.MaxRunDuration
    if maxDuration <= 0 {
        maxDuration = 30 * time.Minute  // 默认 30 分钟
    }

    runnerOpts := []trpcrunner.RunnerOption{
        trpcrunner.WithMaxRunDuration(maxDuration),
        // ... 其他选项
    }

    return trpcrunner.NewRunner(runnerOpts...)
}
```

```go
// pkg/trpc-agent-go/runner/runner.go — 强制校验 MaxRunDuration

func (r *Runner) Run(ctx context.Context, ...) (<-chan *event.Event, error) {
    // 如果 ctx 没有 deadline 且 MaxRunDuration 为 0，
    // 强制设置 30 分钟默认超时
    if _, ok := ctx.Deadline(); !ok && r.maxRunDuration <= 0 {
        r.maxRunDuration = 30 * time.Minute
    }
    // ...
}
```

**Step 4：Durable Resume 使用 Channel 配置的 deadline（S05）**

```go
// internal/service/chat_durable_resume.go

func (w *SessionRunDurableWorker) ResumeDurableSessionRun(
    ctx context.Context, run biz.SessionRun,
) {
    // 从 Channel 配置读取 deadline
    deadline := w.getChannelDurableDeadline(ctx, run.SessionID)
    if deadline <= 0 {
        deadline = biz.DefaultDurableDeadlineSec()
    }

    resumeCtx, resumeCancel := context.WithTimeout(
        context.Background(),
        time.Duration(deadline)*time.Second,
    )
    defer resumeCancel()
    // ...
}
```

#### 4.2.3 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B01 | MarkOrphanedRunsCancelled 排除 durable 阶段（保留无 checkpoint 异常清理） |
| B08 | Resume 心跳机制 + stale 判定基于心跳间隔 |
| B07 | Runner 强制 MaxRunDuration 兜底（默认 30min） |
| S05 | Resume deadline 从 Channel 配置读取 |

---

## 五、前端内存管理方案

### 5.1 问题本质

前端内存问题不是单一 bug，而是**缺少资源生命周期管理**：

| 资源 | 当前行为 | 问题 |
|------|---------|------|
| `messagesBySession` | 永不淘汰 | 24H 使用后内存持续增长 |
| `inboundWriters` | 组件卸载时整体销毁 | 切换 session 不清理旧 writer |
| `sealedTurnBySession` | unseal 时删除 | 异常路径泄漏 |
| `pendingQueue` | disconnect 时清空 | 重连耗尽后不清空且继续追加 |

### 5.2 解决方案：Session 资源池化管理

#### 5.2.1 架构变更

引入 `SessionResourceManager`，统一管理所有 per-session 资源的生命周期：

```typescript
// web/src/stores/chat/sessionResourceManager.ts

interface SessionResource<T> {
  value: T
  lastAccessedAt: number
}

const MAX_ACTIVE_SESSIONS = 5  // 最多保留 5 个 session 的资源
const IDLE_SESSION_TTL_MS = 30 * 60 * 1000  // 30 分钟未访问则可淘汰

export function createSessionResourceManager() {
  const resources = new Map<string, SessionResource<any>>()

  function get<T>(sessionId: string, factory: () => T): T {
    const existing = resources.get(sessionId)
    if (existing) {
      existing.lastAccessedAt = Date.now()
      return existing.value as T
    }

    const value = factory()
    resources.set(sessionId, { value, lastAccessedAt: Date.now() })

    // 超过上限，淘汰最久未访问的
    if (resources.size > MAX_ACTIVE_SESSIONS) {
      evictOldest()
    }

    return value
  }

  function evictOldest() {
    let oldest: string | null = null
    let oldestTime = Infinity

    for (const [sid, res] of resources) {
      if (res.lastAccessedAt < oldestTime) {
        oldest = sid
        oldestTime = res.lastAccessedAt
      }
    }

    if (oldest) {
      release(oldest)
    }
  }

  function release(sessionId: string) {
    const res = resources.get(sessionId)
    if (!res) return

    // 清理消息
    delete messagesBySession.value[sessionId]
    delete sessionRevisionBySession.value[sessionId]

    // 清理 inboundWriter
    const writer = inboundWriters.get(sessionId)
    if (writer) {
      writer.dispose()
      inboundWriters.delete(sessionId)
    }

    // 清理 seal
    sealedTurnBySession.delete(sessionId)

    resources.delete(sessionId)
  }

  function touch(sessionId: string) {
    const res = resources.get(sessionId)
    if (res) {
      res.lastAccessedAt = Date.now()
    }
  }

  return { get, release, touch }
}
```

#### 5.2.2 pendingQueue 重连耗尽处理

```typescript
// web/src/realtime/ws-transport.ts

function scheduleReconnect(): void {
    if (reconnectTimer) return
    if (reconnectAttempts >= WS_MAX_RECONNECT_ATTEMPTS) {
        // 通知用户消息未送达
        opts.onReconnectFailed?.()
        // 清理 pendingQueue，防止无限增长
        const undelivered = [...pendingQueue]
        pendingQueue.length = 0
        // 将未送达消息通知上层，由 Store 标记为 failed
        opts.onMessagesLost?.(undelivered)
        return
    }
    // ... 原有重连逻辑
}

// send 方法在连接断开时检查重连状态
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

#### 5.2.3 解决的问题

| 原始 ID | 如何解决 |
|---------|---------|
| B14 | SessionResourceManager LRU 淘汰，最多保留 5 个 session |
| B10 | 重连耗尽后清空 pendingQueue + 通知上层标记消息 failed |
| S09 | inboundWriters 纳入 SessionResourceManager 统一管理 |
| S10 | sealedTurnBySession 纳入 SessionResourceManager 统一管理 |

---

## 六、实施路线图

### Phase 1：紧急修复（1-2 天）

解决直接影响 24H 长任务可行性的致命缺陷：

| 任务 | 修复项 | 风险 |
|------|--------|------|
| MarkOrphanedRunsCancelled 排除 durable | B01 | 低，SQL WHERE 条件收窄 |
| Runner 强制 MaxRunDuration 兜底 | B07 | 低，添加默认值 |
| writePump drain 迭代保护 | B02 | 低，添加计数器 |

### Phase 2：架构优化（3-5 天）

实现根本性架构变更：

| 任务 | 修复项 | 风险 |
|------|--------|------|
| TurnContextManager — Turn 与传输解耦 | B09, B05 | 中，需修改 WS handler + lifecycle |
| Bus 层三级投递管道 | B03 | 中，需修改 bus.go 投递逻辑 |
| WS 层 Important 事件走 high 队列 | B03 补充 | 低，修改优先级路由 |
| Resume 心跳机制 | B08 | 中，需修改 durable worker + repo |
| 服务器关闭 graceful drain | B04 | 中，需修改 Stop() 逻辑 |

### Phase 3：资源管理（2-3 天）

完善内存和资源管理：

| 任务 | 修复项 | 风险 |
|------|--------|------|
| SessionResourceManager | B14, S09, S10 | 中，需修改 Store + composable |
| pendingQueue 重连耗尽处理 | B10 | 低，修改 ws-transport |
| Buffer 容量可配置化 | B11 | 低，添加配置参数 |
| WAL 自动 Purge | B13 | 低，添加定时任务 |

### Phase 4：加固完善（1-2 天）

| 任务 | 修复项 | 风险 |
|------|--------|------|
| RunRegistry 子管理器提取 | B06 | 低，重构 sync.Map |
| runSingleAgentViaTRPC 拆分 | S08 | 中，需充分测试 |
| Resume deadline 从 Channel 配置读取 | S05 | 低 |

---

## 七、验证标准

### 7.1 功能验证

| 场景 | 预期结果 | 验证方法 |
|------|---------|---------|
| WS 客户端发送消息 → LLM 回复 → UI 显示 | 端到端数据流完整 | 手动测试 |
| WS 断连后重连 | 事件通过 Buffer 重放补发，无丢失 | 模拟网络断连 |
| WS 断连时 turn 正在执行 | Auto-escalate 到 durable，turn 继续执行 | 关闭浏览器后重新打开 |
| 服务器重启 | Durable run 被 Worker 恢复，非 durable run 被清理 | `kill -9` 后重启 |
| 24H 连续执行 | 内存稳定，无泄漏 | 长时间压测 + 内存监控 |
| 高频事件场景 | Critical 事件无丢失，ping 正常 | 10+ 并发 Agent 压测 |

### 7.2 架构合规验证

| 检查项 | 标准 | 验证方法 |
|--------|------|---------|
| Turn context 独立于 WS connCtx | 断连不取消 turn | 代码审查 + 单元测试 |
| Critical 事件投递保证 | BlockUntilAcked，不退化 | 代码审查 + 压测 |
| Durable run 不被孤儿清理 | MarkOrphanedRunsCancelled 排除 durable | SQL 验证 |
| 前端内存上限 | MAX_ACTIVE_SESSIONS=5 | 内存分析 |

### 7.3 回归测试

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

## 八、关键文件变更清单

| 文件 | 变更类型 | 关联修复项 |
|------|---------|-----------|
| `internal/runtime/turn/context_manager.go` | 新增 | B09, B05 |
| `internal/server/ws_message_handler.go` | 修改 | B09 |
| `internal/server/ws_io_pump.go` | 修改 | B02 |
| `internal/server/ws.go` | 修改 | B04 |
| `internal/event/bus.go` | 修改 | B03 |
| `internal/event/buffer.go` | 修改 | B11 |
| `internal/event/wal.go` | 修改 | B13 |
| `internal/data/session_run_repo.go` | 修改 | B01 |
| `internal/service/session_run_durable_worker.go` | 修改 | B08 |
| `internal/service/chat_durable_resume.go` | 修改 | B08, S05 |
| `internal/service/chat_orch_session_run_lifecycle.go` | 修改 | B09 |
| `internal/agent/trpc_runtime.go` | 修改 | B07 |
| `pkg/trpc-agent-go/runner/runner.go` | 修改 | B07 |
| `web/src/stores/chat/sessionResourceManager.ts` | 新增 | B14 |
| `web/src/stores/chat/messageStore.ts` | 修改 | B14 |
| `web/src/realtime/ws-transport.ts` | 修改 | B10 |
| `web/src/features/chat/composables/useChatInboundSync.ts` | 修改 | S09, S10 |
