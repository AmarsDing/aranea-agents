# M13: Knowledge 知识库 — 详细需求

> 对标 `pkg/trpc-agent-go/knowledge` 包，实现 RAG 知识库能力。
>
> 架构设计、Proto/API 契约、数据模型、代码分层详见 [37-knowledge.design.md](./37-knowledge.design.md)。
> 代码锚点、现状评估、任务清单、Phase 划分详见 [37-knowledge.development.md](./37-knowledge.development.md)。

---

## 1. 用户故事

### US-1：知识库管理员创建知识集合

**作为**知识库管理员，**我希望**创建一个命名的知识集合（Collection），指定嵌入模型和维度，**以便**将相关文档组织在一起并统一检索。

**验收标准**：
- 可通过 API 创建 Collection，指定 name、description、embedding_model。
- 创建后 Collection 状态为 `active`，维度默认 1536。
- name 和 embedding_model 为必填项。

### US-2：用户上传文档到知识集合

**作为**用户，**我希望**向知识集合上传文档（文本/Markdown），**以便**文档被自动分块、向量化并索引，供后续语义搜索使用。

**验收标准**：
- 可通过 API 上传文档，传入 base64 编码的文档内容和元信息。
- 文档创建后状态为 `pending`，后台异步完成分块和向量化。
- 成功后状态变为 `indexed`；失败则变为 `error` 并记录错误信息。
- 分块参数（chunk_size、chunk_overlap）可在请求中指定。

### US-3：用户搜索知识库

**作为**用户，**我希望**通过自然语言查询搜索知识集合，**以便**获取与查询语义相关的文档片段。

**验收标准**：
- 可通过 API 发起语义搜索，指定 collection_id、query、top_k、min_score。
- 返回按相似度排序的文档片段（Chunk），包含内容、分数、来源文档 ID。
- 支持通过 filter_json 进行元数据过滤。

### US-4：Agent 通过工具搜索知识库

**作为** Agent，**我希望**在对话中调用 `knowledge_search` 工具搜索知识集合，**以便**获取外部知识增强回答质量。

**验收标准**：
- Agent 启用 `knowledge_search` 工具开关后，工具自动装配到 Agent 工具集。
- 工具接收 collection_id、query、top_k、min_score 参数。
- 搜索结果以结构化 JSON 返回给模型上下文。

### US-5：用户管理知识集合和文档

**作为**用户，**我希望**列出、查看和删除知识集合及文档，**以便**管理知识库生命周期。

**验收标准**：
- 可列出所有 Collection（分页）。
- 可列出某 Collection 下的所有 Document（分页）。
- 删除 Collection 时级联删除其下所有 Document 和 Chunk。
- 删除 Document 时级联删除其下所有 Chunk。

### US-6：LLM 动态过滤搜索结果（AgenticFilter）

**作为** Agent，**我希望** LLM 根据查询意图动态决定过滤条件，**以便**精准检索相关知识片段。

**验收标准**：
- 启用 AgenticFilter 后，LLM 可在搜索时自动生成过滤参数。
- 过滤条件基于文档元数据字段。

### US-7：OCR 文档识别入库

**作为**用户，**我希望**上传图片或 PDF 文档后自动 OCR 提取文本并入库，**以便**非纯文本文档也能被语义搜索。

**验收标准**：
- 图片/PDF 上传后自动 OCR 提取文本。
- 提取的文本进入分块和向量化流水线。
- OCR 失败时文档状态标记为 `error`。
- OCR 提供者通过 `KNOWLEDGE_OCR` 环境变量配置。

### US-8：多租户知识库隔离（超越层）

**作为**系统管理员，**我希望**不同租户的知识库完全隔离，**以便**租户 A 搜索不到租户 B 的知识。

**验收标准**：
- 搜索请求自动注入租户 ID。
- 向量存储按租户分区。
- 跨租户搜索返回空结果。

### US-9：Agent 自校验检索质量（knowledge_reflect）

