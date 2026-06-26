<template>
  <div class="tool-detail">
    <div v-if="command" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.command') }}</div>
      <pre class="tool-detail__code">{{ command }}</pre>
    </div>
    <div v-if="cwd" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.workingDirectory') }}</div>
      <code class="tool-detail__inline">{{ cwd }}</code>
    </div>
    <div v-if="stdout" class="tool-detail__row">
      <div class="tool-detail__label">stdout</div>
      <pre class="tool-detail__code">{{ stdout }}</pre>
    </div>
    <div v-if="stderr" class="tool-detail__row tool-detail__row--error">
      <div class="tool-detail__label">stderr</div>
      <pre class="tool-detail__code">{{ stderr }}</pre>
    </div>
    <div v-if="exitCode != null" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.exitCode') }}</span>
      <code class="tool-detail__inline" :class="{ 'tool-detail__inline--error': exitCode !== 0 }">{{ exitCode }}</code>
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

const command = computed(() => asString(parsedArgs.value?.command) ?? asString(parsedArgs.value?.cmd) ?? '');
const cwd = computed(() => asString(parsedArgs.value?.cwd) ?? asString(parsedArgs.value?.workdir) ?? '');
const stdout = computed(() => asString(parsedResult.value?.stdout) ?? '');
const stderr = computed(() => asString(parsedResult.value?.stderr) ?? '');
const exitCode = computed(() => asNumber(parsedResult.value?.exit_code) ?? asNumber(parsedResult.value?.code));
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
    &--error
      color: var(--color-danger)

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
