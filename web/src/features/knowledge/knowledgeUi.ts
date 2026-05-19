import type { KnowledgeDocument } from "./types";

export const knowledgeDocColumns = [
  { name: "source", label: "来源", field: "source", align: "left" as const },
  { name: "status", label: "状态", field: "status", align: "left" as const },
  { name: "chunk_count", label: "分块", field: "chunk_count", align: "right" as const },
  { name: "size_bytes", label: "大小", field: "size_bytes", align: "right" as const },
  { name: "actions", label: "", field: "id", align: "right" as const }
];

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
