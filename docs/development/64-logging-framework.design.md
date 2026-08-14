# Logging Framework — 设计文档

> **对应需求**：[64-logging-framework.md](./64-logging-framework.md)
>
> **对应开发计划**：[64-logging-framework.development.md](./64-logging-framework.development.md)

---

## 1. 架构总览

### 1.1 双轨制架构

项目采用双轨制日志体系，两者语义不同、入口不同、底层管道统一：

```
                    ┌──────────────────────────────────────────────┐
                    │            日志调用入口（双轨制）              │
                    └──────────────┬───────────────────────────────┘
                                   │
               ┌───────────────────┼───────────────────┐
               ▼                                       ▼
    ┌─────────────────────┐               ┌─────────────────────┐
    │   loggateway.Logger │               │  TraceEmitter       │
    │   通用结构化日志      │               │  (→FlowTracker)     │
    │   "发生了什么"       │               │  "进行到哪了"        │
    └─────────┬───────────┘               └─────────┬───────────┘
              │                                     │
              ▼                                     ▼
    ┌─────────────────────┐               ┌─────────────────────┐
    │   logpipeline       │               │   EventBus          │
    │   异步分发管道        │               │   双总线发布          │
    └─────────┬───────────┘               └─────────┬───────────┘
              │                                     │
    ┌─────────┼─────────┐                 ┌─────────┼─────────┐
    ▼         ▼         ▼                 ▼         ▼         ▼
┌───────┐ ┌───────┐ ┌────────┐      ┌──────┐ ┌──────┐ ┌──────┐
│SinkGrp│ │SinkGrp│ │SinkGrp │      │ WS   │ │JSONL │ │  DB  │
│ File  │ │Stdout │ │EventBus│      │Push  │ │File  │ │Persist│
└───────┘ └───────┘ └────────┘      └──────┘ └──────┘ └──────┘
```

| 系统 | 包路径 | 定位 | 输出目标 |
|------|--------|------|----------|
| **loggateway** | `pkg/loggateway` | 通用结构化日志（红线 #16 唯一日志 API） | Pipeline → File/Stdout/EventBus |
| **Flow Log** | `internal/event` | 业务流程步骤追踪 | EventBus → WS/JSONL/DB |

### 1.2 三层架构

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

### 1.3 事件子系统（平行轨道）

```
┌─────────────────────────────────────────────────────────────────┐
│                  事件层 (internal/event)                         │
│  TraceEmitter → FlowTracker → SpanCollector + UsageAggregator   │
│  → Infra(双总线) → SessionBus + MonitorBus                      │
│  → Buffer(环形回放) + FlowLogEntry 数据模型                      │
└─────────────────────────────────────────────────────────────────┘
```

### 1.4 桥接层

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
    if len(chain) == 1 {
        return zap.Error(err)
    }
    return zap.Array("error_chain", errorChainArray(chain))
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

**Pipeline 集成**：Pipeline 内部维护 `[]*SinkGroup`，`AddSink()` 自动包装为默认 SinkGroup（bufSize=4096, DropNewest），`AddSinkGroup()` 允许自定义参数。

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
| FlowTracker | 流程追踪 API 签名 | 是 |
| SpanCollector | Span 树生命周期 | 是 |
| UsageAggregator | 框架事件→usage 元数据 | 是 |

**依赖关系**：`TraceEmitter` → `FlowTracker` → `SpanCollector` + `UsageAggregator`

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

**关键事件保护**：通过 `contract.RequiresBlockUpTo(envType)` 判定，基于事件可靠性分级（AS-EVT-01）。Critical 与 Important 级别事件强制使用 `BlockUpTo` 策略，详见 `internal/event/contract/reliability.go`。

### 4.5 Buffer 环形回放

```
WS 重连 → Replay(sessionID, lastEventID)
  → ringBuffer 遍历 → 跳过 ≤ lastEventID 的事件 → 返回后续事件
```

- 按 sessionID 分区，每分区 200 条
- 30 分钟未访问的分区自动清理（每 5 分钟扫描一次）

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

**接口适配**：实现 `agentlog.Logger`（Debug/Info/Warn/Error/Fatal 及格式化版本），编译期检查 `var _ agentlog.Logger = (*RuntimeLogAdapter)(nil)`。

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

### 6.2 三层切断

