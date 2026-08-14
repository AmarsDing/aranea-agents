# Logging Framework — 开发计划

> **对应需求**：[64-logging-framework.md](./64-logging-framework.md)
> **对应设计**：[64-logging-framework.design.md](./64-logging-framework.design.md)
>
> **状态**：✅ 主体完成（2026-06），少量遗留偏差跟踪中

---

## 1. 模块定位

日志框架开发分为两大主线：**日志统一迁移**（P0-P3）和 **LogPipeline 渐进式实施**（Phase 1-5），共修复 11 个 Bug，5 轮 aranea-review 验证通过。

---

## 2. 现状评估

### 2.1 已完成

- loggateway + logpipeline 基础设施搭建
- loggateway 全量迁移（log/slog 清零；CtxFlowLog* / `WithFlowLogger` / `FlowLoggerFromContext` / `NewFlowLogger` **已删除**；SysLog* 调用归零）
- LogPipeline 异步分发 + SinkGroup 隔离 + 三态熔断器
- RuntimeLogAdapter 桥接 trpc-agent-go 运行时日志
- 配置驱动 Sink 注册（SinkFactory）
- FlowTracker/SpanCollector/UsageAggregator 拆分
- stepThrottler TTL 淘汰机制
- 反馈环切断（EventBus DropLogger + 熔断器 stderr）

### 2.2 遗留偏差

| 编号 | 描述 | 状态 |
|------|------|------|
| A-1 | Kratos 框架日志未接入 loggateway（`cmd/admin/main.go` 仍用 `log.NewStdLogger(os.Stdout)`） | ⏳ 待解决 |
| P1-残留 | `internal/cronrunner/jobs/` 下 7 个文件仍使用 `log.NewHelper`（非 loggateway） | ⏳ 待迁移 |
| F-2 | `defaultOutputDir()` 在 `pkg/loggateway/gateway.go` 和 `pkg/logpipeline/file_sink.go` 重复 | 🟡 低优先级 |
| B-1 | `Envelope.Clone()` 对 Metadata 浅拷贝，当前 subscriber 只读风险低 | 🟡 低风险 |

---

## 3. 主线一：日志统一迁移

### P0: 基础设施搭建

**目标**：建立 loggateway + Zap Core + lumberjack + BusHook 基础设施

**Task 1: loggateway 包核心实现**

- [x] 定义 `Logger` 接口（Debug/Info/Warn/Error/With）
- [x] 实现 `Gateway` struct（zap 初始化、Pipeline 集成）
- [x] 实现 Field 构造函数（StepID/SessionID/TraceID/RunID/Domain/AgentKey/Phase/Duration/Source/Err/Str/Int/Int64/Float64/Bool/Any）
- [x] 实现错误链展开（`unwrapChain`）
- [x] 实现 `With()` 不可变语义（`loggerWith`）
- [x] 实现 nil 安全和 noop 模式

**Files**: `pkg/loggateway/logger.go`, `pkg/loggateway/gateway.go`

**Task 2: logpipeline 包核心实现**

- [x] 定义 `Pipeline` 接口（Emit/AddSink/Close/Dropped/Throttled/Stats/SetThrottleRules）
- [x] 实现 `pipeline` struct（异步分发、channel 缓冲）
- [x] 定义 `Sink` 接口（Write/Flush/Close）
- [x] 实现 `FileSink`（lumberjack JSON 落盘）
- [x] 实现 `StdoutSink`（stdout JSON）
- [x] 实现 `EventBusSink`（EventBus 发布）

**Files**: `pkg/logpipeline/pipeline.go`, `pkg/logpipeline/file_sink.go`, `pkg/logpipeline/stdout_sink.go`, `pkg/logpipeline/eventbus_sink.go`

**Task 3: Kratos 框架日志桥接**

- [ ] 实现 Kratos `log.Logger` → loggateway 桥接适配器
- [ ] 替换 `cmd/admin/main.go` 中的 `log.NewStdLogger(os.Stdout)` 为 loggateway 桥接
- [ ] 确保框架中间件日志经 Pipeline

