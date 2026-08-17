| GRAPH-01 | Graph 列表 | PASS | 29ms | code=200 first=f76d092c-6fb1-428e-b1a7-1b3c961405d4 |
| GRAPH-02 | Graph 详情 | PASS | 22ms | code=200 |
| GRAPH-03 | Graph 校验 | PASS | 30ms | code=200 {"errors":[], "warnings":[], "valid":true} |
| GRAPH-04 | Graph 可视化数据 | FAIL | 21ms | code=500 len=82 |
| GRAPH-05 | Graph 版本 | PASS | 23ms | code=200 |
| GRAPH-06 | Graph 导出 | PASS | 23ms | code=200 len=38198 |
| GRAPH-07 | Graph 执行列表 | PASS | 21ms | code=200 exec= |
| GRAPH-12 | Graph 模板列表 | PASS | 22ms | code=200 |
| GRAPH-07 | Graph 执行列表 | PASS | 26ms | code=200 count=8 |
| GRAPH-08 | 执行详情 | PASS | 21ms | code=200 status=completed |
| GRAPH-09 | 检查点列表 | FAIL | 19ms | code=404 len=88 |
| GRAPH-10 | 状态快照 | FAIL | 21ms | code=404 len=88 |
| GRAPH-11 | 任务事件流 | PASS | 29ms | code=200 len=23481 |
| GRAPH-04B | visualize 复核(正常图) | PASS | 430ms | code=200 len=1336 |
| GRAPH-09B | checkpoint list (ckpt-enabled exec) | PASS | 912ms | code=200 count=5 |
| GRAPH-10B | state snapshot (latest) | PASS | 22ms | code=200 len=21851 |
| GRAPH-13 | time travel (step 0) | FAIL | 28ms | code=400 len=97 |
| GRAPH-10C | state snapshot (by checkpoint_id) | PASS | 28ms | code=200 len=21851 |
| GRAPH-13B | time travel (step 1) | PASS | 29ms | code=200 len=113 |
| GRAPH-07 | Graph 执行列表 | PASS | 64ms | code=200 count=8 |
| GRAPH-08 | 执行详情 | PASS | 45ms | code=200 status=completed |
| GRAPH-09 | 检查点列表 | PASS | 40ms | code=200 len=12 |
| GRAPH-10 | 状态快照 | PASS | 64ms | code=200 len=17 |
| GRAPH-11 | 任务事件流 | PASS | 50ms | code=200 len=23481 |
| GRAPH-04B | visualize 复核(正常图) | PASS | 691ms | code=200 len=1336 |
| GRAPH-09B | checkpoint list (ckpt-enabled exec) | PASS | 1160ms | code=200 count=5 |
| GRAPH-10B | state snapshot (latest) | PASS | 42ms | code=200 len=21851 |
| GRAPH-13 | time travel (step 0) | PASS | 53ms | code=200 len=112 |
