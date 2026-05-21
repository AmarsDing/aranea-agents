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
            <q-chip
              v-if="showVariantChip"
              dense
              square
              color="teal-1"
              text-color="teal-9"
            >
              {{ config.variant }}
            </q-chip>
            <q-chip
              v-if="haChipLabel"
              dense
              square
              :color="haChipColor"
              :text-color="haChipTextColor"
            >
              {{ haChipLabel }}
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
          <div v-if="hasApiKey" class="row items-center no-wrap q-gutter-xs">
            <span class="masked-secret">{{ listKeyVisible ? listRevealedApiKey : "••••••••••••" }}</span>
            <q-btn
              flat
              dense
              round
              size="sm"
              :icon="listKeyVisible ? 'visibility_off' : 'visibility'"
              :loading="listKeyRevealing"
              :aria-label="listKeyVisible ? '隐藏 API 密钥' : '查看 API 密钥'"
              @click="toggleListApiKeyVisibility"
            />
          </div>
          <q-chip
            v-else
            dense
            color="orange-1"
            text-color="orange-9"
            icon="key"
          >
            未设置
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
import { computed, ref } from "vue";
import { useQuasar } from "quasar";
import { revealProviderModelCredentials, type PlatformResource } from "../../features/platform/api";

type ModelCategory = {
  value: string;
  label: string;
  tooltip: string;
};

type ProviderConfig = {
  provider_type?: string;
  variant?: string;
  ha_mode?: string;
  provider_display_name?: string;
  secret_id?: string;
  aws_region?: string;
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
const hasApiKey = computed(() =>
  Boolean(config.value.api_key_set || config.value.api_key || config.value.secret_id || config.value.aws_region)
);
const listKeyVisible = ref(false);
const listRevealedApiKey = ref("");
const listKeyRevealing = ref(false);

async function toggleListApiKeyVisibility() {
  if (listKeyVisible.value) {
    listKeyVisible.value = false;
    listRevealedApiKey.value = "";
    return;
  }
  listKeyRevealing.value = true;
  try {
    const creds = await revealProviderModelCredentials(props.row.id);
    if (creds.api_key) {
      listRevealedApiKey.value = creds.api_key;
    } else if (creds.secret_key) {
      listRevealedApiKey.value = creds.secret_key;
    } else {
      listRevealedApiKey.value = "(已配置)";
    }
    listKeyVisible.value = true;
  } catch (error) {
    const msg = error instanceof Error ? error.message : "无法读取密钥";
    $q.notify({ type: "negative", message: msg });
  } finally {
    listKeyRevealing.value = false;
  }
}
const showVariantChip = computed(() => {
  const pt = (config.value.provider_type || "").toLowerCase();
  const variant = (config.value.variant || "").toLowerCase();
  return pt === "openai" && variant !== "" && variant !== "openai";
});
const haChipLabel = computed(() => {
  const mode = (config.value.ha_mode || "").toLowerCase();
  if (mode === "failover") return "Failover";
  if (mode === "hedge") return "Hedge";
  return "";
});
const haChipColor = computed(() => ((config.value.ha_mode || "").toLowerCase() === "hedge" ? "purple-1" : "light-blue-1"));
const haChipTextColor = computed(() => ((config.value.ha_mode || "").toLowerCase() === "hedge" ? "purple-9" : "blue-9"));
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
  background: var(--color-status-blue-bg);
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
.masked-secret {
  font-family: ui-monospace, monospace;
  letter-spacing: 0.08em;
  color: var(--q-color-grey-8);
}

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
  border: 1px solid rgb(15 23 42 / 6%);
  border-radius: 14px;
  background: linear-gradient(180deg, var(--color-on-accent), var(--color-surface-soft));
}

.usage-mini-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.provider-title {
  color: var(--color-text-gray-800);
  font-size: 16px;
  font-weight: 700;
}

