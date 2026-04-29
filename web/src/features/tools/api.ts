import { api } from "../../api/http";
import type { AgentEffectiveTools, PaginatedResponse, Tool, ToolInvocation, ToolListQuery, ToolListResponse, ToolRunQuery, ToolUpsertInput } from "./types";

function compactParams(params: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== ""));
}

export async function listTools(query: ToolListQuery = {}): Promise<ToolListResponse> {
  const { data } = await api.get("/tools", {
    params: compactParams({
      search: query.search,
      category: query.category,
      source: query.source,
      risk_level: query.risk_level,
      enabled: query.enabled,
      page: query.page ?? 1,
      page_size: query.page_size ?? 20
    })
  });
  return data;
}

export async function getTool(id: string): Promise<Tool> {
  const { data } = await api.get(`/tools/${encodeURIComponent(id)}`);
  return data;
}

export async function createTool(input: ToolUpsertInput): Promise<Tool> {
  const { data } = await api.post("/tools", input);
  return data;
}

export async function updateTool(id: string, input: ToolUpsertInput): Promise<Tool> {
  const { data } = await api.put(`/tools/${encodeURIComponent(id)}`, input);
  return data;
}

export async function deleteTool(id: string): Promise<void> {
  await api.delete(`/tools/${encodeURIComponent(id)}`);
}

export async function toggleToolEnabled(id: string, enabled: boolean): Promise<Tool> {
  const { data } = await api.patch(`/tools/${encodeURIComponent(id)}/enabled`, { enabled });
  return data;
}

export async function listToolRunsForTool(id: string, query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>> {
  const { data } = await api.get(`/tools/${encodeURIComponent(id)}/runs`, {
    params: compactParams({
      agent_id: query.agent_id,
      session_id: query.session_id,
      status: query.status,
      from: query.from,
      to: query.to,
      page: query.page ?? 1,
      page_size: query.page_size ?? 20
    })
  });
  return data;
}

export async function listToolRuns(query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>> {
  const { data } = await api.get("/tools/runs", {
    params: compactParams({
      tool_key: query.tool_key,
      agent_id: query.agent_id,
      session_id: query.session_id,
      status: query.status,
      from: query.from,
      to: query.to,
      page: query.page ?? 1,
      page_size: query.page_size ?? 20
    })
  });
  return data;
}

export async function getAgentEffectiveTools(agentId: string): Promise<AgentEffectiveTools> {
  const { data } = await api.get(`/agents/${encodeURIComponent(agentId)}/tools/effective`);
  return data;
}
