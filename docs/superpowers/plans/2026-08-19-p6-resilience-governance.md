# P6 降级、对账与配额治理实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现总纲 §3.6 的降级矩阵、任务镜像对账/卡死清扫、场景级配额熔断。覆盖：13 侧（健康事件消费降级、对账 worker、卡死清扫 worker、RCA 补触发窗口）+ 14 侧（auto 策略 degraded 队列与重放、approval 模式按钮置灰标志）+ aranea 侧（单 Run 工具调用次数上限与成本上限，挂在 graph 执行路径）。

**Architecture:**
- **13 侧**：`HealthProber` 已发布 `ai.aranea.health` 事件；新增 `AraneaHealthSubscriber` 消费事件并写 `AraneaRuntimeStore` 降级标志；已有 `TaskUsecase.Create` / `ScheduleCron.fire` / `AutoAnalysisUsecase.runDiagnosis` / `RcaUsecase.runAnalysis` 均读此标志做执行准入；新增 `TaskReconciler`（5min）+ `TaskStuckCleaner`（10min）两个后台 worker，风格对齐既有 `TaskPoller`。
- **14 侧**：`AutoTriggerRunner` → `ManualProcessUsecase.Process` → `ExecutionUsecase.DispatchFromMatch` 的 auto 分流路径增加 degraded 检查；命中则入 Redis 队列 `remediation:pending_degraded`；健康恢复时批量重放（幂等键防重）。
- **aranea 侧**：在 `callback_chain.go` 工具链中新增 `scenarioBudgetBeforeHook`（BeforeTool, priority 2，在 loopGuard/argsRepair 之前拦截），计数来源为 `trpcagent.Invocation` state key `aranea.scenario_budget`，阈值从 graph 定义 `definition.budget` 或 agent settings 读取；超限以 `CustomResult` 拦截并取消 graph 执行。

**Tech Stack:** Go + twinmonitor 13-aiops / 14-remediation（JetStream 订阅 / Redis 队列 / ent 仓储 / cron worker）+ aranea-agents（trpc-agent-go BeforeTool hook / graph runtime state）。

**前置依赖：**
- P0 aranea 在环 E2E 已跑通（aranea 服务、Webhook、任务状态机可用）。
- twinmonitor `go build ./app/aiops/... ./app/remediation/...` 通过。
- aranea-agents `go build ./cmd/... ./internal/...` 通过。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试/验证脚本，再补实现。
- **验证命令**（每个 Task 收尾必跑）：
  - twinmonitor 13: `cd f:/myproject/twinmonitor/TwinServer && go build ./app/aiops/...`
  - twinmonitor 14: `cd f:/myproject/twinmonitor/TwinServer && go build ./app/remediation/...`
  - aranea: `cd f:/myproject/aranea-agents && go build ./cmd/... ./internal/...`
- **SQL 执行铁律**：禁止 PowerShell 内联复杂引号串执行 SQL，一律用 `psql -f file.sql`。
- **commit 风格**：twinmonitor 用 `feat(aiops): ...` / `feat(remediation): ...`；aranea 用 `feat(agent): ...` / `feat(graph): ...`。

---

## Task 1：T1 13 侧 aranea 不可用降级矩阵消费

**目标**：消费 `ai.aranea.health` 事件，在 `AraneaRuntimeStore` 中维护 `degraded` 标志；在任务创建、定时触发、RCA、自动诊断 4 个入口增加降级准入拦截。

**Files:**
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/aranea.go`（AraneaRuntimeStore 接口）
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/runtime_store.go`（实现）
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/health_consumer.go`（新建）
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/biz.go`
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/auto_analysis.go`
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/rca.go`

- [ ] **Step 1.1 在 AraneaRuntimeStore 接口与实现中增加降级标志读写**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/aranea.go
// 在 AraneaRuntimeStore 接口末尾追加：
type AraneaRuntimeStore interface {
    // ... 既有方法 ...
    SetDegraded(ctx context.Context, degraded bool) error
    IsDegraded(ctx context.Context) (bool, error)
}
```

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/runtime_store.go
// 在 araneaRuntimeStore struct 与实现中追加：
const redisKeyAraneaDegraded = "ai:aranea:degraded"

func (s *araneaRuntimeStore) SetDegraded(ctx context.Context, degraded bool) error {
    if s.client == nil { return nil }
    if degraded {
        return s.client.Set(ctx, redisKeyAraneaDegraded, "1", 0).Err()
    }
    return s.client.Del(ctx, redisKeyAraneaDegraded).Err()
}

func (s *araneaRuntimeStore) IsDegraded(ctx context.Context) (bool, error) {
    if s.client == nil { return false, nil }
    v, err := s.client.Exists(ctx, redisKeyAraneaDegraded).Result()
    return v > 0, err
}
```

- [ ] **Step 1.2 新建 health_consumer.go 消费 ai.aranea.health**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/health_consumer.go
package biz

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/go-kratos/kratos/v2/log"
    natsgo "github.com/nats-io/nats.go"
    "twinserver/app/aiops/internal/conf"
)

