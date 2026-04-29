<template>
  <div>
    <q-banner v-if="!factsEndpointReady" rounded class="memory-info-banner q-mb-md">
      L3 facts 暂时不可用。这里会读取 `/api/v1/memory/l3/facts`，请检查后端服务或筛选条件。
    </q-banner>
    <q-card flat bordered class="memory-card">
      <q-card-section class="row q-col-gutter-sm items-center">
        <div class="col-12 col-md-4">
          <q-input :model-value="factKeyword" dense outlined clearable debounce="300" label="搜索知识、偏好或规则" @update:model-value="$emit('update:factKeyword', String($event ?? ''))">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select :model-value="factScope" dense outlined clearable emit-value map-options label="Scope" :options="scopeOptions" @update:model-value="$emit('update:factScope', $event as string | null)" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select :model-value="factStatus" dense outlined clearable emit-value map-options label="状态" :options="factStatusOptions" @update:model-value="$emit('update:factStatus', $event as string | null)" />
        </div>
        <div class="col-12 col-md-4 row justify-end q-gutter-sm">
          <q-btn flat rounded icon="restart_alt" label="重置" @click="$emit('reset')" />
          <q-btn unelevated rounded color="primary" icon="manage_search" label="查询" :loading="loadingFacts" @click="$emit('search')" />
        </div>
      </q-card-section>
      <q-table
        flat
        row-key="id"
        :rows="factRows"
        :columns="factColumns"
        :loading="loadingFacts"
        :pagination="{ rowsPerPage: 12 }"
      >
        <template #body-cell-statement="props">
          <q-td :props="props">
            <div class="text-weight-medium">{{ props.row.statement }}</div>
            <div class="text-caption text-grey-7 ellipsis">{{ props.row.details_markdown || props.row.id }}</div>
          </q-td>
        </template>
        <template #body-cell-scope="props">
          <q-td :props="props">
            <q-chip dense square color="primary" text-color="white">{{ props.row.scope_type || "agent" }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-confidence="props">
          <q-td :props="props" style="min-width: 130px">
            <q-linear-progress rounded size="9px" :value="bounded(props.row.confidence)" :color="scoreColor(props.row.confidence)" />
            <div class="text-caption q-mt-xs">{{ formatPercent(props.row.confidence) }}</div>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <q-btn flat dense round icon="visibility" color="primary" aria-label="查看知识详情" @click="$emit('openFact', props.row)" />
          </q-td>
        </template>
        <template #no-data>
          <div class="full-width column items-center q-pa-xl text-grey-7">
            <q-icon name="psychology_alt" size="44px" />
            <div class="text-subtitle1 q-mt-sm">暂无长期知识</div>
            <div class="text-caption">用户确认的偏好、规则和经验会在 L3 facts 写入后出现在这里。</div>
          </div>
        </template>
      </q-table>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import type { QTableProps } from "quasar";
import type { MemoryFact } from "./api";

defineProps<{
  factsEndpointReady: boolean;
  factKeyword: string;
  factScope: string | null;
  factStatus: string | null;
  scopeOptions: Array<{ label: string; value: string }>;
  factStatusOptions: Array<{ label: string; value: string }>;
  factRows: MemoryFact[];
  factColumns: QTableProps["columns"];
  loadingFacts: boolean;
}>();

defineEmits<{
  "update:factKeyword": [value: string];
  "update:factScope": [value: string | null];
  "update:factStatus": [value: string | null];
  reset: [];
  search: [];
  openFact: [fact: MemoryFact];
}>();

function bounded(value?: number) {
  const numeric = Number(value) || 0;
  return Math.max(0, Math.min(1, numeric));
}

function scoreColor(value?: number) {
  const score = bounded(value);
  if (score >= 0.75) return "positive";
  if (score >= 0.45) return "warning";
  return "negative";
}

function formatPercent(value?: number) {
  return `${Math.round((Number(value) || 0) * 100)}%`;
}
</script>
