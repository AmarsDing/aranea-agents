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
- 集合列表：展示名称、嵌入模型、维度、文档数、块数、状态
- 文档列表：按集合分页展示，显示来源、MIME 类型、大小、状态、错误信息
- 入库进度：WebSocket 实时推送 `knowledge_ingest` 事件，文档状态自动刷新

### 5.2 搜索面板

- 输入：collection_id、query、top_k、min_score、filter_json
- 高级控件：rewrite_strategy（hyde/decomposition/multi_query）、hybrid_search（auto/dense/sparse/rrf）、use_rerank
- 结果展示：按相似度排序的 Chunk 列表，含内容、分数、来源文档 ID

### 5.3 Embedder 配置面板

- 展示当前 Embedder 配置（provider/base_url/model/dim，API Key 脱敏）
- 运行时更新：修改 provider/base_url/api_key/model/dim 并写回 DB
- 系统设置页可写 `knowledge_embed` 字段

### 5.4 文档入库对话框

- 输入：source（文件名/URL/描述）、mime_type、content_base64、metadata_json
- 分块参数：chunk_size、chunk_overlap、chunk_strategy（char/token/markdown/json/recursive）
- 上传后立即返回，异步处理进度通过 WS 推送

### 5.5 拖拽上传区

- 文档面板内嵌拖拽区域，高亮提示可拖入文件
- 支持多文件同时拖入，自动推断 source（文件名）与 mime_type
- 上传队列逐文件展示：文件名、大小、状态（pending → indexing → indexed / error）、错误信息
- 默认参数：整理为 Markdown 开启、分块策略 markdown
- 图片与文本文档共用入口（多模态支持上线前图片给出明确提示）
- 文档列表提供「预览」入口查看整理后的 Markdown 全文
