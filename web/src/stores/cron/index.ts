import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listCronTasks,
  listCronAgents,
  listCronTeams,
  createCronTask,
  updateCronTask,
  deleteCronTask,
  listCronTaskRuns,
  triggerCronTask,
  resetCronTaskFailures,
  listFleetAutomations,
  setFleetAutomationEnabled,
  runFleetAutomationNow,
  type FleetAutomation,
  type FleetProject,
  type PlatformResource,
  type PlatformResourceInput,
} from '../../features/cron/api';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import type { CronTaskRun, CronTaskRunQuery } from '../../features/cron/types';

export const useCronStore = defineStore('cron', () => {
  const tasks = ref<PlatformResource[]>([]);
  const runs = ref<CronTaskRun[]>([]);
  const runsTotal = ref(0);
  const agents = ref<Agent[]>([]);
  const teams = ref<Team[]>([]);
  const loading = ref(false);

  async function loadTasks() {
    loading.value = true;
    try {
      tasks.value = await listCronTasks();
      return tasks.value;
    } finally {
      loading.value = false;
    }
  }

  async function loadTargets() {
    const [agentRows, teamRows] = await Promise.all([listCronAgents(), listCronTeams()]);
    agents.value = agentRows;
    teams.value = teamRows;
    return { agents: agentRows, teams: teamRows };
  }

  async function loadAll() {
    loading.value = true;
    try {
      const [taskRows, targets] = await Promise.all([listCronTasks(), loadTargets()]);
      tasks.value = taskRows;
      return { tasks: taskRows, ...targets };
    } finally {
      loading.value = false;
    }
  }

  async function loadRuns(query?: CronTaskRunQuery) {
    const result = await listCronTaskRuns(query);
    runs.value = result.items;
    runsTotal.value = result.total;
    return result;
  }

  async function addTask(payload: PlatformResourceInput) {
    const created = await createCronTask(payload);
    tasks.value.push(created);
    return created;
  }

  async function editTask(id: string, payload: Partial<PlatformResourceInput>) {
    const updated = await updateCronTask(id, payload);
    tasks.value = tasks.value.map((t) => (t.id === id ? updated : t));
    return updated;
  }

  async function removeTask(id: string) {
    await deleteCronTask(id);
    tasks.value = tasks.value.filter((t) => t.id !== id);
  }

  async function triggerTask(id: string) {
    return triggerCronTask(id);
  }

  /** 某个任务最近几次执行（详情栏用；与执行记录弹窗的分页 runs 互不干扰）。含已忽略的失败。 */
  const recentRuns = ref<CronTaskRun[]>([]);
  async function loadRecentRuns(taskId: string, size = 8) {
    const result = await listCronTaskRuns({ cron_task_id: taskId, page: 1, page_size: size, include_resolved: true });
    recentRuns.value = result.items;
    return result;
  }

  /** 运行台自动化；null = 接口不可用（不显示这一组）。 */
  const automations = ref<FleetAutomation[] | null>(null);
  const fleetProjects = ref<FleetProject[]>([]);
  async function loadAutomations() {
    const res = await listFleetAutomations();
    automations.value = res?.items ?? null;
    fleetProjects.value = res?.projects ?? [];
  }
  async function toggleAutomation(a: FleetAutomation, enabled: boolean) {
    const updated = await setFleetAutomationEnabled(a, enabled);
    automations.value = (automations.value ?? []).map((x) => (x.id === a.id ? updated : x));
    return updated;
  }
  async function runAutomation(id: string) {
    const updated = await runFleetAutomationNow(id);
    automations.value = (automations.value ?? []).map((x) => (x.id === id ? updated : x));
    return updated;
  }

  async function resetFailures(id: string) {
    const updated = await resetCronTaskFailures(id);
    tasks.value = tasks.value.map((t) => (t.id === id ? updated : t));
    return updated;
  }

  return {
    tasks,
    runs,
    runsTotal,
    agents,
    teams,
    loading,
    loadTasks,
    loadTargets,
    loadAll,
    loadRuns,
    addTask,
    editTask,
    removeTask,
    triggerTask,
    resetFailures,
    recentRuns,
    loadRecentRuns,
    automations,
    fleetProjects,
    loadAutomations,
    toggleAutomation,
    runAutomation,
  };
});
