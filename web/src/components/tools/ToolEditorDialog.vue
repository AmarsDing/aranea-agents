<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="tool-dialog-card tool-dialog-card--editor">
      <q-card-section class="row items-center justify-between">
        <div class="text-h6">{{ editingId ? "编辑 Tool" : "新建 Tool" }}</div>
        <q-btn flat dense round icon="close" class="tool-icon-btn" :disable="saving" @click="$emit('update:open', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section>
        <tool-editor-form :form="form" :editing-id="editingId" :errors="errors" :risk-options="riskOptions" />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat no-caps label="取消" :disable="saving" @click="$emit('update:open', false)" />
        <q-btn no-caps unelevated class="tool-primary-btn" label="保存" :loading="saving" @click="$emit('save')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import ToolEditorForm from "./ToolEditorForm.vue";
import type { ToolUpsertInput } from "../../features/tools/types";

defineProps<{
  open: boolean;
  editingId: string;
  saving: boolean;
  form: ToolUpsertInput;
  errors: Record<string, string>;
  riskOptions: { label: string; value: string }[];
}>();

defineEmits<{
  "update:open": [value: boolean];
  save: [];
}>();
</script>

<style scoped lang="sass">
.tool-dialog-card
  max-width: 960px
  width: 92vw
  border-radius: 22px
  border: 1px solid var(--glass-border)
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))
  box-shadow: none

.tool-primary-btn
  background: var(--color-accent)
  color: var(--color-on-accent)

.tool-icon-btn
  color: var(--color-icon-muted)
</style>
