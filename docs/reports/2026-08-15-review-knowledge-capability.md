# 评审报告：知识库深度评审 + 能力天花板调研（2026-08-15）

> 日期：2026-08-15 | 类型：review | 状态：全部结论对照代码行号，关键断言经两轮独立验证；**P0 三刀已实施并通过测试**
> 前序：[2026-08-15-review-knowledge-working-core.md](./2026-08-15-review-knowledge-working-core.md)（下称「前文」，其三刀结论本文逐条复核）
> 方法：主评审直读代码取证 → 交叉验证 agent 独立复核 10 项关键断言（10/10 成立）→ 业界 2026 生产标准调研对照
> 约束：所有建议不动 `pkg/trpc-agent-go`（FW-R1~R3），改动面全部在 `internal/`。
> **2026-08-15 修订（用户评审反馈，已采纳）**：①「检索达业界 80% 线」降调为「架构到 L1 上沿，默认部署质量未实测达标」（rerank 默认关、金标 12 条非召回基线）；② **P0 时态不落 chunk**——chunk 是派生切片、整页重插会冲掉时态且与日记正文形成两套真理，作废落点改为**词条正文 upsert**；③ 实体轨 P1 默认**冻结**而非接线；④ recency 用 `knowledge_documents.updated_at`，禁用 `chunks.created_at`（整页重插会把旧知识洗成"刚才"）；⑤ 引用闭环只覆盖工具检索路径，首轮预检索不发 notice。详见 §7 修订后路线图。

---

## 1. 先给结论

**前文的判断成立，且经代码复核后可再精确一档：现在是「检索链路高于业界平均、维护链路低于业界底线」的畸形体。**

