# Logging Framework 需求文档

> Aranea-Agents 日志框架需求规格。聚焦用户故事、功能需求、验收标准与非功能需求。
>
> 架构设计、接口契约、Proto 定义、数据模型见 [64-logging-framework.design.md](./64-logging-framework.design.md)；
> 代码锚点、实施进度、任务清单、已知偏差见 [64-logging-framework.development.md](./64-logging-framework.development.md)。

---

## 1. 概述

Aranea-Agents 日志框架为开发者、运维人员、终端用户提供统一的结构化日志与流程追踪能力。系统采用双轨制：通用结构化日志（loggateway）记录"发生了什么"，流程追踪（FlowTracker/TraceEmitter）记录"进行到哪了"，两者底层共享异步分发管道但语义独立。

### 1.1 用户角色

| 角色 | 关注点 |
|------|--------|
| **开发者** | 统一日志 API、结构化字段、错误链展开、构造注入 |
| **运维人员** | 日志落盘轮转、级别动态调整、监控指标、熔断保护 |
| **终端用户** | 监控页实时查看 Flow Log、进程日志、执行进度 |
| **插件/Hook 作者** | 安全日志双写（loggateway + EventBus） |

---

## 2. 用户故事

### US-1：统一日志 API（开发者）

**作为**开发者，**我希望**全项目使用统一的 `loggateway.Logger` API，**以便**我不需要在 `log/slog`、`zap`、`kratos log.Helper` 之间选择，且所有日志走同一条管道。

**验收**：
- `internal/` 下 `log/slog` 引用为零（红线 #16）
- `internal/` 下 `zap.` 直接引用为零（trpc-agent-go 运行时除外）
- 所有 Usecase/Service 通过构造注入获取 `loggateway.Logger`

### US-2：结构化字段与上下文关联（开发者）

**作为**开发者，**我希望**通过 `With()` 预设上下文字段（session_id/step_id/trace_id/run_id 等），**以便**日志可跨服务/跨轮次关联，且字段不可变避免并发问题。

**验收**：
- `With()` 返回新实例，原始 Logger 不被修改
- 字段累积：`child.base = parent.base + newFields`
- 预定义字段构造函数覆盖 StepID/SessionID/TraceID/RunID/Domain/AgentKey/Phase/Duration/Source/Err/Str/Int/Int64/Float64/Bool/Any

### US-3：错误链展开（开发者）

**作为**开发者，**我希望**记录错误时自动展开错误链，**以便**多层包装的错误能保留完整上下文。

**验收**：
- 单层错误：输出 `error` 字段
- 多层错误：输出 `error_chain` 数组，按 unwrap 顺序保留每层错误消息

### US-4：日志落盘轮转（运维人员）

**作为**运维人员，**我希望**日志自动落盘并按大小/数量/天数轮转，**以便**磁盘不被撑爆且历史日志可追溯。

**验收**：
- FileSink 使用 lumberjack 轮转
- 可配置：单文件最大 MB、保留旧文件数、最大保留天数、是否压缩
- 默认输出目录：Linux `/var/log/aranea`，Windows `./logs`，可被 `MONITOR_FLOW_LOG_DIR` 环境变量覆盖

### US-5：实时 Flow Log 流（终端用户）

**作为**终端用户，**我希望**在监控页实时查看 Agent 执行流程的步骤日志，**以便**了解长任务的执行进度与状态。

**验收**：
- FlowLog 通过 WebSocket 实时推送到前端
- 前端组件：FlowLogStream.vue、FlowTracePanel.vue、FlowLogExportButton.vue
- WS 重连后可从环形缓冲回放历史事件
- FlowLog 持久化到数据库，支持历史查询

### US-6：进程日志流（终端用户）

**作为**终端用户，**我希望**在监控页查看系统进程日志，**以便**排查运行时问题。

**验收**：
- 进程日志（EnvelopeTypeLog）通过 WebSocket 推送
- 前端组件：ProcessLogStream.vue
- 受 `logEnabled` 开关限制

### US-7：运行时日志桥接（开发者/运维人员）

**作为**开发者，**我希望** trpc-agent-go 运行时日志自动桥接到 loggateway Pipeline，**以便** Agent 生命周期日志与业务日志统一落盘，不再仅输出到 stdout。

