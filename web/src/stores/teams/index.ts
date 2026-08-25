import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listTeams,
  createTeam,
  updateTeam,
  duplicateTeam,
  deleteTeam,
  retryTeam as retryTeamApi,
  listTeamRuns,
  listTeamRunSteps,
  getTeamRun,
  getTeamRunSummary,
  runTeamTest,
  subscribeTeamRunEventsWs,
  findActiveTeamRun,
} from '../../features/teams/api';
import { listAgents } from '../../features/agents/api';
import { TEAM_AGENT_LIST_LIMIT, TEAM_RUNS_LIMIT, TEAM_RUNS_LOCAL_MAX } from '../../features/constants/queryLimits';
import type { Agent } from '../../features/agents/types';
import type { Team, TeamRun, TeamRunEvent, TeamRunStep, TeamRunSummary } from '../../features/teams/types';

export const useTeamsStore = defineStore('teams', () => {
  // ── Team list ──
  const teams = ref<Team[]>([]);
  const activeTeam = ref<Team | null>(null);
  const loading = ref(false);

  // ── Agents (shared across team pages) ──
  const agents = ref<Agent[]>([]);

  // ── Runs ──
  const runs = ref<TeamRun[]>([]);
  const runsLoading = ref(false);
  const runsError = ref('');
  const selectedTeam = ref<Team | null>(null);

  // ── Run steps ──
  const stepsByRun = ref<Record<string, TeamRunStep[]>>({});
  const stepsLoading = ref<Record<string, boolean>>({});

  // ── Run summaries ──
  const summariesByRun = ref<Record<string, TeamRunSummary>>({});
  const summariesLoading = ref<Record<string, boolean>>({});

  // ── Run details（79-runtime-governance 0.1：cache_hit_ratio 载体）──
  const detailsByRun = ref<Record<string, TeamRun>>({});
  const detailsLoading = ref<Record<string, boolean>>({});

  // ── Team CRUD ──

  async function loadTeams() {
    loading.value = true;
    try {
      teams.value = await listTeams();
    } finally {
      loading.value = false;
    }
  }

  async function addTeam(payload: Partial<Team>) {
    const created = await createTeam(payload);
    teams.value.unshift(created);
    activeTeam.value = created;
    return created;
  }

  async function editTeam(id: string, payload: Partial<Team>) {
    const updated = await updateTeam(id, payload);
    teams.value = teams.value.map((t) => (t.id === id ? updated : t));
    if (activeTeam.value?.id === id) activeTeam.value = updated;
    return updated;
  }

  async function copy(id: string) {
    const copy = await duplicateTeam(id);
    teams.value.push(copy);
    return copy;
  }

  async function remove(id: string) {
    await deleteTeam(id);
    teams.value = teams.value.filter((t) => t.id !== id);
    if (activeTeam.value?.id === id) activeTeam.value = null;
  }

  /** Retry a failed/cancelled team: backend flips status → pending; refresh list copy. */
  async function retryTeam(id: string) {
    const res = await retryTeamApi(id);
    teams.value = teams.value.map((t) => (t.id === id ? { ...t, status: res.status || 'pending' } : t));
    if (activeTeam.value?.id === id) activeTeam.value = { ...activeTeam.value, status: res.status || 'pending' };
    return res;
  }

  async function fetchTeam(id: string) {
    let team = teams.value.find((t) => t.id === id) ?? null;
    if (!team) {
      await loadTeams();
      team = teams.value.find((t) => t.id === id) ?? null;
    }
    if (!team) {
      throw new Error('Team not found');
    }
    activeTeam.value = team;
    return team;
  }

  function setActiveTeam(t: Team | null) {
    activeTeam.value = t;
  }

  // ── Agents ──

  async function loadAgents() {
    agents.value = await listAgents({ limit: TEAM_AGENT_LIST_LIMIT });
    return agents.value;
  }

  // ── Runs (state written to Store) ──

  async function loadRuns(teamId?: string, limit = TEAM_RUNS_LIMIT) {
    runsLoading.value = true;
    runsError.value = '';
    try {
      runs.value = await listTeamRuns(teamId, limit);
    } catch (err) {
      runsError.value = err instanceof Error ? err.message : '加载运行轨迹失败';
    } finally {
      runsLoading.value = false;
    }
    return runs.value;
  }

  async function loadRunSteps(runId: string) {
    if (stepsByRun.value[runId]?.length || stepsLoading.value[runId]) return stepsByRun.value[runId] ?? [];
    stepsLoading.value = { ...stepsLoading.value, [runId]: true };
    try {
      const steps = await listTeamRunSteps(runId);
      stepsByRun.value = { ...stepsByRun.value, [runId]: steps };
      return steps;
    } finally {
      stepsLoading.value = { ...stepsLoading.value, [runId]: false };
    }
  }

  async function loadRunSummary(runId: string) {
    if (summariesByRun.value[runId] || summariesLoading.value[runId]) return summariesByRun.value[runId];
    summariesLoading.value = { ...summariesLoading.value, [runId]: true };
    try {
      const summary = await getTeamRunSummary(runId);
      summariesByRun.value = { ...summariesByRun.value, [runId]: summary };
      return summary;
    } finally {
      summariesLoading.value = { ...summariesLoading.value, [runId]: false };
    }
  }

  async function loadRunDetail(runId: string) {
    if (detailsByRun.value[runId] || detailsLoading.value[runId]) return detailsByRun.value[runId];
    detailsLoading.value = { ...detailsLoading.value, [runId]: true };
    try {
      const detail = await getTeamRun(runId);
      detailsByRun.value = { ...detailsByRun.value, [runId]: detail };
      return detail;
    } finally {
      detailsLoading.value = { ...detailsLoading.value, [runId]: false };
    }
  }

  async function testTeam(teamId: string, content?: string) {
    return runTeamTest(teamId, content);
  }

  async function findActiveRun(teamId: string) {
    return findActiveTeamRun(teamId);
  }

  // ── Runs upsert (called from WS events) ──

  function upsertRun(run: TeamRun) {
    const index = runs.value.findIndex((item) => item.id === run.id);
    if (index >= 0) {
      runs.value = runs.value.map((item) => (item.id === run.id ? run : item));
      return;
    }
    runs.value = [run, ...runs.value].slice(0, TEAM_RUNS_LOCAL_MAX);
  }

  function upsertRunStep(step: TeamRunStep) {
    const current = stepsByRun.value[step.run_id] ?? [];
    const exists = current.some((item) => item.id === step.id);
    const next = exists ? current.map((item) => (item.id === step.id ? step : item)) : [...current, step];
    next.sort((a, b) => a.sort_order - b.sort_order || a.created_at.localeCompare(b.created_at));
    stepsByRun.value = { ...stepsByRun.value, [step.run_id]: next };
  }

  function clearRunsState() {
    summariesByRun.value = {};
    stepsByRun.value = {};
    detailsByRun.value = {};
  }

  // ── WS events ──

  function subscribeRunEvents(sessionId: string, teamID: string, onEvent: (event: TeamRunEvent) => void) {
    return subscribeTeamRunEventsWs(sessionId, teamID, onEvent);
  }

  return {
    teams,
    activeTeam,
    loading,
    agents,
    runs,
    runsLoading,
    runsError,
    selectedTeam,
    stepsByRun,
    stepsLoading,
    summariesByRun,
    summariesLoading,
    detailsByRun,
    detailsLoading,
    loadTeams,
    fetchTeam,
    addTeam,
    editTeam,
    copy,
    remove,
    retryTeam,
    setActiveTeam,
    loadAgents,
    loadRuns,
    loadRunSteps,
    loadRunSummary,
    loadRunDetail,
    testTeam,
    findActiveRun,
    upsertRun,
    upsertRunStep,
    clearRunsState,
    subscribeRunEvents,
  };
});