**Files**: 待定（原计划 `pkg/loggateway/kratos_adapter.go` 未实现）

> **状态**：⏳ 未完成。对应已知偏差 A-1。当前 Kratos 框架日志仍走 stdout，不经 Pipeline。

**验证**: `go test ./pkg/loggateway/... ./pkg/logpipeline/... -count=1`

---

### P1: 迁移 Kratos log.NewHelper

**目标**：将 `internal/` 下所有 `log.NewHelper` 调用替换为 `loggateway.Logger`

**Task 4: 批量迁移 Kratos log.Helper**

- [x] 枚举所有 `log.NewHelper` 使用点（原 78 处）
- [x] 大部分替换为构造注入 `loggateway.Logger`
- [x] 更新 Wire ProviderSet
- [x] 验证红线 #10：`grep -r "log/slog" internal/` 为零
- [ ] 清理 `internal/cronrunner/jobs/` 下 7 个残留文件

**残留文件**（仍使用 `log.NewHelper`）：
- `internal/cronrunner/jobs/memory_dead_letter_replayer.go`
- `internal/cronrunner/jobs/monitor_alert_cooldown.go`
- `internal/cronrunner/jobs/auto_heal_ttl_cleanup.go`
- `internal/cronrunner/jobs/provider_health.go`
- `internal/cronrunner/jobs/channel_health.go`
- `internal/cronrunner/jobs/evolution_scanner.go`
- `internal/cronrunner/jobs/channel_delivery.go`

**验证**: `grep -r "log.NewHelper" internal/` 应接近零；`go build ./...`

---

### P2: 迁移 FlowLog SysLog*（262 处）

**目标**：将所有 `SysLog*` 调用替换为 `loggateway.Logger` + `With()` 预设字段

**Task 5: 批量迁移 SysLog***

- [x] 枚举所有 `SysLog*` 使用点（262 处）
- [x] 替换为 `lg.Info/Warn/Error()` + `loggateway.StepID()` 等字段
- [x] `SysLog*` 函数标记 deprecated
- [x] 验证调用归零

**验证**: `grep -r "SysLog" internal/` 应仅剩 deprecated 定义

---

### P3: 迁移 CtxFlowLog* + TraceEmitter（54 处）

**目标**：将所有 `CtxFlowLog*` 调用替换为 `loggateway.Logger` + `With()` 预设字段

**Task 6: 批量迁移 CtxFlowLog***

- [x] 枚举所有 `CtxFlowLog*` 使用点（54 处）
- [x] 替换为 `lg.With(loggateway.SessionID(...), loggateway.StepID(...))` 模式
- [x] `CtxFlowLog*` 函数已删除（`internal/event/flow_context.go` 仅保留 TraceEmitter ctx 传播）
- [x] 验证调用归零
- [x] P2-17：删除 `WithFlowLogger` / `FlowLoggerFromContext` / `NewFlowLogger` 兼容别名（无生产/测试调用）

**验证**: `grep -rE "CtxFlowLog|WithFlowLogger|FlowLoggerFromContext|NewFlowLogger" internal/` 应无 event 包别名定义或调用

---

## 4. 主线二：LogPipeline 渐进式实施

### Phase 1: Pipeline 构建 + Bug 修复

**目标**：建立异步 Pipeline 基础，修复初始 Bug

**Task 7: Pipeline 核心构建**

- [x] 实现 `dispatchLoop` goroutine
- [x] 实现 `Emit` 非阻塞写入
- [x] 实现 `Close` 优雅关闭

**Task 8: Bug #1/#2/#4/#5 修复**

- [x] Bug #1: Pipeline 关闭后仍可 Emit → `closed.Store(true)` 守卫
- [x] Bug #2: 向已关闭 channel 写入 panic → `select` + `ctx.Done()`
- [x] Bug #4: Sink.Write panic 影响 Pipeline → SinkGroup panic 恢复
- [x] Bug #5: 关闭时未排空 channel → `Close()` 先关闭 channel 再等待 goroutine 退出

