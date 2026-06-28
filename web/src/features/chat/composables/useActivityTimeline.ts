import { shallowRef, computed, triggerRef, onUnmounted, getCurrentInstance } from 'vue';
import type { Activity, ActivityStatus, ActivityTreeNode } from '../activityTypes';
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
import { builtinLabels } from '../activityPresentation';
import { i18n } from '../../../i18n';

/**
 * useActivityTimeline manages Activity state for the Activity-First architecture.
 *
 * Phase 3 refactor: activities are isolated per session_id. Each session owns
 * its own Map<activityId, Activity>, and `currentSessionId` drives the public
 * computed properties (activities / activityTree / sortedActivities /
 * rootActivityId / loadError). Switching sessions no longer requires `reset()`
 * — the data for each session is preserved and naturally isolated.
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

  // === Flat sorted Activity list (current session, includes task) ===
  //
  // sortedActivities is the Activity-First render pipeline entry point: it
  // returns every Activity in the current session (including root task) as a
  // flat list, ordered by backend seq then timestamp. ActivityStream.vue
  // dispatches rendering by `kind` (task → UserMessageBubble, thinking →
  // ThinkingBlock, reply → ReplyBlock, …). Replaces the legacy streamEvents
  // computed which filtered out root task activities (dead code — task is the
  // turn container and must be rendered).

  const sortedActivities = computed<Activity[]>(() => {
    const sid = currentSessionId.value;
    if (!sid) return [];
    const map = activitiesBySession.value.get(sid);
    if (!map) return [];
    return Array.from(map.values()).sort(compareActivities);
  });

  // === Activity tree computed from flat list (current session only) ===

  const activityTree = computed<ActivityTreeNode[]>(() => {
    const sid = currentSessionId.value;
    if (!sid) return [];
    const map = activitiesBySession.value.get(sid);
    if (!map) return [];

    // === Deduplicate team_stage activities by teamId ===
    // Backend publishes a NEW team_stage activity for each lifecycle event
    // (assembled/running/completed) instead of UPDATING the same activity.
    // Without dedup the UI shows 4-5 cards per team — most without members.
    // Merge into ONE representative per team:
    //   - base = activity carrying members (usually the "assembled" event)
    //     so the member list is preserved and the UI id stays stable
    //   - status = most advanced across all events (completed > running > pending)
    //   - durationMs = max
    //   - seq = max (latest, so sort order reflects the team's latest event)
    const STATUS_PRIORITY: Record<string, number> = {
      pending: 1,
      running: 2,
      tool_running: 2,
      tool_blocked: 2,
      interrupted: 2,
      partial_failure: 4,
      cancelled: 3,
      failed: 3,
      completed: 5,
    };
    const teamStageByTeam = new Map<string, Activity[]>();
    const dedupMap = new Map<string, Activity>();
    for (const activity of map.values()) {
      if (activity.kind === 'team_stage' && activity.teamId) {
        const arr = teamStageByTeam.get(activity.teamId) ?? [];
        arr.push(activity);
        teamStageByTeam.set(activity.teamId, arr);
      } else {
        dedupMap.set(activity.id, activity);
      }
    }
    for (const arr of teamStageByTeam.values()) {
      if (arr.length === 1) {
        dedupMap.set(arr[0].id, arr[0]);
        continue;
      }
      const hasMembers = (a: Activity): boolean => {
        const m = a.meta?.members;
        return Array.isArray(m) && m.length > 0;
      };
      const base = arr.find(hasMembers) ?? arr[0];
      const bestStatus = arr.reduce((best, a) => {
        const pa = STATUS_PRIORITY[a.status] ?? 0;
        const pb = STATUS_PRIORITY[best] ?? 0;
        return pa > pb ? a.status : best;
      }, arr[0].status);
      const maxDuration = arr.reduce((max, a) => Math.max(max, a.durationMs ?? 0), 0);
      // Find the latest activity by Timestamp (design doc B.3.3: sort by Timestamp ASC,
      // no global Seq). Used to pick the most recent meta overlay.
      const latestWithMeta = arr.slice().sort((a, b) => {
        const ta = new Date(a.timestamp).getTime();
        const tb = new Date(b.timestamp).getTime();
        return tb - ta;
      })[0];
      const mergedMeta = { ...base.meta };
      // Overlay duration/status info from the latest activity that carries it
      // (base.meta may have duration_ms=0 from the "assembled" event).
      if (latestWithMeta?.meta) {
        for (const [k, v] of Object.entries(latestWithMeta.meta)) {
          if (k === 'members') continue; // keep base members
          if (v !== null && v !== undefined && v !== '' && !(typeof v === 'number' && v === 0)) {
            mergedMeta[k] = v;
          }
        }
      }
      dedupMap.set(base.id, {
        ...base,
        status: bestStatus as Activity['status'],
        durationMs: maxDuration || base.durationMs,
        meta: mergedMeta,
      });
    }

    const treeMap = new Map<string, ActivityTreeNode>();
    const roots: ActivityTreeNode[] = [];

    // Build tree nodes from deduplicated map
    for (const activity of dedupMap.values()) {
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

    // Sort tree by Timestamp ASC (design doc B.3.3). The backend guarantees
    // Timestamp monotonicity via the trpc-agent-go Runner's single-goroutine
    // runEventLoop + Channel FIFO, so pure Timestamp ordering is correct and
    // stable — no global Seq or special rules needed.
    const sortTree = (nodes: ActivityTreeNode[]) => {
      nodes.sort(compareActivities);
      for (const node of nodes) sortTree(node.children);
    };
    sortTree(roots);

    return roots;
  });

  // === Internal helpers for per-session root tracking ===

  function setRootForSession(sessionId: string, activityId: string) {
    const next = new Map(rootActivityIdBySession.value);
    next.set(sessionId, activityId);
    rootActivityIdBySession.value = next;
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
      // duration_ms=0 is treated as "not set" (|| null) because the backend
      // uses int64 without omitempty; a real zero-duration tool is impossible
      // in practice. Problem D is fixed on the backend side (problem A fix
      // restores OnToolResult execution so duration gets computed).
      durationMs: a.duration_ms || null,
      seq: a.seq,
      content: a.content || null,
      reasoning: a.reasoning || null,
      toolName: a.tool_name || null,
      toolCategory: a.tool_category || null,
      toolCallId: a.tool_call_id || null,
      toolArguments: a.tool_arguments || null,
      toolResult: a.tool_result || null,
      // Same || null semantic as durationMs above (0 = not set).
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
    const sessionId = snapshot.sessionId || currentSessionId.value || '';
    if (!sessionId) {
      console.warn('[useActivityTimeline] handleActivityEvent: missing sessionId, event dropped', ev);
      return;
    }
    const map = getSessionActivities(sessionId);

    switch (ev.event) {
      case 'created': {
        // B-08: Merge instead of overwrite. The `created` snapshot carries
        // the initial structural state (kind, parentActivityId, sessionId,
        // turnId, …). When the WS event stream is in order, the map has no
        // prior entry and we just `set` it. But the sequencer is a single
        // goroutine consuming from persistChan + sync publish — under load,
        // a `streaming` envelope (which upserts the Activity with the
        // accumulated content) can be observed before the `created` envelope.
        //
        // In that case, a naive `map.set(snapshot.id, snapshot)` would
        // DISCARD the streaming-accumulated content/reasoning/toolArguments
        // (replacing them with the empty initial state from `created`).
        // The fix: take the `created` snapshot as the structural base, then
        // overlay any pre-existing accumulated fields so streaming data
        // survives the late-arriving `created`.
        const existing = map.get(snapshot.id);
        map.set(snapshot.id, existing ? { ...snapshot, ...existing } : snapshot);
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
          } else if (ev.delta_field === 'tool_result') {
            // Tool output may stream in (large file_read / shell stdout / etc.);
            // accumulate it the same way as reasoning/content.
            updated.toolResult = (existing.toolResult || '') + ev.delta_chunk;
          }
          if (snapshot.seq != null) updated.seq = snapshot.seq;
          // Chat UI #2: a streaming envelope may ALSO carry a status change
          // (e.g. the final tool_result chunk arriving alongside
          // `status: completed`). Mirror the snapshot status so the UI
          // updates without waiting for a follow-up `completed` envelope,
          // which can be dropped or coalesced on the wire — leaving the
          // tool card stuck on the previous status until a page refresh.
          if (snapshot.status) updated.status = snapshot.status;
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
    const sid = sessionId || activityList[0]?.sessionId || currentSessionId.value;
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
    console.warn('[activity] failed to load activities from API after', maxAttempts, 'attempts:', msg);
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

  /**
   * §9.1.3 / Phase 3: Lazy-load activities for a session — skips the API
   * call when the session is already cached (even an empty Map counts as
   * loaded). WS replay reconciles missed events on reconnect. Use
   * `loadActivitiesFromAPI` (or `retryLoad`) to force a full refresh.
   *
   * Failed loads do NOT populate the cache, so the next call retries
   * automatically.
   */
  async function ensureActivitiesLoaded(sessionId: string, turnId?: string) {
    if (activitiesBySession.value.has(sessionId)) return;
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
    sortedActivities,
    rootActivityId,
    loadError,
    currentSessionId: computed(() => currentSessionId.value),
    handleActivityEvent,
    reset,
    clearAll,
    clearSession,
    setCurrentSession,
    getSessionActivities,
    loadActivities,
    loadActivitiesFromAPI,
    ensureActivitiesLoaded,
    retryLoad,
  };
}

