<template>
  <q-page class="app-standard-page app-registry-page artifacts-page">
    <AppPageHero
      :kicker="t('artifact.page.kicker')"
      :title="t('artifact.page.title')"
      :subtitle="t('artifact.page.subtitle')"
    >
      <template #actions>
        <q-btn
          v-if="workspaceReturn"
          flat
          no-caps
          icon="arrow_back"
          :label="t('taskWorkspace.returnToWorkspace')"
          @click="returnToWorkspace"
        />
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="upload"
          :label="t('artifact.page.upload')"
          @click="uploadOpen = true"
        />
      </template>
    </AppPageHero>

    <q-tabs
      :model-value="activeTab"
      dense
      no-caps
      align="left"
      class="artifacts-page__tabs text-grey-8 q-mb-md"
      active-color="primary"
      indicator-color="primary"
      @update:model-value="handleTabChange"
    >
      <q-tab name="session" :label="t('artifact.page.tabSession')" />
      <q-tab name="all" :label="t('artifact.page.tabAll')" />
    </q-tabs>

    <AppPageToolbar>
      <q-select
        v-if="activeTab === 'session'"
        v-model="sessionFilter"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        use-input
        input-debounce="0"
        new-value-mode="add-unique"
        emit-value
        map-options
        :options="sessionFilteredOptions"
        :label="t('artifact.page.filterSession')"
        @filter="filterSessionOptions"
        @update:model-value="onSessionFilterChange"
      >
        <template #option="scope">
          <q-item v-bind="scope.itemProps">
            <q-item-section>
              <q-item-label>{{ scope.opt.label }}</q-item-label>
              <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
            </q-item-section>
          </q-item>
        </template>
      </q-select>
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="300"
        :label="t('artifact.page.searchPlaceholder')"
        @update:model-value="onSearchChange"
      />
      <q-select
        v-model="mimeFilter"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        :options="mimeFilterOptions"
        label="MIME"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('artifact.page.reset')" @click="resetFilters" />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('artifact.page.refresh')"
          :loading="loading"
          @click="loadRows"
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
      @action="loadRows"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="sessionFilterRequired"
      variant="empty"
      :title="t('artifact.page.sessionRequiredTitle')"
      :description="t('artifact.page.sessionRequiredHint')"
      icon="filter_alt"
    />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="t('artifact.page.emptyTitle')"
      :description="t('artifact.page.emptyHint')"
      icon="inventory_2"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="error"
        :action-label="t('common.retry')"
        action-icon="refresh"
        @action="loadRows"
      />
      <template v-for="group in tableGroups" :key="group.sessionId">
        <div v-if="group.showHeader" class="artifacts-page__group-header">
          <q-icon name="forum" size="16px" class="text-primary" />
          <span class="artifacts-page__group-title">
            {{ groupHeaderTitle(group.sessionId) }}
            <q-tooltip v-if="isTruncated(group.sessionId)">{{ group.sessionId }}</q-tooltip>
          </span>
          <span v-if="groupHeaderCaption(group.sessionId)" class="artifacts-page__group-caption">
            {{ groupHeaderCaption(group.sessionId) }}
          </span>
          <span class="text-caption text-grey-7">
            {{ t('artifact.page.groupPageStats', { count: group.items.length, size: formatBytes(group.totalSize) }) }}
          </span>
        </div>
        <AppRegistryTable
          :rows="group.items"
          :columns="columns"
          row-key="id"
          :loading="loading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
          :class="{ 'q-mb-lg': group.showHeader }"
        >
          <template #body-cell-name="props">
            <q-td :props="props">
              <div class="app-registry-cell-primary">{{ props.row.name }}</div>
            </q-td>
          </template>
          <template #body-cell-size="props">
            <q-td :props="props">{{ formatBytes(props.row.size) }}</q-td>
          </template>
          <template #body-cell-version="props">
            <q-td :props="props">v{{ props.row.version }}</q-td>
          </template>
          <template #body-cell-created_at="props">
            <q-td :props="props">{{ formatDate(props.row.created_at) }}</q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div class="app-registry-cell-actions">
                <q-btn flat dense round color="primary" icon="visibility" @click="openDetail(props.row)">
                  <q-tooltip>{{ t('artifact.page.view') }}</q-tooltip>
                </q-btn>
                <q-btn flat dense round color="primary" icon="download" @click="downloadRow(props.row)">
                  <q-tooltip>{{ t('artifact.page.signDownload') }}</q-tooltip>
                </q-btn>
                <q-btn flat dense round color="negative" icon="delete" @click="confirmDelete(props.row)">
                  <q-tooltip>{{ t('common.delete') }}</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
        </AppRegistryTable>
      </template>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="tableTotal"
        :loading="loading"
        :label="t('artifact.page.paginationLabel')"
        :page-size-options="[15, 30, 50]"
      />
    </template>

    <ArtifactsUploadDialog
      v-model:open="uploadOpen"
      v-model:file="uploadFile"
      v-model:session-id="uploadForm.session_id"
      v-model:name="uploadForm.name"
      v-model:mime-type="uploadForm.mime_type"
      :loading="uploadLoading"
      :max-size-hint="artifactMaxSizeHint()"
      :session-options="uploadSessionOptions"
      @submit="submitUpload"
    />

    <ArtifactsDetailDialog
      v-model:open="detailOpen"
      :meta="detailMeta"
      :artifact-id="detailArtifactId"
      :selected-version="detailVersion"
      :versions="detailVersions"
      :format-bytes="formatBytes"
      :reveal-enabled="revealEnabled"
      @select-version="selectDetailVersion"
      @delete-version="confirmDeleteVersion"
      @download="onPreviewDownload"
      @reveal="revealDetail"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import ArtifactsDetailDialog from '../components/artifact/ArtifactsDetailDialog.vue';
