import { onMounted, ref, watch, type Ref } from "vue";
import { listChatBackgroundJobs, type ChatBackgroundJobRow } from "./api";

export function useChatBackgroundJobs(
  sessionId: Ref<string | undefined>,
  agentId: Ref<string | undefined>,
  refreshNonce?: Ref<number | undefined>
) {
  const loading = ref(false);
  const error = ref("");
  const rows = ref<ChatBackgroundJobRow[]>([]);

  async function load() {
    const sid = sessionId.value?.trim();
    const aid = agentId.value?.trim();
    if (!sid && !aid) {
      rows.value = [];
      return;
    }
    loading.value = true;
    error.value = "";
    try {
      rows.value = await listChatBackgroundJobs({
        sessionId: sid,
        agentId: !sid ? aid : undefined,
        limit: 50,
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : "load failed";
    } finally {
      loading.value = false;
    }
  }

  watch([sessionId, agentId], () => void load(), { immediate: false });
  if (refreshNonce) {
    watch(refreshNonce, () => void load());
  }
  onMounted(() => void load());

  const runningCount = () =>
    rows.value.filter((r) => ["running", "accepted", "async_queued", "queued"].includes(r.status)).length;

  return { loading, error, rows, load, runningCount };
}
