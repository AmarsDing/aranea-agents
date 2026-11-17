<template>
  <app-dialog
    :model-value="open"
    :title="$t('agentSettings.evolution.detailTitle')"
    :subtitle="suggestion?.title"
    icon="auto_awesome"
    size="md"
    persistent
    @update:model-value="$emit('update:open', $event)"
  >
    <template v-if="suggestion">
      <div class="row q-col-gutter-sm text-body2 q-mb-md">
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

      <div class="q-mb-md">
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
    </template>
    <template #actions>
      <q-btn flat no-caps :label="$t('agentSettings.evolution.close')" @click="$emit('update:open', false)" />
    </template>
  </app-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppDialog from '../common/AppDialog.vue';
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
