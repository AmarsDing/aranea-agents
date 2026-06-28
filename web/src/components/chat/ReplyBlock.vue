<template>
  <div class="reply-block" :class="{ 'reply-block--streaming': activity.streaming }">
    <!-- Card variant -->
    <div class="reply-block__label">
      <span class="reply-block__icon">💬</span>
      <span class="reply-block__label-text">{{ label }}</span>
      <span v-if="activity.streaming" class="pulse-dot"></span>
    </div>
    <div class="reply-block__content">
      <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
      <div class="reply-block__markdown chat-message-prose" v-html="renderedContent"></div>
      <span v-if="activity.streaming" class="cursor-blink"></span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ReplyEvent } from '../../features/chat/streamEventTypes';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';

const { t } = useI18n();

const props = defineProps<{
  activity: ReplyEvent;
}>();

const label = computed(() =>
  props.activity.isFinal ? t('chat.agentBlock.finalReply') : t('chat.agentBlock.intermediateReply'),
);

const renderedContent = computed(() =>
  renderChatMarkdownForMessage(props.activity.id, props.activity.content, props.activity.streaming),
);
</script>

<style lang="sass" scoped>
.reply-block
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

// 夜间助手气泡切换为标准玻璃 token（§6.14 要求夜 --glass-surface）
body.body--dark .reply-block__content
  background: var(--glass-surface)

// Chat UI fix: streaming state — pulsing left border to signal "agent is
// still typing". Pairs with the existing pulse-dot + cursor-blink inside.
.reply-block--streaming
  .reply-block__content
    border-left: 2px solid var(--color-accent)
    box-shadow: -1px 0 0 var(--color-accent)
    animation: reply-streaming-pulse 1.6s ease-in-out infinite

@keyframes reply-streaming-pulse
  0%, 100%
    border-left-color: var(--color-accent)
  50%
    border-left-color: color-mix(in srgb, var(--color-accent) 40%, transparent)
</style>
