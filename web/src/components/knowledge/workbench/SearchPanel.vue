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
      <!-- U1-6 相关度提示：good 不打扰；weak amber 提示条（纯信息，不阻断结果操作） -->
      <div v-if="classification.level === 'weak'" class="kb-search__relhint">
        <div class="kb-search__relhint-head">
          <q-icon name="tips_and_updates" size="15px" class="kb-search__relhint-icon" />
          <span>
            {{ t('knowledgePage.workbench.search.weakTitle', { score: classification.topScore.toFixed(2) }) }}
          </span>
        </div>
        <div class="kb-search__relhint-tips">
          <span v-for="tip in classification.tips" :key="tip" class="kb-search__relhint-tip">{{ tipText(tip) }}</span>
        </div>
      </div>
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
        <!-- G-16：按 marks 区间切片渲染 <mark>，v-text 插值天然防 XSS -->
        <span class="kb-search__item-snippet">
          <template v-for="(seg, si) in snippetSegments(it)" :key="si"
            ><mark v-if="seg.mark" class="kb-search__mark">{{ seg.text }}</mark
            ><template v-else>{{ seg.text }}</template></template
          >
        </span>
      </button>
    </template>
    <div v-else-if="loading" class="kb-search__empty">{{ t('knowledgePage.workbench.search.searching') }}</div>
    <div v-else-if="query.trim()" class="kb-search__empty kb-search__empty--guide">
      <!-- U1-6 空结果引导块：语义层特别提示（条件）+ 扩大范围 + 跳 hit-test -->
      <div class="kb-search__empty-title">
        <q-icon name="search_off" size="17px" class="kb-search__empty-title-icon" />
        <span>{{ t('knowledgePage.workbench.search.noResults') }}</span>
      </div>
      <ul class="kb-search__empty-tips">
        <li
          v-for="tip in emptyTips"
          :key="tip"
          class="kb-search__empty-tip"
          :class="{ 'kb-search__empty-tip--semantic': tip === 'enable-semantic' }"
        >
          {{ tipText(tip) }}
        </li>
      </ul>
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="troubleshoot"
        :label="t('knowledgePage.workbench.search.openHitTestAction')"
        @click="onOpenHitTest"
      />
    </div>
    <div v-else class="kb-search__empty">{{ t('knowledgePage.workbench.search.hint') }}</div>
  </PaletteModal>
</template>

<script setup lang="ts">
// SearchPanel（Ctrl+Shift+F，P1-3）：全库全文搜索浮层。
// 纯受控组件：检索在容器（KnowledgeWorkbench）执行，本组件只渲染结果与键盘导航。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import PaletteModal from './PaletteModal.vue';
import type { KnowledgeChunk } from '../../../features/knowledge/types';
import type { HighlightRange } from '../../../features/knowledge/searchHighlight';
import { classifyResults, type RelevanceTip } from '../../../features/knowledge/relevanceHint';

export type SearchItem = {
  chunk: KnowledgeChunk;
  docId: string;
  name: string;
  path: string;
  snippet: string;
  /** G-16：片段内关键词高亮区间（[start,end) 半开，已归并；缺省=无高亮）。 */
  marks?: HighlightRange[];
  score: number;
};

const props = defineProps<{
  open: boolean;
  query: string;
  items: SearchItem[];
  loading: boolean;
  /** U1-6：当前检索范围全部为纯词法库（未启用语义层）时置 true，空态引导显示特别提示；undefined=未知不提示。 */
  semanticLexicalOnly?: boolean;
}>();

