# 知识库重设计方案评估：Vault 模式（本地路径即知识库）

> 类型：调研 + 方案评估（v2 终版）。本文是 M13 Knowledge 模块重设计的方案论证。
> 配套文档：[无 embedding 检索调研](./2026-07-25-research-embedding-free-retrieval.md) ｜ [评审报告（R-1~R-6）](./2026-07-25-review-knowledge-vault-redesign.md)
> v2 修订：§4 检索方案改为「三层检索 + embedding 可选化」；§7 增加实现契约 R-1~R-6；§9 Phase 调整（P4 拆分 P4a/P4b）。
> 用户确认后，结论同步至 `docs/development/37-knowledge.md`（需求）与 `37-knowledge.design.md`（设计）。
> 调研输入：Obsidian/Logseq/Notion、Dify/FastGPT/RAGFlow、Microsoft GraphRAG、LlamaIndex、MemGPT/Letta、AnythingLLM、Zotero/Calibre、PageIndex、Claude Code；pgvector/HNSW 与 BEIR 性能基准；Everything/Listary/Obsidian 搜索 UX。

---

## 1. 背景与现状评估

### 1.1 现状（Collection 模型的五个根本问题）

| # | 问题 | 后果 |
|---|------|------|
| P1 | 文档不落盘，仅 content_text 存 Postgres | 用户无法用任何外部工具（编辑器/Git/同步盘）接触自己的知识；知识被锁死在系统内 |
| P2 | Collection 是扁平列表，无层级 | 无法像资源管理器一样按文件夹组织；文档一多即失控 |
| P3 | 文档之间零关联（GraphRAG 停留在待实现设计） | 无法回答「这篇文档讲了什么、和什么相关」；agent 只能盲目向量检索 |
| P4 | 上传入口是 base64 API + 拖拽区 | 用户已有的成体系本地文档（如 Obsidian vault、文档目录）无法直接利用 |
| P5 | **中文全文检索实际失效**（评审核验发现） | `ts_rank(to_tsvector('simple',...))` 无 IDF/TF 饱和且 PG simple 配置对中文不分词（连续中文算单 token），BM25 通道对中文近乎不可用，模块被迫重度依赖向量通道 |

### 1.2 重设计目标（用户确认的四项决策 + 一项修订）

1. **双向混合（Obsidian 式）**：给定本地路径，KB 写入与用户放文件双向皆可；文件系统即真相源，DB/pgvector 为可重建派生索引
2. **多 Vault，每库一路径**：Collection 升级为 Vault（库 = 本地目录），保留库级检索隔离
3. **显式双链 + 自动抽取双轨关联**：`[[]]` 双链（人/agent 可写，精确）+ LLM 实体/语义近邻（自动）
4. **全链路 + agent 自编辑**：含 watcher、迁移、`knowledge_navigate` + `knowledge_write` 工具
5. **embedding 可选化（v2 修订）**：检索以无向量方案为主体（强 BM25 + 推理导航），语义层降级为可插拔插件

---

## 2. 著名知识库设计调研结论

### 2.1 设计模式提炼（9 条）

| # | 模式 | 来源 | 本方案采用方式 |
|---|------|------|---------------|
| M1 | 单一真相源：文件系统为真相，DB 为可重建派生索引 | Obsidian | Vault 核心原则 |
| M2 | 元数据卡：每文档 frontmatter（类型/标签/摘要/来源），人可读、机可查、agent 可写 | Obsidian/Dataview | frontmatter 摘要卡 |
| M3 | 文档摘要卡 + 分层摘要：agent 不读全文即知大致内容 | GraphRAG/RAPTOR/PageIndex | LLM 预生成 summary 写入 frontmatter + DB |
| M4 | 关联双轨制：显式链接精确可信 + 隐式关系自动含噪，叠加时显式优先 | Obsidian + GraphRAG | links 表 link_type=explicit/entity/semantic |
| M5 | 粒度 = 检索单元设计：文件级真相源，DB 承载 chunk/关联细粒度派生数据 | Dify/LlamaIndex + Logseq 教训 | markdown 标题分块天然父子（二期增强） |
| M6 | 库即隔离边界：检索最小单元是库，文件夹仅组织视图 | FastGPT/AnythingLLM | Vault = 检索边界（现有 scoped 语义不变） |
| M7 | Watcher 安全默认：只增不删、删除入回收站、冲突留双份 | Zotero/Calibre | SyncEngine + .aranea/trash/ |
| M8 | 知识库工具化 + 推理导航：tree/card/read/grep/write | MemGPT/Letta + PageIndex + Claude Code | knowledge_navigate + knowledge_grep + knowledge_write |
| M9 | 无向量检索为主体：词法/结构/图做精确层，向量做语义兜底 | BEIR-NL(2024) + Claude Code + PageIndex | 三层检索（§4.5） |

