/**
 * 模型用量：`usage/v1` —— `createUsageService()`（见 `services/index.ts`），映射为 `types.ts` snake_case 形状。
 * **`recordModelTokenUsageEvent`** → **`POST /v1/usage/token-events`**（body 与 Go **`json:"…"`** 标签一致为 snake_case；生成的 **`JSON.stringify(TokenUsageEvent)`** 为 camelCase，不经由该方法）。
 */
import { createUsageService } from "../../services/index";
import { kratosApi } from "../../services/axiosHandler";
import type {
  UsageTrendPoint as KTrend,
  TokenUsageEvent as KEvent,
  UsageQuery as KUsageQuery,
  ListUsageTrendsResponse,
  ListUsageEventsResponse
} from "../../services/kratos/usage/v1/index";
import type {
  ModelTokenUsageEvent,
  ModelUsageBreakdownRow,
  ModelUsageOverview,
  ModelUsageQuery,
  ModelUsageTrendPoint,
  ModelUsageSummary
} from "./types";

export type {
  ModelTokenUsageEvent,
  ModelUsageBreakdownRow,
  ModelUsageOverview,
  ModelUsageQuery,
  ModelUsageTrendPoint,
  ModelUsageSummary
} from "./types";

const usage = createUsageService();

function num(v: unknown): number {
  if (typeof v === "number" && !Number.isNaN(v)) {
    return v;
  }
  if (typeof v === "string" && v.trim() !== "") {
    const n = Number(v);
    return Number.isNaN(n) ? 0 : n;
  }
  return 0;
}

function obj(v: unknown): Record<string, unknown> {
  return v !== null && typeof v === "object" ? (v as Record<string, unknown>) : {};
}

function summaryFromUnknown(raw: unknown): ModelUsageSummary {
  const s = obj(raw);
  return {
    call_count: num(s.call_count ?? s.callCount),
    request_count: num(s.request_count ?? s.requestCount),
    success_count: num(s.success_count ?? s.successCount),
    failed_count: num(s.failed_count ?? s.failedCount),
    cancelled_count: num(s.cancelled_count ?? s.cancelledCount),
    input_tokens: num(s.input_tokens ?? s.inputTokens),
    output_tokens: num(s.output_tokens ?? s.outputTokens),
    total_tokens: num(s.total_tokens ?? s.totalTokens),
    total_cost_micro_usd: num(s.total_cost_micro_usd ?? s.totalCostMicroUsd),
    avg_latency_ms: num(s.avg_latency_ms ?? s.avgLatencyMs),
    avg_tokens_per_second: num(s.avg_tokens_per_second ?? s.avgTokensPerSecond),
    success_rate: num(s.success_rate ?? s.successRate)
  };
}

function trendFromUnknown(raw: unknown): ModelUsageTrendPoint {
  const t = obj(raw);
  return {
    date_key: String(t.date_key ?? t.dateKey ?? ""),
    call_count: num(t.call_count ?? t.callCount),
    input_tokens: num(t.input_tokens ?? t.inputTokens),
    output_tokens: num(t.output_tokens ?? t.outputTokens),
    total_tokens: num(t.total_tokens ?? t.totalTokens),
    total_cost_micro_usd: num(t.total_cost_micro_usd ?? t.totalCostMicroUsd),
    success_count: num(t.success_count ?? t.successCount),
    failed_count: num(t.failed_count ?? t.failedCount),
    cancelled_count: num(t.cancelled_count ?? t.cancelledCount),
    avg_latency_ms: num(t.avg_latency_ms ?? t.avgLatencyMs),
    avg_tokens_per_second: num(t.avg_tokens_per_second ?? t.avgTokensPerSecond)
  };
}

function breakdownFromUnknown(raw: unknown): ModelUsageBreakdownRow {
  const r = obj(raw);
  return {
    provider_code: String(r.provider_code ?? r.providerCode ?? ""),
    model_api_id: String(r.model_api_id ?? r.modelApiId ?? ""),
    model_display_name: String(r.model_display_name ?? r.modelDisplayName ?? ""),
    agent_id: String(r.agent_id ?? r.agentId ?? ""),
    agent_key: String(r.agent_key ?? r.agentKey ?? ""),
    call_count: num(r.call_count ?? r.callCount),
    input_tokens: num(r.input_tokens ?? r.inputTokens),
    output_tokens: num(r.output_tokens ?? r.outputTokens),
    total_tokens: num(r.total_tokens ?? r.totalTokens),
    total_cost_micro_usd: num(r.total_cost_micro_usd ?? r.totalCostMicroUsd),
    avg_latency_ms: num(r.avg_latency_ms ?? r.avgLatencyMs),
    avg_tokens_per_second: num(r.avg_tokens_per_second ?? r.avgTokensPerSecond),
    success_rate: num(r.success_rate ?? r.successRate)
  };
}

