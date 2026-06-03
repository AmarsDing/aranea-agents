## Non-Goals

- 不改变日志输出格式（JSON 格式保持兼容）
- 不改变前端 WebSocket 推送协议
- 不引入新的日志库依赖
- 不做 OTel 集成扩展
- 不改变 FlowLogEntry 数据模型
- 不做日志采样策略变更
- 不改变 EnvelopeType 封闭枚举模式
- 不统一 Kratos log.Logger
- 不完全消除 Infra 内部的全局引用（后续独立变更）
- 不迁移测试代码中的 Global() 调用（61 处，测试中合理）

## 1. Phase 1: 切断反馈环 + 消除双写（最高优先级）

- [x] 1.1 Bus.logDrop() 改用 stderr 直写 + droppedCount 计数器：将 `internal/event/bus.go` 中 logDrop() 的 `b.lg.Warn(...)` 替换为 `fmt.Fprintf(os.Stderr, ...)`，同时增加 `droppedCount atomic.Uint64` 计数器。DoD: logDrop 不调用 loggateway，droppedCount 在 Stats 中可见
- [x] 1.2 EventBusSink 增加熔断机制：在 EventBusSink 中增加 failures/openUntil/halfOpenAttempts 原子字段，连续 5 次超时后暂停 10 秒，半开状态 3 次探测。DoD: 熔断单元测试通过（模拟连续超时 + 半开探测）
- [x] 1.3 FileSink 内部改用 zapcore.Core：将 FileSink 的 Write 方法从 json.Marshal 改为 zapcore.Core + JSONEncoder 写入，保持 JSON 格式兼容。**此任务必须先于 1.4 完成**。DoD: FileSink 单元测试通过，输出格式与当前一致
- [x] 1.4 Gateway 去掉 zap.Logger 直写：删除 Gateway 中的 `logger *zap.Logger` 字段和 `g.logger.Info/Debug/Warn/Error` 调用，所有日志统一走 Pipeline.Emit。**依赖 1.3 先完成**。DoD: Gateway 单元测试通过，每条日志只写一份
- [x] 1.5 Gateway 构造时注入 Pipeline：将 `loggateway.New(bc.Logging)` 改为 `loggateway.New(bc.Logging, pipeline)`，删除 `gw.SetPipeline(pipeline)` 后置调用。DoD: 构造时 Pipeline 已就绪，无 nil 丢弃
- [x] 1.6 loggerWith 适配：loggerWith 的 Debug/Info/Warn/Error 方法去掉 `l.g.logger.*` 调用，改为统一走 `l.g.emitToPipeline`。DoD: loggerWith 单元测试通过
- [x] 1.7 删除 Gateway 中多余的 zap 相关字段：清理 `core zapcore.Core` 字段（如不再需要）。DoD: 编译通过，无未使用字段
- [x] 1.8 验证：运行 `make build && make test`，确认日志文件只有一份输出，格式不变，反馈环切断

## 2. Phase 2: SinkGroup 独立队列

- [x] 2.1 定义 DropPolicy 枚举和 SinkGroup 结构体：在 `pkg/logpipeline/sink_group.go` 中创建 `type DropPolicy int` 枚举（DropNewest/DropBlock）和 SinkGroup struct（sink Sink, ch chan LogEntry, wg sync.WaitGroup, dropped atomic.Uint64, dropPolicy DropPolicy）。DoD: 编译通过
- [x] 2.2 实现 SinkGroup.Run 方法：独立 goroutine 从 channel 读取 LogEntry 并调用 Sink.Write，包含 panic recover。DoD: SinkGroup 单元测试通过（panic recover 场景）
- [x] 2.3 实现 SinkGroup.Emit 方法：`Emit(entry LogEntry) error`，非阻塞返回，根据 dropPolicy 决定 DropNewest 或 Block。DoD: Emit 单元测试通过（满 buffer 场景）
- [x] 2.4 Pipeline 改造：将 `sinks []Sink` 替换为 `sinkGroups []*SinkGroup`，dispatchLoop 改为路由到各 SinkGroup channel。DoD: Pipeline 单元测试通过
- [x] 2.5 Pipeline.Close 适配：等待所有 SinkGroup goroutine 排空 channel 后再关闭。DoD: 优雅关闭测试通过
- [x] 2.6 Pipeline.Stats 扩展：返回包含 per-Sink dropped/chanLen/chanCap 的聚合统计，以及 Bus.droppedCount。DoD: Stats 单元测试通过
- [x] 2.7 验证：运行 `make build && make test`，模拟慢 Sink 场景确认其他 Sink 不受影响

