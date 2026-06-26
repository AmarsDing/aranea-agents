<template>
  <div class="tool-detail">
    <div v-if="language" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.language') }}</span>
      <code class="tool-detail__inline">{{ language }}</code>
    </div>
    <div class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.executionStatus') }}</span>
      <span class="tool-detail__text" :class="`tool-detail__text--${executionStatus}`">{{ executionStatusLabel }}</span>
    </div>
    <div v-if="code" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.code') }}</div>
      <pre class="tool-detail__code">{{ code }}</pre>
    </div>
    <div v-if="output" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.output') }}</div>
      <pre class="tool-detail__code">{{ output }}</pre>
    </div>
    <div v-if="stderr" class="tool-detail__row tool-detail__row--error">
      <div class="tool-detail__label">stderr</div>
      <pre class="tool-detail__code">{{ stderr }}</pre>
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
import { tryParseJson, asRecord, asString } from './toolDetailShared';

const { t } = useI18n();

const props = defineProps<{ activity: ActionEvent }>();

const parsedArgs = computed(() => asRecord(tryParseJson(props.activity.tool.arguments)));
const parsedResult = computed(() => asRecord(tryParseJson(props.activity.tool.result)));

const language = computed(
  () =>
    asString(parsedArgs.value?.language) ??
    asString(parsedArgs.value?.lang) ??
    asString(parsedArgs.value?.interpreter) ??
    '',
);
const code = computed(() => asString(parsedArgs.value?.code) ?? asString(parsedArgs.value?.source) ?? '');
const output = computed(() => asString(parsedResult.value?.stdout) ?? asString(parsedResult.value?.output) ?? '');
const stderr = computed(() => asString(parsedResult.value?.stderr) ?? '');

const executionStatus = computed(() => props.activity.tool.status);
const executionStatusLabel = computed(() => {
  switch (executionStatus.value) {
    case 'running':
      return t('chat.toolDetail.statusRunning');
    case 'success':
      return t('chat.toolDetail.statusSuccess');
    case 'failed':
      return t('chat.toolDetail.statusFailed');
    case 'blocked':
      return t('chat.toolDetail.statusBlocked');
    case 'cancelled':
      return t('chat.toolDetail.statusCancelled');
    default:
      return executionStatus.value;
  }
});
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

  &__text
    font-size: 12px
    &--success
      color: var(--color-success)
    &--failed
      color: var(--color-danger)
    &--running
      color: var(--color-accent)
    &--blocked
      color: var(--color-warning)
    &--cancelled
      color: var(--color-text-secondary)

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
