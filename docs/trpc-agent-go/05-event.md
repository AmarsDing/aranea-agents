# 事件系统（Event）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/event/`
> 项目实现路径：`internal/event/`、`internal/biz/event_bus_*.go`、`pkg/logpipeline/eventbus_sink.go`
> 当前对齐度：★★☆☆☆ → ★★★★★（P1-1/P1-2/P1-3 + P2-1/P2-2 已完成）

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `Agent` | `Run(ctx, *Invocation) (<-chan *event.Event, error)` | 事件流源头，返回只读 channel |
| `Flow` | `Run(ctx, *Invocation) (<-chan *event.Event, error)` | 内部流程接口，事件流编排 |
| `PluginManager` | `OnEvent(ctx, *Invocation, *Event) (*Event, error)` | 事件拦截/变换钩子 |
| `EventEmitter` | `Emit(*Event) error` | Graph 节点事件发射 |
| `EventEmitter` | `EmitCustom(string, any) error` | Graph 节点自定义事件 |
| `EventEmitter` | `EmitProgress(float64, string) error` | Graph 节点进度事件 |
| `EventEmitter` | `EmitText(string) error` | Graph 节点文本流事件 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `event.Event` | 核心事件值对象，嵌入 `*model.Response`，含 RequestID/InvocationID/Author/Branch/Tag/StateDelta/Extensions/FilterKey 等 |
| `model.Response` | LLM 响应结构，含 ID/Object/Choices/Usage/Error/Done/IsPartial |
| `event.EventActions` | 流级控制提示（SkipSummarization） |
| `event.EmitEventTimeoutError` | 事件发送超时错误 |
| `graph.EventMetadata` 系列 | 节点/工具/模型/Pregel/通道/状态/检查点等执行元数据 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| `Plugin.OnEvent` | 事件拦截/变换钩子，经 `Registry.OnEvent(hook)` 注册 | 事件变换、过滤、增强 |
| `event.Option` | 函数选项模式（WithBranch/WithResponse/WithTag/WithExtension 等） | 事件构造时配置 |
| `EventEmitter` 接口 | Graph 节点自定义事件发射 | Graph 节点内事件产出 |
| `Event.Extensions` | 命名空间化扩展元数据（`SetExtension`/`GetExtension[T]`） | 附加任意结构化数据 |
| `Event.Tag` | 分号分隔多标签（`WithTag()` 追加、`ContainsTag()` 查询） | 事件分类标记 |
| `Event.FilterKey` | `/` 分隔层级过滤键（`Filter()` 前缀匹配） | 多 Agent 事件分支过滤 |
| `Callback` | Agent/Model/Tool/Graph 节点级回调 | 事件产生前后的拦截处理 |

