import { computed, onMounted, ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useSkillsStore } from '../../stores/skills';
import type { SkillTagInfo } from './types';

/**
 * SkillTagsPage 页面编排：store 调用、dialog/notify、标签名校验、后端错误文案中文化映射。
 * Page 只做模板绑定。
 */
export function useSkillTagsPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const skillsStore = useSkillsStore();

  const search = ref('');
  const sourceFilter = ref<string | null>(null);
  const createOpen = ref(false);
  const createName = ref('');
  const createError = ref('');
  const creating = ref(false);
  const renameOpen = ref(false);
  const renameTarget = ref<SkillTagInfo | null>(null);
  const renameName = ref('');
  const renameError = ref('');
  const renaming = ref(false);

  const tagsLoading = computed(() => skillsStore.tagsLoading);

  const sourceOptions = computed(() => [
    { label: t('skillTags.sourceDict'), value: 'dict' },
    { label: t('skillTags.sourceOrphan'), value: 'orphan' },
    { label: t('skillTags.sourceSystem'), value: 'system' },
  ]);

  const filteredTags = computed(() => {
    const kw = search.value.trim().toLowerCase();
    return skillsStore.skillTags.filter((tag) => {
      if (kw && !tag.name.toLowerCase().includes(kw) && !tag.dimension.toLowerCase().includes(kw)) return false;
      if (sourceFilter.value === 'orphan') return tag.source === 'orphan';
      if (sourceFilter.value === 'system') return tag.source === 'system';
      if (sourceFilter.value === 'dict') return tag.source !== 'orphan';
      return true;
    });
  });

  const orphanCount = computed(() => skillsStore.skillTags.filter((tag) => tag.source === 'orphan').length);

  const filteredGroups = computed(() => {
    const byDim = new Map<string, SkillTagInfo[]>();
    for (const tag of filteredTags.value) {
      const list = byDim.get(tag.dimension) ?? [];
      list.push(tag);
      byDim.set(tag.dimension, list);
    }
    return [...byDim.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([dimension, items]) => ({
        dimension,
        label: dimension ? t('skillTags.groupDimension', { dimension }) : t('skillTags.groupGeneral'),
        items,
      }));
  });

  // 与后端 normalizeTagName 同一规则：小写字母/数字开头，可用 _ -，可选维度前缀。
  const TAG_NAME_PATTERN = /^[a-z0-9][a-z0-9_-]*(:[a-z0-9][a-z0-9_-]*)?$/;

  function errMessage(err: unknown, fallback: string): string {
    const msg = err instanceof Error ? err.message : '';
    if (!msg) return fallback;
    // 后端标签校验错误中文化映射（后端错误消息统一英文，见 biz/skill/tag.go）。
    if (msg.includes('tag name must match')) return t('skillTags.nameInvalid');
    if (msg.includes('tag name is empty')) return t('skillTags.nameRequired');
    if (msg.includes('exceeds 128')) return t('skillTags.nameTooLong');
    if (msg.includes('already exists') || msg.toLowerCase().includes('conflict')) return t('skillTags.nameConflict');
    return msg;
  }

  async function reload() {
    try {
      await skillsStore.loadSkillTags(true);
    } catch (err) {
      $q.notify({ type: 'negative', message: errMessage(err, t('skillTags.loadFailed')) });
    }
  }

  function openCreate() {
    createName.value = '';
    createError.value = '';
    createOpen.value = true;
  }

  async function onCreate() {
    const name = createName.value.trim();
    if (!name) {
      createError.value = t('skillTags.nameRequired');
      return;
    }
    if (!TAG_NAME_PATTERN.test(name.toLowerCase())) {
      createError.value = t('skillTags.nameInvalid');
      return;
    }
    creating.value = true;
    createError.value = '';
    try {
      await skillsStore.createTag(name);
      createOpen.value = false;
      $q.notify({ type: 'positive', message: t('skillTags.created') });
    } catch (err) {
      createError.value = errMessage(err, t('skillTags.createFailed'));
    } finally {
      creating.value = false;
    }
  }

  /** 收录 orphan 标签：等价于以同名预建。 */
  async function onAdopt(tag: SkillTagInfo) {
    try {
      await skillsStore.createTag(tag.name);
      $q.notify({ type: 'positive', message: t('skillTags.adopted', { name: tag.name }) });
    } catch (err) {
      $q.notify({ type: 'negative', message: errMessage(err, t('skillTags.adoptFailed')) });
    }
  }

  function openRename(tag: SkillTagInfo) {
    renameTarget.value = tag;
    renameName.value = tag.name;
    renameError.value = '';
    renameOpen.value = true;
  }

  async function onRename() {
    const target = renameTarget.value;
    if (!target) return;
    const name = renameName.value.trim();
    if (!name) {
      renameError.value = t('skillTags.newNameRequired');
      return;
    }
    if (!TAG_NAME_PATTERN.test(name.toLowerCase())) {
      renameError.value = t('skillTags.nameInvalid');
      return;
    }
    renaming.value = true;
    renameError.value = '';
    try {
      const rewritten = await skillsStore.renameTag(target.name, name);
      renameOpen.value = false;
      $q.notify({ type: 'positive', message: t('skillTags.renamed', { count: rewritten }) });
    } catch (err) {
      renameError.value = errMessage(err, t('skillTags.renameFailed'));
    } finally {
      renaming.value = false;
    }
  }

  function onDelete(tag: SkillTagInfo) {
    const message =
      tag.used_count > 0
        ? t('skillTags.deleteConfirmUsed', { name: tag.name, count: tag.used_count })
        : t('skillTags.deleteConfirmUnused', { name: tag.name });
    $q.dialog({
      title: t('skillTags.deleteTitle'),
      message,
      cancel: { label: t('common.cancel'), flat: true, noCaps: true },
      ok: { label: t('common.delete'), noCaps: true, color: 'negative' },
      persistent: true,
    }).onOk(async () => {
      try {
        const rewritten = await skillsStore.deleteTag(tag.name);
        $q.notify({
          type: 'positive',
          message: rewritten > 0 ? t('skillTags.deletedWithRewrite', { count: rewritten }) : t('skillTags.deleted'),
        });
      } catch (err) {
        $q.notify({ type: 'negative', message: errMessage(err, t('skillTags.deleteFailed')) });
      }
    });
  }

  onMounted(reload);

  return {
    search,
    sourceFilter,
    sourceOptions,
    tagsLoading,
    filteredTags,
    orphanCount,
    filteredGroups,
    createOpen,
    createName,
    createError,
    creating,
    renameOpen,
    renameTarget,
    renameName,
    renameError,
    renaming,
    reload,
    openCreate,
    onCreate,
    onAdopt,
    openRename,
    onRename,
    onDelete,
  };
}
