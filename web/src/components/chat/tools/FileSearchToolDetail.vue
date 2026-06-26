<template>
  <div class="tool-detail">
    <div v-if="pattern" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.searchPattern') }}</div>
      <code class="tool-detail__inline">{{ pattern }}</code>
    </div>
    <div v-if="basePath" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.path') }}</span>
      <code class="tool-detail__inline">{{ basePath }}</code>
    </div>
    <div v-if="hitCount != null" class="tool-detail__row">
      <span class="tool-detail__label-inline">{{ t('chat.toolDetail.hitCount') }}</span>
      <code class="tool-detail__inline">{{ hitCount }}</code>
    </div>
    <div v-if="results.length" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.resultList') }}</div>
      <ul class="tool-detail__list">
        <li v-for="(item, idx) in results" :key="idx" class="tool-detail__list-item">
          <code class="tool-detail__inline tool-detail__inline--path">{{ item.path }}</code>
          <span v-if="item.line != null" class="tool-detail__line">:{{ item.line }}</span>
        </li>
      </ul>
    </div>
    <div v-if="activity.tool.arguments && !pattern" class="tool-detail__row">
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
import { tryParseJson, asRecord, asArray, asString, asNumber } from './toolDetailShared';

const { t } = useI18n();

interface SearchResultEntry {
  path: string;
  line?: number;
}

const props = defineProps<{ activity: ActionEvent }>();

const parsedArgs = computed(() => asRecord(tryParseJson(props.activity.tool.arguments)));
const parsedResult = computed(() => asRecord(tryParseJson(props.activity.tool.result)));

const pattern = computed(
  () =>
    asString(parsedArgs.value?.pattern) ?? asString(parsedArgs.value?.query) ?? asString(parsedArgs.value?.regex) ?? '',
);
const basePath = computed(() => asString(parsedArgs.value?.path) ?? asString(parsedArgs.value?.directory) ?? '');
const hitCount = computed(
  () =>
    asNumber(parsedResult.value?.count) ??
    asNumber(parsedResult.value?.hit_count) ??
    asNumber(parsedResult.value?.matches),
);
const results = computed<SearchResultEntry[]>(() => {
  const arr = asArray(parsedResult.value?.matches ?? parsedResult.value?.files ?? parsedResult.value?.results);
  if (!arr) return [];
  return arr
    .map((item): SearchResultEntry | undefined => {
      const rec = asRecord(item);
      if (!rec) return undefined;
      const path = asString(rec.path) ?? asString(rec.file) ?? asString(rec.name) ?? '';
      if (!path) return undefined;
      const line = asNumber(rec.line) ?? asNumber(rec.line_number);
      return { path, line };
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
    &--path
      color: var(--color-accent)

  &__line
    font-size: 12px
    color: var(--color-text-secondary)

  &__list
    list-style: none
    padding: 0
    margin: 0
    max-height: 240px
    overflow-y: auto

  &__list-item
    font-size: 12px
    padding: 2px 0
    color: var(--color-text-primary)
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
