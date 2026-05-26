/**
 * Ephemeral WS row turn_index alignment for TurnBlock grouping.
 *
 * Backend convention: user messages use odd turn_index (1, 3, 5…); assistant replies use even (2, 4, 6…).
 * Tool activity rows share the user turn's odd index. In-flight rows (ws-stream-*, tool_running) must
 * follow the latest user turn or TurnBlock splits channel inbound UI from the active Feishu turn.
 */
import { isActivityMessage } from "./mergeSessionMessages";
import type { Message } from "./types";

/** Highest user turn_index in session history (defaults to 1 for the first turn). */
export function lastUserTurnIndex(messages: Message[]): number {
  let max = 0;
  for (const row of messages) {
    if (row.role !== "user") continue;
    const ti = row.turn_index ?? 0;
    if (ti > max) max = ti;
  }
  return max > 0 ? max : 1;
}

/** Even turn_index for in-flight assistant stream rows (follows latest user turn). */
export function inferAssistantStreamTurnIndex(messages: Message[]): number {
  const userTi = lastUserTurnIndex(messages);
  return userTi % 2 === 1 ? userTi + 1 : userTi + 2;
}

/** Odd turn_index for in-flight tool activity rows (same user turn as ACTION). */
export function inferToolActivityTurnIndex(messages: Message[]): number {
  const userTi = lastUserTurnIndex(messages);
  return userTi % 2 === 1 ? userTi : Math.max(1, userTi - 1);
}

function isEphemeralStreamRow(message: Message): boolean {
  const id = message.id || "";
  return id.startsWith("ws-stream-") || id.startsWith("ws-team-stream-");
}

function isInFlightToolRow(message: Message): boolean {
  if (!isActivityMessage(message)) return false;
  const status = message.status || "";
  return status === "tool_running" || status === "tool_blocked";
}

/**
 * Re-attach ephemeral WS rows to the latest user turn after history reload or channel focus.
 */
export function realignEphemeralTurnIndexes(messages: Message[]): Message[] {
  if (messages.length === 0) return messages;
  const assistantTi = inferAssistantStreamTurnIndex(messages);
  const toolTi = inferToolActivityTurnIndex(messages);
  let changed = false;
  const next = messages.map((row) => {
    if (isEphemeralStreamRow(row) && row.status === "streaming") {
      if (row.turn_index !== assistantTi) {
        changed = true;
        return { ...row, turn_index: assistantTi };
      }
      return row;
    }
    if (isInFlightToolRow(row) && row.turn_index !== toolTi) {
      changed = true;
      return { ...row, turn_index: toolTi };
    }
    return row;
  });
  return changed ? next : messages;
}
