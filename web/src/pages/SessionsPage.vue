<template>
  <q-page class="app-page-cream app-registry-page">
    <SessionsPageHero :loading="loading" @refresh="loadRows" />

    <SessionsSummaryCards :cards="summaryCards" />

    <SessionsFilterBar
      :keyword="keyword"
      :owner-type="ownerType"
      :status="status"
      :context-status="contextStatus"
      :loading="loading"
      :selection-mode="selectionMode"
      :owner-options="ownerFilterOptions"
      :status-options="statusFilterOptions"
      :context-options="contextFilterOptions"
      @update:keyword="onKeywordUpdate"
      @update:owner-type="ownerType = $event"
      @update:status="status = $event"
      @update:context-status="contextStatus = $event"
      @reset="resetFilters"
      @search="loadRows"
      @toggle-selection="toggleSelectionMode"
      @retention-archive="openRetention('archive')"
      @retention-delete="openRetention('delete')"
    />

    <SessionsBulkProgressBar :progress="bulkProgress" />

    <SessionsBulkSelectionBar
      v-if="selectionMode && selectedCount > 0"
      :count="selectedCount"
      :archiving="bulkArchiving"
      :deleting="bulkDeleting"
      @archive-selected="archiveSelected()"
      @delete-selected="promptDeleteSelected()"
    />

    <SessionsErrorBanner v-if="error" :message="error" @retry="loadRows" />

    <SessionsSelectedDetail
      v-if="selected"
      :session="selected"
      @archive="archiveSelectedDetail"
      @toggle-pin="togglePinSelectedDetail"
      @export="exportSelectedDetail"
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
      @update:page="page = $event"
      @update:page-size="pageSize = $event"
      @toggle-row="toggleRowSelection"
      @toggle-page="togglePageSelection"
      @archive-row="archiveRow"
      @toggle-pin="togglePinRow"
      @delete-row="promptDelete([$event])"
    />

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
import { computed, onMounted } from "vue";
import SessionsBulkProgressBar from "../components/sessions/SessionsBulkProgressBar.vue";
import SessionsBulkSelectionBar from "../components/sessions/SessionsBulkSelectionBar.vue";
import SessionDeleteConfirmDialog from "../components/sessions/SessionDeleteConfirmDialog.vue";
import SessionRetentionDialog from "../components/sessions/SessionRetentionDialog.vue";
import SessionsErrorBanner from "../components/sessions/SessionsErrorBanner.vue";
import SessionsFilterBar from "../components/sessions/SessionsFilterBar.vue";
import SessionsPageHero from "../components/sessions/SessionsPageHero.vue";
import SessionsSelectedDetail from "../components/sessions/SessionsSelectedDetail.vue";
import SessionsSummaryCards from "../components/sessions/SessionsSummaryCards.vue";
import SessionsTableSection from "../components/sessions/SessionsTableSection.vue";
import {
  buildSessionsSummaryCards,
  contextFilterOptions,
  ownerFilterOptions,
  pageSizeSelectOptions,
  statusFilterOptions
} from "../components/sessions/sessionUi";
import { useSessionsPage } from "../features/session/useSessionsPage";
import { exportSession } from "../features/session/api";
import { downloadTextFile } from "../features/session/downloadExport";
import { useQuasar } from "quasar";

const {
  rows,
  selected,
  total,
  loading,
  error,
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
  bulkArchiving,
  bulkDeleting,
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
  promptDeleteSelected,
  archiveSelected
} = useSessionsPage();

const $q = useQuasar();

async function exportSelectedDetail(format: "markdown" | "json") {
  if (!selected.value) return;
  try {
    const payload = await exportSession(selected.value.id, format);
    downloadTextFile(payload.content, payload.filename, payload.content_type);
    $q.notify({ type: "positive", message: "导出成功" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "导出失败" });
  }
}

const summaryCards = computed(() => buildSessionsSummaryCards(rows.value, total.value));

onMounted(loadRows);

async function archiveSelectedDetail() {
  if (!selected.value) return;
  await archiveRow(selected.value.id);
}

async function togglePinSelectedDetail() {
  if (!selected.value) return;
  await togglePinRow(selected.value.id, !selected.value.pinned_at?.trim());
}
</script>
