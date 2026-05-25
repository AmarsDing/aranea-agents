<template>
  <div :class="['provider-row', { 'provider-row--dark': isDark }]">
    <div class="provider-row__inner">
      <ProviderLogo :provider-id="row.provider || ''" size="32px" />

      <div class="provider-table__grid provider-row-grid">
        <div class="provider-table__cell provider-table__cell--identity provider-identity">
          <div class="provider-identity__title">
            <span class="status-dot" :class="{ 'status-dot--off': !row.enabled }" />
            <span class="provider-title ellipsis">{{ providerDisplayName }}</span>
            <span class="model-name ellipsis">{{ modelDisplayName }}</span>
          </div>
          <div class="provider-tags">
            <span class="provider-tag provider-tag--provider">{{ row.provider || "未设置" }}</span>
            <span v-if="config.provider_type" class="provider-tag provider-tag--type">{{ config.provider_type }}</span>
            <span v-if="showVariantChip" class="provider-tag provider-tag--variant">{{ config.variant }}</span>
            <span v-if="haChipLabel" class="provider-tag" :class="haTagClass">{{ haChipLabel }}</span>
            <span
              v-for="category in categories"
              :key="category.value"
              class="provider-tag provider-tag--category"
            >
              {{ category.label }}
              <q-tooltip>{{ category.tooltip }}</q-tooltip>
            </span>
            <span
              v-for="chip in capabilityChips"
              :key="chip.key"
              class="provider-tag provider-tag--capability"
            >
              {{ chip.label }}
            </span>
          </div>
        </div>

        <div class="provider-table__cell provider-table__cell--size">
          <span class="stat-value">{{ config.model_size_label || "—" }}</span>
        </div>
        <div class="provider-table__cell provider-table__cell--ctx">
          <span class="stat-value">{{ formatContextWindow(config.context_window_k) }}</span>
        </div>
        <div class="provider-table__cell provider-table__cell--tps">
          <span class="stat-value">{{ formatTps(config.tokens_per_second) }}</span>
        </div>

        <div class="provider-table__cell provider-table__cell--usage provider-usage">
          <div class="usage-line">
            <span class="usage-badge" :class="`usage-badge--${hotnessTone}`">{{ hotnessLabel }} {{ hotnessScore }}</span>
            <div class="usage-bar-wrap">
              <div class="usage-bar-fill" :style="{ width: `${hotnessScore}%` }" :class="`usage-bar-fill--${hotnessTone}`" />
            </div>
            <q-tooltip>
              近30天调用：{{ formatCount(config.usage_call_count_30d) }}；
              Token：{{ formatCount(config.usage_total_tokens_30d) }}；
              费用：{{ formatMicroUsd(config.usage_cost_micro_usd_30d) }}；
              成功率：{{ formatPercent(config.success_rate_30d) }}
            </q-tooltip>
          </div>
          <span class="usage-meta">
            调用 {{ formatCount(config.usage_call_count_30d) }} · 费用 {{ formatMicroUsd(config.usage_cost_micro_usd_30d) }}
          </span>
        </div>

        <div class="provider-table__cell provider-table__cell--secret provider-secret">
          <template v-if="hasApiKey">
            <span
              class="provider-secret-value"
              :class="{ 'provider-secret-value--masked': !props.listKeyVisible }"
            >{{ listSecretDisplay }}</span>
            <q-btn
              flat
              dense
              round
              size="xs"
              class="provider-secret-toggle"
              :icon="props.listKeyVisible ? 'visibility_off' : 'visibility'"
              :loading="props.listKeyRevealing"
              :aria-label="props.listKeyVisible ? '隐藏 API 密钥' : '查看 API 密钥'"
              @click="emit('toggle-reveal-key', props.row)"
            />
          </template>
          <q-chip v-else dense square color="orange-1" text-color="orange-9" icon="key">未设置</q-chip>
        </div>

        <div class="provider-table__cell provider-table__cell--actions provider-actions">
          <q-toggle
            :model-value="row.enabled"
            color="primary"
            dense
            :disable="saving"
            aria-label="启用模型"
            @update:model-value="$emit('toggle-enabled', row, Boolean($event))"
          />
          <q-btn flat dense round size="sm" icon="query_stats" color="secondary" class="provider-action-btn" :aria-label="`查看 ${row.name} 趋势`" @click="$emit('trend', row)">
            <q-tooltip>历史趋势</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" icon="edit" color="primary" class="provider-action-btn" :aria-label="`编辑 ${row.name}`" @click="$emit('edit', row)" />
          <q-btn flat dense round size="sm" icon="delete" color="negative" class="provider-action-btn provider-action-btn--danger" :aria-label="`删除 ${row.name}`" @click="$emit('delete', row)" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useQuasar } from "quasar";