### 1.4 配置选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `event.WithBranch(branch)` | 设置 Agent 执行链分支 | `""` |
| `event.WithResponse(response)` | 设置 LLM 响应 | `nil` |
| `event.WithObject(o)` | 设置事件对象类型 | `""` |
| `event.WithStateDelta(sd)` | 设置状态变更 | `nil` |
| `event.WithStructuredOutputPayload(p)` | 设置结构化输出 | `nil` |
| `event.WithSkipSummarization()` | 标记跳过摘要 | `false` |
| `event.WithTag(tag)` | 追加业务标签 | `""` |
| `event.WithExtension(key, value)` | 添加扩展元数据 | `nil` |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| `event.New()` | `event/event.go` | 基础事件工厂（自动生成 ID/Timestamp） |
| `event.NewErrorEvent()` | `event/event.go` | 错误事件工厂 |
| `event.NewResponseEvent()` | `event/event.go` | 从 Response 构造事件 |
| `event.EmitEvent()` | `event/event.go` | 无超时发送事件到 channel |
| `event.EmitEventWithTimeout()` | `event/event.go` | 带超时发送事件到 channel |
| `eventEmitter` | `graph/emitter.go` | Graph 节点事件发射器默认实现 |
| `noopEmitter` | `graph/emitter.go` | 空发射器（eventChan 为 nil 时使用） |
| Graph 事件工厂 | `graph/events.go` | 20+ 工厂函数（NodeStart/Complete/Error/ToolExecution/ModelExecution/PregelStep 等） |
| Plugin Manager | `plugin/manager.go` | 事件钩子注册和执行 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `event.Event` 结构体 | 完全使用，消费 Runner 返回的事件流 | ✅ | 框架事件作为项目事件系统的上游源头 |
| `event.FilterKey` | 完全使用 | ✅ | 在 Envelope 中透传 FilterKey |
| `event.StateDelta` | 完全使用 | ✅ | 从框架事件流提取 StateDelta |
| `ev.IsRunnerCompletion()` | 完全使用 | ✅ | 用于判断 Runner 终止 |
| `<-chan *event.Event` 事件流消费 | 完全使用 | ✅ | `turnStreamConsumer.consume()` 消费 |
| `event.New*` 工厂函数 | 部分使用 | ⚠️ | 仅在框架内部交互时使用，项目自建 Envelope 体系 |
| `event.EmitEvent*` | 未使用 | ❌ | 项目使用自建 EventBus.Publish |
| `event.With*` Option | 部分使用 | ⚠️ | 仅消费框架事件时间接使用，项目自建事件不使用 |
| `Plugin.OnEvent` | 已启用 | ✅ | 项目通过 `eventTypeLabel` 细化实现 10 类事件分类，支持 hook 规则精确匹配 |
| `EventEmitter` | 未使用 | ❌ | 项目未使用框架 Graph EventEmitter |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| **EventBus（双总线）** | `internal/event/bus.go`、`internal/event/infra.go` | 框架无内置 | 框架仅提供单通道 `<-chan *event.Event`，不支持多消费者分发 | ✅ 已委托框架 bus.Bus[Envelope]
| **Envelope 事件信封** | `internal/event/contract/envelope.go` | 框架无内置 | 框架 `event.Event` 是 LLM 响应载体，项目需要通用事件信封（70+ 事件类型） |
| **SubscribeOptions（订阅配置）** | `internal/event/contract/bus.go` | 框架无内置 | 框架无订阅概念，项目需按 SessionID/TeamID/Channel/FilterKey/EventType 过滤 |
| **DropPolicy（投递策略）** | `internal/event/contract/bus.go` | 框架无内置 | 框架无背压处理，项目需 DropOldest/DropNewest/BlockUpTo |
| **ChannelPriority（优先级）** | `internal/event/contract/bus.go` | 框架无内置 | 框架无优先级概念，项目需 Critical/Normal 分级投递 |
| **EventWAL（WBPF 保护）** | `internal/event/wal.go` | 框架无内置 | 框架无持久化保护，Critical 事件需先写后发防丢失 | ✅ 已委托框架 wal.WAL[Envelope]
| **可靠性分级 AS-EVT-01** | `internal/event/contract/reliability.go` | 框架无内置 | 框架无可靠性分级，项目按 Critical/Important/Informational 分级 | ✅ 已委托框架 reliability.Classifier
| **Buffer（回放缓冲区）** | `internal/event/buffer.go` | 框架无内置 | WebSocket 重连时需回放历史事件 |
| **FlowTracker/TraceEmitter** | `internal/event/flow_tracker.go`、`trace_emitter.go` | 框架无内置 | 框架无流程追踪，项目需步骤计时/FlowLog/Span 追踪 | ✅ 纯数据层已委托框架 tracing
| **SpanCollector/UsageAggregator** | `internal/event/span_collector.go`、`usage_aggregator.go` | 框架无内置 | 框架无 Span/Usage 聚合，项目需 LLM/Tool Span 追踪和用量聚合 | ✅ 纯数据层已委托框架 tracing
| **EventBusConsumer（主消费者）** | `internal/biz/event_bus_consumer.go` | 框架无内置 | 框架无多消费者编排，项目需 buffer/persist/runnerCompletion/stateDelta 处理 |
| **EventBusSideConsumers（旁路消费者）** | `internal/biz/event_bus_side_consumers.go` | 框架无内置 | 框架无旁路消费，项目需 toolCall/callback/messageStore/flowLog 等独立消费 |
| **DomainEvent 适配层** | `internal/biz/domain_event.go`、`domain_event_adapter.go` | 框架无内置 | 将 Envelope 转换为 biz 层 DomainEvent，解耦事件传输与业务逻辑 |
| **EventBusSink（日志管线桥接）** | `pkg/logpipeline/eventbus_sink.go` | 框架无内置 | 将日志条目转为 Envelope 发布到 EventBus，含熔断器保护 |
| **频道路由** | `internal/event/contract/envelope.go` | 框架无内置 | 框架无频道概念，项目按事件类型路由到 monitor/team/graph/knowledge/chat 频道 |
| **SessionRevisionBumper** | `internal/event/session_revision.go` | 框架无内置 | Session 版本号递增 + 发布 run_status envelope |
| **EventProjector/ActivityProjector** | 项目自建 | 框架无内置 | 事件投影到 Activity-First 视图 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `Plugin.OnEvent` 钩子 | 项目使用自建 EventBus + DomainEvent 适配层实现事件分发和变换 | 评估中 |
| `EventEmitter`（Graph 节点） | 项目 Graph 执行通过 EventBridge 桥接到 EventBus，未直接使用框架 EventEmitter | 评估中 |
| `event.WithExtension` | 项目 Envelope 有独立 Extensions 字段（`map[string]string`），与框架 `map[string]json.RawMessage` 不同 | 否（类型不兼容） |
| Graph 事件工厂（20+） | 项目自建了 graph_node_start/end/error 等 EnvelopeType，未使用框架 `graph.NewNodeStartEvent` 等 | 评估中 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **Plugin.OnEvent 事件钩子**：标准化的拦截/变换机制，支持事件变换和过滤 | 项目无统一的事件拦截层，各消费者独立处理 | 统一事件变换逻辑，减少重复代码 |
| 2 | **Graph EventEmitter**：Graph 节点内标准化事件发射，含 panic recovery | 项目通过 EventBridge 桥接，增加适配层复杂度 | 简化 Graph 事件产出路径 |
| 3 | **event.Option 函数选项模式**：统一的事件构造 API | 项目 Envelope 使用直接字段赋值，无 Option 模式 | API 一致性，减少构造错误 |
| 4 | **Graph 事件工厂**：20+ 预定义事件工厂函数，类型安全 | 项目手动构造 graph 事件 Envelope | 减少构造代码，类型安全 |
| 5 | **Extension 泛型存取**：`GetExtension[T]` 类型安全的扩展元数据读取 | 项目 Envelope.Extensions 为 `map[string]string`，需手动反序列化 | 类型安全，减少序列化错误 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **EventBus（双总线 + 投递策略 + 优先级）** | 框架仅提供裸通道 `<-chan *event.Event`，单消费者 | 贡献回框架（event 扩展包） |
| 2 | **EventWAL（WBPF 保护）** | 框架无持久化保护，进程崩溃丢事件 | 贡献回框架（event 扩展包） |
| 3 | **可靠性分级 AS-EVT-01** | 框架无可靠性概念 | 贡献回框架（event 扩展包） |
| 4 | **Envelope 通用事件信封** | 框架 `event.Event` 面向 LLM 响应，非通用事件信封 | 贡献回框架（event 扩展包） |
| 5 | **Buffer 回放缓冲区** | 框架无事件回放能力 | 贡献回框架（event 扩展包） |
| 6 | **FlowTracker/SpanCollector** | 框架无流程追踪和 Span 聚合 | 贡献回框架（event 扩展包） |
| 7 | **频道路由** | 框架无频道概念 | 贡献回框架（event 扩展包） |
| 8 | **EventBusSink 熔断器** | 框架无熔断保护 | 贡献回框架（event 扩展包） |
| 9 | **多消费者编排** | 框架无消费者编排 | 贡献回框架（event 扩展包） |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| EventBus 自建 | **架构决策**：框架 `<-chan *event.Event` 是单消费者裸通道，项目需多消费者分发（WebSocket/持久化/监控/日志），框架不满足 | 整个事件系统架构 |
| Envelope 自建 | **架构决策**：框架 `event.Event` 面向 LLM 响应设计（嵌入 `*model.Response`），不适合作为通用事件信封（70+ 事件类型中大部分非 LLM 响应） | 所有事件类型定义和消费 |
| EventWAL 自建 | **功能缺失**：框架无 Critical 事件先写后发保护，生产环境必需 | Critical 事件可靠性 |
| 可靠性分级自建 | **架构决策**：AS-EVT-01 是项目架构标准，框架无此概念 | 事件投递策略和持久化 |
| FlowTracker 自建 | **功能缺失**：框架无流程追踪能力，项目需步骤计时和 FlowLog | 运行时可观测性 |
| Plugin.OnEvent 未使用 | **认知缺失**：项目在自建 EventBus 时未考虑框架 Plugin 系统的事件钩子能力 | 事件拦截和变换 |
| Graph EventEmitter 未使用 | **功能缺失**：项目 Graph 执行通过 EventBridge 桥接，未直接使用框架 EventEmitter | Graph 事件产出路径 |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 | 状态 |
|---|--------|------|--------|---------|---------|------|
| 1 | 启用框架 Plugin.OnEvent 事件钩子 | 启用框架功能 | P2 | 事件拦截/变换 | 统一事件变换逻辑，减少重复代码约 200 行 | ✅ 已完成（eventTypeLabel 细化） |
| 2 | Graph 事件使用框架 EventEmitter | 启用框架功能 | P3 | Graph 事件产出 | 简化 EventBridge 适配层，减少代码约 150 行 | ⏳ 待启动 |
| 3 | EventBus 贡献回框架 | 贡献回框架 | P1 | 整个事件系统 | 框架级多消费者支持，减少维护负担 | ✅ 已完成 |
| 4 | EventWAL 贡献回框架 | 贡献回框架 | P1 | Critical 事件可靠性 | 框架级 WBPF 保护 | ✅ 已完成 |
| 5 | 可靠性分级贡献回框架 | 贡献回框架 | P1 | 事件投递策略 | 框架级可靠性保证 | ✅ 已完成 |
| 6 | Envelope 适配框架 Event | 新增适配层 | P2 | 事件类型定义 | 统一事件模型，减少双轨维护 | ✅ 已完成（FromFrameworkEvent 统一转换） |
| 7 | FlowTracker/SpanCollector 贡献回框架 | 贡献回框架 | P2 | 运行时可观测性 | 框架级流程追踪 | ✅ 已完成（纯数据层 tracing 包） |

