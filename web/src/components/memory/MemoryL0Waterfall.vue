<template>
  <div v-if="bars.length" class="l0-waterfall">
    <div class="l0-waterfall__track" role="img" :aria-label="t('memory.snapshotDrawer.waterfallAria')">
      <div
        v-for="bar in bars"
        :key="bar.section"
        class="l0-waterfall__seg"
        :style="{ width: `${Math.max(bar.percent, 0)}%`, background: sectionColor(bar.section) }"
      >
        <q-tooltip>{{ bar.section }} · {{ bar.tokens.toLocaleString() }} tok ({{ formatPercent(bar.percent) }})</q-tooltip>
      </div>
    </div>
    <div class="l0-waterfall__legend">
      <div v-for="bar in bars" :key="`leg-${bar.section}`" class="l0-waterfall__legend-item">
        <span class="l0-waterfall__swatch" :style="{ background: sectionColor(bar.section) }" />
        <span class="ellipsis">{{ bar.section }}</span>
        <span class="text-grey-6">{{ bar.tokens.toLocaleString() }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { memoryLayerColor } from '../../features/memory/panorama/layerMeta';
import type { L0WaterfallBar } from '../../features/memory/l0Waterfall';

defineProps<{ bars: L0WaterfallBar[] }>();

const { t } = useI18n();

function sectionColor(section: string): string {
  const key = section.trim().toLowerCase();
  if (key.includes('l4')) return memoryLayerColor('L4');
  if (key.includes('l3')) return memoryLayerColor('L3');
  if (key.includes('l2')) return memoryLayerColor('L2');
  if (key.includes('l1')) return memoryLayerColor('L1');
  if (key.includes('l0') || key.includes('history') || key.includes('summary') || key.includes('system')) {
    return memoryLayerColor('L0');
  }
  return '#9e9e9e';
}

function formatPercent(value: number): string {
  return `${Math.round(value)}%`;
}
</script>

<style scoped lang="sass">
.l0-waterfall
  display: flex
  flex-direction: column
  gap: var(--space-3)

.l0-waterfall__track
  display: flex
  height: 14px
  border-radius: 999px
  overflow: hidden
  background: color-mix(in srgb, var(--color-text-secondary) 10%, transparent)

.l0-waterfall__seg
  min-width: 2px
  height: 100%

.l0-waterfall__legend
  display: flex
  flex-wrap: wrap
  gap: 8px 14px

.l0-waterfall__legend-item
  display: flex
  align-items: center
  gap: 6px
  min-width: 0
  font-size: var(--text-sm)
  color: var(--color-text-secondary)

.l0-waterfall__swatch
  width: 8px
  height: 8px
  border-radius: 50%
  flex-shrink: 0
</style>
