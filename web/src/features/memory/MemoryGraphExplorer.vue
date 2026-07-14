// Container: approved — L4 neighborhood BFS explorer for memory center graph tab.
<template>
  <div class="row q-col-gutter-md">
    <div class="col-12 col-md-4">
      <q-card flat bordered class="memory-card">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">实体列表</div>
          <q-btn flat dense icon="refresh" :loading="loadingEntities" @click="$emit('refresh')" />
        </q-card-section>
        <q-list separator dense class="memory-graph-entity-list">
          <q-item
            v-for="entity in entities"
            :key="entity.id"
            v-ripple
            clickable
            :active="entity.id === selectedId"
            active-class="memory-active-item"
            @click="selectEntity(entity.id)"
          >
            <q-item-section>
              <q-item-label>
                {{ entity.name }}
                <q-badge
                  :color="confidenceTierColor(entity.confidence)"
                  :label="confidenceTierLabel(entity.confidence)"
                  class="q-ml-xs"
                />
              </q-item-label>
              <q-item-label caption
                >{{ entity.entity_type }} · {{ entity.scope_type }} ·
                {{ (entity.confidence * 100).toFixed(0) }}%</q-item-label
              >
            </q-item-section>
          </q-item>
          <q-item v-if="!entities.length && !loadingEntities">
            <q-item-section class="text-grey-7">选择 Agent 后加载 L4 实体。</q-item-section>
          </q-item>
        </q-list>
      </q-card>
    </div>

    <div class="col-12 col-md-8">
      <q-card flat bordered class="memory-card">
        <q-card-section class="row items-center q-gutter-sm">
          <div class="text-h6">Neighborhood BFS</div>
          <q-space />
          <q-select v-model="hops" :options="hopOptions" dense outlined class="memory-graph-hops-select" label="Hops" />
          <q-btn
            color="primary"
            dense
            label="展开"
            :loading="loadingGraph"
            :disable="!selectedId"
            @click="loadNeighborhood"
          />
          <q-btn
            color="secondary"
            dense
            label="Spreading"
            :loading="loadingActivation"
            :disable="!selectedId"
            @click="runActivation"
          >
            <q-tooltip
              >Run spreading activation from the selected center node; highlights Top-K activated nodes</q-tooltip
            >
          </q-btn>
        </q-card-section>

        <q-banner v-if="graphError" rounded class="bg-negative text-white q-ma-md">{{ graphError }}</q-banner>
        <q-banner v-if="activationError" rounded class="bg-negative text-white q-ma-md">{{ activationError }}</q-banner>

        <q-card-section v-if="neighborhood">
          <div class="text-subtitle2 q-mb-sm">
            中心：{{ neighborhood.center?.name || selectedId }}
            <span class="text-grey-7">
              · {{ neighborhood.entities.length }} nodes · {{ neighborhood.relations.length }} edges</span
            >
            <span v-if="activationResult" class="text-grey-7">
              · activated {{ activationResult.items.length }} nodes</span
            >
          </div>

          <div class="memory-graph-canvas q-mb-md">
            <svg :viewBox="`0 0 ${svgW} ${svgH}`" class="memory-graph-svg">
              <line
                v-for="edge in layoutEdges"
                :key="edge.key"
                :x1="edge.x1"
                :y1="edge.y1"
                :x2="edge.x2"
                :y2="edge.y2"
                :stroke="edge.inhibit ? 'var(--q-negative)' : 'var(--q-primary)'"
                :stroke-opacity="edge.inhibit ? 0.7 : 0.35"
                :stroke-width="edge.inhibit ? 1.8 : 1.5"
                :stroke-dasharray="edge.inhibit ? '4 3' : 'none'"
              />
              <g v-for="node in layoutNodes" :key="node.id">
                <circle
                  :cx="node.x"
                  :cy="node.y"
                  :r="node.radius"
                  :fill="node.fill"
                  :stroke="node.stroke"
                  :stroke-width="node.id === selectedId ? 2 : 1"
                />
                <text :x="node.x" :y="node.y + node.radius + 12" text-anchor="middle" class="memory-graph-label">
                  {{ node.label }}
                </text>
                <text
                  v-if="node.activationLabel"
                  :x="node.x"
                  :y="node.y - node.radius - 6"
                  text-anchor="middle"
                  class="memory-graph-activation"
                >
                  {{ node.activationLabel }}
                </text>
              </g>
            </svg>
          </div>

          <AppRegistryMarkupTable
            :rows="neighborhood.relations"
            :columns="relationColumns as RegistryTableColumn[]"
            row-key="id"
          >
            <template #cell-source_id="{ row }">
              {{ entityName(String(row.source_id)) }}
            </template>
            <template #cell-relation_type="{ row }">
              <q-badge :color="String(row.relation_type) === 'INHIBIT' ? 'negative' : 'primary'" outline>
                {{ row.relation_type }}
              </q-badge>
            </template>
            <template #cell-target_id="{ row }">
              {{ entityName(String(row.target_id)) }}
            </template>
            <template #cell-weight="{ row }">
              {{ Number(row.weight).toFixed(2) }}
            </template>
          </AppRegistryMarkupTable>
        </q-card-section>

        <q-card-section v-else-if="!loadingGraph" class="text-grey-7">
          点击左侧实体，或使用 Hops 展开 neighborhood 图。
        </q-card-section>

        <q-card-section v-if="activationResult" class="memory-activation-panel">
          <div class="text-subtitle2 q-mb-sm">Activation Path Explanation</div>
          <q-list dense separator>
            <q-item v-for="item in activationResult.items" :key="item.node_id" class="memory-activation-item">
              <q-item-section avatar>
                <q-badge :color="activationBadgeColor(item.activation)" :label="item.activation.toFixed(3)" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ entityName(item.node_id) }}</q-item-label>
                <q-item-label caption>
                  hop={{ item.hop_count }}
                  <span v-if="item.activation_path.length" class="q-ml-sm">
                    · path:
                    <template v-for="(step, idx) in item.activation_path" :key="idx">
                      <span v-if="idx > 0"> → </span>
                      <span>{{ entityName(step.to_node_id) }}</span>
                      <span class="text-grey-7"> ({{ step.relation_type }}={{ step.edge_weight.toFixed(2) }})</span>
                    </template>
                  </span>
                  <span v-else class="text-grey-7"> · center node (direct activation)</span>
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import AppRegistryMarkupTable from '../../components/layout/AppRegistryMarkupTable.vue';
import { RELATION_COLUMNS } from './memoryTableUi';
import type { MemoryEntity } from './types';
import type { RegistryTableColumn } from '../ui/registryTableColumns';
import { useMemoryGraphExplorer } from './composables/useMemoryGraphExplorer';
import { useSpreadingActivation } from './composables/useSpreadingActivation';
const {
  neighborhood,
  loadingGraph,
  graphError,
  loadNeighborhood: fetchNeighborhood,
  resetNeighborhood,
} = useMemoryGraphExplorer();
const {
  result: activationResult,
  loadingActivation,
  activationError,
  runSpreadingActivation,
  resetActivation,
} = useSpreadingActivation();

