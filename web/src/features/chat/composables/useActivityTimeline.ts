import { shallowRef, computed, triggerRef, onUnmounted, getCurrentInstance, type ComputedRef } from 'vue';
import type {
  Activity,
  ActivityKind,
  ActivityStatus,
  ActivityTreeNode,
  ActivityStartMeta,
  ActivityDeltaMeta,
  ActivityDoneMeta,
  ActivityChildStartMeta,
} from '../activityTypes';
import type { TaskBoardNodeData, TaskBoardNodeKind, TaskBoardToolStatus } from '../agentTreeTypes';
import type {
  Activity as TimelineActivity,
  ThinkingEvent,
  ActionEvent,
  ReplyEvent,
  ErrorEvent,
  PlanEvent,
  ToolActivity,
} from '../streamEventTypes';
import { listActivities } from '../../session/api';

/**
 * useActivityTimeline manages Activity state for the Activity-First architecture.
 * It consumes WS activity events (activity_start/delta/done/child_start) and
 * provides computed properties for rendering TaskBoard and other components.
 *
 * This composable replaces the legacy inference-based approach with zero-inference
 * Activity consumption from the backend.
 */
export function useActivityTimeline() {
  const activities = shallowRef<Map<string, Activity>>(new Map());
  const rootActivityId = shallowRef<string | null>(null);

  // === Activity tree computed from flat list ===

  const activityTree = computed<ActivityTreeNode[]>(() => {
    const map = new Map<string, ActivityTreeNode>();
    const roots: ActivityTreeNode[] = [];

    // Build tree nodes
    for (const activity of activities.value.values()) {
      map.set(activity.id, { ...activity, children: [] });
    }

    // Link children to parents
    for (const node of map.values()) {
      if (node.parentActivityId && map.has(node.parentActivityId)) {
        map.get(node.parentActivityId)!.children.push(node);
      } else if (!node.parentActivityId) {
        roots.push(node);
      }
    }

    // Sort tree by timestamp to ensure correct order even if WS events arrive out-of-order
    const sortTree = (nodes: ActivityTreeNode[]) => {
      nodes.sort((a, b) => a.timestamp.localeCompare(b.timestamp));
      for (const node of nodes) sortTree(node.children);
    };
    sortTree(roots);

    return roots;
  });

  // === TaskBoardNodes computed from Activity tree ===

  const taskBoardNodes = computed<TaskBoardNodeData[]>(() => {
    return activityTree.value
      .filter((node) => node.kind !== 'delegate' && node.kind !== 'error')
      .map(activityToTaskBoardNode);
  });

  // === Timeline Activities (AF → activityTimelineTypes bridge) ===

  const timelineActivities = computed<TimelineActivity[]>(() => {
    return activityTree.value
      .filter((node) => node.kind !== 'task' && node.kind !== 'sub_task_board' && node.kind !== 'delegate')
      .map(activityToTimelineActivity);
  });

  // === WS Event Handlers ===

  function handleActivityStart(meta: ActivityStartMeta) {
    const activity: Activity = {
      id: meta.activity_id,
      kind: meta.kind,
      status: meta.status,
      sessionId: meta.session_id,
      turnId: meta.turn_id,
      parentActivityId: meta.parent_activity_id || null,
      timestamp: meta.timestamp,
      durationMs: meta.duration_ms || null,
      content: meta.content,
      reasoning: meta.reasoning,
      toolName: meta.tool_name,
      toolCallId: meta.tool_call_id,
      toolArguments: meta.tool_arguments,
      toolResult: meta.tool_result,
      toolDurationMs: meta.tool_duration_ms,
      toolErrorCode: meta.tool_error_code,
      childBoardId: meta.child_board_id,
      spiritSessionId: meta.spirit_session_id,
      teamId: meta.team_id,
      dagNodeId: meta.dag_node_id,
      dependsOn: meta.depends_on,
      agentKey: meta.agent_key,
      agentName: meta.agent_name,
      collapsed: meta.collapsed,
      label: meta.label,
    };
    activities.value.set(activity.id, activity);
    triggerRef(activities);

    // Track root activity
    if (!activity.parentActivityId) {
      rootActivityId.value = activity.id;
    }
  }

  function handleActivityDelta(meta: ActivityDeltaMeta) {
    const existing = activities.value.get(meta.activity_id);
    if (!existing) return;

    // Mutate in place, then trigger reactivity
    if (meta.delta_field === 'reasoning') {
      existing.reasoning = (existing.reasoning || '') + meta.delta_chunk;
    } else if (meta.delta_field === 'content') {
      existing.content = (existing.content || '') + meta.delta_chunk;
    }

    triggerRef(activities);
  }

  function handleActivityDone(meta: ActivityDoneMeta) {
    const existing = activities.value.get(meta.activity_id);
    if (!existing) return;

    // Mutate in place, then trigger reactivity
    existing.status = meta.status as ActivityStatus;
    existing.durationMs = meta.duration_ms;
    existing.collapsed = meta.collapsed;
    if (meta.content !== undefined) existing.content = meta.content;
    if (meta.reasoning !== undefined) existing.reasoning = meta.reasoning;
    if (meta.tool_result !== undefined) existing.toolResult = meta.tool_result;
    if (meta.tool_duration_ms !== undefined) existing.toolDurationMs = meta.tool_duration_ms;
    if (meta.tool_error_code !== undefined) existing.toolErrorCode = meta.tool_error_code;
    if (meta.child_board_id !== undefined) existing.childBoardId = meta.child_board_id;
    if (meta.label !== undefined) existing.label = meta.label;

    triggerRef(activities);
  }

  function handleActivityChildStart(meta: ActivityChildStartMeta) {
    const activity: Activity = {
      id: meta.activity_id,
      kind: meta.kind,
      status: meta.status,
      sessionId: '',
      turnId: '',
      parentActivityId: meta.parent_activity_id,
      timestamp: new Date().toISOString(),
      durationMs: null,
      childBoardId: meta.child_board_id,
      teamId: meta.team_id,
      spiritSessionId: meta.spirit_session_id,
      dagNodeId: meta.dag_node_id,
      dependsOn: meta.depends_on,
      collapsed: false,
    };
    activities.value.set(activity.id, activity);
    triggerRef(activities);
  }

  // === Reset (called on turn start) ===

  function reset() {
    activities.value.clear();
    triggerRef(activities);
    rootActivityId.value = null;
  }

  // === Load activities from API (for history recovery) ===

  function loadActivities(activityList: Activity[]) {
    // Atomic replacement: build a complete new Map before assigning to the
    // shallowRef. This avoids the intermediate empty-map state that causes
    // a flash when the old Map is cleared and the new data hasn't been
    // written yet. shallowRef detects the reference change and triggers
    // reactivity exactly once — no manual triggerRef needed.
    const newMap = new Map<string, Activity>();
    let newRootId: string | null = null;
    for (const a of activityList) {
      newMap.set(a.id, a);
      if (!a.parentActivityId) {
        newRootId = a.id;
      }
    }
    activities.value = newMap;
    rootActivityId.value = newRootId;
  }

  // AF-FE-14: Load activities from backend API for history recovery
  // Retries up to 2 times with exponential backoff (500ms, 1000ms) on failure.
  // On final failure, logs a warning and returns silently — the UI falls back
  // to the legacy "isLegacy" path in useConversationTimeline.
  async function loadActivitiesFromAPI(sessionId: string, turnId?: string) {
    const maxAttempts = 3;
    const baseDelay = 500;
    let lastErr: unknown;

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        const activityList = await listActivities(sessionId, turnId);
        loadActivities(activityList);
        return;
      } catch (err) {
        lastErr = err;
        if (attempt < maxAttempts - 1) {
          const delay = baseDelay * (2 ** attempt);
          await new Promise((resolve) => setTimeout(resolve, delay));
        }
      }
    }

    console.warn('[activity] failed to load activities from API after', maxAttempts, 'attempts:', lastErr);
  }

  // === Cleanup on unmount (only when inside a component instance) ===

  if (getCurrentInstance()) {
    onUnmounted(() => {
      reset();
    });
  }

  return {
    activities: computed(() => Array.from(activities.value.values())),
    activityTree,
    taskBoardNodes,
    timelineActivities,
    rootActivityId,
    handleActivityStart,
    handleActivityDelta,
    handleActivityDone,
    handleActivityChildStart,
    reset,
    loadActivities,
    loadActivitiesFromAPI,
  };
}

