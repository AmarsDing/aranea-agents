/** Format micro-USD integers as display USD (usage totals, event costs). */
export function formatUsdFromMicro(value?: number | null, digits = 4): string {
  return `$${((value ?? 0) / 1_000_000).toFixed(digits)}`;
}

/** Format catalog / pricing rule USD per 1M tokens. */
export function formatUsdPer1M(value?: number | null, digits = 4): string {
  if (value == null || !Number.isFinite(value) || value <= 0) return "—";
  return `$${value.toFixed(digits)}/1M`;
}

/** Compact USD for dashboards (drops trailing zeros when possible). */
export function formatUsdCompact(value?: number | null): string {
  const usd = (value ?? 0) / 1_000_000;
  if (usd >= 1) return `$${usd.toFixed(2)}`;
  if (usd >= 0.01) return `$${usd.toFixed(3)}`;
  return `$${usd.toFixed(4)}`;
}
