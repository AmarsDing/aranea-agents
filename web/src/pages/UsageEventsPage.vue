<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero
      kicker="Token / Usage"
      :title="t('usageEventsPage.title')"
      :subtitle="t('usageEventsPage.subtitle')"
    >
      <template #actions>
        <q-btn outline rounded no-caps icon="download" :label="t('usageEventsPage.export')" :loading="exporting" @click="onExportCsv" />
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
        :label="t('usageEventsPage.range')"
        :options="rangeOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="filters.provider_code"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('usageEventsPage.provider')"
        :options="providerOptions"
        @update:model-value="onProviderChange"
      />
      <q-select
        v-model="filters.model_api_id"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('usageEventsPage.model')"
        :options="modelOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="filters.agent_id"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('usageEventsPage.agent')"
        :options="agentOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="filters.team_id"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('usageEventsPage.team')"
        :options="teamOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="filters.usage_kind"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('usageEventsPage.source')"
        :options="usageKindOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="filters.effort"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('usageEventsPage.effortLabel')"
        :options="effortOptions"
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
        :label="t('usageEventsPage.status')"
        :options="statusOptions"
        @update:model-value="onFilterChange"
      />
      <template #actions>
        <q-btn flat dense rounded no-caps :label="t('usageEventsPage.reset')" icon="restart_alt" @click="onResetFilters" />
        <q-btn flat dense rounded no-caps :label="t('usageEventsPage.search')" icon="manage_search" :loading="loading" @click="onSearch" />
        <q-btn
          flat
          dense
          rounded
          no-caps
          :label="t('usageEventsPage.purge')"
          icon="delete_outline"
          :loading="purging"
          @click="onPurgeConfirm"
        />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md" role="alert">
      {{ error }}
      <template #action><q-btn flat :label="t('usageEventsPage.retry')" :loading="loading" @click="load" /></template>
    </q-banner>

    <q-card v-if="!loading && !error && events.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="insights" />
        <div class="text-h6 q-mt-md">{{ t('usageEventsPage.empty') }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">{{ t('usageEventsPage.emptyHint') }}</div>
      </q-card-section>
    </q-card>

    <template v-else-if="!error || events.length">
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
        <template #body-cell-agent_id="props">
          <q-td :props="props">
            <div class="app-registry-cell-primary ellipsis">
              {{ props.row.agent_name || props.row.agent_key || props.row.agent_id || '—' }}
            </div>
            <div v-if="props.row.agent_name" class="app-registry-cell-sub ellipsis text-mono">
              {{ props.row.agent_id }}
            </div>
          </q-td>
        </template>
        <template #body-cell-session_id="props">
          <q-td :props="props">
            <AppRegistryHoverTip :text="props.row.session_id" :indicator="false">
              <span class="app-registry-cell-primary ellipsis">
                {{ props.row.session_title || shortenId(props.row.session_id) }}
              </span>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <AppStatusChip :status="props.row.status" :tooltip="props.row.error_message" />
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
import { computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import { useUsageEventsPage } from '../features/usage/useUsageEventsPage';
import type { ModelTokenUsageEvent } from '../features/usage/types';
import { type RegistryTableColumn } from '../features/ui/registryTableColumns';
import { buildUsageEventTableColumns } from '../features/usage/usageTableUi';

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
  effortOptions,
  providerOptions,
  modelOptions,
  agentOptions,
  teamOptions,
  loadFilterOptions,
  onProviderFilterChange,
  purging,
  load,
  exportCsv,
  onPurgeConfirm,
  resetFilters,
  formatMoney,
  formatLatency,
} = useUsageEventsPage();

const columns = computed(() => buildUsageEventTableColumns(t) satisfies RegistryTableColumn<ModelTokenUsageEvent>[]);

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
    // Domain composable owns export failure feedback.
  }
}

function onFilterChange() {
  page.value = 1;
  void load();
}

/** Provider 变化：级联清理失效的模型筛选后再查询 */
function onProviderChange() {
  onProviderFilterChange();
  onFilterChange();
}

/** Session 无标题时截断显示 ID（完整 ID 见悬停 tooltip） */
function shortenId(id?: string) {
  const s = (id ?? '').trim();
  if (!s) return '—';
  return s.length <= 12 ? s : `${s.slice(0, 8)}…`;
}

onMounted(() => {
  void loadFilterOptions();
  void load();
});
</script>

<style scoped lang="sass">
// 8 个筛选字段 + 操作按钮：收窄字段宽度，空间不足时换行而非横向滚动
:deep(.app-page-toolbar__body)
  flex-wrap: wrap

:deep(.app-page-toolbar__field)
  min-width: 108px
  max-width: 160px
</style>
