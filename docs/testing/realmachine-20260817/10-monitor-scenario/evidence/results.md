| MON-01 | create diagnosis session | PASS | 318ms | sid=3813f62c-b55a-40f8-baec-d923614c88f4 |
| MON-02 | alarm query via agent (real tool call) | PASS | 17912ms | code=200 replyLen=1161 |
| MON-03 | tool runs evidence (twin calls) | FAIL | 23ms | total=1 twin=0 |
| MON-04 | create change-execution session | PASS | 26ms | sid=58bc205a-9366-4973-a2f6-a816740d9115 |
| MON-05 | fault inject request (expect HITL) | PASS | 139966ms | code=200 len=197878 |
| MON-06 | HITL pending activity found | FAIL | 0ms | actId= |
| MON-08 | fault inject executed | FAIL | 32ms | injectRuns=0 ok=0 status= |
| MON-09 | grant ops_fault_diagnosis twin read tools | PASS | 399ms | code=200 |
| MON-10 | grant ops_change_execution full toolset | PASS | 108ms | code=200 |
| MON-11 | effective tools include twin_alarm_query | PASS | 102ms | code=200 hit=True |
| MON-12 | alarm query retry | FAIL | 22ms | code=400 replyLen=0 |
| MON-13 | twin tool executed via agent | FAIL | 132ms | runs=20 twin=0 ok=0 |
| MON-14 | fault inject request | FAIL | 24ms | code=400 len=97 |
| MON-15 | HITL pending activity | FAIL | 0ms | actId= |
| MON-17 | fault inject executed | FAIL | 73ms | inject=0 ok=0 all=20 |
| MON-12 | alarm query retry | PASS | 10688ms | code=200 replyLen=599 |
| MON-13 | twin tool executed via agent | PASS | 26ms | runs=1 twin=1 ok=1 |
| MON-14 | fault inject request | FAIL | 300026ms | code=000 len=0 |
| MON-15 | HITL pending activity | FAIL | 0ms | actId= |
| MON-17 | fault inject executed | FAIL | 25ms | inject=0 ok=0 all=0 |
| MON-18 | create session C | PASS | 324ms | sid=f425a1e2-d511-4f34-83ba-1c780e855205 |
| MON-19 | async submit inject | PASS | 22ms | code=200 |
| MON-20 | HITL confirm step appeared | FAIL | 0ms | step= |
| MON-23 | async submit clear | PASS | 25ms | code=200 |
| MON-24 | HITL confirm step (clear) | FAIL | 0ms | step= |
| MON-27 | pre-check stale grants for exec agent | INFO | 0ms | grants=1 |
| MON-28 | stale grant deleted | PASS | 0ms | left=0 |
| MON-29 | create session D | PASS | 47ms | sid=d4006c59-253a-4d13-9e3c-0a1b86aac04f |
| MON-30 | async submit inject (explicit) | PASS | 21ms | code=200 |
| MON-31 | HITL confirm step appeared (inject) | PASS | 0ms | step=8128c4cf-8633-4a31-a149-9c07cbf2f32c-s4 |
| MON-32 | HITL approve inject | PASS | 27ms | code=200 |
| MON-33 | fault inject executed | PASS | 0ms | status=success |
| MON-34 | health check shows impact (direct) | PASS | 27ms | code=200 loss100=False len=39 |
| MON-34 | health check shows impact (direct) | FAIL | 465ms | code=400 has100=False len=133 |
| MON-35 | twin alarm raised after inject | FAIL | 0ms | hit=False len=132 |
| MON-36 | async submit clear (explicit) | PASS | 23ms | code=200 |
| MON-37 | HITL confirm step appeared (clear) | PASS | 0ms | step=8f9bd2fe-57e1-490f-9bbe-185a937e91bf-s4 |
| MON-38 | HITL approve clear | PASS | 23ms | code=200 |
| MON-39 | fault clear executed | PASS | 0ms | status=success |
| MON-40 | health check recovered (direct) | FAIL | 166ms | code=400 len=133 |
| MON-41 | alarm events direct query (twin gateway) | PASS | 0ms | code=200 total=74; no alarm in inject window -> coverage gap |
| MON-42 | ground truth: port eth1 down/up via tool output | PASS | 0ms | kernel log: eth1 disabled -> Link Up 1000Mbps |
| MON-B-01 | tool_grants expires_at 列+存量回填(created_at+72h) | PASS | 0ms | 4 rows backfilled, all expired by 2026-08-17 |
| MON-B-02 | API 读径过滤过期授权 (GET /v1/agents/{id}/tool-grants) | PASS | 45ms | code=200 items=0 (4 expired rows filtered) |
| MON-A-RT | 模糊注入请求澄清挂起(BUG-MON-A 复测) | PASS | 0ms | clarify step awaiting_input, 0 tool_invocations, 未 autoResolve 自动取消 |
