/**
 * Activity-First Architecture Types
 *
 * These types align with the backend Activity model defined in
 * internal/biz/activity.go and the WS envelope metadata in
 * internal/agent/activity_projector.go.
 *
 * ActivityKind aligns with TaskBoardNodeKind for zero-inference frontend consumption.
 */

// === ActivityKind (aligned with TaskBoardNodeKind + Spirit extensions) ===

export type ActivityKind =
  | 'task' // Task description (user/agent perspective)
  | 'thinking' // Reasoning content
  | 'action' // Tool invocation
  | 'reply' // Agent reply (including final answer)
  | 'sub_task_board' // Sub-task board (recursive nesting)
  | 'error' // Error information
  | 'delegate' // Spirit delegates to team
  | 'plan' // Execution plan (Spirit→Team orchestration)
  | 'notice' // System notification
  | 'confirm'; // User confirmation required

// === ActivityStatus ===

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

// === Activity Domain Model ===

export interface Activity {
  // === Primary key ===
  id: string;

  // === Classification ===
  kind: ActivityKind;
  status: ActivityStatus;

  // === Ownership ===
  sessionId: string;
  turnId: string;
  parentActivityId: string | null;

  // === Timing ===
  timestamp: string;
  durationMs: number | null;

  // === Content fields (by kind) ===
  content?: string | null;
  reasoning?: string | null;

  // === Tool fields (kind=action) ===
  toolName?: string | null;
  toolCallId?: string | null;
  toolArguments?: string | null;
  toolResult?: string | null;
  toolDurationMs?: number | null;
  toolErrorCode?: string | null;

  // === Sub-task board (kind=sub_task_board) ===
  childBoardId?: string | null;

  // === Spirit extension fields ===
  spiritSessionId?: string | null;
  teamId?: string | null;
  dagNodeId?: string | null;
  dependsOn?: string[] | null;

  // === Agent info ===
  agentKey?: string | null;
  agentName?: string | null;

  // === Token usage (kind=task, root Activity only) ===
  promptTokens?: number | null;
  completionTokens?: number | null;

  // === Display hints ===
  collapsed: boolean;
  label?: string | null;

  // === Kind-specific metadata ===
  meta?: Record<string, unknown> | null;
}

// === Activity Tree Node (computed from flat Activity list) ===

export interface ActivityTreeNode extends Activity {
  children: ActivityTreeNode[];
}

// === Activity Envelope Metadata (from WS events) ===

export interface ActivityStartMeta {
  activity_id: string;
  kind: ActivityKind;
  status: ActivityStatus;
  parent_activity_id: string | null;
  session_id: string;
  turn_id: string;
  timestamp: string;
  duration_ms: number | null;
  collapsed: boolean;
  content?: string | null;
  reasoning?: string | null;
  tool_name?: string | null;
  tool_call_id?: string | null;
  tool_arguments?: string | null;
  tool_result?: string | null;
  tool_duration_ms?: number | null;
  tool_error_code?: string | null;
  child_board_id?: string | null;
  spirit_session_id?: string | null;
  team_id?: string | null;
  dag_node_id?: string | null;
  depends_on?: string[] | null;
  agent_key?: string | null;
  agent_name?: string | null;
  label?: string | null;
  error_type?: string | null;
  error_code?: string | null;
  meta?: Record<string, unknown> | null;
}

export interface ActivityDeltaMeta {
  activity_id: string;
  kind: ActivityKind;
  status: ActivityStatus;
  delta_field: 'content' | 'reasoning';
  delta_chunk: string;
}

export interface ActivityDoneMeta {
  activity_id: string;
  kind: ActivityKind;
  status: ActivityStatus;
  duration_ms: number;
  collapsed: boolean;
  timestamp?: string;
  content?: string | null;
  reasoning?: string | null;
  tool_result?: string | null;
  tool_duration_ms?: number | null;
  tool_error_code?: string | null;
  child_board_id?: string | null;
  label?: string | null;
  usage?: Record<string, unknown>;
  error_type?: string | null;
  error_code?: string | null;
}

export interface ActivityChildStartMeta {
  activity_id: string;
  kind: ActivityKind;
  status: ActivityStatus;
  parent_activity_id: string | null;
  child_board_id?: string | null;
  team_id?: string | null;
  spirit_session_id?: string | null;
  dag_node_id?: string | null;
  depends_on?: string[] | null;
}
