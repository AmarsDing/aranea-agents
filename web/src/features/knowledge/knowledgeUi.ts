import type { KnowledgeDocument } from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

export const KNOWLEDGE_DOC_TABLE_COLUMNS = [
  registryCol<KnowledgeDocument>('source', '来源', 'source', 'left', '22%'),
  registryCol<KnowledgeDocument>('mime_type', '类型', 'mime_type', 'left', '14%'),
  registryCol<KnowledgeDocument>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<KnowledgeDocument>('chunk_count', '分块', 'chunk_count', 'right', REGISTRY_COL_W.metric),
  registryCol<KnowledgeDocument>('size_bytes', '大小', 'size_bytes', 'right', REGISTRY_COL_W.metric),
  registryCol<KnowledgeDocument>('created_at', '入库时间', 'created_at', 'left', REGISTRY_COL_W.timeWide),
  registryColActions<KnowledgeDocument>(REGISTRY_COL_W.actions, ''),
];

/** @deprecated 使用 KNOWLEDGE_DOC_TABLE_COLUMNS */
export const knowledgeDocColumns = KNOWLEDGE_DOC_TABLE_COLUMNS;

export function knowledgeStatusColor(status: string): string {
  if (status === 'active' || status === 'indexed') return 'positive';
  if (status === 'error') return 'negative';
  if (status === 'indexing' || status === 'pending') return 'warning';
  return 'grey';
}

export const INDEXING_DOC_STATUSES = new Set(['indexing', 'pending']);

export function hasIndexingDocuments(documents: KnowledgeDocument[]): boolean {
  return documents.some((d) => INDEXING_DOC_STATUSES.has(d.status));
}

export function formatKnowledgeDocSize(bytes?: number): string {
  const n = Number(bytes) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatKnowledgeTime(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

export const KNOWLEDGE_HYBRID_MODE_OPTIONS = [
  { label: '自动', value: 'auto' },
  { label: '向量检索', value: 'dense' },
  { label: '全文检索', value: 'sparse' },
  { label: '混合 (RRF)', value: 'rrf' },
];

export const KNOWLEDGE_REWRITE_STRATEGY_OPTIONS = [
  { label: '无', value: '' },
  { label: 'HyDE', value: 'hyde' },
  { label: '查询分解', value: 'decomposition' },
  { label: '多查询', value: 'multi_query' },
];

export const KNOWLEDGE_CHUNK_STRATEGY_OPTIONS = [
  { label: '字符 (默认)', value: '' },
  { label: 'Token', value: 'token' },
  { label: 'Markdown', value: 'markdown' },
  { label: 'JSON', value: 'json' },
  { label: '递归', value: 'recursive' },
];
