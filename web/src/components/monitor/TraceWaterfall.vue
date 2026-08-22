<!--
  Pure presentation: Span 瀑布图（APM 风）。
  顶部时间标尺（5 刻度 + 网格线）；行 = 名称列（depth 缩进 + 状态点）+ 轨道（bar 按状态着色，
  运行中为斜纹动画并延伸至轴尾）+ 等宽时长。点击行选中（与 Span 树联动）。
-->
<template>
  <div class="span-wf">
    <div v-if="!rows.length" class="span-wf__empty column items-center q-pa-lg">
      <q-icon name="waterfall_chart" size="32px" class="q-mb-xs" />
      <span class="text-caption">{{ t('monitorPage.traces.spanTreeEmpty') }}</span>
    </div>

    <template v-else>
      <div class="span-wf__ruler">
        <span class="span-wf__ruler-spacer" />
        <span class="span-wf__ruler-track">
          <span
            v-for="tick in rulerTicks"
            :key="tick.pct"
            class="span-wf__ruler-tick"
            :style="{ left: `${tick.pct}%` }"
          >
            <span class="span-wf__ruler-label text-mono">{{ tick.label }}</span>
          </span>
        </span>
        <span class="span-wf__duration-spacer" />
      </div>

      <div class="span-wf__rows">
        <div
          v-for="node in rows"
          :key="node.id"
          class="span-wf__row"
          :class="{ 'span-wf__row--selected': node.id === selectedId }"
          @click="emit('select', node.id)"
        >
          <div class="span-wf__name-cell row items-center no-wrap">
            <span class="span-wf__indent" :style="{ width: `${node.depth * indentPx}px` }" />
            <span class="span-wf__dot" :class="`span-wf__dot--${node.tone}`" />
            <span class="ellipsis" :title="node.name">{{ node.name }}</span>
          </div>
          <div class="span-wf__track">
            <div class="span-wf__bar" :class="`span-wf__bar--${node.tone}`" :style="barStyle(node)">
              <q-tooltip class="text-body2">
                {{ node.name }} · +{{ formatSpanDuration(node.startMs) }} · {{ durationLabel(node) }}
              </q-tooltip>
            </div>
          </div>
          <div class="span-wf__duration text-mono">{{ durationLabel(node) }}</div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  flattenSpanTree,
  formatSpanDuration,
  normalizeTraceSpans,
  spanTimelineMaxEnd,
  type TraceSpanNode,
} from '../../features/monitor/spans';

const { t } = useI18n();

const indentPx = 14;

const props = defineProps<{
  spans: unknown[];
  /** 与 Span 树联动的选中 span id */
  selectedId?: string;
}>();

const emit = defineEmits<{
  select: [id: string];
}>();

const tree = computed(() => normalizeTraceSpans(props.spans));
/** 瀑布图恒为全展开视图 */
const rows = computed(() => flattenSpanTree(tree.value, new Set()));
const maxEnd = computed(() => spanTimelineMaxEnd(tree.value));

const rulerTicks = computed(() =>
  [0, 25, 50, 75, 100].map((pct) => ({
    pct,
    label: formatSpanDuration((maxEnd.value * pct) / 100),
  })),
);

function barStyle(node: TraceSpanNode): Record<string, string> {
  const offsetPct = Math.min(100, (node.startMs / maxEnd.value) * 100);
  const isOpen = node.tone === 'running' || node.durationMs <= 0;
  const widthPct = isOpen
    ? Math.max(1.5, 100 - offsetPct)
    : Math.max(1.5, Math.min(100 - offsetPct, (node.durationMs / maxEnd.value) * 100));
  return { left: `${offsetPct}%`, width: `${widthPct}%` };
}

function durationLabel(node: TraceSpanNode): string {
  if (node.tone === 'running') return t('monitorPage.traces.status.running');
  return node.durationMs > 0 ? formatSpanDuration(node.durationMs) : '-';
}
</script>

