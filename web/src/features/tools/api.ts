import { createToolService, legacyRestApi } from "../../services/index";
import type {
  Tool as KratosTool,
  ToolInvocation as KratosInvocation,
  ToolSummary as KratosSummary
} from "../../services/kratos/tool/v1/index";
import type {
  AgentEffectiveTools,
  PaginatedResponse,
  Tool,
  ToolInvocation,
  ToolListQuery,
  ToolListResponse,
  ToolRunQuery,
  ToolUpsertInput
} from "./types";

const toolApi = createToolService();

function enabledFilter(enabled: ToolListQuery["enabled"]): string | undefined {
  if (enabled === true) {
    return "true";
  }
  if (enabled === false) {
    return "false";
  }
  return undefined;
}

function kratosToolToLegacy(t: KratosTool): Tool {
  return {
    id: t.id ?? "",
    key: t.key ?? "",
    display_name: t.displayName ?? "",
    description: t.description ?? "",
    category: t.category ?? "",
    source: (t.source ?? "") as Tool["source"],
    risk_level: (t.riskLevel ?? "") as Tool["risk_level"],
    enabled: Boolean(t.enabled),
    readonly: Boolean(t.readonly),
    requires_confirmation: Boolean(t.requiresConfirmation),
    supports_streaming: Boolean(t.supportsStreaming),
    supports_concurrency: Boolean(t.supportsConcurrency),
    parameters_schema_json: t.parametersSchemaJson ?? "",
    result_schema_json: t.resultSchemaJson ?? "",
    config_schema_json: t.configSchemaJson ?? "",
    config_json: t.configJson ?? "",
    default_config_json: t.defaultConfigJson ?? "",
    metadata_json: t.metadataJson ?? "",
    runtime_status: (t.runtimeStatus ?? "") as Tool["runtime_status"],
    runtime_kind: (t.runtimeKind ?? "") as Tool["runtime_kind"],
    invoke_count: t.invokeCount ?? 0,
    invoke_count_24h: t.invokeCount24h ?? 0,
    success_count: t.successCount ?? 0,
    failure_count: t.failureCount ?? 0,
    blocked_count: t.blockedCount ?? 0,
    agent_override_count: t.agentOverrideCount ?? 0,
    avg_duration_ms: t.avgDurationMs ?? null,
    last_invoked_at: t.lastInvokedAt ?? "",
    last_status: t.lastStatus ?? "",
    created_at: t.createdAt ?? "",
    updated_at: t.updatedAt ?? "",
    permissions: { can_manage: Boolean(t.permissions?.canManage) }
  };
}

function kratosSummaryToLegacy(s?: KratosSummary) {
  return {
    total_tools: s?.totalTools ?? 0,
    enabled_tools: s?.enabledTools ?? 0,
    high_risk_enabled: s?.highRiskEnabled ?? 0,
    calls_24h: s?.calls24h ?? 0,
    failure_rate_24h: s?.failureRate24h ?? 0
  };
}

function kratosInvocationToLegacy(x: KratosInvocation): ToolInvocation {
  return {
    id: x.id ?? "",
    request_id: x.requestId ?? "",
    invocation_id: x.invocationId ?? "",
    tool_id: x.toolId ?? "",
    tool_key: x.toolKey ?? "",
    tool_display_name: x.toolDisplayName ?? "",
    agent_id: x.agentId ?? "",
    agent_key: x.agentKey ?? "",
    agent_display_name: x.agentDisplayName ?? "",
    session_id: x.sessionId ?? "",
    message_id: x.messageId ?? "",
    user_id: x.userId ?? "",
    source: x.source ?? "",
    status: (x.status ?? "") as ToolInvocation["status"],
    started_at: x.startedAt ?? "",
    ended_at: x.endedAt ?? "",
    duration_ms: x.durationMs ?? 0,
    input_preview: x.inputPreview ?? "",
    input_hash: x.inputHash ?? "",
    output_preview: x.outputPreview ?? "",
    output_hash: x.outputHash ?? "",
    error_code: x.errorCode ?? "",
    error_message: x.errorMessage ?? "",
    redaction_applied: Boolean(x.redactionApplied),
    metadata_json: x.metadataJson ?? "",
    created_at: x.createdAt ?? ""
  };
}

