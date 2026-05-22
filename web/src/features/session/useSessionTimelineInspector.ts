import { ref, watch, type Ref } from "vue";
import type { SessionTimeline } from "./types";
import { useSessionStore } from "../../stores/session/index";

export function useSessionTimelineInspector(opts: {
  open: Ref<boolean>;
  sessionId: Ref<string | null | undefined>;
  traceTabActive: Ref<boolean>;
}) {
  const sessionStore = useSessionStore();
  const timeline = ref<SessionTimeline | null>(null);
  const loading = ref(false);
  const error = ref("");

  async function loadTimeline() {
    const sessionId = String(opts.sessionId.value ?? "").trim();
    if (!opts.open.value || !sessionId || !opts.traceTabActive.value) return;
    loading.value = true;
    error.value = "";
    try {
      timeline.value = await sessionStore.fetchTimeline(sessionId);
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to load session trace.";
      timeline.value = null;
    } finally {
      loading.value = false;
    }
  }

  watch(
    () => [opts.open.value, opts.sessionId.value, opts.traceTabActive.value] as const,
    () => void loadTimeline(),
    { immediate: true },
  );

  return { timeline, loading, error, reload: loadTimeline };
}
