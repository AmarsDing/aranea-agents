<template>
  <q-page class="app-page-cream q-pa-sm q-pa-md-md">
    <section class="usage-events-hero row items-center justify-between q-mb-md">
      <div>
        <div class="text-overline text-primary">Token / Usage</div>
        <h1 class="text-h5 text-weight-bold q-my-xs">用量事件明细</h1>
        <p class="text-caption text-grey-7 q-mb-none">按时间查看 model_token_usage_events 原始记录。</p>
      </div>
      <q-btn color="primary" unelevated rounded icon="refresh" label="刷新" :loading="loading" @click="load" />
    </section>

    <q-card flat bordered class="q-mb-md">
      <q-card-section class="row q-col-gutter-md">
        <q-select v-model="filters.range" class="col-12 col-md-2" dense outlined emit-value map-options label="范围" :options="rangeOptions" />
        <q-input v-model="filters.provider_code" class="col-12 col-md-3" dense outlined clearable label="Provider" />
        <q-input v-model="filters.agent_id" class="col-12 col-md-3" dense outlined clearable label="Agent ID" />
        <q-select v-model="filters.status" class="col-12 col-md-2" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
        <div class="col-12 col-md-2 flex items-center">
          <q-btn color="primary" unelevated label="查询" class="full-width" @click="load" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

    <q-table flat bordered :rows="rows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 20 }">
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-chip dense :color="props.row.status === 'ok' ? 'positive' : 'negative'" text-color="white" size="sm">
            {{ props.row.status }}
          </q-chip>
        </q-td>
      </template>
    </q-table>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { listModelUsageEvents, type ModelTokenUsageEvent, type ModelUsageQuery } from "../features/usage/api";

const loading = ref(false);
const error = ref("");
const rows = ref<ModelTokenUsageEvent[]>([]);
const filters = ref<ModelUsageQuery>({ range: "7d", limit: 200 });

const rangeOptions = [
  { label: "24h", value: "24h" },
  { label: "7d", value: "7d" },
  { label: "30d", value: "30d" }
];
const statusOptions = [
  { label: "ok", value: "ok" },
  { label: "error", value: "error" }
];

const columns = [
  { name: "occurred_at", label: "时间", field: "occurred_at", align: "left" as const },
  { name: "provider_code", label: "Provider", field: "provider_code", align: "left" as const },
  { name: "model_api_id", label: "模型", field: "model_api_id", align: "left" as const },
  { name: "agent_id", label: "Agent", field: "agent_id", align: "left" as const },
  { name: "session_id", label: "Session", field: "session_id", align: "left" as const },
  { name: "total_tokens", label: "Tokens", field: "total_tokens", align: "right" as const },
  { name: "latency_ms", label: "延迟(ms)", field: "latency_ms", align: "right" as const },
  { name: "status", label: "状态", field: "status", align: "left" as const }
];

async function load() {
  loading.value = true;
  error.value = "";
  try {
    rows.value = await listModelUsageEvents(filters.value);
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(() => void load());
</script>
