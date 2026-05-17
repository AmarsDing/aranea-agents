import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listCronTasks,
  createCronTask,
  updateCronTask,
  deleteCronTask,
  listCronTaskRuns,
  type PlatformResource,
  type PlatformResourceInput
} from "../../features/cron/api";
import type { CronTaskRun } from "../../features/cron/types";

export const useCronStore = defineStore("cron", () => {
  const tasks = ref<PlatformResource[]>([]);
  const runs = ref<CronTaskRun[]>([]);
  const loading = ref(false);

  async function loadTasks() {
    loading.value = true;
    try {
      tasks.value = await listCronTasks();
    } finally {
      loading.value = false;
    }
  }

  async function loadRuns(query?: Parameters<typeof listCronTaskRuns>[0]) {
    const result = await listCronTaskRuns(query);
    runs.value = result.items ?? [];
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

  return { tasks, runs, loading, loadTasks, loadRuns, addTask, editTask, removeTask };
});
