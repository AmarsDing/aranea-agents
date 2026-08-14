# Telemetry 遥测 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 Prometheus + OTLP + Runs UI + 细粒度 Span/采样 ✅；🟡 Jaeger 全链路 UI 非目标；🟡 gRPC 采样待 trpc 扩展；🔴 Langfuse 构造函数已就绪但未接入 Wire
> **需求**：[24-telemetry.md](./24-telemetry.md) · **设计**：[24-telemetry.design.md](./24-telemetry.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)（I6-TEL-01/02）· **Monitor UI**：[18-monitor-development.md](./18-monitor-development.md)

---

## 1. 模块定位

基于 OpenTelemetry + Prometheus 的可观测性：传输层 Trace、业务 Metrics、Chat/Team/Graph Run 的 FlowLog/Usage spans 投影。

**代码锚点**：

| 层 | 路径 |
|----|------|
| OTLP 初始化 | `internal/telemetry/telemetry.go` |
| 采样器 | `internal/telemetry/sampler.go` |
| 传输中间件 | `internal/server/http.go`、`internal/server/grpc.go` |
| Prometheus | `internal/metrics/vars.go`、`internal/metrics/callback.go` |
| OTel Turn Bridge | `internal/telemetry/turntrace/bridge.go` |
| Chat OTel Span | `internal/service/turn_trace.go`、`internal/service/chat_orchestrator_turn.go` |
| Graph OTel Span | `internal/service/graph_execution_service.go`、`internal/service/graph_telemetry.go` |
| 应用 Trace | `internal/event/trace_context.go`、`trace_emitter.go`、`span_collector.go` |
| 框架事件 | `internal/event/framework_events.go` |
| Usage 投影 | `internal/service/chat_orchestrator_turn_metrics.go`（`recordTurnUsage`）、`internal/service/turn_usage.go`（薄封装） |
| Team Run | `internal/team/runner_team_trpc.go` |
| 前端 Runs | `web/src/components/monitor/TraceList.vue`、`TraceWaterfall.vue` |
| 启动 | `cmd/admin/main.go` → `telemetry.Init` |
| Wire 注入 | `cmd/admin/wire.go`（`wireApp`） |
| FlowLogger | [52-flow-logger-development.md](./52-flow-logger-development.md) |

---

## 2. 现状评估

### 2.1 需求问题实现状态

> 对应需求文档 §0.1 的用户问题。

| 用户问题 | 实现状态 | 证据 |
|----------|----------|------|
| HTTP/gRPC 请求是否可追踪？ | ✅ | `http.go` / `grpc.go` → `tracing.Server()` |
| Chat 一轮耗时、Token、工具步骤？ | ✅ | `TraceEmitter` → usage `metadata_json.spans` + WS `flow_log` |
| 系统 Prometheus 指标是否可 scrape？ | ✅ | `/metrics` + `internal/metrics` |
| trpc-agent-go 框架级 gen_ai 指标？ | ✅（需配置 endpoint） | `internal/telemetry` → trpc `telemetry/metric` |
| Jaeger 里能否看到 LLM/Tool 细粒度 Span？ | ✅ | `turntrace.Bridge` → `llm.call` / `tool.call` 子 Span |
| Graph / Team 执行是否有 OTel Span？ | ✅ | `graph.execute` / `team.run` 根 Span + 子 Span |
| 高负载下 Trace 采样如何控制？ | ✅ HTTP / ❌ gRPC | `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG`（HTTP only） |
| Langfuse Trace 是否可导出？ | 已移除 | `internal/telemetry/langfuse.go` 从未接入 Wire（零生产引用），2026-08-14 死代码清理删除 |
| 运维在 UI 看单次运行瀑布图？ | ✅ | `TraceList` + `TraceWaterfall` |

### 2.2 组件实现状态

