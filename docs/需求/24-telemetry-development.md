# Telemetry 遥测 — 开发计划

> **版本**：2026-05-18 | **状态**：🟡 部分实现
> **需求**：[24 telemetry.md](./24%20telemetry.md) · **设计**：[24 telemetry.design.md](./24%20telemetry.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-OBS-01/02/04/05 ✅, EP-OBS-06 待实现

---

## 1. 模块定位

Telemetry 遥测：基于 OpenTelemetry 的分布式追踪和指标采集，为系统提供可观测性。

**代码锚点**：
- `internal/server/telemetry.go` — OTel TracerProvider + MeterProvider 初始化入口
- `internal/server/http.go` — OTEL 中间件注册 + `/metrics` 端点
- `internal/server/grpc.go` — OTEL gRPC 拦截器
- `internal/metrics/vars.go` — Prometheus 业务指标定义
- `internal/event/slog_bridge.go` — Log-Trace 关联（traceHandler）
- `configs/` — OTEL 环境变量配置

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| HTTP OTEL 中间件 | ✅ | `http.go` 中 `tracing.Server()` |
| gRPC OTEL 拦截器 | ✅ | `grpc.go` 中 `tracing.Server()` |
| Trace 传播 | ✅ | W3C TraceContext + Baggage |
| OTLP Trace 导出（HTTP） | ✅ | `telemetry.go` → `initHTTPTracerProvider` |
| OTLP Trace 导出（gRPC） | ✅ | `telemetry.go` → `initGRPCTracerProvider`（委托 trpc-agent-go `telemetry/trace`） |
| OTLP Metrics 导出 | ✅ | `telemetry.go` → `initMeterProvider`（委托 trpc-agent-go `telemetry/metric`） |
| Prometheus `/metrics` | ✅ | `internal/metrics/vars.go` + `http.go` Route 注册 |
| Log-Trace 关联 | ✅ | `slog_bridge.go` → `traceHandler` 自动注入 trace_id/span_id |
| 自定义 Span | ❌ | Agent/Team/Graph 运行无自定义 Span（EP-OBS-06） |
| 前端 Trace 面板 | ❌ | MonitorDashboard 无 Trace 列表/瀑布图（EP-OBS-06） |
| Trace 采样策略 | ❌ | 当前全量采样，无 Sampler 配置 |

---

## 3. 差距与优化

1. **P2**：Agent/Team/Graph 运行过程无自定义 Span，无法追踪 LLM 调用、工具调用等关键步骤（EP-OBS-06）。
2. **P3**：无 Trace 采样策略，高负载下全量采样可能造成存储和性能压力。
3. **P3**：前端无 Trace 可视化面板，运维无法在 UI 中查看链路追踪。

---

## 4. 开发阶段

- **Phase 1** ✅：OTLP Trace/Metrics 导出 + Prometheus 指标 + Log-Trace 关联
- **Phase 2**：Agent/Team/Graph 运行关键步骤添加自定义 Span（EP-OBS-06）
- **Phase 3**：Trace 采样策略 + 前端 Trace 面板

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 状态 |
|---|------|--------|-----|------|
| 1 | OTLP Trace 导出（HTTP + gRPC） | P1 | EP-OBS-02 | ✅ |
| 2 | OTLP Metrics 导出（trpc-agent-go 框架指标） | P2 | EP-OBS-02 | ✅ |
| 3 | Prometheus `/metrics` 端点 | P0 | EP-OBS-01 | ✅ |
| 4 | Log-Trace 关联（slog traceHandler） | P2 | EP-OBS-05 | ✅ |
| 5 | `/metrics` 端点合规（Route 替代 HandleFunc） | P1 | R4/R12 | ✅ |
| 6 | `trpc_turn.go`：LLM 调用 / 工具调用添加自定义 Span | P2 | EP-OBS-06 | ❌ |
| 7 | Graph 执行添加自定义 Span | P2 | EP-OBS-06 | ❌ |
| 8 | Trace 采样策略配置 | P3 | — | ❌ |
| 9 | 前端 Trace 面板 | P3 | EP-OBS-06 | ❌ |

---

## 6. 验收标准

- [x] `OTEL_EXPORTER_OTLP_ENDPOINT` 设置后，Jaeger 可看到 HTTP/gRPC 请求的 Trace
- [x] `OTEL_EXPORTER_OTLP_ENDPOINT` 设置后，trpc-agent-go 框架指标通过 OTLP 导出
- [x] Prometheus 可采集系统 Metrics（`/metrics` 端点）
- [x] 日志包含 `trace_id` / `span_id` 字段（当请求在 Trace 上下文中时）
- [ ] Jaeger 可看到 Agent 运行的完整自定义 Span（LLM 调用、工具调用等）
- [ ] 前端 MonitorDashboard 可查看 Trace 列表和 Span 瀑布图

---

## 7. 依赖与风险

- 需部署 OTEL Collector + Jaeger/Prometheus（开发环境可用 `pkg/trpc-agent-go/examples/telemetry/jaeger-prometheus/docker-compose.yml`）
- 自定义 Span 需注意性能开销；建议 Phase 2 配合采样策略
- trpc-agent-go 框架已内置 `telemetry/` 包，Aranea 通过 `internal/server/telemetry.go` 桥接，遵循 R1 红线
