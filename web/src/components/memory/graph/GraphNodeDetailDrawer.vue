// Presentation: 跨层图谱节点详情抽屉 — 摘要/元信息/连接列表 + 「在记忆浏览中打开」跳转（FR-R8）。
<template>
  <q-drawer
    :model-value="modelValue"
    side="right"
    overlay
    bordered
    :width="340"
    class="graph-detail"
    @update:model-value="(v) => emit('update:modelValue', v)"
  >
    <div v-if="node" class="column full-height">
      <div class="row items-center q-pa-md graph-detail__header">
        <q-badge :style="{ background: memoryLayerColor(node.layer) }" rounded class="graph-detail__dot" />
        <div class="text-subtitle1 col ellipsis">{{ t('memory.unifiedGraph.detail.title') }}</div>
        <q-btn flat round dense icon="close" @click="emit('update:modelValue', false)" />
      </div>
      <q-separator />

      <q-scroll-area class="col">
        <div class="q-pa-md column q-gutter-md">
          <div>
            <div class="text-body1 text-weight-medium graph-detail__label">{{ node.label }}</div>
            <div class="row q-gutter-sm q-mt-xs">
              <q-badge
                outline
                :style="{ color: memoryLayerColor(node.layer), borderColor: memoryLayerColor(node.layer) }"
              >
                {{ node.layer }} · {{ t(`memory.panorama.layers.${node.layer}.name`) }}
              </q-badge>
              <q-badge outline color="grey-7">{{ kindLabel }}</q-badge>
            </div>
          </div>

          <q-list dense class="graph-detail__kv">
            <q-item dense>
              <q-item-section side class="text-caption text-grey-7">{{
                t('memory.unifiedGraph.detail.weight')
              }}</q-item-section>
              <q-item-section class="text-right">{{ node.weight.toFixed(2) }}</q-item-section>
            </q-item>
          </q-list>

          <div v-if="metaEntries.length">
            <div class="text-subtitle2 q-mb-xs">{{ t('memory.unifiedGraph.detail.meta') }}</div>
            <q-list dense bordered class="graph-detail__meta">
              <q-item v-for="entry in metaEntries" :key="entry.key" dense>
                <q-item-section side class="text-caption text-grey-7">{{ entry.key }}</q-item-section>
                <q-item-section class="text-right graph-detail__meta-value" :title="entry.value">
                  {{ entry.value }}
                </q-item-section>
              </q-item>
            </q-list>
          </div>

          <div>
            <div class="text-subtitle2 q-mb-xs">
              {{ t('memory.unifiedGraph.detail.connections', { count: edges.length }) }}
            </div>
            <q-list v-if="connections.length" dense bordered class="graph-detail__conns">
              <q-item v-for="conn in connections" :key="conn.key" dense>
                <q-item-section avatar>
                  <q-icon
                    :name="conn.outgoing ? 'north_east' : 'south_west'"
                    size="16px"
                    :title="
                      t(conn.outgoing ? 'memory.unifiedGraph.detail.outgoing' : 'memory.unifiedGraph.detail.incoming')
                    "
                  />
                </q-item-section>
                <q-item-section>
                  <q-item-label class="ellipsis" :title="conn.otherLabel">{{ conn.otherLabel }}</q-item-label>
                  <q-item-label caption>
                    <q-badge
                      :style="{ background: memoryLayerColor(conn.otherLayer) }"
                      rounded
                      class="graph-detail__mini-dot"
                    />
                    {{ conn.typeLabel }} · {{ conn.weight.toFixed(2) }}
                    <q-badge v-if="conn.polarity === 'INHIBIT'" color="negative" class="q-ml-xs">INHIBIT</q-badge>
                  </q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
            <div v-else class="text-caption text-grey-7">{{ t('memory.unifiedGraph.detail.noConnections') }}</div>
          </div>
        </div>
      </q-scroll-area>

      <q-separator />
      <div class="q-pa-md column q-gutter-sm">
        <q-btn
          v-if="node.kind === 'entity'"
          unelevated
          rounded
          color="secondary"
          no-caps
          icon="play_circle"
          class="full-width"
          :label="t('memory.unifiedGraph.detail.replayActivation')"
          @click="emit('replay-activation', node)"
        />
        <q-btn
          unelevated
          rounded
          color="primary"
          no-caps
          icon="open_in_new"
          class="full-width"
          :label="t('memory.unifiedGraph.detail.openInBrowse')"
          @click="emit('open-in-browse', node)"
        />
      </div>
    </div>
  </q-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { UnifiedGraphEdge, UnifiedGraphNode } from '../../../features/memory/types';
import { memoryLayerColor } from '../../../features/memory/panorama/layerMeta';

const props = defineProps<{
  modelValue: boolean;
  node: UnifiedGraphNode | null;
  /** 与选中节点相连的边。 */
  edges: UnifiedGraphEdge[];
  /** 当前图谱全部节点（用于解析对端标签）。 */
  nodes: UnifiedGraphNode[];
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'open-in-browse', node: UnifiedGraphNode): void;
  (e: 'replay-activation', node: UnifiedGraphNode): void;
}>();

const { t } = useI18n();

const kindLabel = computed(() => {
  if (!props.node) return '';
  if (props.node.kind === 'entity') return t('memory.unifiedGraph.detail.kindEntity');
  if (props.node.kind === 'episode') return t('memory.unifiedGraph.detail.kindEpisode');
  return t('memory.unifiedGraph.detail.kindFact');
});

const metaEntries = computed(() => {
  if (!props.node?.meta_json) return [] as { key: string; value: string }[];
  try {
    const parsed = JSON.parse(props.node.meta_json) as Record<string, unknown>;
    return Object.entries(parsed).map(([key, value]) => ({
      key,
      value: typeof value === 'string' ? value : JSON.stringify(value),
    }));
  } catch {
    return [] as { key: string; value: string }[];
  }
});

const connections = computed(() => {
  if (!props.node) return [];
  const byId = new Map(props.nodes.map((n) => [n.id, n]));
  return props.edges.map((edge) => {
    const outgoing = edge.source === props.node!.id;
    const other = byId.get(outgoing ? edge.target : edge.source);
    return {
      key: `${edge.source}->${edge.target}:${edge.type}`,
      outgoing,
      otherLabel: other?.label ?? (outgoing ? edge.target : edge.source),
      otherLayer: other?.layer ?? '',
      typeLabel: t(`memory.unifiedGraph.edgeTypes.${edge.type}`),
      weight: edge.weight,
      polarity: edge.polarity,
    };
  });
});
</script>

<style scoped>
.graph-detail__dot {
  display: inline-block;
  height: 12px;
  margin-right: 8px;
  width: 12px;
}

.graph-detail__mini-dot {
  display: inline-block;
  height: 8px;
  margin-right: 4px;
  width: 8px;
}

.graph-detail__label {
  color: var(--color-text-heading);
  word-break: break-all;
}

.graph-detail__meta,
.graph-detail__conns {
  border-radius: 8px;
  max-height: 260px;
  overflow-y: auto;
}

.graph-detail__meta-value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