| 项 | 状态 | 证据 |
|----|------|------|
| HTTP OTEL 中间件 | ✅ | `http.go` → `tracing.Server()` |
| gRPC OTEL 中间件 | ✅ | `grpc.go` → `tracing.Server()` |
| Trace 传播 | ✅ | W3C TraceContext + Baggage |
| OTLP Trace HTTP | ✅ | `initHTTPTracerProvider` |
| OTLP Trace gRPC | ✅ | `initGRPCTracerProvider` → trpc `telemetry/trace`（service name/version 正确传入） |
| OTLP Metrics | ✅ | `initMeterProvider` → trpc `telemetry/metric` |
| Prometheus `/metrics` | ✅ | `metrics/vars.go` + `callback.go` + Route 注册 |
| FlowLog trace_id | ✅ | `NewTraceContext` + OTel 中间件 |
| OTel Chat 根 Span | ✅ | `startTurnSpan("chat.turn")` |
| Usage spans + Waterfall | ✅ | `TraceEmitter` + `TraceWaterfall.vue` |
| Monitor Runs UI | ✅ | `TraceList` 列表 + 详情 Flow/Waterfall/Span |
| `chat.usage_record` Flow 步骤 | ✅ | 落库失败可见于 monitor flow |
| OTel LLM/Tool 细粒度 Span | ✅ Chat/Team | `turntrace.Bridge` + `ObserveFrameworkEvent` + `RecordToolCallEnd` |
| Graph OTel Span | ✅ Service 层 | `graph_execution_service.go` + `GraphExecutionTelemetry` biz 回调 |
| Team usage spans 投影 | ✅ | turn + member metadata |
| Trace 采样策略 | ✅ HTTP / ❌ gRPC | `sampler.go` + `OTEL_TRACES_SAMPLER*`（HTTP only） |
| OTel ↔ usage 关联 | ✅ root + child | `otel_trace_id` / `otel_root_span_id` + 各 span `otel_id` |
| Langfuse 导出 | 已移除 | `langfuse.go` 从未接入 Wire（零生产引用），2026-08-14 死代码清理删除 |
| OTel ↔ usage span ID 同步 | ✅ | `SyncOtelSpanIDs` → `span_collector.go` |

---

## 3. 差距与优化

| 优先级 | 项 | 说明 |
|--------|-----|------|
| P1 | gRPC Tracer service.name 修复 | ✅ 2026-05-21：不再误用 `resource.SchemaURL()` |
| P2 | OTel 细粒度 Span | ✅ Chat/Team（`turntrace.Bridge` + `RecordToolCallEnd` 唯一关闭路径） |
| P2 | Graph 执行 Span | ✅ Service 层 + `GraphExecutionTelemetry` biz 完成回调 |
| P2 | 流式 Span 合并、tool 真实耗时 | ✅ `RecordToolCallEnd` 唯一关闭路径 |
| P3 | Trace 采样 | ✅ HTTP（`sampler.go`）；gRPC 待 trpc 暴露 Sampler |
| P3 | OTel ↔ usage 关联 | ✅ root span id + child span `otel_id` |
| P3 | Team usage parity | ✅ turn + member metadata |
| P3 | Langfuse 导出 | 🔴 构造函数已就绪，**未接入 `wireApp`**，需注册 `NewLangfuseRuntime` 并管理 shutdown 生命周期 |
| P3 | OTel ↔ usage span ID 同步 | ✅ `SyncOtelSpanIDs` + `span_collector.go` |
| — | gRPC exporter 采样 | ❌ 依赖 trpc `telemetry/trace` 暴露 Sampler 配置 |

---

