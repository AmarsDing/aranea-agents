# 自治理知识图谱架构 — 设计文档（评审稿）

> 对应需求：`37-knowledge.md` · `memory/memory.md`
> 关联设计：`37-knowledge.design.md`（检索/摄取主链路）· `memory/memory.design.md`（L0–L4 记忆）
> 状态：**评审稿**（2026-08-15，待评审定稿后拆里程碑实施）
> 定位：在现有「文档库 + 词边界双链」之上，补三层能力——语义关系层（懂关系）、时序演化层（懂生长）、自治理层（懂维护），并把记忆管家（`__memory__` agent）的治理范围从 `memory_fact` 扩展到知识库词条。

---

## 0. 背景与问题陈述

### 0.1 现状（代码实证，2026-08-15 核查）

当前知识库的「关联」能力真实状态：

| 能力 | 实现 | 状态 |
|---|---|---|
| 词边界双链 | `AutolinkOutgoing` 写入时把提到已有词条标题/别名的词包成 `[[wikilink]]`（`biz/knowledge/autolink.go`） | ✅ 生产在用，唯一在跑的自主关联 |
| 实体共现边 | `ReplaceEntityLinks` / `FindEntityCooccurrences` / `MergeEntities` 管线 + 归一化/别名解析 | ⚠️ 代码与测试齐全，但 `SetEntityHook` 全仓**无生产调用者**（`knowledge/vault_sync.go:49`），未接线 |
| 语义近邻边 | `LinkTypeSemantic` 常量已定义 | ❌ 无建边管线（死代码） |
| 关系类型 | 仅 `explicit`/`entity`/`semantic` 三种**来源类型**，无语义关系谓词 | ❌ 不懂「is-a / part-of / contradicts」 |
| 生长/时序 | 覆盖式修订：新事实经 `replaceH2BlockContaining` 整段顶替旧段 | ❌ 演化轨迹丢失，无 valid_from/valid_to |
| 自治理 | 记忆管家 dream_cycle 治理 `memory_fact`（去重/遗忘/蒸馏） | ⚠️ 只管家 L3 结构化记忆，**不管知识库词条** |

关键结论（`knowledge/graph_expander.go:204-207` 注释自证）：`knowledge_links` 里生产环境**只有 `explicit` 一种边**，`entity×2/semantic×1` 分支为兼容保留的死代码。

### 0.2 根本差距

现在的知识库是「**会分块的文档库 + 词边界双链**」，不是「**自治理、可生长、懂关系的知识体**」。缺三层：

1. **语义关系层**：边没有类型谓词，无法理解「A 是 B 的一种」「A 导致 B」「A 与 B 矛盾」。
2. **时序演化层**：覆盖式写入，新事实顶掉旧事实不留痕，无法回答「截至某天什么是真的」「这个概念从哪演化来」。
3. **自治理层**：知识库词条无人治理——无冲突检测、无陈旧识别、无孤儿治理、无概念涌现。

### 0.3 设计目标的边界声明

本设计**不引入图数据库**（Neo4j 等）。所有能力在 PG 单库上落地（tstzrange + GiST 支持时态区间是 PG 隐藏优势；NeuSymMS arXiv:2605.17596 已实证「符号规则层 + 纯 RDBMS」足够）。检索阶段**零 LLM** 是延迟红线（Zep/LightRAG 共同原则），LLM 只用在写入路径与离线治理。

---

## 1. 理论根基

### 1.1 哲学：受控涌现（governed emergence）

- **schema-first（预定义本体）** 僵化、一改全重构；**schema-free（OpenIE 自由谓词 / folksonomy 打标）** 关系爆炸无法推理。
- 唯一被双边验证的折中（KG 构建综述 arXiv:2510.20345 + Zettelkasten 三层结构论）：**核心关系类型硬编码闭环，LLM 可提议新类型进「候选词表」，由治理周期归并提升为正式类型**。
- **结构不预设**：分类树 / MOC（Map of Content）不预先设计，让密度越阈值的概念簇由 dream_cycle **涌现**生成（GraphRAG 社区摘要的 Zettelkasten 化，呼应卢曼「1000–1500 张卡片时才需要结构笔记」）。

