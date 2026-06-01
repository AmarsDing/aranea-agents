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
import AppRegistryHoverTip from "../layout/AppRegistryHoverTip.vue";
import AppRegistryMarkupTable from "../layout/AppRegistryMarkupTable.vue";
import { REGISTRY_COL_W, registryCol, type RegistryTableColumn } from "../../features/ui/registryTableColumns";
import type { MemoryRecallHit } from "../../features/memory/types";

const props = defineProps<{
  title: string;
  color?: string;
  rows: MemoryRecallHit[];
}>();

const rowRecords = computed(() => props.rows as unknown as Record<string, unknown>[]);

const columns: RegistryTableColumn<MemoryRecallHit>[] = [
  registryCol("id", "ID", "id", "left", REGISTRY_COL_W.nameWide),
  registryCol("total", "Total", (row) => row.scores.total, "right", REGISTRY_COL_W.metric),
  registryCol("cross_encoder", "CE", (row) => row.scores.cross_encoder, "right", REGISTRY_COL_W.metric)
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
