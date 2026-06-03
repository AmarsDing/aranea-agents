## Why

当前日志框架（loggateway + logpipeline + TraceEmitter）在项目早期（~100 文件、~1000 次引用）设计合理，但随着项目规模增长，暴露出 **8 个**结构性隐患：

**原有 5 个（v2 提案已覆盖）**：
1. 双写路径导致日志重复和磁盘翻倍
2. Pipeline 单 worker 串行分发存在瓶颈
3. TraceEmitter 职责过重（5 个职责耦合在一个 struct）
4. 三种日志获取方式并存导致依赖不透明
5. stepThrottler 无淘汰机制存在内存泄漏风险

**架构审查新发现 3 个（v2 提案遗漏）**：
6. **EventBus-日志反馈环**：Bus.logDrop() → Gateway.Warn() → Pipeline → EventBusSink → Bus.Publish() 形成正反馈循环，高负载下可放大日志风暴
7. **trpc-agent-go 运行时日志完全隔离**：Agent 运行时 489 处日志调用走独立 zap.Sugar → stdout，不经过 Pipeline，对运维不可见，且与业务日志无法关联
8. **boundInfraRef() 全局可变状态**：TraceEmitter 绕过构造注入的 `bus Bus`，使用全局 `boundInfraRef()` 发布事件，破坏 DI 原则，启动时序竞态

## What Changes

- **消除双写路径**：Gateway 不再直接持有 zap.Logger 直写文件，改为统一通过 Pipeline 分发；FileSink 内部使用 zapcore.Core 写入，保持 JSON 格式兼容
- **SinkGroup 独立队列**：每个 Sink 拥有独立 goroutine + channel，慢 Sink 不再阻塞其他 Sink
- **TraceEmitter 拆分**：将"上帝对象"拆分为 FlowTracker（流程追踪）、SpanCollector（span 树管理）、UsageAggregator（用量统计）三个独立组件，各自持有独立 context 和 mutex
- **统一依赖获取**：构造注入为主，context 传递为辅，逐步淘汰 loggateway.Global() 全局单例
- **Throttler TTL 淘汰**：stepThrottler 的 buckets map 增加后台淘汰机制（含生命周期管理），防止长时间运行内存膨胀
- **配置驱动 Sink 注册**：Sink 注册从硬编码改为配置驱动（Proto enum 约束类型），新增 Sink 只需加配置 + 实现 Sink 接口
- **切断 EventBus-日志反馈环**：Bus.logDrop() 改用 stderr 直写 + droppedCount 原子计数器，不经过 Pipeline；EventBusSink 增加熔断机制（含半开状态多次探测）
- **桥接 trpc-agent-go 运行时日志**：在 `internal/adapter/` 层创建 RuntimeLogAdapter（loggateway → trpc-agent-go/log.Logger 适配器），在 main() 中替换 `log.Default`，将运行时日志纳入 Pipeline
- **消除 boundInfraRef() 全局引用**：将 Infra 作为 TraceEmitter/FlowTracker 的构造参数注入，替代全局可变状态

## Capabilities

### New Capabilities

- `sink-group`: SinkGroup 独立队列模型，每个 Sink 独立 goroutine + channel + 丢弃策略（DropPolicy 枚举），解决单 worker 串行瓶颈
- `trace-emitter-split`: TraceEmitter 拆分为 FlowTracker / SpanCollector / UsageAggregator 三个独立组件（各自独立 context + mutex），TraceEmitter 改为 embedding wrapper 保持兼容
- `throttler-ttl`: stepThrottler 增加基于 lastAccess 的 TTL 淘汰机制（含 goroutine 生命周期管理 Start/Stop），防止 buckets map 无界增长
- `eventbus-feedback-break`: 切断 EventBus-日志反馈环，Bus.logDrop 改用 stderr 直写 + droppedCount 计数器，EventBusSink 增加熔断机制（半开状态 3 次探测）
- `runtime-log-bridge`: 在 internal/adapter 层桥接 trpc-agent-go 运行时日志到 loggateway Pipeline，创建 RuntimeLogAdapter 适配器

### Modified Capabilities

- `logging-framework`: 消除双写路径（Gateway → Pipeline 单写），统一依赖获取方式（构造注入为主），配置驱动 Sink 注册（Proto enum），消除 boundInfraRef() 全局引用

## Impact

- **pkg/loggateway**: Gateway 核心改造（去掉 zap.Logger 直写，改为 Pipeline.Emit 单写）
- **pkg/logpipeline**: Pipeline 分发模型改造（单 worker → SinkGroup 独立队列），新增 Throttler TTL 淘汰（含生命周期管理），EventBusSink 增加熔断
- **internal/adapter**: 新增 RuntimeLogAdapter（桥接 trpc-agent-go 运行时日志）
- **internal/event**: TraceEmitter 拆分为三个独立组件（各自独立 context），flow_context.go 快捷函数适配，消除 boundInfraRef() 全局引用
- **internal/event/contract/bus.go**: Bus.logDrop() 改用 stderr 直写 + droppedCount 计数器
- **internal/service**: 构造注入 loggateway.Logger（替代部分 Global() 调用）
- **internal/biz**: 同上，构造注入改造
- **internal/data**: 同上，构造注入改造
- **cmd/admin/main.go**: Sink 注册从硬编码改为配置驱动，替换 trpc-agent-go log.Default
- **internal/conf/conf.proto**: 新增 LoggingSink message（SinkType enum + DropPolicy enum）
- **前端**: 无直接影响（日志格式保持兼容，WebSocket 推送协议不变）
