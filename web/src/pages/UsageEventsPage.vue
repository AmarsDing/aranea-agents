<template>
  <q-page class="app-page-cream app-registry-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Token / Usage</div>
        <h1 class="app-page-title">用量事件明细</h1>
        <p class="app-page-subtitle">
          按时间查看 model_token_usage_events 原始记录（费用来自 model_pricing_rules 快照）。
        </p>
      </div>
      <div class="app-actions-bar">
        <q-btn outline rounded no-caps icon="download" label="导出 CSV" :loading="exporting" @click="onExportCsv" />
        <q-btn color="primary" unelevated rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="load" />
      </div>
    </section>

    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
        <q-select v-model="filters.range" dense outlined emit-value map-options label="范围" :options="rangeOptions" />
        <q-input v-model="filters.provider_code" dense outlined clearable label="Provider" />
        <q-input v-model="filters.model_api_id" dense outlined clearable label="模型" />
        <q-input v-model="filters.agent_id" dense outlined clearable label="Agent ID" />
        <q-input v-model="filters.team_id" dense outlined clearable label="Team ID" />
        <q-select v-model="filters.usage_kind" dense outlined clearable emit-value map-options label="来源" :options="usageKindOptions" />
        <q-select v-model="filters.status" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
        <div class="app-actions-bar app-actions-bar--start">
          <q-btn outline rounded no-caps label="重置" icon="restart_alt" @click="resetFilters" />
          <q-btn color="primary" unelevated rounded no-caps label="查询" icon="manage_search" :loading="loading" @click="load" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

    <div class="app-registry-table-shell">
    <q-table flat dense class="app-registry-table" :rows="events" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 20 }">
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
          <span class="app-registry-cell-desc">{{ truncate(props.row.error_message) }}</span>
        </q-td>
      </template>
    </q-table>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from "vue";
import { useUsageEventsPage } from "../features/usage/useUsageEventsPage";
import type { ModelTokenUsageEvent } from "../features/usage/types";
import { registryCol } from "../features/ui/registryTableColumns";

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
  resetFilters,
  formatMoney,
  truncate
} = useUsageEventsPage();

const columns = [
  { name: "occurred_at", label: "时间", field: "occurred_at", align: "left" as const, ...registryCol.time },
  { name: "usage_kind", label: "来源", field: "usage_kind", align: "left" as const, ...registryCol.chips },
  { name: "provider_code", label: "Provider", field: "provider_code", align: "left" as const, ...registryCol.chips },
  { name: "model_api_id", label: "模型", field: "model_api_id", align: "left" as const, ...registryCol.name },
  { name: "agent_id", label: "Agent", field: "agent_id", align: "left" as const, ...registryCol.agent },
  { name: "session_id", label: "Session", field: "session_id", align: "left" as const, ...registryCol.session },
  { name: "total_tokens", label: "Tokens", field: "total_tokens", align: "right" as const, ...registryCol.stats },
  { name: "total_cost_micro_usd", label: "费用", field: "total_cost_micro_usd", align: "right" as const, ...registryCol.size },
  { name: "latency_ms", label: "延迟(ms)", field: "latency_ms", align: "right" as const, ...registryCol.duration },
  { name: "status", label: "状态", field: "status", align: "left" as const, ...registryCol.status },
  { name: "error_message", label: "错误", field: "error_message", align: "left" as const, ...registryCol.error }
] satisfies { name: string; label: string; field: keyof ModelTokenUsageEvent; align: "left" | "right"; style?: string }[];

async function onExportCsv() {
  try {
    await exportCsv();
  } catch {
    // error surfaced via store eventsError
  }
}

onMounted(() => void load());
</script>