**验收**：
- `agentlog.Default` / `agentlog.ContextDefault` 被替换为 RuntimeLogAdapter
- Debug/Info/Warn/Error 走 loggateway Pipeline
- Fatal/Fatalf 直写 stderr + os.Exit(1)（绕过异步 Pipeline 保证落盘）

### US-8：动态日志级别（运维人员）

**作为**运维人员，**我希望**运行时动态调整日志级别，**以便**排查问题时临时提高详细度，无需重启。

**验收**：
- Gateway 集成 `zap.AtomicLevel`
- `SetLevel()` 运行时调整

### US-9：高频步骤限流（运维人员）

**作为**运维人员，**我希望**对高频 step_id 限流，**以便**防止高频日志淹没 Pipeline。

**验收**：
- 基于 step_id 前缀匹配的令牌桶限流（最长前缀匹配）
- 限流日志计入 `Throttled()` 计数，不进入 Sink
- 空闲桶 TTL 淘汰，避免内存无界增长

### US-10：EventBus Sink 熔断保护（运维人员）

**作为**运维人员，**我希望** EventBus Sink 在下游故障时熔断，**以便**不阻塞 Pipeline 且快速失败。

**验收**：
- 三态熔断器：Closed → Open（5 次连续失败）→ HalfOpen（10 秒后）→ Closed（3 次探测成功）
- 熔断期间跳过写入并计数
- Publish 超时 50ms

### US-11：配置驱动 Sink 注册（运维人员）

**作为**运维人员，**我希望**通过配置文件声明式注册 Sink，**以便**不修改代码即可调整日志输出目标。

**验收**：
- Proto 定义 SinkType（file/stdout/eventbus）+ DropPolicy（newest/block）
- 每个 Sink 可配置 buffer_size、drop_policy、类型特定参数
- EventBus Sink 延迟到 BeforeStart 注册（依赖 eventInfra）

### US-12：插件安全日志（插件/Hook 作者）

**作为**插件作者，**我希望**通过 PluginSafeLogger 安全记录日志，**以便**插件日志同时进入 loggateway Pipeline 和 EventBus，且插件异常不影响主流程。

**验收**：
- PluginSafeLogger 双写：loggateway（同步）+ EventBus（异步 safego.Go）
- 支持 Info/Warn/Error/Debug 级别

### US-13：反馈环切断（运维人员）

**作为**运维人员，**我希望**日志系统自身的问题不产生反馈环，**以便**日志丢弃/熔断不会触发更多日志导致雪崩。

**验收**：
- 日志丢弃通知不经过 Pipeline/EventBus（走 loggateway.Warn + Prometheus 指标）
- 熔断状态转换不经过 Pipeline/EventBus（走 stderr）

---

## 3. 功能需求清单

| 编号 | 需求 | 优先级 |
|------|------|--------|
| FR-1 | 统一日志 API（loggateway.Logger：Debug/Info/Warn/Error/With） | P0 |
| FR-2 | 结构化字段构造函数（StepID/SessionID/TraceID/RunID/Domain/AgentKey/Phase/Duration/Source/Err/Str/Int/Int64/Float64/Bool/Any） | P0 |
| FR-3 | 错误链展开（单层 error / 多层 error_chain） | P0 |
| FR-4 | With() 不可变语义（返回新实例，base 切片复制） | P0 |
| FR-5 | nil Gateway 安全（无 nil 检查） | P0 |
| FR-6 | 异步分发管道（Pipeline：Emit/AddSink/Close/Dropped/Throttled/Stats） | P0 |
| FR-7 | Sink 隔离（SinkGroup：独立 goroutine + channel + DropPolicy） | P0 |
| FR-8 | FileSink（lumberjack JSON 轮转） | P0 |
| FR-9 | StdoutSink（stdout JSON，可配级别） | P0 |
| FR-10 | EventBusSink（Pipeline → EventBus，含三态熔断器） | P0 |
| FR-11 | RuntimeLogAdapter（trpc-agent-go agentlog.Logger → loggateway Pipeline） | P0 |
| FR-12 | PluginSafeLogger（插件日志双写） | P1 |
| FR-13 | FlowTracker 流程追踪（LogStart/LogDone/LogSkip/LogWarn/LogError/LogCritical） | P0 |
| FR-14 | TraceEmitter embedding wrapper（嵌入 FlowTracker + ObserveFrameworkEvent） | P0 |
| FR-15 | FlowLogEntry 数据模型（schema flow_log/v1） | P0 |
| FR-16 | step_id 标题注册表（人类可读中文标题） | P1 |
| FR-17 | 动态日志级别（AtomicLevel + SetLevel） | P1 |
| FR-18 | step_id 前缀限流（令牌桶 + TTL 淘汰） | P1 |
| FR-19 | 配置驱动 Sink 注册（SinkConfig + SinkFactoryDeps 工厂） | P1 |
| FR-20 | Buffer 环形回放（WS 重连历史事件） | P1 |
| FR-21 | Infra 双总线路由（SessionBus + MonitorBus，split/dual 模式） | P1 |
| FR-22 | Prometheus 监控指标（published_total / dropped_total） | P2 |
| FR-23 | Global() deprecated（构造注入替代全局单例） | P1 |

