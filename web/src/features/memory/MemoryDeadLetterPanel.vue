// Container: approved — dead-letter queue management with replay/abandon actions. // FD4+FB3 fix: data fetching + error
handling extracted to useMemoryDeadLetterPanel composable.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">{{ t('memory.deadLetter.title') }}</div>
        <div class="text-caption text-grey-7">{{ t('memory.deadLetter.subtitle') }}</div>
      </div>
      <q-btn flat dense icon="refresh" :loading="loading" @click="load" />
    </q-card-section>
    <q-card-section v-if="rows.length">
      <AppRegistryMarkupTable :rows="tableRows" :columns="columns" row-key="id">
        <template #cell-session_id="{ row }">
          <span class="ellipsis block" :title="String(row.session_id ?? '')">{{ row.session_id }}</span>
        </template>
        <template #cell-priority="{ row }">
          <q-badge :color="priorityColor(Number(row.priority))">{{ priorityLabel(Number(row.priority)) }}</q-badge>
        </template>
        <template #cell-drop_reason="{ row }">
          <span class="ellipsis block" :title="String(row.drop_reason ?? '')">{{ row.drop_reason }}</span>
        </template>
        <template #cell-state="{ row }">
          <AppStatusChip :status="String(row.state ?? '')" />
        </template>
        <template #cell-actions="{ row }">
          <div class="row items-center justify-end q-gutter-xs">
            <q-btn
              v-if="row.state === 'pending'"
              flat
              dense
              color="primary"
              icon="replay"
              size="sm"
              @click="replay(Number(row.id))"
            >
              <q-tooltip>{{ t('memory.deadLetter.tooltips.retry') }}</q-tooltip>
            </q-btn>
            <q-btn
              v-if="row.state === 'pending'"
              flat
              dense
              color="negative"
              icon="delete_outline"
              size="sm"
              @click="abandon(Number(row.id))"
            >
              <q-tooltip>{{ t('memory.deadLetter.tooltips.abandon') }}</q-tooltip>
            </q-btn>
          </div>
        </template>
      </AppRegistryMarkupTable>
    </q-card-section>
    <q-card-section v-else-if="!loading" class="text-grey-7 text-caption">{{
      t('memory.deadLetter.empty')
    }}</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryMarkupTable from '../../components/layout/AppRegistryMarkupTable.vue';
import AppStatusChip from '../../components/common/AppStatusChip.vue';
import { registryCol, registryColActions, type RegistryTableColumn } from '../ui/registryTableColumns';
import type { MemoryDeadLetterEntry } from './types';
import { useMemoryDeadLetterPanel } from './composables/useMemoryDeadLetterPanel';

const { t, locale } = useI18n();

const emit = defineEmits<{
  (e: 'replay', id: number): void;
  (e: 'abandon', id: number): void;
}>();

const { rows, loading, load } = useMemoryDeadLetterPanel();

const tableRows = computed(() => rows.value as unknown as Record<string, unknown>[]);

const columns = computed<RegistryTableColumn[]>(() => [
  registryCol('id', t('memory.deadLetter.columns.id'), 'id', 'left', '64px'),
  registryCol('session_id', t('memory.deadLetter.columns.session'), 'session_id', 'left', '140px'),
  registryCol('app_name', t('memory.deadLetter.columns.app'), 'app_name', 'left', '10%'),
  registryCol('priority', t('memory.deadLetter.columns.priority'), 'priority', 'left', '72px'),
  registryCol('drop_reason', t('memory.deadLetter.columns.reason'), 'drop_reason', 'left', '20%'),
  registryCol('attempts', t('memory.deadLetter.columns.attempts'), 'attempts', 'right', '64px'),
  registryCol('state', t('memory.deadLetter.columns.state'), 'state', 'left', '96px'),
  registryCol('failed_at', t('memory.deadLetter.columns.failedAt'), (row) =>
    formatTime(String((row as unknown as MemoryDeadLetterEntry).failed_at ?? '')),
  'left', '160px'),
  registryColActions('120px', t('memory.deadLetter.columns.actions')),
]);

function priorityLabel(p: number) {
  if (p >= 2) return t('memory.deadLetter.priority.high');
  if (p === 1) return t('memory.deadLetter.priority.normal');
  return t('memory.deadLetter.priority.low');
}

function priorityColor(p: number) {
  if (p >= 2) return 'negative';
  if (p === 1) return 'warning';
  return 'grey';
}

function formatTime(value: string) {
  if (!value) return '-';
  try {
    return new Date(value).toLocaleString(locale.value);
  } catch {
    return value;
  }
}

function replay(id: number) {
  emit('replay', id);
}

function abandon(id: number) {
  emit('abandon', id);
}

defineExpose({ load });
</script>
