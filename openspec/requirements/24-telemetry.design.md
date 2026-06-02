# Telemetry 遥测模块 — 实现设计文档

> **对应需求**：[24 telemetry.md](./24%20telemetry.md)  
> **开发计划**：[24-telemetry-development.md](./24-telemetry-development.md)  
> **遵循**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 1. 模块概述

OpenTelemetry 遥测集成：Trace / Metrics / 应用内 Trace 投影，覆盖传输层与 Chat Run 热路径。

**设计原则**：

- **双轨并行**：Prometheus 直出（`internal/metrics`）+ OTLP 导出（`internal/telemetry` + trpc `telemetry/metric`）
- **环境变量驱动**：`OTEL_EXPORTER_OTLP_ENDPOINT` 未设置时 OTLP 全部 noop，零配置零开销
- **协议可配**：`OTEL_EXPORTER_OTLP_PROTOCOL` 支持 `http`（默认）和 `grpc`
- **Log-Trace 关联**：FlowLog `correlation.trace_id` 对齐 OTel W3C TraceID（**不用** slog bridge）
- **分层红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；OTel 初始化仅在 `internal/telemetry`，传输中间件在 `internal/server`

---

## 2. Proto 层

无独立 Telemetry Proto。观测数据通过 Kratos 中间件、Prometheus scrape、Usage/Monitor 既有 API 与 WS 暴露。

---

## 3. Biz 层

无独立领域模型。Trace 数据由 OTel SDK 或 `TraceEmitter` 内存缓冲管理，Usage spans 投影至 `model_token_usage_events.metadata_json`。

---

## 4. 架构分层

### 4.1 初始化入口

```text
cmd/admin/main.go
  └─ telemetry.Init(Name, Version)          ← EP-OBS-02
       ├─ initTracerProvider
       │    ├─ HTTP → otlptracehttp + sdktrace.TracerProvider
       │    └─ gRPC → trpc-agent-go telemetry/trace.Start
       ├─ initMeterProvider → trpc telemetry/metric
       └─ otel.SetTextMapPropagator (W3C TraceContext + Baggage)
```

**`internal/telemetry/telemetry.go`** — OTLP 唯一初始化入口：

| 函数 | 职责 |
|------|------|
| `Init(serviceName, serviceVersion)` | 读环境变量；初始化 Tracer + Meter；返回 shutdown |
| `initTracerProvider` | 按 protocol 分发 HTTP/gRPC TracerProvider |
| `initHTTPTracerProvider` | OTLP/HTTP Trace 导出 |
| `initGRPCTracerProvider` | OTLP/gRPC Trace 导出（委托 trpc `telemetry/trace`，传入 service name/version） |
| `initMeterProvider` | OTLP Metrics 导出（委托 trpc `telemetry/metric`） |
| `shutdownAll` | Tracer + Meter 优雅关闭 |

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
| `aranea_auto_memory_extraction_duration_seconds` | Histogram | — | 自动记忆提取延迟 |
| `aranea_artifact_upload_bytes_total` | Counter | — | Artifact 上传字节 |
| `aranea_artifact_download_bytes_total` | Counter | — | Artifact 下载字节 |
| `aranea_artifact_storage_bytes` | Gauge | — | Artifact 存储总量 |

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
internal/service/trpc_turn.go
  startTurnSpan("chat.turn", ...)     ← OTel 根 Span（I6-TEL-01）
  TraceEmitterForRun → ctx
       │
       ▼
internal/event/trace_emitter.go
  ├─ publishFlowLog → WS monitor（correlation.trace_id）
  └─ span buffer → recordTurnUsage metadata_json.spans

internal/event/trace_context.go
  NewTraceContext(ctx) → 优先 OTel TraceID，否则 tr_<uuid>
