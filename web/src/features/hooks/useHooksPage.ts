import { computed, onMounted, reactive, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useCallbackPointOptions } from '../callback/constants';
import { defaultHookRuleConfig, parseHookConfig, type HookRow, type HookRuleConfig } from '../hooks/types';
import { useHooksStore } from '../../stores/hooks';

export function useHooksPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const hooksStore = useHooksStore();
  const { hooks: storeRows, total, loading: storeLoading } = storeToRefs(hooksStore);
  const loading = storeLoading;
  const saving = ref(false);
  const error = ref('');
  const search = ref('');
  const filterPoint = ref('');
  const page = ref(1);
  const pageSize = ref(20);
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
  const formValid = ref(true);

  function ruleOf(row: HookRow) {
    return parseHookConfig(row.config_json);
  }

  const filteredRows = computed(() => storeRows.value);
  const pagedRows = computed(() => storeRows.value);
  const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, total.value) / pageSize.value)));

  async function loadRows() {
    error.value = '';
    try {
      await hooksStore.loadHooks({
        page: page.value,
        page_size: pageSize.value,
        search: search.value,
        callback_point: filterPoint.value,
      });
      if (page.value > pageMax.value) page.value = pageMax.value;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  function resetFilters() {
    search.value = '';
    filterPoint.value = '';
    page.value = 1;
    void loadRows();
  }

  let skipNextPageWatch = false;
  watch([search, filterPoint], () => {
    if (page.value !== 1) {
      skipNextPageWatch = true;
      page.value = 1;
    }
    void loadRows();
  });
  watch([page, pageSize], () => {
    if (skipNextPageWatch) {
      skipNextPageWatch = false;
      return;
    }
    void loadRows();
  });

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
    if (!formValid.value) {
      $q.notify({ type: 'warning', message: t('hooksPage.notifyFixErrors') });
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
      await loadRows();
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
        await loadRows();
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
    formValid,
    page,
    pageSize,
    total,
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
