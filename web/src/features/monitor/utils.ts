import { formatUsdFromMicro } from '../usage/moneyFormat';
import type { AlertMetricInfo, MonitorAlertRule } from './types';

export function parseJSON(value: string): Record<string, unknown> {
  if (!value?.trim()) return {};
  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

export function formatCount(value?: number) {
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(value ?? 0);
}

export function formatMoney(value?: number) {
  return formatUsdFromMicro(value);
}

export { formatUsdFromMicro, formatUsdPer1M, formatUsdCompact } from '../usage/moneyFormat';

export function formatLatency(value?: number) {
  return `${Math.round(value ?? 0)}ms`;
}

export function formatPercent(value?: number) {
  return `${Math.round((value ?? 0) * 100)}%`;
}

export function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function compactJSON(value: unknown) {
  return JSON.stringify(value, null, 2);
}

/**
 * Alert metric helpers — shared by the metric directory panel and the rule
 * editor on the Alerts tab.
 */

/** i18n key for a metric's localized field; metric keys contain dots which
 * vue-i18n treats as path separators, so they are flattened to underscores. */
export function metricI18nKey(metricKey: string, field: 'name' | 'description'): string {
  return `monitorPage.alerts.metrics.${metricKey.replace(/\./g, '_')}.${field}`;
}

/** Human-readable metric value: ratios render as percentages, counts as integers. */
export function formatMetricValue(unit: string, value?: number): string {
  const v = value ?? 0;
  if (unit === 'ratio') return `${(v * 100).toFixed(1)}%`;
  return formatCount(v);
}

export type AlertRuleState = 'ok' | 'firing' | 'disabled' | 'unknown';

/** Live status of a rule against the metric directory's current value. */
export function alertRuleStateOf(
  rule: Pick<MonitorAlertRule, 'enabled' | 'threshold' | 'metric_key'>,
  metric?: AlertMetricInfo,
): AlertRuleState {
  if (!rule.enabled) return 'disabled';
  if (!metric) return 'unknown';
  return metric.current_value >= rule.threshold ? 'firing' : 'ok';
}

/**
 * 提取审计 detail JSON 契约（{"summary":..., "before":..., "after":...}）中的摘要文本。
 * 非 JSON（历史纯文本）或缺失 summary 时原样返回。
 */
export function auditDetailSummary(detail: string): string {
  if (!detail?.trim()) return '';
  const parsed = parseJSON(detail);
  const summary = parsed.summary;
  return typeof summary === 'string' && summary ? summary : detail;
}
