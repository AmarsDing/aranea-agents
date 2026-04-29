<template>
  <q-item :class="['provider-row', { 'provider-row--dark': isDark }]">
    <q-item-section avatar top>
      <q-avatar color="primary" text-color="white" icon="memory" />
    </q-item-section>

    <q-item-section>
      <div class="provider-row-grid">
        <div class="provider-identity">
          <div class="row items-center no-wrap q-gutter-sm">
            <span class="status-dot" :class="{ 'status-dot--off': !row.enabled }" />
            <div class="provider-title ellipsis">{{ providerDisplayName }}</div>
          </div>
          <div class="model-name ellipsis">{{ modelDisplayName }}</div>
          <div class="row q-gutter-xs q-mt-xs">
            <q-chip dense square color="grey-2" text-color="grey-8">{{ row.provider || "未设置Provider" }}</q-chip>
            <q-chip v-if="config.provider_type" dense square color="blue-1" text-color="primary">
              {{ config.provider_type }}
            </q-chip>
          </div>
        </div>

        <div class="provider-types">
          <div class="field-label">模型类型</div>
          <div class="row q-gutter-xs">
            <q-chip
              v-for="category in categories"
              :key="category.value"
              dense
              color="indigo-1"
              text-color="indigo-8"
            >
              {{ category.label }}
              <q-tooltip>{{ category.tooltip }}</q-tooltip>
            </q-chip>
            <span v-if="!categories.length" class="muted-value">—</span>
          </div>
        </div>

        <div class="provider-metrics">
          <div>
            <div class="field-label">模型大小</div>
            <div class="metric-value">{{ config.model_size_label || "—" }}</div>
          </div>
          <div>
            <div class="field-label">上下文</div>
            <div class="metric-value">{{ formatContextWindow(config.context_window_k) }}</div>
          </div>
          <div>
            <div class="field-label">TPS</div>
            <div class="metric-value">{{ formatTps(config.tokens_per_second) }}</div>
          </div>
        </div>

        <div class="provider-usage">
          <div class="row items-center justify-between q-mb-xs">
            <div class="field-label q-mb-none">热度</div>
            <q-chip dense square :color="hotnessColor" text-color="white">
              {{ hotnessLabel }} {{ hotnessScore }}
            </q-chip>
          </div>
          <q-linear-progress rounded size="8px" :value="hotnessScore / 100" :color="hotnessColor">
            <q-tooltip>
              近30天调用：{{ formatCount(config.usage_call_count_30d) }}；
              Token：{{ formatCount(config.usage_total_tokens_30d) }}；
              费用：{{ formatMicroUsd(config.usage_cost_micro_usd_30d) }}；
              成功率：{{ formatPercent(config.success_rate_30d) }}
            </q-tooltip>
          </q-linear-progress>
          <div class="usage-mini-grid q-mt-sm">
            <div>
              <div class="field-label">30天调用</div>
              <div class="metric-value">{{ formatCount(config.usage_call_count_30d) }}</div>
            </div>
            <div>
              <div class="field-label">30天费用</div>
              <div class="metric-value">{{ formatMicroUsd(config.usage_cost_micro_usd_30d) }}</div>
            </div>
          </div>
        </div>

        <div class="provider-secret">
          <div class="field-label">API 密钥</div>
          <q-chip
            dense
            :color="hasApiKey ? 'green-1' : 'orange-1'"
            :text-color="hasApiKey ? 'green-8' : 'orange-9'"
            icon="key"
          >
            {{ hasApiKey ? "已设置API密钥" : "未设置" }}
          </q-chip>
        </div>

        <div class="provider-actions">
          <q-toggle
            :model-value="row.enabled"
            color="primary"
            dense
            :disable="saving"
            aria-label="启用模型"
            @update:model-value="$emit('toggle-enabled', row, Boolean($event))"
          />
          <q-btn flat dense round icon="query_stats" color="secondary" class="provider-action-btn" :aria-label="`查看 ${row.name} 趋势`" @click="$emit('trend', row)">
            <q-tooltip>历史趋势</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="edit" color="primary" class="provider-action-btn" :aria-label="`编辑 ${row.name}`" @click="$emit('edit', row)" />
          <q-btn flat dense round icon="delete" color="negative" class="provider-action-btn provider-action-btn--danger" :aria-label="`删除 ${row.name}`" @click="$emit('delete', row)" />
        </div>
      </div>
    </q-item-section>
  </q-item>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useQuasar } from "quasar";
