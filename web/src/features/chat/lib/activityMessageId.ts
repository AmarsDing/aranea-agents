/**
 * Stable message-id derivation for tool-call activity rows. Mirrors the Go
 * ActivityMessageID helper (internal/agent/activity_persist.go). Must be used
 * by every code path that produces or upserts a tool activity message so the
 * `act-{tool_call_id}` key stays unique.
 */
import type { ToolUseEvent } from '../types';

export function activityMessageId(event: Pick<ToolUseEvent, 'id' | 'agent_id' | 'agent_key' | 'tool_name'>): string {
  if (event.id?.trim()) return `act-${event.id.trim()}`;
  const owner = event.agent_id || event.agent_key || 'agent';
  return `tool-${owner}-${event.tool_name}`;
}
