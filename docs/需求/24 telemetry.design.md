# Telemetry 遥测模块 — 实现设计文档

> 对应需求：`24 telemetry.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

OpenTelemetry 遥测集成：Trace/Metrics/Logs 三大信号，覆盖 Agent 运行全链路。

**设计原则**：
- 双轨并行：Prometheus 直出指标（`internal/metrics`）+ OTLP 导出指标（trpc-agent-go `telemetry/metric`）
- 环境变量驱动：`OTEL_EXPORTER_OTLP_ENDPOINT` 未设置时全部 noop，零配置零开销
- 协议可配：`OTEL_EXPORTER_OTLP_PROTOCOL` 支持 `http`（默认）和 `grpc`
- Log-Trace 关联：slog handler 自动注入 `trace_id` / `span_id`

---

## 二、Proto 层

无需独立 Proto，通过 Kratos 中间件和 OTLP Exporter 集成。

---

## 三、Biz 层

无需独立领域模型。Trace 数据由 OTel SDK 管理，不落库。业务指标通过 `internal/metrics` 包以 Prometheus Counter/Histogram/Gauge 暴露。

---

## 四、架构分层

### 4.1 初始化入口

```
cmd/admin/main.go
  └─ server.InitTelemetry(Name, Version)     ← EP-OBS-02
       ├─ initHTTPTracerProvider / initGRPCTracerProvider
       ├─ initMeterProvider (trpc-agent-go telemetry/metric)
       └─ otel.SetTextMapPropagator (W3C TraceContext + Baggage)
```

**`internal/server/telemetry.go`** — 唯一初始化入口：

| 函数 | 职责 |
|------|------|
| `InitTelemetry(serviceName, serviceVersion)` | 初始化 TracerProvider + MeterProvider，返回 shutdown 函数 |
| `initHTTPTracerProvider(ctx, res, endpoint)` | OTLP/HTTP Trace 导出 |
| `initGRPCTracerProvider(ctx, res, endpoint)` | OTLP/gRPC Trace 导出（委托 trpc-agent-go `telemetry/trace`） |
| `initMeterProvider(ctx, name, ver, endpoint, protocol)` | OTLP Metrics 导出（委托 trpc-agent-go `telemetry/metric`） |

### 4.2 Kratos 传输层中间件

HTTP 和 gRPC Server 均注册 `tracing.Server()` 中间件（Kratos 内置），自动为每个请求创建 Span：

```go
// internal/server/http.go
http.Middleware(
    tracing.Server(),    // EP-OBS-02: 每个请求自动创建 Span
    recovery.Recovery(),
    validate.Middleware(),
)

