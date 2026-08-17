# 04 Graph 编排 测试用例与结果

## 用例

| ID | 用例 | 预期 |
|----|------|------|
| GRAPH-01 | GET /v1/graphs | 200 + items |
| GRAPH-02 | GET /v1/graphs/{id} | 200 |
| GRAPH-03 | POST /v1/graphs/{id}/validate | 200 valid=true |
| GRAPH-04 | GET /v1/graphs/{id}/visualize | 200 |
| GRAPH-05 | GET /v1/graphs/{id}/versions | 200 |
| GRAPH-06 | GET /v1/graphs/{id}/export | 200 |
| GRAPH-07 | GET /v1/graphs/{id}/executions | 200 |
| GRAPH-08 | GET /v1/graph/executions/{id} | 200 |
| GRAPH-09/09B | GET executions/{id}/checkpoints | 200 + items |
| GRAPH-10/10B/10C | GET executions/{id}/state-snapshot | 200 |
| GRAPH-11 | GET executions/{id}/task-events | 200 |
| GRAPH-12 | GET /v1/graph-templates | 200 |
| GRAPH-13/13B | POST executions/{id}/time-travel | 200 |

## 结果：13 项通过 / 2 项发现（1 缺陷 + 1 设计瑕疵）

| ID | 结果 | 耗时 | 说明 |
|----|------|------|------|
| GRAPH-01 | PASS | 29ms | 列表正常 |
| GRAPH-02 | PASS | 22ms | |
| GRAPH-03 | PASS | 30ms | valid=true |
| GRAPH-04 | **FAIL** | 21ms | 部分图 visualize 500（见分析 BUG-G1） |
| GRAPH-04B | PASS | 430ms | 正常图 visualize 200（1336B DOT） |
| GRAPH-05 | PASS | 23ms | |
| GRAPH-06 | PASS | 23ms | 导出 38KB |
| GRAPH-07 | PASS | 26ms | count=8 |
| GRAPH-08 | PASS | 21ms | status=completed |
| GRAPH-09 | **FAIL(误报)** | 19ms | 无检查点执行返回 404（见分析 ISSUE-G2） |
| GRAPH-09B | PASS | 912ms | 带 lineage 执行返回 5 个检查点 |
| GRAPH-10B | PASS | 22ms | 最新快照 21.8KB |
| GRAPH-10C | PASS | 28ms | 按 checkpoint_id 取快照 |
| GRAPH-11 | PASS | 29ms | 事件流 23.5KB |
| GRAPH-12 | PASS | 22ms | 模板列表 |
| GRAPH-13 | **FAIL(设计瑕疵)** | 28ms | step_index=0 被校验拒绝（见 ISSUE-G3） |
| GRAPH-13B | PASS | 29ms | step_index=1 正常返回 |

## 原因分析

### BUG-G1：部分图 visualize 500（待修）
- 现象：`GET /v1/graphs/{id}/visualize` 对 `f76d092c-...`（列表首条）返回 500，对正常图 `a3608496-...` 返回 200。
- 定位：`internal/service/graph_definition_service.go VisualizeGraph` → biz `VisualizeGraph`，对节点/边定义不完整（如缺 entry/finish、空 nodes）的图未做防御，panic 或错误上抛为 500。
- 影响：前端画布打开历史残缺图直接报错。

### ISSUE-G2：无检查点执行的 checkpoints/state-snapshot 返回 404（语义不当）
- 现象：未启用 checkpoint 的执行（`lineage_id=''`）调 `/checkpoints`、`/state-snapshot` 返回 `404 SHARED_NOT_FOUND`。
- 根因：`graph_execution_usecase.go ensureCheckpointRuntime` 在 `LineageID==""` 时返回 `ErrNotFound`。
- 分析：功能本身正确（无检查点可查），但 404 与「执行不存在」无法区分，前端无法给出「该执行未开启检查点」的准确提示。建议返回 200 + 空 items，或 400/409 + 明确 reason（如 `CHECKPOINT_NOT_ENABLED`）。

### ISSUE-G3：time-travel step_index=0 被拒绝（proto3 零值陷阱）
- 现象：`step_index=0` 返回 `400 VALIDATOR missing required field: step_index`；`step_index=1` 正常。
- 根因：proto 对 `step_index` 标了 `required` 校验，proto3 下标量零值（0）视为「未设置」，protovalidate 拒绝。
- 影响：无法 time-travel 到第 0 步（首个业务步骤）。建议改为 `optional int32` 或去掉 required 由业务层判负值。

## 解决方案
- BUG-G1：在 biz `VisualizeGraph` 对空 nodes/缺 entry 的图降级返回空 DOT 或明确 400；测试侧已用正常图复核链路可用。
- ISSUE-G2：低优，改返回空列表或专属 reason。
- ISSUE-G3：低优，proto 改 `optional` 或业务层校验 `step_index >= 0`（0 合法）。
