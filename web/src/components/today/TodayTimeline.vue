<template>
  <section class="today-timeline">
    <header class="today-panel__head">
      <h2 class="today-panel__title">{{ t('today.timeline.title') }}</h2>
      <router-link v-if="status !== 'denied'" class="today-link" to="/schedule">
        {{ t('today.timeline.openSchedule') }}
      </router-link>
    </header>

    <p v-if="status === 'error'" class="today-note today-note--danger" role="alert">
      {{ t('today.timeline.failed') }}
    </p>
    <p v-else-if="status === 'denied'" class="today-note">{{ t('today.timeline.denied') }}</p>

    <p v-if="entries.length === 0" class="today-empty">{{ t('today.timeline.empty') }}</p>
    <ol v-else class="today-timeline__list">
      <template v-for="(entry, index) in entries" :key="entry.id">
        <li v-if="index === nowIndex" class="today-timeline__now" aria-hidden="true">
          <span class="today-mono">{{ nowClock }}</span>
          <span>{{ t('today.timeline.now') }}</span>
        </li>
        <li class="today-timeline__item" :class="{ 'today-timeline__item--past': entry.past }">
          <time class="today-mono today-timeline__time" :datetime="entry.at">{{ clock(entry.at) }}</time>
          <component
            :is="entry.to ? 'router-link' : 'button'"
            class="today-timeline__body"
            :class="`today-timeline__body--${entry.kind}`"
            v-bind="entry.to ? { to: entry.to } : { type: 'button' }"
            @click="entry.inboxId && emit('pick', entry.inboxId)"
          >
            <span class="today-timeline__kind">{{ t(`today.timeline.kind.${entry.kind}`) }}</span>
            <span class="today-timeline__title">{{ entry.title }}</span>
            <span v-if="entry.endAt" class="today-mono today-timeline__end">– {{ clock(entry.endAt) }}</span>
          </component>
        </li>
      </template>
      <li v-if="nowIndex === entries.length" class="today-timeline__now" aria-hidden="true">
        <span class="today-mono">{{ nowClock }}</span>
        <span>{{ t('today.timeline.now') }}</span>
      </li>
    </ol>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { clock, type TimelineEntry } from '../../features/today/logic';
import type { BlockStatus } from '../../features/today/useTodaySide';

const props = defineProps<{
  entries: TimelineEntry[];
  status: BlockStatus;
  /** 当前时间（ISO），由页面按分钟推进 */
  now: string;
}>();

const emit = defineEmits<{ pick: [inboxId: string] }>();

const { t } = useI18n();

const nowClock = computed(() => clock(props.now));
/** 「现在」分隔线插在第一条晚于当前时间的条目之前；全部已过则放在末尾。 */
const nowIndex = computed(() => {
  const nowMs = new Date(props.now).getTime();
  const index = props.entries.findIndex((entry) => new Date(entry.at).getTime() > nowMs);
  return index < 0 ? props.entries.length : index;
});
</script>
