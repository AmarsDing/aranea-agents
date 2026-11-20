import { computed, ref } from 'vue';
import { listTeamRuns } from './api';
import type { Team, TeamRun } from './types';
import { parseKratosApiError } from '../../utils/kratosError';

const RECENT_RUNS = 5;

/** Team 页检视面板：选中的团队 + 它最近几次运行（每次选中重新读，过期响应丢弃）。 */
export function useTeamInspector() {
  const team = ref<Team | null>(null);
  const recentRuns = ref<TeamRun[]>([]);
  const runsLoading = ref(false);
  const runsError = ref('');
  let seq = 0;

  async function inspect(next: Team) {
    team.value = next;
    const mine = ++seq;
    recentRuns.value = [];
    runsError.value = '';
    runsLoading.value = true;
    try {
      const rows = await listTeamRuns(next.id, RECENT_RUNS);
      if (mine === seq) recentRuns.value = rows;
    } catch (err) {
      if (mine === seq) runsError.value = parseKratosApiError(err).message;
    } finally {
      if (mine === seq) runsLoading.value = false;
    }
  }

  function close() {
    seq++;
    team.value = null;
  }

  const open = computed({
    get: () => team.value !== null,
    set: (value: boolean) => {
      if (!value) close();
    },
  });

  return { team, open, recentRuns, runsLoading, runsError, inspect, close };
}
