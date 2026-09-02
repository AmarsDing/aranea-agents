<template>
  <!-- deliverables：全部团队完成后的交付物清单（P1 会话产物点击查看，
       2026-09-02）。Content 为 {"artifacts":[{artifact_id,name,format,
       size_chars,mime_type}]} 机器载荷，渲染为可点击产物卡片；解析失败
       由 NoticeBlock 退化回普通 markdown 通知（本组件只在解析成功后渲染）。 -->
  <div class="notice-block notice-block--deliverables">
    <q-icon name="inventory_2" size="14px" class="notice-block__icon" />
    <div class="notice-block__deliverables">
      <div class="notice-block__deliverables-title">{{ t('chat.deliverablesNotice') }}</div>
      <ArtifactRefCard
        v-for="a in payload"
        :key="a.artifactId"
        :artifact-id="a.artifactId"
        :name="a.name"
        :mime-type="a.mimeType"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { DeliverableRef } from '../../../features/chat/deliverablesNotice';
import ArtifactRefCard from '../../artifact/ArtifactRefCard.vue';

defineProps<{ payload: DeliverableRef[] }>();

const { t } = useI18n();
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
</style>
