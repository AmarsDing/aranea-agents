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
}): Promise<void> {
  const sessionId = input.sessionId.trim();
  if (!sessionId) return;

  // T7.3c: Legacy reload path removed. AF mode is the only path.
  // Activity events are the source of truth for assistant content.
  // The streaming state IS the final state — no reload needed.
  // Session metadata is still refreshed below.

  await input.sessionStore.fetchAndReconcileSession(sessionId);

  // Refresh session list for the current entity kind
  if (input.sessionStore.entityKind === 'agent') {
    const agentId = input.resolveAgentId?.()?.trim();
    if (agentId) {
      await input.sessionStore.loadSessions(agentId, { refreshOnly: true });
    }
    return;
  }

  if (input.sessionStore.entityKind === 'team') {
    const teamId = input.sessionStore.selectedTeamId?.trim();
    if (teamId) {
      await input.sessionStore.loadSessions(teamId);
    }
  }
}