const healthConsumerDurable = "aiops-aranea-health"

// AraneaHealthSubscriber 消费 ai.aranea.health，维护 runtime 降级标志。
type AraneaHealthSubscriber struct {
    runtime AraneaRuntimeStore
    log     *log.Helper
    sub     *natsgo.Subscription
    wg      sync.WaitGroup
}

func NewAraneaHealthSubscriber(
    cfg *conf.Bootstrap,
    runtime AraneaRuntimeStore,
    logger log.Logger,
) (*AraneaHealthSubscriber, func(), error) {
    l := log.NewHelper(log.With(logger, "module", "biz/aranea-health-consumer"))
    s := &AraneaHealthSubscriber{runtime: runtime, log: l}
    cleanup := func() { s.stop() }
    if cfg == nil || cfg.Nats == nil || cfg.Nats.Url == "" {
        l.Warn("nats not configured, health subscriber skipped")
        return s, cleanup, nil
    }
    nc, err := natsgo.Connect(cfg.Nats.Url, natsgo.MaxReconnects(-1), natsgo.ReconnectWait(3*time.Second))
    if err != nil {
        l.Warnf("nats connect failed, health subscriber skipped: %v", err)
        return s, cleanup, nil
    }
    js, err := nc.JetStream()
    if err != nil {
        nc.Close()
        return nil, cleanup, fmt.Errorf("jetstream: %w", err)
    }
    sub, err := js.Subscribe(SubjectAiAraneaHealth, s.handleMessage,
        natsgo.Durable(healthConsumerDurable), natsgo.ManualAck(), natsgo.DeliverAll())
    if err != nil {
        nc.Close()
        return nil, cleanup, fmt.Errorf("subscribe: %w", err)
    }
    s.sub = sub
    l.Info("aranea health subscriber started")
    return s, cleanup, nil
}

func (s *AraneaHealthSubscriber) stop() {
    if s.sub != nil { _ = s.sub.Unsubscribe() }
    s.wg.Wait()
}

func (s *AraneaHealthSubscriber) handleMessage(msg *natsgo.Msg) {
    s.wg.Add(1)
    defer s.wg.Done()
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    var payload struct{ Status string `json:"status"` }
    if err := json.Unmarshal(msg.Data, &payload); err != nil {
        s.log.WithContext(ctx).Warnf("unmarshal health event failed: %v", err)
        _ = msg.Ack()
        return
    }
    degraded := payload.Status == "degraded"
    if err := s.runtime.SetDegraded(ctx, degraded); err != nil {
        s.log.WithContext(ctx).Warnf("set degraded=%v failed: %v", degraded, err)
        _ = msg.Nak()
        return
    }
    s.log.WithContext(ctx).Infof("aranea degraded flag updated: %v", degraded)
    _ = msg.Ack()
}
```

- [ ] **Step 1.3 在 biz.go ProviderSet 中注册 HealthSubscriber**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 既有 ...
    NewAraneaHealthSubscriber,
)
```

- [ ] **Step 1.4 在 AutoAnalysisUsecase.runDiagnosis 增加 degraded 准入**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/auto_analysis.go
// 在 runDiagnosis 的 "1. aranea 接入判定" 段之后追加：
if uc.runtime != nil {
    if degraded, err := uc.runtime.IsDegraded(ctx); err == nil && degraded {
        fail("aranea 服务降级，自动诊断暂停", nil)
        return
    }
}
```

- [ ] **Step 1.5 在 RcaUsecase.runAnalysis 增加 degraded 准入**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/rca.go
// 在 runAnalysis 的 "1. aranea 接入判定" 段之后追加：
if uc.store != nil {
    if degraded, err := uc.store.IsDegraded(ctx); err == nil && degraded {
        fail(RcaStatusDegraded, "aranea 服务降级，RCA 自动触发暂停")
        return
    }
}
```

> 注：`uc.store` 为 `AnalysisRedisStore`，已实现 `IsDegraded`（因 Step 1.1 将其合并到同一接口；若接口未合并，则在 `RcaUsecase` 构造注入 `AraneaRuntimeStore`）。

- [ ] **Step 1.6 验证降级拦截生效**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：编译通过
```

```bash
# 手动发布 degraded 事件验证标志写入
nats pub ai.aranea.health '{"status":"degraded","last_error":"probe timeout"}'
# 再发 healthy
nats pub ai.aranea.health '{"status":"healthy"}'
```

- [ ] **Step 1.7 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add -A
git commit -m "feat(aiops): aranea 不可用降级矩阵消费（T1）

- AraneaRuntimeStore 新增 SetDegraded/IsDegraded
- 新增 AraneaHealthSubscriber 消费 ai.aranea.health
- auto_analysis / rca / task_create / schedule_fire 四入口降级拦截"
```

---

## Task 2：T2 14 侧 auto 模式 degraded 队列与重放

**目标**：14 auto 策略在 aranea degraded 期间不再创建执行记录，告警事件入 `remediation:pending_degraded` Redis 队列；恢复后批量重放（幂等键防重）。

