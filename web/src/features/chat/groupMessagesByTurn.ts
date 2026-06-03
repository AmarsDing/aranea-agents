/**
 * Stack-based chat message grouping.
 *
 * Messages are sorted by time, then grouped into TurnBlocks:
 * - Grouping uses `turn_id` from the backend; messages sharing the same turn_id
 *   are guaranteed to belong to the same turn.
 * - If turn_id is missing or changes, a new block is started.
 * - Within a block, messages are distributed by role/origin into user/assistant/tools/members.
 *
 * The `turn_number` field is preserved for display/sorting but is NOT used for
 * grouping decisions — `turn_id` is the authoritative FK.
 */
import type { Message, ReactToolLinkIndex } from './types';
import { toolEventFromMessage } from './envelopeToolCall';
import { isActivityMessage, isInFlightLocalRow } from './mergeSessionMessages';
import { isToolLinkedInReactIndex } from './reactToolLinkIndex';
import { isTeamMemberOrigin, ensureOrigin } from './messageOrigin';

export type TurnBlockGroup = {
  /** Sequential block index (0-based). */
  key: number;
  turnId: string;
  user: Message | null;
  assistant: Message | null;
  tools: Message[];
  members: Message[];
};

export function isTeamMemberStreamMessage(message: Message): boolean {
  return isTeamMemberOrigin(message.origin);
}

/**
 * Sort comparator: persisted messages by created_at, in-flight rows last.
 * Within each group, preserve original array order as tiebreaker.
 */
function messageSortRank(message: Message, index: number): [number, string, number] {
  const inFlight = isInFlightLocalRow(message) ? 1 : 0;
  return [inFlight, message.created_at || '', index];
}

function getEffectiveTurnId(msg: Message): string {
  return msg.turn_id?.trim() || '';
}

function shouldStartNewBlock(current: TurnBlockGroup | null, msg: Message, effectiveTurnId: string): boolean {
  if (!current) return true;
  if (effectiveTurnId) return current.turnId !== effectiveTurnId;
  return msg.role === 'user';
}

/**
 * Derive TurnBlock[] from merged messages.
 *
 * Algorithm:
 * 1. Sort: persisted by created_at, in-flight at the end.
 * 2. Group by turn_id (authoritative FK from backend).
 * 3. Within each turn block, distribute messages by role/origin.
 * 4. Consolidate orphan tool-only blocks into previous block.
 */
export function groupMessagesByTurn(messages: Message[]): TurnBlockGroup[] {
  if (messages.length === 0) return [];

  // Sort: persisted first (by time), in-flight last (by time)
  const sorted = messages
    .map((m, i) => ({ m: ensureOrigin(m), i }))
    .sort((a, b) => {
      const ra = messageSortRank(a.m, a.i);
      const rb = messageSortRank(b.m, b.i);
      if (ra[0] !== rb[0]) return ra[0] - rb[0];
      if (ra[1] !== rb[1]) return ra[1].localeCompare(rb[1]);
      return ra[2] - rb[2];
    })
    .map((x) => x.m);

  const blocks: TurnBlockGroup[] = [];
  let current: TurnBlockGroup | null = null;
  let blockIndex = 0;

  for (const msg of sorted) {
    const effectiveTurnId = getEffectiveTurnId(msg);

    if (shouldStartNewBlock(current, msg, effectiveTurnId)) {
      current = {
        key: blockIndex++,
        turnId: effectiveTurnId || `__legacy_${blockIndex}`,
        user: null,
        assistant: null,
        tools: [],
        members: [],
      };
      blocks.push(current);
    }

    // Distribute into current block by role/origin
    if (msg.role === 'user') {
      current.user = msg;
    } else if (isTeamMemberStreamMessage(msg)) {
      current.members.push(msg);
    } else if (isActivityMessage(msg)) {
      current.tools.push(msg);
    } else if (msg.role === 'assistant') {
      current.assistant = msg;
    }
  }

  return consolidateOrphanToolBlocks(blocks);
}

/** Merge tool-only blocks (no user, no assistant) into the previous user turn. */
function consolidateOrphanToolBlocks(blocks: TurnBlockGroup[]): TurnBlockGroup[] {
  const out: TurnBlockGroup[] = [];
  for (const block of blocks) {
    const toolsOnly = !block.user && !block.assistant && block.tools.length > 0;
    if (toolsOnly && out.length > 0) {
      const prev = out[out.length - 1]!;
      prev.tools.push(...block.tools);
      continue;
    }
    if (toolsOnly) continue;
    out.push(block);
  }
  return out;
}

/** Index of last block with visible assistant body (scroll anchor). */
export function lastAssistantTurnBlockIndex(blocks: TurnBlockGroup[]): number {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]!;
    if (b.assistant && (b.assistant.content_markdown ?? '').trim()) return i;
    if (b.user && !b.assistant && b.tools.length > 0) return i;
  }
  return Math.max(0, blocks.length - 1);
}

export function toolStripSummary(tools: Message[]): {
  count: number;
  failed: number;
  totalMs: number;
} {
  let failed = 0;
  let totalMs = 0;
  for (const t of tools) {
    if (t.status === 'tool_failed') failed++;
    totalMs += t.latency_ms ?? 0;
  }
  return { count: tools.length, failed, totalMs };
}

/** CC-C-UX-03: omit tools already rendered under ReAct ACTION in the assistant row. */
export function filterToolsForToolStrip(tools: Message[], reactLinkIndex: ReactToolLinkIndex): Message[] {
  if (!tools.length) return tools;
  return tools.filter((tool) => {
    if (!isActivityMessage(tool)) return true;
    const ev = toolEventFromMessage(tool);
    if (!ev?.id) return true;
    return !isToolLinkedInReactIndex(reactLinkIndex, ev.id);
  });
}
