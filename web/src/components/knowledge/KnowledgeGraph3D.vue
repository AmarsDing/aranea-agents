<template>
  <!-- G4 3D 知识图谱（V12.7）：左 3D 力导向图 + 右操作台。 -->
  <div class="knowledge-graph">
    <!-- 左：3D 画布区 -->
    <q-card flat class="app-pane-card knowledge-graph__stage">
      <div v-if="loading" class="knowledge-graph__overlay">
        <q-spinner-dots color="primary" size="32px" />
        <span>{{ t('knowledgePage.graphLoading') }}</span>
      </div>
      <div v-else-if="error" class="knowledge-graph__overlay knowledge-graph__overlay--error">
        <q-icon name="cloud_off" size="20px" />
        <span>{{ error }}</span>
      </div>
      <div v-else-if="!nodes.length" class="knowledge-graph__overlay">
        <q-icon name="hub" size="20px" />
        <span>{{ t('knowledgePage.graphEmpty') }}</span>
      </div>
      <knowledge-graph-canvas
        v-show="nodes.length && !error"
        ref="canvasRef"
        :nodes="nodes"
        :edges="edges"
        :selected-node-id="selectedNodeId"
        :focus-signal="focusSignal"
        :generation="generation"
        @node-click="$emit('select-node', $event)"
        @background-click="$emit('select-node', '')"
      />
      <!-- 画布工具条（右上浮动）：适应视图 + 配色图例 -->
      <div v-if="nodes.length && !error" class="knowledge-graph__toolbar">
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="fit_screen"
          :aria-label="t('knowledgePage.graphFitView')"
          @click="fitView"
        >
          <q-tooltip>{{ t('knowledgePage.graphFitView') }}</q-tooltip>
        </q-btn>
        <q-btn flat dense round size="sm" icon="palette" :aria-label="t('knowledgePage.graphLegendTitle')">
          <q-tooltip>{{ t('knowledgePage.graphLegendTitle') }}</q-tooltip>
          <q-menu anchor="top right" self="top right" class="knowledge-graph__legend">
            <div class="knowledge-graph__legend-section">
              <div class="knowledge-graph__legend-title">{{ t('knowledgePage.graphLegendNodes') }}</div>
              <div v-for="item in nodeLegend" :key="item.type" class="knowledge-graph__legend-row">
                <span class="knowledge-graph__chip-dot" :style="{ background: item.color }" />
                <span class="ellipsis">{{ item.type }}</span>
                <span class="knowledge-graph__legend-count">{{ item.count }}</span>
              </div>
            </div>
            <div class="knowledge-graph__legend-section">
              <div class="knowledge-graph__legend-title">{{ t('knowledgePage.graphLegendEdges') }}</div>
              <div v-for="lt in graphLinkTypes" :key="lt" class="knowledge-graph__legend-row">
                <span class="knowledge-graph__chip-dot" :style="{ background: graphLinkColor(lt) }" />
                <span>{{ t(`knowledgePage.linkType${lt.charAt(0).toUpperCase() + lt.slice(1)}`) }}</span>
              </div>
            </div>
          </q-menu>
        </q-btn>
      </div>
      <div class="knowledge-graph__stats">
        {{ t('knowledgePage.graphStats', { nodes: totalNodes, edges: totalEdges }) }}
        <span v-if="hiddenIsolated" class="knowledge-graph__stats-hidden">
          · {{ t('knowledgePage.graphIsolatedHidden', { count: hiddenIsolated }) }}
        </span>
      </div>
    </q-card>

    <!-- 右：操作台 -->
    <q-card flat class="app-pane-card knowledge-graph__console">
      <q-card-section class="knowledge-graph__console-body">
        <!-- 库选择 -->
        <q-select
          :model-value="collectionId"
          :options="collectionOptions"
          emit-value
          map-options
          dense
          outlined
          :label="t('knowledgePage.graphVaultLabel')"
          @update:model-value="(v: string) => $emit('select-collection', v)"
        />

        <!-- 边类型过滤 chips -->
        <div class="knowledge-graph__section">
          <div class="knowledge-graph__section-title">{{ t('knowledgePage.graphLinkTypes') }}</div>
          <div class="knowledge-graph__chips">
            <q-chip
              v-for="lt in graphLinkTypes"
              :key="lt"
              dense
              clickable
              :outline="!linkTypes.includes(lt)"
              :class="{ 'knowledge-graph__chip--off': !linkTypes.includes(lt) }"
              @click="$emit('toggle-link-type', lt)"
            >
              <span class="knowledge-graph__chip-dot" :style="{ background: graphLinkColor(lt) }" />
              {{ t(`knowledgePage.linkType${lt.charAt(0).toUpperCase() + lt.slice(1)}`) }}
            </q-chip>
          </div>
        </div>

        <!-- 目录前缀过滤（复用 V12.6 范围选择器）+ 孤立节点开关 -->
        <div class="knowledge-graph__section knowledge-graph__section--row">
          <knowledge-scope-picker
            :scope-prefix="pathPrefix"
            :scope-nodes="scopeNodes"
            @update:scope-prefix="$emit('set-path-prefix', $event)"
            @scope-lazy-load="$emit('scope-lazy-load', $event)"
          />
          <q-toggle
            :model-value="showIsolated"
            dense
            size="sm"
            :label="t('knowledgePage.graphShowIsolated')"
            @update:model-value="(v: boolean) => $emit('update:show-isolated', v)"
          />
        </div>

        <!-- 节点搜索定位 -->
        <q-input
          :model-value="nodeQuery"
          dense
          outlined
          clearable
          :placeholder="t('knowledgePage.graphNodeSearchPlaceholder')"
          @update:model-value="(v: string | number | null) => $emit('update:node-query', v == null ? '' : String(v))"
        >
          <template #prepend><q-icon name="travel_explore" size="16px" /></template>
        </q-input>

        <!-- 节点列表（连接度排序，点击聚焦） -->
        <div class="knowledge-graph__section knowledge-graph__section--grow">
          <div class="knowledge-graph__section-title">{{ t('knowledgePage.graphNodeList') }}</div>
          <q-list v-if="nodeList.length" dense class="knowledge-graph__node-list">
            <q-item
              v-for="n in nodeList"
              :key="n.doc_id"
              clickable
              :active="n.doc_id === selectedNodeId"
              active-class="knowledge-graph__node--active"
              @click="$emit('focus-node', n.doc_id)"
            >
              <q-item-section>
                <q-item-label lines="1">{{ n.name }}</q-item-label>
                <q-item-label caption lines="1">{{ n.rel_path || n.name }}</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge
                  outline
                  color="accent"
                  :label="n.degree"
                  :title="t('knowledgePage.graphSelectedDegree', { count: n.degree })"
                />
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="knowledge-graph__nodes-empty">{{ t('knowledgePage.graphNodesEmpty') }}</div>
        </div>

        <!-- 选中节点卡 -->
        <div class="knowledge-graph__selected">
          <template v-if="selectedNode">
            <div class="knowledge-graph__selected-name" :title="selectedNode.name">{{ selectedNode.name }}</div>
            <div class="knowledge-graph__selected-path" :title="selectedNode.rel_path">
              {{ selectedNode.rel_path || '—' }}
            </div>
            <div class="knowledge-graph__selected-meta">
              <q-chip v-if="selectedNode.doc_type" dense size="sm" outline>{{ selectedNode.doc_type }}</q-chip>
              <span class="knowledge-graph__selected-degree">
                {{ t('knowledgePage.graphSelectedDegree', { count: selectedNode.degree }) }}
              </span>
            </div>
            <q-btn
              unelevated
              rounded
              no-caps
              dense
              color="primary"
              icon="travel_explore"
              :label="t('knowledgePage.graphOpenInExplorer')"
              class="q-mt-sm"
              @click="$emit('open-in-explorer', { docId: selectedNode.doc_id, relPath: selectedNode.rel_path })"
            />
          </template>
          <div v-else class="knowledge-graph__selected-empty">{{ t('knowledgePage.graphSelectedEmpty') }}</div>
        </div>
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import KnowledgeGraphCanvas from './KnowledgeGraphCanvas.vue';
import KnowledgeScopePicker from './KnowledgeScopePicker.vue';
import { graphDocTypeColor, graphLinkColor, GRAPH_LINK_TYPES } from '../../features/knowledge/graphUi';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../features/knowledge/useVaultExplorer';
import type { CollectionGraphEdge, CollectionGraphNode, KnowledgeCollection } from '../../features/knowledge/types';

