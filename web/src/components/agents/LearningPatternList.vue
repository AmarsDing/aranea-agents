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
          <div class="row items-center no-wrap q-gutter-xs">
            <q-badge :color="patternStatusColor(p.status)" :label="patternStatusLabel(p.status)" />
            <template v-if="p.status === 'detected'">
              <q-btn
                flat
                dense
                round
                size="sm"
                icon="check"
                color="positive"
                @click="emit('confirm', p.id)"
              >
                <q-tooltip>确认模式</q-tooltip>
              </q-btn>
              <q-btn
                flat
                dense
                round
                size="sm"
                icon="visibility_off"
                color="grey"
                @click="emit('dismiss', p.id)"
              >
                <q-tooltip>忽略模式</q-tooltip>
              </q-btn>
            </template>
          </div>
        </q-item-section>
      </q-item>
    </q-list>
    <q-banner v-else-if="!loading" rounded class="settings-placeholder-banner"> 暂无学习模式数据。 </q-banner>
  </section>
</template>

<script setup lang="ts">
import type { LearningPattern } from '../../features/agents/learning.types';
import { formatDate } from '../../features/agents/learning.utils';
import AppStatusChip from '../common/AppStatusChip.vue';

defineProps<{
  patterns: LearningPattern[];
  loading: boolean;
  statusFilter: string;
}>();

const emit = defineEmits<{
  'update:status-filter': [value: string];
  confirm: [patternId: string];
  dismiss: [patternId: string];
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
      return t('agents.learning_loop.pattern_status_detected');
    case 'confirmed':
      return t('agents.learning_loop.pattern_status_confirmed');
    case 'dismissed':
      return t('agents.learning_loop.pattern_status_dismissed');
    default:
      return status;
  }
}

function formatConfidence(v: number): string {
  if (v === 0) return '—';
  return (v * 100).toFixed(1) + '%';
}
</script>
