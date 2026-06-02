# Graph 前端 Phase D — 契约补全 + 导入导出 + 版本管理

**日期**：2026-05-23  
**模块**：Graph (36) · 全栈

## 摘要

按 `36-graph-development.md` Phase D 落地 NodeDef 契约扩展、Graph 导入/导出、版本快照/回滚、用户自定义模板，并接线 builder 重试/缓存/Mapper。

## Phase D — 契约与资产复用

| 项 | 变更 |
|----|------|
| D-1 | Proto `NodeDef` 扩展：`retry_max_attempts` / `failure_action` / `fallback_agent` / `cache_*` / `input_mapper_json` / `output_mapper_json` / `isolated_messages` / `input_from_last_response` |
| D-2 | `internal/graph/trpc/mapper.go` + `node_wiring.go` 接线 Retry/Cache/Mapper |
| D-3 | RPC：`ExportGraph` / `ImportGraph` / `ListGraphVersions` / `RollbackGraphVersion` / `SaveGraphAsTemplate` |
| D-4 | 版本历史存于 `metadata._version_history`（UpdateGraph 自动快照，最多 50 条） |
| D-5 | 用户模板存于 `metadata.user_template`；`ListGraphTemplates` 合并 builtin + user |
| D-6 | 前端：`GraphPropertyPanel` 高级策略、`GraphVersionPanel`、编辑器 ⋮ 菜单（导入/导出/版本/模板） |

## 验证

```bash
make api
go test ./internal/biz/... -run GraphVersion
cd web && pnpm build
```
