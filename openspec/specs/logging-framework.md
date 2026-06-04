# Logging Framework Spec

> Aranea-Agents 日志框架权威规范。精简参考，聚焦规则、约束与结构。

---

## 1. 双轨制架构

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
| **loggateway** | `pkg/loggateway` | 通用结构化日志（红线 #10 唯一日志 API） | Pipeline → File/Stdout/EventBus |
| **Flow Log** | `internal/event` | 业务流程步骤追踪 | EventBus → WS/JSONL/DB |

---

## 2. 红线约束

| 编号 | 规则 | 验证方式 |
|------|------|----------|
| #10 | 禁止 `log/slog`，统一使用 `pkg/loggateway.Logger` | `grep -r "log/slog" internal/` 应为零 |
| #10a | 禁止直接使用 `zap` 全局 logger，必须通过 `loggateway` | `grep -r "zap\." internal/` 应为零 |
| #10b | 禁止 `fmt.Print*` 用于日志输出 | 代码审查 |

---

## 3. 接口定义

### 3.1 loggateway.Logger

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

### 3.2 logpipeline.Pipeline

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

### 3.3 logpipeline.Sink

```go
type Sink interface {
    Write(entry LogEntry)
    Flush()
    Close() error
}
```

Sink 实现：`FileSink`（lumberjack JSON 落盘）、`StdoutSink`（stdout JSON）、`EventBusSink`（EventBus 发布）

### 3.4 TraceEmitter（已拆分）

TraceEmitter 已从单一 struct 拆分为三层组件，TraceEmitter 现在是 embedding wrapper：

```go
// TraceEmitter 是 v2 统一写入器：FlowLog (WS) + span buffer (usage metadata)
// 它嵌入 FlowTracker，并添加 ObserveFrameworkEvent 用于 trpc-agent-go 事件流
type TraceEmitter struct {
    *FlowTracker
}
```

**拆分后的三层组件**：

| 组件 | 路径 | 职责 |
|------|------|------|
| **FlowTracker** | `internal/event/flow_tracker.go` | 流程追踪核心，持有 FlowContext + SpanCollector + UsageAggregator，提供 LogStart/LogDone/LogError 等方法签名 |
| **SpanCollector** | `internal/event/span_collector.go` | Span 树管理，管理 LLM/Tool span 的生命周期，生成 usage.metadata_json |
| **UsageAggregator** | `internal/event/usage_aggregator.go` | 用量聚合，观察框架事件并聚合 usage 元数据，桥接 trpc-agent-go 事件流 |

**依赖关系**：`TraceEmitter` → `FlowTracker` → `SpanCollector` + `UsageAggregator`

**关键方法**：
```go
func (e *TraceEmitter) ObserveFrameworkEvent(ev *trpcevent.Event)  // 桥接 trpc-agent-go 事件流
func (ft *FlowTracker) LogStart(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogDone(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogSkip(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogWarn(stepID, title, message string, extra ...Pair)
func (ft *FlowTracker) LogError(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogCritical(stepID, message string, extra ...Pair)
```

### 3.5 FlowLogEntry

Schema 版本 `flow_log/v1`，包含：
- `correlation`（trace_id, session_id, run_id, domain, agent_key, agent_id）
- `step`（id, phase, subsystem）
- `severity`（ok / info / warn / error / critical）
- `title` / `message` / `hint`
- `timing`（duration_ms）
- `error`（code, message）

---

## 4. 初始化流程

```
1. logpipeline.NewPipeline(4096)       → 异步管道（单 worker, buffer=4096）
2. pipeline.AddSink(fileSink)          → FileSink
3. pipeline.AddSink(stdoutSink)        → StdoutSink（可配）
4. loggateway.New(bc.Logging, pipeline)→ Gateway 构造时注入 Pipeline
5. wireApp(..., lg, pipeline)          → Wire DI
6. BeforeStart: pipeline.AddSink(eventBusSink)
7. AfterStop:  pipeline.Close()
```

---

## 4.5 SinkGroup

每个 Sink 由独立的 `SinkGroup` 包装，实现 goroutine 隔离 + channel 缓冲 + DropPolicy 策略，确保慢 Sink 不影响其他 Sink。

```go
type SinkGroup struct {
    sink       Sink
    ch         chan LogEntry       // 独立 channel 缓冲
    wg         sync.WaitGroup
    dropped    atomic.Uint64
    dropPolicy DropPolicy
    name       string
    ctx        context.Context
    cancel     context.CancelFunc
}
```

**设计要点**：

