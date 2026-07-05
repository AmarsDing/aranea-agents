// web/src/stores/chat/activityV2Store.ts
import { ref, computed } from 'vue';
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
  listPlanBoardsV2,
  listPlanStepsV2,
  listGraphStagesV2,
  listGraphNodesV2,
} from '../../features/session/v2Api';

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

  // === Upsert helpers (optimistic-concurrency guarded) ===

  function upsertTask(t: Task) {
    const ex = tasks.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    tasks.value.set(t.ID, { ...t });
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
    // GraphNode 没有 Version 字段；直接覆盖（事件顺序保证最新）
    graphNodes.value.set(gn.ID, { ...gn });
  }

  // === Streaming delta (does NOT bump version) ===

  function appendStepDelta(stepId: string, field: 'content' | 'reasoning', chunk: string) {
    const s = steps.value.get(stepId);
    if (!s) return;
    if (field === 'content') s.Content += chunk;
    else s.Reasoning += chunk;
  }

  // === Query helpers ===

  function getSessionTasks(sessionId: string): Task[] {
    const out: Task[] = [];
    for (const t of tasks.value.values()) {
      if (t.SessionID === sessionId) out.push(t);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTaskTurns(taskId: string): Turn[] {
    const out: Turn[] = [];
    for (const t of turns.value.values()) {
      if (t.TaskID === taskId) out.push(t);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTurnSteps(turnId: string): Step[] {
    const out: Step[] = [];
    for (const s of steps.value.values()) {
      if (s.TurnID === turnId) out.push(s);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTaskTeamStages(taskId: string): TeamStage[] {
    const out: TeamStage[] = [];
    for (const ts of teamStages.value.values()) {
      if (ts.TaskID === taskId) out.push(ts);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTaskPlanBoards(taskId: string): PlanBoard[] {
    const out: PlanBoard[] = [];
    for (const pb of planBoards.value.values()) {
      if (pb.TaskID === taskId) out.push(pb);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTaskGraphStages(taskId: string): GraphStage[] {
    const out: GraphStage[] = [];
    for (const gs of graphStages.value.values()) {
      if (gs.TaskID === taskId) out.push(gs);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
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
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  // Step 无直接外键关联 MemberSession，通过 AuthorAgentKey + TaskID 间接匹配。
  // 同一 agent 在同一 task 下的 thinking/action/reply 等 step 都会返回。
  function getMemberSessionSteps(memberSession: MemberSession): Step[] {
    const out: Step[] = [];
    const agentKey = memberSession.AgentKey;
    const taskId = memberSession.TaskID;
    if (!agentKey || !taskId) return out;
    for (const s of steps.value.values()) {
      if (s.AuthorAgentKey === agentKey && s.TaskID === taskId) {
        out.push(s);
      }
    }
    return out.sort((a, b) => a.Seq - b.Seq);
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
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTeamRunMemberSessions(teamRunId: string): MemberSession[] {
    const out: MemberSession[] = [];
    for (const ms of memberSessions.value.values()) {
      if (ms.TeamRunID === teamRunId) out.push(ms);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
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
      if (s.SpiritSessionID === spiritSessionId) steps.value.delete(id);
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
    teamStages.value.clear();
    teamRuns.value.clear();
    memberSessions.value.clear();
    planBoards.value.clear();
    planSteps.value.clear();
    graphStages.value.clear();
    graphNodes.value.clear();
  }

  // === History fetch (page refresh / WS reconnect) ===

  /**
   * fetchSessionHistory loads the full v2 entity tree for a session:
   *   - Top level (parallel): tasks + steps for the session
   *   - Per task (parallel): turns + team_stages + plan_boards + plan_steps + graph_stages
   *   - Per team_stage (parallel): team_runs
   *   - Per team_run (parallel): member_sessions
   *   - Per graph_stage (parallel): graph_nodes
   *
   * Isolated failures at child levels are swallowed (.catch(() => [])) so
   * partial history can still render. Top-level task/step failures propagate.
   */
  async function fetchSessionHistory(sessionId: string): Promise<void> {
    const [tasksList, stepsList] = await Promise.all([listTasksV2(sessionId), listStepsV2(sessionId)]);
    for (const t of tasksList) upsertTask(t);
    for (const s of stepsList) upsertStep(s);

    // Per-task parallel fetch: turns + 4 task-scoped child entities.
    // Each call catches its own failure → empty array, so one failing task
    // doesn't poison the others.
    const perTaskResults = await Promise.all(
      tasksList.map(async (t) => ({
        turns: await listTurnsV2(t.ID).catch(() => [] as Turn[]),
        teamStages: await listTeamStagesV2(t.ID).catch(() => [] as TeamStage[]),
        planBoards: await listPlanBoardsV2(t.ID).catch(() => [] as PlanBoard[]),
        planSteps: await listPlanStepsV2(t.ID).catch(() => [] as PlanStep[]),
        graphStages: await listGraphStagesV2(t.ID).catch(() => [] as GraphStage[]),
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

    // Per team_stage: fetch team_runs (parallel). Isolated failures → [].
    const teamRunLists = await Promise.all(
      allTeamStages.map((ts) => listTeamRunsV2(ts.ID).catch(() => [] as TeamRun[])),
    );
    const allTeamRuns: TeamRun[] = [];
    for (const runs of teamRunLists) {
      for (const tr of runs) upsertTeamRun(tr);
      allTeamRuns.push(...runs);
    }

    // Per team_run: fetch member_sessions (parallel). Isolated failures → [].
    const memberSessionLists = await Promise.all(
      allTeamRuns.map((tr) => listMemberSessionsV2(tr.ID).catch(() => [] as MemberSession[])),
    );
    for (const sessions of memberSessionLists) {
      for (const ms of sessions) upsertMemberSession(ms);
    }

    // Per graph_stage: fetch graph_nodes (parallel). Isolated failures → [].
    const graphNodeLists = await Promise.all(
      allGraphStages.map((gs) => listGraphNodesV2(gs.ID).catch(() => [] as GraphNode[])),
    );
    for (const nodes of graphNodeLists) {
      for (const gn of nodes) upsertGraphNode(gn);
    }
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
    getTaskPlanBoards,
    getTaskGraphStages,
    getGraphStageByPlanBoard,
    getPlanBoardSteps,
    getMemberSessionSteps,
    getGraphStageNodes,
    getTeamStageTeamRuns,
    getTeamRunMemberSessions,
    clearSession,
    clearAll,
    fetchSessionHistory,
  };
});
