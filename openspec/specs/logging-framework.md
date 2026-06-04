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

---

## logging-framework (from logging-framework-v2)

### Requirement: Gateway single-write path
Gateway SHALL NOT directly write to zap.Logger. All log entries SHALL be emitted through Pipeline.Emit only. FileSink SHALL internally use zapcore.Core for JSON file writing. Gateway SHALL receive Pipeline at construction time (not via post-construction SetPipeline).

#### Scenario: Gateway.Info writes through Pipeline only
- **WHEN** Gateway.Info("message", fields...) is called
- **THEN** Pipeline.Emit SHALL be called exactly once
- **AND** zap.Logger.Info SHALL NOT be called directly on the Gateway

#### Scenario: Gateway constructed with Pipeline
- **WHEN** loggateway.New(bc.Logging, pipeline) is called
- **THEN** the Gateway SHALL hold a reference to Pipeline from construction
- **AND** SetPipeline SHALL NOT be needed

#### Scenario: FileSink uses zapcore internally
- **WHEN** FileSink.Write(entry) is called
- **THEN** the entry SHALL be serialized using zapcore.JSONEncoder
- **AND** written to lumberjack via zapcore.AddSync

#### Scenario: No duplicate log files
- **WHEN** a log entry is emitted at info level
- **THEN** exactly one JSON line SHALL appear in the output file
- **AND** no duplicate entry SHALL exist in any other log file

### Requirement: Configuration-driven Sink registration
Pipeline Sink registration SHALL be driven by conf.Logging.Sinks configuration. Hard-coded Sink registration in main.go SHALL be replaced by config-based registration. SinkType and DropPolicy SHALL use Proto enum (not free-form strings) for compile-time safety.

#### Scenario: FileSink registered via config
- **WHEN** conf.Logging.Sinks contains {name: "file", type: SINK_TYPE_FILE, buffer_size: 8192}
- **THEN** a FileSink SHALL be created with buffer_size=8192
- **AND** registered with Pipeline as a SinkGroup

#### Scenario: EventBusSink registered via config
- **WHEN** conf.Logging.Sinks contains {name: "eventbus", type: SINK_TYPE_EVENTBUS, buffer_size: 2048, drop_policy: DROP_POLICY_NEWEST}
- **THEN** an EventBusSink SHALL be created with the specified config

#### Scenario: Unknown Sink type
- **WHEN** conf.Logging.Sinks contains an unknown SinkType enum value
- **THEN** Pipeline initialization SHALL log a warning and skip that Sink
- **AND** other Sinks SHALL be registered normally

### Requirement: loggateway.Global() deprecation
loggateway.Global() SHALL be marked as deprecated. New code SHALL use constructor injection. Existing call sites SHALL be migrated incrementally per module. Test code (61 call sites) SHALL NOT be migrated — Global() usage in tests is reasonable.

#### Scenario: New code uses constructor injection
- **WHEN** a new service/biz/data struct is created
- **THEN** it SHALL receive loggateway.Logger via constructor parameter
- **AND** it SHALL NOT call loggateway.Global()

#### Scenario: Existing Global() calls continue to work
- **WHEN** existing code calls loggateway.Global()
- **THEN** it SHALL return the global Gateway instance
- **AND** a deprecation comment SHALL be present on the function

#### Scenario: Test code Global() calls remain
- **WHEN** test code calls loggateway.Global()
- **THEN** it SHALL work as before
- **AND** no migration SHALL be required for test code

### Requirement: LoggingSink Proto message with enums
A new LoggingSink message SHALL be added to conf.proto with fields: name, type (SinkType enum), buffer_size, drop_policy (DropPolicy enum), and config map. SinkType and DropPolicy SHALL be Proto enums.

#### Scenario: Proto compilation
- **WHEN** make api is run after adding LoggingSink message and enums
- **THEN** the generated Go code SHALL compile without errors
- **AND** existing Logging message fields SHALL remain unchanged

