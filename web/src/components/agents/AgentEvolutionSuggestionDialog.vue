<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">{{ $t('agentSettings.evolution.detailTitle') }}</div>
          <div v-if="suggestion" class="app-glass-dialog__subtitle ellipsis">{{ suggestion.title }}</div>
        </div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section v-if="suggestion" class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <div class="row q-col-gutter-sm text-body2">
            <div class="col-6">
              <span class="text-grey-7">{{ $t('agentSettings.evolution.detailType') }}：</span>
              {{ typeLabel }}
            </div>
            <div class="col-6">
              <span class="text-grey-7">{{ $t('agentSettings.evolution.detailStatus') }}：</span>
              {{ statusLabel }}
            </div>
            <div class="col-6">
              <span class="text-grey-7">{{ $t('agentSettings.evolution.detailCreatedAt') }}：</span>
              {{ formatDate(suggestion.created_at) }}
            </div>
            <div v-if="suggestion.applied_at" class="col-6">
              <span class="text-grey-7">{{ $t('agentSettings.evolution.detailAppliedAt') }}：</span>
              {{ formatDate(suggestion.applied_at) }}
            </div>
          </div>

          <div>
            <div class="text-weight-medium q-mb-xs">{{ $t('agentSettings.evolution.detailContent') }}</div>
            <div class="text-body2 evolution-suggestion-detail__content">{{ suggestion.content }}</div>
          </div>

          <div>
            <div class="text-weight-medium q-mb-xs">{{ $t('agentSettings.evolution.detailDiffPreview') }}</div>
            <pre v-if="suggestion.diff_preview" class="evolution-suggestion-detail__pre">{{
              suggestion.diff_preview
            }}</pre>
            <div v-else class="text-body2 text-grey-6">{{ $t('agentSettings.evolution.detailNoDiff') }}</div>
          </div>
        </q-card-section>
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps :label="$t('agentSettings.evolution.close')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { EvolutionSuggestion } from '../../features/agents/types';

const props = defineProps<{
  open: boolean;
  suggestion: EvolutionSuggestion | null;
  typeLabelOf: (type: string) => string;
  statusLabelOf: (status: string) => string;
}>();

defineEmits<{
  'update:open': [value: boolean];
}>();

const typeLabel = computed(() => (props.suggestion ? props.typeLabelOf(props.suggestion.type) : ''));
const statusLabel = computed(() => (props.suggestion ? props.statusLabelOf(props.suggestion.status) : ''));

function formatDate(iso: string): string {
  if (!iso) return '—';
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}
</script>

<style scoped lang="sass">
.evolution-suggestion-detail__content
  white-space: pre-wrap
  word-break: break-word

.evolution-suggestion-detail__pre
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  border-radius: 6px
  padding: 12px
  font-size: 13px
  line-height: 1.5
  white-space: pre-wrap
  word-break: break-word
  max-height: 400px
  overflow-y: auto
  margin: 0
</style>