- **检索侧架构到位、实测未达标**：混合检索（dense+BM25 RRF k=60）+ 复杂度自适应路由 + 可选 rerank + 一跳图扩展 + 五级降级链，**链路形状**是 2026 年业界主流形态；但生产默认 rerank 关闭（`KRATOS_KNOWLEDGE_RERANKER` 空=off，[reranker_factory.go](file:///f:/myproject/aranea-agents/internal/knowledge/reranker_factory.go#L17-L25)），金标仅 12 条查询、非召回评测基线，中文 BM25 部署侧命中未验证（代码侧 2026-08-10 有 word_similarity 根修，[knowledge.go](file:///f:/myproject/aranea-agents/internal/data/knowledge.go#L955-L968)，运行侧未回归确认）。准确口径：**架构到 L1 上沿，默认部署的检索质量没有被测到业界线上**。
- **维护侧是短板**：知识域（knowledge_chunks/documents）连「作废」的 schema 字段都没有——不是没实现，是**数据模型层面就不支持**。而同仓库的 memory_facts（AutoMemory L3）早就有时态字段（valid_to/superseded_by）。同一套系统，记忆有时态、知识没有，这是最刺眼的一处不对称。
- **关联侧是半成品**：entity/semantic 两种边的类型、三张实体治理表、归一化/别名 keeper 管线全部建好并通过测试，但**生产装配从未接线**（`SetEntityHook` 无调用者）。图上实际只有 explicit 一种边在跑。
- **业界天花板**：2026 年生产标准是「写 / 召回 / 遗忘」三职俱全 + 时态事实（valid_at/invalid_at）+ 混合检索打底、图扩展按需。对照这个标准，当前位置在 **L1（可检索文档库）顶部，L2（会自我维护的知识库）门口**。修订后的三刀：**词条优先写回 → 别名成链 → 替换语义走正文**（时态不落 chunk，详见 §7）。

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
3. **引用闭环已跑通，但只覆盖工具检索路径**：`knowledge_search`/`knowledge_reflect` 命中时发射 `knowledge_recalled` notice → citation backfill worker → `knowledge_chunks.cited_count` / `knowledge_chunk_citations`，即「召回后被引用率」可度量。[tool.go](file:///f:/myproject/aranea-agents/internal/tools/knowledge/tool.go#L246-L284)。**注意**：首轮 `## Retrieved Knowledge` 预检索注入不发 notice——日常供粮主路径目前不进 cited_count，闭环量的是工具调用，不是预检索。
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
| **L1 可检索文档库** | 混合检索（BM25+dense+RRF）、rerank、降级链、按复杂度路由 | 业界共识：80% 生产负载 hybrid+rerank 即够用，不必上图 | ⚠️ **架构达标、实测未达标**（当前位置）：rerank 默认关、无召回基线回归、中文 BM25 部署侧未验证 |
| **L2 会自我维护的知识库** | 事实可更新/可替换、同义合并去重、词条化写回（Agent 可写指定条目）、别名成链、时间衰减排序、引用闭环 | Zep/Graphiti 时态边（valid_at/invalid_at）是 2026 生产标准；Mem0/Letta 的写时操作 API；Hindsight 四杠杆（importance/merge/decay/eviction）。**注意时态的落点因真相源而异**：记忆系统的行=事实，时态落在行上；本库真相源是 Markdown 词条，更新语义应落在词条正文（upsert 改正文、索引重建自然生效），而非 chunk 派生切片 | ❌ **只做了 importance（过门）+ 引用闭环（限工具路径）**；merge/decay/eviction/正文级更新/写工具全缺 |
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
| 事实可更新/可替换 | 同一事实再写入时更新词条正文旧段；旧内容随索引重建自然失效 | ✅ 已实施（fact_id 整段替换；归一化陈述语义合并仍 P1） | — |
| Agent 主动写 | `knowledge_write` 类工具，写指定词条，高置信直写、低置信 HITL | ✅ 已实施（高置信走 P0-1 词条 upsert 直写，0.6~0.85 进 pending 队列 HITL，不设弹窗门禁） | — |
| 词条优先 | 高置信事实 upsert 进词条页，日记只当流水、默认不检索或降权 | ✅ 已实施（tags→词条 upsert，日记排除默认检索） | — |
| 合并去重 | 同义/同指事实合并（语义级）；跨日换措辞不重复入库 | fact_id/陈述子串，且限单篇日记 body 内 | 🟠 管线级 |
| 时间衰减 | 排序含 recency 因子，用 `documents.updated_at`（人改词条时间） | ✅ 已实施（dense/tsvector/trigram 三路 `分 × exp(-λ·age)`，λ≈6.774e-9 即 180 天 ×0.9；未用 `chunks.created_at`） | — |
| 别名成链 | aliases/title 与 basename 同等参与提及匹配 | ✅ 已实施（多键匹配，歧义跳过） | — |
| 实体边 | 入库期实体抽取 + 共现建边 | 管线建成未接线 | ⚪ P1 默认**冻结**（接线属 L3 成本，等 citation 失败分布够线再说） |
| 引用闭环 | 召回→引用→度量，覆盖预检索+工具两条路径 | ✅ 已实施（首轮预检索注入也发 `knowledge_recalled`，载荷与实际渲染 chunks 一致；同 chunk 同 turn 账本幂等） | — |
| 混合检索+降级 | hybrid+rerank+降级矩阵，默认部署实测达标 | 架构齐；rerank 默认关、无召回基线 | 🟠 部署/评测级 |
| 一跳图扩展 | 物化边查询期扩展 | ✅ 已接 Agent | — |

---

## 7. 路线图（2026-08-15 按用户评审修订：换顺序、换落点，全部不碰框架）

**P0 — 三刀（仍是三刀，顺序和落点已修订）——2026-08-15 已全部实施**

1. **词条优先** ✅ 已实施：过门事实按 tags 匹配词条页（`entries/<slug>.md`，首选 tag 命中 basename/title/aliases 则 upsert，未命中新建页并把其余同义 tag 落 aliases 防重开新页）；无 tags 回退当日日记。日记保留为 provenance 流水并带 `- entry: [[词条]]` 指针，且**默认检索排除**——`SearchQuery.ExcludePathPrefixes` + `pathExcludeClause`（dense/BM25 双路）按 `inbox/writeback-` 字面前缀排除，预检索注入与 knowledge_search/knowledge_reflect 三处全部生效。[writeback_entry.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback_entry.go)、[writeback.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback.go#L166-L196)、[knowledge.go pathExcludeClause](file:///f:/myproject/aranea-agents/internal/data/knowledge.go)、[knowledge_inject.go](file:///f:/myproject/aranea-agents/internal/agent/knowledge_inject.go)
2. **别名成链** ✅ 已实施：autolink 扩为 `AutolinkWikiMentionsMulti`（resolveIndex 接线时 basename+title+aliases 全键匹配、歧义键跳过、self 全键豁免，未接线降级 basename 单键）；mention 侧 `mentionNeedles` 同样多键合并计数。[autolink.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/autolink.go#L169-L259)、[mention.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/mention.go#L120-L156)
3. **替换语义走正文** ✅ 已实施：同一 fact_id 再写入时 `replaceH2BlockContaining` 整段替换词条旧段（不追加、不重复），陈述已在则跳过；chunk 列未动。[writeback_entry.go upsertEntryDoc](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback_entry.go#L200-L280)

> 验证：新增 11 个单测全部通过（词条新建/别名命中/fact_id 替换/无 tag 回退/多键成链/歧义跳过/排除路径 dense+BM25 SQL），`internal/biz/knowledge`、`internal/data`、`internal/cronrunner`、`internal/agent`、`internal/tools/knowledge` 全量回归绿。归一化陈述（换措辞的同一事实）的语义级合并仍是 P1 管线活，不在本刀。

> **为什么时态不落 chunk（关键架构判断）**：记忆系统（memory_facts）的行就是事实，时态落在行上成立；知识域的真相源是 Markdown，chunks 是派生切片——在 chunk 上加 valid_to/superseded_by 会出现「日记正文还写着旧句、检索却把 chunk 藏起来」的两套真理，且文档一保存 chunks 整页重插、chunk 级时态即被冲掉。作废落点必须在词条正文：旧句从正文消失，索引重建后自然不再被搜到。若未来确需「同页旧段可见、检索默认跳过」，用块锚/薄 overlay 表指向 doc_id+block，**不污染 knowledge_chunks 的 8 列派生模型**。禁止先做 chunk 级 bitemporal 大迁移（先迁 schema 再改写路径，容易做出一套从不触发的列）。

**P1 — L2 补完——2026-08-15 已全部实施**

4. **`knowledge_write` 工具** ✅ 已实施：Agent 主动写指定词条，高置信（≥0.85，显式写默认 0.95）走 P0-1 同一词条 upsert 直写；0.6~0.85 进 `inbox/writeback-pending.md` 既有 HITL 审核链（不设弹窗门禁）；<0.6 拒绝。身份安全：user/session 从 trpc invocation 解析，LLM 不能指定写到谁名下；fact_id 留空按归一化陈述派生（幂等重放同键）。注册走惯例四件套：biz key `ToolKeyKnowledgeWrite` → `builtin_tools_seed.go` seed（integration/medium/enabled）→ `sessionBoundToolKeys` → `tool_assembly.go` 按 effective tools 注入 CustomTools。[write_tool.go](file:///f:/myproject/aranea-agents/internal/tools/knowledge/write_tool.go)、[tool_assembly.go](file:///f:/myproject/aranea-agents/internal/agent/tool_assembly.go#L82-L89)、`WriteBackResult.Landed`/`EntryOf` 回执落点词条名
5. **检索轻 recency** ✅ 已实施：dense/tsvector/trigram 三路排序包成 `分 × exp(-λ·age)`，`JOIN knowledge_documents` 取 `updated_at`（人改词条/写回正文才拨动）；λ = -ln(0.9)/(180×86400) ≈ 6.774e-9（180 天 ≈ ×0.9 月尺度微调，5 年 ≈ ×0.34），GREATEST 防时钟回拨负 age。未用 `chunks.created_at`。[knowledge.go recencyScoreSQL](file:///f:/myproject/aranea-agents/internal/data/knowledge.go)
6. **预检索 notice** ✅ 已实施：`EmitKnowledgeRecalledNotice` 公开复用同一 notice 载荷，首轮预检索注入（knowledge_inject）检索到 chunks 后发射，`cueRenderedChunks` 保证载荷与实际渲染内容（非空正文 + TopK 截断）一致；lastUserQuery 只在每轮用户消息首次模型调用取查询，工具循环续跑天然不重复发。[knowledge_inject.go](file:///f:/myproject/aranea-agents/internal/agent/knowledge_inject.go)、[tool.go](file:///f:/myproject/aranea-agents/internal/tools/knowledge/tool.go)
7. **实体轨冻结** ✅ 已实施：GraphExpander `linkTypePriority` 注释写死「entity/semantic 边生产无写入路径（SetEntityHook 全仓无生产调用者、semantic 无建边管线），knowledge_links 只有 explicit 一种边，entity×2/semantic×1 分支当前是死代码，保留仅为兼容未来接线」；解冻判据 = citation 失败分布证明多跳失败 ≥30% 再议。[graph_expander.go](file:///f:/myproject/aranea-agents/internal/knowledge/graph_expander.go)

> P1 验证：新增 5 个写工具单测（nil/unavailable 跳过注册、派生 fact_id 归一化幂等、6 类验证拒绝、低置信进 pending、高置信直写词条+同 fact_id 幂等再写）+ recency 集成测试（3 年旧文 dense/tsvector/trigram 三路衰减排序与比值断言），`internal/biz/...`、`internal/tools/...`、`internal/agent/...`、`internal/data` 全量回归绿（data 层需 `ARANEA_TEST_PG_DSN` 指宿主 5432）。语义级合并去重（差距矩阵 🟠 行）仍是管线活，不在本批。

> **端到端验证揪出并修复「写入成功但检索不可见」事故（2026-08-15 当日闭环）**：Web UI（spirit）调用 knowledge_write 写入「8810 端口」事实，工具回执/词条正文/确认门禁/审计全正常，但词条页 `status=pending`、`chunk_count=0`——**写入成功 ≠ 可检索**。根因三层：
> 1. **词条页不在重放范围**：P0 词条 upsert 的文档不在 `WriteBackResult` 里，service 包装只对日记 DocID 重放 chunks → 修复：`WriteBackResult.EntryDocs` 收集词条 touched docs（`upsertEntryDoc` 返回 changed 标记，内容未变的 fact_id 幂等重写不算改动，避免无效 embedding 重放）。
> 2. **重放挂错层**：chunk 重放原在 service 层 `KnowledgeService.WriteBackSessionFacts` 包装内，而 `knowledge_write` 工具直调 biz `Usecase.WriteBackSessionFacts`（auto_memory 经 wire 消费 service 包装所以未绕过）→ 修复：重放改为 biz 层 `SetWriteBackReplay` 钩子（KnowledgeService 构造时注入，同 SetBlockIndexRepos 装配模式），工具直调/cron/pending 审核应用三路共用；service 包装变薄透传。
> 3. **`ReembedDocuments` 误拒词法库**：team 库是纯词法库（embedding_model 空，83 chunks 全部无向量），但 API 以「无语义层」拒绝——而 tsvector/trigram 检索同样依赖 chunks，词法库 chunks 缺失时唯一自愈路径被堵死 → 修复：放宽为词法库受理纯分块/FTS 重建（embedder=nil 不产向量），既有 `TestReembedDocuments_LexicalCollectionRejected` 反转为 Accepted。
>
> 处置：存量 12 篇（5 词条 + inbox/agents 若干）经放宽后的 reembed 全量 indexed；端到端复验写入「9001 端口」事实，词条 chunks 与正文同窗重建（时间差 5ms）且可检索命中。新增 5 个 biz 测试（touched 新建/既有/无变化、hook 收日记+词条、幂等跳过）+ reembed 词法用例，`internal/biz/...`、`internal/service/...`、`internal/agent/...`、`internal/tools/...`、`internal/cronrunner/...` 全量回归绿。[writeback.go hook](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback.go)、[knowledge_writeback.go](file:///f:/myproject/aranea-agents/internal/service/knowledge_writeback.go)、[knowledge_reembed.go](file:///f:/myproject/aranea-agents/internal/service/knowledge_reembed.go)
>
> **教训（防再犯）**：写路径的新派生副作用必须挂在所有生产调用者都经过的层——判断依据是 grep 调用点而非「包装方法存在」；service 包装有重逻辑而工具直调 biz 是本仓既有形态，未来给写回链路加能力先查 `write_tool.go`/`auto_memory.go`/`wire_gen.go` 三处消费方。

**P2 — L3 按需（先看检索失败分布再决定）**

8. 多跳扩展 / 矛盾检测 / 社区摘要：按业界「多跳失败 ≥30% 才建图」的判据，用补全后的 citation 闭环（含预检索）度量失败分布，够线再建。

**不建议现在做**（2026-08-15 修订新增）：chunk 级 bitemporal 大迁移；生产接线 NER；把 rerank 说成已达标；用 `knowledge_write` 替代词条 upsert（没有词条页，写工具只会把日记写得更碎）。

**明确不做**：插件、市场、主题、移动端、Canvas（沿用前文口径）；框架层（pkg/trpc-agent-go）任何改动。

---

## 8. 证据索引

- 评审复核验证清单：10/10 成立（交叉验证 agent 独立复核，行号见第 3 节链接）
- 业界参照：Zep/Graphiti 时态边、Mem0/Letta/MemGPT 分层记忆与写时操作、Hindsight consolidation 四杠杆（2026-05）、FalkorDB GraphRAG SDK 1.0（自动作废旧事实仍属 roadmap）、GraphRAG-Bench「多跳 ≥30% 才建图」判据、「hybrid+rerank 覆盖 80% 生产负载」共识
- 对照组：同仓库 memory_facts 时态实现（[memory_chain.sql](file:///f:/myproject/aranea-agents/internal/data/sql/memory_chain.sql#L316)、memory_shim_l3.go InvalidateFact/SupersedeFact）
