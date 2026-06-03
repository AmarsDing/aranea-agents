/**
 * Chat message store — manages per-session message state, revision tracking,
 * and message merge logic. Split from the monolithic useChatStore.
 */
import { ref } from "vue";
import { defineStore } from "pinia";
import {
  listSessionChatMessages as listMessages,
  listSessionChatMessagesAfterRevision,
} from "../../features/session/api";
import type { IntentPassResult, Message } from "../../features/chat/types";
import {
  mergeIncrementalSessionMessages,
  mergeSessionMessages,
} from "../../features/chat/mergeSessionMessages";
import { onSessionMutation } from "../sessionSync";

export const useChatMessageStore = defineStore("chatMessage", () => {
  const messagesBySession = ref<Record<string, Message[]>>({});
  const sessionRevisionBySession = ref<Record<string, number>>({});
  const lastIntentPass = ref<IntentPassResult | null>(null);

  onSessionMutation((mutation) => {
    if (mutation.type === "agent_removed") {
      clearAllMessages();
    }
  });

  function getMessages(sessionId: string): Message[] {
    return messagesBySession.value[sessionId] ?? [];
  }

  function setMessages(sessionId: string, rows: Message[]) {
    messagesBySession.value[sessionId] = rows;
  }

  function clearSessionMessages(sessionId: string) {
    if (sessionId) messagesBySession.value[sessionId] = [];
  }

  function clearAllMessages() {
    for (const key of Object.keys(messagesBySession.value)) {
      delete messagesBySession.value[key];
    }
  }

  async function loadMessages(opts: {
    sessionId: string;
    replace?: boolean;
    afterRevision?: number;
    dropStaleInFlight?: boolean;
  }) {
    const sid = opts.sessionId;
    if (!sid) return;

    const local = getMessages(sid);
    const mergeOpts = opts.dropStaleInFlight ? { dropStaleInFlight: true } : undefined;

    if (opts.afterRevision != null && opts.afterRevision > 0) {
      const { items, currentRevision } = await listSessionChatMessagesAfterRevision(
        sid,
        opts.afterRevision
      );
      sessionRevisionBySession.value[sid] = currentRevision;
      if (items.length > 0) {
        setMessages(sid, mergeIncrementalSessionMessages(items, local, mergeOpts));
      } else if (currentRevision > opts.afterRevision || opts?.dropStaleInFlight) {
        const { items: server, currentRevision: fullRev } = await listMessages(sid);
        sessionRevisionBySession.value[sid] = fullRev;
        setMessages(sid, mergeSessionMessages(server, local, mergeOpts));
      }
      return;
    }

    const { items: server, currentRevision } = await listMessages(sid);
    sessionRevisionBySession.value[sid] = currentRevision;
    if (opts.replace || local.length === 0) {
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
    getMessages,
    setMessages,
    clearSessionMessages,
    clearAllMessages,
    loadMessages,
    deleteSessionMessages,
  };
});
