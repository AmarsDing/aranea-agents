# 24 Telemetry 遥测 — 需求规格（占位）

> **状态**：占位文档（2026-05-17 治理，2026-05-18 现状更新）。实际产品需求 / 验收标准待补；当前仅有设计文档 [`24 telemetry.design.md`](./24%20telemetry.design.md) 描述 Trace / Metrics / Logs 三大信号的技术方案。
>
> **代码现状**（与 `guides/execution-plan.md` §3 EP-OBS-01/02 对应）：
> - ✅ Prometheus `/metrics` 已通：`internal/metrics/vars.go` + `http.go` Route 注册 + Grafana 仪表盘 `docs/observability/grafana-aranea.json`。
> - ✅ EventBus / Cron 已加业务指标；HTTP / gRPC Kratos 默认指标已开。
> - ✅ OTLP Trace 导出已通：`internal/server/telemetry.go` 支持 HTTP + gRPC 协议，环境变量 `OTEL_EXPORTER_OTLP_ENDPOINT` 驱动。
> - ✅ OTLP Metrics 导出已通：委托 trpc-agent-go `telemetry/metric` 包，框架级指标（Chat/Tool/InvokeAgent）自动注册。
> - ✅ Log-Trace 关联已实现：`internal/event/slog_bridge.go` 中 `traceHandler` 自动注入 `trace_id` / `span_id`。
> - ❌ Agent/Team/Graph 运行无自定义 Span（EP-OBS-06 待实现）。
> - ❌ 前端 Trace 可视化面板未实现（EP-OBS-06 待实现）。
> - ❌ Trace 采样策略未配置。

---

## 1. 占位说明

本文档定位为 **Telemetry 产品需求基线**，但项目早期把 Telemetry 拆为两条线：
1. **指标 / 告警**：以 Prometheus + Grafana 落地，归口在 `guides/execution-plan.md` §3 EP-OBS-01。
2. **链路追踪**：OTLP Trace 导出已通（EP-OBS-02），自定义业务 Span 待 EP-OBS-06 推进。

后续如需补全本文档，建议章节：
- §2 用户故事（运维 / 开发 / 性能调优）
- §3 必备指标 / 必备 Trace 字段清单
- §4 Sampler 策略 / 数据保留期 / 告警阈值
- §5 与 `34 event-system.md` 的对接（Agent 事件 ↔ Trace Span）

---

## 2. 关联文档

| 文档 | 用途 |
|------|------|
| [`24 telemetry.design.md`](./24%20telemetry.design.md) | Telemetry 技术设计（架构分层 + 环境变量 + trpc 框架对齐） |
| [`24-telemetry-development.md`](./24-telemetry-development.md) | Telemetry 开发计划（现状评估 + 任务清单 + 验收标准） |
| [`34 event-system.md`](./34%20event-system.md) | Agent 事件系统现状对齐 |
| [`guides/execution-plan.md`](../guides/execution-plan.md) §3 EP-OBS-01/02/06 | OTel 接入任务与进度 |
| [`observability/grafana-aranea.json`](../observability/grafana-aranea.json) | 已落地的 Prometheus / Grafana 仪表盘 |
