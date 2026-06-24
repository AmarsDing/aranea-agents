import { shallowRef, computed, triggerRef, onUnmounted, getCurrentInstance } from 'vue';
import type {
  Activity,
  ActivityStatus,
  ActivityTreeNode,
  ActivityStartMeta,
  ActivityDeltaMeta,
  ActivityDoneMeta,
  ActivityChildStartMeta,
} from '../activityTypes';
import type {
  StreamEvent,
  ThinkingEvent,
  ActionEvent,
  ReplyEvent,
  ErrorEvent,
  PlanEvent,
  PlanStep,
  ConfirmEvent,
  NoticeEvent,
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
  const loadError = shallowRef<string | null>(null);

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

    // Sort tree by timestamp, then by backend-assigned global seq, to ensure
    // correct order even when the per-activity sequencer publishes different
    // activities concurrently (e.g. thinking done vs reply start).
    const sortTree = (nodes: ActivityTreeNode[]) => {
      nodes.sort(compareActivities);
      for (const node of nodes) sortTree(node.children);
    };
    sortTree(roots);

    return roots;
  });

  // === Stream Events (AF → streamEventTypes bridge) ===

  const streamEvents = computed<StreamEvent[]>(() => {
    return activityTree.value
      .filter((node) => node.kind !== 'task' && node.kind !== 'sub_task_board' && node.kind !== 'delegate')
      .map(activityToStreamEvent);
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
      meta: meta.meta,
      seq: meta._seq,
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

    // Create a new object to ensure reference change for reactivity
    const updated: Activity = { ...existing };
    if (meta.delta_field === 'reasoning') {
      updated.reasoning = (existing.reasoning || '') + meta.delta_chunk;
    } else if (meta.delta_field === 'content') {
      updated.content = (existing.content || '') + meta.delta_chunk;
    } else if (meta.delta_field === 'tool_arguments') {
      updated.toolArguments = (existing.toolArguments || '') + meta.delta_chunk;
    }
    if (meta._seq != null) updated.seq = meta._seq;
    activities.value.set(meta.activity_id, updated);
    triggerRef(activities);
  }

  function handleActivityDone(meta: ActivityDoneMeta) {
    const existing = activities.value.get(meta.activity_id);
    if (!existing) return;

    // Create a new object to ensure reference change for reactivity
    const updated: Activity = {
      ...existing,
      status: meta.status as ActivityStatus,
      durationMs: meta.duration_ms,
      collapsed: meta.collapsed,
    };
    if (meta.content !== undefined) updated.content = meta.content;
    if (meta.reasoning !== undefined) updated.reasoning = meta.reasoning;
    if (meta.tool_result !== undefined) updated.toolResult = meta.tool_result;
    if (meta.tool_duration_ms !== undefined) updated.toolDurationMs = meta.tool_duration_ms;
    if (meta.tool_error_code !== undefined) updated.toolErrorCode = meta.tool_error_code;
    if (meta.child_board_id !== undefined) updated.childBoardId = meta.child_board_id;
    if (meta.label !== undefined) updated.label = meta.label;
    if (meta._seq != null) updated.seq = meta._seq;
    activities.value.set(meta.activity_id, updated);
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

  // AF-FE-15: Coerce numeric fields that the backend may serialize as JSON
  // strings (e.g. `durationMs: "1745"`) into actual numbers. Without this,
  // downstream `sum + (s.durationMs ?? 0)` becomes implicit string
  // concatenation (`1745 + "1588" === "17451588"`), which corrupts
  // displayed durations like "290m 52s".
  function normalizeActivityNumericFields(a: Activity): Activity {
    return {
      ...a,
      durationMs: toNumberOrNull(a.durationMs),
      toolDurationMs: a.toolDurationMs == null ? undefined : (toNumberOrNull(a.toolDurationMs) ?? undefined),
    };
  }

  function toNumberOrNull(v: unknown): number | null {
    if (v == null) return null;
    if (typeof v === 'number' && Number.isFinite(v)) return v;
    if (typeof v === 'string') {
      const trimmed = v.trim();
      if (!trimmed) return null;
      const n = Number(trimmed);
      return Number.isFinite(n) ? n : null;
    }
    return null;
  }

  function loadActivities(activityList: Activity[]) {
    // Atomic replacement: build a complete new Map before assigning to the
    // shallowRef. This avoids the intermediate empty-map state that causes
    // a flash when the old Map is cleared and the new data hasn't been
    // written yet. shallowRef detects the reference change and triggers
    // reactivity exactly once — no manual triggerRef needed.
    const newMap = new Map<string, Activity>();
    let newRootId: string | null = null;
    let fallbackSeq = 1;
    for (const a of activityList) {
      // AF-FE-15: Backend may return numeric fields as JSON strings (e.g.
      // durationMs="1745"). Coerce them here so downstream reduce/sum/arithmetic
      // never falls into implicit string concatenation (e.g. 1745 + "1588"
      // = "17451588"), which would corrupt displayed durations and turn totals.
      const normalized = normalizeActivityNumericFields(a);
      // Older backends may not persist seq. Preserve it if present, otherwise
      // derive a stable fallback from the API order (already timestamp-sorted).
      const withSeq: Activity = {
        ...normalized,
        seq: normalized.seq ?? fallbackSeq++,
      };
      newMap.set(a.id, withSeq);
      if (!a.parentActivityId) {
        newRootId = a.id;
      }
    }
    activities.value = newMap;
    rootActivityId.value = newRootId;
  }

  // AF-FE-14 / AF-GAP-05: Load activities from backend API for history recovery.
  // Retries up to 5 times with exponential backoff (500ms, 1s, 2s, 4s) on failure.
  // On final failure, sets loadError so the UI can show a "数据加载失败，请刷新"
  // degradation notice instead of silently falling back to Legacy rendering.
  async function loadActivitiesFromAPI(sessionId: string, turnId?: string) {
    const maxAttempts = 5;
    const baseDelay = 500;
    let lastErr: unknown;

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        const activityList = await listActivities(sessionId, turnId);
        loadActivities(activityList);
        loadError.value = null;
        return;
      } catch (err) {
        lastErr = err;
        if (attempt < maxAttempts - 1) {
          const delay = baseDelay * 2 ** attempt;
          await new Promise((resolve) => setTimeout(resolve, delay));
        }
      }
    }

    const msg = lastErr instanceof Error ? lastErr.message : String(lastErr);
    loadError.value = msg;
    console.warn('[activity] failed to load activities from API after', maxAttempts, 'attempts:', msg);
  }

  /** Retry loading activities after a previous failure */
  async function retryLoad(sessionId: string, turnId?: string) {
    loadError.value = null;
    await loadActivitiesFromAPI(sessionId, turnId);
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
    streamEvents,
    rootActivityId,
    loadError: computed(() => loadError.value),
    handleActivityStart,
    handleActivityDelta,
    handleActivityDone,
    handleActivityChildStart,
    reset,
    loadActivities,
    loadActivitiesFromAPI,
    retryLoad,
  };
}

