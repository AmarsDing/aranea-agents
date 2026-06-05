<template>
  <section class="settings-section">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">学习模式</span>
        </div>
        <p class="settings-section__hint">从观察中提取的行为模式，按置信度和频率排序。</p>
      </div>
      <q-btn-toggle
        :model-value="statusFilter"
        rounded
        unelevated
        toggle-color="primary"
        :options="statusOptions"
        @update:model-value="emit('update:status-filter', $event)"
      />
    </div>
    <q-inner-loading :showing="loading" label="加载模式..." />
    <q-list v-if="!loading && patterns.length > 0" separator class="app-glass-list">
      <q-item v-for="p in patterns" :key="p.id" class="app-glass-list__item--md">
        <q-item-section>
          <q-item-label class="text-weight-medium">
            <q-badge :color="patternKindColor(p.kind)" class="q-mr-sm" :label="p.kind" />
            {{ p.description }}
          </q-item-label>
          <q-item-label caption class="q-mt-xs">
            频率 {{ p.frequency }} · 置信度 {{ formatConfidence(p.confidence) }}
          </q-item-label>
          <q-item-label caption class="q-mt-xs text-grey-5">{{ formatDate(p.detected_at) }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-badge :color="patternStatusColor(p.status)" :label="patternStatusLabel(p.status)" />
        </q-item-section>
      </q-item>
    </q-list>
    <q-banner v-else-if="!loading" rounded class="settings-placeholder-banner"> 暂无学习模式数据。 </q-banner>
  </section>
</template>

<script setup lang="ts">
import type { LearningPattern } from '../../features/agents/learning.types';
import { formatDate } from '../../features/agents/learning.utils';

defineProps<{
  patterns: LearningPattern[];
  loading: boolean;
  statusFilter: string;
}>();

const emit = defineEmits<{
  'update:status-filter': [value: string];
}>();

const statusOptions = [
  { label: '全部', value: '' },
  { label: '已检测', value: 'detected' },
  { label: '已确认', value: 'confirmed' },
  { label: '已忽略', value: 'dismissed' },
];

function patternKindColor(kind: string): string {
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

function patternStatusColor(status: string): string {
  switch (status) {
    case 'detected':
      return 'orange';
    case 'confirmed':
      return 'positive';
    case 'dismissed':
      return 'grey';
    default:
      return 'grey';
  }
}

function patternStatusLabel(status: string): string {
  switch (status) {
    case 'detected':
      return '已检测';
    case 'confirmed':
      return '已确认';
    case 'dismissed':
      return '已忽略';
    default:
      return status;
  }
}

function formatConfidence(v: number): string {
  if (v === 0) return '—';
  return (v * 100).toFixed(1) + '%';
}
</script>
