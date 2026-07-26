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
  embedding_model: string;
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
};

export type KnowledgeDocumentContent = {
  id: string;
  content_text: string;
  organized: boolean;
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
