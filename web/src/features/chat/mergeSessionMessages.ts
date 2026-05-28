import type { Message } from "./types";
import { normalizeServerMessageOptions } from "./streamContentPatch";
import { parseMessageOptions } from "./parseMessageOptions";
import { isInFlightStatus } from "../../domain/types";
import {
  ensureOrigin,
  isEphemeralOrigin,
  isInFlightOrigin,
  isPendingUserOrigin,
  isStreamingOrigin,
  isTeamMemberOrigin,
} from "./messageOrigin";

function normalizeMessage(m: Message): Message {
  const withOrigin = ensureOrigin(m);
  const normalized = { ...withOrigin, options_json: normalizeServerMessageOptions(withOrigin.options_json ?? "") };
  const parsed = parseMessageOptions(normalized.options_json);
  return { ...normalized, ...parsed };
}

function isEphemeralMessage(message: Message): boolean {
  return isEphemeralOrigin(message.origin);
}

/** Whether this row is a local-only in-flight message (not yet persisted by server). */
export function isInFlightLocalRow(message: Message): boolean {
  if (isEphemeralMessage(message)) return true;
  return isInFlightStatus(message.status || "");
}

function messageOrder(message: Message): [number, string] {
  const inFlight = isInFlightLocalRow(message) ? 1 : 0;
  return [inFlight, message.created_at || ""];
}

/** Drop optimistic user placeholders after the server turn completes. */
export function dropPendingUserPlaceholders(messages: Message[]): Message[] {
  return messages.filter((m) => !isPendingUserOrigin(m.origin));
}

function shouldDropStaleInFlight(message: Message): boolean {
  const origin = message.origin;
  if (isPendingUserOrigin(origin)) return message.status !== "failed";
  if (isStreamingOrigin(origin)) {
    return !isInFlightStatus(message.status || "");
  }
  if (isTeamMemberOrigin(origin)) return false;
  return isInFlightStatus(message.status || "");
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
  const normalizedIncremental = incremental.map(normalizeMessage);
  const persisted = local.filter((m) => !isInFlightLocalRow(m));
  const byId = new Map(persisted.map((m) => [m.id, m]));
  for (const row of normalizedIncremental) {
    byId.set(row.id, row);
  }
  return mergeSessionMessages([...byId.values()], local, opts);
}

/** Match a pending-user placeholder to a server-persisted user message by content. */
function isPendingUserMatch(pending: Message, serverUser: Message): boolean {
  if (!isPendingUserOrigin(pending.origin)) return false;
  if (pending.role !== "user" || serverUser.role !== "user") return false;
  if (pending.session_id !== serverUser.session_id) return false;
  return pending.content_markdown === serverUser.content_markdown;
}

/** Build a map from (sessionId, role=user, content) → server message for fast lookup. */
function buildServerUserContentMap(server: Message[]): Map<string, Message[]> {
  const map = new Map<string, Message[]>();
  for (const m of server) {
    if (m.role !== "user") continue;
    const key = `${m.session_id}::${m.content_markdown}`;
    const arr = map.get(key) ?? [];
    arr.push(m);
    map.set(key, arr);
  }
  return map;
}

function findServerMatchForPending(pending: Message, serverMap: Map<string, Message[]>, matched: Set<string>): Message | null {
  const key = `${pending.session_id}::${pending.content_markdown}`;
  const candidates = serverMap.get(key);
  if (!candidates) return null;
  for (const c of candidates) {
    if (!matched.has(c.id)) {
      matched.add(c.id);
      return c;
    }
  }
  return null;
}

/** Merge server history with in-flight WS rows (streaming text, running tool cards). */
export function mergeSessionMessages(
  server: Message[],
  local: Message[],
  opts?: { dropStaleInFlight?: boolean }
): Message[] {
  const normalizedServer = server.map(normalizeMessage);
  if (local.length === 0) return normalizedServer;
  const serverById = new Map(normalizedServer.map((m) => [m.id, m]));
  const serverUserByContent = buildServerUserContentMap(normalizedServer);

  const pendingReplacedBy = new Map<string, Message>();
  const matchedServerIDs = new Set<string>();
  for (const row of local) {
    if (!isPendingUserOrigin(row.origin) || row.role !== "user") continue;
    if (row.status === "failed") continue;
    const serverMatch = findServerMatchForPending(row, serverUserByContent, matchedServerIDs);
    if (serverMatch) {
      pendingReplacedBy.set(row.id, serverMatch);
    }
  }

  const merged = [...normalizedServer];
  for (const row of local) {
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
  if (message.tool_event) return true;
  try {
    const raw = JSON.parse(message.options_json || "{}") as { schema?: string; tool_event?: unknown };
    return raw.schema === "chat.activity/v1" || Boolean(raw.tool_event);
  } catch {
    return message.status.startsWith("tool_");
  }
}
