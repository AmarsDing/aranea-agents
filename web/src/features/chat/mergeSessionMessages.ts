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
  // turn_index=0 means "unassigned" (in-flight). These must sort after all
  // persisted messages (turn_index >= 1) so they appear at the bottom.
  const ti = message.turn_index ?? 0;
  const sortTi = ti === 0 ? 9999 : ti;
  return [sortTi, messageTime(message)];
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
	if (id.startsWith("pending-user-")) return message.status !== "failed";
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

/** Extract request_id from a message's options_json. */
function messageRequestId(message: Message): string | undefined {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { request_id?: string };
    return raw.request_id;
  } catch {
    return undefined;
  }
}

/** Merge request_id from a pending-user placeholder into the server message that replaces it. */
function transferRequestId(pending: Message, serverMsg: Message): Message {
  // For pending-user-* rows, the request_id IS the row id itself.
  // For other rows, extract from options_json.
  const rid = pending.id.startsWith("pending-user-")
    ? pending.id
    : messageRequestId(pending);
  if (!rid) return serverMsg;
  // If server message already has request_id, keep it
  if (messageRequestId(serverMsg)) return serverMsg;
  try {
    const opts = JSON.parse(serverMsg.options_json || "{}") as Record<string, unknown>;
    opts.request_id = rid;
    return { ...serverMsg, options_json: JSON.stringify(opts) };
  } catch {
    return serverMsg;
  }
}

/** Match a pending-user placeholder to a server-persisted user message by content. */
function isPendingUserMatch(pending: Message, serverUser: Message): boolean {
  if (!pending.id.startsWith("pending-user-")) return false;
  if (pending.role !== "user" || serverUser.role !== "user") return false;
  if (pending.session_id !== serverUser.session_id) return false;
  return pending.content_markdown === serverUser.content_markdown;
}

/** Build a map from (sessionId, role=user, content) → server message for fast lookup. */
function buildServerUserContentMap(server: Message[]): Map<string, Message> {
  const map = new Map<string, Message>();
  for (const m of server) {
    if (m.role !== "user") continue;
    const key = `${m.session_id}::${m.content_markdown}`;
    if (!map.has(key)) map.set(key, m);
  }
  return map;
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
  const serverUserByContent = buildServerUserContentMap(normalizedServer);

  // Build a mapping: pending-user- id → server message that replaces it
  const pendingReplacedBy = new Map<string, Message>();
  for (const row of local) {
    if (!row.id.startsWith("pending-user-") || row.role !== "user") continue;
    if (row.status === "failed") continue; // keep failed placeholders
    const key = `${row.session_id}::${row.content_markdown}`;
    const serverMatch = serverUserByContent.get(key);
    if (serverMatch) {
      // Transfer request_id from pending-user to server message so that
      // groupMessagesByTurn can still associate ws-stream rows via request_id
      // after the pending-user placeholder is replaced.
      const enriched = transferRequestId(row, serverMatch);
      pendingReplacedBy.set(row.id, enriched);
    }
  }

  // Replace server messages with their enriched versions (carrying request_id)
  const enrichedServer = normalizedServer.map((m) => {
    for (const [, replacement] of pendingReplacedBy) {
      if (replacement.id === m.id) return replacement;
    }
    return m;
  });

  const merged = [...enrichedServer];
  for (const row of local) {
    // If this pending-user row has a server replacement, skip it (already in merged via server)
    if (pendingReplacedBy.has(row.id)) continue;
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
