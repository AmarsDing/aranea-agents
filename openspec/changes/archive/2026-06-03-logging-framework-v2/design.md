## Context

当前日志框架由三个核心包组成：

- `pkg/loggateway` — 通用结构化日志门面（Logger 接口 + Gateway 实现）
- `pkg/logpipeline` — 异步分发管道（Pipeline + Sink 接口）
- `internal/event` — 业务流程追踪（TraceEmitter + FlowLogEntry + EventBus）

经过 P0/P1/P2 迁移和 LogPipeline Phase 1-5 实施，11 个 Bug 已全部修复。但随着项目规模增长（~100 文件、~1146 次引用），8 个结构性隐患开始显现：

1. **双写路径**：Gateway 每次日志调用同时走 zap 直写 + Pipeline 分发，同一条日志在磁盘上存在两份
2. **单 worker 瓶颈**：Pipeline 单 goroutine 串行分发所有 Sink，慢 Sink 阻塞全局
3. **TraceEmitter 上帝对象**：5 个职责（流程追踪 + span 管理 + usage 元数据 + OTel 桥接 + 错误发布）耦合在一个 struct
4. **三种获取方式并存**：Global() / CtxFlowLog*(ctx) / 构造注入，依赖不透明
5. **Throttler 内存泄漏**：buckets map 只增不减
6. **EventBus-日志反馈环**：Bus.logDrop() → Gateway.Warn() → Pipeline → EventBusSink → Bus.Publish() 形成正反馈循环
7. **trpc-agent-go 运行时日志完全隔离**：489 处日志调用走独立 zap.Sugar → stdout，对运维不可见
8. **boundInfraRef() 全局可变状态**：TraceEmitter 绕过构造注入，使用全局引用发布事件

## Goals / Non-Goals

**Goals:**

- 消除双写路径，实现 Pipeline 单写模型
- SinkGroup 独立队列，慢 Sink 不影响其他 Sink
- TraceEmitter 拆分为三个职责单一的组件（各自独立 context + mutex）
- 统一依赖获取方式，构造注入为主
- Throttler 增加 TTL 淘汰机制（含 goroutine 生命周期管理）
- 配置驱动 Sink 注册（Proto enum 约束）
- 切断 EventBus-日志反馈环（stderr + droppedCount + 熔断）
- 桥接 trpc-agent-go 运行时日志到 Pipeline（适配器在 internal/ 层）
- 消除 boundInfraRef() 全局可变状态

**Non-Goals:**

- 不改变日志输出格式（JSON 格式保持兼容）
- 不改变前端 WebSocket 推送协议
- 不引入新的日志库依赖（继续使用 zap + lumberjack）
- 不做 OTel 集成（当前 OTelSink 已有，不扩展）
- 不改变 FlowLogEntry 数据模型（schema flow_log/v1 不变）
- 不做日志采样策略变更（已有 ThrottleRule 机制不变）
- 不改变 EnvelopeType 封闭枚举模式（当前 42 个类型尚可管理）
- 不统一 Kratos log.Logger（框架中间件日志保持 stdout，独立演进）
- 不完全消除 Infra 内部的全局引用（Infra 自身的 boundInfraRef 清理为后续独立变更）
- 不迁移测试代码中的 Global() 调用（61 处，测试中 Global() 使用合理）

## Decisions

### D1: 单写路径 — Gateway 去掉 zap.Logger 直写

**选择**：Gateway 不再直接持有 `*zap.Logger`，所有日志统一通过 `Pipeline.Emit` 分发。FileSink 内部使用 `zapcore.Core` 写入。

**替代方案**：
- A) 保持双写，FileSink 写独立文件 → 日志翻倍，排查时需合并两个文件
- B) 去掉 Pipeline，全部走 zap → 失去异步解耦和 Sink 可插拔能力

**理由**：Pipeline 是唯一的分发中枢，所有日志走一条路径，消除重复和语义混淆。FileSink 内部仍用 zapcore 保证 JSON 格式兼容。

**启动时序保证**：Gateway 构造时必须传入 Pipeline（而非后置 SetPipeline），避免 Pipeline 为 nil 时日志静默丢弃。当前 main.go 中 `gw.SetPipeline(pipeline)` 改为 `loggateway.New(bc.Logging, pipeline)` 构造时注入。

