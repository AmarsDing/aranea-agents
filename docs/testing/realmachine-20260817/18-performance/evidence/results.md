| PERF-01 | baseline /healthz | PASS | 25ms | mean=25ms min=22ms max=29ms n=3 |
| PERF-01 | baseline /v1/agents | PASS | 29ms | mean=29ms min=27ms max=33ms n=3 |
| PERF-01 | baseline /v1/chat/sessions | FAIL | 0ms | all samples non-200 |
| PERF-01 | baseline /v1/tools | PASS | 101ms | mean=101ms min=99ms max=103ms n=3 |
| PERF-01 | baseline /v1/teams | PASS | 44ms | mean=44ms min=31ms max=57ms n=3 |
| PERF-01 | baseline /v1/graphs | PASS | 27ms | mean=27ms min=23ms max=29ms n=3 |
| PERF-01 | baseline /v1/memory/overview | FAIL | 0ms | all samples non-200 |
| PERF-01 | baseline /v1/observability/flowlogs?page_size=20 | FAIL | 0ms | all samples non-200 |
| PERF-01 | baseline /v1/model-catalog/providers | FAIL | 515ms | mean=515ms min=511ms max=519ms n=3 |
| PERF-02 | 10-concurrency x5 waves GET /v1/agents | PASS | 415ms | total=8179ms n=50 fail=0 mean=415ms p95=462ms max=465ms |
| PERF-03 | docker stats snapshot | PASS | 0ms | containers=26; aranea-admin|0.31%|137.8MiB / 15.49GiB|0.87% ; twinserver-postgres|0.01%|366.4MiB / 15.49GiB|2.31% ; twinserver-redis|0.30%|11.72MiB / 15.49GiB|0.07% |
| PERF-04 | messages page_size=100 | FAIL | 0ms | no session found |
| PERF-05 | DB SELECT 1 + table counts | PASS | 230ms | select1=230ms counts=204ms [agents=312] |
| PERF-06 | mixed concurrent reads (3 eps x5) | FAIL | 486ms | n=10 fail=5 mean=486ms max=579ms |
| PERF-01R | baseline(corrected) /v1/sessions | PASS | 26ms | mean=26ms min=22ms max=33ms n=3 |
| PERF-01R | baseline(corrected) /v1/memory/layer-overview | FAIL | 0ms | all non-200 |
| PERF-01R | baseline(corrected) /v1/monitor/flow-logs?page=1&page_size=20 | PASS | 20ms | mean=20ms min=20ms max=21ms n=3 |
| PERF-01R | baseline(corrected) /v1/model-catalog/providers | PASS | 506ms | mean=506ms min=492ms max=524ms n=3 |
| PERF-04R | messages page_size=100 (corrected) | PASS | 30ms | code=200 sid=f23cc178-0808-4620-9f54-c987bbec8d32 bytes=6412 |
| PERF-05R | DB table counts (corrected) | PASS | 0ms | agents=312, sessions_v2=0, turns_v2=751, trpc_session_events=22024 |
| PERF-06R | mixed concurrent reads corrected (3 eps x5) | PASS | 460ms | n=15 fail=0 mean=460ms max=552ms |
| PERF-01R2 | baseline layer-overview?agent_id (corrected) | PASS | 53ms | aid=agent___spirit__ samples=85/36/39 |
