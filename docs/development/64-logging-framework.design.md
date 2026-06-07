# Logging Framework — 设计文档

> **对应需求**：[64-logging-framework.md](./64-logging-framework.md)
>
> **状态**：✅ 已完成实现（2026-06）

---

## 1. 架构总览

### 1.1 三层架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     调用层 (loggateway)                          │
│  Logger 接口 → Gateway 实现 → With() 语义 → Field 类型系统       │
└──────────────────────────┬──────────────────────────────────────┘
                           │ emitToPipeline()
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                     管道层 (logpipeline)                         │
│  Pipeline → channel 缓冲 → dispatchLoop → stepThrottler        │
│  → SinkGroup 扇出 → Sink.Write()                               │
└──────┬──────────────┬──────────────┬───────────────────────────┘
       │              │              │
       ▼              ▼              ▼
  ┌─────────┐  ┌──────────┐  ┌──────────────┐
  │FileSink │  │StdoutSink│  │EventBusSink  │
  │lumberjack│  │JSON      │  │熔断器+超时    │
  └─────────┘  └──────────┘  └──────────────┘
```

### 1.2 事件子系统（平行轨道）

```
┌─────────────────────────────────────────────────────────────────┐
│                  事件层 (internal/event)                         │
│  TraceEmitter → FlowTracker → SpanCollector + UsageAggregator   │
│  → Infra(双总线) → SessionBus + MonitorBus                      │
│  → Buffer(环形回放) + FlowLogEntry 数据模型                      │
└─────────────────────────────────────────────────────────────────┘
```

### 1.3 桥接层

| 桥接器 | 源 | 目标 | 职责 |
|--------|----|------|------|
| RuntimeLogAdapter | trpc-agent-go `agentlog.Logger` | loggateway Pipeline | 运行时日志统一落盘 |
| PluginSafeLogger | 插件/Hook | loggateway + EventBus | 插件日志双写 |
| EventBusSink | logpipeline Pipeline | EventBus | 通用日志→事件总线 |

---

## 2. 核心组件设计

### 2.1 loggateway.Gateway

```go
type Gateway struct {
    base        []Field                         // 预设字段
    outputDir   string                          // 日志输出目录
    pipeline    atomic.Pointer[logpipeline.Pipeline]  // 异步管道（原子指针）
    atomicLevel zap.AtomicLevel                 // 动态级别控制
}
```

**设计决策**：

| 决策 | 原因 |
|------|------|
| Pipeline 用 `atomic.Pointer` | 支持运行时热替换，避免读写锁开销 |
| AtomicLevel 基于 zap | 复用 zap 的成熟动态级别实现，无需自研 |
| nil Gateway 安全 | 简化调用方代码，无需 nil 检查 |
| LoggingConfig 与 conf 解耦 | `pkg/` 不依赖 `internal/conf`，通过中间结构转换 |

**emitToPipeline 流程**：

```
Debug/Info/Warn/Error(msg, fields...)
  → 合并 base + fields
  → zapcore.NewMapObjectEncoder() 序列化
  → 提取路由字段 (session_id/step_id/trace_id/run_id)
  → 构造 LogEntry
  → Pipeline.Emit(entry)
```

### 2.2 With() 不可变语义

```go
type loggerWith struct {
    g    *Gateway
    base []Field
}
```

- 每次 `With()` 创建新的 `loggerWith`，base 切片复制
- 原始 Logger 不受后续 `With()` 影响
- 字段累积：`child.base = parent.base + newFields`
- 避免并发修改共享状态

### 2.3 错误链展开

```go
func Err(err error) Field {
    chain := unwrapChain(err)
    if len(chain) <= 1 {
        return zap.Error(err)
    }
    msgs := make([]string, len(chain))
    for i, e := range chain {
        msgs[i] = e.Error()
    }
    return zap.Strings("error_chain", msgs)
}
```

- 单层错误：标准 `zap.Error` 输出 `error` 字段
- 多层错误：输出 `error_chain` 数组，保留完整上下文

---

## 3. Pipeline 设计

### 3.1 异步分发模型

```
Emit() ──→ [throttler check] ──→ ch (4096) ──→ dispatchLoop ──→ SinkGroup.Emit() ──→ Sink.Write()
                                         │
                                    满则丢弃 + 计数