**作为** Agent，**我希望**在搜索知识库后评估检索结果是否充分，**以便**在结果不足时自动发起补充检索，提升回答质量。

**验收标准**：
- Agent 启用 `knowledge_reflect` 工具开关后，工具自动装配到 Agent 工具集。
- 工具接收 `collection_ids`（支持多个集合）、`query`、`top_k` 参数。
- 返回结构化评估结果：`sufficient`（是否充分）、`confidence`（置信度）、`supplement_query`（补充查询建议）、`chunks`（检索片段）。
- 当 `FederatedRetriever` 可用时，自动跨多个 Collection 并行搜索。
- 当 `RetrievalEvaluator` 可用时，自动评估检索质量。

### US-10：跨 Collection 联邦搜索

**作为**用户/Agent，**我希望**同时搜索多个知识集合，**以便**从不同知识源获取综合信息。

**验收标准**：
- `knowledge_reflect` 工具支持传入多个 `collection_ids`。
- 多 Collection 搜索并行执行，结果合并去重。
- 单 Collection 搜索时自动降级为标准检索路径。
- 部分集合搜索失败时不阻塞其他集合的结果返回。

### US-11：查询重写与混合检索

**作为**用户，**我希望**搜索时自动优化查询并使用混合检索策略，**以便**复杂查询也能获得高召回率结果。

**验收标准**：
- Search API 支持 `rewrite_strategy` 参数（hyde / decomposition / multi_query）。
- Search API 支持 `hybrid_search` 参数（auto / dense / sparse / rrf）。
- 自适应路由根据查询复杂度自动选择检索模式。
- 检索质量评估（CRAG）在结果不足时自动触发补充检索。

### US-12：拖拽上传文档并自动整理入库

**作为**用户，**我希望**将多个 PDF、Word、Markdown、XML 等文本类文档直接拖拽到知识库页面，**以便**系统自动提取内容、整理为结构化 Markdown 文档并完成分块向量化入库，无需手动逐个配置。

**验收标准**：
- 知识库文档面板提供拖拽区域，支持一次拖入多个文件。
- 每个文件生成独立上传任务，实时展示状态（pending → indexing → indexed / error）。
- 文本类（txt/md/json/csv/html/xml/yaml）与 Office 类（pdf/doc/docx/xlsx/pptx）文档均可正确提取文本。
- 默认开启「整理为 Markdown」：提取文本经 LLM 结构化为 Markdown 后再按标题层级分块入库。
- LLM 不可用或整理失败时自动降级为原文本入库，不阻塞流程。
- 整理后的 Markdown 全文可在文档列表中预览。
- 拖入图片等多模态文件时给出明确提示（多模态支持见 US-13），不静默失败。

### US-13：多模态资料入库（图片）

**作为**用户，**我希望**将图片（png/jpg/jpeg/webp）拖入知识库，**以便**系统通过多模态 LLM 理解图片内容并整理为 Markdown 描述文档入库，与文本文档一样可被语义搜索。

**验收标准**：
- 图片与文本文档共用同一拖拽入口与上传队列。
- 图片经多模态 LLM 输出结构化 Markdown 描述（含图中文字、表格、图表含义）后入库。
- 原始图片留存并与文档记录关联（血缘可追）。
- 检索结果与文本文档无差别返回；文档 metadata 记录模态与提取方式。
- 多模态 LLM 不可用时文档状态标记为 `error` 并给出明确错误信息。

### US-14：免选择知识库（存储分类、使用免选）

**作为**用户，**我希望**存入知识库的资料就是我的知识（可分门别类存放），但在会话中使用时**不需要我决定用哪个知识库**——系统自动在我的全部知识中检索，**以便**我无需理解"知识库/Collection"概念即可获得基于私有知识的回答。

