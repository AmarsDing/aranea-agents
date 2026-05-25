// Container: approved — L4 neighborhood BFS explorer for memory center graph tab.
<template>
  <div class="row q-col-gutter-md">
    <div class="col-12 col-md-4">
      <q-card flat bordered class="memory-card">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">实体列表</div>
          <q-btn flat dense icon="refresh" :loading="loadingEntities" @click="$emit('refresh')" />
        </q-card-section>
        <q-list separator dense style="max-height: 420px; overflow: auto">
          <q-item
            v-for="entity in entities"
            :key="entity.id"
            clickable
            v-ripple
            :active="entity.id === selectedId"
            active-class="memory-active-item"
            @click="selectEntity(entity.id)"
          >
            <q-item-section>
              <q-item-label>{{ entity.name }}</q-item-label>
              <q-item-label caption>{{ entity.entity_type }} · {{ entity.scope_type }}</q-item-label>
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
          <q-select v-model="hops" :options="hopOptions" dense outlined style="width: 100px" label="Hops" />
          <q-btn color="primary" dense label="展开" :loading="loadingGraph" :disable="!selectedId" @click="loadNeighborhood" />
        </q-card-section>

        <q-banner v-if="graphError" rounded class="bg-negative text-white q-ma-md">{{ graphError }}</q-banner>

        <q-card-section v-if="neighborhood">
          <div class="text-subtitle2 q-mb-sm">
            中心：{{ neighborhood.center?.name || selectedId }}
            <span class="text-grey-7"> · {{ neighborhood.entities.length }} nodes · {{ neighborhood.relations.length }} edges</span>
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
                stroke="var(--q-primary)"
                stroke-opacity="0.35"
                stroke-width="1.5"
              />
              <g v-for="node in layoutNodes" :key="node.id">
                <circle
                  :cx="node.x"
                  :cy="node.y"
                  :r="node.id === selectedId ? 14 : 10"
                  :fill="node.id === selectedId ? 'var(--q-primary)' : 'var(--color-info-soft)'"
                  stroke="var(--q-primary)"
                  stroke-width="1"
                />
                <text :x="node.x" :y="node.y + 24" text-anchor="middle" class="memory-graph-label">{{ node.label }}</text>
              </g>
            </svg>
          </div>

          <q-markup-table flat dense wrap-cells>
            <thead>
              <tr>
                <th>Source</th>
                <th>Relation</th>
                <th>Target</th>
                <th class="text-right">Weight</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="rel in neighborhood.relations" :key="rel.id">
                <td>{{ entityName(rel.source_id) }}</td>
                <td><q-badge outline color="primary">{{ rel.relation_type }}</q-badge></td>
                <td>{{ entityName(rel.target_id) }}</td>
                <td class="text-right">{{ rel.weight.toFixed(2) }}</td>
              </tr>
            </tbody>
          </q-markup-table>
        </q-card-section>

        <q-card-section v-else-if="!loadingGraph" class="text-grey-7">
          点击左侧实体，或使用 Hops 展开 neighborhood 图。
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { GraphNeighborhood, MemoryEntity } from "./types";
import { getMemoryNeighborhood } from "./api";

const props = defineProps<{
  entities: MemoryEntity[];
  loadingEntities?: boolean;
}>();

defineEmits<{ refresh: [] }>();

const selectedId = ref<string | null>(null);
const hops = ref(2);
const hopOptions = [1, 2, 3];
const neighborhood = ref<GraphNeighborhood | null>(null);
const loadingGraph = ref(false);
const graphError = ref("");

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

const layoutNodes = computed(() => {
  if (!neighborhood.value) return [];
  const nodes = neighborhood.value.entities.length ? neighborhood.value.entities : [neighborhood.value.center];
  const cx = svgW / 2;
  const cy = svgH / 2;
  const r = Math.min(svgW, svgH) * 0.32;
  return nodes.map((n, i) => {
    const angle = (2 * Math.PI * i) / Math.max(nodes.length, 1);
    const isCenter = n.id === selectedId.value;
    return {
      id: n.id,
      label: truncate(n.name || n.id, 12),
      x: isCenter ? cx : cx + r * Math.cos(angle),
      y: isCenter ? cy : cy + r * Math.sin(angle)
    };
  });
});

const layoutEdges = computed(() => {
  if (!neighborhood.value) return [];
  const pos = new Map(layoutNodes.value.map((n) => [n.id, n]));
  const out: Array<{ key: string; x1: number; y1: number; x2: number; y2: number }> = [];
  for (const rel of neighborhood.value.relations) {
    const a = pos.get(rel.source_id);
    const b = pos.get(rel.target_id);
    if (!a || !b) continue;
    out.push({ key: rel.id, x1: a.x, y1: a.y, x2: b.x, y2: b.y });
  }
  return out;
});

watch(
  () => props.entities,
  () => {
    if (selectedId.value && !props.entities.some((e) => e.id === selectedId.value)) {
      selectedId.value = null;
      neighborhood.value = null;
    }
  }
);

function selectEntity(id: string) {
  selectedId.value = id;
  void loadNeighborhood();
}

async function loadNeighborhood() {
  if (!selectedId.value) return;
  loadingGraph.value = true;
  graphError.value = "";
  try {
    neighborhood.value = await getMemoryNeighborhood(selectedId.value, { hops: hops.value, max_nodes: 48 });
  } catch (err) {
    neighborhood.value = null;
    graphError.value = err instanceof Error ? err.message : "加载 neighborhood 失败";
  } finally {
    loadingGraph.value = false;
  }
}

function entityName(id: string) {
  return entityMap.value.get(id)?.name || id.slice(0, 8);
}

function truncate(s: string, n: number) {
  return s.length <= n ? s : `${s.slice(0, n)}…`;
}
</script>
