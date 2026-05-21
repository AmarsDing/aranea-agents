# Telemetry — 实现与文档同步（2026-05-21）

## 摘要

Telemetry 三份文档与代码对齐：`internal/telemetry` 为 OTLP 唯一入口；Monitor **Runs** Tab 为应用内 Trace UI；修正 development 文档内矛盾状态；修复 gRPC Tracer `service.name` 误用 `resource.SchemaURL()` 的缺陷。

## 代码变更

| 项 | 路径 | 说明 |
|----|------|------|
| gRPC service.name 修复 | `internal/telemetry/telemetry.go` | `atrace.WithServiceName/WithServiceVersion` 传入 admin 服务名 |
| 职责拆分 | 同上 | `initTracerProvider` / `shutdownAll` / `newServiceResource` |
| shutdown | 同上 | `errors.Join` 聚合 Tracer + Meter 关闭错误 |
| 单测 | `internal/telemetry/telemetry_test.go` | noop 路径（无 endpoint） |

## 文档变更

- `docs/需求/24 telemetry.md` — 由占位升级为产品边界 + 双轨架构 §0
- `docs/需求/24 telemetry.design.md` — 路径 `internal/telemetry`、FlowLog 关联、Monitor Runs 组件
- `docs/需求/24-telemetry-development.md` — 消除 §2/§3 矛盾；I6-TEL-01/02 ✅；任务 #7 gRPC 修复 ✅
- `docs/README.md` — 技术栈观测行 + §5.2 Telemetry 索引
- `docs/需求/README-development.md` — Telemetry 接入度

## P2–P3 Review 修复（2026-05-21 续）

| 项 | 修复 |
|----|------|
| P0 Bridge 并发 | `sync.Mutex`；`Finish` 幂等 |
| P0 Tool 双关 | stream 只 open；hook `RecordToolCallEnd` / `CompleteToolCall` 唯一 close |
| P1 Graph 分层 | OTel 移至 `GraphService`；biz `WithGraphExecutionComplete` 回调 |
| P1 Graph Finish | 按 `exec.Status` 传 err；SaveRun 失败也回调 |
| P1 Team | `TraceDomainTeam` + `WrapFrameworkEvents` + member metadata |
| P2 依赖 | 删除 `turntrace/wrap.go` → `event.WrapFrameworkEvents` |
| P2 DRY | `event.ToolNameFromResponse` |
| P3 采样 | 文档标明 HTTP ✅ / gRPC ❌ |

## P2–P3 架构 polish（2026-05-21 续 2）

| 项 | 修复 |
|----|------|
| TraceEmitterOpts | `NewTraceEmitterForRun(TraceEmitterOpts)` 替代 7  positional 参数 |
| Graph observer Wire | `biz.GraphExecutionObserver` + `GraphExecutionTelemetry` Wire 注入；删除 `WithGraphExecutionComplete` context.Value |
| Graph run_id | Service 预生成 `execID`，OTel `run_id` 与 `exec.ID` 一致 |
| 删除 deprecated | 移除 `WrapEventsWithTraceEmitter`；文档改用 `WrapFrameworkEvents` |
| Wire | `ProvideGraphUsecase` 替代 `biz.NewGraphUsecase` 直出 |
| 项 | 路径 |
|----|------|
| OTel turn 桥 | `internal/telemetry/turntrace/`（Bridge + WrapEvents） |
| Chat 细粒度 Span | `trpc_turn` → `llm.call` / `chat.llm.invoke` / tool hook |
| usage span 语义 | `trace_emitter`：`mergeLLMSpan`、`CompleteToolCall` |
| Graph Span | `biz/graph.go` → `graph.execute` |
| Team metadata | `usage_record.go` + `runner_team_trpc` OTel refs |
| 采样 | `telemetry/sampler.go` + HTTP `WithSampler` |
| OTel 关联字段 | `metadata_json.otel_trace_id` / `otel_root_span_id` |


- Rerank 降级 monitor 事件（见 Iteration6 changelog）
- gRPC TracerProvider 采样（当前 HTTP 路径已支持 sampler）
- 各 usage span 行写入 `otel_span_id`（当前仅 root 关联）
