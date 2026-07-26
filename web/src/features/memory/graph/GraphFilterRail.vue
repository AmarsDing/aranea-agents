// Container: approved — 图谱左侧过滤栏（搜索定位/层级开关/边图例/权重阈值/跳数）；状态来自父组件 v-model。
<template>
  <div class="graph-rail column q-gutter-md">
    <div>
      <div class="text-subtitle2 q-mb-xs">{{ t('memory.unifiedGraph.searchLabel') }}</div>
      <q-input
        v-model="keyword"
        dense
        outlined
        clearable
        :placeholder="t('memory.unifiedGraph.searchPlaceholder')"
        @update:model-value="onKeywordInput"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-list v-if="matches.length" dense bordered class="graph-rail__matches q-mt-xs">
        <q-item v-for="m in matches" :key="m.id" clickable dense @click="emit('locate', m.id)">
          <q-item-section avatar>
            <q-badge :style="{ background: memoryLayerColor(m.layer) }" rounded />
          </q-item-section>
          <q-item-section>{{ m.label }}</q-item-section>
        </q-item>
      </q-list>
    </div>

    <div>
      <div class="text-subtitle2 q-mb-xs">{{ t('memory.unifiedGraph.layerFilter') }}</div>
      <q-toggle
        v-for="layer in GRAPH_LAYERS"
        :key="layer"
        :model-value="enabledLayers.includes(layer)"
        dense
        :label="`${layer} · ${t(`memory.panorama.layers.${layer}.name`)}`"
        @update:model-value="emit('toggle-layer', layer)"
      >
        <q-badge :style="{ background: memoryLayerColor(layer) }" rounded class="q-ml-sm graph-rail__swatch" />
      </q-toggle>
    </div>

    <div>
      <div class="text-subtitle2 q-mb-xs">{{ t('memory.unifiedGraph.edgeLegend') }}</div>
      <div class="graph-rail__legend text-caption">
        <div>
          <span class="graph-rail__line graph-rail__line--solid" />{{ t('memory.unifiedGraph.legend.relation') }}
        </div>
        <div>
          <span class="graph-rail__line graph-rail__line--dashed" />{{ t('memory.unifiedGraph.legend.source') }}
        </div>
        <div>
          <span class="graph-rail__line graph-rail__line--conflict" />{{ t('memory.unifiedGraph.legend.conflict') }}
        </div>
      </div>
    </div>

    <div>
      <div class="text-subtitle2 q-mb-xs">
        {{ t('memory.unifiedGraph.weightThreshold') }}
        <span class="text-caption text-grey-7">≥ {{ minWeight.toFixed(2) }}</span>
      </div>
      <q-slider
        :model-value="minWeight"
        :min="0"
        :max="1"
        :step="0.05"
        dense
        @update:model-value="(v) => emit('update:minWeight', Number(v))"
      />
    </div>

    <div>
      <div class="text-subtitle2 q-mb-xs">{{ t('memory.unifiedGraph.hopsLabel') }}</div>
      <q-btn-toggle
        :model-value="hops"
        dense
        no-caps
        unelevated
        toggle-color="primary"
        :options="[
          { label: '1', value: 1 },
          { label: '2', value: 2 },
          { label: '3', value: 3 },
        ]"
        @update:model-value="(v) => emit('update:hops', Number(v))"
      />
    </div>

    <q-btn
      outline
      rounded
      no-caps
      icon="refresh"
      :label="t('common.refresh')"
      :loading="loading"
      @click="emit('refresh')"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { UnifiedGraphNode } from '../types';
import { memoryLayerColor } from '../panorama/layerMeta';
import { GRAPH_LAYERS } from './composables/useUnifiedMemoryGraph';

const props = defineProps<{
  enabledLayers: string[];
  minWeight: number;
  hops: number;
  loading: boolean;
  searchNodes: (keyword: string) => UnifiedGraphNode[];
}>();

const emit = defineEmits<{
  (e: 'toggle-layer', layer: string): void;
  (e: 'update:minWeight', value: number): void;
  (e: 'update:hops', value: number): void;
  (e: 'locate', nodeId: string): void;
  (e: 'refresh'): void;
}>();

const { t } = useI18n();
const keyword = ref('');
const matches = ref<UnifiedGraphNode[]>([]);

function onKeywordInput() {
  matches.value = keyword.value ? props.searchNodes(keyword.value) : [];
}
</script>

<style scoped>
.graph-rail {
  width: 230px;
  flex: 0 0 auto;
}

.graph-rail__swatch {
  display: inline-block;
  height: 10px;
  width: 10px;
}

.graph-rail__matches {
  border-radius: 8px;
  max-height: 180px;
  overflow-y: auto;
}

.graph-rail__legend {
  color: var(--color-text-secondary);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.graph-rail__legend > div {
  align-items: center;
  display: flex;
  gap: 8px;
}

.graph-rail__line {
  display: inline-block;
  height: 0;
  width: 28px;
}

.graph-rail__line--solid {
  border-top: 2px solid #ff8a65;
}

.graph-rail__line--dashed {
  border-top: 2px dashed #7986cb;
}

.graph-rail__line--conflict {
  border-top: 2px dashed #ef5350;
}
</style>
