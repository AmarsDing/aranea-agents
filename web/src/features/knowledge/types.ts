export type KnowledgeCollection = {
  id: string;
  name: string;
  description: string;
  embedding_model: string;
  dim: number;
  /** active | indexing | error */
  status: string;
  document_count: number;
  chunk_count: number;
  workspace: string;
  created_at: string;
  updated_at: string;
  /** Vault 根目录（V2：本地文件夹即真相源）；空 = 历史 Collection 未迁移。 */
  root_path: string;
  /** active | paused | error | migrating */
  sync_state: string;
  last_sync_at: string;
  /** SP1-F：存储后端维度。local（文件系统真相源）| team（PG 真相源）。 */
  vault_backend: string;
};

export type KnowledgeDocument = {
  id: string;
  collection_id: string;
  source: string;
  mime_type: string;
  size_bytes: number;
  chunk_count: number;
  /** pending | indexing | indexed | error */
  status: string;
  error_message: string;
  created_at: string;
  updated_at: string;
  /** Computed by backend: true when the MIME type is supported for text extraction. */
  extract_supported?: boolean;
  /** vault 相对路径（'/' 分隔）；空 = 非 vault 文档。 */
  rel_path: string;
  /** LLM 摘要卡（可能过期；不可用时为空）。 */
  summary: string;
  /** LLM 打标（摘要卡）。 */
  tags: string[];
  /** report | manual | note | faq | ... */
  doc_type: string;
};

/** VaultTreeNode 是 vault 文件夹懒加载列表的一项（P3 资源管理器）。 */
export type VaultTreeNode = {
  /** 末段名称 */
  name: string;
  /** vault 相对路径（'/' 分隔）；'' = 根 */
  path: string;
  /** dir | file */
  kind: string;
  // file 专属字段（dir 为空/零值）：
  doc_id: string;
  summary: string;
  tags: string[];
  doc_type: string;
  status: string;
  size_bytes: number;
  updated_at: string;
  /** 解析失败原因（status=error 时非空） */
  error_message: string;
};

/** KnowledgeLink 是一条已解析文档关联（P3 关联区，R-3 来源标注）。 */
export type KnowledgeLink = {
  target_doc_id: string;
  target_source: string;
  target_rel_path: string;
  /** explicit（显式双链）| entity（实体共现）| semantic（语义近邻） */
  link_type: string;
  /** explicit: [[ref]] 原文；entity: 共享实体名 */
  context: string;
  /** out（本文引用目标）| in（目标引用本文） */
  direction: string;
};

/** BlockBacklink 是一条块级反向链接（SP1-E/I-1 反链分组）。 */
export type BlockBacklink = {
  src_block_id: string;
  src_doc_id: string;
  src_collection_id: string;
  /** 来源文档展示名（rel_path） */
  src_doc_name: string;
  /** 原始链接文本 */
  raw_target: string;
  /** ref | embed */
  edge_type: string;
  /** 引用上下文片段（±50 字符） */
  context: string;
  /** 跨库歧义解析（确定性取首） */
  ambiguous: boolean;
};

/** DanglingLink 悬空链聚合（SP1-E/I-2：raw_target 分组，「未创建笔记」语义；
 *  目标创建后自动复活）。 */
export type DanglingLink = {
  raw_target: string;
  ref_count: number;
  /** 块级引用来源（复用 BlockBacklink 形状：src_* 字段有效） */
  refs: BlockBacklink[];
};

/** UnlinkedMention 未链接提及（P2-7）：本文档名在他文档正文中以纯文本出现
 *  （[[wikilink]] 内的出现不计），按来源文档聚合。 */
export type UnlinkedMention = {
  src_doc_id: string;
  /** 来源文档展示名（rel_path） */
  src_doc_name: string;
  /** 纯文本出现次数 */
  count: number;
  /** 首次出现上下文片段 */
  snippet: string;
};

/** LinkUseEntry 一条最近 wikilink 落链记录（B4 #8，last_used_at 降序消费）。 */
export type LinkUseEntry = {
  doc_id: string;
  /** RFC3339；零值时间为空串 */
  last_used_at: string;
};

/** PromoteLineage 源→克隆谱系对（SP1-G/I-3：源块 promoted_to ↔ 新块 promoted_from）。 */
export type PromoteLineage = {
  src_block_id: string;
  new_block_id: string;
  target_doc_id: string;
};

/** PromoteCascadeCandidate 级联提示：晋升块引用了未一并晋升的私有目标，
 *  团队侧落 dangling（raw_target 保留，目标创建后自动复活）。 */
export type PromoteCascadeCandidate = {
  src_block_id: string;
  raw_target: string;
  dst_doc_id: string;
  dst_collection_id: string;
};

/** PromoteResult 晋升结果（新建块谱系 + 级联提示清单）。 */
export type PromoteResult = {
  created_blocks: PromoteLineage[];
  cascade_candidates: PromoteCascadeCandidate[];
};

/** EnableSemanticResult 词法库启用语义层受理结果（B2：绑定的全局 embedder + 重嵌入受理数）。 */
export type EnableSemanticResult = {
  enqueued_docs: number;
  embedding_model: string;
  dim: number;
};

