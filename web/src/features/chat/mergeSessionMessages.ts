import type { Message } from './types';
import { normalizeServerMessageOptions } from './streamContentPatch';
import { parseMessageOptions } from './parseMessageOptions';
import { isInFlightStatus } from '../../domain/types';
import {
  ensureOrigin,
  isEphemeralOrigin,
  isInFlightOrigin,
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
  // ws-snap-* messages are finalized snapshots — treat as persisted for sorting
  if (message.origin?.kind === 'streaming_snapshot') return false;
  if (isEphemeralMessage(message)) return true;
  return isInFlightStatus(message.status || '');
}

/**
 * Whether a local-only message should be included in the merge output.
 *
 * Messages fall into four categories for merge purposes:
 *   1. **persisted** — has a server counterpart, managed by server data
 *   2. **in-flight-active** — still streaming/running, must be preserved
 *   3. **in-flight-snapshot** — finalized local snapshot (ws-snap-*) that
 *      carries correct per-round content when the server message merges
 *      all rounds into one. These must be preserved when excludeMergedAssistant=true.
 *   4. **af-finalized** — Activity-First mode messages (actv-*) that have been
 *      finalized (status='ok'). These carry per-round content from Activity
 *      events and must be preserved when excludeMergedAssistant=true, because
 *      the server's merged assistant ChatMessage is excluded.
 */
function shouldIncludeInMerge(message: Message, excludeMergedAssistant: boolean): boolean {
  // Streaming snapshots: finalized local snapshots carrying round-separated content
  if (message.origin?.kind === 'streaming_snapshot' && excludeMergedAssistant) return true;
  // AF mode: finalized Activity messages (actv-*) carry per-round content.
  // When the server's merged assistant message is excluded, these must be
  // preserved as the source of truth for assistant content.
  if (excludeMergedAssistant && message.id.startsWith('actv-') && message.status === 'ok') return true;
  return isInFlightLocalRow(message);
}

function messageOrder(message: Message): [number, string] {
  // Finalized local messages (streaming snapshots and AF-mode actv-* messages)
  // should sort alongside persisted messages by timestamp, not be pushed to
  // the end with active in-flight messages.
  if (message.origin?.kind === 'streaming_snapshot') return [0, message.created_at || ''];
  if (message.id.startsWith('actv-') && message.status === 'ok') return [0, message.created_at || ''];
  const inFlight = isInFlightLocalRow(message) ? 1 : 0;
  return [inFlight, message.created_at || ''];
}

/** Drop optimistic user placeholders after the server turn completes. */
export function dropPendingUserPlaceholders(messages: Message[]): Message[] {
  return messages.filter((m) => !isPendingUserOrigin(m.origin));
}

