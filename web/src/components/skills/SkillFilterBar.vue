<template>
  <AppPageToolbar class="skill-filter-card">
    <q-input
      :model-value="search"
      class="app-page-toolbar__search"
      dense
      outlined
      clearable
      debounce="350"
      :label="t('skillsPage.filterSearchLabel')"
      :placeholder="t('skillsPage.filterSearchPlaceholder')"
      @update:model-value="emit('update:search', String($event ?? ''))"
    >
      <template #prepend><q-icon name="search" /></template>
    </q-input>
    <q-select
      :model-value="enabled"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="t('skillsPage.filterEnabled')"
      :options="enabledOptions"
      @update:model-value="emit('update:enabled', $event as boolean | null)"
    />
    <q-select
      :model-value="status"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="t('skillsPage.filterStatus')"
      :options="statusOptions"
      @update:model-value="emit('update:status', String($event ?? ''))"
    />
    <q-select
      :model-value="tags"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      multiple
      use-chips
      use-input
      input-debounce="0"
      :label="$t('skillTags.fieldLabel')"
      :options="filteredTagOptions"
      @filter="onTagFilter"
      @update:model-value="emit('update:tags', ($event as string[] | null) ?? [])"
      @add="tagNeedle = ''"
    />
    <q-select
      :model-value="syncOrigin"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="t('skillsPage.filterOrigin')"
      :options="originOptions"
      @update:model-value="emit('update:syncOrigin', String($event ?? ''))"
    />
    <q-select
      :model-value="filesystemMissing"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="t('skillsPage.filterFsState')"
      :options="filesystemOptions"
      @update:model-value="emit('update:filesystemMissing', $event as boolean | null)"
    />
    <q-select
      :model-value="sortBy"
      class="app-page-toolbar__field app-page-toolbar__field--sort"
      dense
      outlined
      emit-value
      map-options
      :label="t('skillsPage.filterSort')"
      :options="sortByOptions"
      @update:model-value="emit('update:sortBy', $event as 'tag' | 'name')"
    />
    <q-btn
      flat
      round
      dense
      class="app-page-toolbar__sort-dir"
      :icon="sortOrder === 'asc' ? 'arrow_upward' : 'arrow_downward'"
      :title="sortOrder === 'asc' ? t('skillsPage.sortAscTip') : t('skillsPage.sortDescTip')"
      @click="emit('update:sortOrder', sortOrder === 'asc' ? 'desc' : 'asc')"
    />
    <template #actions>
      <q-btn flat rounded no-caps icon="restart_alt" :label="t('skillsPage.reset')" @click="emit('reset')" />
      <q-btn flat rounded no-caps icon="refresh" :label="t('common.refresh')" :loading="loading" @click="emit('refresh')" />
    </template>
  </AppPageToolbar>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../layout/AppPageToolbar.vue';

const { t } = useI18n();

const props = defineProps<{
  search: string;
  enabled: boolean | null;
  status: string;
  tags: string[];
  /** 标签字典选项源（规范标签名）。 */
  tagOptions?: string[];
  syncOrigin: string;
  filesystemMissing: boolean | null;
  /** 排序字段：tag=按首个标签名，name=按名称。 */
  sortBy: 'tag' | 'name';
  sortOrder: 'asc' | 'desc';
  loading?: boolean;
}>();

const emit = defineEmits<{
  'update:search': [value: string];
  'update:enabled': [value: boolean | null];
  'update:status': [value: string];
  'update:tags': [value: string[]];
  'update:syncOrigin': [value: string];
  'update:filesystemMissing': [value: boolean | null];
  'update:sortBy': [value: 'tag' | 'name'];
  'update:sortOrder': [value: 'asc' | 'desc'];
  reset: [];
  refresh: [];
}>();

const tagNeedle = ref('');

/** 字典选项按输入关键字过滤；已选中但未收录的标签始终保留在选项中以免回显丢失。 */
const filteredTagOptions = computed(() => {
  const all = new Set(props.tagOptions ?? []);
  for (const t of props.tags) all.add(t);
  const list = [...all].sort();
  const kw = tagNeedle.value.trim().toLowerCase();
  if (!kw) return list;
  return list.filter((t) => t.toLowerCase().includes(kw));
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

const statusOptions = computed(() => [
  { label: t('skillsPage.filterDraft'), value: 'draft' },
  { label: t('skillsPage.filterPublished'), value: 'published' },
  { label: t('skillsPage.filterArchived'), value: 'archived' },
]);

const originOptions = computed(() => [
  { label: t('skillsPage.originFilesystem'), value: 'filesystem' },
  { label: t('skillsPage.originImport'), value: 'import' },
  { label: t('skillsPage.originManual'), value: 'manual' },
]);

const filesystemOptions = computed(() => [
  { label: t('skillsPage.filterFsMissing'), value: true },
  { label: t('skillsPage.filterFsOk'), value: false },
]);

const sortByOptions = computed(() => [
  { label: t('skillsPage.sortByTag'), value: 'tag' },
  { label: t('skillsPage.sortByName'), value: 'name' },
]);
</script>
