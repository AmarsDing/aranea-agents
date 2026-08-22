<!--
  Pure presentation: Span 树（APM 风）。
  数据经 features/monitor/spans.ts 纯函数归一化（扁平 parent_id / 嵌套 children 均兼容）。
  交互：搜索过滤（保留祖先链）、展开/折叠、点击行选中并内联展开属性/错误详情。
-->
<template>
  <div class="span-tree">
    <div class="span-tree__toolbar row items-center q-gutter-sm">
      <q-input
        v-model="keyword"
        dense
        outlined
        clearable
        debounce="200"
        class="span-tree__search"
        :placeholder="t('monitorPage.traces.spanSearchPlaceholder')"
      >
        <template #prepend><q-icon name="search" size="16px" /></template>
      </q-input>
      <span class="span-tree__count text-caption">{{ t('monitorPage.traces.spanCount', { count: totalCount }) }}</span>
      <q-space />
      <q-btn flat dense no-caps icon="unfold_more" :label="t('monitorPage.traces.spanExpandAll')" @click="expandAll" />
      <q-btn
        flat
        dense
        no-caps
        icon="unfold_less"
        :label="t('monitorPage.traces.spanCollapseAll')"
        @click="collapseAll"
      />
    </div>

    <div v-if="!visibleRows.length" class="span-tree__empty column items-center q-pa-lg">
      <q-icon name="account_tree" size="32px" class="q-mb-xs" />
      <span class="text-caption">{{ t('monitorPage.traces.spanTreeEmpty') }}</span>
    </div>

    <div v-else class="span-tree__rows">
      <template v-for="node in visibleRows" :key="node.id">
        <div
          class="span-tree__row row items-center no-wrap"
          :class="{ 'span-tree__row--selected': node.id === selectedId, 'span-tree__row--error': node.hasError }"
          @click="onRowClick(node)"
        >
          <span class="span-tree__indent" :style="{ width: `${node.depth * indentPx}px` }">
            <span v-for="g in node.depth" :key="g" class="span-tree__guide" />
          </span>
          <q-icon
            v-if="node.children.length"
            name="expand_more"
            size="16px"
            class="span-tree__arrow"
            :class="{ 'span-tree__arrow--collapsed': isCollapsed(node.id) }"
            @click.stop="toggleCollapse(node.id)"
          />
          <span v-else class="span-tree__arrow span-tree__arrow--leaf" />
          <span class="span-tree__status" :class="`span-tree__status--${node.tone}`">
            <span v-if="node.tone === 'running'" class="span-tree__pulse" />
            <q-icon v-else :name="statusIcon(node.tone)" size="14px" />
          </span>
          <span v-if="node.kind" class="span-tree__kind">{{ node.kind }}</span>
          <span class="span-tree__name ellipsis" :title="node.name">{{ node.name }}</span>
          <span class="span-tree__bar-track">
            <span class="span-tree__bar" :class="`span-tree__bar--${node.tone}`" :style="{ width: barWidth(node) }" />
          </span>
          <span class="span-tree__duration text-mono">{{ durationLabel(node) }}</span>
        </div>

        <div
          v-if="detailId === node.id"
          class="span-tree__detail"
          :style="{ marginLeft: `${node.depth * indentPx + 24}px` }"
        >
          <div class="span-tree__detail-row">
            <span class="span-tree__detail-label">span_id</span>
            <span class="text-mono ellipsis">{{ node.id }}</span>
            <q-btn flat dense round size="xs" icon="content_copy" @click.stop="copyId(node.id)">
              <q-tooltip>{{ t('monitorPage.traces.spanCopyId') }}</q-tooltip>
            </q-btn>
          </div>
          <div v-if="node.error" class="span-tree__detail-error">
            <q-icon name="error_outline" size="14px" class="q-mr-xs" />{{ node.error }}
          </div>
          <template v-if="attrEntries(node).length">
            <div v-for="entry in attrEntries(node)" :key="entry.key" class="span-tree__detail-row">
              <span class="span-tree__detail-label text-mono">{{ entry.key }}</span>
              <span class="span-tree__detail-value text-mono">{{ entry.value }}</span>
            </div>
          </template>
          <div v-else-if="!node.error" class="span-tree__detail-row">
            <span class="span-tree__detail-label">{{ t('monitorPage.traces.spanNoAttributes') }}</span>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { copyToClipboard } from 'quasar';
import { useI18n } from 'vue-i18n';
import {
  countSpanNodes,
  filterSpanTree,
  flattenSpanTree,
  formatSpanDuration,
  normalizeTraceSpans,
  spanTimelineMaxEnd,
  type SpanStatusTone,
  type TraceSpanNode,
} from '../../features/monitor/spans';

const { t } = useI18n();

const indentPx = 18;

const props = defineProps<{
  spans: unknown[];
  /** 与瀑布图联动的选中 span id */
  selectedId?: string;
}>();

const emit = defineEmits<{
  select: [id: string];
}>();

const keyword = ref('');
const collapsed = ref<Set<string>>(new Set());
const detailId = ref('');

const tree = computed(() => normalizeTraceSpans(props.spans));
const filteredTree = computed(() => filterSpanTree(tree.value, keyword.value));
const totalCount = computed(() => countSpanNodes(tree.value));
const maxEnd = computed(() => spanTimelineMaxEnd(tree.value));
/** 过滤时忽略折叠态，保证命中链完整可见 */
const visibleRows = computed(() =>
  flattenSpanTree(filteredTree.value, keyword.value.trim() ? new Set<string>() : collapsed.value),
);

function isCollapsed(id: string): boolean {
  return collapsed.value.has(id);
}