const emit = defineEmits<{
  'update:open': [v: boolean];
  'update:query': [v: string];
  pick: [item: SearchItem];
  /** U1-6：空态引导跳检索排查面板（复用顶栏 open-hit-test 链路，空载荷契约一致）。 */
  'open-hit-test': [];
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

/** U1-6：相关度分级（score / chunk.rrf_score 喂给纯函数），驱动顶部提示条与空态引导。 */
const classification = computed(() =>
  classifyResults(props.items.map((it) => ({ score: it.score, rrf_score: it.chunk.rrf_score }))),
);

/** U1-6 空态引导 tips：纯词法库（未启用语义层）时前置特别提示。 */
const emptyTips = computed<RelevanceTip[]>(() => {
  if (classification.value.level !== 'empty') return [];
  return props.semanticLexicalOnly ? ['enable-semantic', ...classification.value.tips] : classification.value.tips;
});

/** U1-6 tip 标识 → i18n 键（字面量键，便于 check-i18n 静态提取）。 */
const TIP_LABEL_KEYS: Record<RelevanceTip, string> = {
  'refine-keywords': 'knowledgePage.workbench.search.tips.refineKeywords',
  'try-rerank': 'knowledgePage.workbench.search.tips.tryRerank',
  'narrow-scope': 'knowledgePage.workbench.search.tips.narrowScope',
  'enable-semantic': 'knowledgePage.workbench.search.tips.enableSemantic',
  'expand-scope': 'knowledgePage.workbench.search.tips.expandScope',
  'open-hit-test': 'knowledgePage.workbench.search.tips.openHitTest',
};

function tipText(tip: RelevanceTip): string {
  return t(TIP_LABEL_KEYS[tip] ?? tip);
}

/** G-16：把 snippet 按 marks 切成 mark/plain 片（保序，纯文本插值渲染）。 */
function snippetSegments(it: SearchItem): { text: string; mark: boolean }[] {
  const marks = it.marks ?? [];
  if (!marks.length) return [{ text: it.snippet, mark: false }];
  const segs: { text: string; mark: boolean }[] = [];
  let pos = 0;
  for (const m of marks) {
    const start = Math.max(0, m.start);
    const end = Math.min(it.snippet.length, m.end);
    if (end <= start) continue;
    if (start > pos) segs.push({ text: it.snippet.slice(pos, start), mark: false });
    segs.push({ text: it.snippet.slice(start, end), mark: true });
    pos = end;
  }
  if (pos < it.snippet.length) segs.push({ text: it.snippet.slice(pos), mark: false });
  return segs;
}

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

/** U1-6：跳 hit-test 排查面板（复用顶栏 open-hit-test 链路）；页面层弹面板，先关本浮层避免遮挡。 */
function onOpenHitTest() {
  emit('open-hit-test');
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

  // G-16 命中高亮：青色荧光，暗底可读
  mark
    background: color-mix(in srgb, var(--kb-accent-cyan) 35%, transparent)
    color: var(--kb-text-primary)
    border-radius: 3px
    padding: 0 1px

.kb-search__empty
  padding: 24px
  text-align: center
  color: var(--kb-text-dim)
  font-size: 13px

  &--guide
    display: flex
    flex-direction: column
    align-items: center
    gap: 10px

// U1-6 空态引导块
.kb-search__empty-title
  display: flex
  align-items: center
  gap: 6px
  font-weight: 600
  color: var(--kb-text-primary)

  &-icon
    color: var(--kb-accent-cyan)

.kb-search__empty-tips
  display: flex
  flex-direction: column
  gap: 4px
  margin: 0
  padding: 0
  list-style: none
  font-size: 12px
  text-align: left

.kb-search__empty-tip
  &::before
    content: '· '
    color: var(--kb-accent-cyan)

  &--semantic
    color: color-mix(in srgb, var(--color-warning) 75%, var(--kb-text-primary))

// U1-6 相关度提示条：amber 轻提示，不阻断结果操作
.kb-search__relhint
  margin: 2px 4px 6px
  padding: 8px 10px
  border-radius: 8px
  border: 1px solid color-mix(in srgb, var(--color-warning) 35%, transparent)
  background: color-mix(in srgb, var(--color-warning) 10%, transparent)

  &-head
    display: flex
    align-items: center
    gap: 6px
    font-size: 12.5px
    font-weight: 600
    color: var(--kb-text-primary)

  &-icon
    color: var(--color-warning)
    flex: none

  &-tips
    display: flex
    flex-wrap: wrap
    gap: 4px
    margin-top: 6px

  &-tip
    font-size: 11.5px
    padding: 1px 8px
    border-radius: 999px
    border: 1px solid var(--kb-glass-border)
    color: var(--kb-text-dim)
</style>
