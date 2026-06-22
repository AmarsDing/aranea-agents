import type { Message } from './types';
import { normalizeServerMessageOptions } from './streamContentPatch';
import { parseMessageOptions } from './parseMessageOptions';
import { isInFlightStatus } from '../../domain/types';
import {
  ensureOrigin,
  isEphemeralOrigin,
  isPendingUserOrigin,
  isStreamingOrigin,
  isTeamMemberOrigin,
} from './messageOrigin';

function normalizeMessage(m: Message): Message {
  const withOrigin = ensureOrigin(m);
  const normalized = { ...withOrigin, options_json: normalizeServerMessageOptions(withOrigin.options_json ?? '') };
  const parsed = parseMessageOptions(normalized.options_json);
  return { ...normalized, ...parsed };
}

function isEphemeralMessage(message: Message): boolean {
  return isEphemeralOrigin(message.origin);
}

/** Whether this row is a local-only in-flight message (not yet persisted by server). */
export function isInFlightLocalRow(message: Message): boolean {
  // Finalized snapshots (streaming_snapshot origin) — treat as persisted for sorting.
  // T7.3b: Covers AF mode finalized messages (actv-* upgraded to streaming_snapshot
  // by finalizeStreamingMessageFromDone).
  if (message.origin?.kind === 'streaming_snapshot') return false;
  // Reconstructed action messages (tool_activity origin with terminal status)
  // are finalized snapshots from Activity API data — treat as persisted for sorting.
  if (message.origin?.kind === 'tool_activity' && !isInFlightStatus(message.status || '')) return false;
  if (isEphemeralMessage(message)) return true;
  return isInFlightStatus(message.status || '');
}

/**
 * Whether a local-only message should be included in the merge output.
 *
 * Messages fall into three categories for merge purposes:
 *   1. **persisted** — has a server counterpart, managed by server data
 *   2. **in-flight-active** — still streaming/running, must be preserved
 *   3. **af-finalized** — Activity-First mode messages (streaming_snapshot
 *      origin) that have been finalized. These carry per-round content
 *      from Activity events and must be preserved when
 *      excludeMergedAssistant=true, because the server's merged assistant
 *      ChatMessage is excluded.
 */
function shouldIncludeInMerge(message: Message, excludeMergedAssistant: boolean): boolean {
  // Streaming snapshots: finalized local snapshots carrying round-separated content
  if (message.origin?.kind === 'streaming_snapshot' && excludeMergedAssistant) return true;
  return isInFlightLocalRow(message);
}

function messageOrder(message: Message): [number, string] {
  // Finalized local messages (streaming_snapshot origin) sort alongside
  // persisted messages by timestamp, not pushed to the end with active in-flight.
  if (message.origin?.kind === 'streaming_snapshot') return [0, message.created_at || ''];
  // Reconstructed action messages (tool_activity origin with terminal status)
  // sort alongside persisted messages to maintain thinking → action → reply order.
  if (message.origin?.kind === 'tool_activity' && !isInFlightStatus(message.status || ''))
    return [0, message.created_at || ''];
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
  return messages.filter((m) => !isPendingUserOrigin(m.origin));
}

function shouldDropStaleInFlight(message: Message): boolean {
  const origin = message.origin;
  if (isPendingUserOrigin(origin)) return message.status !== 'failed';
  if (isStreamingOrigin(origin)) {
    // streaming_snapshot messages (AF mode finalized) represent correctly
    // separated LLM round content. They must NOT be dropped during merge,
    // because the server-persisted assistant message contains ALL rounds'
    // content merged into one, and dropping snapshots would lose the round
    // separation.
    if (origin?.kind === 'streaming_snapshot') return false;
    return !isInFlightStatus(message.status || '');
  }
  if (isTeamMemberOrigin(origin)) return false;
  return isInFlightStatus(message.status || '');
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

/** Merge server history with in-flight WS rows (streaming text, running tool cards). */
export function mergeSessionMessages(
  server: Message[],
  local: Message[],
  opts?: { dropStaleInFlight?: boolean },
): Message[] {
  const normalizedServer = server.map(normalizeMessage);
  if (local.length === 0) return normalizedServer;
  const serverById = new Map(normalizedServer.map((m) => [m.id, m]));
  const serverUserByContent = buildServerUserContentMap(normalizedServer);

  // Determine whether to exclude the server's merged assistant message.
  //
  // excludeMergedAssistant is only true when we actually have local
  // streaming_snapshot assistant messages to replace the server's merged
  // content. This is a safety guard: if no snapshots exist (e.g. AF API
  // failed and returned no data), falling back to the server's merged
  // message prevents a blank UI.
  //
  // T7.3b: Legacy ws-snap-* references removed. streaming_snapshot origin
  // now covers both AF mode finalized messages (actv-*) and any remaining
  // snapshot messages. The hasSnapshots guard is retained as an AF safety
  // fallback, not Legacy logic.
  //
  // Trigger for hasSnapshots:
  //   Activity-First mode: Activity events/data provide complete per-round
  //   content (thinking/reply/action). The server-persisted assistant
  //   ChatMessage merges ALL rounds into one — including it would duplicate
  //   Activity content and cause the UI to show a single merged block
  //   instead of correctly separated rounds.
  const hasSnapshots = local.some((m) => m.origin?.kind === 'streaming_snapshot' && m.role === 'assistant');
  const excludeMergedAssistant = hasSnapshots;

  const pendingReplacedBy = new Map<string, Message>();
  const matchedServerIDs = new Set<string>();
  for (const row of local) {
    if (!isPendingUserOrigin(row.origin) || row.role !== 'user') continue;
    if (row.status === 'failed') continue;
    const serverMatch = findServerMatchForPending(row, serverUserByContent, matchedServerIDs);
    if (serverMatch) {
      pendingReplacedBy.set(row.id, serverMatch);
    }
  }

  // When excluding merged assistant messages, identify server assistant
  // messages that are NOT activity messages (tool cards). These "main"
  // assistant messages contain ALL rounds merged into one — including them
  // would duplicate Activity/snapshot content.
  //
  // Tool activity messages (act-*, schema=chat.activity/v1) must NOT
  // be excluded: they contain tool call details (name/args/result) that are
  // distinct from the reasoning/text in Activities/snapshots.
  const serverMainAssistantIds = new Set<string>();
  if (excludeMergedAssistant) {
    for (const m of normalizedServer) {
      if (m.role === 'assistant' && !isActivityMessage(m)) {
        serverMainAssistantIds.add(m.id);
      }
    }
  }

  const merged: Message[] = [];
  for (const srv of normalizedServer) {
    // Skip server main assistant messages when Activity data or snapshots
    // provide correctly separated per-round content
    if (excludeMergedAssistant && serverMainAssistantIds.has(srv.id)) continue;
    merged.push(srv);
  }
  for (const row of local) {
    if (pendingReplacedBy.has(row.id)) continue;
    if (serverById.has(row.id)) continue;
    if (!shouldIncludeInMerge(row, excludeMergedAssistant)) continue;
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

export function isActivityMessage(message: Message): boolean {
  if (message.tool_event) return true;
  try {
    const raw = JSON.parse(message.options_json || '{}') as { schema?: string; tool_event?: unknown };
    return raw.schema === 'chat.activity/v1' || Boolean(raw.tool_event);
  } catch {
    return message.status.startsWith('tool_');
  }
}
