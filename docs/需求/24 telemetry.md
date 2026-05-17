# 24 Telemetry 遥测 — 需求规格（占位）

> **状态**：占位文档（2026-05-17 治理）。实际产品需求 / 验收标准待补；当前仅有设计文档 [`24 telemetry.design.md`](./24%20telemetry.design.md) 描述 Trace / Metrics / Logs 三大信号的技术方案。
>
> **代码现状**（与 `guides/execution-plan.md` §3 EP-OBS-01 对应）：
> - ✅ Prometheus `/metrics` 已通：`internal/server/metrics.go` + Grafana 仪表盘 `docs/observability/grafana-aranea.json`。
> - ✅ EventBus / Cron 已加业务指标；HTTP / gRPC Kratos 默认指标已开。
> - ❌ OpenTelemetry Trace 链路：尚未接入；OTLP Exporter / 项目内 Span 标签 / 跨服务 Trace 传播 全部缺失。
> - ❌ 结构化日志统一规范、采样策略、Trace ↔ Log 关联未实现。

---

## 1. 占位说明

本文档定位为 **Telemetry 产品需求基线**，但项目早期把 Telemetry 拆为两条线：
1. **指标 / 告警**：以 Prometheus + Grafana 落地，归口在 `guides/execution-plan.md` §3 EP-OBS-01。
2. **链路追踪**：未启动，由 EP-OBS-01 统一推进 OTel 接入。

后续如需补全本文档，建议章节：
- §2 用户故事（运维 / 开发 / 性能调优）
- §3 必备指标 / 必备 Trace 字段清单
- §4 Sampler 策略 / 数据保留期 / 告警阈值
- §5 与 `34 event-system.md` 的对接（Agent 事件 ↔ Trace Span）

---

## 2. 关联文档

| 文档 | 用途 |
|------|------|
| [`24 telemetry.design.md`](./24%20telemetry.design.md) | Telemetry 技术设计（Kratos 中间件 + OTLP Exporter） |
| [`34 event-system.md`](./34%20event-system.md) | Agent 事件系统现状对齐 |
| [`guides/execution-plan.md`](../guides/execution-plan.md) §3 EP-OBS-01 | OTel 接入立即可执行任务 |
| [`observability/grafana-aranea.json`](../observability/grafana-aranea.json) | 已落地的 Prometheus / Grafana 仪表盘 |