**验收标准**：
- 上传不强制预选 Collection：未选中时自动落入系统「默认知识库」（首个上传自动创建），不静默丢弃文件。
- 会话中 Agent 检索知识时无需指定 Collection：默认在用户全部 Collection 中智能路由检索（按集合名称/描述与问题的匹配度取 top N 广播 + 结果合并）。
- `knowledge_search` / `knowledge_reflect` 工具的 Collection 参数改为可选，留空即全库检索。
- Agent 绑定特定 Collection 保留为高级配置（默认不绑定 = 全库可搜），用于「专属客服只搜产品文档」类精细化场景。
- 文档可跨 Collection 移动（整理：默认库收件箱 → 分类库归档），移动后检索立即可见于目标库。
- 调试搜索面板默认「全部知识库」。

---

## 2. 功能规格

### 2.1 知识集合管理

| 功能 | 说明 |
|------|------|
| 创建集合 | 指定 name、description、embedding_model |
| 列出集合 | 分页查询，支持 workspace 过滤 |
| 获取集合 | 按 ID 获取单个集合详情 |
| 删除集合 | 级联删除文档和向量块 |

### 2.2 文档管理

| 功能 | 说明 |
|------|------|
| 上传文档 | base64 + 可选 `chunk_strategy`；PDF/DOCX/HTML 自动解析 |
| 拖拽批量上传 | 文档面板拖拽区域，一次拖入多文件，逐文件生成上传任务 |
| 整理为 Markdown | 提取文本经 LLM 结构化为 MD 后按标题层级分块入库（默认开启，可关闭；失败降级原文本） |
| MD 全文预览 | 整理后的 Markdown 全文持久化，文档列表可预览 |
| 多模态入库 | 图片经多模态 LLM 输出 MD 描述入库（png/jpg/jpeg/webp），原图留存血缘 |
| 上传免预选 | 未选中 Collection 时自动落「默认知识库」（懒创建），不静默丢弃 |
| 文档跨库移动 | 文档连同 chunks 移动至目标 Collection，计数同步校正 |
| 列出文档 | 按集合分页查询 |
| 删除文档 | 级联删除向量块 |
| 文档状态 | pending → indexing → indexed / error |
| 进度可观测 | WS `knowledge_ingest` 事件 + 管理页文档 status 轮询 |

### 2.3 语义搜索

| 功能 | 说明 |
|------|------|
| 向量搜索 | 余弦相似度，ivfflat 索引 |
| 元数据过滤 | filter_json 通过 JSONB `@>` 操作符 |
| 最低分数过滤 | min_score 阈值 |
| TopK 限制 | 默认 5 |
| Reranker | topk / cohere / infinity（env + SearchRequest 覆盖） |
| 重排候选 oversample | `rerank_candidates` 或默认 topK×3（上限 50） |
| 查询重写 | HyDE / Decomposition / MultiQuery（`rewrite_strategy` 参数） |
| 混合检索 | Dense + BM25 + RRF 融合（`hybrid_search` 参数） |
| 自适应路由 | 查询复杂度分类 → 自动选择检索模式 |
| 检索质量评估 | CRAG 式自校验，不足时自动补充检索 |
| BM25 全文检索 | PostgreSQL ts_vector + pg_trgm 双路检索 + GIN 索引 |
| 全库免选检索 | Collection 留空 = 全库智能路由（名称/描述匹配取 top N 广播 + RRF 合并），使用时无需选择知识库 |

### 2.4 分块策略

| 策略 | 键 | 说明 |
|------|-----|------|
| 按字符 | `char` | 按 N 字符窗口分割，含重叠 |
| 按 Token | `token` | 空格分词，近似 Token 计数 |
| Markdown 按标题 | `markdown` | 按标题层级分块（trpc chunking） |
| JSON 结构 | `json` | 按 JSON 结构分块 |
| 递归分块 | `recursive` | 递归字符分割 |

### 2.5 嵌入提供者

| 提供者 | 说明 |
|--------|------|
| OpenAI 兼容 | `/v1/embeddings` 端点 |
| Ollama | `/api/embeddings` 端点 |
| Gemini | Google GenAI API |
| HuggingFace | TEI `/embed` 批量 |

### 2.6 Agent 集成