// internal/server/grpc.go
grpc.Middleware(
    tracing.Server(),    // EP-OBS-02: 每个请求自动创建 Span
    auth.GRPCMiddleware(),
    recovery.Recovery(),
    validate.Middleware(),
)
```

### 4.3 Prometheus 指标

**`internal/metrics/vars.go`** — 所有业务指标定义，使用 `promauto` 自动注册：

| 指标名 | 类型 | 标签 | 用途 |
|--------|------|------|------|
| `aranea_chat_turn_duration_seconds` | Histogram | agent_id, status | Chat turn 延迟 |
| `aranea_agent_build_cache_hits_total` | Counter | — | Agent 构建 LRU 缓存命中 |
| `aranea_agent_build_cache_misses_total` | Counter | — | Agent 构建 LRU 缓存未命中 |
| `aranea_event_bus_published_total` | Counter | event_type | EventBus 发布计数 |
| `aranea_event_bus_dropped_total` | Counter | event_type, policy | EventBus 背压丢弃 |
| `aranea_graph_active_executions` | Gauge | — | 当前 Graph 执行数 |
| `aranea_tool_invocation_total` | Counter | tool, status | 工具调用计数 |
| `aranea_provider_request_total` | Counter | provider, model, status | LLM Provider 请求计数 |
| `aranea_provider_request_duration_seconds` | Histogram | provider, model | LLM Provider 请求延迟 |
| `aranea_plugin_invoke_total` | Counter | plugin, point, status | 插件回调计数 |
| `aranea_plugin_block_total` | Counter | plugin, reason | 插件阻断计数 |
| `aranea_auto_memory_job_total` | Counter | status | 自动记忆任务计数 |
| `aranea_auto_memory_extraction_duration_seconds` | Histogram | — | 自动记忆提取延迟 |
| `aranea_artifact_upload_bytes_total` | Counter | — | Artifact 上传字节 |
| `aranea_artifact_download_bytes_total` | Counter | — | Artifact 下载字节 |
| `aranea_artifact_storage_bytes` | Gauge | — | Artifact 存储总量 |

**暴露方式**：`/metrics` 端点通过 `srv.Route("/").GET("/metrics", ...)` 注册，绕过 auth 中间件。

### 4.4 trpc-agent-go 框架指标

当 `OTEL_EXPORTER_OTLP_ENDPOINT` 设置时，`initMeterProvider` 调用 trpc-agent-go 的 `telemetry/metric.NewMeterProvider` + `InitMeterProvider`，自动注册以下框架级指标：

| Meter | 指标 | 用途 |
|-------|------|------|
| Chat | `trpcgoagent_client_request_cnt` | Chat 请求计数 |
| Chat | `gen_ai.client.token_usage` | Token 使用量 |
| Chat | `gen_ai.client.operation_duration` | Chat 操作延迟 |
| Chat | `gen_ai.server.time_to_first_token` | 首 Token 延迟 |
| ExecuteTool | `trpcgoagent_client_request_cnt` | 工具执行请求计数 |
| ExecuteTool | `gen_ai.client.operation_duration` | 工具执行延迟 |
| InvokeAgent | `gen_ai.client.token_usage` | Agent 调用 Token 使用量 |
| InvokeAgent | `gen_ai.client.time_to_first_token` | Agent 调用首 Token 延迟 |
| InvokeAgent | `gen_ai.client.operation_duration` | Agent 调用延迟 |

### 4.5 Log-Trace 关联

- **HTTP/gRPC**：Kratos `tracing` 中间件传播 W3C TraceContext。
- **FlowLog**：`internal/event/trace_context.go` 的 `NewTraceContext` 从 `ctx` 读取 OTel `SpanContext`，写入 `correlation.trace_id`。
- **Turn**：`TraceEmitter` 与 usage `metadata_json.spans` 共用同一 `trace_id`。
- **已移除**：`slog_bridge.go` / `traceHandler` / `LOG_BRIDGE_*`（2026-05-20，见 [changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)）。

---

## 五、Service 层

无需独立 Service，通过中间件和初始化函数自动采集。

---

## 六、Wire 注入

`InitTelemetry` 在 `cmd/admin/main.go` 中直接调用（在 `wireApp` 之前），不走 Wire 注入。返回的 shutdown 函数通过 `defer` 确保进程退出前 flush。

---

## 七、环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | （空） | 设置后启用 OTel Tracer + Meter Provider；未设置则 noop |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http` | 导出协议：`http` 或 `grpc` |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | （继承 ENDPOINT） | Trace 端点覆盖 |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | （继承 ENDPOINT） | Metrics 端点覆盖 |
| `FLOW_LOG_STDERR` | （空） | 设为 `1` 时 FlowLog 同步写 stderr（调试） |
| ~~`LOG_BRIDGE_ENABLED`~~ | — | **已废弃**（SlogBridge 已删除） |
| ~~`LOG_BRIDGE_LEVEL`~~ | — | **已废弃** |

---

## 八、Web 前端设计

### 8.1 组件

**MonitorDashboard.vue** 中增加 Trace 面板（EP-OBS-06，待实现）：
- Trace 列表
- Span 详情瀑布图
- Metrics 图表（Grafana 嵌入或自绘）

### 8.2 API

```typescript
export async function getTraceList(query: TraceQuery): Promise<TraceListResult>
export async function getTraceDetail(traceId: string): Promise<TraceDetail>
```

---

## 九、与 trpc-agent-go 框架对齐

trpc-agent-go 提供了完整的 `telemetry/` 子包：

| 包 | 路径 | 用途 |
|----|------|------|
| trace | `telemetry/trace` | TracerProvider 初始化 + 全局 Tracer |
| metric | `telemetry/metric` | MeterProvider 初始化 + 框架指标注册 |
| langfuse | `telemetry/langfuse` | Langfuse 平台集成 |
| semconv/metrics | `telemetry/semconv/metrics` | 指标命名约定 |
| semconv/trace | `telemetry/semconv/trace` | Span 属性命名约定 |
| metric/histogram | `telemetry/metric/histogram` | 动态 Bucket Histogram |

Aranea 通过 `internal/server/telemetry.go` 桥接这些包，遵循 R1 红线（biz 层不直接 import trpc-agent-go）。