| 层 | 机制 | 说明 |
|----|------|------|
| EventBus 丢弃通知 | `loggateway.Logger.Warn` + Prometheus 指标 `arametrics.EventBusDropped` | 丢弃通知经 loggateway Pipeline 走 FileSink/StdoutSink，但 EventBusSink 自身不会回环（EventBusSink 只接收 LogEntry 不接收 Envelope） |
| EventBusSink 熔断 | 熔断状态转换写 stderr（`fmt.Fprintf(os.Stderr, ...)`） | 熔断事件不经过 Pipeline/EventBus |
| FlowFileAppender 自反馈切断 | 丢弃 `step_id` 前缀 `monitor.flow_file.` 的回流事件 + 连续 3 次写失败熔断 1 分钟 | Appender 写盘失败 → Warn → EventBusSink → MonitorBus → Appender 再次写盘失败 的无限循环（2026-08-13 磁盘满引发日志风暴后根治，详见 18-monitor.design.md §3.4） |

**EventBus 丢弃通知实现**（`internal/event/bus_adapter.go`）：

```go
dropLogger := frameworkbus.DropLogger[Envelope](func(env Envelope, policy string, totalDrops uint64) {
    arametrics.EventBusDropped.WithLabelValues(string(env.Type), policy).Inc()
    if lg != nil {
        lg.Warn("[event_bus] drop",
            loggateway.Str("policy", policy),
            loggateway.Str("type", string(env.Type)),
            loggateway.Str("channel", env.Channel),
            loggateway.SessionID(env.SessionID),
            loggateway.Int64("total_drops", int64(totalDrops)),
        )
    }
})
```

**原则**：任何因日志系统自身问题产生的通知，只走 stderr + 原子计数器/Prometheus 指标，绝不回入 EventBus（避免递归）。loggateway Pipeline 的丢弃通知虽然走 Pipeline，但 EventBusSink 在熔断/丢弃时不会再次发布到 EventBus，因此不构成反馈环。

---

## 7. 配置驱动 Sink 注册

### 7.1 Proto 契约

```protobuf
// internal/conf/conf.proto

enum SinkType {
  SINK_TYPE_UNSPECIFIED = 0;
  SINK_TYPE_FILE = 1;
  SINK_TYPE_STDOUT = 2;
  SINK_TYPE_EVENTBUS = 3;
}

enum DropPolicy {
  DROP_POLICY_UNSPECIFIED = 0;
  DROP_POLICY_NEWEST = 1;
  DROP_POLICY_BLOCK = 2;
}

message LoggingSink {
  string name = 1;
  SinkType type = 2;
  int32 buffer_size = 3;
  DropPolicy drop_policy = 4;
  map<string, string> config = 5;
}

message Logging {
  string level = 1;
  string output_dir = 2;
  int32 max_size_mb = 3;
  int32 max_backups = 4;
  int32 max_age_days = 5;
  bool compress = 6;
  bool stdout_enabled = 7;
  string hook_level = 8;
  repeated LoggingSink sinks = 9;
}
```

### 7.2 工厂模式

```
conf.LoggingSink (proto)
  → protoSinkToConfig() 转换
    → logpipeline.SinkConfig
      → NewSinkFromConfig(cfg, deps)
        → FileSink / StdoutSink / EventBusSink
```

```go
// pkg/logpipeline/sink_factory.go
type SinkConfig struct {
    Name       string
    Type       string            // "file", "stdout", "eventbus"
    BufferSize int
    DropPolicy DropPolicy
    Config     map[string]string // Sink 特定配置
}

type SinkFactoryDeps struct {
    EventBusPublisher Publisher  // EventBus Sink 需要的外部依赖
}

func NewSinkFromConfig(cfg SinkConfig, deps SinkFactoryDeps) (Sink, error)
```

**设计要点**：
- `SinkConfig` 与 `internal/conf` proto 解耦，`cmd/admin/logging.go` 负责转换
- EventBus Sink 的 Publisher 无法从配置推导，通过 `SinkFactoryDeps` 注入
- 每个 Sink 的 `config` map 支持类型特定参数（如 file 的 output_dir/filename，eventbus 的 hook_level）

### 7.3 EventBus 依赖注入

EventBusSink 需要 `Publisher` 接口，此依赖无法从配置推导，通过 `SinkFactoryDeps` 注入。EventBus Sink 延迟到 `BeforeStart` 钩子中注册（此时 eventInfra 已就绪）。

---

## 8. 初始化流程

```
1. logpipeline.NewPipeline(4096)       → 异步管道（单 worker, buffer=4096）
2. pipeline.AddSink(fileSink)          → FileSink
3. pipeline.AddSink(stdoutSink)        → StdoutSink（可配）
4. loggateway.New(bc.Logging, pipeline)→ Gateway 构造时注入 Pipeline
5. adapter.NewRuntimeLogAdapter(gw)    → 桥接 trpc-agent-go 运行时日志
6. agentlog.Default = rla              → 替换框架默认 logger
7. wireApp(..., lg, pipeline)          → Wire DI
8. BeforeStart: pipeline.AddSink(eventBusSink)
9. AfterStop:  pipeline.Close()
```