| 特性 | 说明 |
|------|------|
| **独立 goroutine** | 每个 SinkGroup 启动独立 goroutine 从 channel 读取并写入 Sink，Sink.Write 的阻塞/慢速不影响 Pipeline 分发 |
| **DropPolicy** | `DropNewest`（默认）：缓冲区满时丢弃新条目；`DropBlock`：阻塞调用方直到缓冲区有空间 |
| **Panic 恢复** | `run()` 循环中 Sink.Write 的 panic 会被 recover，不影响 SinkGroup 继续处理后续条目 |
| **优雅关闭** | `Close()` 取消 context → 关闭 channel → 等待 goroutine 退出 → 关闭底层 Sink |
| **统计** | `Stats()` 返回 `SinkGroupStats{Name, Dropped, ChanLen, ChanCap}` |

**Pipeline 集成**：Pipeline 内部维护 `[]*SinkGroup`，`AddSink()` 自动包装为默认 SinkGroup（bufSize=4096, DropNewest），`AddSinkGroup()` 允许自定义参数。

**关键文件**：`pkg/logpipeline/sink_group.go`

---

## 4.6 反馈环切断

日志系统曾存在反馈环问题：当日志条目被丢弃时，系统尝试记录丢弃事件，可能触发更多丢弃，形成无限循环。现已通过两层机制切断：

**Bus.logDrop() 改造**：
- `bus.logDrop()` 不再通过 EventBus 发布丢弃通知（避免递归）
- 改为 `fmt.Fprintf(os.Stderr, ...)` 直写 stderr + `droppedCount atomic.Uint64` 原子计数器
- 丢弃信息不经过 Pipeline/EventBus，确保不产生反馈环
- `droppedCount` 通过 `Pipeline.Stats()` 暴露

**EventBusSink 熔断机制**：
- 三态熔断器：`cbClosed`（正常）→ `cbOpen`（熔断）→ `cbHalfOpen`（探测）
- 连续 5 次超时/失败后进入 `cbOpen`，所有写入跳过，持续 10 秒
- 10 秒后进入 `cbHalfOpen`，允许探测写入；半开状态下连续 3 次失败才重新进入 `cbOpen`
- 熔断状态转换时写入 stderr（不经过 Pipeline/EventBus），确保不产生反馈环
- 超时控制：`Publish` 调用设置 50ms 超时，超时视为失败
- 熔断器指标通过 `Pipeline.Stats()` 暴露：`circuit_breaker_open`、`circuit_breaker_skipped`、`circuit_breaker_half_open_attempts`

**关键文件**：`internal/event/bus.go`（logDrop）、`pkg/logpipeline/eventbus_sink.go`（熔断器）

---

## 4.7 运行时日志桥接

trpc-agent-go 运行时日志（独立 zap.Sugar）已通过 `RuntimeLogAdapter` 桥接到 loggateway Pipeline。

```go
// RuntimeLogAdapter 实现 agentlog.Logger，委托给 loggateway.Logger
// 将 trpc-agent-go 运行时日志桥接到 loggateway Pipeline
type RuntimeLogAdapter struct {
    lg    loggateway.Logger
    base  []loggateway.Field
    fatal *zap.SugaredLogger  // Fatal/Fatalf 特殊处理：直写 stderr + os.Exit(1)
}
```

**设计要点**：

| 特性 | 说明 |
|------|------|
| **接口适配** | 实现 `agentlog.Logger`（Debug/Info/Warn/Error/Fatal 及格式化版本），编译期检查 `var _ agentlog.Logger = (*RuntimeLogAdapter)(nil)` |
| **Fatal 特殊处理** | Fatal/Fatalf 不走异步 Pipeline（进程即将退出），直写 stderr + 独立 zap.SugaredLogger |
| **With 不可变模式** | `With(fields...)` 返回新实例，原始 adapter 不被修改 |
| **解决 A-2/A-3 偏差** | 之前 trpc-agent-go 运行时日志仅 stdout 不持久化，现经 Pipeline 统一落盘 |

**关键文件**：`internal/adapter/runtime_log.go`

---

## 4.8 配置驱动 Sink 注册

Sink 注册已从硬编码改为配置驱动，通过 `conf.proto` SinkType/DropPolicy enum + `sink_factory.go` 工厂模式实现。

