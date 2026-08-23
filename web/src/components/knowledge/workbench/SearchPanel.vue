<template>
  <PaletteModal
    v-if="open"
    :open="open"
    :title="t('knowledgePage.workbench.searchTitle')"
    icon="search"
    :placeholder="t('knowledgePage.workbench.search.placeholder')"
    :query="query"
    @close="close"
    @update:query="$emit('update:query', $event)"
    @keydown="onKeydown"
  >
    <template v-if="items.length">
      <div v-if="citations.length" class="kb-search__cites">
        <div class="kb-search__cites-title">{{ t('knowledgePage.workbench.search.citations') }}</div>
        <div v-for="c in citations" :key="c.docId" class="kb-search__cites-item ellipsis">{{ c.name }}</div>
      </div>
      <button
        v-for="(it, i) in items"
        :key="it.chunk.id"
        type="button"
        class="kb-search__item"
        :class="{ 'kb-search__item--active': i === activeIndex }"
        @mouseenter="activeIndex = i"
        @click="pick(it)"
      >
        <span class="kb-search__item-head">
          <q-icon name="description" size="15px" class="kb-search__item-icon" />
          <span class="kb-search__item-name ellipsis">{{ it.name }}</span>
          <span class="kb-search__item-score">{{ it.score.toFixed(2) }}</span>
        </span>
        <span class="kb-search__item-path ellipsis">{{ it.path }}</span>
        <span class="kb-search__item-snippet">{{ it.snippet }}</span>
      </button>
    </template>
    <div v-else-if="loading" class="kb-search__empty">{{ t('knowledgePage.workbench.search.searching') }}</div>
    <div v-else class="kb-search__empty">
      {{ query.trim() ? t('knowledgePage.workbench.search.noResults') : t('knowledgePage.workbench.search.hint') }}
    </div>
  </PaletteModal>
</template>

<script setup lang="ts">
// SearchPanel（Ctrl+Shift+F，P1-3）：全库全文搜索浮层。
// 纯受控组件：检索在容器（KnowledgeWorkbench）执行，本组件只渲染结果与键盘导航。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import PaletteModal from './PaletteModal.vue';
import type { KnowledgeChunk } from '../../../features/knowledge/types';

export type SearchItem = {
  chunk: KnowledgeChunk;
  docId: string;
  name: string;
  path: string;
  snippet: string;
  score: number;
};

const props = defineProps<{
  open: boolean;
  query: string;
  items: SearchItem[];
  loading: boolean;
}>();

const emit = defineEmits<{
  'update:open': [v: boolean];
  'update:query': [v: string];
  pick: [item: SearchItem];
}>();

const { t } = useI18n();

const citations = computed(() => {
  const seen = new Set<string>();
  const out: { docId: string; name: string }[] = [];
  for (const it of props.items) {
    if (seen.has(it.docId)) continue;
    seen.add(it.docId);
    out.push({ docId: it.docId, name: it.name });
  }
  return out;
});

const activeIndex = ref(0);

watch(
  () => [props.open, props.items],
  () => {
    activeIndex.value = 0;
  },
);

function close() {
  emit('update:open', false);
}

function pick(it: SearchItem) {
  emit('pick', it);
  close();
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    if (props.items.length) activeIndex.value = (activeIndex.value + 1) % props.items.length;
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    if (props.items.length) {
      activeIndex.value = (activeIndex.value - 1 + props.items.length) % props.items.length;
    }
  } else if (e.key === 'Enter') {
    e.preventDefault();
    const it = props.items[activeIndex.value];
    if (it) pick(it);
  } else if (e.key === 'Escape') {
    e.preventDefault();
    close();
  }
}
</script>

<style lang="sass" scoped>
.kb-search__cites
  display: flex
  flex-wrap: wrap
  gap: 6px
  padding: 4px 10px 8px
  align-items: center

  &-title
    font-size: 11px
    color: var(--kb-text-dim)
    text-transform: uppercase
    letter-spacing: 0.06em
    margin-right: 4px

  &-item
    font-size: 11.5px
    padding: 1px 8px
    border-radius: 999px
    border: 1px solid var(--kb-glass-border)
    color: var(--kb-text-primary)
    max-width: 160px

.kb-search__item
  display: flex
  flex-direction: column
  gap: 3px
  width: 100%
  padding: 8px 10px
  border: 0
  border-radius: 8px
  background: transparent
  color: var(--kb-text-primary)
  text-align: left
  cursor: pointer

  &--active
    background: color-mix(in srgb, var(--color-accent) 10%, transparent)

  &-head
    display: flex
    align-items: center
    gap: 8px
    min-width: 0

  &-icon
    color: var(--kb-accent-cyan)
    flex: none

  &-name
    flex: 1
    min-width: 0
    font-size: 13.5px
    font-weight: 600

  &-score
    flex: none
    font-size: 10.5px
    padding: 1px 8px
    border-radius: 999px
    color: var(--kb-text-dim)
    border: 1px solid var(--kb-glass-border)
    font-variant-numeric: tabular-nums

  &-path
    font-size: 11.5px
    color: var(--kb-text-dim)

  &-snippet
    font-size: 12px
    line-height: 1.5
    color: var(--kb-text-dim)
    display: -webkit-box
    -webkit-line-clamp: 2
    -webkit-box-orient: vertical
    overflow: hidden

.kb-search__empty
  padding: 24px
  text-align: center
  color: var(--kb-text-dim)
  font-size: 13px
</style>