```

**关键约束**：

| 约束 | 实现 |
|------|------|
| Emit 非阻塞 | `select` + `default`，通道满则丢弃 |
| 关闭安全 | `closed.Store(true)` → `cancel()` → `close(ch)` → `wg.Wait()` |
| Panic 隔离 | dispatchLoop 和 SinkGroup.run() 均有 `defer recover` |

### 3.2 SinkGroup 隔离

```
Pipeline.Emit()
  ├── SinkGroup[0].Emit() → ch[0] → goroutine[0] → FileSink.Write()
  ├── SinkGroup[1].Emit() → ch[1] → goroutine[1] → StdoutSink.Write()
  └── SinkGroup[2].Emit() → ch[2] → goroutine[2] → EventBusSink.Write()
```

**设计要点**：

- 每个 SinkGroup 拥有独立 channel + goroutine
- 慢 Sink 阻塞不影响其他 SinkGroup
- DropPolicy：`DropNewest`（默认，丢弃新条目）/ `DropBlock`（阻塞等待）
- Panic 恢复：`sink.Write()` panic 被 recover，SinkGroup 继续处理后续条目

### 3.3 stepThrottler 令牌桶

```
Emit(entry)
  → stepThrottler.shouldThrottle(entry.StepID)
    → 前缀匹配 ThrottleRule（最长前缀）
    → tokenBucket.tryConsume()
      → 令牌不足 → throttled++ → 丢弃
      → 令牌充足 → 通过
```

**TTL 淘汰**：

- 每个 bucket 记录 `lastAccess` 时间戳
- 后台 goroutine 每 60 秒扫描，淘汰 5 分钟未访问的桶
- 读写锁分离：淘汰不阻塞 `shouldThrottle` 热路径

### 3.4 EventBusSink 熔断器

**三态状态机**：

```
         5次连续失败                10秒超时
cbClosed ──────────→ cbOpen ──────────→ cbHalfOpen
   ↑                                       │
   └──── 3次连续成功探测 ←──────────────────┘
                     │ 1次失败
                     ↓
                  cbOpen
```

**参数**：

| 参数 | 值 | 说明 |
|------|----|------|
| cbFailureThreshold | 5 | 连续失败次数阈值 |
| cbOpenDuration | 10s | 熔断打开持续时间 |
| cbHalfOpenMaxProbe | 3 | 半开→关闭需连续成功次数 |
| publishTimeout | 50ms | Publish 调用超时 |

**无锁实现**：所有状态通过 `atomic.Int32` / `atomic.Int64` 操作，无互斥锁。

---

## 4. 事件子系统设计

### 4.1 TraceEmitter 拆分架构

```
TraceEmitter (embedding wrapper)
  └── FlowTracker (流程追踪核心)
        ├── FlowContext (关联上下文)
        ├── SpanCollector (Span 树管理)
        │     └── SpanContext (span 数据结构)
        ├── UsageAggregator (用量聚合)
        │     └── UsageContext (OTel 引用)
        └── loggateway.Logger (结构化日志输出)
```

**拆分原则**：

| 组件 | 单一职责 | 可独立测试 |
|------|---------|-----------|
| FlowTracker | 流程追踪 API 签名 | ✅ |
| SpanCollector | Span 树生命周期 | ✅ |
| UsageAggregator | 框架事件→usage 元数据 | ✅ |

### 4.2 FlowTracker.emit 双写

```
LogStart/LogDone/LogError/...
  → 构造 FlowLogEntry
  → [1] lg.Info() → loggateway Pipeline → File/Stdout/EventBus
  → [2] Envelope(EnvelopeTypeFlowLog) → Infra.Publish() → SessionBus/MonitorBus
  → [3] Buffer.Store() → 环形缓冲（WS 重连回放）
