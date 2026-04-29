<template>
  <q-page class="app-page-cream q-pa-md sessions-page">
    <section class="sessions-hero q-mb-md">
      <div>
        <div class="text-overline text-primary">Session history</div>
        <h1 class="sessions-title">会话历史</h1>
        <p class="text-grey-7 q-mt-xs q-mb-none">
          按 Agent、Team、状态与上下文消耗追踪任务运行实例。
        </p>
      </div>
      <q-btn flat round icon="refresh" :loading="loading" aria-label="刷新" @click="loadRows" />
    </section>

    <div class="row q-col-gutter-md q-mb-md">
      <div v-for="card in summaryCards" :key="card.label" class="col-12 col-sm-6 col-lg-3">
        <q-card flat bordered>
          <q-card-section>
            <div class="text-caption text-grey-7">{{ card.label }}</div>
            <div class="text-h5 text-weight-bold q-mt-xs">{{ card.value }}</div>
            <div class="text-caption text-grey-7 q-mt-xs">{{ card.hint }}</div>
          </q-card-section>
        </q-card>
      </div>
    </div>

    <q-card flat bordered class="q-mb-md session-filter-card">
      <q-card-section class="row q-col-gutter-sm items-center">
        <div class="col-12 col-md-4">
          <q-input v-model="keyword" dense outlined clearable debounce="350" placeholder="搜索标题、摘要或 Session ID">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="ownerType" dense outlined clearable emit-value map-options label="类型" :options="ownerOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="status" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="contextStatus" dense outlined clearable emit-value map-options label="上下文" :options="contextOptions" />
        </div>
        <div class="col-12 col-md-2 row justify-end q-gutter-sm">
          <q-btn flat rounded label="重置" icon="restart_alt" @click="resetFilters" />
          <q-btn unelevated rounded color="primary" label="查询" icon="manage_search" :loading="loading" @click="loadRows" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="selected" flat bordered class="q-mb-md session-detail-header">
      <q-card-section class="row items-start justify-between q-gutter-md">
        <div class="col">
          <div class="row items-center q-gutter-sm">
            <div class="text-h6">{{ selected.title }}</div>
            <q-chip dense :color="ownerColor(selected.owner_type)" text-color="white">{{ ownerLabel(selected.owner_type) }}</q-chip>
            <q-badge :color="statusColor(selected.status)">{{ selected.status }}</q-badge>
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">
            {{ selected.id }} · 创建 {{ formatDate(selected.created_at) }} · 最后活跃 {{ formatDate(selected.last_message_at || selected.updated_at) }}
          </div>
          <div v-if="selected.summary" class="text-body2 q-mt-sm">{{ selected.summary }}</div>
        </div>
        <div class="row q-gutter-sm">
          <q-btn outline rounded color="primary" icon="chat" label="继续会话" :to="{ name: 'chat' }" />
          <q-btn flat rounded icon="archive" label="归档" :disable="selected.status === 'archived'" @click="archiveSelected" />
        </div>
      </q-card-section>
      <q-separator />
      <q-card-section class="row q-col-gutter-md">
        <div class="col-12 col-md-4">
          <div class="text-caption text-grey-7">Context</div>
          <q-linear-progress rounded size="12px" :value="ratioValue(selected.context_used_ratio)" :color="contextColor(selected.context_status)" class="q-mt-sm" />
          <div class="text-caption q-mt-xs">
            当前 {{ formatPercent(selected.context_used_ratio) }} · 最高 {{ formatPercent(selected.max_context_used_ratio) }}
          </div>
        </div>
        <div class="col-6 col-md-2">
          <div class="text-caption text-grey-7">消息</div>
          <div class="text-h6">{{ selected.message_count }}</div>
        </div>
        <div class="col-6 col-md-2">
          <div class="text-caption text-grey-7">模型调用</div>
          <div class="text-h6">{{ selected.model_call_count }}</div>
        </div>
        <div class="col-6 col-md-2">
          <div class="text-caption text-grey-7">Token</div>
          <div class="text-h6">{{ formatNumber(selected.total_tokens) }}</div>
        </div>
        <div class="col-6 col-md-2">
          <div class="text-caption text-grey-7">费用</div>
          <div class="text-h6">{{ formatCost(selected.total_cost_micro_usd) }}</div>
        </div>
      </q-card-section>
    </q-card>

    <q-table
      flat
      bordered
      class="session-table"
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      :pagination="{ rowsPerPage: pageSize }"
      hide-pagination
    >
      <template #body-cell-session="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.title }}</div>
          <div class="text-caption text-grey-7 ellipsis">{{ props.row.summary || props.row.id }}</div>
        </q-td>
      </template>

      <template #body-cell-owner="props">
        <q-td :props="props">
          <q-chip dense :color="ownerColor(props.row.owner_type)" text-color="white">{{ ownerLabel(props.row.owner_type) }}</q-chip>
          <div class="text-caption text-grey-7">{{ props.row.owner_type === "team" ? props.row.team_id : props.row.agent_id }}</div>
        </q-td>
      </template>

      <template #body-cell-context="props">
        <q-td :props="props" style="min-width: 160px">
          <q-linear-progress rounded size="10px" :value="ratioValue(props.row.context_used_ratio)" :color="contextColor(props.row.context_status)" />
          <div class="text-caption q-mt-xs">{{ formatPercent(props.row.context_used_ratio) }} · {{ props.row.context_status }}</div>
        </q-td>
      </template>

      <template #body-cell-usage="props">
        <q-td :props="props">
          <div>{{ formatNumber(props.row.total_tokens) }} tokens</div>
          <div class="text-caption text-grey-7">
            {{ props.row.model_call_count }} model · {{ props.row.tool_call_count + props.row.skill_call_count + props.row.mcp_call_count }} calls
          </div>
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatDate(props.row.last_message_at || props.row.updated_at) }}</div>
          <div class="text-caption text-grey-7">创建 {{ formatDate(props.row.created_at) }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge :color="statusColor(props.row.status)">{{ props.row.status }}</q-badge>
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense round color="primary" icon="visibility" :to="{ name: 'session-detail', params: { sessionId: props.row.id } }">
            <q-tooltip>查看详情</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="archive" :disable="props.row.status === 'archived'" @click="archiveRow(props.row.id)">
            <q-tooltip>归档</q-tooltip>
          </q-btn>
        </q-td>
      </template>
    </q-table>

    <div class="row items-center justify-between q-mt-md">
      <div class="text-caption text-grey-7">共 {{ total }} 个 Session</div>
      <div class="row items-center q-gutter-sm">
        <q-select v-model="pageSize" dense outlined emit-value map-options :options="pageSizeOptions" style="width: 110px" />
        <q-pagination v-model="page" :max="pageMax" :max-pages="6" direction-links boundary-links color="primary" />
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { archiveSession, getSession, searchSessions, type Session } from "../api/client";

const route = useRoute();

const rows = ref<Session[]>([]);
const selected = ref<Session | null>(null);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const keyword = ref("");
const ownerType = ref<string | null>(null);
const status = ref<string | null>(null);
const contextStatus = ref<string | null>(null);
const page = ref(1);
const pageSize = ref(20);

const ownerOptions = [
  { label: "Agent", value: "agent" },
  { label: "Team", value: "team" }
];
const statusOptions = ["active", "running", "completed", "failed", "archived"].map((value) => ({ label: value, value }));
const contextOptions = ["normal", "warning", "critical", "exceeded"].map((value) => ({ label: value, value }));
const pageSizeOptions = [10, 20, 50].map((value) => ({ label: `${value} / 页`, value }));

const columns = [
  { name: "session", label: "会话", field: "title", align: "left", sortable: false },
  { name: "owner", label: "类型 / 归属", field: "owner_type", align: "left", sortable: false },
  { name: "context", label: "上下文", field: "context_used_ratio", align: "left", sortable: false },
  { name: "usage", label: "消耗", field: "total_tokens", align: "left", sortable: false },
  { name: "time", label: "时间", field: "last_message_at", align: "left", sortable: false },
  { name: "status", label: "状态", field: "status", align: "left", sortable: false },
  { name: "actions", label: "操作", field: "id", align: "right", sortable: false }
] as const;

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const summaryCards = computed(() => {
  const active = rows.value.filter((item) => item.status === "active" || item.status === "running").length;
  const avgContext = rows.value.length
    ? rows.value.reduce((sum, item) => sum + (item.context_used_ratio || 0), 0) / rows.value.length
    : 0;
  const tokens = rows.value.reduce((sum, item) => sum + (item.total_tokens || 0), 0);
  return [
    { label: "当前页会话", value: rows.value.length, hint: `总计 ${total.value}` },
    { label: "活跃 / 运行", value: active, hint: "当前页统计" },
    { label: "平均上下文", value: formatPercent(avgContext), hint: "当前页平均值" },
    { label: "Token", value: formatNumber(tokens), hint: "当前页累计" }
  ];
});

onMounted(loadRows);

watch([keyword, ownerType, status, contextStatus], () => {
  page.value = 1;
  loadRows();
});

watch([page, pageSize], loadRows);

watch(
  () => route.params.sessionId,
  () => loadSelected()
);

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const result = await searchSessions({
      keyword: keyword.value || undefined,
      owner_type: ownerType.value || undefined,
      status: status.value || undefined,
      context_status: contextStatus.value || undefined,
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value
    });
    rows.value = result.items;
    total.value = result.total;
    await loadSelected();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Session 失败";
  } finally {
    loading.value = false;
  }
}

