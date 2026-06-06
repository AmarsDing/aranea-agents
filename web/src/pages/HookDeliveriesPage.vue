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

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="t('hooksPage.deliveries.retry')" class="text-white" @click="() => loadRows()" />
      </template>
    </q-banner>

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
              <q-chip
                dense
                :color="statusColor(props.row.status)"
                text-color="white"
                class="cursor-pointer"
                @click="openDetail(props.row)"
              >
                {{ props.row.status }}
              </q-chip>
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

    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ t('hooksPage.deliveries.dialogTitle') }}</div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body">
            <div class="text-caption text-grey">{{ detailUrl }}</div>
            <pre class="hook-delivery-detail app-code-block">{{ detailText }}</pre>
            <div v-if="detailError" class="text-negative q-mt-sm">{{ detailError }}</div>
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat no-caps :label="t('hooksPage.deliveries.btnClose')" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';

import {
  createHookDeliveryTableColumns,
  hookDeliveryStatusColor as statusColor,
} from '../components/hooks/hookTableUi';
import { useHookDeliveriesPage } from '../features/hooks/useHookDeliveriesPage';

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
</script>
