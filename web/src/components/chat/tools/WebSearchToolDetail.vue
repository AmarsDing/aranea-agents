<template>
  <div class="tool-detail">
    <div v-if="query" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.query') }}</div>
      <code class="tool-detail__inline">{{ query }}</code>
    </div>
    <div v-if="resultCount != null" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.resultCount') }}</span>
      <code class="tool-detail__inline">{{ resultCount }}</code>
    </div>
    <div v-if="results.length" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.resultSummary') }}</div>
      <ul class="tool-detail__list">
        <li v-for="(item, idx) in results" :key="idx" class="tool-detail__search-item">
          <a v-if="item.url" :href="item.url" target="_blank" rel="noopener noreferrer" class="tool-detail__link">{{
            item.title || item.url
          }}</a>
          <span v-else class="tool-detail__text">{{ item.title }}</span>
          <p v-if="item.snippet" class="tool-detail__snippet">{{ item.snippet }}</p>
        </li>
      </ul>
    </div>
    <div v-if="step.ToolArgs != null && !query" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.args') }}</div>
      <pre class="tool-detail__code">{{ formatToolData(step.ToolArgs) }}</pre>
    </div>
    <div v-if="step.ToolErrorCode" class="tool-detail__row tool-detail__row--error">
      <div class="tool-detail__label">{{ t('chat.toolDetail.error') }}</div>
      <pre class="tool-detail__code">{{ step.ToolErrorCode }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../../features/chat/v2Types';
import { formatToolData, asRecord, asArray, asString, asNumber } from './toolDetailShared';

const { t } = useI18n();

interface SearchResultEntry {
  title: string;
  url: string;
  snippet: string;
}

const props = defineProps<{ step: Step }>();

const parsedArgs = computed(() => asRecord(props.step.ToolArgs));
const parsedResult = computed(() => asRecord(props.step.ToolResult));

const query = computed(
  () =>
    asString(parsedArgs.value?.query) ?? asString(parsedArgs.value?.q) ?? asString(parsedArgs.value?.search_term) ?? '',
);
const resultCount = computed(
  () =>
    asNumber(parsedResult.value?.count) ??
    asNumber(parsedResult.value?.total) ??
    asNumber(parsedResult.value?.result_count),
);
const results = computed<SearchResultEntry[]>(() => {
  const arr = asArray(parsedResult.value?.results ?? parsedResult.value?.items);
  if (!arr) return [];
  return arr
    .map((item): SearchResultEntry | undefined => {
      const rec = asRecord(item);
      if (!rec) return undefined;
      const title = asString(rec.title) ?? asString(rec.name) ?? '';
      const url = asString(rec.url) ?? asString(rec.link) ?? asString(rec.href) ?? '';
      const snippet = asString(rec.snippet) ?? asString(rec.description) ?? asString(rec.summary) ?? '';
      if (!title && !url) return undefined;
      return { title, url, snippet };
    })
    .filter((x): x is SearchResultEntry => x !== undefined);
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
    word-break: break-all

  &__text
    font-size: 13px
    color: var(--color-text-primary)

  &__link
    font-size: 13px
    color: var(--color-accent)
    text-decoration: none
    word-break: break-all
    &:hover
      color: var(--color-accent-hover)
      text-decoration: underline

  &__snippet
    font-size: 12px
    color: var(--color-text-secondary)
    margin: 2px 0 0 0
    line-height: 1.5

  &__list
    list-style: none
    padding: 0
    margin: 0
    max-height: 320px
    overflow-y: auto

  &__search-item
    padding: 4px 0
    border-bottom: 1px solid var(--glass-border)
    &:last-child
      border-bottom: none

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