> 实现代码锚点见 [开发计划 §6](./64-logging-framework.development.md#6-代码锚点)。

---

## 9. 线程安全模型

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

## 10. 性能设计

### 10.1 关键路径优化

| 优化 | 实现 |
|------|------|
| Emit 非阻塞 | channel + select/default，满则丢弃 |
| Sink 隔离 | SinkGroup 独立 goroutine，慢 Sink 不阻塞 Pipeline |
| 令牌桶节流 | 高频 step 限流，防止 Pipeline 淹没 |
| 熔断器 | 下游故障时快速失败，不阻塞 Pipeline |
| 读写分离 | stepThrottler RWMutex，淘汰不阻塞热路径 |
| 原子操作 | 熔断器全 atomic，无锁竞争 |

### 10.2 背压策略

| 场景 | 策略 |
|------|------|
| Pipeline channel 满 | 丢弃新条目 + `dropped` 计数 |
| SinkGroup channel 满 | DropNewest 丢弃 / DropBlock 阻塞 |
| EventBus 发布超时 | 50ms 超时 + 熔断器 |
| stepThrottler 限流 | 令牌桶不足时丢弃 + `throttled` 计数 |

### 10.3 资源控制

| 资源 | 上限 | 说明 |
|------|------|------|
| Pipeline channel | 4096 | 可配置 |
| SinkGroup channel | 4096 (默认) | 可配置 |
| stepThrottler buckets | 无硬上限 | TTL 淘汰 5 分钟未访问 |
| Buffer 分区 | 200 条/分区 | 环形覆盖 |
| Buffer TTL | 30 分钟 | 每 5 分钟扫描清理 |

---

## 11. 可观测性设计

### 11.1 Pipeline 指标

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

### 11.2 SinkGroup 指标

```go
type SinkGroupStats struct {
    Name    string  // Sink 名称
    Dropped uint64  // SinkGroup channel 丢弃数
    ChanLen int     // 当前 channel 长度
    ChanCap int     // channel 容量
}
```

### 11.3 EventBusSink 熔断器指标

| 指标 | 说明 |
|------|------|
| `circuit_breaker_open` | 熔断器打开次数 |
| `circuit_breaker_skipped` | 熔断器打开期间跳过的写入次数 |
| `circuit_breaker_half_open_attempts` | 半开状态探测次数 |

### 11.4 Prometheus 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_event_bus_published_total` | Counter | EventBus 发布总数 |
| `aranea_event_bus_dropped_total` | Counter | EventBus 丢弃总数 |

---

## 12. 接口定义

### 12.1 loggateway.Logger

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    With(fields ...Field) Logger
}
```

字段构造函数：`StepID`, `SessionID`, `TraceID`, `RunID`, `Domain`, `AgentKey`, `Phase`, `Duration`, `Source`, `Err`, `Str`, `Int`, `Int64`, `Float64`, `Bool`, `Any`

特殊：`Err(err)` 支持错误链展开（`unwrapChain`），多层错误输出为 `error_chain` 数组。

### 12.2 logpipeline.Pipeline

```go
type Pipeline interface {
    Emit(entry LogEntry)
    AddSink(sink Sink)
    Close() error
    Dropped() uint64
    Throttled() uint64
    Stats() PipelineStats
    SetThrottleRules(rules []ThrottleRule)
}
```

### 12.3 logpipeline.Sink

```go
type Sink interface {
    Write(entry LogEntry)
    Flush()
    Close() error
}
```

Sink 实现：`FileSink`（lumberjack JSON 落盘）、`StdoutSink`（stdout JSON）、`EventBusSink`（EventBus 发布）

### 12.4 TraceEmitter / FlowTracker

```go
// TraceEmitter 是 v2 统一写入器：FlowLog (WS) + span buffer (usage metadata)
// 它嵌入 FlowTracker，并添加 ObserveFrameworkEvent 用于 trpc-agent-go 事件流
type TraceEmitter struct {
    *FlowTracker
}

