import { onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { ChannelTurnJobRow } from "./types";
import { useChannelsStore } from "../../stores/channels";
import { registryColWidth } from "../ui/registryTableColumns";

export function useChannelTurnJobsPanel(channelId: () => string) {
  const { t } = useI18n();
  const channelsStore = useChannelsStore();
  const loading = ref(false);
  const error = ref("");
  const rows = ref<ChannelTurnJobRow[]>([]);

  const columns = [
    { name: "status", label: "status", field: "status", align: "left" as const, ...registryColWidth("9%") },
    { name: "peer_id", label: "peer_id", field: "peer_id", align: "left" as const, ...registryColWidth("14%") },
    { name: "session_id", label: "session_id", field: "session_id", align: "left" as const, ...registryColWidth("14%") },
    { name: "updated_at", label: "updated_at", field: "updated_at", align: "left" as const, ...registryColWidth("11%") }
  ];

  function statusColor(status: string) {
    switch (status) {
      case "running":
      case "accepted":
        return "info";
      case "completed":
        return "positive";
      case "timeout":
      case "failed":
        return "negative";
      case "cancelled":
        return "warning";
      case "async_queued":
      case "queued":
        return "purple";
      default:
        return "grey";
    }
  }

  async function load() {
    const id = channelId();
    if (!id) return;
    loading.value = true;
    error.value = "";
    try {
      rows.value = await channelsStore.loadTurnJobs(id, 30);
    } catch (err) {
      error.value = err instanceof Error ? err.message : "load failed";
    } finally {
      loading.value = false;
    }
  }

  watch(channelId, () => void load(), { immediate: false });
  onMounted(() => void load());

  return { t, loading, error, rows, columns, statusColor, load };
}
