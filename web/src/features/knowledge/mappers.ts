import { asRecord, pickI32, pickStr } from '../../shared/wireJson';

/** @deprecated Use mapCollection from features/knowledge/api.ts instead — this type omits description/workspace/updated_at. */
export type KnowledgeCollectionRow = {
  id: string;
  name: string;
  embedding_model: string;
  dim: number;
  status: string;
  document_count: number;
  chunk_count: number;
  created_at: string;
};

/** @deprecated Use mapCollection from features/knowledge/api.ts instead. */
export function mapCollection(raw: unknown): KnowledgeCollectionRow {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    name: pickStr(r, 'name', 'name'),
    embedding_model: pickStr(r, 'embedding_model', 'embeddingModel'),
    dim: pickI32(r, 'dim', 'dim'),
    status: pickStr(r, 'status', 'status'),
    document_count: pickI32(r, 'document_count', 'documentCount'),
    chunk_count: pickI32(r, 'chunk_count', 'chunkCount'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
  };
}
