<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">{{ t('memory.pending.title') }}</div>
        <div class="text-caption text-grey-7">{{ t('memory.pending.caption') }}</div>
      </div>
      <div class="row items-center q-gutter-xs">
        <q-chip
          v-for="opt in statusOptions"
          :key="opt"
          dense
          square
          clickable
          :color="statusFilter === opt ? 'primary' : 'grey-3'"
          :text-color="statusFilter === opt ? 'white' : 'grey-8'"
          @click="setStatusFilter(opt)"
        >
          {{ t(`memory.pending.status.${opt || 'all'}`) }}
        </q-chip>
        <q-btn
          flat
          round
          icon="refresh"
          :loading="loading"
          :aria-label="t('memory.pending.refreshAria')"
          @click="load"
        />
      </div>
    </q-card-section>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mx-md q-mb-md">
      {{ error }}
    </q-banner>

    <q-card-section v-if="!loading && rows.length === 0 && !error" class="app-registry-empty app-empty-state-center">
      <q-icon name="pending_actions" size="44px" color="grey-6" />
      <div class="text-subtitle1 q-mt-sm">{{ t('memory.pending.emptyTitle') }}</div>
      <div class="text-caption text-grey-7">{{ t('memory.pending.emptyCaption') }}</div>
    </q-card-section>

    <q-list v-else separator>
      <q-item v-for="item in rows" :key="item.id" class="pending-item">
        <q-item-section>
          <q-item-label class="row items-center q-gutter-xs">
            <q-chip dense square size="sm" :color="verdictColor(item.verdict)" text-color="white">
              {{ item.verdict }}
            </q-chip>
            <q-chip dense square size="sm" :color="statusColor(item.status)" text-color="white">
              {{ t(`memory.pending.status.${item.status}`) }}
            </q-chip>
            <span class="text-caption text-grey-6">{{ formatTs(item.created_at) }}</span>
            <span v-if="item.approver" class="text-caption text-grey-6"> · {{ item.approver }} </span>
          </q-item-label>

          <q-item-label class="q-mt-sm">
            <div class="row q-col-gutter-md">
              <div v-if="item.prior_body" class="col-12 col-md-6">
                <div class="text-caption text-grey-6">{{ t('memory.pending.priorLabel') }}</div>
                <div class="pending-body pending-body--prior">{{ item.prior_body }}</div>
              </div>
              <div class="col-12" :class="item.prior_body ? 'col-md-6' : ''">
                <div class="text-caption text-grey-6">{{ t('memory.pending.proposedLabel') }}</div>
                <div class="pending-body pending-body--proposed">{{ item.proposed_body || '—' }}</div>
              </div>
            </div>
          </q-item-label>

          <q-item-label v-if="item.adjudicator_reason" caption class="q-mt-xs">
            <q-icon name="gavel" size="14px" class="q-mr-xs" />{{ item.adjudicator_reason }}
          </q-item-label>
          <q-item-label v-if="item.fact_key" caption class="text-grey-6">
            {{ t('memory.pending.targetFact') }}: {{ item.fact_key }}
          </q-item-label>
        </q-item-section>
      </q-item>
    </q-list>

    <q-card-section class="text-caption text-grey-6">
      <q-icon name="info_outline" size="14px" class="q-mr-xs" />{{ t('memory.pending.decideHint') }}
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { listMemoryFactPendings } from './api';
import type { MemoryFactPendingItem } from './types';

const props = defineProps<{
  agentId: string | null;
}>();

const { t } = useI18n();

const rows = ref<MemoryFactPendingItem[]>([]);
const loading = ref(false);
const error = ref('');
const statusFilter = ref('pending');
const statusOptions = ['pending', 'approved', 'rejected', ''] as const;

function setStatusFilter(opt: string) {
  if (statusFilter.value === opt) return;
  statusFilter.value = opt;
  void load();
}

async function load() {
  if (!props.agentId) {
    rows.value = [];
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    rows.value = await listMemoryFactPendings(props.agentId, statusFilter.value);
  } catch (e) {
    rows.value = [];
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

function verdictColor(verdict: string): string {
  switch (verdict) {
    case 'UPDATE':
      return 'orange';
    case 'DELETE':
      return 'negative';
    default:
      return 'deep-purple';
  }
}

function statusColor(status: string): string {
  switch (status) {
    case 'approved':
      return 'positive';
    case 'rejected':
      return 'grey-6';
    default:
      return 'warning';
  }
}

function formatTs(unixSec: number): string {
  if (!unixSec) return '';
  return new Date(unixSec * 1000).toLocaleString();
}

watch(
  () => props.agentId,
  () => void load(),
  { immediate: true },
);
</script>

<style scoped>
.pending-body {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  line-height: 1.5;
  padding: 8px 10px;
  border-radius: 6px;
}

.pending-body--prior {
  background: rgba(0, 0, 0, 0.04);
}

.pending-body--proposed {
  background: rgba(25, 118, 210, 0.07);
}
</style>
