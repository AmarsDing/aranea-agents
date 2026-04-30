<template>
  <q-card flat bordered class="usage-panel">
    <q-card-section>
      <div class="text-h6">Top Agent 成本</div>
      <div class="text-caption text-grey-7">按 Agent 汇总调用、费用和延迟</div>
    </q-card-section>
    <q-list separator>
      <q-item v-for="row in rows" :key="row.agent_id || row.agent_key">
        <q-item-section avatar>
          <q-avatar color="primary" text-color="white" icon="smart_toy" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="text-weight-medium">{{ row.agent_key || row.agent_id || "未归属 Agent" }}</q-item-label>
          <q-item-label caption>成功率 {{ formatPercent(row.success_rate) }} · 延迟 {{ formatLatency(row.avg_latency_ms) }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="text-right">
            <div class="text-weight-bold">{{ formatMoney(row.total_cost_micro_usd) }}</div>
            <div class="text-caption text-grey-7">{{ formatCount(row.call_count) }} 次</div>
          </div>
        </q-item-section>
      </q-item>
      <q-item v-if="!rows.length">
        <q-item-section class="text-grey-6">暂无 Agent 成本数据</q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import type { ModelUsageBreakdownRow } from "../../features/usage/types";

defineProps<{
  rows: ModelUsageBreakdownRow[];
}>();

function formatCount(value: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
}

function formatMoney(value: number) {
  return `$${(value / 1_000_000).toFixed(4)}`;
}

function formatPercent(value: number) {
  return `${Math.round(value * 100)}%`;
}

function formatLatency(value: number) {
  return `${Math.round(value)}ms`;
}
</script>

<style scoped>
.usage-panel {
  border-radius: 18px;
}
</style>