### 1.2 神经科学：四个自组织机制的工程映射

| 脑机制 | 工程映射 | 本设计落点 |
|---|---|---|
| 扩散激活（ACT-R `A=B+ΣW·S`） | `score = 相关性 + β·ln(Σ 访问时间^-0.5)`，融合 Ebbinghaus 遗忘曲线 | L1 检索打分 |
| Hebbian 共激活（fire together wire together） | 同批召回的词条两两强化 `co_activated` 边，闲置周期衰减 | L1 边权 |
| 巩固/再巩固（海马→新皮质） | 情景记忆睡眠期蒸馏为语义词条（Letta sleep-time / HippoRAG 三组件） | L4 dream_cycle |
| 情景 vs 语义记忆（Tulving） | episode（原始会话）/ semantic（词条事实）双层 | 贯穿 L2/L3/L4 |

### 1.3 时序：双时态（bitemporal）为演化地基

Graphiti/Zep（arXiv:2501.13956）事实标准：每条边/事实带 `valid_from`/`valid_to`。冲突时**失效旧边而非删除**，支持 as-of 查询、历史可审计可回滚。这是所有高级功能（冲突检测、演化追踪、as-of 检索）的地基，**先于 LLM 关系抽取落地**（零 LLM 成本）。

### 1.4 三条贯穿性元结论（全调研收敛）

1. **增量 + 失效不删除 + 版本化**是所有成熟系统的共同底线——当前「覆盖式修订」是最大技术债。
2. **检索阶段零 LLM** 是延迟红线——RRF + 扩散激活扩展满足此性质。
3. **schema 受控涌现**——硬编码核心闭环 + LLM 提议 + 治理归并。

---

## 2. 目标架构：受控涌现的四层自治理知识图谱

```
┌──────────────────────────────────────────────────────────────┐
│ L4 治理层（dream_cycle 扩展，离线，记忆管家 __memory__ 执行）      │
│   冲突仲裁 / 陈旧标记 / 孤儿词条提案 / 弱边衰减 /                │
│   hub簇蒸馏成 MOC / 新关系类型归并 / 词条→memory_fact 反向提炼    │
│   → 全部写入 governance_proposal：低风险自动应用，高风险人工二审   │
├──────────────────────────────────────────────────────────────┤
│ L3 演化层（写入路径，在线）—— 懂生长                             │
│   双时态边 valid_from/valid_to + supersedes 版本链              │
│   写入时冲突检测：语义召回候选旧事实 → LLM 仲裁 → 失效不删除       │
├──────────────────────────────────────────────────────────────┤
│ L2 语义层（写入路径，在线 + 夜间回填）—— 懂关系                  │
│   两步 LLM 抽取(先实体后三元组) → 嵌入召回 top-k → LLM 判重归一   │
│   → 受控词表 typed edges（is-a/part-of/depends-on/causes/…）    │
│   激活现有 entity 轨（接 SetEntityHook）+ 新增 semantic 建边      │
├──────────────────────────────────────────────────────────────┤
│ L1 统计层（检索路径，零 LLM）—— 现有能力强化                     │
│   wikilink mentions（保留）+ Hebbian co_activated 边             │
│   + base-level 激活分并入 RRF + 受限扩散激活 2 跳扩展             │
│   （替代现有一跳 Lazy 扩展，召回「语义不像但结构相关」的词条）       │
└──────────────────────────────────────────────────────────────┘
存储：PG 单库——knowledge_links 扩展 / knowledge_access_log /
     knowledge_governance_proposal / knowledge_relation_vocab
```

### 2.1 各层如何满足诉求

