<template>
  <PaletteModal
    v-if="open"
    :open="open"
    :title="t('knowledgePage.workbench.quickSwitcher')"
    icon="bolt"
    :placeholder="t('knowledgePage.workbench.switcher.placeholder')"
    :query="query"
    @close="close"
    @update:query="query = $event"
    @keydown="onKeydown"
  >
    <div v-if="truncated" class="kb-switcher__warn" data-test="switcher-truncated">
      {{ t('knowledgePage.workbench.switcher.truncated') }}
    </div>
    <template v-if="flatItems.length">
      <div v-if="!query.trim() && openTabDocs.length" class="kb-switcher__section">
        {{ t('knowledgePage.workbench.switcher.openTabs') }}
      </div>
      <button
        v-for="(it, i) in flatItems"
        :key="it.doc.id"
        type="button"
        class="kb-switcher__item"
        :class="{ 'kb-switcher__item--active': i === activeIndex }"
        @mouseenter="activeIndex = i"
        @click="pick(it.doc)"
      >
        <q-icon :name="it.tab ? 'tab' : 'description'" size="16px" class="kb-switcher__item-icon" />
        <span class="kb-switcher__item-main">
          <span class="kb-switcher__item-name ellipsis">{{ it.name }}</span>
          <span class="kb-switcher__item-path ellipsis">{{ it.path }}</span>
        </span>
        <span class="kb-switcher__item-badge">{{ vaultName }}</span>
      </button>
    </template>
    <div v-else class="kb-switcher__empty">{{ t('knowledgePage.workbench.switcher.noResults') }}</div>
  </PaletteModal>
</template>

<script setup lang="ts">
// QuickSwitcher（⌘O，SP2 §SP2-7）：文件名即时过滤（复用 instantFilter fzf 式子序列匹配）。
// 空查询 = 已打开 tab 优先 + 其余文档补齐；↑↓ 导航、Enter 打开、ESC 关闭。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import PaletteModal from './PaletteModal.vue';
import { instantFilter } from '../../../features/knowledge/instantMatch';
import type { WorkbenchTab } from '../../../features/knowledge/useKnowledgeWorkbench';
import type { KnowledgeDocument } from '../../../features/knowledge/types';

const props = defineProps<{
  open: boolean;
  /** 当前库全部文档 */
  documents: KnowledgeDocument[];
  /** 已打开 tab（空查询时前置展示） */
  tabs: WorkbenchTab[];
  vaultName: string;
  /** 文档列表被分页截断时，⌘O 索引不完整。 */
  truncated?: boolean;
}>();

const emit = defineEmits<{
  'update:open': [v: boolean];
  open: [doc: KnowledgeDocument];
}>();

const { t } = useI18n();

const query = ref('');
const activeIndex = ref(0);

type SwitcherItem = {
  doc: KnowledgeDocument;
  name: string;
  path: string;
  tab: boolean;
};

function baseName(relPath: string): string {
  const seg = relPath.split('/').filter(Boolean);
  return seg.length ? seg[seg.length - 1] : relPath;
}

function toItem(d: KnowledgeDocument, tab: boolean): SwitcherItem {
  const rel = d.rel_path || d.source;
  return { doc: d, name: baseName(rel), path: rel, tab };
}

/** 已打开 tab 对应文档（保持 tab 顺序）。 */
const openTabDocs = computed<SwitcherItem[]>(() => {
  const byId = new Map(props.documents.map((d) => [d.id, d]));
  return props.tabs.map((tb) => byId.get(tb.docId)).filter((d): d is KnowledgeDocument => !!d).map((d) => toItem(d, true));
});

const flatItems = computed<SwitcherItem[]>(() => {
  const q = query.value.trim();
  if (q) {
    return instantFilter(props.documents, q, (d) => [baseName(d.rel_path || d.source), d.rel_path || d.source]).map(
      (d) => toItem(d, false),
    );
  }
  // 空查询：已打开 tab 优先，其余文档补齐 20 条
  const tabIds = new Set(props.tabs.map((tb) => tb.docId));
  const rest = props.documents.filter((d) => !tabIds.has(d.id)).map((d) => toItem(d, false));
  return [...openTabDocs.value, ...rest].slice(0, 20);
});

watch(
  () => props.open,
  (on) => {
    if (on) {
      query.value = '';
      activeIndex.value = 0;
    }
  },
);

watch(flatItems, () => {
  if (activeIndex.value >= flatItems.value.length) activeIndex.value = 0;
});

function close() {
  emit('update:open', false);
}

function pick(doc: KnowledgeDocument) {
  emit('open', doc);
  close();
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    if (flatItems.value.length) activeIndex.value = (activeIndex.value + 1) % flatItems.value.length;
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    if (flatItems.value.length) {
      activeIndex.value = (activeIndex.value - 1 + flatItems.value.length) % flatItems.value.length;
    }
  } else if (e.key === 'Enter') {
    e.preventDefault();
    const it = flatItems.value[activeIndex.value];
    if (it) pick(it.doc);
  } else if (e.key === 'Escape') {
    e.preventDefault();
    close();
  }
}
</script>

<style lang="sass" scoped>
.kb-switcher__warn
  padding: 6px 10px
  margin-bottom: 4px
  border-radius: 8px
  font-size: 12px
  color: var(--kb-text-primary)
  background: color-mix(in srgb, #f5a524 16%, transparent)

.kb-switcher__section
  padding: 6px 10px 2px
  font-size: 11px
  letter-spacing: 0.06em
  text-transform: uppercase
  color: var(--kb-text-dim)

.kb-switcher__item
  display: flex
  align-items: center
  gap: 10px
  width: 100%
  padding: 7px 10px
  border: 0
  border-radius: 8px
  background: transparent
  color: var(--kb-text-primary)
  font-size: 13.5px
  text-align: left
  cursor: pointer

  &--active
    background: rgba(79, 216, 255, 0.1)

  &-icon
    color: var(--kb-accent-cyan)
    flex: none

  &-main
    flex: 1
    min-width: 0
    display: flex
    flex-direction: column

  &-path
    font-size: 11.5px
    color: var(--kb-text-dim)

  &-badge
    flex: none
    font-size: 10.5px
    padding: 1px 8px
    border-radius: 999px
    color: var(--kb-text-dim)
    border: 1px solid var(--kb-glass-border)

.kb-switcher__empty
  padding: 24px
  text-align: center
  color: var(--kb-text-dim)
  font-size: 13px
</style>
