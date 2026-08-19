import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import { copyChannelWebhookURL } from '../../components/channels/channelUi';
import { useChannelsStore } from '../../stores/channels';
import { useListPage } from '../../composables/useListPage';
import type { ChannelCredential, ChannelRow } from './types';

export function useChannelsPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const channelsStore = useChannelsStore();
  const { channels: rows, total, catalog, loading } = storeToRefs(channelsStore);

  const error = ref('');
  const search = ref('');
  const typeFilter = ref('');
  const statusFilter = ref('');
  const togglingId = ref('');
  const testingId = ref('');
  const editorOpen = ref(false);
  const editingRow = ref<ChannelRow | null>(null);
  const editingCredentials = ref<ChannelCredential[]>([]);
  const opsChannelId = ref('');

  const typeOptions = computed(() => catalog.value.map((item) => ({ label: item.label, value: item.type })));
  const statusOptions = computed(() => [
    { label: t('channelsPage.enabled'), value: 'enabled' },
    { label: t('channelsPage.disabled'), value: 'disabled' },
    { label: t('channelsPage.statusActive'), value: 'active' },
    { label: t('channelsPage.statusPendingAuth'), value: 'pending_auth' },
    { label: t('channelsPage.statusError'), value: 'error' },
  ]);

  const pagedRows = computed(() => rows.value);
  const opsChannel = computed(() => rows.value.find((row) => row.id === opsChannelId.value) ?? null);

  const { page, pageSize, pageMax } = useListPage({
    filters: [search, typeFilter, statusFilter],
    total,
    load: loadAll,
  });

  async function loadAll() {
    error.value = '';
    try {
      await channelsStore.loadAll({
        page: page.value,
        page_size: pageSize.value,
        search: search.value,
        type: typeFilter.value,
        status: statusFilter.value,
      });
      if (page.value > pageMax.value) page.value = pageMax.value;
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('channelsPage.loadFailed');
    }
  }

  function resetFilters() {
    search.value = '';
    typeFilter.value = '';
    statusFilter.value = '';
    page.value = 1;
    // filter watch 会自动触发 loadAll
  }

  function openCreate() {
    editingRow.value = null;
    editingCredentials.value = [];
    editorOpen.value = true;
  }

  async function openEdit(row: ChannelRow) {
    try {
      editingCredentials.value = await channelsStore.fetchCredentials(row.id);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.loadFailed') });
      return;
    }
    editingRow.value = row;
    editorOpen.value = true;
  }

  function onSaved(_row: ChannelRow) {
    void loadAll();
  }

  function openOps(row: ChannelRow) {
    opsChannelId.value = row.id;
  }

  function closeOps() {
    opsChannelId.value = '';
  }

  async function toggleRow(row: ChannelRow, enabled: boolean) {
    togglingId.value = row.id;
    try {
      await channelsStore.toggle(row.id, enabled);
      $q.notify({
        type: 'positive',
        message: enabled ? t('channelsPage.toggleOkEnabled') : t('channelsPage.toggleOkDisabled'),
      });
      await loadAll();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.toggleFailed') });
    } finally {
      togglingId.value = '';
    }
  }

  async function testRow(row: ChannelRow) {
    testingId.value = row.id;
    try {
      const result = await channelsStore.testConnection(row.id);
      $q.notify({ type: result.ok ? 'positive' : 'warning', message: result.message || result.status });
      await loadAll();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.testFailed') });
    } finally {
      testingId.value = '';
    }
  }

  async function copyWebhook(row: ChannelRow) {
    try {
      const url = await copyChannelWebhookURL(row);
      $q.notify({ type: 'positive', message: t('channelsPage.copyWebhookOk', { url }) });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('channelsPage.copyWebhookFailed'),
      });
    }
  }

  function confirmDelete(row: ChannelRow) {
    $q.dialog({
      title: t('channelsPage.deleteTitle'),
      message: t('channelsPage.deleteMessage'),
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      try {
        await channelsStore.removeChannel(row.id);
        if (opsChannelId.value === row.id) {
          closeOps();
        }
        $q.notify({ type: 'positive', message: t('channelsPage.deleteOk') });
        await loadAll();
      } catch (err) {
        $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.deleteFailed') });
      }
    });
  }

  return {
    t,
    catalog,
    pagedRows,
    page,
    pageSize,
    pageMax,
    total,
    loading,
    error,
    search,
    typeFilter,
    statusFilter,
    typeOptions,
    statusOptions,
    togglingId,
    testingId,
    editorOpen,
    editingRow,
    editingCredentials,
    opsChannel,
    resetFilters,
    loadAll,
    openCreate,
    openEdit,
    openOps,
    closeOps,
    onSaved,
    toggleRow,
    testRow,
    copyWebhook,
    confirmDelete,
  };
}
