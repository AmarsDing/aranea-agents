<template>
  <q-page class="app-page-cream tool-runs-page">
    <section class="tool-runs-hero">
      <div>
        <div class="tool-runs-kicker">Tool observability</div>
        <h1 class="tool-runs-title">Tool 调用记录</h1>
        <p class="tool-runs-subtitle">查看工具调用参数摘要、结果摘要、耗时、状态和错误信息。</p>
      </div>
      <q-btn outline rounded color="primary" icon="arrow_back" label="返回 Tools 管理" :to="{ name: 'tools' }" />
    </section>

    <q-card flat bordered class="tool-runs-filter q-mb-md">
      <q-card-section class="row q-col-gutter-sm items-center">
        <div class="col-12 col-md-3">
          <q-input v-model="toolKey" dense outlined clearable debounce="350" label="Tool Key" />
        </div>
        <div class="col-12 col-md-3">
          <q-input v-model="agentId" dense outlined clearable debounce="350" label="Agent ID" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="status" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-input v-model="from" dense outlined clearable label="开始时间 ISO" />
        </div>
        <div class="col-12 col-md-2 row justify-end q-gutter-sm">
          <q-btn flat rounded icon="restart_alt" label="重置" @click="resetFilters" />
          <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat bordered class="tool-runs-empty-card">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="history" />
        <div class="text-h6 q-mt-md">暂无 Tool 调用记录</div>
        <div class="text-body2 text-grey-7 q-mt-sm">接入 ADK Tool 执行审计后，这里会显示参数摘要、结果摘要和耗时。</div>
      </q-card-section>
    </q-card>

    <q-table v-else flat bordered class="tool-runs-table" row-key="id" :rows="rows" :columns="columns" :loading="loading" :pagination="tablePagination" hide-pagination>
      <template #body-cell-tool="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.tool_display_name || props.row.tool_key }}</div>
          <div class="text-caption text-grey-7">{{ props.row.tool_key }}</div>
        </q-td>
      </template>

      <template #body-cell-agent="props">
        <q-td :props="props">
          <div>{{ props.row.agent_display_name || props.row.agent_key || "-" }}</div>
          <div class="text-caption text-grey-7">{{ props.row.agent_id }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge rounded :color="statusColor(props.row.status)">{{ statusLabel(props.row.status) }}</q-badge>
        </q-td>
      </template>

      <template #body-cell-preview="props">
        <q-td :props="props">
          <div class="text-caption ellipsis">{{ props.row.input_preview || "无参数摘要" }}</div>
          <div class="text-caption text-grey-7 ellipsis">{{ props.row.output_preview || props.row.error_message || "无结果摘要" }}</div>
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatDate(props.row.started_at) }}</div>
          <div class="text-caption text-grey-7">{{ props.row.duration_ms }}ms</div>
        </q-td>
      </template>
    </q-table>

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="条调用记录" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import type { QTableColumn } from "quasar";
import SkillPagination from "../features/skills/components/SkillPagination.vue";
import { listToolRuns } from "../features/tools/api";
import type { ToolInvocation } from "../features/tools/types";

const toolKey = ref("");
const agentId = ref("");
const status = ref("");
const from = ref("");
const page = ref(1);
const pageSize = ref(20);
const rows = ref<ToolInvocation[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const route = useRoute();

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const statusOptions = [
  { label: "成功", value: "success" },
  { label: "错误", value: "error" },
  { label: "失败", value: "failed" },
  { label: "阻断", value: "blocked" },
  { label: "取消", value: "cancelled" }
];
const tablePagination = { rowsPerPage: 0 };

const columns: QTableColumn<ToolInvocation>[] = [
  { name: "tool", label: "Tool", field: "tool_key", align: "left" },
  { name: "agent", label: "Agent", field: "agent_id", align: "left" },
  { name: "status", label: "状态", field: "status", align: "left" },
  { name: "preview", label: "参数 / 结果摘要", field: "input_preview", align: "left", style: "max-width: 420px;" },
  { name: "session_id", label: "Session", field: "session_id", align: "left" },
  { name: "time", label: "时间 / 耗时", field: "started_at", align: "left" }
];

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listToolRuns({
      tool_key: toolKey.value,
      agent_id: agentId.value,
      status: status.value,
      from: from.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载调用记录失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  toolKey.value = "";
  agentId.value = "";
  status.value = "";
  from.value = "";
  page.value = 1;
  void loadRows();
}

function statusLabel(value: string) {
  return ({ success: "成功", error: "错误", failed: "失败", blocked: "阻断", cancelled: "取消" } as Record<string, string>)[value] ?? value;
}

function statusColor(value: string) {
  return ({ success: "positive", error: "negative", failed: "negative", blocked: "warning", cancelled: "grey" } as Record<string, string>)[value] ?? "grey";
}

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

watch([toolKey, agentId, status, from], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => {
  void loadRows();
});

onMounted(() => {
  if (typeof route.query.tool_key === "string") {
    toolKey.value = route.query.tool_key;
  }
  void loadRows();
});
</script>

<style scoped lang="sass">
.tool-runs-page
  padding: 24px

.tool-runs-hero
  display: flex
  justify-content: space-between
  gap: 16px
  align-items: flex-start
  margin-bottom: 18px

.tool-runs-kicker
  color: var(--q-primary)
  font-size: 12px
  font-weight: 700
  letter-spacing: .12em
  text-transform: uppercase

.tool-runs-title
  margin: 4px 0
  font-size: 34px
  line-height: 1.15

.tool-runs-subtitle
  margin: 0
  color: var(--q-grey-7)

.tool-runs-filter,
.tool-runs-table,
.tool-runs-empty-card
  border-radius: 22px
  overflow: hidden

@media (max-width: 720px)
  .tool-runs-hero
    flex-direction: column
    align-items: stretch
</style>