// === Mapping Functions ===

/**
 * Maps ActivityKind to TaskBoardNodeKind.
 * delegate is not rendered as TaskBoardNodes.
 * Exported for use by activityToTaskBoardNode (used by useConversationTimeline).
 */
export function mapActivityKindToNodeKind(kind: ActivityKind): TaskBoardNodeKind | null {
  switch (kind) {
    case 'task':
    case 'thinking':
    case 'action':
    case 'reply':
    case 'sub_task_board':
    case 'error':
      return kind;
    default:
      return null;
  }
}

/**
 * Maps ActivityStatus to TaskBoardToolStatus.
 * Exported for use by activityToTaskBoardNode (used by useConversationTimeline).
 */
export function mapActivityStatusToToolStatus(status: ActivityStatus): TaskBoardToolStatus {
  switch (status) {
    case 'tool_running':
      return 'running';
    case 'tool_blocked':
      return 'blocked';
    case 'completed':
      return 'success';
    case 'failed':
    case 'partial_failure':
      return 'failed';
    case 'cancelled':
    case 'interrupted':
      return 'cancelled';
    default:
      return 'running';
  }
}

/**
 * Converts an ActivityTreeNode to a TaskBoardNodeData for rendering.
 * Exported for use by useConversationTimeline to compute taskBoardNodes.
 */
