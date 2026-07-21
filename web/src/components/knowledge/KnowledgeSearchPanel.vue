<template>
  <div class="q-gutter-md">
    <q-input
      :model-value="query"
      dense
      outlined
      label="查询文本"
      type="textarea"
      autogrow
      @update:model-value="$emit('update:query', String($event ?? ''))"
    />
    <div class="row q-gutter-sm items-center">
      <q-select
        :model-value="scopeId"
        dense
        options-dense
        outlined
        emit-value
        map-options
        :options="scopeOptions"
        :label="t('knowledgePage.searchScopeLabel')"
        style="min-width: 180px"
        @update:model-value="$emit('update:scopeId', String($event ?? ''))"
      />
      <q-input
        :model-value="topK"
        dense
        outlined
        type="number"
        label="Top K"
        style="max-width: 120px"
        @update:model-value="$emit('update:topK', Number($event) || 5)"
      />
      <q-input
        :model-value="minScore"
        dense
        outlined
        type="number"
        step="0.1"
        min="0"
        max="1"
        label="最低相似度"
        style="max-width: 140px"
        @update:model-value="$emit('update:minScore', Number($event) || 0)"
      />
      <q-btn color="primary" unelevated icon="search" label="检索" :loading="loading" @click="$emit('search')" />
    </div>
    <q-expansion-item
      dense
      dense-toggle
      label="高级检索选项"
      header-class="text-caption text-grey-7"
      expand-icon-class="text-grey-6"
    >
      <div class="q-gutter-sm q-pa-sm">
        <div class="row q-gutter-sm items-center">
          <q-select
            :model-value="hybridMode"
            dense
            outlined
            label="检索模式"
            :options="hybridModeOptions"
            emit-value
            map-options
            style="min-width: 160px"
            @update:model-value="$emit('update:hybridMode', String($event ?? 'auto'))"
          />
          <q-select
            :model-value="rewriteStrategy"
            dense
            outlined
            label="查询重写"
            :options="rewriteStrategyOptions"
            emit-value
            map-options
            style="min-width: 160px"
            @update:model-value="$emit('update:rewriteStrategy', String($event ?? ''))"
          />
          <q-toggle
            :model-value="useRerank"
            dense
            label="Rerank"
            @update:model-value="$emit('update:useRerank', !!$event)"
          />
        </div>
      </div>
    </q-expansion-item>
    <q-list v-if="results.length" bordered separator class="rounded-borders">
      <q-item v-for="chunk in results" :key="chunk.id">
        <q-item-section>
          <q-item-label caption
            >score {{ chunk.score.toFixed(2) }} · chunk #{{ chunk.chunk_index
            }}<template v-if="chunk.doc_id">
              · {{ docSourceMap?.[chunk.doc_id] ?? chunk.doc_id.slice(0, 8) }}</template
            ></q-item-label
          >
          <q-item-label class="q-mt-xs">{{ chunk.content }}</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>
    <div v-else-if="searched" class="text-grey-7">无匹配结果。</div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { KnowledgeChunk } from '../../features/knowledge/types';
import {
  KNOWLEDGE_HYBRID_MODE_OPTIONS,
  KNOWLEDGE_REWRITE_STRATEGY_OPTIONS,
} from '../../features/knowledge/knowledgeUi';

defineProps<{
  query: string;
  topK: number;
  minScore: number;
  hybridMode: string;
  rewriteStrategy: string;
  useRerank: boolean;
  /** US-14：检索范围，空 = 全部知识库（智能路由） */
  scopeId: string;
  scopeOptions: Array<{ label: string; value: string }>;
  results: KnowledgeChunk[];
  docSourceMap?: Record<string, string>;
  loading: boolean;
  searched: boolean;
}>();

defineEmits<{
  'update:query': [value: string];
  'update:topK': [value: number];
  'update:minScore': [value: number];
  'update:hybridMode': [value: string];
  'update:rewriteStrategy': [value: string];
  'update:useRerank': [value: boolean];
  'update:scopeId': [value: string];
  search: [];
}>();

const { t } = useI18n();

const hybridModeOptions = KNOWLEDGE_HYBRID_MODE_OPTIONS;
const rewriteStrategyOptions = KNOWLEDGE_REWRITE_STRATEGY_OPTIONS;
</script>
