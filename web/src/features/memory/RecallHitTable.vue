<template>
  <q-card flat bordered class="memory-card">
    <q-card-section>
      <div class="text-subtitle1">{{ title }}</div>
    </q-card-section>
    <q-markup-table v-if="rows.length" flat dense wrap-cells>
      <thead>
        <tr>
          <th>ID</th>
          <th>Text</th>
          <th class="text-right">Total</th>
          <th class="text-right">CE</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.id">
          <td class="text-caption">{{ row.id.slice(0, 8) }}…</td>
          <td class="ellipsis" style="max-width: 280px">{{ displayText(row) }}</td>
          <td class="text-right">{{ formatScore(row.scores.total) }}</td>
          <td class="text-right">{{ formatScore(row.scores.cross_encoder) }}</td>
        </tr>
      </tbody>
    </q-markup-table>
    <q-card-section v-else class="text-grey-7 text-caption">暂无结果 — 运行 Debug Recall 后展示。</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { MemoryRecallHit } from "./types";

defineProps<{
  title: string;
  color?: string;
  rows: MemoryRecallHit[];
}>();

function displayText(row: MemoryRecallHit) {
  return row.statement || row.title || row.summary || row.id;
}

function formatScore(v: number) {
  return (Number(v) || 0).toFixed(3);
}
</script>
