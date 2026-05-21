<template>
  <q-page class="app-page-cream q-pa-sm q-pa-md-md">
    <section class="usage-events-hero row items-center justify-between q-mb-md">
      <div>
        <div class="text-overline text-primary">Token / Usage</div>
        <h1 class="text-h5 text-weight-bold q-my-xs">用量事件明细</h1>
        <p class="text-caption text-grey-7 q-mb-none">
          按时间查看 model_token_usage_events 原始记录（费用来自 model_pricing_rules 快照）。
        </p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn outline rounded icon="download" label="导出 CSV" :loading="exporting" @click="onExportCsv" />
        <q-btn color="primary" unelevated rounded icon="refresh" label="刷新" :loading="loading" @click="load" />
      </div>
    </section>

    <q-card flat bordered class="q-mb-md">
      <q-card-section class="row q-col-gutter-md">
        <q-select v-model="filters.range" class="col-12 col-md-2" dense outlined emit-value map-options label="范围" :options="rangeOptions" />
        <q-input v-model="filters.provider_code" class="col-12 col-md-2" dense outlined clearable label="Provider" />
        <q-input v-model="filters.model_api_id" class="col-12 col-md-2" dense outlined clearable label="模型" />
        <q-input v-model="filters.agent_id" class="col-12 col-md-2" dense outlined clearable label="Agent ID" />
        <q-input v-model="filters.team_id" class="col-12 col-md-2" dense outlined clearable label="Team ID" />
        <q-select v-model="filters.usage_kind" class="col-12 col-md-2" dense outlined clearable emit-value map-options label="来源" :options="usageKindOptions" />
        <q-select v-model="filters.status" class="col-12 col-md-2" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
        <div class="col-12 col-md-2 flex items-center">
          <q-btn color="primary" unelevated label="查询" class="full-width" @click="load" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

    <q-table flat bordered :rows="events" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 20 }">
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-chip dense :color="props.row.status === 'success' || props.row.status === 'ok' ? 'positive' : 'negative'" text-color="white" size="sm">
            {{ props.row.status }}
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-total_cost_micro_usd="props">
        <q-td :props="props">{{ formatMoney(props.row.total_cost_micro_usd) }}</q-td>
      </template>
      <template #body-cell-error_message="props">
        <q-td :props="props">
          <span class="text-caption text-grey-8">{{ truncate(props.row.error_message) }}</span>
        </q-td>
      </template>
    </q-table>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from "vue";
import { useUsageEventsPage } from "../features/usage/useUsageEventsPage";
import type { ModelTokenUsageEvent } from "../features/usage/types";

const {
  events,
  loading,
  error,
  exporting,
  filters,
  rangeOptions,
  statusOptions,
  usageKindOptions,
  load,
  exportCsv,
  formatMoney,
  truncate
} = useUsageEventsPage();

const columns = [
  { name: "occurred_at", label: "时间", field: "occurred_at", align: "left" as const },
  { name: "usage_kind", label: "来源", field: "usage_kind", align: "left" as const },
  { name: "provider_code", label: "Provider", field: "provider_code", align: "left" as const },
  { name: "model_api_id", label: "模型", field: "model_api_id", align: "left" as const },
  { name: "agent_id", label: "Agent", field: "agent_id", align: "left" as const },
  { name: "session_id", label: "Session", field: "session_id", align: "left" as const },
  { name: "total_tokens", label: "Tokens", field: "total_tokens", align: "right" as const },
  { name: "total_cost_micro_usd", label: "费用", field: "total_cost_micro_usd", align: "right" as const },
  { name: "latency_ms", label: "延迟(ms)", field: "latency_ms", align: "right" as const },
  { name: "status", label: "状态", field: "status", align: "left" as const },
  { name: "error_message", label: "错误", field: "error_message", align: "left" as const }
] satisfies { name: string; label: string; field: keyof ModelTokenUsageEvent; align: "left" | "right" }[];

async function onExportCsv() {
  try {
    await exportCsv();
  } catch {
    // error surfaced via store eventsError
  }
}

onMounted(() => void load());
</script>
