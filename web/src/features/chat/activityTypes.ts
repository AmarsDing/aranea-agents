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
//
// Phase 3 cleanup: legacy kinds (sub_task_board, error, delegate) have been
// removed. Errors are now expressed as status=failed on the relevant kind
// (e.g. task.failed for turn errors, action.failed for tool failures,
// team_stage.failed for team errors). Spirit→Team delegation uses team_stage.

export type ActivityKind =
  | 'task' // Task description (user/agent perspective); task.failed = turn error
  | 'thinking' // Reasoning content
  | 'action' // Tool invocation
  | 'reply' // Agent reply (including final answer)
  | 'plan' // Execution plan (Spirit→Team orchestration)
  | 'notice' // System notification
  | 'confirm' // User confirmation required
  // Phase 3: Stage kinds for unified Team/Graph/Session rendering
  | 'team_stage' // Team formation/execution/completion stage
  | 'graph_stage' // Graph DAG execution stage
  | 'session'; // Child session creation/execution

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

  // === Global emission order (backend-assigned, for stable sorting) ===
  seq?: number;

  // === Content fields (by kind) ===
  content?: string | null;
  reasoning?: string | null;

  // === Tool fields (kind=action) ===
  toolName?: string | null;
  toolCategory?: string | null;
  toolCallId?: string | null;
  toolArguments?: string | null;
  toolResult?: string | null;
  toolDurationMs?: number | null;
  toolErrorCode?: string | null;

  // === Legacy sub-task board field (retained for backward-compat with persisted data) ===
  // Phase 3: sub_task_board kind removed; field kept for reading old Activity records.
  childBoardId?: string | null;

  // === Spirit extension fields ===
  spiritSessionId?: string | null;
  teamId?: string | null;
  dagNodeId?: string | null;
  dependsOn?: string[] | null;

  // === Stage phase (kind=team_stage/graph_stage/session) ===
  // Backend Activity.stage: "assembled"/"planning"/"executing"/"completed"/"failed" etc.
  stage?: string | null;

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