// === Mapping Functions ===

/**
 * Maps ActivityStatus to ToolActivity status.
 * Used by activityToStreamEvent for the action kind.
 */
export function mapActivityStatusToToolStatus(status: ActivityStatus): ToolActivity['status'] {
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
 * Maps backend ActivityStatus to frontend PlanEvent.status.
 * Backend: pending → running → completed / partial_failure / failed
 * Frontend: planning → executing → completed / failed
 */
function mapPlanStatus(status: ActivityStatus): PlanEvent['status'] {
  switch (status) {
    case 'pending':
      return 'planning';
    case 'running':
      return 'executing';
    case 'completed':
      return 'completed';
    case 'partial_failure':
    case 'failed':
      return 'failed';
    default:
      return 'planning';
  }
}

/**
 * Maps an AF ActivityTreeNode to a StreamEvent (streamEventTypes).
 * This bridges the AF backend Activity model to the frontend rendering model.
 *
 * B-04: For plan nodes, the node's tree children are mapped to StreamEvent[]
 * and stored in PlanEvent.children, eliminating the need for PlanBlock to
 * directly consume activityTree.
 */
export function activityToStreamEvent(node: ActivityTreeNode): StreamEvent {
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
        // P3-4: surface the error code so ErrorBlock can pick an inline
        // action. Prefer toolErrorCode (turn-level), then meta.error_code
        // (backend apierror.Code), so the most specific code wins.
        errorCode: node.toolErrorCode || (node.meta?.error_code as string | undefined) || undefined,
      } satisfies ErrorEvent;

    case 'plan': {
      // Map backend ActivityStatus → frontend PlanEvent.status
      const planStatus = mapPlanStatus(node.status);
      // Extract steps from meta (serialized from Go ActivityPlanStep[])
      const metaSteps = Array.isArray(node.meta?.steps) ? (node.meta.steps as PlanStep[]) : [];
      // B-04: Map tree children to StreamEvent[] for PlanBlock rendering.
      // This replaces the direct activityTree consumption in PlanBlock,
      // unifying the data flow through the event model.
      const childEvents: StreamEvent[] = (node.children ?? [])
        .filter((child) => child.kind !== 'task' && child.kind !== 'sub_task_board' && child.kind !== 'delegate')
        .map((child) => activityToStreamEvent(child));
      return {
        kind: 'plan',
        id: node.id,
        title: node.label || node.content || '',
        steps: metaSteps,
        status: planStatus,
        children: childEvents.length > 0 ? childEvents : undefined,
      } satisfies PlanEvent;
    }

    case 'notice': {
      const noticeType = (node.meta?.noticeType as NoticeEvent['type']) ?? 'info';
      return {
        kind: 'notice',
        id: node.id,
        type: noticeType,
        message: node.content || '',
      } satisfies NoticeEvent;
    }

    case 'confirm':
      return {
        kind: 'confirm',
        id: node.id,
        status: node.status as ConfirmEvent['status'],
        content: node.content || '',
        toolName: (node.meta?.toolName as string) || '',
        toolArguments: (node.meta?.toolArguments as string) ?? null,
        autoApproveAt: (node.meta?.autoApproveAt as string) ?? null,
      } satisfies ConfirmEvent;

    default:
      // Fallback: task, sub_task_board, delegate → error (degradation)
      return {
        kind: 'error',
        id: node.id,
        type: 'degradation',
        message: node.content || node.label || '',
      } satisfies ErrorEvent;
  }
}

/** Stable activity comparator: timestamp ASC, then backend seq ASC.
 * When both seqs are absent/identical, return 0 so the stable sort preserves
 * insertion order (e.g. unit tests with identical timestamps). */
function compareActivities(a: ActivityTreeNode, b: ActivityTreeNode): number {
  const ts = a.timestamp.localeCompare(b.timestamp);
  if (ts !== 0) return ts;
  const sa = a.seq ?? 0;
  const sb = b.seq ?? 0;
  if (sa !== sb) return sa - sb;
  return 0;
}
