// web/src/features/chat/knowledgeRecall.ts
/**
 * knowledge_recalled notice payload (citation transparency).
 *
 * Backend source of truth: internal/tools/knowledge/tool.go
 *   - knowledgeRecalledNoticeType = "knowledge_recalled"
 *   - payload { chunks: [{ chunk_id, doc_id, score, line }] }
 *
 * The notice step is hidden from the chat stream (see noticeFilter.ts);
 * chunks are indexed by turn ID in activityV2Store and rendered by
 * KnowledgeRecallChips next to MemoryRecallChips.
 */

export const KNOWLEDGE_RECALLED_NOTICE_TYPE = 'knowledge_recalled';

export interface KnowledgeRecallChunk {
  chunk_id: string;
  doc_id?: string;
  score: number;
  line: string;
}

function isValidChunk(v: unknown): v is KnowledgeRecallChunk {
  if (!v || typeof v !== 'object') return false;
  const h = v as Record<string, unknown>;
  return typeof h.chunk_id === 'string' && h.chunk_id.trim() !== '';
}

/**
 * parseKnowledgeRecallChunks extracts chunks from a knowledge_recalled notice
 * step Content (JSON string). Returns [] on empty/invalid content.
 */
export function parseKnowledgeRecallChunks(content: string): KnowledgeRecallChunk[] {
  const raw = (content ?? '').trim();
  if (!raw) return [];
  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch {
    return [];
  }
  const chunks = (payload as { chunks?: unknown })?.chunks;
  if (!Array.isArray(chunks)) return [];
  return chunks.filter(isValidChunk).map((h) => ({
    chunk_id: h.chunk_id,
    line: typeof h.line === 'string' ? h.line : '',
    score: typeof h.score === 'number' ? h.score : 0,
    ...(typeof h.doc_id === 'string' && h.doc_id.trim() ? { doc_id: h.doc_id } : {}),
  }));
}

/** Merge later notices into the turn index; first occurrence of a chunk_id wins. */
export function mergeKnowledgeRecallChunks(
  existing: KnowledgeRecallChunk[],
  incoming: KnowledgeRecallChunk[],
): KnowledgeRecallChunk[] {
  if (incoming.length === 0) return existing;
  const seen = new Set(existing.map((c) => c.chunk_id));
  const out = existing.slice();
  for (const c of incoming) {
    if (seen.has(c.chunk_id)) continue;
    seen.add(c.chunk_id);
    out.push(c);
  }
  return out;
}
