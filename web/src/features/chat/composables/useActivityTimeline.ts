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
  TeamStageEvent,
  GraphStageEvent,
  SessionStageEvent,
  TeamMemberStatus,
  GraphNodeStatus,
} from '../streamEventTypes';
import type { ActivityEvent as AFActivityEvent, Activity as AFActivity } from '../../../realtime/activityEvent';
import { listActivities } from '../../session/api';

/**
 * useActivityTimeline manages Activity state for the Activity-First architecture.
 *
 * Phase 3 refactor: activities are isolated per session_id. Each session owns
 * its own Map<activityId, Activity>, and `currentSessionId` drives the public
 * computed properties (activities / activityTree / streamEvents / rootActivityId
 * / loadError). Switching sessions no longer requires `reset()` — the data
 * for each session is preserved and naturally isolated.
 *
 * This composable consumes WS activity events (activity_start/delta/done/
 * child_start) and the new ActivityEvent format (handleActivityEvent).
 */
export function useActivityTimeline() {
  // === Per-session storage ===
  //
  // sessionId → (activityId → Activity)
  const activitiesBySession = shallowRef<Map<string, Map<string, Activity>>>(new Map());
  // sessionId → rootActivityId
  const rootActivityIdBySession = shallowRef<Map<string, string>>(new Map());
  // sessionId → loadError
  const loadErrorBySession = shallowRef<Map<string, string | null>>(new Map());
  // Current session: drives the public computed properties
  const currentSessionId = shallowRef<string | null>(null);

  /**
   * Get (or lazily create) the Activity Map for the given session.
   * The Map is stored in `activitiesBySession`; mutation triggers a
   * `triggerRef` so dependent computed properties re-evaluate.
   */
  function getSessionActivities(sessionId: string): Map<string, Activity> {
    let map = activitiesBySession.value.get(sessionId);
    if (!map) {
      map = new Map();
      activitiesBySession.value.set(sessionId, map);
      triggerRef(activitiesBySession);
    }
    return map;
  }

  /** Set the current session that drives the public computed properties. */
  function setCurrentSession(sessionId: string | null) {
    currentSessionId.value = sessionId;
  }

  // === Public computed properties (driven by currentSessionId) ===

  const activities = computed<Activity[]>(() => {
    const sid = currentSessionId.value;
    if (!sid) return [];
    const map = activitiesBySession.value.get(sid);
    return map ? Array.from(map.values()) : [];
  });

  const rootActivityId = computed<string | null>(() => {
    const sid = currentSessionId.value;
    if (!sid) return null;
    return rootActivityIdBySession.value.get(sid) ?? null;
  });

  const loadError = computed<string | null>(() => {
    const sid = currentSessionId.value;
    if (!sid) return null;
    return loadErrorBySession.value.get(sid) ?? null;
  });

  // === Activity tree computed from flat list (current session only) ===

  const activityTree = computed<ActivityTreeNode[]>(() => {
    const sid = currentSessionId.value;
    if (!sid) return [];
    const map = activitiesBySession.value.get(sid);
    if (!map) return [];

    const treeMap = new Map<string, ActivityTreeNode>();
    const roots: ActivityTreeNode[] = [];

    // Build tree nodes
    for (const activity of map.values()) {
      treeMap.set(activity.id, { ...activity, children: [] });
    }

    // Link children to parents
    for (const node of treeMap.values()) {
      if (node.parentActivityId && treeMap.has(node.parentActivityId)) {
        treeMap.get(node.parentActivityId)!.children.push(node);
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
      .filter((node) => {
        // Root task activities are containers, not rendered as blocks —
        // EXCEPT task.failed, which surfaces as an ErrorBlock so turn-level
        // errors are visible to the user.
        if (node.kind === 'task' && node.status !== 'failed') return false;
        return true;
      })
      .map(activityToStreamEvent);
  });

  // === Internal helpers for per-session root tracking ===

  function setRootForSession(sessionId: string, activityId: string) {
    const next = new Map(rootActivityIdBySession.value);
    next.set(sessionId, activityId);
    rootActivityIdBySession.value = next;
  }

  /**
   * Locate which session an activity belongs to (by id).
   * Used by legacy envelope handlers (handleActivityDelta / handleActivityDone)
   * whose meta does not carry session_id — we look up the parent activity
   * across all sessions and route the update accordingly.
   */
  function findSessionOfActivity(activityId: string): string | null {
    for (const [sid, sm] of activitiesBySession.value.entries()) {
      if (sm.has(activityId)) return sid;
    }
    return null;
  }

  // === WS Event Handlers (legacy envelope path) ===

  function handleActivityStart(meta: ActivityStartMeta) {
    const sessionId = meta.session_id;
    if (!sessionId) return;
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
    const map = getSessionActivities(sessionId);
    map.set(activity.id, activity);
    triggerRef(activitiesBySession);

    // Track root activity
    if (!activity.parentActivityId) {
      setRootForSession(sessionId, activity.id);
    }
  }

  function handleActivityDelta(meta: ActivityDeltaMeta) {
    const sessionId = findSessionOfActivity(meta.activity_id);
    if (!sessionId) return;
    const map = getSessionActivities(sessionId);
    const existing = map.get(meta.activity_id);
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
    map.set(meta.activity_id, updated);
    triggerRef(activitiesBySession);
  }

  function handleActivityDone(meta: ActivityDoneMeta) {
    const sessionId = findSessionOfActivity(meta.activity_id);
    if (!sessionId) return;
    const map = getSessionActivities(sessionId);
    const existing = map.get(meta.activity_id);
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
    map.set(meta.activity_id, updated);
    triggerRef(activitiesBySession);
  }

  function handleActivityChildStart(meta: ActivityChildStartMeta) {
    // Child meta doesn't carry session_id — find the parent's session.
    let sessionId = '';
    if (meta.parent_activity_id) {
      sessionId = findSessionOfActivity(meta.parent_activity_id) ?? '';
    }
    if (!sessionId) {
      // Fall back to current session, or an orphan bucket if none selected.
      sessionId = currentSessionId.value ?? '__orphan__';
    }

    const activity: Activity = {
      id: meta.activity_id,
      kind: meta.kind,
      status: meta.status,
      sessionId,
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
    const map = getSessionActivities(sessionId);
    map.set(activity.id, activity);
    triggerRef(activitiesBySession);
  }

  // === Activity-First (AF) Event Handler ===
  //
  // handleActivityEvent consumes the new business-semantic ActivityEvent format
  // (event + full Activity snapshot + optional delta_field/delta_chunk), which
  // replaces the legacy activity_start/delta/done/child_start envelopes.
  //
  // The AF Activity uses snake_case field names (matching backend JSON tags);
  // we convert to the internal camelCase Activity model.

  function afActivityToInternal(a: AFActivity): Activity {
    return {
      id: a.id,
      kind: a.kind as Activity['kind'],
      status: a.status as ActivityStatus,
      sessionId: a.session_id,
      turnId: a.turn_id,
      parentActivityId: a.parent_activity_id || null,
      timestamp: a.timestamp,
      durationMs: a.duration_ms || null,
      seq: a.seq,
      content: a.content || null,
      reasoning: a.reasoning || null,
      toolName: a.tool_name || null,
      toolCategory: a.tool_category || null,
      toolCallId: a.tool_call_id || null,
      toolArguments: a.tool_arguments || null,
      toolResult: a.tool_result || null,
      toolDurationMs: a.tool_duration_ms || null,
      toolErrorCode: a.tool_error_code || null,
      childBoardId: a.child_board_id || null,
      spiritSessionId: a.spirit_session_id || null,
      teamId: a.team_id || null,
      dagNodeId: a.dag_node_id || null,
      dependsOn: a.depends_on || null,
      stage: a.stage || null,
      agentKey: a.agent_key || null,
      agentName: a.agent_name || null,
      promptTokens: a.prompt_tokens || null,
      completionTokens: a.completion_tokens || null,
      collapsed: a.collapsed,
      label: a.label || null,
      meta: a.meta || null,
    };
  }

  function handleActivityEvent(ev: AFActivityEvent) {
    const snapshot = afActivityToInternal(ev.activity);
    const sessionId =
      snapshot.sessionId || currentSessionId.value || '';
    if (!sessionId) {
      console.warn(
        '[useActivityTimeline] handleActivityEvent: missing sessionId, event dropped',
        ev,
      );
      return;
    }
    const map = getSessionActivities(sessionId);

    switch (ev.event) {
      case 'created':
      case 'child_created': {
        // Full snapshot: create or replace the Activity in the map.
        map.set(snapshot.id, snapshot);
        triggerRef(activitiesBySession);
        if (!snapshot.parentActivityId) {
          setRootForSession(sessionId, snapshot.id);
        }
        break;
      }
      case 'streaming': {
        // Streaming append: the Activity snapshot carries the accumulated
        // state, and delta_field/delta_chunk carry the incremental text.
        // We apply the delta to the existing Activity (or create if missing).
        const existing = map.get(snapshot.id);
        if (!existing) {
          // Activity not yet in map (race or missed created event):
          // use the full snapshot as the base.
          map.set(snapshot.id, snapshot);
        } else if (ev.delta_field && ev.delta_chunk) {
          const updated: Activity = { ...existing };
          if (ev.delta_field === 'reasoning') {
            updated.reasoning = (existing.reasoning || '') + ev.delta_chunk;
          } else if (ev.delta_field === 'content') {
            updated.content = (existing.content || '') + ev.delta_chunk;
          } else if (ev.delta_field === 'tool_arguments') {
            updated.toolArguments = (existing.toolArguments || '') + ev.delta_chunk;
          }
          if (snapshot.seq != null) updated.seq = snapshot.seq;
          map.set(snapshot.id, updated);
        } else {
          // No delta info: use the full snapshot (backend may send
          // accumulated content in the snapshot).
          map.set(snapshot.id, { ...existing, ...snapshot });
        }
        triggerRef(activitiesBySession);
        break;
      }
      case 'updated':
      case 'completed':
      case 'failed':
      case 'cancelled': {
        // Terminal or state-change event: merge the full snapshot into
        // the existing Activity (or create if missing).
        const existing = map.get(snapshot.id);
        if (existing) {
          map.set(snapshot.id, { ...existing, ...snapshot });
        } else {
          map.set(snapshot.id, snapshot);
        }
        triggerRef(activitiesBySession);
        break;
      }
    }
  }

  // === Reset / cleanup ===

  /**
   * Clear the current session's activities.
   *保留用于 unmount cleanup 与历史 reset() 调用方；
   * Phase 3 后切换 session 不再调用 reset（自然隔离）。
   */
  function reset() {
    const sid = currentSessionId.value;
    if (!sid) return;
    clearSession(sid);
  }

  /** Clear activities for a specific session. */
  function clearSession(sessionId: string) {
    const nextActivities = new Map(activitiesBySession.value);
    nextActivities.delete(sessionId);
    activitiesBySession.value = nextActivities;

    const nextRoots = new Map(rootActivityIdBySession.value);
    nextRoots.delete(sessionId);
    rootActivityIdBySession.value = nextRoots;

    const nextErrors = new Map(loadErrorBySession.value);
    nextErrors.delete(sessionId);
    loadErrorBySession.value = nextErrors;
  }

  /** Clear all sessions (called on workspace unmount). */
  function clearAll() {
    activitiesBySession.value = new Map();
    rootActivityIdBySession.value = new Map();
    loadErrorBySession.value = new Map();
    currentSessionId.value = null;
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

  /**
   * Atomically replace the Activity list for a session.
   * `sessionId` is optional — when omitted it is derived from the first
   * activity's sessionId, or falls back to currentSessionId.
   */
  function loadActivities(activityList: Activity[], sessionId?: string) {
    const sid =
      sessionId ||
      activityList[0]?.sessionId ||
      currentSessionId.value;
    if (!sid) return;

    // Atomic replacement: build a complete new Map before assigning to the
    // session slot. This avoids the intermediate empty-map state that causes
    // a flash when the old Map is cleared and the new data hasn't been
    // written yet.
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

    const nextActivities = new Map(activitiesBySession.value);
    nextActivities.set(sid, newMap);
    activitiesBySession.value = nextActivities;

    if (newRootId) {
      setRootForSession(sid, newRootId);
    } else {
      // No root found — clear any stale root for this session.
      const nextRoots = new Map(rootActivityIdBySession.value);
      nextRoots.delete(sid);
      rootActivityIdBySession.value = nextRoots;
    }
  }

  // AF-FE-14 / AF-GAP-05: Load activities from backend API for history recovery.
  // Retries up to 5 times with exponential backoff (500ms, 1s, 2s, 4s) on failure.
  // On final failure, sets loadError for the session so the UI can show a
  // "数据加载失败，请刷新" degradation notice instead of silently falling
  // back to Legacy rendering.
  async function loadActivitiesFromAPI(sessionId: string, turnId?: string) {
    const maxAttempts = 5;
    const baseDelay = 500;
    let lastErr: unknown;

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        const activityList = await listActivities(sessionId, turnId);
        loadActivities(activityList, sessionId);
        setSessionLoadError(sessionId, null);
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
    setSessionLoadError(sessionId, msg);
    console.warn(
      '[activity] failed to load activities from API after',
      maxAttempts,
      'attempts:',
      msg,
    );
  }

  function setSessionLoadError(sessionId: string, msg: string | null) {
    const next = new Map(loadErrorBySession.value);
    next.set(sessionId, msg);
    loadErrorBySession.value = next;
  }

  /** Retry loading activities for a session after a previous failure. */
  async function retryLoad(sessionId: string, turnId?: string) {
    setSessionLoadError(sessionId, null);
    await loadActivitiesFromAPI(sessionId, turnId);
  }

  // === Cleanup on unmount (only when inside a component instance) ===

  if (getCurrentInstance()) {
    onUnmounted(() => {
      clearAll();
    });
  }

  return {
    activities,
    activityTree,
    streamEvents,
    rootActivityId,
    loadError,
    currentSessionId: computed(() => currentSessionId.value),
    handleActivityStart,
    handleActivityDelta,
    handleActivityDone,
    handleActivityChildStart,
    handleActivityEvent,
    reset,
    clearAll,
    clearSession,
    setCurrentSession,
    getSessionActivities,
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
    case 'task':
      // task.failed → ErrorBlock (turn-level error surfaced to user).
      // task with non-failed status is filtered out upstream (container).
      return {
        kind: 'error',
        id: node.id,
        type: 'degradation',
        message: node.content || node.toolErrorCode || (node.meta?.error_message as string | undefined) || '',
        errorCode: node.toolErrorCode || (node.meta?.error_code as string | undefined) || undefined,
      } satisfies ErrorEvent;

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
        toolCategory: node.toolCategory || undefined,
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

    case 'plan': {
      // Map backend ActivityStatus → frontend PlanEvent.status
      const planStatus = mapPlanStatus(node.status);
      // Extract steps from meta (serialized from Go ActivityPlanStep[])
      const metaSteps = Array.isArray(node.meta?.steps) ? (node.meta.steps as PlanStep[]) : [];
      // B-04: Map tree children to StreamEvent[] for PlanBlock rendering.
      // This replaces the direct activityTree consumption in PlanBlock,
      // unifying the data flow through the event model.
      const childEvents: StreamEvent[] = (node.children ?? [])
        .filter((child) => !(child.kind === 'task' && child.status !== 'failed'))
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

    case 'team_stage': {
      // Phase 3: Team stage — members list lives in meta.members
      const members = Array.isArray(node.meta?.members) ? (node.meta.members as TeamMemberStatus[]) : undefined;
      const taskSummary = typeof node.meta?.task_summary === 'string' ? node.meta.task_summary : undefined;
      return {
        kind: 'team_stage',
        id: node.id,
        status: mapActivityStatusToStageStatus(node.status),
        title: node.label || node.content || '',
        teamId: node.teamId || undefined,
        members,
        taskSummary,
        durationMs: node.durationMs,
      } satisfies TeamStageEvent;
    }

    case 'graph_stage': {
      // Phase 3: Graph stage — DAG nodes live in meta.nodes
      const nodes = Array.isArray(node.meta?.nodes) ? (node.meta.nodes as GraphNodeStatus[]) : undefined;
      return {
        kind: 'graph_stage',
        id: node.id,
        status: mapActivityStatusToStageStatus(node.status),
        title: node.label || node.content || '',
        dagNodeId: node.dagNodeId || undefined,
        nodes,
        durationMs: node.durationMs,
      } satisfies GraphStageEvent;
    }

    case 'session': {
      // Phase 3: Child session stage — child_session_id lives in meta
      const childSessionId = typeof node.meta?.child_session_id === 'string' ? node.meta.child_session_id : undefined;
      return {
        kind: 'session',
        id: node.id,
        status: mapActivityStatusToStageStatus(node.status),
        title: node.label || node.content || '',
        childSessionId,
        agentKey: node.agentKey || undefined,
        agentName: node.agentName || undefined,
        teamId: node.teamId || undefined,
        spiritSessionId: node.spiritSessionId || undefined,
        durationMs: node.durationMs,
      } satisfies SessionStageEvent;
    }

    default:
      // Fallback for legacy kinds (sub_task_board, delegate, error) that may
      // still appear in persisted Activity records from older backend versions.
      // Maps to ErrorEvent (degradation) so historical data still renders.
      return {
        kind: 'error',
        id: node.id,
        type: 'degradation',
        message: node.content || node.label || '',
      } satisfies ErrorEvent;
  }
}

/**
 * Maps backend ActivityStatus → StageEvent.status for team_stage/graph_stage/session.
 * Mirrors the AgentWorkProcess convention: partial_failure → completed
 * (the stage reached a terminal state; member/node-level failures are
 * visible in their respective lists).
 */
function mapActivityStatusToStageStatus(status: ActivityStatus): 'running' | 'completed' | 'failed' | 'cancelled' {
  switch (status) {
    case 'pending':
    case 'running':
    case 'tool_running':
    case 'tool_blocked':
      return 'running';
    case 'completed':
    case 'partial_failure':
      return 'completed';
    case 'failed':
    case 'interrupted':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    default:
      return 'running';
  }
}

/** Stable activity comparator: backend seq ASC, then timestamp ASC.
 *
 * `_seq` is the projector's global emission counter, so it is the most
 * reliable ordering signal. Timestamp strings are compared numerically as a
 * fallback because RFC3339Nano strings with variable fractional-digit lengths
 * do not sort lexicographically (e.g. `.100` vs `.99`). */
function compareActivities(a: ActivityTreeNode, b: ActivityTreeNode): number {
  const sa = a.seq ?? 0;
  const sb = b.seq ?? 0;
  if (sa !== sb) return sa - sb;

  const ta = new Date(a.timestamp).getTime();
  const tb = new Date(b.timestamp).getTime();
  if (!Number.isNaN(ta) && !Number.isNaN(tb) && ta !== tb) {
    return ta - tb;
  }

  return a.timestamp.localeCompare(b.timestamp);
}
