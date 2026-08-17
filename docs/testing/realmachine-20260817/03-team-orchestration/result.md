# 03 Team 编排 测试用例与结果

## 用例

| ID | 用例 | 预期 |
|----|------|------|
| TEAM-01 | GET /v1/teams | 200 + total>0 |
| TEAM-02 | GET /v1/teams/{id} | 200 |
| TEAM-03 | POST run-test（真实 LLM 顺序执行 2 成员） | 200 + run id + reply |
| TEAM-04 | GET /v1/team-runs/{id} | 200 + status=success |
| TEAM-05 | GET team-runs/{id}/steps | ≥2 步 |
| TEAM-06 | GET team-runs/{id}/summary | 200 |
| TEAM-07 | GET team-runs/{id}/observatory | 200 |
| TEAM-08 | GET /v1/team-runs | 200 |
| TEAM-09 | GET /v1/task-dead-letters | 200（需 session_id 或 team_id） |
| TEAM-10 | POST teams/{id}/compile-graph | 200 + 图定义 |

## 结果：10/10 PASS

| ID | 结果 | 耗时 | 说明 |
|----|------|------|------|
| TEAM-01 | PASS | 50ms | total=169（含 Spirit 自动组建的团队） |
| TEAM-02 | PASS | 23ms | |
| TEAM-03 | PASS | 12.0s | 2 成员 sequential 全链路真实执行，reply 306 字 |
| TEAM-04 | PASS | 23ms | status=success |
| TEAM-05 | PASS | 22ms | steps=2 |
| TEAM-06 | PASS | 23ms | |
| TEAM-07 | PASS | 33ms | |
| TEAM-08 | PASS | 31ms | |
| TEAM-09 | PASS | - | 不带参数 400 是正确校验（"session_id or team_id is required"）；带 team_id 返回 200 `{"items":[]}` |
| TEAM-10 | PASS | 32ms | Team→Graph 编译 4171B |

## 原因分析
- Team 编排链路（定义→编译 Graph→执行→步骤/汇总/观测投影）完整可用，性能良好（2 成员 12s）。
- TEAM-09 初测 400 系用例遗漏必填过滤参数，非缺陷；建议 API 文档/前端空态统一提示。

## 解决方案
- 无需修复。建议：`/v1/task-dead-letters` 在管理端提供「全量」模式（当前强制 session_id/team_id，巡检场景不便）。
