<template>
  <q-drawer
    :model-value="modelValue"
    side="right"
    overlay
    bordered
    :width="520"
    class="memory-drawer"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <q-scroll-area class="fit">
      <div class="q-pa-md">
        <div class="row items-center justify-between q-mb-md">
          <div>
            <div class="text-h6">Fact 详情</div>
            <div class="text-caption text-grey-7">{{ fact?.id }}</div>
          </div>
          <q-btn flat round icon="close" aria-label="关闭知识详情" @click="$emit('update:modelValue', false)" />
        </div>
        <template v-if="fact">
          <div class="text-subtitle1 text-weight-bold">{{ fact.statement }}</div>
          <div class="q-mt-md row q-gutter-sm">
            <q-chip dense color="primary" text-color="white">{{ fact.scope_type }}</q-chip>
            <q-chip dense color="blue-grey" text-color="white">{{ fact.fact_kind || 'fact' }}</q-chip>
            <q-chip dense :color="scoreColor(fact.confidence)" text-color="white"
              >confidence {{ formatPercent(fact.confidence) }}</q-chip
            >
            <q-chip v-if="fact.quality_score > 0" dense :color="scoreColor(fact.quality_score)" text-color="white"
              >quality {{ formatPercent(fact.quality_score) }}</q-chip
            >
            <q-chip v-if="fact.pii_flag" dense color="negative" text-color="white">PII</q-chip>
          </div>
          <div v-if="fact.pii_flag && fact.pii_types?.length" class="q-mt-sm">
            <q-badge v-for="t in fact.pii_types" :key="t" color="deep-orange" class="q-mr-xs">{{ t }}</q-badge>
          </div>
          <q-separator class="q-my-md" />
          <div class="text-caption text-grey-7">Details</div>
          <pre class="memory-pre">{{ fact.details_markdown || '暂无详情' }}</pre>
          <div class="text-caption text-grey-7 q-mt-md">Source</div>
          <div class="text-body2">
            {{ fact.source_kind || 'unknown' }} · {{ fact.source_session_id || fact.source_episode_id || '无来源引用' }}
          </div>
        </template>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import type { MemoryFact } from '../../features/memory/types';

defineProps<{
  modelValue: boolean;
  fact: MemoryFact | null;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
}>();

function bounded(value?: number) {
  const numeric = Number(value) || 0;
  return Math.max(0, Math.min(1, numeric));
}

function scoreColor(value?: number) {
  const score = bounded(value);
  if (score >= 0.75) return 'positive';
  if (score >= 0.45) return 'warning';
  return 'negative';
}

function formatPercent(value?: number) {
  return `${Math.round((Number(value) || 0) * 100)}%`;
}
</script>