import type { PlatformResource } from "../../features/platform/types";
import ProviderLogo from "./ProviderLogo.vue";

type ModelCategory = {
  value: string;
  label: string;
  tooltip: string;
};

type CapabilityChip = {
  key: string;
  label: string;
  source?: string;
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
  capability_chips?: CapabilityChip[];
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
  listKeyVisible?: boolean;
  listKeyRevealing?: boolean;
  listRevealedApiKey?: string;
}>();

const emit = defineEmits<{
  "toggle-enabled": [row: PlatformResource, enabled: boolean];
  "toggle-reveal-key": [row: PlatformResource];
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
const capabilityChips = computed(() => {
  const values = config.value.capability_chips;
  return Array.isArray(values) ? values.filter((chip) => chip?.key && chip?.label) : [];
});
const providerDisplayName = computed(() => config.value.provider_display_name || props.row.provider || props.row.key);
const modelDisplayName = computed(() => props.row.name || props.row.model || "未设置模型");
const hasApiKey = computed(() =>
  Boolean(config.value.api_key_set || config.value.api_key || config.value.secret_id || config.value.aws_region)
);
const listSecretDisplay = computed(() =>
  props.listKeyVisible ? props.listRevealedApiKey || "••••••" : "••••••"
);

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
const haTagClass = computed(() =>
  (config.value.ha_mode || "").toLowerCase() === "hedge" ? "provider-tag--ha-hedge" : "provider-tag--ha-failover"
);
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
const hotnessTone = computed(() => {
  if (hotnessScore.value >= 80) return "hot";
  if (hotnessScore.value >= 50) return "warm";
  if (hotnessScore.value >= 20) return "cool";
  return "idle";
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
  return numberValue === null ? "—" : `${numberValue}/s`;
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
  padding: 8px 16px;
  border-bottom: 1px solid var(--glass-border);
  transition: background-color 180ms ease;
}

.provider-row:last-child {
  border-bottom: none;
}

.provider-row:hover {
  background: var(--interaction-surface-hover);
}

.provider-row__inner {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.provider-table__cell--size,
.provider-table__cell--ctx,
.provider-table__cell--tps {
  text-align: center;
}

.provider-row__avatar {
  flex: 0 0 auto;
}

.provider-row-grid {
  flex: 1;
  min-width: 0;
}

.provider-identity {
  min-width: 0;
}

.provider-identity__title {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 6px 8px;
  min-width: 0;
}

.provider-title {
  color: var(--color-text-heading);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.3;
}

.model-name {
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.3;
}

.provider-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
  max-height: 22px;
  overflow: hidden;
}

.provider-tag {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  padding: 2px 8px;
  border: 1px solid var(--glass-border);
  border-radius: 6px;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.35;
  white-space: nowrap;
}

.provider-tag--provider {
  background: color-mix(in srgb, var(--glass-elevated) 90%, transparent);
  color: var(--color-text-secondary);
  border-color: color-mix(in srgb, var(--color-text-secondary) 22%, var(--glass-border));
}

.provider-tag--type,
.provider-tag--category {
  background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-elevated));
  color: var(--color-link);
  border-color: color-mix(in srgb, var(--color-accent) 24%, var(--glass-border));
}

