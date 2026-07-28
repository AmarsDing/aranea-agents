import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import type { Skill, SkillFilesystemHealth } from './types';
import { useSkillsStore } from '../../stores/skills';
// TECH-DEBT(FD5): file ops bypass store, acceptable for single-use file operations
import { listSkillFiles, readSkillFile, getSkill } from './api';

export function useSkillsPage() {
  const $q = useQuasar();
  const skillsStore = useSkillsStore();

  const uploadRef = ref<{ openDialog: () => void } | null>(null);
  const search = ref('');
  const enabled = ref<boolean | null>(null);
  const status = ref('');
  const syncOrigin = ref('');
  const selectedTags = ref<string[]>([]);
  const filesystemMissing = ref<boolean | null>(null);
  const filesystemHealth = ref<SkillFilesystemHealth | null>(null);
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<Skill[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  const togglingId = ref('');
  const publishingId = ref('');
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
      $q.notify({ type: 'positive', message: 'Skill 已发布；请在列表中打开「启用」以便 Agent 运行时挂载' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '发布失败' });
    } finally {
      publishingId.value = '';
    }
  }

  async function onToggleEnabled(skill: Skill, next: boolean) {
    togglingId.value = skill.id;
    try {
      const updated = await skillsStore.toggle(skill.id, next);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
      $q.notify({ type: 'positive', message: next ? 'Skill 已启用' : 'Skill 已停用' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '更新启用状态失败' });
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
      $q.notify({ type: 'positive', message: 'Skill 已删除' });
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

  watch([search, enabled, status, syncOrigin, selectedTags, filesystemMissing], () => {
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
    notify,
    confirm,
  };
}
