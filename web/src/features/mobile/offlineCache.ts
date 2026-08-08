/**
 * P3.2c: offline cache for the mobile session list.
 *
 * The chat session store only mutates its list on a *successful* load, so on
 * a phone that opens the app without connectivity the list stays empty. This
 * module persists the last known non-empty list per agent in localStorage so
 * the mobile sessions page can render it (flagged as cached) when offline or
 * when the load failed on a weak network.
 *
 * Design notes:
 * - Cache is only ever written with non-empty live data (it represents the
 *   "last known good list"); the offline banner communicates staleness.
 * - Whole SessionView objects are stored (small: ≤50 rows ≈ tens of KB, well
 *   within the localStorage quota) so future mobile views can reuse them.
 * - All storage access is guarded: corrupt JSON, missing key, quota errors and
 *   non-browser environments degrade to "no cache" instead of throwing.
 */
import type { SessionView } from '../../components/chat/types';

const CACHE_KEY_PREFIX = 'aranea.mobile.sessionCache.v1.';
const MAX_CACHED_SESSIONS = 50;

interface CacheEnvelope {
  v: 1;
  agentId: string;
  cachedAt: number;
  sessions: SessionView[];
}

function cacheKey(agentId: string): string {
  return `${CACHE_KEY_PREFIX}${agentId}`;
}

function storage(): Storage | null {
  try {
    if (typeof localStorage === 'undefined') return null;
    return localStorage;
  } catch {
    // Accessing localStorage can throw (e.g. disabled cookies).
    return null;
  }
}

/** Minimal shape guard so corrupt/tampered entries degrade to "no cache". */
function isSessionViewLike(value: unknown): value is SessionView {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const row = value as Record<string, unknown>;
  return typeof row.id === 'string' && row.id.length > 0 && typeof row.title === 'string';
}

export function readCachedMobileSessions(agentId: string): SessionView[] {
  const id = agentId.trim();
  const store = storage();
  if (!id || !store) return [];
  let raw: string | null;
  try {
    raw = store.getItem(cacheKey(id));
  } catch {
    return [];
  }
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return [];
  const envelope = parsed as Partial<CacheEnvelope>;
  if (envelope.v !== 1 || envelope.agentId !== id || !Array.isArray(envelope.sessions)) return [];
  return envelope.sessions.filter(isSessionViewLike);
}

export function writeCachedMobileSessions(agentId: string, sessions: readonly SessionView[]): void {
  const id = agentId.trim();
  const store = storage();
  if (!id || !store || sessions.length === 0) return;
  const envelope: CacheEnvelope = {
    v: 1,
    agentId: id,
    cachedAt: Date.now(),
    sessions: sessions.slice(0, MAX_CACHED_SESSIONS),
  };
  try {
    store.setItem(cacheKey(id), JSON.stringify(envelope));
  } catch {
    // Quota exceeded / storage full — caching is best-effort, never fatal.
  }
}

export function clearCachedMobileSessions(agentId: string): void {
  const id = agentId.trim();
  const store = storage();
  if (!id || !store) return;
  try {
    store.removeItem(cacheKey(id));
  } catch {
    /* best-effort */
  }
}