export type KnowledgeChunk = {
  id: string;
  doc_id: string;
  collection_id: string;
  content: string;
  embedding: number[];
  metadata_json: string;
  chunk_index: number;
  /** similarity score — only populated in search results */
  score: number;
};

export type CreateCollectionInput = {
  name: string;
  description?: string;
  /** V2：可选；留空 = 仅词法检索库（无语义层）。 */
  embedding_model?: string;
  /** V2：Vault 根目录（本地文件夹绝对路径），必填。 */
  root_path: string;
};

export type IngestDocumentInput = {
  collection_id: string;
  source: string;
  mime_type?: string;
  /** raw document payload encoded in standard base64 */
  content_base64: string;
  metadata_json?: string;
  chunk_size?: number;
  chunk_overlap?: number;
  /** char | token | markdown | json | recursive */
  chunk_strategy?: string;
  /** unset/true = LLM 整理为 Markdown（失败降级原文本）；false = 原文本入库 */
  organize_to_markdown?: boolean;
  /** G1-B3：vault 内目标目录（'/' = 库根）；空 = 历史行为（不落盘，仅 vault 集合有效） */
  target_dir?: string;
};

export type KnowledgeDocumentContent = {
  id: string;
  content_text: string;
  organized: boolean;
  /** G2-B5：vault 文件 body 原文（编辑器数据源；frontmatter 不含），非 vault 为空。 */
  raw_content: string;
  /** G2-B5：vault 文件 sha1，编辑保存（UpdateDocumentContent）的 CAS expectedHash。 */
  base_hash: string;
};

export type KnowledgeUploadTask = {
  id: string;
  name: string;
  size: number;
  mime_type: string;
  /** reading | uploading | success | error */
  status: string;
  message?: string;
  /** US-14：免预选上传时标注目标库（未选中集合 = 「默认知识库」）。 */
  collection_label?: string;
};

export type SearchKnowledgeQuery = {
  collection_id: string;
  query: string;
  top_k?: number;
  min_score?: number;
  filter_json?: string;
  use_rerank?: boolean;
  rerank_candidates?: number;
  rewrite_strategy?: string;
  hybrid_search?: string;
  /** G3-B7：搜索范围（vault 相对目录前缀，带尾斜杠）；空 = 全库。 */
  path_prefix?: string;
};

export type EmbedderConfig = {
  provider: string;
  base_url: string;
  model: string;
  dim: number;
  configured: boolean;
  has_api_key: boolean;
};

export type UpdateEmbedderConfigInput = {
  provider?: string;
  base_url?: string;
  api_key?: string;
  model?: string;
  dim?: number;
};

export type ListCollectionsResult = {
  items: KnowledgeCollection[];
  total: number;
};

export type ListDocumentsResult = {
  items: KnowledgeDocument[];
  total: number;
};

/** CollectionGraphNode 是单库图谱的一个文档节点（G4-B8 3D 知识图谱）。 */
export type CollectionGraphNode = {
  doc_id: string;
  name: string;
  rel_path: string;
  doc_type: string;
  /** 入边连接度（大小映射；孤立节点 = 0）。 */
  degree: number;
};

/** CollectionGraphEdge 是一条文档间有向关联（端点均在范围内）。 */
export type CollectionGraphEdge = {
  /** 出向文档 doc_id。 */
  source: string;
  /** 入向文档 doc_id。 */
  target: string;
  /** explicit | entity | semantic */
  type: string;
};

/** CollectionGraph 单库全量图谱（一次性返回，无分页）。 */
export type CollectionGraph = {
  nodes: CollectionGraphNode[];
  edges: CollectionGraphEdge[];
};

/** EntityMergeSuggestion 单条实体合并候选对（G5-F B11 实体治理）。
 *  source = norm（name_norm 相同但展示名不同）| embedding（实体名 embedding 高相似）。 */
export type EntityMergeSuggestion = {
  keeper_id: number;
  keeper_name: string;
  mergee_id: number;
  mergee_name: string;
  /** norm | embedding */
  source: string;
  similarity: number;
  tier: string;
};

/** MergeEntitiesResult 一键合并重写反馈（G5-F B10；内联展示重写条数）。 */
export type MergeEntitiesResult = {
  rewritten_mentions: number;
  rewritten_links: number;
  merged_entities: number;
};

export type AutolinkPreview = {
  doc_id: string;
  replacements: number;
  preview: string;
  unchanged: boolean;
};

export type AutolinkApplyResult = {
  doc_id: string;
  replacements: number;
};

export type CollectionHealth = {
  document_count: number;
  edge_count: number;
  explicit_edges: number;
  isolated_count: number;
  orphan_rate: number;
  link_density: number;
  dangling_count: number;
  writeback_notes: number;
  writeback_latest: string;
};

export type KnowledgeExpert = {
  agent_id: string;
  user_id: string;
  fact_count: number;
  last_kind: string;
};

export type PendingWriteBackItem = {
  fact_id: string;
  statement: string;
  kind: string;
  confidence: number;
  agent_id: string;
  user_id: string;
  session_id: string;
  source: string;
};

/** 工作区写回落点（US-46）：团队收件箱或第一个 team 库；found=false 表示尚未创建。 */
export type WriteBackHome = {
  found: boolean;
  collection_id: string;
  name: string;
  vault_backend: string;
};
