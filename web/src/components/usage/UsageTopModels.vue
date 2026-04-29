<template>
  <q-card flat bordered class="usage-panel">
    <q-card-section>
      <div class="text-h6">Top 模型消耗</div>
      <div class="text-caption text-grey-7">按费用和调用量排序</div>
    </q-card-section>
    <q-list separator>
      <q-item v-for="row in rows" :key="`${row.provider_code}:${row.model_api_id}`">
        <q-item-section>
          <q-item-label class="text-weight-medium">{{ row.model_display_name || row.model_api_id }}</q-item-label>
          <q-item-label caption>{{ row.provider_code }} / {{ row.model_api_id }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="text-right">
            <div class="text-weight-bold">{{ formatMoney(row.total_cost_micro_usd) }}</div>
            <div class="text-caption text-grey-7">{{ formatCount(row.call_count) }} 次 · {{ formatTps(row.avg_tokens_per_second) }}</div>
          </div>
        </q-item-section>
      </q-item>
      <q-item v-if="!rows.length">
        <q-item-section class="text-grey-6">暂无模型用量</q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import type { ModelUsageBreakdownRow } from "../../../features/usage/types";

defineProps<{
  rows: ModelUsageBreakdownRow[];
}>();

function formatCount(value: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
}

function formatMoney(value: number) {
  return `$${(value / 1_000_000).toFixed(4)}`;
}

function formatTps(value: number) {
  return `${value.toFixed(1)} TPS`;
}
</script>

<style scoped>
.usage-panel {
  border-radius: 18px;
}
</style>
