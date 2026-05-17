import { defineStore } from "pinia";
import { ref } from "vue";
import { listTools, getTool, createTool, updateTool, deleteTool, toggleToolEnabled } from "../../features/tools/api";
import type { Tool, ToolListQuery, ToolUpsertInput, ToolListResponse } from "../../features/tools/types";

export const useToolsStore = defineStore("tools", () => {
  const tools = ref<Tool[]>([]);
  const activeTool = ref<Tool | null>(null);
  const total = ref(0);
  const loading = ref(false);

  async function loadTools(query?: ToolListQuery) {
    loading.value = true;
    try {
      const result: ToolListResponse = await listTools(query);
      tools.value = result.items ?? [];
      total.value = result.total ?? tools.value.length;
    } finally {
      loading.value = false;
    }
  }

  async function fetchTool(id: string) {
    activeTool.value = await getTool(id);
    return activeTool.value;
  }

  async function addTool(input: ToolUpsertInput) {
    const created = await createTool(input);
    tools.value.unshift(created);
    return created;
  }

  async function editTool(id: string, input: ToolUpsertInput) {
    const updated = await updateTool(id, input);
    tools.value = tools.value.map((t) => (t.id === id ? updated : t));
    if (activeTool.value?.id === id) activeTool.value = updated;
    return updated;
  }

  async function remove(id: string) {
    await deleteTool(id);
    tools.value = tools.value.filter((t) => t.id !== id);
    if (activeTool.value?.id === id) activeTool.value = null;
  }

  async function toggle(id: string, enabled: boolean, confirmKey?: string) {
    const updated = await toggleToolEnabled(id, enabled, confirmKey);
    tools.value = tools.value.map((t) => (t.id === id ? updated : t));
    return updated;
  }

  return { tools, activeTool, total, loading, loadTools, fetchTool, addTool, editTool, remove, toggle };
});
