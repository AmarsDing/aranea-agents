import { defineStore } from "pinia";
import { ref } from "vue";
import { createHook, deleteHook, listHooks, updateHook } from "../../features/hooks/api";
import type { HookRow, HookRuleConfig } from "../../features/hooks/types";

export const useHooksStore = defineStore("hooks", () => {
  const hooks = ref<HookRow[]>([]);
  const loading = ref(false);

  async function loadHooks(): Promise<HookRow[]> {
    loading.value = true;
    try {
      hooks.value = await listHooks();
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
    return created;
  }

  async function saveHook(
    id: string,
    patch: Partial<Pick<HookRow, "key" | "name" | "description" | "enabled" | "sort_order" | "status">> & {
      rule?: HookRuleConfig;
    }
  ): Promise<HookRow> {
    const updated = await updateHook(id, patch);
    hooks.value = hooks.value.map((row) => (row.id === updated.id ? updated : row));
    return updated;
  }

  async function removeHook(id: string): Promise<void> {
    await deleteHook(id);
    hooks.value = hooks.value.filter((row) => row.id !== id);
  }

  return { hooks, loading, loadHooks, addHook, saveHook, removeHook };
});
