<template>
  <q-card flat bordered class="usage-panel">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">消耗趋势</div>
        <div class="text-caption text-grey-7">{{ hourly ? "按小时展示调用、Token 和费用变化" : "按天展示调用、Token 和费用变化" }}</div>
      </div>
      <q-chip dense color="blue-1" text-color="primary">{{ points.length }} {{ hourly ? "小时" : "天" }}</q-chip>
    </q-card-section>
    <q-separator />
    <q-card-section>
      <div v-if="!points.length" class="usage-empty">暂无趋势数据</div>
      <div v-else class="usage-trend-grid">
        <div v-for="point in points" :key="point.date_key" class="usage-trend-bar">
          <div class="usage-trend-bar__track">
            <div class="usage-trend-bar__fill" :style="{ height: `${barHeight(point.total_tokens)}%` }" />
          </div>
          <div class="text-caption text-grey-7 ellipsis">{{ point.date_key.slice(5) }}</div>
          <q-tooltip>
            调用 {{ formatCount(point.call_count) }}；Token {{ formatCount(point.total_tokens) }}；费用 {{ formatMoney(point.total_cost_micro_usd) }}
          </q-tooltip>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ModelUsageTrendPoint } from "../../features/usage/types";

const props = defineProps<{
  points: ModelUsageTrendPoint[];
  hourly?: boolean;
}>();

const maxTokens = computed(() => Math.max(1, ...props.points.map((point) => point.total_tokens)));

function barHeight(value: number) {
  return Math.max(8, Math.round((value / maxTokens.value) * 100));
}

function formatCount(value: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
}

function formatMoney(value: number) {
  return `$${(value / 1_000_000).toFixed(4)}`;
}
</script>

<style scoped>
.usage-panel {
  border-radius: 18px;
}

.usage-empty {
  min-height: 180px;
  display: grid;
  place-items: center;
  color: var(--color-text-gray-400);
}

.usage-trend-grid {
  align-items: end;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(28px, 1fr));
  gap: 10px;
  min-height: 220px;
}

.usage-trend-bar {
  align-items: center;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.usage-trend-bar__track {
  align-items: end;
  background: rgb(37 99 235 / 8%);
  border-radius: 999px;
  display: flex;
  height: 170px;
  overflow: hidden;
  width: 100%;
}

.usage-trend-bar__fill {
  background: linear-gradient(180deg, var(--color-info), var(--color-accent-blue));
  border-radius: 999px;
  width: 100%;
}
</style>
