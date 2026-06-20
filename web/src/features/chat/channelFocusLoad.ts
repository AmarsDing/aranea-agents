import type { Message } from './types';

export type ChannelFocusLoadDeps = {
  getMessages: (sessionId: string) => Message[];
  loadMessages: (opts: {
    sessionId: string;
    replace?: boolean;
    dropStaleInFlight?: boolean;
    activityMessages?: Message[];
  }) => Promise<void>;
  setMessages: (sessionId: string, rows: Message[]) => void;
  ensureChatStream: (sessionId: string) => void;
  /**
   * AF-FE-15: Load Activity records for the session and reconstruct per-round
   * `actv-*` messages. Returns [] when no activities are available (legacy
   * sessions, pre-AF data, or API failure). Errors are swallowed and logged
   * so the caller can still load the server's merged message as a fallback.
   */
  loadActivitiesAndReconstruct: (sessionId: string) => Promise<Message[]>;
};

/**
 * Load history for channel auto-focus (DECO-R-P2-01).
 * When skipMessageReload is set, preserve ephemeral WS rows but still fetch the inbound user turn if missing locally.
 */
export async function hydrateSessionForChannelFocus(
  deps: ChannelFocusLoadDeps,
  sessionId: string,
  skipMessageReload?: boolean,
): Promise<void> {
  const sid = sessionId.trim();
  if (!sid) return;

  const hasUserContent = deps.getMessages(sid).some((m) => m.role === 'user' && (m.content_markdown ?? '').trim());

  if (!skipMessageReload || !hasUserContent) {
    // AF-FE-15: Pass activity-reconstructed messages so the AF path can drive
    // the UI with per-round thinking/reply blocks instead of the server's
    // single merged assistant message. Without this, the channel-focus path
    // (route watch, sidebar click, session restore) would silently fall back
    // to legacy rendering and the user would see "thinking/reply missing".
    let activityMessages: Message[] = [];
    try {
      activityMessages = (await deps.loadActivitiesAndReconstruct(sid)) ?? [];
    } catch (e) {
      // Swallow — legacy rendering is acceptable when AF load fails.
      console.warn('[channelFocusLoad] loadActivitiesAndReconstruct failed', e);
    }
    await deps.loadMessages({ sessionId: sid, replace: true, activityMessages });
  }

  deps.ensureChatStream(sid);
}