.model-name {
  color: var(--color-text-gray-600);
  font-size: 14px;
  margin-top: 4px;
}

.status-dot {
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: var(--color-success);
  box-shadow: 0 0 0 4px rgb(22 163 74 / 12%);
  flex: 0 0 auto;
}

.status-dot--off {
  background: var(--color-text-gray-400);
  box-shadow: 0 0 0 4px rgb(156 163 175 / 14%);
}

.field-label {
  color: var(--color-text-gray-500);
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 6px;
}

.metric-value {
  color: var(--color-surface-elevated);
  font-size: 14px;
  font-weight: 600;
}

.muted-value {
  color: var(--color-text-gray-400);
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
  border-right: 1px solid rgb(15 23 42 / 8%);
}

.provider-action-btn {
  width: 34px;
  height: 34px;
  min-height: 34px;
  border: 1px solid rgb(25 118 210 / 12%);
  border-radius: 12px;
  background: linear-gradient(180deg, var(--color-status-blue-bg), var(--color-info-soft));
  box-shadow: 0 8px 18px rgb(25 118 210 / 8%);
  transition:
    transform 160ms ease,
    box-shadow 160ms ease,
    border-color 160ms ease;
}

.provider-action-btn--danger {
  border-color: rgb(220 38 38 / 12%);
  background: linear-gradient(180deg, var(--color-status-danger-bg), var(--color-danger-soft));
  box-shadow: 0 8px 18px rgb(220 38 38 / 8%);
}

.provider-action-btn:hover {
  transform: translateY(-1px);
  border-color: rgb(25 118 210 / 28%);
  box-shadow: 0 12px 24px rgb(25 118 210 / 14%);
}

.provider-action-btn--danger:hover {
  border-color: rgb(220 38 38 / 28%);
  box-shadow: 0 12px 24px rgb(220 38 38 / 14%);
}

.provider-row.provider-row--dark {
  color: var(--color-border-soft);
}

.provider-row.provider-row--dark:hover {
  background: rgb(51 65 85 / 46%);
}

.provider-row.provider-row--dark .provider-usage {
  border-color: rgb(148 163 184 / 14%);
  background: linear-gradient(180deg, rgb(30 41 59 / 78%), rgb(15 23 42 / 82%));
}

.provider-row.provider-row--dark .provider-title,
.provider-row.provider-row--dark .metric-value {
  color: var(--color-surface-soft);
}

.provider-row.provider-row--dark .model-name,
.provider-row.provider-row--dark .field-label {
  color: var(--color-text-tertiary);
}

.provider-row.provider-row--dark .muted-value {
  color: var(--color-text-tertiary);
}

.provider-row.provider-row--dark .provider-actions :deep(.q-toggle) {
  border-right-color: rgb(148 163 184 / 16%);
}

.provider-row.provider-row--dark .provider-action-btn {
  border-color: rgb(96 165 250 / 20%);
  background: linear-gradient(180deg, rgb(30 41 59 / 82%), rgb(15 23 42 / 86%));
  box-shadow: 0 8px 18px rgb(0 0 0 / 25%);
}

.provider-row.provider-row--dark .provider-action-btn--danger {
  border-color: rgb(248 113 113 / 22%);
  background: linear-gradient(180deg, rgb(127 29 29 / 22%), rgb(69 10 10 / 18%));
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-grey-2) {
  background: rgb(51 65 85 / 76%) !important;
  color: var(--color-text-slate-300) !important;
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-blue-1),
.provider-row.provider-row--dark :deep(.q-chip--colored.bg-indigo-1) {
  background: rgb(30 64 175 / 26%) !important;
  color: var(--color-accent-blue-light) !important;
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-green-1) {
  background: rgb(22 101 52 / 26%) !important;
  color: var(--color-accent-green) !important;
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-orange-1) {
  background: rgb(120 53 15 / 28%) !important;
  color: var(--color-accent-orange-light) !important;
}

@media (width <= 1023px) {
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
