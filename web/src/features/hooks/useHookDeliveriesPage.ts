import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useHooksStore } from '../../stores/hooks';
import type { HookDeliveryRow } from './deliveries';

export function useHookDeliveriesPage() {
  const { t } = useI18n();
  const route = useRoute();
  const hooksStore = useHooksStore();
  const { deliveries: rows, deliveriesTotal: total, deliveriesLoading: loading } = storeToRefs(hooksStore);

  const hookKey = ref('');
  const status = ref('');
  const from = ref('');
  const to = ref('');
  const error = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const detailOpen = ref(false);
  const detailText = ref('');
  const detailUrl = ref('');
  const detailError = ref('');

  const statusOptions = computed(() => [
    { label: t('hooksPage.deliveries.statusPending'), value: 'pending' },
    { label: t('hooksPage.deliveries.statusSuccess'), value: 'success' },
    { label: t('hooksPage.deliveries.statusFailed'), value: 'failed' },
  ]);

  function toRFC3339(local: string): string | undefined {
    const val = local.trim();
    if (!val) return undefined;
    const d = new Date(val);
    if (Number.isNaN(d.getTime())) return undefined;
    return d.toISOString();
  }

  async function loadRows(nextPage = page.value, nextPageSize = pageSize.value) {
    error.value = '';
    try {
      await hooksStore.loadDeliveries({
        hook_key: hookKey.value.trim() || undefined,
        status: status.value || undefined,
        from: toRFC3339(from.value),
        to: toRFC3339(to.value),
        page: nextPage,
        page_size: nextPageSize,
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('hooksPage.deliveries.loadFailed');
    }
  }

  function onFilterChange() {
    page.value = 1;
    void loadRows(1, pageSize.value);
  }

  watch([page, pageSize], () => void loadRows());

  function resetFilters() {
    hookKey.value = '';
    status.value = '';
    from.value = '';
    to.value = '';
    page.value = 1;
    void loadRows(1, pageSize.value);
  }

  function openDetail(row: HookDeliveryRow) {
    detailUrl.value = row.webhook_url;
    detailError.value = row.last_error;
    detailText.value = row.payload_json?.trim() ? row.payload_json : JSON.stringify(row, null, 2);
    detailOpen.value = true;
  }

  onMounted(() => {
    const qk = route.query.hook_key;
    if (typeof qk === 'string' && qk.trim()) hookKey.value = qk.trim();
    void loadRows();
  });

  return {
    rows,
    total,
    loading,
    hookKey,
    status,
    from,
    to,
    error,
    page,
    pageSize,
    pageMax,
    detailOpen,
    detailText,
    detailUrl,
    detailError,
    statusOptions,
    resetFilters,
    loadRows,
    onFilterChange,
    openDetail,
  };
}
