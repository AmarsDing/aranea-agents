import type { useChatSessionStore } from '../../stores/chat/sessionStore';
import type { useChatMessageStore } from '../../stores/chat/messageStore';
import type { useChatStreamingSnapshots } from '../../stores/chatStreamingSnapshots';

type SessionStore = ReturnType<typeof useChatSessionStore>;
type MessageStore = ReturnType<typeof useChatMessageStore>;
type StreamingSnapshots = ReturnType<typeof useChatStreamingSnapshots>;

/** Shared post-turn reload for Agent + Team session WS (DECO-R-P2-02). */
export async function reloadSessionAfterCompletion(input: {
  sessionStore: SessionStore;
  messageStore: MessageStore;
  streamingSnapshots: StreamingSnapshots;
  sessionId: string;
  resolveAgentId?: () => string | undefined;
  /**
   * When true, Activity-First mode is active for this session.
   * AF mode skips loadMessages because Activity events already provide
   * complete per-round data (thinking/reply/action) — reloading would
   * replace correctly separated streaming state with the server's merged
   * assistant message, causing UI jumping. Only session metadata
   * (token usage, context ratio, session list) is refreshed.
   */
  activityFirst?: boolean;
}): Promise<void> {
  const sessionId = input.sessionId.trim();
  if (!sessionId) return;

  if (!input.activityFirst) {
    // Legacy mode: reload messages to sync with server-persisted data.
    const currentRevision = input.messageStore.sessionRevisionBySession[sessionId] ?? 0;
    await input.messageStore.loadMessages({
      sessionId,
      dropStaleInFlight: true,
      afterRevision: currentRevision > 0 ? currentRevision : undefined,
    });
    input.streamingSnapshots.clear(sessionId);
  }
  // AF mode: Activity events are the source of truth for assistant content.
  // The streaming state IS the final state — no reload needed.
  // Session metadata is still refreshed below.

  await input.sessionStore.fetchAndReconcileSession(sessionId);

  if (input.sessionStore.entityKind === 'agent') {
    const agentId = input.resolveAgentId?.()?.trim();
    if (agentId) {
      await input.sessionStore.loadAgentSessions(agentId, { refreshOnly: true });
    }
    return;
  }

  if (input.sessionStore.entityKind === 'team') {
    const teamId = input.sessionStore.selectedTeamId?.trim();
    if (teamId) {
      await input.sessionStore.loadTeamSessions(teamId);
    }
  }
}