```

- **HTTP/gRPC**：Kratos `tracing` 中间件传播 W3C TraceContext。
- **Turn**：`TraceEmitter` 与 usage spans 共用 `trace_id`。
- **系统域 FlowLog**：`system.telemetry.init|noop|error`（`internal/event/flow_log.go` 步骤注册表）。
- **已移除**：`slog_bridge.go` / `LOG_BRIDGE_*`（2026-05-20）。

### 4.6 Service 层 OTel Span

**`internal/telemetry/turntrace`** — Chat / Team / Graph 共用 OTel turn bridge：

| 路径 | 挂载点 |
|------|--------|
| Chat | `service/trpc_turn.go` → `startTurnSpan` |
| Team | `team/runner_team_trpc.go` → `team.run` |
| Graph | **`service/graph.go`** → `graph.execute`；`GraphExecutionTelemetry`（Wire 注入 `biz.GraphExecutionObserver`）在 execution 完成时结束 Span |

**`internal/service/turn_trace.go`** — Chat 薄封装。

**`internal/event/framework_events.go`** — `WrapFrameworkEvents(emitter, FrameworkSpanObserver)`；tool span **仅 hook 关闭**。

细粒度 LLM/Tool Span 在 OTel 层；usage 投影仍在 `TraceEmitter`。

---

## 5. Service / Wire

- 无独立 Telemetry Service RPC。
- `telemetry.Init` 在 `cmd/admin/main.go` 于 `wireApp` **之前**调用；shutdown 通过 `defer` flush。
- Chat/Team Run 在 `internal/service/trpc_turn.go`、`internal/team/runner_team_trpc.go` 挂载 `TraceEmitter`。

---

## 6. 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | （空） | 设置后启用 OTLP Tracer + Meter；未设置则 noop |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http` | `http` 或 `grpc` |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | （继承 ENDPOINT） | Trace 端点覆盖（trpc trace 包可读） |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | （继承 ENDPOINT） | Metrics 端点覆盖 |
| `OTEL_TRACES_SAMPLER` | `always_on` | HTTP 导出采样：`parentbased_traceidratio` 等 |
| `OTEL_TRACES_SAMPLER_ARG` | `1.0` | 比率采样参数（0.0–1.0）；**gRPC 路径暂未接入** |
| `FLOW_LOG_STDERR` | （空） | `1` 时 FlowLog 同步写 stderr（调试） |

---

## 7. Web 前端

Telemetry **不单独占路由**；排障 UI 归 Monitor **Runs** Tab（路由名 `traces`）：

| 组件 | 路径 | 职责 |
|------|------|------|
| `TraceList.vue` | `web/src/components/monitor/` | Runs 列表（usage events） |
| `TraceWaterfall.vue` | 同上 | usage spans 瀑布图 |
| Flow 详情 | `TraceList` 对话框内 Tab | WS `flow_log` 实时流 |

Jaeger 级全链路 UI **非目标**；`/overview` Dashboard 负责用量大盘（见 [18 monitor-dashboard.md](./18%20monitor-dashboard.md)）。

---

## 8. 与 trpc-agent-go 框架对齐

| 包 | 路径 | Aranea 用法 |
|----|------|-------------|
| trace | `telemetry/trace` | gRPC TracerProvider（`initGRPCTracerProvider`） |
| metric | `telemetry/metric` | MeterProvider + 框架指标 |
| semconv | `telemetry/semconv/*` | 命名约定（框架内部） |

Aranea 通过 **`internal/telemetry`** 桥接，**禁止**在 `internal/biz` 直接 import trpc-agent-go。

---

## 9. 依赖关系图

```text
                    ┌─────────────────┐
                    │  cmd/admin/main │
                    └────────┬────────┘
                             │ telemetry.Init
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
     internal/telemetry   internal/server   internal/metrics
     (OTLP Trace/Metric)  (tracing MW)      (Prometheus vars)
              │              │              │
              └──────────────┼──────────────┘
                             ▼
                    internal/service
                    trpc_turn + turn_trace
                             │
                             ▼
                    internal/event
                    TraceEmitter / TraceContext
```
