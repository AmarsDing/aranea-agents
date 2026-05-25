<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog tool-dialog-card--editor">
      <q-card-section class="app-glass-dialog__head row items-center justify-between no-wrap">
        <div class="app-glass-dialog__title">{{ editingId ? "编辑 Tool" : "新建 Tool" }}</div>
        <q-btn flat dense round icon="close" class="app-registry-icon-btn" :disable="saving" @click="$emit('update:open', false)" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body">
          <tool-editor-form :form="form" :editing-id="editingId" :errors="errors" :risk-options="riskOptions" />
        </q-card-section>
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn flat no-caps label="取消" :disable="saving" @click="$emit('update:open', false)" />
        <q-btn no-caps unelevated class="app-registry-primary-btn" label="保存" :loading="saving" @click="$emit('save')" />
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
