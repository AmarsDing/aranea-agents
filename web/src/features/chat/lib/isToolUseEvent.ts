/**
 * Type guards and structural accessors for {@link ToolUseEvent}. The Message
 * model's `tool_event` field is typed as `unknown` in `domain/types.ts` (a
 * coarse-grain facade used by run/graph/team domains too). Callers that need
 * the chat-only `ToolUseEvent` shape must run their value through
 * {@link isToolUseEvent} so the cast is checked.
 */
import type { ToolUseEvent } from '../types';

export function isToolUseEvent(value: unknown): value is ToolUseEvent {
  if (!value || typeof value !== 'object') return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.tool_name === 'string' &&
    typeof v.tool_label === 'string' &&
    typeof v.phase === 'string' &&
    typeof v.status === 'string'
  );
}
