<template>
  <div
    class="ov-tile"
    :class="[`ov-tile--${tone}`, { 'ov-tile--clickable': clickable }]"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @click="clickable && $emit('drill')"
    @keydown.enter="clickable && $emit('drill')"
  >
    <div class="ov-tile__top">
      <span class="ov-tile__label">{{ label }}</span>
      <span v-if="tone !== 'neutral'" class="ov-tile__dot" aria-hidden="true" />
    </div>
    <div class="ov-tile__value-row">
      <span class="ov-tile__value">{{ value }}</span>
      <span v-if="deltaChip" class="ov-tile__delta" :class="`ov-tile__delta--${deltaChip.tone}`">
        {{ deltaChip.arrow }} {{ deltaChip.text }}
      </span>
    </div>
    <div v-if="caption" class="ov-tile__caption">{{ caption }}</div>
    <OverviewSparkline v-if="spark && spark.length > 1" class="ov-tile__spark" :points="spark" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import OverviewSparkline from './OverviewSparkline.vue';
import type { OvTone } from './ovTypes';

const props = withDefaults(
  defineProps<{
    label: string;
    value: string;
    caption?: string;
    /** 环比（小数）；null/undefined 不渲染 chip（无基线由 caption 披露）。 */
    delta?: number | null;
    /** 成本类指标「涨为坏」。 */
    deltaGoodWhenNegative?: boolean;
    tone?: OvTone;
    spark?: number[];
    clickable?: boolean;
  }>(),
  { tone: 'neutral', delta: undefined, clickable: false, caption: '', spark: () => [] },
);

defineEmits<{ drill: [] }>();

const deltaChip = computed(() => {
  const d = props.delta;
  if (d === null || d === undefined || !Number.isFinite(d)) return null;
  const up = d >= 0;
  const good = props.deltaGoodWhenNegative ? !up : up;
  return {
    arrow: up ? '▲' : '▼',
    text: `${Math.abs(d * 100).toFixed(1)}%`,
    tone: Math.abs(d) < 0.005 ? 'flat' : good ? 'good' : 'bad',
  };
});
</script>
