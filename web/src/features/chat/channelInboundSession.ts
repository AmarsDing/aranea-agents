import { getSession } from '../session/api';
import type { Session } from '../session/types';
import type { ActivityEvent } from '../../realtime/activityEvent';
import { activitySource } from './inboundSyncEnvelope';
import { parseChannelSessionMeta, isChannelSession } from './channelSessionMeta';

const SESSION_CACHE_MAX = 64;
const sessionCache = new Map<string, Session>();

function cacheSession(sessionId: string, row: Session) {
  sessionCache.delete(sessionId);
  sessionCache.set(sessionId, row);
  while (sessionCache.size > SESSION_CACHE_MAX) {
    const oldest = sessionCache.keys().next().value;
    if (!oldest) break;
    sessionCache.delete(oldest);
  }
}

type SessionLookup = {
  findSessionById: (sessionId: string) => Session | undefined | null;
};

export async function resolveInboundSession(sessionId: string, chatStore: SessionLookup): Promise<Session | null> {
  const hit = chatStore.findSessionById(sessionId);
  if (hit) return hit;
  const cached = sessionCache.get(sessionId);
  if (cached) return cached;
  try {
    const row = await getSession(sessionId);
    cacheSession(sessionId, row);
    return row;
  } catch {
    return null;
  }
}

export async function isChannelInboundSession(
  sessionId: string,
  source: string,
  chatStore: SessionLookup,
): Promise<boolean> {
  if (source === 'channel') return true;
  const sess = await resolveInboundSession(sessionId, chatStore);
  if (!sess) return false;
  return parseChannelSessionMeta(sess.metadata_json) !== null || isChannelSession(sess.metadata_json, sess.title);
}

export async function resolveInboundAgentIdFromActivity(
  sessionId: string,
  ev: ActivityEvent,
  chatStore: SessionLookup,
): Promise<string> {
  const meta = ev.activity.meta ?? {};
  const fromMeta = typeof meta.agent_id === 'string' ? meta.agent_id.trim() : '';
  if (fromMeta) return fromMeta;
  const sess = await resolveInboundSession(sessionId, chatStore);
  return sess?.agent_id?.trim() ?? '';
}

/** Toast on channel turn complete; dedupe runner_completion vs revision completed. */
export function shouldChannelInboundCompleteToastActivity(ev: ActivityEvent): boolean {
  if (ev.activity.stage === 'runner_completion') return true;
  if (ev.activity.stage !== 'run_status') return false;
  const status = String(ev.activity.meta?.status ?? '');
  if (status === 'failed' || status === 'cancelled') return true;
  return status === 'completed' && activitySource(ev) === 'channel';
}
