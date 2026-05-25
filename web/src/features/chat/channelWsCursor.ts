/** Latest WS envelope id per session — skip EventBuffer replay on session WS connect. */
const lastEnvelopeIdBySession = new Map<string, string>();

const STORAGE_PREFIX = "aranea:channel-ws-cursor:";

function storageKey(sessionId: string): string {
  return `${STORAGE_PREFIX}${sessionId.trim()}`;
}

function readCursorFromStorage(sessionId: string): string | undefined {
  if (typeof sessionStorage === "undefined") return undefined;
  try {
    const value = sessionStorage.getItem(storageKey(sessionId));
    return value?.trim() || undefined;
  } catch {
    return undefined;
  }
}

function writeCursorToStorage(sessionId: string, envelopeId: string): void {
  if (typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.setItem(storageKey(sessionId), envelopeId);
  } catch {
    /* private mode / quota */
  }
}

function removeCursorFromStorage(sessionId: string): void {
  if (typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.removeItem(storageKey(sessionId));
  } catch {
    /* ignore */
  }
}

export function noteChannelWsEnvelope(sessionId: string, envelopeId: string): void {
  const sid = sessionId.trim();
  const id = envelopeId.trim();
  if (!sid || !id) return;
  lastEnvelopeIdBySession.set(sid, id);
  writeCursorToStorage(sid, id);
}

export function getChannelWsCursor(sessionId: string): string | undefined {
  const sid = sessionId.trim();
  if (!sid) return undefined;

  const cached = lastEnvelopeIdBySession.get(sid)?.trim();
  if (cached) return cached;

  const stored = readCursorFromStorage(sid);
  if (stored) {
    lastEnvelopeIdBySession.set(sid, stored);
  }
  return stored;
}

export function clearChannelWsCursor(sessionId: string): void {
  const sid = sessionId.trim();
  if (!sid) return;
  lastEnvelopeIdBySession.delete(sid);
  removeCursorFromStorage(sid);
}
