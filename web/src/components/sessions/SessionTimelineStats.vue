<template>
  <div class="row q-col-gutter-sm session-timeline__stats">
    <div v-for="stat in stats" :key="stat.key" class="col-6 col-sm-3">
      <div class="session-timeline-stat" :class="`session-timeline-stat--${stat.key}`">
        <div class="session-timeline-stat__icon">
          <q-icon :name="stat.icon" size="18px" />
        </div>
        <div class="session-timeline-stat__body">
          <div class="session-timeline-stat__value">{{ stat.value }}</div>
          <div class="session-timeline-stat__label">{{ stat.label }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { TimelineStat } from './sessionTimelineUi';

defineProps<{
  stats: TimelineStat[];
}>();
</script>