### 2.2 关键反面教训

- **Logseq 的教训**：块级粒度最终迫使它从纯文本迁向 SQLite——本方案保持**文件级**真相源 + DB 承载细粒度派生数据，避开此坑
- **Notion 是反例**：DB 即真相源 + 私有格式 = 知识锁定。但其 database relation 思想被 links 表借鉴
- **AnythingLLM 的教训**：导入即复制、不监视原始路径——用户改原文件后知识库静默过期。本方案 watcher 双向同步解决
- **生成式检索（DSI）的教训**：参数化索引无法增量更新、无法区分相关与随机文档，2025 年业界共识让位于「外部索引 + LLM 导航」

---

## 3. 总体方案：Vault 模型

### 3.1 概念模型

```
Vault（知识库 = Collection 升级）
  └── root_path: 本地目录（如 D:\knowledge\company）
        ├── 文件夹树（用户自由组织 ←→ 资源管理器呈现）
        │     ├── 财报/2026Q2.md
        │     └── 制度/预算制度.md
        └── .aranea/           # 库元数据目录（类比 .obsidian/）
              ├── vault.json   # 库配置（嵌入模型/dim/忽略规则）
              └── trash/       # 回收站（watcher 删除安全网）
```

文档 = Markdown + YAML frontmatter 摘要卡：

```markdown
---
id: doc_01J8...
title: 2026 Q2 财报分析
tags: [财报, 季度]            # ← 受管字段（KB 独占）
type: report                  # ← 受管字段
summary: 本文分析 2026 Q2 营收结构……   # ← 受管字段
summary_hash: sha1:ab12...    # ← 受管字段（过期检测）
source: 财报Q2.pdf
created: 2026-07-25T10:00:00Z
---

# 2026 Q2 财报分析
正文…… 相关：[[预算制度]]   # ← 显式双链（用户/agent 可写）
```

### 3.2 架构与数据流

```
              ┌──── 文件系统（真相源）────┐
              │ vault/**/*.md + .aranea/ │
              └──▲───────────────┬──────┘
   KB 写入(上传/整理/agent_write)      SyncEngine 扫描(mtime+sha1)
              │                       ▼
        VaultFiler（唯一写出口）   变更队列(增/改/删→trash，自写打标防回环)
              │                       │
              └──────► IngestPipeline ◄┘
         Extract → Organize(MD) → Summarize(摘要卡) → ExtractEntities(关联)
                          │
                          ▼
        派生索引（全部可无状态重建）：
          Postgres: documents 镜像 + chunks/embeddings(可选) + links + entities
          检索索引: 强 BM25 倒排（FTS5/Bleve/自研，选型见 R-5）
                          │
        ┌─────────────────┼──────────────────┐
        ▼                 ▼                  ▼
   Retriever 族(保留)  knowledge_navigate   图谱/关联 API
```

**保留复用**：Extractor / MarkdownOrganizer / Chunker / Retriever / HybridRetriever / AdaptiveRouter / FederatedRetriever 全部不动——Vault 改的是「文档从哪来、以什么形态存在、如何组织与关联」，不是检索引擎框架。

### 3.3 新组件（5 个）

| 组件 | 职责 | 关键约束 |
|------|------|---------|
| `VaultFiler` | 唯一写文件出口：.md 生成、frontmatter 序列化（受管字段分区，R-1）、重名冲突、回收站 | 所有 KB 写入必须经它；写入前重读 hash，冲突备份（R-1/R-2） |
| `SyncEngine` | 启动全量扫描 + 定时增量（mtime 预筛 + sha1 去重）→ 变更事件 | 外部删除进 trash 不物理删；hash 匹配识别移动保留 doc_id；KB 自写打标防回环（R-2） |
| `DocSummarizer` | LLM 生成摘要卡（summary/tags/type + summary_hash）回写 frontmatter + DB | LLM 不可用降级跳过，不阻塞入库（NFR-11 哲学） |
| `LinkResolver` | 解析 `[[]]` → links(explicit)；实体抽取 → links(entity)；语义近邻 → links(semantic) | 显式优先；实体抽取停用词/频次过滤（R-3）；隐式关系定期重算 |
| `LexicalIndex` | 强 BM25 倒排（CJK bigram+unigram 分词、字段加权 title×20/tags×5/body×1） | 完全独立派生索引，无业务表耦合、可无状态重建（评审 §6-② 教训） |

