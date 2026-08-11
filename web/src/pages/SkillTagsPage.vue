<template>
  <q-page class="app-standard-page app-registry-page skill-tags-page">
    <AppPageHero kicker="Skill registry" :title="$t('skillTags.title')" :subtitle="$t('skillTags.subtitle')">
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="add" :label="$t('skillTags.create')" @click="openCreate" />
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="refresh"
          :label="$t('common.refresh')"
          :loading="tagsLoading"
          @click="reload"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar class="skill-tags-filter">
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="250"
        :label="$t('skillTags.searchLabel')"
        :placeholder="$t('skillTags.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="sourceFilter"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="$t('skillTags.sourceFilter')"
        :options="sourceOptions"
      />
      <template #actions>
        <span class="skill-tags-stat">
          <q-icon name="sell" size="13px" />
          {{ $t('skillTags.totalChip', { count: filteredTags.length }) }}
        </span>
        <span v-if="orphanCount > 0" class="skill-tags-stat skill-tags-stat--warning">
          <q-icon name="warning_amber" size="13px" />
          {{ $t('skillTags.orphanChip', { count: orphanCount }) }}
        </span>
      </template>
    </AppPageToolbar>

    <q-card v-if="!tagsLoading && filteredGroups.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="sell" />
        <div class="text-h6 q-mt-md">
          {{ search ? $t('skillTags.emptyTitleSearch') : $t('skillTags.emptyTitle') }}
        </div>
        <div class="text-body2 text-grey-7 q-mt-sm">{{ $t('skillTags.emptyHint') }}</div>
      </q-card-section>
    </q-card>

    <template v-else>
      <section v-for="group in filteredGroups" :key="group.dimension" class="skill-tags-group q-mb-md">
        <div class="skill-tags-group__header row items-center q-gutter-sm q-mb-sm">
          <q-icon :name="group.dimension ? 'folder_open' : 'label_important'" color="primary" size="20px" />
          <span class="text-subtitle1 text-weight-medium">{{ group.label }}</span>
          <span class="skill-tags-count">{{ group.items.length }}</span>
        </div>
        <q-card flat class="app-glass-panel">
          <q-list separator>
            <q-item
              v-for="tag in group.items"
              :key="tag.name"
              :class="{ 'skill-tag-row--orphan': tag.source === 'orphan' }"
            >
              <q-item-section>
                <q-item-label class="row items-center q-gutter-xs">
                  <span class="skill-tag-pill" :class="{ 'skill-tag-pill--system': tag.source === 'system' }">
                    {{ tag.name }}
                  </span>
                  <span v-if="tag.source === 'system'" class="skill-tag-badge">
                    {{ $t('skillTags.badgeSystem') }}
                  </span>
                  <span v-else-if="tag.source === 'orphan'" class="skill-tag-badge skill-tag-badge--warning">
                    {{ $t('skillTags.badgeOrphan') }}
                  </span>
                </q-item-label>
                <q-item-label v-if="tag.source === 'orphan'" caption class="q-mt-xs">
                  <span class="skill-tag-row__orphan-hint">{{ $t('skillTags.orphanHint') }}</span>
                </q-item-label>
              </q-item-section>
              <q-item-section side class="skill-tag-row__usage">
                <span class="skill-tag-row__usage-text">
                  {{ $t('skillTags.usageCount', { count: tag.used_count }) }}
                </span>
              </q-item-section>
              <q-item-section side>
                <div class="row q-gutter-xs">
                  <q-btn
                    v-if="tag.source === 'orphan'"
                    flat
                    dense
                    round
                    icon="library_add"
                    color="positive"
                    @click="onAdopt(tag)"
                  >
                    <q-tooltip>{{ $t('skillTags.adoptTooltip') }}</q-tooltip>
                  </q-btn>
                  <q-btn flat dense round icon="edit" color="primary" @click="openRename(tag)">
                    <q-tooltip>{{ $t('skillTags.renameTooltip') }}</q-tooltip>
                  </q-btn>
                  <q-btn flat dense round icon="delete" color="negative" @click="onDelete(tag)">
                    <q-tooltip>{{ $t('skillTags.deleteTooltip') }}</q-tooltip>
                  </q-btn>
                </div>
              </q-item-section>
            </q-item>
          </q-list>
        </q-card>
      </section>
    </template>

    <!-- 新建标签 -->
    <q-dialog v-model="createOpen" persistent>
      <q-card class="skill-tags-dialog-card app-dialog-card app-dialog-card--sm">
        <q-card-section class="text-h6">{{ $t('skillTags.createDialogTitle') }}</q-card-section>
        <q-card-section class="q-pt-none">
          <q-input
            v-model="createName"
            dense
            outlined
            autofocus
            :label="$t('skillTags.nameLabel')"
            :hint="$t('skillTags.nameHint')"
            :error="!!createError"
            :error-message="createError"
            @keyup.enter="onCreate"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps :label="$t('common.cancel')" :disable="creating" />
          <q-btn
            color="primary"
            unelevated
            no-caps
            :label="$t('common.create')"
            :loading="creating"
            @click="onCreate"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 改名 -->
    <q-dialog v-model="renameOpen" persistent>
      <q-card class="skill-tags-dialog-card app-dialog-card app-dialog-card--sm">
        <q-card-section class="text-h6">{{ $t('skillTags.renameDialogTitle') }}</q-card-section>
        <q-card-section class="q-pt-none">
          <div class="text-body2 text-grey-7 q-mb-sm">
            {{ $t('skillTags.renameDesc', { name: renameTarget?.name ?? '' }) }}
          </div>
          <q-input
            v-model="renameName"
            dense
            outlined
            autofocus
            :label="$t('skillTags.newNameLabel')"
            :hint="$t('skillTags.newNameHint')"
            :error="!!renameError"
            :error-message="renameError"
            @keyup.enter="onRename"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps :label="$t('common.cancel')" :disable="renaming" />
          <q-btn
            color="primary"
            unelevated
            no-caps
            :label="$t('skillTags.renameAction')"
            :loading="renaming"
            @click="onRename"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import { useSkillsStore } from '../stores/skills';
