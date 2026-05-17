import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listTools, getTool, createTool, updateTool, deleteTool, toggleToolEnabled,
  listToolAgentOverrides, upsertToolAgentOverride, deleteToolAgentOverride, listToolRunsForTool
} from "../../features/tools/api";
import type {
  Tool, ToolListQuery, ToolUpsertInput, ToolListResponse,
  ToolAgentOverride, ToolInvocation, ToolRunQuery, PaginatedResponse
} from "../../features/tools/types";

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

  async function fetchOverrides(toolId: string): Promise<ToolAgentOverride[]> {
    return listToolAgentOverrides(toolId);
  }

  async function saveOverride(input: { tool_id: string; agent_id: string; mode: string; enabled: boolean; requires_confirmation: boolean; config_override_json: string }): Promise<ToolAgentOverride> {
    return upsertToolAgentOverride(input);
  }

  async function removeOverride(toolId: string, agentId: string): Promise<void> {
    return deleteToolAgentOverride(toolId, agentId);
  }

  async function fetchToolRuns(toolId: string, query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>> {
    return listToolRunsForTool(toolId, query);
  }

  return {
    tools, activeTool, total, loading,
    loadTools, fetchTool, addTool, editTool, remove, toggle,
    fetchOverrides, saveOverride, removeOverride, fetchToolRuns
  };
});