// === Mapping Functions ===

/**
 * Friendly fallback labels for tool_category when neither `label` nor
 * `builtinLabels[toolName]` produces a user-facing name. Keeps the raw
 * tool_name (e.g. "file_read_file") out of the chat UI.
 *
 * i18n: labels are resolved via the global i18n instance so the chat UI
 * respects the user's locale (zh-CN / en-US). Keys live under `chat.tool`.
 */
const categoryI18nKeys: Record<string, string> = {
  shell: 'chat.tool.categoryShell',
  browser: 'chat.tool.categoryBrowser',
  file_read: 'chat.tool.categoryFileRead',
  file_write: 'chat.tool.categoryFileWrite',
  file_search: 'chat.tool.categoryFileSearch',
  web_search: 'chat.tool.categoryWebSearch',
  mcp: 'chat.tool.categoryMcp',
  code: 'chat.tool.categoryCode',
  todo: 'chat.tool.categoryTodo',
  other: 'chat.tool.categoryOther',
};

/**
 * Resolves a user-facing tool label with a multi-layer fallback chain:
 *   1. activity.label (backend-provided display label)
 *   2. builtinLabels[toolName] (curated friendly names per tool_name)
 *   3. categoryI18nKeys[toolCategory] → i18n.t (per-category fallback)
 *   4. raw toolName (last resort)
 *
 * Chat UI fix: previously the chain was `label || toolName`, which leaked
 * internal identifiers like "file_read_file" into the header.
 */
