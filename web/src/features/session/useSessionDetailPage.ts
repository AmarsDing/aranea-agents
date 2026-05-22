import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { Session } from "./types";
import { useSessionStore } from "../../stores/session/index";
import { useSessionTimelinePanel } from "./useSessionTimelinePanel";

export function useSessionDetailPage() {
  const route = useRoute();
  const router = useRouter();
  const sessionStore = useSessionStore();

  const session = ref<Session | null>(null);
  const loadingSession = ref(true);
  const sessionError = ref("");
  const activeTab = ref("turns");

  const focusToolId = computed(() => {
    if (String(route.query.focus ?? "").trim() !== "tool") return "";
    return String(route.query.tool_id ?? "").trim();
  });

  const sessionId = computed(() => String(route.params.sessionId ?? "").trim());

  const timelinePanel = useSessionTimelinePanel(sessionId);

  function applyDeepLinkTab() {
    if (focusToolId.value) {
      activeTab.value = "timeline";
    }
  }

  async function loadSession() {
    const id = sessionId.value;
    if (!id) {
      sessionError.value = "Missing session ID";
      loadingSession.value = false;
      return;
    }
    loadingSession.value = true;
    sessionError.value = "";
    try {
      session.value = await sessionStore.fetchSession(id);
    } catch (err) {
      sessionError.value = err instanceof Error ? err.message : "Failed to load session";
    } finally {
      loadingSession.value = false;
    }
  }

  async function handleArchive() {
    if (!session.value) return;
    await sessionStore.archive(session.value.id);
    session.value = { ...session.value, status: "archived" };
  }

  async function handleRestore() {
    if (!session.value) return;
    try {
      session.value = await sessionStore.restore(session.value.id);
    } catch (err) {
      console.error("Restore failed", err);
    }
  }

  onMounted(() => {
    applyDeepLinkTab();
    void loadSession();
  });

  return {
    router,
    session,
    loadingSession,
    sessionError,
    activeTab,
    focusToolId,
    handleArchive,
    handleRestore,
    timelinePanel
  };
}