**验证**: `go test ./pkg/logpipeline/... -count=1`

---

### Phase 2: EventBusSink 替换 + 消除桥接阻塞

**目标**：用 EventBusSink 替换 busHook，消除同步桥接阻塞

**Task 9: EventBusSink 实现**

- [x] 实现 `EventBusSink` struct
- [x] 实现 `Publisher` 接口
- [x] 替换 `busHook` 同步桥接为 EventBusSink 异步分发

**Task 10: Bug #6/#7 修复**

- [x] Bug #6: busHook 同步阻塞 Pipeline → EventBusSink 异步 + 超时
- [x] Bug #7: EventBus 发布失败无反馈 → 错误计数 + 熔断器

**验证**: `go test ./pkg/logpipeline/... -count=1`

---

### Phase 3: Flow Log 迁移 + EventBus Bug 修复

**目标**：FlowLog 通过 EventBusSink 发布，修复 EventBus 相关 Bug

**Task 11: FlowTracker 集成**

- [x] FlowTracker.emit 通过 loggateway Pipeline + EventBus 双写
- [x] FlowLogEntry 数据模型 + stepTitleRegistry

**Task 12: Bug #9/#11 修复**

- [x] Bug #9: EventBus 低优先级队列满时丢弃关键事件 → `contract.RequiresBlockUpTo` 强制 BlockUpTo（基于 AS-EVT-01 可靠性分级）
- [x] Bug #11: 反馈环 → EventBus DropLogger 改为 `loggateway.Logger.Warn` + Prometheus 指标（`arametrics.EventBusDropped`），不回入 EventBus

**验证**: `go test ./internal/event/... -count=1`

---

### Phase 4: 构造函数注入 + 测试覆盖

**目标**：全面构造注入，补齐测试覆盖

**Task 13: 构造注入改造**

- [x] 所有 Usecase/Service 通过构造注入获取 `loggateway.Logger`
- [x] `Global()` 标记 deprecated
- [x] Wire ProviderSet 更新

**Task 14: Bug #8 修复 + 测试覆盖**

- [x] Bug #8: stepThrottler buckets 无淘汰 → TTL 淘汰机制（60s 扫描，5min 未访问淘汰）
- [x] loggateway 测试（11 场景）
- [x] logpipeline 测试（8 + 9 场景）
- [x] EventBus 测试

**验证**: `go test ./pkg/loggateway/... ./pkg/logpipeline/... ./internal/event/... -count=1`

---

### Phase 5: 功能增强

**目标**：生产级功能增强

**Task 15: AtomicLevel 动态级别**

- [x] Gateway 集成 `zap.AtomicLevel`
- [x] `SetLevel()` 运行时动态调整

**Task 16: Pipeline 采样**

- [x] 采样配置支持
- [x] 采样率运行时调整

**Task 17: 监控指标**

- [x] `PipelineStats` 暴露
- [x] `SinkGroupStats` 暴露
- [x] 熔断器指标暴露
- [x] Prometheus 指标（`aranea_event_bus_published_total`, `aranea_event_bus_dropped_total`）

**Task 18: 配置驱动 Sink 注册**

- [x] `SinkConfig` + `SinkFactoryDeps` 工厂模式
- [x] `conf.proto` SinkType/DropPolicy enum
- [x] `cmd/admin/logging.go` 转换逻辑（`protoSinkToConfig`）

**Task 19: RuntimeLogAdapter 桥接**

- [x] 实现 `agentlog.Logger` 接口适配
- [x] Fatal 特殊处理（直写 stderr + 独立 zap.SugaredLogger）
- [x] With 不可变模式
- [x] `agentlog.Default` / `agentlog.ContextDefault` 替换

**Task 20: SinkGroup 隔离**

- [x] 独立 goroutine + channel 缓冲
- [x] DropPolicy 策略
- [x] Panic 恢复
- [x] 优雅关闭

**验证**: `go test ./pkg/loggateway/... ./pkg/logpipeline/... -count=1`

---

## 5. Bug 修复汇总

