# 06 五层记忆 测试用例与结果

## 用例

| ID | 用例 | 预期 |
|----|------|------|
| MEM-01 | GET /v1/memory/layer-overview | 200 |
| MEM-02 | GET /v1/sessions/{sid}/l0/snapshots | 200 |
| MEM-03 | GET /v1/sessions/{sid}/l1/tasks | 200 |
| MEM-04 | GET /v1/memory/l3/facts | 200 + items |
| MEM-05 | GET l3/facts/conflicts | 200 |
| MEM-06 | GET l4/entities | 200 |
| MEM-07 | GET /v1/memory/episodes | 200 |
| MEM-08 | GET worker/status | 200 |
| MEM-09 | GET worker/dead-letters | 200 |
| MEM-10 | GET platform/settings | 200 |
| MEM-11 | POST l3/facts 写入+列表验证 | 200 + 可检索 |
| MEM-12 | POST recall/debug | 200 |
| MEM-13 | POST search/composite | 200 |
| MEM-14 | GET agents/{id}/identity | 200/404 |
| MEM-15 | GET graph/unified | 200 |
| MEM-16 | GET cascade/proposals | 200 |
| MEM-17 | POST facts/{id}/review confirm | 200 |

## 结果：17/17 PASS

| ID | 结果 | 耗时 | 说明 |
|----|------|------|------|
| MEM-01B | PASS | 68ms | 五层总览 4.1KB |
| MEM-02 | PASS | 27ms | L0 快照空（spirit 平凡对话未沉淀） |
| MEM-03 | PASS | 23ms | L1 任务空 |
| MEM-04 | PASS | 29ms | facts 14.6KB |
| MEM-05B | PASS | 22ms | 无冲突 |
| MEM-06 | PASS | 24ms | L4 实体 6.2KB |
| MEM-07B | PASS | 22ms | episodes 11KB |
| MEM-08 | PASS | 23ms | worker 状态 516B |
| MEM-09 | PASS | 21ms | 死信空 |
| MEM-10 | PASS | 35ms | |
| MEM-11C/D | PASS | 321/27ms | 写入→列表命中，写路径闭环 |
| MEM-12B | PASS | 28ms | 召回调试 5.2KB |
| MEM-13B | PASS | 29ms | 复合检索 833B |
| MEM-14 | PASS | 22ms | identity 137B |
| MEM-15B | PASS | 36ms | 统一图 650B |
| MEM-16B | PASS | 22ms | 级联提案空 |
| MEM-17 | PASS | 24ms | confirm 治理动作成功 |

## 原因分析

- **五层链路全部可用**：L0/L1（会话态）、L2 episodes、L3 facts（写入/列表/治理）、L4 实体、worker/死信、级联提案、统一图、召回调试均 200。
- **入参契约严（非缺陷）**：多数读接口强制 `agent_id`；写接口要求 `scope_type+scope_id+agent_id` 三元组 + `statement`。初测 8 个 400 全部是用例缺参，服务端 reason 精确（`agent_id is required` / `scope_id is required for agent writes`）。
- **记忆沉淀观察**：spirit 平凡 PONG 对话未产生 L0 快照/L1 任务（符合设计：简单轮次不沉淀）。长任务沉淀在 10-monitor-scenario 验证。

## 解决方案

- 无需修复。建议（低优）：API 文档/前端对必填参数做空态引导；`layer-overview` 支持无 agent_id 的平台级总览（当前强制 agent 维度）。
