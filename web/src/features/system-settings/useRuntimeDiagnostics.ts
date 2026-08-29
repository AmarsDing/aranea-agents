import { onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { getDiagnostics } from './api';
import type { DiagnosticsItem } from './types';

// useRuntimeDiagnostics（79-runtime-governance R8 体检面板数据源，H6）：
// 状态与展示映射从面板组件抽离，便于复用与单测。未知 key/status 一律回退
// 原文/中立样式——后端新检查项或新状态先上线时前端不破版、不冒充 pass。
export function useRuntimeDiagnostics() {
  const { t } = useI18n();

  const items = ref<DiagnosticsItem[]>([]);
  const loading = ref(false);
  const error = ref('');
  const lastRunAt = ref('');

  // key → i18n 标签映射；未知 key 回退原文。
  const ITEM_LABEL_KEYS: Record<string, string> = {
    model_providers: 'settingsPage.diagnostics.itemModelProviders',
    mcp_servers: 'settingsPage.diagnostics.itemMcpServers',
    tool_assembly: 'settingsPage.diagnostics.itemToolAssembly',
    memory_stack: 'settingsPage.diagnostics.itemMemoryStack',
    cache_baseline: 'settingsPage.diagnostics.itemCacheBaseline',
    config_graph: 'settingsPage.diagnostics.itemConfigGraph',
  };

  function itemLabel(key: string): string {
    const labelKey = ITEM_LABEL_KEYS[key];
    return labelKey ? t(labelKey) : key;
  }

  // 未知 status（后端新增状态前端未识别）：help 图标 + grey 中立色 + 原文
  // 徽标——绝不落入 pass 分支冒充正常（审计 H6 修复点）。
  function isKnownStatus(status: string): boolean {
    return status === 'pass' || status === 'warn' || status === 'fail';
  }

  function statusIcon(status: string): string {
    if (!isKnownStatus(status)) return 'help';
    switch (status) {
      case 'fail':
        return 'error';
      case 'warn':
        return 'warning';
      default:
        return 'check_circle';
    }
  }

  function statusColor(status: string): string {
    if (!isKnownStatus(status)) return 'grey';
    switch (status) {
      case 'fail':
        return 'negative';
      case 'warn':
        return 'warning';
      default:
        return 'positive';
    }
  }

  async function load() {
    loading.value = true;
    error.value = '';
    try {
      const report = await getDiagnostics();
      items.value = report.items ?? [];
      lastRunAt.value = new Date().toLocaleString();
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  onMounted(load);

  return {
    items,
    loading,
    error,
    lastRunAt,
    load,
    itemLabel,
    statusIcon,
    statusColor,
  };
}