| Bug | 描述 | Phase | 修复方式 |
|-----|------|-------|---------|
| #1 | Pipeline 关闭后仍可 Emit | Phase 1 | `closed.Store(true)` 守卫 |
| #2 | 向已关闭 channel 写入 panic | Phase 1 | `select` + `ctx.Done()` |
| #4 | Sink.Write panic 影响 Pipeline | Phase 1 | SinkGroup panic 恢复 |
| #5 | 关闭时未排空 channel | Phase 1 | Close() 先关闭 channel 再等待 |
| #6 | busHook 同步阻塞 Pipeline | Phase 2 | EventBusSink 异步 + 超时 |
| #7 | EventBus 发布失败无反馈 | Phase 2 | 错误计数 + 熔断器 |
| #8 | stepThrottler buckets 无淘汰 | Phase 4 | TTL 淘汰机制（60s 扫描，5min 淘汰） |
| #9 | 低优先级队列丢弃关键事件 | Phase 3 | `contract.RequiresBlockUpTo` BlockUpTo |
| #11 | 反馈环 | Phase 3 | EventBus DropLogger 走 `loggateway.Warn` + Prometheus 指标，不回入 EventBus |

> Bug #3/#10 在历史记录中跳号，原审计文档已不可考（`openspec/` 目录已不存在）。

---

## 6. 代码锚点

### 6.1 loggateway 包

| 文件 | 作用 | 状态 |
|------|------|------|
| `pkg/loggateway/logger.go` | Logger 接口 + Field 构造函数 + 错误链展开（`unwrapChain`） | ✅ |
| `pkg/loggateway/gateway.go` | Gateway 核心（zap 初始化、Pipeline 集成、AtomicLevel、With、emitToPipeline、nil 安全、noop） | ✅ |
| `pkg/loggateway/gateway_test.go` | Gateway 测试（11 场景） | ✅ |

### 6.2 logpipeline 包

| 文件 | 作用 | 状态 |
|------|------|------|
| `pkg/logpipeline/pipeline.go` | Pipeline 接口/实现 + stepThrottler + tokenBucket + TTL 淘汰 | ✅ |
| `pkg/logpipeline/sink_group.go` | SinkGroup（独立 goroutine + channel + DropPolicy + Panic 恢复） | ✅ |
| `pkg/logpipeline/eventbus_sink.go` | EventBusSink + 三态熔断器（cbClosed/cbOpen/cbHalfOpen） | ✅ |
| `pkg/logpipeline/file_sink.go` | FileSink（lumberjack JSON 轮转，默认文件名 `aranea-pipeline.log`） | ✅ |
| `pkg/logpipeline/stdout_sink.go` | StdoutSink（stdout JSON） | ✅ |
| `pkg/logpipeline/sink_factory.go` | Sink 工厂（`NewSinkFromConfig` + `SinkConfig` + `SinkFactoryDeps`） | ✅ |
| `pkg/logpipeline/sanitizing_sink.go` | SanitizingSink（密钥清洗包装：msg + string fields 经 `preview.RedactAndTruncate`，12 类模式） | ✅ |
| `pkg/logpipeline/pipeline_test.go` | Pipeline 测试（8 场景） | ✅ |
| `pkg/logpipeline/sink_test.go` | Sink 测试（9 场景） | ✅ |
| `pkg/logpipeline/sanitizing_sink_test.go` | SanitizingSink 密钥清洗测试 | ✅ |

### 6.3 internal/event 包

