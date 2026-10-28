<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { computed } from 'vue';
const props = withDefaults(defineProps<{ launch?: boolean }>(), { launch: false });
const { t } = useI18n();
const overviewPaths = [
  { id: 'ingest', tone: 'data', d: 'M302 768V721', x: 302, y: 746 },
  { id: 'analysis', tone: 'ai', d: 'M302 460V411', x: 302, y: 438 },
  { id: 'suggestion', tone: 'ai', d: 'M560 282H610Q642 282 642 314V512Q642 544 674 544H726', x: 642, y: 412 },
  { id: 'up', tone: 'ceo', d: 'M995 502V411', x: 1068, y: 456 },
  { id: 'down', tone: 'owner', d: 'M949 411V502', x: 887, y: 456 },
  { id: 'dispatch', tone: 'ceo', d: 'M1218 544H1268Q1298 544 1298 514V314Q1298 282 1330 282H1383', x: 1298, y: 412 },
  { id: 'execute', tone: 'ceo', d: 'M1642 411V460', x: 1642, y: 437 },
  { id: 'verify', tone: 'ceo', d: 'M1642 721V768', x: 1642, y: 745 },
  { id: 'feedback', tone: 'data', d: 'M1383 960H560', x: 960, y: 960 },
];
const launchPaths: Record<string, string> = {
  ingest: 'M305 768V718',
  analysis: 'M305 461V412',
  suggestion: 'M559 284H610Q642 284 642 316V513Q642 545 674 545H725',
  up: 'M995 503V412',
  down: 'M949 412V503',
  dispatch: 'M1212 545H1268Q1298 545 1298 515V315Q1298 283 1330 283H1377',
  execute: 'M1631 412V463',
  verify: 'M1631 717V768',
  feedback: 'M1377 960H560',
};
const paths = computed(() =>
  overviewPaths.map((edge) => (props.launch ? { ...edge, d: launchPaths[edge.id] ?? edge.d } : edge)),
);
function labelKey(id: string, launch: boolean) {
  return `ecommerceLoop.${launch && !['up', 'down'].includes(id) ? 'launch.edges' : 'edges'}.${id}`;
}
</script>

<template>
  <svg class="ec-flow-lines" viewBox="0 0 1920 1080" aria-hidden="true">
    <defs>
      <marker
        v-for="tone in ['data', 'ai', 'ceo', 'owner']"
        :id="`ec-arrow-${tone}`"
        :key="tone"
        viewBox="0 0 10 10"
        refX="9"
        refY="5"
        markerWidth="8"
        markerHeight="8"
        orient="auto"
      >
        <path d="M0 0L10 5L0 10Z" :class="`ec-arrow-fill ec-tone-${tone}`" />
      </marker>
    </defs>
    <g v-for="edge in paths" :key="edge.id" :class="`ec-tone-${edge.tone}`">
      <path class="ec-flow-track" :d="edge.d" />
      <path class="ec-flow-path" :d="edge.d" :marker-end="`url(#ec-arrow-${edge.tone})`" />
    </g>
  </svg>
  <span
    v-for="edge in paths"
    :key="edge.id"
    class="ec-edge-label"
    :class="`ec-tone-${edge.tone}`"
    :style="{ left: `${edge.x}px`, top: `${edge.y}px` }"
    >{{ t(labelKey(edge.id, launch)) }}</span
  >
</template>