```
当前：Logger.Info() → zap 写文件 + Pipeline 写文件  ← 双写
目标：Logger.Info() → Pipeline → FileSink(zapcore)  ← 单写
```

### D2: SinkGroup 独立队列模型

**选择**：每个 Sink 注册时创建独立的 SinkGroup（goroutine + channel），Pipeline 主循环只负责路由到对应 SinkGroup。

```go
type DropPolicy int

const (
    DropNewest DropPolicy = iota
    DropBlock
)

type SinkGroup struct {
    sink       Sink
    ch         chan LogEntry
    wg         sync.WaitGroup
    dropped    atomic.Uint64
    dropPolicy DropPolicy
}

// Emit 非阻塞返回：根据 dropPolicy 决定 DropNewest 或 Block
func (sg *SinkGroup) Emit(entry LogEntry) error
```

**替代方案**：
- A) 多 worker 分发 → 需要保证 Sink 写入顺序，复杂度高
- B) 每个 Sink 独立 Pipeline → 过度设计，资源浪费

**理由**：独立队列是最小改动方案，慢 Sink 只影响自己的 channel，不阻塞其他 Sink。不同 Sink 可配不同 buffer 大小和丢弃策略。DropPolicy 使用枚举而非 string，与 Proto enum 对齐。

### D3: TraceEmitter 拆分为三个组件（各自独立 context + mutex）

**选择**：

| 组件 | 职责 | 包路径 | 独立 context |
|------|------|--------|-------------|
| FlowTracker | LogStart/LogDone/LogError/LogSkip/LogWarn/LogCritical | `internal/event/flow_tracker.go` | FlowContext（含独立 mutex） |
| SpanCollector | startSpan/endSpan/FinishRoot | `internal/event/span_collector.go` | SpanContext（含独立 mutex） |
| UsageAggregator | ObserveFrameworkEvent/mergeLLMSpan/MetadataJSON | `internal/event/usage_aggregator.go` | UsageContext（含独立 mutex） |

**关键变更**：当前 TraceContext 是一个整体 struct（含 mutex），拆分后三个组件各自持有独立的 context struct 和 mutex，消除共享锁竞争。

**TraceEmitter 兼容方式**：使用 embedding wrapper 而非 type alias（Go type alias 要求底层类型完全相同，字段集不同无法使用）：

```go
// TraceEmitter 是 FlowTracker 的 embedding wrapper，保持现有调用点兼容
type TraceEmitter struct {
    *FlowTracker
}
```

**替代方案**：
- A) 保持单体，内部按职责分区 → 治标不治本，mutex 竞争仍存在
- B) 完全独立，FlowTracker 不持有其他两个引用 → 调用方式大改，迁移成本高
- C) type alias → 不可行，Go type alias 要求底层类型相同

**理由**：组合方式保持 API 兼容（TraceEmitter 嵌入 FlowTracker，仍可代理调用），但内部职责清晰，各自独立 context + mutex，可独立测试和演化。

### D4: 构造注入为主，逐步淘汰 Global()

**选择**：
- 所有 service/biz/data 层对象通过构造函数注入 `loggateway.Logger`
- 请求作用域的 TraceEmitter 通过 context 传递
- `loggateway.Global()` 标记 deprecated，新增调用点禁止使用
- 现有 Global() 调用点按模块逐步迁移，不一次性改完
- 测试代码中的 Global() 调用（61 处）暂不迁移，测试中 Global() 使用合理

**替代方案**：
- A) 一次性全部迁移 → 改动面太大，风险高
- B) 保持 Global() → 隐式依赖，测试困难

**理由**：渐进迁移降低风险。deprecated 标记阻止新增调用点，旧调用点按模块逐步清理。生产代码仅 1 处 Global() 调用（flow_context.go:85），迁移成本低。

### D5: Throttler TTL 淘汰（含 goroutine 生命周期管理）

**选择**：每个 bucket 增加 `lastAccess atomic.Int64`（unix timestamp），后台 goroutine 每 5 分钟扫描，淘汰 > 30min 未访问的 bucket。

```go
type throttledBucket struct {
    *tokenBucket
    lastAccess atomic.Int64
}

type stepThrottler struct {
    // ... existing fields ...
    done chan struct{}  // 生命周期信号
}

// Start 启动后台淘汰 goroutine，由 Pipeline 生命周期管理
func (t *stepThrottler) Start()

// Stop 通知后台 goroutine 退出，等待完成
func (t *stepThrottler) Stop()
```

