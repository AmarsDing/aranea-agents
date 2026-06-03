import type { ChatEntityKind } from '../../components/chat/types';
import type { useChatSessionStore } from '../../stores/chat/sessionStore';

export type ChannelSessionRefreshContext = {
  entityKind?: ChatEntityKind;
  activeAgentId?: string | null;
};

/** Reload agent session list when a channel turn starts or completes (sidebar sync). */
export async function refreshAgentSessionsForChannel(
  sessionStore: ReturnType<typeof useChatSessionStore>,
  agentId: string,
  context?: ChannelSessionRefreshContext,
): Promise<void> {
  const aid = agentId.trim();
  if (!aid) return;
  if (context?.entityKind === 'team') return;
  const active = context?.activeAgentId?.trim();
  if (active && active !== aid) return;
  await sessionStore.loadAgentSessions(aid, { refreshOnly: true });
}
