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
};

export type SearchKnowledgeQuery = {
  collection_id: string;
  query: string;
  top_k?: number;
  min_score?: number;
  filter_json?: string;
};

export type ListCollectionsResult = {
  items: KnowledgeCollection[];
  total: number;
};

export type ListDocumentsResult = {
  items: KnowledgeDocument[];
  total: number;
};
