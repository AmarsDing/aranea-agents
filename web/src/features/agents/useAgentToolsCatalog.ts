import { computed, ref } from 'vue';
import { listTools } from '../tools/api';
import { toolGroupOptions, type AgentRuntimeConfigForm } from './agentRuntimeConfig';

const defaultNativeToolKeys = [
  'datetime',
  'web_search',
  'web_fetch',
  'list_files',
  'read_file',
  'write_file',
  'edit_file',
  'shell_exec',
];

/** Tool catalog loading and select options for Agent settings. */
export function useAgentToolsCatalog(config: AgentRuntimeConfigForm) {
  const catalogTools = ref<{ key: string; display_name: string }[]>([]);
  const loadingCatalogTools = ref(false);

  async function loadCatalogTools() {
    loadingCatalogTools.value = true;
    try {
      const res = await listTools({ page: 1, page_size: 500 });
      catalogTools.value = (res.items ?? [])
        .map((t) => ({
          key: String(t.key ?? '').trim(),
          display_name: String(t.display_name ?? '').trim() || String(t.key ?? '').trim(),
        }))
        .filter((t) => t.key !== '');
    } catch {
      catalogTools.value = [];
    } finally {
      loadingCatalogTools.value = false;
    }
  }

  const toolSelectOptions = computed(() => {
    const byKey = new Map<string, { label: string; value: string }>();
    // group:<name> 组 key（后端 expandToolGroup 支持），排在最前便于按组授权。
    for (const g of toolGroupOptions) {
      byKey.set(g.value, { label: g.label, value: g.value });
    }
    for (const k of defaultNativeToolKeys) {
      byKey.set(k, { label: `${k} · 内置`, value: k });
    }
    for (const t of catalogTools.value) {
      const label = t.display_name && t.display_name !== t.key ? `${t.display_name} (${t.key})` : t.key;
      byKey.set(t.key, { label, value: t.key });
    }
    const extra = [...config.tools.allow, ...config.tools.deny, ...config.tools.concurrent_allow];
    for (const raw of extra) {
      const key = String(raw ?? '').trim();
      if (key && !byKey.has(key)) {
        byKey.set(key, { label: `${key} · 已保存`, value: key });
      }
    }
    return Array.from(byKey.values()).sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'));
  });

  const toolConflicts = computed(() => config.tools.allow.filter((tool) => config.tools.deny.includes(tool)));

  return {
    catalogTools,
    loadingCatalogTools,
    loadCatalogTools,
    toolSelectOptions,
    toolConflicts,
  };
}
