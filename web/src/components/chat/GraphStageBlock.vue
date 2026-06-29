<template>
  <div class="graph-stage-block" :class="`graph-stage-block--${activity.status}`">
    <!-- Header (collapsible after terminal state) -->
    <div class="graph-stage-block__header" @click="toggleCollapse">
      <span class="graph-stage-block__icon">🔀</span>
      <span class="graph-stage-block__label">{{ t('chat.graphStage.dependsLabel') }}</span>
      <span v-if="progressText" class="graph-stage-block__progress">{{ progressText }}</span>
      <span
        v-if="activity.nodes?.length && effectiveStatus !== 'running'"
        class="graph-stage-block__chevron"
        :class="{ 'graph-stage-block__chevron--expanded': !collapsed }"
      >
        ▾
      </span>
      <span class="graph-stage-block__status" :class="`graph-stage-block__status--${effectiveStatus}`">
        {{ statusIcon }}
      </span>
    </div>

    <!-- Duration -->
    <div v-if="activity.durationMs != null && isTerminal" class="graph-stage-block__duration">
      {{ formatDuration(activity.durationMs) }}
    </div>

    <!-- DAG flow diagram (layered by kahnTopoLayers, aligned with m59 v7 design) -->
    <div v-if="showDag" class="graph-stage-block__dag">
      <template v-for="(layer, layerIdx) in dagLayers" :key="`layer-${layerIdx}`">
        <div v-if="layerIdx > 0" class="graph-stage-block__layer-sep">↓</div>
        <div class="graph-stage-block__layer">
          <span
            v-for="node in layer"
            :key="node.nodeId"
            class="graph-stage-block__node"
            :class="`graph-stage-block__node--${node.status}`"
            :title="nodeTitle(node)"
          >
            <span class="graph-stage-block__node-icon">{{ nodeStatusIcon(node.status) }}</span>
            <span class="graph-stage-block__node-label">{{ node.label || node.nodeId }}</span>
            <span v-if="node.dependsOn?.length" class="graph-stage-block__node-deps">
              ← {{ node.dependsOn.join(', ') }}
            </span>
          </span>
        </div>
      </template>
    </div>

    <!-- Empty hint when no nodes are present -->
    <div v-else-if="showDag === false && !activity.nodes?.length" class="graph-stage-block__empty">
      {{ t('chat.graphStage.noNodes') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphStageEvent, GraphNodeStatus } from '../../features/chat/streamEventTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { kahnTopoLayers } from '../../features/spirit/lib/dagTopoSort';

const props = defineProps<{
  activity: GraphStageEvent;
}>();

const { t } = useI18n();

// B.4.5: graph_stage 始终展开（不自动折叠）。终态切换时也不折叠，
// 以保持 DAG 可见性 — 终态自动折叠 ❌ 违反设计。
const collapsed = ref(false);

function toggleCollapse() {
  if (effectiveStatus.value === 'running') return;
  collapsed.value = !collapsed.value;
}

const effectiveStatus = computed(() => props.activity.status);

const isTerminal = computed(
  () =>
    effectiveStatus.value === 'completed' ||
    effectiveStatus.value === 'failed' ||
    effectiveStatus.value === 'cancelled',
);

const progressText = computed(() => {
  const nodes = props.activity.nodes;
  if (!nodes?.length) return '';
  const completed = nodes.filter(
    (n) => n.status === 'completed' || n.status === 'failed' || n.status === 'skipped',
  ).length;
  return `${completed}/${nodes.length}`;
});

const showDag = computed(() => {
  if (!props.activity.nodes?.length) return false;
  if (effectiveStatus.value === 'running') return true;
  return !collapsed.value;
});

/** Group nodes into layered rows using Kahn topological sort.
 *  Each layer becomes a horizontal row; arrows between layers are
 *  represented by the `↓` separator. Aligned with the m59 v7 prototype
 *  which renders DAG as a wrap-flex row with `→` arrows — for richer
 *  dependency graphs we layer them vertically by depth. */
const dagLayers = computed<GraphNodeStatus[][]>(() => {
  const nodes = props.activity.nodes;
  if (!nodes?.length) return [];

  const depths = kahnTopoLayers(
    nodes.map((n) => ({ id: n.nodeId, dependsOn: n.dependsOn ?? [] })),
  );

  const maxDepth = Math.max(0, ...Array.from(depths.values()));
  const layers: GraphNodeStatus[][] = Array.from({ length: maxDepth + 1 }, () => []);
  for (const node of nodes) {
    const depth = depths.get(node.nodeId) ?? 0;
    layers[depth].push(node);
  }
  return layers.filter((l) => l.length > 0);
});

const statusIcon = computed(() => {
  switch (effectiveStatus.value) {
    case 'running':
      return '⏳';
    case 'completed':
      return '✓';
    case 'failed':
      return '✗';
    case 'cancelled':
      return '⊘';
    default:
      return '🔀';
  }
});

function nodeStatusIcon(status: GraphNodeStatus['status']): string {
  switch (status) {
    case 'running':
      return '⚡';
    case 'completed':
      return '✓';
    case 'failed':
      return '✗';
    case 'skipped':
      return '⊘';
    case 'pending':
    default:
      return '⏳';
  }
}

function nodeTitle(node: GraphNodeStatus): string {
  const parts = [node.label || node.nodeId, node.status];
  if (node.dependsOn?.length) parts.push(`depends: ${node.dependsOn.join(', ')}`);
  return parts.join(' · ');
}
</script>

<style lang="sass" scoped>
.graph-stage-block
  padding: 8px 10px
  border-radius: 8px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &--running
    border-color: color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))
  &--failed
    border-color: color-mix(in srgb, var(--color-danger) 40%, var(--glass-border))
  &--cancelled
    opacity: 0.7

  &__header
    display: flex
    align-items: center
    gap: 6px
    cursor: default

  &__icon
    font-size: 13px
    flex-shrink: 0

  &__label
    font-size: 12px
    font-weight: 500
    color: var(--color-text-secondary)
    flex: 1

  &__progress
    font-size: 11px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)
    padding: 0 5px
    border-radius: 8px
    color: var(--color-accent)

  &__status
    font-size: 12px
    flex-shrink: 0
    &--running
      color: var(--color-accent)
    &--completed
      color: var(--color-success)
    &--failed
      color: var(--color-danger)
    &--cancelled
      color: var(--color-text-tertiary)

  &__chevron
    font-size: 10px
    color: var(--color-text-tertiary)
    transition: transform 0.15s ease
    &--expanded
      transform: rotate(180deg)

  &__duration
    font-size: 11px
    color: var(--color-text-secondary)
    margin-top: 2px
    margin-left: 22px

  &__dag
    margin-top: 8px
    padding-left: 22px
    display: flex
    flex-direction: column
    align-items: flex-start
    gap: 2px

  &__layer
    display: flex
    align-items: center
    gap: 6px
    flex-wrap: wrap

  &__layer-sep
    font-size: 12px
    color: var(--color-text-tertiary)
    line-height: 1
    align-self: center
    margin: 1px 0 1px 22px

  &__node
    display: inline-flex
    align-items: center
    gap: 3px
    padding: 3px 8px
    border-radius: 4px
    font-size: 11px
    background: color-mix(in srgb, var(--color-accent) 10%, transparent)
    color: var(--color-text-secondary)
    transition: all 0.15s ease

    &--pending
      background: color-mix(in srgb, var(--color-text-tertiary) 10%, transparent)
      color: var(--color-text-tertiary)
    &--running
      background: color-mix(in srgb, var(--color-accent) 18%, transparent)
      color: var(--color-accent)
      font-weight: 500
    &--completed
      opacity: 0.6
      text-decoration: line-through
    &--failed
      background: color-mix(in srgb, var(--color-danger) 15%, transparent)
      color: var(--color-danger)
    &--skipped
      opacity: 0.4
      text-decoration: line-through

  &__node-icon
    font-size: 10px
    flex-shrink: 0

  &__node-label
    font-size: 11px

  &__node-deps
    font-size: 9px
    color: var(--color-text-tertiary)
    margin-left: 2px

  &__empty
    font-size: 11px
    color: var(--color-text-tertiary)
    margin-top: 6px
    padding-left: 22px
    font-style: italic
</style>