function tokenEventFromUnknown(raw: unknown): ModelTokenUsageEvent {
  const e = obj(raw);
  return {
    id: String(e.id ?? ""),
    occurred_at: String(e.occurred_at ?? e.occurredAt ?? ""),
    agent_id: String(e.agent_id ?? e.agentId ?? ""),
    agent_key: String(e.agent_key ?? e.agentKey ?? ""),
    session_id: String(e.session_id ?? e.sessionId ?? ""),
    message_id: String(e.message_id ?? e.messageId ?? ""),
    provider_code: String(e.provider_code ?? e.providerCode ?? ""),
    provider_type: String(e.provider_type ?? e.providerType ?? ""),
    provider_display_name: String(e.provider_display_name ?? e.providerDisplayName ?? ""),
    model_api_id: String(e.model_api_id ?? e.modelApiId ?? ""),
    model_display_name: String(e.model_display_name ?? e.modelDisplayName ?? ""),
    call_count: num(e.call_count ?? e.callCount),
    input_tokens: num(e.input_tokens ?? e.inputTokens),
    output_tokens: num(e.output_tokens ?? e.outputTokens),
    total_tokens: num(e.total_tokens ?? e.totalTokens),
    total_cost_micro_usd: num(e.total_cost_micro_usd ?? e.totalCostMicroUsd),
    latency_ms: num(e.latency_ms ?? e.latencyMs),
    tokens_per_second: num(e.tokens_per_second ?? e.tokensPerSecond),
    status: String(e.status ?? ""),
    error_message: String(e.error_message ?? e.errorMessage ?? ""),
    prompt_mode: String(e.prompt_mode ?? e.promptMode ?? ""),
    max_output_tokens: num(e.max_output_tokens ?? e.maxOutputTokens),
    context_window_k: num(e.context_window_k ?? e.contextWindowK),
    stream_enabled: Boolean(e.stream_enabled ?? e.streamEnabled)
  };
}

function overviewToLegacy(body: unknown): ModelUsageOverview {
  const o = obj(body);
  const rangeRaw = o.range_summary ?? o.rangeSummary;
  const trendsRaw = o.trends;
  const topModelsRaw = o.top_models ?? o.topModels;
  const topAgentsRaw = o.top_agents ?? o.topAgents;
  const anomaliesRaw = o.anomalies;

  return {
    today: summaryFromUnknown(o.today),
    yesterday: summaryFromUnknown(o.yesterday),
    month: summaryFromUnknown(o.month),
    range: summaryFromUnknown(rangeRaw),
    trends: Array.isArray(trendsRaw) ? trendsRaw.map(trendFromUnknown) : [],
    top_models: Array.isArray(topModelsRaw) ? topModelsRaw.map(breakdownFromUnknown) : [],
    top_agents: Array.isArray(topAgentsRaw) ? topAgentsRaw.map(breakdownFromUnknown) : [],
    anomalies: Array.isArray(anomaliesRaw) ? anomaliesRaw.map(tokenEventFromUnknown) : []
  };
}

function queryToKratos(q: ModelUsageQuery): KUsageQuery {
  return {
    range: q.range,
    startDate: q.start_date,
    endDate: q.end_date,
    providerCode: q.provider_code,
    modelApiId: q.model_api_id,
    agentId: q.agent_id,
    status: q.status,
    limit: q.limit
  };
}

