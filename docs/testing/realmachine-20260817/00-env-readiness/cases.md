# 00 环境就绪 测试用例

| ID | 用例 | 预期 |
|----|------|------|
| ENV-01 | GET /healthz | 200 + status=ok |
| ENV-02 | gRPC :9910 TCP 连接 | 可连接 |
| ENV-03 | WS :8812 TCP 连接 | 可连接 |
| ENV-04 | PG 库表数与迁移数 | tables>50 且迁移已应用 |
| ENV-05 | aranea-redis ping | PONG |
| ENV-06 | TwinMonitor gateway :8000/healthz | 200 |
| ENV-07 | GNS3 agent :18081 监听 | LISTENING |
| ENV-08 | dev/dev 登录 | 签发 JWT |
| ENV-09 | admin 24h 日志 panic/fatal 扫描 | 0 命中 |
| ENV-10 | skills 卷挂载 | 含 /root/.config/Aranea/skills |
| ENV-11 | GET /v1/system/info | 200 |