**Proto 定义**（`internal/conf/conf.proto`）：

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
  // ... 原有字段 ...
  repeated LoggingSink sinks = 9;  // 配置驱动的 Sink 列表
}
```

**工厂模式**（`pkg/logpipeline/sink_factory.go`）：

```go
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
- `SinkConfig` 与 `internal/conf` proto 解耦，cmd/admin/main.go 负责转换
- EventBus Sink 的 Publisher 无法从配置推导，通过 `SinkFactoryDeps` 注入
- 每个 Sink 的 `config` map 支持类型特定参数（如 file 的 output_dir/filename，eventbus 的 hook_level）

---

## 4.9 Global() Deprecated

`loggateway.Global()` 已标记 deprecated，新代码应通过构造注入获取 `loggateway.Logger`。

```go
// Deprecated: use constructor injection instead of global singleton.
func Global() *Gateway
```

**迁移方式**：
- `Gateway` 在 `New()` 时自动设置全局变量（向后兼容），但新代码不应依赖
- 应通过 Wire 构造注入 `loggateway.Logger` 到需要的 Usecase/Service
- `SetGlobal()` 保留用于测试和特殊场景

---

## 4.10 stepThrottler TTL 淘汰机制

`stepThrottler` 的 `buckets` map 曾无淘汰机制，长时间运行会导致内存无界增长。现已通过 TTL 淘汰机制解决：

**设计要点**：

| 特性 | 说明 |
|------|------|
| **lastAccess 追踪** | 每个 bucket 记录 `lastAccess atomic.Int64`（Unix 时间戳），`shouldThrottle` 中更新 |
| **后台淘汰 goroutine** | 每 5 分钟扫描 buckets，淘汰 `lastAccess > 30min` 的条目 |
| **生命周期管理** | `Start()`/`Stop()` 方法由 Pipeline 控制，`Pipeline.Close()` 调用 `Stop()` |
| **可配置 TTL** | `ThrottleConfig` 增加 `TTL` 和 `ScanInterval` 字段 |
| **淘汰安全性** | 读写锁分离，淘汰不阻塞 `shouldThrottle` 热路径 |

**关键文件**：`pkg/logpipeline/pipeline.go`

---

## 5. 配置规格

### 5.1 Proto 定义（conf.Logging）

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| level | string | info | 日志级别 (debug/info/warn/error) |
| output_dir | string | Linux: `/var/log/aranea`, Win: `./logs` | 输出目录 |
| max_size_mb | int32 | 100 | 单文件最大 MB |
| max_backups | int32 | 10 | 保留旧文件数 |
| max_age_days | int32 | 30 | 最大保留天数 |
| compress | bool | false | 是否压缩旧文件 |
| stdout_enabled | bool | false | 是否同时输出 stdout |
| hook_level | string | — | EventBusSink 级别阈值 |

### 5.2 环境变量

| 变量 | 用途 |
|------|------|
| `MONITOR_FLOW_LOG_DIR` | 覆盖日志输出目录 |

---

## 6. 步骤注册表

采用 `{domain}.{subsystem}.{action}` 点分命名，已注册 ~80 个 step_id。

| 域 | 示例 step_id | 数量 |
|----|-------------|------|
| chat | chat.agent.build, chat.turn.start, chat.turn.end | ~15 |
| team | team.graph.compile, team.graph.run | ~10 |
| knowledge | knowledge.search, knowledge.index | ~8 |
| plugin | plugin.hook.invoke, plugin.guard.check | ~10 |
| system | system.cron.tick, system.health.check | ~8 |
| memory | memory.worker.run, memory.deadletter.replay | ~6 |
| channel | channel.deliver, channel.health | ~8 |
| model | model.sync, model.apply | ~6 |
| monitor | monitor.alert, monitor.flow | ~5 |
| 其他 | a2a.*, session.*, evaluation.* | ~4 |

---

## 7. 实施进度

### 7.1 日志统一迁移

| 阶段 | 内容 | 状态 |
|------|------|------|
| P0 | 基础设施（loggateway + Zap Core + lumberjack + BusHook + KratosAdapter） | ✅ 已完成 |
| P1 | 迁移 Kratos log.NewHelper（78 处） | ✅ 已完成 |
| P2 | 迁移 FlowLog SysLog*（262 处 → 调用归零） | ✅ 已完成 |
| P3 | 迁移 CtxFlowLog* + TraceEmitter（54 处 → 调用归零） | ✅ 已完成 |

P3 方式：`loggateway.Logger` + `With()` 预设字段替代 CtxFlowLog*，CtxFlowLog*/FlowLog* 函数已标记 deprecated。

### 7.2 LogPipeline 渐进式实施