const relationColumns = RELATION_COLUMNS;

const props = defineProps<{
  entities: MemoryEntity[];
  loadingEntities?: boolean;
}>();

defineEmits<{ refresh: [] }>();

const selectedId = ref<string | null>(null);
const hops = ref(2);
const hopOptions = [1, 2, 3];

const svgW = 520;
const svgH = 320;

const entityMap = computed(() => {
  const m = new Map<string, MemoryEntity>();
  for (const e of props.entities) m.set(e.id, e);
  if (neighborhood.value) {
    for (const e of neighborhood.value.entities) m.set(e.id, e);
    if (neighborhood.value.center?.id) m.set(neighborhood.value.center.id, neighborhood.value.center);
  }
  return m;
});

// 激活值映射：node_id -> activation，用于在图上以渐变色高亮 Top-K 节点。
const activationMap = computed(() => {
  const m = new Map<string, number>();
  if (!activationResult.value) return m;
  for (const item of activationResult.value.items) m.set(item.node_id, item.activation);
  return m;
});

const maxActivation = computed(() => {
  let max = 0;
  for (const v of activationMap.value.values()) if (v > max) max = v;
  return max > 0 ? max : 1;
});

const layoutNodes = computed(() => {
  if (!neighborhood.value) return [];
  const nodes = neighborhood.value.entities.length ? neighborhood.value.entities : [neighborhood.value.center];
  const cx = svgW / 2;
  const cy = svgH / 2;
  const r = Math.min(svgW, svgH) * 0.32;
  return nodes.map((n, i) => {
    const angle = (2 * Math.PI * i) / Math.max(nodes.length, 1);
    const isCenter = n.id === selectedId.value;
    const activation = activationMap.value.get(n.id);
    const hasActivation = activation !== undefined;
    // 激活强度归一化到 [0,1]，用于半径与填充色渐变。
    const norm = hasActivation ? Math.max(0, Math.min(1, activation / maxActivation.value)) : 0;
    return {
      id: n.id,
      label: truncate(n.name || n.id, 12),
      x: isCenter ? cx : cx + r * Math.cos(angle),
      y: isCenter ? cy : cy + r * Math.sin(angle),
      radius: isCenter ? 14 : hasActivation ? 10 + norm * 4 : 10,
      fill: nodeFill(isCenter, hasActivation, norm),
      stroke: nodeStroke(isCenter, hasActivation),
      activationLabel: hasActivation ? activation.toFixed(2) : '',
    };
  });
});