<style scoped>
.span-wf {
  display: flex;
  flex-direction: column;
}

.span-wf__empty {
  color: var(--color-text-secondary);
}

.span-wf__ruler {
  display: flex;
  align-items: flex-end;
  margin-bottom: 4px;
}

.span-wf__ruler-spacer {
  flex: 0 0 300px;
}

.span-wf__ruler-track {
  position: relative;
  flex: 1;
  height: 18px;
  border-bottom: 1px solid var(--color-border-soft);
}

.span-wf__ruler-tick {
  position: absolute;
  bottom: 0;
  height: 6px;
  border-left: 1px solid var(--color-border-soft);
}

.span-wf__ruler-label {
  position: absolute;
  bottom: 7px;
  left: 3px;
  font-size: 10px;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.span-wf__duration-spacer {
  flex: 0 0 72px;
}

.span-wf__rows {
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  padding: 6px 0;
  max-height: 52vh;
  overflow-y: auto;
  background: color-mix(in srgb, var(--canvas-base) 30%, transparent);
}

.span-wf__row {
  display: flex;
  align-items: center;
  min-height: 28px;
  padding: 2px 10px 2px 0;
  border-radius: 6px;
  cursor: pointer;
  gap: 8px;
}

.span-wf__row:hover {
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
}

.span-wf__row--selected,
.span-wf__row--selected:hover {
  background: color-mix(in srgb, var(--color-accent) 16%, transparent);
  box-shadow: inset 2px 0 0 var(--color-accent);
}

.span-wf__name-cell {
  flex: 0 0 300px;
  min-width: 0;
  font-size: 12px;
  padding-left: 8px;
}

.span-wf__indent {
  flex-shrink: 0;
}

.span-wf__dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-right: 6px;
  background: var(--color-text-icon-muted);
}

.span-wf__dot--ok {
  background: var(--color-success);
}

.span-wf__dot--error {
  background: var(--color-danger);
}

.span-wf__dot--warn {
  background: var(--color-warning);
}

.span-wf__dot--running {
  background: var(--color-accent);
  animation: span-wf-pulse 1.4s ease-in-out infinite;
}

@keyframes span-wf-pulse {
  0%,
  100% {
    opacity: 100%;
  }

  50% {
    opacity: 40%;
  }
}

.span-wf__track {
  position: relative;
  flex: 1;
  height: 14px;
  border-radius: 4px;
  background-image: linear-gradient(to right, var(--color-border-soft) 1px, transparent 1px);
  background-size: 25% 100%;
}

.span-wf__bar {
  position: absolute;
  top: 2px;
  height: 10px;
  border-radius: 4px;
  background: var(--color-info);
  min-width: 4px;
}

.span-wf__bar--ok {
  background: color-mix(in srgb, var(--color-success) 80%, transparent);
}

.span-wf__bar--error {
  background: color-mix(in srgb, var(--color-danger) 85%, transparent);
}

.span-wf__bar--warn {
  background: color-mix(in srgb, var(--color-warning) 85%, transparent);
}

.span-wf__bar--idle {
  background: color-mix(in srgb, var(--color-info) 70%, transparent);
}

.span-wf__bar--running {
  background: repeating-linear-gradient(
    45deg,
    var(--color-accent),
    var(--color-accent) 6px,
    color-mix(in srgb, var(--color-accent) 40%, transparent) 6px,
    color-mix(in srgb, var(--color-accent) 40%, transparent) 12px
  );
  animation: span-wf-slide 1.2s linear infinite;
}

@keyframes span-wf-slide {
  to {
    background-position: 17px 0;
  }
}

.span-wf__duration {
  flex: 0 0 72px;
  text-align: right;
  font-size: 11px;
  color: var(--color-text-secondary);
}

.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

@media (width <= 900px) {
  .span-wf__ruler-spacer,
  .span-wf__name-cell {
    flex-basis: 180px;
  }
}
</style>
