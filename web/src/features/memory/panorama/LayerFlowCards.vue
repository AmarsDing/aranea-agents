// Container: approved — 五层卡横排 + 层间箭头 + 双向流向条；数据来自 MemoryPanoramaTab via props。
<template>
  <div class="layer-flow">
    <div class="layer-flow__bar text-caption">
      <q-icon name="south" size="14px" color="primary" />
      {{ t('memory.panorama.flowDown') }} · {{ t('memory.panorama.todayAdded') }}
      <b>{{ totalTodayAdded }}</b>
    </div>

    <div class="layer-flow__row">
      <template v-for="(stat, i) in orderedLayers" :key="stat.layer">
        <div v-if="i > 0" class="layer-flow__arrow">
          <q-icon name="east" size="18px" color="grey-5" />
        </div>
        <layer-card :stat="stat" class="layer-flow__card" @drill="(layer) => emit('drill', layer)" />
      </template>
    </div>

    <div class="layer-flow__bar text-caption">
      <q-icon name="north" size="14px" color="secondary" />
      {{ t('memory.panorama.flowUp') }} · {{ t('memory.panorama.recallHits') }}
      <b>{{ totalRecallHits }}</b>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MemoryLayerStat } from '../../types';
import { MEMORY_LAYER_ORDER } from './layerMeta';
import LayerCard from './LayerCard.vue';

const props = defineProps<{ layers: MemoryLayerStat[] }>();
const emit = defineEmits<{ (e: 'drill', layer: string): void }>();

const { t } = useI18n();

const orderedLayers = computed(() => {
  const byKey = new Map(props.layers.map((l) => [l.layer, l]));
  return MEMORY_LAYER_ORDER.map(
    (key) =>
      byKey.get(key) ?? {
        layer: key,
        item_count: 0,
        today_added: 0,
        recall_hits: 0,
        health: 'ok',
        headline_json: '{}',
      },
  );
});

const totalTodayAdded = computed(() => props.layers.reduce((sum, l) => sum + l.today_added, 0));
const totalRecallHits = computed(() => props.layers.reduce((sum, l) => sum + l.recall_hits, 0));
</script>

<style scoped>
.layer-flow {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.layer-flow__bar {
  align-items: center;
  color: var(--color-text-secondary);
  display: flex;
  gap: 6px;
  padding-left: 4px;
}

.layer-flow__bar b {
  color: var(--color-text-heading);
}

.layer-flow__row {
  align-items: stretch;
  display: flex;
  gap: 4px;
  overflow-x: auto;
  padding-bottom: 4px;
}

.layer-flow__card {
  flex: 1 1 0;
}

.layer-flow__arrow {
  align-items: center;
  display: flex;
  flex: 0 0 auto;
}
</style>
