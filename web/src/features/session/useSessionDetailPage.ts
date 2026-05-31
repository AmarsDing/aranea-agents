import { computed, onMounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import type { Session } from "./types";
import { downloadTextFile } from "./downloadExport";
import { useSessionStore } from "../../stores/session/index";
import { useSessionTimelinePanel } from "./useSessionTimelinePanel";

export function useSessionDetailPage() {
  const route = useRoute();
  const router = useRouter();
  const $q = useQuasar();
  const sessionStore = useSessionStore();

  const session = ref<Session | null>(null);
  const loadingSession = ref(true);
  const sessionError = ref("");
  const activeTab = ref("turns");
  const exporting = ref(false);

  const focusToolId = computed(() => {
    if (String(route.query.focus ?? "").trim() !== "tool") return "";
    return String(route.query.tool_id ?? "").trim();
  });

  const sessionId = computed(() => String(route.params.sessionId ?? "").trim());
  const showParticipants = computed(() => session.value?.owner_type === "team");

  const timelinePanel = useSessionTimelinePanel(sessionId);

  function applyDeepLinkTab() {
    if (focusToolId.value) {
      activeTab.value = "timeline";
    }
  }

  async function loadSession() {
    const id = sessionId.value;
    if (!id) {
      sessionError.value = "缺少会话 ID";
      loadingSession.value = false;
      return;
    }
    loadingSession.value = true;
    sessionError.value = "";
    try {
      session.value = await sessionStore.fetchSession(id);
    } catch (err) {
      sessionError.value = err instanceof Error ? err.message : "加载会话失败";
    } finally {
      loadingSession.value = false;
    }
  }

  async function handleArchive() {
    if (!session.value) return;
    const prev = session.value;
    session.value = { ...session.value, archived_at: new Date().toISOString() };
    try {
      await sessionStore.archive(prev.id);
      $q.notify({ type: "positive", message: "已归档" });
    } catch (err) {
      session.value = prev;
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "归档失败" });
    }
  }

  async function handleRestore() {
    if (!session.value) return;
    try {
      session.value = await sessionStore.restore(session.value.id);
      $q.notify({ type: "positive", message: "已恢复" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "恢复失败" });
    }
  }

  async function handleExport(format: "markdown" | "json") {
    const id = session.value?.id;
    if (!id || exporting.value) return;
    exporting.value = true;
    try {
      const payload = await sessionStore.exportSession(id, format);
      downloadTextFile(payload.content, payload.filename, payload.content_type);
      $q.notify({ type: "positive", message: format === "json" ? "JSON 已导出" : "Markdown 已导出" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "导出失败" });
    } finally {
      exporting.value = false;
    }
  }

  onMounted(() => {
    applyDeepLinkTab();
    void loadSession();
  });

  watch(sessionId, () => {
    void loadSession();
  });

  return {
    router,
    session,
    loadingSession,
    sessionError,
    activeTab,
    focusToolId,
    showParticipants,
    exporting,
    handleArchive,
    handleRestore,
    handleExport,
    timelinePanel,
  };
}
