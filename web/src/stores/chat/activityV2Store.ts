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
} from '../../features/chat/v2Types';
import { listTasksV2, listTurnsV2, listStepsV2 } from '../../features/session/v2Api';

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

  // === Upsert helpers (optimistic-concurrency guarded) ===

  function upsertTask(t: Task) {
    const ex = tasks.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    tasks.value.set(t.ID, { ...t });
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
    memberSessions.value.set(ms.ID, { ...ms });
  }

  function upsertPlanBoard(pb: PlanBoard) {
    const ex = planBoards.value.get(pb.ID);
    if (ex && pb.Version <= ex.Version) return;
    planBoards.value.set(pb.ID, { ...pb });
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
  }

  // === History fetch (page refresh / WS reconnect) ===

  /**
   * fetchSessionHistory loads the full v2 entity tree for a session:
   * all tasks + all steps for the session (parallel), then turns per-task
   * (parallel). Per-task turn fetch failures are swallowed so partial
   * history can still render. Top-level task/step failures propagate.
   */
  async function fetchSessionHistory(sessionId: string): Promise<void> {
    const [tasksList, stepsList] = await Promise.all([listTasksV2(sessionId), listStepsV2(sessionId)]);
    for (const t of tasksList) upsertTask(t);
    for (const s of stepsList) upsertStep(s);

    // Fetch turns per-task in parallel; isolated failures yield empty arrays.
    const turnLists = await Promise.all(tasksList.map((t) => listTurnsV2(t.ID).catch(() => [] as Turn[])));
    for (const turns of turnLists) {
      for (const turn of turns) upsertTurn(turn);
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
    upsertTask,
    upsertTurn,
    upsertStep,
    upsertTeamStage,
    upsertTeamRun,
    upsertMemberSession,
    upsertPlanBoard,
    upsertPlanStep,
    appendStepDelta,
    getSessionTasks,
    getTaskTurns,
    getTurnSteps,
    getTaskTeamStages,
    getTaskPlanBoards,
    getTeamStageTeamRuns,
    getTeamRunMemberSessions,
    clearSession,
    clearAll,
    fetchSessionHistory,
  };
});
