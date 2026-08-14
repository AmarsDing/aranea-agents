import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listTools,
  getTool,
  createTool,
  updateTool,
  deleteTool,
  toggleToolEnabled,
  updateToolConfig,
  getAgentEffectiveTools,
  listToolAgentOverrides,
  listToolAgentOverridesByAgent,
  upsertToolAgentOverride,
  deleteToolAgentOverride,
  listToolRunsForTool,
  listToolRuns,
  listToolInvocationAudits,
  getToolInvocationParams,
  testTool,
} from '../../features/tools/api';
import type { ToolTestResult } from '../../features/tools/types';
import type {
  Tool,
  ToolListQuery,
  ToolUpsertInput,
  ToolListResponse,
  AgentEffectiveTools,
  ToolAgentOverride,
  ToolInvocation,
  ToolInvocationParamDetail,
  ToolRunQuery,
  ToolAuditQuery,
  ToolInvocationAudit,
  PaginatedResponse,
} from '../../features/tools/types';

export const useToolsStore = defineStore('tools', () => {
  const tools = ref<Tool[]>([]);
  const activeTool = ref<Tool | null>(null);
  const total = ref(0);
  const summary = ref<ToolListResponse['summary']>({
    total_tools: 0,
    enabled_tools: 0,
    high_risk_enabled: 0,
    calls_24h: 0,
    failure_rate_24h: 0,
  });
  const loading = ref(false);

  // 请求序号守卫：快速连续触发（筛选 watch + 手动刷新叠加）时丢弃过期响应，避免旧数据覆盖新筛选结果。
  let loadSeq = 0;

  async function loadTools(query?: ToolListQuery): Promise<ToolListResponse> {
    const seq = ++loadSeq;
    loading.value = true;
    try {
      const result: ToolListResponse = await listTools(query);
      if (seq !== loadSeq) return result;
      tools.value = result.items ?? [];
      total.value = result.total ?? tools.value.length;
      if (result.summary) summary.value = result.summary;
      return result;
    } finally {
      if (seq === loadSeq) loading.value = false;
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

  async function editToolConfig(id: string, configJson: string) {
    const updated = await updateToolConfig(id, configJson);
    tools.value = tools.value.map((t) => (t.id === id ? updated : t));
    if (activeTool.value?.id === id) activeTool.value = updated;
    return updated;
  }

  async function toggle(id: string, enabled: boolean, confirmIntent?: string) {
    const updated = await toggleToolEnabled(id, enabled, confirmIntent);
    tools.value = tools.value.map((t) => (t.id === id ? updated : t));
    return updated;
  }

  async function fetchOverrides(toolId: string): Promise<ToolAgentOverride[]> {
    return listToolAgentOverrides(toolId);
  }

  async function fetchOverridesByAgent(agentId: string): Promise<ToolAgentOverride[]> {
    return listToolAgentOverridesByAgent(agentId);
  }

  async function fetchEffectiveTools(agentId: string): Promise<AgentEffectiveTools> {
    return getAgentEffectiveTools(agentId);
  }

  async function fetchCatalog(query?: ToolListQuery): Promise<ToolListResponse> {
    return listTools(query);
  }

  async function saveOverride(input: {
    tool_id: string;
    agent_id: string;
    mode: string;
    enabled: boolean;
    requires_confirmation: boolean;
    config_override_json: string;
  }): Promise<ToolAgentOverride> {
    return upsertToolAgentOverride(input);
  }

  async function removeOverride(toolId: string, agentId: string): Promise<void> {
    return deleteToolAgentOverride(toolId, agentId);
  }

  async function fetchToolRuns(toolId: string, query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>> {
    return listToolRunsForTool(toolId, query);
  }

  async function loadToolRuns(query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>> {
    return listToolRuns(query);
  }

  async function fetchInvocationParams(invocationId: string): Promise<ToolInvocationParamDetail> {
    return getToolInvocationParams(invocationId);
  }

  async function loadToolAudits(query: ToolAuditQuery = {}): Promise<PaginatedResponse<ToolInvocationAudit>> {
    return listToolInvocationAudits(query);
  }

  async function runToolTest(toolId: string, argumentsJson: string, timeoutSec: number): Promise<ToolTestResult> {
    return testTool(toolId, argumentsJson, timeoutSec);
  }

  return {
    tools,
    activeTool,
    total,
    summary,
    loading,
    loadTools,
    fetchTool,
    addTool,
    editTool,
    editToolConfig,
    remove,
    toggle,
    fetchCatalog,
    fetchEffectiveTools,
    fetchOverrides,
    fetchOverridesByAgent,
    saveOverride,
    removeOverride,
    fetchToolRuns,
    loadToolRuns,
    fetchInvocationParams,
    loadToolAudits,
    runToolTest,
  };
});
