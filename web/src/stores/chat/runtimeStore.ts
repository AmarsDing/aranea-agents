/**
 * Chat runtime store — manages WS connection state, run status,
 * await-reply, stop-generation, and chat API gateway.
 * Split from the monolithic useChatStore.
 */
import { ref } from 'vue';
import { defineStore } from 'pinia';
import {
  awaitUserReply,
  cancelChatBackgroundJob,
  cancelPendingMessage,
  enqueueMessage,
  getPendingMessages,
  getRunStatus,
  listChatOptions as apiListChatOptions,
  stopGeneration,
  submitMessageFeedback as apiSubmitFeedback,
  updatePendingMessage,
  type PendingMessage,
} from '../../features/chat/api';
import type { ChatOption, RunStatus } from '../../features/chat/types';

export const useChatRuntimeStore = defineStore('chatRuntime', () => {
  const wsConnectedBySession = ref<Record<string, boolean>>({});

  function setWsConnected(sessionId: string, connected: boolean) {
    wsConnectedBySession.value[sessionId] = connected;
  }

  function isWsConnected(sessionId: string): boolean {
    return wsConnectedBySession.value[sessionId] ?? false;
  }

  async function fetchRunStatus(sessionId: string): Promise<RunStatus> {
    return getRunStatus(sessionId);
  }

  async function submitAwaitReply(sessionId: string, reply: string, runId?: string): Promise<boolean> {
    return awaitUserReply(sessionId, reply, runId);
  }

  async function stop(sessionId: string) {
    return stopGeneration(sessionId);
  }

  async function enqueue(sessionId: string, content: string) {
    return enqueueMessage(sessionId, content);
  }

  async function fetchPendingMessages(sessionId: string): Promise<PendingMessage[]> {
    return getPendingMessages(sessionId);
  }

  async function cancelPending(sessionId: string, pendingId: string): Promise<boolean> {
    return cancelPendingMessage(sessionId, pendingId);
  }

  async function updatePending(sessionId: string, pendingId: string, content: string): Promise<boolean> {
    return updatePendingMessage(sessionId, pendingId, content);
  }

  async function submitFeedback(payload: { session_id: string; message_id: string; rating: 'positive' | 'negative' }) {
    return apiSubmitFeedback(payload);
  }

  async function cancelBackgroundJob(id: string, source: string): Promise<boolean> {
    return cancelChatBackgroundJob(id, source);
  }

  async function listChatOptions(type?: string): Promise<ChatOption[]> {
    return apiListChatOptions(type);
  }

  function deleteSessionRuntime(sessionId: string) {
    delete wsConnectedBySession.value[sessionId];
  }

  return {
    wsConnectedBySession,
    isWsConnected,
    setWsConnected,
    fetchRunStatus,
    submitAwaitReply,
    stop,
    enqueue,
    fetchPendingMessages,
    cancelPending,
    updatePending,
    submitFeedback,
    cancelBackgroundJob,
    listChatOptions,
    deleteSessionRuntime,
  };
});
