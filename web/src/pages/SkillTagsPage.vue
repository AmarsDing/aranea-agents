<template>
  <q-page class="app-standard-page app-registry-page skill-tags-page">
    <AppPageHero kicker="Skill registry" :title="$t('skillTags.title')" :subtitle="$t('skillTags.subtitle')">
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="add"
          :label="$t('skillTags.create')"
          @click="openCreate"
        />
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
        <q-chip outline color="primary" text-color="primary" icon="sell" square>
          {{ $t('skillTags.totalChip', { count: filteredTags.length }) }}
        </q-chip>
        <q-chip v-if="orphanCount > 0" outline color="warning" text-color="warning" icon="warning_amber" square>
          {{ $t('skillTags.orphanChip', { count: orphanCount }) }}
        </q-chip>
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
          <q-chip dense square color="grey-3" text-color="grey-8">{{ group.items.length }}</q-chip>
        </div>
        <q-card flat bordered>
          <q-list separator>
            <q-item
              v-for="tag in group.items"
              :key="tag.name"
              :class="{ 'skill-tag-row--orphan': tag.source === 'orphan' }"
            >
              <q-item-section>
                <q-item-label class="row items-center q-gutter-xs">
                  <q-chip dense square outline color="primary" text-color="primary" class="text-weight-medium">
                    {{ tag.name }}
                  </q-chip>
                  <q-badge v-if="tag.source === 'system'" color="blue-grey" outline>
                    {{ $t('skillTags.badgeSystem') }}
                  </q-badge>
                  <q-badge v-else-if="tag.source === 'orphan'" color="warning" outline>
                    {{ $t('skillTags.badgeOrphan') }}
                  </q-badge>
                </q-item-label>
                <q-item-label v-if="tag.source === 'orphan'" caption class="text-warning q-mt-xs">
                  {{ $t('skillTags.orphanHint') }}
                </q-item-label>
              </q-item-section>
              <q-item-section side class="text-body2 text-grey-7 skill-tag-row__usage">
                {{ $t('skillTags.usageCount', { count: tag.used_count }) }}
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
      <q-card style="min-width: 420px">
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
      <q-card style="min-width: 420px">
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

function errMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
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
  if (!createName.value.trim()) {
    createError.value = t('skillTags.nameRequired');
    return;
  }
  creating.value = true;
  createError.value = '';
  try {
    await skillsStore.createTag(createName.value);
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
  if (!renameName.value.trim()) {
    renameError.value = t('skillTags.newNameRequired');
    return;
  }
  renaming.value = true;
  renameError.value = '';
  try {
    const rewritten = await skillsStore.renameTag(target.name, renameName.value);
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

<style scoped>
.skill-tags-filter {
  margin-bottom: 16px;
}
.skill-tag-row--orphan {
  background: rgba(255, 193, 7, 0.06);
}
.skill-tag-row__usage {
  min-width: 96px;
}
</style>