### Requirement: SinkGroup stats aggregation
Pipeline.Stats() SHALL aggregate stats from all SinkGroups, including per-Sink dropped counts and buffer utilization.

#### Scenario: Stats include per-Sink metrics
- **WHEN** Pipeline.Stats() is called
- **THEN** the result SHALL include per-Sink dropped count
- **AND** per-Sink channel length and capacity

### Requirement: Eliminate boundInfraRef() global mutable state
FlowTracker SHALL receive Infra as a constructor parameter. The global `boundInfraRef()` function and `BindInfra()` mutation SHALL be eliminated for FlowTracker usage. FlowTracker.emit() SHALL always use the injected Infra.Publish(), with no fallback to a global reference.

**Known limitation**: Infra.Publish() internally still uses monitorBusRef() → boundInfraRef(). This proposal only eliminates the global reference at the FlowTracker layer. Infra-level cleanup is a future independent change.

#### Scenario: FlowTracker uses injected Infra
- **WHEN** FlowTracker.emit() is called
- **THEN** it SHALL use the Infra instance provided at construction time
- **AND** it SHALL NOT call boundInfraRef()

#### Scenario: FlowTracker created before BindInfra
- **WHEN** a FlowTracker is created with an Infra instance
- **AND** BindInfra() has not yet been called
- **THEN** FlowTracker.emit() SHALL still work correctly via the injected Infra
- **AND** no global state dependency SHALL exist

#### Scenario: boundInfraRef removal
- **WHEN** all FlowTracker/TraceEmitter instances use injected Infra
- **THEN** boundInfraRef() and BindInfra() SHALL be marked deprecated
- **AND** they SHALL be removed in a future cleanup iteration

---

## eventbus-feedback-break (from logging-framework-v2)

### Requirement: Bus.logDrop uses stderr directly
Bus.logDrop() SHALL write to os.Stderr directly using fmt.Fprintf, NOT through loggateway.Logger. This structurally prevents the feedback loop: Bus.logDrop → Gateway.Warn → Pipeline → EventBusSink → Bus.Publish.

#### Scenario: Message drop notification does not enter Pipeline
- **WHEN** Bus.deliverToSubscriber() drops a message due to buffer overflow
- **AND** Bus.logDrop() is called
- **THEN** the drop notification SHALL be written to os.Stderr
- **AND** loggateway.Logger.Warn() SHALL NOT be called
- **AND** the notification SHALL NOT enter logpipeline.Pipeline

#### Scenario: Drop notification format
- **WHEN** Bus.logDrop() writes a drop notification
- **THEN** the format SHALL include: timestamp, envelope type, subscriber ID, reason
- **AND** the format SHALL be parseable by standard log analysis tools

### Requirement: Bus droppedCount counter
Bus SHALL maintain an atomic droppedCount counter that increments on every message drop. This counter SHALL be exposed via Pipeline.Stats() or a metrics endpoint, providing visibility into drop events without producing new log entries.

#### Scenario: droppedCount increments on drop
- **WHEN** Bus.deliverToSubscriber() drops a message
- **THEN** Bus.droppedCount SHALL increment by 1
- **AND** no new log entry SHALL be produced

#### Scenario: droppedCount visible in stats
- **WHEN** Pipeline.Stats() is called
- **THEN** the result SHALL include Bus.droppedCount

### Requirement: EventBusSink circuit breaker with half-open probing
EventBusSink SHALL implement a circuit breaker that pauses publishing after consecutive timeout failures. The half-open state SHALL allow 3 probe attempts before re-opening, preventing premature re-closing.

#### Scenario: Circuit breaker opens after consecutive failures
- **WHEN** EventBusSink.Publish() times out 5 consecutive times
- **THEN** the circuit breaker SHALL open
- **AND** subsequent Publish calls SHALL be skipped for 10 seconds
- **AND** skipped entries SHALL increment a circuit_breaker_skipped counter

