import { shallowRef, computed, triggerRef, reactive, onUnmounted, getCurrentInstance } from 'vue';
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
  // Set of sessionIds that have been fully loaded from the API.
  // Tracks API-loaded sessions separately from WS-populated sessions
  // so ensureActivitiesLoaded doesn't skip API loading when a WS event
  // has already created an empty/single-entry Map for the session.
  const apiLoadedSessions = new Set<string>();
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

  // === Activity tree computed from flat list (cross-session) ===
  //
  // Team member activities (thinking/action/reply) live in sub-sessions
  // (different session IDs), not in the spirit session. To build a correct
  // tree where member activities are nested under their parent session
  // activities, we must collect activities from ALL sessions whose
  // spiritSessionId matches the current session.
  //
  // When currentSessionId is a sub-session (non-spirit), no other session
  // will have spiritSessionId matching it, so we fall back to the current
  // session's own activities — preserving the original behavior.

  const activityTree = computed<ActivityTreeNode[]>(() => {
    const sid = currentSessionId.value;
    if (!sid) return [];

    // Collect activities from the current session AND all sub-sessions
    // whose spiritSessionId matches the current session.
    const allActivities: Activity[] = [];
    for (const [sessionId, sessionMap] of activitiesBySession.value) {
      if (sessionId === sid) {
        for (const a of sessionMap.values()) allActivities.push(a);
      } else {
        // Check if this is a sub-session of the current spirit session.
        // Backend may leave spiritSessionId empty on some activities (notices,
        // lazy-created plans); checking only the first activity would filter
        // out the whole sub-session when its first activity lacks the field.
        // Defensive approach: include the sub-session if ANY activity's
        // spiritSessionId matches `sid`.
        let isSubSession = false;
        for (const a of sessionMap.values()) {
          if (a.spiritSessionId === sid) {
            isSubSession = true;
            break;
          }
        }
        if (isSubSession) {
          for (const a of sessionMap.values()) allActivities.push(a);
        }
      }
    }

    if (allActivities.length === 0) return [];

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
    //
    // === Deduplicate plan activities by turnId (defensive) ===
    // 后端已通过 OnPlanStart dedup by turnID 保证同一 turn 只有一个 plan Activity，
    // 但历史数据或边界情况（如 OnPlanStart 与 lazy creation 同时触发）可能产生
    // 多个 plan。这里做防御性合并：保留最早创建的（Timestamp 最小）作为 base，
    // 因为它决定了 plan 在时间线中的位置；叠加最新 plan 的 steps/content。
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
    const planByTurn = new Map<string, Activity[]>();
    const dedupMap = new Map<string, Activity>();
    for (const activity of allActivities) {
      if (activity.kind === 'team_stage' && activity.teamId) {
        const arr = teamStageByTeam.get(activity.teamId) ?? [];
        arr.push(activity);
        teamStageByTeam.set(activity.teamId, arr);
      } else if (activity.kind === 'plan' && activity.turnId) {
        const arr = planByTurn.get(activity.turnId) ?? [];
        arr.push(activity);
        planByTurn.set(activity.turnId, arr);
      } else {
        dedupMap.set(activity.id, activity);
      }
    }
    // Plan dedup: base = earliest Timestamp (stable position), overlay latest steps.
    // Prefer the plan with more steps (from publishPlanCreated) over the lazy
    // plan from processGraphNodeStart (which starts with empty steps). This
    // ensures the frontend PlanBlock shows the correct task decomposition.
    for (const arr of planByTurn.values()) {
      if (arr.length === 1) {
        dedupMap.set(arr[0].id, arr[0]);
        continue;
      }
      const sorted = arr.slice().sort((a, b) => {
        const ta = new Date(a.timestamp).getTime();
        const tb = new Date(b.timestamp).getTime();
        return ta - tb;
      });
      const base = sorted[0];
      const latest = sorted[sorted.length - 1];
      // Prefer the plan with more steps (canonical plan from publishPlanCreated
      // has the correct subtask decomposition; lazy plan from processGraphNodeStart
      // starts with empty steps).
      const baseSteps = Array.isArray(base.meta?.steps) ? base.meta.steps.length : 0;
      const latestSteps = Array.isArray(latest.meta?.steps) ? latest.meta.steps.length : 0;
      const bestMeta = latestSteps > baseSteps ? latest.meta : base.meta;
      dedupMap.set(base.id, {
        ...base,
        content: latest.content || base.content,
        label: latest.label || base.label,
        meta: bestMeta || base.meta,
        status: latest.status,
      });
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

    // 性能优化（方案 B 细粒度响应式）：node 直接引用原 reactive Activity 对象，
    // 不 spread 复制。这样 renderItems computed 读取 node.content 时，建立对
    // 原 reactive 对象 content 字段的细粒度依赖。streaming 事件修改原对象
    // content，只触发 renderItems 重算（O(n)），不触发 activityTree 重算
    // （O(n log n) dedup + sort）。
    //
    // 注意：
    // 1. dedup 后的 merged Activity（team_stage/plan）不是原 reactive 对象，
    //    但这些类型没有 streaming 事件，不影响细粒度响应式优化。
    // 2. node.children = [] 会修改原 reactive 对象，但只在 activityTree 重算
    //    时执行（结构变化时），streaming 事件不触发 activityTree 重算，因此
    //    不会频繁修改 children 字段。
    for (const activity of dedupMap.values()) {
      const node = activity as ActivityTreeNode;
      node.children = [];
      treeMap.set(activity.id, node);
    }

    // === Build session activity lookup by (childSessionId, agentKey) ===
    // Team runner uses ONE shared team session for all member agent
    // execution. The ActivityProjector is configured ONCE with the anchor
    // agent's SessionActivityID as ParentActivityID, so ALL member
    // thinking/action/reply activities get the SAME parentActivityId
    // (the anchor's session activity ID). Without re-parenting, only the
    // anchor AgentCard shows activities; the other 2+ AgentCards render
    // empty.
    //
    // Fix: each member's activities carry a correct `agent_key` field.
    // Build a lookup of (childSessionId, agentKey) → sessionActivityNode,
    // then re-parent activities to the session activity whose agentKey
    // matches the activity's agent_key.
    const sessionActivityByKey = new Map<string, ActivityTreeNode>();
    for (const node of treeMap.values()) {
      if (node.kind === 'session' && node.meta?.child_session_id && node.agentKey) {
        const key = `${node.meta.child_session_id as string}|${node.agentKey}`;
        sessionActivityByKey.set(key, node);
      }
    }

    // Link children to parents
    for (const node of treeMap.values()) {
      if (node.parentActivityId && treeMap.has(node.parentActivityId)) {
        const declaredParent = treeMap.get(node.parentActivityId)!;
        // Re-parenting: if the declared parent is a session activity
        // (AgentCard) but the activity's agent_key doesn't match the
        // parent's agentKey, find the correct sibling session activity
        // with matching agentKey under the same childSessionId.
        if (
          declaredParent.kind === 'session' &&
          declaredParent.meta?.child_session_id &&
          node.agentKey &&
          declaredParent.agentKey !== node.agentKey
        ) {
          const key = `${declaredParent.meta.child_session_id as string}|${node.agentKey}`;
          const correctParent = sessionActivityByKey.get(key);
          if (correctParent && correctParent.id !== node.parentActivityId) {
            correctParent.children.push(node);
            continue;
          }
        }
        declaredParent.children.push(node);
      } else {
        // No parentActivityId OR parent not found in tree: treat as root.
        // Without this, nodes whose parentActivityId doesn't match any
        // entry (e.g. due to backend ID mismatch) are silently dropped,
        // making all child activities invisible.
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

    // Defensive fix: session activities (AgentCards) for synthesizers/anchors
    // that orchestrate the team graph never get a persisted team_run_step, so
    // finalizePendingSessionActivities on the backend should publish
    // "completed" for them. In practice this doesn't always persist (race or
    // early-return), leaving AgentCards stuck in "running" after the team
    // completes. Walk the tree and for each team_stage in terminal status,
    // inherit the terminal status for child session activities still running.
    const inheritTeamTerminalStatus = (nodes: ActivityTreeNode[]) => {
      for (const node of nodes) {
        if (node.kind === 'team_stage') {
          const teamTerminalStatus = mapActivityStatusToStageStatus(node.status);
          if (
            teamTerminalStatus === 'completed' ||
            teamTerminalStatus === 'failed' ||
            teamTerminalStatus === 'cancelled'
          ) {
            for (const child of node.children ?? []) {
              if (child.kind === 'session' && child.status === 'running') {
                child.status = teamTerminalStatus === 'cancelled' ? 'failed' : teamTerminalStatus;
              }
            }
          }
        }
        inheritTeamTerminalStatus(node.children ?? []);
      }
    };
    inheritTeamTerminalStatus(roots);

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
        //
        // Timestamp 保留：`created` 事件携带的 Timestamp 是 Activity 的创建时间
        // （设计 B.3.3：Timestamp 在事件产生时设置）。即使 `created` 晚于
        // `streaming` 到达，也必须使用 `created` 的 Timestamp，因为创建时间
        // 是排序的唯一依据——晚到的 `streaming` Timestamp 会更大，若保留它
        // 会导致 Activity 在时间线中后移，破坏事件自然顺序。
        //
        // 性能优化（方案 B 细粒度响应式）：Activity 用 reactive 包裹，
        // 让 Vue 自动追踪字段依赖。created 是结构变化事件，需要 triggerRef
        // 触发 activityTree 重算。
        const existing = map.get(snapshot.id);
        if (existing) {
          // 已有 reactive 对象：用 Object.assign 合并，保留 streaming 期间累积的内容字段
          Object.assign(existing, { ...snapshot, ...existing, timestamp: snapshot.timestamp });
        } else {
          map.set(snapshot.id, reactive({ ...snapshot }));
        }
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
        //
        // 性能优化（方案 B 细粒度响应式）：streaming 事件直接修改 reactive
        // 对象的内容字段（content/reasoning/toolArguments/toolResult），
        // **不调用 triggerRef**。Vue 自动触发读取该字段的组件重渲染
        // （如 ReplyBlock 读取 content）。activityTree computed 只依赖结构
        // 字段（kind/teamId/turnId/parentActivityId/status/timestamp），
        // 不依赖内容字段，因此不会全量重算。
        // 这解决了"最后输出越来越卡"的问题：511 activities × 60 triggers/sec
        // 的全量重算降到 0 次（streaming 时不触发 activityTree）。
        const existing = map.get(snapshot.id);
        if (!existing) {
          // Activity not yet in map (race or missed created event):
          // use the full snapshot as the base. 这是结构变化，需要 triggerRef。
          map.set(snapshot.id, reactive({ ...snapshot }));
          triggerRef(activitiesBySession);
        } else if (ev.delta_field && ev.delta_chunk) {
          // 直接修改 reactive 对象字段，不创建新对象，不 triggerRef
          if (ev.delta_field === 'reasoning') {
            existing.reasoning = (existing.reasoning || '') + ev.delta_chunk;
          } else if (ev.delta_field === 'content') {
            existing.content = (existing.content || '') + ev.delta_chunk;
          } else if (ev.delta_field === 'tool_arguments') {
            existing.toolArguments = (existing.toolArguments || '') + ev.delta_chunk;
          } else if (ev.delta_field === 'tool_result') {
            // Tool output may stream in (large file_read / shell stdout / etc.);
            // accumulate it the same way as reasoning/content.
            existing.toolResult = (existing.toolResult || '') + ev.delta_chunk;
          }
          if (snapshot.seq != null) existing.seq = snapshot.seq;
          // Chat UI #2: a streaming envelope may ALSO carry a status change
          // (e.g. the final tool_result chunk arriving alongside
          // `status: completed`). Mirror the snapshot status so the UI
          // updates without waiting for a follow-up `completed` envelope,
          // which can be dropped or coalesced on the wire — leaving the
          // tool card stuck on the previous status until a page refresh.
          // 注意：status 变化会触发 activityTree 重算（因为 activityTree 依赖
          // status 做 dedup）。这是 acceptable 的，因为 streaming 期间 status
          // 变化不频繁（通常只在最后一个 chunk 时变化）。
          if (snapshot.status) existing.status = snapshot.status;
          // Timestamp 保留：streaming 事件不更新 Timestamp，保持 created 时的时间，
          // 避免 thinking/action/reply 在流式过程中位置跳动（设计 B.3.3）。
          // 不 triggerRef：内容字段变化由 reactive 自动触发对应组件重渲染
        } else {
          // No delta info: use the full snapshot (backend may send
          // accumulated content in the snapshot). 保留 existing.timestamp。
          Object.assign(existing, { ...snapshot, timestamp: existing.timestamp });
          // 不 triggerRef：内容字段变化由 reactive 自动触发对应组件重渲染
        }
        break;
      }
      case 'updated':
      case 'completed':
      case 'failed':
      case 'cancelled': {
        // Terminal or state-change event: merge the full snapshot into
        // the existing Activity (or create if missing).
        //
        // Timestamp 保留（关键修复）：`updated`/`completed` 事件携带的 Timestamp
        // 是事件发出时间，而非 Activity 创建时间。若用新 Timestamp 覆盖，
        // graph_stage / team_stage 等多次更新的 Activity 会在时间线中不断后移，
        // 导致 plan → graph → team 的设计顺序错乱为 team → graph（或更糟）。
        // 保留 existing.timestamp 确保 Activity 始终位于其创建时的位置，
        // 这与设计 B.3.3「按 Timestamp ASC 排序」的初衷一致——排序依据是
        // 创建时间，而非最后更新时间。
        //
        // 性能优化（方案 B 细粒度响应式）：直接修改 reactive 对象字段。
        // 这些事件可能改变结构字段（如 status），需要 triggerRef 触发
        // activityTree 重算（dedup 逻辑依赖 status）。
        //
        // Issue 2 fix: merge meta instead of overwriting. Direct-publish
        // events (team_stage/graph_stage) carry partial meta — "assembled"
        // has members, "progress" has progress_pct, "completed" has
        // duration_ms. Object.assign overrides all meta keys, so a
        // progress event (no members) arriving after the assembled event
        // would clear the members list. Deep merge preserves existing keys
        // while overlaying incoming ones.
        const existing = map.get(snapshot.id);
        if (existing) {
          const mergedMeta =
            existing.meta && snapshot.meta
              ? { ...existing.meta, ...snapshot.meta }
              : snapshot.meta ?? existing.meta;
          Object.assign(existing, {
            ...snapshot,
            timestamp: existing.timestamp,
            meta: mergedMeta,
          });
        } else {
          map.set(snapshot.id, reactive({ ...snapshot }));
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

    apiLoadedSessions.delete(sessionId);
  }

  /** Clear all sessions (called on workspace unmount). */
  function clearAll() {
    activitiesBySession.value = new Map();
    rootActivityIdBySession.value = new Map();
    loadErrorBySession.value = new Map();
    currentSessionId.value = null;
    apiLoadedSessions.clear();
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
      // 性能优化（方案 B 细粒度响应式）：用 reactive 包裹 Activity，
      // 让 Vue 自动追踪字段依赖。streaming 事件直接修改 reactive 对象的
      // content/reasoning 字段，只触发读取这些字段的组件重渲染，不触发
      // activityTree computed 全量重算（因为 activityTree 只依赖结构字段
      // kind/teamId/turnId/parentActivityId/status/timestamp）。
      newMap.set(a.id, reactive(withSeq));
      if (!a.parentActivityId) {
        newRootId = a.id;
      }
    }

    const nextActivities = new Map(activitiesBySession.value);
    nextActivities.set(sid, newMap);
    activitiesBySession.value = nextActivities;

    // Mark session as API-loaded so ensureActivitiesLoaded doesn't
    // re-fetch it. WS events may have created an empty/single-entry
    // Map for this session, but the API load is the authoritative
    // source for historical activities.
    apiLoadedSessions.add(sid);

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
  //
  // T5.1 / §B.7.1: This loader is called once per spirit session on entry
  // (driven by `setCurrentSession` + `ensureActivitiesLoaded` at the page
  // level). Sub-session activities are NOT pre-loaded here — they are
  // lazy-loaded via `ensureActivitiesLoaded` when team-card / agent-card
  // expands (T5.2/T5.3, see ChatPage.onExpandChildren). The backend
  // `ListActivities` RPC uses `ListBySession(sessionID)` which only returns
  // activities whose `session_id` matches — i.e. spirit-direct activities
  // for a spirit session, NOT nested team/agent sub-session activities.
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
   *
   * T5.4 / §B.7.2: Cache guarantee for sub-session lazy-load. When a
   * team-card or agent-card expands repeatedly (toggle on/off/on), this
   * guard ensures we only hit the API once per session — subsequent
   * expands resolve immediately from the in-memory Map. The cache key is
   * `sessionId`; turn-scoped sub-loads pass `turnId` but the cache check
   * does not differentiate by turn (a session is either loaded or not).
   */
  async function ensureActivitiesLoaded(sessionId: string, turnId?: string) {
    // Use apiLoadedSessions instead of activitiesBySession.has() because
    // WS events may have already created an empty or single-entry Map for
    // this session via getSessionActivities() → handleActivityEvent(),
    // causing the old check to skip API loading. The API is the
    // authoritative source for historical activities that WS events
    // cannot deliver (e.g., activities created before page load).
    if (apiLoadedSessions.has(sessionId)) return;
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
 * cleanTeamDisplayName strips markdown syntax from a team display name or
 * task summary.
 *
 * Backend sets team.DisplayName to the truncated task description, which for
 * the spirit's primary team is a markdown document (`## 任务概述\n...`). Showing
 * raw markdown in the team-card header breaks the layout and looks broken.
 *
 * Strategy:
 *   1. Remove entire markdown header lines (`#`, `##`, `###`...), code fence
 *      lines, and list marker prefixes. We REMOVE header lines entirely (rather
 *      than just stripping `#`) because the header text like "任务概述" is a
 *      section title, not the actual content — keeping it would hide the real
 *      task description below.
 *   2. Split by newlines and pick the first non-empty meaningful line.
 *   3. Truncate to a reasonable length for the header (≤60 chars).
 *
 * For clean team names like "代码分析 (Code Analysis)" this is a no-op.
 */
function cleanTeamDisplayName(raw: string | null | undefined): string {
  if (!raw) return '';
  // Remove entire markdown header lines, code fence lines, and strip list markers.
  const cleaned = raw
    .replace(/^#{1,6}\s+.*$/gm, '') // entire header lines: # Title, ## Title...
    .replace(/^```[^\n]*$/gm, '') // code fence open/close lines
    .replace(/^[-*+]\s+/gm, '') // unordered list markers
    .replace(/^\d+\.\s+/gm, '') // ordered list markers
    .replace(/\*\*([^*]+)\*\*/g, '$1') // bold
    .replace(/\*([^*]+)\*/g, '$1') // italic
    .replace(/`([^`]+)`/g, '$1'); // inline code
  // Pick the first non-empty line.
  const lines = cleaned.split(/\r?\n/).map((l) => l.trim()).filter(Boolean);
  const first = lines[0] || '';
  // Truncate to 60 chars with ellipsis if needed.
  if (first.length <= 60) return first;
  return first.slice(0, 57) + '...';
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
      // Support both camelCase (OnNotice) and snake_case (PrePlanningGate /
      // plan-confirm / feedback) meta keys — backend is inconsistent.
      const noticeType = (node.meta?.noticeType ?? node.meta?.notice_type) as NoticeEvent['type'] ?? 'info';
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
      // Backend stores members under two keys:
      //   - meta.members (from publishSpiritTeamAssembled): correct session_id
      //     per member, but status is always 'pending' (set at assembly time
      //     before members execute — see spirit_team.go:1021).
      //   - meta.team_summary.members (from TeamSummaryActivityEvent, fired at
      //     run completion): authoritative member statuses (completed/failed)
      //     from TeamRunStep.Status, but session_id is the TEAM session id
      //     (run.SessionID — see biz/team_summary.go:24), NOT the individual
      //     member session id. Using team_summary's session_id would break
      //     frontend lazy-load of member execution processes.
      //
      // Merge strategy: base = meta.members (correct session_ids), then
      // overlay status from team_summary.members (matched by agent_key).
      // Without this merge, members always show 'pending' even after the
      // team completes — a regression of the AgentCard status fix.
      const summary = node.meta?.team_summary as Record<string, unknown> | undefined;
      const baseMembers = Array.isArray(node.meta?.members)
        ? (node.meta!.members as Record<string, unknown>[])
        : undefined;
      const summaryMembers = Array.isArray(summary?.members)
        ? (summary!.members as Record<string, unknown>[])
        : undefined;
      // Build agent_key → status map from team_summary for authoritative statuses.
      const statusByKey = new Map<string, string>();
      if (summaryMembers) {
        for (const m of summaryMembers) {
          const key = ((m.agent_key ?? m.agentKey) as string) ?? '';
          if (key) statusByKey.set(key, (m.status as string) ?? '');
        }
      }
      // Fallback: if baseMembers is missing (assembled event lost) but
      // summaryMembers exists, use summaryMembers directly. session_id will
      // be the team session id (less accurate) but members will render.
      const effectiveMembers = baseMembers ?? summaryMembers;
      // Map backend TeamRunStep.Status ("ok"/"error"/"skipped") to frontend
      // TeamMemberStatus.status ("completed"/"failed"/"completed"). The
      // assembled event uses "pending"; team_summary uses step status which
      // is "ok"/"error"/"skipped". Without this mapping, members render with
      // class team-card__member--ok (no CSS rule) instead of --completed.
      const mapMemberStatus = (raw: unknown): TeamMemberStatus['status'] => {
        const s = (raw as string) ?? 'pending';
        switch (s) {
          case 'ok':
          case 'skipped':
            return 'completed';
          case 'error':
            return 'failed';
          case 'pending':
          case 'running':
          case 'completed':
          case 'failed':
            return s;
          default:
            return 'pending';
        }
      };
      const members = effectiveMembers?.map((m) => {
        const agentKey = ((m.agent_key ?? m.agentKey) as string) ?? '';
        // Prefer team_summary's authoritative status (completed/failed) when
        // available; fall back to meta.members' status (pending/running).
        const summaryStatus = statusByKey.get(agentKey);
        let status = mapMemberStatus(summaryStatus || m.status || 'pending');
        // Members that were started (session created at team assembly) but
        // never executed in the graph have no TeamRunStep, so they're absent
        // from team_summary.members and their status stays "pending" from the
        // assembled event. When the team reaches a terminal status, these
        // pending members should inherit it — finalizePendingSessionActivities
        // already publishes "completed" session events for them on the
        // backend, so this aligns the team-card member display.
        if (status === 'pending') {
          const teamStatus = mapActivityStatusToStageStatus(node.status);
          if (teamStatus === 'completed' || teamStatus === 'failed' || teamStatus === 'cancelled') {
            status = teamStatus === 'cancelled' ? 'failed' : teamStatus;
          }
        }
        return {
          agentKey,
          agentName: ((m.agent_name ?? m.agentName) as string) ?? '',
          status,
          session_id: m.session_id as string | undefined,
        };
      });
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
      // B.4.1 team-card: title resolution priority — Label > Content > meta.team_name.
      // Backend team_stage events typically don't set Label/Content (only Meta),
      // so meta.team_name (set by publishSpiritTeamAssembled + cancel path) is the
      // primary source for the team display name. Without this fallback, the inline
      // TeamCard's displayTeamName degrades to generic status text ("assembling").
      //
      // Issue fix: spirit creates the primary team with the full task description
      // (markdown with `## 任务概述` headers) as DisplayName. Showing raw markdown
      // in the team-card header is ugly and breaks the layout. Strip markdown
      // headers/syntax and keep only the first meaningful line.
      const teamNameFromMeta =
        (typeof node.meta?.team_name === 'string' && node.meta.team_name) ||
        (typeof node.meta?.teamName === 'string' && node.meta.teamName) ||
        '';
      const rawTeamTitle = node.label || node.content || teamNameFromMeta;
      // Also clean the task summary — backend may store the full markdown task
      // description in meta.task_summary. Strip markdown for cleaner display.
      const cleanedTaskSummary = taskSummary ? cleanTeamDisplayName(taskSummary) : taskSummary;
      return {
        kind: 'team_stage',
        id: node.id,
        status: mapActivityStatusToStageStatus(node.status),
        title: cleanTeamDisplayName(rawTeamTitle),
        teamId: node.teamId || undefined,
        members,
        taskSummary: cleanedTaskSummary,
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
function mapActivityStatusToStageStatus(
  status: ActivityStatus,
): 'running' | 'paused' | 'completed' | 'failed' | 'cancelled' {
  switch (status) {
    case 'pending':
    case 'running':
    case 'tool_running':
    case 'tool_blocked':
      return 'running';
    case 'paused':
      return 'paused';
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

/** Stable activity comparator: Timestamp ASC, then kind priority, then id.
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
 * numerically via Date.parse. When two events share the same millisecond,
 * a kind priority tiebreaker keeps intermediate outputs in the natural
 * thinking → action → reply order (P1#5). An id-based tiebreaker is the
 * final guard for full determinism.
 *
 * Accepts the base `Activity` type so it can sort both flat lists
 * (sortedActivities) and tree nodes (ActivityTreeNode[] — TreeNode extends
 * Activity).
 */
const ACTIVITY_KIND_PRIORITY: Record<ActivityKind, number> = {
  task: 0,
  thinking: 1,
  action: 2,
  reply: 3,
  plan: 4,
  notice: 5,
  confirm: 6,
  // Issue 3 fix: graph_stage (DAG visualization) must appear before team_stage
  // (team cards) in the timeline. The DAG provides the dependency overview and
  // should be rendered between the plan and the team execution panels.
  graph_stage: 7,
  team_stage: 8,
  session: 9,
};

function isFinalReply(a: Activity): boolean {
  return a.kind === 'reply' && a.meta?.is_final === true;
}

function compareActivities(a: Activity, b: Activity): number {
  // Issue 3 fix: plan is a logical prerequisite for team execution. When the
  // spirit assembles teams before generating a plan, the plan's timestamp is
  // larger than the team_stage/graph_stage timestamps — causing the plan to
  // appear at the bottom. Override timestamp ordering to keep plan before
  // team_stage and graph_stage when they share the same parent.
  if (a.kind === 'plan' && (b.kind === 'team_stage' || b.kind === 'graph_stage')) return -1;
  if (b.kind === 'plan' && (a.kind === 'team_stage' || a.kind === 'graph_stage')) return 1;

  // Final reply (is_final=true) must render at the end of the activity stream
  // (design §1.7.3 conclusion node). The graph_stage timestamp is updated when
  // teams complete (updatedAt), which can be later than the final reply's
  // createdAt — so a pure timestamp sort would place the conclusion before the
  // graph_stage. Push is_final replies after graph_stage/team_stage to keep
  // them at the bottom. Non-final (intermediate) replies use natural timestamp
  // ordering so they appear in their correct position before plan/graph_stage.
  if (isFinalReply(a) && (b.kind === 'graph_stage' || b.kind === 'team_stage')) return 1;
  if (isFinalReply(b) && (a.kind === 'graph_stage' || a.kind === 'team_stage')) return -1;

  // Final reply must also come after non-final replies and after plan.
  if (isFinalReply(a) && !isFinalReply(b)) return 1;
  if (isFinalReply(b) && !isFinalReply(a)) return -1;

  const ta = new Date(a.timestamp).getTime();
  const tb = new Date(b.timestamp).getTime();
  if (!Number.isNaN(ta) && !Number.isNaN(tb) && ta !== tb) {
    return ta - tb;
  }
  // Fallback: lexical timestamp comparison (covers NaN cases) then kind priority.
  const tsCmp = a.timestamp.localeCompare(b.timestamp);
  if (tsCmp !== 0) return tsCmp;
  const priorityDiff = ACTIVITY_KIND_PRIORITY[a.kind] - ACTIVITY_KIND_PRIORITY[b.kind];
  if (priorityDiff !== 0) return priorityDiff;
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}
