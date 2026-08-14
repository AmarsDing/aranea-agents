# Telemetry 遥测模块 — 实现设计文档

> **对应需求**：[24-telemetry.md](./24-telemetry.md)  
> **开发计划**：[24-telemetry-development.md](./24-telemetry-development.md)  
> **遵循**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 1. 模块概述

OpenTelemetry 遥测集成：Trace / Metrics / 应用内 Trace 投影，覆盖传输层与 Chat/Team/Graph Run 热路径。

**设计原则**：

- **三轨并行**：Prometheus 直出（`internal/metrics`）+ OTLP 导出（`internal/telemetry` + trpc `telemetry/metric`）+ 应用内 Trace 投影（`TraceEmitter` + `turntrace.Bridge`）。~~Langfuse 导出（`internal/telemetry/langfuse.go`）~~已删除——从未接入 Wire（零生产引用），2026-08-14 死代码清理移除
- **环境变量驱动**：`OTEL_EXPORTER_OTLP_ENDPOINT` 未设置时 OTLP 全部 noop，零配置零开销
- **协议可配**：`OTEL_EXPORTER_OTLP_PROTOCOL` 支持 `http`（默认）和 `grpc`
- **采样可配**：`OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` 控制 HTTP 导出采样（gRPC 路径暂未接入）
- **Log-Trace 关联**：FlowLog `correlation.trace_id` 对齐 OTel W3C TraceID（**不用** slog bridge）
- **OTel ↔ Usage 关联**：`otel_trace_id` / `otel_root_span_id` 写入 usage `metadata_json`
- **分层红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；OTel 初始化仅在 `internal/telemetry`，传输中间件在 `internal/server`

---

## 2. Proto 层

无独立 Telemetry Proto。观测数据通过 Kratos 中间件、Prometheus scrape、Usage/Monitor 既有 API 与 WS 暴露。

Langfuse 配置通过 `conf.Bootstrap.Langfuse`（`internal/conf/conf.proto` `message Langfuse`）注入，非独立 Proto。

---

## 3. Biz 层

无独立领域模型。Trace 数据由 OTel SDK 或 `TraceEmitter` 内存缓冲管理，Usage spans 投影至 `model_token_usage_events.metadata_json`。

---

## 4. 架构分层

### 4.1 初始化入口

```text
cmd/admin/main.go
  └─ telemetry.Init(Name, Version, lg)
       ├─ initTracerProvider
       │    ├─ HTTP → otlptracehttp + sdktrace.TracerProvider + buildSampler()
       │    └─ gRPC → trpc-agent-go telemetry/trace.Start
       ├─ initMeterProvider → trpc telemetry/metric
       └─ otel.SetTextMapPropagator (W3C TraceContext + Baggage)
```

**`internal/telemetry/telemetry.go`** — OTLP 唯一初始化入口：

| 函数 | 职责 |
|------|------|
| `Init(serviceName, serviceVersion, lg)` | 读环境变量；初始化 Tracer + Meter；返回 shutdown |
| `initTracerProvider` | 按 protocol 分发 HTTP/gRPC TracerProvider |
| `initHTTPTracerProvider` | OTLP/HTTP Trace 导出 + `buildSampler()` |
| `initGRPCTracerProvider` | OTLP/gRPC Trace 导出（委托 trpc `telemetry/trace`，传入 service name/version） |
| `initMeterProvider` | OTLP Metrics 导出（委托 trpc `telemetry/metric`） |
| `shutdownAll` | Tracer + Meter 优雅关闭 |

**`internal/telemetry/sampler.go`** — 采样器构建：

| 函数 | 职责 |
|------|------|
| `buildSampler()` | 读取 `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG`，构建采样器 |
| `parseRatio(s)` | 解析比率参数，无效值 clamp 到 [0, 1] |

支持的采样策略：`always_on` / `parentbased_always_on` / `always_off` / `parentbased_always_off` / `traceidratio` / `parentbased_traceidratio`。仅 HTTP 路径生效。

~~**`internal/telemetry/langfuse.go`** — Langfuse 运行时~~（已删除 2026-08-14：构造函数虽就绪但从未接入 `wireApp`，零生产引用，属 TEST_ONLY 僵尸实现；如需 Langfuse 导出请参考 `docs/development/phase4-生产级增强/06-Langfuse可观测性.md` 重新实现并接线 Wire）

### 4.2 Kratos 传输层中间件

HTTP 与 gRPC Server 均注册 `tracing.Server()`（Kratos 内置），为每个请求创建 Span：

```go
// internal/server/http.go
http.Middleware(
    tracing.Server(),
    recovery.Recovery(),
    validate.Middleware(),
)

// internal/server/grpc.go
grpc.Middleware(
    tracing.Server(),
    auth.GRPCMiddleware(),
    recovery.Recovery(),
    validate.Middleware(),
)
```