#### Scenario: Circuit breaker enters half-open state after cooldown
- **WHEN** the circuit breaker has been open for 10 seconds
- **THEN** the circuit breaker SHALL enter half-open state
- **AND** the next Publish call SHALL be attempted
- **AND** halfOpenAttempts SHALL be set to 1

#### Scenario: Half-open probe succeeds
- **WHEN** a half-open Publish call succeeds
- **THEN** the circuit breaker SHALL close
- **AND** halfOpenAttempts SHALL be reset to 0
- **AND** failures SHALL be reset to 0

#### Scenario: Half-open probe fails, but attempts < 3
- **WHEN** a half-open Publish call times out
- **AND** halfOpenAttempts < 3
- **THEN** halfOpenAttempts SHALL increment by 1
- **AND** the circuit breaker SHALL remain in half-open state
- **AND** the next Publish call SHALL still be attempted

#### Scenario: Half-open probe fails 3 times
- **WHEN** half-open Publish calls time out 3 consecutive times
- **THEN** the circuit breaker SHALL re-open for another 10 seconds
- **AND** halfOpenAttempts SHALL be reset to 0

#### Scenario: Circuit breaker does not affect other Sinks
- **WHEN** EventBusSink's circuit breaker is open
- **THEN** FileSink and StdoutSink SHALL continue processing entries normally
- **AND** no entries SHALL be dropped from FileSink/StdoutSink due to EventBusSink's circuit breaker

### Requirement: EventBusSink circuit breaker metrics
EventBusSink SHALL expose circuit breaker state via Pipeline.Stats() for monitoring.

#### Scenario: Stats include circuit breaker state
- **WHEN** Pipeline.Stats() is called
- **THEN** the result SHALL include EventBusSink circuit_breaker_open (bool), circuit_breaker_skipped (uint64), and half_open_attempts (int64)

---

## runtime-log-bridge (from logging-framework-v2)

### Requirement: RuntimeLogAdapter implements trpc-agent-go/log.Logger
A RuntimeLogAdapter SHALL be created in `internal/adapter/runtime_log.go` that implements the `trpc.group/trpc-go/trpc-agent-go/log.Logger` interface, delegating all calls to a `loggateway.Logger` instance. The adapter SHALL be placed in `internal/adapter/` (NOT `pkg/loggateway/`) to avoid pkg/ layer cross-dependency (loggateway SHALL NOT import trpc-agent-go).

#### Scenario: Info-level runtime log forwarded to Pipeline
- **WHEN** RuntimeLogAdapter.Info("agent started") is called
- **THEN** loggateway.Logger.Info("agent started") SHALL be called
- **AND** the entry SHALL enter logpipeline.Pipeline

#### Scenario: Formatted runtime log forwarded to Pipeline
- **WHEN** RuntimeLogAdapter.Infof("model %s invoked", "gpt-4") is called
- **THEN** loggateway.Logger.Info("model gpt-4 invoked") SHALL be called

#### Scenario: Debug-level runtime log respects loggateway level
- **WHEN** RuntimeLogAdapter.Debug("trace detail") is called
- **AND** loggateway level is set to "info"
- **THEN** the entry SHALL NOT be written to any Sink

### Requirement: Fatal level special handling
RuntimeLogAdapter.Fatal() and RuntimeLogAdapter.Fatalf() SHALL synchronously write to os.Stderr and an independent *zap.Logger (not through Pipeline), then call os.Exit(1). The independent zap.Logger is the only allowed "direct write" exception, used solely for Fatal level because Pipeline's async dispatch may not flush before exit.

#### Scenario: Fatal log is written before exit
- **WHEN** RuntimeLogAdapter.Fatal("critical failure") is called
- **THEN** the message SHALL be written to os.Stderr synchronously
- **AND** the message SHALL be written to the independent zap.Logger synchronously
- **AND** os.Exit(1) SHALL be called after both writes complete

