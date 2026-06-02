# 2026-05-20 — 迭代 6：Knowledge Rerank · Evaluation 人工标注 · Trace 瀑布图

## 摘要

落实 execution-plan 当前焦点：检索 Rerank、评估人工标注、Monitor Trace Span 树与瀑布图 UI。

## 变更

### KN-01 Knowledge Rerank

- `internal/knowledge/reranker_factory.go`：接入 trpc-agent-go `topk` / `cohere` / `infinity`（`KRATOS_KNOWLEDGE_RERANKER`）
- `internal/knowledge/retriever.go`：向量 oversample + Rerank
- `api/kratos/knowledge/v1/knowledge.proto`：`use_rerank`、`rerank_candidates`
- `internal/service/knowledge_retriever.go` + Wire；`knowledge_search` / Chat / Team 注入 `WithRetriever`

### EVAL-02 人工评估

- `evaluation.proto`：`human_pass` / `human_score` / `human_comment` + `AnnotateCaseResult` RPC
- `eval_case_results` 表迁移列 + `UpdateCaseResultAnnotation`
- `EvaluationResultsDialog.vue`：Pass/Fail、分数、评语与保存

### I6-TEL-02 Trace 瀑布图

- `internal/service/turn_spans.go` + `turn_usage.go`：turn 级 Span 写入 `model_token_usage_events.metadata_json.spans`
- `trpc_turn.go` 包装事件流并 `recordTurnUsage`；默认关闭 runner_completion 重复 usage（`CHAT_RECORD_RUNNER_USAGE=1` 可恢复）
- **Usage 口径**：`chat_turn`（含 spans）由 `trpc_turn` 写入；`CHAT_RECORD_USAGE_INGRESS` 控制入口双写；`CHAT_RECORD_RUNNER_USAGE` 恢复 EventBus runner_completion 行

### 优化（review 跟进）

- 评估 `AnnotateCaseResult`：`human_comment` 改为 optional，仅传字段时 PATCH；404 映射
- Rerank 失败回退向量序（静默降级，见下方「后续优化」）
- Turn 全失败路径统一 `turnStatus=error`；usage 从 Turn 开始即记录
- `recordTurnUsage` 失败走 **FlowLogger** `chat.usage_record`（不用 slog，对齐 [Agent-No-Response-Debug-And-FlowLogger](./2026-05-20-Agent-No-Response-Debug-And-FlowLogger.md)）
- `TraceWaterfall.vue` + `TraceList.vue` 瀑布条

### 后续优化计划（🚧 未排期）

> 与 FlowLogger / 可观测口径对齐；细节见 [execution-plan.md](../guides/execution-plan.md) 迭代 6 备注、[24-telemetry-development.md](../需求/24-telemetry-development.md) §6。

| 优先级 | 项 | 说明 |
|--------|-----|------|
| P1 | **Span 语义** | 合并流式重复 `llm.call`；`tool.call` 绑 tool_result 时长，避免瀑布图瞬时条 |
| P1 | **Usage 三路径文档化** | 运维手册：`chat_turn` / `CHAT_RECORD_USAGE_INGRESS` / `CHAT_RECORD_RUNNER_USAGE` 矩阵与推荐组合 |
| P2 | **Rerank 可观测** | `KnowledgeService.Search` 在 rerank 降级时 `flow` 或 monitor 事件（`knowledge.rerank_fallback`），禁止在 `internal/knowledge` 用 slog |
| P2 | **Team / Native Trace** | `runner_team_trpc` / `chat_native` 复用 `TurnSpanCollector` + `recordTurnUsage` |
| P2 | **Eval 权限** | `AnnotateCaseResult` 写权限与审计日志 |
| P3 | **OTel 与 usage spans** | `chat.turn` OTel span 与 `metadata_json.spans` 关联字段；采样策略（TEL-01） |
| P3 | **TraceList 文案** | 详情标题「Span 瀑布图」与 UTF-8 校验 |

## 验证

```bash
make api && make wire && make build && make test
cd web && pnpm build
```
