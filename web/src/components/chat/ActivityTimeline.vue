<template>
  <div class="activity-timeline" :class="`activity-timeline--${variant}`">
    <template v-for="activity in activities" :key="activity.id">
      <ThinkActivity
        v-if="activity.kind === 'think'"
        :activity="activity"
        :variant="variant"
      />
      <ActActivity
        v-else-if="activity.kind === 'act'"
        :activity="activity"
        :variant="variant"
        :agent-color="agentColor"
      />
      <SayActivity
        v-else-if="activity.kind === 'say'"
        :activity="activity"
        :variant="variant"
      />
      <NoticeActivity
        v-else-if="activity.kind === 'notice'"
        :activity="activity"
      />
      <DelegateActivity
        v-else-if="activity.kind === 'delegate'"
        :activity="activity"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import type { Activity, ActivityVariant } from '../../features/chat/activityTimelineTypes';
import ThinkActivity from './ThinkActivity.vue';
import ActActivity from './ActActivity.vue';
import SayActivity from './SayActivity.vue';
import NoticeActivity from './NoticeActivity.vue';
import DelegateActivity from './DelegateActivity.vue';

defineProps<{
  activities: Activity[];
  agentColor?: string;
  variant?: ActivityVariant;
}>();
</script>

<style lang="sass" scoped>
.activity-timeline
  display: flex
  flex-direction: column
  gap: 4px

  &--compact
    gap: 2px
</style>
