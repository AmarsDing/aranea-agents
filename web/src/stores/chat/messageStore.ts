/**
 * Chat message store — manages per-session message state, revision tracking,
 * and message merge logic. Split from the monolithic useChatStore.
 */
import { ref } from 'vue';
import { defineStore } from 'pinia';
import {
  listSessionChatMessages as listMessages,
  listSessionChatMessagesAfterRevision,
} from '../../features/session/api';
import type { Message } from '../../features/chat/types';
import { mergeIncrementalSessionMessages, mergeSessionMessages } from '../../features/chat/mergeSessionMessages';
import { onSessionMutation } from '../sessionSync';

export const useChatMessageStore = defineStore('chatMessage', () => {
  const messagesBySession = ref<Record<string, Message[]>>({});
  const sessionRevisionBySession = ref<Record<string, number>>({});

  onSessionMutation((mutation) => {
    if (mutation.type === 'agent_removed') {
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
    /**
     * When true, exclude server-persisted merged assistant ChatMessages from
     * the merge result. In AF mode, Activity data provides per-round content
     * (thinking/reply/action), so the merged assistant message is redundant.
     *
     * IMPORTANT: Only set this to true when Activity data is confirmed
     * available for the session. Pre-AF sessions (no Activity data) rely on
     * the merged message for content display. Default is false.
     */
    activityFirst?: boolean;
  }) {
    const sid = opts.sessionId;
    if (!sid) return;

    const activityFirst = opts.activityFirst ?? false;
    const local = getMessages(sid);
    const mergeOpts = {
      dropStaleInFlight: opts.dropStaleInFlight ?? false,
      activityFirst,
    };

    if (opts.afterRevision != null && opts.afterRevision > 0) {
      const { items, currentRevision } = await listSessionChatMessagesAfterRevision(sid, opts.afterRevision);
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
    getMessages,
    setMessages,
    clearSessionMessages,
    clearAllMessages,
    loadMessages,
    deleteSessionMessages,
  };
});
