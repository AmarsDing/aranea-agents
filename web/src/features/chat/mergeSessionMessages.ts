import type { Message } from './types';
import { normalizeServerMessageOptions } from './streamContentPatch';
import { parseMessageOptions } from './parseMessageOptions';
import { isInFlightStatus } from '../../domain/types';

function normalizeMessage(m: Message): Message {
  const normalized = { ...m, options_json: normalizeServerMessageOptions(m.options_json ?? '') };
  const parsed = parseMessageOptions(normalized.options_json);
  return { ...normalized, ...parsed };
}

function isPendingUserRow(message: Message): boolean {
  return message.id.startsWith('pending-user-');
}

/** Whether this row is an activity-derived tool / execution card. */
function isActivityDerivedRow(message: Message): boolean {
  return Boolean(message.tool_event);
}

/** Whether this row is a local-only row that should be preserved during merge
 * when the server does not yet have a matching persisted message. */
export function isInFlightLocalRow(message: Message): boolean {
  if (isPendingUserRow(message)) return true;
  if (isInFlightStatus(message.status || '')) return true;
  if (isActivityDerivedRow(message)) return true;
  return false;
}

function messageOrder(message: Message): [number, string] {
  const inFlight = isInFlightLocalRow(message) ? 1 : 0;
  return [inFlight, message.created_at || ''];
}

/**
 * Compare two ISO 8601 timestamps numerically via Date.parse.
 * Falls back to string comparison when either timestamp is unparseable,
 * ensuring stable ordering even with mixed precision (seconds / milliseconds / microseconds).
 */
function compareTimestamps(a: string, b: string): number {
  if (!a && !b) return 0;
  if (!a) return -1;
  if (!b) return 1;
  const ta = Date.parse(a);
  const tb = Date.parse(b);
  if (Number.isNaN(ta) || Number.isNaN(tb)) {
    // Fallback: string comparison for non-standard formats
    return a < b ? -1 : a > b ? 1 : 0;
  }
  if (ta !== tb) return ta - tb;
  // Same millisecond — compare sub-millisecond precision via string to preserve
  // microsecond ordering (e.g. addMicroOffset timestamps).
  return a < b ? -1 : a > b ? 1 : 0;
}

/** Drop optimistic user placeholders after the server turn completes. */
export function dropPendingUserPlaceholders(messages: Message[]): Message[] {
  return messages.filter((m) => !isPendingUserRow(m));
}

function shouldDropStaleInFlight(message: Message): boolean {
  if (isPendingUserRow(message)) return message.status !== 'failed';
  // Team member messages are rendered from Activity events; never drop them here.
  if (message.team_member) return false;
  // Activity-derived tool cards represent completed work and should survive a reconnect.
  if (isActivityDerivedRow(message)) return false;
  const status = message.status || '';
  // Stale tool states are dropped on reconnect; streaming text is kept because
  // the user is actively watching the LLM reply and a reconnect should not blink it.
  if (status === 'tool_running' || status === 'tool_blocked') return true;
  return !isInFlightStatus(status);
}

/**
 * Merge incremental server rows into existing session state without dropping persisted history.
 * Used by `loadMessages({ afterRevision })` where the API returns only new messages.
 */
export function mergeIncrementalSessionMessages(
  incremental: Message[],
  local: Message[],
  opts?: { dropStaleInFlight?: boolean },
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

/** Build a map from (sessionId, role=user, content) → server message for fast lookup. */
function buildServerUserContentMap(server: Message[]): Map<string, Message[]> {
  const map = new Map<string, Message[]>();
  for (const m of server) {
    if (m.role !== 'user') continue;
    const key = `${m.session_id}::${m.content_markdown}`;
    const arr = map.get(key) ?? [];
    arr.push(m);
    map.set(key, arr);
  }
  return map;
}

function findServerMatchForPending(
  pending: Message,
  serverMap: Map<string, Message[]>,
  matched: Set<string>,
): Message | null {
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

/** Merge server history with local-only rows (pending placeholders, streaming text, activity-derived cards). */
export function mergeSessionMessages(
  server: Message[],
  local: Message[],
  opts?: { dropStaleInFlight?: boolean },
): Message[] {
  const normalizedServer = server.map(normalizeMessage);
  const normalizedLocal = local.map(normalizeMessage);
  if (normalizedLocal.length === 0) return normalizedServer;
  const serverById = new Map(normalizedServer.map((m) => [m.id, m]));
  const serverUserByContent = buildServerUserContentMap(normalizedServer);

  const pendingReplacedBy = new Map<string, Message>();
  const matchedServerIDs = new Set<string>();
  for (const row of normalizedLocal) {
    if (!isPendingUserRow(row) || row.role !== 'user') continue;
    if (row.status === 'failed') continue;
    const serverMatch = findServerMatchForPending(row, serverUserByContent, matchedServerIDs);
    if (serverMatch) {
      pendingReplacedBy.set(row.id, serverMatch);
    }
  }

  const merged: Message[] = [];
  for (const srv of normalizedServer) {
    merged.push(srv);
  }
  for (const row of normalizedLocal) {
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
    return compareTimestamps(timeA, timeB);
  });
  return merged;
}