#### Scenario: Fatal does not go through Pipeline
- **WHEN** RuntimeLogAdapter.Fatal("critical failure") is called
- **THEN** loggateway.Logger SHALL NOT be called for this entry
- **AND** the entry SHALL NOT enter logpipeline.Pipeline

### Requirement: Replace log.Default at startup
In cmd/admin/main.go, after loggateway.New() succeeds, `trpc.group/trpc-go/trpc-agent-go/log.Default` and `log.ContextDefault` SHALL be replaced with a RuntimeLogAdapter instance wrapping the loggateway.Logger.

#### Scenario: Runtime logs appear in Pipeline output after startup
- **WHEN** the application starts with loggateway configured
- **AND** an Agent run produces runtime logs via log.Infof()
- **THEN** those logs SHALL appear in the Pipeline output (file/stdout/EventBus)

#### Scenario: Runtime logs before loggateway init go to default stdout
- **WHEN** trpc-agent-go code calls log.Info() before loggateway.New() completes
- **THEN** the log SHALL go to the default zap.Sugar stdout output
- **AND** no logs SHALL be lost during the startup window

### Requirement: RuntimeLogAdapter preserves context fields
RuntimeLogAdapter SHALL support With() to preset context fields (e.g., session_id, agent_key) that are automatically attached to all forwarded log entries. With() SHALL return a new RuntimeLogAdapter instance (immutable pattern).

#### Scenario: Preset fields attached to runtime logs
- **WHEN** RuntimeLogAdapter.With(loggateway.SessionID("abc")) is called
- **AND** the returned adapter.Info("step completed") is called
- **THEN** the log entry SHALL include session_id="abc" in its fields

#### Scenario: With returns new instance
- **WHEN** RuntimeLogAdapter.With(fields...) is called
- **THEN** a new RuntimeLogAdapter instance SHALL be returned
- **AND** the original adapter SHALL remain unchanged

### Requirement: No modification to trpc-agent-go source code
The RuntimeLogAdapter SHALL NOT require any changes to the trpc-agent-go source code. It SHALL only use the public `log.Logger` interface and `log.Default`/`log.ContextDefault` variables.

#### Scenario: trpc-agent-go remains unmodified
- **WHEN** RuntimeLogAdapter is integrated
- **THEN** no files under `pkg/trpc-agent-go/` SHALL be modified

### Requirement: Adapter in internal/ layer, not pkg/
RuntimeLogAdapter SHALL be placed in `internal/adapter/` to avoid pkg/ layer cross-dependency. `pkg/loggateway` SHALL NOT import `pkg/trpc-agent-go`.

#### Scenario: loggateway does not depend on trpc-agent-go
- **WHEN** RuntimeLogAdapter is integrated
- **THEN** `pkg/loggateway/` SHALL NOT import any package from `pkg/trpc-agent-go/`

---

## sink-group (from logging-framework-v2)

### Requirement: SinkGroup independent queue model
Each Sink registered with Pipeline SHALL be wrapped in a SinkGroup with an independent goroutine and channel buffer. A slow Sink SHALL NOT block other SinkGroups.

#### Scenario: Slow Sink does not affect other Sinks
- **WHEN** EventBusSink.Write() takes 200ms per call (e.g., EventBus congestion)
- **AND** FileSink is also registered
- **THEN** FileSink SHALL continue writing without delay
- **AND** EventBusSink's channel SHALL drop entries when full (DropNewest policy)

#### Scenario: SinkGroup buffer overflow
- **WHEN** a SinkGroup's channel buffer is full
- **AND** drop_policy is DropNewest
- **THEN** the new entry SHALL be dropped
- **AND** SinkGroup.dropped counter SHALL increment by 1

#### Scenario: SinkGroup with block policy
- **WHEN** a SinkGroup's channel buffer is full
- **AND** drop_policy is DropBlock
- **THEN** Pipeline.Emit SHALL block until the channel has space
- **AND** no entries SHALL be dropped for this SinkGroup

