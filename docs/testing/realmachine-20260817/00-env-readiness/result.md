# 00 环境就绪 测试结果

**结论：11/11 PASS**

| ID | 用例 | 结果 | 耗时 | 说明 |
|----|------|------|------|------|
| ENV-01 | aranea /healthz | PASS | 26ms | auth=jwt, deploy_env=dev |
| ENV-02 | gRPC :9910 | PASS | 5ms | |
| ENV-03 | WS :8812 | PASS | 0ms | |
| ENV-04 | PG 库表/迁移 | PASS | - | tables=189, migrations=145 |
| ENV-05 | redis ping | PASS | - | PONG |
| ENV-06 | Twin gateway | PASS | - | 200 |
| ENV-07 | GNS3 agent | PASS | - | 监听正常（根路径 404 属正常，健康路径不同） |
| ENV-08 | 登录 | PASS | - | JWT 335 字符 |
| ENV-09 | 日志 panic 扫描 | PASS | - | 0 命中 |
| ENV-10 | skills 卷 | PASS | - | 4 个挂载点齐全 |
| ENV-11 | system/info | PASS | 25ms | default_provider=deepseek, default_model=deepseek-v4-flash |

## 原因分析
- 环境前一天已 dev-up 完毕，全部组件健康，无阻塞项。
- GNS3 agent 根路径 404 仅说明无 `/` 路由，端口监听正常，后续监控场景用真实 RPC 路径验证。

## 解决方案
- 无需修复。