import type { PlatformResource } from "../../features/platform/api";

type ModelCategory = {
  value: string;
  label: string;
  tooltip: string;
};

type ProviderConfig = {
  provider_type?: string;
  provider_display_name?: string;
  api_key?: string;
  api_key_set?: boolean;
  model_category?: ModelCategory[];
  model_size_label?: string;
  context_window_k?: number | string | null;
  max_output_tokens?: number | string | null;
  tokens_per_second?: number | string | null;
  model_hotness_score?: number | string | null;
  usage_call_count_30d?: number | string | null;
  usage_total_tokens_30d?: number | string | null;
  usage_cost_micro_usd_30d?: number | string | null;
  success_rate_30d?: number | string | null;
};

const props = defineProps<{
  row: PlatformResource;
  saving?: boolean;
}>();

defineEmits<{
  "toggle-enabled": [row: PlatformResource, enabled: boolean];
  trend: [row: PlatformResource];
  edit: [row: PlatformResource];
  delete: [row: PlatformResource];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
const config = computed(() => getConfig(props.row));
const categories = computed(() => {
  const values = config.value.model_category;
  return Array.isArray(values) ? values.filter((category) => category?.value && category?.label && category?.tooltip) : [];
});
const providerDisplayName = computed(() => config.value.provider_display_name || props.row.provider || props.row.key);
const modelDisplayName = computed(() => props.row.name || props.row.model || "未设置模型");
const hasApiKey = computed(() => Boolean(config.value.api_key_set || config.value.api_key));
const hotnessScore = computed(() => {
  const score = toNullableNumber(config.value.model_hotness_score);
  return score === null ? 0 : Math.max(0, Math.min(100, Math.round(score)));
});
const hotnessLabel = computed(() => {
  if (hotnessScore.value >= 80) return "热门";
  if (hotnessScore.value >= 50) return "活跃";
  if (hotnessScore.value >= 20) return "低频";
  return "冷门";
});
const hotnessColor = computed(() => {
  if (hotnessScore.value >= 80) return "deep-orange";
  if (hotnessScore.value >= 50) return "primary";
  if (hotnessScore.value >= 20) return "blue-grey";
  return "grey";
});

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

function toNullableNumber(value: unknown) {
  if (value === "" || value === null || value === undefined) return null;
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : null;
}
</script>

<style scoped>
.provider-row {
  padding: 18px 20px;
  transition: background-color 180ms ease, box-shadow 180ms ease;
}

.provider-row:hover {
  background: #f8faff;
}

.provider-row-grid {
  display: grid;
  grid-template-columns: minmax(220px, 1.7fr) minmax(150px, 1.1fr) minmax(210px, 1.35fr) minmax(210px, 1.35fr) minmax(130px, 0.85fr) 168px;
  gap: 18px;
  align-items: center;
  width: 100%;
}

.provider-identity,
.provider-types,
.provider-usage,
.provider-secret {
  min-width: 0;
}

.provider-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(56px, 1fr));
  gap: 12px;
  min-width: 0;
}

