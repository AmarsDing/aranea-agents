<template>
  <!-- deliverables：全部团队完成后的交付物清单（P1 会话产物点击查看，
       2026-09-02）。Content 为 {"artifacts":[{artifact_id,name,format,
       size_chars,mime_type}]} 机器载荷，渲染为可点击产物卡片；解析失败
       退化回普通 markdown 通知。 -->
  <div v-if="deliverableRefs" class="notice-block notice-block--deliverables">
    <q-icon name="inventory_2" size="14px" class="notice-block__icon" />
    <div class="notice-block__deliverables">
      <div class="notice-block__deliverables-title">{{ t('chat.deliverablesNotice') }}</div>
      <ArtifactRefCard
        v-for="a in deliverableRefs"
        :key="a.artifactId"
        :artifact-id="a.artifactId"
        :name="a.name"
        :mime-type="a.mimeType"
      />
    </div>
  </div>
  <div v-else class="notice-block" :class="`notice-block--${noticeType}`">
    <q-icon :name="iconForType" size="14px" class="notice-block__icon" />
    <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
    <div class="notice-block__message chat-message-prose" v-html="renderedContent"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../features/chat/v2Types';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';
import { parseDeliverableRefs } from '../../features/chat/deliverablesNotice';
import ArtifactRefCard from '../artifact/ArtifactRefCard.vue';

const props = defineProps<{ step: Step }>();

const { t } = useI18n();

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

/** deliverables 通知解析为产物卡片列表；非该类型或解析失败为 null（走普通通知）。 */
const deliverableRefs = computed(() => parseDeliverableRefs(props.step.NoticeType, props.step.Content));

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

  &--deliverables
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    color: var(--color-text-secondary)

    .notice-block__icon
      color: var(--color-accent)
      margin-top: 2px

  &__deliverables
    flex: 1
    min-width: 0

  &__deliverables-title
    font-size: 12px
    font-weight: 600
    color: var(--color-text-secondary)
    margin-bottom: 2px

    // ArtifactRefCard 默认带 20px 左缩进（工具结果语境），通知内对齐标题。
    :deep(.artifact-ref-card)
      margin-left: 0

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
