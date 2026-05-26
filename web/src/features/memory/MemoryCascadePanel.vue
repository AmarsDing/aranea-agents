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
        :columns="columns"
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
import { registryColWidth } from "../ui/registryTableColumns";
import type { CascadeProposal } from "./types";

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

const columns = [
  { name: "change", label: "更名", field: "old_value", align: "left" as const, ...registryColWidth("14%") },
  { name: "risk", label: "风险", field: "risk_level", align: "left" as const, ...registryColWidth("9%") },
  { name: "affected", label: "影响实体", field: "affected_entities", align: "center" as const, ...registryColWidth("72px") },
  { name: "created", label: "创建时间", field: "created_at", align: "left" as const, ...registryColWidth("11%") },
  { name: "actions", label: "操作", field: "id", align: "right" as const, ...registryColWidth("108px") }
];

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