| 诉求 | 由哪层实现 |
|---|---|
| 主动记忆意识（knowledge_write 触发） | L1/L2 写入路径 + prompt 层（见 §6） |
| 知识库 = 记忆管家存储器 | L4 把 dream_cycle 治理面扩到词条 |
| 自主关联、关联更多 | L2 LLM 语义关系抽取（超越词边界） |
| 明白事物间关系 | L2 带类型谓词的边 |
| 生长关系 | L3 双时态 + supersedes 版本链 + evolves-from 谱系 |
| 自治理 | L4 治理提案闭环 |

---

## 3. 存储模型设计（存储方案评估结论）

### 3.1 两个候选方案评估

**方案 A：新增独立 `knowledge_edges` 表**，与 `knowledge_links` 并存。
- 优点：不动现有表与死代码，灰度安全；语义/时态边与词边界边物理隔离，治理清晰。
- 缺点：两套边表，扩散激活/图谱查询要 UNION 两源；`graph_expander` 需双读。

**方案 B：扩展现有 `knowledge_links`**，加列承载 typed + 时态。
- 优点：单一边源，图谱/扩散/治理查询统一；`Link` 模型与 `ReplaceLinks`/`ListLinks` 窄接口直接复用；死代码（entity/semantic）原地激活，无需新表迁移对齐。
- 缺点：存量行需回填默认时态；`knowledge_links_unique(doc_id,target_doc_id,link_type)` 唯一索引需扩展以容纳同对文档多谓词。

**评估结论：采用方案 B（扩展 `knowledge_links`）**。理由：
1. `Link` 窄接口已抽象来源类型，`link_type` 本就是开放 string，加谓词是「新增类型值」而非「改模型」；
2. 图谱 `ListCollectionGraph`、扩散激活 `graph_expander`、治理扫描都单表读，避免 UNION 复杂度；
3. 死代码原地激活，消除「两套边」的认知负担。
4. 存量仅 explicit 行，回填 `valid_from=created_at, valid_to=NULL, weight=weight, confidence=1.0` 幂等安全。

### 3.2 `knowledge_links` 扩展 DDL

```sql
ALTER TABLE knowledge_links
  ADD COLUMN IF NOT EXISTS relation     TEXT,                 -- 语义谓词：is-a/part-of/causes/contradicts/supersedes/related-to/…（NULL=纯来源边）
  ADD COLUMN IF NOT EXISTS weight_f     DOUBLE PRECISION NOT NULL DEFAULT 1.0,  -- 浮点权重（Hebbian 用；原 weight INT 保留兼容）
  ADD COLUMN IF NOT EXISTS confidence   DOUBLE PRECISION NOT NULL DEFAULT 1.0,  -- LLM 抽取/仲裁置信
  ADD COLUMN IF NOT EXISTS valid_from   TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS valid_to     TIMESTAMPTZ,          -- NULL=当前有效；冲突失效时关闭
  ADD COLUMN IF NOT EXISTS recorded_at  TIMESTAMPTZ NOT NULL DEFAULT now();     -- 系统摄入时间（双时态第二轴）

-- 唯一约束升级：同对文档同来源同谓词一行（谓词参与去重）
DROP INDEX IF EXISTS knowledge_links_unique;
CREATE UNIQUE INDEX IF NOT EXISTS knowledge_links_unique
  ON knowledge_links (doc_id, target_doc_id, link_type, COALESCE(relation,''));
-- 时态 as-of 查询索引
CREATE INDEX IF NOT EXISTS knowledge_links_valid_idx
  ON knowledge_links USING GIST (tstzrange(valid_from, COALESCE(valid_to,'infinity')));
-- 当前有效边的热路径索引
CREATE INDEX IF NOT EXISTS knowledge_links_active_idx
  ON knowledge_links (collection_id, doc_id) WHERE valid_to IS NULL;
```

