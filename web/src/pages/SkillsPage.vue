<template>
  <q-page class="app-standard-page cap-page skills-page">
    <!-- 标签字典、运行记录、进化建议、经验报告都在子页签里，这里只留新建与上传 -->
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="loadRows"
      />
      <q-btn flat dense no-caps icon="upload_file" :label="t('cap.skills.upload')" @click="openUpload" />
      <q-btn unelevated dense no-caps color="primary" icon="add" :label="t('cap.skills.create')" @click="openCreate" />
    </Teleport>

    <skill-upload-placeholder
      ref="uploadRef"
      :upload-skill-zip="uploadSkillZip"
      :get-skill-import-job="getSkillImportJob"
      :refine-skill-conflict-group="refineSkillConflictGroup"
      :apply-skill-import="applySkillImport"
      :notify="notify"
      @completed="loadRows"
    />

    <CapStats :stats="stats" :label="t('cap.common.statsLabel')" :active="activeStat" @pick="onPickStat" />

    <skill-filesystem-alert-banner
      :health="filesystemHealth"
      @filter-pending="filterPendingFilesystem"
      @filter-missing="filterMissingFilesystem"
    />

    <div class="cap-bar" role="search">
      <q-input
        v-model="search"
        class="cap-bar__search"
        dense
        outlined
        clearable
        debounce="350"
        :placeholder="t('cap.skills.searchPlaceholder')"
        :aria-label="t('cap.skills.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" size="18px" /></template>
      </q-input>
      <div class="cap-seg" role="group" :aria-label="t('cap.skills.filters.status')">
        <button
          v-for="opt in statusSegments"
          :key="opt.value"
          type="button"
          :aria-pressed="status === opt.value"
          @click="status = opt.value"
        >
          {{ opt.label }}
        </button>
      </div>
      <q-select
        v-model="selectedTags"
        dense
        outlined
        clearable
        multiple
        use-chips
        use-input
        input-debounce="0"
        options-dense
        :label="t('cap.skills.filters.tags')"
        :options="filteredTagOptions"
        @filter="onTagFilter"
      />
      <q-btn flat dense no-caps icon="tune" :label="t('cap.skills.filters.more')">
        <q-badge v-if="moreFilterCount" floating rounded color="primary">{{ moreFilterCount }}</q-badge>
        <q-menu anchor="bottom right" self="top right">
          <div class="cap-filter-menu">
            <q-select
              v-model="enabled"
              dense
              outlined
              clearable
              emit-value
              map-options
              :label="t('cap.skills.filters.enabled')"
              :options="enabledOptions"
            />
            <q-select
              v-model="syncOrigin"
              dense
              outlined
              clearable
              emit-value
              map-options
              :label="t('cap.skills.filters.origin')"
              :options="originOptions"
            />
            <q-select
              v-model="filesystemMissing"
              dense
              outlined
              clearable
              emit-value
              map-options
              :label="t('cap.skills.filters.disk')"
              :options="diskOptions"
            />
            <div class="cap-filter-menu__row">
              <q-select
                v-model="sortBy"
                dense
                outlined
                emit-value
                map-options
                :label="t('cap.skills.filters.sort')"
                :options="sortOptions"
              />
              <q-btn
                flat
                dense
                round
                :icon="sortOrder === 'asc' ? 'arrow_upward' : 'arrow_downward'"
                :aria-label="sortOrder === 'asc' ? $t('skillsPage.sortAscTip') : $t('skillsPage.sortDescTip')"
                @click="sortOrder = sortOrder === 'asc' ? 'desc' : 'asc'"
              />
            </div>
          </div>
        </q-menu>
      </q-btn>
      <span class="cap-bar__spacer" />
      <q-btn
        v-if="filtersActive"
        flat
        dense
        no-caps
        icon="filter_alt_off"
        :label="t('cap.common.reset')"
        @click="resetFilters"
      />
    </div>

    <AppStatePanel v-if="pageState === 'loading'" variant="loading" :rows="5" />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      variant="error"
      :title="error"
      :action-label="$t('shell.common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <AppStatePanel
      v-else-if="pageState === 'empty' && filtersActive"
      variant="empty"
      icon="school"
      :title="t('cap.skills.emptyFiltered')"
      :action-label="t('cap.common.reset')"
      action-icon="filter_alt_off"
      @action="resetFilters"
    />
    <!-- 一个技能都没有：讲清楚技能是什么、三种加进来的方式 -->
    <section v-else-if="pageState === 'empty'" class="cap-section" :aria-label="t('cap.skills.emptyTitle')">
      <div class="cap-section__head">
        <span class="cap-card__icon" aria-hidden="true"><q-icon name="school" size="18px" /></span>
        <div class="cap-section__text">
          <h2 class="cap-section__title">{{ t('cap.skills.emptyTitle') }}</h2>
          <p class="cap-section__hint">{{ t('cap.skills.emptyDesc') }}</p>
        </div>
      </div>
      <ol class="cap-ways" role="list">
        <li>
          <q-icon name="edit_note" size="22px" aria-hidden="true" />
          <strong>{{ t('cap.skills.wayWrite') }}</strong>
          <span>{{ t('cap.skills.wayWriteDesc') }}</span>
          <q-btn
            unelevated
            dense
            no-caps
            color="primary"
            icon="add"
            :label="t('cap.skills.create')"
            @click="openCreate"
          />
        </li>
        <li>
          <q-icon name="upload_file" size="22px" aria-hidden="true" />
          <strong>{{ t('cap.skills.wayUpload') }}</strong>
          <span>{{ t('cap.skills.wayUploadDesc') }}</span>
          <q-btn
            outline
            dense
            no-caps
            color="primary"
            icon="upload_file"
            :label="t('cap.skills.upload')"
            @click="openUpload"
          />
        </li>
        <li>
          <q-icon name="folder_special" size="22px" aria-hidden="true" />
          <strong>{{ t('cap.skills.wayFolder') }}</strong>
          <span>{{ t('cap.skills.wayFolderDesc') }}</span>
          <code v-if="filesystemHealth?.resolved_root" class="cap-mono cap-ways__path">{{
            filesystemHealth.resolved_root
          }}</code>
        </li>
      </ol>
    </section>

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
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
        :total="total"
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

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="t('cap.skills.pageUnit')"
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
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapTone } from '../components/capability/capTypes';
import SkillDeleteDialog from '../components/skills/SkillDeleteDialog.vue';
import SkillEcosystemPublishDialog from '../components/skills/SkillEcosystemPublishDialog.vue';
import SkillEditorDialog from '../components/skills/SkillEditorDialog.vue';
import SkillMetaDialog from '../components/skills/SkillMetaDialog.vue';
import SkillFilesystemAlertBanner from '../components/skills/SkillFilesystemAlertBanner.vue';
import SkillTable from '../components/skills/SkillTable.vue';
import SkillUploadPlaceholder from '../components/skills/SkillUploadPlaceholder.vue';
import { useSkillsPage } from '../features/skills/useSkillsPage';
import { resolvePageState } from './pageState';

