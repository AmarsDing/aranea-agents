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

/** Whether this row is a local-only in-flight message (not yet persisted by server). */
export function isInFlightLocalRow(message: Message): boolean {
  if (isEphemeralMessage(message)) return true;
  const status = message.status || "";
  return status === "tool_running" || status === "streaming" || status === "tool_blocked";
}

function messageOrder(message: Message): [number, string] {
  // In-flight rows sort after persisted rows; within each group, sort by time.
  const inFlight = isInFlightLocalRow(message) ? 1 : 0;
  return [inFlight, message.created_at || ""];
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
      pendingReplacedBy.set(row.id, serverMatch);
    }
  }

  const merged = [...normalizedServer];
  for (const row of local) {
    // If this pending-user row has a server replacement, skip it (already in merged via server)
    if (pendingReplacedBy.has(row.id)) continue;
    if (serverById.has(row.id)) continue;
    if (!isInFlightLocalRow(row)) continue;
    if (opts?.dropStaleInFlight && shouldDropStaleInFlight(row)) continue;
    merged.push(row);
  }
  merged.sort((a, b) => {
    const [inFlightA, timeA] = messageOrder(a);
    const [inFlightB, timeB] = messageOrder(b);
    if (inFlightA !== inFlightB) return inFlightA - inFlightB;
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
