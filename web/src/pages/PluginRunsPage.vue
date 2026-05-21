<template>
  <q-page class="app-page-cream plugin-runs-page">
    <section class="plugin-runs-hero">
      <div>
        <div class="plugin-runs-kicker">Callback observability</div>
        <h1 class="plugin-runs-title">Callback / Plugin 运行记录</h1>
        <p class="plugin-runs-subtitle">按生命周期点（phase）、Agent、Plugin 与结果筛选；Hook 阻断/错误以 <code>hook:&lt;key&gt;</code> 落库。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn outline rounded color="primary" icon="rule" label="Hook 规则" to="/hooks" />
        <q-btn outline rounded color="primary" icon="arrow_back" label="Plugin 管理" to="/plugins" />
      </div>
    </section>

    <q-card flat bordered class="plugin-runs-filter q-mb-md">
      <q-card-section class="row q-col-gutter-sm items-center">
        <q-select
          v-model="pluginKey"
          class="col-12 col-md-3"
          dense
          outlined
          clearable
          use-input
          fill-input
          hide-selected
          input-debounce="0"
          label="Plugin Key"
          :options="pluginKeyOptions"
          @filter="filterPluginKeys"
          @update:model-value="onFilterChange"
        />
        <q-input
          v-model="agentId"
          class="col-12 col-md-3"
          dense
          outlined
          clearable
          debounce="350"
          label="Agent ID"
          @update:model-value="onFilterChange"
        />
        <q-select
          v-model="callbackPoint"
          class="col-12 col-md-2"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="生命周期点"
          :options="callbackPointOptions"
          @update:model-value="onFilterChange"
        />
        <q-select
          v-model="status"
          class="col-12 col-md-2"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="结果"
          :options="statusOptions"
          @update:model-value="onFilterChange"
        />
        <q-input v-model="from" class="col-12 col-md-3" dense outlined clearable type="datetime-local" label="起始时间" @update:model-value="onFilterChange" />
        <q-input v-model="to" class="col-12 col-md-3" dense outlined clearable type="datetime-local" label="结束时间" @update:model-value="onFilterChange" />
        <div class="col-12 col-md-6 row justify-end">
          <q-btn flat rounded icon="restart_alt" label="重置" @click="resetFilters" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="() => loadRows()" />
      </template>
    </q-banner>

    <q-card flat bordered class="plugin-runs-table-card">
      <q-table
        flat
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        v-model:pagination="tablePagination"
        :rows-per-page-options="[10, 20, 50]"
        @request="onTableRequest"
      >
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense :color="statusColor(props.row.status)" text-color="white">{{ props.row.status }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-detail_json="props">
          <q-td :props="props">
            <q-btn flat dense size="sm" label="详情" @click="openDetail(props.row)" />
          </q-td>
        </template>
      </q-table>
    </q-card>

    <q-dialog v-model="detailOpen">
      <q-card style="min-width: 420px; max-width: 90vw">
        <q-card-section class="text-h6">运行详情</q-card-section>
        <q-card-section>
          <pre class="plugin-run-detail">{{ detailText }}</pre>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import type { QTableColumn, QTableProps } from "quasar";
import {
  CALLBACK_POINT_OPTIONS,
  PLUGIN_RUN_KEY_PRESETS,
  PLUGIN_RUN_STATUS_OPTIONS,
  pluginRunsQueryFromRoute
} from "../features/callback/constants";
import { listPluginRuns } from "../features/plugins/api";
import type { PluginRun } from "../features/plugins/types";

const route = useRoute();

const pluginKey = ref("");
const agentId = ref("");
const callbackPoint = ref("");
const status = ref("");
const from = ref("");
const to = ref("");
const rows = ref<PluginRun[]>([]);
const loading = ref(false);
const error = ref("");
const tablePagination = ref({ page: 1, rowsPerPage: 20, rowsNumber: 0 });
const detailOpen = ref(false);
const detailText = ref("");

const callbackPointOptions = CALLBACK_POINT_OPTIONS;
const statusOptions = PLUGIN_RUN_STATUS_OPTIONS;
const pluginKeyOptions = ref([...PLUGIN_RUN_KEY_PRESETS.map((p) => p.value)]);

const columns: QTableColumn<PluginRun>[] = [
  { name: "created_at", label: "时间", field: "created_at", align: "left" },
  { name: "plugin_key", label: "Plugin / Hook", field: "plugin_key", align: "left" },
  { name: "agent_id", label: "Agent", field: "agent_id", align: "left" },
  { name: "callback_point", label: "生命周期点", field: "callback_point", align: "left" },
  { name: "status", label: "结果", field: "status", align: "left" },
  { name: "duration_ms", label: "耗时(ms)", field: "duration_ms", align: "right" },
  { name: "detail_json", label: "摘要", field: "detail_json", align: "left" }
];

function filterPluginKeys(val: string, update: (fn: () => void) => void) {
  update(() => {
    const needle = val.toLowerCase();
    pluginKeyOptions.value = PLUGIN_RUN_KEY_PRESETS.map((p) => p.value).filter((k) => k.toLowerCase().includes(needle));
  });
}

function statusColor(st: string) {
  if (st === "blocked") return "orange";
  if (st === "error") return "negative";
  return "positive";
}

function toRFC3339(local: string): string | undefined {
  const t = local.trim();
  if (!t) return undefined;
  const d = new Date(t);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toISOString();
}

async function loadRows(page = tablePagination.value.page, pageSize = tablePagination.value.rowsPerPage) {
  loading.value = true;
  error.value = "";
  try {
    const data = await listPluginRuns({
      plugin_key: pluginKey.value.trim() || undefined,
      agent_id: agentId.value.trim() || undefined,
      callback_point: callbackPoint.value || undefined,
      status: status.value || undefined,
      from: toRFC3339(from.value),
      to: toRFC3339(to.value),
      page,
      page_size: pageSize
    });
    rows.value = data.items;
    tablePagination.value = { ...tablePagination.value, page, rowsPerPage: pageSize, rowsNumber: data.total };
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载运行记录失败";
  } finally {
    loading.value = false;
  }
}

const onTableRequest: QTableProps["onRequest"] = (props) => {
  void loadRows(props.pagination.page, props.pagination.rowsPerPage);
};

function onFilterChange() {
  tablePagination.value.page = 1;
  void loadRows(1, tablePagination.value.rowsPerPage);
}

function resetFilters() {
  pluginKey.value = "";
  agentId.value = "";
  callbackPoint.value = "";
  status.value = "";
  from.value = "";
  to.value = "";
  tablePagination.value.page = 1;
  void loadRows(1, tablePagination.value.rowsPerPage);
}

function openDetail(row: PluginRun) {
  detailText.value = row.detail_json?.trim() ? row.detail_json : JSON.stringify(row, null, 2);
  detailOpen.value = true;
}

function applyRouteQuery() {
  const q = pluginRunsQueryFromRoute(route.query as Record<string, unknown>);
  if (q.plugin_key) pluginKey.value = q.plugin_key;
  if (q.agent_id) agentId.value = q.agent_id;
  if (q.callback_point) callbackPoint.value = q.callback_point;
  if (q.status) status.value = q.status;
}

onMounted(() => {
  applyRouteQuery();
  void loadRows();
});
</script>

<style scoped lang="sass">
.plugin-runs-page
  padding: 24px

.plugin-runs-hero
  display: flex
  align-items: flex-start
  justify-content: space-between
  gap: 16px
  margin-bottom: 18px
  flex-wrap: wrap

.plugin-runs-kicker
  color: var(--q-primary)
  font-size: 12px
  font-weight: 700
  letter-spacing: .12em
  text-transform: uppercase

.plugin-runs-title
  margin: 4px 0
  font-size: 34px

.plugin-runs-subtitle
  margin: 0
  color: var(--q-grey-7)

.plugin-runs-filter,
.plugin-runs-table-card
  border-radius: 22px

.plugin-run-detail
  margin: 0
  white-space: pre-wrap
  word-break: break-word
  font-size: 12px
</style>
