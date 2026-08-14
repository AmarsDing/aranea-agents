// web/src/features/chat/noticeFilter.ts
/**
 * System-internal NoticeType values that should NOT be rendered as user-visible
 * NoticeBlock in the chat stream. These are system metrics/control events that
 * only have side effects (e.g. updating session token usage) and would clutter
 * the conversation if displayed.
 *
 * Backend emits these via ActivityProjector.EmitSystemEvent — when meta lacks
 * a "type" key, the content string itself becomes the NoticeType (see
 * internal/agent/v2/projector.go EmitSystemEvent).
 */
export const SYSTEM_NOTICE_TYPES: ReadonlySet<string> = new Set([
  'context_usage',
  'context_window',
  'metrics_updated',
  'token_usage',
  // R4 recall transparency: the notice Content is a machine-readable JSON
  // payload ({"hits":[...]}), rendered by MemoryRecallChips at the top of the
  // turn — never as a raw NoticeBlock.
  'memory_recalled',
  // 29-token P2-2: knowledge retrieval transparency ({"chunks":[...]}), the
  // knowledge-side counterpart of memory_recalled — machine payload for the
  // citation backfill loop, never rendered as a raw NoticeBlock.
  'knowledge_recalled',
]);

/**
 * Returns true if the step should be hidden from the chat stream because it is
 * a system-internal notice (context_usage, metrics, etc.).
 */
export function isSystemInternalNotice(kind: string, noticeType?: string): boolean {
  if (kind !== 'notice') return false;
  if (!noticeType) return false;
  return SYSTEM_NOTICE_TYPES.has(noticeType);
}