---

## 4. 检索性能方案（毫秒级论证）

> 调研核心结论：**十万 chunk 量级下，检索引擎本身早已是毫秒级；唯一瓶颈是远程 embedding API（600~1300ms，占端到端 90%+）。v2 通过 embedding 可选化将该瓶颈整体移出关键路径。**

### 4.1 延迟预算分解（实测数据）

| 环节 | 典型延迟 | 说明 |
|------|---------|------|
| 意图路由（规则） | <5ms | 正则/启发式，禁止为此调 LLM（+300ms 反模式） |
| Query embedding（远程 API） | **600~1300ms** | OpenAI 实测中位数；v2 起为可选路径 |
| Query embedding（本地 bge-m3） | 25~90ms | GPU T4 23ms / 16 核 CPU 89ms |
| Query embedding（model2vec 静态查表） | **<5ms** | 纯 Go 查表+均值池化，质量保留 ~93% |
| 向量搜索（pgvector HNSW） | 2~7ms | 十万级 p50≈3ms，recall 0.93 时 ~1000 QPS |
| BM25 全文（FTS5/Bleve） | 5~20ms | 万级文档实测毫秒级 |
| Cross-encoder rerank（可选） | 100~500ms | GPU 187ms/12 docs；CPU 不可用 |
| 结果组装 + token 截断 | 1~5ms | — |

### 4.2 端到端延迟目标

| 口径 | 目标 | 手段 |
|------|------|------|
| 检索引擎本身（L0/L1，无 embedding） | **<30ms** | BM25 + 文件名索引 + 树导航 |
| 端到端（无语义层，默认配置） | **<30ms** | 全程无 embedding 调用 |
| 端到端（含语义层，model2vec） | **<50ms** | 静态查表 <5ms |
| 端到端（含语义层，本地 transformer） | <150ms | 本地模型常驻 + 预热 |
| 远程 embedding | 600ms~1.3s | 仅作 fallback |

### 4.3 工程手段清单（按 ROI 排序）

1. **意图路由分流**：query 含 `.md`/路径分隔符/引号精确短语 → L0 文件名/词法索引（<10ms 确定性），跳过一切模型调用
2. **强 BM25 修复**（评审核验：现状中文实际失效，见 §1.1-P5）：CJK bigram+unigram（学术定论：与最优词切分效果相当）+ Lucene 版 BM25（k1=1.2, b=0.75）+ 字段加权
3. **查询扩展**（弥补同义词短板，收益最大的一步，+10%~30%）：RM3 伪相关反馈（无模型）或 LLM 关键词扩展（TREC RAG 2025 第一名做法）
4. **Query embedding 缓存**（语义层启用时）：`sha256(normalize(query))` → SQLite + 内存 LRU
5. **语义层本地化**（启用时）：默认 model2vec 静态查表（零依赖）；可选 bge-m3 via Ollama/TEI（`keep_alive=-1` + 预热）
6. **HNSW 替代 IVFFlat**（语义层启用时）：IVFFlat 质心漂移对天天增删的 vault 是硬伤。参数 `m=16, ef_construction=128, ef_search=40~80`，halfvec 内存减半，全索引驻 RAM
7. **结果 token 预算截断**：按 token 上限（如 2000）贪心填充而非固定 top-k
8. **rerank 默认关**：可选参数，仅「深度调研」意图开启；CPU-only 不推荐
9. **明确不做**：PQ/Scalar 量化、分片、DiskANN——十万级全部不必要（hnswlib 十万级单查询 0.05ms）

### 4.4 诚实结论

- 「毫秒级」在 v2 默认配置（无语义层）下**无条件达成**：全程无模型调用，引擎 <30ms
- 语义层的价值边界（诚实）：跨语言、模糊同义概念、「以文找文」推荐三类查询——由查询扩展部分弥补，残余差距是可选化的代价，用户可自行加回

### 4.5 三层检索架构（v2 修订核心）

