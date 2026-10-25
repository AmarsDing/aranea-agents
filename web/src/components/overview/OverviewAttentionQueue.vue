<template>
  <section class="ov-attention" aria-live="polite">
    <header class="ov-section-head">
      <q-icon name="notification_important" size="18px" class="ov-section-head__icon" />
      <h2 class="ov-section-head__title">{{ t('overviewPage.v2.attentionTitle') }}</h2>
    </header>
    <div class="ov-attention__grid">
      <div v-for="lane in lanes" :key="lane.lane" class="ov-lane" :class="laneTone(lane)">
        <div class="ov-lane__head">
          <q-icon :name="LANE_META[lane.lane].icon" size="15px" />
          <span class="ov-lane__name">{{ t(LANE_META[lane.lane].labelKey) }}</span>
          <span class="ov-lane__count">{{ lane.count }}</span>
        </div>
        <div v-if="!lane.items.length" class="ov-lane__empty">
          <q-icon name="check_circle_outline" size="14px" />
          {{ t('overviewPage.v2.laneEmpty') }}
        </div>
        <ul v-else class="ov-lane__items">
          <li v-for="item in lane.items" :key="item.id">
            <div
              class="ov-item"
              :class="[`ov-item--${item.severity}`, { 'ov-item--link': !!item.to }]"
              :role="item.to ? 'button' : undefined"
              :tabindex="item.to ? 0 : undefined"
              @click="item.to && $emit('drill', item)"
              @keydown.enter="item.to && $emit('drill', item)"
            >
              <div class="ov-item__row">
                <span class="ov-item__sev" aria-hidden="true" />
                <span class="ov-item__title">{{ item.title }}</span>
                <span class="ov-item__time">{{ fmtClock(item.occurred_at) }}</span>
              </div>
              <div class="ov-item__detail">{{ item.detail }}</div>
              <q-icon v-if="item.to" name="chevron_right" size="14px" class="ov-item__go" />
            </div>
          </li>
        </ul>
        <div v-if="lane.count > lane.items.length" class="ov-lane__more">
          {{ t('overviewPage.v2.laneTotal', { count: lane.count }) }}
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { AttentionItem, AttentionLane, AttentionLaneState } from '../../features/usage/types';
import { fmtClock } from './ovTime';

const LANE_META: Record<AttentionLane, { icon: string; labelKey: string }> = {
  alerts: { icon: 'notifications_active', labelKey: 'overviewPage.v2.laneAlerts' },
  failed_runs: { icon: 'error_outline', labelKey: 'overviewPage.v2.laneFailedRuns' },
  call_errors: { icon: 'bolt', labelKey: 'overviewPage.v2.laneCallErrors' },
  backlog: { icon: 'inventory_2', labelKey: 'overviewPage.v2.laneBacklog' },
};

defineProps<{ lanes: AttentionLaneState[] }>();
defineEmits<{ drill: [item: AttentionItem] }>();

const { t } = useI18n();

function laneTone(lane: AttentionLaneState): string {
  if (lane.items.some((i) => i.severity === 'critical')) return 'ov-lane--critical';
  if (lane.count > 0) return 'ov-lane--warning';
  return 'ov-lane--ok';
}
</script>