```

**LogError 特殊行为**：

- 额外发布 `EnvelopeTypeError` 到 SessionBus（聊天错误 toast）
- `flowStepsSkipChatError` 中的 stepID 不发布为聊天错误（避免噪音）

### 4.3 Infra 双总线路由

| 事件类型 | split 模式（默认） | dual 模式 |
|---------|-------------------|-----------|
| flow_log / log | 仅 MonitorBus | 双发 |
| alert / mcp_health | 双发 | 双发 |
| 其他 | 仅 SessionBus | 仅 SessionBus |

**路由模式**：由 `MONITOR_BUS_ROUTING` 环境变量控制，构造时读取一次。

### 4.4 EventBus 分发策略

| 策略 | 行为 | 适用场景 |
|------|------|---------|
| DropNewest | 通道满时丢弃新事件 | 普通订阅者 |
| DropOldest | 丢弃最旧事件腾出空间 | 低优先级事件 |
| BlockUpTo | 阻塞最多指定时间，超时降级 DropOldest | 关键事件（ToolResult/Error/RunnerCompletion 等） |

**关键事件保护**：`criticalTypeSet` 中的事件类型强制使用 `BlockUpTo` 策略。

### 4.5 Buffer 环形回放

```
WS 重连 → Replay(sessionID, lastEventID)
  → ringBuffer 遍历 → 跳过 ≤ lastEventID 的事件 → 返回后续事件
```

- 按 sessionID 分区，每分区 200 条
- 5 分钟未访问的分区自动清理

---

## 5. 桥接器设计

### 5.1 RuntimeLogAdapter

```
trpc-agent-go agentlog.Logger
  → RuntimeLogAdapter
    → Debug/Info/Warn/Error → lg.Debug/Info/Warn/Error (走 Pipeline)
    → Fatal/Fatalf → 直写 stderr + os.Exit(1) (绕过 Pipeline)
    → With(fields) → 新实例 (不可变模式)
```

**Fatal 绕过 Pipeline 的原因**：Pipeline 是异步的，进程即将退出时异步写入无法保证落盘。

### 5.2 PluginSafeLogger

```
插件/Hook 调用
  → PluginSafeLogger.Write(level, msg, attrs)
    → [1] lg.Debug/Info/Warn/Error (走 Pipeline)
    → [2] safego.Go → EventBus.Publish(EnvelopeTypeLog) (异步双写)
```

---

## 6. 反馈环切断设计

### 6.1 问题

日志条目被丢弃 → 尝试记录丢弃事件 → 可能触发更多丢弃 → 无限循环

### 6.2 两层切断

| 层 | 机制 | 说明 |
|----|------|------|
| Bus.logDrop() | `fmt.Fprintf(os.Stderr, ...)` + `droppedCount atomic.Uint64` | 丢弃通知不经过 Pipeline/EventBus |
| EventBusSink 熔断 | 熔断状态转换写 stderr | 熔断事件不经过 Pipeline/EventBus |

**原则**：任何因日志系统自身问题产生的通知，只走 stderr + 原子计数器，绝不回入日志管道。

---

## 7. 配置驱动 Sink 注册

### 7.1 工厂模式

```
conf.LoggingSink (proto)
  → protoSinkToConfig() 转换
    → logpipeline.SinkConfig
      → NewSinkFromConfig(cfg, deps)
        → FileSink / StdoutSink / EventBusSink
