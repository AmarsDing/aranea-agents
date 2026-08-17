| AGT-01 | Agent 列表分页 | PASS | 35ms | code=200 total=308 page_items=24 |
| AGT-02 | Agent 详情 | FAIL | 21ms | code=404 provider= model= |
| AGT-03 | 创建 Agent | FAIL | 33ms | code=400 |
| AGT-04 | 更新 Agent 名称 | FAIL | 22ms | code=404 |
| AGT-05 | 收藏切换 | FAIL | 23ms | code=404 |
| AGT-06 | System Prompt 预览 | FAIL | 21ms | code=404 len=84 |
| AGT-07 | effective tools | FAIL | 21ms | code=404 len=84 |
| AGT-08 | creators 列表 | PASS | 22ms | code=200 |
| AGT-09 | 删除 Agent | FAIL | 24ms | code=404 |
| AGT-10 | 删除后查询应为错误 | PASS | 22ms | code=404 |
| AGT-02 | Agent 详情 | FAIL | 29ms | code=404 provider= model= |
| AGT-03B | 创建 Agent(带 position) | PASS | 45ms | code=200 |
| AGT-04 | 更新 Agent 名称 | FAIL | 23ms | code=404 |
| AGT-05 | 收藏切换 | FAIL | 22ms | code=404 |
| AGT-06 | System Prompt 预览 | FAIL | 23ms | code=404 len=84 |
| AGT-07 | effective tools | FAIL | 21ms | code=404 len=84 |
| AGT-09 | 删除 Agent | FAIL | 21ms | code=404 |
| AGT-10 | 删除后查询应为错误 | PASS | 20ms | code=404 |
| AGT-11 | limit/offset 分页 | PASS | 25ms | code=200 items=5 limit=5 |
| AGT-00 | 按 agentKey 解析 id | PASS | 0ms | ops_fault_diagnosis -> 71096314087d86e2caa20488 |
| AGT-02 | Agent 详情 | PASS | 30ms | code=200 provider=deepseek model=deepseek-v4-flash |
| AGT-04 | 更新 Agent 名称 | PASS | 44ms | code=200 id=6ebed8bb4ffac15c8b3b027c |
| AGT-05 | 收藏切换 | PASS | 28ms | code=200 |
| AGT-06 | System Prompt 预览 | PASS | 189ms | code=200 len=16564 |
| AGT-07 | effective tools | PASS | 102ms | code=200 len=18701 |
| AGT-09 | 删除 Agent | PASS | 38ms | code=200 |
| AGT-10 | 删除后查询应为错误 | PASS | 20ms | code=404 |