**替代方案**：
- A) LRU Cache → 过度设计，访问模式不适合 LRU
- B) 定时全量清空 → 已有（setRules 时），但触发条件不够

**理由**：lastAccess + 后台扫描是最简方案，无锁读取，仅扫描时加写锁。Start/Stop 方法由 Pipeline 生命周期管理，避免关闭后 goroutine 继续运行导致 panic。

### D6: 配置驱动 Sink 注册（Proto enum 约束）

**选择**：在 `conf.Logging` 中新增 `sinks` 配置，Pipeline 初始化时根据配置动态创建 SinkGroup。SinkType 和 DropPolicy 使用 Proto enum 约束，避免运行时才发现未知类型。

```protobuf
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
  // ... existing fields ...
  repeated LoggingSink sinks = 9;
}
```

**替代方案**：
- A) 保持硬编码 → 当前 3 个 Sink 够用，但扩展需改代码
- B) 插件式注册（init 函数）→ Go 生态不友好，隐式依赖
- C) type/drop_policy 用 string → 运行时才能发现未知类型，缺少编译期保障

**理由**：配置驱动 + Proto enum 是显式、可审计、编译期安全的，新增 Sink 只需实现接口 + 加 enum 值 + 加配置。

### D7: 切断 EventBus-日志反馈环

**选择**：三层防护：

1. **Bus.logDrop() 改用 stderr 直写**：`fmt.Fprintf(os.Stderr, ...)` 替代 `b.lg.Warn()`，彻底切断反馈路径
2. **Bus 增加 droppedCount 原子计数器**：替代 logDrop 中的 lg.Warn，丢弃事件通过计数器暴露给监控系统（Pipeline.Stats 或 metrics 端点），而非产生新日志
3. **EventBusSink 增加熔断机制**：连续 N 次 Publish 超时后暂停发布 T 秒，半开状态允许 3 次探测

```go
type EventBusSink struct {
    pub        Publisher
    level      logLevel
    author     string
    channel    string
    // 熔断状态
    failures   atomic.Int64
    openUntil  atomic.Int64  // unix timestamp, 熔断恢复时间
    halfOpenAttempts atomic.Int64  // 半开状态已尝试次数
}
```

**替代方案**：
- A) 在 Gateway.emitToPipeline 中增加 goroutine-local 重入标记 → 复杂，需 goroutine ID 或 context 传递
- B) 在 EventBusSink 中增加 sync.Mutex 重入检测 → 只能防同 goroutine 递归，不能防跨 goroutine 循环

**理由**：logDrop 改 stderr 是结构性切断（最可靠），droppedCount 计数器保证丢弃事件对监控可见（不产生新日志），熔断是运行时保护（防 EventBusSink 高频超时放大负载）。三层防护互补。半开状态允许 3 次探测，避免一次超时就重新熔断（过于激进）。

### D8: 桥接 trpc-agent-go 运行时日志（适配器在 internal/ 层）

**选择**：在 `internal/adapter/runtime_log.go` 中创建 `RuntimeLogAdapter`，实现 `trpc.group/trpc-go/trpc-agent-go/log.Logger` 接口，内部委托给 `loggateway.Logger`。在 `main()` 初始化阶段替换 `log.Default`。

**关键约束**：适配器放在 `internal/adapter/` 而非 `pkg/loggateway/`，因为 `pkg/loggateway` 不应依赖 `pkg/trpc-agent-go`（依赖方向：`internal/` → `pkg/`，不允许 `pkg/` → `pkg/` 交叉依赖）。

```go
// internal/adapter/runtime_log.go
type RuntimeLogAdapter struct {
    lg    loggateway.Logger
    fatal *zap.Logger  // 独立 zap.Logger 仅用于 Fatal 级别同步写入
}

func (a *RuntimeLogAdapter) Info(args ...any) {
    a.lg.Info(fmt.Sprint(args...))
}
func (a *RuntimeLogAdapter) Infof(format string, args ...any) {
    a.lg.Info(fmt.Sprintf(format, args...))
}
// ... Debug/Warn/Error 同理

// Fatal 特殊处理：同步写 stderr + 独立 zap.Logger，然后 os.Exit
func (a *RuntimeLogAdapter) Fatal(args ...any) {
    msg := fmt.Sprint(args...)
    fmt.Fprintln(os.Stderr, "[FATAL]", msg)
    a.fatal.Error(msg)  // 同步写文件
    os.Exit(1)
}
```

