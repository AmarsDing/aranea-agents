<template>
  <AppRegistryTable
    :shell="shell"
    table-class="provider-models-data-table"
    :rows="rows"
    :columns="PROVIDER_MODEL_TABLE_COLUMNS"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
    column-persist-key="provider-models-table"
  >
    <template #body-cell-model="props">
      <q-td :props="props">
        <div class="row items-center no-wrap q-gutter-sm min-width-0">
          <ProviderLogo :provider-id="props.row.provider || ''" size="32px" />
          <div class="provider-identity min-width-0">
            <div class="provider-identity__title">
              <span class="status-dot" :class="{ 'status-dot--off': !props.row.enabled }" />
              <span class="provider-title ellipsis">{{ rowProviderDisplayName(props.row) }}</span>
              <span class="model-name ellipsis">{{ modelDisplayName(props.row) }}</span>
            </div>
            <div class="provider-tags">
              <span class="provider-tag provider-tag--provider">{{ props.row.provider || "未设置" }}</span>
              <span v-if="rowConfig(props.row).provider_type" class="provider-tag provider-tag--type">
                {{ rowConfig(props.row).provider_type }}
              </span>
              <span v-if="showVariantChip(rowConfig(props.row))" class="provider-tag provider-tag--variant">
                {{ rowConfig(props.row).variant }}
              </span>
              <span
                v-if="haChipLabel(rowConfig(props.row))"
                class="provider-tag"
                :class="haTagClass(rowConfig(props.row))"
              >
                {{ haChipLabel(rowConfig(props.row)) }}
              </span>
              <span
                v-for="category in providerCategories(rowConfig(props.row))"
                :key="category.value"
                class="provider-tag provider-tag--category"
              >
                {{ category.label }}
                <q-tooltip>{{ category.tooltip }}</q-tooltip>
              </span>
              <span
                v-for="chip in providerCapabilityChips(rowConfig(props.row))"
                :key="chip.key"
                class="provider-tag provider-tag--capability"
              >
                {{ chip.label }}
              </span>
              <q-icon
                v-if="rowPricingNotConfigured(props.row)"
                name="price_check"
                size="16px"
                color="warning"
                class="provider-pricing-warn"
              >
                <q-tooltip>未配置定价，用量监控费用将显示为 0</q-tooltip>
              </q-icon>
            </div>
          </div>
        </div>
      </q-td>
    </template>

    <template #body-cell-size="props">
      <q-td :props="props">
        <span class="stat-value">{{ rowConfig(props.row).model_size_label || "—" }}</span>
      </q-td>
    </template>

    <template #body-cell-ctx="props">
      <q-td :props="props">
        <span class="stat-value">{{ formatContextWindow(rowConfig(props.row).context_window_k) }}</span>
      </q-td>
    </template>

    <template #body-cell-tps="props">
      <q-td :props="props">
        <span class="stat-value">{{ formatTps(rowConfig(props.row).tokens_per_second) }}</span>
      </q-td>
    </template>

    <template #body-cell-usage="props">
      <q-td :props="props">
        <div class="provider-usage">
          <div class="usage-line">
            <span class="usage-badge" :class="`usage-badge--${rowHotnessTone(props.row)}`">
              {{ rowHotnessLabel(props.row) }} {{ rowHotnessScore(props.row) }}
            </span>
            <div class="usage-bar-wrap">
              <div
                class="usage-bar-fill"
                :style="{ width: `${rowHotnessScore(props.row)}%` }"
                :class="`usage-bar-fill--${rowHotnessTone(props.row)}`"
              />
            </div>
            <q-tooltip>
              近30天调用：{{ formatCount(rowConfig(props.row).usage_call_count_30d) }}；
              Token：{{ formatCount(rowConfig(props.row).usage_total_tokens_30d) }}；
              费用：{{ formatMicroUsd(rowConfig(props.row).usage_cost_micro_usd_30d) }}；
              成功率：{{ formatPercent(rowConfig(props.row).success_rate_30d) }}
            </q-tooltip>
          </div>
          <span class="usage-meta">
            调用 {{ formatCount(rowConfig(props.row).usage_call_count_30d) }} · 费用
            {{ formatMicroUsd(rowConfig(props.row).usage_cost_micro_usd_30d) }}
          </span>
        </div>
      </q-td>
    </template>

    <template #body-cell-secret="props">
      <q-td :props="props">
        <div class="provider-secret">
          <template v-if="providerHasApiKey(rowConfig(props.row))">
            <span
              class="provider-secret-value"
              :class="{ 'provider-secret-value--masked': !listKeyState(props.row.id).visible }"
            >
              {{ listSecretDisplay(listKeyState(props.row.id).visible, listKeyState(props.row.id).value) }}
            </span>
            <q-btn
              flat
              dense
              round
              size="xs"
              class="provider-secret-toggle"
              :icon="listKeyState(props.row.id).visible ? 'visibility_off' : 'visibility'"
              :loading="listKeyState(props.row.id).revealing"
              :aria-label="listKeyState(props.row.id).visible ? '隐藏 API 密钥' : '查看 API 密钥'"
              @click="$emit('toggleRevealKey', props.row)"
            />
          </template>
          <q-chip v-else dense square color="orange-1" text-color="orange-9" icon="key">未设置</q-chip>
        </div>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="provider-actions app-registry-cell-actions">
          <q-toggle
            :model-value="props.row.enabled"
            color="primary"
            dense
            :disable="saving"
            aria-label="启用模型"
            @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
          />
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="query_stats"
            color="secondary"
            class="provider-action-btn"
            :aria-label="`查看 ${props.row.name} 趋势`"
            @click="$emit('trend', props.row)"
          >
            <q-tooltip>历史趋势</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="edit"
            color="primary"
            class="provider-action-btn"
            :aria-label="`编辑 ${props.row.name}`"
            @click="$emit('edit', props.row)"
          />
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="delete"
            color="negative"
            class="provider-action-btn provider-action-btn--danger"
            :aria-label="`删除 ${props.row.name}`"
            @click="$emit('delete', props.row)"
          />
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import ProviderLogo from "./ProviderLogo.vue";
import type { PlatformResource } from "../../features/platform/types";
import {
  PROVIDER_MODEL_TABLE_COLUMNS,
  formatContextWindow,
  formatCount,
  formatMicroUsd,
  formatPercent,
  formatTps,
  getProviderConfig,
  haChipLabel,
  haTagClass,
  hotnessLabel,
  hotnessScore,
  hotnessTone,
  listSecretDisplay,
  modelDisplayName,
  providerCapabilityChips,
  providerCategories,
  providerDisplayName,
  providerHasApiKey,
  rowPricingNotConfigured,
  showVariantChip
} from "./providerModelUi";

type ListKeyEntry = { visible: boolean; revealing: boolean; value: string };

withDefaults(
  defineProps<{
    rows: PlatformResource[];
    loading: boolean;
    saving?: boolean;
    shell?: boolean;
    listKeyState: (id: string) => ListKeyEntry;
  }>(),
  {
    saving: false,
    shell: false
  }
);

defineEmits<{
  toggleEnabled: [row: PlatformResource, enabled: boolean];
  toggleRevealKey: [row: PlatformResource];
  trend: [row: PlatformResource];
  edit: [row: PlatformResource];
  delete: [row: PlatformResource];
}>();

function rowConfig(row: PlatformResource) {
  return getProviderConfig(row);
}

function rowProviderDisplayName(row: PlatformResource) {
  return providerDisplayName(row, rowConfig(row));
}

function rowHotnessScore(row: PlatformResource) {
  return hotnessScore(rowConfig(row));
}

function rowHotnessLabel(row: PlatformResource) {
  return hotnessLabel(rowHotnessScore(row));
}

function rowHotnessTone(row: PlatformResource) {
  return hotnessTone(rowHotnessScore(row));
}
</script>