async function loadSelected() {
  const id = String(route.params.sessionId || "");
  if (!id) {
    selected.value = null;
    return;
  }
  selected.value = rows.value.find((item) => item.id === id) ?? (await getSession(id));
}

function resetFilters() {
  keyword.value = "";
  ownerType.value = null;
  status.value = null;
  contextStatus.value = null;
  page.value = 1;
}

async function archiveRow(id: string) {
  await archiveSession(id);
  if (selected.value?.id === id) {
    selected.value = { ...selected.value, status: "archived" };
  }
  await loadRows();
}

async function archiveSelected() {
  if (!selected.value) return;
  await archiveRow(selected.value.id);
}

function ownerLabel(value: string) {
  return value === "team" ? "Team" : "Agent";
}

function ownerColor(value: string) {
  return value === "team" ? "deep-purple" : "primary";
}

function statusColor(value: string) {
  return value === "failed" ? "negative" : value === "archived" ? "grey" : value === "running" ? "primary" : "positive";
}

function contextColor(value: string) {
  return value === "exceeded" ? "purple" : value === "critical" ? "negative" : value === "warning" ? "warning" : "positive";
}

function ratioValue(value: number) {
  return Math.max(0, Math.min(1, value || 0));
}

function formatPercent(value: number) {
  return `${Math.round(ratioValue(value) * 100)}%`;
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value || 0);
}

