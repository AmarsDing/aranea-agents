# Memory L0–L4 Review

> **评分**：96 / 100 | **风险等级**：P3  
> **文档**：[memory/README.md](../需求/memory/README.md) · [memory.design.md](../需求/memory/memory.design.md) · [memory-development.md](../需求/memory/memory-development.md)  
> **代码锚点**：`internal/biz/memory.go` · `internal/runtime/memory_set.go` · `internal/biz/memory_l4*.go` · `internal/memory/trpc/` · `internal/data/sessionmemory/` · `internal/agent/l3_prompt.go` · `internal/agent/memory_inject.go`  
> **审查时间**：2026-05-24（Wave 1–3 + MEM-R + backlog 优化后）

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 19 | 20 | L3/L2 运行时注入 ✅；Cascade Approve 索引同步 ✅ |
| 架构一致性 | 23 | 25 | L3/L4 去重、provenance、Action Log turn_id ✅ |
| 后端实现质量 | 20 | 20 | CE rerank proxy；PII scan；Composite/Debug RPC；Policy strict ✅ |
| 前端实现质量 | 15 | 15 | Graph Explorer + Recall Tester + Worker 状态 + Cascade 路径修复 ✅ |
| 测试与验证 | 10 | 10 | L2 recall / Policy / PII / CE rerank 单测 ✅ |
| 文档一致性 | 9 | 10 | 本文已同步 backlog 闭合项 |

---

## Memory 五层模型

| 层 | 名称 | 存储 | 状态 |
|----|------|------|------|
| L0 | Sensory（感官）| trpc Session（上下文窗口） | ✅ |
| L1 | Working（工作记忆）| SQLite `sessionmemory` | ✅ |
| L2 | Episodic（情节记忆）| SQLite，按对话分组 | ✅ |
| L3 | Semantic（语义记忆）| SQLite `memory_facts`（主链）；pgvector 可选 | ✅ 基础 |
| L4 | Persistent（持久记忆）| SQLite graph + 实体关系 | ✅ 基础；衰减 ✅；图谱 Tab ✅ |

---

## 架构边界 — P0 已闭合

`memory_runtime_set.go` 已删除；`internal/runtime/memory_set.go` 负责框架 MemoryService 组装，biz 层仅持端口类型。`make runtime-boundary` CI 通过。

## 双轨关系（文档已定稿）

| 轨 | 职责 |
|----|------|
| `trpc-agent-go/memory.Service` | Runner 运行时真相源 |
| Aranea L0–L4 + Admin | 产品实现与治理面；SQLite `sessionmemory` 为默认存储 |

详见 [memory.design.md §二](../需求/memory/memory.design.md)。**代码层**仍待 L3 写路径收敛（facts 权威 vs pgvector 平行轨）。

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| L0-L3 读/写 | ✅ |
| L4 prompt 注入 | ✅ |
| L3/L2 运行时 prompt 注入（`l0_inject_l3` / `l2_recall_enabled`）| ✅ MEM-R01 |
| L4 图自动写入（`L4GraphWriter`）| ✅ M4 |
| MemoryWorker（异步处理）| ✅ |
| AutoMemory 图写入（cronrunner `auto_memory` job）| ✅ |
| `l4_governance.go` 冲突检测 | ✅ I2-MEM-01 |
| 级联更新 | ✅ Approve 后 SyncFactIndex + 整词 fact 重命名 |
| 衰减算法 | 🟡 MVP 元数据 |
| Memory 管理页 5 Tab | ✅ |
| 图谱与进化 Tab | ✅ BFS neighborhood + evolution 卡片 |
| Recall 调试 / Composite Search | ✅ 设置 Tab + RPC |
| Memory Worker 状态 RPC | ✅ |
| `MemorySnapshotDrawer.vue` | ✅ |
| pgvector L3（Agent 级别） | ✅ 基础 |
| pgvector 多租户稳定性 | 🟡 暂缓 |

---

## 主要风险

### P0

