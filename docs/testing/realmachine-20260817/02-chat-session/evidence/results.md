| CHAT-01 | 创建会话 | PASS | 38ms | code=200 sid=972daa64-e24c-4d70-87ac-dd69b36ec317 |
| CHAT-02 | 发送消息→LLM 回复 | FAIL | 6363ms | code=200 reply_len=0 |
| CHAT-03 | 消息列表 | PASS | 27ms | code=200 msgs=2 |
| CHAT-04 | run-status 查询 | PASS | 23ms | code=200 {"runId":"", "status":"completed", "errorMessage":"", "updatedAt":"2026-08-16T18:17:34Z", "invocationId":"", "agentName":"", "startedAt":"", "lastEventAt":"", "eventCount":0, "awaitKind":"", "awaitToolKey":"", "awaitToolCallId":""} |
| CHAT-05 | 会话检索 | PASS | 22ms | code=200 |
| CHAT-06 | 更新会话标题 | PASS | 25ms | code=200 |
| CHAT-07 | pin/unpin | PASS | 0ms | pin=200 unpin=200 |
| CHAT-08 | 会话导出 | PASS | 33ms | code=200 len=656 |
| CHAT-09 | turns 列表 | PASS | 22ms | code=200 |
| CHAT-10 | timeline | PASS | 24ms | code=200 |
| CHAT-11 | 删除会话 | FAIL | 31ms | code=500 |
