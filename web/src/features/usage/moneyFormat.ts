/** Format micro-USD integers as display USD (usage totals, event costs). */
export function formatUsdFromMicro(value?: number | null, digits = 4): string {
  return `$${((value ?? 0) / 1_000_000).toFixed(digits)}`;
}

/** Format catalog / pricing rule USD per 1M tokens. */
export function formatUsdPer1M(value?: number | null, digits = 4): string {
  if (value == null || !Number.isFinite(value) || value <= 0) return '—';
  return `$${value.toFixed(digits)}/1M`;
}

/** Compact USD for dashboards (drops trailing zeros when possible). */
export function formatUsdCompact(value?: number | null): string {
  const usd = (value ?? 0) / 1_000_000;
  if (usd >= 1) return `$${usd.toFixed(2)}`;
  if (usd >= 0.01) return `$${usd.toFixed(3)}`;
  return `$${usd.toFixed(4)}`;
}

/** Format integer count with locale grouping. */
export function formatCount(value?: number | null): string {
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(value ?? 0);
}

/** Format ratio (0–1) as percentage string. */
export function formatPercent(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '—';
  return `${Math.round(value * 100)}%`;
}

/** Format latency in milliseconds. */
export function formatLatencyMs(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '—';
  return `${Math.round(value)} ms`;
}

/** Format tokens-per-second throughput. */
export function formatTps(value?: number | null): string {
  if (!value || !Number.isFinite(value)) return '—';
  return `${value.toFixed(1)} tok/s`;
}
