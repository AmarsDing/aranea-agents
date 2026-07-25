# 无 Embedding 检索可行性调研：自研方向与实践案例

> 类型：调研报告。回答「知识库能否不用 embedding、自研检索这部分功能」。
> 配套文档：Vault 重设计方案已合入 [37-knowledge.design.md §子模块 Vault 重设计](../development/37-knowledge.design.md#子模块vault-重设计)（评审 R-1~R-6 已作为实现契约落入 §V6）；评审见 [2026-07-25-review-knowledge-vault-redesign.md](./2026-07-25-review-knowledge-vault-redesign.md)
> 调研范围：2024–2026 论文（SIGIR/NeurIPS/ICLR/arXiv）、GitHub 开源（PageIndex/LightRAG/model2vec 等）、工业界实践（Claude Code/Cursor/Obsidian）、TREC/BEIR benchmark。

---

## 1. 结论先行

**可以不用 embedding，且有三条成熟实践路线背书**：

| 路线 | 代表 | 核心思想 | 对本项目 |
|------|------|---------|---------|
| **推理导航（无向量）** | PageIndex（GitHub ~3 万星，FinanceBench 98.7%） | 文档组织成树，LLM 像人翻书一样导航 | 我们的 `knowledge_navigate`(tree/card/read) 本质就是此模式——文件夹树 + 摘要卡天然是 PageIndex 结构 |
| **Agentic 词法检索** | Claude Code（Glob/Grep/Read） | 不建任何索引，agent 用 grep 直接搜磁盘 | Anthropic 首席工程师 Boris Cherny：「早期用 RAG+向量库，很快发现 agentic search 明显更好」 |
| **自研稀疏检索** | BM25 + CJK bigram + 查询扩展 | 倒排索引 + 概率排序 + 同义词扩展 | BEIR-NL(2024) 实证：**BM25+rerank 与最佳稠密模型打平** |

**推荐架构转向：embedding 从「必需组件」降级为「可选增强插件」**——三层检索中两层完全无向量，语义层可插拔（本地模型 / model2vec 自研静态化 / 远程 API 三选一，缺省时系统完整可用）。这与模块既有的 NFR-11「无 LLM 降级」哲学一脉相承。

---

## 2. 路线一：推理导航（PageIndex 模式）

- **原理**：「相似 ≠ 相关」。索引期把文档组织为层级目录树（AI 生成 ToC + 自愈校验）；查询期 LLM 在树上推理导航（官方类比 AlphaGo 树搜索），逐层下钻，全程可解释。无 embedding、无 top-k 超参
- **成绩**：FinanceBench 98.7% 准确率，远超向量 RAG
- **独立评估（2026-01 对照实验）**：跨章节推理/综合类问题占优；简单事实抽取打平或略逊；**本质是把索引期 embedding 成本转移到查询期 LLM 成本**——适合少量高价值长文档，不适合高频检索
- **对本项目的映射**：我们的 Vault 文件夹树是用户真实目录（比 AI 生成 ToC 更可靠），frontmatter 摘要卡 ≈ PageIndex 的节点摘要。`knowledge_navigate` 三级工具（tree 缩进树 ≤1k token → card 摘要卡 ≤200 token → read 分页全文）= 现成的推理导航链路，**零向量依赖**

## 3. 路线二：Agentic 词法检索（Claude Code 模式）

- **实践**：Claude Code 的 Glob/Grep/Read（ripgrep 内核）不建索引，agent 动态决定搜索策略
- **胜出理由**（Anthropic Boris Cherny + 社区分析）：①精确匹配需求（标识符/错误码，向量天然近似反而误事）；②高 churn 语料索引必然过期，grep 永远读当前磁盘；③隐私（内容不外发 embedding API）；④中小规模语料索引维护成本永远收不回
- **学术佐证**：金融基准（23,088 查询）BM25 Recall@5 = 0.644 **反超**稠密检索 0.587；混合+重排达 0.816。BRIGHT 复现（SIGIR 2026）：推理密集型长查询上 BM25Q 是最强词法基线
- **对本项目的映射**：agent 工具族加 `knowledge_grep`（内容正则/字面搜索）成本极低，与 navigate 互补

## 4. 路线三：自研稀疏检索栈（BM25 系，可完全自研）

### 4.1 战力证据

- BEIR 原始论文：首批稠密模型零样本均分**低于 BM25**
- BEIR-NL（2024）：BM25 仅被「专为检索训练的大稠密模型」超过；**BM25+rerank 与最佳稠密模型打平**
- SIGIR 2024 复现：稀疏模型跨域泛化优于稠密模型；长文档（>500 词）检索 BM25 明显占优

### 4.2 自研技术栈（每层给出最简实现）

| 层 | 最简自研 | 现成选择 | 要点 |
|----|---------|---------|------|
| 分词 | Unicode script 分段 + CJK bigram(+unigram) + 英文 lowercase，**约 200 行 Go** | gojieba、FTS5 trigram | 学术结论（Nie et al.）：bigram+unigram 与最优中文词切分效果**相当**，超过 13.9 万词条典的最大匹配法。零外部依赖 |
| 倒排 | 内存 `map[term][]posting`（万级文档几十 MB） | SQLite FTS5（内置 bm25()+列权重）、Bleve(纯 Go)、Tantivy(Rust) | FTS5 列权重 title×20/tags×5/body×1 |
| 打分 | Lucene 版 BM25（k1=1.2, b=0.75；长文档试 b=0.4） | 上述全部内置 | **注意：现有 PG tsvector 的 ts_rank 无 IDF/TF 饱和，是弱 BM25**——需换实现 |
| 查询扩展 | RM3 伪相关反馈（top-5 文档扩 10–30 词，原查询权重 0.5–0.7） | LLM 关键词扩展（TREC RAG 2025 第一名系统做法）、HyDE-text 变体 | **收益最大的一步**，相对提升 +10%~30%，弥补同义词短板 |
| rerank | 先不做；需要时 top-50 喂本地小模型 | BGE-reranker / RankGPT 类 | RankVicuna 7B 只 rerank top-20 即接近全部收益 |
| 字段加权 | title/tags/path 加权 + mtime 时间衰减 | FTS5 列权重、Bleve boost | 本地工具（Obsidian/Everything）的共性补偿设计 |

### 4.3 重要现状修正

现有模块的 `SearchChunksBM25` 基于 PG `tsvector + ts_rank`——**无 IDF、无 TF 饱和、长度归一化弱**（ParadeDB/Tiger Data 均有专文指出此硬伤）。若走强 BM25 路线，需替换为 FTS5（SQLite，项目已有双轨）或自研倒排。

---

## 5. 「自研 embedding」路径客观核算（如果仍想要语义层）

| 路径 | 数据/算力 | 质量 | 判断 |
|------|----------|------|------|
| 从零训练中英双语模型 | 数亿文本对 + 数十~数百 A100 时（bge-small-zh 预训练用 24×A100） | 需追平开源才有意义 | **不划算，否决** |
| 蒸馏 tiny student | 10 万–1M 无标注句 + 单卡数小时 | teacher 的 90–97% | 产出上限就是现成小模型，不值得 |
| LSA/fastText 自训 | 万级文档 + CPU 分钟级 | 关键词级，天花板低 | 仅冷启动兜底 |
| **model2vec 静态化** | 无需训练数据，teacher 词表前向 + PCA，CPU ~30 秒 | potion-base-32M 达 MiniLM 的 93.2%；多语版含中文 | **最值得跟进**：蒸馏产物是几十 MB 查表模型，推理=查表+均值池化（纯 Go 零依赖，已有 Go 移植 ammar-ahmed22/model2vec-go），比 teacher 快 500 倍 |

model2vec 的意义：**它把「embedding」变成可自研/自托管的纯本地资产**——一次性蒸馏后无任何模型推理依赖，Go 直接查表。这是「自研 embedding」的现实最优解。

---

## 6. 向量仍不可替代的场景（诚实边界）

1. **跨语言检索**（中文 query 找英文文档）
2. **同义/模糊概念查询**（「找关于 X 理念的内容」，无共享词面）——查询扩展只能部分弥补
3. **「以文找文」相似推荐**（相关推荐区依赖向量近邻）
4. 大规模开放域首轮召回的边际成本（向量按查询近零成本，LLM 导航按查询计费）

务实结论（与业界中间派共识一致）：**词法/结构/图做精确层，向量做语义兜底，LLM 做导航与重排**。

---

## 7. 对 Vault 方案的修订建议（三层检索，embedding 可选化）

```
查询意图路由（规则，<5ms）
  ├── 搜文件/路径/精确短语 ──→ L0 精确层（无向量）
  │     文件名索引 + 强 BM25（CJK bigram + 字段加权 + RM3/LLM 扩展）
  ├── 浏览/导航/关联追问 ────→ L1 导航层（无向量）
  │     knowledge_navigate 树导航 + 摘要卡 + 双链/实体图遍历 + knowledge_grep
  └── 概念/模糊/跨语言 ──────→ L2 语义层（可选插件）
        ├─ 本地开源模型（bge-m3 / bge-small-zh，ONNX/Ollama）
        ├─ model2vec 静态查表（自研蒸馏，纯 Go，~50MB）  ←「自研」落点
        └─ 远程 API（fallback）
        未配置时：L2 缺席，L0+L1 完整可用（NFR-11 降级哲学）
```

**Phase 调整建议**：
- P4「检索性能」改为「强 BM25 自研栈 + 意图路由 + 缓存」（不再以 HNSW 迁移为主体）
- 新增 P4b「语义层插件化」：Embedder 接口抽象（现有 `Embedder` 接口已具备雏形），实现 model2vec 静态查表后端 + 本地模型引导
- 「相关推荐」功能在无语义层时降级为：同文件夹 + 共享标签 + 双链共现

**风险变化**：移除了「远程 embedding 长尾延迟」（R6 降级为可选项）；新增「中文分词自研质量」风险——由学术结论（bigram 与词切分相当）兜底，风险低。

---

## 8. 来源（精选）

- [PageIndex GitHub](https://github.com/VectifyAI/PageIndex) ｜ [对照评估](https://nateking-assets.sfo3.cdn.digitaloceanspaces.com/pageindex-rag-evaluation.pdf)
- [Claude Code 不建索引分析](https://vadim.blog/claude-code-no-indexing/) ｜ [检索成本曲线](https://harrisonsec.com/blog/agent-retrieval-cost-curve-claude-code-grep-vs-rag/)
- [BEIR 论文 arXiv:2104.08663](https://arxiv.org/pdf/2104.08663.pdf) ｜ [BEIR-NL arXiv:2412.08329](https://arxiv.org/pdf/2412.08329v1) ｜ [BM25 vs Dense 金融基准](https://arxiv.org/pdf/2604.01733v1)
- [中文 bigram 与词切分相当（ACM）](https://dl.acm.org/doi/pdf/10.1145/355214.355235)
- [BM25s arXiv:2407.03618](https://arxiv.org/abs/2407.03618) ｜ [pg_textsearch：PG 上做真 BM25](https://www.scien.cx/2026/04/08/pg_textsearch-1-0-how-we-built-a-bm25-search-engine-on-postgres-pages/) ｜ [ParadeDB 谈 tsvector 局限](https://www.paradedb.com/learn/search-in-postgresql/bm25)
- [TREC RAG 2025 第一名：LLM 关键词扩展 BM25](https://trec.nist.gov/pubs/trec34/papers/UTokyo.rag.pdf) ｜ [HyDE arXiv:2212.10496](https://arxiv.org/abs/2212.10496) ｜ [RankVicuna arXiv:2309.15088](https://arxiv.org/pdf/2309.15088.pdf)
- [model2vec GitHub](https://github.com/MinishLab/model2vec) ｜ [model2vec-go](https://github.com/ammar-ahmed22/model2vec-go) ｜ [sentence-transformers 蒸馏文档](https://www.sbert.net/examples/sentence_transformer/training/distillation/README.html)
- [KAG arXiv:2409.13731](https://arxiv.org/pdf/2409.13731) ｜ [HippoRAG NeurIPS 2024](https://proceedings.neurips.cc/paper_files/paper/2024/file/6ddc001d07ca4f319af96a3024f6dbd1-Paper-Conference.pdf) ｜ [生成式检索局限 arXiv:2305.02073](https://arxiv.org/pdf/2305.02073v1)
