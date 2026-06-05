<template>
  <section class="settings-section">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">观察记录</span>
        </div>
        <p class="settings-section__hint">Agent 运行时收集的原始行为数据。</p>
      </div>
    </div>
    <q-inner-loading :showing="loading" label="加载观察记录..." />
    <q-list v-if="!loading && observations.length > 0" separator class="app-glass-list">
      <q-item v-for="item in observations" :key="item.id" class="app-glass-list__item--md">
        <q-item-section side>
          <q-icon :name="observationKindIcon(item.kind)" :color="observationKindColor(item.kind)" />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ item.content || observationKindLabel(item.kind) }}</q-item-label>
          <q-item-label caption>
            Session: {{ item.session_id.slice(0, 12) }}... · {{ formatDate(item.observed_at) }}
          </q-item-label>
        </q-item-section>
      </q-item>
    </q-list>
    <q-banner v-else-if="!loading" rounded class="settings-placeholder-banner"> 暂无观察记录。 </q-banner>
  </section>
</template>

<script setup lang="ts">
import type { LearningObservation } from '../../features/agents/learning.types';
import { formatDate } from '../../features/agents/learning.utils';

defineProps<{
  observations: LearningObservation[];
  loading: boolean;
}>();

function observationKindIcon(kind: string): string {
  switch (kind) {
    case 'tool_call':
      return 'build';
    case 'feedback':
      return 'chat';
    case 'memory_hit':
      return 'psychology';
    case 'memory_miss':
      return 'psychology';
    default:
      return 'visibility';
  }
}

function observationKindColor(kind: string): string {
  switch (kind) {
    case 'tool_call':
      return 'blue';
    case 'feedback':
      return 'purple';
    case 'memory_hit':
      return 'teal';
    case 'memory_miss':
      return 'grey';
    default:
      return 'grey';
  }
}

function observationKindLabel(kind: string): string {
  switch (kind) {
    case 'tool_call':
      return '工具调用';
    case 'feedback':
      return '用户反馈';
    case 'memory_hit':
      return '记忆命中';
    case 'memory_miss':
      return '记忆未命中';
    default:
      return kind;
  }
}
</script>
