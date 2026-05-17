# Telemetry 遥测 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[24 telemetry.md](./24%20telemetry.md) · **设计**：[24 telemetry.design.md](./24%20telemetry.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Telemetry 遥测：基于 OpenTelemetry 的分布式追踪和指标采集，为系统提供可观测性。

**代码锚点**：
- `internal/server/http.go` — OTEL 中间件注册
- `internal/server/grpc.go` — OTEL gRPC 拦截器
- `configs/` — OTEL 配置

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| HTTP OTEL 中间件 | ✅ | `http.go` 中注册 |
| gRPC OTEL 拦截器 | ✅ | `grpc.go` 中注册 |
| Trace 传播 | ✅ | W3C Trace Context |
| 自定义 Span | ❌ | Agent/Team 运行无自定义 Span |
| Metrics 导出 | ❌ | 无 OTLP Metrics 导出 |
| Log 关联 | ❌ | 日志未与 Trace ID 关联 |

---

## 3. 差距与优化

1. **P2**：Agent/Team 运行过程无自定义 Span，无法追踪 LLM 调用、工具调用等关键步骤。
2. **P3**：无 OTLP Metrics 导出，无法接入 Prometheus/Grafana。
3. **P3**：日志未与 Trace ID 关联，无法通过 Trace ID 查询相关日志。

---

## 4. 开发阶段

- **Phase 1**：Agent/Team 运行关键步骤添加自定义 Span
- **Phase 2**：OTLP Metrics 导出
- **Phase 3**：日志与 Trace ID 关联

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `trpc_turn.go`：LLM 调用 / 工具调用添加 Span | P2 | — |
| 2 | OTLP Metrics 导出配置 | P3 | — |
| 3 | 日志中间件注入 Trace ID | P3 | — |

---

## 6. 验收标准

- [ ] Jaeger/Zipkin 可看到 Agent 运行的完整 Trace
- [ ] Prometheus 可采集系统 Metrics
- [ ] 日志可通过 Trace ID 关联查询

---

## 7. 依赖与风险

- 需部署 OTEL Collector
- 自定义 Span 需注意性能开销
