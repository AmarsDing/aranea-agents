# 评审报告：知识库深度评审 + 能力天花板调研（2026-08-15）

> 日期：2026-08-15 | 类型：review | 状态：全部结论对照代码行号，关键断言经两轮独立验证
> 前序：[2026-08-15-review-knowledge-working-core.md](./2026-08-15-review-knowledge-working-core.md)（下称「前文」，其三刀结论本文逐条复核）
> 方法：主评审直读代码取证 → 交叉验证 agent 独立复核 10 项关键断言（10/10 成立）→ 业界 2026 生产标准调研对照
> 约束：所有建议不动 `pkg/trpc-agent-go`（FW-R1~R3），改动面全部在 `internal/`。

---

## 1. 先给结论

**前文的判断成立，且经代码复核后可再精确一档：现在是「检索链路高于业界平均、维护链路低于业界底线」的畸形体。**

- **检索侧是强项**：混合检索（dense+BM25 RRF k=60）+ 复杂度自适应路由 + 可选 rerank + 一跳图扩展 + 五级降级链，已达到 2026 年业界「80% 生产负载够用」的标准线。
- **维护侧是短板**：知识域（knowledge_chunks/documents）连「作废」的 schema 字段都没有——不是没实现，是**数据模型层面就不支持**。而同仓库的 memory_facts（AutoMemory L3）早就有时态字段（valid_to/superseded_by）。同一套系统，记忆有时态、知识没有，这是最刺眼的一处不对称。
- **关联侧是半成品**：entity/semantic 两种边的类型、三张实体治理表、归一化/别名 keeper 管线全部建好并通过测试，但**生产装配从未接线**（`SetEntityHook` 无调用者）。图上实际只有 explicit 一种边在跑。
- **业界天花板**：2026 年生产标准是「写 / 召回 / 遗忘」三职俱全 + 时态事实（valid_at/invalid_at）+ 混合检索打底、图扩展按需。对照这个标准，当前位置在 **L1（可检索文档库）顶部，L2（会自我维护的知识库）门口**，差的就是前文说的三刀——复核后这三刀依然成立，只有优先级微调。

---

## 2. 现状数据流（已核实）

```mermaid
flowchart LR
    subgraph 写入侧
        A[人: 工作台编辑/粘贴/vault 同步] -->|UpdateDocumentContent + CAS| D[(knowledge_documents/chunks)]
        B[AutoMemory 过门事实 ≥0.85] -->|只追加 inbox/writeback-YYYY-MM-DD.md| D
        B2[0.60–0.84 待确认] -->|inbox/writeback-pending.md 人勾后追加| D
        C[L3 活动事实] -->|G1 覆盖投影 agents/id.md| D
    end
    subgraph 派生索引
        D -->|RebuildBlockIndex| E[knowledge_blocks/refs]
        D -->|autolink 编译 [[wikilink]]| F[(knowledge_links: 仅 explicit)]
        E -->|projectExplicitLinks| F
        G[knowledge_entities 三表] -. 生产零写入 .-> F
    end
    subgraph 检索侧
        Q[用户问题] --> R{AdaptiveRouter 复杂度}
        R -->|简单| S[dense 向量]
        R -->|中/复杂| T[RRF: dense+BM25]
        R -->|复杂| U[MultiQuery 改写]
        S & T & U --> V[rerank 可选]
        V --> W[GraphExpander 一跳: 种子5/邻居8/各2块/800ms]
        W --> X[## Retrieved Knowledge 注入<br/>top-4 × 280字 × 2s 仅首轮]
    end
    style B fill:#fff3e0,color:#e65100
    style B2 fill:#fff3e0,color:#e65100
    style G fill:#ffcdd2,color:#b71c1c
    style T fill:#c8e6c9,color:#1a5e20
    style W fill:#bbdefb,color:#0d47a1
```

橙 = 只增不改的写回（长年累库的风险点）；红 = 建好未接线的半成品；绿/蓝 = 已达业界标准线的检索主链。

---

## 3. 前文复核：10 项断言全部成立，2 处需修正表述

