import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import { usePluginsStore } from '../../stores/plugins';
import { useAgentsCatalogStore } from '../../stores/agents/catalog';
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
  const agentsCatalog = useAgentsCatalogStore();
  const { plugins: rows, total } = storeToRefs(pluginsStore);

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

  /** Agent 选项列表，供作用域选择器使用 */
  const agentOptions = ref<{ label: string; value: string }[]>([]);

  async function loadAgentOptions() {
    const agents = await agentsCatalog.fetchAgents({ limit: 200 });
    agentOptions.value = agents.map((a) => ({
      label: a.display_name || a.agent_key || a.id,
      value: a.id,
    }));
  }

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
      await pluginsStore.toggle(plugin.id, next);
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
      await pluginsStore.setConfig(configTarget.value.id, JSON.stringify(JSON.parse(configText.value)));
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
      await pluginsStore.bumpSort(plugin.id, Math.max(0, plugin.sort_order + delta));
      // 排序后重载列表以获取正确的相对位置
      await loadRows();
      // 更新 detailTarget 为排序后最新数据
      const updated = rows.value.find((r) => r.id === plugin.id);
      if (updated && detailTarget.value?.id === updated.id) detailTarget.value = updated;
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
      $q.notify({ type: 'warning', message: '请选择 Agent' });
      return;
    }
    savingScope.value = true;
    try {
      const scope = scopeMode.value === 'global' ? 'global' : scopeAgentId.value.trim();
      await pluginsStore.setScope(detailTarget.value.id, scope);
      // 从 Store 同步最新数据到 detailTarget
      const updated = rows.value.find((r) => r.id === detailTarget.value!.id);
      if (updated) detailTarget.value = updated;
      $q.notify({ type: 'positive', message: '作用域已保存，下次对话生效' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
    } finally {
      savingScope.value = false;
    }
  }

  // 筛选条件变更时：先重置页码，再在 nextTick 后加载（避免竞态）
  watch([search, category, enabled, callbackPoint], async () => {
    if (page.value !== 1) {
      page.value = 1;
      // page 变更会触发下方 watch，由那边执行 loadRows
      return;
    }
    await nextTick();
    void loadRows(1, pageSize.value);
  });

  watch([page, pageSize], () => void loadRows());

  onMounted(() => {
    void loadRows();
    // 预加载 Agent 目录，供作用域选择器使用
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
