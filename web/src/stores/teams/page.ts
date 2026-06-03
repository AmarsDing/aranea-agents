import { defineStore } from 'pinia';
import { ref } from 'vue';
import { listAgents } from '../../features/agents/api';
import type { Agent } from '../../features/agents/types';
import {
  createTeam,
  deleteTeam,
  duplicateTeam,
  getTeamRunSummary,
  listTeamRuns,
  listTeamRunSteps,
  listTeams,
  runTeamTest,
  subscribeTeamRunEventsWs,
  updateTeam,
} from '../../features/teams/api';
import type { Team, TeamRun, TeamRunEvent, TeamRunStep, TeamRunSummary } from '../../features/teams/types';

/** Teams 列表页：运行历史、测试与 WS 事件（HTTP 仅在此 Store actions）。 */
export const useTeamsPageStore = defineStore('teamsPage', () => {
  const agents = ref<Agent[]>([]);

  async function loadAgents() {
    agents.value = await listAgents({ limit: 1000 });
    return agents.value;
  }

  async function loadTeams() {
    return listTeams();
  }

  async function addTeam(payload: Partial<Team>) {
    return createTeam(payload);
  }

  async function editTeam(id: string, payload: Partial<Team>) {
    return updateTeam(id, payload);
  }

  async function copyTeam(id: string) {
    return duplicateTeam(id);
  }

  async function removeTeam(id: string) {
    await deleteTeam(id);
  }

  async function loadRuns(teamId?: string, limit = 50) {
    return listTeamRuns(teamId, limit);
  }

  async function loadRunSteps(runId: string) {
    return listTeamRunSteps(runId);
  }

  async function loadRunSummary(runId: string) {
    return getTeamRunSummary(runId);
  }

  async function testTeam(teamId: string, content?: string) {
    return runTeamTest(teamId, content);
  }

  function subscribeRunEvents(
    sessionId: string,
    teamID: string,
    onEvent: (event: TeamRunEvent) => void,
    onError?: (error: string) => void,
    onReplayState?: (replaying: boolean) => void,
  ) {
    return subscribeTeamRunEventsWs(sessionId, teamID, onEvent, onError, onReplayState);
  }

  return {
    agents,
    loadAgents,
    loadTeams,
    addTeam,
    editTeam,
    copyTeam,
    removeTeam,
    loadRuns,
    loadRunSteps,
    loadRunSummary,
    testTeam,
    subscribeRunEvents,
  };
});
