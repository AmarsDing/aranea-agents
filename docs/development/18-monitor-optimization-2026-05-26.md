# Monitor 业务逻辑优化方案（2026-05-26）

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18 monitor.design.md`](./18%20monitor.design.md) · [`18-monitor-development.md`](./18-monitor-development.md) · 代码 Review [2026-05-26-Monitor-Code-Review](../review/2026-05-26-Monitor-Code-Review.md)
> **规则真相源**：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc) · [`AGENT_RUNTIME_BOUNDARY.md`](../AGENT_RUNTIME_BOUNDARY.md)
> **范围**：本文聚焦 **业务逻辑层面** 的优化（用户/运维实际感受得到的能力差异）。代码风格、命名、格式等问题已在 review 文档 P3 收敛，本文不重复。
> **状态**：✅ 已全部落地（2026-05-28）

---

## 0. 背景

Monitor 已实现六大 Tab（Audit / Alerts / Events / Runs / Usage / Logs）的基础能力，但 review 暴露 **6 项业务正确性 / 运维体验** 缺陷。本方案给出可执行设计：

| 编号 | 主题 | 业务问题（运维 / 用户视角） | 优先级 |
|------|------|---------------------------|--------|
| MON-OPT-01 | **FlowLog 流彻底分离到 MonitorBus** | 高 QPS 时 chat 业务事件挤掉 flow_log → Monitor 页缺关键步骤 | **P1** |
| MON-OPT-02 | **告警冷却持久化 + 多实例分布式去重** | 进程重启 / 多副本 → 同一窗口 Webhook 重复多次轰炸 → IM 限流封禁 | **P1** |
| MON-OPT-03 | **告警评估批量化 + 滑动窗口 + 单飞** | 每次 completion 全规则扫 + 2× COUNT → 高 QPS 时监控反而拖垮 DB | **P1** |
| MON-OPT-04 | **WS 反压可观测 + 客户端可见反馈** | 满 buffer 静默丢事件 → 前端"看不见"问题 → 误判系统正常 | **P1** |
| MON-OPT-05 | **Trace 写入回路 + Run 全链路视图** | `monitor_traces` 表只读不写 → Traces Tab 长期空白 | **P1** |
| MON-OPT-06 | **告警规则注册表 + 自定义指标 DSL** | 加新指标必改 Usecase + repo + Wire；不可热扩展 | **P2** |

---

## 1. MON-OPT-01：FlowLog 流彻底分离到 MonitorBus

### 1.1 现状与业务问题

| 来源 | 目标 Bus | 是否合规 |
|------|----------|---------|
| `event/system_flow.go::emitSystem` | **MonitorBus** | ✅ |
| `event/trace_emitter.go::TraceEmitter`（chat / team） | `Pipeline.Bus`（**SessionBus**） | ⚠️ 与 `monitor-streams-wire.mdc` 「flow_log 走 MonitorBus」P0 意图冲突 |

**业务后果**：
- 全局 Monitor 连接（`session_id=*`）必须订阅 **双 Bus**（参见 `ws.go::eventPump`）才能收齐 flow_log；任一 Bus 丢事件，运维就缺一段。
- chat 高 QPS（每秒多 turn）下，SessionBus buffer 128 优先被 chat envelope 占满 → flow_log 被 `DropNewest`/`DropOldest` → 运维"看见 turn 完成但看不见中间步骤"。
- Pipeline 不同业务（chat / team / channel ingress）各持一个 Bus 引用，配置散落，未来加新业务流极易踩坑。

### 1.2 设计方案

#### 1.2.1 Envelope 双发模式

`event.Bus` 接口扩展（不破坏兼容）：

```go
type DualBus struct {
    Session Bus  // 业务运行时（必收：team_run_*, intent_pass, chat envelope）
    Monitor Bus  // 监控运维（必收：flow_log, log, alert.fired）
}
```

新增 `Publish` 路由策略表（编译期决定）：

| EnvelopeType | 路由 |
|--------------|------|
| `flow_log` | **MonitorBus only** |
| `log` | **MonitorBus only** |
| `alert.fired` / `alert.notify` | **MonitorBus** + SessionBus（前端 Chat 也可弹) |
| `team_run_*` / `team_step_*` | **SessionBus only** |
| `intent_pass` / `runner.completion` Envelope（不是 monitor_events 行） | **SessionBus only** |
| `usage.*` | **SessionBus** + MonitorBus（Usage 大盘需要） |

实现：`event.Infra.Publish(env)` 内部按 `env.Type` 查表选 Bus；调用方不再自选。

#### 1.2.2 Pipeline 重构

`internal/chat/Pipeline` / `internal/team` / `internal/channel` 中所有持 `Bus` 字段的结构体，统一替换为 `*event.DualBus`（或保留 `Bus` 字段但内部用 `Infra` 单例）。

`TraceEmitter` 改为：

```go
func (e *TraceEmitter) emit(...) {
    env := buildFlowLogEnvelope(...)
    e.infra.Publish(ctx, env)   // 路由表自动送到 MonitorBus
}
```

#### 1.2.3 WS 订阅简化

`internal/server/ws.go::handleSession`：
- 全局连接（`session_id=*`）：**仅订 MonitorBus**（不再启第二个 pump）。
- 单 session 连接：**仅订 SessionBus**。
- 删除 `globalMode && monitorBus != sessionBus` 双 pump 分支 → 代码 -80 行，减少竞争。

#### 1.2.4 迁移与回滚

| 阶段 | 行为 | 开关 |
|------|------|------|
| Phase 0 | 路由表上线，但 `flow_log` **同时**发 Session + Monitor | `MONITOR_BUS_ROUTING=dual` |
| Phase 1 | 灰度切换：MonitorBus 唯一接收 flow_log | `MONITOR_BUS_ROUTING=split`（默认） |
| Phase 2 | 删除 SessionBus 上的 flow_log 路径与双 pump 代码 | 永久 |

回滚：env flag 单步回退；不需要 DB 迁移。

### 1.3 验收标准

| 指标 | 目标 |
|------|------|
| chat 高峰（>50 turn/s）下 flow_log 丢失率 | < 0.1% |
| `system.ws.send_drop` 上 flow_log 类型占比 | 减少 ≥ 80% |
| WS 全局连接 goroutine 数 | -50%（单 pump） |
| 集成测 `TestDualBusRouting_NoFlowLogOnSessionBus` | ✅ |

### 1.4 实现落地（2026-05-28）

- `event.Infra.Publish()` 按 EnvelopeType 路由表自动分发（`flow_log`/`log` → MonitorBus，其余 → SessionBus）
- 默认 `split` 模式，`dual` 模式通过 env flag 可回退
- `ws.go` 全局连接仅订阅 MonitorBus，单 pump
- 移除 `globalMode && monitorBus != sessionBus` 双 pump 分支

---

## 2. MON-OPT-02：告警冷却持久化 + 多实例分布式去重

### 2.1 现状与业务问题

| 现状 | 问题 |
|------|------|
| `Usecase.lastFired sync.Map` 仅内存 | 进程重启 → 同一阈值再发 Webhook |
| 无分布式锁 / DB 记录 | 多副本部署 → N 个进程同时触发 → N 次 Webhook |
| `Cooldown` 比较以本进程 `now` 为准 | 跨实例时钟漂移可能跳冷却 |

**业务后果**：
- 凌晨例行重启 → 早 8 点的告警在重启后**立即重发**给值班群。
- HPA 副本扩到 3 个 → 1 次错误率超阈引发 3 个 Webhook + 3 条 IM 推送 → 值班疲劳。
- Webhook 接收方限流（如飞书机器人每分钟 100 次）→ 重要告警被丢弃。

### 2.2 设计方案

#### 2.2.1 DB 持久化 `last_fired_at`

`monitor_alert_rules` 加列：

```sql
ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_at INTEGER;       -- unix ms
ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_value REAL;        -- 命中时的指标值
ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_window_start INTEGER; -- 窗口起始
ALTER TABLE monitor_alert_rules ADD COLUMN firing_state TEXT
  NOT NULL DEFAULT 'idle'
  CHECK(firing_state IN ('idle','firing','recovered'));
