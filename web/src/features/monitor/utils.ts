import { formatUsdFromMicro } from "../usage/moneyFormat";

export function parseJSON(value: string): Record<string, unknown> {
  if (!value?.trim()) return {};
  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

export function formatCount(value?: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value ?? 0);
}

export function formatMoney(value?: number) {
  return formatUsdFromMicro(value);
}

export { formatUsdFromMicro, formatUsdPer1M, formatUsdCompact } from "../usage/moneyFormat";

export function formatLatency(value?: number) {
  return `${Math.round(value ?? 0)}ms`;
}

export function formatPercent(value?: number) {
  return `${Math.round((value ?? 0) * 100)}%`;
}

export function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function compactJSON(value: unknown) {
  return JSON.stringify(value, null, 2);
}
