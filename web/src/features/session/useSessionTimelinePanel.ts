import { computed, ref, watch, type Ref } from "vue";
import type { SessionTimeline } from "./types";
import { useSessionStore } from "../../stores/session";
import { buildTimelineStats } from "../../components/sessions/sessionTimelineUi";

const PAGE_SIZE = 100;

export function useSessionTimelinePanel(sessionId: Ref<string>) {
  const sessionStore = useSessionStore();
  const timeline = ref<SessionTimeline | null>(null);
  const loading = ref(false);
  const error = ref("");
  const kindFilter = ref<string | null>(null);
  const sortOrder = ref("desc");
  const offset = ref(0);
  const total = ref(0);

  const stats = computed(() => buildTimelineStats(timeline.value?.summary));
  const pageLabel = computed(
    () => `${offset.value + 1}-${Math.min(offset.value + PAGE_SIZE, total.value)} / ${total.value}`
  );

  async function loadTimeline() {
    const id = sessionId.value.trim();
    if (!id) {
      timeline.value = null;
      total.value = 0;
      return;
    }
    loading.value = true;
    error.value = "";
    try {
      timeline.value = await sessionStore.fetchTimeline(id, {
        limit: PAGE_SIZE,
        offset: offset.value,
        kind_filter: kindFilter.value || undefined,
        sort_order: sortOrder.value,
      });
      total.value = timeline.value.summary?.total ?? timeline.value.items.length;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载 Timeline 失败";
      timeline.value = null;
      total.value = 0;
    } finally {
      loading.value = false;
    }
  }

  function prevPage() {
    offset.value = Math.max(0, offset.value - PAGE_SIZE);
    void loadTimeline();
  }

  function nextPage() {
    offset.value += PAGE_SIZE;
    void loadTimeline();
  }

  watch([sessionId, kindFilter, sortOrder], () => {
    offset.value = 0;
    void loadTimeline();
  }, { immediate: true });

  return {
    timeline,
    loading,
    error,
    kindFilter,
    sortOrder,
    offset,
    total,
    pageSize: PAGE_SIZE,
    pageLabel,
    stats,
    loadTimeline,
    prevPage,
    nextPage,
  };
}
