<template>
  <div class="user-message-bubble">
    <div class="user-message-bubble__avatar">👤</div>
    <div class="user-message-bubble__content">
      <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
      <div class="user-message-bubble__text" v-html="renderedContent"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Message } from '../../domain/types';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';

const props = defineProps<{
  message: Message;
}>();

const renderedContent = computed(() => renderChatMarkdown(props.message.content_markdown || ''));
</script>

<style lang="sass" scoped>
.user-message-bubble
  display: flex
  gap: 10px
  align-items: flex-start
  margin-bottom: 8px

  &__avatar
    flex-shrink: 0
    width: 28px
    height: 28px
    border-radius: 50%
    display: flex
    align-items: center
    justify-content: center
    font-size: 14px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)

  &__content
    flex: 1
    min-width: 0

  &__text
    background: var(--color-accent)
    color: var(--color-on-accent, #fff)
    border-radius: 12px 12px 12px 4px
    padding: 10px 14px
    font-size: 14px
    line-height: 1.6
    word-break: break-word

    :deep(p)
      margin: 0

    :deep(p + p)
      margin-top: 8px

    :deep(a)
      color: var(--color-on-accent, #fff)
      text-decoration: underline

body.body--dark &
  .user-message-bubble__text
    background: var(--chat-accent-bg-strong)
    color: var(--color-text-primary)
    border: 1px solid var(--chat-accent-border-strong)
    border-radius: 12px 12px 12px 4px

    :deep(a)
      color: var(--color-neon-cyan)
</style>
