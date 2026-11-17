<template>
  <AppDialog
    :model-value="open"
    :title="t('graphs.hitlDialogTitle')"
    :subtitle="t('graphs.hitlDialogSubtitle', { nodeId: interrupt?.nodeId || '—' })"
    size="sm"
    persistent
    :loading="loading"
    @update:model-value="onDialogUpdate"
  >
    <div class="q-gutter-md">
      <q-banner v-if="interrupt?.prompt" dense rounded class="bg-warning text-dark">
        {{ interrupt.prompt }}
      </q-banner>
      <div v-if="interrupt?.lineageId" class="text-caption app-text-secondary">
        {{ t('graphs.hitlDialogLineageLabel') }}:
        <span class="graph-hitl-dialog__mono">{{ interrupt.lineageId }}</span>
      </div>
      <div v-if="interrupt?.checkpointId" class="text-caption app-text-secondary">
        {{ t('graphs.hitlDialogCheckpointLabel') }}:
        <span class="graph-hitl-dialog__mono">{{ interrupt.checkpointId }}</span>
      </div>
      <q-expansion-item dense :label="t('graphs.hitlDialogAdvanced')" icon="code">
        <q-input
          :model-value="advancedJson"
          dense
          outlined
          autogrow
          type="textarea"
          class="app-field-long"
          :label="t('graphs.hitlDialogResumeValue')"
          :hint="t('graphs.hitlDialogResumeValueHint')"
          @update:model-value="$emit('update:advancedJson', String($event ?? ''))"
        />
      </q-expansion-item>
    </div>
    <template #actions>
      <q-btn flat rounded :label="t('graphs.hitlDialogDismiss')" :disable="loading" @click="$emit('dismiss')" />
      <q-btn
        color="primary"
        rounded
        unelevated
        :label="t('graphs.hitlDialogApprove')"
        :loading="loading"
        @click="$emit('approve')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
import type { GraphInterruptInfo } from '../../features/graph/runtime/graphExecutionProjection';

defineProps<{
  open: boolean;
  interrupt: GraphInterruptInfo | null;
  advancedJson: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  approve: [];
  dismiss: [];
  'update:advancedJson': [value: string];
  'update:open': [value: boolean];
}>();

const { t } = useI18n();

function onDialogUpdate(value: boolean) {
  if (!value) emit('update:open', false);
}
</script>
