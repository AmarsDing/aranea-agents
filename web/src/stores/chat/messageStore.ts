/**
 * Chat message store — manages per-session message state, revision tracking,
 * and message merge logic. Split from the monolithic useChatStore.
 */
import { computed, ref } from "vue";
import { defineStore } from "pinia";
import {
  listSessionChatMessages as listMessages,
  listSessionChatMessagesAfterRevision,
} from "../../features/session/api";
import type { IntentPassResult, Message } from "../../features/chat/types";
import { mergeSessionMessages } from "../../features/chat/mergeSessionMessages";
import { useChatSessionStore } from "./sessionStore";

export const useChatMessageStore = defineStore("chatMessage", () => {
  const messagesBySession = ref<Record<string, Message[]>>({});
  const sessionRevisionBySession = ref<Record<string, number>>({});
  const lastIntentPass = ref<IntentPassResult | null>(null);

  function getMessages(sessionId: string): Message[] {
    return messagesBySession.value[sessionId] ?? [];
  }

  function setMessages(sessionId: string, rows: Message[]) {
    messagesBySession.value[sessionId] = rows;
  }

  function clearSessionMessages(sessionId?: string) {
    const session = useChatSessionStore();
    const sid = sessionId ?? session.currentSessionId();
    if (sid) messagesBySession.value[sid] = [];
  }

  function clearAllMessages() {
    for (const key of Object.keys(messagesBySession.value)) {
      delete messagesBySession.value[key];
    }
  }

  /** Convenience computed for the currently active session's messages. */
  const messages = computed({
    get(): Message[] {
      const session = useChatSessionStore();
      const sid = session.currentSessionId();
      return sid ? (messagesBySession.value[sid] ?? []) : [];
    },
    set(rows: Message[]) {
      const session = useChatSessionStore();
      const sid = session.currentSessionId();
      if (sid) messagesBySession.value[sid] = rows;
    },
  });

  async function loadMessages(opts?: {
    sessionId?: string;
    replace?: boolean;
    afterRevision?: number;
    dropStaleInFlight?: boolean;
  }) {
    const session = useChatSessionStore();
    const sid = opts?.sessionId ?? session.currentSessionId();
    if (!sid) return;

    const local = getMessages(sid);
    const mergeOpts = opts?.dropStaleInFlight ? { dropStaleInFlight: true } : undefined;

    if (opts?.afterRevision != null && opts.afterRevision > 0) {
      const { items, currentRevision } = await listSessionChatMessagesAfterRevision(
        sid,
        opts.afterRevision
      );
      sessionRevisionBySession.value[sid] = currentRevision;
      if (items.length > 0) {
        setMessages(sid, mergeSessionMessages(items, local, mergeOpts));
      }
      return;
    }

    const { items: server, currentRevision } = await listMessages(sid);
    sessionRevisionBySession.value[sid] = currentRevision;
    if (opts?.replace || local.length === 0) {
      setMessages(sid, mergeSessionMessages(server, [], mergeOpts));
      return;
    }
    setMessages(sid, mergeSessionMessages(server, local, mergeOpts));
  }

  function deleteSessionMessages(sessionId: string) {
    delete messagesBySession.value[sessionId];
    delete sessionRevisionBySession.value[sessionId];
  }

  return {
    messagesBySession,
    sessionRevisionBySession,
    lastIntentPass,
    messages,
    getMessages,
    setMessages,
    clearSessionMessages,
    clearAllMessages,
    loadMessages,
    deleteSessionMessages,
  };
});