### 4.2 对齐项详情

#### 对齐项 #1：启用框架 Plugin.OnEvent 事件钩子 ✅ 已完成

**类型**：启用框架功能

**现状**：
- 项目当前实现：各消费者独立处理事件变换逻辑，无统一拦截层
- 框架提供能力：`PluginManager.OnEvent(ctx, invocation, e) (*Event, error)` — 标准化事件拦截/变换钩子，支持事件变换和过滤

**对齐方案**：
1. 在 Runner 初始化时注册框架 Plugin，将项目通用事件变换逻辑（如 Envelope 转换、频道路由）迁移到 `Plugin.OnEvent` 钩子
2. 保留 EventBus 作为下游分发层，Plugin.OnEvent 作为上游拦截层
3. 逐步将散落在各消费者中的变换逻辑收敛到 Plugin 钩子

**实施结果**：
- 分析发现 Envelope 转换发生在 Runner 事件循环之后（turnStreamConsumer），而非循环内部，因此不适合在 Plugin.OnEvent 中做重转换
- 实际可行的改进：将 `eventTypeLabel` 从 3 类（event/model_response/error）细化到 10 类（runner_completion/chat.completion.chunk/chat.completion/tool.response/error/agent.transfer/state.update/preprocessing/postprocessing/model_response），覆盖所有框架 `model.ObjectType`
- 细化后的分类使 hook 规则可以精确匹配特定事件类型（如仅拦截 tool.response 或 preprocessing 事件），而非只能匹配粗粒度的 "model_response"
- 变更文件：`internal/plugin/trpc/hook_events.go`

