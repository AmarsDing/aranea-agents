import type { Message, ReactToolLinkIndex } from "./types";
import { toolEventFromMessage } from "./envelopeToolCall";
import { isActivityMessage } from "./mergeSessionMessages";
import { isToolLinkedInReactIndex } from "./reactToolLinkIndex";

export type TurnBlockGroup = {
  /** User message turn_index (odd); grouping key. 0 for in-flight request_id groups. */
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

/** Extract request_id from a message's options_json (stored by patchStreamingEnvelope). */
function messageRequestId(message: Message): string | undefined {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { request_id?: string };
    return raw.request_id;
  } catch {
    return undefined;
  }
}

/** Whether this message is an in-flight placeholder with no server-assigned turn_index. */
function isInFlightPlaceholder(message: Message): boolean {
  return (message.turn_index ?? 0) === 0;
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
 *
 * In-flight messages (turn_index=0) are grouped by request_id:
 * pending-user-{rid} and ws-stream rows carrying the same request_id
 * are placed in the same TurnBlock, appended after all persisted turns.
 */
export function groupMessagesByTurn(messages: Message[]): TurnBlockGroup[] {
  // Build request_id → pending-user message index for in-flight association
  const pendingUserByRequestId = new Map<string, number>();
  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i]!;
    if (msg.id.startsWith("pending-user-") && isInFlightPlaceholder(msg)) {
      // The request_id IS the pending-user id (it was set as pendingUserId in useChatSender)
      pendingUserByRequestId.set(msg.id, i);
    }
  }

  // Also build request_id → persisted user message index, so that ws-stream rows
  // whose pending-user was already replaced by a server message can still be
  // grouped correctly via request_id stored in the server message's options_json.
  const persistedUserByRequestId = new Map<string, number>();
  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i]!;
    if (!msg.id.startsWith("pending-user-") && msg.role === "user" && !isInFlightPlaceholder(msg)) {
      const rid = messageRequestId(msg);
      if (rid) persistedUserByRequestId.set(rid, i);
    }
  }

  const map = new Map<number, TurnBlockGroup>();
  // Separate map for in-flight request_id groups (keyed by a synthetic negative number)
  const inflightMap = new Map<string, TurnBlockGroup>();
  let inflightCounter = -1;

  for (const msg of messages) {
    // In-flight messages with turn_index=0: group by request_id
    if (isInFlightPlaceholder(msg) && !isTeamMemberStreamMessage(msg)) {
      const requestId = msg.id.startsWith("pending-user-")
        ? msg.id
        : messageRequestId(msg);

      // Case 1: pending-user still exists → group into inflightMap
      if (requestId && pendingUserByRequestId.has(requestId)) {
        let block = inflightMap.get(requestId);
        if (!block) {
          block = {
            key: inflightCounter--,
            turnId: requestId,
            user: null,
            assistant: null,
            tools: [],
            members: [],
          };
          inflightMap.set(requestId, block);
        }
        if (isActivityMessage(msg)) {
          block.tools.push(msg);
        } else if (msg.role === "user") {
          block.user = msg;
          block.turnId = msg.id;
        } else if (msg.role === "assistant") {
          block.assistant = msg;
        }
        continue;
      }

      // Case 2: pending-user was replaced by server message → attach to that
      // persisted user's TurnBlock via request_id stored in the server message.
      if (requestId && persistedUserByRequestId.has(requestId)) {
        const serverIdx = persistedUserByRequestId.get(requestId)!;
        const serverMsg = messages[serverIdx]!;
        const key = deriveTurnKey(serverMsg);
        let block = map.get(key);
        if (!block) {
          block = {
            key,
            turnId: serverMsg.id,
            user: serverMsg,
            assistant: null,
            tools: [],
            members: [],
          };
          map.set(key, block);
        }
        // Attach the in-flight ws-stream/tool row to this persisted block
        if (isActivityMessage(msg)) {
          block.tools.push(msg);
        } else if (msg.role === "assistant" && !block.assistant) {
          block.assistant = msg;
        }
        continue;
      }
    }

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

  const persisted = [...map.values()].sort((a, b) => a.key - b.key);
  const inflight = [...inflightMap.values()];
  // In-flight blocks appear after all persisted blocks
  return consolidateOrphanToolBlocks([...persisted, ...inflight]);
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

/** CC-C-UX-03: omit tools already rendered under ReAct ACTION in the assistant row. */
export function filterToolsForToolStrip(
  tools: Message[],
  reactLinkIndex: ReactToolLinkIndex
): Message[] {
  if (!tools.length) return tools;
  return tools.filter((tool) => {
    if (!isActivityMessage(tool)) return true;
    const ev = toolEventFromMessage(tool);
    if (!ev?.id) return true;
    return !isToolLinkedInReactIndex(reactLinkIndex, ev.id);
  });
}
