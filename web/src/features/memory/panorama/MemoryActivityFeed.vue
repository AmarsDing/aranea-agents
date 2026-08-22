// Container: approved — 最近记忆动态事件流；数据来自 MemoryPanoramaTab via props。
<template>
  <q-card flat class="memory-card">
    <q-card-section>
      <div class="text-h6">{{ t('memory.panorama.activityTitle') }}</div>
      <div class="text-caption text-grey-7">{{ t('memory.panorama.activityCaption') }}</div>
    </q-card-section>
    <q-list v-if="items.length" separator class="activity-feed">
      <q-item v-for="(item, i) in items" :key="`${item.ts}-${i}`">
        <q-item-section avatar>
          <q-avatar
            size="28px"
            :style="{ background: layerColor(item.layer_to) }"
            text-color="white"
            :icon="kindIcon[item.kind] ?? 'bolt'"
          />
        </q-item-section>
        <q-item-section>
          <q-item-label>
            <q-chip dense square size="sm" :style="layerChipStyle(item.layer_from)" text-color="white">
              {{ item.layer_from }}
            </q-chip>
            <q-icon name="east" size="12px" class="q-mx-xs" color="grey-6" />
            <q-chip dense square size="sm" :style="layerChipStyle(item.layer_to)" text-color="white">
              {{ item.layer_to }}
            </q-chip>
            <span class="q-ml-sm text-weight-medium">{{ t(`memory.panorama.activity.${item.kind}`) }}</span>
          </q-item-label>
          <q-item-label caption lines="2">{{ item.summary }}</q-item-label>
        </q-item-section>
        <q-item-section side top>
          <span class="text-caption text-grey-6">{{ formatTs(item.ts) }}</span>
        </q-item-section>
      </q-item>
    </q-list>
    <q-card-section v-else class="text-grey-7 text-center q-pa-lg">
      {{ t('memory.panorama.activityEmpty') }}
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { MemoryActivityItem } from '../../types';
import { memoryLayerColor } from './layerMeta';

defineProps<{ items: MemoryActivityItem[] }>();

const { t } = useI18n();

const kindIcon: Record<string, string> = {
  fact_extracted: 'psychology',
  fact_injected: 'input',
  episode_recorded: 'timeline',
  entity_created: 'hub',
};

function layerColor(layer: string): string {
  return memoryLayerColor(layer);
}

function layerChipStyle(layer: string): { background: string } {
  return { background: memoryLayerColor(layer) };
}

function formatTs(value: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(
    date,
  );
}
</script>

<style scoped>
.activity-feed {
  max-height: 420px;
  overflow-y: auto;
}
</style>