const props = defineProps<{
  collections: KnowledgeCollection[];
  /** 当前库 id。 */
  collectionId: string;
  /** 边类型过滤 chips（选中集）。 */
  linkTypes: string[];
  /** 目录前缀过滤（'' = 全库）。 */
  pathPrefix: string;
  /** 渲染节点/边（经孤立裁剪）。 */
  nodes: CollectionGraphNode[];
  edges: CollectionGraphEdge[];
  /** 全量计数（统计行）。 */
  totalNodes: number;
  totalEdges: number;
  /** 被隐藏的孤立节点数。 */
  hiddenIsolated: number;
  loading: boolean;
  error: string;
  /** 数据代际（画布重置视野）。 */
  generation: number;
  showIsolated: boolean;
  nodeQuery: string;
  /** 节点列表（连接度降序 + 搜索过滤）。 */
  nodeList: CollectionGraphNode[];
  selectedNodeId: string;
  selectedNode: CollectionGraphNode | null;
  focusSignal: number;
  /** 范围选择器迷你树根节点。 */
  scopeNodes: VaultQTreeNode[];
}>();

defineEmits<{
  'select-collection': [id: string];
  'toggle-link-type': [type: string];
  'set-path-prefix': [prefix: string];
  'update:show-isolated': [value: boolean];
  'update:node-query': [value: string];
  'select-node': [docId: string];
  'focus-node': [docId: string];
  'open-in-explorer': [payload: { docId: string; relPath: string }];
  'scope-lazy-load': [payload: VaultLazyLoadPayload];
}>();

