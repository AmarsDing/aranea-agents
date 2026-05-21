<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="tool-dialog-card app-dialog-card app-dialog-card--sm">
      <q-card-section class="row items-center justify-between">
        <div class="text-h6">{{ editing ? "编辑 Agent 覆盖" : "添加 Agent 覆盖" }}</div>
        <q-btn flat dense round icon="close" class="tool-icon-btn" @click="$emit('update:open', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="q-gutter-sm">
        <q-input
          :model-value="form.agent_id"
          label="Agent ID"
          dense
          outlined
          :disable="editing"
          @update:model-value="emitFormPatch({ agent_id: String($event ?? '') })"
        />
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
        <q-btn no-caps unelevated class="tool-primary-btn" label="保存" :loading="saving" @click="$emit('save')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
export type ToolOverrideForm = {
  agent_id: string;
  mode: string;
  enabled: boolean;
  requires_confirmation: boolean;
  config_override_json: string;
};

const props = defineProps<{
  open: boolean;
  form: ToolOverrideForm;
  editing: boolean;
  saving: boolean;
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  save: [];
  "update:form": [value: ToolOverrideForm];
}>();

const modeOptions = [
  { label: "继承 (inherit)", value: "inherit" },
  { label: "允许 (allow)", value: "allow" },
  { label: "拒绝 (deny)", value: "deny" }
];

function emitFormPatch(patch: Partial<ToolOverrideForm>) {
  emit("update:form", { ...props.form, ...patch });
}
</script>

<style scoped lang="sass">
.tool-dialog-card
  border-radius: 22px
  border: 1px solid var(--glass-border)
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

.tool-primary-btn
  background: var(--color-accent)
  color: var(--color-on-accent)

.tool-icon-btn
  color: var(--color-icon-muted)
</style>
