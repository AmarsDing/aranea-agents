<template>
  <q-card flat bordered class="memory-card">
    <q-card-section>
      <div class="text-h6">{{ t('memory.evolution.reviewTitle') }}</div>
      <div class="text-caption text-grey-7">{{ t('memory.evolution.reviewCaption') }}</div>
    </q-card-section>

    <q-card-section v-if="!proposals.length && !events.length" class="text-center text-grey-7">
      <q-icon name="rule" size="36px" />
      <div class="q-mt-sm">{{ t('memory.evolution.reviewEmpty') }}</div>
    </q-card-section>

    <q-list v-if="proposals.length" separator>
      <q-item v-for="item in proposals" :key="item.id">
        <q-item-section>
          <q-item-label class="text-weight-medium">
            {{ item.target_field || item.proposal_kind || item.kind || item.id }}
          </q-item-label>
          <q-item-label caption>
            {{ item.rationale || item.expected_impact || item.status }}
          </q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="row q-gutter-xs items-center">
            <q-chip dense :color="riskColor(item.risk_level)" text-color="white">{{ item.risk_level || 'low' }}</q-chip>
            <q-btn
              v-if="item.status === 'pending'"
              outline
              dense
              no-caps
              color="positive"
              :label="t('memory.evolution.approve')"
              :loading="actingId === item.id"
              @click="$emit('approve', item)"
            />
            <q-btn
              v-if="item.status === 'pending'"
              outline
              dense
              no-caps
              color="negative"
              :label="t('memory.evolution.reject')"
              :loading="actingId === item.id"
              @click="$emit('reject', item)"
            />
            <span v-else class="text-caption text-grey-6">{{ item.status }}</span>
          </div>
        </q-item-section>
      </q-item>
    </q-list>

    <q-separator v-if="events.length" />
    <q-list v-if="events.length" separator>
      <q-item v-for="ev in events" :key="ev.id">
        <q-item-section>
          <q-item-label>{{ ev.event_kind || ev.kind }} · {{ ev.target_field }}</q-item-label>
          <q-item-label caption>{{ ev.reason }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-btn
            v-if="!ev.reverted"
            outline
            dense
            no-caps
            color="blue-grey"
            :label="t('memory.evolution.revert')"
            :loading="actingId === ev.id"
            @click="$emit('revert', ev)"
          />
          <q-chip v-else dense color="grey" text-color="white">{{ t('memory.evolution.reverted') }}</q-chip>
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { EvolutionEvent, EvolutionProposal } from './types';

defineProps<{
  proposals: EvolutionProposal[];
  events: EvolutionEvent[];
  actingId: string | null;
}>();

defineEmits<{
  approve: [item: EvolutionProposal];
  reject: [item: EvolutionProposal];
  revert: [item: EvolutionEvent];
}>();

const { t } = useI18n();

function riskColor(risk?: string): string {
  if (risk === 'high') return 'negative';
  if (risk === 'medium') return 'warning';
  return 'blue-grey';
}
</script>