| 文件 | 作用 | 状态 |
|------|------|------|
| `internal/event/flow_tracker.go` | FlowTracker（流程追踪核心，LogStart/LogDone/LogError 等） | ✅ |
| `internal/event/span_collector.go` | SpanCollector（Span 树管理 + usage.metadata_json） | ✅ |
| `internal/event/usage_aggregator.go` | UsageAggregator（用量聚合，桥接 trpc-agent-go 事件流） | ✅ |
| `internal/event/trace_emitter.go` | TraceEmitter（v2 embedding wrapper，嵌入 FlowTracker + ObserveFrameworkEvent + EmitProgress） | ✅ |
| `internal/event/flow_log.go` | FlowLogEntry 数据模型 + stepTitleRegistry（约 80 个 step_id 中文标题） | ✅ |
| `internal/event/flow_context.go` | `WithTraceEmitter` / `TraceEmitterFromContext` / `NewTraceEmitterForRun`（CtxFlowLog* 与 FlowLogger 别名已删除） | ✅ |
| `internal/event/infra.go` | Infra 双总线路由（SessionBus + MonitorBus，split/dual 模式，WAL WBPF） | ✅ |
| `internal/event/bus.go` | EventBus 类型别名 + NewBus | ✅ |
| `internal/event/bus_adapter.go` | busAdapter（framework bus 桥接 + DropLogger 走 loggateway.Warn） | ✅ |
| `internal/event/buffer.go` | Buffer 环形回放（200 条/分区，30min TTL，5min 扫描） | ✅ |
| `internal/event/logpipeline_publisher.go` | busPublisher（EventBusSink → contract.Bus 桥接） | ✅ |

### 6.4 桥接器与初始化

| 文件 | 作用 | 状态 |
|------|------|------|
| `internal/adapter/runtime_log.go` | RuntimeLogAdapter（trpc-agent-go agentlog.Logger → loggateway Pipeline，Fatal 直写 stderr） | ✅ |
| `internal/plugin/trpc/safe_logger.go` | PluginSafeLogger（插件日志双写：loggateway + EventBus） | ✅ |
| `internal/runtime/deps.go` | TurnDeps.Lg 字段（loggateway.Logger 注入到 chat turn） | ✅ |
| `cmd/admin/logging.go` | 初始化流程（`initLogging` + `protoSinkToConfig` + RuntimeLogAdapter 替换） | ✅ |
| `cmd/admin/main.go` | 入口（Kratos logger 仍用 `log.NewStdLogger`，对应 A-1 偏差） | ⏳ A-1 |

### 6.5 trpc-agent-go 框架（vendored）

| 文件 | 作用 | 状态 |
|------|------|------|
| `pkg/trpc-agent-go/log/log.go` | Agent 运行时日志（独立 zap.Sugar，Default/ContextDefault 被 RuntimeLogAdapter 替换） | ✅ |
| `pkg/trpc-agent-go/plugin/logging.go` | Agent 生命周期日志插件 | ✅ |

### 6.6 配置

| 文件 | 作用 | 状态 |
|------|------|------|
| `internal/conf/conf.proto` | Logging 配置 Proto（level/output_dir/max_size_mb/sinks 等 + SinkType/DropPolicy enum） | ✅ |

### 6.7 前端对应

| 文件 | 作用 | 状态 |
|------|------|------|
| `web/src/components/monitor/FlowLogStream.vue` | 实时 Flow Log 流 | ✅ |
| `web/src/components/monitor/FlowTracePanel.vue` | Flow 追踪面板 | ✅ |
| `web/src/components/monitor/FlowLogExportButton.vue` | Flow Log 导出 | ✅ |
| `web/src/components/monitor/ProcessLogStream.vue` | 进程日志流 | ✅ |
| `web/src/features/monitor/useLogStreamHub.ts` | WS 连接管理 | ✅ |

---

## 7. 实施进度

### 7.1 日志统一迁移

| 阶段 | 内容 | 状态 |
|------|------|------|
| P0 | 基础设施（loggateway + Zap Core + lumberjack + BusHook） | ✅ 已完成 |
| P1 | 迁移 Kratos log.NewHelper（原 78 处，残留 7 处 cronrunner） | 🟡 基本完成 |
| P2 | 迁移 FlowLog SysLog*（262 处 → 调用归零） | ✅ 已完成 |
| P3 | 迁移 CtxFlowLog* + TraceEmitter（54 处 → 调用归零） | ✅ 已完成 |
| P2-17 | 删除 FlowLogger 别名（`WithFlowLogger` / `FlowLoggerFromContext` / `NewFlowLogger`） | ✅ 已完成 |

