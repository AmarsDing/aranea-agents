| KB-01 | collections list | PASS | 27ms | code=200 count=2 first=a7310ebb25e82766f6e6 |
| KB-02 | collection detail | PASS | 23ms | code=200 |
| KB-03 | vault tree | PASS | 23ms | code=200 len=511 |
| KB-04 | documents list | PASS | 21ms | code=200 len=7804 |
| KB-05 | collection graph | PASS | 22ms | code=200 len=43337 |
| KB-06 | knowledge search | PASS | 2279ms | code=200 len=3017 |
| KB-07 | embedder config | PASS | 20ms | code=200 len=136 |
| KB-08 | governance proposals | PASS | 25ms | code=200 len=2263 |
| KB-09 | create collection | FAIL | 24ms | code=400 cid= msg=root_path is required |
| KB-09B | create collection (root_path) | FAIL | 312ms | code=400 cid= msg=root_path not found: /tmp/realmachine-kb-vault |
| KB-09B | create collection (root_path) | PASS | 314ms | code=200 cid=1c87a93675155d3164c8 msg= |
| KB-10 | ingest document | FAIL | 22ms | code=400 did= msg=missing required field: source |
| KB-11 | search newly ingested doc | FAIL | 26ms | code=200 hit=False len=13 |
| KB-10B | ingest document (base64) | PASS | 320ms | code=200 did=b6aea2aa902d9b0fe92c msg= |
| KB-11B | search newly ingested doc | PASS | 2058ms | code=200 hit=True len=302 |
| KB-12B | document content | PASS | 30ms | code=200 len=155 |
