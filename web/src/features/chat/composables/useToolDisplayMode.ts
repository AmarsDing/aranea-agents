/**
 * TD-TK-3: Centralized tool display mode decision.
 *
 * When a turn has multiple tool calls, the UI must switch from the
 * per-card `ChatExecutionCard` rendering to the consolidated
 * `ToolCallTimeline`. This decision used to be inlined in TurnBlock.vue
 * (twice), ToolStrip.vue, MemberReadOnlyPanel.vue, and
 * TaskExecutionPanel.vue — all using a literal `>= 2` comparison.
 *
 * The new composable + constant eliminates that magic number and
 * guarantees the same threshold is applied everywhere, so a turn can
 * never end up with BOTH a `ChatExecutionCard` list and a
 * `ToolCallTimeline` rendering side-by-side.
 */
import { computed, type ComputedRef } from 'vue';
import type { ToolUseEvent } from './types';

/**
 * Minimum number of tool calls in a single turn that triggers the
 * timeline rendering. Below this threshold, individual
 * `ChatExecutionCard` items are rendered (richer per-card UI).
 *
 * Exported as a single source of truth — DO NOT inline `>= 2` elsewhere.
 */
export const TOOL_DISPLAY_THRESHOLD = 2;

export type ToolDisplayMode = 'timeline' | 'execution-card';

/**
 * Decide which rendering mode applies to a given list of tool events.
 *
 * - `length >= TOOL_DISPLAY_THRESHOLD` → `'timeline'`
 * - otherwise → `'execution-card'`
 *
 * Note: This composable is pure and side-effect free. It does NOT
 * check `toolDisplay.showToolCalls` — that flag hides the entire tool
 * section in the consuming component. See ChatMessagePanel.vue for the
 * `provide(TOOL_DISPLAY_KEY, ...)`.
 */
export function useToolDisplayMode(
  events: ComputedRef<readonly ToolUseEvent[]>,
): ComputedRef<ToolDisplayMode> {
  return computed<ToolDisplayMode>(() => {
    const list = events.value;
    if (!list) return 'execution-card';
    return list.length >= TOOL_DISPLAY_THRESHOLD ? 'timeline' : 'execution-card';
  });
}
