<!-- web/src/components/chat/KnowledgeRecallChips.vue
  知识召回透明度：turn 顶部展示本轮注入/检索的知识段落。
  数据源：activityV2Store.knowledgeChunksByTurn（knowledge_recalled notice）。
  原始 notice 已被 noticeFilter 隐藏。点击有 doc_id 的 chip 打开知识库工作台。
-->
<template>
  <div v-if="chunks.length > 0" class="knowledge-recall-chips">
    <div class="knowledge-recall-chips__header" role="button" tabindex="0" @click="onToggle" @keydown.enter="onToggle">
      <q-icon name="menu_book" size="14px" class="knowledge-recall-chips__icon" />
      <span class="knowledge-recall-chips__title">
        {{ t('chat.knowledgeRecall.title', { n: chunks.length }) }}
      </span>
      <span class="knowledge-recall-chips__toggle" aria-hidden="true">{{ collapsed ? '▶' : '▼' }}</span>
    </div>
    <div v-if="!collapsed" class="knowledge-recall-chips__list">
      <button
        v-for="(hit, i) in chunks"
        :key="hit.chunk_id || i"
        type="button"
        class="knowledge-recall-chips__chip"
        :class="{ 'knowledge-recall-chips__chip--link': !!hit.doc_id }"
        :disabled="!hit.doc_id"
        @click="onOpen(hit)"
      >
        <span class="knowledge-recall-chips__idx">[{{ i + 1 }}]</span>
        <span class="knowledge-recall-chips__line">{{ hit.line || hit.chunk_id }}</span>
        <span v-if="hit.score > 0" class="knowledge-recall-chips__score">{{ scoreLabel(hit.score) }}</span>
        <q-tooltip :delay="300" max-width="360px">
          <div>{{ hit.line || hit.chunk_id }}</div>
          <div v-if="hit.doc_id" class="knowledge-recall-chips__tip-meta">{{ hit.doc_id }}</div>
        </q-tooltip>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { useActivityQueries } from '../../features/chat/composables/useActivityQueries';
import { useCollapseState } from '../../features/chat/composables/useCollapseState';
import type { KnowledgeRecallChunk } from '../../features/chat/knowledgeRecall';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string, fallback?: unknown) => (typeof fallback === 'string' ? fallback : key) };
  }
}

const props = defineProps<{ turnId: string }>();
const { t } = useSafeI18n();
const store = useActivityQueries();
const chunks = computed(() => store.getTurnKnowledgeChunks(props.turnId));
const { collapsed, toggle } = useCollapseState(`kb-recall:${props.turnId}`, true);

let router: ReturnType<typeof useRouter> | null = null;
try {
  router = useRouter();
} catch {
  router = null;
}

function onToggle() {
  toggle();
}

function scoreLabel(score: number): string {
  return `${Math.round(score * 100)}%`;
}

function onOpen(hit: KnowledgeRecallChunk) {
  if (!hit.doc_id || !router) return;
  void router.push({ path: '/knowledge', query: { doc: hit.doc_id } });
}
</script>

<style lang="sass" scoped>
.knowledge-recall-chips
  margin: 2px 0 6px
  padding: 6px 10px
  border-radius: 10px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 4px

  &__icon
    color: var(--color-accent)

  &__title
    font-size: 12px
    color: var(--color-text-secondary)

  &__list
    display: flex
    flex-wrap: wrap
    gap: 6px

  &__chip
    display: inline-flex
    align-items: center
    gap: 6px
    max-width: 100%
    padding: 2px 8px
    border-radius: 999px
    font-size: 12px
    line-height: 1.6
    background: color-mix(in srgb, var(--color-success) 6%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-success) 18%, transparent)
    color: var(--color-text-secondary)
    cursor: default

    &--link
      cursor: pointer

    &:disabled
      cursor: default

  &__idx
    flex-shrink: 0
    font-size: 10px
    font-weight: 700
    color: var(--color-success)

  &__line
    min-width: 0
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    max-width: 320px

  &__score
    flex-shrink: 0
    font-size: 11px
    opacity: 0.75

  &__tip-meta
    margin-top: 4px
    opacity: 0.8
</style>
