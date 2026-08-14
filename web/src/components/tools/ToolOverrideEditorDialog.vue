<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="row items-center justify-between">
        <div class="text-h6">
          {{ editing ? t('toolsPage.overrideDialog.editTitle') : t('toolsPage.overrideDialog.addTitle') }}
        </div>
        <q-btn
          flat
          dense
          round
          icon="close"
          class="app-dialog-icon-btn"
          :aria-label="t('common.close')"
          @click="$emit('update:open', false)"
        />
      </q-card-section>
      <q-separator />
      <q-card-section class="q-gutter-sm">
        <q-select
          :model-value="form.agent_id"
          :label="t('toolsPage.overrideDialog.agentLabel')"
          dense
          outlined
          :options="agentOptions"
          emit-value
          map-options
          :disable="editing"
          :loading="agentsLoading"
          @update:model-value="emitFormPatch({ agent_id: String($event ?? '') })"
        />
        <q-select
          :model-value="form.mode"
          :label="t('toolsPage.overrideDialog.modeLabel')"
          dense
          outlined
          :options="modeOpts"
          emit-value
          map-options
          @update:model-value="emitFormPatch({ mode: String($event ?? 'inherit') })"
        />
        <q-toggle
          :model-value="form.requires_confirmation"
          :label="t('toolsPage.overrideDialog.confirmLabel')"
          @update:model-value="emitFormPatch({ requires_confirmation: Boolean($event) })"
        />
        <div class="text-caption text-grey-7">
          {{ t('toolsPage.overrideDialog.confirmHint') }}
        </div>
        <q-input
          :model-value="form.config_override_json"
          :label="t('toolsPage.overrideDialog.configLabel')"
          type="textarea"
          dense
          outlined
          autogrow
          :error="Boolean(configJsonError)"
          :error-message="configJsonError"
          @update:model-value="onConfigJsonInput(String($event ?? '{}'))"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps :label="t('common.cancel')" @click="$emit('update:open', false)" />
        <q-btn
          no-caps
          unelevated
          class="app-dialog-accent-btn"
          :label="t('common.save')"
          :loading="saving"
          :disable="Boolean(configJsonError)"
          @click="$emit('save')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { overrideModeOptions } from './toolUi';
import type { ToolOverrideForm } from '../../stores/tools/toolDetail';

const props = defineProps<{
  open: boolean;
  form: ToolOverrideForm;
  editing: boolean;
  saving: boolean;
  agentOptions: { label: string; value: string }[];
  agentsLoading?: boolean;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  save: [];
  'update:form': [value: ToolOverrideForm];
}>();

const { t } = useI18n();
const modeOpts = computed(() => overrideModeOptions());

const configJsonError = ref('');

function emitFormPatch(patch: Partial<ToolOverrideForm>) {
  emit('update:form', { ...props.form, ...patch });
}

function onConfigJsonInput(val: string) {
  try {
    JSON.parse(val || '{}');
    configJsonError.value = '';
  } catch (err) {
    configJsonError.value = err instanceof Error ? err.message : t('toolsPage.invalidJsonFallback');
  }
  emitFormPatch({ config_override_json: val });
}
</script>
