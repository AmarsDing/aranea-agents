<template>
  <div class="tool-detail">
    <div class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.toolName') }}</span>
      <code class="tool-detail__inline">{{ activity.tool.toolName }}</code>
    </div>
    <div v-if="activity.tool.arguments" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.args') }}</div>
      <pre class="tool-detail__code">{{ activity.tool.arguments }}</pre>
    </div>
    <div v-if="activity.tool.result" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.result') }}</div>
      <pre class="tool-detail__code">{{ activity.tool.result }}</pre>
    </div>
    <div v-if="activity.tool.error" class="tool-detail__row tool-detail__row--error">
      <div class="tool-detail__label">{{ t('chat.toolDetail.error') }}</div>
      <pre class="tool-detail__code">{{ activity.tool.error }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { ActionEvent } from '../../../features/chat/streamEventTypes';

const { t } = useI18n();

defineProps<{ activity: ActionEvent }>();
</script>

<style lang="sass" scoped>
.tool-detail
  &__row
    margin-bottom: 6px
    &--error
      .tool-detail__code
        border-color: var(--color-danger)

  &__label
    font-size: 11px
    color: var(--color-text-secondary)
    margin-bottom: 2px

  &__label-inline
    font-size: 12px
    color: var(--color-text-secondary)

  &__inline
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 4px
    padding: 1px 6px
    word-break: break-all

  &__code
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 6px
    padding: 6px 8px
    overflow-x: auto
    max-height: 240px
    overflow-y: auto
    margin: 0
    white-space: pre-wrap
    word-break: break-word
</style>
