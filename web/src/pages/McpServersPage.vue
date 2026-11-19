<template>
  <q-page class="app-standard-page app-registry-page mcp-page">
    <AppPageHero :kicker="t('shell.mcp.kicker')" :title="t('shell.mcp.title')" :subtitle="t('shell.mcp.subtitle')">
      <template #actions>
        <q-btn color="primary" rounded unelevated icon="add" :label="t('shell.mcp.addServer')" @click="openCreate" />
        <q-btn
          outline
          rounded
          color="primary"
          icon="refresh"
          :label="t('shell.mcp.refresh')"
          :loading="loading"
          @click="loadRows(true)"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="200"
        :placeholder="t('shell.mcp.searchPlaceholder')"
        :aria-label="t('shell.mcp.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <div class="app-page-toolbar__meta">
        {{ t('shell.mcp.meta', { total, enabled: enabledCount }) }}
        <template v-if="usageSummaryText">
          ·
          <span :class="showUnusedWarning ? 'text-warning text-weight-medium' : 'text-grey-7'">
            {{ usageSummaryText }}
          </span>
        </template>
      </div>
    </AppPageToolbar>

    <q-banner v-if="showUnusedWarning" rounded dense class="app-notice app-notice--warning q-mb-md">
      <template #avatar>
        <q-icon name="warning_amber" color="warning" />
      </template>
      {{ t('shell.mcp.unusedWarning') }}
    </q-banner>

    <q-banner v-if="mcpGuideVisible" rounded dense class="app-notice app-notice--info q-mb-md">
      <template #avatar>
        <q-icon name="hub" color="primary" />
      </template>
      {{ t('shell.mcp.guide') }}
      <template #action>
        <q-btn flat dense round icon="close" :aria-label="t('shell.mcp.dismissGuide')" @click="dismissMcpGuide" />
      </template>
    </q-banner>

    <AppStatePanel
      v-if="pageState === 'loading'"
      variant="loading"
      :rows="5"
    />

    <AppStatePanel
      v-else-if="pageState === 'error'"
      variant="error"
      :title="error"
      :action-label="t('shell.common.retry')"
      action-icon="refresh"
      @action="loadRows(true)"
    />

    <AppStatePanel
      v-else-if="pageState === 'forbidden'"
      variant="forbidden"
    />

    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="t('shell.mcp.emptyTitle')"
      :description="t('shell.mcp.emptyHint')"
      :action-label="t('shell.mcp.emptyAction')"
      action-icon="add"
      @action="openCreate"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="error"
        :action-label="t('shell.common.retry')"
        action-icon="refresh"
        @action="loadRows(true)"
      />
      <McpServersTable
        :rows="pagedRows"
        :loading="loading"
        :testing-id="testingId"
        :toggling-id="togglingId"
        :health-tone="healthTone"
        :health-tooltip="healthTooltip"
        @edit="openEdit"
        @delete="confirmDelete"
        @test="testRow"
        @credentials="openCredentials"
        @toggle-enabled="toggleEnabled"
      />

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="t('shell.mcp.paginationUnit')"
        :page-size-options="[10, 20, 50]"
      />
    </template>

    <McpServerFormDialog v-model="editorOpen" :row="editingRow" @saved="onSaved" @tested="loadRows" />
    <McpUserCredentialDialog
      v-model="credDialogOpen"
      :mcp-server-id="credServer?.id ?? ''"
      :server-label="credServer?.name || credServer?.key || ''"
      :user-id="credUserId"
      :user-label="credUserLabel"
      @saved="loadRows"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import McpServersTable from '../components/mcp/McpServersTable.vue';
import McpServerFormDialog from '../features/mcp/McpServerFormDialog.vue';
import McpUserCredentialDialog from '../features/mcp/McpUserCredentialDialog.vue';
import { useMcpServersPage } from '../features/mcp/useMcpServersPage';
import { useMcpGuide } from '../features/mcp/useMcpGuide';
import { resolvePageState } from './pageState';

const { t } = useI18n();
const { mcpGuideVisible, dismissMcpGuide } = useMcpGuide();

const {
  search,
  loading,
  error,
  forbidden,
  testingId,
  togglingId,
  editorOpen,
  editingRow,
  credDialogOpen,
  credServer,
  credUserId,
  credUserLabel,
  enabledCount,
  filteredRows,
  usageSummaryText,
  showUnusedWarning,
  total,
  page,
  pageSize,
  pageMax,
  pagedRows,
  loadRows,
  openCreate,
  openEdit,
  openCredentials,
  onSaved,
  testRow,
  toggleEnabled,
  confirmDelete,
  healthTone,
  healthTooltip,
} = useMcpServersPage();

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: filteredRows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
