<template>
  <q-page class="app-standard-page app-registry-page skills-page">
    <AppPageHero
      kicker="Skill registry"
      title="Skill 管理"
      subtitle="查看 Skill 使用频率、成功失败统计、最近调用 Agent，并维护启用状态。"
    >
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="add" label="新建 Skill" @click="openCreate" />
        <q-btn outline rounded no-caps color="primary" icon="upload_file" label="上传 Skill" @click="openUpload" />
        <q-btn outline rounded no-caps color="primary" icon="sell" :label="$t('menu.skillTags')" to="/skills/tags" />
        <q-btn outline rounded no-caps color="primary" icon="history" label="运行记录" to="/skills/runs" />
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="assessment"
          label="经验报告"
          to="/skills/experience-reports"
        />
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="refresh"
          label="刷新"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageHero>

    <skill-upload-placeholder
      ref="uploadRef"
      :upload-skill-zip="uploadSkillZip"
      :get-skill-import-job="getSkillImportJob"
      :refine-skill-conflict-group="refineSkillConflictGroup"
      :apply-skill-import="applySkillImport"
      :notify="notify"
      @completed="loadRows"
    />

    <skill-filesystem-alert-banner
      :health="filesystemHealth"
      @filter-pending="filterPendingFilesystem"
      @filter-missing="filterMissingFilesystem"
    />

    <skill-filter-bar
      v-model:search="search"
      v-model:enabled="enabled"
      v-model:status="status"
      v-model:tags="selectedTags"
      v-model:sync-origin="syncOrigin"
      v-model:filesystem-missing="filesystemMissing"
      v-model:sort-by="sortBy"
      v-model:sort-order="sortOrder"
      :tag-options="tagOptions"
      :loading="loading"
      class="q-mb-md"
      @reset="resetFilters"
      @refresh="loadRows"
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
      :action-label="$t('shell.common.retry')"
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
      :title="search ? '没有匹配的 Skill' : '暂无 Skill'"
      description="点击「新建 Skill」手写草稿，或「上传」导入 ZIP。"
      action-label="新建 Skill"
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
        :action-label="$t('shell.common.retry')"
        action-icon="refresh"
        @action="loadRows"
      />
      <skill-table
        :rows="rows"
        :loading="loading"
        :toggling-id="togglingId"
        :publishing-id="publishingId"
        :publishing-ecosystem-id="publishingEcosystemId"
        :duplicating-id="duplicatingId"
        :ecosystem-publish-state="ecosystemPublishState"
        :load-health="loadSkillHealth"
        @toggle-enabled="onToggleEnabled"
        @publish="onPublishSkill"
        @publish-ecosystem="onPublishToEcosystem"
        @edit-meta="openMetaEditor"
        @edit-files="openEditor"
        @duplicate="onDuplicateSkill"
        @delete="confirmDelete"
      />

      <skill-pagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        label="条 Skill"
      />
    </template>
    <skill-delete-dialog v-model="deleteOpen" :skill="deleteTarget" :loading="deleting" @confirm="deleteTargetSkill" />
    <skill-ecosystem-publish-dialog
      v-model="ecosystemPublishOpen"
      :skill="ecosystemPublishTarget"
      :loading="!!publishingEcosystemId"
      @confirm="confirmPublishToEcosystem"
    />
    <skill-meta-dialog
      v-model="metaOpen"
      :skill="metaTarget"
      :initial-body="metaBody"
      :tag-options="tagOptions"
      :saving="metaSaving"
      :notify="notify"
      :confirm="confirm"
      @submit="onMetaSubmit"
    />
    <skill-editor-dialog
      v-model="editorOpen"
      :skill="editorTarget"
      :list-skill-files="listSkillFiles"
      :read-skill-file="readSkillFile"
      :update-skill-file="updateSkillFile"
      :notify="notify"
      :confirm="confirm"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import SkillDeleteDialog from '../components/skills/SkillDeleteDialog.vue';
import SkillEcosystemPublishDialog from '../components/skills/SkillEcosystemPublishDialog.vue';
import SkillEditorDialog from '../components/skills/SkillEditorDialog.vue';
import SkillMetaDialog from '../components/skills/SkillMetaDialog.vue';
import SkillFilesystemAlertBanner from '../components/skills/SkillFilesystemAlertBanner.vue';
import SkillFilterBar from '../components/skills/SkillFilterBar.vue';
import SkillPagination from '../components/skills/SkillPagination.vue';
import SkillTable from '../components/skills/SkillTable.vue';
import SkillUploadPlaceholder from '../components/skills/SkillUploadPlaceholder.vue';
import { useSkillsPage } from '../features/skills/useSkillsPage';
import { resolvePageState } from './pageState';

const {
  uploadRef,
  openUpload,
  openCreate,
  search,
  enabled,
  status,
  syncOrigin,
  selectedTags,
  tagOptions,
  filesystemMissing,
  filesystemHealth,
  sortBy,
  sortOrder,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  forbidden,
  togglingId,
  publishingId,
  publishingEcosystemId,
  duplicatingId,
  ecosystemPublishOpen,
  ecosystemPublishTarget,
  ecosystemPublishState,
  confirmPublishToEcosystem,
  deleteOpen,
  deleteTarget,
  deleting,
  editorOpen,
  editorTarget,
  metaOpen,
  metaTarget,
  metaBody,
  metaSaving,
  pageMax,
  loadRows,
  resetFilters,
  filterPendingFilesystem,
  filterMissingFilesystem,
  onPublishSkill,
  onPublishToEcosystem,
  onDuplicateSkill,
  onToggleEnabled,
  openEditor,
  openMetaEditor,
  onMetaSubmit,
  confirmDelete,
  deleteTargetSkill,
  uploadSkillZip,
  getSkillImportJob,
  refineSkillConflictGroup,
  applySkillImport,
  listSkillFiles,
  readSkillFile,
  updateSkillFile,
  loadSkillHealth,
  notify,
  confirm,
} = useSkillsPage();

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
