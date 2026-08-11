import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { Skill, SkillFilesystemHealth } from './types';
import { useSkillsStore } from '../../stores/skills';
import { useEcosystemStore } from '../../stores/ecosystem';
// TECH-DEBT(FD5): file ops bypass store, acceptable for single-use file operations
import { listSkillFiles, readSkillFile, getSkill } from './api';

export function useSkillsPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const skillsStore = useSkillsStore();
  const ecosystemStore = useEcosystemStore();

  const uploadRef = ref<{ openDialog: () => void } | null>(null);
  const search = ref('');
  const enabled = ref<boolean | null>(null);
  const status = ref('');
  const syncOrigin = ref('');
  const selectedTags = ref<string[]>([]);
  const filesystemMissing = ref<boolean | null>(null);
  const filesystemHealth = ref<SkillFilesystemHealth | null>(null);
  /** 排序：默认按标签升序（用户需求：默认标签排序）。 */
  const sortBy = ref<'tag' | 'name' | ''>('tag');
  const sortOrder = ref<'asc' | 'desc'>('asc');
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<Skill[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  const togglingId = ref('');
  const publishingId = ref('');
  const publishingEcosystemId = ref('');
  const duplicatingId = ref('');
  /** 生态市场发布确认对话框 */
  const ecosystemPublishOpen = ref(false);
  const ecosystemPublishTarget = ref<Skill | null>(null);
  /** 本次会话内发布失败的 skill id（内存态，刷新即失效；已发布状态以市场 products 为准） */
  const ecosystemFailedIds = ref<Set<string>>(new Set());
  const deleteOpen = ref(false);
  const deleteTarget = ref<Skill | null>(null);
  const deleting = ref(false);
  const editorOpen = ref(false);
  const editorTarget = ref<Skill | null>(null);
  const metaOpen = ref(false);
  const metaTarget = ref<Skill | null>(null);
  const metaBody = ref('');
  const metaSaving = ref(false);

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  /** 标签字典选项源（MetaDialog / FilterBar 共用），首次打开时加载。 */
  const tagOptions = computed(() => skillsStore.tagNameOptions());
  function ensureTagOptionsLoaded() {
    void skillsStore.loadSkillTags().catch(() => {});
  }

  function notify(opts: { type: string; message: string }) {
    $q.notify(opts);
  }

  async function confirm(opts: {
    title: string;
    message: string;
    okLabel?: string;
    cancelLabel?: string;
    okColor?: string;
  }): Promise<boolean> {
    return new Promise((resolve) => {
      $q.dialog({
        title: opts.title,
        message: opts.message,
        cancel: opts.cancelLabel ? { label: opts.cancelLabel, flat: true, noCaps: true } : true,
        ok: { label: opts.okLabel ?? '确定', noCaps: true, color: opts.okColor ?? 'primary' },
        persistent: true,
      })
        .onOk(() => resolve(true))
        .onCancel(() => resolve(false));
    });
  }

  function openUpload() {
    uploadRef.value?.openDialog();
  }

  function openCreate() {
    ensureTagOptionsLoaded();
    metaTarget.value = null;
    metaBody.value = '';
    metaOpen.value = true;
  }

  async function openMetaEditor(skill: Skill) {
    ensureTagOptionsLoaded();
    metaTarget.value = skill;
    metaBody.value = '';
    metaOpen.value = true;
    try {
      const detail = await getSkill(skill.id);
      metaTarget.value = detail.skill;
      metaBody.value = detail.bodyMarkdown || '';
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '加载 Skill 失败' });
    }
  }

  async function onMetaSubmit(payload: {
    id?: string;
    name: string;
    slug: string;
    description: string;
    tags: string[];
    bodyMarkdown: string;
  }) {
    metaSaving.value = true;
    try {
      if (payload.id) {
        const updated = await skillsStore.update(payload.id, {
          name: payload.name,
          description: payload.description,
          tags: payload.tags,
          bodyMarkdown: payload.bodyMarkdown,
        });
        rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
        $q.notify({ type: 'positive', message: 'Skill 已保存' });
      } else {
        await skillsStore.create({
          name: payload.name,
          slug: payload.slug,
          description: payload.description,
          tags: payload.tags,
          bodyMarkdown: payload.bodyMarkdown,
        });
        $q.notify({ type: 'positive', message: 'Skill 草稿已创建' });
        await loadRows();
      }
      metaOpen.value = false;
      // 标签可能新增/变更，字典选项源缓存失效（下次打开时重新拉取）。
      skillsStore.invalidateSkillTags();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
    } finally {
      metaSaving.value = false;
    }
  }

  async function loadFilesystemHealth() {
    try {
      filesystemHealth.value = await skillsStore.loadFilesystemHealth();
    } catch {
      filesystemHealth.value = null;
    }
  }

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      const data = await skillsStore.loadSkills({
        search: search.value,
        enabled: enabled.value,
        status: status.value,
        sync_origin: syncOrigin.value || undefined,
        tags: selectedTags.value.length ? [...selectedTags.value] : undefined,
        filesystem_missing: filesystemMissing.value,
        sort_by: sortBy.value || undefined,
        sort_order: sortBy.value ? sortOrder.value : undefined,
        page: page.value,
        page_size: pageSize.value,
      });
      rows.value = data.items;
      total.value = data.total;
      await loadFilesystemHealth();
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 Skill 失败';
    } finally {
      loading.value = false;
    }
  }

  function resetFilters() {
    search.value = '';
    enabled.value = null;
    status.value = '';
    syncOrigin.value = '';
    selectedTags.value = [];
    filesystemMissing.value = null;
    sortBy.value = 'tag';
    sortOrder.value = 'asc';
    page.value = 1;
    void loadRows();
  }

  function filterPendingFilesystem() {
    syncOrigin.value = 'filesystem';
    status.value = 'draft';
    filesystemMissing.value = null;
    page.value = 1;
    void loadRows();
  }

  function filterMissingFilesystem() {
    filesystemMissing.value = true;
    page.value = 1;
    void loadRows();
  }

  async function onPublishSkill(skill: Skill) {
    publishingId.value = skill.id;
    try {
      const updated = await skillsStore.publish(skill.id);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
      $q.notify({ type: 'positive', message: 'Skill 已启用；如需运行时挂载请再打开「启用」开关' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '启用失败' });
    } finally {
      publishingId.value = '';
    }
  }

  /** 复制 Skill（FN-2：后端/store 已就绪，此处接 UI 事件）。 */
  async function onDuplicateSkill(skill: Skill) {
    duplicatingId.value = skill.id;
    try {
      const copy = await skillsStore.duplicate(skill.id);
      $q.notify({ type: 'positive', message: t('skillsPage.duplicateOk', { name: copy.name }) });
      await loadRows();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('skillsPage.duplicateFailed') });
    } finally {
      duplicatingId.value = '';
    }
  }

  /** 发布到生态市场（区别于生命周期启用：上架为 ecosystem product）。 */
  type EcosystemPublishState = 'published' | 'failed' | 'unpublished';

  /** 市场上已上架的 product 名集合（product.name = skill.slug 兜底 skill.name）。 */
  const ecosystemPublishedNames = computed(() => new Set(ecosystemStore.products.map((p) => p.name)));

  function ecosystemPublishState(skill: Skill): EcosystemPublishState {
    if (ecosystemFailedIds.value.has(skill.id)) return 'failed';
    return ecosystemPublishedNames.value.has(skill.slug || skill.name) ? 'published' : 'unpublished';
  }

  function onPublishToEcosystem(skill: Skill) {
    ecosystemPublishTarget.value = skill;
    ecosystemPublishOpen.value = true;
  }

  async function confirmPublishToEcosystem() {
    const skill = ecosystemPublishTarget.value;
    if (!skill) return;
    publishingEcosystemId.value = skill.id;
    try {
      await ecosystemStore.publish({
        name: skill.slug || skill.name,
        display_name: skill.name,
        description: skill.description,
        type: 'skill',
      });
      ecosystemFailedIds.value.delete(skill.id);
      ecosystemFailedIds.value = new Set(ecosystemFailedIds.value);
      ecosystemPublishOpen.value = false;
      $q.notify({ type: 'positive', message: t('skillsPage.ecosystemPublishOk') });
    } catch (err) {
      ecosystemFailedIds.value = new Set(ecosystemFailedIds.value).add(skill.id);
      ecosystemPublishOpen.value = false;
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('skillsPage.ecosystemPublishFailed'),
      });
    } finally {
      publishingEcosystemId.value = '';
    }
  }

  async function onToggleEnabled(skill: Skill, next: boolean) {
    togglingId.value = skill.id;
    try {
      const updated = await skillsStore.toggle(skill.id, next);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
      $q.notify({
        type: 'positive',
        message: next ? t('skillsPage.toggleEnabledOk') : t('skillsPage.toggleDisabledOk'),
      });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('skillsPage.toggleFailed') });
    } finally {
      togglingId.value = '';
    }
  }

  function openEditor(skill: Skill) {
    editorTarget.value = skill;
    editorOpen.value = true;
  }

  function confirmDelete(skill: Skill) {
    deleteTarget.value = skill;
    deleteOpen.value = true;
  }

  async function deleteTargetSkill() {
    if (!deleteTarget.value) return;
    deleting.value = true;
    try {
      await skillsStore.remove(deleteTarget.value.id);
      deleteOpen.value = false;
      $q.notify({ type: 'positive', message: t('skillsPage.deletedOk') });
      await loadRows();
      if (rows.value.length === 0 && page.value > 1) {
        page.value = Math.max(1, page.value - 1);
        await loadRows();
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    } finally {
      deleting.value = false;
    }
  }

  watch([search, enabled, status, syncOrigin, selectedTags, filesystemMissing, sortBy, sortOrder], () => {
    if (page.value === 1) {
      void loadRows();
    } else {
      page.value = 1;
    }
  });
  watch([page, pageSize], () => {
    void loadRows();
  });

  onMounted(() => {
    ensureTagOptionsLoaded();
    void loadRows();
    // 拉取市场 products 以判定「已发布」状态；失败静默（按钮按未发布展示）。
    void ecosystemStore.load().catch(() => {});
  });

  return {
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
    uploadSkillZip: skillsStore.uploadSkillZip,
    getSkillImportJob: skillsStore.getSkillImportJob,
    refineSkillConflictGroup: skillsStore.refineSkillConflictGroup,
    applySkillImport: skillsStore.applySkillImport,
    listSkillFiles,
    readSkillFile,
    updateSkillFile: skillsStore.updateSkillFile,
    loadSkillHealth: skillsStore.loadSkillHealth,
    notify,
    confirm,
  };
}