| Phase | 目标 | 状态 |
|-------|------|------|
| 1 | Pipeline 构建 + Bug #1/#2/#4/#5 修复 | ✅ |
| 2 | EventBusSink 替换 busHook + 消除桥接阻塞 | ✅ |
| 3 | Flow Log 迁移 + EventBus Bug #6/#7/#9/#11 修复 | ✅ |
| 4 | 构造函数注入 + 测试覆盖（Bug #8 修复） | ✅ |
| 5 | 功能增强（AtomicLevel, Pipeline 采样, 监控指标, OTelSink） | ✅ |

### 7.3 Bug 修复记录

11 个 Bug 全部修复，5 轮 aranea-review 验证通过。详见 `openspec/issues/logging-issues.md`。

---

## 8. 代码量统计

| 指标 | 数值 |
|------|------|
| `internal/` 下 loggateway 引用文件数 | ~100+ |
| `internal/` 下 loggateway 引用总次数 | ~1,146 |
| `log/slog` 残留 | 0（红线 #10 合规） |
| `log.Info/Error/Warn` 残留（非 loggateway） | 64 处 / 31 文件 |
| zap 直接引用 | 7 文件（含 trpc-agent-go 运行时） |
| 已注册 step_id 标题映射 | ~80 个 |
| 已删除废弃文件 | 5 个 |

### 8.1 各模块 loggateway 使用分布

| 层/模块 | 引用次数 | 说明 |
|---------|---------|------|
| service | ~666 | 最重使用层 |
| biz | ~663 | 业务逻辑层 |
| data | ~640 | 数据层 |
| agent | ~259 | Agent 构建/运行 |
| tools | ~199 | 工具集 |
| plugin/trpc | ~149 | trpc 插件 |
| team | ~184 | 团队编排 |
| cronrunner | ~176 | 定时任务 |
| modelregistry | ~168 | 模型注册表 |
| channel | ~155 | 渠道集成 |
| 其他（graph/session/skill/...） | ~200+ | 各子系统 |

---

## 9. 已知偏差

| 编号 | 严重性 | 描述 | 文件 |
|------|--------|------|------|
| R4-2/R5-1 | ~~黄~~ ✅ | ~~TraceEmitter 仍用 `bus Bus` + `boundInfraRef()` 而非 `logpipeline.Pipeline`~~ | 已通过 FlowTracker 构造注入 Infra 解决，`boundInfraRef()` 和 `BindInfra()` 已标记 deprecated |
| R5-2 | ~~黄~~ ✅ | ~~`stepThrottler.buckets` map 无淘汰机制，长时间运行可能内存增长~~ | 已通过 TTL 淘汰机制解决（详见 §4.10） |
| F-2 | 黄 | `defaultOutputDir()` 在 gateway.go 和 file_sink.go 重复 | 两文件 |
| B-1 | 黄 | `Envelope.Clone()` 对 Metadata 浅拷贝，当前 subscriber 只读风险低 | `internal/event/contract/envelope.go` |

### 9.1 架构层面不一致

| 编号 | 描述 | 影响 | 状态 |
|------|------|------|------|
| A-1 | Kratos 框架日志未接入 loggateway（`log.NewStdLogger(os.Stdout)`） | 框架中间件日志走 stdout，不经 Pipeline | 待解决 |
| A-2 | ~~trpc-agent-go 运行时日志未接入 loggateway（独立 zap.Sugar）~~ | ~~Agent 生命周期日志仅 stdout，不持久化~~ | ✅ 已通过 RuntimeLogAdapter 解决 |
| A-3 | ~~双日志接口并存（loggateway.Logger Field 风格 vs trpc-agent-go/log.Logger fmt 风格）~~ | ~~无桥接，运行时日志和业务日志走不同路径~~ | ✅ 已通过 RuntimeLogAdapter 解决 |

---

## 10. 测试覆盖

| 包 | 测试文件 | 场景数 |
|----|---------|--------|
| `pkg/loggateway` | `gateway_test.go` | 11 |
| `pkg/logpipeline` | `pipeline_test.go` + `sink_test.go` | 8 + 9 |
| `internal/event` | 多个测试文件 | bus/flow_context/session_revision 等 |

---

## 11. 模块关联

### 11.1 上游依赖 loggateway 的模块

agent, provider, team, cron, a2a, session/status, plugin

### 11.2 上游依赖 event（日志）的模块

agent, provider, team, cron, a2a, plugin

### 11.3 前端对应

| 后端日志系统 | 前端组件 |
|-------------|---------|
| FlowLog (EnvelopeTypeFlowLog) | MonitorPage FlowLogStream.vue, FlowTracePanel.vue, FlowLogExportButton.vue |
| 进程日志 (EnvelopeTypeLog) | MonitorPage ProcessLogStream.vue |
| WS 推送 | useLogStreamHub.ts |
| Flow Log 落库 | ListFlowLogs HTTP API → 前端历史查询 |