| 功能 | 说明 |
|------|------|
| knowledge_search 工具 | Agent 可调用搜索知识库 |
| knowledge_reflect 工具 | Agent 可评估检索质量 + 跨 Collection 搜索 |
| 工具开关 | Agent 工具配置中启用/禁用 |
| AgenticFilter | LLM 动态生成过滤条件（未实现） |
| code_search 工具 | 代码语义搜索（未实现） |

### 2.7 高级功能

| 功能 | 说明 |
|------|------|
| OCR 识别 | 图片/PDF 自动提取文本 |
| Reranker | 检索结果重排序（TopK/Cohere/Infinity） |
| SourceSync | 数据源增量同步（未实现） |
| 多租户隔离 | 租户间知识库完全隔离（未实现） |
| Extractor | 格式转换统一抽象：文本类（trpc reader 提取 → LLM 整理为 MD）+ 多模态（视觉 LLM 图片 → MD），产出物归一为 Markdown |
| 跨 Collection 联邦搜索 | 多集合并行搜索 + 结果合并（Broadcast + Route 策略） |
| Plan-Then-Retrieve | Agent 系统提示注入 Collection 摘要 |

---

## 3. 非功能需求

| # | 需求 | 说明 |
|---|------|------|
| NFR-1 | 需要 PostgreSQL + pgvector 扩展 | 无 Postgres 时模块不可用，API 返回明确错误 |
| NFR-2 | 嵌入维度每个集合固定 | 更改需重建集合 |
| NFR-3 | 文档内容必须可文本解码 | 图片/PDF 需 OCR 提取 |
| NFR-4 | 文档级 metadata_json 写入 Chunk | 供 filter_json 检索过滤 |
| NFR-5 | 查询重写和检索评估依赖 LLM | 无可用 LLM 时自动降级（透传原始查询 / 跳过评估） |
| NFR-6 | 联邦搜索支持 Broadcast 和 Route 策略 | Route 策略基于 Collection 名称/描述相关性评分 |
| NFR-7 | Plan-Then-Retrieve 通过 BeforeModel 钩子注入 | 高频场景下可能增加延迟 |
| NFR-8 | Embedder 配置优先级 | env `KRATOS_KNOWLEDGE_EMBED_*` > `system_settings` DB > 运行时 Knowledge API/UI |
| NFR-9 | 上传安全守卫 | 32MB 大小限制 + MIME magic 检测 + 白名单；OOXML（zip 容器）按声明 MIME/扩展名二次判定 |
| NFR-10 | Embedder 超时可配 | `KRATOS_KNOWLEDGE_EMBED_TIMEOUT_SEC` 环境变量，默认 60s |
| NFR-11 | LLM 整理降级 | 无可用 LLM 或整理失败时透传原文本入库，不阻塞摄取 |
| NFR-12 | 多模态入库依赖 | 图片理解依赖多模态 LLM Provider，未配置时图片上传返回明确错误 |
| NFR-13 | 模态归一 | 任何模态提取后归一为 Markdown 文本，下游分块/向量化/检索无模态差异 |

---

## 4. 验收标准总览

| # | 验收标准 |
|---|----------|
| 1 | 可创建/列出/获取/删除知识集合 |
| 2 | 可上传文档并异步完成分块和向量化 |
| 3 | 文档向量化后存储到 pgvector，可进行相似度搜索 |
| 4 | Agent 可调用 `knowledge_search` 工具搜索知识库 |
| 5 | 支持元数据过滤（filter_json） |
| 6 | 知识搜索支持动态过滤（AgenticFilter） |
| 7 | 图片/PDF 文档可 OCR 识别入库 |
| 8 | 多租户知识库隔离 |
| 9 | 摄取进度前端可观测（WS / 文档 status） |
| 10 | Embedder 配置从 conf/env 注入 + Admin 运行时更新 |
| 11 | 查询重写（HyDE/Decomposition/MultiQuery）可按请求指定 |
| 12 | 混合检索（Dense+BM25+RRF）可按请求指定模式 |
| 13 | 自适应路由根据查询复杂度自动选择检索策略 |
| 14 | 检索质量评估（CRAG）在结果不足时自动补充检索 |
| 15 | Agent 可调用 `knowledge_reflect` 工具评估检索质量 |
| 16 | 跨 Collection 联邦搜索（多集合并行 + 结果合并） |
| 17 | Plan-Then-Retrieve（Agent 系统提示注入 Collection 摘要） |
| 18 | 联邦搜索 Route 策略（基于相关性智能路由） |
| 19 | 拖拽批量上传文本类文档，自动整理为 Markdown 入库，状态实时可见 |
| 20 | 整理后 Markdown 全文持久化，可预览 |
| 21 | 图片经多模态 LLM 整理为 Markdown 描述入库，原图血缘可追 |

