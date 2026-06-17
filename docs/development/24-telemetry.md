# Telemetry 遥测

本文档描述 Aranea 可观测性（Telemetry）的产品边界、四轨架构与验收口径。技术方案见 [`24-telemetry.design.md`](./24-telemetry.design.md)；实现差距与任务以 [`24-telemetry.development.md`](./24-telemetry.development.md) 为准。

---

## 0. 需求明确性评审与实现边界

### 0.1 Telemetry 要回答的问题

| 用户问题 | 观测面 |
|----------|--------|
| HTTP/gRPC 请求是否可追踪？ | 传输层 Trace |
| Chat 一轮耗时、Token、工具步骤？ | Runs + FlowLog |
| 系统 Prometheus 指标是否可 scrape？ | Metrics |
| trpc-agent-go 框架级 gen_ai 指标？ | OTLP Metrics |
| Jaeger 里能否看到 LLM/Tool 细粒度 Span？ | OTel 自定义 Span |
| Graph / Team 执行是否有 OTel Span？ | 分布式 Trace |
| 高负载下 Trace 采样如何控制？ | Sampler |
| Langfuse Trace 是否可导出？ | Langfuse |
| 运维在 UI 看单次运行瀑布图？ | Monitor Runs Tab |

> 数据来源（代码路径与组件）见设计文档 §4；实现状态（✅/❌）见开发计划 §2。

### 0.2 四轨架构（产品口径）

| 轨道 | 技术 | 用途 | 配置 |
|------|------|------|------|
| **A — Prometheus 直出** | Prometheus client | 业务 Counter/Histogram/Gauge、Grafana 大盘 | 无需 OTEL endpoint |
| **B — OTLP 导出** | OpenTelemetry SDK | Trace + trpc 框架 Metrics 送 Collector/Jaeger | `OTEL_EXPORTER_OTLP_ENDPOINT` |
| **C — 应用内 Trace 投影** | TraceEmitter + turntrace Bridge | FlowLog、Usage spans、OTel `chat.turn` / `team.run` / `graph.execute` | 与 Chat/Team/Graph Run 热路径绑定 |
| **D — Langfuse 导出** | trpc-agent-go telemetry/langfuse | LLM Trace 导出至 Langfuse 平台 | `conf.Bootstrap.Langfuse` |

轨道 A/B/C/D **并行**，未配置 OTLP 时 B 为 noop，未配置 Langfuse 时 D 为 noop，A/C 仍可用。

> 各轨道的代码分层、初始化入口与组件职责见设计文档 §4。

### 0.3 非目标

- 不在 Telemetry 模块内实现独立 Trace 存储 API（Runs 真相源为 `model_token_usage_events` + FlowLog WS）。
- 不恢复 SlogBridge / 业务路径 `slog`（见 [FlowLog v2 changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)）。
- Monitor **Runs** 展示的是 usage/Flow 投影，不是 Jaeger 全量 Span 镜像。

---

## 1. 用户故事

| 角色 | 故事 | 验收 |
|------|------|------|
| SRE | 配置 OTEL Collector 后，可在 Jaeger 看到 API 请求 Trace | endpoint 生效 + Kratos tracing 中间件 |
| 开发 | Chat 失败时可从 Monitor Runs 看 Flow + Waterfall | `TraceList` 详情双 Tab |
| 运维 | Prometheus/Grafana 监控 Agent/Tool/Provider 指标 | `/metrics`（Grafana 仪表盘需用户自建） |
| 平台 | Team/Graph 运行可关联同一 trace_id | FlowLog `correlation.trace_id` |

---

## 2. 必备信号

### 2.1 Prometheus（轨道 A）

业务指标前缀 `aranea_`，覆盖 Chat 延迟、Agent 构建缓存、EventBus、Graph 执行、工具调用、LLM Provider、插件、自动记忆、Artifact、MCP、告警、模型路由、Channel、Team/Graph 运行时、Skill 导入、SafeGo panic 恢复、回调等域。

> 完整指标清单（名称/类型/标签/用途）见设计文档 §4.3。

### 2.2 OTLP（轨道 B）

- Trace：HTTP/gRPC Server 请求 Span。
- Metrics：trpc-agent-go `gen_ai.*` / `trpcgoagent_*`（Chat、ExecuteTool、InvokeAgent）。

> 框架指标清单见设计文档 §4.4。

### 2.3 应用 Trace（轨道 C）

- FlowLog：`correlation.trace_id` 与 OTel W3C TraceID 对齐。
- Usage spans：含 `otel_trace_id` / `otel_root_span_id`，投影至 `metadata_json.spans`。
- OTel：`turntrace.Bridge` 管理 `chat.turn` / `team.run` / `graph.execute` 根 Span + `llm.call` / `tool.call` 子 Span。

> 投影机制与组件交互见设计文档 §4.5。

### 2.4 Langfuse（轨道 D）

- 配置：`conf.Bootstrap.Langfuse`（Enable / PublicKey / SecretKey / BaseUrl / FlushIntervalMS）。
- 用途：将 LLM 调用 Trace 导出至 Langfuse 平台进行可视化分析。
- 生命周期：设计为独立于 OTLP shutdown。

> 配置字段定义与运行时实现见设计文档 §4.1 与 §6。

---

## 3. 验收标准（产品级）

- 未配置 OTEL 时进程正常启动，零 OTLP 开销。
- 配置 OTEL 后 Jaeger 可见 HTTP/gRPC 请求 Trace。
- Prometheus 可 scrape `/metrics`。
- Monitor Runs 可查看单次运行 Flow + Waterfall + Span JSON。
- Jaeger 可见 LLM 调用、工具调用等细粒度 OTel Span（`llm.call` / `tool.call`）。
- Graph / Team 执行有 OTel Span（`graph.execute` / `team.run`）。
- 可配置 Trace 采样率（HTTP exporter；`OTEL_TRACES_SAMPLER`）。
- Langfuse 导出可用（需配置 `conf.Bootstrap.Langfuse`）。
- gRPC exporter 采样（依赖 trpc `telemetry/trace` 扩展）。

> 各项验收的实现完成状态见开发计划 §6。

---

## 4. 关联文档

| 文档 | 用途 |
|------|------|
| [`24-telemetry.design.md`](./24-telemetry.design.md) | 架构分层、环境变量、指标清单 |
| [`24-telemetry.development.md`](./24-telemetry.development.md) | 开发计划、任务状态、代码锚点 |
| [`18-monitor.md`](./18-monitor.md) | Runs / Logs Tab 产品口径 |
| [`52-flow-logger.md`](./52-flow-logger.md) · [design](./52-flow-logger.design.md) | TraceEmitter / FlowLog |
| [`guides/execution-plan.md`](../guides/execution-plan.md) | I6-TEL-01/02 等进度 |
