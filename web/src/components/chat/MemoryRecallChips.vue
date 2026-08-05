<!-- web/src/components/chat/MemoryRecallChips.vue
  R4 召回透明度：在 turn 底部展示本轮注入模型的记忆条目（L1-L4）。
  数据源：activityV2Store.recallHitsByTurn（memory_recalled notice step 解析索引）。
  原始 notice step 已被 noticeFilter 隐藏，本组件是其唯一的用户可见渲染。
-->
<template>
  <div v-if="hits.length > 0" class="memory-recall-chips">
    <div class="memory-recall-chips__header">
      <q-icon name="psychology" size="14px" class="memory-recall-chips__icon" />
      <span class="memory-recall-chips__title">
        {{ t('chat.memoryRecall.title', { n: hits.length }) }}
      </span>
    </div>
    <div class="memory-recall-chips__list">
      <div v-for="(hit, i) in hits" :key="i" class="memory-recall-chips__chip">
        <span class="memory-recall-chips__layer" :class="`memory-recall-chips__layer--${layerKey(hit.layer)}`">
          {{ hit.layer }}
        </span>
        <span class="memory-recall-chips__line">{{ hit.line }}</span>
        <span v-if="hit.score > 0" class="memory-recall-chips__score">{{ scoreLabel(hit.score) }}</span>
        <q-tooltip :delay="300" max-width="360px">
          <div class="memory-recall-chips__tip-layer">{{ layerName(hit.layer) }} · {{ scoreLabel(hit.score) }}</div>
          <div>{{ hit.line }}</div>
          <div v-if="hit.confidence || hit.version" class="memory-recall-chips__tip-meta">
            <span v-if="hit.confidence">{{ t('chat.memoryRecall.confidence') }} {{ scoreLabel(hit.confidence) }}</span>
            <span v-if="hit.version">v{{ hit.version }}</span>
          </div>
        </q-tooltip>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../features/chat/composables/useActivityQueries';

// Safe i18n wrapper — falls back to key when the i18n plugin isn't installed
// (e.g., during unit tests without app.use(i18n)). Project pattern: see
// ThinkingBlock.vue / TaskCard.vue useSafeI18n.
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string, fallback?: unknown) => (typeof fallback === 'string' ? fallback : key) };
  }
}

const props = defineProps<{ turnId: string }>();
const { t } = useSafeI18n();
const store = useActivityQueries();

const hits = computed(() => store.getTurnRecallHits(props.turnId));

function layerKey(layer: string): string {
  const k = (layer || '').trim().toLowerCase();
  if (k === 'l1' || k === 'l2' || k === 'l3' || k === 'l4') return k;
  return 'lx';
}

function layerName(layer: string): string {
  switch (layerKey(layer)) {
    case 'l1':
      return `L1 · ${t('chat.memoryRecall.layerL1')}`;
    case 'l2':
      return `L2 · ${t('chat.memoryRecall.layerL2')}`;
    case 'l3':
      return `L3 · ${t('chat.memoryRecall.layerL3')}`;
    case 'l4':
      return `L4 · ${t('chat.memoryRecall.layerL4')}`;
    default:
      return layer || 'memory';
  }
}

function scoreLabel(score: number): string {
  return `${Math.round(score * 100)}%`;
}
</script>

<style lang="sass" scoped>
.memory-recall-chips
  margin: 2px 0 6px
  padding: 6px 10px
  border-radius: 10px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 4px

  &__icon
    color: var(--color-accent)

  &__title
    font-size: 12px
    color: var(--color-text-secondary)

  &__list
    display: flex
    flex-wrap: wrap
    gap: 6px

  &__chip
    display: inline-flex
    align-items: center
    gap: 6px
    max-width: 100%
    padding: 2px 8px
    border-radius: 999px
    font-size: 12px
    line-height: 1.6
    background: color-mix(in srgb, var(--color-accent) 6%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-accent) 18%, transparent)
    color: var(--color-text-secondary)
    cursor: default

  &__layer
    flex-shrink: 0
    font-size: 10px
    font-weight: 700
    letter-spacing: 0.4px
    padding: 0 4px
    border-radius: 4px

    &--l1
      color: var(--color-info, #5aa7ff)
      background: color-mix(in srgb, var(--color-info, #5aa7ff) 14%, transparent)

    &--l2
      color: var(--color-accent)
      background: color-mix(in srgb, var(--color-accent) 16%, transparent)

    &--l3
      color: var(--color-success)
      background: color-mix(in srgb, var(--color-success) 14%, transparent)

    &--l4
      color: var(--color-warning)
      background: color-mix(in srgb, var(--color-warning) 14%, transparent)

    &--lx
      color: var(--color-text-secondary)
      background: var(--glass-surface)

  &__line
    min-width: 0
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    max-width: 320px

  &__score
    flex-shrink: 0
    font-size: 11px
    opacity: 0.75

  &__tip-layer
    font-weight: 600
    margin-bottom: 2px

  &__tip-meta
    margin-top: 4px
    display: flex
    gap: 10px
    opacity: 0.8
</style>
