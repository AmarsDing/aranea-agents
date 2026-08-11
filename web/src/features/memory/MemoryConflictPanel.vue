// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">{{ t('memory.conflict.title') }}</div>
        <div class="text-caption text-grey-7">{{ t('memory.conflict.caption') }}</div>
      </div>
      <q-btn
        flat
        round
        icon="refresh"
        :loading="loading"
        :aria-label="t('memory.conflict.refreshAria')"
        @click="$emit('refresh')"
      />
    </q-card-section>

    <q-card-section v-if="!loading && rows.length === 0" class="app-registry-empty app-empty-state-center">
      <q-icon name="handshake" size="44px" color="grey-6" />
      <div class="text-subtitle1 q-mt-sm">{{ t('memory.conflict.emptyTitle') }}</div>
      <div class="text-caption text-grey-7">{{ t('memory.conflict.emptyCaption') }}</div>
    </q-card-section>

    <q-list v-else separator class="memory-conflict-list">
      <q-item v-for="fact in rows" :key="fact.id" class="memory-conflict-item">
        <q-item-section>
          <q-item-label class="text-weight-medium">{{ fact.statement }}</q-item-label>
          <q-item-label caption class="q-mt-xs">
            <q-chip dense square size="sm" color="primary" text-color="white" class="q-mr-xs">
              {{ fact.scope_type }}
            </q-chip>
            <q-chip dense square size="sm" :color="statusColor(fact.status)" text-color="white" class="q-mr-xs">
              {{ statusLabel(fact.status) }}
            </q-chip>
            <span>{{
              t('memory.conflict.meta', { confidence: formatPercent(fact.confidence), conflicts: fact.conflict_count })
            }}</span>
          </q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="row q-gutter-xs">
            <q-btn
              flat
              dense
              round
              icon="visibility"
              color="primary"
              :aria-label="t('memory.conflict.detailAria')"
              @click="$emit('openFact', fact)"
            />
            <q-btn
              outline
              dense
              no-caps
              color="positive"
              icon="thumb_up"
              :label="t('memory.conflict.confirm')"
              :loading="actingId === fact.id"
              :disable="fact.status !== 'active'"
              @click="$emit('review', fact, 'confirm')"
            />
            <q-btn
              outline
              dense
              no-caps
              color="negative"
              icon="thumb_down"
              :label="t('memory.conflict.reject')"
              :loading="actingId === fact.id"
              :disable="fact.status !== 'active'"
              @click="$emit('review', fact, 'reject')"
            />
            <q-btn
              outline
              dense
              no-caps
              color="blue-grey"
              icon="block"
              :label="t('memory.conflict.deprecate')"
              :loading="actingId === fact.id"
              :disable="fact.status !== 'active'"
              @click="$emit('review', fact, 'deprecate')"
            />
          </div>
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { FactReviewAction, MemoryFact } from './types';

const { t } = useI18n();

defineProps<{
  rows: MemoryFact[];
  loading: boolean;
  actingId: string | null;
}>();

defineEmits<{
  refresh: [];
  openFact: [fact: MemoryFact];
  review: [fact: MemoryFact, action: FactReviewAction];
}>();

function statusColor(status?: string) {
  switch (status) {
    case 'active':
      return 'positive';
    case 'disputed':
      return 'warning';
    case 'archived':
    case 'deprecated':
      return 'blue-grey';
    default:
      return 'grey';
  }
}

function statusLabel(status?: string) {
  const key = `memory.knowledge.status.${status || 'active'}`;
  const translated = t(key);
  return translated !== key ? translated : status || 'active';
}

function formatPercent(value?: number) {
  return `${Math.round((Number(value) || 0) * 100)}%`;
}
</script>

<style scoped>
.memory-conflict-list {
  max-height: 420px;
  overflow-y: auto;
}
</style>