## 3. Phase 3: TraceEmitter 拆分 + 消除 boundInfraRef + Throttler TTL

- [x] 3.1 拆分 TraceContext 为 FlowContext + SpanContext + UsageContext：将当前 TraceContext 拆分为三个独立 struct，各自持有独立 mutex。DoD: 三个 context struct 编译通过，各自独立 mutex
- [x] 3.2 抽取 SpanCollector：从 TraceEmitter 中提取 startSpan/endSpan/endSpanWithDuration/FinishRoot 逻辑到 `internal/event/span_collector.go`，使用 SpanContext。DoD: SpanCollector 单元测试通过
- [x] 3.3 抽取 UsageAggregator：从 TraceEmitter 中提取 ObserveFrameworkEvent/mergeLLMSpan/MetadataJSON/SyncOtelSpanIDs 逻辑到 `internal/event/usage_aggregator.go`，使用 UsageContext。DoD: UsageAggregator 单元测试通过
- [x] 3.4 创建 FlowTracker：在 `internal/event/flow_tracker.go` 中实现 FlowTracker，持有 FlowContext + 可选 SpanCollector + 可选 UsageAggregator，保持 LogStart/LogDone/LogError 等方法签名。shouldPublishFlowChatError 归属 FlowTracker。DoD: FlowTracker 单元测试通过
- [x] 3.5 TraceEmitter 改为 embedding wrapper：`type TraceEmitter struct { *FlowTracker }`，保持所有现有调用点兼容。**不是 type alias**。DoD: 编译通过，现有测试不中断
- [x] 3.6 FlowTracker 构造注入 Infra：FlowTracker 的构造参数从 `bus Bus` 改为 `infra Infra`，emit() 始终使用注入的 Infra.Publish()，不再调用 boundInfraRef()。DoD: FlowTracker 不引用 boundInfraRef()
- [x] 3.7 flow_context.go 适配：WithTraceEmitter/TraceEmitterFromContext 改为操作 FlowTracker（兼容 TraceEmitter embedding），消除 flow_context.go:85 的 loggateway.Global() 调用。DoD: 编译通过，无 Global() 调用
- [x] 3.8 boundInfraRef() 和 BindInfra() 标记 deprecated：添加 deprecated 注释，FlowTracker 不再使用。DoD: 注释存在，FlowTracker 无引用
- [x] 3.9 Throttler TTL：在 stepThrottler 的 bucket 中增加 lastAccess atomic.Int64，shouldThrottle 中更新。DoD: lastAccess 更新单元测试通过
- [x] 3.10 Throttler 后台淘汰 goroutine（含生命周期管理）：增加 Start/Stop 方法，每 5 分钟扫描 buckets，淘汰 lastAccess > 30min 的条目。Pipeline.Close() 调用 Stop()。DoD: 淘汰单元测试通过（模拟时间推进），Start/Stop 生命周期测试通过
- [x] 3.11 Throttler 配置化：ThrottleConfig 增加 TTL 和 ScanInterval 字段。DoD: 配置单元测试通过
- [x] 3.12 Wire 注入图更新：FlowTracker 构造参数变更（bus → infra）后更新 wire.go 和 wire_gen.go。DoD: `make wire && make build` 通过
- [x] 3.13 验证：运行 `make build && make test`，确认流程追踪功能不变，无全局可变状态，内存不再无界增长

## 4. Phase 4: 桥接运行时日志

- [x] 4.1 创建 RuntimeLogAdapter：在 `internal/adapter/runtime_log.go` 中实现 `trpc.group/trpc-go/trpc-agent-go/log.Logger` 接口，委托给 loggateway.Logger。**不在 pkg/loggateway/ 中**。DoD: 编译通过，接口实现完整
- [x] 4.2 RuntimeLogAdapter Fatal 特殊处理：Fatal/Fatalf 同步写 stderr + 独立 zap.Logger 后调用 os.Exit(1)，不走异步 Pipeline。独立 zap.Logger 仅用于 Fatal 级别。DoD: Fatal 单元测试通过
- [x] 4.3 RuntimeLogAdapter With() 支持：`With(fields ...loggateway.Field) *RuntimeLogAdapter` 返回新实例（不可变模式），支持预设上下文字段。DoD: With() 单元测试通过
- [x] 4.4 main.go 中替换 log.Default 和 log.ContextDefault：在 loggateway.New() 后，将 trpc-agent-go 的 log.Default 替换为 RuntimeLogAdapter 实例。DoD: 运行时日志出现在 Pipeline 输出中
- [x] 4.5 验证：运行 `make build && make test`，启动应用后确认 Agent 运行时日志出现在 aranea-pipeline.log 中

