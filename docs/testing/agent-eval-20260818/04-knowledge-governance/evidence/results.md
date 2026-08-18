| D-00 | create eval-gov-kb | PASS | 46ms | id=1ed7ecca07c946395f69 |
| D-00b | dim correct (BUG-C-01 workaround) | PASS | 0ms | dim=1024 |
| D-00c | grant knowledge_write tool policy | PASS | 121ms | code=200 |
| D18-a0 | ingest entry doc | PASS | 31ms | docId=41e6e73d80d3d1a7ae53 |
| D18-a | chunk 重放（ingest 写入即可检索） | PASS | 0ms | writeToSearchable=0ms searchMs=7601 |
| D18-b0 | ingest note doc indexed | PASS | 0ms | docId=67f1817c1a5a624a38e4 status=indexed |
| D18-b | chunk 重放（第二篇可检索） | PASS | 2272ms | hit=True |
| D01 | 访问日志记录 | PASS | 0ms | access_log rows=15 |
| D03 | Hebbian 共激活边 | INFO | 0ms | co_activated edges=10 |
