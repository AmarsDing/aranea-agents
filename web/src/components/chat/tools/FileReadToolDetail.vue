<template>
  <div class="tool-detail">
    <div v-if="path" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.filePath') }}</div>
      <code class="tool-detail__inline">{{ path }}</code>
    </div>
    <div v-if="startLine != null || endLine != null" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.lineRange') }}</span>
      <code class="tool-detail__inline">{{ startLine ?? '?' }}–{{ endLine ?? '?' }}</code>
    </div>
    <div v-if="content" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.contentSnippet') }}</div>
      <pre class="tool-detail__code">{{ content }}</pre>
    </div>
    <div v-if="encoding" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.encoding') }}</span>
      <code class="tool-detail__inline">{{ encoding }}</code>
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
const startLine = computed(() => asNumber(parsedArgs.value?.start_line) ?? asNumber(parsedArgs.value?.offset));
const endLine = computed(() => asNumber(parsedArgs.value?.end_line));
const content = computed(() => asString(parsedResult.value?.content) ?? asString(parsedResult.value?.text) ?? '');
const encoding = computed(() => asString(parsedArgs.value?.encoding) ?? asString(parsedResult.value?.encoding) ?? '');
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
