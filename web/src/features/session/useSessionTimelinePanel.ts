import { computed, ref, watch, type Ref } from "vue";
import type { SessionTimeline } from "./types";
import { useSessionStore } from "../../stores/session/index";
import { buildTimelineStats, filterTimelineItems } from "../../components/sessions/sessionTimelineUi";

export function useSessionTimelinePanel(sessionId: Ref<string>) {
  const sessionStore = useSessionStore();
  const timeline = ref<SessionTimeline | null>(null);
  const loading = ref(false);
  const error = ref("");
  const kindFilter = ref<string | null>(null);
  const sortOrder = ref("desc");

  const stats = computed(() => buildTimelineStats(timeline.value?.summary));

  const filteredItems = computed(() =>
    filterTimelineItems(timeline.value?.items ?? [], kindFilter.value, sortOrder.value),
  );

  async function loadTimeline() {
    const id = sessionId.value.trim();
    if (!id) {
      timeline.value = null;
      return;
    }
    loading.value = true;
    error.value = "";
    try {
      timeline.value = await sessionStore.fetchTimeline(id);
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载 Timeline 失败";
      timeline.value = null;
    } finally {
      loading.value = false;
    }
  }

  watch(sessionId, () => void loadTimeline(), { immediate: true });

  return {
    timeline,
    loading,
    error,
    kindFilter,
    sortOrder,
    stats,
    filteredItems,
    loadTimeline,
  };
}
