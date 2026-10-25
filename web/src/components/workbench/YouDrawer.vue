<template>
  <aside class="wb-you">
    <div class="wb-you__head">
      <span class="wb-you__title">{{ t('workbench.you.title') }}</span>
      <span v-if="total > 0" class="wb-await-badge">{{ total }}</span>
    </div>
    <div class="wb-you__scroll">
      <section v-for="lane in lanes" :key="lane.lane" class="wb-you__lane">
        <header class="wb-you__lane-head">
          <span class="wb-you__lane-name">{{ t(`workbench.lanes.${lane.lane}`) }}</span>
          <span v-if="lane.count > 0" class="wb-await-badge wb-await-badge--sm">{{ lane.count }}</span>
          <span v-else class="wb-you__lane-clear">·</span>
          <span v-if="lane.degraded" class="wb-you__lane-degraded" :title="t('workbench.you.degradedTip')">!</span>
          <span class="wb-you__lane-fresh">{{ freshness(lane) }}</span>
        </header>
        <div v-if="lane.items.length === 0" class="wb-you__empty">{{ t('workbench.you.laneEmpty') }}</div>
        <div
          v-for="item in lane.items"
          :key="item.id"
          class="wb-you__item wb-await-frame"
          :class="{ 'wb-you__item--selected': selectedId === item.id }"
          @click="$emit('focus-item', item)"
        >
          <div class="wb-you__meta">
            <span class="wb-you__who">{{ item.who }}</span>
            <span class="wb-you__when">{{ shortTime(item.when) }}</span>
          </div>
          <div class="wb-you__summary" :title="item.summary">{{ item.summary }}</div>
          <div class="wb-you__actions">
            <template v-if="item.action === 'approve'">
              <button type="button" class="wb-btn wb-btn--ok" @click.stop="$emit('sign', item, 'approved')">
                {{ t('workbench.you.approve') }}
              </button>
              <button type="button" class="wb-btn wb-btn--ghost" @click.stop="$emit('sign', item, 'rejected')">
                {{ t('workbench.you.reject') }}
              </button>
            </template>
            <button
              v-else-if="item.action === 'ack'"
              type="button"
              class="wb-btn wb-btn--ok"
              @click.stop="$emit('sign', item, 'acked')"
            >
              {{ t('workbench.you.ack') }}
            </button>
            <button v-else type="button" class="wb-btn wb-btn--ghost" @click.stop="$emit('goto-item', item)">
              {{ t('workbench.you.goto') }}
            </button>
          </div>
        </div>
      </section>
    </div>
    <footer class="wb-you__foot">{{ t('workbench.you.footer') }}</footer>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { YouItem, YouLaneState } from '../../features/workbench/types';

defineProps<{
  lanes: YouLaneState[];
  total: number;
  selectedId: string;
}>();

defineEmits<{
  sign: [item: YouItem, verb: 'approved' | 'rejected' | 'acked'];
  'goto-item': [item: YouItem];
  'focus-item': [item: YouItem];
}>();

const { t } = useI18n();

function shortTime(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

function freshness(lane: YouLaneState): string {
  if (!lane.updatedAt) return '—';
  const ms = Date.now() - Date.parse(lane.updatedAt);
  if (Number.isNaN(ms) || ms < 0) return '0s';
  const s = Math.floor(ms / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m`;
}
</script>

<style scoped lang="sass">
.wb-you
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0
  border-left: 1px solid var(--wb-line-hairline)
  background: color-mix(in srgb, var(--wb-await) 4%, var(--canvas-base))
.wb-you__head
  flex: none
  display: flex
  align-items: center
  gap: 8px
  padding: 10px 12px 8px
  border-bottom: 1px solid var(--wb-line-hairline)
.wb-you__title
  font-size: 12px
  font-weight: 700
  letter-spacing: 0.1em
  text-transform: uppercase
.wb-you__scroll
  flex: 1
  min-height: 0
  overflow-y: auto
  padding: 8px
.wb-you__lane
  margin-bottom: 10px
.wb-you__lane-head
  display: flex
  align-items: center
  gap: 6px
  padding: 2px 2px 4px
  font-size: var(--wb-micro-label)
  letter-spacing: 0.12em
  text-transform: uppercase
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__lane-name
  flex: 1
.wb-await-badge--sm
  min-width: 16px
  height: 15px
  font-size: 10px
.wb-you__lane-clear
  color: var(--wb-ok)
.wb-you__lane-degraded
  color: var(--wb-skip)
  font-weight: 700
.wb-you__lane-fresh
  font-variant-numeric: tabular-nums
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__empty
  padding: 6px 4px
  font-size: var(--wb-caption)
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__item
  position: relative
  padding: 8px
  margin-bottom: 6px
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  cursor: pointer
  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 6%, transparent)
.wb-you__item--selected
  outline: 1px solid var(--wb-await)
.wb-you__meta
  display: flex
  justify-content: space-between
  font-size: 10px
  color: var(--color-text-secondary)
.wb-you__who
  font-weight: 600
.wb-you__when
  font-variant-numeric: tabular-nums
.wb-you__summary
  margin: 3px 0 6px
  font-size: var(--wb-caption)
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
.wb-you__actions
  display: flex
  gap: 6px
.wb-btn
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: transparent
  color: var(--color-text-primary)
  font-size: var(--wb-caption)
  padding: 3px 10px
  cursor: pointer
  &:hover
    border-color: color-mix(in srgb, var(--color-text-primary) 25%, transparent)
.wb-btn--ok
  border-color: color-mix(in srgb, var(--wb-ok) 55%, transparent)
  color: var(--wb-ok)
  &:hover
    background: color-mix(in srgb, var(--wb-ok) 14%, transparent)
.wb-btn--ghost
  color: var(--color-text-secondary)
.wb-you__foot
  flex: none
  padding: 8px 12px
  border-top: 1px solid var(--wb-line-hairline)
  font-size: 10px
  color: var(--color-text-tertiary, var(--color-text-secondary))
  letter-spacing: 0.04em
</style>