import ArtifactsUploadDialog from '../components/artifact/ArtifactsUploadDialog.vue';
import { useArtifactsPage, type ArtifactsPageTab } from '../features/artifact/useArtifactsPage';
import { resolvePageState } from './pageState';

const { t } = useI18n();

const {
  workspaceReturn,
  returnToWorkspace,
  loading,
  error,
  forbidden,
  activeTab,
  sessionFilterRequired,
  groupedRows,
  sessionFilter,
  search,
  uploadOpen,
  uploadLoading,
  uploadFile,
  uploadForm,
  detailOpen,
  detailMeta,
  detailArtifactId,
  detailVersions,
  detailVersion,
  revealEnabled,
  mimeFilter,
  mimeFilterOptions,
  sessionFilteredOptions,
  filterSessionOptions,
  uploadSessionOptions,
  groupHeaderTitle,
  groupHeaderCaption,
  columns,
  rows,
  tableTotal,
  page,
  pageSize,
  pageMax,
  formatBytes,
  formatDate,
  loadRows,
  onTabChange,
  onSearchChange,
  onSessionFilterChange,
  resetFilters,
  submitUpload,
  openDetail,
  revealDetail,
  selectDetailVersion,
  onPreviewDownload,
  downloadRow,
  confirmDelete,
  confirmDeleteVersion,
  artifactMaxSizeHint,
} = useArtifactsPage();

/** 五态判定：错误优先于空态；有数据时失败降级为 partial 横幅。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);

function handleTabChange(value: string | number) {
  onTabChange(value as ArtifactsPageTab);
}

/** 组头 tooltip：仅 UUID 被截断时展示完整值。 */
function isTruncated(sessionId: string) {
  return sessionId.length > 12;
}

/** 会话 Tab 单表渲染；全部 Tab 按 session 分组渲染（组头显示统计）。 */
const tableGroups = computed(() => {
  if (activeTab.value === 'session') {
    return [{ sessionId: '__session__', items: rows.value, totalSize: 0, showHeader: false }];
  }
  return groupedRows.value.map((g) => ({ ...g, showHeader: true }));
});
</script>

<style scoped>
.artifacts-page__tabs {
  border-bottom: 1px solid var(--color-border-soft);
}

.artifacts-page__group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 4px 0 8px;
}

.artifacts-page__group-title {
  font-weight: 600;
  font-size: 13px;
  word-break: break-all;
}

.artifacts-page__group-caption {
  font-size: 12px;
  color: var(--color-text-secondary);
}
</style>