### 4.3 Prometheus 指标

**`internal/metrics/vars.go`** — 业务指标定义（独立 package，避免与 `internal/server` 循环依赖）：

| 指标名 | 类型 | 标签 | 用途 |
|--------|------|------|------|
| `aranea_chat_ttft_seconds` | Histogram | agent_id, first_byte_type | Chat 首 Token 延迟（TTFT） |
| `aranea_chat_turn_duration_seconds` | Histogram | agent_id, status | Chat turn 延迟 |
| `aranea_agent_build_cache_hits_total` | Counter | — | Agent 构建 LRU 命中 |
| `aranea_agent_build_cache_misses_total` | Counter | — | Agent 构建 LRU 未命中 |
| `aranea_event_bus_published_total` | Counter | event_type | EventBus 发布 |
| `aranea_event_bus_dropped_total` | Counter | event_type, policy | EventBus 背压丢弃 |
| `aranea_graph_active_executions` | Gauge | — | Graph 执行中数量 |
| `aranea_tool_invocation_total` | Counter | tool, status | 工具调用 |
| `aranea_provider_request_total` | Counter | provider, model, status | LLM 请求 |
| `aranea_provider_request_duration_seconds` | Histogram | provider, model | LLM 延迟 |
| `aranea_plugin_invoke_total` | Counter | plugin, point, status | 插件回调 |
| `aranea_plugin_block_total` | Counter | plugin, reason | 插件阻断 |
| `aranea_auto_memory_job_total` | Counter | status | 自动记忆任务 |
| `aranea_auto_memory_llm_fallback_total` | Counter | — | 自动记忆 LLM 降级 |
| `aranea_auto_memory_extraction_duration_seconds` | Histogram | — | 自动记忆提取延迟 |
| `aranea_artifact_upload_bytes_total` | Counter | — | Artifact 上传字节 |
| `aranea_artifact_download_bytes_total` | Counter | — | Artifact 下载字节 |
| `aranea_artifact_storage_bytes` | Gauge | — | Artifact 存储总量 |
| `aranea_mcp_session_reconnect_total` | Counter | server_key, outcome | MCP 会话重连 |
| `aranea_mcp_invocation_total` | Counter | tool, status | MCP 工具调用 |
| `aranea_alert_notify_total` | Counter | channel, status | 告警通知 |
| `aranea_model_router_fallback_total` | Counter | reason | 模型路由降级 |
| `aranea_channel_delivery_total` | Counter | platform, status | Channel 消息投递 |
| `aranea_channel_delivery_duration_seconds` | Histogram | platform | Channel 投递延迟 |
| `aranea_channel_runtime_reconnect_total` | Counter | platform, receive_mode, outcome | Channel 运行时重连 |
| `aranea_channel_runtime_connected` | Gauge | platform, receive_mode | Channel 运行时连接数 |
| `aranea_channel_stream_update_total` | Counter | platform, phase, result | Channel 流式更新 |
| `aranea_channel_turn_duration_seconds` | Histogram | platform | Channel turn 延迟 |
| `aranea_channel_turn_job_total` | Counter | channel_id, status | Channel turn 任务 |
| `aranea_channel_busy_intent_total` | Counter | intent | Channel 忙意图 |
| `aranea_channel_progress_patch_total` | Counter | platform, result | Channel 进度补丁 |
| `aranea_channel_tool_card_total` | Counter | platform, phase, result | Channel 工具卡片 |
| `aranea_team_graph_runtime_total` | Counter | outcome, reason | Team/Graph 运行时 |
| `aranea_skill_import_total` | Counter | phase, status | Skill 导入 |
| `aranea_skill_import_duration_seconds` | Histogram | phase | Skill 导入延迟 |
| `aranea_safego_panic_recovered_total` | Counter | name | SafeGo panic 恢复 |

**`internal/metrics/callback.go`** — 回调指标：

| 指标名 | 类型 | 标签 | 用途 |
|--------|------|------|------|
| `aranea_callback_duration_seconds` | Histogram | source, point | 回调延迟 |
| `aranea_callback_error_total` | Counter | source, point, reason | 回调错误 |

`ObserveCallback(source, point, start, err)` 辅助函数同时记录延迟和错误。

**暴露**：`internal/server/http.go` 中 `srv.Route("/").GET("/metrics", promhttp.Handler())`，绕过 auth。

### 4.4 trpc-agent-go 框架指标（OTLP）

当 `OTEL_EXPORTER_OTLP_ENDPOINT` 设置时，`initMeterProvider` 注册框架级指标，例如：