const { t } = useI18n();

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

const statusSegments = computed(() => [
  { value: '', label: t('cap.common.all') },
  { value: 'draft', label: t('cap.skills.status.draft') },
  { value: 'published', label: t('cap.skills.status.published') },
  { value: 'archived', label: t('cap.skills.status.archived') },
]);

// 标签：字典选项按输入过滤；已选中但字典里没有的标签也保留，免得回显丢失
const tagNeedle = ref('');
const filteredTagOptions = computed(() => {
  const all = new Set([...(tagOptions.value ?? []), ...selectedTags.value]);
  const kw = tagNeedle.value.trim().toLowerCase();
  return [...all].sort().filter((tag) => !kw || tag.toLowerCase().includes(kw));
});
function onTagFilter(val: string, update: (fn: () => void) => void) {
  update(() => {
    tagNeedle.value = val;
  });
}

const enabledOptions = computed(() => [
  { label: t('skillsPage.filterEnabledOnly'), value: true },
  { label: t('skillsPage.filterDisabledOnly'), value: false },
]);
const originOptions = computed(() => [
  { label: t('skillsPage.originFilesystem'), value: 'filesystem' },
  { label: t('skillsPage.originImport'), value: 'import' },
  { label: t('skillsPage.originManual'), value: 'manual' },
]);
const diskOptions = computed(() => [
  { label: t('skillsPage.filterFsMissing'), value: true },
  { label: t('skillsPage.filterFsOk'), value: false },
]);
const sortOptions = computed(() => [
  { label: t('skillsPage.sortByTag'), value: 'tag' },
  { label: t('skillsPage.sortByName'), value: 'name' },
]);

// 「更多筛选」里有几项不是默认值（排序不算筛选）
const moreFilterCount = computed(
  () => Number(enabled.value !== null) + Number(Boolean(syncOrigin.value)) + Number(filesystemMissing.value !== null),
);
const filtersActive = computed(
  () => Boolean(search.value || status.value || selectedTags.value.length) || moreFilterCount.value > 0,
);

// 列表接口不带汇总，这里只放拿得到的数：总数来自分页，其余来自技能目录体检
const stats = computed(() => {
  const h = filesystemHealth.value;
  const rootTone: CapTone = !h ? 'pending' : h.root_accessible ? 'ok' : 'alert';
  return [
    {
      key: 'total',
      label: t(filtersActive.value ? 'cap.skills.stats.matched' : 'cap.skills.stats.total'),
      value: total.value,
    },
    {
      key: 'pending',
      label: t('cap.skills.stats.pending'),
      value: h ? h.pending_filesystem_count : '—',
      tone: (h && h.pending_filesystem_count > 0 ? 'watch' : undefined) as CapTone | undefined,
      hint: t('cap.skills.stats.pendingHint'),
      clickable: Boolean(h && h.pending_filesystem_count > 0),
    },
    {
      key: 'missing',
      label: t('cap.skills.stats.missing'),
      value: h ? h.missing_count : '—',
      tone: (h && h.missing_count > 0 ? 'alert' : undefined) as CapTone | undefined,
      hint: t('cap.skills.stats.missingHint'),
      clickable: Boolean(h && h.missing_count > 0),
    },
    {
      key: 'root',
      label: t('cap.skills.stats.root'),
      value: !h ? '—' : h.root_accessible ? t('cap.skills.stats.rootOk') : t('cap.skills.stats.rootBad'),
      tone: rootTone,
      hint: h?.resolved_root,
    },
  ];
});

const activeStat = computed(() => {
  if (filesystemMissing.value === true) return 'missing';
  if (syncOrigin.value === 'filesystem' && status.value === 'draft') return 'pending';
  return '';
});

function onPickStat(key: string) {
  if (key === activeStat.value) {
    resetFilters();
    return;
  }
  if (key === 'pending') filterPendingFilesystem();
  if (key === 'missing') filterMissingFilesystem();
}

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