function resolveFriendlyToolLabel(
  label: string | null | undefined,
  toolName: string | null | undefined,
  toolCategory: string | null | undefined,
): string {
  const trimmedLabel = typeof label === 'string' ? label.trim() : '';
  if (trimmedLabel) return trimmedLabel;

  const name = typeof toolName === 'string' ? toolName : '';
  if (name && builtinLabels[name]) return builtinLabels[name];

  const category = typeof toolCategory === 'string' ? toolCategory : '';
  const i18nKey = category ? categoryI18nKeys[category] : '';
  if (i18nKey) {
    const translated = i18n.global.t(i18nKey);
    if (translated && translated !== i18nKey) return translated;
  }

  return name || '';
}

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
      // The error message comes from meta.error_message (set by
      // ActivityProjector.OnError), NOT from node.content — node.content
      // holds the user's input text and is rendered separately via
      // UserMessageBubble in ActivityStream. Echoing content here would
      // surface the user's own message inside the red error box.
      return {
        kind: 'error',
        id: node.id,
        type: 'degradation',
        message: (node.meta?.error_message as string | undefined) || node.toolErrorCode || '',
        errorCode: node.toolErrorCode || (node.meta?.error_code as string | undefined) || undefined,
      } satisfies ErrorEvent;

    case 'thinking': {
      // P2-A: thinking-as-display renders as a reply. The backend keeps
      // kind=thinking for audit but sets meta.as_display=true and populates
      // Content with the display text. Map to ReplyEvent so ReplyBlock
      // renders it as a normal reply instead of a collapsed thinking block.
      if (node.meta?.as_display === true) {
        return {
          kind: 'reply',
          id: node.id,
          content: node.content || node.reasoning || '',
          isFinal: node.meta?.is_final === true,
          streaming: node.status === 'running',
          variant: 'default',
          durationMs: node.durationMs,
        } satisfies ReplyEvent;
      }
      return {
        kind: 'thinking',
        id: node.id,
        content: node.reasoning || node.content || '',
        label: node.label || undefined,
        collapsed: node.collapsed,
        streaming: node.status === 'running',
        durationMs: node.durationMs,
      } satisfies ThinkingEvent;
    }

    case 'action': {
      const toolStatus = mapActivityStatusToToolStatus(node.status);
      const tool: ToolActivity = {
        toolName: node.toolName || '',
        // Chat UI fix: fall back through label → builtin friendly name →
        // category-friendly name → raw toolName. Previously jumped straight
        // to the raw tool_name (e.g. "file_read_file"), which leaked the
        // internal identifier into the UI.
        toolLabel: resolveFriendlyToolLabel(node.label, node.toolName, node.toolCategory),
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
        // P1-A: read is_final from backend meta mark instead of inferring
        // from status. The previous status-based inference (completed ||
        // failed) marked EVERY terminal reply as final, causing ReplyBlock
        // to label all replies as "最终回复". Now only the turn's last reply
        // (marked by ActivityProjector.OnTurnEnd) is final.
        isFinal: node.meta?.is_final === true,
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
      // Backend stores members under meta.team_summary.members with snake_case keys
      // (see internal/team/summary.go:69-73). Support meta.members (future) and
      // meta.team_summary.members (current) with snake_case → camelCase mapping.
      const summary = node.meta?.team_summary as Record<string, unknown> | undefined;
      const rawMembers: Record<string, unknown>[] | undefined = Array.isArray(node.meta?.members)
        ? (node.meta.members as Record<string, unknown>[])
        : Array.isArray(summary?.members)
          ? (summary!.members as Record<string, unknown>[])
          : undefined;
      const members = rawMembers?.map((m) => ({
        agentKey: ((m.agent_key ?? m.agentKey) as string) ?? '',
        agentName: ((m.agent_name ?? m.agentName) as string) ?? '',
        status: (m.status ?? 'pending') as TeamMemberStatus['status'],
        session_id: m.session_id as string | undefined,
      }));
      const taskSummary = typeof node.meta?.task_summary === 'string' ? node.meta.task_summary : undefined;
      // B.4.1 team-card: pass timestamp for header creation time display.
      const teamTimestamp = node.timestamp || undefined;
      // B.4.1 team-card: progress percentage from meta.progress_pct (0-100).
      const progressPct =
        typeof node.meta?.progress_pct === 'number'
          ? (node.meta.progress_pct as number)
          : typeof node.meta?.progressPct === 'number'
            ? (node.meta.progressPct as number)
            : undefined;
      return {
        kind: 'team_stage',
        id: node.id,
        status: mapActivityStatusToStageStatus(node.status),
        title: node.label || node.content || '',
        teamId: node.teamId || undefined,
        members,
        taskSummary,
        durationMs: node.durationMs,
        timestamp: teamTimestamp,
        progressPct,
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
      // B.4.2 agent-card: pass timestamp for header creation time display.
      const sessionTimestamp = node.timestamp || undefined;
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
        timestamp: sessionTimestamp,
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

/** Stable activity comparator: pure Timestamp ASC (design doc B.3.3).
 *
 * The backend guarantees Timestamp monotonicity via the trpc-agent-go Runner's
 * single-goroutine `runEventLoop` + Channel FIFO + event.EmitEvent setting
 * Timestamp at emit time. Therefore Timestamp alone is sufficient for correct
 * ordering — no global Seq or special "task-first / final-reply-last" rules
 * are needed:
 *   - task (user message) has the smallest Timestamp in a turn (naturally first)
 *   - final reply (is_final=true, from runner.completion) has the largest
 *     Timestamp (naturally last)
 *
 * Introducing "must-sort-first / must-sort-last" special rules is patch-style
 * design that breaks ordering consistency — the natural event order is
 * authoritative (design doc B.3.3).
 *
 * RFC3339Nano strings with variable fractional-digit lengths do not sort
 * lexicographically (e.g. `.100` vs `.99`), so Timestamp is compared
 * numerically via Date.parse, with a stable id-based tiebreaker for the
 * extremely rare equal-Timestamp case (single publish worker makes this
 * near-impossible, but stability is required for sort determinism).
 *
 * Accepts the base `Activity` type so it can sort both flat lists
 * (sortedActivities) and tree nodes (ActivityTreeNode[] — TreeNode extends
 * Activity).
 */
function compareActivities(a: Activity, b: Activity): number {
  const ta = new Date(a.timestamp).getTime();
  const tb = new Date(b.timestamp).getTime();
  if (!Number.isNaN(ta) && !Number.isNaN(tb) && ta !== tb) {
    return ta - tb;
  }
  // Fallback: lexical timestamp comparison (covers NaN cases) then id for stability.
  const tsCmp = a.timestamp.localeCompare(b.timestamp);
  if (tsCmp !== 0) return tsCmp;
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}
