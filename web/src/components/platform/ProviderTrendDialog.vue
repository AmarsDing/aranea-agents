<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card :class="['trend-dialog', { 'trend-dialog--dark': isDark }]">
      <q-card-section class="row items-start justify-between q-col-gutter-md">
        <div>
          <div class="text-h6">模型历史趋势</div>
          <div class="text-caption text-grey-7">
            {{ row ? `${providerDisplayName} / ${modelDisplayName}` : "" }}
          </div>
        </div>
        <q-btn flat round dense icon="close" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section v-if="row" class="q-gutter-md">
        <q-inner-loading :showing="loading">
          <q-spinner color="primary" />
        </q-inner-loading>
        <div class="trend-summary-grid">
          <q-card flat bordered class="trend-summary-card">
            <div class="field-label">热度</div>
            <div class="trend-summary-value">{{ hotnessScore }}</div>
            <q-linear-progress rounded :value="hotnessScore / 100" :color="hotnessColor" />
          </q-card>
          <q-card flat bordered class="trend-summary-card">
            <div class="field-label">30天调用</div>
            <div class="trend-summary-value">{{ formatCount(overview?.range.call_count) }}</div>
          </q-card>
          <q-card flat bordered class="trend-summary-card">
            <div class="field-label">30天 Token</div>
            <div class="trend-summary-value">{{ formatCount(overview?.range.total_tokens) }}</div>
          </q-card>
          <q-card flat bordered class="trend-summary-card">
            <div class="field-label">30天费用</div>
            <div class="trend-summary-value">{{ formatMicroUsd(overview?.range.total_cost_micro_usd) }}</div>
          </q-card>
        </div>

        <div class="trend-bars">
          <div v-for="point in overview?.trends ?? []" :key="point.date_key" class="trend-bar">
            <div class="trend-bar__track">
              <div class="trend-bar__fill" :style="{ height: `${barHeight(point.total_tokens)}%` }" />
            </div>
            <div class="text-caption text-grey-7">{{ point.date_key.slice(5) }}</div>
            <q-tooltip>
              调用 {{ formatCount(point.call_count) }}；Token {{ formatCount(point.total_tokens) }}；费用 {{ formatMicroUsd(point.total_cost_micro_usd) }}
            </q-tooltip>
          </div>
          <div v-if="!overview?.trends.length" class="text-grey-6">暂无历史趋势数据</div>
        </div>

        <q-markup-table flat bordered dense>
          <tbody>
            <tr>
              <td>成功率</td>
              <td>{{ formatPercent(overview?.range.success_rate) }}</td>
              <td>平均延迟</td>
              <td>{{ formatLatency(overview?.range.avg_latency_ms) }}</td>
            </tr>
            <tr>
              <td>TPS</td>
              <td>{{ formatTps(overview?.range.avg_tokens_per_second ?? config.tokens_per_second) }}</td>
              <td>最近调用</td>
              <td>{{ latestUsedAt }}</td>
            </tr>
            <tr>
              <td>上下文</td>
              <td>{{ formatContextWindow(config.context_window_k) }}</td>
              <td>最大输出</td>
              <td>{{ formatCount(config.max_output_tokens) }}</td>
            </tr>
          </tbody>
        </q-markup-table>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useQuasar } from "quasar";
import type { ModelUsageOverview } from "../../features/usage/api";
import type { PlatformResource } from "../../features/platform/api";
import { useUsageStore } from "../../stores/usage/index";

const usageStore = useUsageStore();

type ProviderConfig = {
  provider_display_name?: string;
  context_window_k?: number | string | null;
  max_output_tokens?: number | string | null;
  tokens_per_second?: number | string | null;
  model_hotness_score?: number | string | null;
  usage_call_count_30d?: number | string | null;
  usage_total_tokens_30d?: number | string | null;
  usage_cost_micro_usd_30d?: number | string | null;
  success_rate_30d?: number | string | null;
  avg_latency_ms_30d?: number | string | null;
  last_used_at?: string;
};

const props = defineProps<{
  modelValue: boolean;
  row: PlatformResource | null;
}>();

defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
const config = computed(() => (props.row ? getConfig(props.row) : {}));
const overview = ref<ModelUsageOverview | null>(null);
const loading = ref(false);
const providerDisplayName = computed(() => (props.row ? config.value.provider_display_name || props.row.provider || props.row.key : ""));
const modelDisplayName = computed(() => (props.row ? props.row.name || props.row.model || "未设置模型" : ""));
const latestUsedAt = computed(() => {
  const latest = overview.value?.trends?.filter((point) => point.call_count > 0).at(-1);
  return latest?.date_key || config.value.last_used_at || "—";
});
const maxTrendTokens = computed(() => Math.max(1, ...(overview.value?.trends ?? []).map((point) => point.total_tokens)));
const hotnessScore = computed(() => {
  const score = toNullableNumber(config.value.model_hotness_score);
  return score === null ? 0 : Math.max(0, Math.min(100, Math.round(score)));
});
const hotnessColor = computed(() => {
  if (hotnessScore.value >= 80) return "deep-orange";
  if (hotnessScore.value >= 50) return "primary";
  if (hotnessScore.value >= 20) return "blue-grey";
  return "grey";
});

watch(
  () => [props.modelValue, props.row?.id],
  () => {
    if (props.modelValue && props.row) {
      void loadOverview();
    }
  },
  { immediate: true }
);

async function loadOverview() {
  if (!props.row) return;
  loading.value = true;
  try {
    overview.value = await usageStore.fetchOverview({
      range: "30d",
      provider_code: props.row.provider,
      model_api_id: props.row.model
    });
  } finally {
    loading.value = false;
  }
}

function barHeight(value: number) {
  return Math.max(8, Math.round((value / maxTrendTokens.value) * 100));
}

function getConfig(row: PlatformResource): ProviderConfig {
  if (!row.config_json) return {};
  try {
    const value = JSON.parse(row.config_json) as ProviderConfig;
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

function formatTps(value: ProviderConfig["tokens_per_second"]) {
  const numberValue = toNullableNumber(value);
  return numberValue === null ? "—" : `${numberValue} tokens/s`;
}

function formatContextWindow(value: ProviderConfig["context_window_k"]) {
  const numberValue = toNullableNumber(value);
  return numberValue === null ? "—" : `${numberValue}K`;
}

function formatCount(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(numberValue);
}

function formatMicroUsd(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `$${(numberValue / 1_000_000).toFixed(4)}`;
}

function formatPercent(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `${Math.round(numberValue * 100)}%`;
}

function formatLatency(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `${Math.round(numberValue)}ms`;
}

function toNullableNumber(value: unknown) {
  if (value === "" || value === null || value === undefined) return null;
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : null;
}
</script>

<style scoped>
.trend-dialog {
  width: 860px;
  max-width: 94vw;
  border-radius: 18px;
}

.trend-summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.trend-summary-card {
  padding: 14px;
  border-radius: 14px;
  background: linear-gradient(180deg, #ffffff, #f8fafc);
}

.trend-summary-value {
  color: #111827;
  font-size: 22px;
  font-weight: 750;
  margin: 4px 0 8px;
}

.field-label {
  color: #6b7280;
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 6px;
}

.trend-bars {
  align-items: end;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(26px, 1fr));
  gap: 8px;
  min-height: 150px;
}

.trend-bar {
  align-items: center;
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.trend-bar__track {
  align-items: end;
  background: rgba(37, 99, 235, 0.08);
  border-radius: 999px;
  display: flex;
  height: 112px;
  overflow: hidden;
  width: 100%;
}

.trend-bar__fill {
  background: linear-gradient(180deg, #93c5fd, #2563eb);
  border-radius: 999px;
  width: 100%;
}

.trend-dialog.trend-dialog--dark {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(17, 24, 39, 0.94);
  color: #e5e7eb;
}

.trend-dialog.trend-dialog--dark .trend-summary-card {
  border-color: rgba(148, 163, 184, 0.16);
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.78), rgba(15, 23, 42, 0.82));
}

.trend-dialog.trend-dialog--dark .trend-summary-value {
  color: #f8fafc;
}

.trend-dialog.trend-dialog--dark .field-label {
  color: #94a3b8;
}

.trend-dialog.trend-dialog--dark .trend-bar__track {
  background: rgba(51, 65, 85, 0.72);
}

.trend-dialog.trend-dialog--dark :deep(.q-markup-table) {
  background: rgba(15, 23, 42, 0.86);
  color: #e5e7eb;
}

.trend-dialog.trend-dialog--dark :deep(td) {
  border-color: rgba(148, 163, 184, 0.12);
}

@media (max-width: 1023px) {
  .trend-summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
