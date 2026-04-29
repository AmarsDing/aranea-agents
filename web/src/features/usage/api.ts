/**
 * 模型用量：`usage/v1` Kratos HTTP，映射为遗留 snake_case 形状（页面仍用 `ModelUsage*` 类型）。
 */
import { createUsageService } from "../../services/index";
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
} from "../../services/clientLegacy";

export type {
  ModelTokenUsageEvent,
  ModelUsageBreakdownRow,
  ModelUsageOverview,
  ModelUsageQuery,
  ModelUsageTrendPoint,
  ModelUsageSummary
} from "../../services/clientLegacy";

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