---

## 5. 交互规格（用户视角）

### 5.1 知识库管理页

- 路由 `/knowledge`，侧栏入口 `menu.knowledge`
- 页面级三 Tab 结构：**文档 | 检索 | 设置**
  - 文档 Tab：左侧集合列表 + 右侧操作区（顶部常驻拖拽上传区 → 上传队列 → 选中集合详情卡 + 文档面板）
  - 检索 Tab：调试搜索面板（见 5.2）
  - 设置 Tab：Embedder 配置（低频操作收纳，见 5.3）
- 集合列表：展示名称、嵌入模型、维度、文档数、块数、状态
- 文档列表：按集合分页展示，显示来源、MIME 类型、大小、状态、错误信息、入库时间、更新时间
- 入库进度：WebSocket 实时推送 `knowledge_ingest` 事件，文档状态自动刷新；存在 indexing 中文档且文档 Tab 激活时前端轮询兜底
- 服务不可用 / 加载错误时以条件横幅提示（非常驻），错误横幅附重试入口

### 5.2 搜索面板

- 位于「检索」Tab
- 输入：collection_id（默认「全部知识库」）、query、top_k、min_score、filter_json
- 高级控件：rewrite_strategy（hyde/decomposition/multi_query）、hybrid_search（auto/dense/sparse/rrf）、use_rerank，各控件附一行简要说明
- 结果展示：按相似度排序的 Chunk 列表，含内容、分数、来源文档 ID

### 5.3 Embedder 配置面板

- 位于「设置」Tab；面板内注明该配置与系统设置页共享同一配置源，并提供跳转链接
- 展示当前 Embedder 配置（provider/base_url/model/dim，API Key 脱敏）
- 运行时更新：修改 provider/base_url/api_key/model/dim 并写回 DB
- 系统设置页可写 `knowledge_embed` 字段

### 5.4 文档入库对话框（粘贴文本）

- 标题「粘贴文本入库」，仅支持文本粘贴模式；文件上传统一走拖拽上传区（文本输入框内提示引导）
- 输入：source（文件名/URL/描述）、mime_type、text、metadata_json
- 分块参数：chunk_size、chunk_overlap、chunk_strategy（char/token/markdown/json/recursive）
- 上传后立即返回，异步处理进度通过 WS 推送

### 5.5 拖拽上传区（文件统一入口）

- 位于文档 Tab 右侧操作区顶部，常驻，高亮提示可拖入文件
- 支持多文件同时拖入，自动推断 source（文件名）与 mime_type
- 上传队列逐文件展示：文件名、大小、状态（pending → indexing → indexed / error）、错误信息
- 默认参数：整理为 Markdown 开启、分块策略 markdown
- 图片与文本文档共用入口（多模态支持上线前图片给出明确提示）
- 文档列表提供「预览」入口查看整理后的 Markdown 全文

---

## 子模块：Vault 重设计（V2 需求，2026-07-25）

