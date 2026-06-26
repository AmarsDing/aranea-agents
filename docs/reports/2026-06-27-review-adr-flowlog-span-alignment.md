# ADR-05: FlowLog 与 OTel Span 对齐（Phase 1）

## 状态：已接受

## 背景

项目存在三条独立的跟踪能力（见 [52-flow-logger.md §2](../development/52-flow-logger.md#2-与-tracing-的关系能否用-tracing-作为-monitor-日志输出)）：

1. **OTel Tracing**（`internal/telemetry/turntrace/`）— Jaeger/Tempo 跨服务排障
2. **Turn Span**（`internal/event/span_collector.go`）— Usage metadata 瀑布图
3. **FlowLogger v2**（`internal/event/flow_log.go`）— Monitor WS 流程日志 + 可选落库

需求 [§2.3](../development/52-flow-logger.md#23-与用-tracing-作为-monitor-logs-输出的直接回答) 明确要求"FlowLog 条目内嵌 `span_id` / `parent_span_id`，与 OTel TraceID 对齐"，但实现侧 `FlowLogEntry` schema 缺失这两个字段，导致：

- 用户在 Monitor 看到 FlowLog 后，无法跳转到 Jaeger 查看同链路 OTel span
- 三套跟踪系统各自为政，AI 排障包（JSONL）缺少 OTel 关联键
- `SetOtelRefs(traceID, rootSpanID)` 仅写入 `UsageAggregator`（usage metadata），未回流到 FlowLog

## 决策

**Phase 1（本次）**：在 `FlowLogEntry` schema 添加 `SpanID` / `ParentSpanID` 字段，复用既有 `SetOtelRefs` 调用点注入 root span ID。

### 实现要点

1. `internal/event/flow_log.go`：`FlowLogEntry` 新增 `SpanID` / `ParentSpanID`（json tag `span_id` / `parent_span_id`，`omitempty`）；`toMetadata()` 仅在非空时输出；`newFlowLogEntry` 签名增加 `rootSpanID` 参数。
2. `internal/event/flow_tracker.go`：`FlowTracker` 新增 `rootSpanID` 字段；`SetOtelRefs(traceID, rootSpanID)` 扩展为同时存储到 `ft.rootSpanID`；`emit()` 传递 `ft.rootSpanID` 到 `newFlowLogEntry`。
3. 调用点（既有，无需改动）：
   - `internal/service/chat_orchestrator_turn.go:278` — `emitter.SetOtelRefs(traceBridge.TraceID(), traceBridge.RootSpanID())`
   - `internal/team/runner_team_trpc_phases.go:103` — 同上
4. 持久化路径（既有，无需改动）：`event_bus_flow_log_consumer.go` 通过 `json.Marshal(ev.Metadata)` 将 `span_id` 写入 `payload_json`，无需独立 DB 列。
5. HTTP API（`monitor.proto` `FlowLogEntry`）：Phase 1 不新增 proto 字段，`span_id` 通过 `payload_json` 暴露。未来若需按 `span_id` 过滤，再扩展 proto。

### 语义

- `span_id` = OTel turn-root span ID（所有同 turn 的 FlowLog 共享同一 `span_id`）
- `parent_span_id` = turn-root 的上游 OTel parent span ID；Phase 1 留空，reserved for future per-step linkage
- 空 `span_id` 表示未配置 OTel（graceful degradation，不阻塞 FlowLog 发射）

## 后果

### 正面

- **跨系统对齐**：用户/AI 可通过 `span_id` 将 FlowLog 与 Jaeger trace 关联，实现"一次写入、多投影"架构（需求 §2.2）
- **向后兼容**：`omitempty` + 既有 `SetOtelRefs` 调用点，旧消费者无感知
- **零 DB 迁移**：`span_id` 落 `payload_json`，无需新增表列或索引
- **可观测性提升**：为 Phase 2（per-step span linkage）奠定基础

### 负面

- **粒度限制**：Phase 1 所有 FlowLog 共享 root `span_id`，无法区分 `chat.llm.invoke` 与 `chat.stream.consume` 的具体 OTel child span。需 Phase 2 在 `emit()` 中按 stepID 查询 `bridge.LLMSpanOtelID()` / `bridge.ToolSpanOtelID()` 实现 per-step 关联。
- **`newFlowLogEntry` 参数增至 11**：CS-B7 建议参数 ≤5，但属既有签名扩展，未恶化。后续可重构为 Option struct。

## 替代方案

### 方案 A：扩展 proto API，新增 `span_id` 平铺字段

- 优点：HTTP API 可直接按 `span_id` 过滤
- 缺点：需 `make api` 重新生成 + service 层映射 + 前端类型同步，工作量倍增；当前无 HTTP 按 `span_id` 过滤的实际需求（YAGNI）
- **未采纳**：Phase 1 通过 `payload_json` 已能满足可观测性需求

### 方案 B：将 `span_id` 加入 `TraceContext` 而非 `FlowTracker.rootSpanID`

- 优点：`TraceContext` 已是 correlation 数据载体
- 缺点：`TraceContext` 设计为不可变关联数据，`span_id` 属运行时动态注入（OTel span 在 `turntrace.Start` 时才创建），语义不符
- **未采纳**：保持 `TraceContext` 不可变语义

### 方案 C：Phase 1 直接实现 per-step span linkage

- 优点：一次到位，FlowLog 每个 step 独立 `span_id`
- 缺点：需建立 stepID → OTel span 映射表（如 `chat.llm.invoke` → `bridge.LLMSpanOtelID()`），且部分 FlowLog step 无对应 OTel span（如 `chat.user_msg_persist`），需 fallback 策略
- **未采纳**：分阶段推进，Phase 1 先解决跨系统对齐，Phase 2 再做 per-step

## 关联

- 需求：[52-flow-logger.md §2.3](../development/52-flow-logger.md#23-与用-tracing-作为-monitor-logs-输出的直接回答)
- 设计：[52-flow-logger.design.md §3.2](../development/52-flow-logger.design.md#32-flowlogentryflow_logv1)
- 代码：[internal/event/flow_log.go](../../internal/event/flow_log.go)、[internal/event/flow_tracker.go](../../internal/event/flow_tracker.go)
- 测试：`TestFlowLogEntry_carriesSpanIDFromOtelRefs`、`TestFlowLogEntry_emptySpanIDWhenOtelRefsNotSet`
