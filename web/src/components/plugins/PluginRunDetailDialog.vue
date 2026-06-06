<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">运行详情</div>
        <q-btn v-close-popup flat dense round icon="close" aria-label="关闭详情" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body q-pt-none">
        <pre class="plugin-run-detail app-code-block">{{ text }}</pre>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat rounded no-caps label="关闭" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{
  open: boolean;
  text: string;
}>();

defineEmits<{
  'update:open': [value: boolean];
}>();
</script>
