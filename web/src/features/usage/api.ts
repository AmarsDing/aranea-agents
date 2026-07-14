/**
 * 模型用量：`usage/v1` —— `createUsageService()`（见 `services/index.ts`），映射为 `types.ts` snake_case 形状。
 * **`recordModelTokenUsageEvent`** → **`POST /v1/usage/token-events`**（body 与 Go **`json:"…"`** 标签一致为 snake_case；生成的 **`JSON.stringify(TokenUsageEvent)`** 为 camelCase，不经由该方法）。
 */
import { createUsageService } from '../../services/index';
import { kratosApi } from '../../services/axiosHandler';
import type {
  UsageTrendPoint as KTrend,
  TokenUsageEvent as KEvent,
  UsageQuery as KUsageQuery,
  ListUsageTrendsResponse,
  ListUsageEventsResponse,
  ListAllModelsBreakdownRequest as KBreakdownReq,
  ListAllModelsBreakdownResponse as KBreakdownResp,
} from '../../services/kratos/usage/v1/index';
import type {
  ModelTokenUsageEvent,
  ModelUsageBreakdownRow,
  ModelUsageOverview,
  ModelUsageQuery,
  ModelUsageTrendPoint,
  ModelUsageSummary,
  AllModelsBreakdownQuery,
  AllModelsBreakdownResult,
} from './types';

export type {
  ModelTokenUsageEvent,
  ModelUsageBreakdownRow,
  ModelUsageOverview,
  ModelUsageQuery,
  ModelUsageTrendPoint,
  ModelUsageSummary,
  AllModelsBreakdownQuery,
  AllModelsBreakdownResult,
} from './types';

const usage = createUsageService();

function num(v: unknown): number {
  if (typeof v === 'number' && !Number.isNaN(v)) {
    return v;
  }
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    return Number.isNaN(n) ? 0 : n;
  }
  return 0;
}

function obj(v: unknown): Record<string, unknown> {
  return v !== null && typeof v === 'object' ? (v as Record<string, unknown>) : {};
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
    success_rate: num(s.success_rate ?? s.successRate),
  };
}

function trendFromUnknown(raw: unknown): ModelUsageTrendPoint {
  const t = obj(raw);
  return {
    date_key: String(t.date_key ?? t.dateKey ?? ''),
    call_count: num(t.call_count ?? t.callCount),
    input_tokens: num(t.input_tokens ?? t.inputTokens),
    output_tokens: num(t.output_tokens ?? t.outputTokens),
    total_tokens: num(t.total_tokens ?? t.totalTokens),
    total_cost_micro_usd: num(t.total_cost_micro_usd ?? t.totalCostMicroUsd),
    success_count: num(t.success_count ?? t.successCount),
    failed_count: num(t.failed_count ?? t.failedCount),
    cancelled_count: num(t.cancelled_count ?? t.cancelledCount),
    avg_latency_ms: num(t.avg_latency_ms ?? t.avgLatencyMs),
    avg_tokens_per_second: num(t.avg_tokens_per_second ?? t.avgTokensPerSecond),
  };
}

function breakdownFromUnknown(raw: unknown): ModelUsageBreakdownRow {
  const r = obj(raw);
  return {
    provider_code: String(r.provider_code ?? r.providerCode ?? ''),
    model_api_id: String(r.model_api_id ?? r.modelApiId ?? ''),
    model_display_name: String(r.model_display_name ?? r.modelDisplayName ?? ''),
    agent_id: String(r.agent_id ?? r.agentId ?? ''),
    agent_key: String(r.agent_key ?? r.agentKey ?? ''),
    call_count: num(r.call_count ?? r.callCount),
    input_tokens: num(r.input_tokens ?? r.inputTokens),
    output_tokens: num(r.output_tokens ?? r.outputTokens),
    total_tokens: num(r.total_tokens ?? r.totalTokens),
    total_cost_micro_usd: num(r.total_cost_micro_usd ?? r.totalCostMicroUsd),
    avg_latency_ms: num(r.avg_latency_ms ?? r.avgLatencyMs),
    avg_tokens_per_second: num(r.avg_tokens_per_second ?? r.avgTokensPerSecond),
    success_rate: num(r.success_rate ?? r.successRate),
  };
}