**兼容性风险**：
- Plugin.OnEvent 在 Runner 事件循环内同步执行，耗时操作会阻塞事件流
- 需确保钩子逻辑轻量，重操作仍走 EventBus 异步消费

**回退方案**：
- 保留现有消费者变换逻辑，Plugin.OnEvent 仅做日志/监控，不改变事件内容

**验证方法**：
- 单元测试：验证 Plugin 钩子注册后事件变换正确
- 集成测试：验证 WebSocket 推送的事件内容不变

**预期收益**：
- 代码减少：约 200 行（收敛重复变换逻辑）
- 维护成本：减少框架升级时的适配工作量

---

#### 对齐项 #2：Graph 事件使用框架 EventEmitter

**类型**：启用框架功能

**现状**：
- 项目当前实现：Graph 执行通过 `graphtrpc.EventBridge` 将框架 `<-chan *event.Event` 桥接到自建 EventBus
- 框架提供能力：`EventEmitter` 接口 + `graph.NewNodeStartEvent` 等 20+ 工厂函数

**对齐方案**：
1. 在 Graph 节点内直接使用框架 `EventEmitter` 发射事件
2. 保留 EventBridge 作为框架事件流到 EventBus 的桥接层（框架事件流仍是 Graph 执行的输出）
3. 使用框架 `graph.NewNodeStartEvent` 等工厂函数替代手动构造 Envelope

