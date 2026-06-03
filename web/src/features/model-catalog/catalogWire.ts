import { asRecord, pickBool, pickNum, pickStr, pickStrArray } from '../../shared/wireJson';
import type { CatalogModelSummary, CatalogProviderSummary } from '../../services/kratos/model_catalog/v1/index';

function pickUsdPer1m(
  row: Record<string, unknown>,
  cost: Record<string, unknown>,
  snake: string,
  camel: string,
  costKey: string,
): number {
  const direct = pickNum(row, snake, camel);
  if (direct > 0) return direct;
  for (const key of [camel.replace(/1m$/, '_1M'), camel.replace(/1m$/, '1M')]) {
    const v = row[key];
    if (typeof v === 'number' && v > 0 && !Number.isNaN(v)) return v;
  }
  return pickNum(cost, costKey, costKey);
}

export function normalizeCatalogProviderSummary(raw: unknown): CatalogProviderSummary {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    name: pickStr(r, 'name', 'name'),
    doc: pickStr(r, 'doc', 'doc'),
    npm: pickStr(r, 'npm', 'npm'),
    api: pickStr(r, 'api', 'api'),
    modelCount: pickNum(r, 'model_count', 'modelCount'),
    logoUrl: pickStr(r, 'logo_url', 'logoUrl'),
    logoCached: pickBool(r, 'logo_cached', 'logoCached'),
    env: pickStrArray(r, 'env', 'env'),
  };
}

export function normalizeCatalogModelSummary(raw: unknown): CatalogModelSummary {
  const r = asRecord(raw);
  const cost = asRecord(r.cost ?? r.Cost);
  return {
    id: pickStr(r, 'id', 'id'),
    name: pickStr(r, 'name', 'name'),
    status: pickStr(r, 'status', 'status'),
    reasoning: pickBool(r, 'reasoning', 'reasoning'),
    toolCall: pickBool(r, 'tool_call', 'toolCall'),
    attachment: pickBool(r, 'attachment', 'attachment'),
    inputUsdPer1m: pickUsdPer1m(r, cost, 'input_usd_per_1m', 'inputUsdPer1m', 'input'),
    outputUsdPer1m: pickUsdPer1m(r, cost, 'output_usd_per_1m', 'outputUsdPer1m', 'output'),
    contextTokens: pickNum(r, 'context_tokens', 'contextTokens'),
    outputTokens: pickNum(r, 'output_tokens', 'outputTokens'),
    cacheReadUsdPer1m: pickUsdPer1m(r, cost, 'cache_read_usd_per_1m', 'cacheReadUsdPer1m', 'cache_read'),
    cacheWriteUsdPer1m: pickUsdPer1m(r, cost, 'cache_write_usd_per_1m', 'cacheWriteUsdPer1m', 'cache_write'),
    reasoningUsdPer1m: pickUsdPer1m(r, cost, 'reasoning_usd_per_1m', 'reasoningUsdPer1m', 'reasoning'),
    structuredOutput: pickBool(r, 'structured_output', 'structuredOutput'),
    openWeights: pickBool(r, 'open_weights', 'openWeights'),
    temperature: pickBool(r, 'temperature', 'temperature'),
    modalityInput: pickStrArray(r, 'modality_input', 'modalityInput'),
    modalityOutput: pickStrArray(r, 'modality_output', 'modalityOutput'),
    family: pickStr(r, 'family', 'family'),
    knowledge: pickStr(r, 'knowledge', 'knowledge'),
    releaseDate: pickStr(r, 'release_date', 'releaseDate'),
    lastUpdated: pickStr(r, 'last_updated', 'lastUpdated'),
    interleavedJson: pickStr(r, 'interleaved_json', 'interleavedJson'),
  };
}
