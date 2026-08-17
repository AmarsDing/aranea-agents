# 12 额度成本 测试结果

**结论：11/11 PASS**

| ID | 用例 | 结果 | 耗时 | 说明 |
|----|------|------|------|------|
| USG-01 | 用量总览 | PASS | 39ms | 20KB 总览数据 |
| USG-02 | 用量趋势 | PASS | 22ms | |
| USG-03 | top-models | PASS | 21ms | |
| USG-04 | top-agents | PASS | 20ms | |
| USG-05 | 用量事件 | PASS | 27ms | 65KB，token 事件流真实记录 |
| USG-06 | all-models-breakdown | PASS | 23ms | |
| USG-07 | context-budget-stats | PASS | 43ms | |
| USG-08 | 预算告警列表 | PASS | 33ms | 空集（未配置，正常） |
| USG-09 | 配额读取（未配置时） | PASS | 22ms | 404 符合「无配置」语义 |
| USG-10 | 配额 upsert | PASS | 38ms | workspace/default 写入日/月限额 |
| USG-11 | 配额检查 | PASS | 20ms | check 端点 200 |

## 原因分析
- 读路径 8 个端点全部 200 且有真实数据，token 用量在真实 LLM 调用下持续累积（本测试期间 deepseek 调用已被记录）。
- 写路径 upsert→check 闭环正常；未配置时读 404 语义明确。
- 预算告警为空集：从未配置过告警规则，属初始状态。

## 解决方案
- 无需修复。测试写入的 workspace/default 配额（日 1e8/月 3e9 token）为宽松阈值，不影响生产判定；如需还原可由用户裁定。