**代码变更范围**：
- 修改：`internal/adapter/graph_event_bridge.go`（使用框架工厂函数）
- 修改：Graph 节点自定义事件（使用 `EventEmitter.EmitCustom`）

**兼容性风险**：
- 框架 Graph 事件与项目 Envelope 类型不同，需保持桥接层
- 框架 Graph 事件元数据结构与项目 Envelope 字段映射需验证

**回退方案**：
- 保留 EventBridge 现有实现，仅在新节点中使用框架 EventEmitter

**验证方法**：
- 集成测试：验证 Graph 执行事件流与现有行为一致

**预期收益**：
- 代码减少：约 150 行（简化 EventBridge 适配层）
- 维护成本：框架 Graph 事件格式变更时自动适配

---

#### 对齐项 #3：EventBus 贡献回框架 ✅ 已完成

**类型**：贡献回框架

**现状**：
- 项目当前实现：自建双总线（Session + Monitor）+ 投递策略（DropOldest/DropNewest/BlockUpTo）+ 优先级（Critical/Normal）+ 订阅过滤（SessionID/TeamID/Channel/FilterKey/EventType）
- 框架提供能力：仅 `<-chan *event.Event` 裸通道，单消费者

**对齐方案**：
1. 将 `internal/event/bus.go` 的 Bus 接口和实现抽取为框架 `event/bus/` 扩展包
2. 接口设计保持项目现有 Bus 接口不变，实现代码迁移到框架
3. 项目通过 `import "trpc.group/trpc-go/trpc-agent-go/event/bus"` 使用框架实现
4. 分阶段迁移：先迁移 Bus 核心 → 再迁移 Infra 编排 → 最后迁移频道路由

