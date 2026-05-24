/**
 * Chat runtime store — manages WS connection state, run status,
 * await-reply, and stop-generation. Split from the monolithic useChatStore.
 */
import { computed, ref } from "vue";
import { defineStore } from "pinia";
import {
  awaitUserReply,
  cancelPendingMessage,
  enqueueMessage,
  getPendingMessages,
  getRunStatus,
  stopGeneration,
  updatePendingMessage,
  type PendingMessage,
} from "../../features/chat/api";
import type { RunStatus } from "../../features/chat/types";
import { useChatSessionStore } from "./sessionStore";

export const useChatRuntimeStore = defineStore("chatRuntime", () => {
  const wsConnectedBySession = ref<Record<string, boolean>>({});

  function setWsConnected(sessionId: string, connected: boolean) {
    wsConnectedBySession.value[sessionId] = connected;
  }

  const wsConnected = computed(() => {
    const session = useChatSessionStore();
    const sid = session.currentSessionId();
    return sid ? (wsConnectedBySession.value[sid] ?? false) : false;
  });

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

  function deleteSessionRuntime(sessionId: string) {
    delete wsConnectedBySession.value[sessionId];
  }

  return {
    wsConnectedBySession,
    wsConnected,
    setWsConnected,
    fetchRunStatus,
    submitAwaitReply,
    stop,
    enqueue,
    fetchPendingMessages,
    cancelPending,
    updatePending,
    deleteSessionRuntime,
  };
});
