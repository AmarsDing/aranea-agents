/**
 * ActivityEvent type system — the new business-semantic transport contract
 * for Activity lifecycle events, replacing the legacy Envelope-based
 * activity_* envelopes for the chat module.
 *
 * Backend source of truth:
 *   - internal/biz/activity.go        (Activity struct, ActivityKind/Status/ToolCategory)
 *   - internal/biz/activity_event.go  (ActivityEvent struct, ActivityEventType)
 *
 * The backend projects runtime events into Activity records and pushes
 * them to the frontend as ActivityEvent payloads via WS, eliminating the
 * need for frontend inference. This file mirrors the backend types so
 * the frontend can consume them in a strongly-typed way.
 *
 * Reliability levels (AS-EVT-01):
 *   - Important: created/completed/failed/cancelled/child_created
 *     → async persist with retry, sync publish
 *   - Informational: streaming/updated
 *     → async persist with drop-on-failure, sync publish (streaming may batch)
 */

// --- Union types ---

/**
 * ActivityKind classifies the semantic type of an Activity.
 * Aligned with backend biz.ActivityKind (subset used by the new system).
 *
 * Backend also defines legacy kinds (sub_task_board, error, delegate) that
 * are not part of the new Activity-First contract.
 */
export type ActivityKind =
  | 'task'
  | 'thinking'
  | 'action'
  | 'reply'
  | 'plan'
  | 'confirm'
  | 'notice'
  | 'session'
  | 'team_stage'
  | 'graph_stage';

/**
 * ActivityStatus represents the lifecycle status of an Activity.
 * Mirrors backend biz.ActivityStatus.
 */
export type ActivityStatus =
  | 'pending'
  | 'running'
  | 'tool_running'
  | 'tool_blocked'
  | 'completed'
  | 'failed'
  | 'partial_failure'
  | 'cancelled'
  | 'interrupted';

/**
 * ActivityEventType labels the business-semantic event for an Activity lifecycle.
 * Mirrors backend biz.ActivityEventType.
 *
 * These 7 types replace the technical "delta" term with business-oriented
 * "streaming", and consolidate all lifecycle events into a single enum that
 * combines with ActivityKind to express any business semantic.
 */
export type ActivityEventType =
  | 'created'
  | 'streaming'
  | 'updated'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'child_created';

/**
 * ToolCategory classifies a tool by its functional type for UI rendering.
 * Mirrors backend biz.ToolCategory.
 *
 * The frontend uses tool_category to pick the appropriate detail component
 * (shell terminal, browser card, file diff, etc.) without parsing tool_name.
 */
export type ToolCategory =
  | 'shell'
  | 'browser'
  | 'file_read'
  | 'file_write'
  | 'file_search'
  | 'web_search'
  | 'mcp'
  | 'code'
  | 'todo'
  | 'other';

// --- Activity interface ---

/**
 * Activity is the domain model for a projected semantic unit.
 * Mirrors backend biz.Activity struct (internal/biz/activity.go).
 *
 * The backend projects runtime events into Activity records and pushes
 * them to the frontend via WS, eliminating the need for frontend inference.
 *
 * Field naming follows snake_case to match the backend JSON serialization
 * convention (consistent with buildActivityEnvelope metadata keys and
 * the ActivityEvent struct's JSON tags).
 */
export interface Activity {
  /** Unique Activity identifier. */
  id: string;
  /** Semantic type of the Activity. */
  kind: ActivityKind;
  /** Lifecycle status of the Activity. */
  status: ActivityStatus;
  /** Owning session ID. */
  session_id: string;
  /** Owning turn ID (groups activities within a single user turn). */
  turn_id: string;
  /** Parent Activity ID (for child_created events / nested activities). */
  parent_activity_id: string;
  /** Emission timestamp (RFC 3339 / ISO 8601 with nanosecond precision). */
  timestamp: string;
  /** Activity duration in milliseconds. */
  duration_ms: number;
  /** Global emission sequence for stable frontend ordering. */
  seq: number;

  // --- Token usage (kind=task, root Activity only) ---
  /** Prompt tokens consumed (root task Activity only). */
  prompt_tokens: number;
  /** Completion tokens consumed (root task Activity only). */
  completion_tokens: number;

  // --- Content fields (by kind) ---
  /** task/reply/error text content. */
  content: string;
  /** thinking reasoning content. */
  reasoning: string;