function tokenEventFromUnknown(raw: unknown): ModelTokenUsageEvent {
  const e = obj(raw);
  return {
    id: String(e.id ?? ''),
    occurred_at: String(e.occurred_at ?? e.occurredAt ?? ''),
    date_key: String(e.date_key ?? e.dateKey ?? ''),
    hour_key: String(e.hour_key ?? e.hourKey ?? ''),
    user_id: String(e.user_id ?? e.userId ?? ''),
    team_id: String(e.team_id ?? e.teamId ?? ''),
    request_id: String(e.request_id ?? e.requestId ?? ''),
    agent_id: String(e.agent_id ?? e.agentId ?? ''),
    agent_key: String(e.agent_key ?? e.agentKey ?? ''),
    session_id: String(e.session_id ?? e.sessionId ?? ''),
    message_id: String(e.message_id ?? e.messageId ?? ''),
    provider_code: String(e.provider_code ?? e.providerCode ?? ''),
    provider_type: String(e.provider_type ?? e.providerType ?? ''),
    provider_display_name: String(e.provider_display_name ?? e.providerDisplayName ?? ''),
    model_api_id: String(e.model_api_id ?? e.modelApiId ?? ''),
    model_display_name: String(e.model_display_name ?? e.modelDisplayName ?? ''),
    model_category_json: String(e.model_category_json ?? e.modelCategoryJson ?? ''),
    usage_kind: String(e.usage_kind ?? e.usageKind ?? ''),
    call_count: num(e.call_count ?? e.callCount),
    input_tokens: num(e.input_tokens ?? e.inputTokens),
    output_tokens: num(e.output_tokens ?? e.outputTokens),
    cached_input_tokens: e.cached_input_tokens != null ? num(e.cached_input_tokens ?? e.cachedInputTokens) : undefined,
    reasoning_tokens: e.reasoning_tokens != null ? num(e.reasoning_tokens ?? e.reasoningTokens) : undefined,
    embedding_tokens: e.embedding_tokens != null ? num(e.embedding_tokens ?? e.embeddingTokens) : undefined,
    total_tokens: num(e.total_tokens ?? e.totalTokens),
    input_price_micro_usd_per_1k:
      e.input_price_micro_usd_per_1k != null
        ? num(e.input_price_micro_usd_per_1k ?? e.inputPriceMicroUsdPer1k)
        : undefined,
    output_price_micro_usd_per_1k:
      e.output_price_micro_usd_per_1k != null
        ? num(e.output_price_micro_usd_per_1k ?? e.outputPriceMicroUsdPer1k)
        : undefined,
    cached_input_price_micro_usd_per_1k:
      e.cached_input_price_micro_usd_per_1k != null
        ? num(e.cached_input_price_micro_usd_per_1k ?? e.cachedInputPriceMicroUsdPer1k)
        : undefined,
    reasoning_price_micro_usd_per_1k:
      e.reasoning_price_micro_usd_per_1k != null
        ? num(e.reasoning_price_micro_usd_per_1k ?? e.reasoningPriceMicroUsdPer1k)
        : undefined,
    embedding_price_micro_usd_per_1k:
      e.embedding_price_micro_usd_per_1k != null
        ? num(e.embedding_price_micro_usd_per_1k ?? e.embeddingPriceMicroUsdPer1k)
        : undefined,
    input_cost_micro_usd:
      e.input_cost_micro_usd != null ? num(e.input_cost_micro_usd ?? e.inputCostMicroUsd) : undefined,
    output_cost_micro_usd:
      e.output_cost_micro_usd != null ? num(e.output_cost_micro_usd ?? e.outputCostMicroUsd) : undefined,
    cached_input_cost_micro_usd:
      e.cached_input_cost_micro_usd != null
        ? num(e.cached_input_cost_micro_usd ?? e.cachedInputCostMicroUsd)
        : undefined,
    reasoning_cost_micro_usd:
      e.reasoning_cost_micro_usd != null ? num(e.reasoning_cost_micro_usd ?? e.reasoningCostMicroUsd) : undefined,
    embedding_cost_micro_usd:
      e.embedding_cost_micro_usd != null ? num(e.embedding_cost_micro_usd ?? e.embeddingCostMicroUsd) : undefined,
    total_cost_micro_usd: num(e.total_cost_micro_usd ?? e.totalCostMicroUsd),
    latency_ms: num(e.latency_ms ?? e.latencyMs),
    time_to_first_token_ms:
      e.time_to_first_token_ms != null ? num(e.time_to_first_token_ms ?? e.timeToFirstTokenMs) : undefined,
    tokens_per_second: num(e.tokens_per_second ?? e.tokensPerSecond),
    status: String(e.status ?? ''),
    error_code: String(e.error_code ?? e.errorCode ?? ''),
    error_message: String(e.error_message ?? e.errorMessage ?? ''),
    retry_count: e.retry_count != null ? num(e.retry_count ?? e.retryCount) : undefined,
    prompt_mode: String(e.prompt_mode ?? e.promptMode ?? ''),
    max_output_tokens: num(e.max_output_tokens ?? e.maxOutputTokens),
    context_window_k: num(e.context_window_k ?? e.contextWindowK),
    stream_enabled: Boolean(e.stream_enabled ?? e.streamEnabled),
    metadata_json: String(e.metadata_json ?? e.metadataJson ?? ''),
    created_at: String(e.created_at ?? e.createdAt ?? ''),
  };
}