/** `POST /v1/usage/token-events`：与 `api/kratos/usage/v1/usage.pb.go` 中 `TokenUsageEvent` 的 `json:"…"` 键一致。 */
function usageTokenEventIngestBody(e: ModelTokenUsageEvent): Record<string, unknown> {
  const out: Record<string, unknown> = {
    id: e.id,
    agent_id: e.agent_id,
    agent_key: e.agent_key,
    session_id: e.session_id,
    message_id: e.message_id,
    provider_code: e.provider_code,
    provider_type: e.provider_type,
    provider_display_name: e.provider_display_name,
    model_api_id: e.model_api_id,
    model_display_name: e.model_display_name,
    call_count: Math.trunc(e.call_count),
    input_tokens: Math.trunc(e.input_tokens),
    output_tokens: Math.trunc(e.output_tokens),
    total_tokens: Math.trunc(e.total_tokens),
    total_cost_micro_usd: Math.round(Number(e.total_cost_micro_usd)),
    latency_ms: Math.trunc(e.latency_ms),
    tokens_per_second: Number(e.tokens_per_second),
    status: e.status,
    error_message: e.error_message ?? "",
    prompt_mode: e.prompt_mode,
    max_output_tokens: Math.trunc(e.max_output_tokens),
    context_window_k: Math.trunc(e.context_window_k),
    stream_enabled: Boolean(e.stream_enabled)
  };
  if (e.occurred_at) {
    out.occurred_at = e.occurred_at;
  }
  if (e.date_key) {
    out.date_key = e.date_key;
  }
  if (e.hour_key) {
    out.hour_key = e.hour_key;
  }
  if (e.workspace_id) {
    out.workspace_id = e.workspace_id;
  }
  if (e.user_id) {
    out.user_id = e.user_id;
  }
  if (e.team_id) {
    out.team_id = e.team_id;
  }
  if (e.request_id) {
    out.request_id = e.request_id;
  }
  if (e.model_category_json) {
    out.model_category_json = e.model_category_json;
  }
  if (e.usage_kind) {
    out.usage_kind = e.usage_kind;
  }
  if (e.cached_input_tokens !== undefined) {
    out.cached_input_tokens = Math.trunc(e.cached_input_tokens);
  }
  if (e.reasoning_tokens !== undefined) {
    out.reasoning_tokens = Math.trunc(e.reasoning_tokens);
  }
  if (e.embedding_tokens !== undefined) {
    out.embedding_tokens = Math.trunc(e.embedding_tokens);
  }
  if (e.input_price_micro_usd_per_1k !== undefined) {
    out.input_price_micro_usd_per_1k = Math.round(e.input_price_micro_usd_per_1k);
  }
  if (e.output_price_micro_usd_per_1k !== undefined) {
    out.output_price_micro_usd_per_1k = Math.round(e.output_price_micro_usd_per_1k);
  }
  if (e.cached_input_price_micro_usd_per_1k !== undefined) {
    out.cached_input_price_micro_usd_per_1k = Math.round(e.cached_input_price_micro_usd_per_1k);
  }
  if (e.reasoning_price_micro_usd_per_1k !== undefined) {
    out.reasoning_price_micro_usd_per_1k = Math.round(e.reasoning_price_micro_usd_per_1k);
  }
  if (e.embedding_price_micro_usd_per_1k !== undefined) {
    out.embedding_price_micro_usd_per_1k = Math.round(e.embedding_price_micro_usd_per_1k);
  }
  if (e.input_cost_micro_usd !== undefined) {
    out.input_cost_micro_usd = Math.round(e.input_cost_micro_usd);
  }
  if (e.output_cost_micro_usd !== undefined) {
    out.output_cost_micro_usd = Math.round(e.output_cost_micro_usd);
  }
  if (e.cached_input_cost_micro_usd !== undefined) {
    out.cached_input_cost_micro_usd = Math.round(e.cached_input_cost_micro_usd);
  }
  if (e.reasoning_cost_micro_usd !== undefined) {
    out.reasoning_cost_micro_usd = Math.round(e.reasoning_cost_micro_usd);
  }
  if (e.embedding_cost_micro_usd !== undefined) {
    out.embedding_cost_micro_usd = Math.round(e.embedding_cost_micro_usd);
  }
  if (e.time_to_first_token_ms !== undefined) {
    out.time_to_first_token_ms = Math.trunc(e.time_to_first_token_ms);
  }
  if (e.error_code) {
    out.error_code = e.error_code;
  }
  if (e.retry_count !== undefined) {
    out.retry_count = Math.trunc(e.retry_count);
  }
  if (e.metadata_json) {
    out.metadata_json = e.metadata_json;
  }
  if (e.created_at) {
    out.created_at = e.created_at;
  }
  return out;
}

export async function getModelUsageOverview(query: ModelUsageQuery = {}): Promise<ModelUsageOverview> {
  const raw = await usage.GetUsageOverview(queryToKratos(query));
  return overviewToLegacy(raw);
}

export async function listModelUsageTrends(query: ModelUsageQuery = {}): Promise<ModelUsageTrendPoint[]> {
  const raw = (await usage.ListUsageTrends(queryToKratos(query))) as ListUsageTrendsResponse;
  const items = raw.items ?? [];
  return items.map((t: KTrend) => trendFromUnknown(t as unknown as Record<string, unknown>));
}

export async function listModelUsageEvents(query: ModelUsageQuery = {}): Promise<ModelTokenUsageEvent[]> {
  const raw = (await usage.ListUsageEvents(queryToKratos(query))) as ListUsageEventsResponse;
  const items = raw.items ?? [];
  return items.map((e: KEvent) => tokenEventFromUnknown(e as unknown as Record<string, unknown>));
}

/** Persists one usage row + session counters + daily rollup (`usage/v1` ingest). */
export async function recordModelTokenUsageEvent(e: ModelTokenUsageEvent): Promise<ModelTokenUsageEvent> {
  const { data } = await kratosApi.post<unknown>("/v1/usage/token-events", usageTokenEventIngestBody(e), {
    headers: { "Content-Type": "application/json" }
  });
  return tokenEventFromUnknown(data);
}
