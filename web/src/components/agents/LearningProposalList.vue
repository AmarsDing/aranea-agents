<template>
  <section class="settings-section">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">知识提议</span>
        </div>
        <p class="settings-section__hint">基于模式生成的知识注册提议，需审批后生效。</p>
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
    <q-inner-loading :showing="loading" label="加载提议..." />
    <q-list v-if="!loading && proposals.length > 0" separator class="app-glass-list">
      <q-item v-for="p in proposals" :key="p.id" class="app-glass-list__item--lg">
        <q-item-section>
          <q-item-label class="text-weight-medium">
            <q-badge :color="proposalKindColor(p.kind)" class="q-mr-sm" :label="p.kind" />
            {{ p.title }}
          </q-item-label>
          <q-item-label caption class="q-mt-xs">{{ p.content }}</q-item-label>
          <q-item-label caption class="q-mt-xs text-grey-5">
            {{ formatDate(p.created_at) }}
            <span v-if="p.approved_by"> · 审批人: {{ formatApprovedBy(p.approved_by) }}</span>
          </q-item-label>
        </q-item-section>
        <q-item-section side>
          <div v-if="p.status === 'validated'" class="row q-gutter-xs">
            <q-btn
              flat
              round
              dense
              icon="check"
              color="positive"
              size="sm"
              @click="emit('approve', p.id)"
            >
              <q-tooltip>审批</q-tooltip>
            </q-btn>
            <q-btn
              flat
              round
              dense
              icon="close"
              color="negative"
              size="sm"
              @click="emit('reject', p.id)"
            >
              <q-tooltip>拒绝</q-tooltip>
            </q-btn>
          </div>
          <div v-else-if="p.status === 'approved'" class="row items-center no-wrap q-gutter-xs">
            <AppStatusChip :status="p.status" />
            <q-btn
              flat
              round
              dense
              icon="school"
              color="primary"
              size="sm"
              @click="emit('apply', p.id)"
            >
              <q-tooltip>注册到知识库</q-tooltip>
            </q-btn>
          </div>
          <AppStatusChip v-else :status="p.status" />
        </q-item-section>
      </q-item>
    </q-list>
    <q-banner v-else-if="!loading" rounded class="settings-placeholder-banner"> 暂无知识提议。 </q-banner>
  </section>
</template>

<script setup lang="ts">
import type { LearningProposal } from '../../features/agents/learning.types';
import { formatDate, formatApprovedBy } from '../../features/agents/learning.utils';
import AppStatusChip from '../common/AppStatusChip.vue';

defineProps<{
  proposals: LearningProposal[];
  loading: boolean;
  statusFilter: string;
}>();

const emit = defineEmits<{
  'update:status-filter': [value: string];
  approve: [id: string];
  reject: [id: string];
  apply: [id: string];
}>();

const statusOptions = [
  { label: '全部', value: '' },
  { label: '已验证', value: 'validated' },
  { label: '已审批', value: 'approved' },
  { label: '已应用', value: 'applied' },
  { label: '已拒绝', value: 'rejected' },
  { label: '冲突', value: 'conflict' },
];

function proposalKindColor(kind: string): string {
  switch (kind) {
    case 'prompt':
      return 'blue';
    case 'skill':
      return 'teal';
    case 'persona':
      return 'purple';
    case 'behavior':
      return 'orange';
    default:
      return 'grey';
  }
}
</script>
