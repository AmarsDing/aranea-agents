# Logging Framework — 开发计划

> **对应需求**：[64-logging-framework.md](./64-logging-framework.md)
> **对应设计**：[64-logging-framework.design.md](./64-logging-framework.design.md)
>
> **状态**：✅ 全部完成（2026-06）

---

## 总览

日志框架开发分为两大主线：**日志统一迁移**（P0-P3）和 **LogPipeline 渐进式实施**（Phase 1-5），共修复 11 个 Bug，5 轮 aranea-review 验证通过。

---

## 主线一：日志统一迁移

### P0: 基础设施搭建

**目标**：建立 loggateway + Zap Core + lumberjack + BusHook + KratosAdapter 基础设施

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

**Task 3: KratosAdapter 桥接**

- [x] 实现 Kratos log.Helper → loggateway 桥接
- [x] 确保框架中间件日志经 Pipeline

**Files**: `pkg/loggateway/kratos_adapter.go`

**验证**: `go test ./pkg/loggateway/... ./pkg/logpipeline/... -count=1`

---

### P1: 迁移 Kratos log.NewHelper（78 处）

**目标**：将 `internal/` 下所有 `log.NewHelper` 调用替换为 `loggateway.Logger`

**Task 4: 批量迁移 Kratos log.Helper**

- [x] 枚举所有 `log.NewHelper` 使用点（78 处）
- [x] 替换为构造注入 `loggateway.Logger`
- [x] 更新 Wire ProviderSet
- [x] 验证红线 #10：`grep -r "log/slog" internal/` 为零

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
- [x] `CtxFlowLog*` 函数标记 deprecated
- [x] 验证调用归零

**验证**: `grep -r "CtxFlowLog" internal/` 应仅剩 deprecated 定义

---

## 主线二：LogPipeline 渐进式实施

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

- [x] Bug #9: EventBus 低优先级队列满时丢弃关键事件 → `criticalTypeSet` 强制 BlockUpTo
- [x] Bug #11: 反馈环 → Bus.logDrop() 改为 stderr 直写

**验证**: `go test ./internal/event/... -count=1`

---

### Phase 4: 构造函数注入 + 测试覆盖

**目标**：全面构造注入，补齐测试覆盖

**Task 13: 构造注入改造**

- [x] 所有 Usecase/Service 通过构造注入获取 `loggateway.Logger`
- [x] `Global()` 标记 deprecated
- [x] Wire ProviderSet 更新

**Task 14: Bug #8 修复 + 测试覆盖**

- [x] Bug #8: stepThrottler buckets 无淘汰 → TTL 淘汰机制
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
- [x] `cmd/admin/logging.go` 转换逻辑

**Task 19: RuntimeLogAdapter 桥接**

- [x] 实现 `agentlog.Logger` 接口适配
- [x] Fatal 特殊处理（直写 stderr）
- [x] With 不可变模式
- [x] `agentlog.Default` 替换

**Task 20: SinkGroup 隔离**

- [x] 独立 goroutine + channel 缓冲
- [x] DropPolicy 策略
- [x] Panic 恢复
- [x] 优雅关闭

**验证**: `go test ./pkg/loggateway/... ./pkg/logpipeline/... -count=1`

---

## Bug 修复汇总

| Bug | 描述 | Phase | 修复方式 |
|-----|------|-------|---------|
| #1 | Pipeline 关闭后仍可 Emit | Phase 1 | `closed.Store(true)` 守卫 |
| #2 | 向已关闭 channel 写入 panic | Phase 1 | `select` + `ctx.Done()` |
| #4 | Sink.Write panic 影响 Pipeline | Phase 1 | SinkGroup panic 恢复 |
| #5 | 关闭时未排空 channel | Phase 1 | Close() 先关闭 channel 再等待 |
| #6 | busHook 同步阻塞 Pipeline | Phase 2 | EventBusSink 异步 + 超时 |
| #7 | EventBus 发布失败无反馈 | Phase 2 | 错误计数 + 熔断器 |
| #8 | stepThrottler buckets 无淘汰 | Phase 4 | TTL 淘汰机制 |
| #9 | 低优先级队列丢弃关键事件 | Phase 3 | criticalTypeSet BlockUpTo |
| #11 | 反馈环 | Phase 3 | logDrop() stderr 直写 |

---

## 验证清单

### 红线合规

- [x] `grep -r "log/slog" internal/` 为零（红线 #10）
- [x] `grep -r "zap\." internal/` 为零（红线 #10a，7 文件例外含 trpc-agent-go 运行时）
- [x] `Global()` 无新增调用

### 功能验证

- [x] `go test ./pkg/loggateway/... -count=1` 通过
- [x] `go test ./pkg/logpipeline/... -count=1` 通过
- [x] `go test ./internal/event/... -count=1` 通过
- [x] `go build ./cmd/admin` 通过

### 构造注入验证

- [x] Wire 编译通过
- [x] 所有 Usecase/Service 通过构造注入获取 Logger

---

## 已知偏差跟踪

| 编号 | 描述 | 状态 |
|------|------|------|
| R4-2/R5-1 | TraceEmitter 用 bus + boundInfraRef() | ✅ 已通过 FlowTracker 构造注入 Infra 解决 |
| R5-2 | stepThrottler buckets 无淘汰 | ✅ 已通过 TTL 淘汰机制解决 |
| F-2 | defaultOutputDir() 重复 | 待解决 |
| B-1 | Envelope.Clone() 浅拷贝 | 低风险，当前 subscriber 只读 |
| A-1 | Kratos 框架日志未接入 loggateway | 待解决 |
