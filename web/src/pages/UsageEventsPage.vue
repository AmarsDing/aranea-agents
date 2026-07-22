<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero
      kicker="Token / Usage"
      title="用量事件明细"
      subtitle="按时间查看 model_token_usage_events 原始记录（费用来自 model_pricing_rules 快照）。列表为服务端分页。"
    >
      <template #actions>
        <q-btn outline rounded no-caps icon="download" label="导出 CSV" :loading="exporting" @click="onExportCsv" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-select
        v-model="filters.range"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        label="范围"
        :options="rangeOptions"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="filters.provider_code"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        label="Provider"
        @clear="onFilterChange"
      />
      <q-input
        v-model="filters.model_api_id"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        label="模型"
        @clear="onFilterChange"
      />
      <q-input
        v-model="filters.agent_id"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        label="Agent ID"
        @clear="onFilterChange"
      />
      <q-input
        v-model="filters.team_id"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        label="Team ID"
        @clear="onFilterChange"
      />
      <q-select
        v-model="filters.usage_kind"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="来源"
        :options="usageKindOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="filters.status"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="状态"
        :options="statusOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="retainDays"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        label="清理保留天数"
        :options="retainDayOptions"
      />
      <template #actions>
        <q-btn flat rounded no-caps label="重置" icon="restart_alt" @click="onResetFilters" />
        <q-btn flat rounded no-caps label="查询" icon="manage_search" :loading="loading" @click="onSearch" />
        <q-btn flat rounded no-caps label="删除记录" icon="delete_outline" :loading="purging" @click="onPurgeConfirm" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

    <q-card v-if="!loading && events.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="insights" />
        <div class="text-h6 q-mt-md">暂无用量事件</div>
        <div class="text-body2 text-grey-7 q-mt-sm">调整筛选条件后重新查询。</div>
      </q-card-section>
    </q-card>

    <template v-else>
      <AppRegistryTable
        :rows="events"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-model_api_id="props">
          <q-td :props="props">
            <span class="app-registry-cell-primary ellipsis">{{ props.row.model_api_id }}</span>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <AppRegistryHoverTip :text="props.row.error_message">
              <q-chip
                dense
                :color="props.row.status === 'success' ? 'positive' : 'negative'"
                text-color="white"
                size="sm"
              >
                {{ props.row.status }}
              </q-chip>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-total_cost_micro_usd="props">
          <q-td :props="props">{{ formatMoney(props.row.total_cost_micro_usd) }}</q-td>
        </template>
        <template #body-cell-latency_ms="props">
          <q-td :props="props">
            <span :title="`${props.row.latency_ms}ms`">{{ formatLatency(props.row.latency_ms) }}</span>
          </q-td>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        :page="page"
        :page-size="pageSize"
        :page-max="pageMax"
        :total="eventsTotal"
        :loading="loading"
        :label="t('usageEventsPage.paginationLabel')"
        @update:page="onPage"
        @update:page-size="onPageSize"
      />
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import { useUsageEventsPage } from '../features/usage/useUsageEventsPage';
import type { ModelTokenUsageEvent } from '../features/usage/types';
import { type RegistryTableColumn } from '../features/ui/registryTableColumns';
import { USAGE_EVENT_TABLE_COLUMNS } from '../features/usage/usageTableUi';

const { t } = useI18n();

const {
  events,
  eventsTotal,
  page,
  pageSize,
  pageMax,
  onPage,
  onPageSize,
  loading,
  error,
  exporting,
  filters,
  rangeOptions,
  statusOptions,
  usageKindOptions,
  retainDays,
  retainDayOptions,
  purging,
  load,
  exportCsv,
  onPurgeConfirm,
  resetFilters,
  formatMoney,
  formatLatency,
} = useUsageEventsPage();

const columns = USAGE_EVENT_TABLE_COLUMNS satisfies RegistryTableColumn<ModelTokenUsageEvent>[];

function onResetFilters() {
  resetFilters();
}

function onSearch() {
  page.value = 1;
  void load();
}

async function onExportCsv() {
  try {
    await exportCsv();
  } catch {
    // error surfaced via store
  }
}

function onFilterChange() {
  page.value = 1;
  void load();
}

onMounted(() => void load());
</script>
