<template>
  <q-card flat bordered class="memory-card">
    <q-card-section>
      <div class="text-subtitle1">{{ title }}</div>
    </q-card-section>
    <AppRegistryMarkupTable
      v-if="rows.length"
      :rows="rowRecords"
      :columns="columns"
      row-key="id"
    >
      <template #cell-id="{ row }">
        <span class="text-caption">
          <AppRegistryHoverTip :text="displayText(hitRow(row))" empty-label="暂无文本">
            <span>{{ hitRow(row).id.slice(0, 8) }}…</span>
          </AppRegistryHoverTip>
        </span>
      </template>
      <template #cell-total="{ row }">
        <span class="text-right">{{ formatScore(hitRow(row).scores.total) }}</span>
      </template>
      <template #cell-cross_encoder="{ row }">
        <span class="text-right">{{ formatScore(hitRow(row).scores.cross_encoder) }}</span>
      </template>
    </AppRegistryMarkupTable>
    <q-card-section v-else class="text-grey-7 text-caption">暂无结果 — 运行 Debug Recall 后展示。</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import AppRegistryHoverTip from "../../components/layout/AppRegistryHoverTip.vue";
import AppRegistryMarkupTable from "../../components/layout/AppRegistryMarkupTable.vue";
import { REGISTRY_COL_W, registryColWidth, type RegistryTableColumn } from "../ui/registryTableColumns";
import type { MemoryRecallHit } from "./types";

const props = defineProps<{
  title: string;
  color?: string;
  rows: MemoryRecallHit[];
}>();

const rowRecords = computed(() => props.rows as unknown as Record<string, unknown>[]);

const columns: RegistryTableColumn<MemoryRecallHit>[] = [
  { name: "id", label: "ID", field: "id", align: "left", ...registryColWidth(REGISTRY_COL_W.nameWide) },
  { name: "total", label: "Total", field: (row) => row.scores.total, align: "right", ...registryColWidth(REGISTRY_COL_W.metric) },
  { name: "cross_encoder", label: "CE", field: (row) => row.scores.cross_encoder, align: "right", ...registryColWidth(REGISTRY_COL_W.metric) }
];

function hitRow(row: Record<string, unknown>) {
  return row as unknown as MemoryRecallHit;
}

function displayText(row: MemoryRecallHit) {
  return row.statement || row.title || row.summary || row.id;
}

function formatScore(v: number) {
  return (Number(v) || 0).toFixed(3);
}
</script>