const layoutEdges = computed(() => {
  if (!neighborhood.value) return [];
  const pos = new Map(layoutNodes.value.map((n) => [n.id, n]));
  const out: Array<{ key: string; x1: number; y1: number; x2: number; y2: number; inhibit: boolean }> = [];
  for (const rel of neighborhood.value.relations) {
    const a = pos.get(rel.source_id);
    const b = pos.get(rel.target_id);
    if (!a || !b) continue;
    out.push({
      key: rel.id,
      x1: a.x,
      y1: a.y,
      x2: b.x,
      y2: b.y,
      inhibit: rel.relation_type === 'INHIBIT',
    });
  }
  return out;
});

watch(
  () => props.entities,
  () => {
    if (selectedId.value && !props.entities.some((e) => e.id === selectedId.value)) {
      selectedId.value = null;
      resetNeighborhood();
      resetActivation();
    }
  },
);

function selectEntity(id: string) {
  selectedId.value = id;
  resetActivation();
  void fetchNeighborhood(id, hops.value);
}

async function loadNeighborhood() {
  if (!selectedId.value) return;
  resetActivation();
  await fetchNeighborhood(selectedId.value, hops.value);
}

async function runActivation() {
  if (!selectedId.value) return;
  await runSpreadingActivation(selectedId.value, { hops: hops.value, top_k: 12 });
}

function entityName(id: string) {
  return entityMap.value.get(id)?.name || id.slice(0, 8);
}

function truncate(s: string, n: number) {
  return s.length <= n ? s : `${s.slice(0, n)}…`;
}

// 中心节点：主色填充；激活节点：按强度从浅橙 #FFF3E0 到深橙 #FF6F00 渐变；其余：浅灰。
function nodeFill(isCenter: boolean, hasActivation: boolean, norm: number): string {
  if (isCenter) return 'var(--q-primary)';
  if (!hasActivation) return 'var(--color-info-soft)';
  const g = Math.round(243 - (243 - 111) * norm);
  const b = Math.round(224 - 224 * norm);
  return `rgb(255, ${g}, ${b})`;
}

function nodeStroke(isCenter: boolean, hasActivation: boolean): string {
  if (isCenter) return 'var(--q-primary)';
  if (hasActivation) return '#FF6F00';
  return 'var(--q-primary)';
}

function activationBadgeColor(activation: number): string {
  if (activation >= 0.5) return 'positive';
  if (activation >= 0.2) return 'warning';
  return 'grey-6';
}

function confidenceTierLabel(confidence: number) {
  if (confidence >= 0.7) return '高';
  if (confidence >= 0.4) return '中';
  return '低';
}

function confidenceTierColor(confidence: number) {
  if (confidence >= 0.7) return 'positive';
  if (confidence >= 0.4) return 'warning';
  return 'negative';
}
</script>

<style scoped>
.memory-graph-entity-list {
  max-height: 420px;
  overflow: auto;
}

.memory-graph-hops-select {
  width: 100px;
}

.memory-graph-activation {
  font-size: 10px;
  fill: #ff6f00;
  font-weight: 600;
}

.memory-activation-panel {
  border-top: 1px solid var(--q-edge);
}

.memory-activation-item {
  min-height: 40px;
}
</style>