## 4. 开发阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | OTLP Trace/Metrics + Prometheus + FlowLog trace_id | ✅ |
| Phase 1b | Chat OTel 根 Span | ✅ |
| Phase 1c | Runs Waterfall + usage spans | ✅ |
| Phase 2 | LLM/Tool/Graph OTel 细粒度 Span（`turntrace.Bridge`） | ✅ |
| Phase 3 | Trace 采样（HTTP `sampler.go`） | ✅ |
| Phase 4 | Langfuse 导出 + OTel ↔ usage span ID 同步 | 🟡 span ID 同步 ✅；Langfuse 🔴 未接入 Wire |

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 状态 |
|---|------|--------|-----|------|
| 1 | OTLP Trace 导出（HTTP + gRPC） | P1 | EP-OBS-02 | ✅ |
| 2 | OTLP Metrics 导出（trpc 框架指标） | P2 | EP-OBS-02 | ✅ |
| 3 | Prometheus `/metrics` | P0 | EP-OBS-01 | ✅ |
| 4 | Log-Trace 关联（TraceContext + FlowLog） | P2 | EP-OBS-05 | ✅ |
| 5 | `/metrics` Route 注册（绕过 auth） | P1 | R4/R12 | ✅ |
| 6 | `telemetry` 包独立 + `Init` 入口 | P1 | 架构 | ✅ |
| 7 | gRPC Tracer `service.name` / `service.version` | P1 | 质量 | ✅ 2026-05-21 |
| 8 | Chat OTel 根 Span `chat.turn` | P2 | I6-TEL-01 | ✅ |
| 9 | Monitor Runs + Waterfall | P2 | I6-TEL-02 | ✅ |
| 10 | Chat/Team LLM/Tool OTel Span（`turntrace.Bridge`） | P2 | EP-OBS-06 | ✅ |
| 11 | Graph 执行 OTel Span | P2 | EP-OBS-06 | ✅ |
| 12 | Trace 采样策略（HTTP `sampler.go`） | P3 | — | ✅ |
| 13 | Span 语义 / Team parity | P2 | — | ✅ |
| 14 | ~~Langfuse 导出（`langfuse.go` + Wire 注入）~~ | P3 | — | 已关闭：从未接入 Wire，文件 2026-08-14 死代码清理删除 |
| 15 | OTel ↔ usage span ID 同步（`SyncOtelSpanIDs`） | P3 | — | ✅ |

---

## 6. 验收标准

> 产品级验收标准见需求文档 §3。以下为开发验证口径与完成状态。

- [x] `OTEL_EXPORTER_OTLP_ENDPOINT` 设置后 Jaeger 可见 HTTP/gRPC 请求 Trace
- [x] OTLP Metrics 导出 trpc-agent-go 框架指标
- [x] Prometheus scrape `/metrics`
- [x] FlowLog / Usage 含一致 `trace_id`
- [x] Monitor Runs 详情可查看 Flow + Waterfall + Span JSON
- [x] gRPC 协议下 Jaeger `service.name` 为 admin 服务名（非 schema URL）
- [x] Jaeger 可见 LLM/Tool 等细粒度 OTel Span（`llm.call` / `tool.call`）
- [x] Graph / Team 执行有 OTel Span（`graph.execute` / `team.run`）
- [x] 可配置 Trace 采样率（HTTP exporter；`OTEL_TRACES_SAMPLER`）
- [ ] Langfuse 导出可用（需配置 `conf.Bootstrap.Langfuse`）— 🔴 `NewLangfuseRuntime` 未接入 `wireApp`
- [x] OTel ↔ usage 关联含 `otel_trace_id` / `otel_root_span_id` + 各 span `otel_id`
- [ ] gRPC exporter 采样（依赖 trpc `telemetry/trace` 扩展）

**验证命令**：

```bash
go test ./internal/telemetry/...
make build && make test
cd web && pnpm lint && pnpm test && pnpm build
```

**手工**：配置 OTEL → Chat 一轮 → Jaeger 见 API + `chat.turn` → Monitor Runs 见 Waterfall。

---

## 7. 依赖与风险

- 开发环境 OTEL：参考 `pkg/trpc-agent-go/examples/telemetry/jaeger-prometheus/docker-compose.yaml`
- 细粒度 Span 需配合采样，避免高 QPS 存储压力
- Runs UI 与 Jaeger 职责分离：产品排障以 Runs/Flow 为主，Jaeger 供 SRE 全链路
- **Langfuse 接入风险**：`NewLangfuseRuntime` 构造函数已就绪但 `wireApp` 未注册，当前为死代码；接入时需在 `cmd/admin/wire.go` 注册 provider 并在 `cmd/admin/main.go` 或 `app.go` 管理 shutdown 生命周期
