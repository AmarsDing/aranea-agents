| D-00 | create eval-gov-kb | PASS | 46ms | id=1ed7ecca07c946395f69 |
| D-00b | dim correct (BUG-C-01 workaround) | PASS | 0ms | dim=1024 |
| D-00c | grant knowledge_write tool policy | PASS | 121ms | code=200 |
| D18-a0 | ingest entry doc | PASS | 31ms | docId=41e6e73d80d3d1a7ae53 |
| D18-a | chunk 重放（ingest 写入即可检索） | PASS | 0ms | writeToSearchable=0ms searchMs=7601 |
| D18-b0 | ingest note doc indexed | PASS | 0ms | docId=67f1817c1a5a624a38e4 status=indexed |
| D18-b | chunk 重放（第二篇可检索） | PASS | 2272ms | hit=True |
| D01 | 访问日志记录 | PASS | 0ms | access_log rows=15 |
| D03 | Hebbian 共激活边 | INFO | 0ms | co_activated edges=10 |
| D15-chat | chat 写回请求 | PASS | 9653ms | code=200 |
| D15 | knowledge_write 词条落库 | PASS | 0ms | docId=26d3748c08d0c1494ed5 |
| D18-c | chunk 重放（写回词条即可检索，P0 回归点） | FAIL | 0ms | writeToSearchable=42792ms (目标<5000) |
| D16-chat | chat 更新请求 | PASS | 8704ms | code=200 |
| D16 | 词条 upsert 整段替换 | FAIL | 0ms | hasNewIP=False oldIPoccurrences=0 |
| D07 | supersedes 版本链快照 | PASS | 0ms | fact_version rows=1 |
| D17-chat | chat alias 写入 | PASS | 8213ms | code=200 |
| D17 | 别名解析合并（不新建词条） | FAIL | 0ms | newEntries=1 mergedIntoExisting=0 |
| D17b-chat1 | chat 建值班制度词条 | PASS | 8312ms | code=200 |
| D17b-chat2 | chat typed 关系素材 | PASS | 10001ms | code=200 |
| D09-prep-chat | chat 建陈旧词条 | PASS | 9190ms | code=200 |
| D10-prep-chat | chat 建孤儿词条 | PASS | 8783ms | code=200 |
| D08-chat | chat 矛盾写入 | PASS | 13874ms | code=200 |
| D08 | 写入时仲裁（supersedes 或 contradicts 提案） | FAIL | 0ms | neither: hasAlt=False lineage=True verRows=2 |
| D05 | 实体共现边生成 | REVIEW | 0ms | entity edges=0 |
| D06 | typed 语义关系抽取 | REVIEW | 0ms | semantic edges=0 |
| D20-pre | 治理前检索基准 | INFO | 0ms | hit=0/2 |
| D09-prep | stale 场景构造（closed_ratio=0.8） | FAIL | 0ms | semantic edges=9 |
| D19-a | memory_butler_knowledge_curate 可用性 | PASS | 31526ms | code=200 |
| D09 | 陈旧提案（自动 applied + stale_at 置位） | FAIL | 0ms | staleProps=0 stale_at=f |
| D10 | 孤儿提案（pending 人工二审） | FAIL | 0ms | proposalId= |
| D19-b | memory_butler_governance_proposals 可用性 | FAIL | 300020ms | code=000 |
| D11-a | 提案二审（orphan） | FAIL | 0ms | no pending orphan proposal |
| D11-b | 提案二审（supersedes 路径：版本链留痕核验） | PASS | 0ms | no proposal (supersedes); fact_version rows=2 |
| D20 | 自治理不劣化检索 | PASS | 0ms | pre=0/2 post=0/2 |
| D-cleanup | 评测数据清理 | PASS | 0ms | entries/proposals/links/collection cleaned |
| D-00 | create eval-gov-kb | PASS | 33ms | id=57b67e0794803291aa80 |
| D-00b | dim correct (BUG-C-01 workaround) | PASS | 0ms | dim=1024 |
| D-00c | grant knowledge_write tool policy | PASS | 125ms | code=200 |
| D18-a0 | ingest entry doc | PASS | 25ms | docId=d00fdb45e20c9e41b435 |
| D18-a | chunk 重放（ingest 写入即可检索） | PASS | 0ms | writeToSearchable=0ms searchMs=7032 |
| D18-b0 | ingest note doc indexed | PASS | 0ms | docId=dc1609ff05a6079b6ee2 status=indexed |
| D18-b | chunk 重放（第二篇可检索） | PASS | 1987ms | hit=True |
| D01 | 访问日志记录 | PASS | 0ms | access_log rows=18 |
| D03 | Hebbian 共激活边 | INFO | 0ms | co_activated edges=17 |
| D-00 | create eval-gov-kb | PASS | 29ms | id=a38550b3c95c9ba9e15b |
| D-00b | dim correct (BUG-C-01 workaround) | PASS | 0ms | dim=1024 |
| D-00c | grant knowledge_write tool policy | PASS | 115ms | code=200 |
| D18-a0 | ingest entry doc | PASS | 24ms | docId=f092a86c767e2ba2205a |
| D18-a | chunk 重放（ingest 写入即可检索） | PASS | 0ms | writeToSearchable=2084ms searchMs=6879 |
| D18-b0 | ingest note doc indexed | PASS | 0ms | docId=65b1a976ff8e1e28deee status=indexed |
| D18-b | chunk 重放（第二篇可检索） | PASS | 1117ms | hit=True |
| D01 | 访问日志记录 | PASS | 0ms | access_log rows=21 |
| D03 | Hebbian 共激活边 | INFO | 0ms | co_activated edges=17 |
| D15-chat | chat 写回请求 | PASS | 8812ms | code=200 |
| D15 | knowledge_write 词条落库 | PASS | 0ms | docId=9d62cb9e93dd37351d9f |
| D18-c | chunk 重放（写回词条即可检索，P0 回归点） | FAIL | 0ms | writeToSearchable=39535ms (目标<5000) |
| D16-chat | chat 更新请求 | PASS | 9102ms | code=200 |
| D16 | 词条 upsert 整段替换 | PASS | 0ms | hasNewIP=True oldIPoccurrences=0 |
| D07 | supersedes 版本链快照 | PASS | 0ms | fact_version rows=1 |
| D17-chat | chat alias 写入 | PASS | 8093ms | code=200 |
| D17 | 别名解析合并（不新建词条） | FAIL | 0ms | newEntries=1 mergedIntoExisting=0 |
| D17b-chat1 | chat 建值班制度词条 | PASS | 8188ms | code=200 |
| D17b-chat2 | chat typed 关系素材 | PASS | 9216ms | code=200 |
| D09-prep-chat | chat 建陈旧词条 | PASS | 8744ms | code=200 |
| D10-prep-chat | chat 建孤儿词条 | PASS | 8801ms | code=200 |
| D08-chat | chat 矛盾写入 | PASS | 10200ms | code=200 |
| D08 | 写入时仲裁（supersedes 或 contradicts 提案） | PASS | 0ms | contradicts proposal=55 |
| D05 | 实体共现边生成 | REVIEW | 0ms | entity edges=0 |
| D06 | typed 语义关系抽取 | REVIEW | 0ms | semantic edges=0 |
| D20-pre | 治理前检索基准 | INFO | 0ms | hit=0/2 |
| D09-prep | stale 场景构造（closed_ratio=0.8） | PASS | 0ms | semantic edges=5 |
| D19-a | memory_butler_knowledge_curate 可用性 | PASS | 37493ms | code=200 |
| D09 | 陈旧提案（自动 applied + stale_at 置位） | PASS | 0ms | staleProps=1 stale_at=t |
| D10 | 孤儿提案（pending 人工二审） | PASS | 0ms | proposalId=59 |
| D19-b | memory_butler_governance_proposals 可用性 | PASS | 274261ms | code=200 |
| D11-a | 提案二审（orphan applied→删词条） | PASS | 26ms | code=200 status=applied orphanDocLeft=0 |
| D11-b | 提案二审（conflict keep_old→删新段） | PASS | 42ms | code=200 status=applied altIPgone=True |
| D20 | 自治理不劣化检索 | PASS | 0ms | pre=0/2 post=0/2 |
| D-cleanup | 评测数据清理 | PASS | 0ms | entries/proposals/links/collection cleaned |