### Requirement: SinkGroup.Emit method signature
SinkGroup.Emit SHALL be a non-blocking method that returns immediately after routing the entry to the channel (or dropping it). The method signature SHALL be `Emit(entry LogEntry) error`.

#### Scenario: Emit returns nil on successful routing
- **WHEN** SinkGroup.Emit(entry) is called
- **AND** the channel has space
- **THEN** the entry SHALL be placed in the channel
- **AND** nil SHALL be returned

#### Scenario: Emit returns error on drop
- **WHEN** SinkGroup.Emit(entry) is called
- **AND** the channel is full with DropNewest policy
- **THEN** the entry SHALL be dropped
- **AND** an error indicating drop SHALL be returned

### Requirement: Per-SinkGroup configurable buffer size
Each SinkGroup SHALL support a configurable channel buffer size. Default values SHALL be: FileSink=8192, StdoutSink=4096, EventBusSink=2048.

#### Scenario: Custom buffer size via config
- **WHEN** LoggingSink config specifies buffer_size=16384 for a file Sink
- **THEN** the SinkGroup's channel SHALL be created with capacity 16384

#### Scenario: Default buffer size when not specified
- **WHEN** LoggingSink config does not specify buffer_size
- **THEN** the SinkGroup SHALL use the default buffer size for that Sink type

### Requirement: SinkGroup panic recovery
Each SinkGroup goroutine SHALL recover from panics in Sink.Write() and increment an error counter, without crashing the goroutine.

#### Scenario: Sink.Write() panics
- **WHEN** a Sink's Write() method panics
- **THEN** the SinkGroup goroutine SHALL recover
- **AND** sink_errors counter SHALL increment by 1
- **AND** the SinkGroup SHALL continue processing subsequent entries

### Requirement: SinkGroup lifecycle management
Pipeline.Close() SHALL wait for all SinkGroup goroutines to drain their channels before returning.

#### Scenario: Graceful shutdown with pending entries
- **WHEN** Pipeline.Close() is called
- **AND** a SinkGroup channel has 100 pending entries
- **THEN** the SinkGroup SHALL write all 100 entries to the Sink
- **AND** Pipeline.Close() SHALL wait for the SinkGroup goroutine to finish

### Requirement: DropPolicy enum
DropPolicy SHALL be a Go enum type (`type DropPolicy int`) with values DropNewest=0 and DropBlock=1, aligned with the Proto DropPolicy enum.

#### Scenario: DropPolicy values match Proto enum
- **WHEN** a LoggingSink config has drop_policy=DROP_POLICY_NEWEST
- **THEN** the SinkGroup SHALL use DropNewest DropPolicy

---

## throttler-ttl (from logging-framework-v2)

### Requirement: Bucket lastAccess tracking
Each throttled bucket SHALL track its last access timestamp. The lastAccess field SHALL be updated atomically on every shouldThrottle call.

#### Scenario: lastAccess updated on access
- **WHEN** shouldThrottle("chat.llm.invoke") is called
- **THEN** the bucket for the matching rule SHALL have its lastAccess set to the current unix timestamp

#### Scenario: lastAccess not updated for empty stepID
- **WHEN** shouldThrottle("") is called
- **THEN** no bucket SHALL be accessed or created

### Requirement: Background TTL eviction with lifecycle management
A background goroutine SHALL periodically scan all buckets and evict those not accessed within the TTL window. Default TTL SHALL be 30 minutes. Scan interval SHALL be 5 minutes. The goroutine SHALL have explicit Start/Stop lifecycle management controlled by Pipeline.

#### Scenario: Bucket evicted after TTL
- **WHEN** a bucket was last accessed 35 minutes ago
- **AND** TTL is 30 minutes
- **THEN** the bucket SHALL be removed from the map

#### Scenario: Bucket retained within TTL
- **WHEN** a bucket was last accessed 15 minutes ago
- **AND** TTL is 30 minutes
- **THEN** the bucket SHALL remain in the map