function shouldDropStaleInFlight(message: Message): boolean {
  const origin = message.origin;
  if (isPendingUserOrigin(origin)) return message.status !== 'failed';
  if (isStreamingOrigin(origin)) {
    // streaming_snapshot messages (ws-snap-*) represent correctly separated
    // LLM round content. They must NOT be dropped during merge, because the
    // server-persisted assistant message contains ALL rounds' content merged
    // into one, and dropping snapshots would lose the round separation.
    if (origin?.kind === 'streaming_snapshot') return false;
    // AF mode: when a streaming message is finalized (status='ok'), it
    // represents completed per-round content that must be preserved.
    // Dropping it would lose the round separation that Activity data provides.
    if (message.status === 'ok') return false;
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
  opts?: { dropStaleInFlight?: boolean; activityFirst?: boolean },
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

const PENDING_MATCH_TIME_WINDOW_MS = 5_000;

/** Match a pending-user placeholder to a server-persisted user message by content. */
function isPendingUserMatch(pending: Message, serverUser: Message): boolean {
  if (!isPendingUserOrigin(pending.origin)) return false;
  if (pending.role !== 'user' || serverUser.role !== 'user') return false;
  if (pending.session_id !== serverUser.session_id) return false;
  if (pending.content_markdown !== serverUser.content_markdown) return false;

  if (pending.turn_number > 0 && serverUser.turn_number > 0 && pending.turn_number !== serverUser.turn_number) {
    return false;
  }

  const pendingTime = new Date(pending.created_at).getTime();
  const serverTime = new Date(serverUser.created_at).getTime();
  if (Number.isFinite(pendingTime) && Number.isFinite(serverTime)) {
    if (Math.abs(pendingTime - serverTime) > PENDING_MATCH_TIME_WINDOW_MS) return false;
  }

  return true;
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

/** Whether this row is a local streaming placeholder (ws-stream-* prefix). */
function isStreamingPlaceholder(message: Message): boolean {
  return isStreamingOrigin(message.origin);
}

/** Build a map from (sessionId, role=assistant, content) → server message for fast lookup. */
function buildServerAssistantContentMap(server: Message[]): Map<string, Message[]> {
  const map = new Map<string, Message[]>();
  for (const m of server) {
    if (m.role !== 'assistant') continue;
    const key = `${m.session_id}::${(m.content_markdown ?? '').trim()}`;
    const arr = map.get(key) ?? [];
    arr.push(m);
    map.set(key, arr);
  }
  return map;
}

function findServerMatchForStreaming(
  streaming: Message,
  serverMap: Map<string, Message[]>,
  matched: Set<string>,
): Message | null {
  const content = (streaming.content_markdown ?? '').trim();
  const reasoning = (streaming.reasoning_markdown ?? '').trim();
  // M-02: Allow matching when only reasoning exists (no content_markdown).
  // Some LLM rounds produce only thinking with no separate reply text.
  if (!content && !reasoning) return null;

  // Try exact content match first
  if (content) {
    const key = `${streaming.session_id}::${content}`;
    const candidates = serverMap.get(key);
    if (candidates) {
      for (const c of candidates) {
        if (!matched.has(c.id)) {
          matched.add(c.id);
          return c;
        }
      }
    }
  }

  // Try prefix match — server content may be longer (includes reasoning, etc.)
  // Only allow serverContent.startsWith(content): the server message is the
  // superset that contains the streaming content as a prefix. The reverse
  // direction (content.startsWith(serverContent)) is unsafe because a
  // streaming message may contain accumulated multi-round content that
  // coincidentally starts with a different round's server message.
  // PERF: O(n*m) prefix scan over server assistant messages. Acceptable
  // for typical session sizes (< 100 assistant messages per session).
  for (const [mapKey, msgs] of serverMap) {
    const prefix = `${streaming.session_id}::`;
    if (!mapKey.startsWith(prefix)) continue;
    const serverContent = mapKey.slice(prefix.length);
    if (!serverContent || !content) continue;
    if (serverContent.startsWith(content)) {
      for (const c of msgs) {
        if (!matched.has(c.id)) {
          matched.add(c.id);
          return c;
        }
      }
    }
  }
  return null;
}

/** Merge server history with in-flight WS rows (streaming text, running tool cards). */
export function mergeSessionMessages(
  server: Message[],
  local: Message[],
  opts?: { dropStaleInFlight?: boolean; activityFirst?: boolean },
): Message[] {
  const normalizedServer = server.map(normalizeMessage);
  if (local.length === 0) return normalizedServer;
  const serverById = new Map(normalizedServer.map((m) => [m.id, m]));
  const serverUserByContent = buildServerUserContentMap(normalizedServer);
  const serverAssistantByContent = buildServerAssistantContentMap(normalizedServer);

  // Determine whether to exclude the server's merged assistant message.
  //
  // Two independent triggers:
  //   1. **activityFirst** (AF mode): Activity events/data provide complete
  //      per-round content (thinking/reply/action). The server-persisted
  //      assistant ChatMessage merges ALL rounds into one — including it would
  //      duplicate Activity content and cause the UI to show a single merged
  //      block instead of correctly separated rounds.
  //   2. **hasSnapshots** (legacy mode): When local streaming_snapshot messages
  //      exist (ws-snap-*), they carry correctly separated round content from
  //      the legacy tool_call handler's snapshot mechanism. The merged server
  //      message must be excluded to avoid duplication.
  //
  // AF mode is the primary path; hasSnapshots is a legacy compatibility fallback.
  // When AF mode is fully migrated (Phase 3 complete), hasSnapshots can be removed.
  const hasSnapshots = local.some(
    (m) => m.origin?.kind === 'streaming_snapshot' && m.role === 'assistant',
  );
  const excludeMergedAssistant = opts?.activityFirst || hasSnapshots;

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
  // Also match ws-stream assistant placeholders to server-persisted assistant messages.
  for (const row of local) {
    if (!isStreamingPlaceholder(row) || row.role !== 'assistant') continue;
    // Don't match ws-snap-* messages to server messages — they contain partial
    // content that is a subset of the full server message. Matching would cause
    // the server message to replace the snapshot, losing round separation.
    if (row.origin?.kind === 'streaming_snapshot') continue;
    const serverMatch = findServerMatchForStreaming(row, serverAssistantByContent, matchedServerIDs);
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
    return timeA.localeCompare(timeB);
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
