/**
 * Context window status thresholds — keep in sync with internal/llmcontext/metrics.go.
 */
export const CONTEXT_STATUS_EXCEEDED = 0.95;
export const CONTEXT_STATUS_CRITICAL = 0.8;
export const CONTEXT_STATUS_WARNING = 0.6;

export function contextStatusFromRatio(ratio: number): string {
  if (ratio >= CONTEXT_STATUS_EXCEEDED) return 'exceeded';
  if (ratio >= CONTEXT_STATUS_CRITICAL) return 'critical';
  if (ratio >= CONTEXT_STATUS_WARNING) return 'warning';
  return 'normal';
}

export function contextRatioFromPrompt(promptTokens: number, contextWindow: number): number | null {
  if (promptTokens <= 0 || contextWindow <= 0) return null;
  return Math.min(1, promptTokens / contextWindow);
}
