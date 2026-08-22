/**
 * Context window status thresholds — keep in sync with internal/llmcontext/metrics.go.
 */
export const CONTEXT_STATUS_EXCEEDED = 0.95;
export const CONTEXT_STATUS_CRITICAL = 0.8;
export const CONTEXT_STATUS_WARNING = 0.6;

/**
 * Product chat-context budget / compression standard (256K).
 * Keep in sync with internal/llmcontext.DefaultWindowTokens.
 * Provider catalog windows are informational only.
 */
export const CHAT_CONTEXT_WINDOW_TOKENS = 256_000;

export function contextStatusFromRatio(ratio: number): string {
  if (ratio >= CONTEXT_STATUS_EXCEEDED) return 'exceeded';
  if (ratio >= CONTEXT_STATUS_CRITICAL) return 'critical';
  if (ratio >= CONTEXT_STATUS_WARNING) return 'warning';
  return 'normal';
}

export function contextRatioFromPrompt(promptTokens: number, contextWindow: number): number | null {
  if (promptTokens <= 0 || contextWindow <= 0) return null;
  // 返回真实比率（不钳制到 1）；ratio > 1 表示已超出上下文窗口。
  return promptTokens / contextWindow;
}

/** Chat context ratio against the 256K product window. */
export function chatContextRatio(promptTokens: number): number | null {
  return contextRatioFromPrompt(promptTokens, CHAT_CONTEXT_WINDOW_TOKENS);
}