## 5. Phase 5: 配置驱动 + 去全局单例

- [x] 5.1 conf.proto 新增 SinkType/DropPolicy enum + LoggingSink message：定义 SinkType（FILE/STDOUT/EVENTBUS）和 DropPolicy（NEWEST/BLOCK）枚举，LoggingSink message 使用枚举类型。DoD: `make api` 编译通过
- [x] 5.2 实现 Sink 工厂函数：在 `pkg/logpipeline/sink_factory.go` 中实现 NewSinkFromConfig(cfg SinkConfig, deps SinkFactoryDeps) (Sink, error)，根据 Type 字段创建对应 Sink。DoD: 工厂函数单元测试通过
- [x] 5.3 Pipeline 初始化改为配置驱动：cmd/admin/main.go 中根据 conf.Logging.Sinks 动态创建 SinkGroup 并注册。DoD: 启动测试通过，日志正常输出
- [x] 5.4 loggateway.Global() 标记 deprecated：添加 `// Deprecated: use constructor injection` 注释。DoD: 编译通过，注释存在
- [x] 5.5 迁移 internal/event/flow_context.go Global() 调用：改为通过依赖注入传递 logger（如 Phase 3.7 已完成则跳过）。DoD: flow_context.go 无 Global() 调用
- [x] 5.6 Wire 注入图更新：更新所有 wire.go 和 wire_gen.go 确保注入图完整。DoD: `make wire && make build` 通过
- [x] 5.7 验证：运行 `make api && make wire && make build && make test && make lint`，全量通过

## 6. 文档同步

- [x] 6.1 更新 openspec/specs/logging-framework.md：反映 v2 架构变更。DoD: 文档内容与代码一致
- [x] 6.2 更新 openspec/specs/module-cross-reference.md：更新日志模块卡片（新增 FlowTracker/SpanCollector/UsageAggregator/RuntimeLogAdapter/FlowContext/SpanContext/UsageContext）。DoD: 模块卡片完整
- [x] 6.3 更新 openspec/specs/architecture-blueprint.md：更新日志相关章节。DoD: 蓝图与实际一致
- [x] 6.4 更新 openspec/specs/backend-layers.md：更新红线 #10 描述（增加 Global() deprecated 说明）。DoD: 红线描述准确

## 7. Phase 7: 遗留修复 + 文档补全

- [x] 7.1 pkg/auth/ 消除 loggateway.Global() 调用：将 loggateway.Logger 注入到 auth 中间件（grpc_middleware.go/middleware.go/features.go），替换 10 处 Global() 调用。DoD: pkg/auth/ 无 Global() 调用，编译通过
- [ ] 7.2 创建 module-cross-reference-full.md：AGENTS.md 引用的文件不存在，基于 module-cross-reference.md 扩展创建。DoD: 文件存在，包含新模块卡片
- [ ] 7.3 更新 project_rules.md：添加日志红线 #10/#16 引用和 loggateway 使用规范。DoD: 项目规则包含日志架构约束
- [ ] 7.4 更新 AGENTS.md：添加日志架构条目（loggateway 红线、Global() deprecated）。DoD: AI 入口文件提及日志约束
- [ ] 7.5 统一红线编号：将 backend-layers.md 中的 #10 更新为与 SKILL 一致的编号。DoD: 编号一致
- [ ] 7.6 更新 logging-framework.md：添加 SinkGroup 章节、TraceEmitter 拆分设计说明。DoD: 文档反映 v2 架构
- [ ] 7.7 更新 module-cross-reference.md：添加 FlowTracker/SinkGroup/RuntimeLogAdapter/SpanCollector/UsageAggregator 模块卡片。DoD: 新模块有卡片
- [ ] 7.8 更新 architecture-blueprint.md：添加 FlowTracker/SinkGroup/RuntimeLogAdapter 架构描述。DoD: 蓝图包含新组件
- [ ] 7.9 验证：全量编译 + 测试 + 审查通过