> 设计详见 [37-knowledge.design.md §子模块：Vault 重设计](./37-knowledge.design.md#子模块vault-重设计)；任务计划见 [37-knowledge.development.md §子模块：Vault 重设计 Phase 计划](./37-knowledge.development.md#子模块vault-重设计-phase-计划)。
> 方案论证：[2026-07-25-research-knowledge-vault-redesign.md](../reports/2026-07-25-research-knowledge-vault-redesign.md)；评审：[2026-07-25-review-knowledge-vault-redesign.md](../reports/2026-07-25-review-knowledge-vault-redesign.md)。
> 本节为 V2 需求，US-1~US-14（Collection 时代）在迁移完成前继续有效；V2 落地后 Collection 语义升级为 Vault。

### US-15：给定本地路径创建知识库（Vault）

**作为**用户，**我希望**指定一个本地文件夹路径即可创建知识库，**以便**我现有的文档目录（如 Obsidian 库、资料文件夹）直接成为可被 agent 检索的知识库，无需逐个上传。

**验收标准**：
- 创建 Vault 时指定 `root_path`（本地目录），系统校验目录存在且非系统根目录。
- 嵌入模型改为**可选**：不配置时知识库完整可用（词法检索 + 导航），配置后额外获得语义检索。
- 同一 `root_path` 不可重复挂载（规范化后唯一）。
- 仍可创建多个 Vault，每库一路径，检索以库为隔离边界。

### US-16：双向同步（文件夹 ↔ 知识库）

**作为**用户，**我希望**把文件放进/移出/修改该文件夹时知识库自动跟进，**以便**知识库永远反映文件夹真实内容，我不需要在系统里重复维护一份。

**验收标准**：
- 向文件夹新增/修改 .md 文件后，知识库在扫描周期内（≤5min）自动入库/更新索引。
- 从文件夹删除文件后，知识库将其移入回收站（`.aranea/trash/`）并下线索引，**不物理删除**。
- 文件移动/重命名被识别为移动（保留文档身份与索引），而非删除+新增。
- 系统回写（摘要卡、agent 笔记）不会造成自我重复摄取。
- 文件夹不可用（盘拔出/权限）时 Vault 标记 error，已有索引保持只读可检索。

### US-17：每文档摘要卡（大致内容一览）

**作为**用户，**我希望**每篇文档都有系统生成的摘要卡（摘要/标签/类型），**以便**我不用打开全文就知道这篇文档大致讲了什么，并能按标签/类型浏览。

**验收标准**：
- 文档入库后自动生成摘要卡（LLM 可用时），写入文档 frontmatter 与索引。
- 文档内容变化后摘要卡自动更新。
- LLM 不可用时文档正常入库，摘要卡留空待补，不阻塞。
- 摘要/标签/类型为系统受管字段，与用户手写内容互不覆盖。

### US-18：文档关联（双链 + 自动关联）

**作为**用户，**我希望**看到每篇文档与哪些文档相关，**以便**我能顺着关联浏览知识、发现没注意到的联系。

**验收标准**：
- 支持在 Markdown 中写 `[[文档名]]` 显式双链，详情面板展示出链与入链。
- 系统自动抽取实体生成隐式关联，并标注关联来源类型（显式/实体/语义）。
- 详情面板提供相关文档推荐（语义层可用时为向量近邻，否则为同目录+共享标签+共现）。

### US-19：资源管理器式知识库面板

**作为**用户，**我希望**知识库像资源管理器一样呈现（文件夹树 + 文件列表 + 详情面板），**以便**我按自己熟悉的文件夹结构组织与浏览知识。

**验收标准**：
- 左栏 Vault 切换 + 文件夹树；中栏当前目录文档列表；右栏选中文档的详情面板（摘要卡 + 关联 + 预览）。
- 统一搜索框：输入即时显示**文件名匹配**（搜文件，毫秒级），回车/停顿触发**内容检索**（搜知识）。
- 支持路径式搜索（如 `财报\q2` 命中 `财报/2026Q2.md`）与过滤器（文件夹/标签/类型/时间）。
- 拖拽上传到指定文件夹；支持面包屑导航与「在树中定位」。
- 文件可拖拽移动到其他目录（树目录节点/库根/面包屑段为落点，非法落点禁用）；同名冲突弹确认：覆盖 / 保留两份 / 取消。
- 搜索框可选定目录范围（范围选择器），选中后即时区与语义区均只在该目录内匹配，直至清除。

### US-20：Agent 导航与自编辑知识库

**作为** Agent，**我希望**能像人一样浏览知识库（看目录树 → 读摘要卡 → 按需读全文），并能创建笔记，**以便**我低成本定位知识（而非盲目检索）并沉淀会话中学到的内容。

**验收标准**：
- `knowledge_navigate` 工具提供 tree/card/read 三级能力，返回受 token 预算约束，超限给出缩小范围提示。
- `knowledge_grep` 工具提供内容字面/正则搜索（只读）。
- `knowledge_write` 工具可创建/追加笔记，写入后自动被索引；越权路径（库外目录）被拒绝并记录。
- 现有 `knowledge_search` / `knowledge_reflect` 行为不变（collection_id 语义 = vault_id）。

### V2 非功能需求（增补）

| # | 需求 | 说明 |
|---|------|------|
| NFR-14 | 文件系统即真相源 | 全部派生索引（检索/向量/关联）可无状态重建，提供 reindex |
| NFR-15 | 无 embedding 完整可用 | 默认配置端到端检索 <30ms；语义层为可选增强插件 |
| NFR-16 | 写入安全 | 一切写入经唯一出口；覆盖前备份；agent 写入限 vault root 内并记审计日志 |
| NFR-17 | 同步保守默认 | 冲突留双份、删除进回收站，任何场景不丢用户数据 |
| NFR-18 | 迁移不中断 | 存量 Collection 迁移为 Vault 期间检索可用，迁移幂等可重入 |

### V2 验收标准总览（增补）

| # | 验收标准 |
|---|----------|
| 22 | 给定本地路径可创建 Vault；同一路径不可重复挂载 |
| 23 | 文件夹内增/改/删/移文件，知识库在扫描周期内正确跟进，删除入回收站 |
| 24 | 每文档有摘要卡（summary/tags/type），内容变化后自动更新 |
| 25 | `[[双链]]` 出链/入链可见，自动关联标注来源类型 |
| 26 | 资源管理器面板可树形浏览；统一搜索区分搜文件（即时）与搜知识（回车） |
| 27 | 无 embedding 配置时知识库全功能可用（检索/导航/关联降级版） |
| 28 | agent 可 tree→card→read 导航下钻；可写笔记并被自动索引；越权写入被拒 |
| 29 | 存量 Collection 数据完整迁移为 Vault 文件，迁移期检索不中断 |

### V2 交互规格（增补，用户视角）

- **页面结构**：知识库页为「浏览 | 图谱 | 设置」三 Tab（检索能力融入浏览 Tab 顶部双区搜索，独立检索 Tab 已由图谱取代）。
- **浏览 Tab**：三栏（Vault 树 | 目录内容 | 详情面板），顶部统一搜索框 + 面包屑；上传入口在树节点 hover 菜单（新建目录/新建文档/上传文件到此）。
- **搜索框**：即时区（文件名，输入即出，150ms 防抖）+ 内容区（回车触发，结果按文件分组、命中高亮）；前缀「范围」按钮弹出迷你目录树，选中目录后两区均限定该目录。
- **拖拽移动**：中栏文件行可直接拖到左栏目录/库根或面包屑段完成移动；目标发光高亮、非法落点禁用；同名冲突弹「覆盖 / 保留两份 / 取消」。
- **详情面板**：摘要一行（hover 大号浮层卡）+ 关联计数 chips + 正文/媒体区（md/txt 可编辑，图片/音频/视频内联预览，word 下载）。
- **图谱 Tab**：左 3D 力导向图（节点按类型着色、大小=连接度；旋转/平移/缩放；hover 高亮一跳邻居；点击选中）；右操作台（库选择、边类型过滤、目录范围、节点搜索定位、节点列表、选中节点卡「在浏览中打开」）。
- **设置 Tab**：Vault 路径与同步开关、嵌入模型（可选增强）、重建索引入口。