**替代方案**：
- A) 不桥接，保持隔离 → 运行时日志对运维不可见，排查需看两个日志源
- B) 修改 trpc-agent-go 源码支持 loggateway → 违反"不修改第三方库"原则
- C) 适配器放 `pkg/loggateway/` → 违反依赖方向（loggateway 不应依赖 trpc-agent-go）

**理由**：适配器在 `internal/` 层是最小侵入方案，不改 trpc-agent-go 源码，不改 biz 层代码（红线 #1 合规），不违反 pkg/ 层依赖方向。替换 `log.Default` 后，489 处运行时日志调用自动纳入 Pipeline。

**Fatal 特殊处理**：Fatal 不能走异步 Pipeline（可能来不及 flush），也不能直接访问 FileSink（它在 SinkGroup 内部）。方案：保留一个独立的 `*zap.Logger` 仅用于 Fatal 级别同步写入 + stderr 输出，然后 os.Exit(1)。这个独立 zap.Logger 不经过 Pipeline，是唯一允许的"直写"例外。

**注意**：`log.ContextDefault` 也需要替换，否则 Context 系列函数仍走独立 logger。需要在 `init()` 阶段同步替换。

### D9: 消除 boundInfraRef() 全局引用

**选择**：将 `Infra` 作为 FlowTracker 的构造参数注入，替代 `boundInfraRef()` 全局引用。FlowTracker.emit() 始终通过注入的 `Infra.Publish()` 发布，不再有 `e.bus` 降级路径。

```go
type FlowTracker struct {
    infra   Infra       // 替代 bus Bus + boundInfraRef()
    fc      *FlowContext  // 独立 context + mutex
    sc      *SpanCollector  // 可选，独立 context + mutex
    ua      *UsageAggregator // 可选，独立 context + mutex
}
```

**已知限制**：Infra 的 `Publish()` 方法内部仍使用 `monitorBusRef()` → `boundInfraRef()`。注入 Infra 并未完全消除全局引用，而是将全局依赖从 FlowTracker 转移到了 Infra。Infra 自身的全局引用清理为后续独立变更，不在本次 scope 内。

**替代方案**：
- A) 保持 boundInfraRef() → 全局可变状态，启动时序竞态，测试困难
- B) 让 FlowTracker 始终通过 `e.bus` 发布，由上层路由 → 需要改 Infra.Publish 路由逻辑

**理由**：构造注入是 DI 标准做法，消除 FlowTracker 层面的全局可变状态。FlowTracker 在构造时就知道用哪个 Infra，行为确定。Infra 内部的 boundInfraRef 清理为后续迭代。`boundInfraRef()` 和 `BindInfra()` 在 FlowTracker 全部迁移后标记 deprecated。

## Risks / Trade-offs

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| [R1] 双写消除后，Pipeline 成为单点 | Pipeline 挂则全部日志丢失 | Pipeline 已有 panic recover + DropNewest 保护；FileSink 内部 zapcore 写入是同步的，不会丢；Gateway 构造时注入 Pipeline 避免 nil 丢弃 |
| [R2] SinkGroup 增加内存开销 | 每个 Sink 独立 channel，总内存增加 | channel 默认大小与当前 Pipeline 相同（4096），可按 Sink 类型调优 |
| [R3] TraceEmitter embedding wrapper 增加一层间接 | 调用路径多一层 | 嵌入是 Go 零开销抽象，编译器内联；TraceEmitter 仅作为过渡期兼容层 |
| [R4] 构造注入迁移是渐进的，期间 Global() 和注入并存 | 两套方式并存可能混淆 | deprecated 标记 + lint 规则阻止新增 Global() 调用；测试代码 Global() 保留 |
| [R5] 配置驱动 Sink 注册增加 Proto 变更 | 需要 make api 重新生成 | 变更简单，只新增 message + enum，不影响现有字段 |
| [R6] RuntimeLogAdapter 丢失结构化信息 | trpc-agent-go 的 fmt 风格日志无法携带 Field | 适配器将 fmt.Sprint 结果作为 msg 传入，关键上下文（session_id 等）通过 With() 预设 |
| [R7] EventBusSink 熔断可能导致日志丢失 | 熔断期间所有日志跳过 EventBus | 熔断只影响 EventBusSink，FileSink/StdoutSink 不受影响；半开状态 3 次探测避免过早重熔断 |
| [R8] FlowTracker 构造注入 Infra 需要调整启动时序 | FlowTracker 必须在 Infra 就绪后创建 | Infra 在 main.go 中先于 Wire 图创建（已有此逻辑），FlowTracker 在 Wire provider 中创建时 Infra 已就绪 |
| [R9] Fatal 保留独立 zap.Logger 是"直写"例外 | 与"单写路径"原则不完全一致 | Fatal 是极端场景（进程即将退出），独立 zap.Logger 仅用于此级别，不影响正常日志路径 |
| [R10] Infra 内部仍使用 boundInfraRef() | 全局引用未完全消除 | FlowTracker 层面已消除；Infra 层面的清理为后续独立变更 |