P3 方式：`loggateway.Logger` + `With()` 预设字段替代 CtxFlowLog*。P2-17：别名已删除而非继续 Deprecated；流程日志 ctx 用 `WithTraceEmitter`，创建用 `NewTraceEmitterForRun`。

### 7.2 LogPipeline 渐进式实施

| Phase | 目标 | 状态 |
|-------|------|------|
| 1 | Pipeline 构建 + Bug #1/#2/#4/#5 修复 | ✅ |
| 2 | EventBusSink 替换 busHook + 消除桥接阻塞 | ✅ |
| 3 | Flow Log 迁移 + EventBus Bug #6/#7/#9/#11 修复 | ✅ |
| 4 | 构造函数注入 + 测试覆盖（Bug #8 修复） | ✅ |
| 5 | 功能增强（AtomicLevel, Pipeline 采样, 监控指标, 配置驱动 Sink, RuntimeLogAdapter, SinkGroup） | ✅ |
| 6 | 密钥清洗入 Pipeline（SanitizingSink + preview 12 类模式，Grok Build 借鉴） | ✅ 2026-07-20 |

### 7.3 Bug 修复记录

11 个 Bug 全部修复，5 轮 aranea-review 验证通过。

---

## 8. 代码量统计

| 指标 | 数值 |
|------|------|
| `internal/` 下 loggateway 引用文件数 | ~100+ |
| `internal/` 下 loggateway 引用总次数 | ~1,146 |
| `log/slog` 残留 | 0（红线 #16 合规） |
| `log.NewHelper` 残留（非 loggateway） | 7 处 / 7 文件（cronrunner/jobs） |
| zap 直接引用 | 7 文件（含 trpc-agent-go 运行时） |
| 已注册 step_id 标题映射 | ~80 个（`internal/event/flow_log.go` stepTitleRegistry） |

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

## 9. 测试覆盖

| 包 | 测试文件 | 场景数 |
|----|---------|--------|
| `pkg/loggateway` | `gateway_test.go` | 11 |
| `pkg/logpipeline` | `pipeline_test.go` + `sink_test.go` | 8 + 9 |
| `internal/event` | 多个测试文件 | bus/flow_context/session_revision/event_reliability/framework_adapter/trace_emitter 等 |

---

## 10. 已知偏差跟踪

| 编号 | 严重性 | 描述 | 文件 | 状态 |
|------|--------|------|------|------|
| A-1 | 黄 | Kratos 框架日志未接入 loggateway（`log.NewStdLogger(os.Stdout)`） | `cmd/admin/main.go` | ⏳ 待解决 |
| A-2 | ✅ | ~~trpc-agent-go 运行时日志未接入 loggateway~~ | — | ✅ 已通过 RuntimeLogAdapter 解决 |
| A-3 | ✅ | ~~双日志接口并存无桥接~~ | — | ✅ 已通过 RuntimeLogAdapter 解决 |
| R4-2/R5-1 | ✅ | ~~TraceEmitter 仍用 `bus Bus` + `boundInfraRef()`~~ | — | ✅ 已通过 FlowTracker 构造注入 Infra 解决，`boundInfraRef()` 和 `BindInfra()` 已标记 deprecated |
| R5-2 | ✅ | ~~`stepThrottler.buckets` map 无淘汰机制~~ | — | ✅ 已通过 TTL 淘汰机制解决 |
| F-2 | 黄 | `defaultOutputDir()` 在 gateway.go 和 file_sink.go 重复 | `pkg/loggateway/gateway.go`, `pkg/logpipeline/file_sink.go` | 🟡 待解决 |
| B-1 | 黄 | `Envelope.Clone()` 对 Metadata 浅拷贝，当前 subscriber 只读风险低 | `internal/event/contract/envelope.go` | 🟡 低风险 |
| P1-残留 | 黄 | `internal/cronrunner/jobs/` 7 个文件仍用 `log.NewHelper` | `internal/cronrunner/jobs/*.go` | ⏳ 待迁移 |

---

## 11. 验证清单

### 11.1 红线合规