---

## 4. 非功能需求

### 4.1 合规性

| 编号 | 需求 | 验证 |
|------|------|------|
| NFR-C1 | 红线 #16：禁止 `log/slog`，统一 `pkg/loggateway.Logger` | `grep -r "log/slog" internal/` 为零 |
| NFR-C2 | 红线 #10a：禁止直接使用 `zap` 全局 logger | `grep -r "zap\." internal/` 为零（trpc-agent-go 运行时除外） |
| NFR-C3 | 红线 #10b：禁止 `fmt.Print*` 用于日志输出 | 代码审查 |
| NFR-C4 | `Global()` deprecated，新代码构造注入 | 代码审查无新增 Global() 调用 |

### 4.2 性能

| 编号 | 需求 | 说明 |
|------|------|------|
| NFR-P1 | Emit 非阻塞 | channel + select/default，满则丢弃 |
| NFR-P2 | Sink 隔离 | 慢 Sink 不阻塞其他 SinkGroup |
| NFR-P3 | 读写分离 | stepThrottler RWMutex，淘汰不阻塞热路径 |
| NFR-P4 | 无锁熔断器 | 全 atomic 操作，无互斥锁竞争 |

### 4.3 可靠性

| 编号 | 需求 | 说明 |
|------|------|------|
| NFR-R1 | Panic 隔离 | dispatchLoop 和 SinkGroup.run() 均 recover |
| NFR-R2 | 优雅关闭 | Close() 排空 channel → 等待 goroutine → 关闭 Sink |
| NFR-R3 | 反馈环切断 | 日志系统自身问题不走 Pipeline/EventBus |
| NFR-R4 | 内存有界 | stepThrottler 桶 TTL 淘汰；Buffer 分区 TTL 清理 |

### 4.4 可观测性

| 编号 | 需求 | 说明 |
|------|------|------|
| NFR-O1 | Pipeline 指标 | Dropped/Throttled/ChanLen/ChanCap/SinkCount/SinkErrors |
| NFR-O2 | SinkGroup 指标 | Name/Dropped/ChanLen/ChanCap |
| NFR-O3 | 熔断器指标 | open/skipped/half_open_attempts |
| NFR-O4 | Prometheus 指标 | aranea_event_bus_published_total / aranea_event_bus_dropped_total |

---

## 5. 交互规格（用户视角）

### 5.1 监控页日志查看

| 场景 | 前端组件 | 后端数据源 |
|------|---------|-----------|
| 实时 Flow Log 流 | FlowLogStream.vue | EnvelopeTypeFlowLog → WS 推送 |
| Flow 追踪面板 | FlowTracePanel.vue | FlowLog span 树渲染 |
| Flow Log 导出 | FlowLogExportButton.vue | ListFlowLogs HTTP API |
| 进程日志流 | ProcessLogStream.vue | EnvelopeTypeLog → WS 推送 |
| WS 连接管理 | useLogStreamHub.ts | 统一 WS Hub |

### 5.2 FlowLog 严重级别（用户可见）

