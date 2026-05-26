import type { KnowledgeDocument } from "./types";
import { registryColWidth } from "../ui/registryTableColumns";

export const knowledgeDocColumns = [
  { name: "source", label: "来源", field: "source", align: "left" as const, ...registryColWidth("28%") },
  { name: "status", label: "状态", field: "status", align: "left" as const, ...registryColWidth("9%") },
  { name: "chunk_count", label: "分块", field: "chunk_count", align: "right" as const, ...registryColWidth("72px") },
  { name: "size_bytes", label: "大小", field: "size_bytes", align: "right" as const, ...registryColWidth("72px") },
  { name: "actions", label: "", field: "id", align: "right" as const, ...registryColWidth("108px") }
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

export function formatKnowledgeDocSize(bytes?: number): string {
  const n = Number(bytes) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