| Meter | 指标 | 用途 |
|-------|------|------|
| Chat | `trpcgoagent_client_request_cnt` | Chat 请求计数 |
| Chat | `gen_ai.client.token_usage` | Token 使用量 |
| Chat | `gen_ai.client.operation_duration` | Chat 操作延迟 |
| ExecuteTool | `gen_ai.client.operation_duration` | 工具执行延迟 |
| InvokeAgent | `gen_ai.client.token_usage` | Agent 调用 Token |

完整列表见 `pkg/trpc-agent-go/internal/telemetry/`。

### 4.5 应用内 Trace 投影（FlowLog + Usage + OTel）

```text
internal/service/chat_orchestrator_turn.go
  startTurnSpan("chat.turn", ...)          ← OTel 根 Span
  turntrace.Bridge → ctx
  WrapFrameworkEventsWithOtel(events, emitter, bridge, bridge)
       │
       ▼
internal/event/trace_emitter.go (embeds FlowTracker)
  ├─ FlowTracker.LogStart/LogDone/LogError → WS monitor（correlation.trace_id）
  ├─ SpanCollector → recordTurnUsage metadata_json.spans
  └─ SetOtelRefs(traceID, rootSpanID) → otel_trace_id / otel_root_span_id

internal/event/trace_context.go
  NewTraceContext(ctx) → 优先 OTel TraceID，否则 tr_<uuid>

internal/event/span_collector.go
  MetadataJSON() → 含 otel_trace_id / otel_root_span_id / 各 span otel_id
```

- **HTTP/gRPC**：Kratos `tracing` 中间件传播 W3C TraceContext。
- **Turn**：`TraceEmitter` 与 usage spans 共用 `trace_id`；`turntrace.Bridge` 管理 OTel span 树。
- **OTel ↔ Usage 关联**：`SetOtelRefs` 将 OTel trace_id / root_span_id 写入 `MetadataJSON()`，最终存入 `model_token_usage_events.metadata_json`。
- **Span ID 同步**：`SyncOtelSpanIDs(src)` 将 `llm.call` / `tool.call` / `chat.turn` 的 OTel span ID 写入各 span 的 `otel_id` 字段。
- **系统域 FlowLog**：`system.telemetry.init|noop|error`（`internal/event/flow_log.go` 步骤注册表）。
- **已移除**：`slog_bridge.go` / `LOG_BRIDGE_*`（2026-05-20）。

> FlowLog 发布通过 `FlowTracker`（嵌入 `TraceEmitter`）的 `LogStart`/`LogDone`/`LogError` 等方法，经 `event.Bus` 投递至 WS monitor 频道。

### 4.6 Service 层 OTel Span

**`internal/telemetry/turntrace/bridge.go`** — Chat / Team / Graph 共用 OTel turn bridge：

| 路径 | 挂载点 | Span 名 |
|------|--------|---------|
| Chat | `service/chat_orchestrator_turn.go` → `startTurnSpan` | `chat.turn` |
| Team | `team/runner_team_trpc.go` → `turntrace.Start` | `team.run` |
| Graph | `service/graph_execution_service.go` → `turntrace.Start` | `graph.execute` |

**Bridge 核心方法**：

| 方法 | 职责 |
|------|------|
| `Start(ctx, Config)` | 开启根 Span，返回 `(ctx, *Bridge, trace.Span)` |
| `Finish(err)` | 结束根 + 所有子 Span（llm、tool） |
| `StartChild(ctx, name, attrs)` | 开启命名子 Span |
| `ObserveFrameworkEvent(ev)` | 从 trpc-agent-go 事件流创建/更新 `llm.call` / `tool.call` 子 Span |
| `RecordToolCallEnd(toolCallID, toolName, err)` | 关闭 tool Span（**唯一关闭路径**） |
| `RootSpanID()` / `TraceID()` | 获取 OTel ID |
| `LLMSpanOtelID()` / `ToolSpanOtelID(toolCallID)` | 获取子 Span OTel ID（实现 `OtelSpanIDSource` 接口） |

**Config**：Domain（`chat` / `team` / `graph`）、SpanName、SessionID、RunID、AgentKey、Extra attributes。

**`internal/service/turn_trace.go`** — Chat 薄封装。

**`internal/service/graph_telemetry.go`** — `GraphExecutionTelemetry`（Wire 注入 `biz.GraphExecutionObserver`）：
- `Bind(execID, bridge)` — 绑定 bridge 到 execution ID
- `OnGraphExecutionComplete(exec)` — 完成时调用 `bridge.Finish(err)`
- `EnsureFinished(execID, err)` — 异常提前返回时确保关闭

