# FlowLogger 流程日志 — 开发计划

> **版本**：2026-05-20 | **状态**：v2 Phase 1a/1b/3 ✅；SlogBridge 已移除（见 [changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)）；Phase 2 落库待做  
> **需求**：[52-flow-logger.md](./52-flow-logger.md) · **设计**：[52-flow-logger.design.md](./52-flow-logger.design.md)  
> **步骤注册表**：[52-flow-logger.design.md](./52-flow-logger.design.md) §5.1  
> **进度**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

**FlowLogger v2**：业务可观测「流程日志」——按 `trace_id` 聚合、severity 分级、人类可读 + AI 可解析；Turn 热路径统一 `TraceEmitter`，**不保留 v1**。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| 核心 | `internal/event/trace_context.go`、`flow_log.go`、`trace_emitter.go`、`system_flow.go` |
| 集成 | `internal/service/trpc_turn.go`、`turn_usage.go` |
| WS | `internal/server/ws.go`（`flow_log` 免 `enable_log`） |
| 前端 Logs | `web/src/features/monitor/flow.ts`、`api.ts`、`LogStream.vue` |
| 前端 Traces | `FlowTracePanel.vue`、`FlowLogExportButton.vue`、`TraceList.vue` |

---

## 2. 现状评估（2026-05-20）

| 项 | 状态 | 证据 |
|----|------|------|
| v1 FlowLogger 删除 | ✅ | 已移除 `flow_logger.go` |
| `EnvelopeTypeFlowLog` | ✅ | `envelope.go` |
| TraceEmitter + Span 合并 | ✅ | `trace_emitter.go`；`turn_spans.go` 已删 |
| `trpc_turn` 迁移 | ✅ | `NewTraceEmitterForRun` |
| Monitor Logs 收 `flow_log` | ✅ | `api.ts` + `LogStream.vue` |
| Traces 详情「流程」Tab | ✅ | `FlowTracePanel.vue` + `TraceList` Tab |
| JSONL 导出 | ✅ | `FlowLogExportButton.vue` + `buildFlowDiagnosticJsonl` |
| 详情按 trace 过滤 WS | ✅ | `flowLogMatchesTrace` + 详情页订阅 |
| `ListFlowLogs` 落库 | ❌ | Phase 2 |
| Team Runner `TraceEmitter` | ✅ | `runner_team_trpc.go`（迭代 7） |
| Knowledge rerank FlowLog | ✅ | `knowledge.rerank.fallback`（迭代 7） |
| EventBus 域（部分） | ✅ | 持久化/用量失败 `SessionSysLogError`（迭代 7） |
| 全项目 slog 移除 / SlogBridge 删除 | ✅ | [changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md) |
| 系统域 `SysLog*` 基础设施打点 | ✅ | `system_flow.go` + 步骤注册表 §Plugin/System |

---

## 3. 任务拆分

### Phase 1a — 后端 + Logs Tab（已完成）

| ID | 任务 | 状态 |
|----|------|------|
| FL-1a-01 | `trace_context.go` | ✅ |
| FL-1a-02 | `flow_log.go` + `EnvelopeTypeFlowLog` | ✅ |
| FL-1a-03 | `trace_emitter.go` + 单测 | ✅ |
| FL-1a-04 | `trpc_turn` 接 Emitter，删 TurnSpanCollector | ✅ |
| FL-1a-05 | `turn_usage` 用 `MetadataJSON()` | ✅ |
| FL-1a-06 | Chat 步骤 + 中文 title | ✅ |
| FL-1a-07 | WS `flow_log` 免 enable_log | ✅ |
| FL-1a-08 | `flow.ts` + `LogStream` 展示流程日志 | ✅ |
| FL-1a-09 | 删除 `slog_bridge.go`，`internal/` slog→FlowLog | ✅ |
| FL-1a-10 | `SetGlobalBus` + `system_flow.go` | ✅ |

### Phase 1b — Traces 详情 + 导出（已完成）

| ID | 任务 | 状态 |
|----|------|------|
| FL-1b-01 | `FlowTracePanel.vue` | ✅ |
| FL-1b-02 | `TraceList.vue` Tab「流程 \| 瀑布图 \| Span 树」 | ✅ |
| FL-1b-03 | `FlowLogExportButton.vue` | ✅ |
| FL-1b-04 | 详情按 `trace_id`/`run_id` 过滤 WS | ✅ |
| FL-1b-05 | `flow.spec.ts` | ✅ |

### Phase 2 — 持久化（可选）

| ID | 任务 |
|----|------|
| FL-2-01 | `docs/sql/15_flow_log.sql` + data repo |
| FL-2-02 | `monitor.proto` `ListFlowLogs` |
| FL-2-03 | Traces/Logs HTTP 拉历史 |

### Phase 3 — 扩展域（已完成 — 2026-05-20 迭代 7）

| ID | 任务 | 状态 |
|----|------|------|
| FL-3-01 | `runner_team_trpc.go` TraceEmitter | ✅ |
| FL-3-02 | Knowledge rerank fallback → FlowLog | ✅ |
| FL-3-03 | `event_bus` 用量失败 → `SessionSysLogError` | ✅ |
| FL-3-04 | `chat_native` 统一步骤 ID（`chat.turn.enter`） | ✅ |

---

## 4. 实施顺序（后续 AI 施工）

1. 按需 **Phase 2** 落库（`ListFlowLogs`）。  
2. **Phase 3**：Team `TraceEmitter`、Knowledge rerank FlowLog、`event_bus.runner.completion`。

---

## 5. 验证命令

```bash
go test ./internal/event/... -count=1
go build ./...
cd web && pnpm lint && pnpm build
```

**手动**：

1. 发 Chat → Monitor **Logs** → 见中文流程行（无需开进程日志）。  
2. 失败 Turn → 见 `critical`/`error` 红色样式。  
3. Traces 详情 → **流程** Tab 见实时 flow_log；**导出 JSONL** 供 AI 排障。

---

## 6. PR 红线

- [x] Turn 热路径无 `slog`（`trpc_turn` / `turn_usage`）  
- [x] `slog_bridge.go` 已删除；`LOG_BRIDGE_*` 已废弃  
- [ ] 无 `TurnSpanCollector` / v1 `flow_step` 双写  
- [ ] `internal/biz` 无 `trpc-agent-go` import  
- [ ] Monitor 仍 6 个顶层 Tab  
- [ ] 进程 `log` 与 `flow_log` 前端分流  

---

## 7. 验收清单（v2 发布）

- [x] `flow_log` WS 推送 + Logs Tab 中文展示  
- [x] Usage `metadata` 含 `trace_id` + `spans`  
- [x] Traces 详情「流程」Tab  
- [x] JSONL 导出供 AI 排障  
- [ ] HTTP 按 `trace_id` 查历史（Phase 2）  
- [ ] 业务用户无需读 step_id 即可理解全流程  
