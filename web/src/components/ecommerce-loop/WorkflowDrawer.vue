<script setup lang="ts">
import { ref } from 'vue';
import type { DemoWorkflow, WorkflowActionPayload } from '../../features/ecommerce-loop/types';
import WorkflowPanel from './WorkflowPanel.vue';
defineProps<{ open: boolean; workflow: DemoWorkflow; editing: boolean; shopName: string }>();
const emit = defineEmits<{
  (event: 'close', dirty: boolean): void;
  (event: 'action', payload: WorkflowActionPayload): void;
  (event: 'dirty', value: boolean): void;
  (event: 'hidden'): void;
}>();
const dirty = ref(false);
function onDirty(value: boolean) {
  dirty.value = value;
  emit('dirty', value);
}
</script>

<template>
  <q-dialog
    :model-value="open"
    position="right"
    @update:model-value="(value) => !value && emit('close', dirty)"
    @hide="emit('hidden')"
  >
    <WorkflowPanel
      :workflow="workflow"
      :editing="editing"
      :shop-name="shopName"
      @close="emit('close', $event)"
      @action="emit('action', $event)"
      @dirty="onDirty"
    />
  </q-dialog>
</template>
