// Container: approved — feature-local panel; data from Page composable via props.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col">
        <div class="text-h6">Cascade 审核</div>
        <div class="text-body2 text-grey-7">L4 实体更名冲突门控：批准后同步 L3 facts 与图谱邻居。</div>
      </div>
      <div class="col-auto">
        <q-btn flat icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>

    <q-card-section v-if="!agentId" class="text-grey-7">请先选择 Agent。</q-card-section>

    <q-card-section v-else-if="!rows.length && !loading">
      <q-banner rounded class="memory-info-banner">暂无待审核 Cascade 提议。</q-banner>
    </q-card-section>

    <q-card-section v-else class="q-pt-none">
      <AppRegistryTable
        :shell="false"
        :rows="rows"
        :columns="MEMORY_CASCADE_TABLE_COLUMNS"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-change="props">
          <q-td :props="props">
            <AppRegistryHoverTip :text="props.row.rationale">
              <div class="min-width-0">
                <div class="app-registry-cell-primary">{{ props.row.old_value }} → {{ props.row.new_value }}</div>
                <div class="app-registry-cell-sub ellipsis">{{ props.row.trigger_entity_name }}</div>
              </div>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-risk="props">
          <q-td :props="props">
            <q-badge :color="riskColor(props.row.risk_level)" :label="props.row.risk_level || 'unknown'" />
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn dense flat round color="positive" icon="check" :loading="actingId === props.row.id" @click="$emit('approve', props.row)">
                <q-tooltip>批准</q-tooltip>
              </q-btn>
              <q-btn dense flat round color="negative" icon="close" :loading="actingId === props.row.id" @click="$emit('reject', props.row)">
                <q-tooltip>拒绝</q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>
      </AppRegistryTable>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import AppRegistryTable from "../../components/layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../../components/layout/AppRegistryHoverTip.vue";

import type { CascadeProposal } from "./types";
import { MEMORY_CASCADE_TABLE_COLUMNS } from "./memoryTableUi";

defineProps<{
  agentId: string | null;
  rows: CascadeProposal[];
  loading: boolean;
  actingId: string | null;
}>();

defineEmits<{
  refresh: [];
  approve: [row: CascadeProposal];
  reject: [row: CascadeProposal];
}>();

function riskColor(level?: string) {
  switch ((level || "").toLowerCase()) {
    case "high":
      return "negative";
    case "medium":
      return "warning";
    default:
      return "grey-7";
  }
}
</script>
