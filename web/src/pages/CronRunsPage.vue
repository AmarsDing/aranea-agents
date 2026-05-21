<template>
  <q-page class="app-page-cream app-registry-page cron-runs-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Scheduled task runs</div>
        <h1 class="app-page-title">执行历史</h1>
        <p class="app-page-subtitle">查看定时任务触发记录、结果和失败摘要。</p>
      </div>
      <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRuns" />
    </section>

    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
        <q-select
          v-model="taskId"
          class="app-field-md"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="定时任务"
          :options="taskOptions"
          @update:model-value="syncQueryAndLoad"
        />
        <q-select
          v-model="status"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="结果"
          :options="statusOptions"
          @update:model-value="syncQueryAndLoad"
        />
        <div class="app-actions-bar app-actions-bar--start">
          <q-btn flat rounded no-caps icon="restart_alt" label="清空" @click="resetFilters" />
          <q-btn color="primary" rounded unelevated no-caps icon="manage_search" label="查询" :loading="loading" @click="syncQueryAndLoad" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRuns" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
    <q-table
      flat
      dense
      class="app-registry-table"
      row-key="id"
      :rows="runs"
      :columns="columns"
      :loading="loading"
      :pagination="{ rowsPerPage: 15 }"
    >
      <template #body-cell-task="props">
        <q-td :props="props">
          <div class="app-registry-cell-primary">{{ props.row.task_name || taskLabel(props.row.task_id) }}</div>
          <div class="app-registry-cell-sub">{{ props.row.task_id }}</div>
        </q-td>
      </template>
      <template #body-cell-time="props">
        <q-td :props="props">
          <div>开始 {{ formatDate(props.row.started_at || props.row.created_at) }}</div>
          <div class="text-caption text-grey-7">结束 {{ formatDate(props.row.finished_at) }}</div>
        </q-td>
      </template>
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge :color="runStatusColor(props.row.status)">{{ props.row.status }}</q-badge>
        </q-td>
      </template>
      <template #body-cell-error="props">
        <q-td :props="props">
          <div class="app-registry-cell-desc" :title="props.row.error_message || ''">{{ props.row.error_message || "—" }}</div>
          <q-tooltip v-if="props.row.error_message">{{ props.row.error_message }}</q-tooltip>
        </q-td>
      </template>
      <template #body-cell-run="props">
        <q-td :props="props">
          <div class="app-registry-cell-actions">
          <q-btn v-if="props.row.run_id" flat dense round icon="open_in_new" color="primary">
            <q-tooltip>关联运行：{{ props.row.run_id }}</q-tooltip>
          </q-btn>
          <span v-else>—</span>
          </div>
        </q-td>
      </template>
    </q-table>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { QTableColumn } from "quasar";
import { listCronTaskRuns, listCronTasks } from "../features/cron/api";
import type { CronTaskRow, CronTaskRun } from "../features/cron/types";
import { registryCol } from "../features/ui/registryTableColumns";

const route = useRoute();
const router = useRouter();
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
  tasks.value = await listCronTasks();
}

async function loadRuns() {
  loading.value = true;
  error.value = "";
  try {
    runs.value = await listCronTaskRuns({ cron_task_id: taskId.value, status: status.value, limit: 200 });
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载执行历史失败";
  } finally {
    loading.value = false;
  }
}

function syncQueryAndLoad() {
  void router.replace({ query: { cron_task_id: taskId.value || undefined, status: status.value || undefined } });
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
  if (value === "skipped") return "warning";
  return "grey";
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
</script>

