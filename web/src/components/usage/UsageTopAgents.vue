<template>
  <q-card flat class="overview-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">Top Agent 成本</div>
      <div class="text-caption overview-section-caption">按 Agent 汇总调用、费用和延迟</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-list separator class="overview-rank-list">
      <q-item v-for="row in rows" :key="row.agent_id || row.agent_key">
        <q-item-section avatar>
          <q-avatar class="overview-agent-avatar" icon="smart_toy" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="text-weight-medium">{{ row.agent_key || row.agent_id || "未归属 Agent" }}</q-item-label>
          <q-item-label caption class="overview-item-caption">
            成功率 {{ formatPercent(row.success_rate) }} · 延迟 {{ formatLatency(row.avg_latency_ms) }}
          </q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="text-right">
            <div class="overview-rank-value">{{ formatMoney(row.total_cost_micro_usd) }}</div>
            <div class="text-caption overview-item-caption">{{ formatCount(row.call_count) }} 次</div>
          </div>
        </q-item-section>
      </q-item>
      <q-item v-if="!rows.length">
        <q-item-section class="overview-empty-inline">暂无 Agent 成本数据</q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import type { ModelUsageBreakdownRow } from "../../features/usage/types";
import { formatUsdFromMicro } from "../../features/usage/moneyFormat";

defineProps<{
  rows: ModelUsageBreakdownRow[];
}>();

function formatCount(value: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
}

function formatMoney(value: number) {
  return formatUsdFromMicro(value);
}

function formatPercent(value: number) {
  return `${Math.round(value * 100)}%`;
}

function formatLatency(value: number) {
  return `${Math.round(value)} ms`;
}
</script>