func (e *TraceEmitter) ObserveFrameworkEvent(ev *trpcevent.Event)  // 桥接 trpc-agent-go 事件流
func (ft *FlowTracker) LogStart(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogDone(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogSkip(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogWarn(stepID, title, message string, extra ...Pair)
func (ft *FlowTracker) LogError(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogCritical(stepID, message string, extra ...Pair)
```

### 12.5 FlowLogEntry 数据模型

Schema 版本 `flow_log/v1`，包含：
- `correlation`（trace_id, session_id, run_id, team_id, domain, agent_key, agent_id）
- `step`（id, phase, subsystem）
- `severity`（ok / info / warn / error / critical）
- `title` / `message` / `hint`
- `timing`（duration_ms, started_at）
- `error`（code, message）
- `extra`（map[string]any）

---

## 13. EventBus 与日志的关系

- 双总线：SessionBus（业务）+ MonitorBus（监控），物理隔离
- 三级队列：high(64) / normal(128) / low(256)，日志走 low
- DropNewest 策略：low/normal 队列满时丢弃新消息
- `flow_log` 不受 `logEnabled` 开关限制；`log` 仍受限制

---

## 14. 模块关联

### 14.1 上游依赖 loggateway 的模块

agent, provider, team, cron, a2a, session/status, plugin

### 14.2 上游依赖 event（日志）的模块

agent, provider, team, cron, a2a, plugin

### 14.3 前端对应

| 后端日志系统 | 前端组件 |
|-------------|---------|
| FlowLog (EnvelopeTypeFlowLog) | MonitorPage FlowLogStream.vue, FlowTracePanel.vue, FlowLogExportButton.vue |
| 进程日志 (EnvelopeTypeLog) | MonitorPage ProcessLogStream.vue |
| WS 推送 | useLogStreamHub.ts |
| Flow Log 落库 | ListFlowLogs HTTP API → 前端历史查询 |

### 14.4 数据库与文件

| 表/文件 | 用途 |
|---------|------|
| `flow_log_events` | FlowLog 持久化 |
| `aranea-pipeline.log` | Zap 统一日志文件（lumberjack 轮转，FileSink 默认文件名） |
| `flow-*.jsonl` / `system-*.jsonl` / `log-*.jsonl` / `trace-*.jsonl` / `alert-*.jsonl` | FlowFileAppender 分类落盘（`internal/biz/monitor/flow_file_appender.go`） |

---

## 15. 关键文件索引

| 文件 | 设计职责 |
|------|---------|
| `pkg/loggateway/logger.go` | Logger 接口 + Field 类型 + 错误链展开 |
| `pkg/loggateway/gateway.go` | Gateway 核心 + With() + emitToPipeline + AtomicLevel |
| `pkg/logpipeline/pipeline.go` | Pipeline 接口/实现 + stepThrottler + tokenBucket |
| `pkg/logpipeline/sink_group.go` | SinkGroup 隔离 + DropPolicy |
| `pkg/logpipeline/eventbus_sink.go` | EventBusSink + 三态熔断器 |
| `pkg/logpipeline/file_sink.go` | FileSink (lumberjack JSON) |
| `pkg/logpipeline/stdout_sink.go` | StdoutSink (JSON) |
| `pkg/logpipeline/sink_factory.go` | Sink 工厂（配置驱动注册） |
| `internal/event/flow_tracker.go` | FlowTracker 流程追踪核心 |
| `internal/event/span_collector.go` | SpanCollector Span 树管理 |
| `internal/event/usage_aggregator.go` | UsageAggregator 用量聚合 |
| `internal/event/trace_emitter.go` | TraceEmitter embedding wrapper |
| `internal/event/flow_log.go` | FlowLogEntry + stepTitleRegistry |
| `internal/event/flow_context.go` | `WithTraceEmitter` / `TraceEmitterFromContext` / `NewTraceEmitterForRun`（FlowLogger 别名与 CtxFlowLog* 已删除） |
| `internal/event/infra.go` | Infra 双总线路由 |
| `internal/event/bus.go` | EventBus 类型别名 + NewBus |
| `internal/event/bus_adapter.go` | busAdapter（framework bus 桥接 + DropLogger） |
| `internal/event/buffer.go` | Buffer 环形回放 |
| `internal/event/logpipeline_publisher.go` | busPublisher（EventBusSink → contract.Bus 桥接） |
| `internal/adapter/runtime_log.go` | RuntimeLogAdapter |
| `internal/plugin/trpc/safe_logger.go` | PluginSafeLogger |
| `internal/runtime/deps.go` | TurnDeps.Lg 字段（loggateway.Logger 注入到 chat turn） |
| `pkg/trpc-agent-go/log/log.go` | Agent 运行时日志（独立 zap.Sugar，Default/ContextDefault） |
| `pkg/trpc-agent-go/plugin/logging.go` | Agent 生命周期日志插件 |
| `internal/conf/conf.proto` | Logging 配置 Proto 定义 |
| `cmd/admin/logging.go` | 初始化流程（initLogging + protoSinkToConfig） |