import type { SkillTagInfo } from '../features/skills/types';

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
</script>

<style scoped lang="sass">
.skill-tags-filter
  margin-bottom: 16px

.skill-tags-dialog-card
  min-width: 420px

// ── 工具栏统计 pill（中性玻璃 + 强调图标）──
.skill-tags-stat
  display: inline-flex
  align-items: center
  gap: 5px
  padding: 4px 12px
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 600
  line-height: 1.4
  color: var(--color-text-secondary)
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)

  .q-icon
    color: var(--color-accent)

  &--warning
    color: var(--color-warning)
    border-color: color-mix(in srgb, var(--color-warning) 32%, transparent)
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)

    .q-icon
      color: var(--color-warning)

// ── 分组计数 pill ──
.skill-tags-count
  display: inline-flex
  align-items: center
  justify-content: center
  min-width: 22px
  padding: 1px 8px
  border-radius: 999px
  font-size: 11px
  font-weight: 700
  line-height: 1.5
  color: var(--color-text-secondary)
  background: color-mix(in srgb, var(--color-text-secondary) 10%, transparent)

// ── 标签主体 pill：背景锚定主题玻璃色（与卡片同族），强调色只用于文字/描边 ──
// 注意：禁止 color-mix(accent, transparent) 作背景——透明薄纱叠加在玻璃上
// 暗色下会渲染成灰绿浑浊色块，与主题脱节
.skill-tag-pill
  display: inline-flex
  align-items: center
  padding: 3px 10px
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 600
  letter-spacing: 0.02em
  line-height: 1.5
  color: var(--color-accent-hover)
  border: 1px solid color-mix(in srgb, var(--color-accent) 38%, var(--glass-border))
  background: var(--glass-elevated)

  &--system
    color: var(--color-text-secondary)
    border-color: var(--glass-border)
    background: transparent

body.body--dark .skill-tag-pill
  color: var(--color-accent)
  border-color: color-mix(in srgb, var(--color-accent) 48%, var(--glass-border))

  &--system
    color: var(--color-text-secondary)
    border-color: var(--glass-border)
    background: transparent

// ── 来源角标 ──
.skill-tag-badge
  display: inline-flex
  align-items: center
  padding: 1px 7px
  border-radius: 999px
  font-size: 10px
  font-weight: 700
  line-height: 1.5
  letter-spacing: 0.04em
  color: var(--color-text-tertiary)
  border: 1px solid var(--glass-border)

  &--warning
    color: var(--color-warning)
    border-color: color-mix(in srgb, var(--color-warning) 30%, transparent)
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)

// ── 行 ──
.skill-tag-row--orphan
  background: var(--color-warning-soft)

.skill-tag-row__orphan-hint
  color: var(--color-warning)

.skill-tag-row__usage
  min-width: 96px

// 注意：必须作用于内层 span——Quasar 对 .q-item__section--side 的颜色规则
// 特异性高于本文件 scoped 类，直接挂在 section 上会被覆盖
.skill-tag-row__usage-text
  font-size: var(--text-sm)
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums
</style>