```
查询意图路由（规则，<5ms）
  ├── 搜文件/路径/精确短语 ──→ L0 精确层（无向量）
  │     文件名索引 + 强 BM25（CJK bigram + 字段加权 + RM3/LLM 扩展）
  ├── 浏览/导航/关联追问 ────→ L1 导航层（无向量）
  │     knowledge_navigate 树导航 + 摘要卡 + 双链/实体图遍历 + knowledge_grep
  └── 概念/模糊/跨语言 ──────→ L2 语义层（可选插件）
        ├─ model2vec 静态查表（自研蒸馏，纯 Go，~50MB）  ← 零依赖默认
        ├─ 本地开源模型（bge-m3 / bge-small-zh，ONNX/Ollama）
        └─ 远程 API（fallback）
        未配置时：L2 缺席，L0+L1 完整可用（NFR-11 降级哲学）
        「相关推荐」降级为：同文件夹 + 共享标签 + 双链共现
```

---

## 5. UI 面板方案（资源管理器）

### 5.1 布局（KnowledgePage 重做）

```
┌────────────┬──────────────────────────────┬────────────────┐
│ Vault 切换  │  面包屑: 财报 / 2026          │  详情面板       │
│ ─────────  │  ┌────────────────────────┐  │  ┌──────────┐  │
│ 文件夹树    │  │ 名称      类型  标签    │  │  │ 摘要卡    │  │
│  ├ 财报    │  │ 2026Q2.md report 财报..│  │  │ tags/type│  │
│  │  └ 2026 │  │ 2026Q1.md report ..   │  │  │ summary  │  │
│  └ 制度    │  └────────────────────────┘  │  │ 出链/入链 │  │
│            │  拖拽上传区（常驻底部）         │  │ 相关推荐  │  │
│            │                              │  └──────────┘  │
└────────────┴──────────────────────────────┴────────────────┘
顶部：统一搜索框（Ctrl+P 唤起浮层）     Tab: 文档 | 图谱 | 检索 | 设置
```

### 5.2 搜索功能（搜文件 + 搜知识）

**统一搜索框 + 结果双区**（Notion 双模 + Everything 即时搜索融合）：

| 区 | 触发 | 数据通路 | 延迟体感 |
|----|------|---------|---------|
| **即时区（搜文件）** | 输入即出（防抖 150ms） | 前端内存索引（文档 <10k 纯前端 fzf 式模糊匹配）或文件名前缀 API（<10ms） | 毫秒级 |
| **语义区（搜知识）** | 输入停顿/回车 | Search API（L0 强 BM25，或 L2 启用时混合） | 亚秒 |

- **搜文件**：模糊匹配文件名 + 路径段参与匹配（`财报\q2`）；排序 = 匹配分 + 最近访问 + 打开频率（Listary 模式）；快捷动作：打开 / 在树中定位 / 复制路径
- **搜内容/知识**：结果按文件分组、命中片段高亮；过滤器 chips（文件夹/标签/类型/时间）自动翻译为操作符回显（Obsidian 模式）
- **意图自动分流**：含路径分隔符/`.md`/引号 → 即时区优先；自然语言问句 → 语义区优先；手动 Tab（文件/内容）兜底；前后端路由规则共享定义

### 5.3 每文档「大致内容」呈现（两级信息密度）

| 层级 | 交互 | 内容 |
|------|------|------|
| L1 hover 摘要卡 | 树/列表内悬停 ~300ms 弹出 | 标题 + 2-3 句 AI 摘要 + 标签 + 出/入链计数 |
| L2 详情面板 | 选中文档右侧固定（可钉住） | 完整摘要、tags/type、来源血缘、出链、入链（区分 linked/unlinked）、相关推荐 3-5 条（**标注来源类型：显式/实体/语义**，R-3） |

### 5.4 图谱视图（二期）

- 默认局部图（当前文档 1-2 跳，深度滑块）；按一级文件夹自动配色；节点大小 = 被引用次数；显式实线/隐式虚线
- 默认隐藏孤儿节点；节点 >2k 提示加过滤器；与详情面板联动

---

## 6. Agent 集成方案

