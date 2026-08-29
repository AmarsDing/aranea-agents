import { onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ChannelTurnJobRow } from './types';
import { useChannelsStore } from '../../stores/channels';
import { channelTurnJobsColumns } from '../../components/channels/channelUi';

export function useChannelTurnJobsPanel(channelId: () => string) {
  const { t } = useI18n();
  const channelsStore = useChannelsStore();
  const loading = ref(false);
  const error = ref('');
  const rows = ref<ChannelTurnJobRow[]>([]);
  const columns = channelTurnJobsColumns(t);

  async function load() {
    const id = channelId();
    if (!id) return;
    loading.value = true;
    error.value = '';
    try {
      rows.value = await channelsStore.loadTurnJobs(id, 30);
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('channelEditor.loadFailed');
    } finally {
      loading.value = false;
    }
  }

  watch(channelId, () => void load(), { immediate: false });
  onMounted(() => void load());

  return { t, loading, error, rows, columns, load };
}