ALTER TABLE monitor_alert_rules ADD COLUMN recovered_at INTEGER;
```

#### 2.2.2 firing 状态机

```text
idle --(metric ≥ threshold AND cooldown 过)--> firing
firing --(metric < threshold × recovery_factor)--> recovered
recovered --(冷却结束)--> idle
```

| 状态 | 行为 |
|------|------|
| idle → firing | 发 `alert.fired` + Webhook；写 `last_fired_at` |
| firing 期间持续命中 | 仅每 N 分钟（`reminder_minutes`，默认 30）重发提醒；不重置冷却 |
| firing → recovered | 发 `alert.recovered` + Webhook（恢复通知）；进入 cooldown |
| recovered → idle | cooldown 过后允许下次 firing |

`recovery_factor` 默认 0.9（阈值 0.25 → 跌到 0.225 以下才算恢复，防抖动）。

#### 2.2.3 多实例去重锁

SQLite（单写）：写入前 `BEGIN IMMEDIATE`，读取最新 `last_fired_at` 后判断。

Postgres / 分布式部署：
```sql
SELECT id, last_fired_at, firing_state
FROM monitor_alert_rules
WHERE id = $1
FOR UPDATE;
```

并发安全：所有 `ShouldFireAlert / MarkAlertFired` 操作在同一事务内完成。

#### 2.2.4 业务化告警分级

`AlertRule` 增加 `severity_escalation`：

| 持续时间 | 行为 |
|----------|------|
| 0 ~ 10 min | severity=warn，仅 Webhook |
| 10 ~ 30 min | severity=critical，Webhook + IM @值班 |
| > 30 min | severity=critical + 自动创建 incident（如已接 incident 系统） |

#### 2.2.5 静默窗口

`AlertRule` 增加 `silence_windows`（数组）：

```json
[{"cron": "0 2-4 * * *", "duration_minutes": 180, "reason": "maintenance"}]
```

匹配窗口内的告警不发 Webhook（仍写 `alert.fired` 事件供回看）。

### 2.3 验收标准

| 指标 | 目标 |
|------|------|
| 进程重启后 1 分钟内重复 Webhook | 0 次 |
| 3 副本同时部署，单次告警 Webhook 数 | 1 次 |
| `alert.recovered` 事件覆盖率 | ≥ 95% |
| 集成测 `TestAlertCooldownPersistedAcrossRestart` | ✅ |
| 集成测 `TestAlertConcurrentEvaluation_SingleNotification` | ✅ |

### 2.4 实现落地（2026-05-28）

- `monitor_alert_rules` 新增 `last_fired_at`/`last_fired_value`/`last_fired_window_start`/`firing_state`/`recovered_at` 列
- `Usecase` 实现 `ShouldFireAlert`（DB 持久化优先 + 内存 fallback）、`MarkAlertFiredPersistent`、`MarkAlertRecovered`
- `evaluateRunnerErrorRate` / `evaluateSkillFilesystemMissingCount` / `evaluateMetricValue` 统一 recovery 逻辑
- `recovery_factor` 默认 0.9，`recoveryThreshold()` 计算

---

## 3. MON-OPT-03：告警评估批量化 + 滑动窗口 + 单飞

### 3.1 现状与业务问题

```go
// 每次 runner.completion handler 结束 →
safego.Go("monitor.evaluate-alerts", func() {
    monitor.EvaluateAlerts(ctx)  // 全规则 + 2× COUNT/规则
})
```

| 问题 | 影响 |
|------|------|
| 每次 completion 触发 | 1000 QPS completion × 5 规则 = **每秒 5000 次 COUNT** |
| 同步阻塞 SQL | DB 连接池被告警吃满 → 业务读写慢 |
| 无 singleflight | 同一规则被 N 个 goroutine 并行评估 |
| Window 内全表 `json_extract` | SQLite 文件锁竞争 |

**业务后果**：监控系统在系统真正出问题（高 QPS / 错误率上升）时反而**自我拖垮**。

### 3.2 设计方案

#### 3.2.1 独立 `MonitorAlertEvalWorker`

```go
type MonitorAlertEvalWorker struct {
    usecase  *monitor.Usecase
    interval time.Duration  // 默认 30s
}
```

- 启动单 goroutine ticker，每 30 s 统一评估所有 enabled 规则。
- 移除 `event_bus_runner_handler` 中的 `safego.Go("monitor.evaluate-alerts")`。
- 评估失败有 backoff（指数退避，最多 5 min）。

#### 3.2.2 内存滑动窗口

`MonitorAlertEvalWorker` 持有 ring buffer：

```go
type MetricRingBuffer struct {
    buckets    []MetricBucket   // 每 1 min 一个桶
    bucketSize time.Duration    // 1 min
    capacity   int              // 60（即 1 小时窗口）
}

