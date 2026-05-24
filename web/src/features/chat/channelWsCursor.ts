/** Latest WS envelope id per session — skip EventBuffer replay on session WS connect. */
const lastEnvelopeIdBySession = new Map<string, string>();

export function noteChannelWsEnvelope(sessionId: string, envelopeId: string): void {
  const sid = sessionId.trim();
  const id = envelopeId.trim();
  if (sid && id) lastEnvelopeIdBySession.set(sid, id);
}

export function getChannelWsCursor(sessionId: string): string | undefined {
  const id = lastEnvelopeIdBySession.get(sessionId.trim());
  return id?.trim() || undefined;
}

export function clearChannelWsCursor(sessionId: string): void {
  lastEnvelopeIdBySession.delete(sessionId.trim());
}
