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

  const currentRevision = input.messageStore.sessionRevisionBySession[sessionId] ?? 0;
  await input.messageStore.loadMessages({
    sessionId,
    dropStaleInFlight: true,
    afterRevision: currentRevision > 0 ? currentRevision : undefined,
  });
  input.streamingSnapshots.clear(sessionId);

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
