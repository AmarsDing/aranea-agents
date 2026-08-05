// web/src/features/chat/memoryRecall.ts
/**
 * memory_recalled notice payload (R4: chat-level recall transparency).
 *
 * Backend source of truth: internal/agent/memory_inject.go
 *   - memoryRecalledNoticeType = "memory_recalled"
 *   - memoryRecalledNoticePayload { hits: [...] } serialized into Step.Content
 *
 * The notice step (Kind=notice, NoticeType=memory_recalled) is hidden from the
 * chat stream (see noticeFilter.ts); hits are indexed by turn ID in
 * activityV2Store and rendered by MemoryRecallChips below the turn.
 */

/** NoticeType emitted by the backend memory inject hook. */
export const MEMORY_RECALLED_NOTICE_TYPE = 'memory_recalled';

/** One recall hit inside the memory_recalled notice payload. */
export interface MemoryRecallHit {
  /** Memory layer: L1 (session summary) / L2 (episode) / L3 (semantic fact) / L4 (entity). */
  layer: string;
  /** Human-readable memory line (backend-capped to 120 runes). */
  line: string;
  /** Recall score 0..1. */
  score: number;
  /** L3 fact ID (optional, present for fact hits). */
  fact_id?: string;
  /** Fact confidence 0..1 (optional). */
  confidence?: number;
  /** Fact version (optional). */
  version?: number;
}

function isValidHit(v: unknown): v is MemoryRecallHit {
  if (!v || typeof v !== 'object') return false;
  const h = v as Record<string, unknown>;
  return typeof h.layer === 'string' && typeof h.line === 'string' && h.line.trim() !== '';
}

/**
 * parseMemoryRecallHits extracts hits from a memory_recalled notice step
 * Content (JSON string). Returns [] on empty/invalid content so callers can
 * treat parse failure as "no transparency data" without error handling.
 */
export function parseMemoryRecallHits(content: string): MemoryRecallHit[] {
  const raw = (content ?? '').trim();
  if (!raw) return [];
  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch {
    return [];
  }
  const hits = (payload as { hits?: unknown })?.hits;
  if (!Array.isArray(hits)) return [];
  return hits.filter(isValidHit).map((h) => ({
    layer: h.layer,
    line: h.line,
    score: typeof h.score === 'number' ? h.score : 0,
    ...(h.fact_id ? { fact_id: h.fact_id } : {}),
    ...(typeof h.confidence === 'number' ? { confidence: h.confidence } : {}),
    ...(typeof h.version === 'number' ? { version: h.version } : {}),
  }));
}
