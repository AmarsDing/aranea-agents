# 01 Agent 管理 测试用例

| ID | 用例 | 预期 |
|----|------|------|
| AGT-01 | GET /v1/agents 列表分页 | 200 + total>0 |
| AGT-02 | GET /v1/agents/{id} 详情 | 200 + provider/model 正确 |
| AGT-03 | POST /v1/agents 无 position 创建 | 200（实际暴露 BUG） |
| AGT-03B | POST /v1/agents 带 position_key+variant 创建 | 200 |
| AGT-04 | PATCH /v1/agents/{id} 更新名称 | 200 |
| AGT-05 | PATCH /v1/agents/{id}/favorite | 200 |
| AGT-06 | GET system-prompt/preview | 200 + 非空 |
| AGT-07 | GET tools/effective | 200 + 工具清单 |
| AGT-08 | GET /v1/agents/creators | 200 |
| AGT-09 | DELETE /v1/agents/{id} | 200 |
| AGT-10 | 删除后再 GET | 404 |
| AGT-11 | limit/offset 分页语义 | items==limit |
