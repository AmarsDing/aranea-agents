// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-drawer :model-value="modelValue" side="right" overlay bordered :width="520" class="memory-drawer" @update:model-value="$emit('update:modelValue', $event)">
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
            <q-chip dense color="blue-grey" text-color="white">{{ fact.fact_kind || "fact" }}</q-chip>
            <q-chip dense :color="scoreColor(fact.confidence)" text-color="white">confidence {{ formatPercent(fact.confidence) }}</q-chip>
          </div>
          <q-separator class="q-my-md" />
          <div class="text-caption text-grey-7">Details</div>
          <pre class="memory-pre">{{ fact.details_markdown || "暂无详情" }}</pre>
          <div class="text-caption text-grey-7 q-mt-md">Source</div>
          <div class="text-body2">{{ fact.source_kind || "unknown" }} · {{ fact.source_session_id || fact.source_episode_id || "无来源引用" }}</div>
        </template>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import type { MemoryFact } from "./types";

defineProps<{
  modelValue: boolean;
  fact: MemoryFact | null;
}>();

defineEmits<{
  "update:modelValue": [value: boolean];
}>();

function bounded(value?: number) {
  const numeric = Number(value) || 0;
  return Math.max(0, Math.min(1, numeric));
}

function scoreColor(value?: number) {
  const score = bounded(value);
  if (score >= 0.75) return "positive";
  if (score >= 0.45) return "warning";
  return "negative";
}

function formatPercent(value?: number) {
  return `${Math.round((Number(value) || 0) * 100)}%`;
}
</script>
