<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">{{ t('memory.pii.title') }}</div>
        <div class="text-caption text-grey-7">{{ t('memory.pii.caption') }}</div>
      </div>
      <q-btn
        flat
        round
        icon="refresh"
        :loading="loading"
        :aria-label="t('memory.pii.refreshAria')"
        @click="$emit('refresh')"
      />
    </q-card-section>

    <q-card-section v-if="!loading && rows.length === 0" class="app-registry-empty app-empty-state-center">
      <q-icon name="privacy_tip" size="44px" color="grey-6" />
      <div class="text-subtitle1 q-mt-sm">{{ t('memory.pii.emptyTitle') }}</div>
      <div class="text-caption text-grey-7">{{ t('memory.pii.emptyCaption') }}</div>
    </q-card-section>

    <q-list v-else separator>
      <q-item v-for="fact in rows" :key="fact.id">
        <q-item-section>
          <q-item-label class="text-weight-medium">{{ displayStatement(fact) }}</q-item-label>
          <q-item-label caption class="q-mt-xs">
            <q-chip
              v-for="pt in fact.pii_types"
              :key="pt"
              dense
              square
              size="sm"
              color="deep-orange"
              text-color="white"
              class="q-mr-xs"
            >
              {{ pt }}
            </q-chip>
            <span>{{ fact.scope_type }}</span>
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
              :aria-label="t('memory.pii.detailAria')"
              @click="$emit('openFact', fact)"
            />
            <q-btn
              outline
              dense
              no-caps
              color="positive"
              icon="check"
              :label="t('memory.pii.approve')"
              :loading="actingId === fact.id"
              @click="$emit('review', fact, 'approve')"
            />
            <q-btn
              outline
              dense
              no-caps
              color="negative"
              icon="block"
              :label="t('memory.pii.reject')"
              :loading="actingId === fact.id"
              @click="$emit('review', fact, 'reject')"
            />
          </div>
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { MemoryFact } from './types';

defineProps<{
  rows: MemoryFact[];
  loading: boolean;
  actingId: string | null;
}>();

defineEmits<{
  refresh: [];
  openFact: [fact: MemoryFact];
  review: [fact: MemoryFact, action: 'approve' | 'reject'];
}>();

const { t } = useI18n();

function displayStatement(fact: MemoryFact): string {
  return fact.statement || fact.id;
}
</script>
