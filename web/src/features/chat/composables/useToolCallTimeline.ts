/**
 * TK-02/TK-03: Tool Call Timeline composable.
 *
 * Sorts ToolUseEvent[] and builds ToolCallTimelineNode for display.
 */
import { computed } from 'vue';
import type { ComputedRef } from 'vue';
import type { ToolUseEvent } from '../types';
import type { ToolCallTimelineNode } from '../timelineTypes';
import { isStuckTool } from '../lib/isStuckTool';
import { resolveDisplayLabel, formatDurationLabel, maskSensitiveJSON } from '../activityPresentation';

// ── Status point mapping ──

type StatusPoint = ToolCallTimelineNode['statusPoint'];

const STATUS_POINT_MAP: Record<string, StatusPoint> = {
  running: { color: 'var(--color-warning)', icon: 'hourglass_top', animated: true },
  success: { color: 'var(--color-success)', icon: 'check_circle', animated: false },
  failed: { color: 'var(--color-danger)', icon: 'error', animated: false },
  error: { color: 'var(--color-danger)', icon: 'error', animated: false },
  blocked: { color: 'var(--color-warning)', icon: 'warning', animated: false },
  cancelled: { color: 'var(--color-text-tertiary)', icon: 'cancel', animated: false },
};

const DEFAULT_STATUS_POINT: StatusPoint = {
  color: 'var(--color-text-tertiary)',
  icon: 'help',
  animated: false,
};

// ── Helpers ──

function extractTimestamp(occurredAt: string): string {
  try {
    const d = new Date(occurredAt);
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    const ss = String(d.getSeconds()).padStart(2, '0');
    return `${hh}:${mm}:${ss}`;
  } catch {
    return '';
  }
}

function truncate(str: string, max: number): string {
  if (str.length <= max) return str;
  return str.slice(0, max) + '…';
}

function buildArgsPreview(args: unknown): string | undefined {
  if (args == null) return undefined;
  try {
    const safe = maskSensitiveJSON(args);
    const json = JSON.stringify(safe, null, 2);
    return truncate(json, 300);
  } catch {
    return truncate(String(args), 300);
  }
}

function buildResultPreview(result: unknown): string | undefined {
  if (result == null) return undefined;
  try {
    const safe = maskSensitiveJSON(result);
    const json = JSON.stringify(safe, null, 2);
    return truncate(json, 300);
  } catch {
    return truncate(String(result), 300);
  }
}

// ── buildTimelineNode ──

export function buildTimelineNode(event: ToolUseEvent): ToolCallTimelineNode {
  const stuck = isStuckTool(event);
  const status = event.status ?? 'running';

  let statusPoint: StatusPoint;
  if (stuck) {
    statusPoint = { color: 'var(--color-danger)', icon: 'error', animated: false };
  } else {
    statusPoint = STATUS_POINT_MAP[status] ?? DEFAULT_STATUS_POINT;
  }

  const label = resolveDisplayLabel(event);
  const argsPreview = buildArgsPreview(event.arguments);
  let summary = label;
  if (argsPreview) {
    summary += ` ${truncate(argsPreview.split('\n')[0], 60)}`;
  }

  return {
    event,
    timestamp: extractTimestamp(event.occurred_at),
    statusPoint,
    summary: summary.trim(),
    argsPreview,
    resultPreview: buildResultPreview(event.result),
    errorText: event.error?.trim() || undefined,
    i18nKey: event.i18n_key || undefined,
    durationLabel: formatDurationLabel(event.duration_ms),
    isStuck: stuck,
  };
}

// ── Composable ──

export function useToolCallTimeline(events: ComputedRef<ToolUseEvent[]>) {
  const sortedEvents = computed(() => {
    return [...events.value].sort((a, b) => {
      const ta = a.occurred_at ?? '';
      const tb = b.occurred_at ?? '';
      if (ta !== tb) return ta < tb ? -1 : 1;
      // Same ms → dictionary order by id
      const idA = a.id ?? '';
      const idB = b.id ?? '';
      return idA < idB ? -1 : idA > idB ? 1 : 0;
    });
  });

  return { sortedEvents };
}
