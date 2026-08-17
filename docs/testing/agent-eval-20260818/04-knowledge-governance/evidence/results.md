| D-00 | create eval-gov-kb | PASS | 35ms | id=03aedc66c3465ad98969 |
| D-00b | dim correct (BUG-C-01 workaround) | PASS | 0ms | dim=1024 |
| D18-a0 | create vault entry doc | FAIL | 23ms | docId= |
| D18-a | chunk 重放（vault 写入即可检索） | FAIL | 0ms | writeToSearchable=8029ms searchMs=0 (目标<5000) |
| D18-b0 | ingest note doc indexed | PASS | 0ms | docId=c517b47e1c6d17b24ad6 status=indexed |
| D18-b | chunk 重放（ingest 对照） | PASS | 1057ms | hit=True |
| D01 | 访问日志记录 | PASS | 0ms | access_log rows=8 (search x3) |
| D03 | Hebbian 共激活边 | INFO | 0ms | co_activated edges=0 |
| D20-pre | 治理前检索基准 | INFO | 0ms | hit=3/3 |
| D15-chat | chat 写回请求 | FAIL | 2172ms | code=500 |
| D15 | knowledge_write 词条落库 | FAIL | 0ms | entry= |
| D16-chat | chat 更新请求 | FAIL | 1255ms | code=500 |
| D17-chat | chat alias 写入 | FAIL | 1359ms | code=500 |
| D-00 | create eval-gov-kb | PASS | 28ms | id=3d6794c628364d90c139 |
| D-00b | dim correct (BUG-C-01 workaround) | PASS | 0ms | dim=1024 |
| D18-a0 | ingest entry doc | PASS | 22ms | docId=06bdba7b34c4210f7699 |
| D18-a | chunk 重放（ingest 写入即可检索） | PASS | 0ms | writeToSearchable=4122ms searchMs=935 |
| D18-b0 | ingest note doc indexed | PASS | 0ms | docId=27c8fd150babb9188439 status=indexed |
| D18-b | chunk 重放（第二篇可检索） | PASS | 1219ms | hit=True |
| D01 | 访问日志记录 | PASS | 0ms | access_log rows=18 |
| D03 | Hebbian 共激活边 | INFO | 0ms | co_activated edges=0 |
| D15-chat | chat 写回请求 | FAIL | 912ms | code=500 |
| D15 | knowledge_write 词条落库 | FAIL | 0ms | docId= |
| D18-c | chunk 重放（写回词条即可检索，P0 回归点） | FAIL | 0ms | writeToSearchable=0ms (目标<5000) |
| D16-chat | chat 更新请求 | FAIL | 898ms | code=500 |
| D17-chat | chat alias 写入 | FAIL | 920ms | code=500 |
| D17 | 别名解析合并（不新建词条） | FAIL | 0ms | newEntries=0 mergedIntoExisting=0 |
| D17b-chat1 | chat 建值班制度词条 | FAIL | 1369ms | code=500 |
| D17b-chat2 | chat typed 关系素材 | FAIL | 1275ms | code=500 |
| D09-prep-chat | chat 建陈旧词条 | FAIL | 1212ms | code=500 |
| D10-prep-chat | chat 建孤儿词条 | FAIL | 1424ms | code=500 |
| D08-chat | chat 矛盾写入 | FAIL | 1874ms | code=500 |
| D08 | 冲突检测提案生成（P0） | FAIL | 0ms | proposalId= |
| D20-pre | 治理前检索基准 | INFO | 0ms | hit=0/2 |
| D09-prep | stale 场景构造 | FAIL | 0ms | staleDocId= entryInboxId= |
| D19-a | memory_butler_knowledge_curate 可用性 | FAIL | 2169ms | code=500 |
| D09 | 陈旧提案（自动 applied + stale_at 置位） | FAIL | 0ms | staleProps=0 stale_at=? |
| D10 | 孤儿提案（pending 人工二审） | FAIL | 0ms | proposalId= |
| D19-b | memory_butler_governance_proposals 可用性 | FAIL | 1596ms | code=500 |
| D11-a | 提案二审（orphan） | FAIL | 0ms | no pending orphan proposal |
| D11-b | 提案二审（conflict） | FAIL | 0ms | no pending conflict proposal |
| D20 | 自治理不劣化检索 | PASS | 0ms | pre=0/2 post=0/2 |
| D-cleanup | 评测数据清理 | PASS | 0ms | entries/proposals/links/collection cleaned |
| D-00 | create eval-gov-kb | PASS | 28ms | id=2de85b4624d9afabb716 |
| D-00b | dim correct (BUG-C-01 workaround) | PASS | 0ms | dim=1024 |
| D18-a0 | ingest entry doc | PASS | 24ms | docId=ab9cb7f811a089c75c24 |
| D18-a | chunk 重放（ingest 写入即可检索） | PASS | 0ms | writeToSearchable=0ms searchMs=1654 |
| D18-b0 | ingest note doc indexed | PASS | 0ms | docId=c1b4231fe41f55f8277a status=indexed |
| D18-b | chunk 重放（第二篇可检索） | PASS | 2166ms | hit=True |
| D01 | 访问日志记录 | PASS | 0ms | access_log rows=21 |
| D03 | Hebbian 共激活边 | INFO | 0ms | co_activated edges=22 |
| D15-chat | chat 写回请求 | PASS | 21782ms | code=200 |
| D15 | knowledge_write 词条落库 | FAIL | 0ms | docId= |
| D18-c | chunk 重放（写回词条即可检索，P0 回归点） | FAIL | 0ms | writeToSearchable=0ms (目标<5000) |
| D16-chat | chat 更新请求 | PASS | 9069ms | code=200 |
| D17-chat | chat alias 写入 | PASS | 9463ms | code=200 |
| D17 | 别名解析合并（不新建词条） | FAIL | 0ms | newEntries=0 mergedIntoExisting=0 |
| D17b-chat1 | chat 建值班制度词条 | PASS | 8783ms | code=200 |
| D17b-chat2 | chat typed 关系素材 | PASS | 6745ms | code=200 |
| D09-prep-chat | chat 建陈旧词条 | PASS | 6752ms | code=200 |
| D10-prep-chat | chat 建孤儿词条 | PASS | 6488ms | code=200 |
| D08-chat | chat 矛盾写入 | PASS | 5972ms | code=200 |
| D08 | 冲突检测提案生成（P0） | FAIL | 0ms | proposalId= |
| D20-pre | 治理前检索基准 | INFO | 0ms | hit=0/2 |
| D09-prep | stale 场景构造 | FAIL | 0ms | staleDocId= entryInboxId= |
| D19-a | memory_butler_knowledge_curate 可用性 | PASS | 39211ms | code=200 |
| D09 | 陈旧提案（自动 applied + stale_at 置位） | FAIL | 0ms | staleProps=1 stale_at=? |
| D10 | 孤儿提案（pending 人工二审） | FAIL | 0ms | proposalId= |
| D19-b | memory_butler_governance_proposals 可用性 | FAIL | 300031ms | code=000 |
| D11-a | 提案二审（orphan） | FAIL | 0ms | no pending orphan proposal |
| D11-b | 提案二审（conflict） | FAIL | 0ms | no pending conflict proposal |
| D20 | 自治理不劣化检索 | PASS | 0ms | pre=0/2 post=0/2 |
| D-cleanup | 评测数据清理 | PASS | 0ms | entries/proposals/links/collection cleaned |
