<template>
  <q-page class="app-standard-page app-registry-page">
    <SessionsPageHero :loading="loading" @refresh="loadRows" />

    <SessionsSummaryCards :cards="summaryCards" />

    <SessionsFilterBar
      :keyword="keyword"
      :owner-type="ownerType"
      :status="status"
      :context-status="contextStatus"
      :selection-mode="selectionMode"
      :owner-options="ownerFilterOptions"
      :status-options="statusFilterOptions"
      :context-options="contextFilterOptions"
      @update:keyword="onKeywordUpdate"
      @update:owner-type="ownerType = $event"
      @update:status="status = $event"
      @update:context-status="contextStatus = $event"
      @reset="resetFilters"
      @toggle-selection="toggleSelectionMode"
      @retention-archive="openRetention('archive')"
      @retention-delete="openRetention('delete')"
    />

    <SessionsBulkProgressBar :progress="bulkProgress" />

    <BulkActionBar
      v-if="selectionMode && selectedCount > 0"
      :count="selectedCount"
      :total="total"
      :actions="bulkActions"
      :busy="bulkBusy"
      :error="bulkError"
      @run="runBulkAction"
      @clear="clearSelection"
    />

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
      @action="loadRows"
    />
    <AppStatePanel
      v-else-if="pageState === 'forbidden'"
      variant="forbidden"
    />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="t('sessionsPage.emptyTitle')"
      :description="t('sessionsPage.emptyHint')"
    />

    <div
      v-else
      class="sessions-workspace"
      :class="{ 'sessions-workspace--inspecting': sessionInspectorOpen && selected }"
    >
      <div class="sessions-workspace__main">
        <AppStatePanel
          v-if="pageState === 'partial'"
          class="q-mb-md"
          variant="partial"
          dense
          :description="error"
          :action-label="t('shell.common.retry')"
          action-icon="refresh"
          @action="loadRows"
        />
        <SessionsTableSection
          :rows="rows"
          :loading="loading"
          :page="page"
          :page-size="pageSize"
          :page-max="pageMax"
          :total="total"
          :page-size-options="pageSizeSelectOptions"
          :selection-mode="selectionMode"
          :is-row-selected="isRowSelected"
          :page-all-selected="isPageFullySelected()"
          @inspect="inspectSession"
          @update:page="page = $event"
          @update:page-size="pageSize = $event"
          @toggle-row="toggleRowSelection"
          @toggle-page="togglePageSelection"
          @archive-row="archiveRow"
          @toggle-pin="togglePinRow"
          @delete-row="promptDelete([$event])"
        />
      </div>
      <AppRegistryInspector
        v-model="sessionInspectorOpen"
        :title="selected?.title || t('shell.registry.inspect')"
        :eyebrow="selected ? `SESSION / ${selected.id}` : undefined"
      >
        <SessionsSelectedDetail
          v-if="selected"
          :session="selected"
          @archive="archiveSelectedDetail"
          @toggle-pin="togglePinSelectedDetail"
          @export="exportSelectedDetail"
        />
      </AppRegistryInspector>
    </div>

    <SessionDeleteConfirmDialog
      v-model="deleteDialogOpen"
      :count="deleteTargetIds.length"
      :loading="deleteLoading"
      @confirm="confirmDeleteDialog"
    />

    <SessionRetentionDialog
      v-model="retentionOpen"
      :mode="retentionMode"
      :preview="retentionPreview"
      :preview-loading="retentionPreviewLoading"
      :loading="retentionLoading"
      @preview="previewRetention"
      @confirm="confirmRetention"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import BulkActionBar from '../components/common/BulkActionBar.vue';
import SessionsBulkProgressBar from '../components/sessions/SessionsBulkProgressBar.vue';
import SessionDeleteConfirmDialog from '../components/sessions/SessionDeleteConfirmDialog.vue';
import SessionRetentionDialog from '../components/sessions/SessionRetentionDialog.vue';
import SessionsFilterBar from '../components/sessions/SessionsFilterBar.vue';
import SessionsPageHero from '../components/sessions/SessionsPageHero.vue';
import SessionsSelectedDetail from '../components/sessions/SessionsSelectedDetail.vue';
import SessionsSummaryCards from '../components/sessions/SessionsSummaryCards.vue';
import SessionsTableSection from '../components/sessions/SessionsTableSection.vue';
import {
  buildSessionsSummaryCards,
  contextFilterOptions,
  ownerFilterOptions,
  pageSizeSelectOptions,
  statusFilterOptions,
} from '../components/sessions/sessionUi';
import { useSessionsPage } from '../features/session/useSessionsPage';
import type { Session } from '../features/session/types';
import { resolvePageState } from './pageState';

const { t } = useI18n();
const sessionInspectorOpen = ref(false);

const {
  rows,
  selected,
  total,
  loading,
  error,
  forbidden,
  keyword,
  ownerType,
  status,
  contextStatus,
  page,
  pageSize,
  pageMax,
  selectionMode,
  selectedCount,
  bulkProgress,
  bulkActions,
  bulkBusy,
  bulkError,
  runBulkAction,
  clearSelection,
  deleteDialogOpen,
  deleteTargetIds,
  deleteLoading,
  retentionOpen,
  retentionMode,
  retentionPreview,
  retentionPreviewLoading,
  retentionLoading,
  onKeywordUpdate,
  resetFilters,
  loadRows,
  selectSession,
  toggleSelectionMode,
  toggleRowSelection,
  togglePageSelection,
  isRowSelected,
  isPageFullySelected,
  promptDelete,
  confirmDeleteDialog,
  openRetention,
  previewRetention,
  confirmRetention,
  archiveRow,
  togglePinRow,
  exportSelectedDetail,
} = useSessionsPage();

const summaryCards = computed(() => buildSessionsSummaryCards(rows.value, total.value));

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);

onMounted(loadRows);
watch(selected, (value) => {
  if (value) sessionInspectorOpen.value = true;
});

function inspectSession(session: Session) {
  selectSession(session);
  sessionInspectorOpen.value = true;
}

async function archiveSelectedDetail() {
  if (!selected.value) return;
  await archiveRow(selected.value.id);
}

async function togglePinSelectedDetail() {
  if (!selected.value) return;
  await togglePinRow(selected.value.id, !selected.value.pinned_at?.trim());
}
</script>
