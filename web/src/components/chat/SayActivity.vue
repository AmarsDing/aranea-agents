<template>
  <div class="say-activity" :class="`say-activity--${variant}`">
    <!-- Card variant -->
    <template v-if="variant === 'card'">
      <div class="say-activity__label">
        <span class="say-activity__icon">💬</span>
        <span class="say-activity__label-text">{{ label }}</span>
        <span v-if="activity.streaming" class="pulse-dot"></span>
      </div>
      <div class="say-activity__content">
        <div class="say-activity__markdown" v-html="renderedContent"></div>
        <span v-if="activity.streaming" class="cursor-blink"></span>
      </div>
    </template>

    <!-- Compact variant -->
    <template v-else>
      <div class="say-activity__compact">
        <span class="say-activity__compact-text">{{ previewText }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SayActivity as SayActivityType, ActivityVariant } from '../../features/chat/activityTimelineTypes';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';

const { t } = useI18n();

const props = defineProps<{
  activity: SayActivityType;
  variant?: ActivityVariant;
}>();

const label = computed(() =>
  props.activity.isFinal ? t('chat.agentBlock.finalReply') : t('chat.agentBlock.intermediateReply'),
);

const renderedContent = computed(() => renderChatMarkdown(props.activity.content));

const previewText = computed(() => {
  const text = props.activity.content.replace(/[#*`]/g, '').trim();
  return text.length > 80 ? text.slice(0, 80) + '…' : text;
});
</script>

<style lang="sass" scoped>
.say-activity
  &__label
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 0

  &__icon
    font-size: 14px

  &__label-text
    font-size: 13px
    color: var(--color-text-primary)
    font-weight: 600

  &__content
    padding: 10px 14px
    background: var(--glass-elevated)
    border: 1px solid var(--glass-border)
    border-radius: 12px
    font-size: 14px
    line-height: 1.7
    word-break: break-word

    :deep(p)
      margin: 0

    :deep(p + p)
      margin-top: 8px

    :deep(a)
      color: var(--color-accent)

    :deep(code)
      background: var(--glass-surface)
      padding: 2px 4px
      border-radius: 4px
      font-size: 13px

    :deep(pre)
      background: var(--glass-surface)
      border: 1px solid var(--glass-border)
      border-radius: 8px
      padding: 10px 12px
      overflow-x: auto

  &__compact
    padding: 2px 0
    font-size: 13px
    color: var(--color-text-primary)

  &__compact-text
    overflow: hidden
    text-overflow: ellipsis
    display: -webkit-box
    -webkit-line-clamp: 2
    -webkit-box-orient: vertical
</style>
