# 评审报告：知识库 Vault 重设计方案 + 无 Embedding 检索决策

> 类型：评审报告（design review）。
> 评审对象：
> 1. Vault 重设计方案（原方案文档未落盘；方案内容已按本评审 R-1~R-6 修订后权威落档至 [37-knowledge.design.md §子模块 Vault 重设计](../development/37-knowledge.design.md#子模块vault-重设计)，Phase 计划见 [37-knowledge.development.md §子模块 Vault 重设计 Phase 计划](../development/37-knowledge.development.md#子模块vault-重设计-phase-计划)）
> 2. [2026-07-25-research-embedding-free-retrieval.md](./2026-07-25-research-embedding-free-retrieval.md)（无 embedding 调研 + 三层检索修订）
> 评审维度：正确性、优缺点、后期维护成本、技术升级路径、安全与依赖风险。
> 评审方式：方案文本评审 + 代码事实核验（核验记录见 §6）。

---

## 1. 总体结论

**有条件通过**。方向正确、调研扎实、每项决策均有著名系统或 benchmark 背书；但存在 6 项必须修改项（R 系列，见 §7）和 5 项建议项（S 系列，见 §8），集中在：双向同步的所有权细则、自研分词/倒排的范围收敛、embedding 可选化对现有契约的破坏面、agent 写入的安全边界。

一句话评价：**这是一个把「检索引擎选型焦虑」转化为「文档来源与组织革命」的方案**——它正确地识别了瓶颈不在 ANN 算法，而在知识的存在形态（P1–P4 问题），并且用 embedding 可选化消除了唯一的外部模型依赖。

---

## 2. 方案的最大优点（评审确认成立）

| # | 优点 | 评审依据 |
|---|------|---------|
| A1 | **文件系统即真相源**解决了知识锁定（P1）——用户的知识第一次以通用格式（.md）存在，可被 Git/编辑器/同步盘自由使用 | Obsidian 十年验证；Logseq 反向教训（粒度到块才被迫放弃纯文本，本方案保持文件级，避开） |
| A2 | **embedding 可选化**消除了方案中唯一的网络长尾（600~1300ms）与外部依赖，与项目 NFR-11 降级哲学自洽 | BEIR-NL(2024)：BM25+rerank 与最佳稠密模型打平；金融基准 BM25 反超稠密 |
| A3 | **检索引擎零重写**，改动集中在文档来源层（VaultFiler/SyncEngine）与呈现层 | 现有 Retriever/Hybrid/Federated 全家桶保留，降低回归面 |
| A4 | `knowledge_navigate`（tree/card/read）= PageIndex 模式的本地化实现，且**真实文件夹树比 PageIndex 的 AI 生成 ToC 更可靠**（无需自愈校验管道） | PageIndex FinanceBench 98.7% 背书推理导航路线 |
| A5 | 性能论证基于实测数字而非直觉（HNSW 2~7ms、BM25 5~20ms、bge-m3 本地 25~90ms），延迟目标分层诚实（引擎 <30ms / 端到端 <150ms / 缓存命中 <30ms） | pgvector-bench、hnswlib 实测 |

---

## 3. 逐项决策评审（优点 / 缺点 / 维护 / 升级路径）

### D1 文件系统即真相源 + 双向混合（Obsidian 式）

- **优点**：知识可携带、可版本化（用户自行 git init 即得历史）；用户已有文档目录零迁移成本接入；KB 产物（整理后 MD、agent 笔记）对用户透明可见
- **缺点**：双向同步是全方案**复杂度最高的部分**（R1 高风险）；真相源在用户手里意味着系统必须容忍任何野蛮修改（改一半、编码错乱、二进制混入）
- **后期维护**：同步逻辑是永久性维护面——frontmatter 受管字段所有权、冲突备份、回收站清理策略、大目录扫描性能，都需要长期规则演进
- **升级路径**：轮询扫描 →（可选）fsnotify 事件加速；单向 → 双向已有；未来可加 .git 集成做变更审计
- **评审意见**：通过，但 R-1（受管字段分区）与 R-2（写入前重读 hash）必须从「缓解措施」升级为「实现契约」写进设计文档

### D2 多 Vault，每库一路径

- **优点**：保留库级检索隔离（FastGPT/AnythingLLM 共识）；Collection 平滑升级，Agent 绑定 `knowledge_bases` 语义不变；`root_path` 唯一约束天然防重复挂载
- **缺点**：库间文档无法直接双链（跨 vault 的 `[[]]` 无意义）；用户可能把同一目录挂两次（需 root_path 规范化：resolve symlink + 绝对路径 + 尾部斜杠归一）
- **后期维护**：低。Vault 生命周期（创建/暂停同步/删除）需明确「删库不删文件」语义
- **升级路径**：未来可加「库引用」（跨库只读链接），本期不做
- **评审意见**：通过。补充契约：root_path 规范化 + 删除 Vault 只删索引不动文件

### D3 frontmatter 摘要卡（LLM 预生成）

- **优点**：「每个文档大致内容」的最优解——人可读（直接打开 .md 可见）、机可查（DB 索引）、agent 可低 token 消费（card ≤200 token）；GraphRAG/RAPTOR 证明预生成摘要是用索引期成本换查询期能力
- **缺点**：LLM 生成有成本与失败率；文档更新后摘要可能过期（stale summary 比没有摘要更误导 agent）
- **后期维护**：摘要失效策略需明确——content_hash 变化即标记 stale，异步重生成；frontmatter 中 `summary` 是受管字段（KB 独占），用户手改会被覆盖，需文档化告知
- **升级路径**：单文档摘要 → 文件夹级摘要（目录 README 自动生成）→ vault 级摘要树（RAPTOR 式，二期候选）
- **评审意见**：通过。补充：frontmatter 加 `summary_hash`（被摘要内容的 hash），比对即知过期，避免盲目重生成

### D4 双轨关联（explicit / entity / semantic）

- **优点**：显式精确可信 + 隐式自动冷启动，叠加时显式优先是 Obsidian+GraphRAG 的共识做法；links 表三类型共用一套查询
- **缺点**：entity 抽取质量依赖 LLM，噪声实体（「本文」「作者」之类）会污染关联区；semantic 近邻依赖 L2 语义层，未配置 embedding 时缺一条轨
- **后期维护**：实体抽取需要噪声过滤规则（停用实体表、频次阈值）；links 表随文档增删级联维护，重建逻辑简单（全量重扫）
- **升级路径**：实体共现 → 完整 GraphRAG（社区检测 + 社区摘要，9.6 已有设计，旁路异步不阻塞摄取）；无语义层时 semantic 轨降级为「同文件夹 + 共享标签 + 双链共现」
- **评审意见**：有条件通过。R-3：entity 抽取必须做停用词/频次过滤，且关联区 UI 要标注来源类型（显式/实体/语义），避免用户误以为全是可靠关联

### D5 三层检索 + embedding 可选化（核心修订）

- **优点**：见 A2。路由规则化（<5ms）避免 LLM 路由反模式；「搜文件跳过 embedding」修正了走向量搜文件名的架构错误
- **缺点**：**测试矩阵翻倍**（有/无语义层两套）；路由规则需要持续调优（误判：概念查询被路由到 BM25 时召回差）；「相关推荐」降级版（共享标签+共现）质量明显低于向量近邻
- **后期维护**：路由规则表是配置化资产，随 badcase 积累迭代；三层各自独立可测
- **升级路径**：L2 插件化设计（Embedder 接口）保证三层解耦——未来接入任何新 embedding 技术不动 L0/L1
- **评审意见**：通过，但依赖 R-4（Embedder 接口抽象 + Collection 的 EmbeddingModel 必填校验放开，见 §6-③）

### D6 自研 CJK bigram + 强 BM25

- **优点**：学术定论支持（bigram+unigram 与最优中文词切分效果相当）；零外部依赖；**修正现状重大缺陷**（见 §6-①：现有中文全文检索实际失效）；FTS5 列权重天然支持 title×20/tags×5/body×1 字段加权
- **缺点**：自研分词+倒排是新增维护代码（估 600~1000 行 + 测试）；bigram 索引体积约 2×；FTS5 引 SQLite 进知识链路，与 PG 向量形成**双库派生索引**（重建时需双边一致）
- **后期维护**：分词器规则稳定后变动少；FTS5 是纯派生索引（DROP/REBUILD 无状态），维护负担低——但必须吸取 §6-② 的教训（FTS5 曾因与核心表触发器耦合而被整体移除）
- **升级路径**：自研/FTS5 → Bleve（纯 Go，BM25+boost，嵌入式单库）→ Tantivy/pg_textsearch（若坚持 PG 原生）→ SPLADE（学习稀疏，需训练基础设施，本期否决）
- **评审意见**：**修改后通过**。R-5：动手前必须先做 Bleve vs FTS5 vs 自研倒排的三方技术选型验证（各 200 行 spike + 真实语料质量对比），禁止直接默认自研——Bleve 可能用 1/5 代码量达到同等效果

### D7 model2vec 静态化语义层（「自研 embedding」落点）

- **优点**：把 embedding 变成一次性蒸馏的本地资产（几十 MB 查表模型）；推理=查表+均值池化，纯 Go 可实现（~300 行），比 teacher 快 500 倍；质量保留 ~93%（MTEB 52.13）；无 GPU/ONNX 依赖
- **缺点**：丢语序/上下文，难查询与 rerank 场景弱于 transformer；多语版 480MB 不算小；**model2vec-go 是单人社区移植（2026-04），成熟度存疑**
- **后期维护**：模型文件版本化（teacher 更换→dim 变化→全库 reindex）；向量必须按模型命名空间隔离（`embeddings(model, dim)` 维度已在现有 schema，天然支持）
- **升级路径**：model2vec → bge-m3 本地（ONNX/Ollama）→ 远程 API——同一 Embedder 接口，平滑替换
- **评审意见**：通过。定位为「语义层的零依赖默认实现」而非唯一实现。供应链风险用自实现查表（格式简单：tokenizer + 矩阵）对冲，不依赖社区移植

### D8 Agent 工具族（navigate / grep / write）

- **优点**：navigate 三级下钻 + token 预算 + 超限截断提示 = Claude Code 验证过的模式；grep 填补精确内容搜索；write 实现 Letta 式自编辑（agent 自主决定记什么）
- **缺点**：**knowledge_write 是安全敏感面**——agent 在用户文件夹里写文件，路径注入/prompt 注入可导致越权写入；write 与 watcher 形成回环（agent 写→索引→检索到→再写），需防自激
- **后期维护**：工具 schema 演进需与 agent 提示词同步；write 工具的审计日志是永久维护项
- **升级路径**：write 从「创建/追加」→「结构化编辑（frontmatter 字段级）」→「批量整理（agent 当图书管理员）」
- **评审意见**：有条件通过。R-6：write 安全契约必须前置——路径 sanitize（禁 `..`、禁 symlink 逃逸、限制 vault root 内）、覆盖前自动备份到 .aranea/trash、每次写入记审计日志（who/when/what）；watcher 对 KB 自写文件打标防回环重复摄取

### D9 迁移（Collection → Vault）

- **优点**：幂等可重入 + 失败单条跳过 + schema_migrations 门控，符合项目三层迁移体系（L3 数据迁移）惯例；旧「粘贴文本入库」入口保留降低用户断裂感
- **缺点**：content_text 导出 .md 后，原文档的 chunks 需重建——大库迁移期检索可用性下降；文件名清洗（source→合法文件名）冲突处理是脏活
- **后期维护**：一次性代码，迁移完成后可标记废弃；需保留旧表只读兜底一个版本周期
- **升级路径**：—
- **评审意见**：通过。补充：迁移期 Vault 状态机加 `migrating` 态，期间检索走旧索引、写入排队

### D10 资源管理器 UI

- **优点**：双区搜索（即时区纯前端毫秒 + 语义区亚秒）正确分离了「搜文件/搜知识」两种意图；hover 卡 + 详情面板两级密度是 Obsidian/Zotero 验证模式；图谱放二期避免 MVP 膨胀
- **缺点**：前端重写量最大（KnowledgePage 三 Tab → 资源管理器布局 + 树 + 详情面板 + 统一搜索）；前端内存索引（<10k 文档 fzf 式）在多 vault 切换时需重建
- **后期维护**：树组件懒加载 + 缓存策略随库规模演进；搜索意图分流规则与后端路由规则需保持一致（两处维护，需共享定义）
- **升级路径**：列表 → 网格/卡片视图；局部图（二期）→ 全局图；搜索 → Q&A 模式（三期候选）
- **评审意见**：通过

---

## 4. 后期维护成本总账

| 维护面 | 频率 | 成本 | 对冲 |
|--------|------|------|------|
| 双向同步规则（冲突/回收站/所有权） | 持续，随 badcase | **中-高** | 所有权分区契约 + 保守默认（冲突留双份） |
| 路由规则调优（前后端两处） | 随 badcase | 中 | 规则表配置化 + 前后端共享定义 |
| 摘要过期重生成 | 自动 | 低 | summary_hash 比对 |
| 实体噪声过滤 | 定期 | 中 | 停用实体表 + 频次阈值 + UI 标注来源 |
| 分词器（若自研） | 稳定后低 | 低 | Bleve 选型可消除 |
| 派生索引重建（FTS5 + 向量 + links） | 按需 | 低 | 全部无状态可重建，reindex 一键化 |
| EmbeddingModel 契约放宽的回归 | 一次性 | 低 | 见 §6-③ |
| agent write 审计 | 持续 | 低 | 审计日志结构化入 activities |

**总评**：维护重心从「检索引擎运维」（embedding API 配额、向量索引调参）转移到「同步规则与数据质量运营」——这是正确的转移，因为前者是外部不可控成本，后者是内部可积累资产。

---

## 5. 技术升级路径图

```
L0 精确层:  自研bigram/FTS5 ──→ Bleve ──→ pg_textsearch/Tantivy ──→ SPLADE(远期)
L1 导航层:  真实文件夹树+摘要卡 ──→ 目录README自动摘要 ──→ 长文档AI ToC(PageIndex式) ──→ vault摘要树(RAPTOR式)
L2 语义层:  model2vec静态查表 ──→ bge-m3本地(ONNX/Ollama) ──→ 远程API ──→ HNSW(halfvec) 索引升级
关联:      双链+实体共现 ──→ GraphRAG完整版(社区摘要,9.6设计已备) ──→ KAG式符号推理(远期)
agent:     navigate/grep/write ──→ frontmatter字段级编辑 ──→ agent图书管理员(批量整理)
UI:        树+列表+详情+双区搜索 ──→ 局部图谱(二期) ──→ Q&A模式(三期)
```

每层升级**相互独立**（接口隔离：Embedder / Retriever / LinkResolver 各自抽象），这是方案架构上最经得起时间考验的部分。

---

## 6. 代码事实核验记录（评审发现）

**① 现有中文全文检索实际已失效（强化 D6 必要性，发现重大现状缺陷）**

[knowledge.go:514-517](file:///f:/aranea-agents/internal/data/knowledge.go#L514-L517) 使用 `ts_rank(to_tsvector('simple', content), plainto_tsquery('simple', $1))`。双重问题：
- `ts_rank` 无 IDF、无 TF 饱和——弱 BM25（评审前已知）
- **更严重**：PG 的 `simple` 分词配置对 CJK 文本不做切分，连续中文被归为单一 token（PG 需 zhparser/pg_jieba 才有中文分词）——意味着中文查询几乎只能整串精确命中，**现有 BM25 对中文基本不可用**。这解释了为何模块高度依赖向量通道，也反过来证明：强 BM25 自研栈不是「优化」而是「修复」

**② FTS5 的前车之鉴（D6 维护约束）**

项目曾有 `messages_fts`（SQLite FTS5），在 [20260902_drop_messages_subsystem.sql](file:///f:/aranea-agents/internal/data/sql/migrations/20260902_drop_messages_subsystem.sql) 中被整体移除。教训：FTS5 虚拟表 + 触发器与核心业务表耦合后，演进时被迫连根拔。知识库 FTS 必须设计为**完全独立的派生索引**（无触发器耦合、无业务表依赖、DROP/REBUILD 无状态）

**③ embedding 可选化破坏现有契约（D5 前置工作）**

[CreateCollection 校验](file:///f:/aranea-agents/internal/biz/knowledge/knowledge_usecase_test.go#L118)：`ErrEmbeddingModelRequired`——当前 **EmbeddingModel 是必填字段**，且 Collection 创建时 `ValidateEmbeddingModelDim`。embedding 可选化需要：
- CreateVault 时 EmbeddingModel 改可选（空 = 无语义层）
- 摄取流水线对无 embedding 的 vault 跳过向量写入
- knowledge_search 对无语义层 vault 自动降级 L0+L1
- 前端设置页 embedding 配置改「可选增强」
这是 P4 阶段必须显式列出的契约变更清单，评审要求写入设计文档

---

## 7. 必须修改项（R 系列，合入设计文档的前置条件）

| # | 项 | 归属 |
|---|---|------|
| R-1 | frontmatter 受管字段分区契约：KB 独占 `id/summary/tags/type/summary_hash/source/created`，其余归用户；写入前重读 hash，冲突备份 | D1/D3 |
| R-2 | SyncEngine 对 KB 自写文件打标（watcher 回环防护）；外部删除一律进 .aranea/trash | D1 |
| R-3 | entity 抽取加停用词/频次过滤；关联区 UI 必须标注来源类型（显式/实体/语义） | D4 |
| R-4 | Embedder 接口抽象 + CreateVault 的 EmbeddingModel 改可选 + 无语义层降级矩阵（§6-③ 四条） | D5 |
| R-5 | L0 选型 spike 前置：Bleve vs FTS5(trigram) vs 自研倒排，各 200 行验证 + 真实语料质量对比后再定 | D6 |
| R-6 | knowledge_write 安全契约：路径 sanitize + 覆盖备份 + 审计日志；navigate/grep 只读 | D8 |

## 8. 建议项（S 系列，不阻塞）

| # | 项 |
|---|---|
| S-1 | root_path 规范化（resolve symlink + 绝对路径归一）+ 禁挂系统根目录的校验 |
| S-2 | 迁移期 Vault 增加 `migrating` 状态，期间检索走旧索引 |
| S-3 | 向量命名空间按 `(model, dim)` 隔离，为 model2vec→bge 升级预留 reindex 能力 |
| S-4 | 目录级 README 自动摘要（文件夹 hover 也能看「这个目录大致是什么」）列入二期候选 |
| S-5 | 评审建议将「BM25 召回质量」纳入测试基线（构造 50 条中英查询金标准，防分词器回归） |

---

## 9. 评审结论

方向与架构：**通过**。工程细节：**R-1~R-6 合入设计文档后方可进入实施计划**。

建议 Phase 微调：P4 拆为 P4a（L0 强 BM25 栈，含 R-5 选型 spike）+ P4b（L2 语义层插件化，含 R-4 契约变更）；R-6 安全契约并入 P5。

> 后续状态（2026-07-25）：R-1~R-6 已合入 [37-knowledge.design.md §V6](../development/37-knowledge.design.md#子模块vault-重设计)，Phase 计划（P1~P6，含 P4a/P4b）已落档 [37-knowledge.development.md](../development/37-knowledge.development.md#子模块vault-重设计-phase-计划)，前置条件已满足，可进入实施。
