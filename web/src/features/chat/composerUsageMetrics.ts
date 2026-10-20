import { formatUsdCompact } from '../usage/moneyFormat';
import { contextProgressColor } from '../../components/sessions/sessionUi';
import type { ContextBudgetSnapshot } from '../session/types';
import { i18n } from '../../i18n';

export type ComposerUsageSnapshot = {
  contextRatio: number;
  contextStatus?: string;
  contextUsedTokens?: number;
  contextWindow?: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  totalCostMicroUsd: number;
  /** Latest turn's prompt-assembly breakdown (WS context_usage push only). */
  contextBudget?: ContextBudgetSnapshot | null;
};

export function formatTokenCount(value?: number | null): string {
  const n = value ?? 0;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

/** Merged context + session usage line for Composer (aligned with sessions list / usage overview). */
export function formatComposerUsageDetail(snapshot: ComposerUsageSnapshot): string {
  const parts: string[] = [];
  if (snapshot.contextUsedTokens != null && snapshot.contextUsedTokens > 0) {
    const window = snapshot.contextWindow;
    if (window != null && window > 0) {
      parts.push(`ctx ${formatTokenCount(snapshot.contextUsedTokens)}/${formatTokenCount(window)}`);
    } else {
      parts.push(`ctx ${formatTokenCount(snapshot.contextUsedTokens)}`);
    }
  }
  if (snapshot.inputTokens > 0) {
    parts.push(`in ${formatTokenCount(snapshot.inputTokens)}`);
  }
  if (snapshot.outputTokens > 0) {
    parts.push(`out ${formatTokenCount(snapshot.outputTokens)}`);
  }
  if (snapshot.totalTokens > 0) {
    parts.push(`Σ ${formatTokenCount(snapshot.totalTokens)}`);
  }
  if (snapshot.totalCostMicroUsd > 0) {
    parts.push(formatUsdCompact(snapshot.totalCostMicroUsd));
  }
  return parts.join(' · ');
}

/** One chip in the header usage row. `tip` explains the metric's scope:
 * `in`/`out`/`Σ` are CUMULATIVE session-billed tokens (sum over every turn),
 * while `ctx` is the current context-window occupancy — the two scopes sit
 * side by side and need disambiguating (2026-10-20 label review). */
export type ComposerUsagePart = { text: string; tip?: string };

/** Segments for header usage row (wider spacing via flex gap in UI). */
export function composerUsageParts(snapshot: ComposerUsageSnapshot): ComposerUsagePart[] {
  const tip = (key: string) =>
    i18n.global.te(`chat.usageTips.${key}`) ? i18n.global.t(`chat.usageTips.${key}`) : undefined;
  const parts: ComposerUsagePart[] = [];
  if (snapshot.contextUsedTokens != null && snapshot.contextUsedTokens > 0) {
    const window = snapshot.contextWindow;
    const text =
      window != null && window > 0
        ? `ctx ${formatTokenCount(snapshot.contextUsedTokens)}/${formatTokenCount(window)}`
        : `ctx ${formatTokenCount(snapshot.contextUsedTokens)}`;
    parts.push({ text, tip: tip('ctx') });
  }
  if (snapshot.inputTokens > 0) {
    parts.push({ text: `in ${formatTokenCount(snapshot.inputTokens)}`, tip: tip('in') });
  }
  if (snapshot.outputTokens > 0) {
    parts.push({ text: `out ${formatTokenCount(snapshot.outputTokens)}`, tip: tip('out') });
  }
  if (snapshot.totalTokens > 0) {
    parts.push({ text: `Σ ${formatTokenCount(snapshot.totalTokens)}`, tip: tip('total') });
  }
  if (snapshot.totalCostMicroUsd > 0) {
    parts.push({ text: formatUsdCompact(snapshot.totalCostMicroUsd), tip: tip('cost') });
  }
  return parts;
}

export function composerContextColor(contextStatus?: string, contextRatio = 0): string {
  if (contextStatus?.trim()) {
    return contextProgressColor(contextStatus);
  }
  if (contextRatio >= 0.95) return 'purple';
  if (contextRatio >= 0.8) return 'negative';
  if (contextRatio >= 0.6) return 'warning';
  return 'primary';
}
