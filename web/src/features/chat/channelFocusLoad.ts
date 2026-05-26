import { applyStreamingSnapshotToSession } from "../../stores/chatStreamingSnapshots";
import type { Message } from "./types";

export type ChannelFocusLoadDeps = {
  getMessages: (sessionId: string) => Message[];
  loadMessages: (opts: { sessionId: string }) => Promise<void>;
  setMessages: (sessionId: string, rows: Message[]) => void;
  ensureChatStream: (sessionId: string) => void;
};

/**
 * Load history for channel auto-focus (DECO-R-P2-01).
 * When skipMessageReload is set, preserve ephemeral WS rows but still fetch the inbound user turn if missing locally.
 */
export async function hydrateSessionForChannelFocus(
  deps: ChannelFocusLoadDeps,
  sessionId: string,
  skipMessageReload?: boolean
): Promise<void> {
  const sid = sessionId.trim();
  if (!sid) return;

  const hasUserContent = deps
    .getMessages(sid)
    .some((m) => m.role === "user" && (m.content_markdown ?? "").trim());

  if (!skipMessageReload || !hasUserContent) {
    await deps.loadMessages({ sessionId: sid });
  }

  applyStreamingSnapshotToSession(
    (s) => deps.getMessages(s),
    (s, rows) => deps.setMessages(s, rows),
    sid
  );
  deps.ensureChatStream(sid);
}