export function activityToTaskBoardNode(node: ActivityTreeNode): TaskBoardNodeData {
  const nodeKind = mapActivityKindToNodeKind(node.kind);
  if (!nodeKind) {
    // Fallback for delegate
    return {
      kind: 'task',
      content: node.content || node.label || '',
    };
  }

  const result: TaskBoardNodeData = {
    kind: nodeKind,
    content: node.content || node.reasoning,
    durationMs: node.durationMs,
    streaming: node.status === 'running' || node.status === 'tool_running',
  };

  // Tool fields for action kind
  if (node.kind === 'action') {
    result.toolName = node.toolName;
    result.toolStatus = mapActivityStatusToToolStatus(node.status);
    result.arguments = node.toolArguments || null;
    result.result = node.toolResult || null;
  }

  // Children for sub_task_board — recursively map to TaskBoardNodeData[]
  if (node.kind === 'sub_task_board' && node.children.length > 0) {
    const children: TaskBoardNodeData[] = [];
    for (const child of node.children) {
      const childNodeKind = mapActivityKindToNodeKind(child.kind);
      if (childNodeKind === null) continue;
      children.push(activityToTaskBoardNode(child));
    }
    result.children = children.length > 0 ? children : undefined;
  }

  return result;
}

/**
 * Maps an AF ActivityTreeNode to a TimelineActivity (activityTimelineTypes).
 * This bridges the AF backend Activity model to the frontend rendering model.
 */
export function activityToTimelineActivity(node: ActivityTreeNode): TimelineActivity {
  switch (node.kind) {
    case 'thinking':
      return {
        kind: 'thinking',
        id: node.id,
        content: node.reasoning || node.content || '',
        label: node.label || undefined,
        collapsed: node.collapsed,
        streaming: node.status === 'running',
        durationMs: node.durationMs,
      } satisfies ThinkingEvent;

    case 'action': {
      const toolStatus = mapActivityStatusToToolStatus(node.status);
      const tool: ToolActivity = {
        toolName: node.toolName || '',
        toolLabel: node.label || node.toolName || '',
        status: toolStatus,
        durationMs: node.toolDurationMs ?? node.durationMs,
        arguments: node.toolArguments || null,
        result: node.toolResult || null,
        error: node.toolErrorCode || null,
      };
      return {
        kind: 'action',
        id: node.id,
        tool,
      } satisfies ActionEvent;
    }

    case 'reply':
      return {
        kind: 'reply',
        id: node.id,
        content: node.content || '',
        isFinal: node.status === 'completed' || node.status === 'failed',
        streaming: node.status === 'running',
        variant: 'default',
        durationMs: node.durationMs,
      } satisfies ReplyEvent;

    case 'error':
      return {
        kind: 'error',
        id: node.id,
        type: 'degradation',
        message: node.content || node.toolErrorCode || '',
      } satisfies ErrorEvent;

    case 'plan':
      return {
        kind: 'plan',
        id: node.id,
        title: node.label || node.content || '',
        steps: [],
      } satisfies PlanEvent;

    default:
      // Fallback: task, sub_task_board, delegate → error
      return {
        kind: 'error',
        id: node.id,
        type: 'info',
        message: node.content || node.label || '',
      } satisfies ErrorEvent;
  }
}
