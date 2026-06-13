<template>
  <div class="activity-timeline" :class="`activity-timeline--${variant}`">
    <template v-for="(activity, idx) in activities" :key="activity.id">
      <!-- D2: Phase transition indicator between thinking and reply -->
      <div
        v-if="idx > 0 && activity.kind === 'say' && activities[idx - 1].kind === 'think'"
        class="activity-timeline__transition"
      >
        <span class="activity-timeline__transition-line" />
        <span class="activity-timeline__transition-label">{{ t('chat.transition.thinkingToReply', '生成回复') }}</span>
        <span class="activity-timeline__transition-line" />
      </div>
      <ThinkingBlock
        v-if="activity.kind === 'think'"
        :message-id="activity.id"
        :reasoning="activity.content"
        :streaming="activity.streaming"
        :duration-ms="activity.durationMs"
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
import { useI18n } from 'vue-i18n';
import type { Activity, ActivityVariant } from '../../features/chat/activityTimelineTypes';
import ThinkingBlock from './ThinkingBlock.vue';
import ActActivity from './ActActivity.vue';
import SayActivity from './SayActivity.vue';
import NoticeActivity from './NoticeActivity.vue';
import DelegateActivity from './DelegateActivity.vue';

const { t } = useI18n();

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

  &__transition
    display: flex
    align-items: center
    gap: 8px
    padding: 2px 0

  &__transition-line
    flex: 1
    height: 1px
    background: var(--glass-border)

  &__transition-label
    font-size: 11px
    color: var(--color-text-tertiary)
    white-space: nowrap
</style>
