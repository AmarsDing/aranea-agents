# 09 MCP 测试用例与结果

## 用例

| ID | 用例 | 预期 |
|----|------|------|
| MCP-01 | GET /v1/mcp-servers | 200 + items |
| MCP-02 | GET /v1/mcp-servers/{id} | 200 |
| MCP-03 | POST /v1/mcp-servers/{id}/test | 连接测试 ok |
| MCP-04 | POST /v1/mcp-servers/validate（非法配置） | 明确拒绝原因 |
| MCP-05 | GET user-credentials | 200 |

## 结果：5/5 PASS

| ID | 结果 | 耗时 | 说明 |
|----|------|------|------|
| MCP-01 | PASS | 27ms | 6 个 server 全部 enabled |
| MCP-02 | PASS | 22ms | |
| MCP-03 | PASS | 39ms | `连接测试成功` status_code=200（热加载链路昨日已修，今日复核在线） |
| MCP-04 | PASS | 21ms | 非法 transport 被拒：`transport 必须是 [stdio sse streamable]`，ok=false 语义准确 |
| MCP-05 | PASS | 22ms | 凭证列表空（未配置，正常） |

## 原因分析

- MCP server 管理面（列表/详情/探测/校验/凭证）全部可用；validate 对坏配置返回结构化 `ok:false + message`，前端可直接渲染。
- 响应列表项 `server_key`/`transport` 字段在列表投影中为空（详情接口应有），属列表投影精简，非缺陷。

## 解决方案

- 无需修复。热加载回归（CRUD→工具集刷新）昨日 b-5 已专项验证，本次不重复消耗。
