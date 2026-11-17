<template>
  <q-page class="app-standard-page tool-audits-page">
    <tool-hero-section
      :kicker="$t('toolsPage.auditsPage.kicker')"
      :title="$t('toolsPage.auditsPage.title')"
      :subtitle="$t('toolsPage.auditsPage.subtitle')"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          class="tool-audits-outline-btn"
          icon="arrow_back"
          :label="$t('toolsPage.auditsPage.back')"
          :to="{ name: 'tools' }"
        />
      </template>
    </tool-hero-section>

    <tool-audits-filters
      :tool-key="toolKey"
      :agent-id="agentId"
      :user-id="userId"
      :status="status"
      :status-options="statusOptions"
      :loading="loading"
      @update:tool-key="toolKey = $event ?? ''"
      @update:agent-id="agentId = $event ?? ''"
      @update:user-id="userId = $event ?? ''"
      @update:status="status = $event ?? ''"
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

      <tool-audits-table :rows="rows" :loading="loading" />

      <app-registry-pagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="$t('toolsPage.auditsPage.pageUnit')"
      />
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import ToolAuditsFilters from '../components/tools/ToolAuditsFilters.vue';
import ToolAuditsTable from '../components/tools/ToolAuditsTable.vue';
import ToolHeroSection from '../components/tools/ToolHeroSection.vue';
import { useToolAuditsPage } from '../features/tools/useToolAuditsPage';
import { resolvePageState } from './pageState';

const {
  toolKey,
  agentId,
  userId,
  status,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  forbidden,
  pageMax,
  statusOptions,
  loadRows,
  resetFilters,
} = useToolAuditsPage();

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