export async function listTools(query: ToolListQuery = {}): Promise<ToolListResponse> {
  const data = await toolApi.ListTools({
    search: query.search,
    category: query.category,
    source: query.source,
    riskLevel: query.risk_level,
    enabled: enabledFilter(query.enabled),
    page: query.page,
    pageSize: query.page_size
  });
  const items = (data.items ?? []).map(kratosToolToLegacy);
  return {
    items,
    page: data.page ?? query.page ?? 1,
    page_size: data.pageSize ?? query.page_size ?? 20,
    total: data.total ?? items.length,
    summary: kratosSummaryToLegacy(data.summary)
  };
}

export async function getTool(id: string): Promise<Tool> {
  const data = await toolApi.GetTool({ id });
  return kratosToolToLegacy(data);
}

export async function createTool(input: ToolUpsertInput): Promise<Tool> {
  const data = await toolApi.CreateTool({
    key: input.key,
    displayName: input.display_name,
    description: input.description,
    category: input.category,
    source: input.source,
    riskLevel: input.risk_level,
    enabled: input.enabled,
    readonly: input.readonly,
    requiresConfirmation: input.requires_confirmation,
    supportsStreaming: input.supports_streaming,
    supportsConcurrency: input.supports_concurrency,
    parametersSchemaJson: input.parameters_schema_json,
    resultSchemaJson: input.result_schema_json,
    configSchemaJson: input.config_schema_json,
    configJson: input.config_json,
    defaultConfigJson: input.default_config_json,
    metadataJson: input.metadata_json
  });
  return kratosToolToLegacy(data);
}

export async function updateTool(id: string, input: ToolUpsertInput): Promise<Tool> {
  const data = await toolApi.UpdateTool({
    id,
    key: input.key,
    displayName: input.display_name,
    description: input.description,
    category: input.category,
    source: input.source,
    riskLevel: input.risk_level,
    enabled: input.enabled,
    readonly: input.readonly,
    requiresConfirmation: input.requires_confirmation,
    supportsStreaming: input.supports_streaming,
    supportsConcurrency: input.supports_concurrency,
    parametersSchemaJson: input.parameters_schema_json,
    resultSchemaJson: input.result_schema_json,
    configSchemaJson: input.config_schema_json,
    configJson: input.config_json,
    defaultConfigJson: input.default_config_json,
    metadataJson: input.metadata_json
  });
  return kratosToolToLegacy(data);
}

export async function deleteTool(id: string): Promise<void> {
  await toolApi.DeleteTool({ id });
}

export async function toggleToolEnabled(id: string, enabled: boolean): Promise<Tool> {
  const data = await toolApi.ToggleToolEnabled({ id, enabled });
  return kratosToolToLegacy(data);
}

export async function listToolRunsForTool(id: string, query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>> {
  const data = await toolApi.ListToolRunsForTool({
    toolId: id,
    agentId: query.agent_id,
    sessionId: query.session_id,
    status: query.status,
    from: query.from,
    to: query.to,
    page: query.page,
    pageSize: query.page_size
  });
  const items = (data.items ?? []).map(kratosInvocationToLegacy);
  return {
    items,
    page: data.page ?? query.page ?? 1,
    page_size: data.pageSize ?? query.page_size ?? 20,
    total: data.total ?? items.length
  };
}

export async function listToolRuns(query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>> {
  const data = await toolApi.ListToolRuns({
    toolKey: query.tool_key,
    agentId: query.agent_id,
    sessionId: query.session_id,
    status: query.status,
    from: query.from,
    to: query.to,
    page: query.page,
    pageSize: query.page_size
  });
  const items = (data.items ?? []).map(kratosInvocationToLegacy);
  return {
    items,
    page: data.page ?? query.page ?? 1,
    page_size: data.pageSize ?? query.page_size ?? 20,
    total: data.total ?? items.length
  };
}

/** 仍走遗留 `pkg/backend`：尚无 `agent/v1` 等价 RPC（baseURL 已含 `/api/v1`）。 */
export async function getAgentEffectiveTools(agentId: string): Promise<AgentEffectiveTools> {
  const { data } = await legacyRestApi.get(`/agents/${encodeURIComponent(agentId)}/tools/effective`);
  return data;
}