function toggleCollapse(id: string) {
  const next = new Set(collapsed.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  collapsed.value = next;
}

function allIds(nodes: TraceSpanNode[], out: string[] = []): string[] {
  for (const n of nodes) {
    out.push(n.id);
    allIds(n.children, out);
  }
  return out;
}

function expandAll() {
  collapsed.value = new Set();
}

function collapseAll() {
  collapsed.value = new Set(allIds(tree.value));
}

function onRowClick(node: TraceSpanNode) {
  detailId.value = detailId.value === node.id ? '' : node.id;
  emit('select', node.id);
}

function statusIcon(tone: SpanStatusTone): string {
  switch (tone) {
    case 'ok':
      return 'check_circle';
    case 'error':
      return 'error';
    case 'warn':
      return 'warning';
    default:
      return 'radio_button_unchecked';
  }
}

function barWidth(node: TraceSpanNode): string {
  if (node.tone === 'running' || node.durationMs <= 0) return '100%';
  return `${Math.max(4, Math.min(100, (node.durationMs / maxEnd.value) * 100))}%`;
}

function durationLabel(node: TraceSpanNode): string {
  if (node.tone === 'running') return t('monitorPage.traces.status.running');
  return node.durationMs > 0 ? formatSpanDuration(node.durationMs) : '-';
}

function attrEntries(node: TraceSpanNode): { key: string; value: string }[] {
  return Object.entries(node.attributes)
    .slice(0, 24)
    .map(([key, v]) => ({
      key,
      value: typeof v === 'string' ? v : JSON.stringify(v),
    }));
}

async function copyId(id: string) {
  await copyToClipboard(id);
}
</script>

<style scoped>
.span-tree {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.span-tree__search {
  width: 260px;
}

.span-tree__count {
  color: var(--color-text-secondary);
}

.span-tree__empty {
  color: var(--color-text-secondary);
}

.span-tree__rows {
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  padding: 6px;
  max-height: 52vh;
  overflow-y: auto;
  background: color-mix(in srgb, var(--canvas-base) 30%, transparent);
}

.span-tree__row {
  min-height: 30px;
  padding: 2px 8px 2px 0;
  border-radius: 8px;
  cursor: pointer;
  gap: 6px;
}

.span-tree__row:hover {
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
}

.span-tree__row--selected,
.span-tree__row--selected:hover {
  background: color-mix(in srgb, var(--color-accent) 16%, transparent);
  box-shadow: inset 2px 0 0 var(--color-accent);
}

.span-tree__indent {
  display: inline-flex;
  flex-shrink: 0;
  align-self: stretch;
}

.span-tree__guide {
  width: 18px;
  border-left: 1px solid var(--color-border-soft);
}

.span-tree__guide:first-child {
  margin-left: 7px;
}

.span-tree__arrow {
  flex-shrink: 0;
  width: 16px;
  color: var(--color-text-secondary);
  transition: transform 0.15s ease;
  cursor: pointer;
}

.span-tree__arrow--collapsed {
  transform: rotate(-90deg);
}

.span-tree__arrow--leaf {
  display: inline-block;
}

.span-tree__status {
  flex-shrink: 0;
  width: 16px;
  display: inline-flex;
  justify-content: center;
}

.span-tree__status--ok {
  color: var(--color-success);
}

.span-tree__status--error {
  color: var(--color-danger);
}

.span-tree__status--warn {
  color: var(--color-warning);
}

.span-tree__status--idle {
  color: var(--color-text-icon-muted);
}

.span-tree__pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-accent);
  animation: span-pulse 1.4s ease-in-out infinite;
}

@keyframes span-pulse {
  0%,
  100% {
    opacity: 100%;
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-accent) 45%, transparent);
  }

  50% {
    opacity: 60%;
    box-shadow: 0 0 0 4px transparent;
  }
}

.span-tree__kind {
  flex-shrink: 0;
  font-size: 10px;
  line-height: 1;
  padding: 3px 6px;
  border-radius: 6px;
  color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 12%, transparent);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.span-tree__name {
  flex: 1;
  min-width: 0;
  font-size: 13px;
}

.span-tree__row--error > .span-tree__name {
  color: var(--color-danger);
}

.span-tree__bar-track {
  flex-shrink: 0;
  width: 88px;
  height: 5px;
  border-radius: 3px;
  background: var(--color-border-soft);
  overflow: hidden;
}

.span-tree__bar {
  display: block;
  height: 100%;
  border-radius: 3px;
  background: var(--color-info);
}

.span-tree__bar--ok {
  background: var(--color-success);
}

.span-tree__bar--error {
  background: var(--color-danger);
}

.span-tree__bar--warn {
  background: var(--color-warning);
}

.span-tree__bar--running {
  background: repeating-linear-gradient(
    45deg,
    var(--color-accent),
    var(--color-accent) 6px,
    color-mix(in srgb, var(--color-accent) 45%, transparent) 6px,
    color-mix(in srgb, var(--color-accent) 45%, transparent) 12px
  );
  animation: span-bar-slide 1.2s linear infinite;
}

@keyframes span-bar-slide {
  to {
    background-position: 17px 0;
  }
}

.span-tree__duration {
  flex-shrink: 0;
  width: 64px;
  text-align: right;
  font-size: 11px;
  color: var(--color-text-secondary);
}

.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

.span-tree__detail {
  margin: 2px 8px 6px 24px;
  padding: 8px 12px;
  border-radius: 10px;
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--color-accent) 5%, transparent);
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
}

.span-tree__detail-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.span-tree__detail-label {
  flex-shrink: 0;
  min-width: 120px;
  color: var(--color-text-secondary);
}

.span-tree__detail-value {
  word-break: break-all;
}

.span-tree__detail-error {
  color: var(--color-danger);
  display: flex;
  align-items: center;
  word-break: break-all;
}
</style>
