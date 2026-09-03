<template>
  <!-- 展示型通知（deliverables 等机器载荷）：注册表命中且解析成功 → 动态组件；
       未注册或解析失败 → 普通 markdown 通知（兜底语义不变，LBG-8）。 -->
  <component :is="display.component" v-if="display" :payload="display.payload" />
  <div v-else class="notice-block" :class="`notice-block--${noticeType}`">
    <q-icon :name="iconForType" size="14px" class="notice-block__icon" />
    <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
    <div class="notice-block__message chat-message-prose" v-html="renderedContent"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Step } from '../../features/chat/v2Types';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';
import { resolveDisplayPayload } from '../../features/chat/displayRegistry';

const props = defineProps<{ step: Step }>();

/** NoticeType 来源：后端 meta.notice_type（"info" / "warning" / "success"）。
 *  v2 Step.NoticeType 字段在 step.created 事件中已映射，优先使用。
 */
const noticeType = computed<'info' | 'warning' | 'success'>(() => {
  const t = (props.step.NoticeType ?? '').trim().toLowerCase();
  if (t === 'warning' || t === 'warn') return 'warning';
  if (t === 'success' || t === 'ok') return 'success';
  return 'info';
});

const iconForType = computed(() => {
  switch (noticeType.value) {
    case 'warning':
      return 'warning';
    case 'success':
      return 'check_circle';
    default:
      return 'info';
  }
});

/** 注册表分发：命中且载荷解析成功 → 展示组件；否则 null 回退普通通知。 */
const display = computed(() => resolveDisplayPayload(props.step.NoticeType, props.step.Content));

const renderedContent = computed(() => renderChatMarkdownForMessage(props.step.ID, props.step.Content, false));
</script>

<style lang="sass" scoped>
.notice-block
  display: flex
  align-items: flex-start
  gap: 8px
  padding: 8px 12px
  border-radius: 10px
  font-size: 13px
  line-height: 1.5
  margin: 4px 0

  &--warning
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-warning) 30%, transparent)
    color: var(--color-warning)

    .notice-block__icon
      color: var(--color-warning)

  &--success
    background: color-mix(in srgb, var(--color-success) 8%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-success) 30%, transparent)
    color: var(--color-success)

    .notice-block__icon
      color: var(--color-success)

  &--info
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    color: var(--color-text-secondary)

    .notice-block__icon
      color: var(--color-text-secondary)

  &__icon
    flex-shrink: 0
    margin-top: 1px

  &__message
    flex: 1
    min-width: 0
    overflow-wrap: break-word

    :deep(p)
      margin: 0

    :deep(code)
      background: rgba(127, 127, 127, 0.15)
      padding: 1px 4px
      border-radius: 3px
      font-size: 12px
</style>
