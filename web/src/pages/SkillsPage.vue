<template>
  <q-page class="app-standard-page app-registry-page skills-page">
    <AppPageHero
      kicker="Skill registry"
      title="Skill 管理"
      subtitle="查看 Skill 使用频率、成功失败统计、最近调用 Agent，并维护启用状态。"
    >
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="upload_file" label="上传 Skill" @click="openUpload" />
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
      v-model:sync-origin="syncOrigin"
      v-model:filesystem-missing="filesystemMissing"
      :loading="loading"
      class="q-mb-md"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="psychology" />
        <div class="text-h6 q-mt-md">{{ search ? '没有匹配的 Skill' : '暂无 Skill' }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">点击右上角「上传」导入 ZIP，或先查看已有 Skill 与运行统计。</div>
      </q-card-section>
    </q-card>

    <template v-else>
      <skill-table
        :rows="rows"
        :loading="loading"
        :toggling-id="togglingId"
        :publishing-id="publishingId"
        @toggle-enabled="onToggleEnabled"
        @publish="onPublishSkill"
        @edit="openEditor"
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
import AppPageHero from '../components/layout/AppPageHero.vue';
import SkillDeleteDialog from '../components/skills/SkillDeleteDialog.vue';
import SkillEditorDialog from '../components/skills/SkillEditorDialog.vue';
import SkillFilesystemAlertBanner from '../components/skills/SkillFilesystemAlertBanner.vue';
import SkillFilterBar from '../components/skills/SkillFilterBar.vue';
import SkillPagination from '../components/skills/SkillPagination.vue';
import SkillTable from '../components/skills/SkillTable.vue';
import SkillUploadPlaceholder from '../components/skills/SkillUploadPlaceholder.vue';
import { useSkillsPage } from '../features/skills/useSkillsPage';

const {
  uploadRef,
  openUpload,
  search,
  enabled,
  status,
  syncOrigin,
  filesystemMissing,
  filesystemHealth,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  togglingId,
  publishingId,
  deleteOpen,
  deleteTarget,
  deleting,
  editorOpen,
  editorTarget,
  pageMax,
  loadRows,
  resetFilters,
  filterPendingFilesystem,
  filterMissingFilesystem,
  onPublishSkill,
  onToggleEnabled,
  openEditor,
  confirmDelete,
  deleteTargetSkill,
  uploadSkillZip,
  getSkillImportJob,
  refineSkillConflictGroup,
  applySkillImport,
  listSkillFiles,
  readSkillFile,
  updateSkillFile,
  notify,
  confirm,
} = useSkillsPage();
</script>
