<template>
  <q-page class="app-page-cream cron-runs-page q-pa-sm q-pa-md-md">
    <section class="cron-runs-hero">
      <div>
        <div class="cron-runs-kicker">Scheduled task runs</div>
        <h1 class="cron-runs-title">执行历史</h1>
        <p class="cron-runs-subtitle">查看定时任务触发记录、结果和失败摘要。</p>
      </div>
      <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRuns" />
    </section>

    <q-card flat bordered class="cron-runs-filter q-mb-md">
      <q-card-section class="row q-col-gutter-md items-center">
        <q-select
          v-model="taskId"
          class="col-12 col-md-5"
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
          class="col-12 col-md-3"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="结果"
          :options="statusOptions"
          @update:model-value="syncQueryAndLoad"
        />
        <div class="col row justify-end q-gutter-sm">
          <q-btn flat rounded icon="restart_alt" label="清空" @click="resetFilters" />
          <q-btn color="primary" rounded unelevated icon="manage_search" label="查询" :loading="loading" @click="syncQueryAndLoad" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRuns" />
      </template>
    </q-banner>

    <q-table
      flat
      bordered
      class="cron-runs-table"
      row-key="id"
      :rows="runs"
      :columns="columns"
      :loading="loading"
      :pagination="{ rowsPerPage: 15 }"
    >
      <template #body-cell-task="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.task_name || taskLabel(props.row.task_id) }}</div>
          <div class="text-caption text-grey-7">{{ props.row.task_id }}</div>
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
          <div class="ellipsis cron-run-error">{{ props.row.error_message || "—" }}</div>
          <q-tooltip v-if="props.row.error_message">{{ props.row.error_message }}</q-tooltip>
        </q-td>
      </template>
      <template #body-cell-run="props">
        <q-td :props="props">
          <q-btn v-if="props.row.run_id" flat dense round icon="open_in_new" color="primary">
            <q-tooltip>关联运行：{{ props.row.run_id }}</q-tooltip>
          </q-btn>
          <span v-else>—</span>
        </q-td>
      </template>
    </q-table>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { QTableColumn } from "quasar";
import { listCronTaskRuns, listCronTasks } from "../features/cron/api";
import type { CronTaskRow, CronTaskRun } from "../features/cron/types";

const route = useRoute();
const router = useRouter();
const tasks = ref<CronTaskRow[]>([]);
const runs = ref<CronTaskRun[]>([]);
const loading = ref(false);
const error = ref("");
const taskId = ref(String(route.query.cron_task_id || ""));
const status = ref(String(route.query.status || ""));

const columns: QTableColumn<CronTaskRun>[] = [
  { name: "task", label: "任务名称", field: "task_name", align: "left" },
  { name: "time", label: "时间", field: "started_at", align: "left" },
  { name: "status", label: "结果", field: "status", align: "left" },
  { name: "error", label: "错误摘要", field: "error_message", align: "left" },
  { name: "trigger", label: "触发", field: "trigger", align: "left" },
  { name: "run", label: "Agent 运行", field: "run_id", align: "right" }
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

<style scoped>
.cron-runs-page {
  min-height: 100%;
}

.cron-runs-hero {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 4px 20px;
}

.cron-runs-kicker {
  color: #9a6a4f;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.cron-runs-title {
  color: #4e342e;
  font-size: clamp(28px, 4vw, 44px);
  font-weight: 900;
  letter-spacing: -0.04em;
  line-height: 1.05;
  margin: 4px 0;
}

.cron-runs-subtitle {
  color: #795548;
  margin: 0;
  max-width: 720px;
}

.cron-runs-filter,
.cron-runs-table {
  border-radius: 18px;
}

.cron-run-error {
  max-width: 360px;
}

body.body--dark .cron-runs-title,
body.body--dark .cron-runs-kicker,
body.body--dark .cron-runs-subtitle {
  color: inherit;
}

@media (max-width: 720px) {
  .cron-runs-hero {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
