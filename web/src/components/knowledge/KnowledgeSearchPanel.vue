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
    <q-list v-if="results.length" bordered separator class="rounded-borders">
      <q-item v-for="chunk in results" :key="chunk.id">
        <q-item-section>
          <q-item-label caption>score {{ chunk.score.toFixed(4) }} · chunk #{{ chunk.chunk_index }}</q-item-label>
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
  results: KnowledgeChunk[];
  loading: boolean;
  searched: boolean;
}>();

defineEmits<{
  "update:query": [value: string];
  "update:topK": [value: number];
  search: [];
}>();
</script>
