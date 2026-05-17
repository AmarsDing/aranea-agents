import { defineStore } from "pinia";
import { ref } from "vue";
import { listPlugins, togglePluginEnabled, updatePluginConfig } from "../../features/plugins/api";
import type { Plugin, PluginListQuery, PaginatedResponse } from "../../features/plugins/types";

export const usePluginsStore = defineStore("plugins", () => {
  const plugins = ref<Plugin[]>([]);
  const total = ref(0);
  const loading = ref(false);

  async function loadPlugins(query?: PluginListQuery) {
    loading.value = true;
    try {
      const result: PaginatedResponse<Plugin> = await listPlugins(query);
      plugins.value = result.items ?? [];
      total.value = result.total ?? plugins.value.length;
    } finally {
      loading.value = false;
    }
  }

  async function toggle(id: string, enabled: boolean) {
    const updated = await togglePluginEnabled(id, enabled);
    plugins.value = plugins.value.map((p) => (p.id === id ? updated : p));
    return updated;
  }

  async function setConfig(id: string, configJSON: string) {
    const updated = await updatePluginConfig(id, configJSON);
    plugins.value = plugins.value.map((p) => (p.id === id ? updated : p));
    return updated;
  }

  return { plugins, total, loading, loadPlugins, toggle, setConfig };
});
