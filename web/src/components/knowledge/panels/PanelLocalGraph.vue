<template>
  <div class="kb-local-graph">
    <canvas
      ref="canvasRef"
      class="kb-local-graph__canvas"
      @click="onClick"
    />
    <div class="kb-local-graph__footer">
      <q-slider
        :model-value="hops"
        :min="1"
        :max="5"
        :step="1"
        dense
        label
        :label-value="t('knowledgePage.workbench.panels.graphHops', { hops })"
        class="kb-local-graph__slider"
        @update:model-value="(v: number | null) => v != null && $emit('update:hops', v)"
      />
      <q-btn
        flat
        dense
        round
        size="sm"
        icon="open_in_full"
        :title="t('knowledgePage.workbench.panels.expandGraph')"
        class="kb-local-graph__expand"
        @click="$emit('expand')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
// 局部图谱面板（SP2 §SP2-8）：迷你 2D 力导向 canvas（≤200 节点，确定性布局）。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { layoutLocalGraph, type LayoutPoint } from '../../../features/knowledge/localGraphLayout';
import type { CollectionGraphEdge, CollectionGraphNode } from '../../../features/knowledge/types';

const props = defineProps<{
  nodes: CollectionGraphNode[];
  edges: CollectionGraphEdge[];
  rootId: string;
  hops: number;
}>();

const emit = defineEmits<{
  'open-doc-id': [docId: string];
  'update:hops': [hops: number];
  expand: [];
}>();

const { t } = useI18n();
const canvasRef = ref<HTMLCanvasElement | null>(null);

let positions = new Map<string, LayoutPoint>();
let resizeObserver: ResizeObserver | null = null;

const NODE_R = 5;

function draw() {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const parent = canvas.parentElement;
  if (!parent) return;
  const w = parent.clientWidth;
  const h = Math.max(180, parent.clientHeight - 44);
  const dpr = window.devicePixelRatio || 1;
  canvas.width = w * dpr;
  canvas.height = h * dpr;
  canvas.style.width = `${w}px`;
  canvas.style.height = `${h}px`;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  ctx.scale(dpr, dpr);
  ctx.clearRect(0, 0, w, h);

  positions = layoutLocalGraph(props.nodes, props.edges, w, h);

  // 边
  const styles = getComputedStyle(canvas);
  const cyan = styles.getPropertyValue('--kb-accent-cyan').trim() || '#4DD8E8';
  const dim = styles.getPropertyValue('--kb-text-dim').trim() || '#7A8EAA';
  ctx.lineWidth = 1;
  for (const e of props.edges) {
    const a = positions.get(e.source);
    const b = positions.get(e.target);
    if (!a || !b) continue;
    ctx.strokeStyle = e.type === 'explicit' ? `${cyan}55` : `${dim}33`;
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();
  }

  // 节点（根节点高亮加大）
  for (const n of props.nodes) {
    const p = positions.get(n.doc_id);
    if (!p) continue;
    const isRoot = n.doc_id === props.rootId;
    ctx.beginPath();
    ctx.arc(p.x, p.y, isRoot ? NODE_R + 2 : NODE_R, 0, Math.PI * 2);
    ctx.fillStyle = isRoot ? cyan : `${cyan}aa`;
    ctx.shadowColor = cyan;
    ctx.shadowBlur = isRoot ? 12 : 5;
    ctx.fill();
    ctx.shadowBlur = 0;
    // 标签（根 + 高度节点）
    if (isRoot || n.degree >= 2) {
      ctx.font = '10px sans-serif';
      ctx.fillStyle = dim;
      ctx.fillText(n.name, p.x + NODE_R + 3, p.y + 3);
    }
  }
}

function onClick(e: MouseEvent) {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  const x = e.clientX - rect.left;
  const y = e.clientY - rect.top;
  // 命中检测：最近节点 ≤10px
  let best: { id: string; d: number } | null = null;
  for (const n of props.nodes) {
    const p = positions.get(n.doc_id);
    if (!p) continue;
    const d = Math.hypot(p.x - x, p.y - y);
    if (d <= 10 && (!best || d < best.d)) best = { id: n.doc_id, d };
  }
  if (best && best.id !== props.rootId) emit('open-doc-id', best.id);
}

onMounted(() => {
  draw();
  const parent = canvasRef.value?.parentElement;
  if (parent) {
    resizeObserver = new ResizeObserver(() => draw());
    resizeObserver.observe(parent);
  }
});

watch(() => [props.nodes, props.edges, props.rootId], draw, { deep: false });

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
});
</script>

<style lang="sass" scoped>
.kb-local-graph
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0

  &__canvas
    flex: 1
    min-height: 0
    cursor: pointer

  &__footer
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 4px 0
    flex: none

  &__slider
    flex: 1

  &__expand
    color: var(--kb-text-dim)

    &:hover
      color: var(--kb-accent-cyan)
</style>