const { t } = useI18n();
const graphLinkTypes = GRAPH_LINK_TYPES;

const collectionOptions = computed(() => props.collections.map((c) => ({ label: c.name || c.id, value: c.id })));

/** 画布实例（工具条「适应视图」）。 */
const canvasRef = ref<{ zoomToFit: (ms?: number) => void } | null>(null);

function fitView() {
  canvasRef.value?.zoomToFit();
}

/** 节点图例：doc_type 按频次降序取前 8，色板与画布一致。 */
const nodeLegend = computed(() => {
  const counts = new Map<string, number>();
  for (const n of props.nodes) {
    const k = n.doc_type || '';
    counts.set(k, (counts.get(k) ?? 0) + 1);
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 8)
    .map(([type, count]) => ({
      type: type || t('knowledgePage.graphLegendUntyped'),
      count,
      color: graphDocTypeColor(type),
    }));
});
</script>

<style lang="scss" scoped>
.knowledge-graph {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 16px;
  align-items: stretch;

  &__stage {
    position: relative;
    min-height: 560px;
    padding: 8px;
    display: flex;
  }

  &__overlay {
    position: absolute;
    inset: 0;
    z-index: 5;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    font-size: 13px;
    color: var(--color-text-secondary);

    &--error {
      color: var(--q-negative);
    }
  }

  &__stats {
    position: absolute;
    left: 16px;
    bottom: 12px;
    z-index: 4;
    font-size: 11px;
    color: var(--color-text-secondary);
    background: var(--interaction-surface-hover);
    border-radius: 6px;
    padding: 2px 8px;
  }

  // 画布工具条：右上浮动玻璃片，不遮挡节点主区域。
  &__toolbar {
    position: absolute;
    top: 10px;
    right: 10px;
    z-index: 4;
    display: flex;
    gap: 2px;
    padding: 2px 4px;
    border-radius: 8px;
    background: var(--interaction-surface-hover);
    backdrop-filter: blur(6px);
  }

  &__legend {
    min-width: 180px;
    max-width: 240px;
    padding: 10px 12px;
  }

  &__legend-section + &__legend-section {
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--color-border-soft);
  }

  &__legend-title {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--color-text-secondary);
    margin-bottom: 6px;
  }

  &__legend-row {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    padding: 2px 0;
    min-width: 0;
  }

  &__legend-count {
    margin-left: auto;
    font-size: 11px;
    color: var(--color-text-tertiary);
    font-variant-numeric: tabular-nums;
  }

  &__console-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    height: 100%;
    max-height: 560px;
  }

  &__section {
    display: flex;
    flex-direction: column;
    gap: 6px;

    &--row {
      flex-direction: row;
      align-items: center;
      justify-content: space-between;
    }

    &--grow {
      flex: 1;
      min-height: 0;
    }
  }

  &__section-title {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--color-text-secondary);
    text-transform: uppercase;
  }

  &__chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  &__chip-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 4px;
  }

  &__chip--off {
    opacity: 0.55;
  }

  &__node-list {
    flex: 1;
    min-height: 0;
    max-height: 220px;
    overflow-y: auto;
  }

  &__node--active {
    color: var(--q-primary);
    background: var(--interaction-surface-hover);
    border-radius: 6px;
  }

  &__nodes-empty {
    font-size: 12px;
    color: var(--color-text-secondary);
    padding: 8px 0;
  }

  &__selected {
    border-top: 1px solid var(--color-border-soft);
    padding-top: 10px;
  }

  &__selected-name {
    font-size: 14px;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__selected-path {
    font-size: 12px;
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__selected-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 6px;
  }

  &__selected-degree {
    font-size: 12px;
    color: var(--color-text-secondary);
  }

  &__selected-empty {
    font-size: 12px;
    color: var(--color-text-secondary);
  }
}

@media (max-width: 1100px) {
  .knowledge-graph {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