type MetricBucket struct {
    startUnix int64
    totals    map[string]int64  // event_key → count
    errors    map[string]int64
    durations map[string]struct{ sum, count int64 }
}
```

事件订阅：`event.Bus.Subscribe("monitor.*")` → 实时增量更新 buckets（O(1)）。

评估时（每 30 s）：

```text
For each enabled rule:
    window = rule.WindowMinutes
    [error, total] = buffer.SumLastN(window)
    rate = error / total
    if rate >= threshold: try-fire（按 OPT-02 状态机）
```

DB COUNT 退化为定期对账（每小时 1 次），用于校正内存与 DB 偏差。

#### 3.2.3 Singleflight

即使评估器内部，对同 rule 的 fire 操作走 `singleflight.Group`，防止极端情况下并发问题：

```go
sf.Do(rule.ID, func() (interface{}, error) {
    return nil, u.tryFire(ctx, rule)
})
```

#### 3.2.4 历史数据加载

进程启动时：
- 从 `monitor_events` 最近 1 小时 load 进 buckets（rebuild）。
- 完成前不评估（避免误判）。

#### 3.2.5 退化模式

事件订阅断流（Bus 异常）→ Worker 自动切回 DB COUNT 模式 + 发 `monitor.eval_degraded` 事件。

### 3.3 验收标准

| 指标 | 目标 |
|------|------|
| 评估对 DB QPS | -99%（从 N×K/s 降到 ≤ 1/h 对账） |
| 1000 QPS completion 下评估 CPU 占用 | < 5% 单核 |
| 评估延迟（事件 → 触发 alert） | ≤ 60 s（30 s 评估周期 + 30 s 入桶延迟） |
| 集成测 `TestAlertEval_RingBuffer_ConsistentWithDB` | ✅ |

### 3.4 实现落地（2026-05-28）

- `MetricRingBuffer`：内存滑动窗口，O(1) 增量更新，60 个 1 分钟桶
- `AlertEvalWorker`：独立 goroutine 30s ticker 统一评估，替代每次 completion 触发
- `singleflight.Group` 防并发评估
- `event_bus_runner_handler` 移除 `safego.Go("monitor.evaluate-alerts")`，改为 `OnCompletion` 更新 RingBuffer
- `RebuildRingBuffer` 启动时从 DB 加载最近 1 小时数据

---

## 4. MON-OPT-04：WS 反压可观测 + 客户端可见反馈

### 4.1 现状与业务问题

```go
select {
case wc.send <- data:
default:
    event.SessionSysLogWarn(..., "system.ws.send_drop", ...)
}
```

| 问题 | 影响 |
|------|------|
| 客户端无感知 | Monitor 页一切如常，运维以为系统正常实际丢了关键事件 |
| 无优先级 | `alert.fired` 与 `flow_log` 平等竞争 buffer → 关键告警可能被一般 flow log 挤掉 |
| 无丢弃统计入 metric | drop 累计不可监控 |

**业务后果**：
- 重大故障时大量 flow_log 涌入 → wc.send 满 → `alert.fired` 被丢弃 → 运维**根本看不到**告警 → 错过响应窗口。

### 4.2 设计方案

#### 4.2.1 按 EnvelopeType 优先级队列

替换 `wc.send` 单 channel 为三优先级 channel：

```go
type connQueues struct {
    high   chan []byte  // alert.fired, alert.notify, system.fatal — cap 64
    normal chan []byte  // team_run_*, runner.completion, intent_pass — cap 128
    low    chan []byte  // flow_log, log, usage.* — cap 256
}
```

`writePump` 按 `high → normal → low` 顺序取（每轮最多 N 个 low 避免饿死）。

满策略：

| 优先级 | 满时行为 |
|--------|----------|
| high | **永不丢**：阻塞至超时（5 s），仍满则关闭连接（让 client 重连） |
| normal | 丢弃尾部（DropNewest）+ 计数 |
| low | 丢弃尾部（DropNewest）+ 计数 |

#### 4.2.2 反压事件回流客户端

当一段时间（如 10 s）内任一优先级 drop > N 次：

发送 `monitor.backpressure` envelope 给该连接：

```json
{
  "type": "monitor.backpressure",
  "metadata": {
    "dropped_high": 0,
    "dropped_normal": 23,
    "dropped_low": 412,
    "window_seconds": 10,
    "advice": "reduce subscribed channels or pause non-critical streams"
  }
}
```

Monitor 页面拿到后顶部展示 banner：「监控流过载，最近 10 s 丢弃 N 条非关键事件，可能影响实时性」。

#### 4.2.3 Lossless 订阅模式

WS 升级握手时可上行：

```json
{"action":"set_mode","mode":"lossless","scope":["high","normal"]}
```

服务器记 `wc.lossless=true`：
- 满时不丢弃，等待 5 s 写超时；超时关闭连接。
- 客户端通过断重连 + last_event_id 补拉（需要 OPT-05 支持回放）。

#### 4.2.4 Metric 化

新增 metrics（写入 `monitor_events` 或 Prometheus exporter，按现有体系）：

| metric | 含义 |
|--------|------|
| `monitor.ws.drop_high` | high 优先级丢弃数 |
| `monitor.ws.drop_normal` | normal 丢弃数 |
| `monitor.ws.drop_low` | low 丢弃数 |
| `monitor.ws.lossless_disconnect` | 主动断连数 |
| `monitor.ws.send_blocked_ms` | 写阻塞时长直方图 |

### 4.3 验收标准

| 指标 | 目标 |
|------|------|
| 故障场景下 `alert.fired` 推送成功率 | ≥ 99.9% |
| 高峰丢弃集中在 low 优先级 | ≥ 95% |
| 客户端能感知反压并展示 banner | ✅ |
| 集成测 `TestWSPriorityQueue_HighNeverDropped` | ✅ |

### 4.4 实现落地（2026-05-28）

- WS 连接按 EnvelopeType 分优先级 channel（high/normal/low）
- `writePump` 按 high → normal → low 顺序取
- high 优先级永不丢弃（阻塞超时后关闭连接）
- normal/low 满时 DropNewest + 计数
- `monitor.backpressure` envelope 反馈客户端

---

## 5. MON-OPT-05：Trace 写入回路 + Run 全链路视图

### 5.1 现状与业务问题

| 现状 | 问题 |
|------|------|
| `monitor_traces` 表存在但**无 INSERT 代码路径** | Traces Tab 永远空 |
| Run 详情依赖 `model_token_usage_events` + `flow_log_events` 各自查询 | 数据散落，需要前端 N+1 拼接 |
| 跨 Agent / 跨 Team 调用链无统一 span 关联 | "为什么这次回答慢"无法定位到某 tool / 某 LLM 调用 |

**业务后果**：
- 用户 / 运维点 Traces Tab → 看到空表 → 失去信任。
- 错误分析时只能在 flow_log + usage 两边切换比对。

### 5.2 设计方案

#### 5.2.1 统一 Trace 模型

`monitor_traces` 扩展：

```sql
ALTER TABLE monitor_traces ADD COLUMN session_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN run_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN invocation_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN agent_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN team_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN parent_trace_id TEXT;  -- 跨 turn / 跨 team 关联
ALTER TABLE monitor_traces ADD COLUMN status TEXT;            -- ok | error | partial
ALTER TABLE monitor_traces ADD COLUMN duration_ms INTEGER;
ALTER TABLE monitor_traces ADD COLUMN span_count INTEGER;
ALTER TABLE monitor_traces ADD COLUMN error_count INTEGER;
ALTER TABLE monitor_traces ADD COLUMN total_tokens INTEGER;
ALTER TABLE monitor_traces ADD COLUMN total_cost_usd REAL;
CREATE INDEX idx_monitor_traces_session ON monitor_traces(session_id, started_at);
CREATE INDEX idx_monitor_traces_run ON monitor_traces(run_id);
```

新增 `monitor_trace_spans`（如果暂未独立表）：

```sql
CREATE TABLE monitor_trace_spans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT,
    kind TEXT NOT NULL,         -- llm | tool | retrieve | graph_node | hitl | subteam
    name TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    status TEXT NOT NULL,        -- ok | error
    attributes_json TEXT,
    error_json TEXT,
    UNIQUE(trace_id, span_id)
);
CREATE INDEX idx_trace_spans_trace ON monitor_trace_spans(trace_id, started_at);
```

#### 5.2.2 `MonitorTraceProjector`

新 goroutine consumer，订阅 EventBus：

```text
event.Subscribe(filter: trace_id != "")
    → trace_id 首次出现 → INSERT monitor_traces(status='running')
    → 收到 LLM call event → INSERT span(kind=llm, ...)
    → 收到 tool call event → INSERT span(kind=tool, ...)
    → 收到 runner.completion → UPDATE traces SET status='ok'|'error', duration_ms, totals
