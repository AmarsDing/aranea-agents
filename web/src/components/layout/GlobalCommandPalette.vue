<template>
  <q-dialog
    v-model="open"
    position="top"
    transition-show="fade"
    transition-hide="fade"
    class="app-command"
    @show="onShow"
  >
    <q-card class="app-command__card">
      <q-input
        ref="inputRef"
        v-model="query"
        class="app-command__input"
        borderless
        dense
        autofocus
        :placeholder="t('shell.command.placeholder')"
        :aria-label="t('shell.command.placeholder')"
        role="combobox"
        aria-autocomplete="list"
        :aria-expanded="results.length > 0"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
        @keydown.enter.prevent="selectActive"
      >
        <template #prepend>
          <q-icon name="search" size="20px" />
        </template>
        <template #append>
          <kbd class="app-command__kbd">ESC</kbd>
        </template>
      </q-input>
      <q-separator />
      <q-list v-if="results.length > 0" class="app-command__list" role="listbox">
        <q-item
          v-for="(entry, index) in results"
          :key="entry.to"
          clickable
          :active="index === activeIndex"
          active-class="app-command__item--active"
          class="app-command__item"
          role="option"
          :aria-selected="index === activeIndex"
          @mouseenter="activeIndex = index"
          @click="select(entry)"
        >
          <q-item-section avatar>
            <q-icon :name="entry.icon" size="18px" />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ entry.label }}</q-item-label>
            <q-item-label caption>
              {{ entry.groupLabel }}<template v-if="entry.sectionLabel"> · {{ entry.sectionLabel }}</template>
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <span v-if="entry.recent" class="app-command__badge">{{ t('shell.command.recent') }}</span>
            <span v-else class="app-command__path">{{ entry.to }}</span>
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="app-command__empty">{{ t('shell.command.empty') }}</div>
      <div class="app-command__footer">
        <span class="app-command__hint">↑ ↓ {{ t('shell.common.search') }}</span>
        <span class="app-command__hint">↵ {{ t('shell.common.confirm') }}</span>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { flatNavLeaves } from '../../config/navView';

const props = defineProps<{ modelValue: boolean }>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>();

const { t } = useI18n();
const router = useRouter();
const query = ref('');
const activeIndex = ref(0);
const inputRef = ref<{ focus: () => void } | null>(null);

const RECENT_KEY = 'nav:recent';
const MAX_RECENT = 5;

const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
});

type Entry = {
  to: string;
  icon: string;
  label: string;
  groupLabel: string;
  sectionLabel: string;
  recent?: boolean;
};

const entries = computed<Entry[]>(() =>
  flatNavLeaves().map((leaf) => ({
    to: leaf.to,
    icon: leaf.icon,
    label: t(leaf.labelKey),
    groupLabel: t(leaf.groupKey),
    sectionLabel: leaf.sectionLabelKey ? t(leaf.sectionLabelKey) : '',
  })),
);

function loadRecent(): string[] {
  try {
    const raw = localStorage.getItem(RECENT_KEY);
    const parsed = raw ? (JSON.parse(raw) as unknown) : [];
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
}

function remember(path: string) {
  try {
    const next = [path, ...loadRecent().filter((item) => item !== path)].slice(0, MAX_RECENT);
    localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // 隐私模式下 localStorage 不可写：忽略，不影响跳转
  }
}

const results = computed<Entry[]>(() => {
  const keyword = query.value.trim().toLowerCase();
  if (!keyword) {
    const recents = loadRecent();
    const recentEntries = recents
      .map((path) => entries.value.find((entry) => entry.to === path))
      .filter((entry): entry is Entry => Boolean(entry))
      .map((entry) => ({ ...entry, recent: true }));
    const rest = entries.value.filter((entry) => !recents.includes(entry.to));
    return [...recentEntries, ...rest].slice(0, 40);
  }
  return entries.value
    .filter((entry) =>
      [entry.label, entry.groupLabel, entry.sectionLabel, entry.to].some((field) =>
        field.toLowerCase().includes(keyword),
      ),
    )
    .slice(0, 40);
});

watch(query, () => {
  activeIndex.value = 0;
});

watch(results, () => {
  if (activeIndex.value >= results.value.length) activeIndex.value = 0;
});

function onShow() {
  query.value = '';
  activeIndex.value = 0;
  void nextTick(() => inputRef.value?.focus());
}

function move(delta: number) {
  if (results.value.length === 0) return;
  const total = results.value.length;
  activeIndex.value = (activeIndex.value + delta + total) % total;
}

function selectActive() {
  const entry = results.value[activeIndex.value];
  if (entry) select(entry);
}

async function select(entry: Entry) {
  remember(entry.to);
  open.value = false;
  await router.push(entry.to);
}
</script>
