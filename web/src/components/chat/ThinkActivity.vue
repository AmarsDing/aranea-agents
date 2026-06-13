<template>
  <div class="think-activity" :class="[`think-activity--${variant}`, { 'think-activity--streaming': activity.streaming }]">
    <!-- Card variant -->
    <template v-if="variant === 'card'">
      <div class="think-activity__label" @click="toggleCollapse">
        <span class="think-activity__icon">🧠</span>
        <span class="think-activity__label-text">{{ label }}</span>
        <span v-if="activity.streaming" class="pulse-dot"></span>
        <span v-if="activity.durationMs != null" class="think-activity__duration">{{ formattedDuration }}</span>
        <span class="think-activity__toggle">{{ localCollapsed ? '▶' : '▼' }}</span>
      </div>
      <div class="think-activity__body" :class="{ 'think-activity__body--collapsed': localCollapsed }">
        <div class="think-activity__body-inner" :class="{ 'think-activity__body-inner--streaming': activity.streaming }">
          <div v-html="renderedContent"></div>
          <span v-if="activity.streaming" class="cursor-blink"></span>
        </div>
      </div>
    </template>

    <!-- Compact variant -->
    <template v-else>
      <div class="think-activity__compact">
        <span class="think-activity__compact-icon">🧠</span>
        <span class="think-activity__compact-text">{{ previewText }}</span>
        <span v-if="activity.streaming" class="pulse-dot"></span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ThinkActivity as ThinkActivityType, ActivityVariant } from '../../features/chat/activityTimelineTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';

const { t } = useI18n();

const props = defineProps<{
  activity: ThinkActivityType;
  variant?: ActivityVariant;
}>();

const localCollapsed = ref(props.activity.collapsed);

watch(
  () => props.activity.streaming,
  (isStreaming, wasStreaming) => {
    if (wasStreaming && !isStreaming && props.activity.collapsed) {
      setTimeout(() => { localCollapsed.value = true; }, 500);
    }
  },
);

function toggleCollapse() {
  localCollapsed.value = !localCollapsed.value;
}

const label = computed(() => {
  if (props.activity.label) return props.activity.label;
  return t('chat.thinking.summary');
});

const formattedDuration = computed(() => props.activity.durationMs != null ? formatDuration(props.activity.durationMs) : '');

const renderedContent = computed(() => renderChatMarkdown(props.activity.content));

const previewText = computed(() => {
  const text = props.activity.content.replace(/[#*`]/g, '').trim();
  return text.length > 60 ? text.slice(0, 60) + '…' : text;
});
</script>

<style lang="sass" scoped>
.think-activity
  &--card
    margin-bottom: 4px

  &__label
    display: flex
    align-items: center
    gap: 6px
    cursor: pointer
    padding: 4px 0
    user-select: none

  &__icon
    font-size: 14px

  &__label-text
    font-size: 13px
    color: var(--color-text-secondary)
    font-weight: 500

  &__duration
    font-size: 11px
    color: var(--color-text-secondary)

  &__toggle
    font-size: 10px
    color: var(--color-text-secondary)

  &__body
    overflow: hidden
    transition: max-height 0.3s ease
    max-height: 500px

    &--collapsed
      max-height: 0

  &__body-inner
    padding: 8px 12px
    margin-left: 20px
    border-left: 2px solid var(--glass-border)
    font-size: 13px
    color: var(--color-text-secondary)
    line-height: 1.6

    &--streaming
      color: var(--color-text-primary)

  &__compact
    display: flex
    align-items: center
    gap: 6px
    padding: 2px 0
    font-size: 12px
    color: var(--color-text-secondary)
    font-style: italic

  &__compact-icon
    font-size: 12px

  &__compact-text
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
</style>