  // --- Tool fields (kind=action) ---
  /** Tool name (e.g. "shell.exec", "browser.navigate"). */
  tool_name: string;
  /** Tool functional category for UI rendering. */
  tool_category: ToolCategory;
  /** Tool call ID (correlates with tool_call envelope). */
  tool_call_id: string;
  /** Tool call arguments (JSON string, redacted to 512 bytes preview). */
  tool_arguments: string;
  /** Tool call result (JSON string, redacted to 512 bytes preview). */
  tool_result: string;
  /** Tool execution duration in milliseconds. */
  tool_duration_ms: number;
  /** Tool error code (when tool execution failed). */
  tool_error_code: string;

  // --- Stage (kind=session/team_stage/graph_stage) ---
  /** Current phase: assembled/planning/executing/completed/failed etc. */
  stage: string;

  // --- Sub-task board (kind=sub_task_board) ---
  /** Child board ID for recursive nesting. */
  child_board_id: string;

  // --- Spirit extension fields ---
  /** Spirit session ID (cross-team correlation). */
  spirit_session_id: string;
  /** Team ID (for team_stage / delegate kinds). */
  team_id: string;
  /** DAG node ID (for graph-based orchestration). */
  dag_node_id: string;
  /** DAG dependency list (node IDs this activity depends on). */
  depends_on: string[];

  // --- Agent info ---
  /** Agent key (e.g. "default", "researcher"). */
  agent_key: string;
  /** Agent display name. */
  agent_name: string;

  // --- Display hints ---
  /** Whether the activity should render collapsed by default. */
  collapsed: boolean;
  /** Display label (overrides default kind-based label). */
  label: string;

  // --- Meta ---
  /** Kind-specific metadata (noticeType, toolName, steps, etc.). */
  meta: Record<string, unknown>;
}

// --- ActivityEvent interface ---

/**
 * ActivityEvent is the unified transport format for Activity lifecycle events.
 * Mirrors backend biz.ActivityEvent struct (internal/biz/activity_event.go).
 *
 * It carries the event type (what happened) and the full Activity snapshot
 * (the current state after the event). This replaces the legacy Envelope for
 * Activity-related events, providing a simpler, business-semantic contract.
 */
export interface ActivityEvent {
  /** The lifecycle event type (what happened). */
  event: ActivityEventType;
  /** Full Activity snapshot (current state after the event). */
  activity: Activity;
}

// --- WS downstream message ---

/**
 * WsDownstreamActivity is the WS downstream message wrapper for ActivityEvent.
 *
 * The WS message `type` field is 'activity_event', and the `event` field
 * carries the ActivityEvent payload. This replaces the legacy
 * activity_start/activity_delta/activity_done/activity_child_start envelopes.
 */
export interface WsDownstreamActivity {
  type: 'activity_event';
  event: ActivityEvent;
}

// --- Type guards / helpers ---

const ACTIVITY_EVENT_TYPES: readonly string[] = [
  'created',
  'streaming',
  'updated',
  'completed',
  'failed',
  'cancelled',
  'child_created',
];

/**
 * Type guard: checks whether an unknown message is an ActivityEvent.
 *
 * Validates that:
 *   1. The value is an object.
 *   2. `event` is a string and one of the ActivityEventType union values.
 *   3. `activity` is an object (further field validation is deferred to
 *      consumers to keep this guard cheap for high-frequency streaming).
 */
export function isActivityEvent(msg: unknown): msg is ActivityEvent {
  if (typeof msg !== 'object' || msg === null) return false;
  const candidate = msg as { event?: unknown; activity?: unknown };
  if (typeof candidate.event !== 'string') return false;
  if (!ACTIVITY_EVENT_TYPES.includes(candidate.event)) return false;
  if (typeof candidate.activity !== 'object' || candidate.activity === null) return false;
  return true;
}

/**
 * Checks whether an ActivityEvent is a streaming event (event === 'streaming').
 *
 * Streaming events carry incremental text/argument chunks that should be
 * appended to the existing Activity block (cursor blinking). The append
 * field is identified by activity.meta.delta_field: content/reasoning/tool_arguments.
 */
export function isStreamingEvent(e: ActivityEvent): boolean {
  return e.event === 'streaming';
}

/**
 * Checks whether an ActivityEvent is a terminal event
 * (completed / failed / cancelled).
 *
 * Terminal events indicate the Activity has reached a final state and no
 * further lifecycle events will be emitted for it. The frontend should
 * stop the streaming cursor and mark the block as terminal.
 */
export function isTerminalEvent(e: ActivityEvent): boolean {
  return e.event === 'completed' || e.event === 'failed' || e.event === 'cancelled';
}
