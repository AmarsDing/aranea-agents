import type { Message } from "./types";
import { normalizeServerMessageOptions } from "./streamContentPatch";

function isEphemeralMessage(message: Message): boolean {
  const id = message.id || "";
  return (
    id.startsWith("ws-stream-") ||
    id.startsWith("ws-team-stream-") ||
    id.startsWith("pending-user-") ||
    id.startsWith("member-")
  );
}

function messageTime(message: Message): string {
  return message.created_at || "";
}

function messageOrder(message: Message): [number, string] {
  return [message.turn_index ?? 0, messageTime(message)];
}

function isInFlightLocalRow(message: Message): boolean {
  if (isEphemeralMessage(message)) return true;
  const status = message.status || "";
  return status === "tool_running" || status === "streaming" || status === "tool_blocked";
}

/** Drop optimistic user placeholders after the server turn completes. */
export function dropPendingUserPlaceholders(messages: Message[]): Message[] {
  return messages.filter((m) => !String(m.id).startsWith("pending-user-"));
}

function shouldDropStaleInFlight(message: Message): boolean {
  const id = message.id || "";
  if (id.startsWith("pending-user-")) return true;
  if (id.startsWith("ws-stream-") || id.startsWith("ws-team-stream-")) {
    // Terminal ephemeral row is superseded by persisted server assistant message.
    return (message.status || "") !== "streaming";
  }
  if (id.startsWith("member-")) return false;
  const status = message.status || "";
  return status === "tool_running" || status === "tool_blocked";
}

/**
 * Merge incremental server rows into existing session state without dropping persisted history.
 * Used by `loadMessages({ afterRevision })` where the API returns only new messages.
 */
export function mergeIncrementalSessionMessages(
  incremental: Message[],
  local: Message[],
  opts?: { dropStaleInFlight?: boolean }
): Message[] {
  if (incremental.length === 0) return local;
  const normalizedIncremental = incremental.map((m) => ({
    ...m,
    options_json: normalizeServerMessageOptions(m.options_json ?? ""),
  }));
  const persisted = local.filter((m) => !isInFlightLocalRow(m));
  const byId = new Map(persisted.map((m) => [m.id, m]));
  for (const row of normalizedIncremental) {
    byId.set(row.id, row);
  }
  return mergeSessionMessages([...byId.values()], local, opts);
}

/** Merge server history with in-flight WS rows (streaming text, running tool cards). */
export function mergeSessionMessages(
  server: Message[],
  local: Message[],
  opts?: { dropStaleInFlight?: boolean }
): Message[] {
  const normalizedServer = server.map((m) => ({
    ...m,
    options_json: normalizeServerMessageOptions(m.options_json ?? ""),
  }));
  if (local.length === 0) return normalizedServer;
  const serverById = new Map(normalizedServer.map((m) => [m.id, m]));
  const merged = [...normalizedServer];
  for (const row of local) {
    if (serverById.has(row.id)) continue;
    if (!isInFlightLocalRow(row)) continue;
    if (opts?.dropStaleInFlight && shouldDropStaleInFlight(row)) continue;
    merged.push(row);
  }
  merged.sort((a, b) => {
    const [turnA, timeA] = messageOrder(a);
    const [turnB, timeB] = messageOrder(b);
    if (turnA !== turnB) return turnA - turnB;
    return timeA.localeCompare(timeB);
  });
  return merged;
}

export function isActivityMessage(message: Message): boolean {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { schema?: string; tool_event?: unknown };
    return raw.schema === "chat.activity/v1" || Boolean(raw.tool_event);
  } catch {
    return message.status.startsWith("tool_");
  }
}
