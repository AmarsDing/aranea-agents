import { getSession } from "../session/api";
import type { Session } from "../session/types";
import type { Envelope } from "./envelope";
import { envelopeSource } from "./inboundSyncEnvelope";
import { parseChannelSessionMeta, isChannelSession } from "./channelSessionMeta";

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

export async function resolveInboundSession(
  sessionId: string,
  chatStore: SessionLookup
): Promise<Session | null> {
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
  chatStore: SessionLookup
): Promise<boolean> {
  if (source === "channel") return true;
  const sess = await resolveInboundSession(sessionId, chatStore);
  if (!sess) return false;
  return (
    parseChannelSessionMeta(sess.metadata_json) !== null ||
    isChannelSession(sess.metadata_json, sess.title)
  );
}

export async function resolveInboundAgentId(
  sessionId: string,
  env: Envelope,
  chatStore: SessionLookup
): Promise<string> {
  const md = env.metadata as Record<string, unknown> | undefined;
  const fromMeta = typeof md?.agent_id === "string" ? md.agent_id.trim() : "";
  if (fromMeta) return fromMeta;
  const sess = await resolveInboundSession(sessionId, chatStore);
  return sess?.agent_id?.trim() ?? "";
}

/** Toast on channel turn complete; dedupe runner_completion vs revision completed. */
export function shouldChannelInboundCompleteToast(env: Envelope): boolean {
  if (env.type === "runner_completion") return true;
  if (env.type !== "run_status") return false;
  const status = String((env.metadata as Record<string, unknown> | undefined)?.status ?? "");
  if (status === "failed" || status === "cancelled") return true;
  return status === "completed" && envelopeSource(env) === "channel";
}
