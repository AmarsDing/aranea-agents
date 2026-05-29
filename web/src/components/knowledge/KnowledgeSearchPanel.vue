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
      <q-input
        :model-value="topK"
        dense
        outlined
        type="number"
        label="Top K"
        style="max-width: 120px"
        @update:model-value="$emit('update:topK', Number($event) || 5)"
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
          <q-item-label caption>score {{ chunk.score.toFixed(2) }} · chunk #{{ chunk.chunk_index }}</q-item-label>
          <q-item-label class="q-mt-xs">{{ chunk.content }}</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>
    <div v-else-if="searched" class="text-grey-7">无匹配结果。</div>
  </div>
</template>

<script setup lang="ts">
import type { KnowledgeChunk } from "../../features/knowledge/types";

defineProps<{
  query: string;
  topK: number;
  hybridMode: string;
  rewriteStrategy: string;
  useRerank: boolean;
  results: KnowledgeChunk[];
  loading: boolean;
  searched: boolean;
}>();

defineEmits<{
  "update:query": [value: string];
  "update:topK": [value: number];
  "update:hybridMode": [value: string];
  "update:rewriteStrategy": [value: string];
  "update:useRerank": [value: boolean];
  search: [];
}>();

const hybridModeOptions = [
  { label: "自动", value: "auto" },
  { label: "向量检索", value: "dense" },
  { label: "全文检索", value: "sparse" },
  { label: "混合 (RRF)", value: "rrf" }
];

const rewriteStrategyOptions = [
  { label: "无", value: "" },
  { label: "HyDE", value: "hyde" },
  { label: "查询分解", value: "decomposition" },
  { label: "多查询", value: "multi_query" }
];
</script>
