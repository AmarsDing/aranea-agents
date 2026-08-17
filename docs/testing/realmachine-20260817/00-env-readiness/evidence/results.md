| ENV-01 | aranea /healthz | PASS | 26ms | code=200 auth=jwt |
| ENV-02 | gRPC :9910 TCP 可达 | PASS | 5ms |  |
| ENV-03 | WS :8812 TCP 可达 | PASS | 0ms |  |
| ENV-04 | PG 库表/迁移 | PASS | 0ms | tables=189 migrations=145 |
| ENV-05 | aranea-redis ping | PASS | 0ms | PONG |
| ENV-06 | TwinMonitor gateway :8000 | PASS | 0ms | code=200 |
| ENV-07 | GNS3 agent :18081 监听 | PASS | 0ms | root_code=404 |
| ENV-08 | dev/dev 登录签发 JWT | PASS | 0ms | token_len=335 |
| ENV-09 | admin 24h 日志 panic/fatal | PASS | 0ms | hits=0 |
| ENV-10 | skills 卷挂载 | PASS | 0ms | /app/logs;/app/data;/root/.config/Aranea/skills;/app/conf; |
| ENV-11 | 系统信息接口 | PASS | 25ms | code=200 {"version":"","git_commit":"","build_time":"","default_provider":"deepseek","default_model":"deepseek-v4-flash","skill_s |
