import { onMounted, reactive, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { hookRuleOf } from '../../components/hooks/hookTableUi';
import { useCallbackPointOptions } from '../callback/constants';
import { defaultHookRuleConfig, type HookRow, type HookRuleConfig } from '../hooks/types';
import { useHooksStore } from '../../stores/hooks';
import { useLocalPagination } from '../../composables/useLocalPagination';

export function useHooksPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const hooksStore = useHooksStore();
  const { hooks: storeRows, loading: storeLoading } = storeToRefs(hooksStore);
  const loading = storeLoading;
  const saving = ref(false);
  const error = ref('');
  const search = ref('');
  const filterPoint = ref('');
  const callbackPointOptions = useCallbackPointOptions();
  const editorOpen = ref(false);
  const editingId = ref('');
  const busyId = ref('');
  const form = reactive({
    key: '',
    name: '',
    description: '',
    enabled: true,
    sort_order: 0,
    rule: defaultHookRuleConfig() as HookRuleConfig,
  });

  function ruleOf(row: HookRow) {
    return hookRuleOf(row);
  }

  const {
    page,
    rowsPerPage: pageSize,
    filteredRows,
    pagedRows,
    totalPages: pageMax,
  } = useLocalPagination<HookRow>({
    rows: storeRows,
    filter: search,
    filterFn: (r, q) => {
      if (filterPoint.value) {
        const rule = ruleOf(r);
        if (rule.callback_point !== filterPoint.value) return false;
      }
      if (!q) return true;
      return r.name.toLowerCase().includes(q) || r.key.toLowerCase().includes(q);
    },
  });

  function resetFilters() {
    search.value = '';
    filterPoint.value = '';
    page.value = 1;
  }

  watch(filterPoint, () => {
    page.value = 1;
  });

  async function loadRows() {
    error.value = '';
    try {
      await hooksStore.loadHooks();
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  function openCreate() {
    editingId.value = '';
    form.key = '';
    form.name = '';
    form.description = '';
    form.enabled = true;
    form.sort_order = 0;
    form.rule = defaultHookRuleConfig();
    editorOpen.value = true;
  }

  function openEdit(row: HookRow) {
    editingId.value = row.id;
    form.key = row.key;
    form.name = row.name;
    form.description = row.description;
    form.enabled = row.enabled;
    form.sort_order = row.sort_order;
    form.rule = ruleOf(row);
    editorOpen.value = true;
  }

  async function saveHook() {
    if (!form.key.trim() || !form.name.trim()) {
      $q.notify({ type: 'warning', message: t('hooksPage.notifyRequired') });
      return;
    }
    saving.value = true;
    try {
      if (editingId.value) {
        await hooksStore.saveHook(editingId.value, {
          key: form.key,
          name: form.name,
          description: form.description,
          enabled: form.enabled,
          sort_order: form.sort_order,
          rule: form.rule,
        });
      } else {
        await hooksStore.addHook({
          key: form.key,
          name: form.name,
          description: form.description,
          enabled: form.enabled,
          sort_order: form.sort_order,
          rule: form.rule,
        });
      }
      editorOpen.value = false;
      $q.notify({ type: 'positive', message: t('hooksPage.notifySaved') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      saving.value = false;
    }
  }

  async function toggleEnabled(row: HookRow, enabled: boolean) {
    busyId.value = row.id;
    try {
      await hooksStore.saveHook(row.id, { enabled });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      busyId.value = '';
    }
  }

  function confirmDelete(row: HookRow) {
    $q.dialog({
      title: t('hooksPage.confirmDeleteTitle'),
      message: t('hooksPage.confirmDeleteMessage', { name: row.name }),
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      try {
        await hooksStore.removeHook(row.id);
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
    });
  }

  onMounted(loadRows);

  return {
    t,
    loading,
    saving,
    error,
    search,
    filterPoint,
    callbackPointOptions,
    editorOpen,
    editingId,
    busyId,
    form,
    page,
    pageSize,
    filteredRows,
    pagedRows,
    pageMax,
    resetFilters,
    loadRows,
    openCreate,
    openEdit,
    saveHook,
    toggleEnabled,
    confirmDelete,
  };
}
