# 2026-05-20 — 迭代 7：项目优化计划执行（FlowLogger Phase 3 · 评估报告导出）

## 摘要

按 [docs/README.md](../README.md) 与 [execution-plan.md](../guides/execution-plan.md) 当前冲刺焦点，完成 **FlowLogger v2 Phase 3** 可观测补齐、**Evaluation 报告导出（前端）**，并同步进度真相文档。

## 优化计划（文档驱动）

| 优先级 | 项 | 状态 | 说明 |
|--------|-----|------|------|
| P1 | FlowLogger Phase 3（FL-3-01～04） | ✅ | Team TraceEmitter、Rerank fallback、EventBus 错误级、chat 步骤 ID |
| P2 | Evaluation 报告导出（EVAL-02 余项） | ✅ | 结果对话框 CSV/JSON 客户端导出 |
| P2 | 文档 §8 与 execution-plan 同步 | ✅ | 标记已完成 P1/P2 项 |
| P3 | FlowLogger Phase 2 落库 | ⏳ | `ListFlowLogs` + SQL，下一迭代 |
| P3 | Knowledge OCR | ⏳ | 见 [37-knowledge-development.md](../需求/37-knowledge-development.md) |
| P3 | A2A 远程发现 | ⏳ | 见 [26-a2a-development.md](../需求/26-a2a-development.md) |
| P3 | Span 语义 / Usage 三路径手册 | ⏳ | 见 [Iteration6](./2026-05-20-Iteration6-TRACE-EVAL-KN.md) §后续优化 |

## 代码变更

### FlowLogger Phase 3

- **FL-3-01** `internal/team/runner_team_trpc.go`：`NewTraceEmitterForRun` + `team.run.start/build/execute/finish`
- **FL-3-02** `internal/knowledge/retriever.go`：Rerank 失败 → `knowledge.rerank.fallback`（SysLogWarn，无 slog）
- **FL-3-03** `internal/event/system_flow.go`：`SessionSysLogError`；EventBus 持久化/用量失败改为 error 级
- **FL-3-04** `internal/service/chat_native.go`：`chat.turn_enter` → `chat.turn.enter`；步骤注册表补别名

### Evaluation 报告导出

- `web/src/features/evaluation/exportRunResults.ts`
- `EvaluationResultsDialog.vue`：导出 CSV / JSON 按钮

## 验证

```bash
go test ./internal/knowledge/... ./internal/event/... ./internal/biz/... ./internal/team/... -count=1
go build ./...
cd web && pnpm test
```

## 文档

- [execution-plan.md](../guides/execution-plan.md) — 迭代 7 任务板
- [52-flow-logger-development.md](../需求/52-flow-logger-development.md) — Phase 3 ✅
- [33-evaluation-development.md](../需求/33-evaluation-development.md) — 报告导出 ✅
- [README-development.md](../需求/README-development.md) — 进度快照