**`internal/event/framework_events.go`** — `WrapFrameworkEventsWithOtel(in, emitter, observer, otelSrc)`：
- `FrameworkSpanObserver` 接口：`ObserveFrameworkEvent(ev)` — Bridge 实现
- `OtelSpanIDSource` 接口：`LLMSpanOtelID()` / `ToolSpanOtelID(toolCallID)` — Bridge 实现
- tool span **仅 hook 关闭**（`RecordToolCallEnd` 为唯一关闭路径）

细粒度 LLM/Tool Span 在 OTel 层；usage 投影仍在 `TraceEmitter`。

---

## 5. Service / Wire

- 无独立 Telemetry Service RPC。
- `telemetry.Init` 在 `cmd/admin/main.go` 于 `wireApp` **之前**调用；shutdown 通过 `defer` flush。
- `LangfuseRuntime` 构造函数已就绪（`NewLangfuseRuntime`），设计为 Wire 注入、独立于 OTLP 生命周期；当前接入状态见开发计划 §2。
- Chat Run 在 `internal/service/chat_orchestrator_turn.go` 挂载 `turntrace.Bridge` + `TraceEmitter`。
- Team Run 在 `internal/team/runner_team_trpc.go` 挂载 `turntrace.Bridge` + `TraceEmitter`，trace_id 持久化到 team_run 记录。
- Graph Run 在 `internal/service/graph_execution_service.go` 挂载 `turntrace.Bridge`，通过 `GraphExecutionTelemetry` 观察者管理生命周期。

---

## 6. 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | （空） | 设置后启用 OTLP Tracer + Meter；未设置则 noop |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http` | `http` 或 `grpc` |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | （继承 ENDPOINT） | Trace 端点覆盖（trpc trace 包可读） |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | （继承 ENDPOINT） | Metrics 端点覆盖 |
| `OTEL_TRACES_SAMPLER` | `always_on` | HTTP 导出采样：`always_on` / `parentbased_always_on` / `always_off` / `parentbased_always_off` / `traceidratio` / `parentbased_traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | `1.0` | 比率采样参数（0.0–1.0）；**仅 HTTP 路径生效，gRPC 路径暂未接入** |
| `FLOW_LOG_STDERR` | （空） | `1` 时 FlowLog 同步写 stderr（调试） |

**Langfuse 配置**（`conf.Bootstrap.Langfuse`，非环境变量）：

| 字段 | Proto 字段 | 说明 |
|------|-----------|------|
| `Enable` | `enable` | 是否启用 Langfuse |
| `PublicKey` | `public_key` | Langfuse Public Key |
| `SecretKey` | `secret_key` | Langfuse Secret Key |
| `BaseUrl` | `base_url` | Langfuse 服务端地址 |
| `FlushIntervalMS` | `flush_interval_ms` | Flush 间隔（毫秒） |

---

## 7. Web 前端

Telemetry **不单独占路由**；排障 UI 归 Monitor **Runs** Tab（路由名 `traces`）：

| 组件 | 路径 | 职责 |
|------|------|------|
| `TraceList.vue` | `web/src/components/monitor/` | Runs 列表（usage events） |
| `TraceWaterfall.vue` | 同上 | usage spans 瀑布图 |
| Flow 详情 | `TraceList` 对话框内 Tab | WS `flow_log` 实时流 |

Jaeger 级全链路 UI **非目标**；`/overview` Dashboard 负责用量大盘（见 [18-monitor.md](./18-monitor.md)）。

---

## 8. 与 trpc-agent-go 框架对齐

| 包 | 路径 | Aranea 用法 |
|----|------|-------------|
| trace | `telemetry/trace` | gRPC TracerProvider（`initGRPCTracerProvider`） |
| metric | `telemetry/metric` | MeterProvider + 框架指标 |
| langfuse | `telemetry/langfuse` | Langfuse 导出（`LangfuseRuntime`） |
| semconv | `telemetry/semconv/*` | 命名约定（框架内部） |

Aranea 通过 **`internal/telemetry`** 桥接，**禁止**在 `internal/biz` 直接 import trpc-agent-go。

---

## 9. 依赖关系图

```text
                    ┌─────────────────┐
                    │  cmd/admin/main │
                    └────────┬────────┘
                             │ telemetry.Init + LangfuseRuntime (Wire)
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
     internal/telemetry   internal/server   internal/metrics
     (OTLP Trace/Metric   (tracing MW)      (Prometheus vars)
      + Sampler           │                  │
      + Langfuse)         │                  │
              │            │                  │
              └────────────┼──────────────────┘
                           ▼
                  internal/service
                  chat_orchestrator_turn + turn_trace
                  graph_execution_service + graph_telemetry
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
     internal/telemetry   internal/team   internal/event
     /turntrace/bridge    runner_team     TraceEmitter / TraceContext
     (OTel Span 树)       trpc           / span_collector
```
