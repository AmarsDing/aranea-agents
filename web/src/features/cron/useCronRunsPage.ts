import { computed, onMounted, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useRoute, useRouter } from "vue-router";

import type { CronTaskRow } from "./types";
import { CRON_RUN_TABLE_COLUMNS } from "./cronTableUi";
import { useCronStore } from "../../stores/cron";

const TRIGGER_LABELS: Record<string, string> = {
  schedule: "定时",
  manual: "手动",
  api: "API"
};

export function useCronRunsPage() {
  const route = useRoute();
  const router = useRouter();
  const cronStore = useCronStore();
  const { runs } = storeToRefs(cronStore);

  const tasks = ref<CronTaskRow[]>([]);
  const loading = ref(false);
  const error = ref("");
  const taskId = ref(String(route.query.cron_task_id || ""));
  const status = ref(String(route.query.status || ""));

  const page = ref(1);
  const pageSize = ref(15);

  const columns = CRON_RUN_TABLE_COLUMNS;
  const taskOptions = computed(() => tasks.value.map((task) => ({ label: task.name, value: task.id })));
  const statusOptions = [
    { label: "成功", value: "success" },
    { label: "失败", value: "failure" },
    { label: "跳过", value: "skipped" },
    { label: "待执行", value: "pending" }
  ];
  const pageMax = computed(() => Math.max(1, Math.ceil(runs.value.length / pageSize.value)));
  const pagedRuns = computed(() => {
    const start = (page.value - 1) * pageSize.value;
    return runs.value.slice(start, start + pageSize.value);
  });

  watch(runs, () => {
    page.value = 1;
  });

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
      await cronStore.loadRuns({
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
  }

  function resetFilters() {
    taskId.value = "";
    status.value = "";
    syncQueryAndLoad();
  }

  function taskLabel(id: string) {
    return tasks.value.find((task) => task.id === id)?.name || id || "—";
  }

  function runStatusLabel(value: string) {
    if (value === "success") return "成功";
    if (value === "failure") return "失败";
    if (value === "skipped") return "跳过";
    if (value === "pending") return "待执行";
    return value;
  }

  function runStatusColor(value: string) {
    if (value === "success") return "positive";
    if (value === "failure") return "negative";
    if (value === "skipped") return "grey";
    return "warning";
  }

  function triggerLabel(value: string) {
    return TRIGGER_LABELS[value] || value || "定时";
  }

  function triggerColor(value: string) {
    if (value === "manual") return "primary";
    if (value === "api") return "accent";
    return "grey-7";
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
    page,
    pageSize,
    pageMax,
    pagedRuns,
    loadRuns,
    syncQueryAndLoad,
    resetFilters,
    taskLabel,
    runStatusLabel,
    runStatusColor,
    triggerLabel,
    triggerColor,
    formatDate
  };
}