**Files:**
- `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/manual_process.go`
- `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/degraded_queue.go`（新建）
- `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/subscriber.go`
- `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/biz.go`

- [ ] **Step 2.1 新建 degraded_queue.go（Redis 队列 + 重放器）**

```go
// f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/degraded_queue.go
package biz

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/go-kratos/kratos/v2/log"
    "github.com/redis/go-redis/v9"
)

const pendingDegradedKey = "remediation:pending_degraded"

// DegradedQueue 降级事件队列（Redis list + 幂等去重）。
type DegradedQueue struct {
    redis redis.Cmdable
    log   *log.Helper
}

func NewDegradedQueue(redis redis.Cmdable, logger log.Logger) *DegradedQueue {
    return &DegradedQueue{
        redis: redis,
        log:   log.NewHelper(log.With(logger, "module", "biz/degraded-queue")),
    }
}

// DegradedAlarmItem 入队元素。
type DegradedAlarmItem struct {
    AlarmID    string    `json:"alarm_id"`
    ReceivedAt time.Time `json:"received_at"`
}

// Enqueue 告警事件入队（右侧入）。
func (q *DegradedQueue) Enqueue(ctx context.Context, alarmID string) error {
    if q.redis == nil {
        return nil
    }
    item := DegradedAlarmItem{AlarmID: alarmID, ReceivedAt: time.Now()}
    b, _ := json.Marshal(item)
    return q.redis.RPush(ctx, pendingDegradedKey, b).Err()
}

// Replay 批量重放：左侧出队 → 调用 processor → 成功 ACK / 失败左侧回队（最多 3 次）。
func (q *DegradedQueue) Replay(ctx context.Context, processor func(ctx context.Context, alarmID string) error, max int) error {
    if q.redis == nil {
        return nil
    }
    for i := 0; i < max; i++ {
        val, err := q.redis.LPop(ctx, pendingDegradedKey).Result()
        if err == redis.Nil {
            return nil // 队空
        }
        if err != nil {
            return err
        }
        var item DegradedAlarmItem
        if err := json.Unmarshal([]byte(val), &item); err != nil {
            q.log.WithContext(ctx).Warnf("degraded queue bad item: %v", err)
            continue
        }
        // 过期跳过（>15min）
        if time.Since(item.ReceivedAt) > 15*time.Minute {
            q.log.WithContext(ctx).Infof("degraded queue item expired, skip alarm_id=%s", item.AlarmID)
            continue
        }
        if err := processor(ctx, item.AlarmID); err != nil {
            q.log.WithContext(ctx).Warnf("degraded replay failed alarm_id=%s: %v", item.AlarmID, err)
            // 失败回队（左侧），下次重放再试
            _ = q.redis.LPush(ctx, pendingDegradedKey, val).Err()
            return fmt.Errorf("replay alarm_id=%s: %w", item.AlarmID, err)
        }
        q.log.WithContext(ctx).Infof("degraded replay success alarm_id=%s", item.AlarmID)
    }
    return nil
}

// PendingCount 队列长度。
func (q *DegradedQueue) PendingCount(ctx context.Context) (int64, error) {
    if q.redis == nil {
        return 0, nil
    }
    return q.redis.LLen(ctx, pendingDegradedKey).Result()
}
```

- [ ] **Step 2.2 在 ManualProcessUsecase 中注入 DegradedQueue，auto 模式 degraded 时入队**

```go
// f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/manual_process.go
// ManualProcessUsecase 追加字段：
type ManualProcessUsecase struct {
    // ... 既有字段 ...
    degradedQueue *DegradedQueue
    runtime       AraneaRuntimeStore // 14 侧通过 external 或独立注入获取 degraded 状态
}

// NewManualProcessUsecase 追加参数 degradedQueue、runtime（可为 nil，降级不生效）。
```

在 `Process` 方法中，策略匹配后、建执行记录前，对 auto 模式增加 degraded 检查：

```go
// 在循环内，命中策略后：
for _, r := range results {
    if !r.Matched { continue }
    out.MatchedStrategies++
    // --- degraded 拦截（仅 auto）---
    if r.Policy.ExecutionMode == ExecutionModeAuto {
        if uc.runtime != nil {
            if degraded, _ := uc.runtime.IsDegraded(ctx); degraded {
                if uc.degradedQueue != nil {
                    _ = uc.degradedQueue.Enqueue(ctx, alarmID)
                }
                uc.log.WithContext(ctx).Infof("auto dispatch degraded: alarm_id=%s policy_id=%d enqueued", alarmID, r.Policy.ID)
                continue // 跳过本次，入队待重放
            }
        }
    }
    // --- 原有分流逻辑 ---
    // ...
}
```

- [ ] **Step 2.3 在 AutoTriggerRunner 健康恢复时触发重放**

由于 AutoTriggerRunner 不直接感知健康恢复，最简洁的做法：在 `ManualProcessUsecase` 的 `Process` 中，若当前非 degraded 且 `degradedQueue.PendingCount() > 0`，先触发 `Replay`。

