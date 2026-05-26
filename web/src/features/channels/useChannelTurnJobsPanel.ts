import { onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { ChannelTurnJobRow } from "./types";
import { useChannelsStore } from "../../stores/channels";
import { CHANNEL_TURN_JOBS_TABLE_COLUMNS } from "../../components/channels/channelUi";

export function useChannelTurnJobsPanel(channelId: () => string) {
  const { t } = useI18n();
  const channelsStore = useChannelsStore();
  const loading = ref(false);
  const error = ref("");
  const rows = ref<ChannelTurnJobRow[]>([]);
  const columns = CHANNEL_TURN_JOBS_TABLE_COLUMNS;

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
