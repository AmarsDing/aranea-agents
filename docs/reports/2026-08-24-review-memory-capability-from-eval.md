# 从记忆评测反推的系统能力优化（非刷分）

> 类型：review（能力分析 + 已落地的生产改动）
> 日期：2026-08-24
> 范围：L3 召回热路径、事实种类、时间轴；评测适配层只作消费者

---

## 1. 立场

Agent Memory Challenge 的 A–H 维是 **Agent 长期记忆的产品能力清单**，不是排行榜配方。

| 评测维 | 产品能力 | 优化对象 |
|--------|----------|----------|
| A 显式事实 | 每轮聊天能从 L3 找回埋在长历史里的陈述 | 生产 `RecallL3Facts` |
| B 多跳 | L4 + composite + 链接 | 已有 composite 注入，本轮不另开评测 hops |
| C 时序 | 世界状态年龄 ≠ 刚写过/刚用过 | 打分时间轴拆开 |
| D 治理 | 同槽更新覆盖、冲突管道 | `CanonicalizeFactKind` + 已有 FactWritePipeline |
| E 个性化 | 偏好常青、用户作用域 | `preference` 与 `user_preference` 对齐常青 |
| G 规则 | constraint 种类 | 陈述启发式，不用「always」误伤习惯句 |
| H 隐私 | 生产默认脱敏 | **不**为评测关掉 PII 门禁 |

评测 Add/Search 继续调用生产召回。禁止再往共享 L3 上堆「只为 AML 的全表扫 blob / SkipPII」。

---

## 2. 评测暴露的真实缺口（已修）

### 2.1 有向量的小库从不走语义检索

典型 Agent 活跃事实远小于 5000。旧逻辑 `count ≤ 阈值` 就 brute-force，**pgvector+FTS RRF 在聊天热路径上等于没开**。

这是产品缺陷，不是评测缺陷：用户问「Alice 喜欢什么颜色」时，语义通道本应连接 color↔blue。

**修法**：`RecallL3Facts` 在 vector store + query embedding 可用时 **一律**走 RRF；词法扫描只给无向量通道。

### 2.2 词法扫描拖着 embedding_blob

小库全扫带 1536-d blob，每轮十几 MB。向量路径已经用 hit map，不需要把 blob 拉进 Go。

**修法**：无 query embedding 时 `NULL AS embedding_blob`。

### 2.3 衰减和 recency 抢同一根时间轴

巩固/同指纹 upsert 刷新 `updated_at`，但 `valid_from` 保留。只用事件时间会让「刚改过的偏好」看起来像旧事实；只用入库时间会让「很久以前发生、从未再确认」的事件一直新。

**修法**：decay ← `valid_from`/`created_at`；recency ← `last_used_at`/`updated_at`。

### 2.4 问句是产品查询形态

用户不会提交 BM25 AND 关键词。停用 what/does/the、FTS 对内容词 OR，是聊天检索该有的行为。`may`/`will` 从停用词拿掉（人名）；`user`/`用户` 因库内高频而丢掉。

### 2.5 种类词表分裂

抽取管道写 `preference`，常青表只认 `user_preference`，偏好会按事件衰减。vague kind（event/fact/general）也不从陈述恢复成偏好/规则。

**修法**：`isEvergreenFactKind` 走 `CanonicalizeFactKind`；空/event/fact 陈述启发式（like/prefer/favorite/live → preference；must/never/必须 → constraint；**不用 always**）。AutoMemory 写入前 canonicalize。

---

## 3. 明确不做的（迎合评测）

| 项 | 原因 |
|----|------|
| 评测 Search 专用全表扫描 / 更大 FTS 池 | 生产热路径延迟不可接受 |
| `SkipPIIRedact` 作为产品默认 | 违反隐私门禁；适配层可继续用，但不得扩大到 Admin |
| 把每条原始消息当 L3 event | 产品写入应走抽取 + FactWritePipeline |
| 评测 regex 独占的 supersede | 槽位规则已收到 `ShouldSupersedeSameSlotFact`，给生产复用 |

---

## 4. 后续能继续抬系统、仍与评测无关的方向

1. **ImmediateFactWriter 同槽覆盖** — ✅ 对话里「现在改成红色」当轮 `SupersedeFact`，不接 FactWritePipeline 白名单。
2. **L4 增产 + one-hop 进 composite** — 多跳是图谱能力，不是评测 Search 里互链整批消息。
3. **P1 中文 pg_trgm / 自适应 minScore** — ✅ DDL 20261242 + `word_similarity` 入 RRF；`AdaptiveRecallMinScore`。
4. **把 SkipPIIRedact 移出公共 `FactUpsert`** — 评测专用写口，避免 DTO 被 JSON 解进来绕过脱敏。

---

## 5. 代码锚点

| 能力 | 文件 |
|------|------|
| 向量优先路由 / lite SELECT | `internal/data/memory_shim_l3.go` |
| FTS OR / queryFactRowsByIDs blob 开关 | `internal/data/memory_l3_fts.go` |
| 停用词 / touch vs event / evergreen | `internal/data/memory_helpers.go` |
| 种类启发式 / 同槽覆盖 | `internal/biz/fact_kind.go` |
| ImmediateFactWriter 同槽 | `internal/biz/immediate_fact_writer.go` |
| 自适应 minScore | `internal/biz/memory_l3_fused_recall.go` `AdaptiveRecallMinScore`；`data/memory_shim_l3.go` `finalizeScoredFacts` |
| CJK trigram | `internal/data/memory_l3_trgm.go` + DDL 20261242 |
| AutoMemory canonicalize | `internal/cronrunner/jobs/auto_memory.go` |
