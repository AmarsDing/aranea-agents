| B-00 | 创建 eval agent | FAIL | 38ms | code=503 |
| B07-pre | prompt 基线 | FAIL | 22ms | len=195 |
| PLANT-1 | 植入 mem-sh-01 | FAIL | 22ms | code=503 |
| PLANT-2 | 植入 mem-sh-02 | FAIL | 22ms | code=503 |
| PLANT-3 | 植入 mem-sh-03 | FAIL | 21ms | code=503 |
| B-00 | 创建 eval agent | PASS | 52ms | code=200 |
| B07-pre | prompt 基线 | FAIL | 21ms | len=81 |
| PLANT-1 | 植入 mem-sh-01 | FAIL | 20629ms | code=200 |
| PLANT-2 | 植入 mem-sh-02 | FAIL | 32914ms | code=200 |
| PLANT-3 | 植入 mem-sh-03 | FAIL | 45989ms | code=200 |
| A01-facts | 植入事实落库抽查 | PASS | 50ms | 命中植入词 1/25 |
| B02-mem-sh-01 | 召回段 | FAIL | 30ms | recall_debug |
| B02-mem-sh-02 | 召回段 | FAIL | 29ms | recall_debug |
| B02-mem-sh-03 | 召回段 | FAIL | 29ms | recall_debug |
| ASK-mem-sh-01 | 提问(single_hop) | FAIL | 55310ms | kw 0/2 |
| ASK-mem-sh-02 | 提问(single_hop) | PASS | 42353ms | kw 2/2 |
| ASK-mem-sh-03 | 提问(single_hop) | PASS | 22363ms | kw 1/1 |
| B07-post | prompt 植入后 | FAIL | 24ms | len=81 before=81 |
