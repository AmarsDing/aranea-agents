<template>
  <q-page class="app-standard-page app-registry-page hook-deliveries-page">
    <AppPageHero
      :kicker="t('hooksPage.deliveries.kicker')"
      :title="t('hooksPage.deliveries.title')"
      :subtitle="t('hooksPage.deliveries.subtitle')"
    >
      <template #actions>
        <q-btn outline rounded no-caps icon="rule" :label="t('hooksPage.deliveries.btnHookRules')" to="/hooks" />
        <q-btn
          outline
          rounded
          no-caps
          icon="history"
          :label="t('hooksPage.deliveries.btnBlockErrors')"
          to="/plugins/runs"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="hookKey"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        debounce="350"
        :label="t('hooksPage.deliveries.filterHookKey')"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="status"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('hooksPage.deliveries.filterStatus')"
        :options="statusOptions"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="from"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        type="datetime-local"
        :label="t('hooksPage.deliveries.filterFrom')"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="to"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        type="datetime-local"
        :label="t('hooksPage.deliveries.filterTo')"
        @update:model-value="onFilterChange"
      />
      <template #actions>
        <q-btn
          flat
          rounded
          no-caps
          icon="restart_alt"
          :label="t('hooksPage.deliveries.btnReset')"
          @click="resetFilters"
        />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('hooksPage.deliveries.btnRefresh')"
          :loading="loading"
          @click="() => loadRows()"
        />
      </template>
    </AppPageToolbar>

    <AppStatePanel
      v-if="pageState === 'error'"
      class="q-mb-md"
      variant="error"
      :description="error"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="() => loadRows()"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="t('shell.runs.emptyTitle')"
      :description="t('shell.runs.emptyDesc')"
      icon="outbox"
      :action-label="t('shell.runs.emptyAction')"
      action-icon="restart_alt"
      @action="resetFilters"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="t('shell.runs.staleError', { message: error })"
        :action-label="t('common.retry')"
        action-icon="refresh"
        @action="() => loadRows()"
      />

      <div class="app-registry-table-shell">
        <AppRegistryTable
          :rows="rows"
          :columns="columns"
          row-key="id"
          :loading="loading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-hook_key="props">
            <q-td :props="props">
              <AppRegistryHoverTip :text="props.row.webhook_url" :empty-label="t('hooksPage.deliveries.noUrl')">
                <span class="app-registry-cell-primary ellipsis">{{ props.row.hook_key }}</span>
              </AppRegistryHoverTip>
            </q-td>
          </template>
          <template #body-cell-status="props">
            <q-td :props="props">
              <AppRegistryHoverTip
                :text="props.row.payload_json"
                :indicator="Boolean(String(props.row.payload_json ?? '').trim())"
              >
                <AppStatusChip :status="props.row.status" class="cursor-pointer" @click="openDetail(props.row)" />
              </AppRegistryHoverTip>
            </q-td>
          </template>
        </AppRegistryTable>

        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="total"
          :loading="loading"
          :label="t('hooksPage.deliveries.paginationLabel')"
        />
      </div>
    </template>

    <AppDialog v-model="detailOpen" size="sm" :title="t('hooksPage.deliveries.dialogTitle')">
      <div class="text-caption text-grey">{{ detailUrl }}</div>
      <JsonCodeViewer :text="detailText" />
      <div v-if="detailError" class="text-negative q-mt-sm">{{ detailError }}</div>
      <template #actions>
        <q-btn flat no-caps :label="t('hooksPage.deliveries.btnClose')" @click="detailOpen = false" />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import JsonCodeViewer from '../components/common/JsonCodeViewer.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';

import { createHookDeliveryTableColumns } from '../components/hooks/hookTableUi';
import { useHookDeliveriesPage } from '../features/hooks/useHookDeliveriesPage';
import { resolvePageState } from './pageState';

const { t } = useI18n();
const columns = createHookDeliveryTableColumns(t);

const {
  rows,
  total,
  loading,
  hookKey,
  status,
  from,
  to,
  error,
  forbidden,
  page,
  pageSize,
  pageMax,
  detailOpen,
  detailText,
  detailUrl,
  detailError,
  statusOptions,
  resetFilters,
  loadRows,
  onFilterChange,
  openDetail,
} = useHookDeliveriesPage();

/** 五态判定：错误优先于空态；有数据时刷新失败只加 partial 横幅，不清空已渲染列表。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