- [x] `grep -r "log/slog" internal/` 为零（红线 #16）
- [x] `grep -r "zap\." internal/` 为零（红线 #10a，7 文件例外含 trpc-agent-go 运行时）
- [x] `Global()` 无新增调用

### 11.2 功能验证

- [x] `go test ./pkg/loggateway/... -count=1` 通过
- [x] `go test ./pkg/logpipeline/... -count=1` 通过
- [x] `go test ./internal/event/... -count=1` 通过
- [x] `go build ./cmd/admin` 通过

### 11.3 构造注入验证

- [x] Wire 编译通过
- [x] 所有 Usecase/Service 通过构造注入获取 Logger

---

## 12. 改动文件清单

### 12.1 已完成改动

- `pkg/loggateway/logger.go` — Logger 接口 + Field 构造函数 + 错误链展开
- `pkg/loggateway/gateway.go` — Gateway 核心 + With + emitToPipeline + AtomicLevel
- `pkg/logpipeline/pipeline.go` — Pipeline + stepThrottler + TTL 淘汰
- `pkg/logpipeline/sink_group.go` — SinkGroup 隔离
- `pkg/logpipeline/eventbus_sink.go` — EventBusSink + 三态熔断器
- `pkg/logpipeline/file_sink.go` — FileSink (lumberjack)
- `pkg/logpipeline/stdout_sink.go` — StdoutSink
- `pkg/logpipeline/sink_factory.go` — Sink 工厂
- `pkg/logpipeline/sanitizing_sink.go` — SanitizingSink 密钥清洗包装（Phase 6，2026-07-20 Grok Build 借鉴）
- `internal/tools/preview/preview.go` — `RedactAndTruncate` 扩至 12 类密钥模式（厂商 key / AWS / GitHub PAT / Anthropic / Slack / Stripe / Google / PEM 私钥 / Bearer / JWT / DSN / 赋值）
- `internal/event/flow_tracker.go` — FlowTracker 流程追踪核心
- `internal/event/span_collector.go` — SpanCollector
- `internal/event/usage_aggregator.go` — UsageAggregator
- `internal/event/trace_emitter.go` — TraceEmitter embedding wrapper
- `internal/event/flow_log.go` — FlowLogEntry + stepTitleRegistry
- `internal/event/flow_context.go` — TraceEmitter ctx 传播（FlowLogger 别名已删除）
- `internal/event/infra.go` — Infra 双总线路由
- `internal/event/bus.go` — EventBus 类型别名
- `internal/event/bus_adapter.go` — busAdapter + DropLogger
- `internal/event/buffer.go` — Buffer 环形回放
- `internal/event/logpipeline_publisher.go` — busPublisher
- `internal/adapter/runtime_log.go` — RuntimeLogAdapter
- `internal/plugin/trpc/safe_logger.go` — PluginSafeLogger
- `internal/runtime/deps.go` — TurnDeps.Lg
- `cmd/admin/logging.go` — initLogging + protoSinkToConfig
- `internal/conf/conf.proto` — Logging + SinkType + DropPolicy + LoggingSink

### 12.2 待改动（遗留偏差）

- `cmd/admin/main.go` — A-1：替换 `log.NewStdLogger(os.Stdout)` 为 loggateway 桥接
- `internal/cronrunner/jobs/*.go` — P1-残留：7 个文件迁移 `log.NewHelper` → `loggateway.Logger`
- `pkg/loggateway/gateway.go` + `pkg/logpipeline/file_sink.go` — F-2：提取公共 `defaultOutputDir()`

---

## 13. 相关文档

| 文档 | 路径 | 定位 |
|------|------|------|
| 日志框架需求 | `docs/development/64-logging-framework.md` | 用户故事、功能需求、验收标准 |
| 日志框架设计 | `docs/development/64-logging-framework.design.md` | 架构、接口、Proto、数据模型 |
| 项目规则-日志架构约束 | `.trae/rules/project_rules.md` §日志架构约束 | 红线、组件表、使用规则 |
| 监控模块开发计划 | `docs/development/18-monitor.development.md` | FlowFileAppender 分类落盘 |
