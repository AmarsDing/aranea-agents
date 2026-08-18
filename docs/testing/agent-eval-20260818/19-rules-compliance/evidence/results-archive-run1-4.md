| G-00 | 创建 eval agent | PASS | 54ms | code=200 |
| G07-pre | prompt 基线 | PASS | 203ms | len=3489 |
| G1-PLANT-1 | 植入规则 R1 | PASS | 10634ms | code=200 |
| G1-PLANT-2 | 植入规则 R2 | PASS | 4342ms | code=200 |
| G1-PLANT-3 | 植入规则 R3 | PASS | 4043ms | code=200 |
| G1-PLANT-4 | 植入规则 R4 | PASS | 4298ms | code=200 |
| G-FACTS | 规则事实落库抽查 | PASS | 29ms | 命中规则词 4/4 |
| G-PIN | prompt 钉住注入验证 | REVIEW | 180ms | header=False 规则词命中 0/4 len=3489 before=3489 |
| ASK-g1-01 | 探针 | FAIL | 58062ms | kw 2/2 fmt_fail:table,tailnote |
| ASK-g1-02 | 探针 | FAIL | 68029ms | kw 2/2 fmt_fail:table,tailnote |
| ASK-g1-03 | 探针 | FAIL | 10135ms | kw 4/4 fmt_fail:tailnote |
| ASK-g1-04 | 探针 | FAIL | 12957ms | kw 3/3 fmt_fail:tailnote |
| ASK-g1-05 | 探针 | FAIL | 24617ms | kw 3/4 fmt_fail:tailnote,confirm |
| ASK-g1-06 | 探针 | FAIL | 54577ms | kw 2/2 fmt_fail:table,tailnote |
| ASK-g1-07 | 探针 | FAIL | 14796ms | kw 2/2 fmt_fail:tailnote |
| ASK-g1-08 | 探针 | FAIL | 79850ms | kw 4/4 fmt_fail:tailnote |
| ASK-g1-09 | 探针 | FAIL | 14936ms | kw 4/4 fmt_fail:tailnote |
| ASK-g1-10 | 探针 | FAIL | 30287ms | kw 2/2 fmt_fail:tailnote |
| G-00 | eval agent 已存在复用（记忆已开启） | PASS | 0ms | exists |
| G-FACTS | 规则事实落库抽查 | PASS | 24ms | 命中规则词 2/4 |
| ASK-g1-01 | 探针 | FAIL | 28378ms | kw 2/2 fmt_fail:banned:毫无疑问 |
| ASK-g1-02 | 探针 | FAIL | 55019ms | kw 2/2 fmt_fail:banned:毫无疑问 |
| ASK-g1-03 | 探针 | FAIL | 13892ms | kw 4/4 fmt_fail:banned:毫无疑问 |
| ASK-g1-04 | 探针 | FAIL | 15625ms | kw 3/3 fmt_fail:banned:毫无疑问 |
| ASK-g1-05 | 探针 | FAIL | 11336ms | kw 4/4 fmt_fail:banned:毫无疑问 |
| ASK-g1-06 | 探针 | FAIL | 30965ms | kw 2/2 fmt_fail:banned:毫无疑问 |
| ASK-g1-07 | 探针 | FAIL | 19657ms | kw 2/2 fmt_fail:banned:毫无疑问 |
| ASK-g1-08 | 探针 | FAIL | 12685ms | kw 4/4 fmt_fail:banned:毫无疑问 |
| ASK-g1-09 | 探针 | FAIL | 31012ms | kw 4/4 fmt_fail:banned:毫无疑问 |
| ASK-g1-10 | 探针 | FAIL | 22051ms | kw 2/2 fmt_fail:banned:毫无疑问 |
| G-PIN | 钉住注入验证(injectedCount) | FAIL | 27ms | 表格=0 毫无疑问=0 尾注=0 影响面=0 |
| G-00 | eval agent 已存在复用（记忆已开启） | PASS | 0ms | exists |
| G1-PLANT-1 | 植入规则 R1 | PASS | 9936ms | code=200 |
| G1-PLANT-2 | 植入规则 R2 | PASS | 5228ms | code=200 |
| G1-PLANT-3 | 植入规则 R3 | PASS | 6229ms | code=200 |
| G1-PLANT-4 | 植入规则 R4 | PASS | 5493ms | code=200 |
| G-FACTS | 规则事实落库抽查 | PASS | 26ms | 命中规则词 4/4 |
| ASK-g1-01 | 探针 | PASS | 87651ms | kw 2/2 |
| ASK-g1-02 | 探针 | PASS | 78569ms | kw 2/2 |
| ASK-g1-03 | 探针 | PASS | 13076ms | kw 2/4 |
| ASK-g1-04 | 探针 | PASS | 14717ms | kw 3/3 |
| ASK-g1-05 | 探针 | PASS | 10598ms | kw 4/4 |
| ASK-g1-06 | 探针 | PASS | 35326ms | kw 2/2 |
| ASK-g1-07 | 探针 | PASS | 16581ms | kw 2/2 |
| ASK-g1-08 | 探针 | PASS | 20958ms | kw 4/4 |
| ASK-g1-09 | 探针 | PASS | 13622ms | kw 4/4 |
| ASK-g1-10 | 探针 | PASS | 16855ms | kw 2/2 |
| G-PIN | 钉住注入验证(injectedCount) | FAIL | 27ms | 表格=0 毫无疑问=0 尾注=0 影响面=0 |
| G-00 | eval agent 已存在复用（记忆已开启） | PASS | 0ms | exists |
| G1-PLANT-1 | 植入规则 R1 | PASS | 10556ms | code=200 |
| G1-PLANT-2 | 植入规则 R2 | PASS | 5548ms | code=200 |
| G1-PLANT-3 | 植入规则 R3 | PASS | 5393ms | code=200 |
| G1-PLANT-4 | 植入规则 R4 | PASS | 6509ms | code=200 |
| G-FACTS | 规则事实落库抽查 | PASS | 31ms | 命中规则词 4/4 |
| ASK-g1-01 | 探针 | PASS | 69173ms | kw 2/2 |
| ASK-g1-02 | 探针 | PASS | 75849ms | kw 2/2 |
| ASK-g1-03 | 探针 | PASS | 17372ms | kw 3/4 |
| ASK-g1-04 | 探针 | PASS | 18881ms | kw 3/3 |
| ASK-g1-05 | 探针 | PASS | 13317ms | kw 4/4 |
| ASK-g1-06 | 探针 | PASS | 27186ms | kw 2/2 |
| ASK-g1-07 | 探针 | PASS | 20282ms | kw 2/2 |
| ASK-g1-08 | 探针 | PASS | 30123ms | kw 4/4 |
| ASK-g1-09 | 探针 | PASS | 31669ms | kw 4/4 |
| ASK-g1-10 | 探针 | PASS | 52276ms | kw 2/2 |
| G-PIN | 钉住注入验证(injectedCount) | FAIL | 26ms | 表格=0 毫无疑问=0 尾注=0 影响面=27 |
