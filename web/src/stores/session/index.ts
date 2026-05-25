import { defineStore } from "pinia";
import { ref } from "vue";
import {
  searchSessions,
  getSession,
  createSession,
  deleteSession,
  archiveSession,
  updateSession,
  listSessionTurns,
  getSessionTimeline,
  listSessionChatMessages,
  previewSessionBatch,
  batchArchiveSessions,
  batchDeleteSessions,
  pinSession,
  restoreSession,
  unpinSession
} from "../../features/session/api";
import type { Session, SessionListResult } from "../../features/session/types";
import type { BatchOperationResult, BatchPreviewResult, SessionBatchScope } from "../../features/session/types";
import type { Message } from "../../features/chat/types";

export const useSessionStore = defineStore("session", () => {
  const sessions = ref<Session[]>([]);
  const activeSession = ref<Session | null>(null);
  const loading = ref(false);
  const total = ref(0);
  const keyword = ref("");

  async function loadSessions(params?: { keyword?: string; agent_id?: string; limit?: number; offset?: number }) {
    loading.value = true;
    try {
      const result: SessionListResult = await searchSessions(params ?? {});
      sessions.value = result.items ?? [];
      total.value = result.total ?? sessions.value.length;
    } finally {
      loading.value = false;
    }
  }

  async function searchPage(params: {
    keyword?: string;
    owner_type?: string;
    status?: string;
    context_status?: string;
    limit?: number;
    offset?: number;
  }): Promise<SessionListResult> {
    return searchSessions(params);
  }

  async function fetchSession(id: string) {
    const s = await getSession(id);
    activeSession.value = s;
    return s;
  }

  async function newSession(payload: { agent_id?: string; team_id?: string; owner_type?: string; title?: string }) {
    const s = await createSession({ ...payload, title: payload.title ?? "" });
    sessions.value.unshift(s);
    activeSession.value = s;
    return s;
  }

  async function removeSession(id: string) {
    await deleteSession(id);
    sessions.value = sessions.value.filter((s) => s.id !== id);
    if (activeSession.value?.id === id) activeSession.value = null;
  }

  async function archive(id: string) {
    await archiveSession(id);
    sessions.value = sessions.value.filter((s) => s.id !== id);
  }

  async function rename(id: string, title: string) {
    const updated = await updateSession(id, { title });
    sessions.value = sessions.value.map((s) => (s.id === id ? updated : s));
    if (activeSession.value?.id === id) activeSession.value = updated;
  }

  async function setPinned(id: string, pinned: boolean) {
    const updated = pinned ? await pinSession(id) : await unpinSession(id);
    sessions.value = sessions.value.map((s) => (s.id === id ? updated : s));
    if (activeSession.value?.id === id) activeSession.value = updated;
    return updated;
  }

  function setActiveSession(s: Session | null) {
    activeSession.value = s;
  }

  async function previewBatch(payload: {
    mode: "archive" | "delete";
    ids?: string[];
    older_than_days?: number;
    scope?: SessionBatchScope;
    include_archived?: boolean;
  }): Promise<BatchPreviewResult> {
    return previewSessionBatch(payload);
  }

  async function batchArchive(payload: {
    ids?: string[];
    older_than_days?: number;
    scope?: SessionBatchScope;
  }): Promise<BatchOperationResult> {
    return batchArchiveSessions(payload);
  }

  async function batchDelete(payload: {
    ids?: string[];
    older_than_days?: number;
    scope?: SessionBatchScope;
    include_archived?: boolean;
  }): Promise<BatchOperationResult> {
    return batchDeleteSessions(payload);
  }

  // EP-FE-01: store-level actions so components don't need to import features/*/api directly.
  async function fetchTurns(sessionId: string, limit = 20, offset = 0) {
    return listSessionTurns(sessionId, limit, offset);
  }

  async function fetchTimeline(sessionId: string) {
    return getSessionTimeline(sessionId);
  }

  async function fetchMessages(sessionId: string): Promise<{ items: Message[]; currentRevision: number }> {
    return listSessionChatMessages(sessionId);
  }

  return {
    sessions,
    activeSession,
    loading,
    total,
    keyword,
    loadSessions,
    searchPage,
    fetchSession,
    newSession,
    removeSession,
    archive,
    rename,
    setPinned,
    setActiveSession,
    previewBatch,
    batchArchive,
    batchDelete,
    fetchTurns,
    fetchTimeline,
    fetchMessages,
    restore: restoreSession
  };
});