## Migration Plan

五阶段渐进迁移，每阶段可独立部署和回滚：

### Phase 1: 切断反馈环 + 消除双写（最高优先级）

1. Bus.logDrop() 改用 stderr 直写 + droppedCount 计数器
2. EventBusSink 增加熔断机制（半开状态 3 次探测）
3. FileSink 内部改用 zapcore.Core（**必须先于 Gateway 去掉 zap**）
4. Gateway 去掉 zap.Logger 直写，改为 Pipeline.Emit 单写
5. Gateway 构造时注入 Pipeline（替代后置 SetPipeline）
6. 验证：日志格式不变，磁盘只写一份，反馈环切断

### Phase 2: SinkGroup 独立队列

1. Pipeline 内部为每个 Sink 创建 SinkGroup
2. Pipeline 主循环改为路由到 SinkGroup channel
3. 验证：慢 Sink 不影响其他 Sink

### Phase 3: TraceEmitter 拆分 + 消除 boundInfraRef + Throttler TTL

1. 拆分 TraceContext 为 FlowContext + SpanContext + UsageContext
2. 从 TraceEmitter 抽取 SpanCollector 和 UsageAggregator
3. 创建 FlowTracker（持有独立 context 的三个组件）
4. TraceEmitter 改为 embedding wrapper
5. FlowTracker 构造注入 Infra，替代 boundInfraRef()
6. stepThrottler 增加 lastAccess + 后台淘汰（含 Start/Stop 生命周期）
7. Wire 注入图更新
8. 验证：流程追踪功能不变，无全局可变状态，内存不再无界增长

### Phase 4: 桥接运行时日志

1. 在 internal/adapter/ 创建 RuntimeLogAdapter（实现 trpc-agent-go/log.Logger）
2. Fatal 特殊处理（独立 zap.Logger + stderr）
3. main() 中替换 log.Default 和 log.ContextDefault
4. 验证：trpc-agent-go 运行时日志出现在 Pipeline 输出中

### Phase 5: 配置驱动 + 去全局单例

1. conf.proto 新增 LoggingSink message + SinkType/DropPolicy enum
2. 实现 Sink 工厂函数
3. Pipeline 初始化改为配置驱动
4. loggateway.Global() 标记 deprecated
5. 按模块逐步迁移 Global() 调用为构造注入
6. 验证：Wire 注入图完整，无新增 Global() 调用

## Open Questions

- Q1: Phase 1 消除双写后，是否需要保留 `aranea.log`（zap 直写）作为备份？还是统一到 `aranea-pipeline.log`？
- Q2: SinkGroup 的默认 buffer 大小是否按 Sink 类型差异化？建议值：FileSink=8192, StdoutSink=4096, EventBusSink=2048
- Q3: FlowTracker 代理 SpanCollector/UsageAggregator 的方式是组合还是接口？组合更简单但耦合，接口更灵活但多一层抽象 → **审查建议：考虑将 FlowTracker 方法集限制为流程追踪，SpanCollector/UsageAggregator 通过独立注入获取**
- Q4: ~~RuntimeLogAdapter 是否需要为 Fatal 级别特殊处理？~~ → **已决定：Fatal 保留独立 zap.Logger + stderr，不走异步 Pipeline**
- Q5: ~~EventBusSink 熔断参数？~~ → **已决定：连续 5 次超时后暂停 10 秒，半开状态 3 次探测**
- Q6: boundInfraRef() 删除后，monitorBusRef() 是否也需要改造？当前它也使用全局引用 → **Infra 层面的全局引用清理为后续独立变更**
