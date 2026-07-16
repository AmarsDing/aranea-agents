// web/src/stores/chat/activityV2Store.ts
import { ref } from 'vue';
import { defineStore } from 'pinia';
import type {
  Task,
  Turn,
  Step,
  TeamStage,
  TeamRun,
  MemberSession,
  PlanBoard,
  PlanStep,
  GraphStage,
  GraphNode,
} from '../../features/chat/v2Types';
import {
  listTasksV2,
  listTurnsV2,
  listStepsV2,
  listTeamStagesV2,
  listTeamRunsV2,
  listMemberSessionsV2,
  listOrphanMemberSessionsV2,
  listPlanBoardsV2,
  listPlanStepsV2,
  listGraphStagesV2,
  listGraphNodesV2,
} from '../../features/session/v2Api';

// P2-07: record sub-resource fetch failures during history hydration.
export interface HydrationError {
  scope: string; // entity type: "turns" | "team_stages" | "team_runs" | ...
  parentId: string; // parent entity ID (task ID, team_stage ID, etc.)
  message: string; // error message
}

/**
 * useChatActivityStore holds all v2 chat entities in flat Maps keyed by ID.
 * Entities are keyed by their own ID; associations are via task_id / turn_id etc.
 *
 * Optimistic concurrency: upsert methods reject updates with Version <= existing.
 */
