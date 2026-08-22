# 记忆中心信任闭环复审（P0–P3 后）

> 日期：2026-08-22  
> 类型：review  
> 对照：同日初评（聊天侧「五层内核 + 身份分裂管理页」）+ `memory.md` §23 / Phase 8 T8-1～T8-7  
> 范围：Memory Center 产品闭环、租户门禁、全景聚合、页编排。不含 Neural Memory。

## 概要

产品适配分（相对初评，满分 10）：**4 → 7**。§23 验收 15 项均已勾。主缺口从「体验断裂」变成「门禁未铺满 + 进化只改状态」。

| 维度 | 🔴 阻断 | 🟡 建议 | 说明 |
|------|---------|---------|------|
| 产品闭环（四句用户问题） | 0 | 2 | 已从弱/部分抬到可用/部分偏强 |
| 安全（IDOR） | 1 | 1 | 聚合口已锁；浏览/进化/L0L1 未锁 |
| 性能 | 0 | 2 | L3/L4 SQL 聚合已上；L0/L1/图谱仍扫描 |
| 前端数据流 | 0 | 1 | 页已拆；图谱/激活仍直调 api |
| 业务正确性 | 1 | 1 | 进化批准不 apply 画像 |

## 四个用户问题：前后对照

| 用户问题 | 初评 | 复审 | 抬升 |
|----------|------|------|------|
| Agent 现在看见了什么？ | 部分：有 L0 表，无瀑布；页顶 KPI 与全景两套数字 | 强：L0 瀑布、全景单源、`fact_injected` 动态 | +2 |
| 正在记住什么任务状态？ | 弱：只有 L1 任务列表 | 部分+：字段树 + used/budget；无写回滚 | +2 |
| 这次会话发生过什么？ | 弱：L2 `sessionId` 写死 `null` | 强：时间线绑定会话 + 会话详情「查看记忆」 | +3 |
| 长期知道什么、如何进化？ | 部分：L3 CRUD/冲突可用，进化只读 | 部分+：可批/拒/回滚**状态**；不自动改画像/策略 | +1.5 |

## 计划项落地

| 阶段 | 目标 | 结果 | 折扣 |
|------|------|------|------|
| P0 | ACL / 深链 / KPI 单源 / 假清单 / L2 会话 | 五件全落地 | 无 |
| P1 | Trust/Ops / 注入动态 / 会话入口 | 三件全落地 | 无 |
| P2 | L0 瀑布 / L1 树 / 进化审批 / PII | UI 全落地 | 批准只改 `proposal.status`；L1 无写回滚 |
| P3 | SQL 聚合 / 页拆分 / Store / 文档 | 主路径落地 | 图谱/激活仍直调 api；L0/L1 全景仍扫描 |

## 阻断项（复审仍建议修）

| ID | 端 | 文件 | 问题 | 修复建议 |
|----|----|------|------|----------|
| R1 | 后端 | `internal/service/memory.go` | `ListMemoryFacts` / `List|AppendEvolution*` / `GetAgentIdentity|Strategy` 未走 `assertAgentMemoryAccess`。`authorizeMemoryScope(agent)` 只要求登录。浏览/信任主路径仍可带他人 `agent_id` 读改。 | 凡带 `agent_id` 的 Memory Center RPC 复用全景同一 IDOR（跨租户 NotFound） |
| R2 | 产品 | `memory_shim_l4.go` `applyEvolutionEventSideEffects` + Trust UI | 批准只把 `pending→approved`，不写 identity/strategy | 接 apply 通道，或 UI 标明「仅记录决议」 |

## 建议项

| ID | 端 | 问题 | 建议 |
|----|----|------|------|
| S1 | 后端 | `GetLayerOverview` L0 拉 100 行扫今日；L1 按任务 N+1 `ListL1FieldRows` | COUNT/SUM，字段数一次聚合 |
| S2 | 后端 | 统一图谱仍 `unifiedGraphScanLimit=500` | 与全景一样截断/聚合 |
| S3 | 前端 | `useUnifiedMemoryGraph` / `useActivationReplay` 仍 `import api` | 迁 `useMemoryStore`，对齐 P3 |
| S4 | 产品 | L1 回滚无写 API | 保持只读 revision，或单开写 RPC |
| S5 | 后端 | `ListL0Snapshots` / `ListL1Tasks|Fields` 只校验 session_id 非空 | 经 session→agent 再走 IDOR |

## 亮点

- 聊天 → `/memory?...&factId=` → 事实抽屉已通；L3 keyword 可按 id 命中。
- `requireAdmin` 改名为 `requireWired`，避免「已鉴权」幻觉。
- 全景生产路径 `MemoryOverviewStatsReader`：L3 一条 SQL（active / today / `SUM(use_count)`），动态只拉最近 N 条抽取+注入。
- Trust / Ops 拆分：非管理员看不到 Worker/死信；`tab=ops` 回落治理。
- 页编排从 900+ 行 composable 拆为 facts / session / trust，全景与情景经 store。

## 合规清单（本轮相关）

后端：

- [x] 全景 / 图谱 / 情景 / Debug / CompositeSearch / PII 列表：Agent 租户 IDOR
- [ ] 事实列表 / 进化读写 / 画像 / L0 / L1：同一 IDOR
- [x] 跨租户统一 NotFound（已覆盖的口）
- [x] 业务错误 `apierror`；日志 `loggateway`
- [x] 全景聚合端口未塞进 `MemoryAdminDeps`（fake 扫描兜底）

前端：

- [x] Page 不 import `features/*/api`
- [x] 展示组件不 import Store
- [x] 页编排已拆 composable
- [ ] 图谱 / 激活 composable 仍直调 api（FL5）

## As-built（同日 T8-8）

R1 主路径已铺：事实列表 / 冲突 / 进化读写 / 画像 / L0·L1（带 `agent_id`）走同一 IDOR。R2 取诚实文案，不自动 apply。图谱 / 激活改经 store。级联按 id 的写口、L0/L1 全景扫描、统一图谱 500 行仍待清理。
