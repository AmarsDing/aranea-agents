/**
 * Stack-based chat message grouping.
 *
 * Messages are sorted by time, then grouped into TurnBlocks:
 * - Grouping uses `turn_id` from the backend; messages sharing the same turn_id
 *   are guaranteed to belong to the same turn.
 * - If turn_id is missing or changes, a new block is started.
 * - Within a block, messages are distributed by role/origin into user/assistants/tools/members.
 * - Multi-round ReAct loops produce multiple assistant messages (ws-snap-*) per turn;
 *   each round pairs an assistant message with its subsequent tool calls.
 *
 * The `turn_number` field is preserved for display/sorting but is NOT used for
 * grouping decisions — `turn_id` is the authoritative FK.
 */
import type { Message, ReactToolLinkIndex } from './types';
import { toolEventFromMessage } from './envelopeToolCall';
import { isActivityMessage, isInFlightLocalRow } from './mergeSessionMessages';
import { isToolLinkedInReactIndex } from './reactToolLinkIndex';
import { isTeamMemberOrigin, ensureOrigin } from './messageOrigin';

/** A single LLM round within a turn: assistant thinking/reply + subsequent tool calls. */
export type TurnRound = {
  /** The assistant message for this round (ws-snap-* or ws-stream-* or persisted). */
  assistant: Message;
  /** Tool messages called after this assistant, before the next assistant in the same turn. */
  tools: Message[];
};

export type TurnBlockGroup = {
  /** Sequential block index (0-based). */
  key: number;
  turnId: string;
  user: Message | null;
  /** All assistant messages in chronological order (replaces single `assistant`). */
  assistants: Message[];
  /** Pre-computed rounds: each round pairs an assistant with its subsequent tools. */
  rounds: TurnRound[];
  /** All tool/activity messages (flat, for backward compat and strip summary). */
  tools: Message[];
  members: Message[];
  /** true when all tools completed and last assistant message arrived — eligible for auto-collapse. */
  isCompleted: boolean;
};

/** Convenience: get the last assistant message from a block. */
export function lastAssistant(block: TurnBlockGroup): Message | null {
  return block.assistants.length > 0 ? block.assistants[block.assistants.length - 1]! : null;
}

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
 * 4. Build rounds: each assistant message starts a new round; subsequent tools
 *    belong to that round until the next assistant message.
 * 5. Consolidate orphan tool-only blocks into previous block.
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
  let currentRound: TurnRound | null = null;

  function closeRound() {
    if (currentRound && current) {
      current.rounds.push(currentRound);
      currentRound = null;
    }
  }

  for (const msg of sorted) {
    const effectiveTurnId = getEffectiveTurnId(msg);

    if (shouldStartNewBlock(current, msg, effectiveTurnId)) {
      closeRound();
      current = {
        key: blockIndex++,
        turnId: effectiveTurnId || `__legacy_${blockIndex}`,
        user: null,
        assistants: [],
        rounds: [],
        tools: [],
        members: [],
        isCompleted: false,
      };
      blocks.push(current);
      currentRound = null;
    }

    // Distribute into current block by role/origin
    if (msg.role === 'user') {
      current!.user = msg;
    } else if (isTeamMemberStreamMessage(msg)) {
      current!.members.push(msg);
    } else if (msg.role === 'assistant' && !isActivityMessage(msg)) {
      // Real assistant message (including ws-snap-* streaming snapshots).
      // Close previous round, start a new one.
      closeRound();
      currentRound = { assistant: msg, tools: [] };
      current!.assistants.push(msg);
    } else if (isActivityMessage(msg)) {
      // Tool/activity message — add to flat tools and current round.
      current!.tools.push(msg);
      if (currentRound) {
        currentRound.tools.push(msg);
      }
    }
  }

  // Close last round
  closeRound();

  const result = consolidateOrphanToolBlocks(blocks);
  for (const block of result) {
    block.isCompleted = computeBlockCompleted(block);
  }
  return result;
}

/** Check whether a block is completed — all tools done and last assistant arrived. */
function computeBlockCompleted(block: TurnBlockGroup): boolean {
  const last = lastAssistant(block);
  if (!last || last.status === 'streaming') return false;
  if (block.tools.length === 0) return true;
  const completedStatuses: ReadonlySet<string> = new Set(['success', 'failed', 'cancelled']);
  return block.tools.every((t) => {
    const status = toolEventFromMessage(t)?.status;
    return status != null && completedStatuses.has(status);
  });
}

/** Merge tool-only blocks (no user, no assistants) into the previous user turn. */
function consolidateOrphanToolBlocks(blocks: TurnBlockGroup[]): TurnBlockGroup[] {
  const out: TurnBlockGroup[] = [];
  for (const block of blocks) {
    const toolsOnly = !block.user && block.assistants.length === 0 && block.tools.length > 0;
    if (toolsOnly && out.length > 0) {
      const prev = out[out.length - 1]!;
      prev.tools.push(...block.tools);
      prev.rounds.push(...block.rounds);
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
    const last = lastAssistant(b);
    if (last && (last.content_markdown ?? '').trim()) return i;
    if (b.user && b.assistants.length === 0 && b.tools.length > 0) return i;
  }
  return Math.max(0, blocks.length - 1);
}

export function toolStripSummary(tools: Message[]): {
  count: number;
  failed: number;
  cancelled: number;
  totalMs: number;
} {
  let failed = 0;
  let cancelled = 0;
  let totalMs = 0;
  for (const t of tools) {
    if (t.status === 'tool_failed') failed++;
    if (t.status === 'tool_cancelled') cancelled++;
    totalMs += t.latency_ms ?? 0;
  }
  return { count: tools.length, failed, cancelled, totalMs };
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