#### Scenario: Background scan continues after eviction
- **WHEN** the background goroutine evicts 10 buckets
- **THEN** the goroutine SHALL continue running
- **AND** the next scan SHALL proceed normally

#### Scenario: Start begins the eviction goroutine
- **WHEN** stepThrottler.Start() is called
- **THEN** the background eviction goroutine SHALL begin running
- **AND** it SHALL scan at the configured interval

#### Scenario: Stop terminates the eviction goroutine
- **WHEN** stepThrottler.Stop() is called
- **THEN** the background goroutine SHALL exit gracefully
- **AND** no further scans SHALL occur
- **AND** Stop SHALL block until the goroutine has exited

#### Scenario: Pipeline lifecycle manages throttler
- **WHEN** Pipeline.Close() is called
- **THEN** stepThrottler.Stop() SHALL be called before Pipeline returns
- **AND** no panic SHALL occur from the goroutine accessing closed resources

### Requirement: TTL configuration
The TTL duration and scan interval SHALL be configurable via ThrottleConfig.

#### Scenario: Custom TTL via config
- **WHEN** ThrottleConfig.TTL is set to 60 minutes
- **THEN** buckets not accessed for 60 minutes SHALL be evicted

#### Scenario: Default TTL when not specified
- **WHEN** ThrottleConfig.TTL is not set
- **THEN** default TTL of 30 minutes SHALL be used

### Requirement: Eviction safety
The background eviction goroutine SHALL NOT block or interfere with shouldThrottle calls. Eviction SHALL use a write lock; shouldThrottle SHALL use a read lock when possible.

#### Scenario: Concurrent eviction and throttle check
- **WHEN** the eviction goroutine is scanning buckets
- **AND** a goroutine calls shouldThrottle
- **THEN** shouldThrottle SHALL use a read lock that does not block on the eviction write lock for more than 1ms

---

## trace-emitter-split (from logging-framework-v2)

### Requirement: FlowTracker component
FlowTracker SHALL be a standalone component responsible for flow step lifecycle tracking (LogStart/LogDone/LogSkip/LogWarn/LogError/LogCritical). It SHALL NOT contain span management or usage aggregation logic. It SHALL hold its own FlowContext with an independent mutex.

#### Scenario: LogStart emits flow log entry
- **WHEN** FlowTracker.LogStart("chat.agent.build", "Building agent") is called
- **THEN** a FlowLogEntry with step.phase="start" SHALL be emitted via EventBus
- **AND** the step timer SHALL be recorded in FlowContext

#### Scenario: LogDone calculates duration
- **WHEN** FlowTracker.LogDone("chat.agent.build", "Agent built") is called
- **AND** LogStart was called 150ms earlier for the same stepID
- **THEN** a FlowLogEntry with step.phase="done" and timing.duration_ms=150 SHALL be emitted

#### Scenario: LogError publishes error envelope
- **WHEN** FlowTracker.LogError("chat.llm.invoke", "Model timeout") is called
- **THEN** a FlowLogEntry with step.phase="error" and severity="error" SHALL be emitted
- **AND** if shouldPublishFlowChatError returns true, an EnvelopeTypeError SHALL also be published to EventBus

### Requirement: SpanCollector component
SpanCollector SHALL be a standalone component responsible for span tree management (startSpan/endSpan/FinishRoot). It SHALL hold its own SpanContext with an independent mutex. It SHALL NOT depend on FlowTracker or UsageAggregator.

#### Scenario: Start and end span
- **WHEN** SpanCollector.StartSpan("llm.call", rootID, attrs) is called
- **THEN** a new span entry with status="running" SHALL be created in SpanContext
- **AND** the span ID SHALL be returned

#### Scenario: EndSpan records duration
- **WHEN** SpanCollector.EndSpan(spanID, "ok") is called
- **AND** the span was started 200ms earlier
- **THEN** the span's duration_ms SHALL be set to 200
- **AND** the span's status SHALL be set to "ok"

