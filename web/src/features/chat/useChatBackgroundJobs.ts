import { onMounted, onUnmounted, ref, watch, type Ref } from "vue";
import { listChatBackgroundJobs, type ChatBackgroundJobRow } from "./api";
import {
  acquireGlobalWsConsumer,
  releaseGlobalWsConsumer,
} from "./globalWsHub";
import type { Envelope } from "./envelope";

function isBackgroundJobRefreshEnvelope(env: Envelope, sessionId?: string): boolean {
  const md = env.metadata as Record<string, unknown> | undefined;
  if (!md?.background_job_refresh) return false;
  const sid = (env.session_id ?? "").trim();
  if (sessionId && sid && sid !== sessionId.trim()) return false;
  return true;
}

export function useChatBackgroundJobs(
  sessionId: Ref<string | undefined>,
  agentId: Ref<string | undefined>,
  refreshNonce?: Ref<number | undefined>
) {
  const loading = ref(false);
  const error = ref("");
  const rows = ref<ChatBackgroundJobRow[]>([]);
  let hubId: string | null = null;

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
  onMounted(() => {
    void load();
    hubId = acquireGlobalWsConsumer({
      channels: ["chat"],
      logEnabled: false,
      onEnvelope: (env) => {
        if (env.channel !== "chat") return;
        if (isBackgroundJobRefreshEnvelope(env, sessionId.value)) {
          void load();
        }
      },
    });
  });
  onUnmounted(() => {
    if (hubId) {
      releaseGlobalWsConsumer(hubId);
      hubId = null;
    }
  });

  const runningCount = () =>
    rows.value.filter((r) => ["running", "accepted", "async_queued", "queued"].includes(r.status)).length;

  return { loading, error, rows, load, runningCount };
}
