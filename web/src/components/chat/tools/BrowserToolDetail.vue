<template>
  <div class="tool-detail">
    <div v-if="url" class="tool-detail__row">
      <div class="tool-detail__label">URL</div>
      <a v-if="isValidUrl" :href="url" target="_blank" rel="noopener noreferrer" class="tool-detail__link">{{ url }}</a>
      <code v-else class="tool-detail__inline">{{ url }}</code>
    </div>
    <div v-if="action" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.action') }}</span>
      <code class="tool-detail__inline">{{ action }}</code>
    </div>
    <div v-if="title" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.pageTitle') }}</span>
      <span class="tool-detail__text">{{ title }}</span>
    </div>
    <div v-if="screenshotUrl" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.screenshot') }}</div>
      <img :src="screenshotUrl" :alt="action || 'browser screenshot'" class="tool-detail__screenshot" />
    </div>
    <div v-if="domSummary" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.domOperations') }}</div>
      <pre class="tool-detail__code">{{ domSummary }}</pre>
    </div>
    <div v-if="activity.tool.arguments" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.args') }}</div>
      <pre class="tool-detail__code">{{ activity.tool.arguments }}</pre>
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

const url = computed(() => asString(parsedArgs.value?.url) ?? asString(parsedResult.value?.url) ?? '');
const action = computed(() => asString(parsedArgs.value?.action) ?? asString(parsedArgs.value?.operation) ?? '');
const title = computed(() => asString(parsedResult.value?.title) ?? '');
const screenshotUrl = computed(
  () => asString(parsedResult.value?.screenshot) ?? asString(parsedResult.value?.screenshot_url) ?? '',
);
const domSummary = computed(() => {
  const summary = asString(parsedResult.value?.dom_summary) ?? asString(parsedResult.value?.summary);
  return summary;
});

const isValidUrl = computed(() => {
  if (!url.value) return false;
  try {
    const u = new URL(url.value);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
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

  &__text
    font-size: 13px
    color: var(--color-text-primary)

  &__inline
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 4px
    padding: 1px 6px

  &__link
    font-size: 12px
    color: var(--color-accent)
    text-decoration: none
    word-break: break-all
    &:hover
      color: var(--color-accent-hover)
      text-decoration: underline

  &__screenshot
    max-width: 100%
    max-height: 320px
    border: 1px solid var(--glass-border)
    border-radius: 6px
    display: block

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
