import { computed, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useQuasar, type QTableColumn } from "quasar";
import { registryCol } from "../ui/registryTableColumns";
import { parseCronConfig, parseCronMetadata } from "./api";
import type { PlatformResourceInput } from "../platform/types";
import type { CronFailureSummary, CronTaskConfig, CronTaskMetadata, CronTaskRow } from "./types";
import { useCronStore } from "../../stores/cron";

export function useCronTasksPage() {
  const $q = useQuasar();
  const router = useRouter();
  const cronStore = useCronStore();

  const rows = ref<CronTaskRow[]>([]);
  const agents = computed(() => cronStore.agents);
  const teams = computed(() => cronStore.teams);
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

  const columns: QTableColumn<CronTaskRow>[] = [
    { name: "name", label: "??", field: "name", align: "left", sortable: true, ...registryCol.name },
    { name: "description", label: "??", field: "description", align: "left", ...registryCol.desc },
    { name: "schedule", label: "??", field: "config_json", align: "left", ...registryCol.chips },
    { name: "agent", label: "??", field: "agent_id", align: "left", ...registryCol.agent },
    { name: "counts", label: "????", field: "metadata_json", align: "left", ...registryCol.stats },
    { name: "status", label: "??", field: "status", align: "left", ...registryCol.status },
    { name: "last", label: "????", field: "metadata_json", align: "left", ...registryCol.time },
    { name: "next", label: "????", field: "metadata_json", align: "left", ...registryCol.time },
    { name: "actions", label: "??", field: "id", align: "right", ...registryCol.actions }
  ];
  const statusOptions = [
    { label: "??", value: "active" },
    { label: "??", value: "paused" },
    { label: "??", value: "dead" }
  ];
  const activeCount = computed(() => rows.value.filter((row) => row.enabled).length);
  const filteredRows = computed(() => {
    const keyword = search.value.trim().toLowerCase();
    return rows.value.filter((row) => {
      const cfg = config(row);
      if (statusFilter.value === "active" && !row.enabled) return false;
      if (statusFilter.value === "paused" && row.enabled) return false;
      if (statusFilter.value === "dead" && row.status !== "dead") return false;
      if (!keyword) return true;
      return [row.key, row.name, row.description, cfg.schedule_type, cfg.cron_expression, cfg.message, targetLabel(row)]
        .some((value) => String(value || "").toLowerCase().includes(keyword));
    });
  });

  onMounted(() => void loadAll());

  watch(editorOpen, (open) => {
    if (open) formServerError.value = "";
  });

  async function onFormSubmit(payload: PlatformResourceInput) {
    formServerError.value = "";
    formSubmitting.value = true;
    try {
      const row = editingRow.value
        ? await cronStore.editTask(editingRow.value.id, payload)
        : await cronStore.addTask(payload);
      onSaved(row);
      editorOpen.value = false;
      $q.notify({ type: "positive", message: "???????" });
    } catch (err) {
      formServerError.value = err instanceof Error ? err.message : "????";
      $q.notify({ type: "negative", message: formServerError.value });
    } finally {
      formSubmitting.value = false;
    }
  }

  async function loadAll() {
    loading.value = true;
    error.value = "";
    try {
      const result = await cronStore.loadAll();
      rows.value = result.tasks;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "????????";
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

  function onSaved(row: CronTaskRow) {
    const index = rows.value.findIndex((item) => item.id === row.id);
    if (index >= 0) rows.value[index] = row;
    else rows.value.unshift(row);
  }

  async function toggleRow(row: CronTaskRow, enabled: boolean) {
    savingId.value = row.id;
    try {
      const updated = await cronStore.editTask(row.id, { ...row, enabled, status: enabled ? "active" : "paused" });
      onSaved(updated);
      $q.notify({ type: "positive", message: enabled ? "???????" : "???????" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    } finally {
      savingId.value = "";
    }
  }

  function confirmDelete(row: CronTaskRow) {
    $q.dialog({
      title: "????????",
      message: `???${row.name}???????????`,
      cancel: true,
      persistent: true
    }).onOk(async () => {
      await cronStore.removeTask(row.id);
      rows.value = rows.value.filter((item) => item.id !== row.id);
      $q.notify({ type: "positive", message: "???????" });
    });
  }

  function openRuns(row: CronTaskRow, status = "") {
    void router.push({ name: "cron-runs", query: { cron_task_id: row.id, status } });
  }

  async function resetDeadTask(row: CronTaskRow) {
    savingId.value = row.id;
    try {
      const updated = await cronStore.resetFailures(row.id);
      onSaved(updated);
      $q.notify({ type: "positive", message: "??????????????" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    } finally {
      savingId.value = "";
    }
  }

  async function runNow(row: CronTaskRow) {
    triggeringId.value = row.id;
    try {
      const run = await cronStore.triggerTask(row.id);
      if (run.status === "pending") {
        $q.notify({ type: "info", message: "?????????????????" });
        openRuns(row);
        return;
      }
      await loadAll();
      $q.notify({
        type: run.status === "success" ? "positive" : "negative",
        message: run.status === "success" ? "???????" : run.error_message || "??????"
      });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    } finally {
      triggeringId.value = "";
    }
  }

  function scheduleLabel(row: CronTaskRow) {
    const cfg = config(row);
    if (cfg.schedule_type === "cron") return `cron: ${cfg.cron_expression || "-"}`;
    if (cfg.schedule_type === "once") return `once @ ${formatDate(cfg.run_at)}`;
    return `?? ${Math.max(1, Math.round((cfg.interval_seconds || 0) / 60))} ??`;
  }

  function agentLabel(agentId: string) {
    if (!agentId) return "??";
    const agent = agents.value.find((item) => item.id === agentId);
    return agent?.display_name || agent?.agent_key || agentId;
  }

  function teamLabel(teamId: string) {
    const team = teams.value.find((item) => item.id === teamId);
    return team?.display_name || team?.team_key || teamId || "??? Team";
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