| 级别 | 含义 | UI 表现 |
|------|------|--------|
| ok | 成功 | 绿色 |
| info | 信息 | 蓝色 |
| warn | 警告 | 黄色 |
| error | 错误 | 红色 |
| critical | 严重 | 红色高亮 |

### 5.3 Flow Log 与聊天错误的关系

- FlowLog error 默认发布为聊天错误 toast（EnvelopeTypeError → SessionBus）
- `flowStepsSkipChatError` 中的 step_id 不发布为聊天错误（避免噪音，如 `chat.usage_record`、`system.agent.tool_build`）

---

## 6. 配置项（用户可配）

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| level | string | info | 日志级别（debug/info/warn/error） |
| output_dir | string | Linux: `/var/log/aranea`, Win: `./logs` | 输出目录 |
| max_size_mb | int32 | 100 | 单文件最大 MB |
| max_backups | int32 | 10 | 保留旧文件数 |
| max_age_days | int32 | 30 | 最大保留天数 |
| compress | bool | false | 是否压缩旧文件 |
| stdout_enabled | bool | false | 是否同时输出 stdout |
| hook_level | string | — | EventBusSink 级别阈值 |
| sinks | repeated LoggingSink | — | 配置驱动的 Sink 列表 |

### 6.1 环境变量

| 变量 | 用途 |
|------|------|
| `MONITOR_FLOW_LOG_DIR` | 覆盖日志输出目录 |
| `MONITOR_BUS_ROUTING` | Infra 双总线路由模式（split/dual，默认 split） |

> Proto 契约（SinkType/DropPolicy/LoggingSink/Logging message 定义）见 [设计文档 §7](./64-logging-framework.design.md#7-配置驱动-sink-注册)。

---

## 7. 验收标准

### 7.1 红线合规

- [x] `grep -r "log/slog" internal/` 为零（红线 #16）
- [x] `grep -r "zap\." internal/` 为零（红线 #10a，trpc-agent-go 运行时除外）
- [x] `Global()` 无新增调用（NFR-C4）

### 7.2 功能验证

- [x] `go test ./pkg/loggateway/... -count=1` 通过
- [x] `go test ./pkg/logpipeline/... -count=1` 通过
- [x] `go test ./internal/event/... -count=1` 通过
- [x] `go build ./cmd/admin` 通过

### 7.3 构造注入验证

- [x] Wire 编译通过
- [x] 所有 Usecase/Service 通过构造注入获取 Logger

### 7.4 桥接验证

- [x] trpc-agent-go 运行时日志经 RuntimeLogAdapter 进入 Pipeline（解决 A-2/A-3 偏差）
- [x] `agentlog.Default` / `agentlog.ContextDefault` 被替换

---

## 8. 步骤注册表（用户视角）

采用 `{domain}.{subsystem}.{action}` 点分命名，已注册约 80 个 step_id，每个 step_id 映射人类可读中文标题。

| 域 | 示例 step_id | 数量 |
|----|-------------|------|
| chat | chat.agent.build, chat.turn.start, chat.turn.end | ~15 |
| team | team.graph.compile, team.graph.run | ~10 |
| knowledge | knowledge.search, knowledge.index | ~8 |
| plugin | plugin.hook.invoke, plugin.guard.check | ~10 |
| system | system.cron.tick, system.health.check | ~40 |
| memory | memory.worker.run, memory.deadletter.replay | ~6 |
| channel | channel.deliver, channel.health | ~8 |
| model | model.sync, model.apply | ~6 |
| monitor | monitor.alert, monitor.flow | ~5 |
| 其他 | a2a.*, session.*, evaluation.* | ~4 |

> 完整注册表与代码位置见 [开发计划 §代码锚点](./64-logging-framework.development.md#6-代码锚点)。

---

## 9. 相关文档

| 文档 | 路径 | 定位 |
|------|------|------|
| 日志框架设计 | `docs/development/64-logging-framework.design.md` | 架构、接口、Proto、数据模型 |
| 日志框架开发计划 | `docs/development/64-logging-framework.development.md` | 代码锚点、进度、任务、已知偏差 |
| 项目规则-日志架构约束 | `.trae/rules/project_rules.md` §日志架构约束 | 红线、组件表、使用规则 |
| 监控模块 | `docs/development/18-monitor.md` | 监控页 Flow Log 展示需求 |
