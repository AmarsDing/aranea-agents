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
