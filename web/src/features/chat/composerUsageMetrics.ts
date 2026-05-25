import { formatUsdCompact } from "../usage/moneyFormat";
import { contextProgressColor } from "../../components/sessions/sessionUi";

export type ComposerUsageSnapshot = {
  contextRatio: number;
  contextStatus?: string;
  contextUsedTokens?: number;
  contextWindow?: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  totalCostMicroUsd: number;
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
  if (snapshot.inputTokens > 0 || snapshot.outputTokens > 0) {
    parts.push(`in ${formatTokenCount(snapshot.inputTokens)} · out ${formatTokenCount(snapshot.outputTokens)}`);
  }
  if (snapshot.totalTokens > 0) {
    parts.push(`Σ ${formatTokenCount(snapshot.totalTokens)}`);
  }
  if (snapshot.totalCostMicroUsd > 0) {
    parts.push(formatUsdCompact(snapshot.totalCostMicroUsd));
  }
  return parts.join(" · ");
}

export function composerContextColor(contextStatus?: string, contextRatio = 0): string {
  if (contextStatus?.trim()) {
    return contextProgressColor(contextStatus);
  }
  if (contextRatio >= 0.95) return "purple";
  if (contextRatio >= 0.8) return "negative";
  if (contextRatio >= 0.6) return "warning";
  return "primary";
}
