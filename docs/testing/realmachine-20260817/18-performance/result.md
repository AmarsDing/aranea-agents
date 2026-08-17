# 18-performance 性能测试结果（2026-08-17）

> 真机 Docker 部署版 aranea-admin :8810。结果：**14 PASS / 0 FAIL**（首轮 4 个 FAIL 均为测试脚本路径/表名错误，修正后全过；见「踩坑记录」）。

## 接口耗时基线（×3 均值）

| 接口 | mean | min | max | 评价 |
|------|------|-----|-----|------|
| /healthz | 25ms | 22 | 29 | 优 |
| /v1/agents | 29ms | 27 | 33 | 优（312 个 agent） |
| /v1/sessions | 26ms | 22 | 33 | 优 |
| /v1/tools | 101ms | 99 | 103 | 良 |
| /v1/teams | 44ms | 31 | 57 | 优 |
| /v1/graphs | 27ms | 23 | 29 | 优 |
| /v1/memory/layer-overview?agent_id | 53ms | 36 | 85 | 优 |
| /v1/monitor/flow-logs | 20ms | 20 | 21 | 优 |
| **/v1/model-catalog/providers** | **~510ms** | 492 | 524 | **劣化 20-50 倍，需排查** |

## 并发与吞吐

| 场景 | 结果 | 数据 |
|------|------|------|
| 10 并发 × 5 波 GET /v1/agents | 50/50 成功 0 失败 | mean=415ms p95=462ms max=465ms 总 8.2s |
| 混合并发（agents+tools+sessions 各 5） | 15/15 成功 0 失败 | mean=460ms max=552ms |

> 测量口径说明：客户端为 PowerShell Start-Job（每 job 独立进程，启动开销数百 ms），415ms 均值含显著客户端开销；同期容器 CPU 0.31% 未见压力，服务端真实并发能力需用 wrk/k6 类工具复核。

## 资源占用（docker stats 快照）

| 容器 | CPU | MEM |
|------|-----|-----|
| aranea-admin | 0.31% | 137.8MiB / 15.49GiB (0.87%) |
| twinserver-postgres | 0.01% | 366.4MiB (2.31%) |
| twinserver-redis | 0.30% | 11.72MiB (0.07%) |

26 容器整体平稳，无内存/CPU 异常。

## DB 响应与数据规模

SELECT 1 = 230ms（含 docker exec 进程开销）；表行数：agents=312, turns_v2=751, trpc_session_events=22024, sessions_v2=0（会话实际存 sessions 系表）。

## 发现与解决方案

### F1（真实性能问题·待优化）：/v1/model-catalog/providers 均值 ~510ms
- **现象**：稳定 492~524ms，是其他只读接口的 20-50 倍；15-provider 模块测试时已观察到该接口偏慢。
- **原因假设**：catalog 提供方列表可能同步聚合了模型数/定价/同步状态等多表 join 或内存态拼装，未走索引/缓存。
- **解决方案**：①对该 handler 加 pprof 或 span 计时定位热点；②若为聚合统计，加缓存或改异步预计算；③前端该页做骨架屏容忍。优先级 P2（后台管理页，非热路径）。

### F2（并发测量口径）：PS Start-Job 客户端开销大
- 后续若需精确并发指标，改用 k6/wrk/vegeta 在容器网络内打流。

## 踩坑记录（测试脚本自身问题，已修正）

1. `/v1/chat/sessions` → 正确为 `/v1/sessions`（02 模块一致）。
2. `/v1/memory/overview` → 正确为 `/v1/memory/layer-overview`，且**必须带 agent_id**（否则 400 MEMORY_BAD_REQUEST）。
3. `/v1/observability/flowlogs` → 正确为 `/v1/monitor/flow-logs`。
4. DB 表名 `chat_sessions` 不存在 → 实际为 `sessions_v2` / `turns_v2` / `trpc_session_events`（与 02 模块发现的 messages→turns_v2 改名一致）。
