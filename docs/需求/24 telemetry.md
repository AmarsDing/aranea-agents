# Telemetry 遥测

本文档描述 Aranea 可观测性（Telemetry）的产品边界、双轨架构与验收口径。技术方案见 [`24 telemetry.design.md`](./24%20telemetry.design.md)；实现差距与任务以 [`24-telemetry-development.md`](./24-telemetry-development.md) 为准。

---

## 0. 需求明确性评审与实现边界

### 0.1 Telemetry 要回答的问题

| 用户问题 | 观测面 | 数据来源 | 实现状态 |
|----------|--------|----------|----------|
| HTTP/gRPC 请求是否可追踪？ | 传输层 Trace | Kratos `tracing.Server()` + OTLP 导出 | ✅ |
| Chat 一轮耗时、Token、工具步骤？ | Runs + FlowLog | `TraceEmitter` → usage `metadata_json.spans` + WS `flow_log` | ✅ |
| 系统 Prometheus 指标是否可 scrape？ | Metrics | `/metrics` + `internal/metrics` | ✅ |
| trpc-agent-go 框架级 gen_ai 指标？ | OTLP Metrics | `internal/telemetry` → trpc `telemetry/metric` | ✅（需配置 endpoint） |
| Jaeger 里能否看到 LLM/Tool 细粒度 Span？ | OTel 自定义 Span | 当前仅 `chat.turn` 根 Span | 🟡 部分（EP-OBS-06 待深化） |
| Graph / Team 执行是否有 OTel Span？ | 分布式 Trace | — | ❌ |
| 高负载下 Trace 采样如何控制？ | Sampler | — | ❌ |
| 运维在 UI 看单次运行瀑布图？ | Monitor Runs Tab | `TraceList` + `TraceWaterfall` | ✅ |

### 0.2 双轨架构（产品口径）

| 轨道 | 技术 | 用途 | 配置 |
|------|------|------|------|
| **A — Prometheus 直出** | `internal/metrics` + `/metrics` | 业务 Counter/Histogram/Gauge、Grafana 大盘 | 无需 OTEL endpoint |
| **B — OTLP 导出** | `internal/telemetry.Init` | Trace + trpc 框架 Metrics 送 Collector/Jaeger | `OTEL_EXPORTER_OTLP_ENDPOINT` |
| **C — 应用内 Trace 投影** | `TraceEmitter` + `turn_trace.go` | FlowLog、Usage spans、可选 OTel `chat.turn` | 与 Chat/Team Run 热路径绑定 |

轨道 A/B/C **并行**，未配置 OTLP 时 B 为 noop，A/C 仍可用。

### 0.3 非目标

- 不在 Telemetry 模块内实现独立 Trace 存储 API（Runs 真相源为 `model_token_usage_events` + FlowLog WS）。
- 不恢复 SlogBridge / 业务路径 `slog`（见 [FlowLog v2 changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)）。
- Monitor **Runs** 展示的是 usage/Flow 投影，不是 Jaeger 全量 Span 镜像。

---

## 1. 用户故事（摘要）

| 角色 | 故事 | 验收 |
|------|------|------|
| SRE | 配置 OTEL Collector 后，可在 Jaeger 看到 API 请求 Trace | endpoint 生效 + Kratos tracing 中间件 |
| 开发 | Chat 失败时可从 Monitor Runs 看 Flow + Waterfall | `TraceList` 详情双 Tab |
| 运维 | Prometheus/Grafana 监控 Agent/Tool/Provider 指标 | `/metrics` + `grafana-aranea.json` |
| 平台 | Team/Graph 运行可关联同一 trace_id | FlowLog `correlation.trace_id` |

---

## 2. 必备信号（摘要）

### 2.1 Prometheus（轨道 A）

业务指标前缀 `aranea_`，完整清单见设计文档 §4.3 与 `internal/metrics/vars.go`。

### 2.2 OTLP（轨道 B）

- Trace：HTTP/gRPC Server 请求 Span。
- Metrics：trpc-agent-go `gen_ai.*` / `trpcgoagent_*`（Chat、ExecuteTool、InvokeAgent）。

### 2.3 应用 Trace（轨道 C）

- FlowLog：`correlation.trace_id` 与 OTel W3C TraceID 对齐（`internal/event/trace_context.go`）。
- Usage spans：`TraceEmitter.MetadataJSON()` → `recordTurnUsage`。
- OTel：`startTurnSpan("chat.turn", ...)`（`internal/service/turn_trace.go`）。

---

## 3. 验收标准（产品级）

- [x] 未配置 OTEL 时进程正常启动，零 OTLP 开销。
- [x] 配置 OTEL 后 Jaeger 可见 HTTP/gRPC 请求 Trace。
- [x] Prometheus 可 scrape `/metrics`。
- [x] Monitor Runs 可查看单次运行 Flow + Waterfall + Span JSON。
- [ ] Jaeger 可见 LLM 调用、工具调用等细粒度 OTel Span（当前仅根 Span）。
- [ ] 可配置 Trace 采样率。

---

## 4. 关联文档

| 文档 | 用途 |
|------|------|
| [`24 telemetry.design.md`](./24%20telemetry.design.md) | 架构分层、环境变量、指标清单 |
| [`24-telemetry-development.md`](./24-telemetry-development.md) | 开发计划、任务状态、代码锚点 |
| [`18 monitor.md`](./18%20monitor.md) | Runs / Logs Tab 产品口径 |
| [`52-flow-logger.md`](./52-flow-logger.md) · [design](./52-flow-logger.design.md) | TraceEmitter / FlowLog |
| [`guides/execution-plan.md`](../guides/execution-plan.md) | I6-TEL-01/02 等进度 |
| [`observability/grafana-aranea.json`](../observability/grafana-aranea.json) | Grafana 仪表盘 |
