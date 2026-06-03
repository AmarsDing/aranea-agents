import { defineStore } from 'pinia';
import { compileTeamGraph } from '../../features/orchestration/compileApi';
import type { CompileTeamGraphResult } from '../../features/orchestration/compileApi';
import { getTeamRunObservatory } from '../../features/orchestration/api';
import type { TeamRunObservatory } from '../../features/orchestration/types';

export const useOrchestrationStore = defineStore('orchestration', () => {
  async function compileTeam(teamId: string, definitionJson?: string): Promise<CompileTeamGraphResult> {
    return compileTeamGraph(teamId, definitionJson);
  }

  async function fetchRunObservatory(runId: string): Promise<TeamRunObservatory> {
    return getTeamRunObservatory(runId);
  }

  return { compileTeam, fetchRunObservatory };
});
