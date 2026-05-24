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
      <q-table
        flat
        bordered
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 10 }"
      >
        <template #body-cell-change="props">
          <q-td :props="props">
            <div class="text-weight-medium">{{ props.row.old_value }} → {{ props.row.new_value }}</div>
            <div class="text-caption text-grey-7">{{ props.row.trigger_entity_name }}</div>
          </q-td>
        </template>
        <template #body-cell-risk="props">
          <q-td :props="props">
            <q-badge :color="riskColor(props.row.risk_level)" :label="props.row.risk_level || 'unknown'" />
          </q-td>
        </template>
        <template #body-cell-affected="props">
          <q-td :props="props">
            {{ props.row.affected_entities?.length ?? 0 }}
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props" class="text-right">
            <q-btn dense flat color="positive" label="批准" :loading="actingId === props.row.id" @click="$emit('approve', props.row)" />
            <q-btn dense flat color="negative" label="拒绝" :loading="actingId === props.row.id" @click="$emit('reject', props.row)" />
          </q-td>
        </template>
      </q-table>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
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
  { name: "change", label: "更名", field: "old_value", align: "left" as const },
  { name: "risk", label: "风险", field: "risk_level", align: "left" as const },
  { name: "affected", label: "影响实体", field: "affected_entities", align: "left" as const },
  { name: "rationale", label: "原因", field: "rationale", align: "left" as const },
  { name: "created", label: "创建时间", field: "created_at", align: "left" as const },
  { name: "actions", label: "操作", field: "id", align: "right" as const }
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
