# Graph Phase A–D — P1–P3 优化闭合

> **依据**：[`docs/README.md`](../README.md) · [`docs/review/README.md`](./README.md) · [`36-graph-development.md`](../需求/36-graph-development.md)  
> **审查时间**：2026-05-23 · **优化闭合**：2026-05-23（P1–P3）

---

## 综合评级（Phase A–D 后）

| 指标 | 结果 |
|------|------|
| **总分** | **92 / 100**（较 Phase A–D 初评 87 提升） |
| **风险等级** | **P2**（Agent/Router 节点 builder、E2E 手工验收） |

---

## 优化闭合项

| ID | 优先级 | 状态 | 落点 |
|----|--------|------|------|
| GRAPH-FE-P1-01 Run 页 WS session 硬编码 | P1 | ✅ | `GetGraphExecution.session_id` + `useGraphRunStream` 使用真实 session |
| GRAPH-P1-01 Checkpoint provider 在 data | P1 | ✅ | 已在 `internal/runtime/providers.go` |
| GRAPH-FE-P2-01 task_status filter_key | P2 | ✅ | `graph/{graphId}/{execId}` · `graph_task_status.go` |
| GRAPH-FE-P2-02 Kanban ↔ 画布 focus | P2 | ✅ | `GraphTaskKanban` scroll + `GraphEditorCanvas` `focus-selected-node` + composable 联动 |
| GRAPH-BE-P2-01 service 拆分 | P2 | ✅ | `graph_definition_service.go` / `graph_execution_service.go` / `graph_task_service.go` / `graph_mapping.go` |
| GRAPH-BE-P2-02 ValidateGraph mapper 校验 | P2 | ✅ | `internal/graph/trpc/validator.go` · `invalid_mapper_json` |
| GRAPH-BE-P3-01 版本快照 metadata 膨胀 | P3 | ✅ | `biz/graph_version.go` · `snapshotForVersion` 仅保留 layout |
| GRAPH-FE-P3-01 composable 拆分 | P3 | ✅ | `useGraphRunStream` / `useGraphRunTasks` / `useGraphRunHitl` / `useGraphEditorAssets` |

---

## 验证

```bash
go build ./...
go test ./internal/biz/... ./internal/graph/trpc/... -count=1
cd web && pnpm build
```