.provider-tag--capability {
  background: color-mix(in srgb, var(--color-positive, #21ba45) 10%, var(--glass-elevated));
  color: var(--color-positive, #21ba45);
  border-color: color-mix(in srgb, var(--color-positive, #21ba45) 22%, var(--glass-border));
}

.provider-tag--variant {
  background: color-mix(in srgb, var(--color-success) 12%, var(--glass-elevated));
  color: var(--color-success);
  border-color: color-mix(in srgb, var(--color-success) 24%, var(--glass-border));
}

.provider-tag--ha-failover {
  background: color-mix(in srgb, var(--color-link) 12%, var(--glass-elevated));
  color: var(--color-link);
}

.provider-tag--ha-hedge {
  background: color-mix(in srgb, var(--color-accent-indigo) 14%, var(--glass-elevated));
  color: var(--color-accent-indigo-light);
  border-color: color-mix(in srgb, var(--color-accent-indigo) 28%, var(--glass-border));
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: var(--color-success);
  box-shadow: 0 0 0 3px rgb(22 163 74 / 12%);
  flex: 0 0 auto;
  align-self: center;
}

.status-dot--off {
  background: var(--color-text-gray-400);
  box-shadow: 0 0 0 3px rgb(156 163 175 / 14%);
}

.stat-value {
  color: var(--color-text-primary);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.2;
}

.provider-usage {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
  min-width: 0;
}

.usage-line {
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.usage-badge {
  flex-shrink: 0;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  line-height: 1.35;
  white-space: nowrap;
}

.usage-badge--idle {
  background: color-mix(in srgb, var(--color-text-tertiary) 24%, var(--glass-elevated));
  color: var(--color-text-secondary);
}

.usage-badge--cool {
  background: color-mix(in srgb, var(--color-link) 18%, var(--glass-elevated));
  color: var(--color-link);
}

.usage-badge--warm {
  background: color-mix(in srgb, var(--color-accent) 20%, var(--glass-elevated));
  color: var(--color-accent);
}

.usage-badge--hot {
  background: color-mix(in srgb, var(--color-warning) 24%, var(--glass-elevated));
  color: var(--color-accent-amber);
}

.usage-bar-wrap {
  flex: 1;
  height: 4px;
  min-width: 36px;
  border-radius: 999px;
  overflow: hidden;
  background: color-mix(in srgb, var(--color-text-tertiary) 18%, transparent);
}

.usage-bar-fill {
  height: 100%;
  border-radius: inherit;
  transition: width 180ms ease;
}

.usage-bar-fill--idle {
  background: var(--color-text-tertiary);
}

.usage-bar-fill--cool {
  background: var(--color-link);
}

.usage-bar-fill--warm {
  background: var(--color-accent);
}

.usage-bar-fill--hot {
  background: var(--color-accent-amber);
}

.usage-meta {
  color: var(--color-text-secondary);
  font-size: 11px;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.provider-secret {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.provider-secret-value {
  flex: 1;
  min-width: 0;
  font-family: ui-monospace, monospace;
  font-size: 11px;
  line-height: 1.3;
  color: var(--color-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-secret-value--masked {
  letter-spacing: 0.06em;
}

.provider-secret-toggle {
  flex-shrink: 0;
}

.provider-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  min-width: 0;
}

.provider-actions :deep(.q-toggle) {
  padding-right: 6px;
  margin-right: 2px;
  border-right: 1px solid var(--glass-border);
}

.provider-action-btn {
  width: 28px;
  height: 28px;
  min-height: 28px;
  border: 1px solid color-mix(in srgb, var(--color-accent) 16%, var(--glass-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--glass-elevated) 90%, transparent);
}

.provider-action-btn--danger {
  border-color: color-mix(in srgb, var(--color-danger) 20%, var(--glass-border));
}

.provider-row.provider-row--dark:hover {
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface));
}

.provider-row.provider-row--dark .provider-title {
  color: var(--color-text-heading);
}

.provider-row.provider-row--dark .model-name,
.provider-row.provider-row--dark .usage-meta {
  color: var(--color-text-secondary);
}

.provider-row.provider-row--dark .stat-value {
  color: var(--color-text-primary);
}

.provider-row.provider-row--dark .provider-secret-value {
  color: var(--color-text-tertiary);
}

.provider-row.provider-row--dark .provider-tag--provider {
  background: color-mix(in srgb, var(--glass-elevated) 88%, transparent);
  color: var(--color-text-secondary);
  border-color: color-mix(in srgb, var(--color-text-secondary) 28%, var(--glass-border));
}

.provider-row.provider-row--dark .provider-tag--type,
.provider-row.provider-row--dark .provider-tag--category {
  background: color-mix(in srgb, var(--color-accent) 14%, var(--glass-elevated));
  color: var(--color-accent);
  border-color: color-mix(in srgb, var(--color-accent) 32%, var(--glass-border));
}

.provider-row.provider-row--dark :deep(.q-chip--colored.bg-orange-1) {
  background: rgb(120 53 15 / 28%) !important;
  color: var(--color-accent-orange-light) !important;
}

@media (width <= 1023px) {
  .provider-table__head,
  .provider-table__body {
    min-width: 920px;
  }
}
</style>
