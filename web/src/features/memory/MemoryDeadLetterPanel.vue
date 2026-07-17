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
      <q-markup-table flat dense bordered>
        <thead>
          <tr>
            <th>{{ t('memory.deadLetter.columns.id') }}</th>
            <th>{{ t('memory.deadLetter.columns.session') }}</th>
            <th>{{ t('memory.deadLetter.columns.app') }}</th>
            <th>{{ t('memory.deadLetter.columns.priority') }}</th>
            <th>{{ t('memory.deadLetter.columns.reason') }}</th>
            <th>{{ t('memory.deadLetter.columns.attempts') }}</th>
            <th>{{ t('memory.deadLetter.columns.state') }}</th>
            <th>{{ t('memory.deadLetter.columns.failedAt') }}</th>
            <th>{{ t('memory.deadLetter.columns.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.id">
            <td>{{ r.id }}</td>
            <td class="ellipsis-cell" style="max-width: 120px">{{ r.session_id }}</td>
            <td>{{ r.app_name }}</td>
            <td>
              <q-badge :color="priorityColor(r.priority)">{{ priorityLabel(r.priority) }}</q-badge>
            </td>
            <td class="ellipsis-cell" style="max-width: var(--col-w-lg, 200px)" :title="r.drop_reason">
              {{ r.drop_reason }}
            </td>
            <td>{{ r.attempts }}</td>
            <td>
              <q-badge :color="stateColor(r.state)">{{ stateLabel(r.state) }}</q-badge>
            </td>
            <td>{{ formatTime(r.failed_at) }}</td>
            <td class="q-gutter-xs">
              <q-btn
                v-if="r.state === 'pending'"
                flat
                dense
                color="primary"
                icon="replay"
                size="sm"
                @click="replay(r.id)"
              >
                <q-tooltip>{{ t('memory.deadLetter.tooltips.retry') }}</q-tooltip>
              </q-btn>
              <q-btn
                v-if="r.state === 'pending'"
                flat
                dense
                color="negative"
                icon="delete_outline"
                size="sm"
                @click="abandon(r.id)"
              >
                <q-tooltip>{{ t('memory.deadLetter.tooltips.abandon') }}</q-tooltip>
              </q-btn>
            </td>
          </tr>
        </tbody>
      </q-markup-table>
    </q-card-section>
    <q-card-section v-else-if="!loading" class="text-grey-7 text-caption">{{
      t('memory.deadLetter.empty')
    }}</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useMemoryDeadLetterPanel } from './composables/useMemoryDeadLetterPanel';

const { t, te, locale } = useI18n();

const emit = defineEmits<{
  (e: 'replay', id: number): void;
  (e: 'abandon', id: number): void;
}>();

const { rows, loading, load } = useMemoryDeadLetterPanel();

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

function stateColor(s: string) {
  if (s === 'pending') return 'warning';
  if (s === 'replayed') return 'positive';
  if (s === 'abandoned') return 'grey';
  return 'dark';
}

function stateLabel(s: string) {
  const key = `memory.deadLetter.state.${s}`;
  return te(key) ? t(key) : s;
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