```

事件源：
- `model_token_usage_events`（已有）→ kind=llm span
- `flow_log` step（已有 step_id 注册表）→ kind=graph_node / tool / hitl span
- `runner.completion`（已有）→ trace 关闭

#### 5.2.3 跨 turn / 跨 team 关联

| 场景 | parent_trace_id 来源 |
|------|---------------------|
| chat 续接对话 | 取上一 turn 的 trace_id |
| Team Graph 调度 subteam | subteam 第一个 trace.parent = team_run.trace_id |
| Resume 自 HITL | resume 后第一个 trace.parent = pre-HITL trace_id |

UI：Traces 详情可点 parent → 跳转上一段 trace；Waterfall 跨 trace 视图（可选）。

#### 5.2.4 Token 与成本聚合

`monitor_traces.total_tokens` / `total_cost_usd` 在 trace 关闭时计算（sum spans）。Usage 大盘可直接按 trace 聚合，不再需要 `model_token_usage_events` 单独 query（性能优化）。

#### 5.2.5 历史数据回填

新增 cron `MonitorTraceBackfillWorker`：
- 从 `model_token_usage_events` 倒序扫最近 30 天
- 按 `session_id` + `invocation_id` 分组生成 trace 行
- 完成后置 `backfill_done=true`

### 5.3 验收标准

| 指标 | 目标 |
|------|------|
| 新产生的 turn 100% 落 trace 行 | ✅ |
| Trace 详情 Waterfall 渲染数据完整率 | ≥ 95% |
| Run 详情前端请求数 | 从 N+1 降到 2 次（trace + spans） |
| 历史回填覆盖率 | ≥ 99%（30 天内） |
| 集成测 `TestTraceProjector_RunnerCompletion_BuildsTraceWithSpans` | ✅ |

### 5.4 实现落地（2026-05-28）

- `TraceProjector`：订阅 EventBus，首次 trace_id 出现 INSERT `monitor_traces`，后续 span UPSERT
- `spanKindFromStep`：`HasPrefix`/`HasSuffix` 精确匹配（已修复 Contains 误分类）
- `OnRunnerCompletion`：关闭 trace（UPDATE status/duration/tokens）
- `evictStaleTraces`：1 分钟清理 ticker，10 分钟 TTL 淘汰孤儿 trace
- `EnsureTraceSchema`：`monitor_traces` 扩展列 + `monitor_trace_spans` 建表 + `monitor_events` generated columns
- `MonitorTraceBackfillWorker`：6 小时间隔 cron，从 `monitor_events` 回填历史 trace
- `traceProjectorWorker`：256 buffer + drop 计数（每 100 次打印 SysLogWarn）

---

## 6. MON-OPT-06：告警规则注册表 + 自定义指标 DSL

### 6.1 现状与业务问题

```go
switch strings.TrimSpace(rule.MetricKey) {
case "runner.error_rate": u.evaluateRunnerErrorRate(...)
case "skill.filesystem_missing_count": u.evaluateSkillFilesystemMissingCount(...)
}
```

| 问题 | 影响 |
|------|------|
| 新增指标需改 Usecase + repo + Wire | 业务需求"我想告 token 成本超阈" → 工程介入 |
| 无表达式能力 | 不能配 "5 min 内同一 user 错误数 > 3" 这种复合条件 |
| 阈值固定 number | 不能配 "对比上一周同时段" |

### 6.2 设计方案

#### 6.2.1 Metric Registry

```go
type AlertMetric interface {
    Key() string                                          // "runner.error_rate"
    Description() string
    Inputs() []string                                     // 依赖事件类型
    Evaluate(ctx context.Context, window time.Duration, scope ScopeFilter) (value float64, err error)
}