export const useChatActivityStore = defineStore('chatActivityV2', () => {
  const tasks = ref(new Map<string, Task>());
  const turns = ref(new Map<string, Turn>());
  const steps = ref(new Map<string, Step>());
  const teamStages = ref(new Map<string, TeamStage>());
  const teamRuns = ref(new Map<string, TeamRun>());
  const memberSessions = ref(new Map<string, MemberSession>());
  const planBoards = ref(new Map<string, PlanBoard>());
  const planSteps = ref(new Map<string, PlanStep>());
  const graphStages = ref(new Map<string, GraphStage>());
  const graphNodes = ref(new Map<string, GraphNode>());

  // P2-07: track sub-resource fetch failures so the UI can distinguish
  // "no data" from "failed to load" and show a partial/stale indicator.
  const hydrationErrors = ref<HydrationError[]>([]);
  /** Cache of member session IDs whose steps were lazy-loaded (A.4.7). */
  const loadedMemberStepSessions = ref(new Set<string>());

  function catchHydrationError<T>(scope: string, parentId: string): (e: unknown) => T[] {
    return (e: unknown) => {
      hydrationErrors.value.push({
        scope,
        parentId,
        message: e instanceof Error ? e.message : String(e),
      });
      return [] as T[];
    };
  }

  // === Upsert helpers (optimistic-concurrency guarded) ===

  function upsertTask(t: Task) {
    const ex = tasks.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    const merged: Task = ex
      ? {
          ...ex,
          ...t,
          UserMessage: t.UserMessage || ex.UserMessage,
          CreatedAt: t.CreatedAt || ex.CreatedAt,
          UpdatedAt: t.UpdatedAt || ex.UpdatedAt,
          CompletedAt: t.CompletedAt ?? ex.CompletedAt ?? null,
        }
      : { ...t };
    tasks.value.set(t.ID, merged);
    if (!t.ID.startsWith('pending-user-')) {
      for (const [id, task] of tasks.value) {
        if (id.startsWith('pending-user-') && task.SessionID === t.SessionID) {
          tasks.value.delete(id);
        }
      }
    }
  }

  /** removeTask 删除指定 ID 的 Task（用于乐观 Task 失败时清理） */
  function removeTask(taskId: string) {
    tasks.value.delete(taskId);
  }

  function upsertTurn(t: Turn) {
    const ex = turns.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    turns.value.set(t.ID, { ...t });
  }

  function upsertStep(s: Step) {
    const ex = steps.value.get(s.ID);
    if (ex && s.Version < ex.Version) return;
    // For same version (streaming updates), merge content fields instead of replacing
    if (ex && s.Version === ex.Version) {
      steps.value.set(s.ID, { ...ex, ...s });
    } else {
      steps.value.set(s.ID, { ...s });
    }
  }

  function upsertTeamStage(ts: TeamStage) {
    const ex = teamStages.value.get(ts.ID);
    if (ex && ts.Version <= ex.Version) return;
    if (ex && (!ts.Members || ts.Members.length === 0) && ex.Members && ex.Members.length > 0) {
      ts = { ...ts, Members: ex.Members };
    }
    teamStages.value.set(ts.ID, { ...ts });
  }

  function upsertTeamRun(tr: TeamRun) {
    const ex = teamRuns.value.get(tr.ID);
    if (ex && tr.Version <= ex.Version) return;
    teamRuns.value.set(tr.ID, { ...tr });
  }

  function upsertMemberSession(ms: MemberSession) {
    const ex = memberSessions.value.get(ms.ID);
    if (ex && ms.Version <= ex.Version) return;
    if (ex) {
      memberSessions.value.set(ms.ID, { ...ex, ...ms });
    } else {
      memberSessions.value.set(ms.ID, { ...ms });
    }
  }

  function upsertPlanBoard(pb: PlanBoard) {
    const ex = planBoards.value.get(pb.ID);
    if (ex && pb.Version <= ex.Version) return;
    planBoards.value.set(pb.ID, { ...pb });
    if (pb.Steps && pb.Steps.length > 0) {
      for (const ps of pb.Steps) {
        const exStep = planSteps.value.get(ps.ID);
        if (!exStep || ps.Version > exStep.Version) {
          planSteps.value.set(ps.ID, { ...ps });
        }
      }
    }
  }

  function upsertPlanStep(ps: PlanStep) {
    const ex = planSteps.value.get(ps.ID);
    if (ex && ps.Version < ex.Version) return;
    if (ex && ps.Version === ex.Version) {
      planSteps.value.set(ps.ID, { ...ex, ...ps });
    } else {
      planSteps.value.set(ps.ID, { ...ps });
    }
  }

  function upsertGraphStage(gs: GraphStage) {
    const ex = graphStages.value.get(gs.ID);
    if (ex && gs.Version <= ex.Version) return;
    graphStages.value.set(gs.ID, { ...gs });
    if (gs.Nodes && gs.Nodes.length > 0) {
      for (const gn of gs.Nodes) {
        // GraphNode 无 Version 字段，直接覆盖
        graphNodes.value.set(gn.ID, { ...gn });
      }
    }
  }

  function upsertGraphNode(gn: GraphNode) {
    const ex = graphNodes.value.get(gn.ID);
    if (!ex) {
      graphNodes.value.set(gn.ID, { ...gn });
      return;
    }
    // 合并：终态事件可能不带 TeamStageID/DependsOn/Label，不得用空值擦除已有字段。
    graphNodes.value.set(gn.ID, {
      ...ex,
      ...gn,
      TeamStageID: gn.TeamStageID || ex.TeamStageID,
      DependsOn: gn.DependsOn?.length ? gn.DependsOn : ex.DependsOn,
      Label: gn.Label || ex.Label,
      DagNodeID: gn.DagNodeID || ex.DagNodeID,
    });
  }

  // === Streaming delta (does NOT bump version) ===

  // E2E-P1-05: fingerprint recent streaming deltas to skip WS redelivery
  // duplicates. Key = stepId:field; ring-bounded fingerprints per key.
  const deltaDedupMaxPerKey = 64;
  const recentDeltas = new Map<string, string[]>();

  function deltaFingerprint(chunk: string): string {
    const n = chunk.length;
    if (n <= 48) return `${n}:${chunk}`;
    return `${n}:${chunk.slice(0, 24)}:${chunk.slice(-24)}`;
  }

  function shouldApplyDelta(stepId: string, field: string, chunk: string): boolean {
    if (!chunk) return false;
    const key = `${stepId}:${field}`;
    const fp = deltaFingerprint(chunk);
    let ring = recentDeltas.get(key);
    if (!ring) {
      ring = [];
      recentDeltas.set(key, ring);
    }
    if (ring.includes(fp)) return false;
    ring.push(fp);
    if (ring.length > deltaDedupMaxPerKey) {
      ring.splice(0, ring.length - deltaDedupMaxPerKey);
    }
    return true;
  }

  // P2-06 fix: accept any DeltaField string from the backend and handle only
  // the known string-appendable fields (content, reasoning). Unknown fields
  // (e.g. tool_args, state) are silently ignored — the final value arrives
  // via the subsequent step.completed event which carries the complete Step
  // entity. The previous if/else caught ALL non-content fields and appended
  // them to Reasoning, causing silent data corruption.
  function appendStepDelta(stepId: string, field: string, chunk: string) {
    const s = steps.value.get(stepId);
    if (!s) return;
    if (!shouldApplyDelta(stepId, field, chunk)) return;
    switch (field) {
      case 'content':
        s.Content += chunk;
        break;
      case 'reasoning':
        s.Reasoning += chunk;
        break;
      default:
        // Unknown delta field — ignore; final value comes from step.completed.
        break;
    }
  }

  // === Query helpers ===

  /** Primary sort: StartedAt/CreatedAt ASC; Seq as tiebreaker (streaming order). */
  function compareByTimeThenSeq(
    a: { Seq: number; StartedAt?: string; CreatedAt?: string },
    b: { Seq: number; StartedAt?: string; CreatedAt?: string },
  ): number {
    const ta = Date.parse(a.StartedAt || a.CreatedAt || '') || 0;
    const tb = Date.parse(b.StartedAt || b.CreatedAt || '') || 0;
    if (ta !== tb) return ta - tb;
    return a.Seq - b.Seq;
  }

  function getSessionTasks(sessionId: string): Task[] {
    const out: Task[] = [];
    for (const t of tasks.value.values()) {
      if (t.SessionID === sessionId) out.push(t);
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getTaskTurns(taskId: string): Turn[] {
    const out: Turn[] = [];
    for (const t of turns.value.values()) {
      if (t.TaskID === taskId) out.push(t);
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getTurnSteps(turnId: string): Step[] {
    const out: Step[] = [];
    for (const s of steps.value.values()) {
      if (s.TurnID === turnId) out.push(s);
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getTaskTeamStages(taskId: string): TeamStage[] {
    const out: TeamStage[] = [];
    for (const ts of teamStages.value.values()) {
      if (ts.TaskID === taskId) out.push(ts);
    }
    return out.sort(compareByTimeThenSeq);
  }

  // 按触发 turn 查询 team stages（设计稿 §3.6.2：teamStages.filter(ts => ts.turnId === turn.id)）
  function getTurnTeamStages(turnId: string): TeamStage[] {
    const out: TeamStage[] = [];
    for (const ts of teamStages.value.values()) {
      if (ts.TurnID === turnId) out.push(ts);
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getTaskPlanBoards(taskId: string): PlanBoard[] {
    const out: PlanBoard[] = [];
    for (const pb of planBoards.value.values()) {
      if (pb.TaskID === taskId) out.push(pb);
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getTaskGraphStages(taskId: string): GraphStage[] {
    const out: GraphStage[] = [];
    for (const gs of graphStages.value.values()) {
      if (gs.TaskID === taskId) out.push(gs);
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getGraphStageByPlanBoard(planBoardId: string): GraphStage | undefined {
    for (const gs of graphStages.value.values()) {
      if (gs.PlanBoardID === planBoardId) return gs;
    }
    return undefined;
  }

  function getPlanBoardSteps(planBoardId: string): PlanStep[] {
    const out: PlanStep[] = [];
    for (const ps of planSteps.value.values()) {
      if (ps.PlanID === planBoardId) out.push(ps);
    }
    return out.sort(compareByTimeThenSeq);
  }

  // Match steps for a MemberSession.
  // 1) Prefer Step.SessionID === MemberSession.SessionID (lazy-load / Mode B safe)
  // 2) Else Turn.TeamStageID + AuthorAgentKey
  // 3) Fallback AuthorAgentKey + TaskID (legacy; may cross-team)
  function getMemberSessionSteps(memberSession: MemberSession): Step[] {
    const agentKey = memberSession.AgentKey;
    const taskId = memberSession.TaskID;
    const sessionId = memberSession.SessionID;
    if (!agentKey || !taskId) return [];

    const out: Step[] = [];
    if (sessionId) {
      for (const s of steps.value.values()) {
        if (s.SessionID === sessionId) out.push(s);
      }
      if (out.length > 0) return out.sort(compareByTimeThenSeq);
    }

    const turnIds = new Set<string>();
    for (const t of turns.value.values()) {
      if (t.TeamStageID && t.TeamStageID === memberSession.TeamStageID && t.AgentKey === agentKey) {
        turnIds.add(t.ID);
      }
    }
    if (turnIds.size > 0) {
      for (const s of steps.value.values()) {
        if (turnIds.has(s.TurnID) && s.AuthorAgentKey === agentKey) {
          out.push(s);
        }
      }
    }

    if (out.length === 0) {
      for (const s of steps.value.values()) {
        if (s.AuthorAgentKey === agentKey && s.TaskID === taskId) {
          out.push(s);
        }
      }
    }

    return out.sort(compareByTimeThenSeq);
  }

  /** Mode B / orphan: member sessions on a task not rendered under any TeamRun.
   *  Also hosts Mode B cards with empty TaskID on the session's running (else latest) task. */
  function getTaskOrphanMemberSessions(taskId: string): MemberSession[] {
    const task = tasks.value.get(taskId);
    if (!task) return [];

    const shown = new Set<string>();
    for (const tr of teamRuns.value.values()) {
      if (tr.TaskID !== taskId) continue;
      for (const ms of memberSessions.value.values()) {
        if (ms.TeamRunID === tr.ID) shown.add(ms.ID);
      }
    }

    const sessionTasks = getSessionTasks(task.SessionID);
    const host =
      sessionTasks.find((t) => t.Status === 'running') ?? sessionTasks[sessionTasks.length - 1] ?? null;
    const isHost = host?.ID === taskId;

    const out: MemberSession[] = [];
    for (const ms of memberSessions.value.values()) {
      if (shown.has(ms.ID)) continue;
      if (ms.TaskID === taskId) {
        out.push(ms);
        continue;
      }
      // Mode B: empty TaskID + matching spirit session → host on latest/running task
      if (
        isHost &&
        !ms.TaskID &&
        !ms.TeamRunID &&
        ms.SpiritSessionID === task.SessionID
      ) {
        out.push(ms);
      }
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getGraphStageNodes(graphStageId: string): GraphNode[] {
    const out: GraphNode[] = [];
    for (const gn of graphNodes.value.values()) {
      if (gn.GraphStageID === graphStageId) out.push(gn);
    }
    return out;
  }

  function getTeamStageTeamRuns(teamStageId: string): TeamRun[] {
    const out: TeamRun[] = [];
    for (const tr of teamRuns.value.values()) {
      if (tr.TeamStageID === teamStageId) out.push(tr);
    }
    return out.sort(compareByTimeThenSeq);
  }

  function getTeamRunMemberSessions(teamRunId: string): MemberSession[] {
    const out: MemberSession[] = [];
    for (const ms of memberSessions.value.values()) {
      if (ms.TeamRunID === teamRunId) out.push(ms);
    }
    return out.sort(compareByTimeThenSeq);
  }

  // === Bulk operations ===

  function clearSession(spiritSessionId: string) {
    for (const [id, t] of tasks.value) {
      if (t.SessionID === spiritSessionId) tasks.value.delete(id);
    }
    for (const [id, t] of turns.value) {
      if (t.SpiritSessionID === spiritSessionId) turns.value.delete(id);
    }
    for (const [id, s] of steps.value) {
      if (s.SpiritSessionID === spiritSessionId) {
        steps.value.delete(id);
        recentDeltas.delete(`${id}:content`);
        recentDeltas.delete(`${id}:reasoning`);
      }
    }
    for (const [id, ts] of teamStages.value) {
      if (ts.SessionID === spiritSessionId) teamStages.value.delete(id);
    }
    for (const [id, tr] of teamRuns.value) {
      if (tr.SpiritSessionID === spiritSessionId) teamRuns.value.delete(id);
    }
    for (const [id, ms] of memberSessions.value) {
      if (ms.SpiritSessionID === spiritSessionId) memberSessions.value.delete(id);
    }
    for (const [id, pb] of planBoards.value) {
      if (pb.SessionID === spiritSessionId) planBoards.value.delete(id);
    }
    for (const [id, ps] of planSteps.value) {
      if (ps.TaskID && tasks.value.has(ps.TaskID) === false) planSteps.value.delete(id);
    }
    for (const [id, gs] of graphStages.value) {
      if (gs.SessionID === spiritSessionId) graphStages.value.delete(id);
    }
    for (const [id, gn] of graphNodes.value) {
      if (gn.GraphStageID && graphStages.value.has(gn.GraphStageID) === false) graphNodes.value.delete(id);
    }
  }

  function clearAll() {
    tasks.value.clear();
    turns.value.clear();
    steps.value.clear();
    recentDeltas.clear();
    teamStages.value.clear();
    teamRuns.value.clear();
    memberSessions.value.clear();
    planBoards.value.clear();
    planSteps.value.clear();
    graphStages.value.clear();
    graphNodes.value.clear();
    loadedMemberStepSessions.value.clear();
  }

  // === History fetch (page refresh / WS reconnect) ===

  /**
   * fetchSessionHistory loads the v2 entity tree for a spirit (root) session:
   *   - Top level: tasks + root-session steps only (SessionID = spirit session)
   *   - Per task: turns + team_stages + plan_boards + plan_steps + graph_stages
   *   - Per team_stage: team_runs → member_sessions (metadata only)
   *   - Mode B: orphan member_sessions (empty TeamRunID) by spirit session
   *   - Per graph_stage: graph_nodes
   * Member step content is lazy-loaded via ensureMemberStepsLoaded on expand (A.4.7).
   */
  async function fetchSessionHistory(sessionId: string): Promise<void> {
    // P2-07: clear previous hydration errors at the start of each fetch.
    hydrationErrors.value = [];

    const [tasksList, stepsList] = await Promise.all([listTasksV2(sessionId), listStepsV2(sessionId)]);
    for (const t of tasksList) upsertTask(t);
    for (const s of stepsList) upsertStep(s);

    // Per-task parallel fetch: turns + 4 task-scoped child entities.
    // Each call catches its own failure → empty array + recorded error,
    // so one failing task doesn't poison the others and the UI can show
    // a partial/stale indicator.
    const perTaskResults = await Promise.all(
      tasksList.map(async (t) => ({
        turns: await listTurnsV2(t.ID).catch(catchHydrationError<Turn>('turns', t.ID)),
        teamStages: await listTeamStagesV2(t.ID).catch(catchHydrationError<TeamStage>('team_stages', t.ID)),
        planBoards: await listPlanBoardsV2(t.ID).catch(catchHydrationError<PlanBoard>('plan_boards', t.ID)),
        planSteps: await listPlanStepsV2(t.ID).catch(catchHydrationError<PlanStep>('plan_steps', t.ID)),
        graphStages: await listGraphStagesV2(t.ID).catch(catchHydrationError<GraphStage>('graph_stages', t.ID)),
      })),
    );
    for (const r of perTaskResults) {
      for (const turn of r.turns) upsertTurn(turn);
      for (const ts of r.teamStages) upsertTeamStage(ts);
      for (const pb of r.planBoards) upsertPlanBoard(pb);
      for (const ps of r.planSteps) upsertPlanStep(ps);
      for (const gs of r.graphStages) upsertGraphStage(gs);
    }

    // Flatten team_stages and graph_stages across all tasks for next-level fetch.
    const allTeamStages: TeamStage[] = [];
    const allGraphStages: GraphStage[] = [];
    for (const r of perTaskResults) {
      allTeamStages.push(...r.teamStages);
      allGraphStages.push(...r.graphStages);
    }

    // Per team_stage: fetch team_runs (parallel). Isolated failures → [] + error.
    const teamRunLists = await Promise.all(
      allTeamStages.map((ts) => listTeamRunsV2(ts.ID).catch(catchHydrationError<TeamRun>('team_runs', ts.ID))),
    );
    const allTeamRuns: TeamRun[] = [];
    for (const runs of teamRunLists) {
      for (const tr of runs) upsertTeamRun(tr);
      allTeamRuns.push(...runs);
    }

    // Per team_run: fetch member_sessions (parallel). Isolated failures → [] + error.
    const memberSessionLists = await Promise.all(
      allTeamRuns.map((tr) =>
        listMemberSessionsV2(tr.ID).catch(catchHydrationError<MemberSession>('member_sessions', tr.ID)),
      ),
    );
    for (const sessions of memberSessionLists) {
      for (const ms of sessions) upsertMemberSession(ms);
    }

    // Mode B: orphan member sessions (empty TeamRunID) for this spirit session.
    const orphans = await listOrphanMemberSessionsV2(sessionId).catch(
      catchHydrationError<MemberSession>('orphan_member_sessions', sessionId),
    );
    for (const ms of orphans) upsertMemberSession(ms);

    // Per graph_stage: fetch graph_nodes (parallel). Isolated failures → [] + error.
    const graphNodeLists = await Promise.all(
      allGraphStages.map((gs) => listGraphNodesV2(gs.ID).catch(catchHydrationError<GraphNode>('graph_nodes', gs.ID))),
    );
    for (const nodes of graphNodeLists) {
      for (const gn of nodes) upsertGraphNode(gn);
    }
  }

  /** Lazy-load steps for member/child sessions (A.4.7). Cache-aware. */
  async function ensureMemberStepsLoaded(sessionIds: string[]): Promise<void> {
    const pending = sessionIds.filter((id) => id && !loadedMemberStepSessions.value.has(id));
    if (pending.length === 0) return;
    await Promise.all(
      pending.map(async (sid) => {
        try {
          const list = await listStepsV2(sid);
          for (const s of list) upsertStep(s);
          loadedMemberStepSessions.value.add(sid);
        } catch (e) {
          hydrationErrors.value.push({
            scope: 'member_steps',
            parentId: sid,
            message: e instanceof Error ? e.message : String(e),
          });
        }
      }),
    );
  }

  return {
    tasks,
    turns,
    steps,
    teamStages,
    teamRuns,
    memberSessions,
    planBoards,
    planSteps,
    graphStages,
    graphNodes,
    upsertTask,
    removeTask,
    upsertTurn,
    upsertStep,
    upsertTeamStage,
    upsertTeamRun,
    upsertMemberSession,
    upsertPlanBoard,
    upsertPlanStep,
    upsertGraphStage,
    upsertGraphNode,
    appendStepDelta,
    getSessionTasks,
    getTaskTurns,
    getTurnSteps,
    getTaskTeamStages,
    getTurnTeamStages,
    getTaskPlanBoards,
    getTaskGraphStages,
    getGraphStageByPlanBoard,
    getPlanBoardSteps,
    getMemberSessionSteps,
    getTaskOrphanMemberSessions,
    getGraphStageNodes,
    getTeamStageTeamRuns,
    getTeamRunMemberSessions,
    clearSession,
    clearAll,
    fetchSessionHistory,
    ensureMemberStepsLoaded,
    hydrationErrors,
  };
});
