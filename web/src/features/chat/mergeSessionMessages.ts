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

function messageOrder(message: Message): [number, string] {
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
  if (!content) return null;
  // Try exact match first
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
  // Try prefix match — server content may be longer (includes reasoning, etc.)
  for (const [mapKey, msgs] of serverMap) {
    const prefix = `${streaming.session_id}::`;
    if (!mapKey.startsWith(prefix)) continue;
    const serverContent = mapKey.slice(prefix.length);
    if (serverContent.startsWith(content) || content.startsWith(serverContent)) {
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
  opts?: { dropStaleInFlight?: boolean },
): Message[] {
  const normalizedServer = server.map(normalizeMessage);
  if (local.length === 0) return normalizedServer;
  const serverById = new Map(normalizedServer.map((m) => [m.id, m]));
  const serverUserByContent = buildServerUserContentMap(normalizedServer);
  const serverAssistantByContent = buildServerAssistantContentMap(normalizedServer);

  // Check if there are streaming_snapshot messages — these represent correctly
  // separated LLM round content. When snapshots exist, the server-persisted
  // assistant message contains ALL rounds merged into one, which would
  // duplicate the snapshot content. We need to handle this carefully.
  const hasSnapshots = local.some(
    (m) => m.origin?.kind === 'streaming_snapshot' && m.role === 'assistant',
  );

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

  // When snapshots exist, exclude server assistant messages that would duplicate
  // the snapshot content. The snapshots + ws-stream-* already have the correct
  // per-round content. The server message contains ALL rounds merged into one.
  const serverAssistantIds = new Set<string>();
  if (hasSnapshots) {
    for (const m of normalizedServer) {
      if (m.role === 'assistant') serverAssistantIds.add(m.id);
    }
  }

  const merged: Message[] = [];
  for (const srv of normalizedServer) {
    // Skip server assistant messages when snapshots exist — they'd duplicate content
    if (hasSnapshots && serverAssistantIds.has(srv.id)) continue;
    merged.push(srv);
  }
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
    const raw = JSON.parse(message.options_json || '{}') as { schema?: string; tool_event?: unknown };
    return raw.schema === 'chat.activity/v1' || Boolean(raw.tool_event);
  } catch {
    return message.status.startsWith('tool_');
  }
}
