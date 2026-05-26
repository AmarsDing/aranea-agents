import type { KnowledgeDocument } from "./types";
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions
} from "../ui/registryTableColumns";

export const KNOWLEDGE_DOC_TABLE_COLUMNS = [
  registryCol<KnowledgeDocument>("source", "来源", "source", "left", "28%"),
  registryCol<KnowledgeDocument>("status", "状态", "status", "left", REGISTRY_COL_W.status),
  registryCol<KnowledgeDocument>("chunk_count", "分块", "chunk_count", "right", REGISTRY_COL_W.metric),
  registryCol<KnowledgeDocument>("size_bytes", "大小", "size_bytes", "right", REGISTRY_COL_W.metric),
  registryColActions<KnowledgeDocument>(REGISTRY_COL_W.actions, "")
];

/** @deprecated 使用 KNOWLEDGE_DOC_TABLE_COLUMNS */
export const knowledgeDocColumns = KNOWLEDGE_DOC_TABLE_COLUMNS;

export function knowledgeStatusColor(status: string): string {
  if (status === "active" || status === "indexed") return "positive";
  if (status === "error") return "negative";
  if (status === "indexing" || status === "pending") return "warning";
  return "grey";
}

export const INDEXING_DOC_STATUSES = new Set(["indexing", "pending"]);

export function hasIndexingDocuments(documents: KnowledgeDocument[]): boolean {
  return documents.some((d) => INDEXING_DOC_STATUSES.has(d.status));
}

export function formatKnowledgeDocSize(bytes?: number): string {
  const n = Number(bytes) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
