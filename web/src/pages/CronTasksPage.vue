<template>
  <q-page class="app-page-cream cron-page q-pa-sm q-pa-md-md">
    <section class="cron-hero">
      <div>
        <div class="cron-kicker">Scheduled tasks</div>
        <h1 class="cron-title">定时任务</h1>
        <p class="cron-subtitle">安排定期 Agent 任务，查看最近执行、失败次数和下一次触发时间。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn color="orange" text-color="white" rounded unelevated icon="add" label="新建任务" @click="openCreate" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadAll" />
      </div>
    </section>

    <q-card flat bordered class="cron-toolbar q-mb-md">
      <q-card-section class="row q-col-gutter-md items-center">
        <q-input v-model="search" class="col-12 col-md-5" dense outlined rounded clearable debounce="300" placeholder="搜索定时任务...">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select v-model="statusFilter" class="col-12 col-md-3" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
        <div class="col text-caption text-grey-7">共 {{ filteredRows.length }} 个任务，{{ activeCount }} 个启用</div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll" />
      </template>
    </q-banner>

    <q-card flat bordered class="cron-table-card">
      <q-table
        v-if="filteredRows.length || loading"
        flat
        row-key="id"
        :rows="filteredRows"
        :columns="columns"
        :loading="loading"
        :pagination="{ rowsPerPage: 12 }"
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="text-weight-bold">{{ props.row.name }}</div>
            <div class="text-caption text-grey-7">{{ props.row.key }}</div>
          </q-td>
        </template>

        <template #body-cell-description="props">
          <q-td :props="props">
            <div class="ellipsis cron-description">{{ props.row.description || "—" }}</div>
            <q-tooltip v-if="props.row.description">{{ props.row.description }}</q-tooltip>
          </q-td>
        </template>

        <template #body-cell-schedule="props">
          <q-td :props="props">{{ scheduleLabel(props.row) }}</q-td>
        </template>

        <template #body-cell-agent="props">
          <q-td :props="props">{{ targetLabel(props.row) }}</q-td>
        </template>

        <template #body-cell-counts="props">
          <q-td :props="props">
            <div class="row q-gutter-xs items-center no-wrap">
              <q-badge color="grey-7">{{ metadata(props.row).run_count || 0 }} 次</q-badge>
              <q-badge color="positive">{{ metadata(props.row).success_count || 0 }} 成功</q-badge>
              <q-btn
                flat
                dense
                no-caps
                padding="2px 6px"
                :color="(metadata(props.row).failure_count || 0) > 0 ? 'negative' : 'grey-7'"
                :label="`${metadata(props.row).failure_count || 0} 失败`"
                @click="openRuns(props.row, 'failure')"
              >
                <q-tooltip max-width="320px">
                  <div v-if="recentFailures(props.row).length" class="q-gutter-xs">
                    <div v-for="(failure, index) in recentFailures(props.row)" :key="index">
                      {{ formatDate(failure.started_at) }} · {{ failure.error_message || "未知错误" }}
                    </div>
                  </div>
                  <span v-else>暂无失败记录</span>
                </q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>

        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense square :color="statusColor(props.row)" text-color="white">
              {{ props.row.enabled ? props.row.status || "active" : "paused" }}
            </q-chip>
          </q-td>
        </template>

        <template #body-cell-last="props">
          <q-td :props="props">{{ formatDate(metadata(props.row).last_run_at) }}</q-td>
        </template>

        <template #body-cell-next="props">
          <q-td :props="props">{{ formatDate(metadata(props.row).next_run_at) }}</q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :props="props" class="q-gutter-xs">
            <q-toggle :model-value="props.row.enabled" color="primary" dense :disable="savingId === props.row.id" @update:model-value="toggleRow(props.row, Boolean($event))" />
            <q-btn flat dense round icon="history" color="primary" @click="openRuns(props.row)">
              <q-tooltip>执行历史</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)">
              <q-tooltip>编辑</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="delete" color="negative" @click="confirmDelete(props.row)">
              <q-tooltip>删除</q-tooltip>
            </q-btn>
          </q-td>
        </template>
      </q-table>

      <q-card-section v-else class="cron-empty">
        <q-avatar size="80px" color="grey-9" text-color="grey-5">
          <q-icon name="schedule" size="46px" />
        </q-avatar>
        <div class="text-h6">暂无定时任务</div>
        <div class="text-body2 text-grey-7">创建定时任务以安排定期 Agent 任务。</div>
        <q-btn color="orange" text-color="white" rounded unelevated icon="add" label="新建任务" @click="openCreate" />
      </q-card-section>
    </q-card>

    <CronTaskFormDialog
      v-model="editorOpen"
      :row="editingRow"
      :agents="agents"
      :teams="teams"
      :submitting="formSubmitting"
      :server-error="formServerError"
      @submit="onFormSubmit"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useQuasar, type QTableColumn } from "quasar";
