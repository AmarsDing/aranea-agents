<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="text-h6">{{ editing ? '编辑工具覆盖' : '添加工具覆盖' }}</q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body q-gutter-sm q-pt-none">
        <div class="text-body2 text-weight-medium">{{ row?.display_name }} ({{ row?.tool_key }})</div>
        <q-select
          :model-value="form.mode"
          label="模式"
          dense
          outlined
          :options="modeOptions"
          emit-value
          map-options
          @update:model-value="emitFormPatch({ mode: String($event ?? 'inherit') })"
        />
        <q-toggle
          :model-value="form.enabled"
          label="启用"
          @update:model-value="emitFormPatch({ enabled: Boolean($event) })"
        />
        <q-toggle
          :model-value="form.requires_confirmation"
          label="需要确认"
          @update:model-value="emitFormPatch({ requires_confirmation: Boolean($event) })"
        />
        <q-input
          :model-value="form.config_override_json"
          label="配置覆盖 JSON"
          type="textarea"
          dense
          outlined
          autogrow
          @update:model-value="emitFormPatch({ config_override_json: String($event ?? '{}') })"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" @click="$emit('update:open', false)" />
        <q-btn no-caps unelevated color="primary" label="保存" :loading="saving" @click="$emit('save')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { AgentToolOverrideForm, AgentToolOverrideRow } from '../../features/agents/useAgentToolOverrides';

const props = defineProps<{
  open: boolean;
  editing: boolean;
  saving: boolean;
  row: AgentToolOverrideRow | null;
  form: AgentToolOverrideForm;
  modeOptions: { label: string; value: string }[];
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  save: [];
  'update:form': [value: AgentToolOverrideForm];
}>();

function emitFormPatch(patch: Partial<AgentToolOverrideForm>) {
  emit('update:form', { ...props.form, ...patch });
}
</script>
