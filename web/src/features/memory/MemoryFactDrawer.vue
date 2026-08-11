// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-drawer
    :model-value="modelValue"
    side="right"
    overlay
    bordered
    :width="520"
    class="memory-drawer"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <q-scroll-area class="fit">
      <div class="q-pa-md">
        <div class="row items-center justify-between q-mb-md">
          <div>
            <div class="text-h6">{{ t('memory.factDrawer.title') }}</div>
            <div class="text-caption text-grey-7">{{ fact?.id }}</div>
          </div>
          <q-btn
            flat
            round
            icon="close"
            :aria-label="t('memory.factDrawer.closeAria')"
            @click="$emit('update:modelValue', false)"
          />
        </div>
        <template v-if="fact">
          <div class="text-subtitle1 text-weight-bold">{{ fact.statement }}</div>
          <div class="q-mt-md row q-gutter-sm">
            <q-chip dense color="primary" text-color="white">{{ fact.scope_type }}</q-chip>
            <q-chip dense color="blue-grey" text-color="white">{{ fact.fact_kind || 'fact' }}</q-chip>
            <q-chip dense :color="statusColor(fact.status)" text-color="white">{{ statusLabel(fact.status) }}</q-chip>
            <q-chip dense :color="scoreColor(fact.confidence)" text-color="white">{{
              t('memory.factDrawer.confidence', { percent: formatPercent(fact.confidence) })
            }}</q-chip>
            <q-chip v-if="fact.quality_score > 0" dense :color="scoreColor(fact.quality_score)" text-color="white">{{
              t('memory.factDrawer.quality', { percent: formatPercent(fact.quality_score) })
            }}</q-chip>
            <q-chip v-if="fact.pii_flag" dense color="negative" text-color="white">{{
              t('memory.factDrawer.pii')
            }}</q-chip>
          </div>

          <div class="q-mt-md row q-gutter-sm memory-fact-actions">
            <q-btn
              outline
              rounded
              no-caps
              dense
              color="positive"
              icon="thumb_up"
              :label="t('memory.factDrawer.confirm')"
              :loading="acting"
              :disable="fact.status !== 'active'"
              @click="$emit('review', 'confirm')"
            />
            <q-btn
              outline
              rounded
              no-caps
              dense
              color="negative"
              icon="thumb_down"
              :label="t('memory.factDrawer.reject')"
              :loading="acting"
              :disable="fact.status !== 'active'"
              @click="$emit('review', 'reject')"
            />
            <q-btn
              outline
              rounded
              no-caps
              dense
              color="primary"
              icon="edit"
              :label="t('memory.factDrawer.refine')"
              :disable="acting || fact.status !== 'active'"
              @click="$emit('refine')"
            />
            <q-btn
              outline
              rounded
              no-caps
              dense
              color="blue-grey"
              icon="archive"
              :label="t('memory.factDrawer.archive')"
              :loading="acting"
              :disable="fact.status !== 'active'"
              @click="$emit('review', 'archive')"
            />
          </div>
          <div class="text-caption text-grey-6 q-mt-xs">{{ t('memory.factDrawer.actionsHint') }}</div>
          <div v-if="fact.pii_flag && fact.pii_types?.length" class="q-mt-sm">
            <q-badge v-for="pt in fact.pii_types" :key="pt" color="deep-orange" class="q-mr-xs">{{ pt }}</q-badge>
          </div>
          <q-separator class="q-my-md" />
          <div class="text-caption text-grey-7">{{ t('memory.factDrawer.usage') }}</div>
          <div class="q-mt-xs row q-gutter-sm">
            <q-chip dense outline color="primary">
              {{ t('memory.factDrawer.usageRecalled') }} · {{ fact.recalled_count }}
            </q-chip>
            <q-chip dense outline color="positive">
              {{ t('memory.factDrawer.usageInjected') }} · {{ fact.injected_count }}
            </q-chip>
            <q-chip dense outline color="deep-purple">
              {{ t('memory.factDrawer.usageCited') }} · {{ fact.cited_count }}
            </q-chip>
          </div>
          <div class="text-caption text-grey-6 q-mt-xs">{{ t('memory.factDrawer.usageHint') }}</div>
          <q-separator class="q-my-md" />
          <div class="text-caption text-grey-7">{{ t('memory.factDrawer.details') }}</div>
          <pre class="memory-pre">{{ fact.details_markdown || t('memory.factDrawer.noDetails') }}</pre>
          <div class="text-caption text-grey-7 q-mt-md">{{ t('memory.factDrawer.source') }}</div>
          <div class="text-body2">
            {{ fact.source_kind || t('memory.factDrawer.unknown') }} ·
            {{ fact.source_session_id || fact.source_episode_id || t('memory.factDrawer.noSourceRef') }}
          </div>
        </template>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { FactReviewAction, MemoryFact } from './types';

const { t } = useI18n();

defineProps<{
  modelValue: boolean;
  fact: MemoryFact | null;
  acting?: boolean;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
  review: [action: FactReviewAction];
  refine: [];
}>();

function bounded(value?: number) {
  const numeric = Number(value) || 0;
  return Math.max(0, Math.min(1, numeric));
}

function scoreColor(value?: number) {
  const score = bounded(value);
  if (score >= 0.75) return 'positive';
  if (score >= 0.45) return 'warning';
  return 'negative';
}

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