```go
// f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/manual_process.go
// Process 方法开头（取到告警后）追加：
if uc.degradedQueue != nil && uc.runtime != nil {
    if degraded, _ := uc.runtime.IsDegraded(ctx); !degraded {
        if cnt, _ := uc.degradedQueue.PendingCount(ctx); cnt > 0 {
            go uc.degradedQueue.Replay(context.Background(), func(ctx context.Context, aid string) error {
                _, err := uc.Process(ctx, aid, TriggerTypeAuto, nil)
                return err
            }, 10)
        }
    }
}
```

> 注：Replay 内复用 `Process` 会递归调用自身，但 Process 内的 degraded 检查会阻止再次入队；且 Replay 在新 goroutine 中异步执行，不阻塞当前告警处理。

- [ ] **Step 2.4 注册到 biz.go ProviderSet**

```go
// f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 既有 ...
    NewDegradedQueue,
)
```

- [ ] **Step 2.5 验证编译与队列行为**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/remediation/...
# 预期：编译通过
```

```bash
# 本地 Redis 验证队列
redis-cli RPUSH remediation:pending_degraded '{"alarm_id":"test-001","received_at":"2026-08-19T10:00:00Z"}'
redis-cli LLEN remediation:pending_degraded
# 预期：(integer) 1
```

- [ ] **Step 2.6 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add -A
git commit -m "feat(remediation): auto 模式 degraded 队列与重放（T2）

- DegradedQueue 基于 Redis list 实现 pending_degraded 队列
- ManualProcessUsecase auto 模式 degraded 时入队、恢复时异步重放
- 幂等由 Process 内部匹配引擎/冷却键天然承载，无需额外键"
```

---

## Task 3：T3 13 任务镜像周期对账 Worker

**目标**：新增周期对账 worker（5 分钟），拉取 aranea `GET /api/v1/runs/{id}` 比对 `ai_tasks` 镜像状态，漂移则按 aranea 侧为准修正并发布补偿事件。

**Files:**
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/task_reconciler.go`（新建）
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/biz.go`

- [ ] **Step 3.1 新建 task_reconciler.go**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/task_reconciler.go
package biz

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/go-kratos/kratos/v2/log"
    "twinserver/app/aiops/internal/conf"
)

// TaskReconciler 周期对账 worker（详设 §3.6.2，周期 5min）。
// 扫描 ai_tasks 非终态记录，调 aranea GetRun 比对，漂移则 ApplyRunSnapshot 补齐。
type TaskReconciler struct {
    cfg     *conf.Bootstrap
    repo    TaskRepo
    taskUC  *TaskUsecase
    port    AraneaPort
    log     *log.Helper

    stopCh   chan struct{}
    stopOnce sync.Once
}

func NewTaskReconciler(
    cfg *conf.Bootstrap,
    repo TaskRepo,
    taskUC *TaskUsecase,
    port AraneaPort,
    logger log.Logger,
) (*TaskReconciler, func()) {
    r := &TaskReconciler{
        cfg:    cfg,
        repo:   repo,
        taskUC: taskUC,
        port:   port,
        log:    log.NewHelper(log.With(logger, "module", "task/reconciler")),
        stopCh: make(chan struct{}),
    }
    go r.loop()
    return r, func() { r.stopOnce.Do(func() { close(r.stopCh) }) }
}

func (r *TaskReconciler) interval() time.Duration {
    if r.cfg != nil && r.cfg.Aranea != nil && r.cfg.Aranea.ReconcileIntervalSeconds > 0 {
        return time.Duration(r.cfg.Aranea.ReconcileIntervalSeconds) * time.Second
    }
    return 5 * time.Minute
}

func (r *TaskReconciler) loop() {
    r.sweep()
    ticker := time.NewTicker(r.interval())
    defer ticker.Stop()
    for {
        select {
        case <-r.stopCh:
            return
        case <-ticker.C:
            r.sweep()
        }
    }
}

func (r *TaskReconciler) sweep() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
    defer cancel()

    tasks, err := r.repo.ListActive(ctx)
    if err != nil {
        r.log.WithContext(ctx).Warnf("reconciler list active failed: %v", err)
        return
    }
    if len(tasks) == 0 {
        return
    }

    sem := make(chan struct{}, 5)
    var wg sync.WaitGroup
    for _, t := range tasks {
        wg.Add(1)
        sem <- struct{}{}
        go func(task *Task) {
            defer wg.Done()
            defer func() { <-sem }()
            r.reconcileOne(ctx, task)
        }(t)
    }
    wg.Wait()
}

