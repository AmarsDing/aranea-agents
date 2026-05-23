import { onMounted, ref, watch, type Ref } from "vue";
import { listTaskDeadLetters, resolveTaskDeadLetter, type TaskDeadLetterRow } from "../teams/api";

export function useTaskDeadLetters(
  sessionId: Ref<string | undefined>,
  refreshNonce?: Ref<number | undefined>
) {
  const loading = ref(false);
  const error = ref("");
  const rows = ref<TaskDeadLetterRow[]>([]);

  async function load() {
    const sid = sessionId.value?.trim();
    if (!sid) {
      rows.value = [];
      return;
    }
    loading.value = true;
    error.value = "";
    try {
      rows.value = await listTaskDeadLetters({ sessionId: sid, status: "pending", limit: 50 });
    } catch (err) {
      error.value = err instanceof Error ? err.message : "load failed";
    } finally {
      loading.value = false;
    }
  }

  async function resolve(id: string) {
    await resolveTaskDeadLetter(id);
    await load();
  }

  watch(sessionId, () => void load(), { immediate: false });
  if (refreshNonce) {
    watch(refreshNonce, () => void load());
  }
  onMounted(() => void load());

  const pendingCount = () => rows.value.filter((r) => r.status === "pending").length;

  return { loading, error, rows, load, resolve, pendingCount };
}