import type { Agent } from "../features/agents/api";
import type { Team } from "../features/teams/api";
import CronTaskFormDialog from "../components/cron/CronTaskFormDialog.vue";
import { createCronTask, deleteCronTask, listCronAgents, listCronTasks, listCronTeams, updateCronTask } from "../features/cron/api";
import type { PlatformResourceInput } from "../features/platform/api";
import type { CronFailureSummary, CronTaskConfig, CronTaskMetadata, CronTaskRow } from "../features/cron/types";

const $q = useQuasar();
const router = useRouter();
const rows = ref<CronTaskRow[]>([]);
const agents = ref<Agent[]>([]);
const teams = ref<Team[]>([]);
const loading = ref(false);
const error = ref("");
const search = ref("");
const statusFilter = ref("");
const editorOpen = ref(false);
const editingRow = ref<CronTaskRow | null>(null);
const savingId = ref("");
const formSubmitting = ref(false);
const formServerError = ref("");

const columns: QTableColumn<CronTaskRow>[] = [
  { name: "name", label: "名称", field: "name", align: "left", sortable: true },
  { name: "description", label: "描述", field: "description", align: "left" },
  { name: "schedule", label: "计划", field: "config_json", align: "left" },
  { name: "agent", label: "目标", field: "agent_id", align: "left" },
  { name: "counts", label: "执行统计", field: "metadata_json", align: "left" },
  { name: "status", label: "状态", field: "status", align: "left" },
  { name: "last", label: "上次运行", field: "metadata_json", align: "left" },
  { name: "next", label: "下次运行", field: "metadata_json", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];
const statusOptions = [
  { label: "启用", value: "active" },
  { label: "暂停", value: "paused" },
  { label: "失败", value: "failed" }
];
const activeCount = computed(() => rows.value.filter((row) => row.enabled).length);
const filteredRows = computed(() => {
  const keyword = search.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    const cfg = config(row);
    if (statusFilter.value === "active" && !row.enabled) return false;
    if (statusFilter.value === "paused" && row.enabled) return false;
    if (statusFilter.value === "failed" && row.status !== "failed") return false;
    if (!keyword) return true;
    return [row.key, row.name, row.description, cfg.schedule_type, cfg.cron_expression, cfg.message, targetLabel(row)]
      .some((value) => String(value || "").toLowerCase().includes(keyword));
  });
});

onMounted(loadAll);

watch(editorOpen, (open) => {
  if (open) formServerError.value = "";
});

