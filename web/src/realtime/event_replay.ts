/**
 * Event Replay — B2 channel separation.
 *
 * Responsibility: request replay of missed events after WS reconnect.
 *
 * The WS transport already supports event-ID-based replay (via `lastEventId`
 * and the `replay_start`/`replay_end` protocol). This module adds
 * revision-based sync for message-level consistency: after reconnect, the
 * client sends a `sync_request { afterRevision }` so the server can replay
 * any envelopes that were missed during the disconnection.
 *
 * Design:
 * - Client maintains `lastRevision` per session (updated on each envelope
 *   carrying `session_revision`).
 * - On reconnect, `requestSyncReplay` sends a `sync_request` upstream with
 *   the last known revision. The server replays envelopes with
 *   `session_revision > afterRevision`.
 * - If `afterRevision` is 0 or undefined, no sync is requested (the server's
 *   event-ID-based replay handles it).
 */
import type { WsUpstream } from './envelope';

/** Per-session revision tracker for sync replay. */
export class RevisionTracker {
  private revisions = new Map<string, number>();

  /** Update the last-known revision for a session. Higher revisions win. */
  update(sessionId: string, revision: number): void {
    if (!sessionId || revision <= 0) return;
    const current = this.revisions.get(sessionId) ?? 0;
    if (revision > current) {
      this.revisions.set(sessionId, revision);
    }
  }

  /** Get the last-known revision for a session (0 if unknown). */
  get(sessionId: string): number {
    return this.revisions.get(sessionId) ?? 0;
  }

  /** Clear revision tracking for a session. */
  clear(sessionId: string): void {
    this.revisions.delete(sessionId);
  }

  /** Clear all revision tracking. */
  clearAll(): void {
    this.revisions.clear();
  }
}

/**
 * Build a `sync_request` upstream message for revision-based replay.
 *
 * The server should replay all envelopes with `session_revision > afterRevision`
 * for the given session. If `afterRevision` is 0, returns `null` (no sync
 * needed — event-ID-based replay is sufficient).
 */
export function buildSyncRequest(sessionId: string, afterRevision: number): WsUpstream | null {
  if (!sessionId || afterRevision <= 0) return null;
  return {
    direction: 'client_to_server',
    channel: 'system',
    type: 'sync_request',
    payload: {
      session_id: sessionId,
      after_revision: afterRevision,
    },
  };
}

/**
 * Request sync replay after WS reconnect.
 *
 * Sends a `sync_request { afterRevision }` via the provided `send` function
 * (typically `transport.send`). The server replays missed envelopes.
 *
 * @param send The WS send function (from `WsTransport.send`).
 * @param sessionId The session to sync.
 * @param afterRevision The last-known revision for the session.
 * @returns `true` if a sync request was sent, `false` if skipped (no revision).
 */
export function requestSyncReplay(
  send: (upstream: WsUpstream) => void,
  sessionId: string,
  afterRevision: number,
): boolean {
  const request = buildSyncRequest(sessionId, afterRevision);
  if (!request) return false;
  send(request);
  return true;
}