#### Scenario: FinishRoot closes all pending spans
- **WHEN** FinishRoot("error") is called
- **AND** there are 3 pending spans (1 LLM, 2 tool calls)
- **THEN** all pending spans SHALL be ended with status "error"

### Requirement: UsageAggregator component
UsageAggregator SHALL be a standalone component responsible for usage metadata collection (ObserveFrameworkEvent/mergeLLMSpan/MetadataJSON). It SHALL hold its own UsageContext with an independent mutex. It SHALL NOT depend on FlowTracker or SpanCollector.

#### Scenario: Merge LLM span with token counts
- **WHEN** UsageAggregator.MergeLLMSpan(100, 50) is called
- **THEN** the current open LLM span SHALL be updated with prompt_tokens=100, completion_tokens=50

#### Scenario: MetadataJSON output
- **WHEN** UsageAggregator.MetadataJSON() is called
- **THEN** it SHALL return a JSON string containing trace_id, spans array, and trace_root_ms

#### Scenario: OTel span ID sync
- **WHEN** UsageAggregator.SyncOtelSpanIDs(src) is called
- **THEN** matching spans SHALL have their otel_id field populated from the source

### Requirement: TraceContext split into independent contexts
The current monolithic TraceContext SHALL be split into three independent structs, each with its own mutex:
- FlowContext: flow step timing data
- SpanContext: span tree data
- UsageContext: usage metadata data

#### Scenario: No shared mutex between components
- **WHEN** FlowTracker is writing to FlowContext
- **AND** SpanCollector is writing to SpanContext
- **THEN** neither SHALL block the other (independent mutexes)

### Requirement: FlowTracker composes SpanCollector and UsageAggregator
FlowTracker SHALL hold optional references to SpanCollector and UsageAggregator. When present, FlowTracker.LogStart/LogDone SHALL delegate span tracking to SpanCollector, and ObserveFrameworkEvent SHALL delegate to UsageAggregator.

#### Scenario: FlowTracker with SpanCollector
- **WHEN** FlowTracker.LogStart("chat.llm.invoke", "Calling LLM") is called
- **AND** FlowTracker holds a SpanCollector reference
- **THEN** SpanCollector.StartSpan SHALL be called with the step ID
- **AND** a FlowLogEntry SHALL be emitted

#### Scenario: FlowTracker without SpanCollector
- **WHEN** FlowTracker.LogStart("chat.llm.invoke", "Calling LLM") is called
- **AND** FlowTracker does NOT hold a SpanCollector reference
- **THEN** only a FlowLogEntry SHALL be emitted (no span tracking)

### Requirement: shouldPublishFlowChatError belongs to FlowTracker
The shouldPublishFlowChatError function SHALL remain in FlowTracker, as it is a flow-level decision (based on flow type), not a span-level or usage-level concern.

#### Scenario: FlowTracker decides whether to publish chat error
- **WHEN** FlowTracker.LogError is called
- **THEN** FlowTracker SHALL evaluate shouldPublishFlowChatError using its own FlowContext data
- **AND** if true, publish EnvelopeTypeError via Infra

### Requirement: TraceEmitter backward compatibility via embedding wrapper
TraceEmitter SHALL be an embedding wrapper for FlowTracker (`type TraceEmitter struct { *FlowTracker }`), preserving all existing method signatures. Existing call sites SHALL NOT require changes. Type alias is NOT used because Go type alias requires identical underlying types.

#### Scenario: Existing code using TraceEmitter
- **WHEN** existing code calls TraceEmitter.LogStart(stepID, message)
- **THEN** the call SHALL work identically to FlowTracker.LogStart(stepID, message)

#### Scenario: TraceEmitter is not a type alias
- **WHEN** TraceEmitter is defined
- **THEN** it SHALL be `type TraceEmitter struct { *FlowTracker }` (embedding wrapper)
- **AND** it SHALL NOT be `type TraceEmitter = FlowTracker` (type alias)
