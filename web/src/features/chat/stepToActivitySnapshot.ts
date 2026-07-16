/**
 * Map a v2 Step (PascalCase WS/REST payload) onto the inspector's camelCase
 * Activity domain model. Mirrors backend biz.StepToActivity field coverage.
 */
import type { Activity, ActivityKind, ActivityStatus } from './activityTypes';
import type { Step } from './v2Types';

function toolArgsToString(v: unknown): string | undefined {
  if (v == null) return undefined;
  if (typeof v === 'string') return v;
  try {
    return JSON.stringify(v);
  } catch {
    return String(v);
  }
}

function durationMs(startedAt: string, completedAt: string | null): number | null {
  if (!startedAt || !completedAt) return null;
  const a = Date.parse(startedAt);
  const b = Date.parse(completedAt);
  if (!Number.isFinite(a) || !Number.isFinite(b) || b < a) return null;
  return b - a;
}

/** Convert a v2 Step snapshot into an Activity for SessionEventInspector. */
export function stepToActivitySnapshot(step: Step): Activity {
  const meta: Record<string, unknown> = {};
  if (step.IsFinal) meta.is_final = true;
  if (step.NoticeType) meta.notice_type = step.NoticeType;
  if (step.AuthorAgentKey) meta.agent_key = step.AuthorAgentKey;

  return {
    id: step.ID ?? '',
    kind: (step.Kind || 'notice') as ActivityKind,
    status: (step.Status || 'pending') as ActivityStatus,
    sessionId: step.SessionID || step.SpiritSessionID || '',
    turnId: step.TurnID || '',
    parentActivityId: null,
    timestamp: step.StartedAt || new Date().toISOString(),
    durationMs: durationMs(step.StartedAt, step.CompletedAt),
    seq: step.Seq,
    content: step.Content || undefined,
    reasoning: step.Reasoning || undefined,
    toolName: step.ToolName || undefined,
    toolCallId: step.ToolCallID || undefined,
    toolArguments: toolArgsToString(step.ToolArgs),
    toolResult: toolArgsToString(step.ToolResult),
    toolDurationMs: step.ToolDurationMs || undefined,
    toolErrorCode: step.ToolErrorCode || undefined,
    spiritSessionId: step.SpiritSessionID || undefined,
    agentKey: step.AuthorAgentKey || undefined,
    collapsed: false,
    meta: Object.keys(meta).length ? meta : undefined,
  };
}