function formatCost(value: number) {
  return `$${((value || 0) / 1_000_000).toFixed(4)}`;
}

function formatDate(value: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}
</script>

<style scoped>
.sessions-page {
  min-height: 100%;
}

.sessions-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.sessions-title {
  margin: 0;
  font-size: clamp(28px, 4vw, 42px);
  line-height: 1.1;
  font-weight: 800;
}

.session-detail-header {
  border-color: rgba(25, 118, 210, 0.25);
}

:global(body.body--dark) .sessions-page {
  background: linear-gradient(160deg, #0b1220 0%, #111827 48%, #0f172a 100%);
  color: #e5e7eb;
}

:global(body.body--dark) .sessions-page .q-card,
:global(body.body--dark) .sessions-page .session-table {
  background: rgba(17, 24, 39, 0.88) !important;
  border-color: rgba(148, 163, 184, 0.16);
  box-shadow: 0 12px 34px rgba(0, 0, 0, 0.32);
}

:global(body.body--dark) .sessions-page .q-table th {
  background: rgba(15, 23, 42, 0.86);
  color: #cbd5e1;
}

:global(body.body--dark) .sessions-page .q-table tbody tr:hover {
  background: rgba(51, 65, 85, 0.46);
}

:global(body.body--dark) .sessions-page .text-grey-7 {
  color: #94a3b8 !important;
}

:global(body.body--dark) .sessions-page .q-field__control {
  background: rgba(30, 41, 59, 0.72);
  border-color: rgba(148, 163, 184, 0.16);
}
</style>