> 注：`link_type`（来源：explicit/entity/semantic/hebbian）与 `relation`（谓词：is-a/…）**正交**——前者回答「这条边怎么来的」，后者回答「这条边是什么意思」。词边界 wikilink 的 `relation` 为 NULL（纯提及）。

### 3.3 配套新表

```sql
-- 检索访问日志（Hebbian 共激活 + base-level 激活分的数据源）
CREATE TABLE IF NOT EXISTS knowledge_access_log (
  id            BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  doc_id        TEXT NOT NULL,
  accessed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  query_hash    TEXT,                    -- 共激活批次标识（同一次检索返回的词条同 hash）
  session_id    TEXT
);
CREATE INDEX ON knowledge_access_log (collection_id, doc_id, accessed_at DESC);

-- 治理提案（dream_cycle 输出；低风险自动应用，高风险人工二审）
CREATE TABLE IF NOT EXISTS knowledge_governance_proposal (
  id           BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  kind         TEXT NOT NULL,   -- conflict/stale/orphan/merge/moc_emerge/relation_promote/distill
  payload      JSONB NOT NULL,  -- 提案内容（涉及边/文档/词条）
  risk         TEXT NOT NULL,   -- low/high
  status       TEXT NOT NULL DEFAULT 'pending',  -- pending/applied/rejected
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at  TIMESTAMPTZ
);

-- 关系谓词词表（受控涌现：核心硬编码 + LLM 候选 + 治理归并）
CREATE TABLE IF NOT EXISTS knowledge_relation_vocab (
  relation     TEXT PRIMARY KEY,
  tier         TEXT NOT NULL,   -- core（硬编码闭环）/ candidate（LLM 提议）/ promoted（治理提升）
  proposed_by  TEXT,            -- llm / governance
  use_count    INT NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.4 核心谓词闭集（tier=core，硬编码）

| 谓词 | 方向性 | 含义 | 互斥性 |
|---|---|---|---|
| `is-a` | 有向 | A 是 B 的一种 | 非互斥 |
| `part-of` | 有向 | A 是 B 的组成部分 | 非互斥 |
| `depends-on` | 有向 | A 依赖 B | 非互斥 |
| `causes` | 有向 | A 导致 B | 非互斥 |
| `applies-to` | 有向 | A 适用于 B（场景/约束） | 非互斥 |
| `contradicts` | 无向 | A 与 B 矛盾（触发治理仲裁） | — |
| `supersedes` | 有向 | A 取代 B（版本链） | 互斥（同主体同谓词仅一条 active） |
| `evolves-from` | 有向 | A 从 B 演化而来（概念谱系） | 非互斥 |
| `co-activated` | 无向 | Hebbian 共激活（统计边，无谓词语义） | — |

LLM 抽取时归一化到此词表；LLM 发现词表外关系 → 写入 `tier=candidate`，治理周期评估提升。

---

## 4. 各层详细设计

### 4.1 L1 统计层（检索路径，零 LLM，M1 核心）

**(a) base-level 激活分并入 RRF**：检索打分加一项
```
final = RRF_score + β · ln( Σ_t (now - access_t)^-0.5 )   // 访问日志聚合，ACT-R base-level
```
`β` 可调（默认 0.1）。纯 SQL 聚合 `knowledge_access_log`，零新依赖。

**(b) Hebbian 共激活边**：一次检索返回的 top-k 词条两两 `co_activated` 边 `weight_f += η`（η≈0.1，同 `query_hash` 判定同批）；dream_cycle 周期 `weight_f *= 0.9` 衰减，低于阈值（0.05）失效。

**(c) 受限扩散激活 2 跳扩展**：替代现有一跳 Lazy 扩展。种子 = 混合检索 top-k，沿 `explicit + entity + semantic + co_activated`（仅 `valid_to IS NULL`）传播 2 跳，每跳能量 ×0.5 衰减，侧抑制 = 只保留激活值 top-N。激活值并入最终排序。**仍零 LLM**。

### 4.2 L2 语义层（写入路径，M2 核心）

**(a) 两步 LLM 关系抽取管线**（词条写入/更新时触发 + 存量夜间回填）：
```
词条正文 → [Step1 实体抽取] → 实体清单
        → [Step2 基于实体抽三元组] → (主语, 谓词, 宾语, confidence)
        → 嵌入召回 top-k 已有同义实体/谓词 → LLM 判重归一（KGGen 方法）
        → 写 knowledge_links（link_type=semantic, relation=谓词）
