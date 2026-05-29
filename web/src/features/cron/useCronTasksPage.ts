import { computed, onMounted, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useRouter } from "vue-router";
import { useQuasar } from "quasar";

import { parseCronConfig, parseCronMetadata } from "./api";
import type { PlatformResourceInput } from "../platform/types";
import type { CronFailureSummary, CronTaskConfig, CronTaskMetadata, CronTaskRow } from "./types";
import { CRON_TASK_TABLE_COLUMNS } from "./cronTableUi";
import { useCronStore } from "../../stores/cron";

export function useCronTasksPage() {
  const $q = useQuasar();
  const router = useRouter();
  const cronStore = useCronStore();
  const { tasks: rows, agents, teams } = storeToRefs(cronStore);

  const loading = ref(false);
  const error = ref("");
  const search = ref("");
  const statusFilter = ref("");
  const editorOpen = ref(false);
  const editingRow = ref<CronTaskRow | null>(null);
  const savingId = ref("");
  const triggeringId = ref("");
  const formSubmitting = ref(false);
  const formServerError = ref("");

  const columns = CRON_TASK_TABLE_COLUMNS;
  const statusOptions = [
    { label: "运行中", value: "active" },
    { label: "已暂停", value: "paused" },
    { label: "已失败", value: "dead" }
  ];
  const activeCount = computed(() => rows.value.filter((row) => row.enabled).length);
  const page = ref(1);
  const pageSize = ref(12);
  const filteredRows = computed(() => {
    const keyword = search.value.trim().toLowerCase();
    return rows.value.filter((row) => {
      const cfg = config(row);
      if (statusFilter.value === "active" && !(row.enabled && row.status !== "dead")) return false;
      if (statusFilter.value === "paused" && !(!row.enabled && row.status !== "dead")) return false;
      if (statusFilter.value === "dead" && row.status !== "dead") return false;
      if (!keyword) return true;
      return [row.key, row.name, row.description, cfg.schedule_type, cfg.cron_expression, cfg.message, targetLabel(row)]
        .some((value) => String(value || "").toLowerCase().includes(keyword));
    });
  });
  const pageMax = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / pageSize.value)));
  const pagedRows = computed(() => {
    const start = (page.value - 1) * pageSize.value;
    return filteredRows.value.slice(start, start + pageSize.value);
  });

  watch([search, statusFilter], () => {
    page.value = 1;
  });

  onMounted(() => void loadAll());

  watch(editorOpen, (open) => {
    if (open) formServerError.value = "";
  });

  async function onFormSubmit(payload: PlatformResourceInput) {
    formServerError.value = "";
    formSubmitting.value = true;
    try {
      if (editingRow.value) {
        await cronStore.editTask(editingRow.value.id, payload);
      } else {
        await cronStore.addTask(payload);
      }
      editorOpen.value = false;
      $q.notify({ type: "positive", message: "定时任务已保存" });
    } catch (err) {
      formServerError.value = err instanceof Error ? err.message : "保存失败";
      $q.notify({ type: "negative", message: formServerError.value });
    } finally {
      formSubmitting.value = false;
    }
  }

  async function loadAll() {
    loading.value = true;
    error.value = "";
    try {
      await cronStore.loadAll();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载定时任务失败";
    } finally {
      loading.value = false;
    }
  }

  function openCreate() {
    editingRow.value = null;
    editorOpen.value = true;
  }

  function openEdit(row: CronTaskRow) {
    editingRow.value = row;
    editorOpen.value = true;
  }

  async function toggleRow(row: CronTaskRow, enabled: boolean) {
    savingId.value = row.id;
    try {
      await cronStore.editTask(row.id, { enabled, status: enabled ? "active" : "paused" });
      $q.notify({ type: "positive", message: enabled ? "任务已启用" : "任务已暂停" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "更新失败" });
    } finally {
      savingId.value = "";
    }
  }

  function confirmDelete(row: CronTaskRow) {
    $q.dialog({
      title: "删除定时任务",
      message: `确定删除「${row.name}」？此操作不可撤销。`,
      cancel: true,
      persistent: true
    }).onOk(async () => {
      try {
        await cronStore.removeTask(row.id);
        $q.notify({ type: "positive", message: "任务已删除" });
      } catch (err) {
        $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
      }
    });
  }

  function openRuns(row: CronTaskRow, status = "") {
    void router.push({ name: "cron-runs", query: { cron_task_id: row.id, status } });
  }

  async function resetDeadTask(row: CronTaskRow) {
    savingId.value = row.id;
    try {
      await cronStore.resetFailures(row.id);
      $q.notify({ type: "positive", message: "失败计数已重置，任务恢复运行" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "重置失败" });
    } finally {
      savingId.value = "";
    }
  }

  async function runNow(row: CronTaskRow) {
    triggeringId.value = row.id;
    try {
      const run = await cronStore.triggerTask(row.id);
      if (run.status === "pending") {
        await loadAll();
        $q.notify({ type: "info", message: "任务已提交，请在执行历史中查看进度" });
        openRuns(row);
        return;
      }
      await loadAll();
      $q.notify({
        type: run.status === "success" ? "positive" : "negative",
        message: run.status === "success" ? "执行成功" : run.error_message || "执行失败"
      });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "触发失败" });
    } finally {
      triggeringId.value = "";
    }
  }

  function scheduleLabel(row: CronTaskRow) {
    const cfg = config(row);
    if (cfg.schedule_type === "cron") return `cron: ${cfg.cron_expression || "-"}`;
    if (cfg.schedule_type === "once") return `once @ ${formatDate(cfg.run_at)}`;
    return `每 ${Math.max(1, Math.round((cfg.interval_seconds || 0) / 60))} 分钟`;
  }

  function agentLabel(agentId: string) {
    if (!agentId) return "未指定";
    const agent = agents.value.find((item) => item.id === agentId);
    return agent?.display_name || agent?.agent_key || agentId;
  }

  function teamLabel(teamId: string) {
    const team = teams.value.find((item) => item.id === teamId);
    return team?.display_name || team?.team_key || teamId || "未知 Team";
  }

  function targetLabel(row: CronTaskRow) {
    const cfg = config(row);
    if (cfg.target_type === "team" || cfg.team_id) return `Team: ${teamLabel(cfg.team_id || "")}`;
    return `Agent: ${agentLabel(row.agent_id)}`;
  }

  function statusColor(row: CronTaskRow) {
    if (!row.enabled || row.status === "paused") return "grey";
    if (row.status === "dead") return "negative";
    return "positive";
  }

  function recentFailures(row: CronTaskRow): CronFailureSummary[] {
    const failures = metadata(row).recent_failures;
    return Array.isArray(failures) ? failures : [];
  }

  function config(row: CronTaskRow): CronTaskConfig {
    return parseCronConfig(row);
  }

  function metadata(row: CronTaskRow): CronTaskMetadata {
    return parseCronMetadata(row);
  }

  function formatDate(value?: string) {
    if (!value) return "-";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  }

  return {
    rows,
    agents,
    teams,
    loading,
    error,
    search,
    statusFilter,
    editorOpen,
    editingRow,
    savingId,
    triggeringId,
    formSubmitting,
    formServerError,
    columns,
    statusOptions,
    activeCount,
    filteredRows,
    pagedRows,
    page,
    pageSize,
    pageMax,
    loadAll,
    onFormSubmit,
    openCreate,
    openEdit,
    toggleRow,
    confirmDelete,
    openRuns,
    resetDeadTask,
    runNow,
    scheduleLabel,
    targetLabel,
    statusColor,
    recentFailures,
    metadata,
    formatDate
  };
}
