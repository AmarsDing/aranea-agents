# 18-performance 性能测试用例

> 目标：接口耗时基线 / 并发吞吐 / 资源占用 / 大载荷 / DB 响应。真机 Docker 部署版 aranea-admin :8810。

| ID | 用例 | 预期/度量 |
|----|------|-----------|
| PERF-01 | 关键只读接口耗时基线（9 接口 × 3 次取均值） | 记录 mean/min/max，均值 <500ms 为佳 |
| PERF-02 | 并发吞吐：10 并发 × 5 轮 GET /v1/agents | 全部 200，记录总耗时/均值/P95 |
| PERF-03 | 容器资源占用 docker stats（admin/postgres/redis） | 记录 CPU%/MEM，无异常飙高 |
| PERF-04 | 大载荷：会话消息分页 page_size=100 | 200 且耗时记录 |
| PERF-05 | DB 响应：SELECT 1 + 核心表行数统计耗时 | <100ms 级 |
| PERF-06 | 并发下混合只读接口（agents+tools+sessions 各 5 并发） | 全部 200，无 5xx/超时 |
