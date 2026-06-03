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
  type PlatformResource,
  type PlatformResourceInput,
} from '../../features/cron/api';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import type { CronTaskRun, CronTaskRunQuery } from '../../features/cron/types';

export const useCronStore = defineStore('cron', () => {
  const tasks = ref<PlatformResource[]>([]);
  const runs = ref<CronTaskRun[]>([]);
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
    runs.value = result ?? [];
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

  async function resetFailures(id: string) {
    const updated = await resetCronTaskFailures(id);
    tasks.value = tasks.value.map((t) => (t.id === id ? updated : t));
    return updated;
  }

  return {
    tasks,
    runs,
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
  };
});