| 工具 | 状态 | 说明 |
|------|------|------|
| `knowledge_search` / `knowledge_reflect` | 保留 | collection_id 语义=vault_id，全库免选路由不变；内部接三层路由 |
| `knowledge_navigate` | **新增** | 三级下钻：`tree(path,depth,cursor)` 缩进树文本（单层 ≤50 条 ≤1k token）→ `card(path)` 摘要卡 ≤200 token → `read(path,offset,limit)` 全文分页（500 行硬上限）。超限截断并返回「如何缩小范围」的机器可读提示 |
| `knowledge_grep` | **新增** | 内容字面/正则搜索（ripgrep 式，跨 vault 根），精确术语/错误码场景（Claude Code 模式） |
| `knowledge_write` | **新增** | 创建/追加笔记、写 `[[]]` 双链 → VaultFiler 落盘 → watcher 自动索引。**安全契约（R-6）**：路径 sanitize（禁 `..`/symlink 逃逸/限 vault root 内）+ 覆盖前备份到 trash + 审计日志（who/when/what） |
| Plan-Then-Retrieve 钩子 | 改造 | 注入 vault 摘要 + 顶层目录结构（替代 flat collection 列表） |

---

## 7. 数据模型、迁移与实现契约

### 7.1 Postgres 变更

| 表 | 变更 |
|----|------|
| `knowledge_collections` | 语义升级 Vault：`+root_path`(唯一)、`+sync_enabled`、`+last_sync_at`；**embedding_model 改可选**（空=无语义层，评审 §6-③ 契约变更） |
| `knowledge_documents` | `+rel_path`、`+content_hash`(sha1)、`+summary`、`+summary_hash`、`+tags`(JSONB)、`+doc_type` |
| `knowledge_links`（新） | `doc_id→target_doc_id, link_type=explicit/entity/semantic, context` |
| `knowledge_entities` + `knowledge_doc_entities`（新） | 实体自动抽取（9.6 GraphRAG 设计简化版，旁路异步不进摄取主链路） |
| `knowledge_chunks` | 不变；语义层启用时索引 IVFFlat→HNSW；向量按 `(model, dim)` 命名空间隔离（S-3） |

### 7.2 迁移（启动期自动、幂等可重入）

1. 现有 Collection → 创建 `data/knowledge_vaults/<collection-id>/` 根目录 + vault.json
2. Document.content_text → 导出 .md（frontmatter 含原 metadata；rel_path=清洗后 source 文件名，冲突去重）
3. 导出后走标准摄取流水线重建派生索引（强 BM25 + 可选向量）
4. 失败单条记录继续；schema_migrations 门控防重；迁移期 Vault 置 `migrating` 态，检索走旧索引（S-2）

### 7.3 降级矩阵

| 场景 | 行为 |
|------|------|
| LLM 不可用 | 摘要卡/实体抽取跳过，原文入库（NFR-11 哲学） |
| 语义层未配置 | L2 缺席，L0+L1 完整可用；相关推荐降级共现法 |
| root_path 不可用 | Vault 置 error，检索降级 DB 已有索引（只读） |
| watcher 失败 | 定时全量扫描兜底（5min） |
| 非 md 文件（pdf/docx/图片） | 原样保留作 asset 血缘，Extractor 提取生成同名 .md 入库 |

### 7.4 实现契约（评审 R-1~R-6，强制）

| # | 契约 |
|---|------|
| R-1 | frontmatter 受管字段分区：KB 独占 `id/summary/tags/type/summary_hash/source/created`，其余归用户；写入前重读 hash，冲突备份原文件到 trash |
| R-2 | SyncEngine 对 KB 自写文件打标（watcher 回环防护）；外部删除一律进 .aranea/trash，不物理删除 |
| R-3 | entity 抽取加停用词/频次过滤；关联区 UI 标注来源类型（显式/实体/语义） |
| R-4 | Embedder 接口抽象 + CreateVault 的 EmbeddingModel 改可选 + 无语义层降级（摄取跳过向量写入、search 自动走 L0+L1、前端设置页改「可选增强」） |
| R-5 | L0 选型 spike 前置：Bleve vs FTS5(trigram) vs 自研倒排各 200 行验证 + 真实语料质量对比后再定 |
| R-6 | knowledge_write 安全契约：路径 sanitize + 覆盖备份 + 审计日志；navigate/grep 只读 |

---

## 8. 复杂度与风险评估（诚实清单）