.provider-usage {
  padding: 10px 12px;
  border: 1px solid rgba(15, 23, 42, 0.06);
  border-radius: 14px;
  background: linear-gradient(180deg, #ffffff, #f8fafc);
}

.usage-mini-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.provider-title {
  color: #1f2937;
  font-size: 16px;
  font-weight: 700;
}

.model-name {
  color: #4b5563;
  font-size: 14px;
  margin-top: 4px;
}

.status-dot {
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: #16a34a;
  box-shadow: 0 0 0 4px rgba(22, 163, 74, 0.12);
  flex: 0 0 auto;
}

.status-dot--off {
  background: #9ca3af;
  box-shadow: 0 0 0 4px rgba(156, 163, 175, 0.14);
}

.field-label {
  color: #6b7280;
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 6px;
}

.metric-value {
  color: #111827;
  font-size: 14px;
  font-weight: 600;
}

.muted-value {
  color: #9ca3af;
}

.provider-actions {
  align-items: center;
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  min-width: 0;
}

.provider-actions :deep(.q-toggle) {
  padding-right: 10px;
  border-right: 1px solid rgba(15, 23, 42, 0.08);
}

.provider-action-btn {
  width: 34px;
  height: 34px;
  min-height: 34px;
  border: 1px solid rgba(25, 118, 210, 0.12);
  border-radius: 12px;
  background: linear-gradient(180deg, #f4f9ff, #edf5ff);
  box-shadow: 0 8px 18px rgba(25, 118, 210, 0.08);
  transition:
    transform 160ms ease,
    box-shadow 160ms ease,
    border-color 160ms ease;
}

.provider-action-btn--danger {
  border-color: rgba(220, 38, 38, 0.12);
  background: linear-gradient(180deg, #fff7f7, #fff1f2);
  box-shadow: 0 8px 18px rgba(220, 38, 38, 0.08);
}

.provider-action-btn:hover {
  transform: translateY(-1px);
  border-color: rgba(25, 118, 210, 0.28);
  box-shadow: 0 12px 24px rgba(25, 118, 210, 0.14);
}

.provider-action-btn--danger:hover {
  border-color: rgba(220, 38, 38, 0.28);
  box-shadow: 0 12px 24px rgba(220, 38, 38, 0.14);
}

.provider-row.provider-row--dark {
  color: #e5e7eb;
}

.provider-row.provider-row--dark:hover {
  background: rgba(51, 65, 85, 0.46);
}

.provider-row.provider-row--dark .provider-usage {
  border-color: rgba(148, 163, 184, 0.14);
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.78), rgba(15, 23, 42, 0.82));
}

.provider-row.provider-row--dark .provider-title,
.provider-row.provider-row--dark .metric-value {
  color: #f8fafc;
}

.provider-row.provider-row--dark .model-name,
.provider-row.provider-row--dark .field-label {
  color: #94a3b8;
}

.provider-row.provider-row--dark .muted-value {
  color: #64748b;
}

.provider-row.provider-row--dark .provider-actions :deep(.q-toggle) {
  border-right-color: rgba(148, 163, 184, 0.16);
}

.provider-row.provider-row--dark .provider-action-btn {
  border-color: rgba(96, 165, 250, 0.2);
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.82), rgba(15, 23, 42, 0.86));
  box-shadow: 0 8px 18px rgba(0, 0, 0, 0.25);
}

.provider-row.provider-row--dark .provider-action-btn--danger {
  border-color: rgba(248, 113, 113, 0.22);
  background: linear-gradient(180deg, rgba(127, 29, 29, 0.22), rgba(69, 10, 10, 0.18));
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-grey-2) {
  background: rgba(51, 65, 85, 0.76) !important;
  color: #cbd5e1 !important;
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-blue-1),
.provider-row.provider-row--dark :deep(.q-chip--colored.bg-indigo-1) {
  background: rgba(30, 64, 175, 0.26) !important;
  color: #bfdbfe !important;
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-green-1) {
  background: rgba(22, 101, 52, 0.26) !important;
  color: #86efac !important;
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-orange-1) {
  background: rgba(120, 53, 15, 0.28) !important;
  color: #fdba74 !important;
}

@media (max-width: 1023px) {
  .provider-row-grid {
    grid-template-columns: 1fr;
    gap: 14px;
  }

  .provider-metrics {
    grid-template-columns: repeat(3, minmax(72px, 1fr));
  }

  .provider-actions {
    justify-content: flex-start;
  }
}
</style>
