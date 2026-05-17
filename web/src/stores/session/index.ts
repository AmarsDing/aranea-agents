import { defineStore } from "pinia";
import { ref } from "vue";
import {
  searchSessions,
  getSession,
  createSession,
  deleteSession,
  archiveSession,
  updateSession,
  type Session,
  type SessionListResult
} from "../../features/session/api";

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

  async function fetchSession(id: string) {
    const s = await getSession(id);
    activeSession.value = s;
    return s;
  }

  async function newSession(payload: { agent_id?: string; team_id?: string; owner_type?: string; title?: string }) {
    const s = await createSession(payload);
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

  function setActiveSession(s: Session | null) {
    activeSession.value = s;
  }

  return { sessions, activeSession, loading, total, keyword, loadSessions, fetchSession, newSession, removeSession, archive, rename, setActiveSession };
});
