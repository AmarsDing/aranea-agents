<template>
  <q-page class="app-standard-page tool-runs-page">
    <tool-hero-section
      :kicker="$t('toolsPage.runsPage.kicker')"
      :title="$t('toolsPage.runsPage.title')"
      :subtitle="$t('toolsPage.runsPage.subtitle')"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          class="app-outline-btn"
          icon="arrow_back"
          :label="$t('toolsPage.runsPage.back')"
          :to="{ name: 'tools' }"
        />
      </template>
    </tool-hero-section>

    <tool-runs-filters
      :tool-key="toolKey"
      :agent-id="agentId"
      :session-id="sessionId"
      :status="status"
      :has-error="hasError"
      :from="from"
      :status-options="statusOptions"
      :loading="loading"
      @update:tool-key="toolKey = $event ?? ''"
      @update:agent-id="agentId = $event ?? ''"
      @update:session-id="sessionId = $event ?? ''"
      @update:status="status = $event ?? ''"
      @update:has-error="hasError = $event"
      @update:from="from = $event ?? ''"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="$t('common.retry')" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <tool-runs-table :rows="rows" :loading="loading" @view-detail="openDetail" />

    <app-registry-pagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      :label="$t('toolsPage.runsPage.pageUnit')"
    />

    <tool-run-detail-dialog
      :open="detailOpen"
      :invocation="detailRow"
      :params="detailParams"
      :params-loading="detailParamsLoading"
      :params-error="detailParamsError"
      @update:open="detailOpen = $event"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import ToolHeroSection from '../components/tools/ToolHeroSection.vue';
import ToolRunsFilters from '../components/tools/ToolRunsFilters.vue';
import ToolRunsTable from '../components/tools/ToolRunsTable.vue';
import ToolRunDetailDialog from '../components/tools/ToolRunDetailDialog.vue';
import { useToolRunsPage } from '../features/tools/useToolRunsPage';

const {
  toolKey,
  agentId,
  sessionId,
  status,
  hasError,
  from,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  pageMax,
  statusOptions,
  detailOpen,
  detailRow,
  detailParams,
  detailParamsLoading,
  detailParamsError,
  openDetail,
  resetFilters,
  loadRows,
} = useToolRunsPage();
</script>