async function onFormSubmit(payload: PlatformResourceInput) {
  formServerError.value = "";
  formSubmitting.value = true;
  try {
    const row = editingRow.value ? await updateCronTask(editingRow.value.id, payload) : await createCronTask(payload);
    onSaved(row);
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
    const [taskRows, agentRows, teamRows] = await Promise.all([listCronTasks(), listCronAgents(), listCronTeams()]);
    rows.value = taskRows;
    agents.value = agentRows;
    teams.value = teamRows;
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

function onSaved(row: CronTaskRow) {
  const index = rows.value.findIndex((item) => item.id === row.id);
  if (index >= 0) rows.value[index] = row;
  else rows.value.unshift(row);
}

async function toggleRow(row: CronTaskRow, enabled: boolean) {
  savingId.value = row.id;
  try {
    const updated = await updateCronTask(row.id, { ...row, enabled, status: enabled ? "active" : "paused" });
    onSaved(updated);
    $q.notify({ type: "positive", message: enabled ? "定时任务已启用" : "定时任务已暂停" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "启停失败" });
  } finally {
    savingId.value = "";
  }
}

function confirmDelete(row: CronTaskRow) {
  $q.dialog({
    title: "确认删除定时任务？",
    message: `删除「${row.name}」后将不再触发该任务。`,
    cancel: true,
    persistent: true
  }).onOk(async () => {
    await deleteCronTask(row.id);
    rows.value = rows.value.filter((item) => item.id !== row.id);
    $q.notify({ type: "positive", message: "定时任务已删除" });
  });
}

function openRuns(row: CronTaskRow, status = "") {
  void router.push({ name: "cron-runs", query: { cron_task_id: row.id, status } });
}

function scheduleLabel(row: CronTaskRow) {
  const cfg = config(row);
  if (cfg.schedule_type === "cron") return `cron: ${cfg.cron_expression || "-"}`;
  if (cfg.schedule_type === "once") return `一次 · ${formatDate(cfg.run_at)}`;
  return `每隔 ${Math.max(1, Math.round((cfg.interval_seconds || 0) / 60))} 分钟`;
}

function agentLabel(agentId: string) {
  if (!agentId) return "默认";
  const agent = agents.value.find((item) => item.id === agentId);
  return agent?.display_name || agent?.agent_key || agentId;
}

function teamLabel(teamId: string) {
  const team = teams.value.find((item) => item.id === teamId);
  return team?.display_name || team?.team_key || teamId || "未选择 Team";
}

function targetLabel(row: CronTaskRow) {
  const cfg = config(row);
  if (cfg.target_type === "team" || cfg.team_id) return `Team · ${teamLabel(cfg.team_id || "")}`;
  return `Agent · ${agentLabel(row.agent_id)}`;
}

function statusColor(row: CronTaskRow) {
  if (!row.enabled || row.status === "paused") return "grey";
  if (row.status === "failed") return "negative";
  return "positive";
}

function recentFailures(row: CronTaskRow): CronFailureSummary[] {
  const failures = metadata(row).recent_failures;
  return Array.isArray(failures) ? failures : [];
}

function config(row: CronTaskRow): CronTaskConfig {
  return parseJSON<CronTaskConfig>(row.config_json, {});
}

function metadata(row: CronTaskRow): CronTaskMetadata {
  return parseJSON<CronTaskMetadata>(row.metadata_json, {});
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}
</script>

<style scoped>
.cron-page {
  min-height: 100%;
}

.cron-hero {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 4px 20px;
}

.cron-kicker {
  color: #9a6a4f;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.cron-title {
  color: #4e342e;
  font-size: clamp(28px, 4vw, 44px);
  font-weight: 900;
  letter-spacing: -0.04em;
  line-height: 1.05;
  margin: 4px 0;
}

.cron-subtitle {
  color: #795548;
  margin: 0;
  max-width: 720px;
}

.cron-toolbar,
.cron-table-card {
  border-radius: 18px;
}

.cron-description {
  max-width: 240px;
}

.cron-empty {
  align-items: center;
  color: #8d6e63;
  display: grid;
  gap: 10px;
  justify-items: center;
  min-height: 280px;
}

body.body--dark .cron-title,
body.body--dark .cron-kicker,
body.body--dark .cron-subtitle {
  color: inherit;
}

@media (max-width: 720px) {
  .cron-hero {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
