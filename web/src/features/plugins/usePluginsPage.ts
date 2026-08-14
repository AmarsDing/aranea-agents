import { computed, onMounted, ref } from 'vue';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { usePluginsStore } from '../../stores/plugins';
import { useAgentsCatalogStore } from '../../stores/agents/catalog';
import { useListPage } from '../../composables/useListPage';
import type { Plugin } from './types';
import { PLUGIN_CATEGORY_OPTIONS, pluginEnabledOptions, prettyJSON } from '../../components/plugins/pluginUi';
import { useCallbackPointOptions } from '../callback/constants';

export function usePluginsPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const pluginsStore = usePluginsStore();
  const agentsCatalog = useAgentsCatalogStore();
  const { plugins: rows, total } = storeToRefs(pluginsStore);

  const search = ref('');
  const category = ref('');
  const enabled = ref<boolean | null>(null);
  const callbackPoint = ref('');
  const loading = ref(false);
  const error = ref('');
  const togglingId = ref('');

  const detailOpen = ref(false);
  const detailTarget = ref<Plugin | null>(null);
  const scopeMode = ref<'global' | 'agent'>('global');
  const scopeAgentId = ref('');
  const savingScope = ref(false);
  const bumpingSort = ref(false);

  const configOpen = ref(false);
  const configTarget = ref<Plugin | null>(null);
  const configText = ref('{}');
  const configMode = ref<'form' | 'json'>('form');
  const savingConfig = ref(false);

  const configError = computed(() => {
    try {
      JSON.parse(configText.value || '{}');
      return '';
    } catch (err) {
      return err instanceof Error ? err.message : t('pluginsPage.config.invalidJson');
    }
  });

  /** Agent 选项列表，供作用域选择器使用 */
  const agentOptions = ref<{ label: string; value: string }[]>([]);

  async function loadAgentOptions() {
    const agents = await agentsCatalog.fetchAgents({ limit: 200 });
    agentOptions.value = agents.map((a) => ({
      label: a.display_name || a.agent_key || a.id,
      value: a.id,
    }));
  }

  const { page, pageSize, pageMax } = useListPage({
    filters: [search, category, enabled, callbackPoint],
    total,
    load: loadRows,
  });

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      await pluginsStore.loadPlugins({
        search: search.value,
        category: category.value,
        enabled: enabled.value,
        callback_point: callbackPoint.value,
        page: page.value,
        page_size: pageSize.value,
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('pluginsPage.notifyLoadFailed');
    } finally {
      loading.value = false;
    }
  }

  function resetFilters() {
    search.value = '';
    category.value = '';
    enabled.value = null;
    callbackPoint.value = '';
    page.value = 1;
    // filter watch 会自动触发 loadRows
  }

  async function toggleEnabled(plugin: Plugin, next: boolean) {
    togglingId.value = plugin.id;
    try {
      await pluginsStore.toggle(plugin.id, next);
      // 带 enabled 筛选时就地更新会让行状态与筛选条件矛盾，改为重载列表
      if (enabled.value !== null) await loadRows();
      $q.notify({
        type: 'positive',
        message: next ? t('pluginsPage.notifyEnabled') : t('pluginsPage.notifyDisabled'),
      });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('pluginsPage.notifyUpdateFailed'),
      });
    } finally {
      togglingId.value = '';
    }
  }

  function openDetail(plugin: Plugin) {
    detailTarget.value = plugin;
    const isGlobal = !plugin.scope || plugin.scope === 'global';
    scopeMode.value = isGlobal ? 'global' : 'agent';
    scopeAgentId.value = isGlobal ? '' : plugin.scope;
    detailOpen.value = true;
  }

  function openConfig(plugin: Plugin) {
    configTarget.value = plugin;
    configText.value = prettyJSON(plugin.config_json || plugin.default_config_json || '{}', '{}');
    configMode.value = 'form';
    configOpen.value = true;
  }

  async function saveConfig() {
    if (!configTarget.value || configError.value) return;
    savingConfig.value = true;
    try {
      await pluginsStore.setConfig(configTarget.value.id, JSON.stringify(JSON.parse(configText.value)));
      configOpen.value = false;
      $q.notify({ type: 'positive', message: t('pluginsPage.notifyConfigSaved') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('pluginsPage.notifySaveFailed') });
    } finally {
      savingConfig.value = false;
    }
  }

  async function bumpSort(plugin: Plugin, delta: number) {
    bumpingSort.value = true;
    try {
      await pluginsStore.bumpSort(plugin.id, Math.max(0, plugin.sort_order + delta));
      // 排序后重载列表以获取正确的相对位置
      await loadRows();
      // 更新 detailTarget 为排序后最新数据
      const updated = rows.value.find((r) => r.id === plugin.id);
      if (updated && detailTarget.value?.id === updated.id) detailTarget.value = updated;
      $q.notify({ type: 'positive', message: t('pluginsPage.notifySortUpdated') });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('pluginsPage.notifyUpdateFailed'),
      });
    } finally {
      bumpingSort.value = false;
    }
  }

  async function saveScope() {
    if (!detailTarget.value) return;
    if (scopeMode.value === 'agent' && !scopeAgentId.value.trim()) {
      $q.notify({ type: 'warning', message: t('pluginsPage.notifySelectAgent') });
      return;
    }
    savingScope.value = true;
    try {
      const scope = scopeMode.value === 'global' ? 'global' : scopeAgentId.value.trim();
      await pluginsStore.setScope(detailTarget.value.id, scope);
      // 从 Store 同步最新数据到 detailTarget
      const updated = rows.value.find((r) => r.id === detailTarget.value!.id);
      if (updated) detailTarget.value = updated;
      $q.notify({ type: 'positive', message: t('pluginsPage.notifyScopeSaved') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('pluginsPage.notifySaveFailed') });
    } finally {
      savingScope.value = false;
    }
  }

  onMounted(() => {
    // 预加载 Agent 目录，供作用域选择器使用（loadRows 由 useListPage autoLoad 触发）
    void loadAgentOptions();
  });

  function onSchemaValidationError(message: string) {
    $q.notify({ type: 'warning', message });
  }

  return {
    rows,
    total,
    page,
    pageSize,
    pageMax,
    search,
    category,
    enabled,
    callbackPoint,
    loading,
    error,
    togglingId,
    detailOpen,
    detailTarget,
    scopeMode,
    scopeAgentId,
    savingScope,
    bumpingSort,
    configOpen,
    configTarget,
    configText,
    configMode,
    savingConfig,
    configError,
    agentOptions,
    categoryOptions: PLUGIN_CATEGORY_OPTIONS,
    enabledOptions: computed(() => pluginEnabledOptions(t)),
    callbackPointOptions: useCallbackPointOptions(),
    loadRows,
    resetFilters,
    toggleEnabled,
    openDetail,
    openConfig,
    saveConfig,
    bumpSort,
    saveScope,
    onSchemaValidationError,
  };
}
