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
 * activityV2Store and rendered by MemoryRecallChips at the top of the turn
 * (recall happens at BeforeModel — before thinking/reply steps).
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

export type MemoryCenterRouteQuery = {
  tab?: string;
  layer?: string;
  factId?: string;
  agentId?: string;
  agentKey?: string;
  sessionId?: string;
  q?: string;
};

/** Builds the Memory Center deep-link consumed by MemoryCenterPage (FR-T1). */
export function memoryCenterRoute(opts: MemoryCenterRouteQuery): {
  path: string;
  query: Record<string, string>;
} {
  const query: Record<string, string> = {};
  const assign = (key: keyof MemoryCenterRouteQuery) => {
    const v = opts[key]?.trim();
    if (v) query[key] = v;
  };
  assign('tab');
  assign('layer');
  assign('factId');
  assign('agentId');
  assign('agentKey');
  assign('sessionId');
  assign('q');
  return { path: '/memory', query };
}

/** Maps a recall hit + current turn identity to a Memory Center route. */
export function memoryCenterRouteFromHit(
  hit: MemoryRecallHit,
  ctx: { sessionId?: string; agentId?: string; agentKey?: string },
): { path: string; query: Record<string, string> } {
  const layer = (hit.layer || '').trim().toUpperCase();
  const base = {
    sessionId: ctx.sessionId,
    agentId: ctx.agentId,
    agentKey: ctx.agentKey,
  };
  if (layer === 'L4') {
    return memoryCenterRoute({ tab: 'graph', layer: 'L4', ...base });
  }
  if (layer === 'L3' && hit.fact_id) {
    return memoryCenterRoute({ tab: 'browse', layer: 'L3', factId: hit.fact_id, ...base });
  }
  if (layer === 'L3') {
    return memoryCenterRoute({ tab: 'browse', layer: 'L3', q: hit.line, ...base });
  }
  if (layer === 'L0' || layer === 'L1' || layer === 'L2') {
    return memoryCenterRoute({ tab: 'browse', layer, ...base });
  }
  return memoryCenterRoute({ tab: 'browse', ...base });
}