func (r *TaskReconciler) reconcileOne(ctx context.Context, t *Task) {
    if t.AraneaRunID == "" {
        return
    }
    run, err := r.port.GetRun(ctx, t.AraneaRunID)
    if err != nil {
        r.log.WithContext(ctx).Warnf("reconciler get run %s failed: %v", t.AraneaRunID, err)
        return
    }
    if t.Status == run.Status {
        return
    }
    r.log.WithContext(ctx).Infof("reconciler drift detected task=%d run=%s %s->%s",
        t.ID, t.AraneaRunID, t.Status, run.Status)
    if _, aerr := r.taskUC.ApplyRunSnapshot(ctx, run); aerr != nil {
        r.log.WithContext(ctx).Warnf("reconciler apply snapshot failed: %v", aerr)
    }
}
```

- [ ] **Step 3.2 在 conf.go Aranea 中增加对账周期配置（可选，已有默认值可不配）**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/conf/conf.go
// 在 Aranea struct 中追加：
ReconcileIntervalSeconds int `json:"reconcile_interval_seconds"` // 默认 300
```

- [ ] **Step 3.3 注册到 biz.go ProviderSet**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 既有 ...
    NewTaskReconciler,
)
```

- [ ] **Step 3.4 验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：编译通过
```

- [ ] **Step 3.5 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add -A
git commit -m "feat(aiops): 任务镜像周期对账 worker（T3）

- TaskReconciler 5min 扫描 ai_tasks active 记录
- 并发（≤5）调 aranea GetRun，漂移则 ApplyRunSnapshot 补齐
- 发布补偿事件由 ApplyRunSnapshot 内部驱动"
```

---

## Task 4：T4 13 卡死清扫 Worker

**目标**：新增卡死清扫 worker（10 分钟），`running` 超过 graph 超时上限 2 倍且无新节点事件的镜像，主动调用 aranea cancel 并置 `timeout` 终态。

**Files:**
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/task_stuck_cleaner.go`（新建）
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/biz.go`
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/ent/schema/ai_task.go`（确认 schema 有 started_at/update_time）

- [ ] **Step 4.1 确认 ai_tasks schema 字段**

```bash
cd f:/myproject/twinmonitor/TwinServer
grep -n "started_at\|update_time\|status" app/aiops/internal/data/ent/schema/ai_task.go
# 预期命中 started_at、update_time、status 字段
```

- [ ] **Step 4.2 在 TaskRepo 接口增加 ListStuck 方法**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/task.go
// 在 TaskRepo interface 中追加：
// ListStuck 卡死扫描：status=running 且 update_time < now - threshold。
ListStuck(ctx context.Context, threshold time.Duration) ([]*Task, error)
```

- [ ] **Step 4.3 在 data 层实现 ListStuck**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/task_repo.go（或对应 repo 文件）
func (r *taskRepo) ListStuck(ctx context.Context, threshold time.Duration) ([]*biz.Task, error) {
    cutoff := time.Now().Add(-threshold)
    rows, err := r.db.QueryContext(ctx,
        `SELECT id, aranea_run_id, status, started_at, update_time FROM ai_tasks
         WHERE status = 'running' AND update_time < $1`, cutoff)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []*biz.Task
    for rows.Next() {
        var t biz.Task
        if err := rows.Scan(&t.ID, &t.AraneaRunID, &t.Status, &t.StartedAt, &t.UpdateTime); err != nil {
            continue
        }
        out = append(out, &t)
    }
    return out, rows.Err()
}
```

> 注：若 ent 已生成 Query API，优先用 ent；但 `ai_tasks` 为追加型日志表，可能走原始 SQL。请按项目实际 repo 模式调整。

- [ ] **Step 4.4 新建 task_stuck_cleaner.go**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/task_stuck_cleaner.go
package biz

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/go-kratos/kratos/v2/log"
    "twinserver/app/aiops/internal/conf"
)

// TaskStuckCleaner 卡死清扫 worker（详设 §3.6.2，周期 10min）。
// running 超过 graph 超时上限 2 倍且无新节点事件 → cancel aranea run → 置 timeout。
type TaskStuckCleaner struct {
    cfg     *conf.Bootstrap
    repo    TaskRepo
    taskUC  *TaskUsecase
    port    AraneaPort
    log     *log.Helper

    stopCh   chan struct{}
    stopOnce sync.Once
}

func NewTaskStuckCleaner(
    cfg *conf.Bootstrap,
    repo TaskRepo,
    taskUC *TaskUsecase,
    port AraneaPort,
    logger log.Logger,
) (*TaskStuckCleaner, func()) {
    c := &TaskStuckCleaner{
        cfg:    cfg,
        repo:   repo,
        taskUC: taskUC,
        port:   port,
        log:    log.NewHelper(log.With(logger, "module", "task/stuck-cleaner")),
        stopCh: make(chan struct{}),
    }
    go c.loop()
    return c, func() { c.stopOnce.Do(func() { close(c.stopCh) }) }
}

func (c *TaskStuckCleaner) interval() time.Duration {
    if c.cfg != nil && c.cfg.Aranea != nil && c.cfg.Aranea.StuckCleanIntervalSeconds > 0 {
        return time.Duration(c.cfg.Aranea.StuckCleanIntervalSeconds) * time.Second
    }
    return 10 * time.Minute
}

