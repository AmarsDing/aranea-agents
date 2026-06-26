<template>
  <div class="tool-detail">
    <div v-if="path" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.filePath') }}</div>
      <code class="tool-detail__inline">{{ path }}</code>
    </div>
    <div v-if="changedLines != null" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.changedLines') }}</span>
      <code class="tool-detail__inline">{{ changedLines }}</code>
    </div>
    <div v-if="diff" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.diff') }}</div>
      <pre class="tool-detail__code tool-detail__code--diff">{{ diff }}</pre>
    </div>
    <div v-if="content && !diff" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.writtenContent') }}</div>
      <pre class="tool-detail__code">{{ content }}</pre>
    </div>
    <div v-if="activity.tool.error" class="tool-detail__row tool-detail__row--error">
      <div class="tool-detail__label">{{ t('chat.toolDetail.error') }}</div>
      <pre class="tool-detail__code">{{ activity.tool.error }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ActionEvent } from '../../../features/chat/streamEventTypes';
import { tryParseJson, asRecord, asString, asNumber } from './toolDetailShared';

const { t } = useI18n();

const props = defineProps<{ activity: ActionEvent }>();

const parsedArgs = computed(() => asRecord(tryParseJson(props.activity.tool.arguments)));
const parsedResult = computed(() => asRecord(tryParseJson(props.activity.tool.result)));

const path = computed(
  () =>
    asString(parsedArgs.value?.path) ??
    asString(parsedArgs.value?.file_path) ??
    asString(parsedResult.value?.path) ??
    '',
);
const changedLines = computed(
  () =>
    asNumber(parsedResult.value?.changed_lines) ??
    asNumber(parsedResult.value?.lines_changed) ??
    asNumber(parsedResult.value?.lines_affected),
);
const diff = computed(() => asString(parsedResult.value?.diff) ?? asString(parsedResult.value?.patch) ?? '');
const content = computed(() => asString(parsedArgs.value?.content) ?? asString(parsedArgs.value?.text) ?? '');
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
    &--diff
      font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace
</style>
