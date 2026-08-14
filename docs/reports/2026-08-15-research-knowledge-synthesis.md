# 调研报告：知识库综合方案（论文 / 开源 / Obsidian → Aranea）

> 日期：2026-08-15 | 类型：research | 状态：对照仓库内既有调研 + 2026 文献刷新后落地写路径成链
> 对象：模块 37 Knowledge。实施对应 [37-knowledge.design.md §V12.11](../development/37-knowledge.design.md#v1211-编译期-wiki-成链2026-08-15)。
> 既有材料：`test/pkm-research/A-academic-frontier.md`、`B-oss-landscape.md`、`C-obsidian-graph-ui.md`、`D-siyuan-kernel.md`；`docs/reports/2026-08-08-research-pkm-obsidian-blueprint.md`。

---

## 1. 问题

Aranea 已有完整的 **Advanced RAG 底座**（混合检索、重写、CRAG）和 **PKM 外壳**（Vault、双链、3D 星河）。2026-08-15 上午落地了查询期 **Lazy GraphRAG** 与对话 **Retrieve-Then-Generate**。仍缺的关键供粮：图是稀疏的——用户很少手写 `[[wikilink]]`，`knowledge_links` 的 explicit 边不够，一跳扩展几乎无路可走。

本报告回答：论文与优秀软件如何让「笔记互链 / 检索 / Agent 使用」长在一起，以及 Aranea 应采用哪条可实施路径。

---

## 2. 论文侧（可迁移的机制，不是名词）

| 机制 | 代表 | 对 Aranea 的含义 |
|------|------|------------------|
| Compile-time wiki | Karpathy llm-wiki、Notemd：写时把提及变成链接，读时图已存在 | **最高杠杆**。不抽 NER 也能让 explicit 边变密 |
| Query-time graph RAG | Microsoft LazyGraphRAG；HippoRAG 2（ICML’25，双节点+PPR）；LightRAG（增量图便宜）；PathRAG（剪噪声路径） | 查询期走已有边，避免入库期全量抽实体。**一跳已落地**；PPR/社区摘要仍贵 |
| Temporal KG memory | Graphiti / Zep | 会话事实随时间失效；属记忆层，不是 Vault 笔记层 |
| Agentic vs grep | Claude Code 实践：代码库 **grep/glob 优于向量 RAG** | 知识库仍要向量；**精确意图**（路径/扩展名/引号）必须跳过图扩展（已做） |
| Write-back flywheel | Mem0、Collaborative Memory、SP7 G2 | 会话沉淀回团队库，需出处。**本轮已落地验证门切片** |
| Universal / Video RAG | UniversalRAG、Video-RAG | 多模态路由已有视觉提取；不是当前瓶颈 |

结论：不要上全量 GraphRAG 2.0 四模式。要用 **写时编译 + 查时一跳** 的组合，这是 HippoRAG/LazyGraphRAG 的工程近似，成本低两个数量级。

---

## 3. 开源软件（许可证：只学设计）

| 产品 | 内核设计 | 可学 | 不可做 |
|------|----------|------|--------|
| **Obsidian** | 文件=真相；MetadataCache 派生索引；未链接提及**只展示**；图谱是人用 UI | 保护代码/frontmatter；词边界；歧义跳过 | 核心闭源；未链接提及不写回文件导致图永远稀 |
| **社区插件** Link Unlinked Mentions / Link Plus | 把纯文本包成 `[[title]]`，确认后写回 | **本轮直接对标的写路径** | 不复制插件代码 |
| **SiYuan** | 文件=源，SQLite=可重建索引；引用整篇替换 | 派生索引可重建（我们已有 `RebuildBlockIndex`） | AGPL，禁止抄内核 |
| **Logseq** | 大纲块 + 双向引用 | 块级反链（SP1 已有） | AGPL |
| **AFFiNE** | CRDT 协作文档 | 团队库 eventual consistency 思路 | 协作层远重于当前 Vault |
| **Anything-LLM / Cognee** | 工作区 RAG + 可选知识图谱 | Agent 侧 retrieve-then-generate | 不要把它们的 ontology 管道整段搬来 |
| **Joplin** | 笔记+插件+同步 | 同步与冲突留双份（我们 CAS+trash 已有） | — |

Obsidian 的关键教训：**展示未链接提及 ≠ 图可用**。3D 星河好看，检索仍是扁平 chunk——这正是本仓库 2026-08-15 之前的状态。

---

## 4. 综合架构（四层，按杠杆排序）

```
L1  Write-time compile     未链接提及 → [[wikilink]]     ← 已实施
L2  Query-time Lazy graph  knowledge_links 一跳扩展       ← 已实施
L3  Agent retrieve-then-gen 首轮注入段落而非库名           ← 已实施
L4  Write-back flywheel    会话→团队库+出处               ← 本轮验证门切片
```

刻意不做（现在）：入库期 NER 实体表、社区摘要、frontmatter alias 扫描。历史回填 / 确认成链 / 金标 / G1 G7 G8 / pending 过门见 V12.13（2026-08-15 已落地）。

---

## 5. 本轮实施对照

| 方案决策 | 代码 |
|----------|------|
| 纯函数、TDD、无 IO | `internal/biz/knowledge/autolink.go` |
| 标题 = basename 去扩展（复用 `mentionNeedle`） | 与 P2-7 未链接提及同一针 |
| 摄取后、索引前成链 | `IngestDocument`；vault `.md` 落盘用成链后正文 |
| 编辑保存成链 | `UpdateVaultDocumentContent` → `ApplyOne` |
| 外部编辑器 / watcher 不成链 | 避免静默改用户文件 |
| 失败降级原文 | 不阻断入库 |

| 方案决策 | 代码 |
|----------|------|
| 写回验证门（kind ∩ ≥0.85 ∩ ≥8 字） | `FilterWriteBackFacts` |
| UTC 日切分日记 + provenance | `inbox/writeback-YYYY-MM-DD.md` |
| AutoMemory 接线、失败不阻断 | `maybeWriteBack` |
| chunk/FTS 重放 | `KnowledgeService.WriteBackSessionFacts` |

验收见需求 US-37。成链见 US-34；查询与 Agent 层仍走 V12.10。

---

## 6. 下一步（不在本轮）

已于同日收口：历史回填、检索金标、G1/G7/G8、写回确认过门（US-38~US-44）。仍不做：

1. 入库期 NER / PPR / 社区摘要 / GraphRAG 2.0 四模式。
2. SP5 知识成熟度 FSM、SP6 JITAI 伙伴、时间 KG、视频 RAG。
3. 50 条中英 BM25 对真实 Postgres 的回归（当前金标是合成语料 + GraphExpander）。