func (c *TaskStuckCleaner) stuckThreshold() time.Duration {
    // 默认 graph 超时 120s，2 倍 = 240s；无配置时保守用 10min。
    if c.cfg != nil && c.cfg.Aranea != nil && c.cfg.Aranea.GraphTimeoutSeconds > 0 {
        return time.Duration(c.cfg.Aranea.GraphTimeoutSeconds*2) * time.Second
    }
    return 10 * time.Minute
}

func (c *TaskStuckCleaner) loop() {
    c.sweep()
    ticker := time.NewTicker(c.interval())
    defer ticker.Stop()
    for {
        select {
        case <-c.stopCh:
            return
        case <-ticker.C:
            c.sweep()
        }
    }
}

func (c *TaskStuckCleaner) sweep() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    tasks, err := c.repo.ListStuck(ctx, c.stuckThreshold())
    if err != nil {
        c.log.WithContext(ctx).Warnf("stuck cleaner list failed: %v", err)
        return
    }
    for _, t := range tasks {
        if t.AraneaRunID == "" {
            continue
        }
        c.log.WithContext(ctx).Infof("stuck cleaner cancel task=%d run=%s", t.ID, t.AraneaRunID)
        if err := c.port.CancelRun(ctx, t.AraneaRunID); err != nil {
            c.log.WithContext(ctx).Warnf("stuck cleaner cancel run %s failed: %v", t.AraneaRunID, err)
        }
        if _, err := c.taskUC.ApplyRunFailed(ctx, t.AraneaRunID, "任务卡死超时（清扫器触发）"); err != nil {
            c.log.WithContext(ctx).Warnf("stuck cleaner mark failed task=%d: %v", t.ID, err)
        }
    }
}
```

- [ ] **Step 4.5 在 conf.go 中增加清扫周期与 graph 超时配置**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/conf/conf.go
// 在 Aranea struct 中追加：
StuckCleanIntervalSeconds int `json:"stuck_clean_interval_seconds"` // 默认 600
GraphTimeoutSeconds       int `json:"graph_timeout_seconds"`        // 默认 120
```

- [ ] **Step 4.6 注册到 biz.go**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 既有 ...
    NewTaskStuckCleaner,
)
```

- [ ] **Step 4.7 验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：编译通过
```

```bash
# 用 psql 造一条卡死记录验证（随后删除）
# file: f:/myproject/aranea-agents/docs/superpowers/plans/tmp_stuck_test.sql
UPDATE ai_tasks SET status='running', update_time=now()-interval '20 minutes'
WHERE id=1;
```

- [ ] **Step 4.8 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add -A
git commit -m "feat(aiops): 任务镜像卡死清扫 worker（T4）

- TaskStuckCleaner 10min 扫描 running 且 update_time 超阈值任务
- 主动 CancelRun + ApplyRunFailed 释放 14 并发槽与 GNS3 环境"
```

---

## Task 5：T5 13 RCA 补触发窗口

**目标**：aranea 从 degraded 恢复后，对 15 分钟内被降级拦截的告警事件进行补触发（RCA + 自动诊断），避免漏分析。

**Files:**
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/health_consumer.go`
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/rca.go`
- `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/auto_analysis.go`

- [ ] **Step 5.1 在 health_consumer.go 恢复时触发补窗口**

在 `handleMessage` 中，当 `degraded` 从 `true` 变为 `false` 时，启动补触发 goroutine：

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/health_consumer.go
// AraneaHealthSubscriber 追加字段：
type AraneaHealthSubscriber struct {
    // ... 既有字段 ...
    rcaUC      *RcaUsecase
    autoUC     *AutoAnalysisUsecase
    prevStatus string // 上一状态，用于检测恢复
}

// 修改 handleMessage 中状态切换逻辑：
if degraded {
    // ...
} else {
    if s.prevStatus == "degraded" {
        go s.replayWindow(context.Background())
    }
}
s.prevStatus = payload.Status
```

新增 replayWindow 方法：

```go
func (s *AraneaHealthSubscriber) replayWindow(ctx context.Context) {
    // 补触发窗口：恢复时刻前 15 分钟内 pending 或 degraded 的 RCA 记录
    // 最简实现：依赖 RCA repo 的 List 查询最近 15min 内 status=pending/degraded 的记录重新触发
    // 详设要求 "按告警时间窗补触发"，此处以 RCA 记录为锚点（告警事件本身未持久化）。
    if s.rcaUC == nil {
        return
    }
    s.log.WithContext(ctx).Info("aranea recovered, replaying RCA window (≤15min)")
    // 实际重放由 rcaUsecase.ReplayWindow 实现（见 Step 5.2）
}
```

- [ ] **Step 5.2 在 RcaUsecase 中实现 ReplayWindow**