```
两步抽取（先实体后三元组）一致性显著优于单次联合抽取（KGGen/NeurIPS2025）。用 mini 级模型控成本（2–3 次调用/词条）。

**(b) 激活现有 entity 轨**：把 `SetEntityHook` 接到 vault 同步/写回的生产装配层（wire/service），让已有的实体共现管线真正跑起来（当前是接好了内脏没接神经）。

### 4.3 L3 演化层（写入路径，懂生长）

**(a) supersedes 版本链替代覆盖式修订**：`replaceH2BlockContaining` 当前直接顶替旧段。改为：替换前把旧段快照写入版本链，新段与旧段间建 `supersedes` 边，旧段 `valid_to=now()`。**失效不删除**。

**(b) 写入时冲突检测**：新事实写入前，用语义检索召回同词条/同实体的候选旧事实 → LLM 仲裁（`contradicts` 还是 `supersedes` 还是无关）→ 矛盾建 `contradicts` 边并进治理提案，取代关系关闭旧边 `valid_to`。

**(c) evolves-from 谱系**：新概念词条创建时，若由旧概念改写/拆分而来，建 `evolves-from` 边，形成概念谱系（concept lineage）。

### 4.4 L4 治理层（dream_cycle 扩展，记忆管家主管）

把 `__memory__` agent 的 dream_cycle 从「只管 memory_fact」扩展为「同时治理知识库词条」。新增治理任务，**全部输出到 `knowledge_governance_proposal`**：

| 任务 | kind | risk | 应用方式 |
|---|---|---|---|
| 冲突仲裁（contradicts 边裁决） | `conflict` | high | 人工/agent 二审 |
| 陈旧词条标记（valid 关闭比例高 + 久未检索 + 被 supersedes 多次） | `stale` | low | 自动 |
| 孤儿词条（度=0 且创建超 N 天且从未检索）→ 补链/降级/归档提案 | `orphan` | high | 二审 |
| 弱边衰减（co_activated 周期 ×0.9） | `decay` | low | 自动 |
| hub 簇蒸馏成 MOC 词条（密度越阈值概念簇） | `moc_emerge` | high | 二审 |
| 候选关系类型归并提升 | `relation_promote` | low | 自动 |
| 词条→memory_fact 反向提炼（高频召回词条蒸馏成 L3 轻量事实，进 L0 注入） | `distill` | low | 自动 |

**新工具**：`memory_butler_knowledge_curate`（扫描词条库健康，产出上述提案；dry_run 默认）。

---

## 5. 数据流（端到端）

```
写入路径（在线）：
  knowledge_write / 词条编辑 / vault 同步
    → L2 两步关系抽取（semantic typed edges）
    → L3 冲突检测 + supersedes 版本链（失效不删除）
    → L1 autolink（explicit mentions，保留）
    → chunk 重放（既有 SetWriteBackReplay 钩子，不变）

检索路径（在线，零 LLM）：
  knowledge_search / knowledge cue 注入
    → 混合检索（向量+BM25，RRF）
    → L1 base-level 激活分并入
    → L1 受限扩散激活 2 跳（沿 explicit/entity/semantic/co_activated，仅 active 边）
    → 写 knowledge_access_log（共激活批次）→ Hebbian 强化
    → knowledge_recalled 引用回采（既有，不变）