| ID | 问题 | 状态 |
|----|------|------|
| MEM-P0-01 | `biz/memory_runtime_set.go` 违反 biz 不 import trpc 红线 | ✅ 已闭合 |

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| MEM-P1-01 | L3 三写路径未收敛 | ✅ facts 权威 + pgvector 索引 |
| MEM-P1-02 | `MemoryWorker` 无 LLM 管道 | 🟡 LLM→启发式链 + fallback 指标 ✅ |
| MEM-P1-03 | 记忆中心「图谱与进化」Tab 为占位 | ✅ Graph Explorer + evolution 面板 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| MEM-P2-01 | 级联 BFS + CascadeProposal 未产品化 | ✅ 后端 RPC + 门控 + Memory Center Tab |
| MEM-P2-02 | pgvector 多租户稳定性待验证 | ⏸ 暂缓 — 补多租户集成测试 |

---

## MEM-R 综合 review 修复（2026-05-24）

| ID | 级别 | 问题 | 状态 |
|----|------|------|------|
| MEM-R01 | P1 | `l0_inject_l3` 无运行时实现 | ✅ `L3MemoryCue` + BeforeModel 注入 |
| MEM-R01b | P1 | `l2_recall_enabled` 无注入 | ✅ `L2MemoryCue` + `RecallL2Episodes`（keyword/vector/decay rerank） |
| MEM-R02 | P1 | L3/L4 重复 regex 提取 | ✅ auto_memory 跳过已有 L3 的 user 消息 |
| MEM-R03 | P2 | Cascade Approve 后 pgvector stale | ✅ `SetIndexSync` + 批量 `SyncFactIndexFromRow` |
| MEM-R04 | P2 | 缺集成测 | ✅ queue→extract→fact→episode 单测 + 注入队列 drain 测 |
| MEM-R05 | P3 | Action Log 不完整 | ✅ `turn_id` 写入 metadata；Upsert/Cascade 扩面 |
| MEM-R06 | P3 | L2 recall / decay / rerank | ✅ `store_l2_recall.go` + episode embedding sync |
| 架构 | — | Policy 审计分散 | ✅ `MemoryPolicyEngine` + store `recordPolicyBestEffort` |
| 架构 | — | provenance 粗 | ✅ `MemoryProposal.SourceMessageID` + 启发式/LLM 回溯 |
| 代码 | — | `ReplaceNameInAgentFacts` 子串误伤 | ✅ 整词 regex + 两阶段读写 |
| 代码 | — | AutoMemory 队列耦合 | ✅ Wire 显式 `MemoryJobQueue`；移除 global 默认 |

---

## 建议优化路径

### 已完成（本迭代）

| 项 | 说明 |
|----|------|
| L2 decay cron | `MemoryL2DecayWorker` 24h 批量衰减 `memory_episodes.importance` |
| L3 rerank | `RecallL3Facts` keyword/vector/importance/recency 融合；`L3MemoryCue` 已切换 |
| L3 全局 importance 衰减 cron | `MemoryL3DecayWorker` 24h + `ApplyAllFactImportanceDecay` |
| Cross-Encoder rerank | 词级 bigram Jaccard proxy；L2/L3 recall 末段 CE 重排 |
| MemoryWorker 指标 | 内存计数器 + `GetMemoryWorkerStatus` RPC |
| 图谱与进化 Tab | `MemoryGraphExplorer` BFS SVG + evolution 卡片 |
| L2/L3 recall tester UI | 设置 Tab `MemoryRecallTesterPanel` + `DebugMemoryRecall` |
| Episode embedding backfill | `MemoryEpisodeBackfillWorker` 6h + `ListEpisodesPendingEmbedding` |
| PII 检测与 redaction | `ScanPII` on fact upsert → `pii_flag` / `redacted_statement` |
| Composite Search | `CompositeSearchMemories` RPC（L2+L3 融合） |
| Policy strict 模式 | `MEMORY_POLICY_STRICT` env；关键 mutation 审计失败阻断写 |
| Cascade HTTP 路径 | 前端 `v1/memory/cascade/...` 与 proto 对齐 |

### 后期优化 backlog

| 优先级 | 项 | 说明 |
|--------|-----|------|
| P2 | pgvector 多租户集成测 | agent+user 分区隔离、并发 upsert（**暂缓**） |
| P3 | 真实 Cross-Encoder 模型 | 替换 lexical proxy；需模型服务 |
| P3 | Recall debug 分数分解 | 从 recall JSON 解析 keyword/vector 分量（当前 CE+total） |
| 长期 | 图谱 Tab 高级可视化 | 力导向布局、时间轴 valid_from/valid_to |