| # | 风险 | 等级 | 缓解 |
|---|------|------|------|
| R1 | 双向同步冲突（用户改文件 vs KB 回写摘要卡） | **高** | R-1/R-2 契约：受管字段分区 + 写入前重读 hash + 冲突留双份 |
| R2 | watcher 跨平台（Windows 主力） | 中 | 定时扫描 + hash 比对（Zotero 本质也是轮询）；启动全量 + 5min 兜底 |
| R3 | 重命名/移动误判为删+增 | 中 | content_hash 匹配 → 判定移动，保留 doc_id 与索引 |
| R4 | 大 vault 扫描成本（万级文件 sha1） | 低 | mtime 预筛，仅 mtime 变化才重算 hash |
| R5 | 自研分词/倒排新维护面 | 中 | R-5 spike 前置（Bleve 可能 1/5 代码量）；S-5 金标准回归测试 |
| R6 | 双库派生索引（SQLite FTS + PG 向量）一致性 | 低 | 全部无状态可重建，reindex 一键化；FTS 无业务表耦合（评审 §6-② 教训） |
| R7 | agent write 越权/回环 | 中 | R-6 安全契约 + watcher 自写打标 |
| R8 | 前端资源管理器交互复杂度 | 中 | 一期只做 树+列表+详情+统一搜索；图谱二期 |
| R9 | model2vec-go 社区移植成熟度 | 低 | 格式简单（tokenizer+矩阵），可自实现查表 ~300 行 Go 对冲 |

**明确不做（YAGNI）**：Git 内置集成、块级引用、多设备同步、协同编辑、量化/分片、Canvas 白板、SPLADE 训练。

---

## 9. Phase 划分与验收指标

| Phase | 内容 | 验收指标 |
|-------|------|---------|
| **P1 Vault 基座** | VaultFiler（R-1/R-2/R-6 路径安全）+ SyncEngine + Schema 迁移 + 现有数据迁移 + reindex | 给定路径建库；外部放文件 5min 内可检索；现有 Collection 数据完整迁移为 .md |
| **P2 资源管理器 UI** | 树+列表+详情面板+hover 摘要卡+统一搜索双区+拖拽上传改造 | 搜文件毫秒出结果；摘要卡可见；文件夹自由组织 |
| **P3 摘要卡+双轨关联** | DocSummarizer（summary_hash）+ LinkResolver（R-3）+ links/entities 表 + 详情面板关联区 | 每文档有 summary/tags/type；出链/入链/相关推荐可见且标注来源 |
| **P4a 强 BM25 栈** | R-5 选型 spike → LexicalIndex（CJK bigram + 字段加权 + RM3/LLM 扩展）+ 意图路由 + 缓存 | 中文检索金标准（S-5，50 条）召回达标；引擎 <30ms；文件名路由 <30ms |
| **P4b 语义层插件化** | R-4 契约变更（EmbeddingModel 可选）+ model2vec 静态查表后端 + HNSW 迁移（启用时） | 无语义层 vault 全功能可用；model2vec 端到端 <50ms |
| **P5 Agent 工具** | knowledge_navigate（tree/card/read）+ knowledge_grep + knowledge_write（R-6）+ 钩子改造 | agent 可导航下钻、可写笔记且被自动索引；越权写入被拒 |
| **P6 图谱视图（二期）** | 局部图 + 文件夹配色 + 详情联动 | 局部图可用，>2k 节点有过滤引导 |

> P1→P2 是 MVP（可用资源管理器）；P3→P5 是差异化价值；P4a/P4b 可互换顺序；P6 可独立插排。

---

## 10. 结论

方案可行（评审有条件通过，R-1~R-6 合入 §7.4）。核心论据：

1. **检索性能不是风险**——默认配置（无语义层）端到端 <30ms 无条件达成；语义层启用时也有成熟工程手段（model2vec <50ms）
2. **现有中文全文检索实际已失效**（评审核验）——强 BM25 栈是修复而非优化
3. **最大复杂度在双向同步**——通过 R-1/R-2 契约（受管字段分区 + 轮询扫描 + hash 识别移动 + 回收站安全默认）收敛
4. **检索引擎框架零重写**——改动集中在文档来源层（VaultFiler/SyncEngine/LexicalIndex）与呈现层（UI/工具）
5. **设计模式全部有著名系统背书**——Obsidian（vault/双链/摘要卡）、Zotero（watcher 安全默认）、GraphRAG（预生成摘要）、PageIndex（推理导航）、Claude Code（agentic 检索 + 工具 token 预算）、MemGPT（agent 自编辑）、BEIR（BM25 战力）