**实施结果**：
- 框架侧：`pkg/trpc-agent-go/event/bus/bus.go` — 泛型 `Bus[T any]` 接口和实现，含 DropPolicy/ChannelPriority/SubscribeOptions/EventMatcher/DropLogger/DefaultBufferSize/MaxBufferSize 常量
- 项目侧：`internal/event/bus_adapter.go` — busAdapter 将框架 Bus[Envelope] 适配到 contract.Bus，含 loggateway DropLogger、SubscribeOptions 转换、Filter 组合
- 项目侧：`internal/event/bus.go` — NewBus 委托到 busAdapter，legacyBus 保留并标注 TECH-DEBT
- API 完全兼容：contract.Bus 接口不变，所有下游消费者无需修改

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/event/bus/`（框架扩展包）
- 修改：`internal/event/bus.go` → 委托到框架实现
- 删除：`internal/event/bus.go` 中自建实现代码（约 300 行）

**兼容性风险**：
- 框架可能不接受此贡献（需与框架维护者协商）
- Bus 接口设计需满足框架通用性要求，可能需调整

**回退方案**：
- 框架不接受贡献时，保持项目自建实现

**验证方法**：
- 单元测试：验证框架 Bus 实现与项目行为一致
- 压力测试：验证多消费者场景下投递策略正确

**预期收益**：
- 代码减少：约 300 行（Bus 核心实现迁移到框架）
- 维护成本：框架升级时 Bus 实现自动更新
- 功能增强：其他框架用户可使用 EventBus 能力

---

#### 对齐项 #4：EventWAL 贡献回框架 ✅ 已完成

**类型**：贡献回框架

**现状**：
- 项目当前实现：`EventWAL`（WBPF 保护），Critical 事件先写 SQLite 再发布，启动时重放未发布事件
- 框架提供能力：无持久化保护

**对齐方案**：
1. 将 `internal/event/wal.go` 抽取为框架 `event/wal/` 扩展包
2. 抽象存储后端接口（`Storage`），默认提供 SQLite 实现
3. 与 EventBus 贡献配合，WAL 作为 Bus 的可选中间层

**实施结果**：
- 框架侧：`pkg/trpc-agent-go/event/wal/wal.go` — 泛型 `WAL[T any]` 实现，含 Storage 接口（Insert/MarkPublished/ListUnpublished/PurgePublished/Close）、ExistChecker、IsCriticalFunc、SerializeFunc/DeserializeFunc、Logger 接口、WALOption 函数选项模式
- 框架侧：`pkg/trpc-agent-go/event/wal/memory_storage.go` — MemoryStorage 测试实现
- 项目侧：`internal/event/wal_storage.go` — sqliteWALStorage 适配 *sql.DB 到框架 wal.Storage，含 ctx 参数、Scan/Parse 错误日志
- 项目侧：`internal/event/wal.go` — EventWAL 委托到框架 WAL[Envelope]，walLogger 适配器透传 kv 参数，legacyEventWAL 保留并标注 TECH-DEBT
- API 完全兼容：EventWAL 公开接口不变

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/event/wal/`（框架扩展包）
- 修改：`internal/event/wal.go` → 委托到框架实现
- 删除：`internal/event/wal.go` 中自建实现代码（约 200 行）

**兼容性风险**：
- WAL 依赖 SQLite，框架可能需要抽象存储后端
- 重放逻辑与框架 Runner 启动流程需协调

**回退方案**：
- 框架不接受贡献时，保持项目自建实现

**验证方法**：
- 单元测试：验证 WBPF 行为（先写后发、崩溃恢复）
- 集成测试：验证 Runner 启动时 WAL 重放正确

**预期收益**：
- 代码减少：约 200 行
- 功能增强：框架级 Critical 事件可靠性保证

---

#### 对齐项 #5：可靠性分级贡献回框架 ✅ 已完成

**类型**：贡献回框架

**现状**：
- 项目当前实现：`ClassifyEventReliability(t EnvelopeType) EventReliability`，Critical/Important/Informational 三级分类
- 框架提供能力：无可靠性概念

**对齐方案**：
1. 将可靠性分级作为 `event/reliability/` 扩展包贡献回框架
2. 定义 `Tier` 类型和 `Classifier[T]` 泛型分级器
3. 与 EventWAL 配合，框架级提供基于可靠性分级的投递策略

