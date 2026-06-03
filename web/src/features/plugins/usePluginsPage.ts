import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import { usePluginsStore } from '../../stores/plugins';
import type { Plugin } from './types';
import {
  PLUGIN_CATEGORY_OPTIONS,
  PLUGIN_ENABLED_OPTIONS,
  formatPluginDate,
  prettyJSON,
} from '../../components/plugins/pluginUi';
import { CALLBACK_POINT_OPTIONS } from '../callback/constants';

export function usePluginsPage() {
  const $q = useQuasar();
  const pluginsStore = usePluginsStore();
  const { plugins: storePlugins, total: storeTotal } = storeToRefs(pluginsStore);

  const rows = ref<Plugin[]>([]);
  const total = ref(0);
  const page = ref(1);
  const pageSize = ref(20);
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

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
      return err instanceof Error ? err.message : 'JSON 格式错误';
    }
  });

  async function loadRows(nextPage = page.value, nextPageSize = pageSize.value) {
    loading.value = true;
    error.value = '';
    try {
      await pluginsStore.loadPlugins({
        search: search.value,
        category: category.value,
        enabled: enabled.value,
        callback_point: callbackPoint.value,
        page: nextPage,
        page_size: nextPageSize,
      });
      rows.value = [...storePlugins.value];
      total.value = storeTotal.value;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 Plugin 失败';
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
    void loadRows(1, pageSize.value);
  }

  async function toggleEnabled(plugin: Plugin, next: boolean) {
    togglingId.value = plugin.id;
    try {
      const updated = await pluginsStore.toggle(plugin.id, next);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
      $q.notify({ type: 'positive', message: next ? 'Plugin 已启用' : 'Plugin 已停用' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '更新失败' });
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
      const updated = await pluginsStore.setConfig(configTarget.value.id, JSON.stringify(JSON.parse(configText.value)));
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
      configOpen.value = false;
      $q.notify({ type: 'positive', message: 'Plugin 配置已保存' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
    } finally {
      savingConfig.value = false;
    }
  }

  async function bumpSort(plugin: Plugin, delta: number) {
    bumpingSort.value = true;
    try {
      const updated = await pluginsStore.bumpSort(plugin.id, Math.max(0, plugin.sort_order + delta));
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
      if (detailTarget.value?.id === updated.id) detailTarget.value = updated;
      $q.notify({ type: 'positive', message: '执行顺序已更新' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '更新失败' });
    } finally {
      bumpingSort.value = false;
    }
  }

  async function saveScope() {
    if (!detailTarget.value) return;
    if (scopeMode.value === 'agent' && !scopeAgentId.value.trim()) {
      $q.notify({ type: 'warning', message: '请输入 Agent ID' });
      return;
    }
    savingScope.value = true;
    try {
      const scope = scopeMode.value === 'global' ? 'global' : scopeAgentId.value.trim();
      const updated = await pluginsStore.setScope(detailTarget.value.id, scope);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
      detailTarget.value = updated;
      $q.notify({ type: 'positive', message: '作用域已保存，下次对话生效' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
    } finally {
      savingScope.value = false;
    }
  }

  watch([search, category, enabled, callbackPoint], () => {
    if (page.value === 1) {
      void loadRows(1, pageSize.value);
    } else {
      page.value = 1;
    }
  });

  watch([page, pageSize], () => void loadRows());

  onMounted(() => loadRows());

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
    categoryOptions: PLUGIN_CATEGORY_OPTIONS,
    enabledOptions: PLUGIN_ENABLED_OPTIONS,
    callbackPointOptions: CALLBACK_POINT_OPTIONS,
    formatDate: formatPluginDate,
    prettyJSON,
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