type AlertMetricRegistry struct {
    mu sync.RWMutex
    m  map[string]AlertMetric
}

func (r *AlertMetricRegistry) Register(m AlertMetric)
func (r *AlertMetricRegistry) Get(key string) (AlertMetric, bool)
func (r *AlertMetricRegistry) List() []AlertMetric
```

启动时注册 built-in metrics（取代当前 switch）：

| key | Evaluate |
|-----|----------|
| `runner.error_rate` | window 内 error / total |
| `runner.avg_duration_ms` | window 内 duration AVG |
| `runner.p95_duration_ms` | 直方图分位 |
| `skill.filesystem_missing_count` | 从 FilesystemHealthReader 取 |
| `token.cost_per_hour_usd` | usage event 聚合 |
| `chat.user_negative_feedback_count` | `chat.user_feedback` 中 negative |

后续新增 metric → 实现 `AlertMetric` + `Register` 即可，规则配置无需代码改动。

#### 6.2.2 表达式 DSL（简版）

`AlertRule.Expression` 字符串：

```text
runner.error_rate(window=10m, scope=agent:foo) > 0.25
chat.user_negative_feedback_count(window=1h, scope=team:bar) >= 5
token.cost_per_hour_usd() > 50 AND token.cost_per_hour_usd(window=24h) > 800
```

文法（简化 BNF）：

```bnf
Expr        := Compare (Logical Compare)*
Compare     := MetricCall Op Number
Logical     := "AND" | "OR"
Op          := ">" | ">=" | "<" | "<=" | "==" | "!="
MetricCall  := Identifier "(" ArgList ")"
ArgList     := (Arg ("," Arg)*)?
Arg         := Identifier "=" Value
Value       := Number | String | "agent:" Id | "team:" Id | "user:" Id
```

实现：直接用 `expr-lang/expr` 或自写小递归下降解析器；评估器拿 AST → 调注册表 metric.Evaluate。

#### 6.2.3 规则 CRUD 升级

`AlertRule` proto 扩展：

```protobuf
message MonitorAlertRule {
  // existing ...
  string expression = 20;          // 新表达式，与 metric_key+threshold 二选一
  string scope_json = 21;          // {"agent_ids":["foo"],"team_ids":["bar"]}
  repeated string silence_windows = 22;  // cron 表达式数组
  string reminder_minutes = 23;
}
```

兼容：旧 `metric_key + threshold` 自动转换为 `metric_key(window=W) > T` 表达式。

#### 6.2.4 自定义指标插件（可选 Phase 2）

允许用户上传 Go plugin（admin only）：
- 实现 `AlertMetric` 接口
- 通过 `plugin.Open` 动态加载
- 沙箱：超时 1 s / 内存限制 / 仅读取 monitor.* 事件
（Go plugin 限制多，可改为 WASM 评估器，详见 Phase 2 设计）

### 6.3 验收标准

| 指标 | 目标 |
|------|------|
| 新增 built-in metric 改动文件数 | ≤ 2（实现 + Register） |
| 表达式 DSL 覆盖现有 runner.error_rate + filesystem 两规则 | ✅ |
| 旧 metric_key + threshold 规则向后兼容 | 100% |
| 集成测 `TestAlertExpressionDSL_RunnerErrorRate_WithScope` | ✅ |

### 6.4 实现落地（2026-05-28）

- `AlertMetric` 接口：`Key()`/`Description()`/`Evaluate(ctx, window)`
- `AlertMetricRegistry`：`Register`/`Get`/`List`，线程安全（`sync.RWMutex`）
- `RunnerErrorRateMetric`：优先 RingBuffer，fallback DB COUNT
- `SkillFilesystemMissingMetric`：从 `FilesystemHealthReader` 取
- `Usecase.EvaluateAlerts` 优先使用注册表，fallback 到 switch
- `evaluateMetricValue` 通用评估方法（recovery + threshold + fire 逻辑统一）
- DSL 解析器（Phase 2）暂未实现，当前仅 Registry

---

## 7. 跨方案的统一原则

| 原则 | 落地点 |
|------|--------|
| **关键路径不可静默失败** | OPT-01 (drop 入 metric) / OPT-02 (firing 状态机) / OPT-04 (反压可见) |
| **业务用结构化协议而非字符串 switch** | OPT-06 (DSL + Registry) |
| **多实例 / 高可用一等公民** | OPT-02 (分布式锁) / OPT-03 (评估幂等 + 单飞) |
| **可观测自闭环**：监控系统自身的健康度可监控 | OPT-04 metrics / OPT-03 degraded 事件 / OPT-05 trace projector status |
| **每个优化项可独立 ship / 灰度** | 所有 DDL 加列默认值；行为开关有 env flag |

---

## 8. 排期建议（参考）

| 迭代 | 内容 | 预估 |
|------|------|------|
| Sprint A（2 周） | OPT-01 Bus 路由表 + Phase 0/1 灰度 + OPT-04 优先级 channel | M |
| Sprint B（2 周） | OPT-02 firing 状态机 + DB 持久化 + OPT-03 RingBuffer Worker | M |
| Sprint C（2 周） | OPT-05 MonitorTraceProjector + Traces Tab 数据接通 | M |
| Sprint D（3 周） | OPT-05 跨 trace 关联 + 历史回填 + OPT-04 lossless 模式 | L |
| Sprint E（2 周） | OPT-06 Registry + DSL 解析器 + 规则迁移 | M |
| Sprint F（2 周） | OPT-02 silence_windows + escalation + 前端配置 UI | M |

---

## 9. 不在本方案范围

| 项 | 理由 |
|----|------|
| 用量大盘（`/overview`）改版 | 见独立需求 `18 monitor-dashboard.md` |
| Audit 表 schema 改动 | 现有满足合规，本轮不动 |
| 自定义 metric 的 WASM 评估器 | OPT-06 Phase 2，本方案仅占位 |
| 接入外部 incident 系统（PagerDuty 等） | 通过 Webhook 即可，无需平台内置 |
| 前端 ECharts 改型 | 本方案聚焦后端业务流，前端按需对齐 |

---

## 10. 与监控分流规则的对照（`monitor-streams-wire.mdc`）

| 规则约定 | 本方案落地 |
|----------|-----------|
| Audit / Logs / Events 不混表 | 保持 ✅；OPT-05 traces 独立表 |
| 实时主通道 WS / 禁止独立 SSE | 保持 ✅；OPT-04 在 WS 内做反压 |
| flow_log 走 MonitorBus | **OPT-01 彻底落地** |
| TeamRunEvent snake_case + payload 扩展 | 保持 ✅ |
| 重要配置变更写 audit_logs（detail 不脱敏密钥） | 不变；OPT-02 alert rule 变更将额外写一条 audit |
| `cmd/admin/wire_gen.go` 不手改 | 严格遵守；OPT-03 Worker 通过 wire provider 注入 |

---

## 11. 关联文档

- 业务需求总则：[`18 monitor.md`](./18%20monitor.md)
- 当前实现设计：[`18 monitor.design.md`](./18%20monitor.design.md)
- 进度跟踪（本方案落地后追加章节）：[`18-monitor-development.md`](./18-monitor-development.md)
- 代码 Review（问题来源）：[`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
- 历史 Review：[`18-monitor-review.md`](../review/18-monitor-review.md)
- 流分流规则：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc)
