import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listPlugins,
  togglePluginEnabled,
  updatePluginConfig,
  updatePluginScope,
  updatePluginSortOrder,
} from '../../features/plugins/api';
import type { Plugin, PluginListQuery } from '../../features/plugins/types';

export const usePluginsStore = defineStore('plugins', () => {
  const plugins = ref<Plugin[]>([]);
  const total = ref(0);

  async function loadPlugins(query?: PluginListQuery) {
    const result = await listPlugins(query);
    plugins.value = result.items ?? [];
    total.value = result.total ?? plugins.value.length;
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

  async function setScope(id: string, scope: string) {
    const updated = await updatePluginScope(id, scope);
    plugins.value = plugins.value.map((p) => (p.id === id ? updated : p));
    return updated;
  }

  async function bumpSort(id: string, sortOrder: number) {
    const updated = await updatePluginSortOrder(id, sortOrder);
    plugins.value = plugins.value.map((p) => (p.id === id ? updated : p));
    return updated;
  }

  return { plugins, total, loadPlugins, toggle, setConfig, setScope, bumpSort };
});
