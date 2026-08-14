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

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="$t('common.retry')" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <tool-audits-table :rows="rows" :loading="loading" />

    <app-registry-pagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      :label="$t('toolsPage.auditsPage.pageUnit')"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import ToolAuditsFilters from '../components/tools/ToolAuditsFilters.vue';
import ToolAuditsTable from '../components/tools/ToolAuditsTable.vue';
import ToolHeroSection from '../components/tools/ToolHeroSection.vue';
import { useToolAuditsPage } from '../features/tools/useToolAuditsPage';

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
  pageMax,
  statusOptions,
  loadRows,
  resetFilters,
} = useToolAuditsPage();
</script>