**实施结果**：
- 框架侧：`pkg/trpc-agent-go/event/reliability/reliability.go` — 泛型 `Classifier[T comparable]` 分级器，含 Tier（Critical/Important/Informational）、RWMutex 并发安全、Register/RegisterBulk/Classify/IsRegistered/Tiers、RequiresBlockUpTo/IsCriticalWBPF 辅助函数
- 项目侧：`internal/event/contract/reliability.go` — 从自包含 switch 分级改为委托 `reliability.Classifier[EnvelopeType]`，EventReliability 成为 `reliability.Tier` 类型别名
- API 完全兼容：ClassifyEventReliability/IsCriticalWBPFType/RequiresBlockUpTo 函数签名不变

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/event/reliability/`（框架扩展包）
- 修改：`internal/event/contract/reliability.go` → 委托到框架实现

**兼容性风险**：
- 框架事件类型（`model.ObjectType*`）与项目 EnvelopeType 映射需维护
- 可靠性分级策略可能因项目而异，需支持自定义

**回退方案**：
- 框架不接受贡献时，保持项目自建实现

**验证方法**：
- 单元测试：验证分级分类正确性

**预期收益**：
- 代码减少：约 80 行
- 功能增强：框架级事件可靠性标准

---

#### 对齐项 #6：Envelope 适配框架 Event ✅ 已完成

**类型**：新增适配层

**现状**：
- 项目当前实现：自建 `Envelope` 体系（70+ 事件类型），与框架 `event.Event` 并行存在
- 框架提供能力：`event.Event`（面向 LLM 响应）+ `graph.New*Event` 工厂（面向 Graph 事件）

**对齐方案**：
1. 短期：保持 Envelope 体系不变，在框架事件到 Envelope 的转换层（`WrapFrameworkEvents`）中增强映射
2. 中期：评估将项目特有事件类型（如 Spirit/Butler/Monitor/Skill 等）注册为框架 `event.Event` 的 Extension，减少双轨维护
3. 长期：当框架接受 EventBus 贡献后，Envelope 可作为 Bus 的标准信封类型

**实施结果**：
- 新增 `FromFrameworkEvent(ev, meta, typ)` 统一转换函数，作为 framework `*event.Event` → project `Envelope` 字段映射的单源真相
- 新增 `FrameworkEventMeta` 结构体，携带 turn-scoped 元数据（SessionID/RequestID/InvocationID/ParentInvocationID/TeamID/Branch/FilterKey/Source），解耦转换函数与 turn-scoped 状态
- EventProjector 的 `baseEnvelope` 和 Graph EventBridge 的 `convertEvent` 均改为调用 `FromFrameworkEvent`，消除了两处重复的手动字段提取逻辑
- Extensions 转换：框架 `map[string]json.RawMessage` → 项目 `map[string]string`，含 JSON string 引号剥离逻辑
- Actions 转换：框架 `EventActions.SkipSummarization` → 项目 `EnvelopeActions.SkipSummarization`
- 变更文件：`internal/event/framework_adapter.go`（新增）、`internal/event/framework_adapter_test.go`（新增 9 个测试）、`internal/agent/event_projector.go`、`internal/graph/trpc/event_bridge.go`

**兼容性风险**：
- Envelope 与 Event 结构差异较大（Envelope 面向通用事件，Event 面向 LLM 响应），强行统一可能引入复杂性
- 70+ 事件类型的迁移影响面广

**回退方案**：
- 保持双轨体系，仅优化转换层

**验证方法**：
- 单元测试：验证框架事件到 Envelope 的映射完整性
- 集成测试：验证所有事件类型的端到端传递正确

**预期收益**：
- 代码减少：约 100 行（优化转换层，减少手动映射）
- 维护成本：框架事件格式变更时自动适配

---

#### 对齐项 #7：FlowTracker/SpanCollector 贡献回框架 ✅ 已完成（纯数据层）

**类型**：贡献回框架

**现状**：
- 项目当前实现：FlowTracker（步骤计时/FlowLog/Span 追踪）+ SpanCollector（LLM/Tool Span 管理）+ UsageAggregator（用量聚合）
- 框架提供能力：`ExecutionTrace` 字段（`*trace.Trace`，不序列化），无流程追踪和 Span 聚合

**对齐方案**：
1. 将 FlowTracker 抽取为框架 `event/tracing/` 扩展包
2. 与框架 `ExecutionTrace` 集成，提供标准化的流程追踪能力
3. SpanCollector 作为 FlowTracker 的子模块贡献

**实施结果**：
- 采用最小可行贡献策略：仅提取纯数据层（FlowContext/SpanContext/UsageContext）到 `pkg/trpc-agent-go/event/tracing/`，零外部依赖
- FlowContext：步骤级计时（RecordStart/TakeTiming），sync.Mutex 并发安全
- SpanContext：Span 树管理（StartSpan/EndSpan/FinishRoot/OpenToolSpan/TakeToolSpan/HasToolSpan/SetOpenLLMSpan/MergeLLMSpanTokens/RootID/Spans/IterateSpans），sync.Mutex 并发安全
- UsageContext：OTel 关联（SetOtelRefs/OtelTraceID/OtelRootID）+ turn 计时（TurnStart），sync.Mutex 并发安全
- 项目侧通过 type alias 委托：`FlowContext = frameworktracing.FlowContext`、`SpanContext = frameworktracing.SpanContext`、`UsageContext = frameworktracing.UsageContext`
- FlowTracker/SpanCollector/UsageAggregator 暂未贡献（含 loggateway/Envelope/Bus 等重依赖），后续迭代评估
- 项目 `FlowTiming` 保留 `StartedAt` 字段（框架仅 `DurationMS`），`flow_tracker.emit()` 内部做 framework → project 转换
- 变更文件：`pkg/trpc-agent-go/event/tracing/tracing.go`（新增）、`pkg/trpc-agent-go/event/tracing/tracing_test.go`（新增 13 个测试）、`internal/event/flow_context_state.go`、`internal/event/span_context.go`、`internal/event/usage_context.go`（删除，合并到 span_context.go）、`internal/event/flow_tracker.go`

**兼容性风险**：
- FlowTracker 依赖项目 loggateway，需抽象日志接口
- Span 模型与框架 `trace.Trace` 的映射需设计

**回退方案**：
- 框架不接受贡献时，保持项目自建实现

**验证方法**：
- 单元测试：验证 FlowTracker 步骤计时和 FlowLog 生成正确
- 集成测试：验证 Span 追踪与 OTel 集成正确

**预期收益**：
- 代码减少：约 400 行（FlowTracker + SpanCollector + UsageAggregator）
- 功能增强：框架级流程追踪能力

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #3 EventBus 贡献、#4 EventWAL 贡献、#5 可靠性分级贡献 | 与框架维护者协商贡献方案 | 中 |
| Phase 2 | #1 启用 Plugin.OnEvent、#6 Envelope 适配 | Phase 1（EventBus 贡献后接口稳定） | 中 |
| Phase 3 | #2 Graph EventEmitter、#7 FlowTracker 贡献 | Phase 2（事件体系稳定后） | 中 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架不接受 EventBus/WAL 贡献 | 中 | 高 | 保持项目自建实现，仅对齐接口命名和模式 |
| Plugin.OnEvent 阻塞事件流 | 低 | 高 | 钩子逻辑仅做轻量变换，重操作走 EventBus 异步消费 |
| Envelope 与 Event 双轨维护成本 | 高 | 中 | 优化转换层，长期目标统一事件模型 |
| Graph EventEmitter 与 EventBridge 冲突 | 低 | 中 | 保留 EventBridge 作为桥接层，新节点使用框架 EventEmitter |
| 贡献回框架的代码需满足框架通用性 | 中 | 中 | 抽象存储后端和日志接口，提供默认实现 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| Runner 事件流消费 | `examples/runner/main.go` | `Runner.Run()` → `<-chan *event.Event`，`for range` 消费，`IsFinalResponse()` 终止 | `runner.NewRunner(appName, agent, opts...)` | 项目通过 `turnStreamConsumer.consume()` 消费框架事件流后转为 Envelope 发布到 EventBus，非直接消费 |
| Runner 事件处理 | `examples/runner/main.go` | `handleEvent(evt)` 按错误/ToolCall/ToolResponse/内容分类处理 | 直接在消费循环中处理 | 项目将事件转换为 Envelope 后由 EventBus 分发给多个消费者异步处理 |

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Event 概述（中文） | `docs/mkdocs/zh/event.md` |
| Event 概述（英文） | `docs/mkdocs/en/event.md` |
| Callbacks（中文） | `docs/mkdocs/zh/callbacks.md` |
| AG-UI 事件翻译 | `docs/mkdocs/zh/agui/index.md` |
| AG-UI Chat 事件流 | `docs/mkdocs/zh/agui/chat.md` |
| 错误处理 | `docs/mkdocs/zh/error-handling.md` |
| Runner | `docs/mkdocs/zh/runner.md` |
| Graph | `docs/mkdocs/zh/graph.md` |
