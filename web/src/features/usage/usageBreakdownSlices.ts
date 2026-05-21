import type { ModelUsageBreakdownRow } from "./types";

export const USAGE_BREAKDOWN_TOP_N = 5;

export type UsageBreakdownSlice = { name: string; value: number };

/** Top N models by cost (USD). */
export function buildModelCostSlices(rows: ModelUsageBreakdownRow[], topN = USAGE_BREAKDOWN_TOP_N): UsageBreakdownSlice[] {
  return [...rows]
    .filter((r) => (r.total_cost_micro_usd ?? 0) > 0)
    .sort((a, b) => (b.total_cost_micro_usd ?? 0) - (a.total_cost_micro_usd ?? 0))
    .slice(0, topN)
    .map((r) => ({
      name: `${r.provider_code}/${r.model_display_name || r.model_api_id}`,
      value: (r.total_cost_micro_usd ?? 0) / 1_000_000
    }));
}

/**
 * Provider cost from the same Top-N model rows returned by overview API.
 * Not a full provider rollup — UI must disclose sample scope.
 */
export function buildProviderCostSlicesFromTopModels(
  rows: ModelUsageBreakdownRow[],
  topN = USAGE_BREAKDOWN_TOP_N
): UsageBreakdownSlice[] {
  const map = new Map<string, number>();
  for (const r of rows) {
    const key = r.provider_code || "unknown";
    map.set(key, (map.get(key) ?? 0) + (r.total_cost_micro_usd ?? 0));
  }
  return [...map.entries()]
    .map(([name, micro]) => ({ name, value: micro / 1_000_000 }))
    .filter((s) => s.value > 0)
    .sort((a, b) => b.value - a.value)
    .slice(0, topN);
}
