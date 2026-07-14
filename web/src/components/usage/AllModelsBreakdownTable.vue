<template>
  <q-card flat class="overview-panel all-models-breakdown">
    <q-card-section class="row items-center justify-between no-wrap">
      <div>
        <div class="text-h6 overview-section-title">{{ t('overviewPage.allModelsTitle') }}</div>
        <div class="text-caption overview-section-caption">
          {{ t('overviewPage.allModelsCaption') }}
        </div>
      </div>
      <q-input
        v-model="searchInput"
        dense
        outlined
        clearable
        :placeholder="t('overviewPage.searchPlaceholder')"
        class="all-models-breakdown__search"
        @update:model-value="onSearchInput"
        @clear="onSearchInput"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
    </q-card-section>
    <q-separator class="overview-separator" />

    <q-banner v-if="error" class="bg-negative text-white">
      <template #avatar><q-icon name="error" /></template>
      {{ error }}
    </q-banner>

    <AppRegistryTable
      :shell="false"
      :rows="rows"
      :columns="ALL_MODELS_BREAKDOWN_TABLE_COLUMNS"
      :loading="loading"
      :pagination="qPagination"
      :rows-per-page-options="[10, 20, 50, 100]"
      row-key="model_api_id"
      :binary-state-sort="true"
      :external-pagination="false"
      column-persist-key="all-models-breakdown-table"
      table-class="all-models-breakdown-table"
      @request="onRequest"
    >
      <template #body-cell-model="p">
        <q-td :props="p">
          <div class="text-weight-medium">
            {{ p.row.model_display_name || p.row.model_api_id }}
          </div>
          <div class="all-models-breakdown__subtext">{{ p.row.provider_code }} / {{ p.row.model_api_id }}</div>
        </q-td>
      </template>

      <template #body-cell-call_count="p">
        <q-td :props="p" class="text-right">{{ formatCount(p.row.call_count) }}</q-td>
      </template>
      <template #body-cell-total_tokens="p">
        <q-td :props="p" class="text-right">{{ formatCount(p.row.total_tokens) }}</q-td>
      </template>
      <template #body-cell-total_cost_micro_usd="p">
        <q-td :props="p" class="text-right">{{ formatMoney(p.row.total_cost_micro_usd) }}</q-td>
      </template>
      <template #body-cell-success_rate="p">
        <q-td :props="p" class="text-right">{{ formatPercent(p.row.success_rate) }}</q-td>
      </template>
      <template #body-cell-avg_latency_ms="p">
        <q-td :props="p" class="text-right">{{ formatLatencyMs(p.row.avg_latency_ms) }}</q-td>
      </template>
      <template #body-cell-avg_tokens_per_second="p">
        <q-td :props="p" class="text-right">{{ formatTps(p.row.avg_tokens_per_second) }}</q-td>
      </template>

      <template #no-data>
        <div class="all-models-breakdown__empty">{{ t('overviewPage.allModelsEmpty') }}</div>
      </template>
    </AppRegistryTable>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import { ALL_MODELS_BREAKDOWN_TABLE_COLUMNS } from './allModelsBreakdownTableUi';
import { useAllModelsBreakdown } from '../../features/usage/useAllModelsBreakdown';
import {
  formatUsdCompact,
  formatCount,
  formatPercent,
  formatLatencyMs,
  formatTps,
} from '../../features/usage/moneyFormat';

const props = defineProps<{
  range: string;
  providerCode: string;
}>();

const { t } = useI18n();

const { rows, loading, error, searchInput, qPagination, onSearchInput, onRequest } = useAllModelsBreakdown({
  range: () => props.range,
  providerCode: () => props.providerCode,
  initialPageSize: 10,
});

function formatMoney(value?: number) {
  return formatUsdCompact(value);
}
</script>

<style scoped>
.all-models-breakdown__search {
  width: 240px;
}

.all-models-breakdown__subtext {
  font-size: 11px;
  opacity: 0.7;
}

.all-models-breakdown__empty {
  padding: 24px;
  text-align: center;
  opacity: 0.6;
}
</style>
