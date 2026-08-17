# 11 可观测性 测试结果

**结论：13/13 PASS**

| ID | 用例 | 结果 | 耗时 | 说明 |
|----|------|------|------|------|
| OBS-01 | audit 审计日志列表 | PASS | 223ms | len=63KB，数据充足 |
| OBS-02 | monitor events 列表 | PASS | 188ms | len=57KB |
| OBS-03 | traces 列表 | PASS | 112ms | len=56KB |
| OBS-04 | logs 查询 | PASS | 21ms | |
| OBS-05 | flow-logs | PASS | 22ms | len=23（空集，属数据量问题非故障） |
| OBS-06 | alert-rules | PASS | 23ms | |
| OBS-07 | alert-metrics | PASS | 28ms | 2KB 指标数据 |
| OBS-08 | runner-metrics | PASS | 24ms | window=60min totalRuns=0（近 1h 无 runner 执行，与测试节奏一致） |
| OBS-09 | code-executor-capabilities | PASS | 22ms | |
| OBS-10 | self-check-reports | PASS | 62ms | 42KB 自检报告 |
| OBS-11 | heal-stats | PASS | 37ms | |
| OBS-12 | heal-records | PASS | 22ms | 18KB 自愈记录 |
| OBS-13 | trace 详情 | PASS | 24ms | id=b6a9f533... |

## 原因分析
- 全部端点 200 且返回真实业务数据（非空壳），审计/事件/trace 积累量大，说明可观测性采集在真实运行中持续工作。
- OBS-05 flow-logs 为空集：该数据源依赖特定 flow 场景触发，当前环境近期无匹配数据，记为数据覆盖说明而非缺陷。
- OBS-08 runner-metrics 近 60min totalRuns=0：runner 指标窗口与测试执行窗口错开（监控场景测试走 chat 链路非 runner），无异常。

## 解决方案
- 无需修复。建议（非阻塞）：flow-logs 空集时前端可提示「近期无 flow 数据」而非空白，列入 UX 优化候选。