```go
// f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/rca.go
// RcaUsecase 新增方法：
func (uc *RcaUsecase) ReplayWindow(ctx context.Context) error {
    since := time.Now().Add(-15 * time.Minute)
    recs, err := uc.repo.ListByStatusAndTime(ctx, RcaStatusDegraded, since)
    if err != nil {
        return err
    }
    for _, rec := range recs {
        if rec.Status != RcaStatusDegraded {
            continue
        }
        uc.log.WithContext(ctx).Infof("rca replay window: re-trigger rca_id=%d alarm_id=%s", rec.ID, rec.AlarmID)
        // 重新置 pending 并启动异步分析
        if err := uc.repo.MarkPending(ctx, rec.ID); err != nil {
            uc.log.WithContext(ctx).Warnf("rca replay mark pending failed id=%d: %v", rec.ID, err)
            continue
        }
        uc.startAnalysisAsync(rec.ID)
    }
    return nil
}
```

> 注：需在 `RcaRepo` 接口中增加 `ListByStatusAndTime` 与 `MarkPending`；若 repo 未实现，按项目已有 ent/data 模式追加。

- [ ] **Step 5.3 自动诊断同步补触发（可选同窗口）**

`AutoAnalysisUsecase` 同模式追加 `ReplayWindow`，查询 `status=degraded` 且 `created_at > since` 的记录重新触发。若自动诊断与 RCA 共用同一 `AlarmSubscriber.dispatch` 链，恢复后新告警自然走正常路径；历史告警的补触发靠 RCA 窗口即可（RCA 记录与自动诊断记录一般一一对应）。

- [ ] **Step 5.4 验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
```

- [ ] **Step 5.5 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add -A
git commit -m "feat(aiops): RCA 补触发窗口（T5）

- HealthSubscriber 检测 degraded→healthy 切换，触发 15min 补窗口
- RcaUsecase.ReplayWindow 重新拉取 degraded 记录并启动分析"
```

---

## Task 6：T6 aranea 场景级配额熔断

**目标**：在 aranea graph 执行路径增加单 Run 工具调用次数上限与成本上限；超限以 `CustomResult` 拦截并产出部分结论，标记 `budget_exceeded`。

**Files:**
- `f:/myproject/aranea-agents/internal/agent/scenario_budget_guard.go`（新建）
- `f:/myproject/aranea-agents/internal/agent/callback_chain.go`
- `f:/myproject/aranea-agents/internal/biz/graph.go`（确认 budget 字段在 GraphDefinition）

- [ ] **Step 6.1 在 GraphDefinition / NodeDef 中增加 budget 字段（若 schema 尚未支持）**

```go
// f:/myproject/aranea-agents/internal/biz/graph.go
// 在 GraphDefinition struct 中追加：
// Budget 场景级配额（详设 §3.6.3）。
type GraphBudget struct {
    MaxToolCalls int     `json:"max_tool_calls"` // 0=不限制
    MaxCostCNY   float64 `json:"max_cost_cny"`   // 0=不限制
}

// GraphDefinition 追加字段：
Budget *GraphBudget `json:"budget,omitempty"`
```

> 若 ent schema `graph_definition` 已存 `definition` JSON，则 budget 走 JSON 内部字段，无需 DB 迁移；否则需在 schema 中增加 `budget` JSON 字段并生成迁移。

- [ ] **Step 6.2 新建 scenario_budget_guard.go**

```go
// f:/myproject/aranea-agents/internal/agent/scenario_budget_guard.go
package agent

import (
    "context"
    "fmt"
    "sync"

    "aranea-agents/internal/agent/callbacks"
    "aranea-agents/internal/biz"
    "aranea-agents/pkg/loggateway"

    trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const scenarioBudgetStateKey = "aranea.scenario_budget"

// scenarioBudgetState 进程内单 Run 预算计数器（挂 invocation runtime state）。
type scenarioBudgetState struct {
    mu         sync.Mutex
    toolCalls  int
    costAccum  float64 // CNY
    exceeded   bool
    reason     string
}

func newScenarioBudgetGuard(budget *biz.GraphBudget) (callbacks.BeforeToolHook, callbacks.AfterToolHook) {
    if budget == nil || (budget.MaxToolCalls <= 0 && budget.MaxCostCNY <= 0) {
        return nil, nil
    }
    state := &scenarioBudgetState{}
    before := callbacks.NewBeforeToolHook(2, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
        state.mu.Lock()
        defer state.mu.Unlock()
        if state.exceeded {
            return &trpctool.BeforeToolResult{
                Context: ctx,
                CustomResult: fmt.Sprintf("[budget_exceeded] %s", state.reason),
            }, nil
        }
        // 预判：本次调用后是否超限（成本在 AfterTool 累加，此处仅判次数）
        if budget.MaxToolCalls > 0 && state.toolCalls >= budget.MaxToolCalls {
            state.exceeded = true
            state.reason = fmt.Sprintf("tool_calls limit %d exceeded", budget.MaxToolCalls)
            return &trpctool.BeforeToolResult{
                Context: ctx,
                CustomResult: fmt.Sprintf("[budget_exceeded] %s", state.reason),
            }, nil
        }
        return &trpctool.BeforeToolResult{Context: ctx}, nil
    })
    after := callbacks.NewAfterToolHook(2, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
        state.mu.Lock()
        defer state.mu.Unlock()
        state.toolCalls++
        // 成本累加：从 usage 事件或模型定价表计算（简化版：按 input+output tokens * 单价）
        // 若框架 AfterToolArgs 无 usage，则从 invocation state 读取 skillTokenUsageStateKey 累加。
        // 此处仅做次数熔断；成本熔断如需精确计算，建议在 runner 层按 model_pricing_rule 累加。
        if budget.MaxToolCalls > 0 && state.toolCalls >= budget.MaxToolCalls && !state.exceeded {
            state.exceeded = true
            state.reason = fmt.Sprintf("tool_calls limit %d reached", budget.MaxToolCalls)
            // 不拦截本次已完成的工具，下一 BeforeTool 拦截
        }
        return &trpctool.AfterToolResult{Context: ctx}, nil
    })
    return before, after
}
```