function overviewToLegacy(body: unknown): ModelUsageOverview {
  const o = obj(body);
  const rangeRaw = o.range_summary ?? o.rangeSummary;
  const trendsRaw = o.trends;
  const topModelsRaw = o.top_models ?? o.topModels;
  const topAgentsRaw = o.top_agents ?? o.topAgents;
  const anomaliesRaw = o.anomalies;
  const inefficientRaw = o.inefficient_models ?? o.inefficientModels;

  const dashRaw = o.quota_dashboard ?? o.quotaDashboard;
  const dash = obj(dashRaw);

  return {
    today: summaryFromUnknown(o.today),
    yesterday: summaryFromUnknown(o.yesterday),
    month: summaryFromUnknown(o.month),
    range: summaryFromUnknown(rangeRaw),
    trends: Array.isArray(trendsRaw) ? trendsRaw.map(trendFromUnknown) : [],
    top_models: Array.isArray(topModelsRaw) ? topModelsRaw.map(breakdownFromUnknown) : [],
    top_agents: Array.isArray(topAgentsRaw) ? topAgentsRaw.map(breakdownFromUnknown) : [],
    anomalies: Array.isArray(anomaliesRaw) ? anomaliesRaw.map(tokenEventFromUnknown) : [],
    inefficient_models: Array.isArray(inefficientRaw)
      ? inefficientRaw.map((raw: unknown) => {
          const m = obj(raw);
          return {
            provider_code: String(m.provider_code ?? m.providerCode ?? ''),
            model_api_id: String(m.model_api_id ?? m.modelApiId ?? ''),
            model_display_name: String(m.model_display_name ?? m.modelDisplayName ?? ''),
            call_count: num(m.call_count ?? m.callCount),
            total_tokens: num(m.total_tokens ?? m.totalTokens),
            total_cost_micro_usd: num(m.total_cost_micro_usd ?? m.totalCostMicroUsd),
            avg_latency_ms: num(m.avg_latency_ms ?? m.avgLatencyMs),
            avg_tokens_per_second: num(m.avg_tokens_per_second ?? m.avgTokensPerSecond),
            success_rate: num(m.success_rate ?? m.successRate),
            flags: Array.isArray(m.flags) ? m.flags.map((f) => String(f)) : [],
          };
        })
      : [],
    quota_dashboard: dashRaw
      ? {
          configured_count: num(dash.configured_count ?? dash.configuredCount),
          total_cap_micro_usd: num(dash.total_cap_micro_usd ?? dash.totalCapMicroUsd),
          total_spent_micro_usd: num(dash.total_spent_micro_usd ?? dash.totalSpentMicroUsd),
          max_utilization_ratio: num(dash.max_utilization_ratio ?? dash.maxUtilizationRatio),
        }
      : undefined,
  };
}

function queryToKratos(q: ModelUsageQuery): KUsageQuery {
  const out: KUsageQuery = {
    range: q.range,
    startDate: q.start_date,
    endDate: q.end_date,
    providerCode: q.provider_code,
    modelApiId: q.model_api_id,
    agentId: q.agent_id,
    status: q.status,
    limit: q.limit,
    granularity: q.granularity,
    teamId: q.team_id?.trim() || undefined,
    usageKind: q.usage_kind?.trim() || undefined,
  };
  if (q.team_id?.trim()) out.teamId = q.team_id.trim();
  if (q.usage_kind?.trim()) out.usageKind = q.usage_kind.trim();
  return out;
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
    error_message: e.error_message ?? '',
    prompt_mode: e.prompt_mode,
    max_output_tokens: Math.trunc(e.max_output_tokens),
    context_window_k: Math.trunc(e.context_window_k),
    stream_enabled: Boolean(e.stream_enabled),
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
export async function exportUsageEventsCsv(query: ModelUsageQuery = {}): Promise<string> {
  const raw = await usage.ExportUsageEvents(queryToKratos(query));
  return String((raw as { csv?: string }).csv ?? '');
}

export async function recordModelTokenUsageEvent(e: ModelTokenUsageEvent): Promise<ModelTokenUsageEvent> {
  const { data } = await kratosApi.post<unknown>('/v1/usage/token-events', usageTokenEventIngestBody(e), {
    headers: { 'Content-Type': 'application/json' },
  });
  return tokenEventFromUnknown(data);
}

export async function purgeUsageEvents(retainDays: number): Promise<{ deleted_count: number }> {
  const raw = await usage.PurgeUsageEvents({ retainDays });
  return {
    deleted_count: num((raw as Record<string, unknown>).deletedCount ?? (raw as Record<string, unknown>).deleted_count),
  };
}

/**
 * listAllModelsBreakdown —— 全模型消耗总览表的服务端分页查询。
 * 与 listTopModels (top-N capped) 不同：支持 LIKE 搜索 + 动态排序 + 分页。
 * 后端通过 GET /v1/usage/all-models-breakdown 实现。
 */
export async function listAllModelsBreakdown(query: AllModelsBreakdownQuery = {}): Promise<AllModelsBreakdownResult> {
  const req: KBreakdownReq = {
    range: query.range,
    startDate: query.start_date,
    endDate: query.end_date,
    providerCode: query.provider_code,
    search: query.search,
    sortField: query.sort_field,
    sortDir: query.sort_dir,
    page: query.page,
    pageSize: query.page_size,
  };
  const raw = (await usage.ListAllModelsBreakdown(req)) as KBreakdownResp;
  const items = Array.isArray(raw.items) ? raw.items.map(breakdownFromUnknown) : [];
  return {
    items,
    total: num(raw.total),
    page: num(raw.page),
    page_size: num(raw.pageSize),
  };
}
