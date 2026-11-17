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

    <AppStatePanel
      v-if="pageState === 'error'"
      class="q-mb-md"
      variant="error"
      :description="error"
      :action-label="$t('common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="$t('shell.runs.emptyTitle')"
      :description="$t('shell.runs.emptyDesc')"
      icon="outbox"
      :action-label="$t('shell.runs.emptyAction')"
      action-icon="restart_alt"
      @action="resetFilters"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="$t('shell.runs.staleError', { message: error })"
        :action-label="$t('common.retry')"
        action-icon="refresh"
        @action="loadRows"
      />

      <tool-runs-table :rows="rows" :loading="loading" @view-detail="openDetail" />

      <app-registry-pagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="$t('toolsPage.runsPage.pageUnit')"
      />
    </template>

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
import { computed } from 'vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import ToolHeroSection from '../components/tools/ToolHeroSection.vue';
import ToolRunsFilters from '../components/tools/ToolRunsFilters.vue';
import ToolRunsTable from '../components/tools/ToolRunsTable.vue';
import ToolRunDetailDialog from '../components/tools/ToolRunDetailDialog.vue';
import { useToolRunsPage } from '../features/tools/useToolRunsPage';
import { resolvePageState } from './pageState';

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
  forbidden,
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