```

### 7.2 EventBus 依赖注入

EventBusSink 需要 `Publisher` 接口，此依赖无法从配置推导，通过 `SinkFactoryDeps` 注入：

```go
type SinkFactoryDeps struct {
    EventBusPublisher Publisher
}
```

EventBus Sink 延迟到 `BeforeStart` 钩子中注册（此时 eventInfra 已就绪）。

---

## 8. 线程安全模型

| 组件 | 并发安全机制 | 说明 |
|------|-------------|------|
| Gateway | `atomic.Pointer` + `sync.RWMutex` (全局单例) | Pipeline 热替换、Global/SetGlobal |
| Pipeline | `atomic.Bool` (closed) + `atomic.Uint64` (dropped/throttled) | Emit 非阻塞、关闭安全 |
| SinkGroup | `atomic.Uint64` (dropped) + channel | 独立 goroutine 隔离 |
| stepThrottler | `sync.RWMutex` (buckets map) | 读多写少，淘汰不阻塞热路径 |
| EventBusSink 熔断器 | 全部 `atomic` 操作 | 无锁三态状态机 |
| EventBus | `sync.RWMutex` (subscribers map) | 订阅/取消/发布并发安全 |
| Buffer | `sync.RWMutex` (buffers map) | 读写分离，清理不阻塞回放 |
| loggerWith | 不可变设计 | 每次 With() 返回新实例 |

---

## 9. 性能设计

### 9.1 关键路径优化

| 优化 | 实现 |
|------|------|
| Emit 非阻塞 | channel + select/default，满则丢弃 |
| Sink 隔离 | SinkGroup 独立 goroutine，慢 Sink 不阻塞 Pipeline |
| 令牌桶节流 | 高频 step 限流，防止 Pipeline 淹没 |
| 熔断器 | 下游故障时快速失败，不阻塞 Pipeline |
| 读写分离 | stepThrottler RWMutex，淘汰不阻塞热路径 |
| 原子操作 | 熔断器全 atomic，无锁竞争 |

### 9.2 背压策略

| 场景 | 策略 |
|------|------|
| Pipeline channel 满 | 丢弃新条目 + `dropped` 计数 |
| SinkGroup channel 满 | DropNewest 丢弃 / DropBlock 阻塞 |
| EventBus 发布超时 | 50ms 超时 + 熔断器 |
| stepThrottler 限流 | 令牌桶不足时丢弃 + `throttled` 计数 |

### 9.3 资源控制

| 资源 | 上限 | 说明 |
|------|------|------|
| Pipeline channel | 4096 | 可配置 |
| SinkGroup channel | 4096 (默认) | 可配置 |
| stepThrottler buckets | 无硬上限 | TTL 淘汰 5 分钟未访问 |
| Buffer 分区 | 200 条/分区 | 环形覆盖 |
| Buffer TTL | 30 分钟 | 自动清理 |

---

## 10. 可观测性设计

### 10.1 Pipeline 指标

```go
type PipelineStats struct {
    Dropped    uint64   // Pipeline channel 丢弃数
    Throttled  uint64   // stepThrottler 限流数
    ChanLen    int      // 当前 channel 长度
    ChanCap    int      // channel 容量
    SinkCount  int      // Sink 数量
    SinkErrors uint64   // Sink 写入错误数
}
```

### 10.2 SinkGroup 指标

```go
type SinkGroupStats struct {
    Name    string  // Sink 名称
    Dropped uint64  // SinkGroup channel 丢弃数
    ChanLen int     // 当前 channel 长度
    ChanCap int     // channel 容量
}
```

### 10.3 EventBusSink 熔断器指标

| 指标 | 说明 |
|------|------|
| `circuit_breaker_open` | 熔断器打开次数 |
| `circuit_breaker_skipped` | 熔断器打开期间跳过的写入次数 |
| `circuit_breaker_half_open_attempts` | 半开状态探测次数 |

### 10.4 Prometheus 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_event_bus_published_total` | Counter | EventBus 发布总数 |
| `aranea_event_bus_dropped_total` | Counter | EventBus 丢弃总数 |

---

## 11. 关键文件索引

| 文件 | 设计职责 |
|------|---------|
| `pkg/loggateway/logger.go` | Logger 接口 + Field 类型 + 错误链展开 |
| `pkg/loggateway/gateway.go` | Gateway 核心 + With() + emitToPipeline + AtomicLevel |
| `pkg/logpipeline/pipeline.go` | Pipeline 接口/实现 + stepThrottler + tokenBucket |
| `pkg/logpipeline/sink_group.go` | SinkGroup 隔离 + DropPolicy |
| `pkg/logpipeline/eventbus_sink.go` | EventBusSink + 三态熔断器 |
| `pkg/logpipeline/file_sink.go` | FileSink (lumberjack JSON) |
| `pkg/logpipeline/stdout_sink.go` | StdoutSink (JSON) |
| `pkg/logpipeline/sink_factory.go` | Sink 工厂 |
| `internal/event/flow_tracker.go` | FlowTracker 流程追踪核心 |
| `internal/event/span_collector.go` | SpanCollector Span 树管理 |
| `internal/event/usage_aggregator.go` | UsageAggregator 用量聚合 |
| `internal/event/trace_emitter.go` | TraceEmitter embedding wrapper |
| `internal/event/flow_log.go` | FlowLogEntry + stepTitleRegistry |
| `internal/event/infra.go` | Infra 双总线路由 |
| `internal/event/bus.go` | EventBus 实现 + logDrop |
| `internal/event/buffer.go` | Buffer 环形回放 |
| `internal/adapter/runtime_log.go` | RuntimeLogAdapter |
| `internal/plugin/trpc/safe_logger.go` | PluginSafeLogger |
| `cmd/admin/logging.go` | 初始化流程 |