| # | 前文断言 | 复核 | 证据 |
|---|----------|------|------|
| 1 | 预检索 top-4、每段 280 字、2 秒超时、写进 `## Retrieved Knowledge` | ✅ 成立 | [knowledge_inject.go](file:///f:/myproject/aranea-agents/internal/agent/knowledge_inject.go#L18-L24)（常量）, [L152-L153](file:///f:/myproject/aranea-agents/internal/agent/knowledge_inject.go#L152-L153)（注入段头） |
| 2 | 无 `knowledge_write`，只有 search/reflect | ✅ 成立 | [tool.go](file:///f:/myproject/aranea-agents/internal/tools/knowledge/tool.go#L182-L187)（search）, [L393-L397](file:///f:/myproject/aranea-agents/internal/tools/knowledge/tool.go#L393-L397)（reflect）；全仓无 write 工具声明 |
| 3 | 写回按日追加日记，白名单 kind ∩ ≥0.85 ∩ ≥8 字 | ✅ 成立 | [writeback.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback.go#L20-L26)（门常量）, [L66-L73](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback.go#L66-L73)（kind 白名单）, [L101-L103](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback.go#L101-L103)（UTC 日记路径） |
| 4 | 0.60–0.84 进 pending，人勾后再写 | ✅ 成立 | [writeback_pending.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback_pending.go#L13-L19), [L256-L331](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback_pending.go#L256-L331)（ApplyPendingWriteBack） |
| 5 | 去重键太窄（fact_id/陈述子串，无语义合并） | ✅ 成立 | [writeback.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback.go#L137-L145)（仅两种子串匹配，无归一化、无向量） |
| 6 | 图扩展一跳：种子5/邻居8/explicit×3>entity×2>semantic×1/各2块/800ms | ✅ 成立 | [graph_expander.go](file:///f:/myproject/aranea-agents/internal/knowledge/graph_expander.go#L17-L23), [L203-L214](file:///f:/myproject/aranea-agents/internal/knowledge/graph_expander.go#L203-L214) |
| 7 | 成链只认文件名 basename，aliases 有口子没接上 | ✅ 成立，且更精确：title/aliases 已物化到列，但 autolink/mention 不消费 | [mention.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/mention.go#L110-L124)（needle=basename）, [autolink.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/autolink.go#L142-L155)；title/aliases 仅 [link_resolver.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/link_resolver.go) 跨库解析消费 |
| 8 | 文档级更新齐（CAS/watcher/重嵌入/块索引） | ✅ 成立 | 前文已述，复核无反例；chunks 随文档 ON DELETE CASCADE（[knowledge.go](file:///f:/myproject/aranea-agents/internal/data/knowledge.go#L100-L109)） |
| 9 | 知识级更新是空的：无 valid_to/superseded，检索不过滤 | ✅ 成立，且是 schema 级缺失 | chunks 表全文段：[knowledge.go](file:///f:/myproject/aranea-agents/internal/data/knowledge.go#L100-L109)（仅 8 列，唯一后补列是 cited_count L156）；全 migrations 无 knowledge 时态字段。对照组：memory_facts 有 valid_to/superseded_by（[memory_chain.sql](file:///f:/myproject/aranea-agents/internal/data/sql/memory_chain.sql#L316)） |
| 10 | G1 投影覆盖写 agents/{id}.md | ✅ 成立 | [memory_project.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/memory_project.go#L14)（注释明示覆盖写）, [L65-L66](file:///f:/myproject/aranea-agents/internal/biz/knowledge/memory_project.go#L65-L66)，上限 80 条事实 |

**修正 1（前文说得偏弱）**：「晋升是人把整篇文档推到团队库」不准确。[promote.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/promote.go#L93-L169) 实际是**块级复制**：任意 blockIDs → 团队库同名 rel_path 文档 find-or-create + 尾部追加 + 谱系对回写（promoted_from/promoted_to）+ 私有引用 cascade 提示。比「整篇推库」细——但目标定位仍是「同名 rel_path」，**不能指定合并进 `值班制度.md` 这类词条**，前文的核心判断（日记事实晋升不成正文）不受影响。

**修正 2（前文说得偏轻）**：「入库期 NER 没做，entity 边不会自己变密」——实际是**生产零写入**。`ReplaceEntityLinks`（[link.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/link.go#L101-L106)）与 `Usecase.ReplaceDocEntities`（[link.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/link.go#L118-L123)）全仓无生产调用者（仅测试 mock）；entity 轨唯一触发器 `SetEntityHook`（vault_sync.go:49）从未被装配调用，hook 恒 nil。semantic 边同样只有类型常量和读侧优先级，无写入路径。即 `knowledge_links` 里**只有 explicit 一种边**（[block_pipeline.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/block_pipeline.go) projectExplicitLinks 硬编码 LinkTypeExplicit）。GraphExpander 的 entity×2 / semantic×1 权重分支在当前生产数据上是死代码。

**补充（前文未提的细微点）**：知识库目录清单（Available Knowledge Bases + 检索策略提示）**每次** before-model 调用都注入，只有 chunks 预检索是首轮限定（[knowledge_inject.go](file:///f:/myproject/aranea-agents/internal/agent/knowledge_inject.go#L83-L97) lastUserQuery 工具循环跳过检索但目录照注）。这不是 bug，是 TTFT 取舍，但意味着多轮对话中后续轮次的「新话题」拿不到首轮预检索，只能靠模型主动调工具。

---

## 4. 前文未覆盖的新发现

### 4.1 好的（超过前文描述的家底）

1. **检索降级链是完整的五级矩阵**：embedder 未配 → BM25；collection 无语义层（前置判定，省一次 embed）→ BM25；embed 调用失败 → BM25；RRF 中 dense 失败 → sparse、sparse 失败 → dense；rerank 失败 → 原向量序。每层都有 step_id 日志。[retriever.go](file:///f:/myproject/aranea-agents/internal/knowledge/retriever.go#L93-L108), [hybrid_retriever.go](file:///f:/myproject/aranea-agents/internal/knowledge/hybrid_retriever.go#L80-L129)
2. **AdaptiveRouter 按查询复杂度选路**：简单→dense，中/复杂→RRF，复杂→自动 MultiQuery 改写（失败降级原查询），即时定位查询（路径/扩展名/引号短语）跳过图扩展防污染。[adaptive_router.go](file:///f:/myproject/aranea-agents/internal/knowledge/adaptive_router.go#L92-L102), [graph_expander.go](file:///f:/myproject/aranea-agents/internal/knowledge/graph_expander.go#L62-L64)
3. **引用闭环已跑通**：每次检索发射 `knowledge_recalled` notice → citation backfill worker → `knowledge_chunks.cited_count` / `knowledge_chunk_citations`，即「召回后被引用率」可度量。[tool.go](file:///f:/myproject/aranea-agents/internal/tools/knowledge/tool.go#L246-L284)
4. **健康度与专家定位**：health.go 提供文档/边/孤儿率/链接密度/dangling/写回日记数快照；ParseWriteBackExperts 从 provenance 聚合「哪个 agent/人在哪类事实上贡献多」。[health.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/health.go)
5. **金标测试证明一跳有真实价值**：6 文档 12 查询，断言「纯混合种子不含答案、图扩展后 miss=0」——一跳扩展确实能把种子拿不到的邻文档事实捞回来（测试特意要求种子不含答案，防金标污染）。[gold_recall_test.go](file:///f:/myproject/aranea-agents/internal/knowledge/gold_recall_test.go)
6. **实体治理管线质量不低**（只是没接线）：name_norm 归一化唯一约束、别名命中 keeper 合并、同批撞车 mentions 求和、孤儿实体事务内清理、R-3 高频实体噪声过滤。[knowledge_links.go](file:///f:/myproject/aranea-agents/internal/data/knowledge_links.go#L107-L165)

### 4.2 坏的（前文没点到名的）

1. **schema 级不支持作废**：这不是「补个过滤」能解决的，要动迁移（chunks 或块级加 valid_from/valid_to/superseded_by）+ 写入路径 + 检索过滤三处联动。是路线图里最大的一笔。
2. **检索排序无任何时间因子**：`SearchChunks` 仅 `ORDER BY embedding <=> vec`，BM25/trigram 仅 `ORDER BY score DESC`。旧日记与新日记完全平权——库越大越脏，这是前文「累计越久检索越脏」的机理根因。
3. **entity/semantic 是僵尸资产**：三表 + 治理管线 + 读侧权重都占了维护成本，生产却不产数据。要么接线（vault_sync SetEntityHook + 语义近邻建边），要么明确冻结，不应长期挂着。
4. **写回日记也去不了重**：去重只在「同一篇日记 body」内做子串匹配；跨日换措辞的同一约束会重复入库，且无任何合并机制。

---

## 5. 业界调研：知识库应该能做到哪一步

把「Agent 知识底座」的能力分五级。每级标注业界代表与我们的位置。

| 级 | 能力定义 | 业界 2026 参照 | 我们 |
|----|----------|----------------|------|
| **L0 文件柜** | 入库、切分、词法检索 | 任何向量库教程 | ✅ 超 |
| **L1 可检索文档库** | 混合检索（BM25+dense+RRF）、rerank、降级链、按复杂度路由 | 业界共识：80% 生产负载 hybrid+rerank 即够用，不必上图 | ✅ **达标**（当前位置） |
| **L2 会自我维护的知识库** | 时态事实（valid/superseded）、同义合并去重、词条化写回（Agent 可写指定条目）、别名成链、时间衰减排序、引用闭环 | Zep/Graphiti 时态边（valid_at/invalid_at）是 2026 生产标准；Mem0/Letta 的写时操作 API；Hindsight 四杠杆（importance/merge/decay/eviction） | ❌ **只做了 importance（过门）+ 引用闭环**；merge/decay/eviction/时态/写工具全缺 |
| **L3 会关联推理的知识库** | 实体解析、多跳图遍历、矛盾检测、社区摘要（全局问答） | GraphRAG 共识：向量+图混合，图只在「多跳失败 ≥30%」时值得建；实体解析是图质量前提 | ⚠️ **一半**：一跳扩展已上 Agent 路径；实体管线建成未接线；多跳/矛盾检测/社区摘要未做 |
| **L4 自进化知识底座** | sleep-time consolidation（闲时整理记忆）、衰减/驱逐策略、检索评测基线常态化回归 | Letta sleep-time compute；Hindsight consolidation；Mem0/Letta 的 forget 职 | ❌ 未做（评测器有，基线回归未常态化） |

业界调研的三个关键判断（决定路线图取舍）：

1. **「遗忘」是和「写、召回」并列的第三职**。2026 年生产系统（Mem0/Letta/Zep/Hindsight）的共识是：记忆系统必须显式规定什么过期、什么合并、什么驱逐，否则六个月后就敢拿旧政策回答新客户。我们的知识库目前只有「写」和「召回」两职。
2. **时态事实是底线不是前沿**。Zep/Graphiti 的 valid_at/invalid_at 已是生产标配；连 FalkorDB GraphRAG SDK 1.0 都把「自动移除过时事实」列为 roadmap（即业界也还没完全做好自动作废）——但人家的 schema 有时态字段，我们连字段都没有。**先补字段、再补过滤、最后才谈自动作废**，是符合业界成熟度的合理次序。
3. **图不必贪多**。业界基准显示 hybrid BM25+dense+rerank 在 80% 负载上与图方案质量相当；图只在多跳关系型查询占比高时回本。我们的一跳 Lazy GraphRAG 路线（索引期零成本、查询期物化边扩展）方向正确且成本得当——**先把 explicit 边的密度喂饱（别名成链），比上多跳更值钱**。

---

## 6. 差距矩阵（现状 vs L2 达标线）

| 能力 | L2 要求 | 现状 | 差距 |
|------|---------|------|------|
| 时态事实 | 事实可作废、可替换，检索默认过滤失效 | schema 无字段 | 🔴 迁移级 |
| Agent 主动写 | `knowledge_write` 类工具，写指定词条，带门禁 | 无写工具，AutoMemory 副作用写日记 | 🔴 工具+管线 |
| 词条优先 | 高置信事实 upsert 进词条页，日记只当流水 | 只追加日记 | 🔴 写回路径 |
| 合并去重 | 同义/同指事实合并（语义级） | fact_id/陈述子串 | 🟠 管线级 |
| 时间衰减 | 排序含 recency 因子 | 无 | 🟠 检索级 |
| 别名成链 | aliases/title 与 basename 同等参与提及匹配 | 只 basename（列已物化） | 🟢 小改 |
| 实体边 | 入库期实体抽取 + 共现建边 | 管线建成未接线 | 🟢 接线 |
| 引用闭环 | 召回→引用→度量 | ✅ 已跑通 | — |
| 混合检索+降级 | hybrid+rerank+降级矩阵 | ✅ 已达业界线 | — |
| 一跳图扩展 | 物化边查询期扩展 | ✅ 已接 Agent | — |

---

## 7. 路线图（复核前文三刀 + 分期，全部不碰框架）

**P0 — 前文三刀（复核后依然成立，顺序微调为：先 schema 后路径）**

1. **事实可替换（先动 schema）**。给知识域补时态字段（块级或 chunk 级 valid_from/valid_to/superseded_by）+ 迁移；同 `(kind, 归一化陈述或 fact_id)` 再写入时旧块置 superseded；检索 WHERE 默认过滤。memory_facts 的 bitemporal 实现（memory_shim_l3.go 的 InvalidateFact/SupersedeFact）就在同仓库，可直接对齐语义。没有这一刀，更新能力永远停在「改文件」。
2. **词条优先于日记**。高置信写回默认 upsert 到按 kind/标题匹配的既有词条页（匹配键含文件名 + 已物化的 title/aliases），日记降级为纯 provenance 流水。没有这一刀，累计越久检索越脏。
3. **别名参与成链**。autolink/mention 的 needle 从「basename 单键」扩为「basename + title + aliases 多键」（列和物化已就绪，只改匹配侧）。没有这一刀，关联网长不出来。

**P1 — L2 补完（三刀落地后立刻跟）**

4. `knowledge_write` 工具：Agent 主动写指定词条，走 HITL 确认门禁（复用现有 requires_confirmation 机制），写路径复用第 2 刀的 upsert。
5. 检索排序加 recency 因子（chunks.created_at 已在表里，纯排序层改动，无需迁移）。
6. 实体轨接线或冻结：给 vault_sync 装 SetEntityHook（LLM 抽实体 → ReplaceDocEntities → 共现建 entity 边）；若判断成本不值，则从 GraphExpander 权重注释中明示冻结，消除僵尸资产。

**P2 — L3 按需（先看检索失败分布再决定）**

7. 多跳扩展 / 矛盾检测 / 社区摘要：按业界「多跳失败 ≥30% 才建图」的判据，先用 citation 闭环的召回数据度量失败分布，够线再建。

**明确不做**：插件、市场、主题、移动端、Canvas（沿用前文口径）；框架层（pkg/trpc-agent-go）任何改动。

---

## 8. 证据索引

- 评审复核验证清单：10/10 成立（交叉验证 agent 独立复核，行号见第 3 节链接）
- 业界参照：Zep/Graphiti 时态边、Mem0/Letta/MemGPT 分层记忆与写时操作、Hindsight consolidation 四杠杆（2026-05）、FalkorDB GraphRAG SDK 1.0（自动作废旧事实仍属 roadmap）、GraphRAG-Bench「多跳 ≥30% 才建图」判据、「hybrid+rerank 覆盖 80% 生产负载」共识
- 对照组：同仓库 memory_facts 时态实现（[memory_chain.sql](file:///f:/myproject/aranea-agents/internal/data/sql/memory_chain.sql#L316)、memory_shim_l3.go InvalidateFact/SupersedeFact）