治理路径（离线，记忆管家 dream_cycle）：
  __memory__ agent 每日 dream_cycle
    → 扫描 knowledge_links / access_log / 词条
    → 产出 governance_proposal（7 类任务）
    → 低风险自动应用 / 高风险进二审（沿用 writeback pending HITL 链）
    → hub 簇涌现 MOC / 候选谓词归并 / 词条反向蒸馏
```

---

## 6. 与「主动记忆意识」方案的关系（已评审的 P0/P1 prompt 改动）

本架构是**存储与治理层**；之前评审的主动记忆方案是**触发层**，两者互补、统一于记忆管家语境：

- `memory_remember` → 个人偏好/约束 → `memory_fact`（L3 语义记忆）
- `knowledge_write` → 团队可复用事实 → 词条库（本设计的 L1–L4）
- prompt 层改动（knowledge_write description 触发时机 + 系统提示记忆职责段 + knowledge cue 写入引导）**不变**，作为 L1/L2 写入路径的「意识入口」。
- 分工写进系统提示：个人偏好走 `memory_remember`，团队知识走 `knowledge_write`。

---

## 7. 里程碑（每期独立可验证，符合小步快跑）

| 期 | 内容 | LLM 成本 | 风险 | 价值 |
|---|---|---|---|---|
| **M1** | L1 统计层（base-level 打分 + Hebbian co_activated 边 + 扩散激活 2 跳）+ L3 双时态边表（§3.2 DDL + access_log）+ 接 SetEntityHook | 零 | 低 | 检索质变 + 生长/关联地基 |
| **M2** | L2 两步关系抽取管线 + 存量回填（激活 semantic typed edges） | 低（mini 模型） | 中 | 真正懂关系 |
| **M3** | L4 治理提案闭环（dream_cycle 接管词条，7 类任务 + knowledge_curate 工具） | 低 | 中 | 自治理 |
| **M4** | MOC 涌现 + 概念谱系可视化（前端） | 中 | 低 | 生长可见 |

**M1 先行理由**：纯 Go/SQL、零 LLM 成本、风险最低，立刻让检索带「用进废退 + 结构联想」，并铺好双时态这个所有高级功能的地基。M1 同时「接上神经」（SetEntityHook）让已有实体共现管线真正生产运行，是性价比最高的一步。

---

## 8. 风险与开放问题

1. **Hebbian 边的噪声**：共激活不等于真相关。缓解：`co_activated` 边独立 `link_type`，扩散时权重低于 explicit/semantic；衰减机制兜底。
2. **LLM 关系抽取的准确率现实上限**：显式关系 F1 0.9+，隐式/多跳衰减。缓解：confidence 门槛 + 治理提案二审 + 「人工抽检 + LLM 判分 + 下游增益」三角评估（LREC26 方法论警告：勿只信自动 F1）。
3. **覆盖式修订改版本链的回归面**：`replaceH2BlockContaining` 是写回飞轮热点。M3 才动，配 writeback 全量测试。
4. **谓词词表膨胀**：candidate 层级需治理归并节奏，否则退化回 folksonomy。
5. **开放问题（待定）**：as-of 查询是否暴露给 agent 工具（`knowledge_search` 加 `as_of` 参数）？MOC 词条是否参与默认检索？

---

## 9. 验收标准（草案）

- M1：同一词条多次被检索后 base-level 分上升；co_activated 边随共现增强、随周期衰减；扩散激活 2 跳能召回「无词重叠但有显式/实体边」的词条；双时态边可 as-of 查询。
- M2：给定含「X 是 Y 的一种」「X 依赖 Y」的词条，能抽出 `is-a`/`depends-on`  typed 边；同义实体判重合并。
- M3：dream_cycle 产出孤儿/陈旧/冲突提案，低风险自动应用、高风险进二审队列；词条反向蒸馏进 memory_fact。
- M4：密度越阈值的概念簇自动生成 MOC 词条；谱系链可追溯。

---

> 本评审稿待你确认后，按 M1 拆实施任务并 TDD 落地。
