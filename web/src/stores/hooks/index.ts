import { defineStore } from 'pinia';
import { ref } from 'vue';
import { createHook, deleteHook, listHooks, listHooksPaged, updateHook, type HookListQuery } from '../../features/hooks/api';
import { listHookDeliveries, type HookDeliveryListQuery, type HookDeliveryRow } from '../../features/hooks/deliveries';
import type { HookRow, HookRuleConfig } from '../../features/hooks/types';

export const useHooksStore = defineStore('hooks', () => {
  const hooks = ref<HookRow[]>([]);
  const total = ref(0);
  const loading = ref(false);

  const deliveries = ref<HookDeliveryRow[]>([]);
  const deliveriesTotal = ref(0);
  const deliveriesLoading = ref(false);

  async function loadHooks(query?: HookListQuery): Promise<HookRow[]> {
    loading.value = true;
    try {
      if (query) {
        const result = await listHooksPaged(query);
        hooks.value = result.items;
        total.value = result.total;
        return hooks.value;
      }
      hooks.value = await listHooks();
      total.value = hooks.value.length;
      return hooks.value;
    } finally {
      loading.value = false;
    }
  }

  async function addHook(input: {
    key: string;
    name: string;
    description?: string;
    enabled?: boolean;
    sort_order?: number;
    rule: HookRuleConfig;
  }): Promise<HookRow> {
    const created = await createHook(input);
    hooks.value = [created, ...hooks.value.filter((row) => row.id !== created.id)];
    total.value += 1;
    return created;
  }

  async function saveHook(
    id: string,
    patch: Partial<Pick<HookRow, 'key' | 'name' | 'description' | 'enabled' | 'sort_order' | 'status'>> & {
      rule?: HookRuleConfig;
    },
  ): Promise<HookRow> {
    const updated = await updateHook(id, patch);
    hooks.value = hooks.value.map((row) => (row.id === updated.id ? updated : row));
    return updated;
  }

  async function removeHook(id: string): Promise<void> {
    await deleteHook(id);
    hooks.value = hooks.value.filter((row) => row.id !== id);
    total.value = Math.max(0, total.value - 1);
  }

  async function loadDeliveries(
    query: HookDeliveryListQuery = {},
  ): Promise<{ items: HookDeliveryRow[]; total: number }> {
    deliveriesLoading.value = true;
    try {
      const data = await listHookDeliveries(query);
      deliveries.value = data.items;
      deliveriesTotal.value = data.total;
      return data;
    } finally {
      deliveriesLoading.value = false;
    }
  }

  return {
    hooks,
    total,
    loading,
    loadHooks,
    addHook,
    saveHook,
    removeHook,
    deliveries,
    deliveriesTotal,
    deliveriesLoading,
    loadDeliveries,
  };
});
