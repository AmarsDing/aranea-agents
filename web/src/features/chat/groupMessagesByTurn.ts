import type { Message } from "./types";
import { isActivityMessage } from "./mergeSessionMessages";

export type TurnBlockGroup = {
  /** User message turn_index (odd); grouping key */
  key: number;
  turnId: string;
  user: Message | null;
  assistant: Message | null;
  tools: Message[];
  members: Message[];
};

export function isTeamMemberStreamMessage(message: Message): boolean {
  return String(message.id).startsWith("member-");
}

/** Map any message row to its user-turn grouping key. */
export function deriveTurnKey(message: Message): number {
  const ti = message.turn_index ?? 0;
  if (isTeamMemberStreamMessage(message)) return ti;
  if (isActivityMessage(message)) {
    return ti % 2 === 0 ? Math.max(0, ti - 1) : ti;
  }
  if (message.role === "assistant" && ti > 0 && ti % 2 === 0) {
    return ti - 1;
  }
  if (message.role === "user") return ti;
  return ti % 2 === 0 ? Math.max(0, ti - 1) : ti;
}

/**
 * Derive TurnBlock[] from merged messages (after mergeSessionMessages).
 * Team member stream rows stay nested under their turn block.
 */
export function groupMessagesByTurn(messages: Message[]): TurnBlockGroup[] {
  const map = new Map<number, TurnBlockGroup>();

  for (const msg of messages) {
    const key = deriveTurnKey(msg);
    let block = map.get(key);
    if (!block) {
      block = {
        key,
        turnId: msg.role === "user" ? msg.id : `turn-${key}`,
        user: null,
        assistant: null,
        tools: [],
        members: [],
      };
      map.set(key, block);
    }
    if (isTeamMemberStreamMessage(msg)) {
      block.members.push(msg);
      continue;
    }
    if (isActivityMessage(msg)) {
      block.tools.push(msg);
      continue;
    }
    if (msg.role === "user") {
      block.user = msg;
      block.turnId = msg.id;
    } else if (msg.role === "assistant") {
      block.assistant = msg;
    }
  }

  return consolidateOrphanToolBlocks([...map.values()].sort((a, b) => a.key - b.key));
}

/** Merge tool-only blocks (failed/partial turns) into the previous user turn. */
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
    if (b.assistant && (b.assistant.content_markdown ?? "").trim()) return i;
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
    if (t.status === "tool_failed") failed++;
    totalMs += t.latency_ms ?? 0;
  }
  return { count: tools.length, failed, totalMs };
}
