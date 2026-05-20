<template>
  <q-card flat bordered class="usage-panel">
    <q-card-section>
      <div class="text-h6">低性价比模型</div>
      <div class="text-caption text-grey-7">高费用且低 TPS 或成功率偏低的模型（任务 #24）</div>
    </q-card-section>
    <q-separator />
    <q-card-section>
      <div v-if="!rows.length" class="usage-empty">当前筛选范围内暂无标记</div>
      <q-list v-else dense>
        <q-item v-for="row in rows" :key="`${row.provider_code}:${row.model_api_id}`">
          <q-item-section>
            <q-item-label>{{ row.model_display_name || row.model_api_id }}</q-item-label>
            <q-item-label caption>{{ row.provider_code }} · {{ formatMoney(row.total_cost_micro_usd) }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row q-gutter-xs justify-end">
              <q-chip v-for="f in row.flags" :key="f" dense size="sm" color="orange-2" text-color="orange-10">{{ flagLabel(f) }}</q-chip>
            </div>
          </q-item-section>
        </q-item>
      </q-list>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { UsageModelInsight } from "../../features/usage/types";

defineProps<{
  rows: UsageModelInsight[];
}>();

function formatMoney(value: number) {
  return `$${(value / 1_000_000).toFixed(4)}`;
}

function flagLabel(f: string) {
  switch (f) {
    case "low_tps":
      return "低 TPS";
    case "high_failure":
      return "高失败率";
    case "high_cost":
      return "高费用";
    default:
      return f;
  }
}
</script>

<style scoped>
.usage-panel {
  border-radius: 18px;
}
.usage-empty {
  min-height: 80px;
  display: grid;
  place-items: center;
  color: var(--color-text-gray-400);
}
</style>