### 11.4 数据库

| 表/文件 | 用途 |
|---------|------|
| `flow_log_events` | FlowLog 持久化 |
| `aranea-*.log` | Zap 统一日志文件（lumberjack 轮转） |
| `flow-*.jsonl` / `system-*.jsonl` / `log-*.jsonl` / `trace-*.jsonl` / `alert-*.jsonl` | FlowFileAppender 分类落盘 |

---

## 12. EventBus 与日志的关系

- 双总线：SessionBus（业务）+ MonitorBus（监控），物理隔离
- 三级队列：high(64) / normal(128) / low(256)，日志走 low
- DropNewest 策略：low/normal 队列满时丢弃新消息
- `flow_log` 不受 `logEnabled` 开关限制；`log` 仍受限制

---

## 13. 关键文件索引

| 文件 | 作用 |
|------|------|
| `pkg/loggateway/logger.go` | Logger 接口 + Field 构造函数 + 错误链展开 |
| `pkg/loggateway/gateway.go` | Gateway 核心实现（zap 初始化、Pipeline 集成、全局单例） |
| `pkg/logpipeline/pipeline.go` | Pipeline 接口 + 实现（异步分发、采样、监控） |
| `pkg/logpipeline/file_sink.go` | FileSink（lumberjack JSON 落盘） |
| `pkg/logpipeline/stdout_sink.go` | StdoutSink（开发调试） |
| `pkg/logpipeline/eventbus_sink.go` | EventBusSink（Pipeline → EventBus，含三态熔断器） |
| `pkg/logpipeline/sink_group.go` | SinkGroup（独立 goroutine + channel + DropPolicy 隔离） |
| `pkg/logpipeline/sink_factory.go` | Sink 工厂（配置驱动 Sink 注册） |
| `internal/event/flow_log.go` | FlowLogEntry 数据模型 + stepTitle 注册表 |
| `internal/event/flow_tracker.go` | FlowTracker（流程追踪核心，替代 TraceEmitter 核心逻辑） |
| `internal/event/span_collector.go` | SpanCollector（Span 树管理 + usage.metadata_json） |
| `internal/event/usage_aggregator.go` | UsageAggregator（用量聚合，桥接 trpc-agent-go 事件流） |
| `internal/event/trace_emitter.go` | TraceEmitter（v2 embedding wrapper，嵌入 FlowTracker） |
| `internal/event/flow_context.go` | CtxFlowLog* 快捷函数 |
| `internal/event/logpipeline_publisher.go` | busPublisher（EventBusSink → contract.Bus 桥接） |
| `internal/adapter/runtime_log.go` | RuntimeLogAdapter（trpc-agent-go 运行时日志 → loggateway Pipeline） |
| `internal/plugin/trpc/safe_logger.go` | PluginSafeLogger（插件日志 → loggateway + EventBus） |
| `internal/runtime/deps.go` | TurnDeps.Lg 字段（loggateway.Logger 注入到 chat turn） |
| `pkg/trpc-agent-go/log/log.go` | Agent 运行时日志（独立 zap.Sugar） |
| `pkg/trpc-agent-go/plugin/logging.go` | Agent 生命周期日志插件 |
| `internal/conf/conf.proto` | Logging 配置 Proto 定义 |
| `cmd/admin/main.go` | 日志初始化 + Pipeline 构造 + 生命周期钩子 |

---

## 14. 相关文档

| 文档 | 路径 | 定位 |
|------|------|------|
| 日志问题审计 | `openspec/issues/logging-issues.md` | 11 Bug + 5 Phase + 5 次审查 |
| 日志统一迁移 | `openspec/changelog/2026-05-31-Logging-Unification.md` | P0-P3 四期迁移方案 |
| FlowLogger 需求 | `openspec/requirements/52-flow-logger.md` | FlowLogger v2 需求规格 |
| FlowLogger 设计 | `openspec/requirements/52-flow-logger.design.md` | TraceEmitter API + 步骤注册表 |
| FlowLogger 开发 | `openspec/requirements/52-flow-logger-development.md` | Phase 1a/1b/1c/2/3 任务拆分 |
| SlogBridge 移除 | `openspec/changelog/2026-05-20-FlowLog-V2-SlogRemoval.md` | slog 全量迁移 |
| FlowLogger 审查 | `docs/review/52-flowlogger-review.md` | 79/100 评分 + P1/P2 风险 |
