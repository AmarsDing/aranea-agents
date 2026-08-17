# 02 对话会话 测试用例

| ID | 用例 | 预期 |
|----|------|------|
| CHAT-01 | POST /v1/sessions 创建会话 | 200 + sid |
| CHAT-02 | POST /v1/chat/messages 真实 LLM 对话 | 200 + 助手回复落库 |
| CHAT-03 | GET sessions/{id}/messages | ≥2 条（user+assistant） |
| CHAT-04 | GET /v1/chat/run-status | 200 + status |
| CHAT-05 | GET /v1/sessions?query= 检索 | 200 |
| CHAT-06 | PATCH 更新标题 | 200 |
| CHAT-07 | pin/unpin | 200/200 |
| CHAT-08 | 会话导出 | 200 |
| CHAT-09 | turns 列表 | 200 |
| CHAT-10 | timeline | 200 |
| CHAT-11 | DELETE 会话 | 200（实际暴露 BUG） |