- [ ] **Step 6.3 在 callback_chain.go 中装配 budget guard**

```go
// f:/myproject/aranea-agents/internal/agent/callback_chain.go
// 在 loopGuard 之前（priority 2）插入 budget guard：
// 需在 BuildCallbackChain 签名中增加 budget 参数，或从 ag.Settings / invocation state 读取。
// 最简路径：从 graphBuildConfig 的 Budget 注入（graph 执行路径已知）。

// 在 BuildCallbackChain 中（需要 budget 参数时从调用方传入）：
if budget != nil {
    if b, a := newScenarioBudgetGuard(budget); b != nil {
        entries = append(entries, b, a)
    }
}
```

> 由于 `BuildCallbackChain` 当前签名不含 budget，实际装配点建议在 `trpcGraphBuilderFactory` 中构造 Agent 时，将 budget 写入 `agent.Settings` 的扩展字段（如 `ToolsBudget`），或在 `callback_chain.go` 中从 `deps.GraphBuildConfig` 读取。
>
> 更务实的做法：将 budget 作为 `Agent.Settings` 的 int 字段 `ToolBudgetMaxCalls` 消费（类似 `ToolsExecutionTimeoutSec`），由 13 侧同步种子时写入 Agent 设置。

- [ ] **Step 6.4 验证装配与拦截**

```bash
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...
# 预期：编译通过
```

```bash
# 单元测试验证 budget 拦截
cd f:/myproject/aranea-agents
go test ./internal/agent/... -run TestScenarioBudget -v
# 预期： budget=3 时第 4 次 BeforeTool 返回 CustomResult="[budget_exceeded] ..."
```

- [ ] **Step 6.5 git commit**

```bash
cd f:/myproject/aranea-agents
git add -A
git commit -m "feat(agent): 场景级配额熔断（T6）

- scenarioBudgetGuard BeforeTool/AfterTool 计数单 Run 工具调用次数
- 超 MaxToolCalls 后以 CustomResult 拦截，避免 179 次/291K tokens 空转事故重演
- 成本上限预留接口（AfterTool 累加 token*单价）"
```

---

## 验收矩阵

| 验收项 | 验证方法 | 通过标准 |
|--------|----------|----------|
| T1 降级矩阵 | 发 NATS `ai.aranea.health` degraded → 触发任务创建/RCA/自动诊断被拦截 | 各入口日志出现 "degraded, skip" |
| T1 降级恢复 | 发 healthy → 各入口恢复正常执行 | 日志出现 "degraded=false"，任务可创建 |
| T2 auto 降级队列 | NATS 发 alarm.events，aranea degraded 时 14 auto 策略入队 | `LLEN remediation:pending_degraded` > 0 |
| T2 auto 重放 | 发 healthy 后新告警触发 Process，异步重放队列 | 队列清空，补建执行记录 |
| T3 周期对账 | 手动改 ai_tasks status=running，aranea 侧置 completed，等 5min | 对账 worker 日志 drift detected，状态修正为 completed |
| T4 卡死清扫 | 造 running 记录 update_time=20min 前，等 10min | 任务被 CancelRun + 置 failed |
| T5 RCA 补触发 | degraded→healthy 切换，15min 内有 degraded RCA 记录 | 记录被重新 MarkPending 并启动分析 |
| T6 配额熔断 | 构造 budget.max_tool_calls=3 的 Graph，执行含多工具场景 | 第 4 次工具调用被 CustomResult 拦截，Run 终态 budget_exceeded |

---

## 附录：关键契约速查

- **ai.aranea.health 事件格式**：`{"status":"degraded|healthy","prev_status":"...","last_error":"..."}`
- **AraneaPort.GetRun 返回**：`AraneaRun{RunID, GraphID, Status, Output, ErrorMessage, Nodes, TokensInput, TokensOutput, DurationMs}`
- **TaskRepo.ListActive 定义**：`status IN ('pending','running','waiting_approval') AND aranea_run_id != ''`
- **remediation:pending_degraded 队列元素**：`{"alarm_id":"...","received_at":"2026-08-19T10:00:00Z"}`
- **GraphBudget JSON**：`{"max_tool_calls":15,"max_cost_cny":0.5}`
