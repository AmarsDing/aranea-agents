/**
 * Agent utility functions shared between features and components.
 * Extracted from components/agents/agentUi.ts to fix F-07 (api.ts reverse dependency on components).
 */

/**
 * Rough token estimate matching the backend formula (internal/biz
 * estimateTokensForFiles): UTF-8 bytes / 4, min 1 for non-empty text.
 * JS `length` counts UTF-16 units, which undercounts CJK ~3x vs the backend's
 * byte-based `len(body)/4`; encoding first keeps both sides consistent.
 */
export function tokenEstimateFor(value: string) {
  const text = value || '';
  if (!text) return 0;
  const bytes = new TextEncoder().encode(text).length;
  return Math.max(1, Math.floor(bytes / 4));
}
