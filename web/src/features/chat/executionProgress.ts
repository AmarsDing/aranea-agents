/**
 * executionProgress — pure functions to merge `execution_progress` ActivityEvents
 * into the `ProgressSection` shape consumed by the execution progress timeline.
 *
 * The backend emits an ActivityEvent per step transition (start → done/error).
 * Multiple events for the same `step_id` are merged into a single
 * `ProgressSection` whose status reflects the latest phase.
 *
 * Auto-timeout: a step still in `running` past the per-category threshold
 * (S9) flips to `timeout` so the UI can show a "(等待中)" hint. Different
 * categories have different expected durations:
 *   - thinking:    8s    (LLM thinking)
 *   - orchestration: 15s  (top-level dispatch)
 *   - tool:        60s   (network I/O, sandbox exec)
 *   - team:        30s   (multi-agent fan-out)
 *
 * @see docs/reports/2026-06-10-proposal-execution-progress-inline.md
 */
import type { ActivityEvent } from '../../realtime/activityEvent';
import type { ProgressCategory, ProgressSection } from './agentTreeTypes';

/** Metadata carried by an execution_progress ActivityEvent (on `activity.meta`). */
export type ExecutionProgressMetadata = {
  step_id: string;
  phase: 'start' | 'done' | 'error';
  message: string;
  category: 'orchestration' | 'team' | 'tool' | 'thinking';
  duration_ms?: number;
  agent_key?: string;
  tool_name?: string;
  error?: string;
};

export type ProgressStatus = ProgressSection['status'];

/**
 * Per-category auto-timeout thresholds, in milliseconds.
 * Tuning notes (calibrate from P50/P90 of real workloads):
 *   - thinking    :  8s — typical LLM first-token latency
 *   - orchestration: 15s — LLM dispatch + intent pass
 *   - team        : 30s — multi-agent fan-out, plan+execute
 *   - tool        : 60s — network / sandbox / long-running tool
 */
export const AUTO_TIMEOUT_MS: Readonly<Record<ProgressCategory, number>> = Object.freeze({
  thinking: 8_000,
  orchestration: 15_000,
  team: 30_000,
  tool: 60_000,
});

/**
 * Sentinel for "no auto-timeout" — pass `null` for a category in the
 * `timeouts` argument of `mergeProgressEventsFromActivity` to disable
 * auto-timeout.
 */
export const NO_AUTO_TIMEOUT = null;

const KNOWN_CATEGORIES: ReadonlySet<ProgressCategory> = new Set<ProgressCategory>([
  'orchestration',
  'team',
  'tool',
  'thinking',
]);

function stringField(value: unknown): string {
  if (typeof value !== 'string') return '';
  return value.trim();
}

// ── ActivityEvent-based functions ──────────────────────────────────────
// The backend projects execution_progress as ActivityEvent with
// `activity.stage = 'execution_progress'` and the metadata fields
// (step_id / phase / message / category / duration_ms / agent_key /
// tool_name / error) carried on `activity.meta`.

/**
 * Read and validate execution_progress metadata from an ActivityEvent.
 *
 * Field mapping (envelope.metadata → activity.meta):
 *   step_id, phase, message, category, duration_ms, agent_key, tool_name, error
 *
 * Returns null if the event is not an execution_progress activity or
 * metadata is missing required fields.
 */
export function readExecutionProgressMetadataFromActivity(ev: ActivityEvent): ExecutionProgressMetadata | null {
  if (ev.activity.stage !== 'execution_progress') return null;
  const meta = ev.activity.meta;
  if (!meta || typeof meta !== 'object') return null;

  const stepId = stringField(meta.step_id);
  const phase = stringField(meta.phase);
  const message = stringField(meta.message);
  const categoryRaw = stringField(meta.category);
  if (!stepId || !phase || !message) return null;
  if (phase !== 'start' && phase !== 'done' && phase !== 'error') return null;

  const category: ProgressCategory = KNOWN_CATEGORIES.has(categoryRaw as ProgressCategory)
    ? (categoryRaw as ProgressCategory)
    : 'orchestration';

  const out: ExecutionProgressMetadata = {
    step_id: stepId,
    phase,
    message,
    category,
  };
  if (typeof meta.duration_ms === 'number') out.duration_ms = meta.duration_ms;
  if (typeof meta.agent_key === 'string' && meta.agent_key.length > 0) out.agent_key = meta.agent_key;
  if (typeof meta.tool_name === 'string' && meta.tool_name.length > 0) out.tool_name = meta.tool_name;
  if (typeof meta.error === 'string' && meta.error.length > 0) out.error = meta.error;
  return out;
}

/**
 * Merge an ordered list of execution_progress ActivityEvents into a map of
 * `step_id → ProgressSection`. The last event for a given step_id wins, and
 * the start event's timestamp is used as `startedAt`.
 *
 * @param events - the stream of progress ActivityEvents in arrival order
 * @param now       - injected clock; defaults to Date.now (testability)
 * @param timeouts  - optional per-category override of AUTO_TIMEOUT_MS.
 *                    Pass `null` to disable auto-timeout for a category.
 */
export function mergeProgressEventsFromActivity(
  events: readonly ActivityEvent[],
  now: () => number = () => Date.now(),
  timeouts: Readonly<Record<ProgressCategory, number | null>> = AUTO_TIMEOUT_MS,
): Map<string, ProgressSection> {
  const byStep = new Map<string, ProgressSection>();
  for (const ev of events) {
    const meta = readExecutionProgressMetadataFromActivity(ev);
    if (!meta) continue;
    const startedAt = Date.parse(ev.activity.timestamp);
    const existing = byStep.get(meta.step_id);
    if (meta.phase === 'start') {
      byStep.set(meta.step_id, {
        id: meta.step_id,
        category: meta.category,
        message: meta.message,
        status: 'running',
        durationMs: null,
        startedAt: Number.isNaN(startedAt) ? now() : startedAt,
      });
    } else if (meta.phase === 'done') {
      if (existing) {
        existing.status = 'done';
        existing.message = meta.message;
        existing.durationMs = typeof meta.duration_ms === 'number' ? meta.duration_ms : startedAt - existing.startedAt;
      } else {
        byStep.set(meta.step_id, {
          id: meta.step_id,
          category: meta.category,
          message: meta.message,
          status: 'done',
          durationMs: typeof meta.duration_ms === 'number' ? meta.duration_ms : null,
          startedAt: Number.isNaN(startedAt) ? now() : startedAt,
        });
      }
    } else if (meta.phase === 'error') {
      if (existing) {
        existing.status = 'failed';
        existing.message = meta.message || existing.message;
      } else {
        byStep.set(meta.step_id, {
          id: meta.step_id,
          category: meta.category,
          message: meta.message,
          status: 'failed',
          durationMs: null,
          startedAt: Number.isNaN(startedAt) ? now() : startedAt,
        });
      }
    }
  }
  // Auto-timeout: any still-running step past its per-category threshold flips
  // to "timeout" so the UI can show a "(等待中)" hint. A `null` value for a
  // category disables auto-timeout for that category.
  const nowMs = now();
  for (const node of byStep.values()) {
    if (node.status !== 'running') continue;
    const threshold = timeouts[node.category];
    if (threshold == null) continue;
    if (nowMs - node.startedAt > threshold) {
      node.status = 'timeout';
    }
  }
  return byStep;
}
