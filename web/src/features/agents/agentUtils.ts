/**
 * Agent utility functions shared between features and components.
 * Extracted from components/agents/agentUi.ts to fix F-07 (api.ts reverse dependency on components).
 */

export function tokenEstimateFor(value: string) {
  return Math.ceil((value || '').length / 4);
}
