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
  | 'end' // Task completion marker
  | 'error' // Error information
  | 'delegate' // Spirit delegates to team
  | 'notice'; // System notification (context loading, status change)

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
  content?: string;
  reasoning?: string;

  // === Tool fields (kind=action) ===
  toolName?: string;
  toolCallId?: string;
  toolArguments?: string;
  toolResult?: string;
  toolDurationMs?: number;
  toolErrorCode?: string;

  // === Sub-task board (kind=sub_task_board) ===
  childBoardId?: string;

  // === Spirit extension fields ===
  spiritSessionId?: string;
  teamId?: string;
  dagNodeId?: string;
  dependsOn?: string[];

  // === Agent info ===
  agentKey?: string;
  agentName?: string;

  // === Display hints ===
  collapsed: boolean;
  label?: string;
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
  parent_activity_id: string;
  session_id: string;
  turn_id: string;
  timestamp: string;
  duration_ms: number;
  collapsed: boolean;
  content?: string;
  reasoning?: string;
  tool_name?: string;
  tool_call_id?: string;
  tool_arguments?: string;
  tool_result?: string;
  tool_duration_ms?: number;
  tool_error_code?: string;
  child_board_id?: string;
  spirit_session_id?: string;
  team_id?: string;
  dag_node_id?: string;
  depends_on?: string[];
  agent_key?: string;
  agent_name?: string;
  label?: string;
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
  content?: string;
  reasoning?: string;
  tool_result?: string;
  tool_duration_ms?: number;
  tool_error_code?: string;
  child_board_id?: string;
  label?: string;
}

export interface ActivityChildStartMeta {
  activity_id: string;
  kind: ActivityKind;
  status: ActivityStatus;
  parent_activity_id: string;
  child_board_id?: string;
  team_id?: string;
  spirit_session_id?: string;
  dag_node_id?: string;
  depends_on?: string[];
}
