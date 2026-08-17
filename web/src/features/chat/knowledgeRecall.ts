// web/src/features/chat/knowledgeRecall.ts
/**
 * knowledge_recalled notice payload (citation transparency).
 *
 * Backend source of truth: internal/tools/knowledge/tool.go
 *   - knowledgeRecalledNoticeType = "knowledge_recalled"
 *   - payload { chunks: [{ n?, chunk_id, doc_id, score, line }] }
 * Pre-retrieval numbered notices set `n` to match cue [n]; tool-path notices omit n.
 *
 * The notice step is hidden from the chat stream (see noticeFilter.ts);
 * chunks are indexed by turn ID in activityV2Store and rendered by
 * KnowledgeRecallChips next to MemoryRecallChips. Reply markdown [n]
 * is rewritten to clickable footnotes by linkKnowledgeCitations.
 */

export const KNOWLEDGE_RECALLED_NOTICE_TYPE = 'knowledge_recalled';

export interface KnowledgeRecallChunk {
  chunk_id: string;
  doc_id?: string;
  score: number;
  line: string;
  /** 1-based [n] aligned with ## Retrieved Knowledge. */
  n?: number;
}

function isValidChunk(v: unknown): v is KnowledgeRecallChunk {
  if (!v || typeof v !== 'object') return false;
  const h = v as Record<string, unknown>;
  return typeof h.chunk_id === 'string' && h.chunk_id.trim() !== '';
}

function parseN(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isInteger(v) && v >= 1 ? v : undefined;
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
  return chunks.filter(isValidChunk).map((h) => {
    const n = parseN((h as { n?: unknown }).n);
    return {
      chunk_id: h.chunk_id,
      line: typeof h.line === 'string' ? h.line : '',
      score: typeof h.score === 'number' ? h.score : 0,
      ...(typeof h.doc_id === 'string' && h.doc_id.trim() ? { doc_id: h.doc_id } : {}),
      ...(n !== undefined ? { n } : {}),
    };
  });
}

/** Fill missing / colliding n so chips and reply footnotes share one sequence. */
export function assignCitationNumbers(chunks: KnowledgeRecallChunk[]): KnowledgeRecallChunk[] {
  const used = new Set<number>();
  const out = chunks.map((c) => {
    const n = typeof c.n === 'number' && c.n >= 1 && !used.has(c.n) ? c.n : 0;
    if (n) used.add(n);
    return n ? { ...c, n } : { ...c, n: undefined };
  });
  let next = 1;
  for (const c of out) {
    if (c.n) continue;
    while (used.has(next)) next += 1;
    c.n = next;
    used.add(next);
    next += 1;
  }
  return out;
}

/** Merge later notices into the turn index; first occurrence of a chunk_id wins. */
export function mergeKnowledgeRecallChunks(
  existing: KnowledgeRecallChunk[],
  incoming: KnowledgeRecallChunk[],
): KnowledgeRecallChunk[] {
  if (incoming.length === 0) return assignCitationNumbers(existing);
  const seen = new Set(existing.map((c) => c.chunk_id));
  const out = existing.slice();
  for (const c of incoming) {
    if (seen.has(c.chunk_id)) continue;
    seen.add(c.chunk_id);
    out.push(c);
  }
  return assignCitationNumbers(out);
}

function escapeAttr(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
}

/**
 * Turn leftover `[n]` in sanitized markdown HTML into footnote buttons.
 * Skips pre/code/a/button so code samples and existing links stay intact.
 */
export function linkKnowledgeCitations(html: string, chunks: KnowledgeRecallChunk[]): string {
  if (!html) return html;
  const byN = new Map<number, KnowledgeRecallChunk>();
  for (const c of chunks) {
    if (typeof c.n === 'number' && c.n >= 1 && !byN.has(c.n)) byN.set(c.n, c);
  }
  if (byN.size === 0) return html;
  return html.replace(
    /(<pre[\s\S]*?<\/pre>|<code[\s\S]*?<\/code>|<a[\s\S]*?<\/a>|<button[\s\S]*?<\/button>)|\[(\d+)\]/gi,
    (all, skip: string | undefined, nRaw: string | undefined) => {
      if (skip) return skip;
      const n = Number(nRaw);
      const hit = byN.get(n);
      if (!hit) return all;
      const doc = hit.doc_id ? ` data-doc-id="${escapeAttr(hit.doc_id)}"` : '';
      return `<button type="button" class="kb-cite" data-n="${n}"${doc}>[${n}]</button>`;
    },
  );
}

export function knowledgeDocRoute(docId: string): { path: string; query: { doc: string } } {
  return { path: '/knowledge', query: { doc: docId } };
}
