import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { QTableColumn } from "quasar";
import { registryCol } from "../ui/registryTableColumns";
import type { CronTaskRow, CronTaskRun } from "./types";
import { useCronStore } from "../../stores/cron";

export function useCronRunsPage() {
  const route = useRoute();
  const router = useRouter();
  const cronStore = useCronStore();

  const tasks = ref<CronTaskRow[]>([]);
  const runs = ref<CronTaskRun[]>([]);
  const loading = ref(false);
  const error = ref("");
  const taskId = ref(String(route.query.cron_task_id || ""));
  const status = ref(String(route.query.status || ""));

  const columns: QTableColumn<CronTaskRun>[] = [
    { name: "task", label: "任务名称", field: "task_name", align: "left", ...registryCol.name },
    { name: "time", label: "时间", field: "started_at", align: "left", ...registryCol.time },
    { name: "status", label: "结果", field: "status", align: "left", ...registryCol.status },
    { name: "error", label: "错误摘要", field: "error_message", align: "left", ...registryCol.error },
    { name: "trigger", label: "触发", field: "trigger", align: "left", ...registryCol.trigger },
    { name: "run", label: "Agent 运行", field: "run_id", align: "right", ...registryCol.actions }
  ];
  const taskOptions = computed(() => tasks.value.map((task) => ({ label: task.name, value: task.id })));
  const statusOptions = [
    { label: "成功", value: "success" },
    { label: "失败", value: "failure" },
    { label: "跳过", value: "skipped" },
    { label: "待执行", value: "pending" }
  ];

  onMounted(async () => {
    await loadTasks();
    await loadRuns();
  });

  watch(
    () => route.query,
    (query) => {
      taskId.value = String(query.cron_task_id || "");
      status.value = String(query.status || "");
      void loadRuns();
    }
  );

  async function loadTasks() {
    tasks.value = await cronStore.loadTasks();
  }

  async function loadRuns() {
    loading.value = true;
    error.value = "";
    try {
      runs.value = await cronStore.loadRuns({
        cron_task_id: taskId.value || undefined,
        status: status.value || undefined,
        limit: 200
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载执行历史失败";
    } finally {
      loading.value = false;
    }
  }

  function syncQueryAndLoad() {
    void router.replace({
      query: { cron_task_id: taskId.value || undefined, status: status.value || undefined }
    });
    void loadRuns();
  }

  function resetFilters() {
    taskId.value = "";
    status.value = "";
    syncQueryAndLoad();
  }

  function taskLabel(id: string) {
    return tasks.value.find((task) => task.id === id)?.name || id || "—";
  }

  function runStatusColor(value: string) {
    if (value === "success") return "positive";
    if (value === "failure") return "negative";
    if (value === "skipped") return "grey";
    return "warning";
  }

  function formatDate(value?: string) {
    if (!value) return "—";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  }

  return {
    tasks,
    runs,
    loading,
    error,
    taskId,
    status,
    columns,
    taskOptions,
    statusOptions,
    loadRuns,
    syncQueryAndLoad,
    resetFilters,
    taskLabel,
    runStatusColor,
    formatDate
  };
}
